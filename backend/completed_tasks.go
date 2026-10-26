package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	raintorrent "github.com/cenkalti/rain/torrent"
)

// Only receipts created by a successful archive commit can remove a task.
func processCompletedTasks() {
	rows, err := db.Query("SELECT item_id,downloader,task_id,submitted_at,connection FROM completed_tasks")
	if err != nil {
		return
	}
	type receipt struct {
		taskRef
		connection string
	}
	var pending []receipt
	for rows.Next() {
		var r receipt
		if rows.Scan(&r.ID, &r.Downloader, &r.TaskID, &r.SubmittedAt, &r.connection) == nil {
			pending = append(pending, r)
		}
	}
	rows.Close()
	for _, r := range pending {
		func() {
			l, release := downloadLock(r.ID)
			defer release()
			l.operation.Lock()
			defer l.operation.Unlock()
			animeCleanupMu.RLock()
			defer animeCleanupMu.RUnlock()
			item, err := readControlledItem(r.ID)
			if err != nil || item.taskRef != r.taskRef {
				var action *downloadActionError
				if err == nil || (errors.As(err, &action) && action.Code == 404) {
					db.Exec("DELETE FROM completed_tasks WHERE item_id=?", r.ID)
				}
				return
			}
			if item.Status != "已保存" || r.connection != cleanupConnection(r.Downloader) {
				return
			}
			var shared int
			var archive string
			if db.QueryRow("SELECT COUNT(*) FROM items WHERE id<>? AND downloader=? AND task_id=?", r.ID, r.Downloader, r.TaskID).Scan(&shared) != nil || shared != 0 {
				return
			}
			if db.QueryRow("SELECT COALESCE(file_path,'') FROM items WHERE id=?", r.ID).Scan(&archive) != nil {
				return
			}
			if _, err := os.Stat(archive); err != nil {
				return
			}
			if done, err := removeFinishedTask(r.taskRef); err == nil && done {
				db.Exec("DELETE FROM completed_tasks WHERE item_id=?", r.ID)
			}
		}()
	}
}

func removeFinishedTask(t taskRef) (bool, error) {
	switch t.Downloader {
	case "native":
		if nativeTaskHash(t.TaskID) == "" {
			return false, fmt.Errorf("内置 BT 任务标识无效")
		}
		done := false
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := nativeDownloads.withSession(ctx, func(s *raintorrent.Session) error {
			torrent := s.GetTorrent(t.TaskID)
			if torrent == nil {
				if !s.HasFreePort() {
					return nil
				}
				var err error
				torrent, err = s.LoadTorrent(t.TaskID)
				if err != nil {
					return err
				}
				if torrent == nil {
					done = true // A previous deletion may have succeeded before receipt removal.
					return nil
				}
				if err := nativeStartTorrent(torrent); err != nil {
					return err
				}
			}
			stats := torrent.Stats()
			if stats.Error != nil || !nativeFinished(stats) || stats.Bytes.Uploaded < stats.Bytes.Total {
				return nil
			}
			if err := s.ForgetTorrent(t.TaskID); err != nil {
				return err
			}
			done = true
			return nil
		})
		return done, err
	case "aria2":
		if !aria2CleanupGID.MatchString(t.TaskID) {
			return false, fmt.Errorf("Aria2 任务标识无效")
		}
		var st aria2Status
		if err := aria2RPC("aria2.tellStatus", []any{t.TaskID, []string{"status", "followedBy"}}, &st); err != nil {
			return false, err
		}
		if st.Status != "complete" || len(st.FollowedBy) != 0 {
			return false, nil
		}
		return true, aria2RPC("aria2.removeDownloadResult", []any{t.TaskID}, nil)
	case "qbittorrent":
		if !qbitControlHash.MatchString(t.TaskID) {
			return false, fmt.Errorf("qBittorrent 任务标识无效")
		}
		c, err := qbitLogin()
		if err != nil {
			return false, err
		}
		found, err := c.info(url.Values{"hashes": {t.TaskID}})
		if err != nil {
			return false, err
		}
		if len(found) == 0 {
			return true, nil
		}
		if len(found) != 1 || !strings.EqualFold(found[0].Hash, t.TaskID) {
			return false, fmt.Errorf("无法确认唯一的 qBittorrent 任务")
		}
		st := found[0]
		if st.Progress < 1 || (st.State != "pausedUP" && st.State != "stoppedUP") {
			return false, nil
		}
		// A manually paused upload is not proof that the seeding target was met.
		if st.RatioLimit == nil || st.SeedingTimeLimit == nil {
			return false, nil
		}
		ratio, minutes := *st.RatioLimit, *st.SeedingTimeLimit
		if ratio == -2 || minutes == -2 {
			res, err := c.http.Get(c.config.URL + "/api/v2/app/preferences")
			if err != nil {
				return false, err
			}
			defer res.Body.Close()
			var prefs struct {
				RatioEnabled bool    `json:"max_ratio_enabled"`
				Ratio        float64 `json:"max_ratio"`
				TimeEnabled  bool    `json:"max_seeding_time_enabled"`
				Minutes      int64   `json:"max_seeding_time"`
			}
			if res.StatusCode != 200 {
				return false, fmt.Errorf("qBittorrent 做种策略查询失败")
			}
			if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&prefs); err != nil {
				return false, err
			}
			if ratio == -2 {
				ratio = -1
				if prefs.RatioEnabled {
					ratio = prefs.Ratio
				}
			}
			if minutes == -2 {
				minutes = -1
				if prefs.TimeEnabled {
					minutes = prefs.Minutes
				}
			}
		}
		if !(ratio >= 0 && st.Ratio >= ratio) && !(minutes >= 0 && st.SeedingTime >= minutes*60) {
			return false, nil
		}
		return true, c.remove(t.TaskID) // deleteFiles=false is enforced by the client.
	case "transmission":
		id, err := strconv.Atoi(t.TaskID)
		if err != nil || id < 0 {
			return false, fmt.Errorf("Transmission 任务标识无效")
		}
		c := &transmissionClient{}
		var out struct {
			Result    string `json:"result"`
			Arguments struct {
				Torrents []struct {
					ID          int     `json:"id"`
					Status      int     `json:"status"`
					PercentDone float64 `json:"percentDone"`
					IsFinished  bool    `json:"isFinished"`
				} `json:"torrents"`
			} `json:"arguments"`
		}
		if err := c.call(map[string]any{"method": "torrent-get", "arguments": map[string]any{"ids": []int{id}, "fields": []string{"id", "status", "percentDone", "isFinished"}}}, &out); err != nil {
			return false, err
		}
		if out.Result != "success" {
			return false, fmt.Errorf("Transmission 查询失败")
		}
		if len(out.Arguments.Torrents) == 0 {
			return true, nil
		}
		if len(out.Arguments.Torrents) != 1 || out.Arguments.Torrents[0].ID != id {
			return false, fmt.Errorf("无法确认唯一的 Transmission 任务")
		}
		st := out.Arguments.Torrents[0]
		if st.Status != 0 || st.PercentDone < 1 || !st.IsFinished {
			return false, nil
		}
		var result struct {
			Result string `json:"result"`
		}
		if err := c.call(map[string]any{"method": "torrent-remove", "arguments": map[string]any{"ids": []int{id}, "delete-local-data": false}}, &result); err != nil {
			return false, err
		}
		if result.Result != "success" {
			return false, fmt.Errorf("Transmission 未确认删除任务")
		}
		return true, nil
	}
	return false, nil
}

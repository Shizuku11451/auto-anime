package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	raintorrent "github.com/cenkalti/rain/torrent"
)

// Only receipts created by a successful archive commit can remove a task.
func processCompletedTasks() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	processCompletedTasksContext(ctx)
}

func processCompletedTasksContext(ctx context.Context) {
	rows, err := db.Query("SELECT item_id,downloader,task_id,submitted_at,connection FROM completed_tasks ORDER BY item_id")
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
	cleaner := completedTaskCleaner{}
	broken := make(map[string]bool)
	// Rotate the snapshot so unfinished or slow receipts cannot starve later ones.
	start := 0
	for start < len(pending) && pending[start].ID <= completedTaskCursor {
		start++
	}
	if len(pending) > 0 {
		completedTaskCursor = pending[start%len(pending)].ID
	}
	for i := 0; i < len(pending) && ctx.Err() == nil; i++ {
		r := pending[(start+i)%len(pending)]
		if broken[r.Downloader] {
			continue
		}
		func() {
			l, release := downloadLock(r.ID)
			defer release()
			if !l.operation.TryLock() {
				return
			}
			defer l.operation.Unlock()
			if !animeCleanupMu.TryRLock() {
				return
			}
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
			if done, err := cleaner.remove(ctx, r.taskRef); err != nil {
				broken[r.Downloader] = true
			} else if done {
				db.Exec("DELETE FROM completed_tasks WHERE item_id=?", r.ID)
			}
		}()
	}
}

// The downloader poller runs cleanup serially; the cursor survives its rounds.
var completedTaskCursor int

type completedTaskCleaner struct {
	qbit         *qbitClient
	transmission transmissionClient
}

func removeFinishedTask(t taskRef) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return (&completedTaskCleaner{}).remove(ctx, t)
}

func (c *completedTaskCleaner) remove(ctx context.Context, t taskRef) (bool, error) {
	switch t.Downloader {
	case "native":
		if nativeTaskHash(t.TaskID) == "" {
			return false, fmt.Errorf("内置 BT 任务标识无效")
		}
		done := false
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
		if err := aria2RPCContext(ctx, "aria2.tellStatus", []any{t.TaskID, []string{"status", "followedBy"}}, &st); err != nil {
			return false, err
		}
		if st.Status != "complete" || len(st.FollowedBy) != 0 {
			return false, nil
		}
		return true, aria2RPCContext(ctx, "aria2.removeDownloadResult", []any{t.TaskID}, nil)
	case "qbittorrent":
		if !qbitControlHash.MatchString(t.TaskID) {
			return false, fmt.Errorf("qBittorrent 任务标识无效")
		}
		cfg := runtimeConfig()
		config := qbitConnection{cfg.qbitURL, cfg.qbitUser, cfg.qbitPassword, cfg.qbitSavePath, cfg.qbitLocalDir}
		config.URL = strings.TrimRight(config.URL, "/")
		if c.qbit == nil || c.qbit.config != config {
			var err error
			c.qbit, err = loginQBittorrentContext(ctx, config, 10*time.Second)
			if err != nil {
				return false, err
			}
		}
		client := c.qbit
		found, err := client.infoContext(ctx, url.Values{"hashes": {t.TaskID}})
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
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, client.config.URL+"/api/v2/app/preferences", nil)
			if err != nil {
				return false, err
			}
			res, err := client.http.Do(req)
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
		return true, client.removeContext(ctx, t.TaskID) // deleteFiles=false is enforced by the client.
	case "transmission":
		id, err := strconv.Atoi(t.TaskID)
		if err != nil || id < 0 {
			return false, fmt.Errorf("Transmission 任务标识无效")
		}
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
		if err := c.transmission.callContext(ctx, map[string]any{"method": "torrent-get", "arguments": map[string]any{"ids": []int{id}, "fields": []string{"id", "status", "percentDone", "isFinished"}}}, &out); err != nil {
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
		if err := c.transmission.callContext(ctx, map[string]any{"method": "torrent-remove", "arguments": map[string]any{"ids": []int{id}, "delete-local-data": false}}, &result); err != nil {
			return false, err
		}
		if result.Result != "success" {
			return false, fmt.Errorf("Transmission 未确认删除任务")
		}
		return true, nil
	}
	return false, nil
}

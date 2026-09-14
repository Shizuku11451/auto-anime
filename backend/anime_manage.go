package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func animeCreate(w http.ResponseWriter, r *http.Request) {
	var in animeUpdateInput
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in) != nil {
		http.Error(w, "请求参数无效", 400)
		return
	}
	a := animeRow{Title: sanitizeText(in.Title, titleMaxLength), Season: 1}
	if a.Title == "" {
		http.Error(w, "请填写番剧名称", 400)
		return
	}
	if in.Season != nil {
		a.Season = *in.Season
	}
	if a.Season < 1 || a.Season > 999 {
		http.Error(w, "新建番剧的默认季号必须是 1-999", 400)
		return
	}
	if in.StorageDir != nil {
		var err error
		a.StorageDir, err = sanitizeStorageDir(*in.StorageDir)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
	}
	if in.SearchName != nil {
		a.SearchName = sanitizeText(*in.SearchName, searchNameLimit)
	}
	if in.IncludeRules != nil {
		a.IncludeRules = sanitizeText(*in.IncludeRules, rulesMaxLength)
	}
	if in.ExcludeRules != nil {
		a.ExcludeRules = sanitizeText(*in.ExcludeRules, rulesMaxLength)
	}
	animeCleanupMu.Lock()
	defer animeCleanupMu.Unlock()
	a.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	result, err := db.Exec(`INSERT INTO animes(title,search_name,season,storage_dir,include_rules,exclude_rules,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(title) DO NOTHING`, a.Title, a.SearchName, a.Season, a.StorageDir, a.IncludeRules, a.ExcludeRules, a.UpdatedAt, a.UpdatedAt)
	if err != nil {
		http.Error(w, "创建失败", 500)
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		http.Error(w, "已存在同名番剧", 409)
		return
	}
	a.ID, _ = result.LastInsertId()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	jsonOut(w, a)
}

type animeManageInput struct {
	Action      string  `json:"action"`
	IDs         []int64 `json:"ids"`
	TargetID    int64   `json:"target_id"`
	DeleteItems bool    `json:"delete_items"`
}

type animeManageResult struct {
	OK         bool  `json:"ok"`
	AnimeCount int   `json:"anime_count"`
	ItemCount  int64 `json:"item_count"`
	FeedCount  int64 `json:"feed_count"`
}

func animeManageAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "方法不支持", 405)
		return
	}
	var in animeManageInput
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in) != nil {
		http.Error(w, "请求参数无效", 400)
		return
	}
	result, status, err := manageAnime(in)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	jsonOut(w, result)
}

// The caller holds animeCleanupMu exclusively, protecting RSS sync and edits.
// Each batch is atomic; media and downloader files are never removed or moved.
func manageAnime(in animeManageInput) (animeManageResult, int, error) {
	out := animeManageResult{}
	bad := func(code int, message string) (animeManageResult, int, error) {
		return out, code, fmt.Errorf("%s", message)
	}
	if (in.Action != "delete" && in.Action != "merge") || len(in.IDs) == 0 || len(in.IDs) > 500 {
		return bad(400, "请选择 1-500 部番剧及有效操作")
	}
	seen := map[int64]bool{}
	for _, id := range in.IDs {
		if id <= 0 || seen[id] || (in.Action == "merge" && id == in.TargetID) {
			return bad(400, "番剧选择无效，合并目标不能包含在来源中")
		}
		seen[id] = true
	}
	tx, err := db.Begin()
	if err != nil {
		return bad(500, "操作失败，请重试")
	}
	defer tx.Rollback()
	read := func(id int64) (animeRow, error) {
		var a animeRow
		err := tx.QueryRow("SELECT id,title,COALESCE(season,0) FROM animes WHERE id=?", id).Scan(&a.ID, &a.Title, &a.Season)
		return a, err
	}
	var target animeRow
	if in.Action == "merge" {
		target, err = read(in.TargetID)
		if err == sql.ErrNoRows {
			return bad(404, "目标番剧不存在，请刷新后重试")
		}
		if err != nil {
			return bad(500, "读取目标番剧失败")
		}
	}
	for _, id := range in.IDs {
		a, err := read(id)
		if err == sql.ErrNoRows {
			return bad(404, "所选番剧已不存在，请刷新后重试")
		}
		if err != nil {
			return bad(500, "读取番剧失败")
		}
		// Include legacy title-only associations without stealing another anime's items.
		const related = "(anime_id=? OR (COALESCE(anime_id,0)=0 AND anime_title=?))"
		var active, count int
		if err = tx.QueryRow("SELECT COUNT(*),COALESCE(SUM(CASE WHEN status IN ('等待下载','等待继续',"+activeTaskStatusSQL+") THEN 1 ELSE 0 END),0) FROM items WHERE "+related, a.ID, a.Title).Scan(&count, &active); err != nil {
			return bad(500, "读取资源失败")
		}
		if active > 0 {
			return bad(409, "《"+a.Title+"》存在排队或下载中的资源，请等待下载完成后再操作")
		}
		if in.Action == "delete" && count > 0 && !in.DeleteItems {
			return bad(409, "该番剧仍有资源，请明确选择同时删除资源记录")
		}
		if in.Action == "delete" && in.DeleteItems {
			rows, queryErr := tx.Query("SELECT downloader,task_id FROM items WHERE "+related+" AND COALESCE(task_id,'')<>''", a.ID, a.Title)
			if queryErr == nil {
				for rows.Next() {
					var downloader, taskID string
					if rows.Scan(&downloader, &taskID) == nil {
						removeDownloaderTask(downloader, taskID)
					}
				}
				rows.Close()
			}
		}
		var itemResult, feedResult sql.Result
		if in.Action == "merge" {
			season := a.Season
			if season <= 0 {
				season = 1
			}
			itemResult, err = tx.Exec("UPDATE items SET anime_id=?,anime_title=?,season=CASE WHEN season>0 THEN season ELSE ? END,manual=1 WHERE "+related, target.ID, target.Title, season, a.ID, a.Title)
			if err == nil {
				feedResult, err = tx.Exec("UPDATE feeds SET target_title=? WHERE target_title=?", target.Title, a.Title)
			}
		} else {
			itemResult, err = tx.Exec("DELETE FROM items WHERE "+related, a.ID, a.Title)
			if err == nil {
				feedResult, err = tx.Exec("UPDATE feeds SET enabled=0,target_title='' WHERE target_title=?", a.Title)
			}
		}
		if err != nil {
			return bad(500, "更新资源或订阅失败，操作已回滚")
		}
		n, _ := itemResult.RowsAffected()
		out.ItemCount += n
		n, _ = feedResult.RowsAffected()
		out.FeedCount += n
		for _, query := range []string{"DELETE FROM anime_meta WHERE anime_title=?", "DELETE FROM anime_storage WHERE anime_title=?", "DELETE FROM animes WHERE title=?"} {
			if _, err = tx.Exec(query, a.Title); err != nil {
				return bad(500, "更新番剧失败，操作已回滚")
			}
		}
		out.AnimeCount++
	}
	if in.Action == "merge" {
		if _, err = tx.Exec("UPDATE animes SET updated_at=? WHERE id=?", time.Now().UTC().Format(time.RFC3339), target.ID); err != nil {
			return bad(500, "合并失败，操作已回滚")
		}
	}
	if err = tx.Commit(); err != nil {
		return bad(500, "提交失败，操作已回滚")
	}
	out.OK = true
	return out, 200, nil
}

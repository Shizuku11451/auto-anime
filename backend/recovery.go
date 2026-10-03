package main

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
)

// Run once, before HTTP handlers or the scheduler start. A direct download
// marked downloading has no persistent task identity, so its outcome is
// uncertain and it requires an explicit retry. External downloader tasks carry
// a task ID, so the poller simply resumes tracking them.
func recoverDownloads(database *sql.DB) ([]int, error) {
	legacyTasks, err := legacyEmbeddedAria2Tasks(nil)
	if err != nil {
		return nil, err
	}
	tx, err := database.BeginTx(context.Background(), nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = retireLegacyAria2Tasks(tx, legacyTasks); err != nil {
		return nil, err
	}
	if _, err = tx.Exec("UPDATE items SET status='下载失败' WHERE status='下载中' AND COALESCE(task_id,'')='' AND COALESCE(downloader,'') NOT IN ('builtin','qbittorrent')"); err != nil {
		return nil, err
	}
	rows, err := tx.Query(`SELECT items.id FROM items LEFT JOIN feeds ON feeds.id=items.feed_id
		WHERE items.status='等待继续' OR ((feeds.id IS NULL OR feeds.enabled=1) AND
		(items.status='等待下载' OR (items.status='待下载' AND feeds.auto_download=1))) ORDER BY items.id`)
	if err != nil {
		return nil, err
	}
	var ids []int
	for rows.Next() {
		var id int
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	// Paused subscriptions remain manually retryable. Never leave a persisted
	// queue state with no worker capable of claiming it.
	if _, err = tx.Exec("UPDATE items SET status='待下载' WHERE status='等待下载'"); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return ids, nil
}

func resumeDownloads(ids []int) {
	jobs := make(chan int)
	var workers sync.WaitGroup
	for i := 0; i < maximumConcurrentDownloads; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for id := range jobs {
				var status string
				if db.QueryRow("SELECT status FROM items WHERE id=?", id).Scan(&status) == nil && status == "等待继续" {
					runQueuedDownload(id)
				} else {
					downloadItem(id)
				}
			}
		}()
	}
	for _, id := range ids {
		jobs <- id
	}
	close(jobs)
	workers.Wait()
}

func validateDownloadTarget() error {
	cfg := runtimeConfig()
	if cfg.downloadMode != "builtin" && cfg.downloadMode != "direct" && cfg.downloadMode != "aria2" && cfg.downloadMode != "qbittorrent" && cfg.downloadMode != "transmission" {
		return fmt.Errorf("未知下载模式")
	}
	if (cfg.downloadMode == "aria2" && cfg.aria2URL == "") || (cfg.downloadMode == "qbittorrent" && cfg.qbitURL == "") || (cfg.downloadMode == "transmission" && cfg.transmissionURL == "") {
		return fmt.Errorf("未配置下载器地址")
	}
	return nil
}

package main

import (
	"io"
	"sync"
	"sync/atomic"
	"time"
)

const maximumConcurrentDownloads = 32

var configuredDownloads atomic.Int64
var downloadClaimMu sync.Mutex

func downloadLimit() int {
	if n := configuredDownloads.Load(); n > 0 {
		return int(n)
	}
	return 2
}

// Count persisted external tasks as well as direct transfers. A submitted BT
// job keeps its slot through completion, including after an application restart.
func claimDownload(id int) (claimed, waiting bool, err error) {
	animeCleanupMu.RLock()
	defer animeCleanupMu.RUnlock()
	downloadClaimMu.Lock()
	defer downloadClaimMu.Unlock()
	var status string
	if err = db.QueryRow("SELECT status FROM items WHERE id=?", id).Scan(&status); err != nil {
		return
	}
	if status != "等待下载" && status != "等待继续" {
		return
	}
	var active int
	if err = db.QueryRow("SELECT COUNT(*) FROM items WHERE status IN (" + activeTaskStatusSQL + ")").Scan(&active); err != nil {
		return
	}
	if active >= downloadLimit() {
		return false, true, nil
	}
	query := "UPDATE items SET status='下载中',downloader='',task_id='',progress=0,task_submitted_at=0 WHERE id=? AND status='等待下载'"
	if status == "等待继续" {
		query = "UPDATE items SET status='继续中' WHERE id=? AND status='等待继续'"
	}
	result, err := db.Exec(query, id)
	if err != nil {
		return false, false, err
	}
	n, err := result.RowsAffected()
	return n == 1, false, err
}

type transferSample struct {
	Speed    int64
	Progress int
	Updated  time.Time
	TTL      time.Duration
}

var transferStats = struct {
	sync.Mutex
	items map[int]transferSample
}{items: make(map[int]transferSample)}

func setTransfer(id int, speed int64, progress int, ttl time.Duration) {
	transferStats.Lock()
	defer transferStats.Unlock()
	now := time.Now()
	for key, sample := range transferStats.items {
		if now.Sub(sample.Updated) > sample.TTL {
			delete(transferStats.items, key)
		}
	}
	transferStats.items[id] = transferSample{max(speed, 0), min(max(progress, 0), 100), now, ttl}
}

func clearTransfer(id int) {
	transferStats.Lock()
	delete(transferStats.items, id)
	transferStats.Unlock()
}

func transferSnapshot(id int, status string, progress int) (int64, int) {
	if status != "下载中" {
		return 0, progress
	}
	transferStats.Lock()
	sample, ok := transferStats.items[id]
	transferStats.Unlock()
	if !ok || time.Since(sample.Updated) > sample.TTL {
		return 0, progress
	}
	return sample.Speed, sample.Progress
}

type transferReader struct {
	reader                       io.Reader
	id                           int
	total, received, windowBytes int64
	windowStart                  time.Time
}

func (r *transferReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.received += int64(n)
	r.windowBytes += int64(n)
	elapsed := time.Since(r.windowStart)
	if elapsed >= 500*time.Millisecond {
		progress := 0
		if r.total > 0 {
			progress = int(float64(r.received) / float64(r.total) * 100)
		}
		setTransfer(r.id, int64(float64(r.windowBytes)/elapsed.Seconds()), progress, 3*time.Second)
		r.windowStart = time.Now()
		r.windowBytes = 0
	}
	return n, err
}

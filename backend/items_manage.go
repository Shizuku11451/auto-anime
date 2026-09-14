package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

func itemActionError(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	var actionErr *downloadActionError
	if errors.As(err, &actionErr) {
		code = actionErr.Code
	}
	http.Error(w, err.Error(), code)
}

func itemsEntryAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	rel := strings.TrimPrefix(r.URL.Path, "/api/items/")
	if rel == "manage" {
		manageItemsAPI(w, r)
		return
	}
	parts := strings.Split(rel, "/")
	id, err := strconv.Atoi(parts[0])
	if err != nil || id <= 0 || len(parts) > 2 {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodPatch:
			correctItem(w, r, int64(id))
		case http.MethodDelete:
			if err := deleteItemRecords([]int{id}); err != nil {
				itemActionError(w, err)
				return
			}
			jsonOut(w, map[string]bool{"ok": true})
		default:
			http.Error(w, "方法不支持", 405)
		}
		return
	}
	if parts[1] == "stream" {
		streamItem(w, r, int64(id))
		return
	}
	if parts[1] != "download" && parts[1] != "pause" && parts[1] != "resume" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "方法不支持", 405)
		return
	}
	changed, err := controlItemDownload(id, parts[1])
	if err != nil {
		itemActionError(w, err)
		return
	}
	jsonOut(w, map[string]any{"ok": true, "changed": changed})
}

func controlItemDownload(id int, action string) (bool, error) {
	if action == "pause" {
		return pauseItemDownload(id)
	}
	queued, err := queueItemDownload(id, action == "resume")
	if queued && err == nil {
		go runQueuedDownload(id)
	}
	return queued, err
}

type itemBatchResult struct {
	OK      bool                `json:"ok"`
	Count   int                 `json:"count"`
	Skipped int                 `json:"skipped"`
	Failed  []itemActionFailure `json:"failed"`
}

type itemActionFailure struct {
	ID    int    `json:"id"`
	Error string `json:"error"`
}

func manageItemsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "方法不支持", 405)
		return
	}
	var in struct {
		Action string `json:"action"`
		IDs    []int  `json:"ids"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	if decoder.Decode(&in) != nil || decoder.Decode(new(any)) != io.EOF || len(in.IDs) == 0 || len(in.IDs) > 500 {
		http.Error(w, "请选择 1-500 条记录及有效操作", 400)
		return
	}
	if in.Action != "download" && in.Action != "pause" && in.Action != "resume" && in.Action != "delete" {
		http.Error(w, "批量操作无效", 400)
		return
	}
	seen := map[int]bool{}
	for _, id := range in.IDs {
		if id <= 0 || seen[id] {
			http.Error(w, "条目标识无效", 400)
			return
		}
		seen[id] = true
	}
	out := itemBatchResult{OK: true, Failed: []itemActionFailure{}}
	if in.Action == "delete" {
		if err := deleteItemRecords(in.IDs); err != nil {
			itemActionError(w, err)
			return
		}
		out.Count = len(in.IDs)
		jsonOut(w, out)
		return
	}
	// Remote controls cannot be atomic across downloaders. Report every result
	// and bound network concurrency instead of claiming the entire batch worked.
	jobs := make(chan int)
	var workers sync.WaitGroup
	var resultMu sync.Mutex
	for range min(4, len(in.IDs)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for id := range jobs {
				changed, err := controlItemDownload(id, in.Action)
				resultMu.Lock()
				if err != nil {
					out.OK = false
					out.Failed = append(out.Failed, itemActionFailure{ID: id, Error: err.Error()})
				} else if changed {
					out.Count++
				} else {
					out.Skipped++
				}
				resultMu.Unlock()
			}
		}()
	}
	for _, id := range in.IDs {
		jobs <- id
	}
	close(jobs)
	workers.Wait()
	sort.Slice(out.Failed, func(i, j int) bool { return out.Failed[i].ID < out.Failed[j].ID })
	jsonOut(w, out)
}

// Finish in-flight work before removing its record. In particular, direct
// transfers must close and clean their temporary files before IDs can be reused.
// Delete records atomically; downloader cleanup still preserves source files.
func deleteItemRecords(ids []int) error {
	ordered := append([]int(nil), ids...)
	sort.Ints(ordered)
	for _, id := range ordered {
		l, release := downloadLock(id)
		defer release()
		endInterrupt := l.interrupt()
		l.operation.Lock()
		defer func() {
			endInterrupt()
			l.operation.Unlock()
		}()
	}
	animeCleanupMu.RLock()
	defer animeCleanupMu.RUnlock()
	committed := false
	defer func() {
		if !committed {
			// A failed deletion must not leave cancelled direct I/O looking active.
			for _, id := range ordered {
				db.Exec("UPDATE items SET status='已暂停',progress=0 WHERE id=? AND status='下载中' AND COALESCE(downloader,'')=''", id)
			}
		}
	}()
	tasks := make([]controlledItem, 0, len(ordered))
	for _, id := range ordered {
		item, err := readControlledItem(id)
		if err != nil {
			return err
		}
		tasks = append(tasks, item)
	}
	tx, err := db.Begin()
	if err != nil {
		return actionError(500, "移除失败，请重试")
	}
	defer tx.Rollback()
	for _, id := range ordered {
		if _, err = tx.Exec("DELETE FROM items WHERE id=?", id); err != nil {
			return actionError(500, "移除失败，记录已保留；中断的直链下载可点击继续")
		}
	}
	if err = tx.Commit(); err != nil {
		return actionError(500, "移除失败，记录已保留；中断的直链下载可点击继续")
	}
	committed = true
	for _, item := range tasks {
		clearTransfer(item.ID)
		removeDownloaderTask(item.Downloader, item.TaskID)
	}
	return nil
}

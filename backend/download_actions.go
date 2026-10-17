package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Serialize submission, polling, controls and final archive writes per item.
// A direct transfer holds operation while streaming, but can be interrupted
// without that lock. worker prevents a resumed transfer from overlapping the
// old transfer's cleanup. References keep the registry bounded by live work.
type itemDownloadLock struct {
	operation  sync.Mutex
	worker     sync.Mutex
	control    sync.Mutex
	cancel     context.CancelFunc
	interrupts int
	refs       int
}

var itemDownloadLocks = struct {
	sync.Mutex
	items map[int]*itemDownloadLock
}{items: make(map[int]*itemDownloadLock)}

func downloadLock(id int) (*itemDownloadLock, func()) {
	itemDownloadLocks.Lock()
	l := itemDownloadLocks.items[id]
	if l == nil {
		l = &itemDownloadLock{}
		itemDownloadLocks.items[id] = l
	}
	l.refs++
	itemDownloadLocks.Unlock()
	return l, func() {
		itemDownloadLocks.Lock()
		l.refs--
		if l.refs == 0 {
			delete(itemDownloadLocks.items, id)
		}
		itemDownloadLocks.Unlock()
	}
}

func (l *itemDownloadLock) interrupt() func() {
	l.control.Lock()
	l.interrupts++
	if l.cancel != nil {
		l.cancel()
	}
	l.control.Unlock()
	return func() {
		l.control.Lock()
		l.interrupts--
		l.control.Unlock()
	}
}

func (l *itemDownloadLock) transferContext() (context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	l.control.Lock()
	l.cancel = cancel
	if l.interrupts > 0 {
		cancel()
	}
	l.control.Unlock()
	return ctx, func() {
		l.control.Lock()
		l.cancel = nil
		cancel()
		l.control.Unlock()
	}
}

type downloadActionError struct {
	Code    int
	Message string
}

func (e *downloadActionError) Error() string { return e.Message }

func actionError(code int, message string) error {
	return &downloadActionError{Code: code, Message: message}
}

type controlledItem struct {
	taskRef
	Status   string
	Progress int
}

func readControlledItem(id int) (controlledItem, error) {
	item := controlledItem{taskRef: taskRef{ID: id}}
	err := db.QueryRow(`SELECT COALESCE(status,'待下载'),COALESCE(downloader,''),COALESCE(task_id,''),
		COALESCE(progress,0),COALESCE(task_submitted_at,0) FROM items WHERE id=?`, id).
		Scan(&item.Status, &item.Downloader, &item.TaskID, &item.Progress, &item.SubmittedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return item, actionError(404, "条目不存在，请刷新后重试")
	}
	if err != nil {
		return item, actionError(500, "读取下载状态失败")
	}
	return item, nil
}

func activeDownloadStatus(status string) bool {
	return status == "下载中" || status == "继续中" || status == "已发送到 Aria2" || status == "已发送到 qBittorrent" || status == "已发送到 Transmission"
}

// Queueing is synchronous so the API can report the actual changed count.
// The worker is launched only after the persisted transition succeeds.
func queueItemDownload(id int, resume bool) (bool, error) {
	l, release := downloadLock(id)
	defer release()
	l.operation.Lock()
	defer l.operation.Unlock()
	animeCleanupMu.RLock()
	defer animeCleanupMu.RUnlock()
	item, err := readControlledItem(id)
	if err != nil {
		return false, err
	}
	next := "等待下载"
	if resume {
		if item.Status == "等待继续" || item.Status == "等待下载" {
			return false, nil
		}
		if item.Status != "已暂停" {
			return false, actionError(409, "只有已暂停的任务可以继续")
		}
		if item.Downloader != "" {
			next = "等待继续"
		}
	} else {
		if item.Status == "等待下载" {
			return false, nil
		}
		if item.Status != "待下载" && item.Status != "下载失败" && item.Status != "等待下载器" {
			return false, actionError(409, "当前状态不能重新下载；已暂停的任务请点击继续")
		}
	}
	if _, err := db.Exec("UPDATE items SET status=?,task_error='' WHERE id=?", next, id); err != nil {
		return false, actionError(500, "保存下载队列失败")
	}
	return true, nil
}

func pauseItemDownload(id int) (bool, error) {
	l, release := downloadLock(id)
	defer release()
	endInterrupt := l.interrupt()
	l.operation.Lock()
	defer func() {
		endInterrupt()
		l.operation.Unlock()
	}()
	animeCleanupMu.RLock()
	defer animeCleanupMu.RUnlock()
	item, err := readControlledItem(id)
	if err != nil {
		return false, err
	}
	if item.Status == "已暂停" {
		return false, nil
	}
	if !activeDownloadStatus(item.Status) && item.Status != "待下载" && item.Status != "等待下载" && item.Status != "等待下载器" && item.Status != "等待继续" {
		return false, actionError(409, "任务已完成或失败，请刷新后重试")
	}
	if activeDownloadStatus(item.Status) && item.Downloader != "" {
		item.TaskID, err = controlDownloaderTask(item.taskRef, true)
		if err != nil {
			return false, actionError(502, "暂停失败："+err.Error())
		}
	}
	if item.Downloader == "" {
		// Direct downloads remove their temporary file before releasing operation.
		// Resume starts a fresh HTTP transfer; never advertise resumable bytes.
		item.Progress = 0
	}
	if _, err = db.Exec("UPDATE items SET status='已暂停',task_error='',task_id=?,progress=? WHERE id=?", item.TaskID, item.Progress, id); err != nil {
		return false, actionError(500, "保存暂停状态失败，请刷新后重试")
	}
	clearTransfer(id)
	return true, nil
}

// Return the effective task ID, including an Aria2 magnet's payload GID or a
// qBittorrent hash discovered by the item's unique tag. Never use global RPCs.
func controlDownloaderTask(t taskRef, pause bool) (string, error) {
	switch t.Downloader {
	case "aria2":
		state, err := aria2Poll(t.TaskID)
		if err != nil {
			return "", err
		}
		if state.Fail != "" {
			return "", errors.New(state.Fail)
		}
		if len(state.Files) > 0 {
			// Already complete: resuming should let the next poll archive it.
			if !pause {
				return state.TaskID, nil
			}
			return "", fmt.Errorf("任务已完成，正在等待归档")
		}
		if state.Paused == pause {
			return state.TaskID, nil
		}
		method := "aria2.unpause"
		if pause {
			method = "aria2.pause"
		}
		var gid string
		if err := aria2RPC(method, []any{state.TaskID}, &gid); err != nil {
			return "", err
		}
		if gid == "" || gid != state.TaskID {
			return "", fmt.Errorf("Aria2 未确认任务控制结果")
		}
		return gid, nil
	case "native":
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return nativeDownloads.control(ctx, t, pause)
	case "qbittorrent", "builtin":
		client, err := qbitLoginFor(t.Downloader)
		if err != nil {
			return "", err
		}
		query := url.Values{"hashes": {t.TaskID}}
		if t.TaskID == "" {
			query = url.Values{"tag": {qbitTagFor(t.ID)}}
		} else if !qbitControlHash.MatchString(t.TaskID) {
			return "", fmt.Errorf("qBittorrent 任务标识无效")
		}
		found, err := client.info(query)
		if err != nil {
			return "", err
		}
		if len(found) == 0 {
			if client.awaitingTask(t) {
				return "", fmt.Errorf("下载器仍在接收任务，请稍后重试")
			}
			return "", fmt.Errorf("任务在 qBittorrent 中不存在")
		}
		if len(found) != 1 || !qbitControlHash.MatchString(found[0].Hash) || (t.TaskID != "" && !strings.EqualFold(t.TaskID, found[0].Hash)) {
			return "", fmt.Errorf("无法确认唯一的 qBittorrent 任务")
		}
		hash := strings.ToLower(found[0].Hash)
		if err := client.setPaused(hash, pause); err != nil {
			return "", err
		}
		if err := client.waitPaused(hash, pause); err != nil {
			return "", err
		}
		return hash, nil
	case "transmission":
		id, err := strconv.Atoi(t.TaskID)
		if err != nil || id < 0 {
			return "", fmt.Errorf("Transmission 任务标识无效")
		}
		client := &transmissionClient{}
		state, err := transmissionPoll(client, t)
		if err != nil {
			return "", err
		}
		if state.Fail != "" {
			return "", errors.New(state.Fail)
		}
		method := "torrent-start"
		if pause {
			method = "torrent-stop"
		}
		var out struct {
			Result string `json:"result"`
		}
		if err := client.call(map[string]any{"method": method, "arguments": map[string]any{"ids": []int{id}}}, &out); err != nil {
			return "", err
		}
		if out.Result != "success" {
			return "", fmt.Errorf("Transmission 未确认任务控制结果")
		}
		return t.TaskID, nil
	default:
		return "", fmt.Errorf("无法控制此下载器的任务")
	}
}

var qbitControlHash = regexp.MustCompile(`(?i)^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

func qbitPausedState(state string) bool {
	return state == "pausedDL" || state == "stoppedDL" || state == "pausedUP" || state == "stoppedUP"
}

// qBittorrent can acknowledge a command before the torrent changes state.
// Confirm it before freeing capacity or letting the normal poller observe it.
func (c *qbitClient) waitPaused(hash string, pause bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		found, err := c.infoContext(ctx, url.Values{"hashes": {hash}})
		if err != nil {
			return fmt.Errorf("无法确认下载器控制后的状态，请刷新后重试")
		}
		if len(found) != 1 || !strings.EqualFold(found[0].Hash, hash) {
			return fmt.Errorf("下载器中的任务已变更，请刷新后重试")
		}
		if found[0].State == "error" || found[0].State == "errored" || found[0].State == "missingFiles" {
			return fmt.Errorf("qBittorrent 任务异常：%s", found[0].State)
		}
		if qbitPausedState(found[0].State) == pause {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("下载器尚未确认控制结果，请稍后重试")
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (c *qbitClient) setPaused(hash string, pause bool) error {
	method, legacy := "start", "resume"
	if pause {
		method, legacy = "stop", "pause"
	}
	form := url.Values{"hashes": {hash}}
	err := c.post("torrents/"+method, form)
	var statusErr *qbitHTTPError
	// qBittorrent 5 renamed these endpoints. Only a missing endpoint warrants
	// the 4.x fallback; authentication or server failures must be reported.
	if errors.As(err, &statusErr) && (statusErr.Code == http.StatusNotFound || statusErr.Code == http.StatusMethodNotAllowed) {
		return c.post("torrents/"+legacy, form)
	}
	return err
}

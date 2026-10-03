package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// External downloader task lifecycle: a submitted task keeps its downloader and
// task ID on the item row. The poller advances 已发送到 X → 下载中 → 已保存/下载失败
// and archives completed files from the downloader's mapped local path. A task
// the downloader no longer knows about is a failure; transport errors only skip
// the current round.

const activeTaskStatusSQL = `'已发送到 Aria2','已发送到 qBittorrent','已发送到 Transmission','下载中','继续中'`

var taskWake = make(chan struct{}, 1)

type taskRef struct {
	ID          int
	Downloader  string
	TaskID      string
	SubmittedAt int64
}

// archiveFile copies Src (a local path inside this container) into Rel inside
// the resource archive directory.
type archiveFile struct {
	Rel string
	Src string
}

type taskState struct {
	// TaskID changes when Aria2 replaces magnet metadata with a payload task.
	TaskID        string
	DownloadSpeed int64
	Progress      int
	Files         []archiveFile
	Fail          string
	Pending       bool
	Paused        bool
}

func wakeTaskPoller() {
	select {
	case taskWake <- struct{}{}:
	default:
	}
}

func startTaskPoller() {
	go func() {
		ticker := time.NewTicker(time.Duration(downloaderPollSeconds) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
			case <-taskWake:
			}
			pollDownloaderTasks()
		}
	}()
}

func activeTaskRefs() ([]taskRef, error) {
	rows, err := db.Query(`SELECT id,COALESCE(downloader,''),COALESCE(task_id,''),COALESCE(task_submitted_at,0) FROM items
		WHERE status IN (` + activeTaskStatusSQL + `)
		AND (COALESCE(task_id,'')<>'' OR COALESCE(downloader,'') IN ('qbittorrent','builtin'))`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var refs []taskRef
	for rows.Next() {
		var t taskRef
		if err := rows.Scan(&t.ID, &t.Downloader, &t.TaskID, &t.SubmittedAt); err != nil {
			return nil, err
		}
		refs = append(refs, t)
	}
	return refs, rows.Err()
}

func pollDownloaderTasks() {
	if err := recoverArchiveCommits(); err != nil {
		log.Printf("恢复归档提交失败: %v", err)
	}
	refs, err := activeTaskRefs()
	if err != nil {
		log.Println("查询下载任务失败:", err)
		return
	}
	qbits := map[string]*qbitClient{}
	qbitBroken := map[string]bool{}
	var transmission *transmissionClient
	transmissionBroken := false
	for _, ref := range refs {
		func() {
			l, release := downloadLock(ref.ID)
			defer release()
			l.operation.Lock()
			defer l.operation.Unlock()
			item, readErr := readControlledItem(ref.ID)
			if readErr != nil || !activeDownloadStatus(item.Status) || item.Downloader != ref.Downloader || item.TaskID != ref.TaskID {
				return
			}
			t := item.taskRef
			var (
				state taskState
				err   error
			)
			switch t.Downloader {
			case "aria2":
				state, err = aria2Poll(t.TaskID)
			case "qbittorrent", "builtin":
				if qbitBroken[t.Downloader] {
					return
				}
				if qbits[t.Downloader] == nil {
					qbits[t.Downloader], err = qbitLoginFor(t.Downloader)
					if err != nil {
						qbitBroken[t.Downloader] = true
						recordTaskQueryError(t, err)
						log.Println("qBittorrent 登录失败:", err)
						return
					}
				}
				state, err = qbitPoll(qbits[t.Downloader], t)
			case "transmission":
				if transmissionBroken {
					return
				}
				if transmission == nil {
					transmission = &transmissionClient{}
				}
				state, err = transmissionPoll(transmission, t)
			default:
				return
			}
			if err != nil {
				log.Printf("条目 %d 下载器查询失败: %v", t.ID, err)
				recordTaskQueryError(t, err)
				return
			}
			applyTaskUpdateLocked(t, state)
		}()
	}
}

func recordTaskQueryError(t taskRef, err error) {
	message := "下载器查询暂时失败，将自动重试"
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		message = "下载器响应超时，将自动重试"
	}
	// A temporary RPC failure is not a failed download. Keep its identity and
	// progress, and avoid applying a late error to a replacement task.
	result, updateErr := db.Exec("UPDATE items SET task_error=? WHERE id=? AND downloader=? AND task_id=? AND status IN ("+activeTaskStatusSQL+")", message, t.ID, t.Downloader, t.TaskID)
	if updateErr == nil {
		if n, _ := result.RowsAffected(); n > 0 {
			clearTransfer(t.ID)
		}
	}
}

func applyTaskUpdate(t taskRef, s taskState) {
	l, release := downloadLock(t.ID)
	defer release()
	l.operation.Lock()
	defer l.operation.Unlock()
	applyTaskUpdateLocked(t, s)
}

func applyTaskUpdateLocked(t taskRef, s taskState) {
	if s.Pending {
		return
	}
	animeCleanupMu.RLock()
	defer animeCleanupMu.RUnlock()
	var active int
	// qBittorrent discovers and persists an initially empty hash during polling.
	if err := db.QueryRow("SELECT COUNT(*) FROM items WHERE id=? AND COALESCE(downloader,'')=? AND (COALESCE(task_id,'')=? OR (? IN ('qbittorrent','builtin') AND ?='')) AND status IN ("+activeTaskStatusSQL+")", t.ID, t.Downloader, t.TaskID, t.Downloader, t.TaskID).Scan(&active); err != nil || active == 0 {
		return
	}
	if s.TaskID != "" && s.TaskID != t.TaskID && t.Downloader == "aria2" {
		result, err := db.Exec("UPDATE items SET task_id=? WHERE id=? AND downloader=? AND task_id=? AND status IN ("+activeTaskStatusSQL+")", s.TaskID, t.ID, t.Downloader, t.TaskID)
		if err != nil {
			log.Printf("条目 %d 更新 Aria2 后续任务失败: %v", t.ID, err)
			return
		}
		if n, _ := result.RowsAffected(); n != 1 {
			return
		}
		t.TaskID = s.TaskID
	}
	if s.Fail != "" {
		clearTransfer(t.ID)
		result, err := db.Exec("UPDATE items SET status='下载失败',task_error=? WHERE id=? AND status IN ("+activeTaskStatusSQL+")", s.Fail, t.ID)
		if err == nil {
			if n, _ := result.RowsAffected(); n > 0 {
				notify("download_failed", t.ID, "下载失败")
			}
		}
		return
	}
	if len(s.Files) > 0 {
		clearTransfer(t.ID)
		path, err := pendingArchivePath(t.ID)
		if err == nil && path == "" {
			path, err = archiveFromFiles(t.ID, s.Files)
		}
		if err != nil {
			// Completion state is re-observed every round, so the archive
			// retries automatically once the user fixes the mapping or mount.
			log.Printf("条目 %d 归档失败: %v", t.ID, err)
			db.Exec("UPDATE items SET status='下载中',task_error=? WHERE id=? AND status IN ("+activeTaskStatusSQL+")", "归档失败："+err.Error(), t.ID)
			return
		}
		if err := commitSavedArchive(t.ID, path); err != nil {
			log.Printf("条目 %d 归档记录提交失败，下轮重试: %v", t.ID, err)
		}
		return
	}
	if s.Paused {
		clearTransfer(t.ID)
		db.Exec("UPDATE items SET status='已暂停',task_error=CASE WHEN status='继续中' THEN task_error ELSE '' END,progress=? WHERE id=? AND status IN ("+activeTaskStatusSQL+")", s.Progress, t.ID)
		return
	}
	var status, taskError string
	setTransfer(t.ID, s.DownloadSpeed, s.Progress, max(10*time.Second, 2*time.Duration(downloaderPollSeconds)*time.Second))
	var progress int
	db.QueryRow("SELECT COALESCE(status,''),COALESCE(progress,0),COALESCE(task_error,'') FROM items WHERE id=?", t.ID).Scan(&status, &progress, &taskError)
	if status == "下载中" && progress == s.Progress && taskError == "" {
		return
	}
	db.Exec("UPDATE items SET status='下载中',task_error='',progress=? WHERE id=? AND status IN ("+activeTaskStatusSQL+")", s.Progress, t.ID)
}

// mapRemotePath translates a downloader-container path into a path visible to
// this container. Empty localBase means both sides share the same path.
func mapRemotePath(remotePath, remoteBase, localBase string) string {
	remoteBase = strings.TrimSuffix(remoteBase, "/")
	remotePath = strings.TrimSuffix(remotePath, "/")
	if localBase == "" || localBase == remoteBase || remoteBase == "" {
		return remotePath
	}
	if remotePath == remoteBase {
		return localBase
	}
	if strings.HasPrefix(remotePath, remoteBase+"/") {
		return filepath.Join(localBase, filepath.FromSlash(strings.TrimPrefix(remotePath, remoteBase+"/")))
	}
	// Unmappable path: returned unchanged so the archive step reports the
	// concrete missing file instead of silently archiving the wrong one.
	return remotePath
}

func qbitTagFor(id int) string {
	return fmt.Sprintf("auto-anime-%d", id)
}

// ---- resource classification ----

var magnetHashPattern = regexp.MustCompile(`(?i)xt=urn:btih:([A-Za-z2-7]{32}|[0-9A-Fa-f]{40})`)

func magnetInfoHash(link string) string {
	m := magnetHashPattern.FindStringSubmatch(link)
	if m == nil {
		return ""
	}
	value := m[1]
	if len(value) == 32 {
		data, err := base32.StdEncoding.DecodeString(strings.ToUpper(value))
		if err != nil {
			return ""
		}
		value = hex.EncodeToString(data)
	}
	return strings.ToLower(value)
}

func torrentLink(link string) bool {
	u, err := url.Parse(link)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	return strings.EqualFold(path.Ext(u.Path), ".torrent")
}

func fetchTorrentBytes(link string) ([]byte, error) {
	if data, ok := readTorrentCache(link); ok {
		return data, nil
	}
	return fetchTorrentWithRetry(link, time.Sleep)
}

func torrentCachePath(link string) string {
	sum := sha1.Sum([]byte(link))
	return filepath.Join(downloadDir, ".torrent-cache", hex.EncodeToString(sum[:8])+".torrent")
}

func readTorrentCache(link string) ([]byte, bool) {
	data, err := os.ReadFile(torrentCachePath(link))
	return data, err == nil && len(data) > 0 && data[0] == 'd'
}

// Serialize torrent fetches so batch downloads do not burst against RSS hosts.
var torrentFetchMu sync.Mutex

func fetchTorrentWithRetry(link string, wait func(time.Duration)) ([]byte, error) {
	torrentFetchMu.Lock()
	defer torrentFetchMu.Unlock()
	defer wait(time.Second)
	for attempt := 0; ; attempt++ {
		data, delay, err := fetchTorrentAttempt(link)
		if delay < 0 || attempt >= 2 {
			return data, err
		}
		backoff := time.Duration(5*(1<<attempt)) * time.Second
		if delay > backoff {
			backoff = delay
		}
		if backoff > time.Minute {
			return nil, err
		}
		wait(backoff)
	}
}

func fetchTorrentAttempt(link string) ([]byte, time.Duration, error) {
	return fetchTorrentAttemptContext(context.Background(), link)
}

func fetchTorrentAttemptContext(ctx context.Context, link string) ([]byte, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return nil, -1, fmt.Errorf("构造种子请求失败: %w", err)
	}
	res, err := torrentClient.Do(req)
	if err != nil {
		return nil, -1, fmt.Errorf("获取种子文件失败: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusTooManyRequests {
		delay := time.Duration(0)
		value := strings.TrimSpace(res.Header.Get("Retry-After"))
		if seconds, parseErr := strconv.ParseInt(value, 10, 64); parseErr == nil && seconds > 0 {
			if seconds > 60 {
				seconds = 61
			}
			delay = time.Duration(seconds) * time.Second
		} else if until, parseErr := http.ParseTime(value); parseErr == nil {
			delay = time.Until(until)
			if delay < 0 {
				delay = 0
			}
		}
		return nil, delay, fmt.Errorf("种子站点限流（HTTP 429），请稍后重试")
	}
	if res.StatusCode >= 400 {
		return nil, -1, fmt.Errorf("获取种子文件 HTTP %d", res.StatusCode)
	}
	const limit = 20 << 20
	data, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, -1, fmt.Errorf("读取种子文件失败")
	}
	if len(data) > limit {
		return nil, -1, fmt.Errorf("种子文件超过 20 MB")
	}
	if len(data) == 0 || data[0] != 'd' {
		return nil, -1, fmt.Errorf("不是有效的种子文件")
	}
	cache := torrentCachePath(link)
	if err := os.MkdirAll(filepath.Dir(cache), 0755); err == nil {
		tmp := cache + ".tmp"
		if os.WriteFile(tmp, data, 0644) == nil {
			if os.Rename(tmp, cache) != nil {
				os.Remove(tmp)
			}
		}
	}
	return data, -1, nil
}

// torrentInfoHash returns the lowercase hex SHA-1 of the bencoded "info" value.
func torrentInfoHash(data []byte) string {
	info, ok := bencodeFindInfo(data)
	if !ok {
		return ""
	}
	sum := sha1.Sum(info)
	return hex.EncodeToString(sum[:])
}

func bencodeFindInfo(data []byte) ([]byte, bool) {
	if len(data) == 0 || data[0] != 'd' {
		return nil, false
	}
	i := 1
	for i < len(data) && data[i] != 'e' {
		key, n, ok := bencodeString(data[i:])
		if !ok {
			return nil, false
		}
		i += n
		start := i
		if !bencodeSkip(data, &i) {
			return nil, false
		}
		if string(key) == "info" {
			return data[start:i], true
		}
	}
	return nil, false
}

func bencodeString(b []byte) ([]byte, int, bool) {
	colon := bytes.IndexByte(b, ':')
	if colon <= 0 {
		return nil, 0, false
	}
	length := 0
	available := len(b) - colon - 1
	for _, d := range b[:colon] {
		if d < '0' || d > '9' {
			return nil, 0, false
		}
		digit := int(d - '0')
		// Bound every step by the actual payload, before multiplication can
		// overflow int. This also rejects truncated strings on 32-bit hosts.
		if length > available/10 || (length == available/10 && digit > available%10) {
			return nil, 0, false
		}
		length = length*10 + digit
	}
	return b[colon+1 : colon+1+length], colon + 1 + length, true
}

func bencodeSkip(b []byte, i *int) bool {
	return bencodeSkipDepth(b, i, 0)
}

func bencodeSkipDepth(b []byte, i *int, depth int) bool {
	if *i < 0 || *i >= len(b) || depth > 64 {
		return false
	}
	switch b[*i] {
	case 'i':
		end := bytes.IndexByte(b[*i:], 'e')
		if end < 0 {
			return false
		}
		digits := b[*i+1 : *i+end]
		negative := len(digits) > 0 && digits[0] == '-'
		if negative {
			digits = digits[1:]
		}
		if len(digits) == 0 || (digits[0] == '0' && (negative || len(digits) > 1)) {
			return false
		}
		for _, digit := range digits {
			if digit < '0' || digit > '9' {
				return false
			}
		}
		*i += end + 1
		return true
	case 'l':
		*i++
		for *i < len(b) && b[*i] != 'e' {
			if !bencodeSkipDepth(b, i, depth+1) {
				return false
			}
		}
	case 'd':
		*i++
		for *i < len(b) && b[*i] != 'e' {
			_, n, ok := bencodeString(b[*i:])
			if !ok {
				return false
			}
			*i += n
			if !bencodeSkipDepth(b, i, depth+1) {
				return false
			}
		}
	default:
		_, n, ok := bencodeString(b[*i:])
		if !ok {
			return false
		}
		*i += n
		return true
	}
	if *i >= len(b) {
		return false
	}
	*i++
	return true
}

// ---- Aria2 ----

// downloaderClient only talks to the user's own downloader RPC endpoints,
// which are usually on the LAN, so it always connects directly.
var downloaderClient = &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{Proxy: nil}}

// torrentClient fetches .torrent payloads from public RSS sites and honors
// PROXY_URL like the other content clients.
var torrentClient = newClient(60 * time.Second)

func aria2RPC(method string, params []any, out any) error {
	cfg := runtimeConfig()
	if cfg.aria2Token != "" {
		params = append([]any{"token:" + cfg.aria2Token}, params...)
	}
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": "auto-anime", "method": method, "params": params})
	if err != nil {
		return err
	}
	res, err := downloaderClient.Post(cfg.aria2URL, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var rpc struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	// Aria2 1.37 answers JSON-RPC faults with HTTP 400 and a parseable error
	// body; prefer that message over the bare status code.
	if err := json.NewDecoder(io.LimitReader(res.Body, 10<<20)).Decode(&rpc); err != nil {
		if res.StatusCode >= 400 {
			return fmt.Errorf("Aria2 返回 HTTP %d", res.StatusCode)
		}
		return err
	}
	if rpc.Error != nil {
		return fmt.Errorf("Aria2: %s", rpc.Error.Message)
	}
	if res.StatusCode >= 400 {
		return fmt.Errorf("Aria2 返回 HTTP %d", res.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(rpc.Result, out)
}

// removeDownloaderTask cancels only the task recorded on an item. It never
// deletes archived media files; completed Aria2 results are removed from its
// history so stale entries do not occupy the managed queue.
func removeDownloaderTask(downloader, taskID string) {
	if taskID == "" {
		return
	}
	switch downloader {
	case "builtin":
		if client, err := qbitLoginFor("builtin"); err == nil {
			if err := client.remove(taskID); err != nil {
				log.Printf("取消内置 qBittorrent 任务失败: %v", err)
			}
		}
	case "aria2":
		if err := aria2RPC("aria2.remove", []any{taskID}, nil); err != nil {
			_ = aria2RPC("aria2.removeDownloadResult", []any{taskID}, nil)
		}
	}
}

func aria2Add(link string) (string, error) {
	// Use multiple connections for large HTTP files and let Aria2 split them
	// into reasonably sized ranges. These options affect only tasks submitted
	// by Auto-Anime and preserve the downloader's global limits.
	options := map[string]string{
		"split":                     "8",
		"max-connection-per-server": "8",
		"min-split-size":            "5M",
		"continue":                  "true",
	}
	if torrentLink(link) || magnetInfoHash(link) != "" {
		// Reuse verified pieces when a previous run left the payload without
		// its .aria2 control file. Never opt into blindly overwriting it.
		options["check-integrity"] = "true"
	}
	var gid string
	if torrentLink(link) {
		data, err := fetchTorrentBytes(link)
		if err != nil {
			return "", err
		}
		// aria2.addUri would download the .torrent file itself; torrent
		// payloads must go through aria2.addTorrent as base64.
		err = aria2RPC("aria2.addTorrent", []any{base64.StdEncoding.EncodeToString(data), []any{}, options}, &gid)
		return gid, err
	}
	err := aria2RPC("aria2.addUri", []any{[]string{link}, options}, &gid)
	return gid, err
}

type aria2Status struct {
	FollowedBy      []string `json:"followedBy"`
	VerifyPending   string   `json:"verifyIntegrityPending"`
	VerifiedLength  string   `json:"verifiedLength"`
	DownloadSpeed   string   `json:"downloadSpeed"`
	Status          string   `json:"status"`
	CompletedLength string   `json:"completedLength"`
	TotalLength     string   `json:"totalLength"`
	ErrorMessage    string   `json:"errorMessage"`
	Dir             string   `json:"dir"`
	Files           []struct {
		Path   string `json:"path"`
		Length string `json:"length"`
	} `json:"files"`
	BitTorrent *struct {
		Info struct {
			Name string `json:"name"`
		} `json:"info"`
		Mode        string `json:"mode"`
		NumSeeders  int    `json:"numSeeders"`
		Connections int    `json:"connections"`
	} `json:"bittorrent"`
}

func aria2Poll(gid string) (taskState, error) {
	keys := []string{"status", "completedLength", "totalLength", "errorMessage", "dir", "files", "bittorrent", "downloadSpeed", "followedBy", "verifyIntegrityPending", "verifiedLength"}
	seen := make(map[string]bool)
	for range 8 {
		if seen[gid] {
			return taskState{}, fmt.Errorf("Aria2 后续任务形成循环")
		}
		seen[gid] = true
		var st aria2Status
		if err := aria2RPC("aria2.tellStatus", []any{gid, keys}, &st); err != nil {
			msg := strings.ToLower(err.Error())
			// Aria2 1.37 also returns missing GIDs as HTTP 400 JSON-RPC faults.
			if strings.Contains(msg, "is not found") || strings.Contains(msg, "invalid gid") {
				return taskState{TaskID: gid, Fail: "任务在 Aria2 中不存在"}, nil
			}
			return taskState{}, err
		}
		if len(st.FollowedBy) > 0 {
			if len(st.FollowedBy) != 1 || strings.TrimSpace(st.FollowedBy[0]) == "" {
				return taskState{}, fmt.Errorf("无法确定 Aria2 后续下载任务")
			}
			gid = st.FollowedBy[0]
			continue
		}
		state := aria2PayloadState(&st)
		state.TaskID = gid
		return state, nil
	}
	return taskState{}, fmt.Errorf("Aria2 后续任务层数超过限制")
}

func aria2PayloadState(st *aria2Status) taskState {
	switch st.Status {
	case "error":
		message := st.ErrorMessage
		if message == "" {
			message = "未知错误"
		}
		return taskState{Fail: "Aria2 下载失败：" + message}
	case "removed":
		return taskState{Fail: "任务在 Aria2 中被移除"}
	}
	// Metadata is an in-memory placeholder, even when its byte count is 100%.
	// The payload GID may only appear in the next poll; never archive this row.
	for _, file := range st.Files {
		if strings.HasPrefix(file.Path, "[METADATA]") || strings.HasPrefix(file.Path, "[MEMORY][METADATA]") {
			return taskState{Paused: st.Status == "paused"}
		}
	}
	// An existing file is not complete until Aria2 has checked its pieces.
	if st.VerifyPending == "true" || st.VerifiedLength != "" {
		return taskState{Paused: st.Status == "paused"}
	}
	if st.Status == "complete" {
		files, err := aria2CompletedFiles(st)
		if err != nil {
			return taskState{Fail: err.Error()}
		}
		return taskState{Progress: 100, Files: files}
	}
	total, _ := strconv.ParseInt(st.TotalLength, 10, 64)
	done, _ := strconv.ParseInt(st.CompletedLength, 10, 64)
	progress := 0
	if total > 0 {
		progress = int(done * 100 / total)
	}
	// A seeding torrent stays "active" forever; once the payload is fully
	// downloaded its files can be archived without waiting for seeding to end.
	if st.Status == "active" && total > 0 && done >= total && st.BitTorrent != nil {
		if files, err := aria2CompletedFiles(st); err == nil {
			return taskState{Progress: 100, Files: files}
		}
	}
	speed, _ := strconv.ParseInt(st.DownloadSpeed, 10, 64)
	return taskState{Progress: progress, DownloadSpeed: speed, Paused: st.Status == "paused"}
}
func aria2CompletedFiles(st *aria2Status) ([]archiveFile, error) {
	cfg := runtimeConfig()
	remoteBase := cfg.aria2Dir
	if remoteBase == "" {
		remoteBase = st.Dir
	}
	localBase := cfg.aria2LocalDir
	if localBase == "" {
		localBase = remoteBase
	}
	root := ""
	if st.BitTorrent != nil && st.BitTorrent.Info.Name != "" && len(st.Files) > 1 {
		root = strings.TrimRight(st.Dir, "/") + "/" + st.BitTorrent.Info.Name
	}
	files := make([]archiveFile, 0, len(st.Files))
	for _, f := range st.Files {
		if f.Path == "" {
			continue
		}
		if length, _ := strconv.ParseInt(f.Length, 10, 64); length == 0 {
			continue
		}
		rel := ""
		if root != "" {
			if !strings.HasPrefix(f.Path, root+"/") {
				return nil, fmt.Errorf("无法确定文件相对路径：%s", f.Path)
			}
			rel = strings.TrimPrefix(f.Path, root+"/")
		} else {
			rel = path.Base(f.Path)
		}
		files = append(files, archiveFile{Rel: rel, Src: mapRemotePath(f.Path, remoteBase, localBase)})
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("Aria2 未返回可用文件")
	}
	return files, nil
}

// ---- qBittorrent ----

type qbitConnection struct {
	URL, User, Password, SavePath, LocalDir string
}

type qbitClient struct {
	http     *http.Client
	config   qbitConnection
	embedded *managedQBittorrent
}

func qbitLogin() (*qbitClient, error) {
	cfg := runtimeConfig()
	return loginQBittorrent(qbitConnection{cfg.qbitURL, cfg.qbitUser, cfg.qbitPassword, cfg.qbitSavePath, cfg.qbitLocalDir}, 30*time.Second)
}

func qbitLoginFor(downloader string) (*qbitClient, error) {
	if downloader != "builtin" {
		return qbitLogin()
	}
	p, err := embeddedQbit.connection(launchEmbeddedQBittorrent)
	if err != nil {
		return nil, err
	}
	client, err := loginQBittorrent(p.config, 30*time.Second)
	if err == nil {
		client.embedded = p
	}
	return client, err
}

func loginQBittorrent(config qbitConnection, timeout time.Duration) (*qbitClient, error) {
	return loginQBittorrentContext(context.Background(), config, timeout)
}

func loginQBittorrentContext(ctx context.Context, config qbitConnection, timeout time.Duration) (*qbitClient, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	config.URL = strings.TrimRight(config.URL, "/")
	client := &qbitClient{http: &http.Client{Jar: jar, Timeout: timeout, Transport: downloaderClient.Transport}, config: config}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.URL+"/api/v2/auth/login", strings.NewReader(url.Values{"username": {config.User}, "password": {config.Password}}.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := client.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	// qBittorrent 5.2 returns 204; older versions return 200 with "Ok.".
	if res.StatusCode != http.StatusNoContent && (res.StatusCode != http.StatusOK || strings.TrimSpace(string(body)) != "Ok.") {
		return nil, fmt.Errorf("qBittorrent 登录 HTTP %d", res.StatusCode)
	}
	return client, nil
}

func qbitAdd(link, tag string) error {
	client, err := qbitLogin()
	if err != nil {
		return err
	}
	return client.add(link, tag)
}

func (client *qbitClient) add(link, tag string) error {
	form := url.Values{"urls": {link}, "tags": {tag}}
	if client.config.SavePath != "" {
		form.Set("savepath", client.config.SavePath)
	}
	res, err := client.http.PostForm(client.config.URL+"/api/v2/torrents/add", form)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("qBittorrent 添加任务 HTTP %d", res.StatusCode)
	}
	if text := strings.TrimSpace(string(body)); text == "Ok." || text == "" {
		return nil
	}
	// qBittorrent 5.2 reports accepted and asynchronously pending additions as
	// JSON. An HTTP success alone must not turn a rejected resource into a task.
	var result struct {
		Success *int `json:"success_count"`
		Failure *int `json:"failure_count"`
		Pending *int `json:"pending_count"`
	}
	if json.Unmarshal(body, &result) == nil && result.Success != nil && result.Failure != nil && result.Pending != nil &&
		*result.Success >= 0 && *result.Pending >= 0 && *result.Failure == 0 && (*result.Success > 0 || *result.Pending > 0) {
		return nil
	}
	return fmt.Errorf("qBittorrent 未接受下载任务")
}

type qbitHTTPError struct{ Code int }

func (e *qbitHTTPError) Error() string { return fmt.Sprintf("qBittorrent 请求 HTTP %d", e.Code) }

func (c *qbitClient) post(path string, form url.Values) error {
	return c.postContext(context.Background(), path, form)
}

func (c *qbitClient) postContext(ctx context.Context, path string, form url.Values) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.URL+"/api/v2/"+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return &qbitHTTPError{Code: res.StatusCode}
	}
	_, err = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
	return err
}

func (c *qbitClient) remove(hash string) error {
	return c.removeContext(context.Background(), hash)
}

func (c *qbitClient) removeContext(ctx context.Context, hash string) error {
	return c.postContext(ctx, "torrents/delete", url.Values{"hashes": {hash}, "deleteFiles": {"false"}})
}

type qbitTorrent struct {
	DownloadSpeed int64   `json:"dlspeed"`
	Hash          string  `json:"hash"`
	Name          string  `json:"name"`
	SavePath      string  `json:"save_path"`
	Progress      float64 `json:"progress"`
	State         string  `json:"state"`
	Tags          string  `json:"tags"`
}

func (c *qbitClient) info(query url.Values) ([]qbitTorrent, error) {
	return c.infoContext(context.Background(), query)
}

func (c *qbitClient) infoContext(ctx context.Context, query url.Values) ([]qbitTorrent, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.config.URL+"/api/v2/torrents/info?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if p := c.embedded; p != nil && (!p.alive() || !p.ready.Load()) {
		return nil, fmt.Errorf("内置 qBittorrent 正在恢复，等待下一轮查询")
	}
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("qBittorrent 查询任务 HTTP %d", res.StatusCode)
	}
	var torrents []qbitTorrent
	if err := json.NewDecoder(io.LimitReader(res.Body, 5<<20)).Decode(&torrents); err != nil {
		return nil, err
	}
	if p := c.embedded; p != nil && (!p.alive() || !p.ready.Load()) {
		return nil, fmt.Errorf("内置 qBittorrent 正在恢复，等待下一轮查询")
	}
	return torrents, nil
}

func (c *qbitClient) fileNames(hash string) ([]string, error) {
	res, err := c.http.Get(c.config.URL + "/api/v2/torrents/files?hash=" + url.QueryEscape(hash))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("qBittorrent 查询文件 HTTP %d", res.StatusCode)
	}
	var list []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 5<<20)).Decode(&list); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(list))
	for _, f := range list {
		names = append(names, f.Name)
	}
	return names, nil
}

func qbitPoll(c *qbitClient, t taskRef) (taskState, error) {
	var torrents []qbitTorrent
	if t.TaskID != "" {
		found, err := c.info(url.Values{"hashes": {t.TaskID}})
		if err != nil {
			return taskState{}, err
		}
		if len(found) == 0 {
			if c.awaitingTask(t) {
				return taskState{Pending: true}, nil
			}
			return taskState{Fail: "任务在 qBittorrent 中不存在"}, nil
		}
		if len(found) != 1 || !strings.EqualFold(found[0].Hash, t.TaskID) {
			return taskState{}, fmt.Errorf("无法确认唯一的 qBittorrent 任务")
		}
		torrents = found
	} else {
		// The hash was unknown at submit time (older qBittorrent ignores the
		// tags form field too, so this needs qBittorrent 4.4+).
		found, err := c.info(url.Values{"tag": {qbitTagFor(t.ID)}})
		if err != nil {
			return taskState{}, err
		}
		if len(found) == 0 {
			if t.SubmittedAt == 0 || c.awaitingTask(t) {
				return taskState{Pending: true}, nil
			}
			return taskState{Fail: "qBittorrent 未能接收资源，请检查下载器中的错误后重试"}, nil
		}
		if len(found) != 1 || !qbitControlHash.MatchString(found[0].Hash) {
			return taskState{}, fmt.Errorf("无法确认唯一的 qBittorrent 任务")
		}
		torrents = found
		result, err := db.Exec("UPDATE items SET task_id=? WHERE id=? AND downloader=? AND COALESCE(task_id,'')='' AND status IN ("+activeTaskStatusSQL+")", strings.ToLower(torrents[0].Hash), t.ID, t.Downloader)
		if err != nil {
			return taskState{}, err
		}
		if n, _ := result.RowsAffected(); n != 1 {
			return taskState{Pending: true}, nil
		}
	}
	torrent := torrents[0]
	if torrent.State == "error" || torrent.State == "errored" || torrent.State == "missingFiles" {
		return taskState{Fail: "qBittorrent 任务异常：" + torrent.State}, nil
	}
	if torrent.Progress >= 1 && qbitFinishedState(torrent.State) {
		names, err := c.fileNames(torrent.Hash)
		if err != nil {
			return taskState{}, err
		}
		files, err := qbitCompletedFiles(&torrent, names, c.config)
		if err != nil {
			return taskState{Fail: err.Error()}, nil
		}
		return taskState{Progress: 100, Files: files}, nil
	}
	progress := int(torrent.Progress * 100)
	if progress > 100 {
		progress = 100
	}
	return taskState{Progress: progress, DownloadSpeed: torrent.DownloadSpeed, Paused: qbitPausedState(torrent.State)}, nil
}

// URL additions in qBittorrent 5.2 are asynchronous. Persisting the submission
// time gives the downloader a bounded grace period that survives app restarts.
func qbitAwaitingSubmission(t taskRef) bool {
	age := time.Now().Unix() - t.SubmittedAt
	return t.SubmittedAt > 0 && age >= 0 && age < 90
}

func (c *qbitClient) awaitingTask(t taskRef) bool {
	return qbitAwaitingSubmission(t) || (c.embedded != nil && time.Since(c.embedded.readyAt) < embeddedQbitRestoreGrace)
}

func qbitFinishedState(state string) bool {
	switch state {
	case "uploading", "completed", "pausedUP", "stoppedUP", "queuedUP", "stalledUP", "forcedUP":
		return true
	default:
		// checkingUP can still report the old progress before rechecking data.
		return false
	}
}

func qbitCompletedFiles(torrent *qbitTorrent, names []string, config qbitConnection) ([]archiveFile, error) {
	savePath := strings.TrimRight(torrent.SavePath, "/")
	remoteBase := config.SavePath
	if remoteBase == "" {
		remoteBase = savePath
	}
	localBase := config.LocalDir
	if localBase == "" {
		localBase = remoteBase
	}
	files := make([]archiveFile, 0, len(names))
	for _, name := range names {
		if name == "" {
			continue
		}
		files = append(files, archiveFile{Rel: name, Src: mapRemotePath(savePath+"/"+name, remoteBase, localBase)})
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("qBittorrent 未返回可用文件")
	}
	return files, nil
}

// ---- Transmission ----

type transmissionClient struct {
	sessionID string
}

func (c *transmissionClient) call(payload map[string]any, out any) error {
	cfg := runtimeConfig()
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	do := func() (*http.Response, error) {
		req, err := http.NewRequest("POST", cfg.transmissionURL+"/transmission/rpc", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.SetBasicAuth(cfg.transmissionUser, cfg.transmissionPassword)
		req.Header.Set("Content-Type", "application/json")
		if c.sessionID != "" {
			req.Header.Set("X-Transmission-Session-Id", c.sessionID)
		}
		return downloaderClient.Do(req)
	}
	res, err := do()
	if err != nil {
		return err
	}
	if res.StatusCode == http.StatusConflict {
		session := res.Header.Get("X-Transmission-Session-Id")
		res.Body.Close()
		if session == "" {
			return fmt.Errorf("Transmission 缺少 session ID")
		}
		c.sessionID = session
		res, err = do()
		if err != nil {
			return err
		}
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return fmt.Errorf("Transmission 返回 HTTP %d", res.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(res.Body, 10<<20)).Decode(out)
}

func transmissionAdd(link string) (string, error) {
	cfg := runtimeConfig()
	arguments := map[string]any{"filename": link}
	if cfg.transmissionDownloadDir != "" {
		arguments["download-dir"] = cfg.transmissionDownloadDir
	}
	var out struct {
		Result    string `json:"result"`
		Arguments struct {
			Added *struct {
				ID int `json:"id"`
			} `json:"torrent-added"`
			Duplicate *struct {
				ID int `json:"id"`
			} `json:"torrent-duplicate"`
		} `json:"arguments"`
	}
	client := &transmissionClient{}
	if err := client.call(map[string]any{"method": "torrent-add", "arguments": arguments}, &out); err != nil {
		return "", err
	}
	if out.Result != "success" {
		return "", fmt.Errorf("Transmission 返回 %s", out.Result)
	}
	if out.Arguments.Added != nil {
		return strconv.Itoa(out.Arguments.Added.ID), nil
	}
	if out.Arguments.Duplicate != nil {
		return strconv.Itoa(out.Arguments.Duplicate.ID), nil
	}
	return "", fmt.Errorf("Transmission 未返回任务 ID")
}

type transmissionTorrent struct {
	DownloadSpeed int64   `json:"rateDownload"`
	Status        int     `json:"status"`
	PercentDone   float64 `json:"percentDone"`
	IsFinished    bool    `json:"isFinished"`
	DownloadDir   string  `json:"downloadDir"`
	Error         int     `json:"error"`
	ErrorString   string  `json:"errorString"`
	Files         []struct {
		Name string `json:"name"`
	} `json:"files"`
}

func transmissionPoll(c *transmissionClient, t taskRef) (taskState, error) {
	id, err := strconv.Atoi(t.TaskID)
	if err != nil {
		return taskState{Fail: "Transmission 任务标识无效"}, nil
	}
	var out struct {
		Result    string `json:"result"`
		Arguments struct {
			Torrents []transmissionTorrent `json:"torrents"`
		} `json:"arguments"`
	}
	arguments := map[string]any{
		"ids":    []int{id},
		"fields": []string{"id", "status", "percentDone", "isFinished", "downloadDir", "error", "errorString", "files", "rateDownload"},
	}
	if err := c.call(map[string]any{"method": "torrent-get", "arguments": arguments}, &out); err != nil {
		return taskState{}, err
	}
	if out.Result != "success" {
		return taskState{}, fmt.Errorf("Transmission 返回 %s", out.Result)
	}
	if len(out.Arguments.Torrents) == 0 {
		return taskState{Fail: "任务在 Transmission 中不存在"}, nil
	}
	torrent := out.Arguments.Torrents[0]
	if torrent.Error != 0 {
		message := torrent.ErrorString
		if message == "" {
			message = "错误码 " + strconv.Itoa(torrent.Error)
		}
		return taskState{Fail: "Transmission 下载失败：" + message}, nil
	}
	// status: 0 stopped, 5 seed wait, 6 seeding.
	finished := torrent.IsFinished || (torrent.PercentDone >= 1 && (torrent.Status == 5 || torrent.Status == 6))
	if finished {
		files, err := transmissionCompletedFiles(&torrent)
		if err != nil {
			return taskState{Fail: err.Error()}, nil
		}
		return taskState{Progress: 100, Files: files}, nil
	}
	progress := int(torrent.PercentDone * 100)
	if progress > 100 {
		progress = 100
	}
	return taskState{Progress: progress, DownloadSpeed: torrent.DownloadSpeed, Paused: torrent.Status == 0}, nil
}

func transmissionCompletedFiles(torrent *transmissionTorrent) ([]archiveFile, error) {
	cfg := runtimeConfig()
	if len(torrent.Files) == 0 {
		return nil, fmt.Errorf("Transmission 未返回文件列表")
	}
	dir := strings.TrimRight(torrent.DownloadDir, "/")
	remoteBase := cfg.transmissionDownloadDir
	if remoteBase == "" {
		remoteBase = dir
	}
	localBase := cfg.transmissionLocalDir
	if localBase == "" {
		localBase = remoteBase
	}
	files := make([]archiveFile, 0, len(torrent.Files))
	for _, f := range torrent.Files {
		if f.Name == "" {
			continue
		}
		files = append(files, archiveFile{Rel: f.Name, Src: mapRemotePath(dir+"/"+f.Name, remoteBase, localBase)})
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("Transmission 未返回可用文件")
	}
	return files, nil
}

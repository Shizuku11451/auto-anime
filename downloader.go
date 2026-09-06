package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// External downloader task lifecycle: a submitted task keeps its downloader and
// task ID on the item row. The poller advances 已发送到 X → 下载中 → 已保存/下载失败
// and archives completed files from the downloader's mapped local path. A task
// the downloader no longer knows about is a failure; transport errors only skip
// the current round.

const activeTaskStatusSQL = `'已发送到 Aria2','已发送到 qBittorrent','已发送到 Transmission','下载中'`

var taskWake = make(chan struct{}, 1)

type taskRef struct {
	ID         int
	Downloader string
	TaskID     string
}

// archiveFile copies Src (a local path inside this container) into Rel inside
// the resource archive directory.
type archiveFile struct {
	Rel string
	Src string
}

type taskState struct {
	Progress int
	Files    []archiveFile
	Fail     string
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
	rows, err := db.Query(`SELECT id,COALESCE(downloader,''),COALESCE(task_id,'') FROM items
		WHERE status IN (` + activeTaskStatusSQL + `)
		AND (COALESCE(task_id,'')<>'' OR COALESCE(downloader,'')='qbittorrent')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var refs []taskRef
	for rows.Next() {
		var t taskRef
		if err := rows.Scan(&t.ID, &t.Downloader, &t.TaskID); err != nil {
			return nil, err
		}
		refs = append(refs, t)
	}
	return refs, rows.Err()
}

func pollDownloaderTasks() {
	refs, err := activeTaskRefs()
	if err != nil {
		log.Println("查询下载任务失败:", err)
		return
	}
	var qbit *qbitClient
	qbitBroken := false
	var transmission *transmissionClient
	transmissionBroken := false
	for _, t := range refs {
		var (
			state taskState
			err   error
		)
		switch t.Downloader {
		case "aria2":
			state, err = aria2Poll(t.TaskID)
		case "qbittorrent":
			if qbitBroken {
				continue
			}
			if qbit == nil {
				qbit, err = qbitLogin()
				if err != nil {
					qbitBroken = true
					log.Println("qBittorrent 登录失败:", err)
					continue
				}
			}
			state, err = qbitPoll(qbit, t)
		case "transmission":
			if transmissionBroken {
				continue
			}
			if transmission == nil {
				transmission = &transmissionClient{}
			}
			state, err = transmissionPoll(transmission, t)
		default:
			continue
		}
		if err != nil {
			log.Printf("条目 %d 下载器查询失败: %v", t.ID, err)
			continue
		}
		applyTaskUpdate(t, state)
	}
}

func applyTaskUpdate(t taskRef, s taskState) {
	if s.Fail != "" {
		result, err := db.Exec("UPDATE items SET status='下载失败',task_error=? WHERE id=? AND status IN ("+activeTaskStatusSQL+")", s.Fail, t.ID)
		if err == nil {
			if n, _ := result.RowsAffected(); n > 0 {
				notify("download_failed", t.ID, "下载失败")
			}
		}
		return
	}
	if len(s.Files) > 0 {
		path, err := archiveFromFiles(t.ID, s.Files)
		if err != nil {
			// Completion state is re-observed every round, so the archive
			// retries automatically once the user fixes the mapping or mount.
			log.Printf("条目 %d 归档失败: %v", t.ID, err)
			db.Exec("UPDATE items SET status='下载中',task_error=? WHERE id=? AND status IN ("+activeTaskStatusSQL+")", "归档失败："+err.Error(), t.ID)
			return
		}
		db.Exec("UPDATE items SET status='已保存',file_path=?,task_error='',progress=100 WHERE id=?", path, t.ID)
		notify("download_saved", t.ID, "已保存")
		return
	}
	var status string
	var progress int
	db.QueryRow("SELECT COALESCE(status,''),COALESCE(progress,0) FROM items WHERE id=?", t.ID).Scan(&status, &progress)
	if status == "下载中" && progress == s.Progress {
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
	res, err := downloaderClient.Get(link)
	if err != nil {
		return nil, fmt.Errorf("获取种子文件失败: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("获取种子文件 HTTP %d", res.StatusCode)
	}
	const limit = 20 << 20
	data, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("读取种子文件失败")
	}
	if len(data) > limit {
		return nil, fmt.Errorf("种子文件超过 20 MB")
	}
	if len(data) == 0 || data[0] != 'd' {
		return nil, fmt.Errorf("不是有效的种子文件")
	}
	return data, nil
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
	for _, d := range b[:colon] {
		if d < '0' || d > '9' {
			return nil, 0, false
		}
		length = length*10 + int(d-'0')
	}
	if colon+1+length > len(b) {
		return nil, 0, false
	}
	return b[colon+1 : colon+1+length], colon + 1 + length, true
}

func bencodeSkip(b []byte, i *int) bool {
	if *i >= len(b) {
		return false
	}
	switch b[*i] {
	case 'i':
		end := bytes.IndexByte(b[*i:], 'e')
		if end < 0 {
			return false
		}
		*i += end + 1
		return true
	case 'l':
		*i++
		for *i < len(b) && b[*i] != 'e' {
			if !bencodeSkip(b, i) {
				return false
			}
		}
	case 'd':
		*i++
		for *i < len(b) && b[*i] != 'e' {
			if !bencodeSkip(b, i) || !bencodeSkip(b, i) {
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

var downloaderClient = &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{Proxy: nil}}

func aria2RPC(method string, params []any, out any) error {
	if aria2Token != "" {
		params = append([]any{"token:" + aria2Token}, params...)
	}
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": "auto-anime", "method": method, "params": params})
	if err != nil {
		return err
	}
	res, err := downloaderClient.Post(aria2URL, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return fmt.Errorf("Aria2 返回 HTTP %d", res.StatusCode)
	}
	var rpc struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 10<<20)).Decode(&rpc); err != nil {
		return err
	}
	if rpc.Error != nil {
		return fmt.Errorf("Aria2: %s", rpc.Error.Message)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(rpc.Result, out)
}

func aria2Add(link string) (string, error) {
	var gid string
	if torrentLink(link) {
		data, err := fetchTorrentBytes(link)
		if err != nil {
			return "", err
		}
		// aria2.addUri would download the .torrent file itself; torrent
		// payloads must go through aria2.addTorrent as base64.
		err = aria2RPC("aria2.addTorrent", []any{base64.StdEncoding.EncodeToString(data), []any{}}, &gid)
		return gid, err
	}
	err := aria2RPC("aria2.addUri", []any{[]string{link}}, &gid)
	return gid, err
}

type aria2Status struct {
	Status          string `json:"status"`
	CompletedLength string `json:"completedLength"`
	TotalLength     string `json:"totalLength"`
	ErrorMessage    string `json:"errorMessage"`
	Dir             string `json:"dir"`
	Files           []struct {
		Path   string `json:"path"`
		Length string `json:"length"`
	} `json:"files"`
	BitTorrent *struct {
		Info struct {
			Name string `json:"name"`
		} `json:"info"`
		Mode string `json:"mode"`
	} `json:"bittorrent"`
}

func aria2Poll(gid string) (taskState, error) {
	var st aria2Status
	keys := []string{"status", "completedLength", "totalLength", "errorMessage", "dir", "files", "bittorrent"}
	if err := aria2RPC("aria2.tellStatus", []any{gid, keys}, &st); err != nil {
		if strings.Contains(err.Error(), "is not found") {
			return taskState{Fail: "任务在 Aria2 中不存在"}, nil
		}
		return taskState{}, err
	}
	switch st.Status {
	case "error":
		message := st.ErrorMessage
		if message == "" {
			message = "未知错误"
		}
		return taskState{Fail: "Aria2 下载失败：" + message}, nil
	case "removed":
		return taskState{Fail: "任务在 Aria2 中被移除"}, nil
	case "complete":
		files, err := aria2CompletedFiles(&st)
		if err != nil {
			return taskState{Fail: err.Error()}, nil
		}
		return taskState{Progress: 100, Files: files}, nil
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
		if files, err := aria2CompletedFiles(&st); err == nil {
			return taskState{Progress: 100, Files: files}, nil
		}
	}
	return taskState{Progress: progress}, nil
}
func aria2CompletedFiles(st *aria2Status) ([]archiveFile, error) {
	remoteBase := aria2Dir
	if remoteBase == "" {
		remoteBase = st.Dir
	}
	localBase := aria2LocalDir
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

type qbitClient struct {
	http *http.Client
}

func qbitLogin() (*qbitClient, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	client := &qbitClient{http: &http.Client{Jar: jar, Timeout: 30 * time.Second, Transport: &http.Transport{Proxy: nil}}}
	res, err := client.http.PostForm(qbitURL+"/api/v2/auth/login", url.Values{"username": {qbitUser}, "password": {qbitPassword}})
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 400 || strings.TrimSpace(string(body)) != "Ok." {
		return nil, fmt.Errorf("qBittorrent 登录 HTTP %d", res.StatusCode)
	}
	return client, nil
}

func qbitAdd(link, tag string) error {
	client, err := qbitLogin()
	if err != nil {
		return err
	}
	form := url.Values{"urls": {link}, "tags": {tag}}
	if qbitSavePath != "" {
		form.Set("savepath", qbitSavePath)
	}
	res, err := client.http.PostForm(qbitURL+"/api/v2/torrents/add", form)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 400 {
		return fmt.Errorf("qBittorrent 添加任务 HTTP %d", res.StatusCode)
	}
	if strings.TrimSpace(string(body)) != "Ok." && len(body) > 0 {
		return fmt.Errorf("qBittorrent 添加任务失败: %s", strings.TrimSpace(string(body)))
	}
	return nil
}

type qbitTorrent struct {
	Hash     string  `json:"hash"`
	Name     string  `json:"name"`
	SavePath string  `json:"save_path"`
	Progress float64 `json:"progress"`
	State    string  `json:"state"`
}

func (c *qbitClient) info(query url.Values) ([]qbitTorrent, error) {
	res, err := c.http.Get(qbitURL + "/api/v2/torrents/info?" + query.Encode())
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("qBittorrent 查询任务 HTTP %d", res.StatusCode)
	}
	var torrents []qbitTorrent
	if err := json.NewDecoder(io.LimitReader(res.Body, 5<<20)).Decode(&torrents); err != nil {
		return nil, err
	}
	return torrents, nil
}

func (c *qbitClient) fileNames(hash string) ([]string, error) {
	res, err := c.http.Get(qbitURL + "/api/v2/torrents/files?hash=" + url.QueryEscape(hash))
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
			return taskState{Fail: "任务在 qBittorrent 中不存在"}, nil
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
			return taskState{Progress: 0}, nil
		}
		torrents = found[:1]
		db.Exec("UPDATE items SET task_id=? WHERE id=? AND COALESCE(task_id,'')=''", strings.ToLower(torrents[0].Hash), t.ID)
	}
	torrent := torrents[0]
	if torrent.State == "errored" || torrent.State == "missingFiles" {
		return taskState{Fail: "qBittorrent 任务异常：" + torrent.State}, nil
	}
	if torrent.Progress >= 1 && qbitFinishedState(torrent.State) {
		names, err := c.fileNames(torrent.Hash)
		if err != nil {
			return taskState{}, err
		}
		files, err := qbitCompletedFiles(&torrent, names)
		if err != nil {
			return taskState{Fail: err.Error()}, nil
		}
		return taskState{Progress: 100, Files: files}, nil
	}
	progress := int(torrent.Progress * 100)
	if progress > 100 {
		progress = 100
	}
	return taskState{Progress: progress}, nil
}

func qbitFinishedState(state string) bool {
	if state == "uploading" || state == "completed" {
		return true
	}
	// pausedUP/stoppedUP/queuedUP/stalledUP/forcedUP/checkingUP all end in UP.
	return strings.HasSuffix(state, "UP")
}

func qbitCompletedFiles(torrent *qbitTorrent, names []string) ([]archiveFile, error) {
	savePath := strings.TrimRight(torrent.SavePath, "/")
	remoteBase := qbitSavePath
	if remoteBase == "" {
		remoteBase = savePath
	}
	localBase := qbitLocalDir
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
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	do := func() (*http.Response, error) {
		req, err := http.NewRequest("POST", transmissionURL+"/transmission/rpc", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.SetBasicAuth(transmissionUser, transmissionPassword)
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
	arguments := map[string]any{"filename": link}
	if transmissionDownloadDir != "" {
		arguments["download-dir"] = transmissionDownloadDir
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
	Status      int     `json:"status"`
	PercentDone float64 `json:"percentDone"`
	IsFinished  bool    `json:"isFinished"`
	DownloadDir string  `json:"downloadDir"`
	Error       int     `json:"error"`
	ErrorString string  `json:"errorString"`
	Files       []struct {
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
		"fields": []string{"id", "status", "percentDone", "isFinished", "downloadDir", "error", "errorString", "files"},
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
	return taskState{Progress: progress}, nil
}

func transmissionCompletedFiles(torrent *transmissionTorrent) ([]archiveFile, error) {
	if len(torrent.Files) == 0 {
		return nil, fmt.Errorf("Transmission 未返回文件列表")
	}
	dir := strings.TrimRight(torrent.DownloadDir, "/")
	remoteBase := transmissionDownloadDir
	if remoteBase == "" {
		remoteBase = dir
	}
	localBase := transmissionLocalDir
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

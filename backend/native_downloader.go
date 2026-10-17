package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	raintorrent "github.com/cenkalti/rain/torrent"
)

// native downloader 是 Windows 版默认的进程内 BT 引擎。它不依赖 qBittorrent，
// 但仍复用现有条目、任务身份、并发名额、归档与源文件清理。
//
// 新任务身份为 infohash-itemID；旧 40 位 hash 继续恢复。取消/清理使用本地
// Rain 补丁的 ForgetTorrent，只释放会话状态和端口，绝不删除源文件。

const (
	nativeDefaultBTDataDir = "bt"
	nativeDefaultBTPort    = 6881
	nativeDefaultPortRange = 10
)

var nativeInfoHashPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
var nativeTaskIDPattern = regexp.MustCompile(`^[0-9a-f]{40}(-[1-9][0-9]*)?$`)

func nativeTaskID(hash string, itemID int) string {
	return fmt.Sprintf("%s-%d", strings.ToLower(hash), itemID)
}

func nativeTaskHash(id string) string {
	if !nativeTaskIDPattern.MatchString(id) {
		return ""
	}
	return strings.SplitN(id, "-", 2)[0]
}

var errNativeStopping = errors.New("内置 BT 下载器正在关闭")
var errNativeSharedTask = errors.New("旧内置 BT 任务被多个条目共用，无法独立控制；请移除重复记录（保留媒体）后重试")

type nativeSettings struct {
	Root      string
	DataDir   string
	SessionDB string
	BTPort    uint16
	PortEnd   uint16
	PortRange int
	DHTPort   uint16
}

type nativeDownloader struct {
	mu        sync.RWMutex
	session   *raintorrent.Session
	stopping  bool
	requested bool
	starting  chan struct{}
	startErr  error
	retryAt   time.Time
	failures  int
}

var nativeDownloads = &nativeDownloader{}

func nativeBTDataRoot() (string, error) {
	root := strings.TrimSpace(env("NATIVE_BT_DATA_DIR", ""))
	if root == "" {
		root = filepath.Join(filepath.Dir(env("DB_PATH", defaultDBPath())), nativeDefaultBTDataDir)
	}
	return filepath.Abs(root)
}

func qbitNativeDownloadDir(root string) string {
	return filepath.Join(root, "downloads")
}

func nativeDownloadDir() string {
	root, err := nativeBTDataRoot()
	if err != nil || root == "" {
		return ""
	}
	return qbitNativeDownloadDir(root)
}

func nativeSourceRoot() string {
	return nativeDownloadDir()
}

func nativeEnvPort(key string, fallback int) (uint16, error) {
	raw := strings.TrimSpace(env(key, ""))
	value := fallback
	if raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 65535 {
			return 0, fmt.Errorf("%s 必须是 1-65535 的端口", key)
		}
		value = n
	}
	return uint16(value), nil
}

func nativeEnvRange() (int, error) {
	raw := strings.TrimSpace(env("NATIVE_BT_PORT_RANGE", ""))
	if raw == "" {
		return nativeDefaultPortRange, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 100 {
		return 0, fmt.Errorf("NATIVE_BT_PORT_RANGE 必须是 1-100 的整数")
	}
	return n, nil
}

func nativeSettingsFromEnv() (nativeSettings, error) {
	root, err := nativeBTDataRoot()
	if err != nil {
		return nativeSettings{}, err
	}
	btPort, err := nativeEnvPort("NATIVE_BT_PORT", nativeDefaultBTPort)
	if err != nil {
		return nativeSettings{}, err
	}
	rangeN, err := nativeEnvRange()
	if err != nil {
		return nativeSettings{}, err
	}
	end := int(btPort) + rangeN
	if end > 65535 {
		return nativeSettings{}, fmt.Errorf("NATIVE_BT_PORT 与 NATIVE_BT_PORT_RANGE 超出端口范围")
	}
	dhtPort, err := nativeEnvPort("NATIVE_DHT_PORT", int(btPort))
	if err != nil {
		return nativeSettings{}, err
	}
	return nativeSettings{
		Root:      root,
		DataDir:   qbitNativeDownloadDir(root),
		SessionDB: filepath.Join(root, "session.db"),
		BTPort:    btPort,
		PortEnd:   uint16(end),
		PortRange: rangeN,
		DHTPort:   dhtPort,
	}, nil
}

func nativeNeeded() (bool, error) {
	if downloadMode == "native" {
		return true, nil
	}
	if db == nil {
		return false, nil
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM items WHERE downloader='native'").Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func nativeRetryDelay(failures int) time.Duration {
	if failures < 1 {
		failures = 1
	}
	if failures > 5 {
		failures = 5
	}
	return min(2*time.Second*time.Duration(1<<(failures-1)), time.Minute)
}

// ensure 懒启动 rain 会话；失败后按退避重试。启动失败不阻止 Web 界面打开，
// 但会让健康检查失败，提交任务时返回具体原因。
func (n *nativeDownloader) ensure(ctx context.Context) error {
	n.mu.Lock()
	if n.stopping {
		n.mu.Unlock()
		return errNativeStopping
	}
	if n.session != nil {
		n.mu.Unlock()
		return nil
	}
	if n.starting != nil {
		done := n.starting
		n.mu.Unlock()
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
		return n.ensure(ctx)
	}
	if n.startErr != nil && time.Now().Before(n.retryAt) {
		err := n.startErr
		n.mu.Unlock()
		return fmt.Errorf("内置 BT 下载器正在恢复：%w", err)
	}
	n.requested = true
	done := make(chan struct{})
	n.starting = done
	n.mu.Unlock()

	session, err := openNativeSession(ctx)

	n.mu.Lock()
	n.starting = nil
	if err != nil {
		n.failures = min(n.failures+1, 5)
		n.startErr = err
		n.retryAt = time.Now().Add(nativeRetryDelay(n.failures))
		n.mu.Unlock()
		close(done)
		return err
	}
	if n.stopping {
		n.mu.Unlock()
		close(done)
		closeNativeSession(session)
		return errNativeStopping
	}
	n.session = session
	n.startErr = nil
	n.retryAt = time.Time{}
	n.failures = 0
	n.mu.Unlock()
	close(done)
	if settings, err := nativeSettingsFromEnv(); err == nil {
		log.Printf("内置 BT 下载器已就绪，监听 TCP %d-%d、UDP %d", settings.BTPort, settings.PortEnd-1, settings.DHTPort)
	} else {
		log.Println("内置 BT 下载器已就绪")
	}
	wakeTaskPoller()
	return nil
}

func openNativeSession(ctx context.Context) (*raintorrent.Session, error) {
	settings, err := nativeSettingsFromEnv()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(settings.Root, 0700); err != nil {
		return nil, fmt.Errorf("创建内置 BT 数据目录失败：%w", err)
	}
	if err := os.MkdirAll(settings.DataDir, 0700); err != nil {
		return nil, fmt.Errorf("创建内置 BT 下载目录失败：%w", err)
	}
	// 所有 BT 流量直连；rain 的 tracker/peer 连接不使用系统代理，与 qBittorrent 一致。
	raintorrent.DisableLogging()
	cfg := raintorrent.DefaultConfig
	cfg.Database = settings.SessionDB
	cfg.DataDir = settings.DataDir
	cfg.DataDirIncludesTorrentID = true
	cfg.ResumeOnStartup = false
	cfg.RPCEnabled = false
	cfg.DHTEnabled = true
	cfg.DHTPort = settings.DHTPort
	cfg.Host = "0.0.0.0"
	cfg.PortBegin = settings.BTPort
	cfg.PortEnd = settings.PortEnd
	cfg.MaxOpenFiles = 0
	cfg.MaxTorrentSize = 20 << 20
	cfg.MaxMetadataSize = 20 << 20
	cfg.FilePermissions = fs.FileMode(0700)
	cfg.TrackerStopTimeout = 2 * time.Second
	// rain 的健康检查超时会直接 panic。BT 引擎在 UI 进程内运行，不能让一个
	// 卡住的 torrent 杀掉整个应用；把检查间隔拉到实际不会触发。
	cfg.HealthCheckInterval = 24 * time.Hour
	cfg.HealthCheckTimeout = 24 * time.Hour
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	session, err := raintorrent.NewSession(cfg)
	if err != nil {
		return nil, fmt.Errorf("启动内置 BT 下载器失败：%w", err)
	}
	if ctx.Err() != nil {
		closeNativeSession(session)
		return nil, ctx.Err()
	}
	reconcileNativeSession(session)
	return session, nil
}

func closeNativeSession(session *raintorrent.Session) {
	if session == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("内置 BT 下载器关闭时发生异常：%v", recovered)
			}
		}()
		_ = session.Close()
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		log.Println("内置 BT 下载器未及时关闭，继续退出")
	}
}

// 活动条目由轮询恢复，暂停条目保持停止；孤立会话释放资源但不删除数据。
func reconcileNativeSession(session *raintorrent.Session) {
	if db == nil {
		return
	}
	rows, err := db.Query("SELECT id,COALESCE(status,''),COALESCE(task_id,''),COALESCE(link,'') FROM items WHERE downloader='native'")
	if err != nil {
		log.Printf("读取内置 BT 任务失败：%v", err)
		return
	}
	type nativeItem struct {
		ID     int
		Status string
		TaskID string
		Link   string
	}
	items := map[string][]nativeItem{}
	for rows.Next() {
		var item nativeItem
		if rows.Scan(&item.ID, &item.Status, &item.TaskID, &item.Link) == nil && item.TaskID != "" {
			key := strings.ToLower(item.TaskID)
			items[key] = append(items[key], item)
		}
	}
	rows.Close()
	keepSource := runtimeConfig().keepSourceFiles
	for _, torrent := range session.ListTorrents() {
		id := strings.ToLower(torrent.ID())
		owners, ok := items[id]
		if !ok {
			_ = session.ForgetTorrent(id)
			log.Printf("释放无条目关联的内置 BT 任务 %s，保留源文件", id)
			continue
		}
		allSaved := true
		for _, item := range owners {
			allSaved = allSaved && item.Status == "已保存"
		}
		if allSaved && keepSource {
			if err := nativeStartTorrent(torrent); err != nil {
				log.Printf("恢复内置 BT 做种失败（%s）：%v", id, err)
			}
		} else if allSaved {
			_ = session.ForgetTorrent(id)
		}
	}
}

func (n *nativeDownloader) healthy() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.stopping {
		return false
	}
	if n.session != nil {
		return true
	}
	return !n.requested
}

func (n *nativeDownloader) stop() {
	n.mu.Lock()
	if n.stopping {
		n.mu.Unlock()
		return
	}
	n.stopping = true
	session := n.session
	n.session = nil
	n.mu.Unlock()
	closeNativeSession(session)
}

func (n *nativeDownloader) withSession(ctx context.Context, fn func(*raintorrent.Session) error) error {
	if err := n.ensure(ctx); err != nil {
		return err
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.stopping || n.session == nil {
		return errNativeStopping
	}
	return fn(n.session)
}

func (n *nativeDownloader) add(ctx context.Context, ref taskRef, link string, data []byte) error {
	if nativeTaskHash(strings.ToLower(ref.TaskID)) == "" {
		return fmt.Errorf("内置 BT 任务标识无效")
	}
	return n.withSession(ctx, func(session *raintorrent.Session) error {
		_, err := nativeAddToSession(session, ref, link, data)
		return err
	})
}

func nativeAddToSession(session *raintorrent.Session, ref taskRef, link string, data []byte) (*raintorrent.Torrent, error) {
	if nativeTaskHash(ref.TaskID) == "" {
		return nil, fmt.Errorf("内置 BT 任务标识无效")
	}
	if len(data) > 0 && torrentInfoHash(data) != nativeTaskHash(ref.TaskID) {
		return nil, fmt.Errorf("种子文件与任务标识不一致")
	}
	if len(data) == 0 && magnetInfoHash(link) != nativeTaskHash(ref.TaskID) {
		return nil, fmt.Errorf("磁力链接与任务标识不一致")
	}
	if torrent := session.GetTorrent(strings.ToLower(ref.TaskID)); torrent != nil {
		return torrent, nativeStartTorrent(torrent)
	}
	if err := nativeMakeRoom(session); err != nil {
		return nil, err
	}
	if torrent, err := session.LoadTorrent(ref.TaskID); err != nil {
		return nil, err
	} else if torrent != nil {
		return torrent, nativeStartTorrent(torrent)
	}
	if len(data) > 0 {
		torrent, err := session.AddTorrent(bytes.NewReader(data), &raintorrent.AddTorrentOptions{ID: strings.ToLower(ref.TaskID), Stopped: true})
		if err != nil {
			return nil, fmt.Errorf("内置 BT 下载器添加种子失败：%w", err)
		}
		if err := nativeStartTorrent(torrent); err != nil {
			_ = session.ForgetTorrent(torrent.ID())
			return nil, err
		}
		return torrent, nil
	}
	if magnetInfoHash(link) == "" {
		return nil, fmt.Errorf("资源不是有效的磁力链接")
	}
	torrent, err := session.AddURI(link, &raintorrent.AddTorrentOptions{ID: strings.ToLower(ref.TaskID), StopAfterMetadata: true})
	if err != nil {
		if existing := session.GetTorrent(strings.ToLower(ref.TaskID)); existing != nil {
			return existing, nil
		}
		return nil, fmt.Errorf("内置 BT 下载器添加磁力失败：%w", err)
	}
	return torrent, nil
}

// Stopped/history torrents retain resume data on disk, not a permanent port.
// Seeding yields capacity to a new download; active downloads are never evicted.
func nativeMakeRoom(session *raintorrent.Session) error {
	if session.HasFreePort() {
		return nil
	}
	for _, torrent := range session.ListTorrents() {
		if db != nil {
			var active int
			if err := db.QueryRow("SELECT COUNT(*) FROM items WHERE downloader='native' AND task_id=? AND status IN ("+activeTaskStatusSQL+")", torrent.ID()).Scan(&active); err != nil {
				return err
			}
			if active > 0 {
				continue // Completed payload must be archived before yielding its slot.
			}
		}
		switch torrent.Stats().Status {
		case raintorrent.Stopped, raintorrent.Seeding:
			return session.UnloadTorrent(torrent.ID())
		}
	}
	return raintorrent.ErrNoFreePort
}

func nativeStartTorrent(torrent *raintorrent.Torrent) error {
	files, err := torrent.Files()
	if err != nil {
		// rain 目前用普通错误表示元数据未就绪；统一按等待处理，避免把正在
		// 下载元数据的磁力任务误判为失败。
		return torrent.Start()
	}
	if err := validateNativeFiles(torrent.Dir(), files); err != nil {
		return err
	}
	return torrent.Start()
}

func (n *nativeDownloader) poll(ctx context.Context, ref taskRef) (taskState, error) {
	var state taskState
	err := n.withSession(ctx, func(session *raintorrent.Session) error {
		// Older releases shared one torrent across items. Do not restart it
		// behind a paused owner's back, even before the user resolves duplicates.
		if db != nil && nativeInfoHashPattern.MatchString(ref.TaskID) {
			var paused int
			if err := db.QueryRow("SELECT COUNT(*) FROM items WHERE downloader='native' AND task_id=? AND id<>? AND status IN ('已暂停','等待继续')", ref.TaskID, ref.ID).Scan(&paused); err != nil {
				return err
			}
			if paused > 0 {
				return errNativeSharedTask
			}
		}
		torrent := session.GetTorrent(strings.ToLower(ref.TaskID))
		if torrent == nil {
			var err error
			torrent, err = nativeEnsureTorrent(session, ref)
			if err != nil {
				if errors.Is(err, raintorrent.ErrNoFreePort) {
					state = taskState{Pending: true}
					return nil
				}
				return err
			}
		}
		if torrent == nil {
			return fmt.Errorf("任务在内置 BT 下载器中不存在")
		}
		state = nativeTorrentState(torrent)
		return nil
	})
	return state, err
}

func nativeEnsureTorrent(session *raintorrent.Session, ref taskRef) (*raintorrent.Torrent, error) {
	if torrent := session.GetTorrent(strings.ToLower(ref.TaskID)); torrent != nil {
		return torrent, nil
	}
	if nativeTaskHash(ref.TaskID) == "" {
		return nil, fmt.Errorf("内置 BT 任务标识无效")
	}
	if err := nativeMakeRoom(session); err != nil {
		return nil, err
	}
	if torrent, err := session.LoadTorrent(ref.TaskID); err != nil || torrent != nil {
		return torrent, err
	}
	var link string
	if db == nil || db.QueryRow("SELECT link FROM items WHERE id=?", ref.ID).Scan(&link) != nil {
		return nil, fmt.Errorf("读取内置 BT 任务资源失败")
	}
	hash := nativeTaskHash(strings.ToLower(ref.TaskID))
	var data []byte
	if magnetHash := magnetInfoHash(link); magnetHash != "" {
		if hash == "" {
			hash = magnetHash
		}
		if magnetHash != hash {
			return nil, fmt.Errorf("磁力链接与任务标识不一致")
		}
	} else {
		var err error
		data, err = fetchTorrentBytes(link)
		if err != nil {
			return nil, err
		}
		computed := torrentInfoHash(data)
		if hash == "" {
			hash = computed
		}
		if computed == "" || computed != hash {
			return nil, fmt.Errorf("种子文件与任务标识不一致")
		}
	}
	if !nativeInfoHashPattern.MatchString(hash) {
		return nil, fmt.Errorf("无法识别内置 BT 任务标识")
	}
	return nativeAddToSession(session, ref, link, data)
}

func nativeTorrentState(torrent *raintorrent.Torrent) taskState {
	stats := torrent.Stats()
	if stats.Error != nil {
		return taskState{Fail: "内置下载器任务失败：" + stats.Error.Error()}
	}
	files, err := torrent.Files()
	if err != nil {
		// 磁力元数据还没到；rain 的超时/重启由会话自己处理，这里只保持等待。
		if stats.Status == raintorrent.Stopped {
			if err := torrent.Start(); err != nil {
				return taskState{Fail: "启动内置下载器任务失败：" + err.Error()}
			}
		}
		return taskState{Pending: true}
	}
	if err := validateNativeFiles(torrent.Dir(), files); err != nil {
		return taskState{Fail: "种子文件路径不安全：" + err.Error()}
	}
	if nativeFinished(stats) {
		archive, err := nativeArchiveFiles(torrent, files)
		if err != nil {
			return taskState{Fail: err.Error()}
		}
		return taskState{Progress: 100, Files: archive}
	}
	if stats.Status == raintorrent.Stopped {
		// StopAfterMetadata 或意外停止：活动条目由轮询重新启动，不在这里误报暂停。
		if err := torrent.Start(); err != nil {
			return taskState{Fail: "启动内置下载器任务失败：" + err.Error()}
		}
		return taskState{Pending: true}
	}
	progress := 0
	if stats.Bytes.Total > 0 {
		progress = int(stats.Bytes.Completed * 100 / stats.Bytes.Total)
	}
	progress = min(max(progress, 0), 100)
	return taskState{Progress: progress, DownloadSpeed: int64(stats.Speed.Download)}
}

func nativeFinished(stats raintorrent.Stats) bool {
	if stats.Bytes.Total <= 0 || stats.Bytes.Completed < stats.Bytes.Total {
		return false
	}
	switch stats.Status {
	case raintorrent.Seeding, raintorrent.Downloading, raintorrent.Stopped:
		return true
	default:
		return false
	}
}

func nativeArchiveFiles(torrent *raintorrent.Torrent, files []raintorrent.File) ([]archiveFile, error) {
	dir := torrent.Dir()
	if dir == "" {
		return nil, fmt.Errorf("内置下载器未返回下载目录")
	}
	archive := make([]archiveFile, 0, len(files))
	for _, file := range files {
		rel := filepath.ToSlash(filepath.Clean(file.Path()))
		src := filepath.Join(dir, filepath.FromSlash(rel))
		abs, err := filepath.Abs(src)
		if err != nil {
			return nil, fmt.Errorf("内置下载器文件路径无效：%s", file.Path())
		}
		archive = append(archive, archiveFile{Rel: rel, Src: abs})
	}
	if len(archive) == 0 {
		return nil, fmt.Errorf("内置下载器未返回可用文件")
	}
	return archive, nil
}

func (n *nativeDownloader) control(ctx context.Context, ref taskRef, pause bool) (string, error) {
	if nativeTaskHash(ref.TaskID) == "" {
		return "", fmt.Errorf("内置 BT 任务标识无效")
	}
	err := n.withSession(ctx, func(session *raintorrent.Session) error {
		if db != nil {
			var shared int
			if err := db.QueryRow("SELECT COUNT(*) FROM items WHERE downloader='native' AND task_id=? AND id<>?", ref.TaskID, ref.ID).Scan(&shared); err != nil {
				return err
			}
			if shared > 0 {
				return errNativeSharedTask
			}
		}
		if pause && session.GetTorrent(ref.TaskID) == nil {
			return nil // An unloaded torrent is already stopped.
		}
		torrent := session.GetTorrent(strings.ToLower(ref.TaskID))
		if torrent == nil {
			var err error
			torrent, err = nativeEnsureTorrent(session, ref)
			if err != nil {
				return err
			}
		}
		if torrent == nil {
			return fmt.Errorf("任务在内置 BT 下载器中不存在")
		}
		if pause {
			return nativePause(torrent)
		}
		return nativeResume(torrent)
	})
	if err != nil {
		return "", err
	}
	return strings.ToLower(ref.TaskID), nil
}

func nativePause(torrent *raintorrent.Torrent) error {
	if torrent.Stats().Status == raintorrent.Stopped {
		return nil
	}
	if err := torrent.Stop(); err != nil {
		return err
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		stats := torrent.Stats()
		if stats.Error != nil {
			return fmt.Errorf("内置下载器任务异常：%w", stats.Error)
		}
		if stats.Status == raintorrent.Stopped {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("下载器尚未确认暂停")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func nativeResume(torrent *raintorrent.Torrent) error {
	if err := nativeStartTorrent(torrent); err != nil {
		return err
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		stats := torrent.Stats()
		if stats.Error != nil {
			return fmt.Errorf("内置下载器任务异常：%w", stats.Error)
		}
		if stats.Status != raintorrent.Stopped {
			return nil
		}
		if files, err := torrent.Files(); err == nil {
			// StopAfterMetadata 在元数据到达后主动停止；继续时再次启动，
			// 让它进入分配/下载阶段。
			if err := validateNativeFiles(torrent.Dir(), files); err != nil {
				return err
			}
			return torrent.Start()
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("下载器尚未确认继续")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (n *nativeDownloader) remove(ctx context.Context, taskID string) error {
	taskID = strings.ToLower(taskID)
	if nativeTaskHash(taskID) == "" {
		return nil
	}
	return n.withSession(ctx, func(session *raintorrent.Session) error {
		if db != nil {
			var owners int
			if err := db.QueryRow("SELECT COUNT(*) FROM items WHERE downloader='native' AND task_id=?", taskID).Scan(&owners); err != nil {
				return err
			}
			if owners > 0 {
				return nil
			}
		}
		return session.ForgetTorrent(taskID)
	})
}

func (n *nativeDownloader) retireSource(ctx context.Context, ref taskRef) error {
	return n.withSession(ctx, func(session *raintorrent.Session) error {
		if nativeTaskHash(ref.TaskID) == "" {
			return fmt.Errorf("内置 BT 任务标识无效")
		}
		return session.ForgetTorrent(ref.TaskID)
	})
}

func validateNativeFiles(root string, files []raintorrent.File) error {
	if len(files) == 0 {
		return fmt.Errorf("种子没有可用文件")
	}
	for _, file := range files {
		if err := validateNativePath(root, file.Path()); err != nil {
			return err
		}
	}
	return nil
}

func validateNativePath(root, raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("文件名不能为空")
	}
	if filepath.IsAbs(raw) || filepath.VolumeName(raw) != "" || strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, `\`) {
		return fmt.Errorf("文件路径不能是绝对路径：%q", raw)
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == '/' || r == '\\' })
	if len(parts) == 0 {
		return fmt.Errorf("文件路径无效：%q", raw)
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("文件路径包含非法组件：%q", raw)
		}
		if strings.ContainsAny(part, `<>:"|?*`) {
			return fmt.Errorf("文件名包含非法字符：%q", part)
		}
		if strings.HasSuffix(part, " ") || strings.HasSuffix(part, ".") {
			return fmt.Errorf("文件名不能以空格或点结尾：%q", part)
		}
		for _, r := range part {
			if unicode.IsControl(r) {
				return fmt.Errorf("文件名包含控制字符")
			}
		}
		if windowsReservedName(part) {
			return fmt.Errorf("文件名是 Windows 保留名称：%q", part)
		}
	}
	cleaned := strings.ReplaceAll(raw, `\`, "/")
	target := filepath.Join(root, filepath.FromSlash(cleaned))
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == "." || !filepath.IsLocal(rel) {
		return fmt.Errorf("文件路径越过下载目录：%q", raw)
	}
	return nil
}

func windowsReservedName(name string) bool {
	base := strings.ToUpper(strings.TrimRight(strings.SplitN(strings.TrimSpace(name), ".", 2)[0], " "))
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || base == "CONIN$" || base == "CONOUT$" {
		return true
	}
	if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
		return true
	}
	return false
}

func startNativeDownloader() error {
	needed, err := nativeNeeded()
	if err != nil {
		return err
	}
	if !needed {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return nativeDownloads.ensure(ctx)
}

func stopNativeDownloader() {
	nativeDownloads.stop()
}

func nativeHealthy() bool {
	return nativeDownloads.healthy()
}

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	_ "modernc.org/sqlite"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	keywordfilter "auto-anime/internal/filter"
	titleparse "auto-anime/internal/title"
	raintorrent "github.com/cenkalti/rain/torrent"
)

var db *sql.DB
var downloadDir = env("DOWNLOAD_DIR", filepath.Join(defaultDataDir(), "downloads"))
var dbFile = defaultDBPath()
var libraryDir = env("LIBRARY_DIR", defaultLibraryDir())
var pollMinutes = 30
var downloadMode = env("DOWNLOAD_MODE", defaultDownloadMode())
var aria2URL = env("ARIA2_URL", "")
var aria2Token = env("ARIA2_TOKEN", "")
var qbitURL = strings.TrimRight(env("QBIT_URL", ""), "/")
var qbitUser = env("QBIT_USER", "")
var qbitPassword = env("QBIT_PASSWORD", "")
var qbitSavePath = env("QBIT_SAVE_PATH", "")
var transmissionURL = strings.TrimRight(env("TRANSMISSION_URL", ""), "/")
var transmissionUser = env("TRANSMISSION_USER", "")
var transmissionPassword = env("TRANSMISSION_PASSWORD", "")
var transmissionDownloadDir = env("TRANSMISSION_DOWNLOAD_DIR", "")

// Download paths inside the downloader containers and inside this container
// usually differ; the *_LOCAL_DIR side is where completed files are read from.
var aria2Dir = env("ARIA2_DIR", "")
var aria2LocalDir = env("ARIA2_LOCAL_DIR", "")
var qbitLocalDir = env("QBIT_LOCAL_DIR", "")
var transmissionLocalDir = env("TRANSMISSION_LOCAL_DIR", "")

// Keep completed downloader files for seeding by default. Disabling this only
// removes source files after the archived copy is committed successfully.
var keepSourceFiles = envBool("KEEP_SOURCE_FILES", true)
var downloaderPollSeconds = 10
var webhookURL = strings.TrimSpace(env("WEBHOOK_URL", ""))
var listenPort = env("PORT", "8000")
var webUser = env("WEB_USER", "")
var webPassword = env("WEB_PASSWORD", "")
var httpClient = newClient(60 * time.Second)

type Feed struct {
	CompletionNote     string `json:"completion_note"`
	EpisodeOffset      *int   `json:"episode_offset"`
	ID                 int    `json:"id"`
	Name               string `json:"name"`
	URL                string `json:"url"`
	Include            string `json:"include_words"`
	Exclude            string `json:"exclude_words"`
	Auto               bool   `json:"auto_download"`
	Enabled            bool   `json:"enabled"`
	LastSync           string `json:"last_sync"`
	LastError          string `json:"last_error"`
	TargetTitle        string `json:"target_title"`
	StorageDir         string `json:"storage_dir"`
	SeasonFilter       int    `json:"season_filter"`
	AllowUnknownSeason bool   `json:"allow_unknown_season"`
}
type FeedInput struct {
	EpisodeOffset      *int    `json:"episode_offset"`
	Name               string  `json:"name"`
	URL                string  `json:"url"`
	Include            string  `json:"include_words"`
	Exclude            string  `json:"exclude_words"`
	Auto               bool    `json:"auto_download"`
	Enabled            *bool   `json:"enabled"`
	TargetTitle        *string `json:"target_title"`
	StorageDir         *string `json:"storage_dir"`
	SeasonFilter       *int    `json:"season_filter"`
	AllowUnknownSeason *bool   `json:"allow_unknown_season"`
}
type webhookEvent struct {
	Event    string `json:"event"`
	ItemID   int    `json:"item_id"`
	Title    string `json:"title"`
	Link     string `json:"link"`
	Status   string `json:"status,omitempty"`
	FeedName string `json:"feed_name,omitempty"`
	Time     string `json:"time"`
}
type PreviewInput struct {
	URL                string `json:"url"`
	Include            string `json:"include_words"`
	Exclude            string `json:"exclude_words"`
	SeasonFilter       int    `json:"season_filter"`
	AllowUnknownSeason bool   `json:"allow_unknown_season"`
}
type backupData struct {
	Version       int                 `json:"version"`
	ExportedAt    string              `json:"exported_at"`
	Feeds         []Feed              `json:"feeds"`
	Items         []map[string]any    `json:"items"`
	Animes        []animeRow          `json:"animes,omitempty"`
	AnimeStorages []animeStorageEntry `json:"anime_storages,omitempty"`
}
type animeStorageEntry struct {
	AnimeTitle string `json:"anime_title"`
	StorageDir string `json:"storage_dir"`
	Season     int    `json:"season"`
}
type restoreRequest struct {
	Version       *int                `json:"version"`
	Feeds         []Feed              `json:"feeds"`
	Animes        []animeRow          `json:"animes"`
	AnimeStorages []animeStorageEntry `json:"anime_storages"`
}
type opmlDocument struct {
	Body struct {
		Outlines []opmlOutline `xml:"outline"`
	} `xml:"body"`
}
type opmlOutline struct {
	Text     string        `xml:"text,attr"`
	Title    string        `xml:"title,attr"`
	XMLURL   string        `xml:"xmlUrl,attr"`
	Children []opmlOutline `xml:"outline"`
}
type rssNode struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Pub         string `xml:"pubDate"`
	Description string `xml:"description"`
	Enclosure   struct {
		URL string `xml:"url,attr"`
	} `xml:"enclosure"`
}
type rss struct {
	XMLName      xml.Name  `xml:""`
	Items        []rssNode `xml:"channel>item"`
	RootItems    []rssNode `xml:"item"`
	Links        []rssLink `xml:"link"`
	ChannelLinks []rssLink `xml:"channel>link"`
	Entries      []struct {
		Title string `xml:"title"`
		Links []struct {
			Href string `xml:"href,attr"`
			Rel  string `xml:"rel,attr"`
		} `xml:"link"`
		Pub         string `xml:"published"`
		Updated     string `xml:"updated"`
		Description string `xml:"summary"`
	} `xml:"entry"`
}

type rssLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

// defaultDownloadMode 是未设置 DOWNLOAD_MODE 时的默认下载方式。Windows 本机与
// 容器镜像都默认使用进程内 BT 下载器，不再要求随包 qBittorrent（镜像不再安装，
// Windows 桌面包只作旧版回退）；源码运行保持 direct，需要时用 DOWNLOAD_MODE 指定。
func defaultDownloadMode() string {
	if runtime.GOOS == "windows" {
		return "native"
	}
	return "direct"
}

// defaultDataDir 是未配置环境变量时数据目录的默认值：容器与 Linux 服务器沿用 /data
// （与镜像、compose 和文档一致），Windows 本机改成程序同级的 data\——「/data」这类有根
// 无盘符的路径在 Windows 上按当前盘解析成 <盘根>:\data，既反直觉，盘根不可写时还会
// 直接启动失败。
func defaultDataDir() string {
	if runtime.GOOS == "windows" {
		if dir := windowsDataDir(); dir != "" {
			return dir
		}
	}
	return "/data"
}

// windowsProgramDir 给出 Windows 本机的程序目录：exe 所在目录；`go run`/`go test`
// 的程序在临时构建目录里（退出即清理），此时退回当前目录，否则开发时的数据会跟着
// 临时目录一起消失。
func windowsProgramDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	dir := filepath.Dir(exe)
	if isTemporaryBuildDir(dir) {
		cwd, err := os.Getwd()
		if err != nil || cwd == "" {
			return ""
		}
		dir = cwd
	}
	return dir
}

// windowsDataDir 给出 Windows 本机的默认数据目录：程序同级的 data\。
func windowsDataDir() string {
	dir := windowsProgramDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "data")
}

// defaultLibraryDir 是未配置 LIBRARY_DIR 时媒体库根目录的默认值。容器与 Linux 服务
// 端沿用空值（归档落下载目录，部署模板显式设置 /media）；Windows 桌面版结构与容器
// 不同——没有部署时的卷映射，因此给归档一个程序同级的 media\，与下载器源文件所在
// 的 data\ 分开，媒体服务器可以直接扫描。
func defaultLibraryDir() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	dir := windowsProgramDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "media")
}

// isTemporaryBuildDir 判断目录是不是 go 工具的临时构建目录。
func isTemporaryBuildDir(dir string) bool {
	if strings.HasPrefix(filepath.Base(dir), "go-build") {
		return true
	}
	tmp := os.TempDir()
	return tmp != "" && strings.HasPrefix(filepath.Clean(dir), filepath.Clean(tmp))
}

// defaultDBPath 是未设置 DB_PATH 时的数据库路径，跟着数据目录走。
func defaultDBPath() string {
	return filepath.Join(defaultDataDir(), "auto-anime.db")
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func jsonOut(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}
func notify(event string, id int, status string) {
	if webhookURL == "" {
		return
	}
	var title, link, feed string
	if db.QueryRow("SELECT items.title,items.link,COALESCE(feeds.name,'') FROM items LEFT JOIN feeds ON feeds.id=items.feed_id WHERE items.id=?", id).Scan(&title, &link, &feed) != nil {
		return
	}
	body, _ := json.Marshal(webhookEvent{Event: event, ItemID: id, Title: title, Link: link, Status: status, FeedName: feed, Time: time.Now().UTC().Format(time.RFC3339)})
	go func() {
		req, e := http.NewRequest("POST", webhookURL, bytes.NewReader(body))
		if e != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		res, e := httpClient.Do(req)
		if e == nil {
			res.Body.Close()
		}
	}()
}
func initDB() {
	if n, e := strconv.Atoi(env("POLL_MINUTES", "30")); e == nil && n >= 5 {
		pollMinutes = n
	}
	if n, e := strconv.Atoi(env("DOWNLOADER_POLL_SECONDS", "10")); e == nil && n >= 5 {
		downloaderPollSeconds = n
	}
	var e error
	// dbFile 是最终使用的数据库路径：启动日志用它说明在读哪个库；封面、内置下载器与更新
	// 暂存的默认目录同样按「数据库同级」计算（各自用 DB_PATH + defaultDBPath() 推导）。
	dbFile = env("DB_PATH", defaultDBPath())
	if err := os.MkdirAll(filepath.Dir(dbFile), 0755); err != nil {
		// 目录建不出来时 sqlite 只会给出难以理解的错误，这里先把原因说清楚。
		log.Printf("数据目录不可用 %s: %v", filepath.Dir(dbFile), err)
	}
	db, e = sql.Open("sqlite", dbFile)
	if e != nil {
		log.Fatalf("打开数据库失败 %s: %v", dbFile, e)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS feed_episode_totals(anime_id INTEGER NOT NULL,season INTEGER NOT NULL,lookup_key TEXT NOT NULL,subject_id INTEGER NOT NULL,total INTEGER NOT NULL,checked_at TEXT NOT NULL,PRIMARY KEY(anime_id,season))`); err != nil {
		log.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS completed_tasks(item_id INTEGER PRIMARY KEY,downloader TEXT NOT NULL,task_id TEXT NOT NULL,submitted_at INTEGER NOT NULL,connection TEXT NOT NULL)`); err != nil {
		log.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS source_cleanup(item_id INTEGER PRIMARY KEY,downloader TEXT NOT NULL,task_id TEXT NOT NULL,submitted_at INTEGER NOT NULL,archive_path TEXT NOT NULL,files TEXT NOT NULL,source_root TEXT NOT NULL,stopped INTEGER NOT NULL DEFAULT 0)`); err != nil {
		log.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS archive_commits(item_id INTEGER PRIMARY KEY,signature TEXT NOT NULL,file_path TEXT NOT NULL,marker TEXT NOT NULL,saved_at TEXT NOT NULL)`); err != nil {
		log.Fatal(err)
	}
	_, e = db.Exec(`CREATE TABLE IF NOT EXISTS feeds(id INTEGER PRIMARY KEY,name TEXT NOT NULL,url TEXT UNIQUE NOT NULL,include_words TEXT DEFAULT '',exclude_words TEXT DEFAULT '',auto_download INTEGER DEFAULT 0,enabled INTEGER DEFAULT 1,last_sync TEXT DEFAULT '',last_error TEXT DEFAULT '',created_at TEXT);CREATE TABLE IF NOT EXISTS items(id INTEGER PRIMARY KEY,feed_id INTEGER,title TEXT,link TEXT UNIQUE,published TEXT,description TEXT,status TEXT DEFAULT '待下载',file_path TEXT,created_at TEXT);CREATE TABLE IF NOT EXISTS anime_meta(anime_title TEXT PRIMARY KEY,subject_id INTEGER DEFAULT 0,name TEXT DEFAULT '',name_cn TEXT DEFAULT '',cover_url TEXT DEFAULT '',cover_file TEXT DEFAULT '',source TEXT DEFAULT '',updated_at TEXT);CREATE TABLE IF NOT EXISTS web_sessions(token_hash TEXT PRIMARY KEY,user TEXT NOT NULL,created_at TEXT DEFAULT '',expires_at TEXT NOT NULL);CREATE TABLE IF NOT EXISTS anime_storage(anime_title TEXT PRIMARY KEY,storage_dir TEXT DEFAULT '',season INTEGER DEFAULT 1,updated_at TEXT);CREATE TABLE IF NOT EXISTS settings(key TEXT PRIMARY KEY,value TEXT NOT NULL);CREATE TABLE IF NOT EXISTS animes(id INTEGER PRIMARY KEY,title TEXT UNIQUE NOT NULL,search_name TEXT DEFAULT '',bangumi_id INTEGER DEFAULT 0,season INTEGER DEFAULT 0,storage_dir TEXT DEFAULT '',include_rules TEXT DEFAULT '',exclude_rules TEXT DEFAULT '',created_at TEXT DEFAULT '',updated_at TEXT DEFAULT '');`)
	if e != nil {
		log.Fatalf("初始化数据库失败 %s: %v", dbFile, e)
	}
	if _, err := db.Exec(`CREATE TRIGGER IF NOT EXISTS delete_feed_episode_totals AFTER DELETE ON animes BEGIN DELETE FROM feed_episode_totals WHERE anime_id=OLD.id; END`); err != nil {
		log.Fatal(err)
	}
	for _, col := range []string{"enabled INTEGER DEFAULT 1", "last_sync TEXT DEFAULT ''", "last_error TEXT DEFAULT ''", "target_title TEXT DEFAULT ''", "storage_dir TEXT DEFAULT ''", "season_filter INTEGER DEFAULT 0", "allow_unknown_season INTEGER DEFAULT 0", "episode_offset INTEGER DEFAULT 0", "completion_note TEXT NOT NULL DEFAULT ''"} {
		name := strings.Fields(col)[0]
		var count int
		db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('feeds') WHERE name=?", name).Scan(&count)
		if count == 0 {
			if _, e = db.Exec("ALTER TABLE feeds ADD COLUMN " + col); e != nil {
				log.Fatal(e)
			}
		}
	}
	for _, col := range []string{"anime_title TEXT DEFAULT ''", "episode TEXT DEFAULT ''", "quality TEXT DEFAULT ''", "fansub_group TEXT DEFAULT ''", "downloader TEXT DEFAULT ''", "task_id TEXT DEFAULT ''", "task_error TEXT DEFAULT ''", "progress INTEGER DEFAULT 0", "anime_id INTEGER DEFAULT 0", "season INTEGER DEFAULT 0", "season_auto INTEGER DEFAULT 0", "manual INTEGER DEFAULT 0", "task_submitted_at INTEGER DEFAULT 0", "page_url TEXT DEFAULT ''"} {
		name := strings.Fields(col)[0]
		var count int
		db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('items') WHERE name=?", name).Scan(&count)
		if count == 0 {
			if _, e = db.Exec("ALTER TABLE items ADD COLUMN " + col); e != nil {
				log.Fatal(e)
			}
		}
	}
	var weekdayColumn int
	if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('animes') WHERE name='update_weekday'").Scan(&weekdayColumn); err != nil {
		log.Fatal(err)
	}
	if weekdayColumn == 0 {
		if _, err := db.Exec("ALTER TABLE animes ADD COLUMN update_weekday INTEGER NOT NULL DEFAULT 0 CHECK(update_weekday BETWEEN 0 AND 7)"); err != nil {
			log.Fatal(err)
		}
	}
	// Add columns only when upgrading an older database; avoid noisy duplicate-column errors.
	for _, spec := range []struct{ name, ddl string }{
		{"rating", "rating REAL DEFAULT 0"},
		{"summary", "summary TEXT DEFAULT ''"},
		{"voices", "voices TEXT DEFAULT '[]'"},
		{"match_key", "match_key TEXT DEFAULT ''"},
		{"broadcast_weekday", "broadcast_weekday INTEGER NOT NULL DEFAULT 0"},
		{"broadcast_subject_id", "broadcast_subject_id INTEGER NOT NULL DEFAULT 0"},
		{"broadcast_checked_at", "broadcast_checked_at TEXT NOT NULL DEFAULT ''"},
		{"episode_total", "episode_total INTEGER NOT NULL DEFAULT 0"},
		{"episode_season", "episode_season INTEGER NOT NULL DEFAULT 0"},
		{"episode_subject_id", "episode_subject_id INTEGER NOT NULL DEFAULT 0"},
	} {
		var exists int
		if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('anime_meta') WHERE name=?", spec.name).Scan(&exists); err == nil && exists == 0 {
			if _, err := db.Exec("ALTER TABLE anime_meta ADD COLUMN " + spec.ddl); err != nil {
				log.Printf("add anime_meta.%s: %v", spec.name, err)
			}
		}
	}
	for _, ddl := range []string{
		"CREATE TRIGGER IF NOT EXISTS archive_commits_item_deleted AFTER DELETE ON items BEGIN DELETE FROM archive_commits WHERE item_id=OLD.id; END",
		"CREATE INDEX IF NOT EXISTS items_anime_id_id ON items(anime_id,id)",
		"CREATE INDEX IF NOT EXISTS items_status_id ON items(status,id)",
		"CREATE INDEX IF NOT EXISTS items_feed_id ON items(feed_id)",
	} {
		if _, err := db.Exec(ddl); err != nil {
			log.Fatal(err)
		}
	}
}

type feedItem struct{ Title, Link, Pub, Description, PageURL string }

func syncFeedContext(ctx context.Context, id int) (int, error) {
	processCompletedFeeds()
	var stopped bool
	if err := db.QueryRow("SELECT enabled=0 AND completion_note<>'' FROM feeds WHERE id=?", id).Scan(&stopped); err != nil {
		return 0, err
	}
	if stopped {
		return 0, nil
	}
	animeCleanupMu.RLock()
	defer animeCleanupMu.RUnlock()
	var f Feed
	var a int
	e := db.QueryRow("SELECT id,name,url,include_words,exclude_words,auto_download,COALESCE(target_title,''),COALESCE(storage_dir,''),COALESCE(season_filter,0),COALESCE(allow_unknown_season,0),episode_offset FROM feeds WHERE id=?", id).Scan(&f.ID, &f.Name, &f.URL, &f.Include, &f.Exclude, &a, &f.TargetTitle, &f.StorageDir, &f.SeasonFilter, &f.AllowUnknownSeason, &f.EpisodeOffset)
	if e != nil {
		return 0, e
	}
	f.Auto = a == 1
	if strings.HasPrefix(f.URL, "aggregate:") && f.EpisodeOffset == nil {
		f.EpisodeOffset = new(int) // Aggregation returns candidates, never a complete numbering baseline.
	}
	fetched, e := fetchSubscription(ctx, f.URL)
	if e != nil {
		db.Exec("UPDATE feeds SET last_error=? WHERE id=?", e.Error(), id)
		return 0, e
	}
	n := 0
	var writeErr error
	filter := keywordfilter.New(f.Include, f.Exclude)
	if f.EpisodeOffset == nil {
		if fetched.Incomplete {
			e := fmt.Errorf("历史资源未读取完整，暂不推断集数偏移：%s", fetched.Warning)
			db.Exec("UPDATE feeds SET last_error=? WHERE id=?", e.Error(), id)
			return 0, e
		}
		matched := filteredFeedItems(fetched.Entries, f.Include, f.Exclude, f.SeasonFilter, f.AllowUnknownSeason)
		offset := inferEpisodeOffset(matched)
		if len(matched) == 0 {
			return 0, nil // An empty feed has not supplied a numbering baseline yet.
		}
		if offset == nil {
			offset = new(int)
		}
		if _, err := db.Exec("UPDATE feeds SET episode_offset=? WHERE id=? AND episode_offset IS NULL", *offset, id); err != nil {
			return 0, err
		}
		if err := db.QueryRow("SELECT episode_offset FROM feeds WHERE id=?", id).Scan(&f.EpisodeOffset); err != nil {
			return 0, err
		}
	}
	for _, it := range feedItemsFromEntries(fetched.Entries) {
		if err := ctx.Err(); err != nil {
			return n, err
		}
		if err := db.QueryRow("SELECT enabled=0 AND completion_note<>'' FROM feeds WHERE id=?", id).Scan(&stopped); err != nil {
			return n, err
		}
		if stopped {
			return n, nil // A late RSS response must not restart a completed subscription.
		}
		if it.Title == "" || it.Link == "" || !filter.Allows(it.Title) || !titleparse.AllowsSeason(it.Title, f.SeasonFilter, f.AllowUnknownSeason) {
			continue
		}
		// Older rows kept only the download URL. Fill the original page on the
		// next sync without changing task identity, progress or user corrections.
		if err := backfillItemPageURL(it.Link, it.PageURL); err != nil {
			writeErr = fmt.Errorf("资源页地址保存失败: %w", err)
			continue
		}
		// 已收录的资源不能因更改订阅归属而额外创建空番剧。
		if f.TargetTitle != "" {
			var exists bool
			if err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM items WHERE link=?)", it.Link).Scan(&exists); err != nil {
				writeErr = fmt.Errorf("条目查询失败: %w", err)
				continue
			}
			if exists {
				continue
			}
		}
		info := titleparse.Extract(it.Title)
		info.Episode = offsetEpisode(info.Episode, *f.EpisodeOffset)
		// 多来源归并：某部番剧的规则命中原始标题时，条目归入该番剧，
		// 不再按本地识别标题另立门户。
		animeID := int64(0)
		animeTitle := info.Title
		ownSeries := false // 番剧来自本订阅的目标或识别结果，而非归并规则
		if f.TargetTitle != "" {
			animeTitle = f.TargetTitle
			animeID, e = ensureAnime(animeTitle)
			if e != nil {
				writeErr = fmt.Errorf("番剧写入失败: %w", e)
				continue
			}
			ownSeries = true
		} else if id, title := matchAnimeRule(it.Title); id > 0 {
			animeID, animeTitle = id, title
		} else if animeTitle != "" {
			animeID, _ = ensureAnime(animeTitle)
			ownSeries = true
		}
		// 标题里解析出的季写进 season_auto：items.season 留给用户（0 = 跟随番剧设置），
		// 归档时季号优先级见 resolveSeason。
		r, e := db.Exec("INSERT OR IGNORE INTO items(feed_id,title,link,published,description,anime_title,anime_id,episode,quality,fansub_group,season_auto,created_at,page_url) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)", id, it.Title, it.Link, it.Pub, it.Description, animeTitle, animeID, info.Episode, info.Quality, info.Group, info.Season, time.Now().Format(time.RFC3339), it.PageURL)
		if e != nil {
			// Keep processing the remaining entries, but make sure the
			// failure is visible on the feed and returned to the caller.
			if writeErr == nil {
				writeErr = fmt.Errorf("条目写入失败: %w", e)
			}
			log.Printf("订阅 %d 条目写入失败: %v", id, e)
			continue
		}
		x, _ := r.RowsAffected()
		n += int(x)
		if x > 0 && ownSeries {
			// 订阅选择的保存位置；只在番剧还没有指定位置时写入。
			applyFeedStorageOnSync(f.StorageDir, animeTitle)
		}
		if x > 0 && f.Auto {
			var itemID int
			db.QueryRow("SELECT id FROM items WHERE link=?", it.Link).Scan(&itemID)
			go downloadItem(itemID)
		}
		if x > 0 {
			var itemID int64
			if db.QueryRow("SELECT id FROM items WHERE link=?", it.Link).Scan(&itemID) == nil {
				notify("new_item", int(itemID), "待下载")
			}
		}
	}
	if writeErr == nil && fetched.Incomplete {
		writeErr = fmt.Errorf("历史资源未读取完整：%s", fetched.Warning)
	}
	lastError := ""
	if writeErr != nil {
		lastError = writeErr.Error()
	}
	db.Exec("UPDATE feeds SET last_sync=?,last_error=? WHERE id=?", time.Now().Format(time.RFC3339), lastError, id)
	// Cover lookups run in the background; a slow metadata source must not
	// stretch the sync request.
	enqueueMissingMeta()
	return n, writeErr
}
func downloadItem(id int) {
	queued, err := queueItemDownload(id, false)
	if err != nil || !queued {
		return
	}
	runQueuedDownload(id)
}

func runQueuedDownload(id int) {
	l, release := downloadLock(id)
	defer release()
	l.worker.Lock()
	defer l.worker.Unlock()
	for {
		l.operation.Lock()
		ctx, finish := l.transferContext()
		if ctx.Err() != nil {
			finish()
			l.operation.Unlock()
			return
		}
		claimed, waiting, claimErr := claimDownload(id)
		if claimed {
			performDownload(ctx, id)
		}
		finish()
		l.operation.Unlock()
		if claimErr != nil || claimed || !waiting {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// The caller holds the item's operation lock through submission / archival.
// Cancellation only interrupts direct HTTP I/O; accepted external submissions
// are persisted before a pause or removal can control that exact task.
func performDownload(ctx context.Context, id int) {
	cfg := runtimeConfig()
	clearTransfer(id)
	defer clearTransfer(id)
	if ctx.Err() != nil {
		return
	}
	if path, err := pendingArchivePath(id); err != nil {
		failDownload(id, "归档提交恢复失败："+err.Error())
		return
	} else if path != "" {
		if err := commitSavedArchive(id, path); err != nil {
			log.Printf("条目 %d 归档记录提交失败: %v", id, err)
		}
		return
	}
	item, err := readControlledItem(id)
	if err != nil {
		return
	}
	if item.Downloader != "" {
		taskID, err := controlDownloaderTask(item.taskRef, false)
		if err != nil {
			// A lost RPC response does not prove that the downloader stayed
			// paused. Keep the slot until polling confirms its actual state.
			db.Exec("UPDATE items SET task_error=? WHERE id=?", "继续请求未确认，将自动检查下载器："+err.Error(), id)
			wakeTaskPoller()
			return
		}
		db.Exec("UPDATE items SET status='下载中',task_id=?,task_error='' WHERE id=?", taskID, id)
		wakeTaskPoller()
		return
	}
	var link string
	if db.QueryRow("SELECT link FROM items WHERE id=?", id).Scan(&link) != nil {
		return
	}
	u, e := url.Parse(link)
	if e != nil || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "magnet") {
		failDownload(id, "资源地址无效")
		return
	}
	if vErr := validateDownloadTarget(); vErr != nil {
		failDownload(id, vErr.Error())
		return
	}
	if cfg.downloadMode == "native" && (u.Scheme == "magnet" || torrentLink(link)) {
		submitNativeDownload(id, link)
		return
	}
	if cfg.downloadMode == "qbittorrent" && cfg.qbitURL != "" {
		submitQBittorrent(id, link)
		return
	}
	if cfg.downloadMode == "transmission" && cfg.transmissionURL != "" {
		taskID, e := transmissionAdd(link)
		if e != nil {
			failDownload(id, "Transmission 提交失败："+e.Error())
			return
		}
		setSubmitted(id, "transmission", taskID, "已发送到 Transmission")
		return
	}
	if cfg.downloadMode == "aria2" && cfg.aria2URL != "" {
		taskID, e := aria2Add(link)
		if e != nil {
			failDownload(id, "Aria2 提交失败："+e.Error())
			return
		}
		setSubmitted(id, "aria2", taskID, "已发送到 Aria2")
		return
	}
	// No BT downloader configured: a magnet or torrent payload can never
	// become the episode video here, so it waits instead of being archived
	// as if it were finished.
	if u.Scheme == "magnet" {
		waitForDownloader(id, "磁力链接需要配置 BT 下载器")
		return
	}
	if torrentLink(link) {
		waitForDownloader(id, "种子文件需要配置 BT 下载器")
		return
	}
	path, e := fetchArchiveContext(ctx, id, link)
	if e == nil {
		if err := commitSavedArchive(id, path); err != nil {
			log.Printf("条目 %d 归档记录提交失败，将在重启后恢复: %v", id, err)
		}
		return
	}
	if ctx.Err() != nil {
		// The waiting control request persists the final paused/deleted state.
		return
	}
	if errors.Is(e, errTorrentResource) {
		if cfg.downloadMode == "native" {
			submitNativeDownload(id, link)
			return
		}
		waitForDownloader(id, "资源是种子文件，需要配置 BT 下载器")
		return
	}
	failDownload(id, "下载失败："+e.Error())
}

func submitQBittorrent(id int, link string) {
	tag := qbitTagFor(id)
	hash := magnetInfoHash(link)
	if hash == "" && torrentLink(link) {
		if data, e := fetchTorrentBytes(link); e == nil {
			hash = torrentInfoHash(data)
		}
	}
	client, err := qbitLogin()
	if err != nil {
		failDownload(id, "qBittorrent 连接失败："+err.Error())
		return
	}
	if e := client.add(link, tag); e != nil {
		failDownload(id, "qBittorrent 提交失败："+e.Error())
		return
	}
	setSubmitted(id, "qbittorrent", hash, "已发送到 qBittorrent")
}

// submitNativeDownload 把磁力或种子交给进程内 BT 引擎。任务标识使用小写
// info hash 与条目 ID 组合，避免不同条目互相控制；普通 HTTP 直链仍走直链。
func submitNativeDownload(id int, link string) {
	hash := magnetInfoHash(link)
	var data []byte
	if hash == "" {
		var err error
		data, err = fetchTorrentBytes(link)
		if err != nil {
			failDownload(id, "获取种子文件失败："+err.Error())
			return
		}
		hash = torrentInfoHash(data)
	}
	if !nativeInfoHashPattern.MatchString(hash) {
		failDownload(id, "无法识别种子标识；暂不支持 BT v2 种子或磁力链接")
		return
	}
	ref := taskRef{ID: id, Downloader: "native", TaskID: nativeTaskID(hash, id), SubmittedAt: time.Now().Unix()}
	if err := nativeDownloads.add(context.Background(), ref, link, data); err != nil {
		if errors.Is(err, raintorrent.ErrNoFreePort) {
			setSubmitted(id, "native", ref.TaskID, "下载中")
			return
		}
		failDownload(id, "内置下载器提交失败："+err.Error())
		return
	}
	setSubmitted(id, "native", ref.TaskID, "下载中")
}

func setSubmitted(id int, downloaderName, taskID, status string) {
	if _, err := db.Exec("UPDATE items SET status=?,downloader=?,task_id=?,task_error='',progress=0,task_submitted_at=? WHERE id=?", status, downloaderName, taskID, time.Now().Unix(), id); err != nil {
		return
	}
	notify("download_submitted", id, status)
	wakeTaskPoller()
}

func failDownload(id int, reason string) {
	db.Exec("UPDATE items SET status='下载失败',task_error=?,downloader='',task_id='',progress=0 WHERE id=?", reason, id)
	notify("download_failed", id, "下载失败")
}

func waitForDownloader(id int, reason string) {
	db.Exec("UPDATE items SET status='等待下载器',task_error=?,downloader='',task_id='',progress=0 WHERE id=?", reason, id)
}

// sanitizeStorageDir validates a user-provided series folder: absolute and
// free of ".." components in the original input; empty means "use the default
// location".
func sanitizeStorageDir(raw string) (string, error) {
	d := strings.TrimSpace(raw)
	if d == "" {
		return "", nil
	}
	if runtime.GOOS == "windows" {
		// Windows 本机没有容器：只有盘符路径或 UNC 共享是完整绝对路径。"/番剧"、
		// "\\番剧" 会按当前盘解析，可能让文件落在用户没预期的位置，直接拒绝并给出示例。
		if !windowsAbsolutePath(d) {
			return "", fmt.Errorf("存储位置必须是绝对路径，例如 %s", storageExampleDir())
		}
	} else if !filepath.IsAbs(d) && !strings.HasPrefix(d, "/") && !strings.HasPrefix(d, "\\") {
		// "/volume1/..." is absolute inside the Linux container even though
		// Windows builds report it as relative.
		return "", fmt.Errorf("存储位置必须是绝对路径，例如 %s", storageExampleDir())
	}
	for _, part := range strings.Split(filepath.ToSlash(d), "/") {
		if part == ".." {
			return "", fmt.Errorf("存储位置不能包含 ..")
		}
	}
	// Persist forward slashes while retaining the host filesystem's volume
	// semantics, including Windows UNC shares.
	return cleanStoragePath(d), nil
}

// warnStoredPathsForPlatform 在启动时提示与当前平台不匹配的历史保存位置：从 Linux/
// 容器数据库搬到 Windows 后，"/vol1/1000/番剧" 这类路径会按当前盘解析，归档写到用户
// 没预期的位置。这里只提示如何处理，不直接改写数据库（用户可能仍要参考原值）。
func warnStoredPathsForPlatform() {
	if runtime.GOOS != "windows" {
		return
	}
	var items []string
	collect := func(kind, name, dir string) {
		if _, err := sanitizeStorageDir(dir); err != nil {
			items = append(items, fmt.Sprintf("%s「%s」：%s", kind, name, strings.TrimSpace(dir)))
		}
	}
	if rows, err := db.Query("SELECT COALESCE(name,''),COALESCE(storage_dir,'') FROM feeds WHERE COALESCE(storage_dir,'')<>'' ORDER BY id"); err == nil {
		for rows.Next() {
			var name, dir string
			if rows.Scan(&name, &dir) == nil {
				collect("订阅", name, dir)
			}
		}
		rows.Close()
	}
	if rows, err := db.Query("SELECT title,COALESCE(storage_dir,'') FROM animes WHERE COALESCE(storage_dir,'')<>'' ORDER BY id"); err == nil {
		for rows.Next() {
			var title, dir string
			if rows.Scan(&title, &dir) == nil {
				collect("番剧", title, dir)
			}
		}
		rows.Close()
	}
	if len(items) == 0 {
		return
	}
	log.Printf("以下保存位置不适用于 Windows（缺少盘符或 UNC 共享），归档会写到当前盘根：%s；请改为 %s 形式", strings.Join(items, "；"), storageExampleDir())
}

// animeStorageAPI keeps the legacy /api/anime/{title}/storage endpoint working
// on top of the animes table (which absorbed anime_storage at migration).
func animeStorageAPI(w http.ResponseWriter, r *http.Request) {
	animeCleanupMu.RLock()
	defer animeCleanupMu.RUnlock()
	title := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/anime/"), "/storage"))
	if title == "" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		dir, stored := "", 0
		db.QueryRow("SELECT COALESCE(storage_dir,''),COALESCE(season,0) FROM animes WHERE title=?", title).Scan(&dir, &stored)
		season := stored
		if season < 1 {
			season = 1
		}
		def := filepath.Join(archiveBaseRoot(), safeComponent(title))
		effective := def
		if clean, err := sanitizeStorageDir(dir); err == nil && clean != "" {
			effective = clean
		}
		w.Header().Set("Cache-Control", "no-store")
		jsonOut(w, map[string]any{"anime_title": title, "storage_dir": dir, "season": season, "default_path": def, "effective_path": effective})
	case http.MethodPut:
		var in struct {
			StorageDir string `json:"storage_dir"`
			Season     int    `json:"season"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in) != nil {
			http.Error(w, "请求参数无效", 400)
			return
		}
		if in.Season == 0 {
			in.Season = 1
		}
		if in.Season < 1 || in.Season > 999 {
			http.Error(w, "季号必须是 1-999", 400)
			return
		}
		clean, err := sanitizeStorageDir(in.StorageDir)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if _, err = ensureAnime(title); err != nil {
			http.Error(w, "保存失败", 500)
			return
		}
		if _, err = db.Exec(`UPDATE animes SET storage_dir=?,season=?,updated_at=? WHERE title=?`,
			clean, in.Season, time.Now().UTC().Format(time.RFC3339), title); err != nil {
			http.Error(w, "保存失败", 500)
			return
		}
		jsonOut(w, map[string]any{"ok": true, "storage_warning": unmappedPathWarning(clean)})
	default:
		http.Error(w, "方法不支持", 405)
	}
}

func feeds(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		rows, err := db.Query("SELECT id,name,url,include_words,exclude_words,auto_download,enabled,last_sync,last_error,COALESCE(target_title,''),COALESCE(storage_dir,''),COALESCE(season_filter,0),COALESCE(allow_unknown_season,0),episode_offset,completion_note FROM feeds ORDER BY id DESC")
		if err != nil {
			http.Error(w, "读取订阅失败", 500)
			return
		}
		defer rows.Close()
		out := []Feed{}
		for rows.Next() {
			var f Feed
			var a int
			var enabled int
			rows.Scan(&f.ID, &f.Name, &f.URL, &f.Include, &f.Exclude, &a, &enabled, &f.LastSync, &f.LastError, &f.TargetTitle, &f.StorageDir, &f.SeasonFilter, &f.AllowUnknownSeason, &f.EpisodeOffset, &f.CompletionNote)
			f.Auto = a == 1
			f.Enabled = enabled == 1
			out = append(out, f)
		}
		if err := rows.Err(); err != nil {
			http.Error(w, "读取订阅失败", 500)
			return
		}
		jsonOut(w, out)
		return
	}
	var in FeedInput
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		http.Error(w, "请求参数无效", 400)
		return
	}
	in.Name, in.URL = strings.TrimSpace(in.Name), strings.TrimSpace(in.URL)
	if in.Name == "" {
		http.Error(w, "订阅名称不能为空", 400)
		return
	}
	if !validFeedURL(in.URL) {
		http.Error(w, "RSS 地址无效或聚合订阅规则无效", 400)
		return
	}
	if strings.HasPrefix(in.URL, "aggregate:") {
		id, rules, _ := aggregateSubscription(in.URL)
		in.URL = "aggregate://subject/" + strconv.Itoa(id) + "?" + rules.Encode()
		if in.EpisodeOffset == nil {
			in.EpisodeOffset = new(int) // Search results are not a complete episode-numbering baseline.
		}
	}
	target := ""
	if in.TargetTitle != nil {
		target = sanitizeText(*in.TargetTitle, titleMaxLength)
	}
	season := 0
	if in.SeasonFilter != nil {
		season = *in.SeasonFilter
	}
	if !titleparse.ValidSeasonFilter(season) {
		http.Error(w, "季数条件无效", 400)
		return
	}
	if !validEpisodeOffset(in.EpisodeOffset) {
		http.Error(w, "集数偏移必须在 -9999 到 9999 之间", 400)
		return
	}
	allowUnknown := in.AllowUnknownSeason != nil && *in.AllowUnknownSeason
	// 新建订阅没有旧值，选择的位置直接生效。
	storage, err := feedStorageDir(in.StorageDir)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	result, e := db.Exec("INSERT INTO feeds(name,url,include_words,exclude_words,auto_download,created_at,target_title,storage_dir,season_filter,allow_unknown_season,episode_offset) VALUES(?,?,?,?,?,?,?,?,?,?,?)", in.Name, in.URL, in.Include, in.Exclude, boolInt(in.Auto), time.Now().Format(time.RFC3339), target, storage, season, boolInt(allowUnknown), in.EpisodeOffset)
	if e != nil {
		http.Error(w, "订阅已存在，请在订阅管理中编辑", 409)
		return
	}
	newID, _ := result.LastInsertId()
	storagePath, warning := applySubscriptionStorage(storage, target, "")
	jsonOut(w, map[string]any{"ok": true, "id": newID, "storage_path": storagePath, "storage_warning": warning})
}

// Subscription addresses are either user-provided RSS or a validated aggregate selection.
func validFeedURL(raw string) bool {
	if strings.HasPrefix(raw, "aggregate:") {
		_, _, err := aggregateSubscription(raw)
		return err == nil
	}
	u, err := url.Parse(raw)
	return err == nil && u.Hostname() != "" && (u.Scheme == "http" || u.Scheme == "https")
}

// feedStorageDir validates the subscription save location (番剧根目录) of a
// request: an absent field keeps the stored value, an empty one clears it and
// lets archiving fall back to the media library directory.
func feedStorageDir(field *string) (string, error) {
	if field == nil {
		return "", nil
	}
	clean, err := sanitizeStorageDir(*field)
	if err != nil {
		if strings.Contains(err.Error(), "..") {
			return "", fmt.Errorf("保存位置不能包含 ..")
		}
		return "", fmt.Errorf("保存位置必须是绝对路径，例如 %s", storageExampleDir())
	}
	return clean, nil
}

func preview(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "方法不支持", 405)
		return
	}
	var in PreviewInput
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		http.Error(w, "请求参数无效", 400)
		return
	}
	if !titleparse.ValidSeasonFilter(in.SeasonFilter) {
		http.Error(w, "季数条件无效", 400)
		return
	}
	fetched, err := fetchSubscription(r.Context(), strings.TrimSpace(in.URL))
	if err != nil {
		http.Error(w, "订阅预览失败："+err.Error(), 502)
		return
	}
	items := feedItemsFromEntries(fetched.Entries)
	total := len(items)
	filtered := filteredFeedItems(fetched.Entries, in.Include, in.Exclude, in.SeasonFilter, in.AllowUnknownSeason)
	offset := inferEpisodeOffset(filtered)
	if fetched.Incomplete {
		offset = nil
	}
	if strings.HasPrefix(strings.TrimSpace(in.URL), "aggregate:") {
		offset = new(int)
	}
	count := len(filtered)
	items = filtered
	if len(items) > 20 {
		items = items[:20]
	}
	type previewItem struct {
		Title       string `json:"title"`
		Link        string `json:"link"`
		Published   string `json:"published"`
		Description string `json:"description"`
	}
	out := make([]previewItem, 0, len(items))
	for _, item := range items {
		out = append(out, previewItem{item.Title, item.Link, item.Pub, item.Description})
	}
	jsonOut(w, map[string]any{"items": out, "count": count, "total": total,
		"pages": fetched.Pages, "history": fetched.History, "limited": fetched.Limited, "warning": fetched.Warning, "episode_offset": offset})
}

func backup(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "方法不支持", 405)
		return
	}
	data := backupData{Version: 4, ExportedAt: time.Now().UTC().Format(time.RFC3339), Feeds: []Feed{}, Items: []map[string]any{}}
	tx, err := db.BeginTx(r.Context(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		http.Error(w, "备份失败", 500)
		return
	}
	defer tx.Rollback()
	feedRows, err := tx.QueryContext(r.Context(), "SELECT id,name,url,COALESCE(include_words,''),COALESCE(exclude_words,''),COALESCE(auto_download,0),COALESCE(enabled,1),COALESCE(last_sync,''),COALESCE(last_error,''),COALESCE(target_title,''),COALESCE(storage_dir,''),COALESCE(season_filter,0),COALESCE(allow_unknown_season,0),episode_offset FROM feeds ORDER BY id")
	if err != nil {
		http.Error(w, "备份失败", 500)
		return
	}
	for feedRows.Next() {
		var f Feed
		var auto, enabled int
		if err = feedRows.Scan(&f.ID, &f.Name, &f.URL, &f.Include, &f.Exclude, &auto, &enabled, &f.LastSync, &f.LastError, &f.TargetTitle, &f.StorageDir, &f.SeasonFilter, &f.AllowUnknownSeason, &f.EpisodeOffset); err != nil {
			feedRows.Close()
			http.Error(w, "备份失败", 500)
			return
		}
		f.Auto = auto == 1
		f.Enabled = enabled == 1
		data.Feeds = append(data.Feeds, f)
	}
	err = feedRows.Err()
	feedRows.Close()
	if err != nil {
		http.Error(w, "备份失败", 500)
		return
	}
	rows, err := tx.QueryContext(r.Context(), "SELECT id,COALESCE(feed_id,0),COALESCE(title,''),COALESCE(link,''),COALESCE(published,''),COALESCE(description,''),COALESCE(status,'待下载'),COALESCE(file_path,''),COALESCE(anime_title,''),COALESCE(episode,''),COALESCE(quality,''),COALESCE(fansub_group,''),COALESCE(anime_id,0),COALESCE(season,0),COALESCE(season_auto,0),COALESCE(created_at,'') FROM items ORDER BY id")
	if err != nil {
		http.Error(w, "备份失败", 500)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, feedID int
		var title, link, published, description, status, filePath, animeTitle, episode, quality, group, created string
		var animeID int64
		var season, seasonAuto int
		if err = rows.Scan(&id, &feedID, &title, &link, &published, &description, &status, &filePath, &animeTitle, &episode, &quality, &group, &animeID, &season, &seasonAuto, &created); err != nil {
			http.Error(w, "备份失败", 500)
			return
		}
		data.Items = append(data.Items, map[string]any{"id": id, "feed_id": feedID, "title": title, "link": link, "published": published, "description": description, "status": status, "file_path": filePath, "anime_title": animeTitle, "anime_id": animeID, "season": season, "season_auto": seasonAuto, "episode": episode, "quality": quality, "fansub_group": group, "created_at": created})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		http.Error(w, "备份失败", 500)
		return
	}
	animeRows, err := tx.QueryContext(r.Context(), `SELECT id,title,COALESCE(search_name,''),COALESCE(bangumi_id,0),COALESCE(season,0),COALESCE(storage_dir,''),COALESCE(include_rules,''),COALESCE(exclude_rules,''),COALESCE(updated_at,''),COALESCE(update_weekday,0) FROM animes ORDER BY id`)
	if err != nil {
		http.Error(w, "备份失败", 500)
		return
	}
	for animeRows.Next() {
		var a animeRow
		if err = animeRows.Scan(&a.ID, &a.Title, &a.SearchName, &a.BangumiID, &a.Season, &a.StorageDir, &a.IncludeRules, &a.ExcludeRules, &a.UpdatedAt, &a.UpdateWeekday); err != nil {
			animeRows.Close()
			http.Error(w, "备份失败", 500)
			return
		}
		data.Animes = append(data.Animes, a)
	}
	err = animeRows.Err()
	animeRows.Close()
	if err != nil {
		http.Error(w, "备份失败", 500)
		return
	}
	if err = tx.Commit(); err != nil {
		http.Error(w, "备份失败", 500)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", "attachment; filename=auto-anime-backup.json")
	jsonOut(w, data)
}

func restore(w http.ResponseWriter, r *http.Request) {
	animeCleanupMu.RLock()
	defer animeCleanupMu.RUnlock()
	if r.Method != "POST" {
		http.Error(w, "方法不支持", 405)
		return
	}
	var input restoreRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 5<<20))
	if decoder.Decode(&input) != nil || input.Feeds == nil || (input.Version != nil && *input.Version != 1 && *input.Version != 2 && *input.Version != 3 && *input.Version != 4) {
		http.Error(w, "备份文件无效", 400)
		return
	}
	if decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "备份文件包含额外内容或超过 5 MB", 400)
		return
	}
	for i := range input.Feeds {
		f := &input.Feeds[i]
		if f.EpisodeOffset == nil && (input.Version == nil || *input.Version < 4) {
			f.EpisodeOffset = new(int) // Legacy backups must not renumber their future episodes.
		}
		f.Name, f.URL = strings.TrimSpace(f.Name), strings.TrimSpace(f.URL)
		if f.Name == "" || !validFeedURL(f.URL) || !titleparse.ValidSeasonFilter(f.SeasonFilter) || !validEpisodeOffset(f.EpisodeOffset) {
			http.Error(w, "备份包含无效订阅", 400)
			return
		}
		if strings.HasPrefix(f.URL, "aggregate:") {
			id, rules, _ := aggregateSubscription(f.URL)
			f.URL = "aggregate://subject/" + strconv.Itoa(id) + "?" + rules.Encode()
			if f.EpisodeOffset == nil {
				f.EpisodeOffset = new(int)
			}
		}
	}
	tx, err := db.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "恢复失败", 500)
		return
	}
	defer tx.Rollback()
	restored := 0
	for _, f := range input.Feeds {
		if strings.TrimSpace(f.Name) == "" || !validFeedURL(f.URL) {
			tx.Rollback()
			http.Error(w, "备份包含无效订阅", 400)
			return
		}
		// 保存位置非法时只丢弃该字段，订阅本身仍可恢复。
		feedStorage, err := sanitizeStorageDir(f.StorageDir)
		if err != nil {
			feedStorage = ""
		}
		_, err = tx.ExecContext(r.Context(), `INSERT INTO feeds(name,url,include_words,exclude_words,auto_download,enabled,last_sync,last_error,created_at,target_title,storage_dir,season_filter,allow_unknown_season,episode_offset) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(url) DO UPDATE SET name=excluded.name,include_words=excluded.include_words,exclude_words=excluded.exclude_words,auto_download=excluded.auto_download,enabled=excluded.enabled,target_title=excluded.target_title,storage_dir=excluded.storage_dir,season_filter=excluded.season_filter,allow_unknown_season=excluded.allow_unknown_season,episode_offset=excluded.episode_offset,completion_note=''`, f.Name, f.URL, f.Include, f.Exclude, boolInt(f.Auto), boolInt(f.Enabled), "", "", time.Now().Format(time.RFC3339), sanitizeText(f.TargetTitle, titleMaxLength), feedStorage, f.SeasonFilter, boolInt(f.AllowUnknownSeason), f.EpisodeOffset)
		if err != nil {
			tx.Rollback()
			http.Error(w, "恢复失败", 500)
			return
		}
		restored++
	}
	// v3 backups carry the series library; titles are the merge key and the
	// backup's row ids are ignored.
	for _, a := range input.Animes {
		title := strings.TrimSpace(a.Title)
		if title == "" || len([]rune(title)) > titleMaxLength {
			tx.Rollback()
			http.Error(w, "备份包含无效番剧", 400)
			return
		}
		season := a.Season
		if a.UpdateWeekday < 0 || a.UpdateWeekday > 7 {
			http.Error(w, "备份包含无效番剧更新日", 400)
			return
		}
		if season < 0 || season > 999 {
			tx.Rollback()
			http.Error(w, "备份包含无效番剧季号", 400)
			return
		}
		clean, err := sanitizeStorageDir(a.StorageDir)
		if err != nil {
			tx.Rollback()
			http.Error(w, "备份包含无效存储位置", 400)
			return
		}
		bangumiID := a.BangumiID
		if bangumiID < 0 {
			bangumiID = 0
		}
		if _, err = tx.ExecContext(r.Context(), `INSERT INTO animes(title,search_name,bangumi_id,season,storage_dir,include_rules,exclude_rules,created_at,updated_at,update_weekday) VALUES(?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(title) DO UPDATE SET search_name=excluded.search_name,
				bangumi_id=CASE WHEN excluded.bangumi_id>0 THEN excluded.bangumi_id ELSE animes.bangumi_id END,
				season=excluded.season,
				update_weekday=excluded.update_weekday,
				storage_dir=CASE WHEN COALESCE(excluded.storage_dir,'')!='' THEN excluded.storage_dir ELSE animes.storage_dir END,
				include_rules=excluded.include_rules,exclude_rules=excluded.exclude_rules`,
			title, sanitizeText(a.SearchName, searchNameLimit), bangumiID, season, clean,
			sanitizeText(a.IncludeRules, rulesMaxLength), sanitizeText(a.ExcludeRules, rulesMaxLength),
			time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339), a.UpdateWeekday); err != nil {
			tx.Rollback()
			http.Error(w, "恢复失败", 500)
			return
		}
	}
	// v2 backups carry per-series storage overrides; merge them into animes.
	for _, s := range input.AnimeStorages {
		title := strings.TrimSpace(s.AnimeTitle)
		clean, err := sanitizeStorageDir(s.StorageDir)
		season := s.Season
		if season == 0 {
			season = 1
		}
		if title == "" || err != nil || season < 1 || season > 999 {
			tx.Rollback()
			http.Error(w, "备份包含无效存储位置", 400)
			return
		}
		if _, err = tx.ExecContext(r.Context(), `INSERT INTO animes(title,storage_dir,season,created_at,updated_at) VALUES(?,?,?,?,?)
			ON CONFLICT(title) DO UPDATE SET
				storage_dir=CASE WHEN COALESCE(excluded.storage_dir,'')!='' THEN excluded.storage_dir ELSE animes.storage_dir END,
				season=CASE WHEN excluded.season>0 THEN excluded.season ELSE animes.season END`,
			title, clean, season, time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339)); err != nil {
			tx.Rollback()
			http.Error(w, "恢复失败", 500)
			return
		}
	}
	if err = tx.Commit(); err != nil {
		http.Error(w, "恢复失败", 500)
		return
	}
	jsonOut(w, map[string]int{"restored": restored})
}

func importOPML(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "方法不支持", 405)
		return
	}
	var doc opmlDocument
	if err := xml.NewDecoder(http.MaxBytesReader(w, r.Body, 5<<20)).Decode(&doc); err != nil {
		http.Error(w, "OPML 文件无效", 400)
		return
	}
	var outlines []opmlOutline
	var collect func([]opmlOutline)
	collect = func(items []opmlOutline) {
		for _, item := range items {
			if item.XMLURL != "" {
				outlines = append(outlines, item)
			}
			collect(item.Children)
		}
	}
	collect(doc.Body.Outlines)
	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "导入失败", 500)
		return
	}
	count := 0
	for _, item := range outlines {
		name := item.Text
		if name == "" {
			name = item.Title
		}
		if name == "" {
			name = "OPML 订阅"
		}
		if !strings.HasPrefix(item.XMLURL, "http://") && !strings.HasPrefix(item.XMLURL, "https://") {
			continue
		}
		result, err := tx.Exec("INSERT INTO feeds(name,url,created_at,episode_offset) VALUES(?,?,?,NULL) ON CONFLICT(url) DO NOTHING", name, item.XMLURL, time.Now().Format(time.RFC3339))
		if err != nil {
			tx.Rollback()
			http.Error(w, "导入失败", 500)
			return
		}
		// 已存在同一 URL 时冲突分支不写入任何行，不能计入新增数量。
		if n, rowsErr := result.RowsAffected(); rowsErr == nil && n > 0 {
			count++
		}
	}
	if err = tx.Commit(); err != nil {
		http.Error(w, "导入失败", 500)
		return
	}
	jsonOut(w, map[string]int{"imported": count})
}

func deleteFeed(w http.ResponseWriter, r *http.Request) {
	var id int
	fmt.Sscanf(strings.TrimPrefix(r.URL.Path, "/api/feeds/"), "%d", &id)
	if r.Method != "DELETE" || id == 0 {
		http.Error(w, "方法不支持", 405)
		return
	}
	// 先只读任务标识，再关掉结果集走网络：SQLite 连接池只有一条连接，持着 rows
	// 调下载器（内置 qBittorrent 登录最长 30 秒）会阻塞其它数据库读写。
	tasks := pendingDownloaderTasks(db, "SELECT downloader,task_id FROM items WHERE feed_id=? AND COALESCE(task_id,'')<>''", id)
	_, _ = db.Exec("DELETE FROM items WHERE feed_id=?", id)
	cancelDownloaderTasks(tasks)
	result, err := db.Exec("DELETE FROM feeds WHERE id=?", id)
	if err != nil {
		http.Error(w, "删除失败", 500)
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		http.Error(w, "订阅不存在", 404)
		return
	}
	jsonOut(w, map[string]bool{"ok": true})
}

func updateFeed(w http.ResponseWriter, r *http.Request) {
	var id int
	fmt.Sscanf(strings.TrimPrefix(r.URL.Path, "/api/feeds/"), "%d", &id)
	if r.Method != "PUT" || id == 0 {
		http.Error(w, "方法不支持", 405)
		return
	}
	var in FeedInput
	if json.NewDecoder(r.Body).Decode(&in) != nil || strings.TrimSpace(in.Name) == "" {
		http.Error(w, "订阅参数无效", 400)
		return
	}
	if !validEpisodeOffset(in.EpisodeOffset) {
		http.Error(w, "集数偏移必须在 -9999 到 9999 之间", 400)
		return
	}
	var seasonValue, unknownValue any
	if in.SeasonFilter != nil {
		if !titleparse.ValidSeasonFilter(*in.SeasonFilter) {
			http.Error(w, "季数条件无效", 400)
			return
		}
		seasonValue = *in.SeasonFilter
	}
	if in.AllowUnknownSeason != nil {
		unknownValue = boolInt(*in.AllowUnknownSeason)
	}
	storage, err := feedStorageDir(in.StorageDir)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	var previousStorage, previousTarget string
	if err := db.QueryRow("SELECT COALESCE(storage_dir,''),COALESCE(target_title,'') FROM feeds WHERE id=?", id).Scan(&previousStorage, &previousTarget); err != nil {
		http.Error(w, "订阅不存在", 404)
		return
	}
	// 编辑弹窗不含启用开关：请求未携带 enabled 时保留原值，不能按零值把订阅停用
	var enabled any
	if in.Enabled != nil {
		enabled = boolInt(*in.Enabled)
	}
	target := previousTarget
	var targetValue any
	if in.TargetTitle != nil {
		target = sanitizeText(*in.TargetTitle, titleMaxLength)
		targetValue = target
	}
	var storageValue any
	if in.StorageDir != nil {
		storageValue = storage
	}
	result, err := db.Exec("UPDATE feeds SET name=?,include_words=?,exclude_words=?,auto_download=?,enabled=COALESCE(?,enabled),target_title=COALESCE(?,target_title),storage_dir=COALESCE(?,storage_dir),season_filter=COALESCE(?,season_filter),allow_unknown_season=COALESCE(?,allow_unknown_season),episode_offset=COALESCE(?,episode_offset),completion_note=CASE WHEN ?=1 THEN '' ELSE completion_note END WHERE id=?", strings.TrimSpace(in.Name), in.Include, in.Exclude, boolInt(in.Auto), enabled, targetValue, storageValue, seasonValue, unknownValue, in.EpisodeOffset, enabled, id)
	if err != nil {
		http.Error(w, "保存失败", 500)
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		http.Error(w, "订阅不存在", 404)
		return
	}
	// 保存位置变化时才动番剧记录：只改订阅名或规则不会覆盖已有的存储设置。
	storagePath, warning := "", ""
	if in.StorageDir != nil && storage != previousStorage {
		storagePath, warning = applySubscriptionStorage(storage, target, previousStorage)
	}
	jsonOut(w, map[string]any{"ok": true, "storage_path": storagePath, "storage_warning": warning})
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
func items(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "方法不支持", 405)
		return
	}
	where := []string{}
	args := []any{}
	if group := r.URL.Query().Get("group"); group != "" && group != "全部" {
		switch group {
		case "下载中":
			where = append(where, "items.status IN ("+activeTaskStatusSQL+")")
		case "待下载":
			where = append(where, "items.status IN ('待下载','等待下载','等待继续')")
		case "已暂停", "已保存", "下载失败", "等待下载器":
			where = append(where, "items.status=?")
			args = append(args, group)
		default:
			http.Error(w, "状态筛选无效", 400)
			return
		}
	}
	if raw := r.URL.Query().Get("before"); raw != "" {
		before, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || before < 1 {
			http.Error(w, "分页游标无效", 400)
			return
		}
		where = append(where, "items.id<?")
		args = append(args, before)
	}
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		where = append(where, "(items.title LIKE ? OR items.anime_title LIKE ? OR items.fansub_group LIKE ?)")
		pattern := "%" + q + "%"
		args = append(args, pattern, pattern, pattern)
	}
	if status := strings.TrimSpace(r.URL.Query().Get("status")); status != "" {
		where = append(where, "items.status=?")
		args = append(args, status)
	}
	query := "SELECT items.id,COALESCE(items.title,''),COALESCE(items.link,''),COALESCE(items.published,''),COALESCE(items.status,'待下载'),COALESCE(items.anime_title,''),COALESCE(items.episode,''),COALESCE(items.quality,''),COALESCE(items.fansub_group,''),COALESCE(feeds.name,''),COALESCE(items.downloader,''),COALESCE(items.task_error,''),COALESCE(items.progress,0),COALESCE(items.file_path,''),COALESCE(items.anime_id,0),COALESCE(items.season,0),COALESCE(items.season_auto,0),COALESCE(items.manual,0),COALESCE(anime_meta.cover_file,'') FROM items LEFT JOIN feeds ON feeds.id=items.feed_id LEFT JOIN anime_meta ON anime_meta.anime_title=items.anime_title"
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	paginated := r.URL.Query().Get("page") == "1"
	query += " ORDER BY items.id DESC LIMIT 101"
	rows, err := db.Query(query, args...)
	if err != nil {
		http.Error(w, "查询失败", 500)
		return
	}
	defer rows.Close()
	type item struct {
		ID            int    `json:"id"`
		Title         string `json:"title"`
		Link          string `json:"link"`
		Published     string `json:"published"`
		Status        string `json:"status"`
		AnimeTitle    string `json:"anime_title"`
		Episode       string `json:"episode"`
		Quality       string `json:"quality"`
		Group         string `json:"fansub_group"`
		Feed          string `json:"feed_name"`
		Downloader    string `json:"downloader"`
		TaskError     string `json:"task_error"`
		Progress      int    `json:"progress"`
		DownloadSpeed int64  `json:"download_speed"`
		FilePath      string `json:"file_path"`
		AnimeID       int64  `json:"anime_id"`
		Season        int    `json:"season"`
		SeasonAuto    int    `json:"season_auto"`
		Manual        bool   `json:"manual"`
		CoverFile     string `json:"-"`
		Cover         string `json:"cover"`
	}
	out := []item{}
	for rows.Next() {
		var x item
		var manual int
		if err := rows.Scan(&x.ID, &x.Title, &x.Link, &x.Published, &x.Status, &x.AnimeTitle, &x.Episode, &x.Quality, &x.Group, &x.Feed, &x.Downloader, &x.TaskError, &x.Progress, &x.FilePath, &x.AnimeID, &x.Season, &x.SeasonAuto, &manual, &x.CoverFile); err != nil {
			http.Error(w, "读取条目失败", 500)
			return
		}
		x.DownloadSpeed, x.Progress = transferSnapshot(x.ID, x.Status, x.Progress)
		x.Manual = manual == 1
		if x.CoverFile != "" {
			x.Cover = "/api/covers/" + x.CoverFile
		}
		out = append(out, x)
	}
	nextBefore := 0
	if len(out) > 100 {
		out = out[:100]
		nextBefore = out[99].ID
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "查询失败", 500)
		return
	}
	if paginated {
		jsonOut(w, map[string]any{"items": out, "next_before": nextBefore})
		return
	}
	jsonOut(w, out)
}
func main() {
	// 发布包是 GUI 子系统：从终端查询版本或运行服务时附着父控制台，让输出可见；
	// 没有父控制台（双击桌面版）时什么也不做，不会新开窗口。
	attachParentConsole()
	// -version：供容器启动脚本与脚本化部署查询版本，不触碰数据库。
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-version", "--version", "version":
			fmt.Println(versionLabel())
			return
		}
	}
	// 启动引导：若上一次更新把新版本暂存在数据目录，先应用它再启动服务。
	bootstrapStagedUpdate()
	if desktopRequested() {
		runDesktop()
		return
	}
	// 仅用于演练/联调：让重启后的新进程与旧进程错开端口释放时间。
	if raw := os.Getenv("AUTO_ANIME_STARTUP_DELAY_SECONDS"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 60 {
			log.Printf("启动延迟 %d 秒（AUTO_ANIME_STARTUP_DELAY_SECONDS）", n)
			time.Sleep(time.Duration(n) * time.Second)
		}
	}
	if err := runApplication(); err != nil {
		log.Fatal(err)
	}
}

func runApplication() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return runApplicationContext(ctx, nil, nil)
}

// Desktop owns the listener from startup, so no port probe/rebind race can
// navigate the window to an unrelated local server.
func runApplicationContext(ctx context.Context, listener net.Listener, ready func()) error {
	releaseLock, err := acquireApplicationLock(env("DB_PATH", defaultDBPath()))
	if err != nil {
		return err
	}
	defer releaseLock()
	initDB()
	defer db.Close()
	cleanupSessions()
	// 番剧库一次性数据迁移：识别标题 → animes、旧存储位置表并入、items 回填 anime_id。
	if err := migrateAnimeData(); err != nil {
		log.Printf("番剧数据迁移失败（功能可能不完整）: %v", err)
	}
	if raw := env("PROXY_URL", ""); raw != "" {
		if err := applyProxyConfig(raw); err != nil {
			return fmt.Errorf("PROXY_URL 无效: %w", err)
		}
		log.Printf("出站代理已启用: %s（RSS/下载/封面走代理，下载器 RPC 始终直连）", redactProxy(currentProxy()))
	}
	// 数据库里保存的用户设置优先于环境变量，最后加载。
	loadSettings()
	// 用户经常搞不清程序在用哪个库、往哪写，把生效路径直接打进启动日志。
	log.Printf("数据目录：数据库 %s；下载 %s；封面 %s", dbFile, downloadDir, coversDir)
	if strings.TrimSpace(libraryDir) == "" {
		log.Printf("媒体库目录未设置：归档到下载目录 %s", downloadDir)
	} else {
		log.Printf("媒体库目录：%s", libraryDir)
	}
	warnStoredPathsForPlatform()
	if err := startNativeDownloader(); err != nil {
		// BT 引擎故障不应让整个桌面窗口打不开；设置页可切换下载方式，轮询/提交
		// 时会继续按退避重试。健康检查在该状态下返回 503。
		log.Printf("内置 BT 下载器暂不可用：%v", err)
	}
	defer stopNativeDownloader()
	startUpdateChecker()
	if authEnabled() {
		log.Println("Web 登录已启用")
	} else if setupDone {
		log.Println("Web 登录未启用：已跳过首次安装引导，可在设置页配置")
	} else if (webUser == "") != (webPassword == "") {
		log.Println("Web 认证未启用：需要同时设置 WEB_USER 和 WEB_PASSWORD")
	}
	if setupRequired() {
		log.Printf("首次安装：请访问 http://127.0.0.1:%s/setup 完成初始化", listenPort)
	}
	if err := recoverArchiveCommits(); err != nil {
		return fmt.Errorf("恢复归档提交失败: %w", err)
	}
	recovered, err := recoverDownloads(db)
	if err != nil {
		return fmt.Errorf("恢复下载队列失败: %w", err)
	}
	go resumeDownloads(recovered)
	startTaskPoller()
	startMetaWorker()
	go enqueueMissingMeta()
	startAnimeCleanupWorker()
	startBangumiWishSync(ctx)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			serveWebPage(w, r, "index.html")
			return
		}
		http.NotFound(w, r)
	})
	http.HandleFunc("/login", loginPage)
	http.HandleFunc("/setup", setupPage)
	http.HandleFunc("/install", installPage)
	http.HandleFunc("/api/login", handleLogin)
	http.HandleFunc("/api/logout", handleLogout)
	http.HandleFunc("/api/settings", settingsHandler)
	http.HandleFunc("/api/settings/storage", settingsStorageHandler)
	http.HandleFunc("/api/setup", setupHandler)
	http.HandleFunc("/api/setup/status", setupStatusHandler)
	http.HandleFunc("/api/setup/storage", setupStorageHandler)
	http.HandleFunc("/api/feeds", feeds)
	http.HandleFunc("/api/preview", preview)
	http.HandleFunc("/api/backup", backup)
	http.HandleFunc("/api/restore", restore)
	http.HandleFunc("/api/import-opml", importOPML)
	http.Handle("/static/", http.StripPrefix("/static/", webAssetHandler()))
	http.HandleFunc("/api/feeds/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			updateFeed(w, r)
			return
		}
		deleteFeed(w, r)
	})
	http.HandleFunc("/api/items", items)
	http.HandleFunc("/api/anime/", animeStorageAPI)
	http.HandleFunc("/api/animes", animesAPI)
	http.HandleFunc("/api/animes/", animeEntryAPI)
	http.HandleFunc("/api/meta/search", metaSearchAPI)
	http.HandleFunc("/api/bangumi/subjects/", bangumiSubjectAPI)
	http.HandleFunc("/api/bangumi/wishes", bangumiWishAPI)
	http.HandleFunc("/api/bangumi/wishes/sync", bangumiWishSyncAPI)
	http.HandleFunc("/api/discover/subjects", discoverSubjectsAPI)
	http.HandleFunc("/api/discover/resources", discoverResourcesAPI)
	http.HandleFunc("/api/discover/availability", discoverAvailabilityAPI)
	http.HandleFunc("/api/health", healthStatus)
	http.HandleFunc("/api/version", updateHandler)
	http.HandleFunc("/api/update", updateHandler)
	http.HandleFunc("/api/covers/", serveCover)
	http.HandleFunc("/api/status", runtimeStatus)
	// 桌面窗口的原生标题栏跟随应用主题（服务模式返回 applied=false）。
	http.HandleFunc("/api/desktop/theme", desktopThemeHandler)
	http.HandleFunc("/api/items/", itemsEntryAPI)
	http.HandleFunc("/api/sync/", syncFeedHandler)
	go func() {
		for {
			if err := syncEnabledFeedsContext(ctx); err != nil {
				log.Println("定时同步失败:", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(runtimeConfig().pollMinutes) * time.Minute):
			}
		}
	}()
	log.Printf("自动追番运行在 :%s", listenPort)
	server := &http.Server{Addr: ":" + listenPort, Handler: auth(http.DefaultServeMux), ReadHeaderTimeout: 10 * time.Second}
	serveErr := make(chan error, 1)
	go func() {
		if listener != nil {
			if ready != nil {
				ready()
			}
			serveErr <- server.Serve(listener)
		} else {
			serveErr <- server.ListenAndServe()
		}
	}()
	select {
	case <-ctx.Done():
	case err := <-serveErr:
		return err
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		server.Close()
		return err
	}
	return nil
}

func syncEnabledFeedsContext(ctx context.Context) error {
	rows, err := db.Query("SELECT id FROM feeds WHERE enabled=1")
	if err != nil {
		return err
	}
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := syncFeedContext(ctx, id); err != nil {
			log.Printf("订阅 %d 同步失败: %v", id, err)
		}
	}
	return nil
}

func syncFeedHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "方法不支持", 405)
		return
	}
	id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/api/sync/"))
	if err != nil || id < 1 {
		http.Error(w, "订阅编号无效", 400)
		return
	}
	n, err := syncFeedContext(r.Context(), id)
	if err != nil {
		http.Error(w, fmt.Sprintf("已新增 %d 条；%v", n, err), 502)
		return
	}
	jsonOut(w, map[string]int{"added": n})
}

package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	_ "modernc.org/sqlite"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var db *sql.DB
var downloadDir = env("DOWNLOAD_DIR", "/data/downloads")
var pollMinutes = 30
var downloadMode = env("DOWNLOAD_MODE", "direct")
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
var downloaderPollSeconds = 30
var webhookURL = strings.TrimSpace(env("WEBHOOK_URL", ""))
var listenPort = env("PORT", "8000")
var webUser = env("WEB_USER", "")
var webPassword = env("WEB_PASSWORD", "")
var httpClient = newClient(60 * time.Second)
var downloadSlots = make(chan struct{}, 2)
var qualityPattern = regexp.MustCompile(`(?i)(2160p|1080p|720p|480p|4k|8k)`)
var groupPattern = regexp.MustCompile(`^\s*[\[【（(]([^\]】）)]+)[\]】）)]\s*`)
var separatorPattern = regexp.MustCompile(`\s+[-–—]\s*`)

// Ordered from the most explicit to the most ambiguous episode marker; the
// first match wins. Season markers like 第三季 or S01 never satisfy the
// episode suffixes, and 4-digit runs such as 1080 or years never match.
var episodePatterns = []*regexp.Regexp{
	regexp.MustCompile(`第\s*([0-9]{1,3})\s*(?:话|集|話)`),
	regexp.MustCompile(`(?i)(?:^|[\s._\-\[])\s*s[0-9]{1,3}\s*e([0-9]{1,3})(?:[^0-9a-z]|$)`),
	regexp.MustCompile(`[\[【(]\s*([0-9]{1,3})(?:v[0-9]{1,2})?\s*[\]】)]`),
	regexp.MustCompile(`(?:^|[\s._-])([0-9]{1,3})(?:v[0-9]{1,2})?(?:\s*(?:话|集|話)|[^0-9a-z]|$)`),
}

// Season markers (第三季, 3期) are part of the series name, never the episode;
// they are masked out before episode matching but kept in the title.
var seasonPattern = regexp.MustCompile(`第\s*[0-9一二三四五六七八九十]{1,3}\s*[季期]|[0-9]{1,2}\s*[季期]`)

type Feed struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	Include   string `json:"include_words"`
	Exclude   string `json:"exclude_words"`
	Auto      bool   `json:"auto_download"`
	Enabled   bool   `json:"enabled"`
	LastSync  string `json:"last_sync"`
	LastError string `json:"last_error"`
}
type FeedInput struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Include string `json:"include_words"`
	Exclude string `json:"exclude_words"`
	Auto    bool   `json:"auto_download"`
	Enabled *bool  `json:"enabled"`
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
	URL string `json:"url"`
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
	XMLName   xml.Name  `xml:""`
	Items     []rssNode `xml:"channel>item"`
	RootItems []rssNode `xml:"item"`
	Entries   []struct {
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
func fetchFeedItems(feedURL string) ([]feedItem, error) {
	if !strings.HasPrefix(feedURL, "http://") && !strings.HasPrefix(feedURL, "https://") {
		return nil, fmt.Errorf("RSS 地址无效")
	}
	req, err := http.NewRequest("GET", feedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Auto-Anime/1.0")
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("RSS 返回 HTTP %d", res.StatusCode)
	}
	items, err := parseFeed(res.Body)
	if err != nil {
		return nil, err
	}
	base, _ := url.Parse(feedURL)
	for i := range items {
		link, parseErr := url.Parse(strings.TrimSpace(items[i].Link))
		if parseErr == nil && link.Scheme == "" {
			items[i].Link = base.ResolveReference(link).String()
		}
	}
	return items, nil
}
func initDB() {
	if n, e := strconv.Atoi(env("POLL_MINUTES", "30")); e == nil && n >= 5 {
		pollMinutes = n
	}
	if n, e := strconv.Atoi(env("DOWNLOADER_POLL_SECONDS", "30")); e == nil && n >= 5 {
		downloaderPollSeconds = n
	}
	var e error
	p := env("DB_PATH", "/data/auto-anime.db")
	os.MkdirAll(filepath.Dir(p), 0755)
	db, e = sql.Open("sqlite", p)
	if e != nil {
		log.Fatal(e)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	_, e = db.Exec(`CREATE TABLE IF NOT EXISTS feeds(id INTEGER PRIMARY KEY,name TEXT NOT NULL,url TEXT UNIQUE NOT NULL,include_words TEXT DEFAULT '',exclude_words TEXT DEFAULT '',auto_download INTEGER DEFAULT 0,enabled INTEGER DEFAULT 1,last_sync TEXT DEFAULT '',last_error TEXT DEFAULT '',created_at TEXT);CREATE TABLE IF NOT EXISTS items(id INTEGER PRIMARY KEY,feed_id INTEGER,title TEXT,link TEXT UNIQUE,published TEXT,description TEXT,status TEXT DEFAULT '待下载',file_path TEXT,created_at TEXT);CREATE TABLE IF NOT EXISTS anime_meta(anime_title TEXT PRIMARY KEY,subject_id INTEGER DEFAULT 0,name TEXT DEFAULT '',name_cn TEXT DEFAULT '',cover_url TEXT DEFAULT '',cover_file TEXT DEFAULT '',source TEXT DEFAULT '',updated_at TEXT);CREATE TABLE IF NOT EXISTS web_sessions(token_hash TEXT PRIMARY KEY,user TEXT NOT NULL,created_at TEXT DEFAULT '',expires_at TEXT NOT NULL);CREATE TABLE IF NOT EXISTS anime_storage(anime_title TEXT PRIMARY KEY,storage_dir TEXT DEFAULT '',season INTEGER DEFAULT 1,updated_at TEXT);CREATE TABLE IF NOT EXISTS settings(key TEXT PRIMARY KEY,value TEXT NOT NULL);CREATE TABLE IF NOT EXISTS animes(id INTEGER PRIMARY KEY,title TEXT UNIQUE NOT NULL,search_name TEXT DEFAULT '',bangumi_id INTEGER DEFAULT 0,season INTEGER DEFAULT 0,storage_dir TEXT DEFAULT '',include_rules TEXT DEFAULT '',exclude_rules TEXT DEFAULT '',created_at TEXT DEFAULT '',updated_at TEXT DEFAULT '');`)
	if e != nil {
		log.Fatal(e)
	}
	for _, col := range []string{"enabled INTEGER DEFAULT 1", "last_sync TEXT DEFAULT ''", "last_error TEXT DEFAULT ''"} {
		name := strings.Fields(col)[0]
		var count int
		db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('feeds') WHERE name=?", name).Scan(&count)
		if count == 0 {
			if _, e = db.Exec("ALTER TABLE feeds ADD COLUMN " + col); e != nil {
				log.Fatal(e)
			}
		}
	}
	for _, col := range []string{"anime_title TEXT DEFAULT ''", "episode TEXT DEFAULT ''", "quality TEXT DEFAULT ''", "fansub_group TEXT DEFAULT ''", "downloader TEXT DEFAULT ''", "task_id TEXT DEFAULT ''", "task_error TEXT DEFAULT ''", "progress INTEGER DEFAULT 0", "anime_id INTEGER DEFAULT 0", "season INTEGER DEFAULT 0", "manual INTEGER DEFAULT 0"} {
		name := strings.Fields(col)[0]
		var count int
		db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('items') WHERE name=?", name).Scan(&count)
		if count == 0 {
			if _, e = db.Exec("ALTER TABLE items ADD COLUMN " + col); e != nil {
				log.Fatal(e)
			}
		}
	}
}
func allowed(t, in, ex string) bool {
	low := strings.ToLower(t)
	for _, x := range strings.Split(in, ",") {
		x = strings.TrimSpace(strings.ToLower(x))
		if x != "" && !strings.Contains(low, x) {
			return false
		}
	}
	for _, x := range strings.Split(ex, ",") {
		x = strings.TrimSpace(strings.ToLower(x))
		if x != "" && strings.Contains(low, x) {
			return false
		}
	}
	return true
}

type feedItem struct{ Title, Link, Pub, Description string }

type animeInfo struct{ Title, Episode, Quality, Group string }

func extractAnimeInfo(raw string) animeInfo {
	info := animeInfo{Title: strings.TrimSpace(raw)}
	clean := strings.TrimSpace(raw)
	if m := groupPattern.FindStringSubmatch(clean); len(m) > 1 {
		info.Group = strings.TrimSpace(m[1])
		clean = strings.TrimSpace(clean[len(m[0]):])
	}
	if m := qualityPattern.FindString(clean); m != "" {
		info.Quality = strings.ToUpper(m)
	}
	// Season markers stay in the title but must never become the episode.
	scoped := seasonPattern.ReplaceAllString(clean, " ")
	for _, pattern := range episodePatterns {
		if m := pattern.FindStringSubmatch(scoped); len(m) > 1 {
			info.Episode = m[1]
			clean = strings.TrimSpace(strings.Replace(clean, m[0], " ", 1))
			break
		}
	}
	clean = separatorPattern.ReplaceAllString(clean, " ")
	info.Title = strings.Trim(strings.TrimSpace(clean), "- _·")
	return info
}

func parseFeed(r io.Reader) ([]feedItem, error) {
	var doc rss
	if err := xml.NewDecoder(r).Decode(&doc); err != nil {
		return nil, err
	}
	if doc.XMLName.Local != "rss" && doc.XMLName.Local != "RDF" && doc.XMLName.Local != "feed" {
		return nil, fmt.Errorf("不是 RSS 或 Atom 文档")
	}
	out := make([]feedItem, 0, len(doc.Items)+len(doc.Entries))
	for _, item := range append(doc.Items, doc.RootItems...) {
		link := item.Link
		if item.Enclosure.URL != "" {
			link = item.Enclosure.URL
		}
		out = append(out, feedItem{item.Title, link, item.Pub, item.Description})
	}
	for _, entry := range doc.Entries {
		link := ""
		for _, candidate := range entry.Links {
			if candidate.Rel == "enclosure" {
				link = candidate.Href
				break
			}
		}
		if link == "" {
			for _, candidate := range entry.Links {
				if candidate.Rel == "alternate" || link == "" {
					link = candidate.Href
				}
			}
		}
		pub := entry.Pub
		if pub == "" {
			pub = entry.Updated
		}
		out = append(out, feedItem{entry.Title, link, pub, entry.Description})
	}
	return out, nil
}
func syncFeed(id int) (int, error) {
	var f Feed
	var a int
	e := db.QueryRow("SELECT id,name,url,include_words,exclude_words,auto_download FROM feeds WHERE id=?", id).Scan(&f.ID, &f.Name, &f.URL, &f.Include, &f.Exclude, &a)
	if e != nil {
		return 0, e
	}
	f.Auto = a == 1
	feedItems, e := fetchFeedItems(f.URL)
	if e != nil {
		db.Exec("UPDATE feeds SET last_error=? WHERE id=?", e.Error(), id)
		return 0, e
	}
	n := 0
	var writeErr error
	for _, it := range feedItems {
		if it.Title == "" || it.Link == "" || !allowed(it.Title, f.Include, f.Exclude) {
			continue
		}
		info := extractAnimeInfo(it.Title)
		// 多来源归并：某部番剧的规则命中原始标题时，条目归入该番剧，
		// 不再按本地识别标题另立门户。
		animeID := int64(0)
		animeTitle := info.Title
		if id, title := matchAnimeRule(it.Title); id > 0 {
			animeID, animeTitle = id, title
		} else if animeTitle != "" {
			animeID, _ = ensureAnime(animeTitle)
		}
		r, e := db.Exec("INSERT OR IGNORE INTO items(feed_id,title,link,published,description,anime_title,anime_id,episode,quality,fansub_group,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)", id, it.Title, it.Link, it.Pub, it.Description, animeTitle, animeID, info.Episode, info.Quality, info.Group, time.Now().Format(time.RFC3339))
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
	result, err := db.Exec("UPDATE items SET status='等待下载',task_error='' WHERE id=? AND status IN ('待下载','下载失败','等待下载器')", id)
	if err != nil {
		return
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		return
	}
	downloadSlots <- struct{}{}
	defer func() { <-downloadSlots }()
	if _, err = db.Exec("UPDATE items SET status='下载中',downloader='',task_id='',progress=0 WHERE id=?", id); err != nil {
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
	if downloadMode == "qbittorrent" && qbitURL != "" {
		submitQBittorrent(id, link)
		return
	}
	if downloadMode == "transmission" && transmissionURL != "" {
		taskID, e := transmissionAdd(link)
		if e != nil {
			failDownload(id, "Transmission 提交失败："+e.Error())
			return
		}
		setSubmitted(id, "transmission", taskID, "已发送到 Transmission")
		return
	}
	if downloadMode == "aria2" && aria2URL != "" {
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
	path, e := fetchArchive(id, link)
	if e == nil {
		db.Exec("UPDATE items SET status='已保存',file_path=?,task_error='',progress=100 WHERE id=?", path, id)
		notify("download_saved", id, "已保存")
		return
	}
	if errors.Is(e, errTorrentResource) {
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
	if e := qbitAdd(link, tag); e != nil {
		failDownload(id, "qBittorrent 提交失败："+e.Error())
		return
	}
	setSubmitted(id, "qbittorrent", hash, "已发送到 qBittorrent")
}

func setSubmitted(id int, downloaderName, taskID, status string) {
	if _, err := db.Exec("UPDATE items SET status=?,downloader=?,task_id=?,task_error='',progress=0 WHERE id=?", status, downloaderName, taskID, id); err != nil {
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
	// "/volume1/..." is absolute inside the Linux container even though
	// Windows builds report it as relative.
	if !filepath.IsAbs(d) && !strings.HasPrefix(d, "/") && !strings.HasPrefix(d, "\\") {
		return "", fmt.Errorf("存储位置必须是绝对路径，例如 /volume1/anime/番剧名")
	}
	for _, part := range strings.Split(filepath.ToSlash(d), "/") {
		if part == ".." {
			return "", fmt.Errorf("存储位置不能包含 ..")
		}
	}
	// Keep forward slashes: the storage dir runs inside the Linux container,
	// and path.Clean avoids the platform-dependent separator of filepath.Clean.
	return path.Clean(filepath.ToSlash(d)), nil
}

// animeStorageAPI keeps the legacy /api/anime/{title}/storage endpoint working
// on top of the animes table (which absorbed anime_storage at migration).
func animeStorageAPI(w http.ResponseWriter, r *http.Request) {
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
		def := filepath.Join(downloadDir, safeComponent(title))
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
		jsonOut(w, map[string]bool{"ok": true})
	default:
		http.Error(w, "方法不支持", 405)
	}
}

func feeds(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		rows, _ := db.Query("SELECT id,name,url,include_words,exclude_words,auto_download,enabled,last_sync,last_error FROM feeds ORDER BY id DESC")
		defer rows.Close()
		out := []Feed{}
		for rows.Next() {
			var f Feed
			var a int
			var enabled int
			rows.Scan(&f.ID, &f.Name, &f.URL, &f.Include, &f.Exclude, &a, &enabled, &f.LastSync, &f.LastError)
			f.Auto = a == 1
			f.Enabled = enabled == 1
			out = append(out, f)
		}
		jsonOut(w, out)
		return
	}
	var in FeedInput
	if json.NewDecoder(r.Body).Decode(&in) != nil || !strings.HasPrefix(in.URL, "http") {
		http.Error(w, "RSS 地址无效", 400)
		return
	}
	result, e := db.Exec("INSERT INTO feeds(name,url,include_words,exclude_words,auto_download,created_at) VALUES(?,?,?,?,?,?)", in.Name, in.URL, in.Include, in.Exclude, boolInt(in.Auto), time.Now().Format(time.RFC3339))
	if e != nil {
		http.Error(w, "RSS 地址已经存在", 409)
		return
	}
	newID, _ := result.LastInsertId()
	jsonOut(w, map[string]any{"ok": true, "id": newID})
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
	items, err := fetchFeedItems(strings.TrimSpace(in.URL))
	if err != nil {
		http.Error(w, "RSS 预览失败："+err.Error(), 502)
		return
	}
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
	jsonOut(w, map[string]any{"items": out, "count": len(out)})
}

func backup(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "方法不支持", 405)
		return
	}
	data := backupData{Version: 3, ExportedAt: time.Now().UTC().Format(time.RFC3339), Feeds: []Feed{}, Items: []map[string]any{}}
	tx, err := db.BeginTx(r.Context(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		http.Error(w, "备份失败", 500)
		return
	}
	defer tx.Rollback()
	feedRows, err := tx.QueryContext(r.Context(), "SELECT id,name,url,COALESCE(include_words,''),COALESCE(exclude_words,''),COALESCE(auto_download,0),COALESCE(enabled,1),COALESCE(last_sync,''),COALESCE(last_error,'') FROM feeds ORDER BY id")
	if err != nil {
		http.Error(w, "备份失败", 500)
		return
	}
	for feedRows.Next() {
		var f Feed
		var auto, enabled int
		if err = feedRows.Scan(&f.ID, &f.Name, &f.URL, &f.Include, &f.Exclude, &auto, &enabled, &f.LastSync, &f.LastError); err != nil {
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
	rows, err := tx.QueryContext(r.Context(), "SELECT id,COALESCE(feed_id,0),COALESCE(title,''),COALESCE(link,''),COALESCE(published,''),COALESCE(description,''),COALESCE(status,'待下载'),COALESCE(file_path,''),COALESCE(anime_title,''),COALESCE(episode,''),COALESCE(quality,''),COALESCE(fansub_group,''),COALESCE(anime_id,0),COALESCE(season,0),COALESCE(created_at,'') FROM items ORDER BY id")
	if err != nil {
		http.Error(w, "备份失败", 500)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, feedID int
		var title, link, published, description, status, filePath, animeTitle, episode, quality, group, created string
		var animeID int64
		var season int
		if err = rows.Scan(&id, &feedID, &title, &link, &published, &description, &status, &filePath, &animeTitle, &episode, &quality, &group, &animeID, &season, &created); err != nil {
			http.Error(w, "备份失败", 500)
			return
		}
		data.Items = append(data.Items, map[string]any{"id": id, "feed_id": feedID, "title": title, "link": link, "published": published, "description": description, "status": status, "file_path": filePath, "anime_title": animeTitle, "anime_id": animeID, "season": season, "episode": episode, "quality": quality, "fansub_group": group, "created_at": created})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		http.Error(w, "备份失败", 500)
		return
	}
	animeRows, err := tx.QueryContext(r.Context(), `SELECT id,title,COALESCE(search_name,''),COALESCE(bangumi_id,0),COALESCE(season,0),COALESCE(storage_dir,''),COALESCE(include_rules,''),COALESCE(exclude_rules,''),COALESCE(updated_at,'') FROM animes ORDER BY id`)
	if err != nil {
		http.Error(w, "备份失败", 500)
		return
	}
	for animeRows.Next() {
		var a animeRow
		if err = animeRows.Scan(&a.ID, &a.Title, &a.SearchName, &a.BangumiID, &a.Season, &a.StorageDir, &a.IncludeRules, &a.ExcludeRules, &a.UpdatedAt); err != nil {
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
	if r.Method != "POST" {
		http.Error(w, "方法不支持", 405)
		return
	}
	var input restoreRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 5<<20))
	if decoder.Decode(&input) != nil || input.Feeds == nil || (input.Version != nil && *input.Version != 1 && *input.Version != 2 && *input.Version != 3) {
		http.Error(w, "备份文件无效", 400)
		return
	}
	if decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "备份文件包含额外内容或超过 5 MB", 400)
		return
	}
	for i := range input.Feeds {
		f := &input.Feeds[i]
		f.Name, f.URL = strings.TrimSpace(f.Name), strings.TrimSpace(f.URL)
		u, err := url.Parse(f.URL)
		if f.Name == "" || err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
			http.Error(w, "备份包含无效订阅", 400)
			return
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
		if strings.TrimSpace(f.Name) == "" || (!strings.HasPrefix(f.URL, "http://") && !strings.HasPrefix(f.URL, "https://")) {
			tx.Rollback()
			http.Error(w, "备份包含无效订阅", 400)
			return
		}
		_, err = tx.ExecContext(r.Context(), `INSERT INTO feeds(name,url,include_words,exclude_words,auto_download,enabled,last_sync,last_error,created_at) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(url) DO UPDATE SET name=excluded.name,include_words=excluded.include_words,exclude_words=excluded.exclude_words,auto_download=excluded.auto_download,enabled=excluded.enabled`, f.Name, f.URL, f.Include, f.Exclude, boolInt(f.Auto), boolInt(f.Enabled), "", "", time.Now().Format(time.RFC3339))
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
		if _, err = tx.ExecContext(r.Context(), `INSERT INTO animes(title,search_name,bangumi_id,season,storage_dir,include_rules,exclude_rules,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)
			ON CONFLICT(title) DO UPDATE SET search_name=excluded.search_name,
				bangumi_id=CASE WHEN excluded.bangumi_id>0 THEN excluded.bangumi_id ELSE animes.bangumi_id END,
				season=excluded.season,
				storage_dir=CASE WHEN COALESCE(excluded.storage_dir,'')!='' THEN excluded.storage_dir ELSE animes.storage_dir END,
				include_rules=excluded.include_rules,exclude_rules=excluded.exclude_rules`,
			title, sanitizeText(a.SearchName, searchNameLimit), bangumiID, season, clean,
			sanitizeText(a.IncludeRules, rulesMaxLength), sanitizeText(a.ExcludeRules, rulesMaxLength),
			time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339)); err != nil {
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
		_, err = tx.Exec("INSERT INTO feeds(name,url,created_at) VALUES(?,?,?) ON CONFLICT(url) DO NOTHING", name, item.XMLURL, time.Now().Format(time.RFC3339))
		if err != nil {
			tx.Rollback()
			http.Error(w, "导入失败", 500)
			return
		}
		count++
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
	_, _ = db.Exec("DELETE FROM items WHERE feed_id=?", id)
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
	// 编辑弹窗不含启用开关：请求未携带 enabled 时保留原值，不能按零值把订阅停用
	var enabled any
	if in.Enabled != nil {
		enabled = boolInt(*in.Enabled)
	}
	result, err := db.Exec("UPDATE feeds SET name=?,include_words=?,exclude_words=?,auto_download=?,enabled=COALESCE(?,enabled) WHERE id=?", strings.TrimSpace(in.Name), in.Include, in.Exclude, boolInt(in.Auto), enabled, id)
	if err != nil {
		http.Error(w, "保存失败", 500)
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		http.Error(w, "订阅不存在", 404)
		return
	}
	jsonOut(w, map[string]bool{"ok": true})
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
func items(w http.ResponseWriter, r *http.Request) {
	where := []string{}
	args := []any{}
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		where = append(where, "(items.title LIKE ? OR items.anime_title LIKE ? OR items.fansub_group LIKE ?)")
		pattern := "%" + q + "%"
		args = append(args, pattern, pattern, pattern)
	}
	if status := strings.TrimSpace(r.URL.Query().Get("status")); status != "" {
		where = append(where, "items.status=?")
		args = append(args, status)
	}
	query := "SELECT items.id,COALESCE(items.title,''),COALESCE(items.link,''),COALESCE(items.published,''),COALESCE(items.status,'待下载'),COALESCE(items.anime_title,''),COALESCE(items.episode,''),COALESCE(items.quality,''),COALESCE(items.fansub_group,''),COALESCE(feeds.name,''),COALESCE(items.downloader,''),COALESCE(items.task_error,''),COALESCE(items.progress,0),COALESCE(items.file_path,''),COALESCE(items.anime_id,0),COALESCE(items.season,0),COALESCE(items.manual,0),COALESCE(anime_meta.cover_file,'') FROM items LEFT JOIN feeds ON feeds.id=items.feed_id LEFT JOIN anime_meta ON anime_meta.anime_title=items.anime_title"
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY items.id DESC LIMIT 100"
	rows, err := db.Query(query, args...)
	if err != nil {
		http.Error(w, "查询失败", 500)
		return
	}
	defer rows.Close()
	type item struct {
		ID         int    `json:"id"`
		Title      string `json:"title"`
		Link       string `json:"link"`
		Published  string `json:"published"`
		Status     string `json:"status"`
		AnimeTitle string `json:"anime_title"`
		Episode    string `json:"episode"`
		Quality    string `json:"quality"`
		Group      string `json:"fansub_group"`
		Feed       string `json:"feed_name"`
		Downloader string `json:"downloader"`
		TaskError  string `json:"task_error"`
		Progress   int    `json:"progress"`
		FilePath   string `json:"file_path"`
		AnimeID    int64  `json:"anime_id"`
		Season     int    `json:"season"`
		Manual     bool   `json:"manual"`
		CoverFile  string `json:"-"`
		Cover      string `json:"cover"`
	}
	out := []item{}
	for rows.Next() {
		var x item
		var manual int
		rows.Scan(&x.ID, &x.Title, &x.Link, &x.Published, &x.Status, &x.AnimeTitle, &x.Episode, &x.Quality, &x.Group, &x.Feed, &x.Downloader, &x.TaskError, &x.Progress, &x.FilePath, &x.AnimeID, &x.Season, &manual, &x.CoverFile)
		x.Manual = manual == 1
		if x.CoverFile != "" {
			x.Cover = "/api/covers/" + x.CoverFile
		}
		out = append(out, x)
	}
	jsonOut(w, out)
}
func main() {
	initDB()
	cleanupSessions()
	// 番剧库一次性数据迁移：识别标题 → animes、旧存储位置表并入、items 回填 anime_id。
	if err := migrateAnimeData(); err != nil {
		log.Printf("番剧数据迁移失败（功能可能不完整）: %v", err)
	}
	if raw := env("PROXY_URL", ""); raw != "" {
		if err := applyProxyConfig(raw); err != nil {
			log.Fatalf("PROXY_URL 无效: %v", err)
		}
		log.Printf("出站代理已启用: %s（RSS/下载/封面走代理，下载器 RPC 始终直连）", redactProxy(proxyURL))
	}
	// 数据库里保存的用户设置优先于环境变量，最后加载。
	loadSettings()
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
	recovered, err := recoverDownloads(db)
	if err != nil {
		log.Fatal("恢复下载队列失败: ", err)
	}
	go resumeDownloads(recovered)
	startTaskPoller()
	startMetaWorker()
	go enqueueMissingMeta()
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.ServeFile(w, r, "static/index.html")
			return
		}
		http.NotFound(w, r)
	})
	http.HandleFunc("/login", loginPage)
	http.HandleFunc("/setup", setupPage)
	http.HandleFunc("/api/login", handleLogin)
	http.HandleFunc("/api/logout", handleLogout)
	http.HandleFunc("/api/settings", settingsHandler)
	http.HandleFunc("/api/setup", setupHandler)
	http.HandleFunc("/api/setup/status", setupStatusHandler)
	http.HandleFunc("/api/feeds", feeds)
	http.HandleFunc("/api/preview", preview)
	http.HandleFunc("/api/backup", backup)
	http.HandleFunc("/api/restore", restore)
	http.HandleFunc("/api/import-opml", importOPML)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
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
	http.HandleFunc("/api/discover/sources", discoverSourcesAPI)
	http.HandleFunc("/api/discover/search", discoverSearchAPI)
	http.HandleFunc("/api/health", healthStatus)
	http.HandleFunc("/api/covers/", serveCover)
	http.HandleFunc("/api/status", runtimeStatus)
	http.HandleFunc("/api/items/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			var id int64
			n, _ := fmt.Sscanf(strings.TrimPrefix(r.URL.Path, "/api/items/"), "%d", &id)
			if n != 1 || id <= 0 {
				http.Error(w, "条目不存在", 404)
				return
			}
			correctItem(w, r, id)
			return
		}
		if r.Method == "DELETE" {
			var id int
			n, _ := fmt.Sscanf(strings.TrimPrefix(r.URL.Path, "/api/items/"), "%d", &id)
			if n != 1 || id <= 0 {
				http.Error(w, "条目不存在", 404)
				return
			}
			result, err := db.Exec("DELETE FROM items WHERE id=?", id)
			if err != nil {
				http.Error(w, "移除失败", 500)
				return
			}
			affected, _ := result.RowsAffected()
			if affected == 0 {
				http.Error(w, "条目不存在", 404)
				return
			}
			jsonOut(w, map[string]bool{"ok": true})
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/download") {
			http.NotFound(w, r)
			return
		}
		var id int
		fmt.Sscanf(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/items/"), "/download"), "%d", &id)
		if r.Method != "POST" || id <= 0 {
			http.Error(w, "方法不支持", 405)
			return
		}
		go downloadItem(id)
		jsonOut(w, map[string]bool{"ok": true})
	})
	http.HandleFunc("/api/sync/", func(w http.ResponseWriter, r *http.Request) {
		var id int
		fmt.Sscanf(strings.TrimPrefix(r.URL.Path, "/api/sync/"), "%d", &id)
		n, e := syncFeed(id)
		if e != nil {
			http.Error(w, e.Error(), 502)
			return
		}
		jsonOut(w, map[string]int{"added": n})
	})
	go func() {
		for {
			if err := syncEnabledFeeds(); err != nil {
				log.Println("定时同步失败:", err)
			}
			time.Sleep(time.Duration(pollMinutes) * time.Minute)
		}
	}()
	log.Printf("自动追番运行在 :%s", listenPort)
	log.Fatal(http.ListenAndServe(":"+listenPort, auth(http.DefaultServeMux)))
}

func syncEnabledFeeds() error {
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
		if _, err := syncFeed(id); err != nil {
			log.Printf("订阅 %d 同步失败: %v", id, err)
		}
	}
	return nil
}

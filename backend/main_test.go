package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseRSSAndAtom(t *testing.T) {
	data := `<rss><channel><item><title>[组] 第一话</title><link>https://example.test/1.torrent</link><pubDate>today</pubDate></item></channel></rss>`
	page, err := decodeFeedPage(strings.NewReader(data))
	items := feedItemsFromEntries(page.Entries)
	if err != nil || len(items) != 1 || items[0].Link == "" {
		t.Fatalf("RSS parse failed: %#v %v", items, err)
	}
	atom := `<feed xmlns="http://www.w3.org/2005/Atom"><entry><title>第二话</title><link rel="alternate" href="magnet:?xt=urn:btih:test"/><updated>tomorrow</updated><summary>简介</summary></entry></feed>`
	page, err = decodeFeedPage(strings.NewReader(atom))
	items = feedItemsFromEntries(page.Entries)
	if err != nil || len(items) != 1 || !strings.HasPrefix(items[0].Link, "magnet:") {
		t.Fatalf("Atom parse failed: %#v %v", items, err)
	}
}

func TestRelativeFeedLinksResolveAgainstFeedURL(t *testing.T) {
	oldClient := httpClient
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<rss><channel><item><title>第一话</title><link>/download/1.torrent</link></item></channel></rss>`)
	}))
	defer server.Close()
	httpClient = server.Client()
	defer func() { httpClient = oldClient }()
	result, err := fetchRSS(context.Background(), server.URL+"/rss.xml")
	items := feedItemsFromEntries(result.Entries)
	if err != nil || len(items) != 1 || items[0].Link != server.URL+"/download/1.torrent" {
		t.Fatalf("relative link: %#v %v", items, err)
	}
}

func TestScheduledSyncSkipsPausedFeeds(t *testing.T) {
	testDatabase(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		fmt.Fprint(w, `<rss><channel></channel></rss>`)
	}))
	defer server.Close()
	for _, enabled := range []int{0, 1} {
		if _, err := db.Exec("INSERT INTO feeds(name,url,enabled) VALUES(?,?,?)", "订阅", fmt.Sprintf("%s/%d", server.URL, enabled), enabled); err != nil {
			t.Fatal(err)
		}
	}
	if err := syncEnabledFeedsContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 {
		t.Fatalf("requested %d feeds", requests.Load())
	}
}

func TestRepeatedDownloadDoesNotOverwrite(t *testing.T) {
	testDatabase(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); fmt.Fprint(w, "media") }))
	defer server.Close()
	id := insertItem(t, server.URL+"/episode.mp4")
	downloadItem(id)
	downloadItem(id)
	if requests.Load() != 1 {
		t.Fatalf("downloaded %d times", requests.Load())
	}
}

func testDatabase(t *testing.T) {
	t.Helper()
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "state.db"))
	initDB()
	oldDir, oldMode, oldURL, oldToken := downloadDir, downloadMode, aria2URL, aria2Token
	// 媒体库默认值跟随平台（Windows 目录与下载目录分开）；测试统一回退到「未设置」，
	// 需要媒体库的用例自行设置 libraryDir。
	oldLibrary := libraryDir
	libraryDir = ""
	// 挂载检测读取运行环境的 /proc；测试固定用不存在的挂载表，
	// 需要验证提示的测试自行调用 useMountInfo 替换。
	oldMountInfo := mountInfoPath
	mountInfoPath = filepath.Join(t.TempDir(), "mountinfo")
	downloadDir, downloadMode = t.TempDir(), "direct"
	t.Cleanup(func() {
		db.Close()
		downloadDir, downloadMode, aria2URL, aria2Token = oldDir, oldMode, oldURL, oldToken
		libraryDir = oldLibrary
		mountInfoPath = oldMountInfo
	})
}

func insertItem(t *testing.T, link string) int {
	t.Helper()
	r, err := db.Exec("INSERT INTO items(title,link) VALUES(?,?)", "测试动画", link)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := r.LastInsertId()
	return int(id)
}

func feedEnabled(t *testing.T, id int64) int {
	t.Helper()
	var enabled int
	if err := db.QueryRow("SELECT enabled FROM feeds WHERE id=?", id).Scan(&enabled); err != nil {
		t.Fatal(err)
	}
	return enabled
}

func TestUpdateFeedPreservesEnabledWhenOmitted(t *testing.T) {
	testDatabase(t)
	r, err := db.Exec("INSERT INTO feeds(name,url) VALUES(?,?)", "订阅", "https://example.test/rss.xml")
	if err != nil {
		t.Fatal(err)
	}
	id, _ := r.LastInsertId()
	path := fmt.Sprintf("/api/feeds/%d", id)

	// 编辑弹窗请求不带 enabled，启用中的订阅必须保持启用
	res := httptest.NewRecorder()
	updateFeed(res, httptest.NewRequest("PUT", path, strings.NewReader(`{"name":"新名称","include_words":"1080p"}`)))
	if res.Code != 200 {
		t.Fatalf("update code %d: %s", res.Code, res.Body)
	}
	if got := feedEnabled(t, id); got != 1 {
		t.Fatalf("enabled = %d, want 1", got)
	}

	// 已暂停的订阅编辑后同样保持暂停
	if _, err := db.Exec("UPDATE feeds SET enabled=0 WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	res = httptest.NewRecorder()
	updateFeed(res, httptest.NewRequest("PUT", path, strings.NewReader(`{"name":"新名称"}`)))
	if res.Code != 200 {
		t.Fatalf("update code %d: %s", res.Code, res.Body)
	}
	if got := feedEnabled(t, id); got != 0 {
		t.Fatalf("enabled = %d, want 0", got)
	}

	// 显式携带 enabled 时照常生效，暂停/启用开关依赖此路径
	res = httptest.NewRecorder()
	updateFeed(res, httptest.NewRequest("PUT", path, strings.NewReader(`{"name":"新名称","enabled":true}`)))
	if res.Code != 200 {
		t.Fatalf("update code %d: %s", res.Code, res.Body)
	}
	if got := feedEnabled(t, id); got != 1 {
		t.Fatalf("enabled = %d, want 1", got)
	}
}

func TestAriaRPCErrorMustNotBecomeSuccess(t *testing.T) {
	testDatabase(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":"auto-anime","error":{"code":1,"message":"Unauthorized"}}`)
	}))
	defer server.Close()
	downloadMode, aria2URL = "aria2", server.URL
	id := insertItem(t, "magnet:?xt=urn:btih:0123456789012345678901234567890123456789")
	downloadItem(id)
	var status string
	db.QueryRow("SELECT status FROM items WHERE id=?", id).Scan(&status)
	if status != "下载失败" {
		t.Fatalf("RPC failure was recorded as %q", status)
	}
}

func TestAutomaticDownloadAndDeduplication(t *testing.T) {
	testDatabase(t)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/video.mp4" {
			w.Header().Set("Content-Type", "video/mp4")
			fmt.Fprint(w, "test media")
			return
		}
		fmt.Fprintf(w, `<rss><channel><item><title>动画 1080p 简中</title><link>%s/page</link><enclosure url="%s/video.mp4"/></item><item><title>动画 720p</title><link>%s/other.mp4</link></item></channel></rss>`, server.URL, server.URL, server.URL)
	}))
	defer server.Close()
	r, err := db.Exec("INSERT INTO feeds(name,url,include_words,auto_download) VALUES(?,?,?,1)", "动画订阅", server.URL+"/rss", "1080p")
	if err != nil {
		t.Fatal(err)
	}
	id, _ := r.LastInsertId()
	n, err := syncFeedContext(context.Background(), int(id))
	if err != nil || n != 1 {
		t.Fatalf("sync: %d %v", n, err)
	}
	deadline := time.Now().Add(3 * time.Second)
	var status, path string
	for time.Now().Before(deadline) {
		db.QueryRow("SELECT status,COALESCE(file_path,'') FROM items WHERE feed_id=?", id).Scan(&status, &path)
		if status == "已保存" || status == "下载失败" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if status != "已保存" {
		t.Fatalf("auto download did not complete: %s", status)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "test media" {
		t.Fatalf("download bytes: %q %v", data, err)
	}
	// 无识别集数的资源归档在番剧根目录，文件名保留原始标题。
	// 番剧文件夹用清理掉技术标签后的识别名，文件名仍保留原始标题。
	if want := filepath.Join(downloadDir, "动画", "动画 1080p 简中.mp4"); filepath.ToSlash(path) != filepath.ToSlash(want) {
		t.Fatalf("archive layout: %s, want %s", path, want)
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	archiveMeta, err := os.ReadFile(filepath.Join(filepath.Dir(path), base+"-info.json"))
	if err != nil || !strings.Contains(string(archiveMeta), `"episode": ""`) {
		t.Fatalf("archive metadata: %q %v", archiveMeta, err)
	}
	if partial, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".partial-*")); len(partial) != 0 {
		t.Fatalf("partial archive remains: %v", partial)
	}
	nfo, err := os.ReadFile(filepath.Join(filepath.Dir(path), strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))+".nfo"))
	if err != nil || !strings.Contains(string(nfo), "episodedetails") {
		t.Fatalf("NFO: %q %v", nfo, err)
	}
	n, err = syncFeedContext(context.Background(), int(id))
	if err != nil || n != 0 {
		t.Fatalf("dedup: %d %v", n, err)
	}
}

func TestAtomEnclosureHasPriority(t *testing.T) {
	for _, links := range []string{
		`<link rel="alternate" href="https://example.test/page"/><link rel="enclosure" href="https://example.test/video.mp4"/>`,
		`<link rel="enclosure" href="https://example.test/video.mp4"/><link rel="alternate" href="https://example.test/page"/>`,
	} {
		page, err := decodeFeedPage(strings.NewReader(`<feed xmlns="http://www.w3.org/2005/Atom"><entry><title>动画</title>` + links + `</entry></feed>`))
		items := feedItemsFromEntries(page.Entries)
		if err != nil || len(items) != 1 || items[0].Link != "https://example.test/video.mp4" {
			t.Fatalf("enclosure selection: %#v %v", items, err)
		}
	}
}

func TestRSS1AndInvalidDocument(t *testing.T) {
	page, err := decodeFeedPage(strings.NewReader(`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns="http://purl.org/rss/1.0/"><item><title>第一话</title><link>https://example.test/1.mp4</link></item></rdf:RDF>`))
	items := feedItemsFromEntries(page.Entries)
	if err != nil || len(items) != 1 || items[0].Title != "第一话" {
		t.Fatalf("RSS1: %#v %v", items, err)
	}
	if _, err = decodeFeedPage(strings.NewReader(`<html><body>访问受限</body></html>`)); err == nil {
		t.Fatal("HTML response should be rejected")
	}
}

func TestQBittorrentSubmit(t *testing.T) {
	var login, added bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			login = true
			fmt.Fprint(w, "Ok.")
		case "/api/v2/torrents/add":
			added = true
			fmt.Fprint(w, "Ok.")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	oldURL, oldUser, oldPassword := qbitURL, qbitUser, qbitPassword
	qbitURL, qbitUser, qbitPassword = server.URL, "admin", "secret"
	t.Cleanup(func() { qbitURL, qbitUser, qbitPassword = oldURL, oldUser, oldPassword })
	client, err := qbitLogin()
	if err != nil {
		t.Fatalf("qBittorrent login: %v", err)
	}
	if err := client.add("magnet:?xt=urn:btih:test", "auto-anime-1"); err != nil || !login || !added {
		t.Fatalf("qBittorrent submit: %v login=%v added=%v", err, login, added)
	}
}

func TestSyncFeedRecordsWriteErrors(t *testing.T) {
	testDatabase(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<rss><channel><item><title>第一话</title><link>https://example.test/1.mp4</link></item></channel></rss>`)
	}))
	defer server.Close()
	result, err := db.Exec("INSERT INTO feeds(name,url) VALUES('订阅',?)", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	feedID, _ := result.LastInsertId()
	if _, err := db.Exec("DROP TABLE items"); err != nil {
		t.Fatal(err)
	}
	if _, err := syncFeedContext(context.Background(), int(feedID)); err == nil {
		t.Fatal("item write error was swallowed")
	}
	var lastError string
	if err := db.QueryRow("SELECT COALESCE(last_error,'') FROM feeds WHERE id=?", feedID).Scan(&lastError); err != nil {
		t.Fatal(err)
	}
	// 资源页地址回填先于条目插入执行，DROP items 后最先在此暴露写失败。
	if !strings.Contains(lastError, "资源页地址保存失败") {
		t.Fatalf("last_error: %q", lastError)
	}
}

func TestAllowedKeywords(t *testing.T) {
	if !allowed("番剧 1080p 简中", "1080p,简中", "") {
		t.Fatal("expected item to pass")
	}
	if allowed("番剧 720p", "1080p", "") {
		t.Fatal("expected include filter to reject")
	}
	if allowed("番剧 1080p 广告", "1080p", "广告") {
		t.Fatal("expected exclude filter to reject")
	}
}

func TestExtractAnimeInfo(t *testing.T) {
	info := extractAnimeInfo("[字幕组] 夏日番外篇 - 03 [1080p]")
	if info.Group != "字幕组" || info.Episode != "03" || info.Quality != "1080P" || info.Title != "夏日番外篇" {
		t.Fatalf("unexpected info: %+v", info)
	}
	cases := []struct{ title, episode string }{
		{"[绿茶字幕组] 与你相恋到生命尽头 / Kimi ga Shinu made Koi wo Shitai [07][WebRip][1080p][简繁日内封]", "07"},
		{"[绿茶字幕组] 与你相恋到生命尽头 / Kimi ga Shinu made Koi wo Shitai [09v2][WebRip][1080p][简日内嵌]", "09"},
		{"[Nix-Raws] 与你相恋到生命尽头 / Kimi ga Shinu made Koi wo Shitai S01E09 [CR WEB-DL 1080p AVC AAC][简繁内封]", "09"},
		{"[ANi] Kimi ga Shinu made Koi wo Shitai / 與妳相戀到生命盡頭 - 09 [1080P][Baha][WEB-DL][AAC AVC][CHT][MP4]", "09"},
		{"[桜都字幕组] 无职转生 第三季 ～到了异世界就拿出真本事～ / Mushoku Tensei III [10][1080P][简体内嵌]", "10"},
		{"[LoliHouse] 无职转生 3期 / Mushoku Tensei S3 - 10 [WebRip 1080p HEVC-10bit AAC][简繁内封字幕]", "10"},
		{"【TSDM字幕组】[与你相恋到生命尽头][09][GB][1080P][MP4][简日双语内嵌][Kimi ga Shinu made Koi wo Shitai]", "09"},
		{"[字幕组] 夏日番外篇 第08话 [720p]", "08"},
		{"[黒ネズミたち] 與妳相戀到生命盡頭 - 09 (ABEMA 1920x1080 AVC AAC MKV)", "09"},
	}
	for _, c := range cases {
		if got := extractAnimeInfo(c.title).Episode; got != c.episode {
			t.Errorf("episode of %q: got %q want %q", c.title, got, c.episode)
		}
	}
	nonEpisodes := []string{
		"番剧 2023 [1080p]",
		"无职转生 第三季 ～到了异世界～ [1080P]",
		"[字幕组] 番名 [WebRip HEVC-10bit AAC]",
	}
	for _, title := range nonEpisodes {
		if got := extractAnimeInfo(title).Episode; got != "" {
			t.Errorf("unexpected episode %q from %q", got, title)
		}
	}
}

func TestQBittorrentLoginFailureDoesNotSubmit(t *testing.T) {
	var submitted bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			fmt.Fprint(w, "Fails.")
			return
		}
		submitted = true
		fmt.Fprint(w, "Ok.")
	}))
	defer server.Close()
	old := qbitURL
	qbitURL = server.URL
	defer func() { qbitURL = old }()
	client, err := qbitLogin()
	if err == nil {
		err = client.add("magnet:?xt=urn:btih:test", "auto-anime-1")
	}
	if err == nil || submitted {
		t.Fatalf("failed login continued: error=%v submitted=%v", err, submitted)
	}
}

func TestTransmissionSessionHandshake(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("X-Transmission-Session-Id", "session-1")
			w.WriteHeader(http.StatusConflict)
			return
		}
		if r.Header.Get("X-Transmission-Session-Id") != "session-1" {
			t.Errorf("missing session ID")
		}
		fmt.Fprint(w, `{"result":"success","arguments":{"torrent-added":{"id":3}}}`)
	}))
	defer server.Close()
	old := transmissionURL
	transmissionURL = server.URL
	defer func() { transmissionURL = old }()
	id, err := transmissionAdd("magnet:?xt=urn:btih:test")
	if err != nil || calls != 2 || id != "3" {
		t.Fatalf("Transmission handshake: id=%q %v calls=%d", id, err, calls)
	}
}

func TestRestoreSubscriptionsUpsertsByURL(t *testing.T) {
	testDatabase(t)
	request := httptest.NewRequest("POST", "/api/restore", bytes.NewBufferString(`{"feeds":[{"name":"迁移订阅","url":"https://example.test/rss","include_words":"1080p","enabled":true}]}`))
	recorder := httptest.NewRecorder()
	restore(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("restore status: %d %s", recorder.Code, recorder.Body.String())
	}
	var name, include string
	if err := db.QueryRow("SELECT name,include_words FROM feeds WHERE url=?", "https://example.test/rss").Scan(&name, &include); err != nil || name != "迁移订阅" || include != "1080p" {
		t.Fatalf("restored feed: %s %s %v", name, include, err)
	}
}

func TestImportOPMLNestedOutlines(t *testing.T) {
	testDatabase(t)
	request := httptest.NewRequest("POST", "/api/import-opml", strings.NewReader(`<opml><body><outline text="季度"><outline text="新番" xmlUrl="https://example.test/a.xml"/><outline title="非法" xmlUrl="file:///local"/></outline></body></opml>`))
	recorder := httptest.NewRecorder()
	importOPML(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("OPML import status: %d %s", recorder.Code, recorder.Body.String())
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM feeds WHERE url=?", "https://example.test/a.xml").Scan(&count)
	if count != 1 {
		t.Fatalf("nested OPML subscription count: %d", count)
	}
}

func TestWebAuth(t *testing.T) {
	oldUser, oldPassword := webUser, webPassword
	webUser, webPassword = "admin", "secret"
	defer func() { webUser, webPassword = oldUser, oldPassword }()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") })
	handler := auth(next)

	// 未登录的页面请求重定向到登录页，而不是弹出 Basic Auth。
	request := httptest.NewRequest("GET", "/", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusFound || recorder.Header().Get("Location") != "/login" {
		t.Fatalf("unauthenticated page: %d %s", recorder.Code, recorder.Header().Get("Location"))
	}

	// 未登录的 API 返回 401，且不触发浏览器原生 Basic Auth 对话框。
	request = httptest.NewRequest("GET", "/api/items", nil)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated api status: %d", recorder.Code)
	}
	if recorder.Header().Get("WWW-Authenticate") != "" {
		t.Fatal("API 401 must not carry WWW-Authenticate")
	}

	// Basic Auth 仍对脚本与旧客户端有效。
	request = httptest.NewRequest("GET", "/", nil)
	request.SetBasicAuth("admin", "secret")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("authenticated status: %d", recorder.Code)
	}

	request = httptest.NewRequest("GET", "/api/health", nil)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("health status: %d", recorder.Code)
	}
}

func TestAuthRequiresCompleteConfig(t *testing.T) {
	oldUser, oldPassword := webUser, webPassword
	oldSetupDone := setupDone
	setupDone = true // 引导已跳过：部分 env 配置时保持完全开放
	defer func() { webUser, webPassword = oldUser, oldPassword; setupDone = oldSetupDone }()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") })
	for _, tc := range []struct{ user, password string }{
		{"", ""},
		{"admin", ""},
		{"", "secret"},
	} {
		webUser, webPassword = tc.user, tc.password
		recorder := httptest.NewRecorder()
		auth(next).ServeHTTP(recorder, httptest.NewRequest("GET", "/", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("partial config %q/%q should stay open, got %d", tc.user, tc.password, recorder.Code)
		}
	}
}

func TestItemSearchAndStatusFilter(t *testing.T) {
	testDatabase(t)
	_, err := db.Exec("INSERT INTO items(title,link,anime_title,fansub_group,status) VALUES(?,?,?,?,?)", "动画 1080P", "https://example.test/a", "夏日动画", "字幕组A", "已保存")
	if err != nil {
		t.Fatal(err)
	}
	query := url.Values{"q": {"夏日"}, "status": {"已保存"}}
	request := httptest.NewRequest("GET", "/api/items?"+query.Encode(), nil)
	recorder := httptest.NewRecorder()
	items(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "夏日动画") {
		t.Fatalf("filtered items: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestWebhookNotification(t *testing.T) {
	testDatabase(t)
	received := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- string(body)
		fmt.Fprint(w, "ok")
	}))
	defer server.Close()
	old := webhookURL
	webhookURL = server.URL
	defer func() { webhookURL = old }()
	id := insertItem(t, "magnet:?xt=urn:btih:test")
	notify("new_item", id, "待下载")
	select {
	case body := <-received:
		if !strings.Contains(body, `"event":"new_item"`) || !strings.Contains(body, "测试动画") {
			t.Fatalf("bad webhook: %s", body)
		}
	case <-time.After(time.Second):
		t.Fatal("webhook was not delivered")
	}
}

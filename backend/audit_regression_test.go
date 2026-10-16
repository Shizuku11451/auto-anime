package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBasicAuthSharesLoginThrottle(t *testing.T) {
	settingsFixture(t)
	for i := 0; i < 5; i++ {
		loginRequest("admin", "wrong")
	}
	if got := authenticatedStatus(nil, "admin", "test-password"); got != 429 {
		t.Fatalf("Basic Auth bypassed lockout: %d", got)
	}
	loginLimiter = newLoginThrottle()
	for i := 0; i < 5; i++ {
		authenticatedStatus(nil, "admin", "wrong")
	}
	if got := loginRequest("admin", "test-password").Code; got != 429 {
		t.Fatalf("login bypassed Basic lockout: %d", got)
	}
}

func TestLegacyPasswordUpgradesWithoutRevokingSessions(t *testing.T) {
	settingsFixture(t)
	legacy := "v1$salt$" + hashWebPassword("test-password", "salt")
	applyStoredSettings(map[string]string{"web_user": "admin", "web_hash": legacy})
	token, err := createSession("admin")
	if err != nil {
		t.Fatal(err)
	}
	if got := authenticatedStatus(nil, "admin", "test-password"); got != 200 {
		t.Fatalf("legacy auth: %d", got)
	}
	if !strings.HasPrefix(runtimeConfig().webHash, "v2$") || !verifyWebCredentials("admin", "test-password") || verifyWebCredentials("admin", "wrong") {
		t.Fatal("password migration failed")
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	if sessionUser(req) != "admin" {
		t.Fatal("migration revoked valid session")
	}
	password := strings.Repeat("长密码", 50)
	hash, err := newPasswordHash(password)
	if err != nil {
		t.Fatal(err)
	}
	applyStoredSettings(map[string]string{"web_user": "admin", "web_hash": hash})
	if !verifyWebCredentials("admin", password) {
		t.Fatal("long password was truncated")
	}
}

func TestArchiveCommitFailureRecoversWithoutCopy(t *testing.T) {
	testDatabase(t)
	srv := mediaServer(t)
	id := insertItem(t, srv.URL+"/video.mp4")
	db.Exec("UPDATE items SET status='下载中' WHERE id=?", id)
	path, err := fetchArchiveContext(context.Background(), id, srv.URL+"/video.mp4")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TRIGGER fail_saved BEFORE UPDATE OF status ON items WHEN NEW.status='已保存' BEGIN SELECT RAISE(FAIL,'simulated commit failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	if err = commitSavedArchive(id, path); err == nil {
		t.Fatal("failed update reported success")
	}
	if pending, err := pendingArchivePath(id); err != nil || pending != path {
		t.Fatalf("receipt lost: %s %v", pending, err)
	}
	db.Exec("DROP TRIGGER fail_saved")
	if err = recoverArchiveCommits(); err != nil {
		t.Fatal(err)
	}
	var status, stored string
	db.QueryRow("SELECT status,file_path FROM items WHERE id=?", id).Scan(&status, &stored)
	if status != "已保存" || stored != path {
		t.Fatalf("not recovered: %s %s", status, stored)
	}
	if pending, err := pendingArchivePath(id); err != nil || pending != "" {
		t.Fatal("receipt not cleared")
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestArchiveReceiptCannotOverrideCorrectionOrPause(t *testing.T) {
	testDatabase(t)
	srv := mediaServer(t)
	id := insertItem(t, srv.URL+"/video.mp4")
	db.Exec("UPDATE items SET status='下载中' WHERE id=?", id)
	path, err := fetchArchiveContext(context.Background(), id, srv.URL+"/video.mp4")
	if err != nil {
		t.Fatal(err)
	}
	db.Exec("UPDATE items SET status='已暂停' WHERE id=?", id)
	if err = recoverArchiveCommits(); err != nil {
		t.Fatal(err)
	}
	var status string
	db.QueryRow("SELECT status FROM items WHERE id=?", id).Scan(&status)
	if status != "已暂停" {
		t.Fatal("paused task changed")
	}
	db.Exec("UPDATE items SET manual=1,anime_title='人工纠正' WHERE id=?", id)
	if err = commitSavedArchive(id, path); err == nil {
		t.Fatal("stale receipt overwrote correction")
	}
	if pending, err := pendingArchivePath(id); err != nil || pending != "" {
		t.Fatal("stale receipt not retired")
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatal("retiring receipt deleted media")
	}
}

func TestArchiveDirectoryCommitRecoversWithoutCopy(t *testing.T) {
	testDatabase(t)
	id := insertItem(t, "https://example.test/collection.torrent")
	if _, err := db.Exec("UPDATE items SET status='下载中',downloader='qbittorrent',task_id='collection',episode='' WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	var files []archiveFile
	for _, name := range []string{"01.mp4", "02.mp4"} {
		file := filepath.Join(source, name)
		if err := os.WriteFile(file, []byte("collection-media"), 0600); err != nil {
			t.Fatal(err)
		}
		files = append(files, archiveFile{Rel: "Collection/" + name, Src: file})
	}
	path, err := archiveFromFiles(id, files)
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Fatalf("collection archive must be a directory: %v", err)
	}
	if _, err := db.Exec(`CREATE TRIGGER fail_saved BEFORE UPDATE OF status ON items WHEN NEW.status='已保存' BEGIN SELECT RAISE(FAIL,'simulated'); END`); err != nil {
		t.Fatal(err)
	}
	if err := commitSavedArchive(id, path); err == nil {
		t.Fatal("failed commit reported success")
	}
	if pending, err := pendingArchivePath(id); err != nil || pending != path {
		t.Fatalf("directory receipt cannot recover: %q %v", pending, err)
	}
	if _, err := db.Exec("DROP TRIGGER fail_saved"); err != nil {
		t.Fatal(err)
	}
	if err := recoverArchiveCommits(); err != nil {
		t.Fatal(err)
	}
	var status, stored string
	if err := db.QueryRow("SELECT status,file_path FROM items WHERE id=?", id).Scan(&status, &stored); err != nil {
		t.Fatal(err)
	}
	if status != "已保存" || stored != path {
		t.Fatalf("directory archive not recovered: %s %s", status, stored)
	}
	for _, name := range []string{"01.mp4", "02.mp4"} {
		if body, err := os.ReadFile(filepath.Join(path, name)); err != nil || string(body) != "collection-media" {
			t.Fatalf("archived media changed: %s %v", name, err)
		}
	}
}

func TestExternalArchiveRetryDoesNotDuplicateMedia(t *testing.T) {
	testDatabase(t)
	id := insertItem(t, "https://example.test/episode.torrent")
	if _, err := db.Exec("UPDATE items SET status='下载中',downloader='qbittorrent',task_id='test-task' WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "video.mp4")
	if err := os.WriteFile(source, []byte("test-media"), 0600); err != nil {
		t.Fatal(err)
	}
	ref := taskRef{ID: id, Downloader: "qbittorrent", TaskID: "test-task"}
	state := taskState{Files: []archiveFile{{Rel: "video.mp4", Src: source}}}
	if _, err := db.Exec(`CREATE TRIGGER fail_saved BEFORE UPDATE OF status ON items WHEN NEW.status='已保存' BEGIN SELECT RAISE(FAIL,'simulated'); END`); err != nil {
		t.Fatal(err)
	}
	applyTaskUpdateLocked(ref, state)
	first, err := pendingArchivePath(id)
	if err != nil || first == "" {
		t.Fatalf("missing pending archive: %s %v", first, err)
	}
	applyTaskUpdateLocked(ref, state)
	second, err := pendingArchivePath(id)
	if err != nil || second != first {
		t.Fatal("retry copied another archive")
	}
	db.Exec("DROP TRIGGER fail_saved")
	applyTaskUpdateLocked(ref, state)
	var stored string
	db.QueryRow("SELECT file_path FROM items WHERE id=?", id).Scan(&stored)
	if stored != first {
		t.Fatal("wrong archive committed")
	}
	count := 0
	filepath.Walk(downloadDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ".mp4" {
			count++
		}
		return nil
	})
	if count != 1 {
		t.Fatalf("duplicated media: %d", count)
	}
}

func TestItemPaginationFiltersBeforeLimit(t *testing.T) {
	testDatabase(t)
	for i := 0; i < 205; i++ {
		status := "已保存"
		if i < 102 {
			status = "已暂停"
		}
		if _, err := db.Exec("INSERT INTO items(title,link,status) VALUES(?,?,?)", fmt.Sprint(i), fmt.Sprint(i), status); err != nil {
			t.Fatal(err)
		}
	}
	read := func(url string) ([]struct {
		ID     int    `json:"id"`
		Status string `json:"status"`
	}, int) {
		t.Helper()
		rec := httptest.NewRecorder()
		items(rec, httptest.NewRequest("GET", url, nil))
		if rec.Code != 200 {
			t.Fatal(rec.Body.String())
		}
		var out struct {
			Items []struct {
				ID     int    `json:"id"`
				Status string `json:"status"`
			} `json:"items"`
			Next int `json:"next_before"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out.Items, out.Next
	}
	first, next := read("/api/items?page=1&group=已暂停")
	if len(first) != 100 || next == 0 || first[0].ID != 102 {
		t.Fatalf("old paused tasks hidden: %d %d", len(first), next)
	}
	second, last := read(fmt.Sprintf("/api/items?page=1&group=已暂停&before=%d", next))
	if len(second) != 2 || last != 0 || second[0].ID >= first[len(first)-1].ID {
		t.Fatal("cursor repeats or skips rows")
	}
}

func TestChecksumFetchFailureNeverDowngrades(t *testing.T) {
	old := updateSource
	updateSource = "owner/repo"
	t.Cleanup(func() { updateSource = old })
	file := filepath.Join(t.TempDir(), "app")
	os.WriteFile(file, []byte("payload"), 0600)
	for _, body := range []string{"unavailable", strings.Repeat("0", 64) + "  another-asset"} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if body == "unavailable" {
				w.WriteHeader(503)
			}
			fmt.Fprint(w, body)
		}))
		rel := &releaseInfo{AssetName: "app", SumURL: srv.URL, PublicURL: "https://github.com/owner/repo/releases/download/v1/app"}
		if _, err := verifyDownload(rel, file); err == nil {
			t.Error("failed/mismatched checksum accepted")
		}
		srv.Close()
	}
}

func TestSyncRequestCancellation(t *testing.T) {
	testDatabase(t)
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done() }))
	defer srv.Close()
	result, err := db.Exec("INSERT INTO feeds(name,url) VALUES('test',?)", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := result.LastInsertId()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := syncFeedContext(ctx, int(id)); done <- err }()
	<-started
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancel ignored")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sync did not cancel")
	}
	rec := httptest.NewRecorder()
	syncFeedHandler(rec, httptest.NewRequest("GET", "/api/sync/1", nil))
	if rec.Code != 405 {
		t.Fatal("GET can synchronize")
	}
}

func TestConcurrentRuntimeConfig(t *testing.T) {
	settingsFixture(t)
	old := runtimeConfig()
	t.Cleanup(func() { applyStoredSettings(map[string]string{"qbit_url": old.qbitURL, "qbit_user": old.qbitUser}) })
	applyStoredSettings(map[string]string{"qbit_url": "initial", "qbit_user": "initial"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") }))
	defer srv.Close()
	client := newClient(time.Second)
	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func(writer bool) {
			defer wg.Done()
			for i := 0; i < 80; i++ {
				if writer {
					v := fmt.Sprint(i)
					applyStoredSettings(map[string]string{"qbit_url": v, "qbit_user": v, "proxy_url": ""})
				} else {
					cfg := runtimeConfig()
					if cfg.qbitURL != cfg.qbitUser {
						t.Error("torn configuration")
					}
					res, err := client.Get(srv.URL)
					if err != nil {
						t.Error(err)
					} else {
						res.Body.Close()
					}
				}
			}
		}(worker == 0)
	}
	wg.Wait()
}

// 删除订阅必须先读完任务标识并关闭结果集，再走下载器 RPC：SQLite 连接池只有
// 一条连接，持着 rows 调网络（内置 qBittorrent 登录最长 30 秒）会让同步、
// 轮询和界面请求全部排队等待。
func TestDeleteFeedReleasesDatabaseBeforeCancellingTasks(t *testing.T) {
	testDatabase(t)
	probes := make(chan error, 4)
	var rpcCalls atomic.Int32
	useAria2RPCServer(t, func(w http.ResponseWriter, r *http.Request) {
		rpcCalls.Add(1)
		// 下载器响应期间数据库连接必须已经空闲：仍被占用时这里会等到超时。
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		var one int
		probes <- db.QueryRowContext(ctx, "SELECT 1").Scan(&one)
		fmt.Fprint(w, `{"result":"ok"}`)
	})
	feed, err := db.Exec("INSERT INTO feeds(name,url) VALUES('订阅','https://test/feed.xml')")
	if err != nil {
		t.Fatal(err)
	}
	feedID, _ := feed.LastInsertId()
	if _, err := db.Exec("INSERT INTO items(feed_id,title,link,downloader,task_id) VALUES(?,?,?,?,?)", feedID, "测试动画", "https://test/1", "aria2", "gid-1"); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	deleteFeed(rec, httptest.NewRequest("DELETE", fmt.Sprintf("/api/feeds/%d", feedID), nil))
	if rec.Code != 200 {
		t.Fatalf("删除订阅返回 %d", rec.Code)
	}
	if rpcCalls.Load() == 0 {
		t.Fatal("删除订阅没有取消下载器任务")
	}
	select {
	case probeErr := <-probes:
		if probeErr != nil {
			t.Fatalf("下载器 RPC 期间数据库连接仍被占用: %v", probeErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("下载器 RPC 未在预期时间内完成")
	}
	var feeds, items int
	if err := db.QueryRow("SELECT (SELECT COUNT(*) FROM feeds),(SELECT COUNT(*) FROM items)").Scan(&feeds, &items); err != nil {
		t.Fatal(err)
	}
	if feeds != 0 || items != 0 {
		t.Fatalf("删除订阅后仍有数据：feeds=%d items=%d", feeds, items)
	}
}

// 番剧删除只在事务提交后取消下载器任务：事务失败不能留下「记录还在、任务已被
// 取消」的不一致状态，成功时也不该在网络等待期间占住唯一连接。
func TestAnimeDeleteCancelsTasksOnlyAfterCommit(t *testing.T) {
	testDatabase(t)
	var rpcCalls atomic.Int32
	useAria2RPCServer(t, func(w http.ResponseWriter, r *http.Request) {
		rpcCalls.Add(1)
		fmt.Fprint(w, `{"result":"ok"}`)
	})
	id, err := ensureAnime("待删除番剧")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO items(anime_id,anime_title,title,link,status,downloader,task_id) VALUES(?,?,?,?,?,?,?)",
		id, "待删除番剧", "第 1 集", "https://test/ep1", "已暂停", "aria2", "gid-2"); err != nil {
		t.Fatal(err)
	}
	animeCleanupMu.Lock()
	defer animeCleanupMu.Unlock()
	if _, err := db.Exec("CREATE TRIGGER fail_anime_delete BEFORE DELETE ON animes BEGIN SELECT RAISE(FAIL,'simulated failure'); END"); err != nil {
		t.Fatal(err)
	}
	if _, status, err := manageAnime(animeManageInput{Action: "delete", IDs: []int64{id}, DeleteItems: true}); err == nil || status != 500 {
		t.Fatalf("事务失败仍报告成功：status=%d err=%v", status, err)
	}
	if rpcCalls.Load() != 0 {
		t.Fatal("事务回滚后仍取消了下载器任务")
	}
	if _, err := db.Exec("DROP TRIGGER fail_anime_delete"); err != nil {
		t.Fatal(err)
	}
	if _, status, err := manageAnime(animeManageInput{Action: "delete", IDs: []int64{id}, DeleteItems: true}); err != nil || status != 200 {
		t.Fatalf("删除番剧失败：status=%d err=%v", status, err)
	}
	if rpcCalls.Load() == 0 {
		t.Fatal("提交后未取消下载器任务")
	}
}

func importOPMLForTest(t *testing.T, body string) int {
	t.Helper()
	rec := httptest.NewRecorder()
	importOPML(rec, httptest.NewRequest("POST", "/api/import-opml", strings.NewReader(body)))
	if rec.Code != 200 {
		t.Fatalf("导入 OPML 返回 %d：%s", rec.Code, rec.Body.String())
	}
	var out map[string]int
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out["imported"]
}

// 重复导入同一份 OPML 时，冲突分支不写入任何行，不能计入新增数量。
func TestImportOPMLCountsOnlyNewFeeds(t *testing.T) {
	testDatabase(t)
	body := `<?xml version="1.0" encoding="UTF-8"?><opml version="2.0"><body>
		<outline text="A" xmlUrl="https://example.test/a.xml"/>
		<outline text="B" xmlUrl="https://example.test/b.xml"/>
	</body></opml>`
	if got := importOPMLForTest(t, body); got != 2 {
		t.Fatalf("首次导入新增 %d，期望 2", got)
	}
	if got := importOPMLForTest(t, body); got != 0 {
		t.Fatalf("重复导入新增 %d，期望 0", got)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM feeds").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("订阅行数 %d，期望 2", count)
	}
}

// 列表接口在数据库不可用时必须返回 500；丢弃 db.Query 错误会在 defer
// rows.Close() 上触发 nil 指针 panic。
func TestFeedsListReportsDatabaseFailure(t *testing.T) {
	testDatabase(t)
	db.Close()
	defer initDB()
	rec := httptest.NewRecorder()
	feeds(rec, httptest.NewRequest("GET", "/api/feeds", nil))
	if rec.Code != 500 {
		t.Fatalf("数据库不可用时返回 %d，期望 500", rec.Code)
	}
}

// 创建订阅没有空名称校验、URL 只做 http 前缀检查时，会写入无名订阅，
// 并让 "httpfoo://" 这类无法抓取的地址拖到同步阶段才报错。
func TestCreateFeedValidatesNameAndURL(t *testing.T) {
	testDatabase(t)
	for _, tc := range []struct{ label, body, want string }{
		{"空名称", `{"name":"   ","url":"https://example.test/rss"}`, "订阅名称不能为空"},
		{"伪协议", `{"name":"订阅","url":"httpfoo://example.test/rss"}`, "RSS 地址无效"},
		{"缺主机名", `{"name":"订阅","url":"http:///rss"}`, "RSS 地址无效"},
		{"无协议", `{"name":"订阅","url":"example.test/rss"}`, "RSS 地址无效"},
	} {
		rec := httptest.NewRecorder()
		feeds(rec, httptest.NewRequest("POST", "/api/feeds", strings.NewReader(tc.body)))
		if rec.Code != 400 || !strings.Contains(rec.Body.String(), tc.want) {
			t.Fatalf("%s 应返回 400 %q：%d %s", tc.label, tc.want, rec.Code, rec.Body.String())
		}
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM feeds").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("非法订阅被写入：%d 条", count)
	}
}

// 创建订阅会预建番剧记录并写 animes.storage_dir，必须和自动清理、改名的写锁互斥：
// 测试持写锁期间发起请求，请求应阻塞到锁释放，而不是在清理窗口里改番剧。
func TestCreateFeedTakesAnimeCleanupLock(t *testing.T) {
	testDatabase(t)
	body, _ := json.Marshal(map[string]any{
		"name": "订阅", "url": "https://example.test/rss",
		"target_title": "夏日动画", "storage_dir": filepath.ToSlash(t.TempDir()),
	})
	done := make(chan int, 1)
	animeCleanupMu.Lock()
	go func() {
		rec := httptest.NewRecorder()
		feeds(rec, httptest.NewRequest("POST", "/api/feeds", strings.NewReader(string(body))))
		done <- rec.Code
	}()
	select {
	case code := <-done:
		animeCleanupMu.Unlock()
		t.Fatalf("写锁期间创建订阅已完成（%d），未持有 animeCleanupMu", code)
	case <-time.After(300 * time.Millisecond):
	}
	animeCleanupMu.Unlock()
	select {
	case code := <-done:
		if code != 200 {
			t.Fatalf("释放锁后创建订阅返回 %d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("释放锁后创建订阅仍未完成")
	}
}

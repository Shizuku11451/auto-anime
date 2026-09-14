package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDownloadLimitSettings(t *testing.T) {
	testDatabase(t)
	old := configuredDownloads.Load()
	t.Cleanup(func() { configuredDownloads.Store(old) })
	configuredDownloads.Store(0)
	if downloadLimit() != 2 {
		t.Fatal("default must be 2")
	}
	for _, value := range []string{"0", "33", "-1", "1.5", "null"} {
		if value == "null" {
			continue
		}
		rec := settingsPUT(t, `{"max_concurrent_downloads":`+value+`}`)
		if rec.Code != 400 || downloadLimit() != 2 {
			t.Fatalf("invalid %s: %d", value, rec.Code)
		}
	}
	if rec := settingsPUT(t, `{"max_concurrent_downloads":7}`); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if downloadLimit() != 7 {
		t.Fatal("setting not applied")
	}
	if rec := settingsPUT(t, `{}`); rec.Code != 200 || downloadLimit() != 7 {
		t.Fatal("omitted setting changed limit")
	}
	configuredDownloads.Store(0)
	loadSettings()
	if downloadLimit() != 7 {
		t.Fatal("setting not restored")
	}
	rec := httptest.NewRecorder()
	settingsHandler(rec, httptest.NewRequest("GET", "/api/settings", nil))
	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	if out["max_concurrent_downloads"] != float64(7) {
		t.Fatal(out)
	}
	if _, err := db.Exec(`CREATE TRIGGER reject_limit BEFORE UPDATE ON settings WHEN NEW.key='max_concurrent_downloads' BEGIN SELECT RAISE(FAIL,'blocked'); END`); err != nil {
		t.Fatal(err)
	}
	if rec := settingsPUT(t, `{"max_concurrent_downloads":4}`); rec.Code == 200 || downloadLimit() != 7 {
		t.Fatal("failed save changed live limit")
	}
}

func TestDownloadClaimsIncludeExternalTasks(t *testing.T) {
	testDatabase(t)
	old := configuredDownloads.Load()
	t.Cleanup(func() { configuredDownloads.Store(old) })
	configuredDownloads.Store(2)
	// Existing tasks represent downloads recovered from a prior process.
	for _, mode := range []string{"aria2", "qbittorrent"} {
		id := insertItem(t, "https://example.test/"+mode)
		db.Exec("UPDATE items SET status='下载中',downloader=?,task_id='existing' WHERE id=?", mode, id)
	}
	id := insertItem(t, "https://example.test/new")
	db.Exec("UPDATE items SET status='等待下载' WHERE id=?", id)
	if claimed, waiting, err := claimDownload(id); claimed || !waiting || err != nil {
		t.Fatalf("existing tasks not counted: %v %v %v", claimed, waiting, err)
	}
	configuredDownloads.Store(3)
	if claimed, _, err := claimDownload(id); !claimed || err != nil {
		t.Fatal("raising limit must admit task", err)
	}
	configuredDownloads.Store(1)
	next := insertItem(t, "https://example.test/next")
	db.Exec("UPDATE items SET status='等待下载' WHERE id=?", next)
	if claimed, waiting, err := claimDownload(next); claimed || !waiting || err != nil {
		t.Fatal("lowering limit admitted task")
	}
	db.Exec("UPDATE items SET status='已保存' WHERE status='下载中'")
	if claimed, _, err := claimDownload(next); !claimed || err != nil {
		t.Fatal("completion did not release capacity")
	}
}

func TestConcurrentClaimsNeverExceedLimit(t *testing.T) {
	testDatabase(t)
	old := configuredDownloads.Load()
	t.Cleanup(func() { configuredDownloads.Store(old) })
	configuredDownloads.Store(3)
	var ids []int
	for i := 0; i < 20; i++ {
		id := insertItem(t, fmt.Sprintf("https://example.test/%d", i))
		db.Exec("UPDATE items SET status='等待下载' WHERE id=?", id)
		ids = append(ids, id)
	}
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if _, _, err := claimDownload(id); err != nil {
				t.Error(err)
			}
		}(id)
	}
	wg.Wait()
	var count int
	db.QueryRow("SELECT COUNT(*) FROM items WHERE status='下载中'").Scan(&count)
	if count != 3 {
		t.Fatalf("active=%d", count)
	}
}

func TestSubmittedTaskHoldsSlotUntilFailure(t *testing.T) {
	testDatabase(t)
	oldLimit, oldURL := configuredDownloads.Load(), aria2URL
	configuredDownloads.Store(1)
	t.Cleanup(func() { configuredDownloads.Store(oldLimit); aria2URL = oldURL })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"result":"test-gid"}`)
	}))
	defer server.Close()
	downloadMode, aria2URL = "aria2", server.URL
	first := insertItem(t, "https://example.test/first.mp4")
	second := insertItem(t, "https://example.test/second.mp4")
	downloadItem(first)
	done := make(chan struct{})
	go func() { downloadItem(second); close(done) }()
	defer func() {
		db.Exec("UPDATE items SET status='下载失败' WHERE id=?", first)
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("queue worker did not finish")
		}
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		var status string
		db.QueryRow("SELECT status FROM items WHERE id=?", second).Scan(&status)
		if status == "等待下载" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("second download did not queue", status)
		}
		time.Sleep(10 * time.Millisecond)
	}
	select {
	case <-done:
		t.Fatal("external task released slot at submission")
	default:
	}
	applyTaskUpdate(taskRef{ID: first, Downloader: "aria2", TaskID: "test-gid"}, taskState{Fail: "test failure"})
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("failed task did not release slot")
	}
	var status string
	db.QueryRow("SELECT status FROM items WHERE id=?", second).Scan(&status)
	if status != "已发送到 Aria2" {
		t.Fatal(status)
	}
}

func TestTransferSpeedAPIsAndExpiry(t *testing.T) {
	testDatabase(t)
	id := insertItem(t, "https://example.test/speed")
	aid, err := ensureAnime("测试动画")
	if err != nil {
		t.Fatal(err)
	}
	db.Exec("UPDATE items SET anime_id=?,status='下载中',progress=25 WHERE id=?", aid, id)
	t.Cleanup(func() { clearTransfer(id) })
	setTransfer(id, 1048576, 27, time.Minute)
	rec := httptest.NewRecorder()
	items(rec, httptest.NewRequest("GET", "/api/items", nil))
	if !strings.Contains(rec.Body.String(), `"download_speed":1048576`) || !strings.Contains(rec.Body.String(), `"progress":27`) {
		t.Fatal(rec.Body.String())
	}
	rec = httptest.NewRecorder()
	animeDetail(rec, aid)
	if !strings.Contains(rec.Body.String(), `"download_speed":1048576`) {
		t.Fatal(rec.Body.String())
	}
	if speed, _ := transferSnapshot(id, "已保存", 100); speed != 0 {
		t.Fatal("saved task has speed")
	}
	if speed, _ := transferSnapshot(id, "下载失败", 0); speed != 0 {
		t.Fatal("failed task has speed")
	}
	setTransfer(id, 100, 27, -time.Second)
	if speed, progress := transferSnapshot(id, "下载中", 25); speed != 0 || progress != 25 {
		t.Fatal("stale speed returned")
	}
	r := &transferReader{reader: strings.NewReader(strings.Repeat("x", 10000)), id: id, total: 20000, windowStart: time.Now().Add(-time.Second)}
	if _, err := io.Copy(io.Discard, r); err != nil {
		t.Fatal(err)
	}
	if speed, progress := transferSnapshot(id, "下载中", 0); speed <= 0 || progress <= 0 {
		t.Fatal("direct transfer did not report speed")
	}
}

func TestAllDownloaderSpeeds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/aria":
			fmt.Fprint(w, `{"result":{"status":"active","completedLength":"20","totalLength":"100","downloadSpeed":"12345"}}`)
		case "/api/v2/torrents/info":
			fmt.Fprint(w, `[{"hash":"abc","state":"downloading","progress":0.2,"dlspeed":23456}]`)
		default:
			fmt.Fprint(w, `{"result":"success","arguments":{"torrents":[{"id":1,"status":4,"percentDone":0.2,"rateDownload":34567}]}}`)
		}
	}))
	defer server.Close()
	oldAria, oldQbit, oldTransmission := aria2URL, qbitURL, transmissionURL
	t.Cleanup(func() { aria2URL, qbitURL, transmissionURL = oldAria, oldQbit, oldTransmission })
	aria2URL, qbitURL, transmissionURL = server.URL+"/aria", server.URL, server.URL
	aria, err := aria2Poll("gid")
	if err != nil || aria.DownloadSpeed != 12345 {
		t.Fatal(aria, err)
	}
	qbit, err := qbitPoll(&qbitClient{http: server.Client(), config: qbitConnection{URL: server.URL}}, taskRef{TaskID: "abc"})
	if err != nil || qbit.DownloadSpeed != 23456 {
		t.Fatal(qbit, err)
	}
	tr, err := transmissionPoll(&transmissionClient{}, taskRef{TaskID: "1"})
	if err != nil || tr.DownloadSpeed != 34567 {
		t.Fatal(tr, err)
	}
}

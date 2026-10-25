package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestRecoveryAcrossDatabaseReopen(t *testing.T) {
	testDatabase(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "video/mp4")
		fmt.Fprint(w, "media")
	}))
	defer server.Close()
	for _, f := range []struct{ id, enabled, auto int }{{1, 1, 1}, {2, 0, 1}, {3, 1, 0}} {
		if _, err := db.Exec("INSERT INTO feeds(id,name,url,enabled,auto_download) VALUES(?,'番剧',?,?,?)", f.id, fmt.Sprintf("https://example.test/%d", f.id), f.enabled, f.auto); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		feed          int
		before, after string
	}{
		{1, "等待下载", "已保存"}, {1, "待下载", "已保存"},
		{1, "下载中", "下载失败"}, {1, "已保存", "已保存"},
		{1, "已发送到 qBittorrent", "已发送到 qBittorrent"},
		{2, "等待下载", "待下载"}, {2, "待下载", "待下载"},
		{3, "等待下载", "已保存"}, {3, "待下载", "待下载"},
		{1, "下载失败", "下载失败"},
	}
	for i, c := range cases {
		if _, err := db.Exec("INSERT INTO items(id,feed_id,title,link,status) VALUES(?,?,'动画',?,?)", i+1, c.feed, fmt.Sprintf("%s/%d.mp4", server.URL, i), c.before); err != nil {
			t.Fatal(err)
		}
	}
	// Reopen the actual on-disk database, as a fresh process would.
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	initDB()
	ids, err := recoverDownloads(db)
	if err != nil || !reflect.DeepEqual(ids, []int{1, 2, 8}) {
		t.Fatalf("recovery IDs: %v %v", ids, err)
	}
	resumeDownloads(ids)
	if calls.Load() != 3 {
		t.Fatalf("network requests: %d", calls.Load())
	}
	for i, c := range cases {
		var status string
		if err := db.QueryRow("SELECT status FROM items WHERE id=?", i+1).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != c.after {
			t.Errorf("item %d: want %s got %s", i+1, c.after, status)
		}
	}
	ids, err = recoverDownloads(db)
	if err != nil || len(ids) != 0 {
		t.Fatalf("second startup duplicated work: %v %v", ids, err)
	}
}

func TestRecoveryKeepsTrackedDownloaderTasks(t *testing.T) {
	testDatabase(t)
	if _, err := db.Exec("INSERT INTO items(title,link,status,downloader,task_id) VALUES('外部任务','magnet:?xt=urn:btih:0123456789012345678901234567890123456789','下载中','aria2','gid-9')"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO items(title,link,status) VALUES('直链任务','https://example.test/a.mp4','下载中')"); err != nil {
		t.Fatal(err)
	}
	if _, err := recoverDownloads(db); err != nil {
		t.Fatal(err)
	}
	var tracked, direct string
	if err := db.QueryRow("SELECT status FROM items WHERE task_id='gid-9'").Scan(&tracked); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT status FROM items WHERE task_id='' AND link='https://example.test/a.mp4'").Scan(&direct); err != nil {
		t.Fatal(err)
	}
	if tracked != "下载中" {
		t.Fatalf("tracked task reset to %s", tracked)
	}
	if direct != "下载失败" {
		t.Fatalf("direct task left as %s", direct)
	}
	refs, err := activeTaskRefs()
	if err != nil || len(refs) != 1 || refs[0].ID == 0 || refs[0].TaskID != "gid-9" {
		t.Fatalf("poller refs after recovery: %+v %v", refs, err)
	}
}

// 旧版内置 qBittorrent 已随本版本移除：启动恢复必须把它的活跃与暂停任务退役成
// 可重试的失败状态，并撤销永远无法完成的源文件清理；外部 qBittorrent 任务不受影响。
func TestRecoveryRetiresRemovedBuiltinTasks(t *testing.T) {
	testDatabase(t)
	for _, item := range []struct {
		id         int
		status     string
		downloader string
		task       string
	}{
		{1, "下载中", "builtin", "hash-1"},
		{2, "已暂停", "builtin", "hash-2"},
		{3, "等待继续", "builtin", "hash-3"},
		{4, "已保存", "builtin", "hash-4"},
		{5, "下载中", "qbittorrent", "hash-5"},
	} {
		if _, err := db.Exec("INSERT INTO items(id,title,link,status,downloader,task_id) VALUES(?,'动画',?,?,?,?)", item.id, fmt.Sprintf("magnet:?xt=urn:btih:%040d", item.id), item.status, item.downloader, item.task); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec("INSERT INTO source_cleanup(item_id,downloader,task_id,submitted_at,archive_path,files,source_root) VALUES(4,'builtin','hash-4',1,'/media/a.mp4','[]','/root')"); err != nil {
		t.Fatal(err)
	}
	if _, err := recoverDownloads(db); err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct {
		id     int
		status string
		failed bool
	}{
		{1, "下载失败", true},
		{2, "下载失败", true},
		{3, "下载失败", true},
		{4, "已保存", false},
		{5, "下载中", false},
	} {
		var status, taskError string
		if err := db.QueryRow("SELECT status,COALESCE(task_error,'') FROM items WHERE id=?", want.id).Scan(&status, &taskError); err != nil {
			t.Fatal(err)
		}
		if status != want.status || want.failed != (taskError != "") {
			t.Fatalf("item %d after recovery: %s %q", want.id, status, taskError)
		}
	}
	var pending int
	if err := db.QueryRow("SELECT COUNT(*) FROM source_cleanup WHERE downloader='builtin'").Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Fatalf("builtin source cleanup rows left: %d", pending)
	}
	refs, err := activeTaskRefs()
	if err != nil || len(refs) != 1 || refs[0].Downloader != "qbittorrent" {
		t.Fatalf("poller refs after builtin retirement: %+v %v", refs, err)
	}
}

func TestMissingDownloaderDoesNotFallBackToDirect(t *testing.T) {
	testDatabase(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	downloadMode, aria2URL = "aria2", ""
	id := insertItem(t, server.URL+"/test.mp4")
	downloadItem(id)
	var status string
	if err := db.QueryRow("SELECT status FROM items WHERE id=?", id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "下载失败" || calls.Load() != 0 {
		t.Fatalf("missing downloader: %s calls=%d", status, calls.Load())
	}
}

func TestInvalidResourceDoesNotRemainDownloading(t *testing.T) {
	testDatabase(t)
	id := insertItem(t, "file:///bad-resource")
	downloadItem(id)
	var status string
	if err := db.QueryRow("SELECT status FROM items WHERE id=?", id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "下载失败" {
		t.Fatalf("invalid resource stuck in %s", status)
	}
}

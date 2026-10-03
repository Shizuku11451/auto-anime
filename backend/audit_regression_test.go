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
	path, err := fetchArchive(id, srv.URL+"/video.mp4")
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
	path, err := fetchArchive(id, srv.URL+"/video.mp4")
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
	applyTaskUpdate(ref, state)
	first, err := pendingArchivePath(id)
	if err != nil || first == "" {
		t.Fatalf("missing pending archive: %s %v", first, err)
	}
	applyTaskUpdate(ref, state)
	second, err := pendingArchivePath(id)
	if err != nil || second != first {
		t.Fatal("retry copied another archive")
	}
	db.Exec("DROP TRIGGER fail_saved")
	applyTaskUpdate(ref, state)
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

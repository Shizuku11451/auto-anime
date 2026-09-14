package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEmptyAnimeCleanupPreservesAssociationsAndConfiguration(t *testing.T) {
	testDatabase(t)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, title := range []string{"empty", "auto", "item", "legacy-item", "feed", "paused", "manual", "binding", "season", "storage", "search", "include", "exclude", "legacy-storage"} {
		exec("INSERT INTO animes(title) VALUES(?)", title)
	}
	exec("INSERT INTO items(anime_id,title,link) SELECT id,'resource','item-link' FROM animes WHERE title='item'")
	exec("INSERT INTO items(anime_title,title,link) VALUES('legacy-item','resource','legacy-link')")
	exec("INSERT INTO feeds(name,url,target_title,enabled) VALUES('feed','https://test/1','feed',1),('paused','https://test/2','paused',0)")
	exec("INSERT INTO anime_meta(anime_title,source) VALUES('auto','bangumi'),('manual','manual')")
	for column, title := range map[string]string{"bangumi_id": "binding", "season": "season", "storage_dir": "storage", "search_name": "search", "include_rules": "include", "exclude_rules": "exclude"} {
		exec("UPDATE animes SET "+column+"=? WHERE title=?", 1, title)
	}
	exec("INSERT INTO anime_storage(anime_title,storage_dir) VALUES('legacy-storage','/library')")
	media := filepath.Join(downloadDir, "keep.mp4")
	if err := os.WriteFile(media, []byte("media"), 0600); err != nil {
		t.Fatal(err)
	}
	if n, err := cleanupEmptyAnime(); err != nil || n != 0 {
		t.Fatalf("default: %d %v", n, err)
	}
	exec("INSERT INTO settings(key,value) VALUES('auto_cleanup_empty_anime','1')")
	if n, err := cleanupEmptyAnime(); err != nil || n != 2 {
		t.Fatalf("cleanup: %d %v", n, err)
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM animes").Scan(&count)
	if count != 12 {
		t.Fatalf("protected rows lost: %d", count)
	}
	db.QueryRow("SELECT COUNT(*) FROM anime_meta WHERE anime_title='auto'").Scan(&count)
	if count != 0 {
		t.Fatal("automatic metadata remains")
	}
	if data, err := os.ReadFile(media); err != nil || string(data) != "media" {
		t.Fatal("media changed")
	}
	if n, err := cleanupEmptyAnime(); err != nil || n != 0 {
		t.Fatalf("repeat: %d %v", n, err)
	}
	exec("DELETE FROM feeds WHERE target_title='feed'")
	if n, err := cleanupEmptyAnime(); err != nil || n != 1 {
		t.Fatalf("unlinked: %d %v", n, err)
	}
}

func TestEmptyAnimeCleanupRollsBack(t *testing.T) {
	testDatabase(t)
	for _, query := range []string{
		"INSERT INTO settings(key,value) VALUES('auto_cleanup_empty_anime','1')",
		"INSERT INTO animes(title) VALUES('empty')",
		"INSERT INTO anime_meta(anime_title,source) VALUES('empty','bangumi')",
		"CREATE TRIGGER fail_cleanup BEFORE DELETE ON anime_meta BEGIN SELECT RAISE(ABORT,'test failure'); END",
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := cleanupEmptyAnime(); err == nil {
		t.Fatal("expected failure")
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM animes").Scan(&count)
	if count != 1 {
		t.Fatal("partial deletion was committed")
	}
}

func TestEmptyAnimeCleanupSetting(t *testing.T) {
	testDatabase(t)
	get := func() bool {
		t.Helper()
		rec := httptest.NewRecorder()
		settingsHandler(rec, httptest.NewRequest("GET", "/api/settings", nil))
		var s struct {
			Enabled bool `json:"auto_cleanup_empty_anime"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
			t.Fatal(err)
		}
		return s.Enabled
	}
	put := func(body string, code int) {
		t.Helper()
		rec := httptest.NewRecorder()
		settingsHandler(rec, httptest.NewRequest("PUT", "/api/settings", strings.NewReader(body)))
		if rec.Code != code {
			t.Fatalf("save: %d %s", rec.Code, rec.Body.String())
		}
	}
	if get() {
		t.Fatal("must default off")
	}
	if _, err := db.Exec("INSERT INTO animes(title) VALUES('empty')"); err != nil {
		t.Fatal(err)
	}
	put(`{"auto_cleanup_empty_anime":true,"poll_minutes":1}`, 400)
	if get() {
		t.Fatal("invalid settings partially enabled cleanup")
	}
	put(`{"auto_cleanup_empty_anime":true}`, 200)
	if !get() {
		t.Fatal("setting not persisted")
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM animes").Scan(&count)
	if count != 0 {
		t.Fatal("enabling must clean immediately")
	}
	put(`{}`, 200)
	if !get() {
		t.Fatal("omitted setting reset")
	}
	put(`{"auto_cleanup_empty_anime":false}`, 200)
	if get() {
		t.Fatal("disable not saved")
	}
	if _, err := db.Exec("INSERT INTO animes(title) VALUES('empty')"); err != nil {
		t.Fatal(err)
	}
	if n, err := cleanupEmptyAnime(); err != nil || n != 0 {
		t.Fatalf("disabled: %d %v", n, err)
	}
}

func TestEmptyAnimeCleanupWaitsForFirstResource(t *testing.T) {
	testDatabase(t)
	if err := setSetting("auto_cleanup_empty_anime", "1"); err != nil {
		t.Fatal(err)
	}
	animeCleanupMu.RLock()
	id, err := ensureAnime("正在入库")
	if err != nil {
		animeCleanupMu.RUnlock()
		t.Fatal(err)
	}
	type result struct {
		n   int
		err error
	}
	done := make(chan result, 1)
	go func() { n, err := cleanupEmptyAnime(); done <- result{n, err} }()
	select {
	case r := <-done:
		animeCleanupMu.RUnlock()
		t.Fatalf("cleanup ran during resource attachment: %+v", r)
	case <-time.After(30 * time.Millisecond):
	}
	_, err = db.Exec("INSERT INTO items(anime_id,title,link) VALUES(?,'首集','https://test/first')", id)
	animeCleanupMu.RUnlock()
	r := <-done
	if err != nil || r.err != nil || r.n != 0 {
		t.Fatalf("resource attachment: %v cleanup: %+v", err, r)
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM animes WHERE id=?", id).Scan(&count)
	if count != 1 {
		t.Fatal("in-flight series was deleted")
	}
}

package main

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCoverContentHashAndPosterRefresh(t *testing.T) {
	testDatabase(t)
	withMetaTestEnv(t)
	if _, err := ensureAnime("测试动画"); err != nil {
		t.Fatal(err)
	}
	type picture struct{ body, contentType, ext string }
	var current atomic.Value
	current.Store(picture{"first-picture", "image/jpeg", ".jpg"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/characters") {
			fmt.Fprint(w, `[]`)
			return
		}
		p := current.Load().(picture)
		w.Header().Set("Content-Type", p.contentType)
		fmt.Fprint(w, p.body)
	}))
	oldAPI, oldClient, oldAllowed := bangumiAPI, metaClient, coverHostAllowed
	bangumiAPI, metaClient = server.URL, server.Client()
	coverHostAllowed = func(host string) bool { return host == server.Listener.Addr().String() }
	t.Cleanup(func() {
		server.Close()
		bangumiAPI, metaClient, coverHostAllowed = oldAPI, oldClient, oldAllowed
	})
	legacy := "0123456789abcdef.jpg"
	if err := os.WriteFile(filepath.Join(coversDir, legacy), []byte("legacy-picture"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO anime_meta(anime_title,cover_file,source) VALUES('测试动画',?,'bangumi')`, legacy); err != nil {
		t.Fatal(err)
	}
	refreshSeriesPoster("测试动画")
	root, _, _ := archiveTarget("测试动画", "", "", 0)
	if data, err := os.ReadFile(filepath.Join(root, "poster.jpg")); err != nil || string(data) != "legacy-picture" {
		t.Fatalf("legacy poster: %q %v", data, err)
	}
	subject := &bangumiSubject{ID: 4242, NameCN: "测试动画", Summary: "测试简介"}
	subject.Images.Large = server.URL + "/cover"
	names := map[string]string{legacy: "legacy-picture"}
	previous := ""
	for _, pic := range []picture{{"first-picture", "image/jpeg", ".jpg"}, {"second-picture", "image/jpeg", ".jpg"}, {"third-picture", "image/png", ".png"}} {
		current.Store(pic)
		if err := os.WriteFile(filepath.Join(root, "poster.gif"), []byte("stale"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := storeMetaSubject("测试动画", subject, "bangumi", "", nil); err != nil {
			t.Fatal(err)
		}
		var name string
		if err := db.QueryRow(`SELECT cover_file FROM anime_meta WHERE anime_title='测试动画'`).Scan(&name); err != nil {
			t.Fatal(err)
		}
		urlHash, contentHash := sha1.Sum([]byte(subject.Images.Large)), sha256.Sum256([]byte(pic.body))
		want := hex.EncodeToString(urlHash[:6]) + "-" + hex.EncodeToString(contentHash[:4]) + pic.ext
		if name != want || name == previous || !coverFilePattern.MatchString(name) {
			t.Fatalf("content-addressed name: got=%q want=%q previous=%q", name, want, previous)
		}
		if same, err := cacheCover(subject.Images.Large); err != nil || same != name {
			t.Fatalf("identical bytes changed name: %q %v", same, err)
		}
		if data, err := os.ReadFile(filepath.Join(root, "poster"+pic.ext)); err != nil || string(data) != pic.body {
			t.Fatalf("poster did not refresh for %q: %q %v", name, data, err)
		}
		posters, err := filepath.Glob(filepath.Join(root, "poster.*"))
		if err != nil || len(posters) != 1 {
			t.Fatalf("stale poster extensions remain: %v %v", posters, err)
		}
		names[name], previous = pic.body, name
	}
	// Old URL-only covers and both new image versions keep working under the
	// immutable route. Replacing a picture must not mutate an older URL's bytes.
	for name, body := range names {
		rec := httptest.NewRecorder()
		serveCover(rec, httptest.NewRequest(http.MethodGet, "/api/covers/"+name, nil))
		if rec.Code != http.StatusOK || rec.Body.String() != body || rec.Header().Get("Cache-Control") != "public, max-age=2592000, immutable" {
			t.Fatalf("cover compatibility: name=%q status=%d body=%q cache=%q", name, rec.Code, rec.Body.String(), rec.Header().Get("Cache-Control"))
		}
	}
}

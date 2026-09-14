package main

import (
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

func postAnimeRematch(id int64) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	animeEntryAPI(rec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/animes/%d/rematch", id), nil))
	return rec
}

func seedRematchAnime(t *testing.T, source string, binding int64) int64 {
	t.Helper()
	id, err := ensureAnime("测试动画")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE animes SET bangumi_id=?,season=2,include_rules='测试',updated_at='2026-01-01T00:00:00Z' WHERE id=?`, binding, id); err != nil {
		t.Fatal(err)
	}
	if source != "" {
		if _, err := db.Exec(`INSERT INTO anime_meta(anime_title,subject_id,source,rating,summary,cover_file) VALUES('测试动画',999,?,8.5,'旧匹配','0123456789abcdef.jpg')`, source); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func TestAnimeRematchClearsAndQueues(t *testing.T) {
	testDatabase(t)
	startMetaTestServer(t, "测试动画")
	id := seedRematchAnime(t, "bangumi", 0)
	before, err := getAnime(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(coversDir, "0123456789abcdef.jpg"), []byte("old-cover"), 0644); err != nil {
		t.Fatal(err)
	}
	refreshSeriesPoster(before.Title)
	root, _, _ := archiveTarget(before.Title, "", "", 0)
	media := filepath.Join(root, "episode.mp4")
	if err := os.WriteFile(media, []byte("preserve-media"), 0644); err != nil {
		t.Fatal(err)
	}
	rec := postAnimeRematch(id)
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), `"queued":true`) {
		t.Fatalf("rematch: %d %s", rec.Code, rec.Body.String())
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM anime_meta WHERE anime_title='测试动画'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("metadata was not cleared: %d %v", count, err)
	}
	after, err := getAnime(id)
	if err != nil || before.UpdatedAt == after.UpdatedAt {
		t.Fatalf("lookup generation was not changed: %+v %v", after, err)
	}
	before.UpdatedAt = after.UpdatedAt
	if *before != *after {
		t.Fatalf("rematch changed user configuration: before=%+v after=%+v", before, after)
	}
	if _, err := os.Stat(filepath.Join(root, "poster.jpg")); !os.IsNotExist(err) {
		t.Fatalf("cleared metadata left old poster: %v", err)
	}
	if data, err := os.ReadFile(media); err != nil || string(data) != "preserve-media" {
		t.Fatalf("rematch changed media: %q %v", data, err)
	}
	select {
	case title := <-metaQueue:
		if title != "测试动画" {
			t.Fatalf("queued %q", title)
		}
		lookupAndCacheMeta(title)
	default:
		t.Fatal("rematch was not queued")
	}
	var subjectID int
	if err := db.QueryRow(`SELECT subject_id FROM anime_meta WHERE anime_title='测试动画'`).Scan(&subjectID); err != nil || subjectID != 4242 {
		t.Fatalf("new match: %d %v", subjectID, err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "poster.jpg")); err != nil || string(data) != "fake-jpeg-bytes" {
		t.Fatalf("poster did not follow rematch: %q %v", data, err)
	}
}

func TestAnimeRematchRejectsManualBindings(t *testing.T) {
	for _, tc := range []struct {
		source  string
		binding int64
	}{{"manual", 4242}, {"manual", 0}, {"bangumi", 4242}} {
		t.Run(fmt.Sprintf("%s/%d", tc.source, tc.binding), func(t *testing.T) {
			testDatabase(t)
			withMetaTestEnv(t)
			id := seedRematchAnime(t, tc.source, tc.binding)
			rec := postAnimeRematch(id)
			if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "解除绑定") || len(metaQueue) != 0 {
				t.Fatalf("manual protection: %d %s queued=%d", rec.Code, rec.Body.String(), len(metaQueue))
			}
			var subjectID int
			if err := db.QueryRow(`SELECT subject_id FROM anime_meta WHERE anime_title='测试动画'`).Scan(&subjectID); err != nil || subjectID != 999 {
				t.Fatalf("rejected rematch changed metadata: %d %v", subjectID, err)
			}
		})
	}
}

func TestAnimeRematchRejectsInvalidRequests(t *testing.T) {
	testDatabase(t)
	withMetaTestEnv(t)
	id := seedRematchAnime(t, "bangumi", 0)
	for _, path := range []string{"0", "-1", "no-id", "999999", "999999999999999999999999", "1/rematch/extra"} {
		rec := httptest.NewRecorder()
		target := "/api/animes/" + path + "/rematch"
		animeEntryAPI(rec, httptest.NewRequest(http.MethodPost, target, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("invalid ID/path %q: %d", target, rec.Code)
		}
	}
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		animeEntryAPI(rec, httptest.NewRequest(method, fmt.Sprintf("/api/animes/%d/rematch", id), nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("method %s: %d", method, rec.Code)
		}
	}
	if len(metaQueue) != 0 {
		t.Fatal("invalid request queued a lookup")
	}
}

func TestAnimeRematchFailurePreservesOriginalMatch(t *testing.T) {
	for _, kind := range []string{"disabled", "full-queue", "database"} {
		t.Run(kind, func(t *testing.T) {
			testDatabase(t)
			withMetaTestEnv(t)
			id := seedRematchAnime(t, "bangumi", 0)
			wantStatus := http.StatusConflict
			switch kind {
			case "disabled":
				bangumiEnabled = false
			case "full-queue":
				metaQueue = make(chan string, 1)
				metaQueue <- "队列中的其他番剧"
				wantStatus = http.StatusServiceUnavailable
			case "database":
				if _, err := db.Exec(`CREATE TRIGGER fail_rematch BEFORE UPDATE OF updated_at ON animes BEGIN SELECT RAISE(ABORT,'test rollback'); END`); err != nil {
					t.Fatal(err)
				}
				wantStatus = http.StatusInternalServerError
			}
			rec := postAnimeRematch(id)
			if rec.Code != wantStatus {
				t.Fatalf("failure status: %d %s", rec.Code, rec.Body.String())
			}
			var subjectID int
			var updated string
			if err := db.QueryRow(`SELECT anime_meta.subject_id,animes.updated_at FROM anime_meta JOIN animes ON anime_meta.anime_title=animes.title`).Scan(&subjectID, &updated); err != nil || subjectID != 999 || updated != "2026-01-01T00:00:00Z" {
				t.Fatalf("failed rematch partially committed: %d %q %v", subjectID, updated, err)
			}
		})
	}
}

func TestAnimeRematchDiscardsInFlightResults(t *testing.T) {
	for _, kind := range []string{"subject-refresh", "negative-search"} {
		t.Run(kind, func(t *testing.T) {
			testDatabase(t)
			counts := startMikanLookupTest(t, mikanTestScenario{searchHit: kind == "subject-refresh"})
			id := seedRematchAnime(t, "", 0)
			blockedPath := "/search/subject/"
			if kind == "subject-refresh" {
				if _, err := db.Exec(`INSERT INTO anime_meta(anime_title,subject_id,source) VALUES('测试动画',4242,'bangumi')`); err != nil {
					t.Fatal(err)
				}
				blockedPath = "/v0/subjects/4242"
			}
			transport := metaClient.Transport
			started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var once, releaseOnce sync.Once
			metaClient.Transport = mikanTestRoundTripper(func(r *http.Request) (*http.Response, error) {
				if strings.HasPrefix(r.URL.Path, blockedPath) {
					once.Do(func() { close(started); <-release })
				}
				return transport.RoundTrip(r)
			})
			go func() { defer close(done); lookupAndCacheMeta("测试动画") }()
			t.Cleanup(func() {
				releaseOnce.Do(func() { close(release) })
				select {
				case <-done:
				case <-time.After(3 * time.Second):
					t.Error("metadata lookup did not stop")
				}
			})
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("lookup did not start")
			}
			if rec := postAnimeRematch(id); rec.Code != http.StatusAccepted {
				t.Fatalf("rematch blocked by network lookup: %d %s", rec.Code, rec.Body.String())
			}
			releaseOnce.Do(func() { close(release) })
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("late result did not finish")
			}
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM anime_meta WHERE anime_title='测试动画'`).Scan(&count); err != nil || count != 0 {
				t.Fatalf("old result survived rematch: %d %v", count, err)
			}
			lookupAndCacheMeta(<-metaQueue)
			wantSearches := int32(1)
			if kind == "negative-search" {
				wantSearches = 2
			}
			if counts.searches.Load() != wantSearches {
				t.Fatalf("queued rematch was skipped: %d searches", counts.searches.Load())
			}
		})
	}
}

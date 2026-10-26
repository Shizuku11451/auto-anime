package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSubjectEpisodes(t *testing.T) {
	for _, tc := range []struct {
		name        string
		eps, season int
	}{
		{"测试动画", 12, 1}, {"我推的孩子 第二季", 13, 2},
		{"测试动画 II", 12, 2}, {"测试动画", 0, 1}, {"测试动画", 10000, 1},
	} {
		n, s := subjectEpisodes(&bangumiSubject{NameCN: tc.name, Episodes: tc.eps})
		want := tc.eps
		if want > 9999 {
			want = 0
		}
		if s != tc.season || n != want {
			t.Fatalf("%+v: %d %d", tc, n, s)
		}
	}
}

func seedCompletion(t *testing.T) string {
	t.Helper()
	testDatabase(t)
	withMetaTestEnv(t)
	if _, err := ensureAnime("完结测试"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "video.mp4")
	if err := os.WriteFile(path, []byte("media"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`INSERT INTO feeds(id,name,url) VALUES(1,'test','https://example.test/rss')`,
		`INSERT INTO anime_meta(anime_title,subject_id,source,name_cn,rating,summary) VALUES('完结测试',123,'bangumi','完结测试 第二季',8,'cached')`,
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= 3; i++ {
		if _, err := db.Exec(`INSERT INTO items(feed_id,title,link,anime_title,episode,season_auto,status,file_path) VALUES(1,'test',?,'完结测试',?,2,'已保存',?)`, fmt.Sprintf("https://example.test/%d", i), fmt.Sprintf("%02d", i), path); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func assertCompletion(t *testing.T, want bool) {
	t.Helper()
	if err := completeFeeds(); err != nil {
		t.Fatal(err)
	}
	var enabled bool
	var note string
	if err := db.QueryRow(`SELECT enabled,completion_note FROM feeds WHERE id=1`).Scan(&enabled, &note); err != nil {
		t.Fatal(err)
	}
	if enabled == want || (want && !strings.Contains(note, "3 集已全部归档")) {
		t.Fatalf("completed=%v enabled=%v note=%q", want, enabled, note)
	}
}

func TestFeedCompletionBangumiBackfill(t *testing.T) {
	path := seedCompletion(t)
	assertCompletion(t, false)
	scheduleTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v0/subjects/123" {
			t.Errorf("unexpected %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"id":123,"name_cn":"完结测试 第二季","eps":3,"total_episodes":5}`)
	})
	refreshBroadcastWeekday("完结测试")
	assertCompletion(t, true)
	assertCompletion(t, true)
	if _, err := os.Stat(path); err != nil {
		t.Fatal("media changed", err)
	}
	// Restart must preserve the reason, not silently re-enable the feed.
	db.Close()
	initDB()
	assertCompletion(t, true)
}

func TestFeedCompletionRejectsIncompleteOrAmbiguous(t *testing.T) {
	for _, scenario := range []string{"hole", "duplicate", "active", "missing-file", "empty-file", "directory", "wrong-season", "multiple-series", "multiple-seasons", "wrong-subject", "wrong-binding", "unknown-total", "special", "failed-meta"} {
		t.Run(scenario, func(t *testing.T) {
			path := seedCompletion(t)
			s := &bangumiSubject{ID: 123, NameCN: "完结测试 第二季", Episodes: 3, DetailsFetched: true}
			if err := cacheSubjectEpisodes("完结测试", s); err != nil {
				t.Fatal(err)
			}
			var query string
			switch scenario {
			case "hole":
				query = `DELETE FROM items WHERE episode='02'`
			case "duplicate":
				query = `UPDATE items SET episode='01' WHERE episode='02'`
			case "active":
				query = `UPDATE items SET status='下载中' WHERE episode='02'`
			case "missing-file":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "empty-file":
				if err := os.Truncate(path, 0); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if _, err := db.Exec("UPDATE items SET file_path=?", filepath.Dir(path)); err != nil {
					t.Fatal(err)
				}
			case "wrong-season":
				query = `UPDATE anime_meta SET episode_season=1`
			case "multiple-series":
				query = `UPDATE items SET anime_title='其他番剧' WHERE episode='02'`
			case "multiple-seasons":
				query = `UPDATE items SET season=1 WHERE episode='02'`
			case "wrong-subject":
				query = `UPDATE anime_meta SET subject_id=456`
			case "wrong-binding":
				query = `UPDATE animes SET bangumi_id=456`
			case "unknown-total":
				query = `UPDATE anime_meta SET episode_total=0`
			case "special":
				query = `UPDATE items SET episode='SP02' WHERE episode='02'`
			case "failed-meta":
				query = `UPDATE anime_meta SET source='none'`
			}
			if query != "" {
				if _, err := db.Exec(query); err != nil {
					t.Fatal(err)
				}
			}
			assertCompletion(t, false)
		})
	}
}

func TestFeedCompletionChecksArchiveNotSourceAndLateLastEpisode(t *testing.T) {
	seedCompletion(t)
	if err := cacheSubjectEpisodes("完结测试", &bangumiSubject{ID: 123, NameCN: "完结测试 第二季", Episodes: 3, DetailsFetched: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE items SET status='下载中' WHERE episode='03'`); err != nil {
		t.Fatal(err)
	}
	assertCompletion(t, false)
	if _, err := db.Exec(`UPDATE items SET status='已保存' WHERE episode='03'`); err != nil {
		t.Fatal(err)
	}
	assertCompletion(t, true)
}

func TestFeedCompletionSeasonSpecificLookupAndCorrection(t *testing.T) {
	seedCompletion(t)
	if _, err := db.Exec(`UPDATE anime_meta SET episode_total=12,episode_season=1,episode_subject_id=123`); err != nil {
		t.Fatal(err)
	}
	hits := 0
	scheduleTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		if strings.HasPrefix(r.URL.Path, "/search/subject/") {
			fmt.Fprint(w, `{"list":[{"id":456,"name_cn":"完结测试 第二季"}]}`)
		} else if r.URL.Path == "/v0/subjects/456" {
			fmt.Fprint(w, `{"id":456,"name_cn":"完结测试 第二季","eps":3}`)
		} else {
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	})
	refreshFeedEpisodeTotals("完结测试")
	if hits != 2 {
		t.Fatalf("lookup hits %d", hits)
	}
	refreshFeedEpisodeTotals("完结测试")
	if hits != 2 {
		t.Fatal("cache not reused")
	}
	assertCompletion(t, true)
	if n, err := syncFeedContext(context.Background(), 1); err != nil || n != 0 {
		t.Fatalf("completed feed fetched again: %d %v", n, err)
	}
	// A user correction invalidates the season-specific cached match.
	if _, err := db.Exec(`UPDATE feeds SET enabled=1,completion_note=''; UPDATE animes SET search_name='纠正的番名'`); err != nil {
		t.Fatal(err)
	}
	assertCompletion(t, false)
}

func TestFeedCompletionLateSeasonLookupCannotOverrideCorrection(t *testing.T) {
	seedCompletion(t)
	scheduleTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/search/subject/") {
			fmt.Fprint(w, `{"list":[{"id":456,"name_cn":"完结测试 第二季"}]}`)
		} else {
			if _, err := db.Exec(`UPDATE animes SET search_name='用户纠正'`); err != nil {
				t.Error(err)
			}
			fmt.Fprint(w, `{"id":456,"name_cn":"完结测试 第二季","eps":3}`)
		}
	})
	refreshFeedEpisodeTotals("完结测试")
	assertCompletion(t, false)
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM feed_episode_totals").Scan(&count); err != nil || count != 0 {
		t.Fatalf("stale cache %d %v", count, err)
	}
}

func TestFeedCompletionDeletedAnimeDropsSeasonCache(t *testing.T) {
	seedCompletion(t)
	if _, err := db.Exec(`INSERT INTO feed_episode_totals(anime_id,season,lookup_key,subject_id,total,checked_at)
		SELECT id,2,'test',456,3,'' FROM animes WHERE title='完结测试';
		DELETE FROM animes WHERE title='完结测试'`); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM feed_episode_totals").Scan(&count); err != nil || count != 0 {
		t.Fatalf("orphaned season cache %d %v", count, err)
	}
}

func TestFeedCompletionRejectsOldLibraryMatchAfterCorrection(t *testing.T) {
	for _, key := range []string{"", metaLookupKey("完结测试", ""), "mikan-link:" + metaLookupKey("完结测试", "")} {
		t.Run(key, func(t *testing.T) {
			seedCompletion(t)
			if _, err := db.Exec(`UPDATE anime_meta SET episode_total=3,episode_season=2,episode_subject_id=123,match_key=?`, key); err != nil {
				t.Fatal(err)
			}
			rec := putAnime(t, animeIDByTitle(t, "完结测试"), `{"search_name":"用户纠正的另一部作品"}`)
			if rec.Code != http.StatusOK {
				t.Fatal(rec.Code, rec.Body.String())
			}
			assertCompletion(t, false)
			hits := 0
			scheduleTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				hits++
				if strings.HasPrefix(r.URL.Path, "/search/subject/") {
					fmt.Fprint(w, `{"list":[{"id":456,"name_cn":"用户纠正的另一部作品 第二季"}]}`)
				} else if r.URL.Path == "/v0/subjects/456" {
					fmt.Fprint(w, `{"id":456,"name_cn":"用户纠正的另一部作品 第二季","eps":4}`)
				} else {
					t.Error("unexpected request", r.URL.Path)
				}
			})
			refreshFeedEpisodeTotals("完结测试")
			if hits != 2 {
				t.Fatalf("old match suppressed corrected lookup: %d", hits)
			}
			assertCompletion(t, false)
			if _, err := db.Exec(`UPDATE animes SET bangumi_id=123`); err != nil {
				t.Fatal(err)
			}
			assertCompletion(t, true)
		})
	}
}

func TestFeedCompletionRenamePreservesCurrentMatchKey(t *testing.T) {
	for _, prefix := range []string{"", "mikan-link:"} {
		t.Run(prefix, func(t *testing.T) {
			seedCompletion(t)
			if _, err := db.Exec(`UPDATE items SET anime_id=(SELECT id FROM animes WHERE title=items.anime_title)`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`UPDATE anime_meta SET episode_total=3,episode_season=2,episode_subject_id=123,match_key=?`, prefix+metaLookupKey("完结测试", "")); err != nil {
				t.Fatal(err)
			}
			if err := renameAnime("完结测试", "改名测试"); err != nil {
				t.Fatal(err)
			}
			assertCompletion(t, true)
		})
	}
}

func TestFeedCompletionRealArchiveAndExplicitReenable(t *testing.T) {
	testDatabase(t)
	withMetaTestEnv(t)
	server := mediaServer(t)
	if _, err := ensureAnime("归档完结测试"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO feeds(id,name,url) VALUES(1,'test','https://example.test/rss');
		INSERT INTO anime_meta(anime_title,subject_id,source,episode_total,episode_season,episode_subject_id) VALUES('归档完结测试',123,'bangumi',2,1,123)`); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 2; i++ {
		link := fmt.Sprintf("%s/%d.mp4", server.URL, i)
		res, err := db.Exec(`INSERT INTO items(feed_id,title,link,anime_title,episode) VALUES(1,?,?,'归档完结测试',?)`, fmt.Sprintf("归档完结测试 [%02d]", i), link, fmt.Sprint(i))
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		path, err := fetchArchiveContext(context.Background(), int(id), link)
		if err != nil {
			t.Fatal(err)
		}
		if err := commitSavedArchive(int(id), path); err != nil {
			t.Fatal(err)
		}
		pollDownloaderTasks()
		if got := feedEnabled(t, 1); got != map[bool]int{true: 0, false: 1}[i == 2] {
			t.Fatalf("after episode %d enabled=%d", i, got)
		}
	}
	rec := httptest.NewRecorder()
	updateFeed(rec, httptest.NewRequest("PUT", "/api/feeds/1", strings.NewReader(`{"name":"test","enabled":true}`)))
	if rec.Code != 200 || feedEnabled(t, 1) != 1 {
		t.Fatalf("reenable %d %s", rec.Code, rec.Body.String())
	}
	var note string
	if err := db.QueryRow("SELECT completion_note FROM feeds WHERE id=1").Scan(&note); err != nil || note != "" {
		t.Fatalf("stale reason %q %v", note, err)
	}
	if err := completeFeeds(); err != nil {
		t.Fatal(err)
	}
	if feedEnabled(t, 1) != 0 {
		t.Fatal("still complete but not stopped")
	}
}

func TestFeedCompletionLateRSSCannotAddAfterAutoStop(t *testing.T) {
	seedCompletion(t)
	if _, err := db.Exec(`UPDATE items SET status='下载中' WHERE episode='03';
		UPDATE anime_meta SET episode_total=3,episode_season=2,episode_subject_id=123`); err != nil {
		t.Fatal(err)
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := db.Exec(`UPDATE items SET status='已保存' WHERE episode='03'`); err != nil {
			t.Error(err)
		}
		processCompletedFeeds()
		fmt.Fprint(w, `<rss><channel><item><title>完结测试 第二季 [04]</title><link>https://example.test/late.mp4</link></item></channel></rss>`)
	}))
	defer s.Close()
	if _, err := db.Exec("UPDATE feeds SET url=? WHERE id=1", s.URL); err != nil {
		t.Fatal(err)
	}
	if n, err := syncFeedContext(context.Background(), 1); err != nil || n != 0 {
		t.Fatalf("late sync: %d %v", n, err)
	}
	assertCompletion(t, true)
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM items").Scan(&count); err != nil || count != 3 {
		t.Fatalf("late resources added: %d %v", count, err)
	}
}

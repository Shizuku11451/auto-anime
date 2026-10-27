package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRomanAndUnderscoreSeasonPreviewSync(t *testing.T) {
	testDatabase(t)
	titles := []string{
		"[Group] 测试动画 II [01][1080p]",
		"[Group] 测试动画_S02_02_1080p",
		"[Group] 测试动画Ⅱ [03][1080p]",
		"[Group] 测试动画 第一季 / Example III [04][1080p]",
		"[Group] 测试动画 Ｓ０２Ｅ０５ [1080p]",
		"[Group] 测试动画 Season-02 [06][1080p]",
		"[Group] 测试动画 Second Season [07][1080p]",
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<rss><channel>`)
		for i, title := range titles {
			fmt.Fprintf(w, `<item><title>%s</title><link>/item/%d</link></item>`, title, i)
		}
		fmt.Fprint(w, `</channel></rss>`)
	}))
	defer server.Close()
	for _, tc := range []struct {
		season  int
		unknown bool
		count   int
	}{{2, false, 6}, {1, true, 0}, {-1, false, 0}} {
		payload, _ := json.Marshal(PreviewInput{URL: server.URL, SeasonFilter: tc.season, AllowUnknownSeason: tc.unknown})
		rec := httptest.NewRecorder()
		preview(rec, httptest.NewRequest("POST", "/api/preview", strings.NewReader(string(payload))))
		var out struct{ Count int }
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || rec.Code != 200 || out.Count != tc.count {
			t.Fatalf("preview season %d: %d %s err=%v", tc.season, rec.Code, rec.Body.String(), err)
		}
	}
	if _, err := db.Exec(`INSERT INTO feeds(id,name,url,season_filter,allow_unknown_season,target_title) VALUES(1,'第一季',?,1,1,'测试动画')`, server.URL); err != nil {
		t.Fatal(err)
	}
	if n, err := syncFeedContext(context.Background(), 1); err != nil || n != 0 {
		t.Fatalf("second seasons leaked into first: %d %v", n, err)
	}
	if _, err := db.Exec(`UPDATE feeds SET season_filter=2,allow_unknown_season=0 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	for _, count := range []int{6, 0} {
		if n, err := syncFeedContext(context.Background(), 1); err != nil || n != count {
			t.Fatalf("second season sync: %d want %d err=%v", n, count, err)
		}
	}
	var wrong int
	if err := db.QueryRow(`SELECT COUNT(*) FROM items WHERE season_auto<>2`).Scan(&wrong); err != nil || wrong != 0 {
		t.Fatalf("wrong archival seasons: %d err=%v", wrong, err)
	}
}

func TestFeedSharedReadBudget(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if strings.Contains(r.URL.Query().Get("term"), "第一季") {
			w.WriteHeader(503)
			return
		}
		fmt.Fprint(w, `<rss><channel><item><title>测试动画 S1</title><link>/one</link></item></channel></rss>`)
	}))
	defer server.Close()
	pagedTestSource(t, server.URL+"/?term={keyword}", 1)
	budget := feedReadBudget{pages: 2, items: 10, bytes: 1 << 20}
	calls.Store(0)
	for i := 0; i < 2; i++ {
		_, _ = fetchRSSBudget(context.Background(), server.URL+"/?term=S1", &budget, 20)
	}
	if calls.Load() != 2 || budget.pages != 0 {
		t.Fatalf("budget not shared: %d %+v", calls.Load(), budget)
	}
}

func TestSeasonSubscriptionPreviewSyncAndBackup(t *testing.T) {
	testDatabase(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<rss><channel>`)
		for i, title := range []string{"测试动画 [S1 Reseed Fin]", "测试动画 S01 [02]", "测试动画 第三季 [01]", "测试动画 [03]", "测试动画 第一季 / Example S2"} {
			fmt.Fprintf(w, `<item><title>[组] %s</title><link>/item/%d</link></item>`, title, i)
		}
		fmt.Fprint(w, `</channel></rss>`)
	}))
	defer server.Close()
	body, _ := json.Marshal(map[string]any{"name": "第一季", "url": server.URL, "include_words": "测试动画", "season_filter": 1})
	rec := httptest.NewRecorder()
	feeds(rec, httptest.NewRequest("POST", "/api/feeds", strings.NewReader(string(body))))
	var created struct{ ID int }
	json.Unmarshal(rec.Body.Bytes(), &created)
	if rec.Code != 200 || created.ID == 0 {
		t.Fatalf("create: %s", rec.Body.String())
	}
	for _, tc := range []struct {
		season  int
		unknown bool
		count   int
	}{{1, false, 2}, {1, true, 3}, {-1, false, 1}, {0, false, 5}} {
		payload, _ := json.Marshal(PreviewInput{URL: server.URL, Include: "测试动画", SeasonFilter: tc.season, AllowUnknownSeason: tc.unknown})
		p := httptest.NewRecorder()
		preview(p, httptest.NewRequest("POST", "/api/preview", strings.NewReader(string(payload))))
		var out struct{ Count int }
		json.Unmarshal(p.Body.Bytes(), &out)
		if p.Code != 200 || out.Count != tc.count {
			t.Fatalf("preview: %s", p.Body.String())
		}
	}
	for _, count := range []int{2, 0} {
		if n, err := syncFeedContext(context.Background(), created.ID); n != count || err != nil {
			t.Fatalf("sync: %d %v", n, err)
		}
	}
	var wrong int
	db.QueryRow("SELECT COUNT(*) FROM items WHERE season_auto<>1").Scan(&wrong)
	if wrong != 0 {
		t.Fatal("wrong archival season")
	}
	updated := httptest.NewRecorder()
	updateFeed(updated, httptest.NewRequest("PUT", fmt.Sprintf("/api/feeds/%d", created.ID), strings.NewReader(`{"name":"改名"}`)))
	b := httptest.NewRecorder()
	backup(b, httptest.NewRequest("GET", "/api/backup", nil))
	var saved backupData
	json.Unmarshal(b.Body.Bytes(), &saved)
	if len(saved.Feeds) != 1 || saved.Feeds[0].SeasonFilter != 1 {
		t.Fatalf("edit/backup: %s", b.Body.String())
	}
	db.Exec("UPDATE feeds SET season_filter=0")
	r := httptest.NewRecorder()
	restore(r, httptest.NewRequest("POST", "/api/restore", strings.NewReader(b.Body.String())))
	var season int
	db.QueryRow("SELECT season_filter FROM feeds").Scan(&season)
	if r.Code != 200 || season != 1 {
		t.Fatalf("restore: %s", r.Body.String())
	}
	invalid := httptest.NewRecorder()
	updateFeed(invalid, httptest.NewRequest("PUT", fmt.Sprintf("/api/feeds/%d", created.ID), strings.NewReader(`{"name":"不能保存","season_filter":-2}`)))
	if invalid.Code != 400 {
		t.Fatal("invalid season accepted")
	}
	// Validate every row before restoring, so a later invalid row cannot partly
	// overwrite an existing subscription.
	saved.Feeds[0].Name = "不能部分恢复"
	saved.Feeds = append(saved.Feeds, Feed{Name: "无效季数", URL: server.URL + "/invalid", SeasonFilter: -2})
	invalidBackup, _ := json.Marshal(saved)
	invalid = httptest.NewRecorder()
	restore(invalid, httptest.NewRequest("POST", "/api/restore", strings.NewReader(string(invalidBackup))))
	var name string
	db.QueryRow("SELECT name FROM feeds WHERE id=?", created.ID).Scan(&name)
	if invalid.Code != 400 || name != "改名" {
		t.Fatalf("partial restore: %d %s", invalid.Code, name)
	}
	legacy := fmt.Sprintf(`{"feeds":[{"name":"旧备份","url":%q,"enabled":true}]}`, server.URL)
	r = httptest.NewRecorder()
	restore(r, httptest.NewRequest("POST", "/api/restore", strings.NewReader(legacy)))
	db.QueryRow("SELECT season_filter FROM feeds WHERE id=?", created.ID).Scan(&season)
	if r.Code != 200 || season != 0 {
		t.Fatalf("legacy restore changed default: %s", r.Body.String())
	}
}

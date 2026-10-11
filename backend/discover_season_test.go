package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDiscoverSeasonIntent(t *testing.T) {
	for _, tc := range []struct {
		input, base string
		season      int
	}{
		{"无职转生 第一季", "无职转生", 1}, {"无职转生第一季", "无职转生", 1},
		{"Example S01", "Example", 1}, {"Example Season 2", "Example", 2},
		{"Example 3rd Season", "Example", 3}, {"86", "86", 0},
		{"Example Part 2", "Example Part 2", 0}, {"Example S01E02", "Example S01E02", 0},
		{"第一季", "第一季", 0}, {"三月的狮子", "三月的狮子", 0},
	} {
		base, season := discoverIntent(tc.input)
		if base != tc.base || season != tc.season {
			t.Fatalf("%q: %q %d", tc.input, base, season)
		}
	}
}

func TestResourceSeasonEvidence(t *testing.T) {
	for _, tc := range []struct {
		title  string
		season int
	}{
		{"[组] 测试动画 [S1 Reseed Fin]", 1}, {"[组] 测试动画 [S01] [01]", 1},
		{"测试动画 第二季 / Example S2 Part 2", 2}, {"测试动画 第一季 / Example S2", -1},
		{"测试动画 Part 2 - 01", 0}, {"测试动画 [2021][01-24合集]", 0},
		{"SPY X FAMILY [01]", 0}, {"测试动画 II", 2},
		{"[VCB-Studio] 无职转生 II [01][1080p]", 2},
		{"[Group] Example II - 01 [1080p]", 2},
		{"[Group] Example ii - 01 [1080p]", 2},
		{"[Group] Example_S02_01_1080p", 2},
		{"[Group] Example_S02E01_1080p", 2},
		{"[Group] 无职转生Ⅱ [01][1080p]", 2},
		{"[Group] Example XII [01][1080p]", 0},
		{"[Group] 测试动画Ⅻ [01][1080p]", 12},
		{"[Group] 测试动画 第二季 / Example III [01][1080p]", -1},
		{"[Group] Example_S02_S03E01_1080p", -1},
		{"[Group] SPY X FAMILY [01][1080p]", 0},
		{"[Group] That Time I Got Reincarnated [01][1080p]", 0},
		{"四季青春 [01]", 0}, {"四季青春 第三季 [01]", 3},
		{"Example S01E02", 1}, {"Example S01E02 / Example S02E02", -1},
	} {
		if got := resourceSeason(tc.title); got != tc.season {
			t.Fatalf("%s: %d want %d", tc.title, got, tc.season)
		}
		if allowsSeason(tc.title, 1, false) != (tc.season == 1) {
			t.Fatal("strict filter mismatch")
		}
		if allowsSeason(tc.title, 1, true) != (tc.season == 1 || tc.season == 0) {
			t.Fatal("unknown opt-in mismatch")
		}
		if tc.season != 0 {
			want := max(tc.season, 0)
			if got := extractAnimeInfo(tc.title).Season; got != want {
				t.Fatalf("filter/archive season mismatch for %q: %d want %d", tc.title, got, want)
			}
		}
	}
	info := extractAnimeInfo("[组] 测试动画 [S1 Reseed Fin] [1080p]")
	if info.Season != 1 || info.Title != "测试动画" {
		t.Fatalf("mixed bracket: %+v", info)
	}
	for _, depth := range []string{"10-bit", "10bit", "10 bit", "8-bit"} {
		info = extractAnimeInfo("[组] 测试动画 / Example " + depth + " 1080p HEVC BDRip [S1 Reseed Fin]")
		if info.Season != 1 || info.Title != "测试动画" || info.Episode != "" {
			t.Fatalf("bit depth became episode: %+v", info)
		}
		info = extractAnimeInfo("[组] 测试动画 - 02 " + depth + " 1080p [S1]")
		if info.Episode != "02" {
			t.Fatalf("real episode lost: %+v", info)
		}
	}
	info = extractAnimeInfo("[7ACG] 测试动画 / Example S01 | 01-24 [简繁字幕] BDrip 1080p AV1 OPUS 2.0(v2)")
	if info.Season != 1 || info.Episode != "" {
		t.Fatalf("batch audio channels became episode: %+v", info)
	}
}

func TestDiscoverSeasonExpansionAndProvenance(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		fmt.Fprint(w, `<rss><channel>`)
		switch r.URL.Query().Get("term") {
		case "测试动画 S1":
			fmt.Fprint(w, `<item><title>[组] 测试动画 [S1 Reseed Fin]</title><link>/one</link></item>`)
		case "测试动画 S01":
			fmt.Fprint(w, `<item><title>[组] 测试动画 S01 [02]</title><link>/two</link></item>`)
		case "测试动画":
			fmt.Fprint(w, `<item><title>[组] 测试动画 [S1 Reseed Fin]</title><link>/one</link></item><item><title>[组] 测试动画 S3 [01]</title><link>/three</link></item><item><title>[组] 测试动画 [01]</title><link>/unknown</link></item>`)
		}
		fmt.Fprint(w, `</channel></rss>`)
	}))
	defer server.Close()
	overrideDiscoverSource(t, server.URL+"/?term={keyword}")
	rec := discoverGet(t, "/api/discover/search?source=acgrip&keyword="+url.QueryEscape("测试动画 第一季"))
	var out struct {
		Items           []discoverItem
		RequestedSeason int `json:"requested_season"`
		Queries         []string
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || rec.Code != 200 || len(out.Items) != 4 || out.RequestedSeason != 1 || requests.Load() != 4 {
		t.Fatalf("response: %s %v", rec.Body.String(), err)
	}
	for _, item := range out.Items {
		u, _ := url.Parse(item.FeedURL)
		if u.Query().Get("term") != item.Query {
			t.Fatalf("lost provenance: %+v", item)
		}
	}
	requests.Store(0)
	rec = discoverGet(t, "/api/discover/search?source=acgrip&literal=1&keyword="+url.QueryEscape("测试动画 第一季"))
	if rec.Code != 200 || requests.Load() != 1 {
		t.Fatal("literal search expanded")
	}
}

func TestRomanAndUnderscoreSeasonPreviewSync(t *testing.T) {
	testDatabase(t)
	titles := []string{
		"[Group] 测试动画 II [01][1080p]",
		"[Group] 测试动画_S02_02_1080p",
		"[Group] 测试动画Ⅱ [03][1080p]",
		"[Group] 测试动画 第一季 / Example III [04][1080p]",
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
	}{{2, false, 3}, {1, true, 0}, {-1, false, 0}} {
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
	if n, err := syncFeed(1); err != nil || n != 0 {
		t.Fatalf("second seasons leaked into first: %d %v", n, err)
	}
	if _, err := db.Exec(`UPDATE feeds SET season_filter=2,allow_unknown_season=0 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	for _, count := range []int{3, 0} {
		if n, err := syncFeed(1); err != nil || n != count {
			t.Fatalf("second season sync: %d want %d err=%v", n, count, err)
		}
	}
	var wrong int
	if err := db.QueryRow(`SELECT COUNT(*) FROM items WHERE season_auto<>2`).Scan(&wrong); err != nil || wrong != 0 {
		t.Fatalf("wrong archival seasons: %d err=%v", wrong, err)
	}
}

func TestDiscoverPartialFailureAndSharedBudget(t *testing.T) {
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
	rec := discoverGet(t, "/api/discover/search?source=acgrip&keyword="+url.QueryEscape("测试动画 第一季"))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "503") || !strings.Contains(rec.Body.String(), `"count":1`) {
		t.Fatalf("partial: %s", rec.Body.String())
	}
	budget := feedReadBudget{pages: 2, items: 10, bytes: 1 << 20}
	calls.Store(0)
	for i := 0; i < 2; i++ {
		_, _ = fetchRSSBudget(context.Background(), server.URL+"/?term=S1", &budget, 20)
	}
	if calls.Load() != 2 || budget.pages != 0 {
		t.Fatalf("budget not shared: %d %+v", calls.Load(), budget)
	}
}

func TestDiscoverExpansionCancellation(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	overrideDiscoverSource(t, server.URL+"/?term={keyword}")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest("GET", "/api/discover/search?source=acgrip&keyword="+url.QueryEscape("测试动画 第一季"), nil).WithContext(ctx)
	done := make(chan struct{})
	go func() {
		discoverSearchAPI(httptest.NewRecorder(), req)
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("request not started")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not stop search")
	}
	if calls.Load() != 1 {
		t.Fatal("cancelled search started expansion")
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
		if n, err := syncFeed(created.ID); n != count || err != nil {
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

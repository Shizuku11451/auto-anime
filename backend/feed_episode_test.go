package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInferEpisodeOffset(t *testing.T) {
	for _, tc := range []struct {
		titles []string
		want   int
		known  bool
	}{
		{[]string{"无职转生 第二季 [02]", "无职转生 第二季 [00]", "无职转生 第二季 [01]"}, 1, true},
		{[]string{"我推的孩子 第二季 [14]", "我推的孩子 第二季 [12]", "我推的孩子 第二季 [13]"}, -11, true},
		{[]string{"测试动画 [01]", "测试动画 [03]"}, 0, true},
		{[]string{"测试动画 [01-12合集]", "测试动画 SP"}, 0, false},
		{[]string{"测试动画 第一季 [01]", "测试动画 第二季 [12]"}, 0, false},
		{[]string{"作品甲 [01]", "作品乙 [12]"}, 0, false},
	} {
		var items []feedItem
		for _, title := range tc.titles {
			items = append(items, feedItem{Title: title})
		}
		got := inferEpisodeOffset(items)
		if (got != nil) != tc.known || (got != nil && *got != tc.want) {
			t.Errorf("%v: %v", tc.titles, got)
		}
	}
	for _, tc := range []struct {
		raw    string
		offset int
		want   string
	}{{"00", 1, "1"}, {"12", -11, "1"}, {"13", -11, "2"}, {"07", 0, "07"}, {"SP01", 1, "SP01"}, {"", 1, ""}, {"01", -11, "01"}} {
		if got := offsetEpisode(tc.raw, tc.offset); got != tc.want {
			t.Errorf("%+v: %q", tc, got)
		}
	}
}

func TestFeedOffsetPreviewSyncAndBackup(t *testing.T) {
	testDatabase(t)
	withMetaTestEnv(t)
	first := 12
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<rss><channel>`)
		for _, n := range []int{first + 1, first} {
			fmt.Fprintf(w, `<item><title>我推的孩子 第二季 [%02d] 1080p</title><link>https://example.test/%d.mp4</link></item>`, n, n)
		}
		fmt.Fprint(w, `<item><title>我推的孩子 第二季 [00] 480p</title><link>https://example.test/excluded</link></item></channel></rss>`)
	}))
	defer s.Close()
	payload, _ := json.Marshal(map[string]any{"url": s.URL, "include_words": "1080p"})
	rec := httptest.NewRecorder()
	preview(rec, httptest.NewRequest("POST", "/api/preview", bytes.NewReader(payload)))
	var result struct {
		Offset *int `json:"episode_offset"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || result.Offset == nil || *result.Offset != -11 {
		t.Fatalf("preview: %s %v", rec.Body.String(), err)
	}
	payload, _ = json.Marshal(map[string]any{"url": s.URL, "name": "推子", "include_words": "1080p"})
	rec = httptest.NewRecorder()
	feeds(rec, httptest.NewRequest("POST", "/api/feeds", bytes.NewReader(payload)))
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if _, err := syncFeedContext(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	var offset int
	var episode string
	if err := db.QueryRow("SELECT episode_offset FROM feeds WHERE id=1").Scan(&offset); err != nil || offset != -11 {
		t.Fatalf("offset: %d %v", offset, err)
	}
	if err := db.QueryRow("SELECT episode FROM items WHERE link='https://example.test/12.mp4'").Scan(&episode); err != nil || episode != "1" {
		t.Fatalf("episode: %q %v", episode, err)
	}
	first = 14
	if _, err := syncFeedContext(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT episode FROM items WHERE link='https://example.test/14.mp4'").Scan(&episode); err != nil || episode != "3" {
		t.Fatalf("rolling baseline: %q %v", episode, err)
	}
	rec = httptest.NewRecorder()
	backup(rec, httptest.NewRequest("GET", "/api/backup", nil))
	var data backupData
	if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil || data.Version != 4 || *data.Feeds[0].EpisodeOffset != -11 {
		t.Fatalf("backup: %s %v", rec.Body.String(), err)
	}
	if _, err := db.Exec("UPDATE feeds SET episode_offset=0"); err != nil {
		t.Fatal(err)
	}
	request, _ := json.Marshal(data)
	rec = httptest.NewRecorder()
	restore(rec, httptest.NewRequest("POST", "/api/restore", bytes.NewReader(request)))
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if err := db.QueryRow("SELECT episode_offset FROM feeds WHERE id=1").Scan(&offset); err != nil || offset != -11 {
		t.Fatalf("restore: %d %v", offset, err)
	}
}

func TestFeedOffsetLegacyAndValidation(t *testing.T) {
	testDatabase(t)
	for _, path := range []string{"/api/feeds", "/api/feeds/1"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", path, strings.NewReader(`{"name":"test","url":"https://example.test/rss","episode_offset":10000}`))
		if path == "/api/feeds" {
			feeds(rec, req)
		} else {
			req.Method = "PUT"
			updateFeed(rec, req)
		}
		if rec.Code != 400 {
			t.Fatal(path, rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	restore(rec, httptest.NewRequest("POST", "/api/restore", strings.NewReader(`{"version":3,"feeds":[{"name":"legacy","url":"https://example.test/rss"}]}`)))
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	var offset *int
	if err := db.QueryRow("SELECT episode_offset FROM feeds").Scan(&offset); err != nil || offset == nil || *offset != 0 {
		t.Fatalf("legacy offset: %v %v", offset, err)
	}
}

func TestFeedOffsetWaitsForCompleteNonemptyFeed(t *testing.T) {
	testDatabase(t)
	withMetaTestEnv(t)
	mode := "incomplete"
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/next" {
			http.Error(w, "unavailable", 503)
			return
		}
		fmt.Fprint(w, `<rss xmlns:atom="http://www.w3.org/2005/Atom"><channel>`)
		if mode != "empty" {
			fmt.Fprint(w, `<item><title>测试动画 [00]</title><link>https://example.test/0.mp4</link></item>`)
		}
		if mode == "incomplete" {
			fmt.Fprint(w, `<atom:link rel="next" href="/next"/>`)
		}
		fmt.Fprint(w, `</channel></rss>`)
	}))
	defer s.Close()
	if _, err := db.Exec("INSERT INTO feeds(name,url,episode_offset) VALUES('test',?,NULL)", s.URL); err != nil {
		t.Fatal(err)
	}
	if n, err := syncFeedContext(context.Background(), 1); err == nil || n != 0 {
		t.Fatalf("incomplete feed: %d %v", n, err)
	}
	mode = "empty"
	if n, err := syncFeedContext(context.Background(), 1); err != nil || n != 0 {
		t.Fatalf("empty feed: %d %v", n, err)
	}
	var offset *int
	if err := db.QueryRow("SELECT episode_offset FROM feeds").Scan(&offset); err != nil || offset != nil {
		t.Fatalf("premature baseline: %v %v", offset, err)
	}
	mode = "complete"
	if n, err := syncFeedContext(context.Background(), 1); err != nil || n != 1 {
		t.Fatalf("complete feed: %d %v", n, err)
	}
	if err := db.QueryRow("SELECT episode_offset FROM feeds").Scan(&offset); err != nil || offset == nil || *offset != 1 {
		t.Fatalf("baseline: %v %v", offset, err)
	}
}

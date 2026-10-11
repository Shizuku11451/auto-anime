package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestTraditionalReleasePreviewAndSyncAtReadLimit(t *testing.T) {
	testDatabase(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("term") != "无职转生" {
			t.Errorf("search term changed: %s", r.URL.RawQuery)
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		page = max(1, page)
		fmt.Fprint(w, `<rss><channel>`)
		for i := 0; i < 30; i++ {
			if page == 1 && i < 16 {
				fmt.Fprintf(w, `<item><title>[綠茶字幕組] 無職轉生 第三季 ～到了異世界就拿出真本事～ / Mushoku Tensei S3 [%02d][WebRip][1080p][繁日內嵌]</title><link>/selected/%d</link></item>`, i%14+1, i)
			} else {
				fmt.Fprintf(w, `<item><title>[其他组] 无职转生 [01][720p][简中]</title><link>/other/%d/%d</link></item>`, page, i)
			}
		}
		fmt.Fprint(w, `</channel></rss>`)
	}))
	defer server.Close()
	pagedTestSource(t, server.URL+"/search.xml?term={keyword}", 30)
	feedURL := discoverSearchURL(discoverBuiltinSources[0].Template, "无职转生")
	// The generated rule keeps the simplified search term and traditional name.
	words := "无职转生,無職轉生,～到了異世界就拿出真本事～,[綠茶字幕組],1080P,繁日"
	for _, tc := range []struct {
		words string
		count int
	}{
		{"无职转生,綠茶字幕組,1080P,繁日", 16},
		{words, 16},
		{words + ",不存在的关键词", 0},
	} {
		body, _ := json.Marshal(PreviewInput{URL: feedURL, Include: tc.words})
		rec := httptest.NewRecorder()
		preview(rec, httptest.NewRequest("POST", "/api/preview", strings.NewReader(string(body))))
		var result struct {
			Count, Total, Pages int
			Limited             bool
			Warning             string
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || rec.Code != 200 || result.Count != tc.count || result.Pages != 20 || result.Total != 600 || !result.Limited || !strings.Contains(result.Warning, "读取上限") {
			t.Fatalf("preview at ceiling: %d %s err=%v", rec.Code, rec.Body.String(), err)
		}
	}
	// Existing subscriptions also benefit without rewriting their saved rules.
	storedWords := "无职转生,綠茶字幕組,1080P,繁日"
	if _, err := db.Exec(`INSERT INTO feeds(id,name,url,include_words,target_title) VALUES(1,'繁体版本',?,?,'無職轉生')`, feedURL, storedWords); err != nil {
		t.Fatal(err)
	}
	for _, want := range []int{16, 0} {
		if added, err := syncFeedContext(context.Background(), 1); added != want || err == nil || !strings.Contains(err.Error(), "读取上限") {
			t.Fatalf("sync must retain matching items and report incomplete history: added=%d want=%d err=%v", added, want, err)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM items WHERE anime_title='無職轉生'`).Scan(&count); err != nil || count != 16 {
		t.Fatalf("stored selected releases=%d err=%v", count, err)
	}
	var gotWords, rawTitle string
	if err := db.QueryRow(`SELECT include_words FROM feeds WHERE id=1`).Scan(&gotWords); err != nil || gotWords != storedWords {
		t.Fatalf("saved rules changed: %q err=%v", gotWords, err)
	}
	if err := db.QueryRow(`SELECT title FROM items WHERE link=?`, server.URL+"/selected/0").Scan(&rawTitle); err != nil || !strings.Contains(rawTitle, "無職轉生") || !strings.Contains(rawTitle, "繁日內嵌") {
		t.Fatalf("raw title changed: %q err=%v", rawTitle, err)
	}
}

func TestSubscriptionPreviewAppliesFiltersBeforeLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<rss><channel>`)
		for i := 0; i < 25; i++ {
			fmt.Fprintf(w, `<item><title>[其他组] 夏日动画 [%02d] [720p]</title><link>https://example.test/other/%d</link></item>`, i, i)
		}
		fmt.Fprint(w, `<item><title>[目标组] 夏日动画 [07] [1080p]</title><link>https://example.test/7</link></item></channel></rss>`)
	}))
	defer server.Close()
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{"url": server.URL, "include_words": "夏日动画,目标组,1080p", "exclude_words": "合集"})
	preview(rec, httptest.NewRequest("POST", "/api/preview", strings.NewReader(string(body))))
	var result struct {
		Count int                      `json:"count"`
		Total int                      `json:"total"`
		Items []struct{ Title string } `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || result.Count != 1 || result.Total != 26 || len(result.Items) != 1 || !strings.Contains(result.Items[0].Title, "目标组") {
		t.Fatalf("preview must show only subscribed releases: %s", rec.Body.String())
	}
}

func TestTargetedSubscriptionKeepsOneSeries(t *testing.T) {
	testDatabase(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<rss><channel>
<item><title>[目标组] 夏日动画 / Summer [07] [1080p]</title><link>https://example.test/7</link></item>
<item><title>[目标组] 夏日动画 / Natsu [08] [1080p]</title><link>https://example.test/8</link></item>
<item><title>[其他组] 夏日动画 [07] [1080p]</title><link>https://example.test/other</link></item>
<item><title>[目标组] 夏日动画 [07] [720p]</title><link>https://example.test/720</link></item>
</channel></rss>`)
	}))
	defer server.Close()
	body, _ := json.Marshal(map[string]any{"name": "夏日动画 · 目标组", "url": server.URL, "include_words": "夏日动画,目标组,1080p", "target_title": "夏日动画"})
	rec := httptest.NewRecorder()
	feeds(rec, httptest.NewRequest("POST", "/api/feeds", strings.NewReader(string(body))))
	var created struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.ID == 0 {
		t.Fatalf("create: %s", rec.Body.String())
	}
	if n, err := syncFeedContext(context.Background(), created.ID); err != nil || n != 2 {
		t.Fatalf("sync: %d %v", n, err)
	}
	var series, items int
	db.QueryRow("SELECT COUNT(*) FROM animes").Scan(&series)
	db.QueryRow("SELECT COUNT(*) FROM items WHERE anime_title='夏日动画'").Scan(&items)
	if series != 1 || items != 2 {
		t.Fatalf("same subscription split into %d series, canonical items=%d", series, items)
	}
	if n, err := syncFeedContext(context.Background(), created.ID); err != nil || n != 0 {
		t.Fatalf("repeat sync: %d %v", n, err)
	}
}

func TestSubscriptionTargetRoundTripAndEdit(t *testing.T) {
	testDatabase(t)
	_, err := db.Exec(`INSERT INTO feeds(id,name,url,target_title) VALUES(1,'订阅','https://example.test/rss','夏日动画')`)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	backup(rec, httptest.NewRequest("GET", "/api/backup", nil))
	var saved backupData
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil || len(saved.Feeds) != 1 || saved.Feeds[0].TargetTitle != "夏日动画" {
		t.Fatalf("backup lost target: %s", rec.Body.String())
	}
	if _, err := db.Exec("UPDATE feeds SET target_title='' WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	restored := httptest.NewRecorder()
	restore(restored, httptest.NewRequest("POST", "/api/restore", strings.NewReader(rec.Body.String())))
	if restored.Code != 200 {
		t.Fatalf("restore: %s", restored.Body.String())
	}
	check := func(want string) {
		t.Helper()
		var got string
		if err := db.QueryRow("SELECT target_title FROM feeds WHERE id=1").Scan(&got); err != nil || got != want {
			t.Fatalf("target=%q want=%q err=%v", got, want, err)
		}
	}
	check("夏日动画")
	for _, tc := range []struct{ body, want string }{
		{`{"name":"订阅"}`, "夏日动画"},
		{`{"name":"订阅","target_title":"秋日动画"}`, "秋日动画"},
		{`{"name":"订阅","target_title":""}`, ""},
	} {
		updated := httptest.NewRecorder()
		updateFeed(updated, httptest.NewRequest("PUT", "/api/feeds/1", strings.NewReader(tc.body)))
		if updated.Code != 200 {
			t.Fatalf("update: %s", updated.Body.String())
		}
		check(tc.want)
	}
}

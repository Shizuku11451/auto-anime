package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
	if n, err := syncFeed(created.ID); err != nil || n != 2 {
		t.Fatalf("sync: %d %v", n, err)
	}
	var series, items int
	db.QueryRow("SELECT COUNT(*) FROM animes").Scan(&series)
	db.QueryRow("SELECT COUNT(*) FROM items WHERE anime_title='夏日动画'").Scan(&items)
	if series != 1 || items != 2 {
		t.Fatalf("same subscription split into %d series, canonical items=%d", series, items)
	}
	if n, err := syncFeed(created.ID); err != nil || n != 0 {
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

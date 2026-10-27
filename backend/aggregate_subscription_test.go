package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"auto-anime/internal/aggregate"
	keywordfilter "auto-anime/internal/filter"
)

func testAggregateAddress() string {
	return "aggregate://subject/1?" + url.Values{"title": {"夏日动画"}, "group": {"目标组"}, "quality": {"1080P"}, "season": {"2"}, "language": {""}}.Encode()
}

func TestAggregateSubscriptionValidation(t *testing.T) {
	address := testAggregateAddress()
	if !validFeedURL(address) {
		t.Fatal("valid selection rejected")
	}
	for _, raw := range []string{
		"aggregate://evil/1", strings.Replace(address, "/1?", "/0?", 1), address + "&title=other",
		address + "&url=http://internal", strings.Replace(address, "season=2", "season=-1", 1),
		strings.Replace(address, "subject/", "user:secret@subject/", 1), address + "#fragment",
	} {
		if validFeedURL(raw) {
			t.Fatalf("invalid selection accepted: %s", raw)
		}
	}
	if !keywordfilter.Equivalent("字幕組１０８０Ｐ", "字幕组1080p") || keywordfilter.Equivalent("A,B", "B,A") {
		t.Fatal("release field comparison must be literal and normalized")
	}
}

func TestAggregateSubscriptionPreviewSyncBackup(t *testing.T) {
	testDatabase(t)
	result := aggregate.Result{SubjectID: 1}
	add := func(title string, hash int) {
		result.Items = append(result.Items, aggregate.Resource{Title: title, Magnet: fmt.Sprintf("magnet:?xt=urn:btih:%040x", hash), DetailsURL: "https://example.test/release"})
	}
	add("[目标组] 夏日动画 S2 [07][1080p]", 1)
	add("[其他组] 夏日动画 S2 [07][1080p]", 2)
	add("[目标组] 夏日动画 S2 [07][720p]", 3)
	add("[目标组] 夏日动画 S1 [07][1080p]", 4)
	add("[目标组] 夏日动画 [07][1080p]", 5)
	add("[目标组] 夏日动画剧场版 S2 [07][1080p]", 6)
	add("[目标组] 夏日动画 S2 [07][1080p][简日]", 8)
	status := 200
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/search" || r.URL.Query().Get("subject_id") != "1" || r.Header.Get("Authorization") != "Bearer test-aggregate-subscription-token-123456" {
			t.Error("invalid or unauthenticated upstream request")
		}
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(result)
	}))
	defer upstream.Close()
	oldURL, oldToken := aggregateSearchURL, aggregateSearchToken
	aggregateSearchURL, aggregateSearchToken = upstream.URL, "test-aggregate-subscription-token-123456"
	t.Cleanup(func() { aggregateSearchURL, aggregateSearchToken = oldURL, oldToken })
	address := testAggregateAddress()
	body, _ := json.Marshal(PreviewInput{URL: address})
	w := httptest.NewRecorder()
	preview(w, httptest.NewRequest("POST", "/api/preview", strings.NewReader(string(body))))
	var previewResult struct{ Count int }
	if json.Unmarshal(w.Body.Bytes(), &previewResult) != nil || w.Code != 200 || previewResult.Count != 1 {
		t.Fatalf("preview: %d %s", w.Code, w.Body.String())
	}
	_, rules, _ := aggregateSubscription(address)
	rules.Set("language", "简日")
	languageResult, err := fetchSubscription(context.Background(), "aggregate://subject/1?"+rules.Encode())
	if err != nil || len(languageResult.Entries) != 1 || !strings.Contains(languageResult.Entries[0].Title, "简日") {
		t.Fatalf("wrong language selected: %+v %v", languageResult, err)
	}
	rules.Set("language", "")
	rules.Set("group", "")
	unknownGroup, err := fetchSubscription(context.Background(), "aggregate://subject/1?"+rules.Encode())
	if err != nil || len(unknownGroup.Entries) != 0 {
		t.Fatalf("unknown group must not match every group: %+v %v", unknownGroup, err)
	}
	body, _ = json.Marshal(map[string]any{"name": "版本订阅", "url": address, "auto_download": false, "target_title": "夏日动画 第二季"})
	w = httptest.NewRecorder()
	feeds(w, httptest.NewRequest("POST", "/api/feeds", strings.NewReader(string(body))))
	var created struct{ ID int }
	if json.Unmarshal(w.Body.Bytes(), &created) != nil || created.ID == 0 {
		t.Fatalf("create: %s", w.Body.String())
	}
	for _, want := range []int{1, 0} {
		if n, err := syncFeedContext(context.Background(), created.ID); err != nil || n != want {
			t.Fatalf("sync=%d want=%d err=%v", n, want, err)
		}
	}
	add("[目标组] 夏日动画 S2 [08][1080p]", 7)
	if n, err := syncFeedContext(context.Background(), created.ID); err != nil || n != 1 {
		t.Fatalf("new episode not followed: %d %v", n, err)
	}
	var episode, title, itemStatus string
	if err := db.QueryRow("SELECT episode,anime_title,status FROM items WHERE link=?", result.Items[0].Magnet).Scan(&episode, &title, &itemStatus); err != nil || episode != "07" || title != "夏日动画 第二季" || itemStatus != "待下载" {
		t.Fatalf("wrong ownership/numbering/status: %q %q %q %v", episode, title, itemStatus, err)
	}
	w = httptest.NewRecorder()
	feeds(w, httptest.NewRequest("POST", "/api/feeds", strings.NewReader(string(body))))
	if w.Code != 409 {
		t.Fatal("duplicate subscription accepted")
	}
	status = 502
	if _, err := syncFeedContext(context.Background(), created.ID); err == nil {
		t.Fatal("upstream failure reported as empty success")
	}
	status = 200
	result.Warnings = []string{"mikan 达到单次读取上限，结果可能不完整", "已达到 300 条结果上限"}
	fetched, err := fetchSubscription(context.Background(), address)
	if err != nil || fetched.Incomplete || !fetched.Limited || fetched.Warning == "" {
		t.Fatalf("result cap must remain a warning: %+v %v", fetched, err)
	}
	w = httptest.NewRecorder()
	preview(w, httptest.NewRequest("POST", "/api/preview", strings.NewReader(string(body))))
	var limitedPreview struct {
		Limited       bool   `json:"limited"`
		Warning       string `json:"warning"`
		EpisodeOffset *int   `json:"episode_offset"`
	}
	if json.Unmarshal(w.Body.Bytes(), &limitedPreview) != nil || w.Code != 200 || !limitedPreview.Limited || limitedPreview.Warning == "" || limitedPreview.EpisodeOffset == nil || *limitedPreview.EpisodeOffset != 0 {
		t.Fatalf("preview lost cap warning or original numbering: %d %s", w.Code, w.Body.String())
	}
	add("[目标组] 夏日动画 S2 [09][1080p]", 9)
	for _, want := range []int{1, 0} {
		if n, err := syncFeedContext(context.Background(), created.ID); err != nil || n != want {
			t.Fatalf("capped sync=%d want=%d err=%v", n, want, err)
		}
	}
	var lastError, lastSync string
	if err := db.QueryRow("SELECT last_error,last_sync FROM feeds WHERE id=?", created.ID).Scan(&lastError, &lastSync); err != nil || lastError != "" || lastSync == "" {
		t.Fatalf("capped success kept stale error: %q %q %v", lastError, lastSync, err)
	}
	result.Warnings = []string{"部分来源失败"}
	fetched, err = fetchSubscription(context.Background(), address)
	if err != nil || !fetched.Incomplete || fetched.Warning == "" {
		t.Fatalf("partial result lost warning: %+v %v", fetched, err)
	}
	if _, err := syncFeedContext(context.Background(), created.ID); err == nil {
		t.Fatal("partial source failure must still fail sync")
	}
	w = httptest.NewRecorder()
	backup(w, httptest.NewRequest("GET", "/api/backup", nil))
	if w.Code != 200 || strings.Contains(w.Body.String(), aggregateSearchToken) {
		t.Fatalf("unsafe backup: %d", w.Code)
	}
	restored := httptest.NewRecorder()
	restore(restored, httptest.NewRequest("POST", "/api/restore", strings.NewReader(w.Body.String())))
	if restored.Code != 200 {
		t.Fatalf("restore: %s", restored.Body.String())
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM feeds").Scan(&count); err != nil || count != 1 {
		t.Fatalf("restore duplicated selection: %d %v", count, err)
	}
}

func TestAggregateSubscriptionWarningClassification(t *testing.T) {
	result := aggregate.Result{SubjectID: 1}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(result)
	}))
	defer upstream.Close()
	oldURL, oldToken := aggregateSearchURL, aggregateSearchToken
	aggregateSearchURL, aggregateSearchToken = upstream.URL, "test-aggregate-subscription-token-123456"
	t.Cleanup(func() { aggregateSearchURL, aggregateSearchToken = oldURL, oldToken })
	for _, tc := range []struct {
		name       string
		warnings   []string
		incomplete bool
	}{
		{"complete", nil, false},
		{"source cap", []string{"mikan 达到单次读取上限，结果可能不完整"}, false},
		{"merged cap", []string{"已达到 300 条结果上限"}, false},
		{"mixed", []string{"mikan 达到单次读取上限，结果可能不完整", "dmhy 部分查询失败，结果可能不完整"}, true},
		{"unknown", []string{"新类型的读取警告"}, true},
		{"cap with failure", []string{"mikan 达到单次读取上限，结果可能不完整；请求失败"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result.Warnings = tc.warnings
			fetched, err := fetchSubscription(context.Background(), testAggregateAddress())
			if err != nil || fetched.Incomplete != tc.incomplete || fetched.Limited != (len(tc.warnings) > 0) || fetched.Warning != strings.Join(tc.warnings, "；") {
				t.Fatalf("unexpected warning classification: %+v %v", fetched, err)
			}
		})
	}
}

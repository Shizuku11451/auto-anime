package main

import (
	keywordfilter "auto-anime/internal/filter"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func noFeedPageDelay(t *testing.T) {
	t.Helper()
	previous := feedPageDelay
	feedPageDelay = 0
	t.Cleanup(func() { feedPageDelay = previous })
}

func pagedTestSource(t *testing.T, template string, pageSize int) {
	t.Helper()
	overrideDiscoverSource(t, template)
	discoverBuiltinSources[0].PageParam = "page"
	discoverBuiltinSources[0].PageSize = pageSize
	noFeedPageDelay(t)
}

// Reproduce the screenshot: 30 latest entries include only episodes 05–08;
// the following page contains 01–04. All three entry points must see them.
func TestSearchPreviewAndSyncReadHistoricalEpisodes(t *testing.T) {
	testDatabase(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("term") != "夏日动画" {
			t.Errorf("lost search term: %s", r.URL.RawQuery)
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		page = max(1, page)
		fmt.Fprint(w, `<rss><channel>`)
		size := 30
		if page == 3 {
			size = 12
		}
		for i := 0; i < size; i++ {
			if i < 4 && page <= 2 {
				episode := i + 1
				if page == 1 {
					episode += 4
				}
				fmt.Fprintf(w, `<item><title>[目标组] 夏日动画 [%02d][1080p][简繁日内封]</title><link>/topic/%d</link><enclosure url="/dl/%d.torrent"/><description>保留简介</description></item>`, episode, episode, episode)
			} else {
				fmt.Fprintf(w, `<item><title>[其他组] 夏日动画 [%02d][720p]</title><link>/other/%d/%d</link></item>`, i+1, page, i)
			}
		}
		fmt.Fprint(w, `</channel></rss>`)
	}))
	defer server.Close()
	pagedTestSource(t, server.URL+"/search.xml?term={keyword}", 30)
	feedURL := discoverSearchURL(discoverBuiltinSources[0].Template, "夏日动画")
	words := "夏日动画,[目标组],1080p,简繁"
	if _, err := db.Exec(`INSERT INTO feeds(id,name,url,include_words,target_title) VALUES(1,?,?,?,?)`, "夏日动画", feedURL, words, "夏日动画"); err != nil {
		t.Fatal(err)
	}
	// A subscription created by the previous single-page implementation.
	discoverBuiltinSources[0].PageParam = ""
	if n, err := syncFeedContext(context.Background(), 1); n != 4 || err != nil {
		t.Fatalf("initial window: %d %v", n, err)
	}
	discoverBuiltinSources[0].PageParam = "page"
	search := discoverGet(t, "/api/discover/search?source=acgrip&keyword="+url.QueryEscape("夏日动画"))
	var found struct {
		Count, Pages     int
		History, Limited bool
		Items            []discoverItem
	}
	if err := json.Unmarshal(search.Body.Bytes(), &found); err != nil || search.Code != 200 || found.Count != 72 || found.Pages != 3 || !found.History || found.Limited {
		t.Fatalf("historical search: %d %s, %v", search.Code, search.Body.String(), err)
	}
	matched := 0
	for _, item := range found.Items {
		if keywordfilter.Allowed(item.Title, words, "") {
			matched++
			if !strings.HasPrefix(item.Link, server.URL+"/topic/") || !strings.HasPrefix(item.Torrent, server.URL+"/dl/") {
				t.Fatalf("page and resource URLs: %+v", item)
			}
		}
	}
	if matched != 8 {
		t.Fatalf("historical episodes found=%d", matched)
	}
	body, _ := json.Marshal(PreviewInput{URL: feedURL, Include: words})
	rec := httptest.NewRecorder()
	preview(rec, httptest.NewRequest("POST", "/api/preview", strings.NewReader(string(body))))
	var projected struct {
		Count, Total, Pages int
		History             bool
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &projected); err != nil || rec.Code != 200 || projected.Count != 8 || projected.Total != 72 || projected.Pages != 3 || !projected.History {
		t.Fatalf("historical preview: %d %s", rec.Code, rec.Body.String())
	}
	if n, err := syncFeedContext(context.Background(), 1); n != 4 || err != nil {
		t.Fatalf("backfill existing subscription: %d %v", n, err)
	}
	if n, err := syncFeedContext(context.Background(), 1); n != 0 || err != nil {
		t.Fatalf("repeat backfill: %d %v", n, err)
	}
	var count, series int
	if err := db.QueryRow(`SELECT COUNT(*) FROM items WHERE anime_title='夏日动画' AND description='保留简介'`).Scan(&count); err != nil || count != 8 {
		t.Fatalf("stored episodes=%d: %v", count, err)
	}
	db.QueryRow("SELECT COUNT(*) FROM animes").Scan(&series)
	if series != 1 {
		t.Fatalf("backfill split into %d series", series)
	}
}

func TestFeedExplicitNextLinks(t *testing.T) {
	noFeedPageDelay(t)
	for _, format := range []string{"rss", "atom", "rdf"} {
		t.Run(format, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.URL.Path == "/start" {
					http.Redirect(w, r, "/feeds/current.xml", http.StatusFound)
					return
				}
				next, episode := "next.xml", 2
				if r.URL.Path == "/feeds/next.xml" {
					next, episode = "", 1
				}
				if format == "atom" {
					fmt.Fprintf(w, `<feed xmlns="http://www.w3.org/2005/Atom"><link rel="next" href="%s"/><entry><title>动画 [%02d]</title><link rel="enclosure" href="../dl/%d.torrent"/><link rel="alternate" href="../topic/%d"/><updated>today</updated><summary>简介</summary></entry></feed>`, next, episode, episode, episode)
				} else if format == "rdf" {
					fmt.Fprintf(w, `<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns:atom="http://www.w3.org/2005/Atom"><channel><atom:link rel="next" href="%s"/></channel><item><title>动画 [%02d]</title><link>../topic/%d</link><enclosure url="../dl/%d.torrent"/></item></rdf:RDF>`, next, episode, episode, episode)
				} else {
					fmt.Fprintf(w, `<rss xmlns:atom="http://www.w3.org/2005/Atom"><channel><atom:link rel="next" href="%s"/><item><title>动画 [%02d]</title><link>../topic/%d</link><enclosure url="../dl/%d.torrent"/></item></channel></rss>`, next, episode, episode, episode)
				}
			}))
			defer server.Close()
			result, err := fetchRSS(context.Background(), server.URL+"/start")
			if err != nil || result.Pages != 2 || len(result.Entries) != 2 || result.Limited || !result.History || requests.Load() != 3 {
				t.Fatalf("next links: %+v err=%v requests=%d", result, err, requests.Load())
			}
			if result.Entries[1].Torrent != server.URL+"/dl/1.torrent" || result.Entries[1].Link != server.URL+"/topic/1" {
				t.Fatalf("resolved against redirected page: %+v", result.Entries)
			}
		})
	}
}

func TestFeedHistoryDedupAndIgnoredPagination(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		page := r.URL.Query().Get("page")
		fmt.Fprint(w, `<rss><channel><item><title>动画 [02]</title><link>/2.torrent</link></item>`)
		if page != "" {
			fmt.Fprint(w, `<item><title>动画 [01]</title><link>/1.torrent</link></item>`)
		}
		fmt.Fprint(w, `</channel></rss>`)
	}))
	defer server.Close()
	pagedTestSource(t, server.URL+"/rss?term={keyword}", 1)
	result, err := fetchRSS(context.Background(), server.URL+"/rss?term=anime")
	if err != nil || len(result.Entries) != 2 || requests.Load() != 3 || !result.Incomplete || !strings.Contains(result.Warning, "重复") {
		t.Fatalf("ignored pagination must stop without duplicates: %+v %v requests=%d", result, err, requests.Load())
	}
}

func TestFeedDoesNotGuessPaginationForUnknownSources(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		fmt.Fprint(w, `<rss><channel>`)
		for i := 0; i < 80; i++ {
			fmt.Fprintf(w, `<item><title>动画 [%02d]</title><link>/dl/%d</link></item>`, i+1, i+1)
		}
		fmt.Fprint(w, `</channel></rss>`)
	}))
	defer server.Close()
	overrideDiscoverSource(t, server.URL+"/rss?term={keyword}")
	rec := discoverGet(t, "/api/discover/search?source=acgrip&keyword=anime")
	var result struct {
		Count, Pages     int
		History, Limited bool
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || result.Count != 80 || result.Pages != 1 || result.History || result.Limited || requests.Load() != 1 {
		t.Fatalf("single RSS must retain more than 60 results without guessing pages: %s", rec.Body.String())
	}
}

func TestHistoryReadFailureKeepsItemsAndRetries(t *testing.T) {
	testDatabase(t)
	var fail atomic.Bool
	fail.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		if page == "2" && fail.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, `<rss><channel>`)
		if page == "" || page == "2" {
			episode := 2
			if page == "2" {
				episode = 1
			}
			fmt.Fprintf(w, `<item><title>动画 [%02d]</title><link>/dl/%d</link></item>`, episode, episode)
		}
		fmt.Fprint(w, `</channel></rss>`)
	}))
	defer server.Close()
	pagedTestSource(t, server.URL+"/rss?term={keyword}", 1)
	feedURL := server.URL + "/rss?term=anime"
	if _, err := db.Exec(`INSERT INTO feeds(id,name,url,target_title) VALUES(1,'动画',?,'动画')`, feedURL); err != nil {
		t.Fatal(err)
	}
	rec := discoverGet(t, "/api/discover/search?source=acgrip&keyword=anime")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"limited":true`) || !strings.Contains(rec.Body.String(), "503") {
		t.Fatalf("partial search hidden: %d %s", rec.Code, rec.Body.String())
	}
	if n, err := syncFeedContext(context.Background(), 1); n != 1 || err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("partial sync must store available items and report failure: %d %v", n, err)
	}
	var lastError string
	db.QueryRow("SELECT last_error FROM feeds WHERE id=1").Scan(&lastError)
	if !strings.Contains(lastError, "503") {
		t.Fatalf("partial sync error not saved: %q", lastError)
	}
	fail.Store(false)
	if n, err := syncFeedContext(context.Background(), 1); n != 1 || err != nil {
		t.Fatalf("retry missed old episode: %d %v", n, err)
	}
	db.QueryRow("SELECT last_error FROM feeds WHERE id=1").Scan(&lastError)
	if lastError != "" {
		t.Fatalf("successful retry kept error: %q", lastError)
	}
}

func TestFeedHistoryBoundaries(t *testing.T) {
	for _, mode := range []string{"pages", "items", "size", "timeout", "cycle", "cross-origin", "cross-origin-redirect"} {
		t.Run(mode, func(t *testing.T) {
			noFeedPageDelay(t)
			var otherRequests atomic.Int32
			other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { otherRequests.Add(1) }))
			defer other.Close()
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				page, _ := strconv.Atoi(r.URL.Query().Get("page"))
				page = max(1, page)
				if page == 2 && mode == "timeout" {
					<-r.Context().Done()
					return
				}
				if page == 2 && mode == "cross-origin-redirect" {
					http.Redirect(w, r, other.URL, http.StatusFound)
					return
				}
				next := fmt.Sprintf("?term=anime&amp;page=%d", page+1)
				if mode == "cycle" {
					next = "?term=anime"
				} else if mode == "cross-origin" {
					next = other.URL
				}
				fmt.Fprintf(w, `<rss xmlns:atom="http://www.w3.org/2005/Atom"><channel><atom:link rel="next" href="%s"/>`, next)
				count := 1
				if mode == "items" {
					count = feedMaxItems + 1
				}
				for i := 0; i < count; i++ {
					fmt.Fprintf(w, `<item><title>动画</title><link>/dl/%d/%d</link></item>`, page, i)
				}
				if page == 2 && mode == "size" {
					fmt.Fprintf(w, `<description>%s</description>`, strings.Repeat("x", 2048))
				}
				fmt.Fprint(w, `</channel></rss>`)
			}))
			defer server.Close()
			if mode == "size" {
				old := feedMaxTotalBytes
				feedMaxTotalBytes = 1024
				t.Cleanup(func() { feedMaxTotalBytes = old })
			}
			ctx := context.Background()
			if mode == "timeout" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
				defer cancel()
			}
			result, err := fetchRSS(ctx, server.URL+"/?term=anime")
			if err != nil || !result.Incomplete || !result.Limited || result.Warning == "" || len(result.Entries) == 0 || otherRequests.Load() != 0 {
				t.Fatalf("boundary not surfaced: %+v err=%v cross-origin requests=%d", result, err, otherRequests.Load())
			}
			if mode == "pages" && requests.Load() != feedMaxPages {
				t.Fatalf("page ceiling: %d", requests.Load())
			}
			if mode == "items" && len(result.Entries) != feedMaxItems {
				t.Fatalf("item ceiling: %d", len(result.Entries))
			}
			if mode == "cycle" && requests.Load() != 1 {
				t.Fatalf("cycle refetched: %d", requests.Load())
			}
		})
	}
}

func TestVerifiedSourcesAndRecentWindowLimits(t *testing.T) {
	expected := map[string]struct {
		host  string
		limit int
	}{
		"acgrip": {"acg.rip", 0}, "mikan": {"mikanani.me", 100}, "dmhy": {"share.dmhy.org", 500},
	}
	for id, want := range expected {
		source, ok := discoverSourceByID(id)
		if !ok {
			t.Fatalf("missing source %s", id)
		}
		address, err := url.Parse(discoverSearchURL(source.Template, "夏日动画 字幕组"))
		if err != nil || address.Host != want.host || source.ResultLimit != want.limit || feedSource(address).ID != id {
			t.Fatalf("invalid source %s: %+v %v", id, source, err)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("page") {
			t.Error("unverified page parameter sent")
		}
		fmt.Fprint(w, `<rss><channel>`)
		for i := 0; i < 100; i++ {
			fmt.Fprintf(w, `<item><title>动画 [%d]</title><link>/dl/%d</link></item>`, i+1, i+1)
		}
		fmt.Fprint(w, `</channel></rss>`)
	}))
	defer server.Close()
	overrideDiscoverSource(t, server.URL+"/rss?term={keyword}")
	discoverBuiltinSources[0].ResultLimit = 100
	result, err := fetchRSS(context.Background(), server.URL+"/rss?term=anime")
	if err != nil || len(result.Entries) != 100 || result.Pages != 1 || result.History || !result.Limited || result.Incomplete || !strings.Contains(result.Warning, "最近 100 条") {
		t.Fatalf("source limit must be visible without dropping results: %+v %v", result, err)
	}
}

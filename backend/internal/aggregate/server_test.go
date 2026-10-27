package aggregate

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

const testToken = "test-search-token-not-a-real-secret-123456"
const testHash = "0123456789abcdef0123456789abcdef01234567"

func fixture(t *testing.T, handler http.HandlerFunc) *Server {
	t.Helper()
	upstream := httptest.NewServer(handler)
	t.Cleanup(upstream.Close)
	s, err := New(Config{Token: testToken, JackettURL: upstream.URL, JackettKey: "test-indexer-key", BangumiURL: upstream.URL, Indexers: []string{"mikan", "dmhy"}})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func request(s *Server, path, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestSearchAggregationCacheAndSecrets(t *testing.T) {
	var calls atomic.Int32
	s := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/v0/subjects/101" {
			fmt.Fprint(w, `{"id":101,"type":2,"name":"Test Anime","name_cn":"测试动画"}`)
			return
		}
		if r.URL.Query().Get("apikey") != "test-indexer-key" || r.URL.Query().Get("q") == "" || r.URL.Query().Get("cat") != "5070" {
			t.Error("incorrect indexer request")
		}
		fmt.Fprintf(w, `<rss xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel>
		<item><title>[Group] 测试动画 S2 - 01 [1080p]</title><comments>https://site.example/view/1</comments><pubDate>Tue, 27 Oct 2026 12:00:00 GMT</pubDate><torznab:attr name="infohash" value="%s"/><torznab:attr name="size" value="12345"/></item>
		<item><title>[Group] 另一部动画 - 01</title><torznab:attr name="infohash" value="%s"/></item>
		<item><title>[Group] 测试动画 - 02</title><enclosure url="http://internal/api?apikey=test-indexer-key"/></item>
		</channel></rss>`, testHash, strings.Repeat("a", 40))
	})
	w := request(s, "/v1/search?subject_id=101", testToken)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var result Result
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || len(result.Items[0].Sources) != 2 || result.Items[0].Size != 12345 || !result.Items[0].NeedsReview || result.Items[0].Season != 2 {
		t.Fatalf("unexpected result %+v", result)
	}
	if strings.Contains(w.Body.String(), "test-indexer-key") || strings.Contains(w.Body.String(), testToken) {
		t.Fatal("credentials leaked")
	}
	count := calls.Load()
	if count != 5 {
		t.Fatalf("expected subject + two queries per source, got %d", count)
	}
	if request(s, "/v1/search?subject_id=101", testToken).Code != 200 || calls.Load() != count {
		t.Fatal("cache missed")
	}
}

func TestSourceCategories(t *testing.T) {
	s := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		wantCategory := "5070"
		if strings.Contains(r.URL.Path, "/acgrip/") {
			wantCategory = "5000"
		}
		if r.URL.Query().Get("cat") != wantCategory {
			t.Errorf("%s: category = %q, want %q", r.URL.Path, r.URL.Query().Get("cat"), wantCategory)
		}
		fmt.Fprint(w, `<rss><channel></channel></rss>`)
	})
	for _, source := range []string{"mikan", "dmhy", "acgrip", "nyaasi", "bangumi-moe"} {
		t.Run(source, func(t *testing.T) {
			if _, _, err := s.searchSource(context.Background(), source, "Test Anime", []string{"Test Anime"}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestValidationAuthenticationAndRateLimit(t *testing.T) {
	var calls atomic.Int32
	s := fixture(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); http.Error(w, "upstream", 500) })
	for _, tc := range []struct {
		path, token string
		code        int
	}{
		{"/healthz", "", 200}, {"/v1/search?subject_id=1", "", 401}, {"/v1/search?subject_id=1", "wrong", 401},
		{"/v1/search?subject_id=0", testToken, 400}, {"/v1/search?subject_id=-1", testToken, 400},
		{"/v1/search?subject_id=2147483648", testToken, 400}, {"/unknown", testToken, 404},
	} {
		if w := request(s, tc.path, tc.token); w.Code != tc.code {
			t.Fatalf("%s: %d", tc.path, w.Code)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid request reached upstream")
	}
	s.minute, s.count = time.Now().Unix()/60, 60
	if w := request(s, "/v1/search?subject_id=1", testToken); w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatal("rate limit missing")
	}
	for _, ids := range [][]string{nil, {"all"}, {"../mikan"}, {"mikan", "mikan"}} {
		if _, err := New(Config{Token: testToken, JackettURL: "http://localhost:9117", JackettKey: "test", Indexers: ids}); err == nil {
			t.Fatalf("invalid indexers accepted %v", ids)
		}
	}
}

func TestPublicSearchOptInAuthenticationAndLimits(t *testing.T) {
	s := fixture(t, func(http.ResponseWriter, *http.Request) { t.Error("cached public test reached upstream") })
	s.cache[1] = cacheEntry{result: Result{SubjectID: 1, Items: []Resource{}, Warnings: []string{}}, until: time.Now().Add(time.Minute)}
	if request(s, "/v1/search?subject_id=1", "").Code != 401 {
		t.Fatal("private search must require authentication by default")
	}
	s.config.Public = true
	for _, tc := range []struct {
		path, token string
		code        int
	}{
		{"/v1/search?subject_id=1", "", 200},
		{"/v1/search?subject_id=1", testToken, 200},
		{"/v1/search?subject_id=1", "wrong", 401},
		{"/v1/search?subject_id=0", "", 400},
		{"/unknown", "", 404},
	} {
		if w := request(s, tc.path, tc.token); w.Code != tc.code {
			t.Fatalf("public search %s: got %d want %d", tc.path, w.Code, tc.code)
		}
	}
	s.minute, s.count = time.Now().Unix()/60, 60
	if w := request(s, "/v1/search?subject_id=1", ""); w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatal("public searches must retain rate limiting, including cache hits")
	}
	config := s.config
	config.Token = ""
	if _, err := New(config); err == nil {
		t.Fatal("public mode must not accept missing private configuration")
	}
}

func TestPartialFailureNotCachedAndUpstreamErrorsRedacted(t *testing.T) {
	var calls atomic.Int32
	s := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if strings.HasPrefix(r.URL.Path, "/v0/") {
			fmt.Fprint(w, `{"id":1,"type":2,"name":"Test Anime"}`)
			return
		}
		if strings.Contains(r.URL.Path, "/dmhy/") {
			http.Error(w, "secret test-indexer-key", 500)
			return
		}
		fmt.Fprint(w, `<rss><channel/></rss>`)
	})
	w := request(s, "/v1/search?subject_id=1", testToken)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "dmhy") || strings.Contains(w.Body.String(), "test-indexer-key") {
		t.Fatal(w.Body.String())
	}
	count := calls.Load()
	request(s, "/v1/search?subject_id=1", testToken)
	if calls.Load() == count {
		t.Fatal("partial response must not poison cache")
	}
}

func TestCancellationRedirectAndMalformedFeed(t *testing.T) {
	for _, mode := range []string{"cancel", "redirect", "invalid", "oversized"} {
		t.Run(mode, func(t *testing.T) {
			started := make(chan struct{}, 1)
			s := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/v0/") {
					fmt.Fprint(w, `{"id":1,"type":2,"name":"Test Anime"}`)
					return
				}
				switch mode {
				case "cancel":
					select {
					case started <- struct{}{}:
					default:
					}
					<-r.Context().Done()
				case "redirect":
					http.Redirect(w, r, "/should-not-follow", 302)
				case "invalid":
					fmt.Fprint(w, `<error code="100"/>`)
				case "oversized":
					fmt.Fprint(w, strings.Repeat("x", (8<<20)+1))
				}
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { _, err := s.searchCached(ctx, 1); done <- err }()
			if mode == "cancel" {
				<-started
				cancel()
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("invalid upstream accepted")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("request not bounded/cancelled")
			}
		})
	}
}

func TestPublicLinks(t *testing.T) {
	s := fixture(t, func(http.ResponseWriter, *http.Request) {})
	for _, raw := range []string{"javascript:alert(1)", "file:///secret", "https://user:pass@example.org/a", "https://example.org/a?apikey=secret", s.config.JackettURL + "/dl",
		"http://192.168.1.1/page", "http://localhost/page", "https://example.org/?id=%74est-indexer-key", "https://example.org/?id=" + url.QueryEscape(testToken),
		"https://example.org/?apikey;secret", "https://example.org/%74est-search-token-not-a-real-secret-123456"} {
		if s.publicDetails(raw) != "" {
			t.Fatalf("unsafe link accepted: %s", raw)
		}
	}
	if publicMagnet("magnet:?xt=urn:btih:"+strings.ToUpper(testHash)+"&tr=https://tracker.example/?passkey=secret") != "magnet:?xt=urn:btih:"+testHash {
		t.Fatal("magnet not normalized")
	}
	if publicMagnet("magnet:?xt=urn:btih:invalid") != "" {
		t.Fatal("invalid hash accepted")
	}
}

func TestConcurrentSearchCoalescingAndLimit(t *testing.T) {
	started, release := make(chan struct{}, 8), make(chan struct{})
	defer close(release)
	var calls atomic.Int32
	s := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		started <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
		}
		http.Error(w, "unavailable", 500)
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 5)
	for id := 1; id <= 4; id++ {
		go func(id int) { _, err := s.searchCached(ctx, id); done <- err }(id)
	}
	for i := 0; i < 4; i++ {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("search did not start")
		}
	}
	if _, err := s.searchCached(ctx, 5); err == nil {
		t.Fatal("independent search limit missing")
	}
	follower, stop := context.WithCancel(ctx)
	stop()
	if _, err := s.searchCached(follower, 1); err == nil {
		t.Fatal("cancelled follower did not return")
	}
	if calls.Load() != 4 {
		t.Fatal("same subject was not coalesced")
	}
	cancel()
	for i := 0; i < 4; i++ {
		select {
		case err := <-done:
			if err == nil {
				t.Fatal("cancelled search succeeded")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("search ignored cancellation")
		}
	}
}

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestAggregateServiceDefaultsAndCredentialIsolation(t *testing.T) {
	for _, key := range []string{"AGGREGATE_SEARCH_URL", "AGGREGATE_SEARCH_TOKEN"} {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	if configuredAggregateSearchURL() != defaultAggregateSearchURL {
		t.Fatal("default service address missing")
	}
	if configuredAggregateSearchToken("https://custom.example") != "" {
		t.Fatal("local credential leaked to custom endpoint")
	}
	t.Setenv("AGGREGATE_SEARCH_URL", " https://custom.example/ ")
	if configuredAggregateSearchURL() != "https://custom.example" {
		t.Fatal("custom address ignored")
	}
	t.Setenv("AGGREGATE_SEARCH_TOKEN", "test-explicit-token-not-a-real-secret")
	if configuredAggregateSearchToken("https://custom.example") != os.Getenv("AGGREGATE_SEARCH_TOKEN") {
		t.Fatal("explicit credential ignored")
	}
	t.Setenv("AGGREGATE_SEARCH_URL", "")
	t.Setenv("AGGREGATE_SEARCH_TOKEN", "")
	if configuredAggregateSearchURL() != "" || configuredAggregateSearchToken(defaultAggregateSearchURL) != "" {
		t.Fatal("explicit empty configuration must disable defaults")
	}
}

func TestDiscoverSubjectsWithoutRSSOrDatabase(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("metadata request leaked authentication")
		}
		fmt.Fprint(w, `{"list":[{"id":1,"name":"Original","name_cn":"动画","summary":"简介","images":{"large":"https://lain.bgm.tv/test.jpg"}}]}`)
	}))
	defer upstream.Close()
	oldAPI, oldURL, oldToken := bangumiAPI, aggregateSearchURL, aggregateSearchToken
	bangumiAPI, aggregateSearchURL, aggregateSearchToken = upstream.URL, "", ""
	t.Cleanup(func() { bangumiAPI, aggregateSearchURL, aggregateSearchToken = oldAPI, oldURL, oldToken })
	w := httptest.NewRecorder()
	discoverSubjectsAPI(w, httptest.NewRequest(http.MethodGet, "/api/discover/subjects?keyword=anime", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"resources_enabled":false`) || !strings.Contains(w.Body.String(), `"name_cn":"动画"`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	for _, word := range []string{"", strings.Repeat("x", 101)} {
		w = httptest.NewRecorder()
		discoverSubjectsAPI(w, httptest.NewRequest(http.MethodGet, "/api/discover/subjects?keyword="+word, nil))
		if w.Code != 400 {
			t.Fatal("invalid keyword accepted")
		}
	}
}

func TestDefaultPublicAggregateService(t *testing.T) {
	oldURL, oldToken, oldClient := aggregateSearchURL, aggregateSearchToken, httpClient
	t.Cleanup(func() { aggregateSearchURL, aggregateSearchToken, httpClient = oldURL, oldToken, oldClient })
	for _, tc := range []struct {
		address, token string
		configured     bool
	}{
		{defaultAggregateSearchURL, "", true},
		{defaultAggregateSearchURL, "short", false},
		{"https://custom.example", "", false},
		{"https://custom.example", "test-private-token-not-a-real-secret-123456", true},
		{"", "", false},
	} {
		aggregateSearchURL, aggregateSearchToken = tc.address, tc.token
		if aggregateConfigured() != tc.configured {
			t.Fatalf("unexpected configured state for %q", tc.address)
		}
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.URL.Query().Get("subject_id") != "1" {
			t.Error("public search must not send credentials or lose subject ID")
		}
		fmt.Fprint(w, `{"subject_id":1,"items":[],"warnings":[]}`)
	}))
	defer upstream.Close()
	aggregateSearchURL, aggregateSearchToken = defaultAggregateSearchURL, ""
	httpClient = &http.Client{Transport: rewriteTransport{target: upstream.URL}}
	w := httptest.NewRecorder()
	discoverResourcesAPI(w, httptest.NewRequest(http.MethodGet, "/api/discover/resources?subject_id=1", nil))
	if w.Code != 200 {
		t.Fatalf("default service must work without local secrets: %d %s", w.Code, w.Body.String())
	}
}

func TestDiscoverResourcesCredentialsStatusAndIdentity(t *testing.T) {
	status, response := 200, `{"subject_id":1,"items":[],"warnings":[]}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-search-token-not-a-real-secret-123456" || r.URL.Query().Get("subject_id") != "1" {
			t.Error("incorrect aggregation request")
		}
		w.WriteHeader(status)
		fmt.Fprint(w, response)
	}))
	defer upstream.Close()
	oldURL, oldToken := aggregateSearchURL, aggregateSearchToken
	aggregateSearchURL, aggregateSearchToken = upstream.URL, "test-search-token-not-a-real-secret-123456"
	t.Cleanup(func() { aggregateSearchURL, aggregateSearchToken = oldURL, oldToken })
	for _, tc := range []struct {
		status int
		body   string
		code   int
	}{
		{200, response, 200}, {401, "upstream-secret", 502}, {429, "upstream-secret", 429},
		{200, `{"subject_id":2}`, 502}, {200, "invalid", 502}, {200, strings.Repeat("x", (2<<20)+1), 502},
	} {
		status, response = tc.status, tc.body
		w := httptest.NewRecorder()
		discoverResourcesAPI(w, httptest.NewRequest(http.MethodGet, "/api/discover/resources?subject_id=1", nil))
		if w.Code != tc.code || strings.Contains(w.Body.String(), "upstream-secret") || strings.Contains(w.Body.String(), aggregateSearchToken) {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
	for _, address := range []string{"http://public.example", "https://user:pass@example.org", "https://example.org/?key=secret", "file:///tmp/service"} {
		aggregateSearchURL = address
		if aggregateConfigured() {
			t.Fatal("unsafe service configuration accepted")
		}
	}
}

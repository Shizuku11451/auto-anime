package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBangumiSubjectReadOnlyDetails(t *testing.T) {
	response := `{"id":101,"name":"original","name_cn":"动画","summary":"官方简介","eps":12,"date":"2026-10-01","rating":{"score":8.2,"total":500,"rank":42},"images":{"large":"https://lain.bgm.tv/cover.jpg"}}`
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodGet || r.URL.Path != "/v0/subjects/101" || r.Header.Get("Authorization") != "" {
			t.Error("unexpected metadata request")
		}
		fmt.Fprint(w, response)
	}))
	defer server.Close()
	old := bangumiAPI
	bangumiAPI = server.URL
	t.Cleanup(func() { bangumiAPI = old })
	w := httptest.NewRecorder()
	// No database is needed: browsing cloud details cannot create a local anime.
	bangumiSubjectAPI(w, httptest.NewRequest(http.MethodGet, "/api/bangumi/subjects/101", nil))
	var result map[string]any
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil {
		t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
	}
	if result["rating"] != 8.2 || result["rating_total"] != float64(500) || result["eps"] != float64(12) || result["summary"] != "官方简介" || result["rank"] != float64(42) || result["date"] != "2026-10-01" {
		t.Fatalf("missing details: %+v", result)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("details must not use browser cache")
	}
	for _, path := range []string{"0", "-1", "abc", "101/extra", "2147483648"} {
		w = httptest.NewRecorder()
		bangumiSubjectAPI(w, httptest.NewRequest(http.MethodGet, "/api/bangumi/subjects/"+path, nil))
		if w.Code != 400 {
			t.Fatalf("invalid id %q accepted", path)
		}
	}
	w = httptest.NewRecorder()
	bangumiSubjectAPI(w, httptest.NewRequest(http.MethodPost, "/api/bangumi/subjects/101", nil))
	if w.Code != 405 || calls != 1 {
		t.Fatal("invalid requests reached upstream")
	}
	for _, bad := range []string{`{"id":202}`, `invalid`, `{"id":101}` + strings.Repeat(" ", 2<<20)} {
		response = bad
		w = httptest.NewRecorder()
		bangumiSubjectAPI(w, httptest.NewRequest(http.MethodGet, "/api/bangumi/subjects/101", nil))
		if w.Code != 502 {
			t.Fatalf("invalid upstream response accepted: %d", w.Code)
		}
	}
}

func TestBangumiSubjectCancellationAndConcurrency(t *testing.T) {
	cancelled := make(chan struct{})
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(cancelled)
	}))
	defer server.Close()
	old := bangumiAPI
	bangumiAPI = server.URL
	t.Cleanup(func() { bangumiAPI = old })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		bangumiSubjectAPI(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/bangumi/subjects/101", nil).WithContext(ctx))
		close(done)
	}()
	<-started
	cancel()
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream request was not cancelled")
	}
	<-done
	for i := 0; i < cap(bangumiSubjectSlots); i++ {
		bangumiSubjectSlots <- struct{}{}
	}
	w := httptest.NewRecorder()
	bangumiSubjectAPI(w, httptest.NewRequest(http.MethodGet, "/api/bangumi/subjects/101", nil))
	for i := 0; i < cap(bangumiSubjectSlots); i++ {
		<-bangumiSubjectSlots
	}
	if w.Code != 429 {
		t.Fatalf("concurrent requests not bounded: %d", w.Code)
	}
}

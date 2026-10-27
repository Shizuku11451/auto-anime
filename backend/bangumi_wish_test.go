package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBangumiWishSyncPreservesPreviousListOnPageFailure(t *testing.T) {
	testDatabase(t)
	fail := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing authorization")
		}
		if r.URL.Query().Get("subject_type") != "2" || r.URL.Query().Get("type") != "1" {
			t.Error("incorrect filters")
		}
		if r.URL.Query().Get("offset") == "50" {
			if fail {
				http.Error(w, "failed", 500)
				return
			}
			fmt.Fprint(w, `{"total":51,"data":[{"subject_id":51,"subject":{"name":"last"}}]}`)
			return
		}
		fmt.Fprint(w, `{"total":51,"data":[`)
		for i := 1; i <= 50; i++ {
			if i > 1 {
				fmt.Fprint(w, ",")
			}
			fmt.Fprintf(w, `{"subject_id":%d,"subject":{"name":"anime"}}`, i)
		}
		fmt.Fprint(w, `]}`)
	}))
	defer server.Close()
	old := bangumiAPI
	bangumiAPI = server.URL
	t.Cleanup(func() { bangumiAPI = old })
	if err := saveBangumiWishSettings(map[string]string{"bangumi_token": "test-token", "bangumi_username": "tester"}); err != nil {
		t.Fatal(err)
	}
	if err := bangumiWishSync(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := getSetting("bangumi_wishes")
	if !strings.Contains(before, `"id":51`) {
		t.Fatal("missing second page")
	}
	fail = true
	if err := bangumiWishSync(context.Background()); err == nil {
		t.Fatal("expected failure")
	}
	if getSetting("bangumi_wishes") != before {
		t.Fatal("failed sync replaced list")
	}
	if strings.Contains(getSetting("bangumi_sync_error"), "test-token") {
		t.Fatal("token leaked")
	}
}

func TestBangumiWishAccountDoesNotExposeToken(t *testing.T) {
	testDatabase(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"username":"tester"}`) }))
	defer server.Close()
	old := bangumiAPI
	bangumiAPI = server.URL
	t.Cleanup(func() { bangumiAPI = old })
	request := httptest.NewRequest("POST", "/api/bangumi/wishes", strings.NewReader(`{"token":"secret"}`))
	response := httptest.NewRecorder()
	bangumiWishAPI(response, request)
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	response = httptest.NewRecorder()
	bangumiWishAPI(response, httptest.NewRequest("GET", "/api/bangumi/wishes", nil))
	if strings.Contains(response.Body.String(), "secret") {
		t.Fatal("token exposed")
	}
	response = httptest.NewRecorder()
	bangumiWishAPI(response, httptest.NewRequest("DELETE", "/api/bangumi/wishes", nil))
	if response.Code != 200 || getSetting("bangumi_token") != "" {
		t.Fatal("disconnect failed")
	}
}

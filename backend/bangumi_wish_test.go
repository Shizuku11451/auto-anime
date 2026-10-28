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

func TestBangumiWishCalendarFallback(t *testing.T) {
	for _, scenario := range []string{"success", "unavailable", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			testDatabase(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calendarHits := 0
			scheduleTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/calendar" {
					if scenario == "cancelled" {
						fmt.Fprint(w, `{"total":1,"data":[{"subject_id":123,"type":3,"subject":{"name":"airing"}}]}`)
					} else {
						fmt.Fprint(w, `{"total":2,"data":[{"subject_id":123,"type":3,"subject":{"name":"airing"}},{"subject_id":456,"type":1,"subject":{"name":"unknown"}}]}`)
					}
					return
				}
				calendarHits++
				if scenario == "cancelled" && calendarHits == 1 {
					cancel()
					select {
					case <-r.Context().Done():
					case <-time.After(time.Second):
						t.Error("calendar request did not inherit sync cancellation")
					}
					return
				}
				if scenario == "unavailable" {
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
					return
				}
				fmt.Fprint(w, `[{"weekday":{"id":4},"items":[{"id":123}]}]`)
			})
			calendarWeekdayLookup = calendarWeekday
			const previous = `[{"id":7,"name":"previous","type":1}]`
			if err := saveBangumiWishSettings(map[string]string{"bangumi_token": "test-token", "bangumi_username": "tester", "bangumi_wishes": previous, "bangumi_last_sync": "previous-sync"}); err != nil {
				t.Fatal(err)
			}
			err := bangumiWishSync(ctx)
			if scenario == "cancelled" {
				if err == nil || getSetting("bangumi_wishes") != previous || getSetting("bangumi_last_sync") != "previous-sync" {
					t.Fatalf("cancelled sync replaced previous cache: %v", err)
				}
				if day := calendarWeekday(context.Background(), 123); day != 4 || calendarHits != 2 {
					t.Fatalf("cancelled request poisoned calendar cache: day=%d, requests=%d", day, calendarHits)
				}
				return
			}
			var items []bangumiWish
			if err != nil || json.Unmarshal([]byte(getSetting("bangumi_wishes")), &items) != nil || len(items) != 2 {
				t.Fatalf("optional calendar blocked collection sync: %v", err)
			}
			want := 0
			if scenario == "success" {
				want = 4
			}
			if items[0].BroadcastWeekday != want || items[1].BroadcastWeekday != 0 || calendarHits != 1 {
				t.Fatalf("incorrect calendar fallback: %+v, requests=%d", items, calendarHits)
			}
		})
	}
}

func TestBangumiWishSyncPreservesPreviousListOnPageFailure(t *testing.T) {
	testDatabase(t)
	fail := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing authorization")
		}
		if r.Method != http.MethodGet || r.URL.Query().Get("subject_type") != "2" || r.URL.Query().Has("type") {
			t.Error("incorrect filters")
		}
		if r.URL.Query().Get("offset") == "50" {
			if fail {
				http.Error(w, "failed", 500)
				return
			}
			fmt.Fprint(w, `{"total":51,"data":[{"subject_id":51,"type":3,"rate":10,"subject":{"name":"last","score":8.2,"short_summary":"  简介第一行\n第二行  ","images":{"large":"https://example.test/large.jpg"}}}]}`)
			return
		}
		fmt.Fprint(w, `{"total":51,"data":[`)
		for i := 1; i <= 50; i++ {
			if i > 1 {
				fmt.Fprint(w, ",")
			}
			fmt.Fprintf(w, `{"subject_id":%d,"type":%d,"subject":{"name":"anime"}}`, i, (i-1)%5+1)
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
	var items []bangumiWish
	if err := json.Unmarshal([]byte(before), &items); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if items[i].Type != i+1 {
			t.Fatalf("collection type lost: %+v", items[i])
		}
	}
	if items[50].Summary != "简介第一行\n第二行" || items[0].Summary != "" {
		t.Fatalf("summary not cached: %+v / %+v", items[50], items[0])
	}
	if !strings.Contains(before, `"id":51`) {
		t.Fatal("missing second page")
	}
	if !strings.Contains(before, `"cover":"https://example.test/large.jpg"`) {
		t.Fatal("missing wish cover")
	}
	if items[50].Rating != 8.2 || items[0].Rating != 0 {
		t.Fatalf("expected community score, not personal rate: %+v", items[50])
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

func TestBangumiCollectionChangesReplaceCacheOnly(t *testing.T) {
	testDatabase(t)
	if _, err := db.Exec(`INSERT INTO animes(id,title,bangumi_id) VALUES(7,'local anime',101)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO feeds(id,name,url,enabled,auto_download) VALUES(7,'local feed','https://example.test/rss',1,1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO items(id,feed_id,title,status,file_path) VALUES(7,7,'local release','已保存','media/local.mkv')`); err != nil {
		t.Fatal(err)
	}
	response := `{"total":1,"data":[{"subject_id":101,"type":1,"subject":{"name":"anime","images":{"medium":"https://example.test/medium.jpg"}}}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Error("remote mutation attempted")
		}
		fmt.Fprint(w, response)
	}))
	defer server.Close()
	old := bangumiAPI
	bangumiAPI = server.URL
	t.Cleanup(func() { bangumiAPI = old })
	if err := saveBangumiWishSettings(map[string]string{"bangumi_token": "test-token", "bangumi_username": "tester"}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []int{1, 3, 2, 4, 5} {
		response = fmt.Sprintf(`{"total":1,"data":[{"subject_id":101,"type":%d,"subject":{"name":"anime","images":{"medium":"https://example.test/medium.jpg"}}}]}`, kind)
		if err := bangumiWishSync(context.Background()); err != nil {
			t.Fatal(err)
		}
		var items []bangumiWish
		if err := json.Unmarshal([]byte(getSetting("bangumi_wishes")), &items); err != nil || len(items) != 1 || items[0].Type != kind || items[0].Cover != "https://example.test/medium.jpg" {
			t.Fatalf("incorrect sync result: %+v, %v", items, err)
		}
	}
	before, lastSync := getSetting("bangumi_wishes"), getSetting("bangumi_last_sync")
	for _, invalid := range []string{
		`{"total":1,"data":[{"subject_id":101,"subject":{"name":"anime"}}]}`,
		`{"total":1,"data":[{"subject_id":101,"type":6,"subject":{"name":"anime"}}]}`,
		`{"total":2,"data":[{"subject_id":101,"type":1,"subject":{"name":"anime"}},{"subject_id":101,"type":3,"subject":{"name":"anime"}}]}`,
		`{"total":1,"data":[]}`,
		`{"total":10001,"data":[]}`,
	} {
		response = invalid
		if err := bangumiWishSync(context.Background()); err == nil || getSetting("bangumi_wishes") != before || getSetting("bangumi_last_sync") != lastSync {
			t.Fatal("invalid response replaced successful cache")
		}
	}
	response = `{"total":0,"data":[]}`
	if err := bangumiWishSync(context.Background()); err != nil || getSetting("bangumi_wishes") != "[]" || getSetting("bangumi_sync_error") != "" {
		t.Fatalf("empty collection sync failed: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM animes WHERE id=7 AND bangumi_id=101`).Scan(&count); err != nil || count != 1 {
		t.Fatal("local anime changed")
	}
	if err := db.QueryRow(`SELECT count(*) FROM feeds WHERE id=7 AND enabled=1 AND auto_download=1`).Scan(&count); err != nil || count != 1 {
		t.Fatal("subscription changed")
	}
	if err := db.QueryRow(`SELECT count(*) FROM items WHERE id=7 AND status='已保存' AND file_path='media/local.mkv'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("local media record changed")
	}
}

func TestBangumiLegacyWishCacheHasWishType(t *testing.T) {
	testDatabase(t)
	if err := saveBangumiWishSettings(map[string]string{"bangumi_wishes": `[{"id":101,"name":"legacy"}]`}); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	bangumiWishAPI(w, httptest.NewRequest("GET", "/api/bangumi/wishes", nil))
	var state bangumiWishState
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil || len(state.Items) != 1 || state.Items[0].Type != 1 {
		t.Fatalf("legacy cache incompatible: %s", w.Body.String())
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

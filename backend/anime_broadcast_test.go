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

func TestSubjectBroadcastWeekday(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  int
	}{
		{`"星期一"`, 1}, {`"每周二"`, 2}, {`"水曜日"`, 3}, {`"毎週木曜"`, 4}, {`"Friday"`, 5}, {`"週六"`, 6}, {`"星期天"`, 7},
		{`[{"v":"星期日"},{"v":"日曜日"}]`, 7}, {`"每周五 23:30"`, 5},
		{`"星期一、星期四"`, 0}, {`"星期五、六"`, 0}, {`"星期一至五"`, 0}, {`"未定（原星期一）"`, 0},
		{`"每日"`, 0}, {`"月～金"`, 0}, {`"2026-10-04"`, 0}, {`123`, 0}, {`null`, 0}, {`{"v":"星期一"}`, 0},
	} {
		subject := &bangumiSubject{Infobox: json.RawMessage(`[{"key":"放送星期","value":` + tc.value + `}]`)}
		if got := subjectBroadcastWeekday(subject); got != tc.want {
			t.Errorf("%s: got %d want %d", tc.value, got, tc.want)
		}
	}
	for _, raw := range []string{`[{"key":"放送开始","value":"2026-10-04"}]`, `[{"key":"别名","value":"星期一的朋友"}]`, `[`, `null`} {
		if got := subjectBroadcastWeekday(&bangumiSubject{Infobox: json.RawMessage(raw)}); got != 0 {
			t.Errorf("invalid infobox: %s", raw)
		}
	}
}

func scheduleTestServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	s := httptest.NewServer(handler)
	oldAPI, oldClient, oldEnabled := bangumiAPI, metaClient, bangumiEnabled
	bangumiAPI, metaClient, bangumiEnabled = s.URL, s.Client(), true
	t.Cleanup(func() { s.Close(); bangumiAPI, metaClient, bangumiEnabled = oldAPI, oldClient, oldEnabled })
}

func seedBroadcastAnime(t *testing.T) int64 {
	t.Helper()
	id, err := ensureAnime("自动周更测试")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO anime_meta(anime_title,subject_id,source,rating,summary) VALUES('自动周更测试',123,'bangumi',8,'已缓存资料')`)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestBroadcastBackfillAndManualOverride(t *testing.T) {
	testDatabase(t)
	withMetaTestEnv(t)
	id := seedBroadcastAnime(t)
	hits := 0
	scheduleTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path != "/v0/subjects/123" {
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"id":123,"infobox":[{"key":"放送星期","value":"星期日"}]}`)
	})
	enqueueMissingMeta()
	select {
	case title := <-metaQueue:
		lookupMetaWithSchedule(title)
	default:
		t.Fatal("cached library anime without resources was not queued")
	}
	putAnime(t, id, `{"update_weekday":2}`)
	refreshBroadcastWeekday("自动周更测试")
	if hits != 1 {
		t.Fatalf("cache not reused: %d", hits)
	}
	assertBroadcast := func() {
		t.Helper()
		r := httptest.NewRecorder()
		animeDetail(r, id)
		var detail struct {
			Manual int `json:"update_weekday"`
			Auto   int `json:"broadcast_weekday"`
		}
		if err := json.Unmarshal(r.Body.Bytes(), &detail); err != nil || detail.Manual != 2 || detail.Auto != 7 {
			t.Fatalf("detail: %s %v", r.Body.String(), err)
		}
		list := httptest.NewRecorder()
		animesAPI(list, httptest.NewRequest("GET", "/api/animes", nil))
		if !strings.Contains(list.Body.String(), `"broadcast_weekday":7`) {
			t.Fatalf("list: %s", list.Body.String())
		}
	}
	assertBroadcast()
	// 刷新和重启保持手动值及自动缓存。
	db.Close()
	initDB()
	assertBroadcast()
	if _, err := db.Exec("UPDATE anime_meta SET broadcast_checked_at=?", time.Now().Add(-8*24*time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	refreshBroadcastWeekday("自动周更测试")
	if hits != 2 {
		t.Fatalf("expired cache not refreshed: %d", hits)
	}
	assertBroadcast()
}

func TestBroadcastFailuresAndLateResponses(t *testing.T) {
	for _, scenario := range []string{"failure", "timeout", "unknown", "changed-binding", "manual-edit", "wrong-id"} {
		t.Run(scenario, func(t *testing.T) {
			testDatabase(t)
			id := seedBroadcastAnime(t)
			if _, err := db.Exec(`UPDATE anime_meta SET broadcast_weekday=3,broadcast_subject_id=123`); err != nil {
				t.Fatal(err)
			}
			hits := 0
			scheduleTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				hits++
				switch scenario {
				case "timeout":
					<-r.Context().Done()
					return
				case "failure":
					http.Error(w, "unavailable", 503)
					return
				case "unknown":
					fmt.Fprint(w, `{"id":123,"infobox":[]}`)
					return
				case "changed-binding":
					db.Exec(`UPDATE anime_meta SET subject_id=456`)
				case "manual-edit":
					var updatedAt string
					if err := db.QueryRow("SELECT updated_at FROM animes WHERE id=?", id).Scan(&updatedAt); err != nil {
						t.Error(err)
						return
					}
					putAnime(t, id, `{"update_weekday":4}`)
					// Force equal timestamps to cover edits within one clock tick.
					if _, err := db.Exec("UPDATE animes SET updated_at=? WHERE id=?", updatedAt, id); err != nil {
						t.Error(err)
						return
					}
				case "wrong-id":
					fmt.Fprint(w, `{"id":456,"infobox":[{"key":"放送星期","value":"周日"}]}`)
					return
				}
				fmt.Fprint(w, `{"id":123,"infobox":[{"key":"放送星期","value":"周日"}]}`)
			})
			if scenario == "timeout" {
				metaClient.Timeout = 25 * time.Millisecond
			}
			refreshBroadcastWeekday("自动周更测试")
			var day int
			db.QueryRow("SELECT broadcast_weekday FROM anime_meta").Scan(&day)
			want := 3
			if scenario == "unknown" {
				want = 0
				refreshBroadcastWeekday("自动周更测试")
				if hits != 1 {
					t.Fatal("unknown weekday should be cached")
				}
			}
			if day != want {
				t.Fatalf("got %d want %d", day, want)
			}
			if scenario == "changed-binding" {
				r := httptest.NewRecorder()
				animeDetail(r, id)
				if !strings.Contains(r.Body.String(), `"broadcast_weekday":0`) {
					t.Fatalf("old subject leaked: %s", r.Body.String())
				}
			}
			if scenario == "manual-edit" {
				a, _ := getAnime(id)
				if a.UpdateWeekday != 4 {
					t.Fatal("manual weekday overwritten")
				}
			}
		})
	}
}

func TestBroadcastCalendarFallback(t *testing.T) {
	testDatabase(t)
	withMetaTestEnv(t)
	seedBroadcastAnime(t)
	calendarHits := 0
	scheduleTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/calendar":
			calendarHits++
			fmt.Fprint(w, `[{"weekday":{"id":4},"items":[{"id":123},{"id":9}]},{"weekday":{"id":5},"items":[{"id":9}]}]`)
		case "/v0/subjects/123":
			// 多日字段不可靠，退回放送表。
			fmt.Fprint(w, `{"id":123,"infobox":[{"key":"放送星期","value":"星期四、星期日"}]}`)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
	})
	calendarWeekdayLookup = calendarWeekday
	bangumiCalendarCache.api = ""
	refreshBroadcastWeekday("自动周更测试")
	var day int
	db.QueryRow("SELECT broadcast_weekday FROM anime_meta").Scan(&day)
	if day != 4 {
		t.Fatalf("calendar fallback: got %d", day)
	}
	if calendarWeekday(context.Background(), 9) != 0 {
		t.Fatal("subject listed on several days must stay unknown")
	}
	if calendarHits != 1 {
		t.Fatalf("calendar not cached: %d", calendarHits)
	}
}

func TestUnknownBroadcastRetriedDaily(t *testing.T) {
	now := time.Now().UTC()
	if !broadcastWeekdayFresh(now.Add(-2*time.Hour).Format(time.RFC3339), 0) || broadcastWeekdayFresh(now.Add(-25*time.Hour).Format(time.RFC3339), 0) {
		t.Fatal("unknown weekday should be retried after a day")
	}
	if !broadcastWeekdayFresh(now.Add(-3*24*time.Hour).Format(time.RFC3339), 3) || broadcastWeekdayFresh(now.Add(-8*24*time.Hour).Format(time.RFC3339), 3) {
		t.Fatal("known weekday should be kept for a week")
	}
}

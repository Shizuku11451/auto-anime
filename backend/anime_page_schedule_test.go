package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnimesPageScheduleFieldsDayFilterAndCounts(t *testing.T) {
	testDatabase(t)
	// title, manual update day, bound Bangumi ID, metadata subject, broadcast subject, weekday, end
	fixtures := []struct {
		title                                string
		manual, bound, subject, broadcastFor int
		weekday                              int
		end                                  string
		want                                 int
	}{
		{"手动周三", 3, 0, 0, 0, 0, "", 3},
		{"放送周五", 0, 0, 10, 10, 5, "", 5},
		{"放送资料不属于该条目", 0, 0, 11, 12, 5, "", 0},
		{"已完结", 0, 0, 13, 13, 2, "2020-01-01", 8},
		{"手动优先于完结", 2, 0, 14, 14, 6, "2020-01-01", 2},
		{"无资料", 0, 0, 0, 0, 0, "", 0},
		{"绑定了其他条目", 0, 99, 15, 15, 1, "", 0},
		{"未来完结", 0, 0, 16, 16, 4, "2030-01-01", 4},
	}
	for _, f := range fixtures {
		if _, err := db.Exec("INSERT INTO animes(title,update_weekday,bangumi_id) VALUES(?,?,?)", f.title, f.manual, f.bound); err != nil {
			t.Fatal(err)
		}
		if f.subject > 0 {
			if _, err := db.Exec(`INSERT INTO anime_meta(anime_title,subject_id,source,broadcast_weekday,broadcast_end,broadcast_subject_id) VALUES(?,?,'bangumi',?,?,?)`,
				f.title, f.subject, f.weekday, f.end, f.broadcastFor); err != nil {
				t.Fatal(err)
			}
		}
	}
	type page struct {
		Items []struct {
			Title            string `json:"title"`
			UpdateWeekday    int    `json:"update_weekday"`
			BroadcastWeekday int    `json:"broadcast_weekday"`
			BroadcastEnd     string `json:"broadcast_end"`
		} `json:"items"`
		DayCounts []int `json:"day_counts"`
	}
	get := func(query string) (int, page) {
		rec := httptest.NewRecorder()
		animesAPI(rec, httptest.NewRequest("GET", "/api/animes?page=1&limit=100"+query, nil))
		var p page
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
				t.Fatalf("%s: %v %s", query, err, rec.Body.String())
			}
		}
		return rec.Code, p
	}

	_, all := get("&today=2026-11-03&counts=1")
	if len(all.Items) != len(fixtures) {
		t.Fatalf("unfiltered page returned %d rows", len(all.Items))
	}
	want := make([]int, 9)
	for _, f := range fixtures {
		want[f.want]++
	}
	if len(all.DayCounts) != 9 {
		t.Fatalf("day_counts: %v", all.DayCounts)
	}
	for d := range want {
		if all.DayCounts[d] != want[d] {
			t.Fatalf("day_counts = %v, want %v", all.DayCounts, want)
		}
	}
	for _, item := range all.Items {
		if item.Title == "放送资料不属于该条目" && item.BroadcastWeekday != 0 {
			t.Fatal("broadcast data of another subject leaked into the card")
		}
		if item.Title == "已完结" && (item.BroadcastWeekday != 2 || item.BroadcastEnd != "2020-01-01") {
			t.Fatalf("schedule fields missing: %+v", item)
		}
		if item.Title == "手动周三" && item.UpdateWeekday != 3 {
			t.Fatalf("manual update day missing: %+v", item)
		}
	}
	for _, f := range fixtures {
		_, p := get("&today=2026-11-03&day=" + string(rune('0'+f.want)))
		found := false
		for _, item := range p.Items {
			found = found || item.Title == f.title
		}
		if !found {
			t.Fatalf("%s not listed under day %d", f.title, f.want)
		}
	}
	// The client's calendar day decides "finished".
	if _, p := get("&today=2031-01-01&day=8"); len(p.Items) != 2 {
		t.Fatalf("after 2030-01-01 two anime are finished, got %+v", p.Items)
	}
	if _, p := get("&today=2026-11-03"); p.DayCounts != nil {
		t.Fatal("day_counts is opt-in")
	}
	for _, bad := range []string{"&day=9", "&day=-1", "&day=x", "&today=2026-13-01", "&today=yesterday"} {
		if code, _ := get(bad); code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d", bad, code)
		}
	}
}

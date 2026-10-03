package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnimeUpdateWeekday(t *testing.T) {
	testDatabase(t)
	create := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRecorder()
		animesAPI(r, httptest.NewRequest("POST", "/api/animes", strings.NewReader(body)))
		return r
	}
	for _, value := range []string{"-1", "8", "1.5", `"周一"`} {
		if r := create(`{"title":"无效","update_weekday":` + value + `}`); r.Code != 400 {
			t.Fatalf("invalid weekday accepted: %s: %d", value, r.Code)
		}
	}
	r := create(`{"title":"每周追番","update_weekday":7}`)
	if r.Code != 201 {
		t.Fatalf("create: %d %s", r.Code, r.Body.String())
	}
	id := animeIDByTitle(t, "每周追番")
	for _, tc := range []struct {
		body         string
		status, want int
	}{
		{`{"season":2}`, 200, 7},
		{`{"update_weekday":1}`, 200, 1},
		{`{"title":"不应改名","update_weekday":8}`, 400, 1},
		{`{"update_weekday":0}`, 200, 0},
	} {
		if r := putAnime(t, id, tc.body); r.Code != tc.status {
			t.Fatalf("update: %d %s", r.Code, r.Body.String())
		}
		a, err := getAnime(id)
		if err != nil || a.UpdateWeekday != tc.want || a.Title != "每周追番" {
			t.Fatalf("stored: %+v %v", a, err)
		}
	}
	putAnime(t, id, `{"update_weekday":6}`)
	list := httptest.NewRecorder()
	animesAPI(list, httptest.NewRequest("GET", "/api/animes", nil))
	var cards []animeRow
	if err := json.Unmarshal(list.Body.Bytes(), &cards); err != nil || len(cards) != 1 || cards[0].UpdateWeekday != 6 {
		t.Fatalf("list: %s %v", list.Body.String(), err)
	}
	detail := httptest.NewRecorder()
	animeDetail(detail, id)
	var a animeRow
	if err := json.Unmarshal(detail.Body.Bytes(), &a); err != nil || a.UpdateWeekday != 6 {
		t.Fatalf("detail: %s %v", detail.Body.String(), err)
	}

	exported := httptest.NewRecorder()
	backup(exported, httptest.NewRequest("GET", "/api/backup", nil))
	if exported.Code != 200 {
		t.Fatalf("export: %s", exported.Body.String())
	}
	putAnime(t, id, `{"update_weekday":0}`)
	restored := httptest.NewRecorder()
	restore(restored, httptest.NewRequest("POST", "/api/restore", strings.NewReader(exported.Body.String())))
	after, err := getAnime(id)
	if restored.Code != 200 || err != nil || after.UpdateWeekday != 6 {
		t.Fatalf("restore: %s %+v %v", restored.Body.String(), after, err)
	}
}

func TestAnimeWeekdayLegacyMigration(t *testing.T) {
	testDatabase(t)
	id, err := ensureAnime("旧番剧")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("ALTER TABLE animes DROP COLUMN update_weekday"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	initDB()
	a, err := getAnime(id)
	if err != nil || a.UpdateWeekday != 0 {
		t.Fatalf("migration: %+v %v", a, err)
	}
	putAnime(t, id, `{"update_weekday":3}`)
	db.Close()
	initDB()
	a, err = getAnime(id)
	if err != nil || a.UpdateWeekday != 3 {
		t.Fatalf("restart: %+v %v", a, err)
	}
}

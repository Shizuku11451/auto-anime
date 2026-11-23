package main

import (
	"net/http/httptest"
	"testing"
)

func TestAnimeRuleUsesChineseVariantMatching(t *testing.T) {
	testDatabase(t)
	if _, err := db.Exec(`INSERT INTO animes(id,title,include_rules,exclude_rules) VALUES(1,'无职转生','无职转生,[绿茶字幕组],繁日','广告')`); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		title string
		id    int64
	}{
		{"[綠茶字幕組] 無職轉生 [1080p][繁日]", 1},
		{"[綠茶字幕組] 無職轉生 [1080p][繁日] 廣告", 0},
		{"[綠茶字幕組] 無職轉生 [1080p][簡日]", 0},
	} {
		if id, _ := matchAnimeRule(tc.title); id != tc.id {
			t.Fatalf("matchAnimeRule(%q)=%d want=%d", tc.title, id, tc.id)
		}
	}
}

func TestApplyRulesKeepsItemsClaimedByMoreSpecificRule(t *testing.T) {
	testDatabase(t)
	if _, err := db.Exec(`INSERT INTO animes(id,title,include_rules) VALUES(1,'星海','星海'),(2,'星海 第二季','星海,第二季')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO items(id,title,link,anime_id,anime_title) VALUES
		(1,'星海 第二季 01','l1',2,'星海 第二季'),(2,'星海 05','l2',0,'')`); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	animeApplyRules(rec, 1)
	if rec.Code != 200 {
		t.Fatalf("apply-rules: %d %s", rec.Code, rec.Body.String())
	}
	for item, want := range map[int]int64{1: 2, 2: 1} {
		var owner int64
		if err := db.QueryRow("SELECT anime_id FROM items WHERE id=?", item).Scan(&owner); err != nil || owner != want {
			t.Fatalf("item %d owner=%d err=%v want %d", item, owner, err, want)
		}
	}
}

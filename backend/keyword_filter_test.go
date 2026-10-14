package main

import "testing"

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

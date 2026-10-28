package main

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

// 列表返回资料实际对应的 Bangumi 条目，供番剧库与收藏合并；手动绑定冲突或非 Bangumi 来源时不返回。
func TestAnimeListMetaSubjectID(t *testing.T) {
	testDatabase(t)
	for _, title := range []string{"自动识别", "手动一致", "手动冲突", "本地资料", "无资料"} {
		if _, err := ensureAnime(title); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`UPDATE animes SET bangumi_id=CASE title WHEN '手动一致' THEN 202 WHEN '手动冲突' THEN 303 ELSE 0 END`); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		title, source string
		subject       int64
	}{{"自动识别", "bangumi", 101}, {"手动一致", "manual", 202}, {"手动冲突", "bangumi", 999}, {"本地资料", "nfo", 404}} {
		if _, err := db.Exec(`INSERT INTO anime_meta(anime_title,subject_id,source) VALUES(?,?,?)`, row.title, row.subject, row.source); err != nil {
			t.Fatal(err)
		}
	}
	rec := httptest.NewRecorder()
	animesAPI(rec, httptest.NewRequest("GET", "/api/animes", nil))
	var list []struct {
		Title     string `json:"title"`
		BangumiID int64  `json:"bangumi_id"`
		Subject   int64  `json:"meta_subject_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	want := map[string][2]int64{"自动识别": {0, 101}, "手动一致": {202, 202}, "手动冲突": {303, 0}, "本地资料": {0, 0}, "无资料": {0, 0}}
	if len(list) != len(want) {
		t.Fatalf("unexpected list: %s", rec.Body.String())
	}
	for _, a := range list {
		if got := [2]int64{a.BangumiID, a.Subject}; got != want[a.Title] {
			t.Errorf("%s: got bangumi_id/meta_subject_id %v want %v", a.Title, got, want[a.Title])
		}
	}
}

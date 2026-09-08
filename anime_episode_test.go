package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
)

func TestEpisodeIdentityGroupsPaddingAndCountsSeasons(t *testing.T) {
	testDatabase(t)
	a, err := ensureAnime("Series")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE animes SET season=2 WHERE id=?", a); err != nil {
		t.Fatal(err)
	}
	for i, it := range []struct {
		ep     string
		season int
	}{{"7", 0}, {"07", 2}, {"007", 2}, {"07", 1}, {"SP01", 2}, {"SP1", 2}, {"", 2}} {
		if _, err := db.Exec("INSERT INTO items(title,link,anime_title,anime_id,episode,season) VALUES(?,?,?,?,?,?)", "Series", fmt.Sprintf("https://example.test/%d", i), "Series", a, it.ep, it.season); err != nil {
			t.Fatal(err)
		}
	}
	rec := httptest.NewRecorder()
	animeDetail(rec, a)
	var detail struct {
		Seasons []seasonGroup `json:"seasons"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.Seasons) != 2 || len(detail.Seasons[0].Episodes) != 1 || len(detail.Seasons[1].Episodes) != 3 || len(detail.Seasons[1].Loose) != 1 {
		t.Fatalf("wrong grouping: %s", rec.Body.String())
	}
	versions := detail.Seasons[1].Episodes[0].Items
	if len(versions) != 3 {
		t.Fatal("numeric padding split one episode")
	}
	raw := map[string]bool{}
	for _, item := range versions {
		raw[item.Episode] = true
	}
	if !raw["7"] || !raw["07"] || !raw["007"] {
		t.Fatal("original resource numbering lost")
	}
	rec = httptest.NewRecorder()
	animesAPI(rec, httptest.NewRequest("GET", "/api/animes", nil))
	var list []struct {
		Count int `json:"episode_count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Count != 4 {
		t.Fatalf("list does not match grouped season/episode count: %s", rec.Body.String())
	}
}

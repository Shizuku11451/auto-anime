package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestInferDiscoverSeasons(t *testing.T) {
	const feed = "https://example.test/rss"
	item := func(group, episode string, season int) discoverItem {
		return discoverItem{AnimeTitle: "与你相恋到生命尽头", Group: group, Episode: episode, Season: season, FeedURL: feed}
	}
	for _, tc := range []struct {
		name string
		edit func([]discoverItem)
		want int
	}{
		{"matching episodes across groups", func(items []discoverItem) {}, 1},
		{"competing seasons", func(items []discoverItem) { items[1].Season = 2 }, 0},
		{"conflicting title", func(items []discoverItem) { items[1].Season = -1 }, 0},
		{"different RSS", func(items []discoverItem) { items[1].FeedURL += "?other" }, 0},
		{"different name", func(items []discoverItem) { items[1].AnimeTitle += "续篇" }, 0},
		{"only one overlap", func(items []discoverItem) { items[1].Episode = "03" }, 0},
		{"duplicate episodes", func(items []discoverItem) { items[1].Episode = "1" }, 0},
		{"unmarked only", func(items []discoverItem) { items[0].Season, items[1].Season = 0, 0 }, 0},
		{"different unmarked versions", func(items []discoverItem) { items[3].Quality = "720P" }, 0},
		{"missing RSS identity", func(items []discoverItem) {
			for i := range items {
				items[i].FeedURL = ""
			}
		}, 0},
		{"part is not season", func(items []discoverItem) {
			items[2].AnimeTitle += " Part 2"
			items[3].AnimeTitle += " Part 2"
		}, 0},
		{"second season hint", func(items []discoverItem) { items[0].Season, items[1].Season = 2, 2 }, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			items := []discoverItem{item("Nix-Raws", "01", 1), item("Nix-Raws", "02", 1), item("绿茶字幕组", "1", 0), item("绿茶字幕组", "02", 0)}
			tc.edit(items)
			// Stale hints must not become evidence when recalculating.
			items[2].InferredSeason = 3
			inferDiscoverSeasons(items)
			if items[2].Season != 0 || items[2].InferredSeason != tc.want {
				t.Fatalf("hint: %+v want=%d", items[2], tc.want)
			}
		})
	}
}

func TestDiscoverSeasonHintsPreserveEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<rss><channel>`)
		for i, title := range []string{
			"[Nix-Raws] 与你相恋到生命尽头 / Kimi ga Shinu made Koi wo Shitai S01E01 [1080p]",
			"[Nix-Raws] 与你相恋到生命尽头 / Kimi ga Shinu made Koi wo Shitai S01E02 [1080p]",
			"[绿茶字幕组] 与你相恋到生命尽头 / Kimi ga Shinu made Koi wo Shitai [01][1080p]",
			"[绿茶字幕组] 与你相恋到生命尽头 / Kimi ga Shinu made Koi wo Shitai [02][1080p]",
		} {
			fmt.Fprintf(w, `<item><title>%s</title><link>/item/%d</link></item>`, title, i)
		}
		fmt.Fprint(w, `</channel></rss>`)
	}))
	defer server.Close()
	overrideDiscoverSource(t, server.URL+"/?term={keyword}")
	rec := discoverGet(t, "/api/discover/search?source=acgrip&keyword="+url.QueryEscape("与你相恋"))
	var out struct{ Items []discoverItem }
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || rec.Code != 200 || len(out.Items) != 4 {
		t.Fatalf("search: %s err=%v", rec.Body.String(), err)
	}
	for _, item := range out.Items[2:] {
		if item.Season != 0 || item.InferredSeason != 1 || item.Query != "与你相恋" {
			t.Fatalf("lost evidence/provenance: %+v", item)
		}
	}
}

func TestExpandedDiscoverSeasonHintsBeforeDeduplication(t *testing.T) {
	for _, earlyUnknown := range []bool{false, true} {
		t.Run(fmt.Sprintf("earlyUnknown=%t", earlyUnknown), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				query := r.URL.Query().Get("term")
				fmt.Fprint(w, `<rss><channel>`)
				if query == "测试动画 S2" || query == "测试动画 S02" || query == "测试动画" {
					for i := 1; i <= 2; i++ {
						fmt.Fprintf(w, `<item><title>[Known] 测试动画 S02E0%d [1080p]</title><link>/known/%d</link></item>`, i, i)
					}
				}
				if query == "测试动画" || (earlyUnknown && query == "测试动画 第二季") {
					for i := 1; i <= 2; i++ {
						fmt.Fprintf(w, `<item><title>[Unknown] 测试动画 [0%d][1080p]</title><link>/unknown/%d</link></item>`, i, i)
					}
				}
				fmt.Fprint(w, `</channel></rss>`)
			}))
			defer server.Close()
			overrideDiscoverSource(t, server.URL+"/?term={keyword}")
			for _, keyword := range []string{"测试动画", "测试动画 第二季"} {
				rec := discoverGet(t, "/api/discover/search?source=acgrip&keyword="+url.QueryEscape(keyword))
				var out struct{ Items []discoverItem }
				if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || rec.Code != 200 || len(out.Items) != 4 {
					t.Fatalf("search: %s err=%v", rec.Body.String(), err)
				}
				unknown := 0
				for _, item := range out.Items {
					if item.Group != "Unknown" {
						continue
					}
					unknown++
					if item.Season != 0 || item.InferredSeason != 2 || item.Query != "测试动画" || item.FeedURL != server.URL+"/?term="+url.QueryEscape("测试动画") {
						t.Fatalf("keyword=%q lost same-RSS evidence/provenance: %+v", keyword, item)
					}
				}
				if unknown != 2 {
					t.Fatalf("unknown versions=%d want=2", unknown)
				}
			}
		})
	}
}

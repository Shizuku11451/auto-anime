package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFeedKeepsPageAndDownloadURLs(t *testing.T) {
	for _, data := range []string{
		`<rss><channel><item><title>测试动画</title><link>https://mikanani.me/Home/Episode/abc123</link><enclosure url="https://mikanani.me/Download/abc.torrent"/></item></channel></rss>`,
		`<feed xmlns="http://www.w3.org/2005/Atom"><entry><title>测试动画</title><link rel="enclosure" href="https://mikanani.me/Download/abc.torrent"/><link rel="alternate" href="https://mikanani.me/Home/Episode/abc123"/></entry></feed>`,
	} {
		page, err := decodeFeedPage(strings.NewReader(data))
		items := feedItemsFromEntries(page.Entries)
		if err != nil || len(items) != 1 || items[0].PageURL != mikanTestEpisode || items[0].Link != "https://mikanani.me/Download/abc.torrent" {
			t.Fatalf("page/download separation: %+v %v", items, err)
		}
	}
}

func TestSyncBackfillsPageWithoutChangingExistingTask(t *testing.T) {
	testDatabase(t)
	withMetaTestEnv(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<rss><channel><item><title>[字幕组] 测试动画 [01]</title><link>%s</link><enclosure url="https://mikanani.me/Download/abc.torrent"/></item></channel></rss>`, mikanTestEpisode)
	}))
	defer server.Close()
	if _, err := db.Exec(`INSERT INTO feeds(id,name,url,target_title) VALUES(1,'测试',?,'测试动画')`, server.URL); err != nil {
		t.Fatal(err)
	}
	if n, err := syncFeedContext(context.Background(), 1); n != 1 || err != nil {
		t.Fatalf("first sync: %d %v", n, err)
	}
	var page string
	if err := db.QueryRow(`SELECT page_url FROM items`).Scan(&page); err != nil || page != mikanTestEpisode {
		t.Fatalf("resource page was not saved: %q %v", page, err)
	}
	if _, err := db.Exec(`UPDATE items SET page_url='',manual=1,status='已暂停',downloader='qbittorrent',task_id='preserve-task',progress=47`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO anime_meta(anime_title,source,updated_at,match_key) VALUES('测试动画','none',?,?)`, time.Now().UTC().Format(time.RFC3339), metaLookupKey("测试动画", "")); err != nil {
		t.Fatal(err)
	}
	// Even a feed with a newly corrected target must enrich, not reassign, an
	// already known resource. No empty target series should be created.
	if _, err := db.Exec(`UPDATE feeds SET target_title='不要创建新番剧'`); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 2; n++ {
		if added, err := syncFeedContext(context.Background(), 1); added != 0 || err != nil {
			t.Fatalf("backfill: %d %v", added, err)
		}
	}
	var title, status, downloader, task string
	var manual, progress int
	if err := db.QueryRow(`SELECT page_url,anime_title,status,downloader,task_id,manual,progress FROM items`).Scan(&page, &title, &status, &downloader, &task, &manual, &progress); err != nil {
		t.Fatal(err)
	}
	if page != mikanTestEpisode || title != "测试动画" || status != "已暂停" || downloader != "qbittorrent" || task != "preserve-task" || manual != 1 || progress != 47 {
		t.Fatalf("backfill changed existing task: %q %q %q %q %q %d %d", page, title, status, downloader, task, manual, progress)
	}
	var series, misses int
	if err := db.QueryRow(`SELECT COUNT(*) FROM animes`).Scan(&series); err != nil || series != 1 {
		t.Fatalf("new target was incorrectly created: %d %v", series, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM anime_meta WHERE source='none'`).Scan(&misses); err != nil || misses != 0 {
		t.Fatalf("page backfill kept an obsolete negative: %d %v", misses, err)
	}
}

func TestPageBackfillRollsBackAndPreservesCorrections(t *testing.T) {
	for _, kind := range []string{"database", "search-name", "manual"} {
		t.Run(kind, func(t *testing.T) {
			testDatabase(t)
			id := seedRematchAnime(t, "none", 0)
			if _, err := db.Exec(`INSERT INTO items(link,anime_title,anime_id) VALUES('resource','测试动画',?)`, id); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "database":
				if _, err := db.Exec(`CREATE TRIGGER fail_page BEFORE DELETE ON anime_meta BEGIN SELECT RAISE(ABORT,'test rollback'); END`); err != nil {
					t.Fatal(err)
				}
			case "search-name":
				if _, err := db.Exec(`UPDATE animes SET search_name='用户纠正' WHERE id=?`, id); err != nil {
					t.Fatal(err)
				}
			case "manual":
				if _, err := db.Exec(`UPDATE anime_meta SET source='manual'`); err != nil {
					t.Fatal(err)
				}
			}
			err := backfillItemPageURL("resource", mikanTestEpisode)
			if (err != nil) != (kind == "database") {
				t.Fatalf("backfill result: %v", err)
			}
			var page, updated string
			var count int
			if err := db.QueryRow(`SELECT page_url,animes.updated_at FROM items JOIN animes ON animes.id=items.anime_id`).Scan(&page, &updated); err != nil {
				t.Fatal(err)
			}
			if kind == "database" && page != "" {
				t.Fatalf("failed transaction saved page: %q", page)
			}
			if err := db.QueryRow(`SELECT COUNT(*) FROM anime_meta`).Scan(&count); err != nil || count != 1 || updated != "2026-01-01T00:00:00Z" {
				t.Fatalf("backfill changed correction/cache: count=%d updated=%q err=%v", count, updated, err)
			}
		})
	}
}

func TestPageURLMigrationPreservesLegacyItems(t *testing.T) {
	testDatabase(t)
	if _, err := db.Exec(`INSERT INTO items(title,link) VALUES('旧条目','https://example.test/old.torrent')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`ALTER TABLE items DROP COLUMN page_url`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	initDB()
	var title, page string
	if err := db.QueryRow(`SELECT title,page_url FROM items`).Scan(&title, &page); err != nil || title != "旧条目" || page != "" {
		t.Fatalf("migration lost legacy row: %q %q %v", title, page, err)
	}
}

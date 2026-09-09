package main

import (
	"log"
	"sync"
	"time"
)

// Prevent cleanup between creating a series and attaching its first item or
// saving its configuration. The existing SQLite pool serializes SQL, but not
// these multi-statement workflows.
var animeCleanupMu sync.RWMutex

func cleanupEmptyAnime() (int, error) {
	if getSetting("auto_cleanup_empty_anime") != "1" {
		return 0, nil
	}
	animeCleanupMu.Lock()
	defer animeCleanupMu.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`DELETE FROM animes
		WHERE EXISTS (SELECT 1 FROM settings WHERE key='auto_cleanup_empty_anime' AND value='1')
		AND NOT EXISTS (SELECT 1 FROM items WHERE items.anime_id=animes.id OR items.anime_title=animes.title)
		AND NOT EXISTS (SELECT 1 FROM feeds WHERE feeds.target_title=animes.title)
		AND NOT EXISTS (SELECT 1 FROM anime_meta WHERE anime_meta.anime_title=animes.title AND source='manual')
		AND NOT EXISTS (SELECT 1 FROM anime_storage WHERE anime_storage.anime_title=animes.title)
		AND COALESCE(bangumi_id,0)=0 AND COALESCE(season,0)=0
		AND trim(COALESCE(search_name,''))='' AND trim(COALESCE(storage_dir,''))=''
		AND trim(COALESCE(include_rules,''))='' AND trim(COALESCE(exclude_rules,''))=''
		RETURNING title`)
	if err != nil {
		return 0, err
	}
	var titles []string
	for rows.Next() {
		var title string
		if err = rows.Scan(&title); err != nil {
			rows.Close()
			return 0, err
		}
		titles = append(titles, title)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	for _, title := range titles {
		if _, err = tx.Exec("DELETE FROM anime_meta WHERE anime_title=? AND COALESCE(source,'')!='manual'", title); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return len(titles), nil
}

func startAnimeCleanupWorker() {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			if n, err := cleanupEmptyAnime(); err != nil {
				log.Printf("自动清理空番剧失败: %v", err)
			} else if n > 0 {
				log.Printf("自动清理空番剧: %d 条记录", n)
			}
			<-ticker.C
		}
	}()
}

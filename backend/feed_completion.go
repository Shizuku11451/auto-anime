package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	keywordfilter "auto-anime/internal/filter"
	titleparse "auto-anime/internal/title"
)

// The library shares one cover across seasons. Episode totals must not: a
// second-season subscription may coexist with a first-season library binding.
func refreshFeedEpisodeTotals(title string) {
	if !bangumiEnabled {
		return
	}
	expected, err := readMetaLookupState(db, title)
	if err != nil || expected.ID == 0 {
		return
	}
	rows, err := db.Query(`SELECT DISTINCT CASE WHEN i.season>0 THEN i.season WHEN a.season>0 THEN a.season WHEN i.season_auto>0 THEN i.season_auto ELSE 1 END
		FROM items i JOIN feeds f ON f.id=i.feed_id JOIN animes a ON a.title=i.anime_title WHERE i.anime_title=? AND f.enabled=1`, title)
	if err != nil {
		return
	}
	var seasons []int
	for rows.Next() {
		var season int
		if err = rows.Scan(&season); err != nil {
			break
		}
		seasons = append(seasons, season)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return
	}
	for _, season := range seasons {
		key := fmt.Sprintf("%d:%s:%s", expected.BangumiID, expected.SearchName, expected.UpdatedAt)
		var checked string
		db.QueryRow(`SELECT checked_at FROM feed_episode_totals WHERE anime_id=? AND season=? AND lookup_key=?`, expected.ID, season, key).Scan(&checked)
		if broadcastCacheFresh(checked) {
			continue
		}
		// A matching verified library subject already supplies this season.
		var known bool
		db.QueryRow(`SELECT EXISTS(SELECT 1 FROM anime_meta WHERE anime_title=? AND source IN ('bangumi','manual') AND subject_id=episode_subject_id
			AND episode_season=? AND (episode_total>0 OR ?>0) AND (?=0 OR subject_id=?))`, title, season, expected.BangumiID, expected.BangumiID, expected.BangumiID).Scan(&known)
		if known {
			continue
		}
		name := title
		if strings.TrimSpace(expected.SearchName) != "" {
			name = expected.SearchName
		}
		base := titleparse.StandardSeriesName(name, "")
		ctx, cancel := context.WithTimeout(context.Background(), metaLookupTimeout)
		subject, e := searchBangumiAliases(ctx, []string{fmt.Sprintf("%s S%d", base, season)})
		if e == nil && subject != nil {
			id := subject.ID
			subject, e = fetchBangumiSubjectContext(ctx, int64(id))
			if e == nil && subject.ID != id {
				e = fmt.Errorf("Bangumi 返回了不同的条目 ID")
			}
		}
		cancel()
		if e != nil {
			log.Printf("订阅总集数获取失败 %q 第 %d 季: %v", title, season, e)
			continue
		}
		total, subjectID := 0, 0
		if subject != nil {
			if n, s := subjectEpisodes(subject); s == season {
				total, subjectID = n, subject.ID
			}
		}
		_, e = writeMetaResult(title, &expected, `INSERT INTO feed_episode_totals(anime_id,season,lookup_key,subject_id,total,checked_at) VALUES(?,?,?,?,?,?)
			ON CONFLICT(anime_id,season) DO UPDATE SET lookup_key=excluded.lookup_key,subject_id=excluded.subject_id,total=excluded.total,checked_at=excluded.checked_at`, expected.ID, season, key, subjectID, total, time.Now().UTC().Format(time.RFC3339))
		if e != nil {
			log.Printf("订阅总集数保存失败: %v", e)
		}
	}
	wakeTaskPoller()
}

// Use the wiki's episode count, not the number of episode records in Bangumi.
func subjectEpisodes(subject *bangumiSubject) (int, int) {
	season := titleparse.ResourceSeason(subject.NameCN + " " + subject.Name)
	if season == 0 {
		season = 1
	}
	if season < 1 {
		return 0, 0
	}
	if subject.Episodes < 1 || subject.Episodes > 9999 {
		return 0, season
	}
	return subject.Episodes, season
}

func cacheSubjectEpisodes(title string, subject *bangumiSubject) error {
	if !subject.DetailsFetched {
		return nil
	}
	total, season := subjectEpisodes(subject)
	_, err := db.Exec(`UPDATE anime_meta SET episode_total=?,episode_season=?,episode_subject_id=?
		WHERE anime_title=? AND subject_id=? AND source IN ('bangumi','manual')`, total, season, subject.ID, title, subject.ID)
	if err == nil {
		wakeTaskPoller()
	}
	return err
}

func processCompletedFeeds() {
	if err := completeFeeds(); err != nil {
		log.Printf("订阅集齐检查失败: %v", err)
	}
}

func completeFeeds() error {
	// One snapshot prevents a concurrent correction/rebind from disabling the
	// subscription using obsolete episode ownership or metadata.
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT id FROM feeds WHERE enabled=1`)
	if err != nil {
		return err
	}
	var ids []int
	for rows.Next() {
		var id int
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := completeFeed(tx, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func completeFeed(tx *sql.Tx, id int) error {
	var target, include, exclude string
	var filterSeason int
	var unknown bool
	if err := tx.QueryRow(`SELECT COALESCE(target_title,''),COALESCE(include_words,''),COALESCE(exclude_words,''),COALESCE(season_filter,0),COALESCE(allow_unknown_season,0) FROM feeds WHERE id=?`, id).Scan(&target, &include, &exclude, &filterSeason, &unknown); err != nil {
		return err
	}
	filter := keywordfilter.New(include, exclude)
	rows, err := tx.Query(`SELECT i.anime_title,i.title,
		CASE WHEN i.season>0 THEN i.season WHEN a.season>0 THEN a.season WHEN i.season_auto>0 THEN i.season_auto ELSE 1 END
		FROM items i LEFT JOIN animes a ON a.title=i.anime_title WHERE i.feed_id=?`, id)
	if err != nil {
		return err
	}
	title, season := "", 0
	groups := make(map[string]bool)
	for rows.Next() {
		var raw, currentTitle string
		var currentSeason int
		if err = rows.Scan(&currentTitle, &raw, &currentSeason); err != nil {
			break
		}
		if !filter.Allows(raw) || !titleparse.AllowsSeason(raw, filterSeason, unknown) || (target != "" && target != currentTitle) {
			continue
		}
		title, season = currentTitle, currentSeason
		groups[fmt.Sprintf("%s:%d", title, season)] = true
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil || len(groups) != 1 || title == "" {
		return err
	}
	var total, metaSeason int
	err = tx.QueryRow(`SELECT m.episode_total,m.episode_season FROM anime_meta m JOIN animes a ON a.title=m.anime_title
		WHERE m.anime_title=? AND m.source IN ('bangumi','manual') AND m.subject_id>0 AND m.episode_subject_id=m.subject_id
		AND (COALESCE(a.bangumi_id,0)=0 OR a.bangumi_id=m.subject_id)`, title).Scan(&total, &metaSeason)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if total < 1 || season != metaSeason {
		err = tx.QueryRow(`SELECT t.total FROM feed_episode_totals t JOIN animes a ON a.id=t.anime_id WHERE a.title=? AND t.season=?
			AND t.lookup_key=CAST(COALESCE(a.bangumi_id,0) AS TEXT)||':'||COALESCE(a.search_name,'')||':'||COALESCE(a.updated_at,'') AND t.subject_id>0`, title, season).Scan(&total)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
	}
	if total < 1 || total > 9999 {
		return nil
	}
	rows, err = tx.Query(`SELECT i.episode,COALESCE(i.file_path,'') FROM items i JOIN animes a ON a.title=i.anime_title
		WHERE i.anime_title=? AND i.status='已保存' AND
		CASE WHEN i.season>0 THEN i.season WHEN a.season>0 THEN a.season WHEN i.season_auto>0 THEN i.season_auto ELSE 1 END=?`, title, season)
	if err != nil {
		return err
	}
	saved := make(map[int]bool)
	for rows.Next() {
		var episode, path string
		if err = rows.Scan(&episode, &path); err != nil {
			break
		}
		n, e := strconv.Atoi(strings.TrimSpace(episode))
		if e == nil && n >= 1 && n <= total && !saved[n] {
			if info, e := os.Stat(path); e == nil && info.Mode().IsRegular() && info.Size() > 0 {
				saved[n] = true
			}
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil || len(saved) != total {
		return err
	}
	_, err = tx.Exec(`UPDATE feeds SET enabled=0,completion_note=? WHERE id=? AND enabled=1`, fmt.Sprintf("Bangumi 正篇 %d 集已全部归档，已自动停订（第 %d 季）", total, season), id)
	return err
}

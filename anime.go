package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// 番剧订阅管理：animes 表把番剧、季、集与资源统一成一等实体。
// 订阅条目(items)通过 anime_id 指向番剧，番剧携带季号、存储位置、
// 多来源归并规则（include/exclude 关键词）和 Bangumi 元数据绑定。
// 集与季不单独建表：由条目按 (番剧, 季, 集) 聚合展示，保证与资源
// 记录始终一致。anime_storage 是旧版存储位置表，启动时一次性迁入 animes。

const (
	rulesMaxLength  = 300
	titleMaxLength  = 200
	searchNameLimit = 200
)

type animeRow struct {
	ID           int64  `json:"id"`
	Title        string `json:"title"`
	SearchName   string `json:"search_name"`
	BangumiID    int64  `json:"bangumi_id"`
	Season       int    `json:"season"`
	StorageDir   string `json:"storage_dir"`
	IncludeRules string `json:"include_rules"`
	ExcludeRules string `json:"exclude_rules"`
	UpdatedAt    string `json:"updated_at"`
}

// ensureAnime returns the animes row id for a title, creating it on demand so
// every identified series shows up in the library even without custom rules.
func ensureAnime(title string) (int64, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return 0, nil
	}
	now := time.Now().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO animes(title,created_at,updated_at) VALUES(?,?,?) ON CONFLICT(title) DO NOTHING`, title, now, now); err != nil {
		return 0, err
	}
	var id int64
	err := db.QueryRow("SELECT id FROM animes WHERE title=?", title).Scan(&id)
	return id, err
}

func getAnime(id int64) (*animeRow, error) {
	var a animeRow
	err := db.QueryRow(`SELECT id,title,COALESCE(search_name,''),COALESCE(bangumi_id,0),COALESCE(season,0),
		COALESCE(storage_dir,''),COALESCE(include_rules,''),COALESCE(exclude_rules,''),COALESCE(updated_at,'')
		FROM animes WHERE id=?`, id).
		Scan(&a.ID, &a.Title, &a.SearchName, &a.BangumiID, &a.Season, &a.StorageDir, &a.IncludeRules, &a.ExcludeRules, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// migrateAnimeData unifies legacy data: identified titles become animes rows,
// anime_storage overrides move into animes, and items get their anime_id
// backfilled. The legacy storage table is consumed afterwards so a later
// override cleared in the new UI cannot be resurrected by the next startup.
func migrateAnimeData() error {
	now := time.Now().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT OR IGNORE INTO animes(title,created_at,updated_at)
		SELECT DISTINCT anime_title,?,? FROM items WHERE COALESCE(anime_title,'')!=''`, now, now); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT INTO animes(title,storage_dir,season,updated_at)
		SELECT anime_title,COALESCE(storage_dir,''),COALESCE(season,0),? FROM anime_storage
		WHERE COALESCE(anime_title,'')!=''
		ON CONFLICT(title) DO UPDATE SET
			storage_dir=CASE WHEN COALESCE(animes.storage_dir,'')='' THEN excluded.storage_dir ELSE animes.storage_dir END,
			season=CASE WHEN COALESCE(animes.season,0)=0 THEN excluded.season ELSE animes.season END`, now); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE items SET anime_id=(SELECT id FROM animes WHERE title=items.anime_title)
		WHERE COALESCE(anime_id,0)=0 AND COALESCE(anime_title,'')!=''`); err != nil {
		return err
	}
	_, err := db.Exec("DELETE FROM anime_storage")
	return err
}

// matchAnimeRule finds the anime whose multi-source rules claim a raw RSS
// title. Include keywords are required so an exclude-only rule never grabs
// every item; the longest include list wins as the most specific claim.
func matchAnimeRule(rawTitle string) (int64, string) {
	rows, err := db.Query(`SELECT id,title,include_rules,COALESCE(exclude_rules,'') FROM animes
		WHERE COALESCE(include_rules,'')!='' ORDER BY length(include_rules) DESC, id ASC`)
	if err != nil {
		return 0, ""
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var title, include, exclude string
		if rows.Scan(&id, &title, &include, &exclude) != nil {
			continue
		}
		if allowed(rawTitle, include, exclude) {
			return id, title
		}
	}
	return 0, ""
}

// sanitizeText strips control characters, trims and truncates by runes so a
// long input never splits a Chinese code point.
func sanitizeText(raw string, limit int) string {
	clean := strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 32 {
			return -1
		}
		return r
	}, raw))
	runes := []rune(clean)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return clean
}

func coverPath(coverFile string) string {
	if coverFile == "" {
		return ""
	}
	return "/api/covers/" + coverFile
}

// ---- GET /api/animes：番剧库列表 ----

func animesAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		animeCreate(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "方法不支持", 405)
		return
	}
	rows, err := db.Query(`SELECT animes.id,animes.title,COALESCE(animes.search_name,''),COALESCE(animes.bangumi_id,0),
		COALESCE(animes.season,0),COALESCE(animes.storage_dir,''),COALESCE(animes.include_rules,''),COALESCE(animes.exclude_rules,''),
		COALESCE(anime_meta.cover_file,''),COALESCE(anime_meta.name,''),COALESCE(anime_meta.name_cn,''),
		(SELECT COUNT(*) FROM items WHERE items.anime_id=animes.id),
		(SELECT COUNT(*) FROM items WHERE items.anime_id=animes.id AND items.status='已保存'),
		(SELECT COUNT(DISTINCT
			CAST(CASE WHEN items.season>0 THEN items.season WHEN animes.season>0 THEN animes.season ELSE 1 END AS TEXT) || ':' ||
			CASE WHEN trim(items.episode) NOT GLOB '*[^0-9]*' THEN COALESCE(NULLIF(ltrim(trim(items.episode),'0'),''),'0') ELSE trim(items.episode) END)
		 FROM items WHERE items.anime_id=animes.id AND trim(COALESCE(items.episode,''))!='')
		FROM animes LEFT JOIN anime_meta ON anime_meta.anime_title=animes.title
		ORDER BY COALESCE((SELECT MAX(items.id) FROM items WHERE items.anime_id=animes.id),0) DESC, animes.id DESC`)
	if err != nil {
		http.Error(w, "查询失败", 500)
		return
	}
	defer rows.Close()
	type animeCard struct {
		animeRow
		Cover      string `json:"cover"`
		MetaName   string `json:"meta_name"`
		MetaNameCN string `json:"meta_name_cn"`
		ItemCount  int    `json:"item_count"`
		SavedCount int    `json:"saved_count"`
		Episodes   int    `json:"episode_count"`
	}
	out := []animeCard{}
	for rows.Next() {
		var c animeCard
		var coverFile string
		if err := rows.Scan(&c.ID, &c.Title, &c.SearchName, &c.BangumiID, &c.Season, &c.StorageDir, &c.IncludeRules, &c.ExcludeRules,
			&coverFile, &c.MetaName, &c.MetaNameCN, &c.ItemCount, &c.SavedCount, &c.Episodes); err != nil {
			http.Error(w, "查询失败", 500)
			return
		}
		c.Cover = coverPath(coverFile)
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "查询失败", 500)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	jsonOut(w, out)
}

// ---- /api/animes/{id}[/metadata|/apply-rules] ----

func animeEntryAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		animeCleanupMu.RLock()
		defer animeCleanupMu.RUnlock()
	} else {
		animeCleanupMu.Lock()
		defer animeCleanupMu.Unlock()
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/animes/"), "/")
	if rest == "manage" {
		animeManageAPI(w, r)
		return
	}
	if rest == "" {
		http.NotFound(w, r)
		return
	}
	idText, sub := rest, ""
	if i := strings.Index(rest, "/"); i >= 0 {
		idText, sub = rest[:i], rest[i+1:]
	}
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	switch sub {
	case "":
		switch r.Method {
		case http.MethodGet:
			animeDetail(w, id)
		case http.MethodPut:
			animeUpdate(w, r, id)
		case http.MethodDelete:
			animeDelete(w, id)
		default:
			http.Error(w, "方法不支持", 405)
		}
	case "metadata":
		if r.Method != http.MethodPut {
			http.Error(w, "方法不支持", 405)
			return
		}
		animeMetadataBind(w, r, id)
	case "apply-rules":
		if r.Method != http.MethodPost {
			http.Error(w, "方法不支持", 405)
			return
		}
		animeApplyRules(w, id)
	default:
		http.NotFound(w, r)
	}
}

type resourceItem struct {
	DownloadSpeed int64  `json:"download_speed"`
	Progress      int    `json:"progress"`
	ID            int64  `json:"id"`
	AnimeID       int64  `json:"anime_id"`
	Title         string `json:"title"`
	Status        string `json:"status"`
	Episode       string `json:"episode"`
	Season        int    `json:"season"`
	Quality       string `json:"quality"`
	Group         string `json:"fansub_group"`
	Feed          string `json:"feed_name"`
	FilePath      string `json:"file_path"`
	Cover         string `json:"cover"`
	Link          string `json:"link"`
	Manual        bool   `json:"manual"`
	EpisodeNo     int    `json:"episode_no"`
}

type episodeGroup struct {
	Episode string         `json:"episode"`
	Items   []resourceItem `json:"items"`
}

type seasonGroup struct {
	Season   int            `json:"season"`
	Episodes []episodeGroup `json:"episodes"`
	Loose    []resourceItem `json:"loose"`
}

func animeDetail(w http.ResponseWriter, id int64) {
	a, err := getAnime(id)
	if err != nil {
		http.Error(w, "番剧不存在", 404)
		return
	}
	coverFile, metaName, metaNameCN := "", "", ""
	var rating float64
	var summary, voices string
	db.QueryRow(`SELECT COALESCE(cover_file,''),COALESCE(name,''),COALESCE(name_cn,''),COALESCE(rating,0),COALESCE(summary,''),COALESCE(voices,'[]') FROM anime_meta WHERE anime_title=?`, a.Title).
		Scan(&coverFile, &metaName, &metaNameCN, &rating, &summary, &voices)
	if !json.Valid([]byte(voices)) {
		voices = "[]"
	}
	rows, err := db.Query(`SELECT items.id,COALESCE(items.anime_id,0),COALESCE(items.title,''),COALESCE(items.status,'待下载'),COALESCE(items.episode,''),
		COALESCE(items.season,0),COALESCE(items.quality,''),COALESCE(items.fansub_group,''),COALESCE(feeds.name,''),
		COALESCE(items.file_path,''),COALESCE(items.link,''),COALESCE(items.manual,0),COALESCE(items.progress,0)
		FROM items LEFT JOIN feeds ON feeds.id=items.feed_id
		WHERE items.anime_id=? ORDER BY items.id ASC`, id)
	if err != nil {
		http.Error(w, "查询失败", 500)
		return
	}
	defer rows.Close()
	seasons := map[int]*seasonGroup{}
	sources := map[string]bool{}
	total, saved := 0, 0
	for rows.Next() {
		var it resourceItem
		var manual int
		if err := rows.Scan(&it.ID, &it.AnimeID, &it.Title, &it.Status, &it.Episode, &it.Season, &it.Quality, &it.Group, &it.Feed, &it.FilePath, &it.Link, &manual, &it.Progress); err != nil {
			http.Error(w, "查询失败", 500)
			return
		}
		it.Manual = manual == 1
		it.DownloadSpeed, it.Progress = transferSnapshot(int(it.ID), it.Status, it.Progress)
		it.Cover = coverPath(coverFile)
		if n, err := strconv.Atoi(strings.TrimSpace(it.Episode)); err == nil {
			it.EpisodeNo = n
		}
		if it.Feed != "" {
			sources[it.Feed] = true
		}
		total++
		if it.Status == "已保存" {
			saved++
		}
		season := it.Season
		if season < 1 {
			season = a.Season
		}
		if season < 1 {
			season = 1
		}
		group := seasons[season]
		if group == nil {
			group = &seasonGroup{Season: season, Episodes: []episodeGroup{}, Loose: []resourceItem{}}
			seasons[season] = group
		}
		if strings.TrimSpace(it.Episode) == "" {
			group.Loose = append(group.Loose, it)
			continue
		}
		group.Episodes = append(group.Episodes, episodeGroup{Episode: it.Episode, Items: []resourceItem{it}})
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "查询失败", 500)
		return
	}
	// 合并同集多来源资源并按集数排序，缺号一目了然。
	seasonList := make([]seasonGroup, 0, len(seasons))
	for _, group := range seasons {
		merged := map[string]*episodeGroup{}
		order := []string{}
		for _, ep := range group.Episodes {
			key := episodeKey(ep.Episode)
			if got, ok := merged[key]; ok {
				got.Items = append(got.Items, ep.Items...)
				continue
			}
			merged[key] = &episodeGroup{Episode: ep.Episode, Items: ep.Items}
			order = append(order, key)
		}
		sortEpisodes(order)
		sorted := make([]episodeGroup, 0, len(order))
		for _, key := range order {
			sorted = append(sorted, *merged[key])
		}
		group.Episodes = sorted
		seasonList = append(seasonList, *group)
	}
	sortSeasons(seasonList)
	feeds := make([]string, 0, len(sources))
	for name := range sources {
		feeds = append(feeds, name)
	}
	sortStrings(feeds)
	w.Header().Set("Cache-Control", "no-store")
	jsonOut(w, map[string]any{
		"id": a.ID, "title": a.Title, "search_name": a.SearchName, "bangumi_id": a.BangumiID,
		"season": a.Season, "storage_dir": a.StorageDir, "include_rules": a.IncludeRules, "exclude_rules": a.ExcludeRules,
		"cover": coverPath(coverFile), "meta_name": metaName, "meta_name_cn": metaNameCN,
		"item_count": total, "saved_count": saved, "sources": feeds, "rating": rating, "summary": summary, "voices": json.RawMessage(voices),
		"seasons": seasonList,
	})
}

// Only normalize decimal integer identity. Keep original release numbering on
// each resource, and do not guess how specials or revision strings map to it.
func episodeKey(episode string) string {
	ep := strings.TrimSpace(episode)
	if ep == "" {
		return ""
	}
	for _, r := range ep {
		if r < '0' || r > '9' {
			return ep
		}
	}
	key := strings.TrimLeft(ep, "0")
	if key == "" {
		return "0"
	}
	return key
}

// sortEpisodes orders "2","10","07v2" numerically when possible so episode 10
// never sorts before episode 2.
func sortEpisodes(keys []string) {
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0; j-- {
			a, b := keys[j-1], keys[j]
			an, aerr := strconv.Atoi(strings.TrimSpace(a))
			bn, berr := strconv.Atoi(strings.TrimSpace(b))
			var outOfOrder bool
			switch {
			case aerr == nil && berr == nil:
				outOfOrder = an > bn
			case aerr == nil:
				outOfOrder = false // numeric episodes sort before non-numeric
			case berr == nil:
				outOfOrder = true
			default:
				outOfOrder = a > b
			}
			if !outOfOrder {
				break
			}
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
}

func sortSeasons(list []seasonGroup) {
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && list[j-1].Season > list[j].Season; j-- {
			list[j-1], list[j] = list[j], list[j-1]
		}
	}
}

func sortStrings(list []string) {
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && list[j-1] > list[j]; j-- {
			list[j-1], list[j] = list[j], list[j-1]
		}
	}
}

type animeUpdateInput struct {
	Title        string  `json:"title"`
	SearchName   *string `json:"search_name"`
	Season       *int    `json:"season"`
	StorageDir   *string `json:"storage_dir"`
	IncludeRules *string `json:"include_rules"`
	ExcludeRules *string `json:"exclude_rules"`
}

// animeUpdate edits series-level fields. A rename cascades to items and the
// metadata cache so the cover join and archive layout keep one canonical title.
func animeUpdate(w http.ResponseWriter, r *http.Request, id int64) {
	var in animeUpdateInput
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in) != nil {
		http.Error(w, "请求参数无效", 400)
		return
	}
	a, err := getAnime(id)
	if err != nil {
		http.Error(w, "番剧不存在", 404)
		return
	}
	newTitle := a.Title
	if strings.TrimSpace(in.Title) != "" {
		newTitle = sanitizeText(in.Title, titleMaxLength)
		if newTitle == "" {
			http.Error(w, "番剧名称无效", 400)
			return
		}
	}
	searchName := a.SearchName
	if in.SearchName != nil {
		searchName = sanitizeText(*in.SearchName, searchNameLimit)
	}
	season := a.Season
	if in.Season != nil {
		if *in.Season < 0 || *in.Season > 999 {
			http.Error(w, "季号必须是 0-999（0 表示自动）", 400)
			return
		}
		season = *in.Season
	}
	storageDir := a.StorageDir
	if in.StorageDir != nil {
		clean, err := sanitizeStorageDir(*in.StorageDir)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		storageDir = clean
	}
	include, exclude := a.IncludeRules, a.ExcludeRules
	if in.IncludeRules != nil {
		include = sanitizeText(*in.IncludeRules, rulesMaxLength)
	}
	if in.ExcludeRules != nil {
		exclude = sanitizeText(*in.ExcludeRules, rulesMaxLength)
	}
	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "保存失败", 500)
		return
	}
	defer tx.Rollback()
	if newTitle != a.Title {
		var taken int
		if err := tx.QueryRow("SELECT COUNT(*) FROM animes WHERE title=? AND id!=?", newTitle, id).Scan(&taken); err != nil {
			http.Error(w, "保存失败", 500)
			return
		}
		if taken > 0 {
			http.Error(w, "已存在同名番剧", 409)
			return
		}
		for _, update := range []struct {
			query string
			args  []any
		}{
			{"DELETE FROM anime_meta WHERE anime_title=?", []any{newTitle}},
			{"UPDATE anime_meta SET anime_title=? WHERE anime_title=?", []any{newTitle, a.Title}},
			{"UPDATE animes SET title=? WHERE id=?", []any{newTitle, id}},
			{"UPDATE items SET anime_title=? WHERE anime_id=?", []any{newTitle, id}},
			{"UPDATE feeds SET target_title=? WHERE target_title=?", []any{newTitle, a.Title}},
		} {
			if _, err := tx.Exec(update.query, update.args...); err != nil {
				http.Error(w, "保存失败，操作已回滚", 500)
				return
			}
		}
	}
	if _, err := tx.Exec(`UPDATE animes SET search_name=?,season=?,storage_dir=?,include_rules=?,exclude_rules=?,updated_at=? WHERE id=?`,
		searchName, season, storageDir, include, exclude, time.Now().UTC().Format(time.RFC3339), id); err != nil {
		http.Error(w, "保存失败", 500)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "保存失败", 500)
		return
	}
	updated, err := getAnime(id)
	if err != nil {
		http.Error(w, "保存失败", 500)
		return
	}
	writeTvshowNFO(updated)
	w.Header().Set("Cache-Control", "no-store")
	jsonOut(w, updated)
}

func animeDelete(w http.ResponseWriter, id int64) {
	result, status, err := manageAnime(animeManageInput{Action: "delete", IDs: []int64{id}})
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	jsonOut(w, result)
}

// animeMetadataBind 手动绑定/解绑 Bangumi 条目。绑定立即拉取元数据并刷新封面，
// 解绑后清空本地缓存让自动识别重新尝试。
func animeMetadataBind(w http.ResponseWriter, r *http.Request, id int64) {
	var in struct {
		BangumiID int64 `json:"bangumi_id"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in) != nil || in.BangumiID < 0 {
		http.Error(w, "请求参数无效", 400)
		return
	}
	a, err := getAnime(id)
	if err != nil {
		http.Error(w, "番剧不存在", 404)
		return
	}
	if in.BangumiID > 0 {
		subject, err := fetchBangumiSubject(in.BangumiID)
		if err != nil {
			http.Error(w, "Bangumi 查询失败："+err.Error(), 502)
			return
		}
		coverURL := preferredCoverURL(subject)
		coverFile := ""
		if coverURL != "" {
			coverFile, err = cacheCover(coverURL)
			if err != nil {
				http.Error(w, "封面下载失败："+err.Error(), 502)
				return
			}
		}
		if _, err := db.Exec(`INSERT INTO anime_meta(anime_title,subject_id,name,name_cn,cover_url,cover_file,source,updated_at) VALUES(?,?,?,?,?,?,?,?)
			ON CONFLICT(anime_title) DO UPDATE SET subject_id=excluded.subject_id,name=excluded.name,name_cn=excluded.name_cn,
			cover_url=excluded.cover_url,cover_file=excluded.cover_file,source=excluded.source,updated_at=excluded.updated_at`,
			a.Title, subject.ID, subject.Name, subject.NameCN, coverURL, coverFile, "manual", time.Now().UTC().Format(time.RFC3339)); err != nil {
			http.Error(w, "保存元数据失败", 500)
			return
		}
		if _, err := db.Exec("UPDATE animes SET bangumi_id=?,updated_at=? WHERE id=?", in.BangumiID, time.Now().UTC().Format(time.RFC3339), id); err != nil {
			http.Error(w, "保存失败", 500)
			return
		}
	} else {
		if _, err := db.Exec("UPDATE animes SET bangumi_id=0,updated_at=? WHERE id=?", time.Now().UTC().Format(time.RFC3339), id); err != nil {
			http.Error(w, "保存失败", 500)
			return
		}
		db.Exec("DELETE FROM anime_meta WHERE anime_title=?", a.Title)
		select {
		case metaQueue <- a.Title:
		default:
		}
	}
	updated, err := getAnime(id)
	if err != nil {
		http.Error(w, "保存失败", 500)
		return
	}
	writeTvshowNFO(updated)
	w.Header().Set("Cache-Control", "no-store")
	jsonOut(w, map[string]any{"ok": true, "bangumi_id": updated.BangumiID})
}

// animeApplyRules re-applies the multi-source rules to existing items so rules
// added after syncing can still pull resources in from other series. Manually
// corrected items never move.
func animeApplyRules(w http.ResponseWriter, id int64) {
	a, err := getAnime(id)
	if err != nil {
		http.Error(w, "番剧不存在", 404)
		return
	}
	if strings.TrimSpace(a.IncludeRules) == "" {
		http.Error(w, "请先在编辑中填写包含规则关键词", 400)
		return
	}
	rows, err := db.Query(`SELECT id,COALESCE(title,'') FROM items
		WHERE anime_id!=? AND COALESCE(manual,0)=0 ORDER BY id DESC LIMIT 2000`, id)
	if err != nil {
		http.Error(w, "查询失败", 500)
		return
	}
	var candidates []int64
	for rows.Next() {
		var itemID int64
		var title string
		if rows.Scan(&itemID, &title) == nil && allowed(title, a.IncludeRules, a.ExcludeRules) {
			candidates = append(candidates, itemID)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		http.Error(w, "查询失败", 500)
		return
	}
	matched := 0
	for _, itemID := range candidates {
		result, err := db.Exec("UPDATE items SET anime_id=?,anime_title=? WHERE id=? AND COALESCE(manual,0)=0", id, a.Title, itemID)
		if err == nil {
			if n, _ := result.RowsAffected(); n > 0 {
				matched++
			}
		}
	}
	enqueueMissingMeta()
	w.Header().Set("Cache-Control", "no-store")
	jsonOut(w, map[string]int{"matched": matched})
}

// ---- GET /api/meta/search?q=：Bangumi 绑定候选 ----

func metaSearchAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "方法不支持", 405)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(query)) < 2 {
		http.Error(w, "请输入至少 2 个字符的关键词", 400)
		return
	}
	list, err := searchBangumiList(query)
	if err != nil {
		http.Error(w, "Bangumi 搜索失败："+err.Error(), 502)
		return
	}
	type candidate struct {
		ID     int64  `json:"id"`
		Name   string `json:"name"`
		NameCN string `json:"name_cn"`
	}
	out := make([]candidate, 0, len(list))
	for i := range list {
		out = append(out, candidate{ID: int64(list[i].ID), Name: list[i].Name, NameCN: list[i].NameCN})
	}
	w.Header().Set("Cache-Control", "no-store")
	jsonOut(w, map[string]any{"items": out})
}

// ---- PATCH /api/items/{id}：手动纠错（改判番剧/季/集） ----

func correctItem(w http.ResponseWriter, r *http.Request, id int64) {
	animeCleanupMu.RLock()
	defer animeCleanupMu.RUnlock()
	if r.Method != http.MethodPatch {
		http.Error(w, "方法不支持", 405)
		return
	}
	var in struct {
		AnimeID    int64   `json:"anime_id"`
		AnimeTitle string  `json:"anime_title"`
		Season     int     `json:"season"`
		Episode    *string `json:"episode"`
		Rearchive  bool    `json:"rearchive"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&in) != nil {
		http.Error(w, "请求参数无效", 400)
		return
	}
	if in.Season < 0 || in.Season > 999 {
		http.Error(w, "季号必须是 0-999（0 表示跟随番剧）", 400)
		return
	}
	var curAnimeID int64
	var curSeason int
	var curTitle, curEpisode, status, filePath string
	if err := db.QueryRow(`SELECT COALESCE(anime_id,0),COALESCE(anime_title,''),COALESCE(episode,''),COALESCE(status,''),COALESCE(file_path,''),COALESCE(season,0)
		FROM items WHERE id=?`, id).Scan(&curAnimeID, &curTitle, &curEpisode, &status, &filePath, &curSeason); err != nil {
		http.Error(w, "条目不存在", 404)
		return
	}
	newAnimeID, newTitle := curAnimeID, curTitle
	if in.AnimeID > 0 {
		a, err := getAnime(in.AnimeID)
		if err != nil {
			http.Error(w, "目标番剧不存在", 404)
			return
		}
		newAnimeID, newTitle = a.ID, a.Title
	} else if strings.TrimSpace(in.AnimeTitle) != "" {
		newTitle = sanitizeText(in.AnimeTitle, titleMaxLength)
		if newTitle == "" {
			http.Error(w, "番剧名称无效", 400)
			return
		}
		created, err := ensureAnime(newTitle)
		if err != nil {
			http.Error(w, "保存失败", 500)
			return
		}
		newAnimeID = created
	}
	newEpisode := curEpisode
	if in.Episode != nil {
		newEpisode = sanitizeText(*in.Episode, 64)
	}
	if newAnimeID == curAnimeID && newTitle == curTitle && in.Season == curSeason && newEpisode == curEpisode && !in.Rearchive {
		http.Error(w, "没有需要修改的内容", 400)
		return
	}
	// Guard against a concurrent correction or deletion while files are staged.
	persist := func(newPath string) error {
		result, err := db.Exec(`UPDATE items SET anime_id=?,anime_title=?,season=?,episode=?,manual=1,file_path=?
			WHERE id=? AND COALESCE(anime_id,0)=? AND COALESCE(anime_title,'')=? AND COALESCE(season,0)=?
			AND COALESCE(episode,'')=? AND COALESCE(status,'')=? AND COALESCE(file_path,'')=?`,
			newAnimeID, newTitle, in.Season, newEpisode, newPath, id, curAnimeID, curTitle, curSeason, curEpisode, status, filePath)
		if err != nil {
			return err
		}
		if n, err := result.RowsAffected(); err != nil || n != 1 {
			return fmt.Errorf("条目已被其他操作修改，请刷新后重试")
		}
		return nil
	}
	cleanupWarning := ""
	if in.Rearchive {
		if status != "已保存" || filePath == "" {
			http.Error(w, "只有已保存的条目才能迁移文件", 400)
			return
		}
		if _, err := os.Stat(filePath); err != nil {
			http.Error(w, "已保存的文件不存在，无法迁移", 400)
			return
		}
		warning, err := rearchiveSavedItem(id, filePath, newAnimeID, newTitle, newEpisode, in.Season, persist)
		if err != nil {
			http.Error(w, "文件迁移失败："+err.Error(), 500)
			return
		}
		cleanupWarning = warning
	} else if err := persist(filePath); err != nil {
		http.Error(w, "保存失败", 500)
		return
	}
	enqueueMissingMeta()
	w.Header().Set("Cache-Control", "no-store")
	jsonOut(w, map[string]any{"ok": true, "moved": in.Rearchive, "cleanup_warning": cleanupWarning})
}

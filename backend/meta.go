package main

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Cover metadata via Bangumi (bgm.tv). Lookups run in a background worker so
// a slow or unreachable metadata source never blocks syncing or downloading.
var bangumiAPI = strings.TrimRight(env("BANGUMI_API", "https://api.bgm.tv"), "/")
var bangumiEnabled = env("BANGUMI_ENABLED", "true") == "true"

// 封面缓存默认放在数据库同级（与内置下载器的数据目录同一约定），Windows 本机因此
// 跟着数据库落到程序同级的 data\，而不是盘根的 C:\data。
var coversDir = env("COVERS_DIR", filepath.Join(filepath.Dir(env("DB_PATH", defaultDBPath())), "covers"))
var metaClient = newClient(15 * time.Second)
var metaQueue = make(chan string, 256)

const metaUserAgent = "auto-anime/1.0 (https://github.com/sssshj114/auto-anime)"

const metaLookupTimeout = 30 * time.Second

// Old misses are retried after a matcher upgrade or a corrected search name.
func metaLookupKey(title, searchName string) string {
	sum := sha1.Sum([]byte("bracket-titles-v2\x00" + title + "\x00" + searchName))
	return hex.EncodeToString(sum[:])
}

func metaCacheFresh(source, updated, key, currentKey string, rating float64, summary string) bool {
	if source == "manual" || source == "bangumi" {
		// Matches made before rating/summary were stored carry only a cover;
		// they are refreshed once by the bound subject ID, not re-searched.
		return rating > 0 || summary != ""
	}
	if source != "none" || key != currentKey {
		return false
	}
	at, err := time.Parse(time.RFC3339, updated)
	return err == nil && at.After(time.Now().Add(-7*24*time.Hour))
}

// Keep legacy URL-only hashes readable alongside URL + content hashes.
var coverFilePattern = regexp.MustCompile(`^([0-9a-f]{16}|[0-9a-f]{12}-[0-9a-f]{8})\.(jpg|png|webp|gif)$`)

// Covers are only downloaded from Bangumi image hosts; the var exists so tests
// can point it at a local fake server.
var coverHostAllowed = isBangumiHost

func isBangumiHost(host string) bool {
	return host == "bgm.tv" || strings.HasSuffix(host, ".bgm.tv")
}

type bangumiSubject struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	NameCN  string `json:"name_cn"`
	Summary string `json:"summary"`
	Rating  struct {
		Score float64 `json:"score"`
		Total int     `json:"total"`
	} `json:"rating"`
	Images struct {
		Large  string `json:"large"`
		Common string `json:"common"`
		Medium string `json:"medium"`
		Grid   string `json:"grid"`
	} `json:"images"`
}

type bangumiVoice struct {
	Character struct {
		Name   string `json:"name"`
		Images struct {
			Medium string `json:"medium"`
			Grid   string `json:"grid"`
		} `json:"images"`
	} `json:"character"`
	Person struct {
		Name   string `json:"name"`
		Images struct {
			Medium string `json:"medium"`
			Grid   string `json:"grid"`
		} `json:"images"`
	} `json:"person"`
}

// startMetaWorker launches the single background lookup worker. Titles are
// processed one at a time with a pause to stay gentle with Bangumi.
func startMetaWorker() {
	if !bangumiEnabled {
		return
	}
	go func() {
		for title := range metaQueue {
			lookupAndCacheMeta(title)
			time.Sleep(500 * time.Millisecond)
		}
	}()
}

// enqueueMissingMeta queues every recognized anime title that has no metadata
// yet; unchanged negative results are retried after a week.
func enqueueMissingMeta() {
	if !bangumiEnabled {
		return
	}
	rows, err := db.Query(`SELECT DISTINCT items.anime_title,
		COALESCE(anime_meta.source,''),COALESCE(anime_meta.updated_at,''),
		COALESCE(anime_meta.match_key,''),COALESCE(animes.search_name,''),
		COALESCE(anime_meta.rating,0),COALESCE(anime_meta.summary,'') FROM items
		LEFT JOIN anime_meta ON anime_meta.anime_title = items.anime_title
		LEFT JOIN animes ON animes.title = items.anime_title
		WHERE COALESCE(items.anime_title,'') != ''`)
	if err != nil {
		log.Println("封面元数据扫描失败:", err)
		return
	}
	var titles []string
	for rows.Next() {
		var t, source, updated, key, searchName, summary string
		var rating float64
		if rows.Scan(&t, &source, &updated, &key, &searchName, &rating, &summary) == nil && strings.TrimSpace(t) != "" &&
			!metaCacheFresh(source, updated, key, metaLookupKey(t, searchName), rating, summary) {
			titles = append(titles, t)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		log.Println("封面元数据扫描失败:", err)
		return
	}
	for _, t := range titles {
		select {
		case metaQueue <- t:
		default:
			return
		}
	}
}

func lookupAndCacheMeta(animeTitle string) {
	// Capture user intent before reading the cache or doing network I/O. A
	// rematch changes UpdatedAt, so a previous lookup cannot restore its result.
	expected, err := readMetaLookupState(db, animeTitle)
	if err != nil {
		log.Printf("封面搜索配置读取失败 %q: %v", animeTitle, err)
		return
	}
	searchName, bangumiID := expected.SearchName, expected.BangumiID
	var source, updated, key, summary string
	var subjectID int64
	var rating float64
	if err := db.QueryRow(`SELECT COALESCE(source,''),COALESCE(updated_at,''),COALESCE(match_key,''),COALESCE(rating,0),COALESCE(summary,''),COALESCE(subject_id,0)
		FROM anime_meta WHERE anime_title=?`, animeTitle).Scan(&source, &updated, &key, &rating, &summary, &subjectID); err != nil && err != sql.ErrNoRows {
		log.Printf("封面元数据读取失败 %q: %v", animeTitle, err)
		return
	}
	currentKey := metaLookupKey(animeTitle, searchName)
	if metaCacheFresh(source, updated, key, currentKey, rating, summary) {
		return
	}
	// Refresh an existing match by its recorded subject ID; a manual binding
	// wins, and old auto matches keep their ID instead of re-running search.
	target := bangumiID
	if target == 0 && (source == "bangumi" || source == "manual") {
		target = subjectID
	}
	if target > 0 {
		subject, err := fetchBangumiSubject(target)
		if err != nil {
			// Network/API trouble: record nothing so the next sync retries.
			log.Printf("封面查询失败 %q: %v", animeTitle, err)
			return
		}
		// 补全旧记录时保持原有来源，自动匹配不因补全升级为手动绑定。
		src := source
		if src != "bangumi" && src != "manual" {
			src = "manual"
		}
		matchKey := currentKey
		if strings.HasPrefix(key, "mikan-link:") {
			matchKey = "mikan-link:" + currentKey
		}
		if err := storeMetaSubject(animeTitle, subject, src, matchKey, &expected); err != nil {
			log.Printf("封面元数据保存失败 %q: %v", animeTitle, err)
		}
		return
	}
	// A custom search name is an explicit correction. Only try the resource
	// page when there is no correction, binding or previous subject to refresh.
	mikanFailed := false
	if strings.TrimSpace(searchName) == "" {
		episode, err := mikanEpisodeURL(animeTitle)
		if err != nil {
			mikanFailed = true
		} else if episode != nil {
			subject, err := lookupMikanSubject(episode)
			if err == nil {
				err = storeMetaSubject(animeTitle, subject, "bangumi", "mikan-link:"+currentKey, &expected)
			}
			if err == nil {
				return
			}
			// A page/API failure is inconclusive. Fall back quietly and keep
			// the next sync eligible to retry even if text search finds nothing.
			mikanFailed = true
		}
	}
	aliases := metaLookupAliases(animeTitle, searchName)
	ctx, cancel := context.WithTimeout(context.Background(), metaLookupTimeout)
	defer cancel()
	subject, err := searchBangumiAliases(ctx, aliases)
	if err != nil {
		// Network/API trouble: do not renew the negative cache.
		log.Printf("封面查询失败 %q: %v", animeTitle, err)
		return
	}
	if subject != nil {
		if err := storeMetaSubject(animeTitle, subject, "bangumi", currentKey, &expected); err != nil {
			log.Printf("封面元数据保存失败 %q: %v", animeTitle, err)
		}
		return
	}
	if mikanFailed {
		return
	}
	// Search worked but nothing matched: remember it so we stop asking for a week.
	_, err = writeMetaResult(animeTitle, &expected, `INSERT INTO anime_meta(anime_title,source,updated_at,match_key) VALUES(?,'none',?,?)
		ON CONFLICT(anime_title) DO UPDATE SET source=excluded.source,updated_at=excluded.updated_at,match_key=excluded.match_key
		WHERE COALESCE(anime_meta.source,'') IN ('','none')`,
		animeTitle, time.Now().UTC().Format(time.RFC3339), currentKey)
	if err != nil {
		log.Printf("封面查询结果保存失败 %q: %v", animeTitle, err)
	}
}

// storeMetaSubject caches the cover of a resolved subject and records the
// metadata row; source is "bangumi" (auto) or "manual" (user binding). Automatic
// lookups carry the user configuration they read before doing network I/O, so
// a late result cannot supersede a correction or a forced rematch.
func storeMetaSubject(animeTitle string, subject *bangumiSubject, source, matchKey string, expected *metaLookupState) error {
	coverURL := preferredCoverURL(subject)
	if coverURL == "" {
		return fmt.Errorf("条目没有可用封面")
	}
	coverFile, err := cacheCover(coverURL)
	if err != nil {
		return err
	}
	voices := fetchBangumiVoices(subject.ID)
	changed, err := writeMetaResult(animeTitle, expected, `INSERT INTO anime_meta(anime_title,subject_id,name,name_cn,cover_url,cover_file,source,updated_at,rating,summary,voices,match_key)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(anime_title) DO UPDATE SET subject_id=excluded.subject_id,name=excluded.name,name_cn=excluded.name_cn,cover_url=excluded.cover_url,cover_file=excluded.cover_file,source=excluded.source,updated_at=excluded.updated_at,rating=excluded.rating,summary=excluded.summary,voices=excluded.voices,match_key=excluded.match_key
		WHERE excluded.source='manual' OR COALESCE(anime_meta.source,'')!='manual'`,
		animeTitle, subject.ID, subject.Name, subject.NameCN, coverURL, coverFile, source, time.Now().UTC().Format(time.RFC3339), subject.Rating.Score, subject.Summary, voices, matchKey)
	if err == nil && changed {
		// 先按旧名字刷新封面，再用官方名规范化番剧名与目录（见 anime_naming.go）。
		refreshSeriesPoster(animeTitle)
		standardizeAnimeTitle(animeTitle)
	}
	return err
}

type metaLookupState struct {
	ID, BangumiID         int64
	SearchName, UpdatedAt string
}

func readMetaLookupState(q interface {
	QueryRow(string, ...any) *sql.Row
}, animeTitle string) (metaLookupState, error) {
	var state metaLookupState
	err := q.QueryRow(`SELECT id,COALESCE(search_name,''),COALESCE(bangumi_id,0),COALESCE(updated_at,'') FROM animes WHERE title=?`, animeTitle).
		Scan(&state.ID, &state.SearchName, &state.BangumiID, &state.UpdatedAt)
	if err == sql.ErrNoRows {
		err = nil
	}
	return state, err
}

// Validate and write in one transaction on SQLite's single connection. Do not
// hold a transaction while fetching pages, subjects, covers or voice credits.
func writeMetaResult(animeTitle string, expected *metaLookupState, query string, args ...any) (bool, error) {
	animeCleanupMu.RLock()
	defer animeCleanupMu.RUnlock()
	tx, err := db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if expected != nil {
		current, err := readMetaLookupState(tx, animeTitle)
		if err != nil || current != *expected {
			return false, err
		}
	}
	result, err := tx.Exec(query, args...)
	if err != nil {
		return false, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return changed > 0, nil
}

func fetchBangumiVoices(id int) string {
	u := fmt.Sprintf("%s/v0/subjects/%d/characters", bangumiAPI, id)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return "[]"
	}
	req.Header.Set("User-Agent", metaUserAgent)
	res, err := metaClient.Do(req)
	if err != nil || res.StatusCode >= 400 {
		if res != nil {
			res.Body.Close()
		}
		return "[]"
	}
	defer res.Body.Close()
	var rows []bangumiVoice
	if json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&rows) != nil {
		return "[]"
	}
	type voice struct {
		Character      string `json:"character"`
		CharacterImage string `json:"character_image"`
		Person         string `json:"person"`
		PersonImage    string `json:"person_image"`
	}
	out := make([]voice, 0, min(len(rows), 12))
	for _, row := range rows {
		if row.Person.Name == "" {
			continue
		}
		image := row.Person.Images.Medium
		if image == "" {
			image = row.Person.Images.Grid
		}
		out = append(out, voice{row.Character.Name, row.Character.Images.Medium, row.Person.Name, image})
		if len(out) >= 12 {
			break
		}
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// searchBangumiList queries Bangumi and returns the raw candidate list for
// the binding UI (no heuristic filtering).
func searchBangumiList(query string) ([]bangumiSubject, error) {
	return searchBangumiListContext(context.Background(), query)
}

func searchBangumiListContext(ctx context.Context, query string) ([]bangumiSubject, error) {
	u := fmt.Sprintf("%s/search/subject/%s?type=2&responseGroup=small&max_results=8&format=json", bangumiAPI, url.PathEscape(query))
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", metaUserAgent)
	res, err := metaClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return nil, nil // no hit for this keyword
	}
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("Bangumi 返回 HTTP %d", res.StatusCode)
	}
	var out struct {
		List []bangumiSubject `json:"list"`
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, (2<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 2<<20 {
		return nil, fmt.Errorf("Bangumi 搜索响应超过 2 MB")
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	if len(out.List) > 8 {
		out.List = out.List[:8]
	}
	return out.List, nil
}

func searchBangumi(query string) (*bangumiSubject, error) {
	ctx, cancel := context.WithTimeout(context.Background(), metaLookupTimeout)
	defer cancel()
	return searchBangumiAliases(ctx, metaQueryTitles(query))
}

func searchBangumiAliases(ctx context.Context, aliases []string) (*bangumiSubject, error) {
	var candidates []bangumiSubject
	seen := map[int]bool{}
	for i, query := range metaSearchQueries(aliases) {
		if i > 0 {
			timer := time.NewTimer(500 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
		list, err := searchBangumiListContext(ctx, query)
		if err != nil {
			return nil, err
		}
		for _, subject := range list {
			if subject.ID > 0 && !seen[subject.ID] {
				seen[subject.ID] = true
				candidates = append(candidates, subject)
			}
		}
		if subject, score := bestMetaCandidate(aliases, candidates); score == 1000 {
			return subject, nil
		}
	}
	subject, _ := bestMetaCandidate(aliases, candidates)
	return subject, nil
}

func bestMetaCandidate(aliases []string, candidates []bangumiSubject) (*bangumiSubject, int) {
	best, score, runnerUp := -1, 0, 0
	for i := range candidates {
		n := metaAliasesScore(aliases, candidates[i].Name, candidates[i].NameCN)
		if n > score {
			best, score, runnerUp = i, n, score
		} else {
			runnerUp = max(runnerUp, n)
		}
	}
	// Ambiguous matches require a manual binding, independent of API order.
	if best < 0 || (runnerUp > 0 && score-runnerUp < 40) {
		return nil, 0
	}
	return &candidates[best], score
}

// fetchBangumiSubject reads a known subject through the v0 API.
func fetchBangumiSubject(id int64) (*bangumiSubject, error) {
	return fetchBangumiSubjectContext(context.Background(), id)
}

func fetchBangumiSubjectContext(ctx context.Context, id int64) (*bangumiSubject, error) {
	u := fmt.Sprintf("%s/v0/subjects/%d", bangumiAPI, id)
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", metaUserAgent)
	res, err := metaClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("Bangumi 条目 %d 不存在", id)
	}
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("Bangumi 返回 HTTP %d", res.StatusCode)
	}
	var subject bangumiSubject
	if err := json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&subject); err != nil {
		return nil, err
	}
	if subject.ID == 0 {
		subject.ID = int(id)
	}
	return &subject, nil
}

func preferredCoverURL(subject *bangumiSubject) string {
	for _, candidate := range []string{subject.Images.Large, subject.Images.Common, subject.Images.Medium, subject.Images.Grid} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		u, err := url.Parse(candidate)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
			continue
		}
		// The search API links Bangumi CDN images over http; upgrade them.
		if u.Scheme == "http" && isBangumiHost(u.Host) {
			candidate = "https://" + candidate[len("http://"):]
		}
		if strings.HasPrefix(candidate, "https://") || !isBangumiHost(u.Host) {
			return candidate
		}
	}
	return ""
}

// cacheCover downloads the cover into coversDir under a content-addressed
// name; the browser only ever loads local copies via /api/covers/.
func cacheCover(coverURL string) (string, error) {
	u, err := url.Parse(coverURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return "", fmt.Errorf("封面地址无效")
	}
	if !coverHostAllowed(u.Host) {
		return "", fmt.Errorf("封面域名不受信任: %s", u.Host)
	}
	req, err := http.NewRequest("GET", coverURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", metaUserAgent)
	res, err := metaClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return "", fmt.Errorf("封面下载返回 HTTP %d", res.StatusCode)
	}
	ext := map[string]string{
		"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp", "image/gif": ".gif",
	}[strings.TrimSpace(strings.SplitN(res.Header.Get("Content-Type"), ";", 2)[0])]
	if ext == "" {
		return "", fmt.Errorf("封面内容类型不受支持: %s", res.Header.Get("Content-Type"))
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 10<<20+1))
	if err != nil {
		return "", err
	}
	if len(data) > 10<<20 {
		return "", fmt.Errorf("封面文件超过 10 MB")
	}
	urlSum := sha1.Sum([]byte(coverURL))
	contentSum := sha256.Sum256(data)
	name := hex.EncodeToString(urlSum[:6]) + "-" + hex.EncodeToString(contentSum[:4]) + ext
	if err := os.MkdirAll(coversDir, 0755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(coversDir, ".tmp-cover-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0644); err != nil {
		tmp.Close()
		return "", err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(coversDir, name)); err != nil {
		return "", err
	}
	return name, nil
}

func serveCover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "方法不支持", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/api/covers/")
	if !coverFilePattern.MatchString(name) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=2592000, immutable")
	http.ServeFile(w, r, filepath.Join(coversDir, name))
}

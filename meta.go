package main

import (
	"crypto/sha1"
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
var coversDir = env("COVERS_DIR", "/data/covers")
var metaClient = newClient(15 * time.Second)
var metaQueue = make(chan string, 256)

const metaUserAgent = "auto-anime/1.0 (https://github.com/sssshj114/auto-anime)"

// Local cache names are content-addressed: 16 hex chars plus a known image extension.
var coverFilePattern = regexp.MustCompile(`^[0-9a-f]{16}\.(jpg|png|webp|gif)$`)
var metaTagPattern = regexp.MustCompile(`[\[【(（][^\]】)）]*[\]】)）]`)

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
// yet; negative results are retried only after a week.
func enqueueMissingMeta() {
	if !bangumiEnabled {
		return
	}
	stale := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	rows, err := db.Query(`SELECT DISTINCT items.anime_title FROM items
		LEFT JOIN anime_meta ON anime_meta.anime_title = items.anime_title
		WHERE COALESCE(items.anime_title,'') != ''
		AND (anime_meta.anime_title IS NULL OR (COALESCE(anime_meta.source,'') = 'none' AND COALESCE(anime_meta.updated_at,'') < ?))`, stale)
	if err != nil {
		log.Println("封面元数据扫描失败:", err)
		return
	}
	var titles []string
	for rows.Next() {
		var t string
		if rows.Scan(&t) == nil && strings.TrimSpace(t) != "" {
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
	var source string
	if err := db.QueryRow("SELECT COALESCE(source,'') FROM anime_meta WHERE anime_title=?", animeTitle).Scan(&source); err == nil && (source == "bangumi" || source == "manual") {
		return
	}
	// 手动绑定优先：番剧绑定了 Bangumi 条目就按 ID 直接取，不再猜。
	var searchName string
	var bangumiID int64
	db.QueryRow(`SELECT COALESCE(search_name,''),COALESCE(bangumi_id,0) FROM animes WHERE title=?`, animeTitle).Scan(&searchName, &bangumiID)
	if bangumiID > 0 {
		subject, err := fetchBangumiSubject(bangumiID)
		if err != nil {
			// Network/API trouble: record nothing so the next sync retries.
			log.Printf("封面查询失败 %q: %v", animeTitle, err)
			return
		}
		if err := storeMetaSubject(animeTitle, subject, "manual"); err != nil {
			log.Printf("封面元数据保存失败 %q: %v", animeTitle, err)
		}
		return
	}
	queries := metaQueryTitles(animeTitle)
	if custom := strings.TrimSpace(searchName); custom != "" {
		queries = append([]string{custom}, queries...)
	}
	for _, q := range queries {
		subject, err := searchBangumi(q)
		if err != nil {
			// Network/API trouble: record nothing so the next sync retries.
			log.Printf("封面查询失败 %q: %v", q, err)
			return
		}
		if subject == nil {
			continue
		}
		if err := storeMetaSubject(animeTitle, subject, "bangumi"); err != nil {
			log.Printf("封面元数据保存失败 %q: %v", animeTitle, err)
		}
		return
	}
	// Search worked but nothing matched: remember it so we stop asking for a week.
	db.Exec(`INSERT INTO anime_meta(anime_title,source,updated_at) VALUES(?,'none',?) ON CONFLICT(anime_title) DO NOTHING`,
		animeTitle, time.Now().UTC().Format(time.RFC3339))
}

// storeMetaSubject caches the cover of a resolved subject and records the
// metadata row; source is "bangumi" (auto) or "manual" (user binding).
func storeMetaSubject(animeTitle string, subject *bangumiSubject, source string) error {
	coverURL := preferredCoverURL(subject)
	if coverURL == "" {
		return fmt.Errorf("条目没有可用封面")
	}
	coverFile, err := cacheCover(coverURL)
	if err != nil {
		return err
	}
	voices := fetchBangumiVoices(subject.ID)
	_, err = db.Exec(`INSERT INTO anime_meta(anime_title,subject_id,name,name_cn,cover_url,cover_file,source,updated_at,rating,summary,voices) VALUES(?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(anime_title) DO UPDATE SET subject_id=excluded.subject_id,name=excluded.name,name_cn=excluded.name_cn,cover_url=excluded.cover_url,cover_file=excluded.cover_file,source=excluded.source,updated_at=excluded.updated_at,rating=excluded.rating,summary=excluded.summary,voices=excluded.voices`,
		animeTitle, subject.ID, subject.Name, subject.NameCN, coverURL, coverFile, source, time.Now().UTC().Format(time.RFC3339), subject.Rating.Score, subject.Summary, voices)
	if err == nil {
		refreshSeriesPoster(animeTitle)
	}
	return err
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

// metaQueryTitles turns a recognized title into search keywords. Bracket tags
// such as [WebRip][1080p] are noise for Bangumi, and "中文名 / Romaji" splits
// into two independent queries.
func metaQueryTitles(animeTitle string) []string {
	trimmed := strings.TrimSpace(animeTitle)
	if trimmed == "" {
		return nil
	}
	primary, rest := trimmed, ""
	if i := strings.Index(trimmed, " / "); i > 0 {
		primary, rest = trimmed[:i], trimmed[i+3:]
	}
	clean := func(s string) string {
		s = metaTagPattern.ReplaceAllString(s, " ")
		s = separatorPattern.ReplaceAllString(s, " ")
		return strings.Trim(strings.TrimSpace(s), "- _·")
	}
	out := []string{}
	if q := clean(primary); q != "" {
		out = append(out, q)
	}
	if q := clean(rest); q != "" && len(out) > 0 && q != out[0] {
		out = append(out, q)
	}
	return out
}

// metaAccept requires the query and one of the subject names to overlap after
// normalization, so an unrelated first hit does not steal the cover.
func metaAccept(query, name, nameCN string) bool {
	return metaMatchScore(query, name, nameCN) > 0
}

func normalizeMetaText(s string) string {
	s = strings.ToLower(strings.NewReplacer("０", "0", "１", "1", "２", "2", "３", "3", "４", "4", "５", "5", "６", "6", "７", "7", "８", "8", "９", "9").Replace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || (r >= '一' && r <= '龯') || (r >= 'ぁ' && r <= 'ヿ') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// metaMatchScore returns a positive score only when enough identifying text
// overlaps. Exact normalized aliases score highest; short season suffixes and
// punctuation are ignored by normalization, while unrelated short names do
// not pass merely because they share one character.
func metaMatchScore(query, name, nameCN string) int {
	q := normalizeMetaText(query)
	if len([]rune(q)) < 2 {
		return 0
	}
	best := 0
	for _, candidate := range []string{nameCN, name} {
		c := normalizeMetaText(candidate)
		if len([]rune(c)) < 2 {
			continue
		}
		if q == c {
			best = max(best, 1000)
		} else if strings.Contains(c, q) || strings.Contains(q, c) {
			best = max(best, 700+min(len([]rune(q)), len([]rune(c))))
		} else {
			common := longestCommonRunes(q, c)
			if common >= 4 || (common >= 3 && len([]rune(q)) >= 6) {
				best = max(best, common*10)
			}
		}
	}
	return best
}

func longestCommonRunes(a, b string) int {
	ar, br := []rune(a), []rune(b)
	best := 0
	for i := range ar {
		for j := range br {
			n := 0
			for i+n < len(ar) && j+n < len(br) && ar[i+n] == br[j+n] {
				n++
			}
			best = max(best, n)
		}
	}
	return best
}

// searchBangumiList queries Bangumi and returns the raw candidate list for
// the binding UI (no heuristic filtering).
func searchBangumiList(query string) ([]bangumiSubject, error) {
	u := fmt.Sprintf("%s/search/subject/%s?type=2&responseGroup=small&max_results=8&format=json", bangumiAPI, url.PathEscape(query))
	req, err := http.NewRequest("GET", u, nil)
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
	if err := json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.List) > 8 {
		out.List = out.List[:8]
	}
	return out.List, nil
}

func searchBangumi(query string) (*bangumiSubject, error) {
	list, err := searchBangumiList(query)
	if err != nil {
		return nil, err
	}
	best, score := -1, 0
	for i := range list {
		if n := metaMatchScore(query, list[i].Name, list[i].NameCN); n > score {
			best, score = i, n
		}
	}
	if best >= 0 {
		return &list[best], nil
	}
	return nil, nil
}

// fetchBangumiSubject reads one subject by ID through the v0 API; used when a
// series has a manual metadata binding.
func fetchBangumiSubject(id int64) (*bangumiSubject, error) {
	u := fmt.Sprintf("%s/v0/subjects/%d", bangumiAPI, id)
	req, err := http.NewRequest("GET", u, nil)
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

// cacheCover downloads the cover once into coversDir under a content-addressed
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
	sum := sha1.Sum([]byte(coverURL))
	name := hex.EncodeToString(sum[:8]) + ext
	if err := os.MkdirAll(coversDir, 0755); err != nil {
		return "", err
	}
	tmp := filepath.Join(coversDir, ".tmp-"+name)
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, filepath.Join(coversDir, name)); err != nil {
		os.Remove(tmp)
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

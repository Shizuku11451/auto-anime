package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// startMetaTestServer runs a fake Bangumi search + image CDN and points the
// metadata config at it. The fake only "knows" knownTitle; anything else gets
// an empty result list. imageHits counts cover downloads.
func startMetaTestServer(t *testing.T, knownTitle string) (*httptest.Server, *int) {
	t.Helper()
	imageHits := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/pic/cover/", func(w http.ResponseWriter, r *http.Request) {
		imageHits++
		w.Header().Set("Content-Type", "image/jpeg")
		fmt.Fprint(w, "fake-jpeg-bytes")
	})
	mux.HandleFunc("/search/subject/", func(w http.ResponseWriter, r *http.Request) {
		keyword, err := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/search/subject/"))
		if err != nil || keyword != knownTitle {
			fmt.Fprint(w, `{"results":0,"list":[]}`)
			return
		}
		base := "http://" + r.Host
		fmt.Fprintf(w, `{"results":1,"list":[{"id":4242,"name":"テストアニメ","name_cn":%q,"summary":"搜索匹配的测试简介。%s","rating":{"score":8.1,"total":120},"images":{"large":"%s/pic/cover/l/x.jpg","medium":"%s/pic/cover/m/x.jpg"}}]}`,
			knownTitle, knownTitle, base, base)
	})
	// v0 单条目接口：手动绑定按 ID 直接拉取；characters 供声优列表拉取。
	mux.HandleFunc("/v0/subjects/", func(w http.ResponseWriter, r *http.Request) {
		base := "http://" + r.Host
		if strings.TrimPrefix(r.URL.Path, "/v0/subjects/") == "4242/characters" {
			fmt.Fprintf(w, `[{"character":{"name":"主角","images":{"medium":"%s/pic/cover/m/c.jpg"}},"person":{"name":"田中声优","images":{"medium":"%s/pic/cover/m/p.jpg"}}}]`, base, base)
			return
		}
		if strings.TrimPrefix(r.URL.Path, "/v0/subjects/") != "4242" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		fmt.Fprintf(w, `{"id":4242,"name":"テストアニメ","name_cn":%q,"summary":"这是测试简介第一行。%s","rating":{"score":8.1,"total":120},"images":{"large":"%s/pic/cover/l/x.jpg"}}`, knownTitle, knownTitle, base)
	})
	server := httptest.NewServer(mux)
	oldAPI, oldDir, oldEnabled, oldQueue, oldClient, oldHost := bangumiAPI, coversDir, bangumiEnabled, metaQueue, metaClient, coverHostAllowed
	bangumiAPI = server.URL
	coversDir = t.TempDir()
	bangumiEnabled = true
	metaQueue = make(chan string, 256)
	metaClient = server.Client()
	host := server.Listener.Addr().String()
	coverHostAllowed = func(h string) bool { return h == host }
	t.Cleanup(func() {
		server.Close()
		bangumiAPI, coversDir, bangumiEnabled, metaQueue, metaClient, coverHostAllowed = oldAPI, oldDir, oldEnabled, oldQueue, oldClient, oldHost
	})
	return server, &imageHits
}

func withMetaTestEnv(t *testing.T) {
	t.Helper()
	oldDir, oldEnabled, oldQueue := coversDir, bangumiEnabled, metaQueue
	coversDir, bangumiEnabled, metaQueue = t.TempDir(), true, make(chan string, 256)
	t.Cleanup(func() { coversDir, bangumiEnabled, metaQueue = oldDir, oldEnabled, oldQueue })
}

func TestMetaQueryTitles(t *testing.T) {
	cases := []struct {
		title string
		want  []string
	}{
		{"与你相恋到生命尽头 / Kimi ga Shinu made Koi wo Shitai [WebRip][1080p][简日内嵌]", []string{"与你相恋到生命尽头", "Kimi ga Shinu made Koi wo Shitai"}},
		{"夏日番外篇 [1080p]", []string{"夏日番外篇"}},
		{"无职转生 第三季 ～到了异世界就拿出真本事～ / Mushoku Tensei III [10][1080P]", []string{"无职转生 第三季 ～到了异世界就拿出真本事～", "Mushoku Tensei III"}},
		{"", nil},
	}
	for _, c := range cases {
		got := metaQueryTitles(c.title)
		if len(got) != len(c.want) {
			t.Fatalf("queries of %q: got %#v want %#v", c.title, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("queries of %q: got %#v want %#v", c.title, got, c.want)
			}
		}
	}
}

func TestMetaAccept(t *testing.T) {
	if !metaAccept("葬送的芙莉莲", "葬送のフリーレン", "葬送的芙莉莲") {
		t.Fatal("exact name_cn should match")
	}
	if !metaAccept("无职转生 第二季", "Mushoku Tensei II", "无职转生 第二季") {
		t.Fatal("season suffix should still match")
	}
	if metaAccept("葬送的芙莉莲", "夏日动画", "夏日动画") {
		t.Fatal("unrelated subject must not match")
	}
	if metaAccept("与", "与", "与") {
		t.Fatal("single-character queries must not match")
	}
	if metaAccept("  ", "テスト", "测试") {
		t.Fatal("empty query must not match")
	}
	if !metaAccept("葬送的芙莉莲 S01 · 1080P", "葬送のフリーレン", "葬送的芙莉莲") {
		t.Fatal("season and quality noise should be ignored")
	}
	if !metaAccept("Kimi-ga-Shinu-made-Koi-wo-Shitai", "Kimi ga Shinu made Koi wo Shitai", "") {
		t.Fatal("punctuation differences should match")
	}
}

func TestCoverLookupCachesAndServes(t *testing.T) {
	testDatabase(t)
	server, imageHits := startMetaTestServer(t, "测试动画")
	if _, err := db.Exec("INSERT INTO items(title,link,anime_title) VALUES('测试动画 01','https://example.test/a','测试动画')"); err != nil {
		t.Fatal(err)
	}
	lookupAndCacheMeta("测试动画")
	if *imageHits != 1 {
		t.Fatalf("cover downloads: %d", *imageHits)
	}
	var coverFile, coverURL, source string
	if err := db.QueryRow("SELECT cover_file,cover_url,source FROM anime_meta WHERE anime_title='测试动画'").Scan(&coverFile, &coverURL, &source); err != nil {
		t.Fatalf("meta row missing: %v", err)
	}
	if source != "bangumi" || !strings.HasPrefix(coverURL, server.URL+"/pic/cover/") {
		t.Fatalf("meta row: source=%s url=%s", source, coverURL)
	}
	if !coverFilePattern.MatchString(coverFile) {
		t.Fatalf("cache name %q is not content-addressed", coverFile)
	}
	if data, err := os.ReadFile(filepath.Join(coversDir, coverFile)); err != nil || string(data) != "fake-jpeg-bytes" {
		t.Fatalf("cached cover: %q %v", data, err)
	}
	// A repeat lookup must not re-download anything.
	lookupAndCacheMeta("测试动画")
	if *imageHits != 1 {
		t.Fatalf("second lookup downloaded again: %d", *imageHits)
	}
	// The items API exposes the local cover path.
	recorder := httptest.NewRecorder()
	items(recorder, httptest.NewRequest("GET", "/api/items", nil))
	var payload []struct {
		Cover string `json:"cover"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil || len(payload) != 1 {
		t.Fatalf("items payload: %s %v", recorder.Body.String(), err)
	}
	if payload[0].Cover != "/api/covers/"+coverFile {
		t.Fatalf("cover field: %q", payload[0].Cover)
	}
	// Serving goes through the validated local cache only.
	res := httptest.NewRecorder()
	serveCover(res, httptest.NewRequest("GET", payload[0].Cover, nil))
	if res.Code != http.StatusOK || res.Body.String() != "fake-jpeg-bytes" {
		t.Fatalf("cover serve: %d %q", res.Code, res.Body.String())
	}
	if res.Header().Get("Cache-Control") == "" {
		t.Fatal("missing Cache-Control on cover response")
	}
}

func TestCoverLookupBackfillsRatingAndSummaryForOldMatches(t *testing.T) {
	testDatabase(t)
	server, imageHits := startMetaTestServer(t, "测试动画")
	// 旧版本只匹配了封面：来源 bangumi、meta 里留有 subject_id，但评分与简介为空；
	// animes.bangumi_id 只在手动绑定时写入，这里保持 0 以贴近自动匹配的旧数据。
	if _, err := db.Exec(`INSERT INTO animes(title) VALUES('测试动画')`); err != nil {
		t.Fatal(err)
	}
	id := animeIDByTitle(t, "测试动画")
	if _, err := db.Exec(`INSERT INTO items(title,link,anime_title,anime_id) VALUES('测试动画 01','https://example.test/old','测试动画',?)`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO anime_meta(anime_title,subject_id,name,name_cn,cover_url,cover_file,source,updated_at)
		VALUES('测试动画',4242,'テストアニメ','测试动画','http://localhost/pic/cover/l/x.jpg','legacy.jpg','bangumi','2026-09-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	// 手动绑定过的番剧按 animes.bangumi_id 补全。
	if _, err := db.Exec(`INSERT INTO animes(title,bangumi_id) VALUES('绑定动画',4242)`); err != nil {
		t.Fatal(err)
	}
	boundID := animeIDByTitle(t, "绑定动画")
	if _, err := db.Exec(`INSERT INTO items(title,link,anime_title,anime_id) VALUES('绑定动画 01','https://example.test/bound','绑定动画',?)`, boundID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO anime_meta(anime_title,subject_id,name,name_cn,cover_url,cover_file,source,updated_at)
		VALUES('绑定动画',4242,'テストアニメ','测试动画','http://localhost/pic/cover/l/x.jpg','legacy2.jpg','manual','2026-09-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	// 启动扫描把缺评分/简介的旧匹配入队，按已记录的条目 ID 补全而不是重新搜索。
	enqueueMissingMeta()
	queued := map[string]bool{}
	for {
		select {
		case v := <-metaQueue:
			queued[v] = true
			continue
		default:
		}
		break
	}
	if len(queued) != 2 || !queued["测试动画"] || !queued["绑定动画"] {
		t.Fatalf("queued lookups: %v", queued)
	}
	lookupAndCacheMeta("测试动画")
	lookupAndCacheMeta("绑定动画")
	for _, animeTitle := range []string{"测试动画", "绑定动画"} {
		var rating float64
		var summary, voices, source string
		if err := db.QueryRow(`SELECT COALESCE(rating,0),COALESCE(summary,''),COALESCE(voices,'[]'),COALESCE(source,'') FROM anime_meta WHERE anime_title=?`, animeTitle).
			Scan(&rating, &summary, &voices, &source); err != nil {
			t.Fatal(err)
		}
		if rating != 8.1 || summary == "" || !json.Valid([]byte(voices)) || voices == "[]" {
			t.Fatalf("backfilled meta %q: rating=%g summary=%q voices=%s", animeTitle, rating, summary, voices)
		}
		wantSource := "bangumi"
		if animeTitle == "绑定动画" {
			wantSource = "manual"
		}
		if source != wantSource {
			t.Fatalf("source must stay %s after backfill: %s", wantSource, source)
		}
	}
	if *imageHits < 2 {
		t.Fatalf("covers should be re-cached during backfill: %d", *imageHits)
	}
	// 数据齐全后不再重复入队或重查。
	enqueueMissingMeta()
	select {
	case v := <-metaQueue:
		t.Fatalf("complete metadata must not be re-queued: %q", v)
	default:
	}
	lookupAndCacheMeta("测试动画")
	lookupAndCacheMeta("绑定动画")
	_ = server
}

func TestCoverLookupNoMatchRecordsNegativeCache(t *testing.T) {
	testDatabase(t)
	_, imageHits := startMetaTestServer(t, "别的动画")
	lookupAndCacheMeta("完全不存在的作品")
	if *imageHits != 0 {
		t.Fatalf("unrelated subject must not trigger downloads: %d", *imageHits)
	}
	var source string
	if err := db.QueryRow("SELECT COALESCE(source,'') FROM anime_meta WHERE anime_title='完全不存在的作品'").Scan(&source); err != nil || source != "none" {
		t.Fatalf("negative cache row: %q %v", source, err)
	}
}

func TestCoverLookupFailureLeavesRowForRetry(t *testing.T) {
	testDatabase(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()
	oldAPI, oldClient := bangumiAPI, metaClient
	bangumiAPI, metaClient = server.URL, server.Client()
	t.Cleanup(func() { bangumiAPI, metaClient = oldAPI, oldClient })
	lookupAndCacheMeta("测试动画")
	var count int
	db.QueryRow("SELECT COUNT(*) FROM anime_meta").Scan(&count)
	if count != 0 {
		t.Fatal("failed lookup must not write any cache row")
	}
}

func TestServeCoverRejectsUnsafeNames(t *testing.T) {
	testDatabase(t)
	withMetaTestEnv(t)
	for _, name := range []string{"..%2f..%2fsecret.txt", "zzzz.txt", "%2e%2e%2fstate.db", "0123456789abcdef.jpeg"} {
		res := httptest.NewRecorder()
		serveCover(res, httptest.NewRequest("GET", "/api/covers/"+name, nil))
		if res.Code != http.StatusNotFound {
			t.Fatalf("unsafe cover name %q got %d", name, res.Code)
		}
	}
}

func TestEnqueueMissingMetaSkipsCachedAndRetriesStaleNegative(t *testing.T) {
	testDatabase(t)
	withMetaTestEnv(t)
	seed := func(animeTitle, source, updated string) {
		if _, err := db.Exec("INSERT INTO items(title,link,anime_title) VALUES(?,?,?)", animeTitle+" 01", "https://example.test/"+url.PathEscape(animeTitle), animeTitle); err != nil {
			t.Fatal(err)
		}
		if source != "" {
			// 完整匹配记录带评分；缺评分/简介的旧记录会被补全流程重新入队。
			rating := 0.0
			if source == "bangumi" || source == "manual" {
				rating = 8.5
			}
			if _, err := db.Exec("INSERT INTO anime_meta(anime_title,source,updated_at,match_key,rating) VALUES(?,?,?,?,?)",
				animeTitle, source, updated, metaLookupKey(animeTitle, ""), rating); err != nil {
				t.Fatal(err)
			}
		}
	}
	fresh := time.Now().UTC().Format(time.RFC3339)
	stale := time.Now().AddDate(0, 0, -8).UTC().Format(time.RFC3339)
	seed("已缓存动画", "bangumi", fresh)
	seed("新负缓存", "none", fresh)
	seed("旧负缓存", "none", stale)
	seed("没查过的动画", "", "")
	seed("旧版新负缓存", "none", fresh)
	if _, err := db.Exec("UPDATE anime_meta SET match_key='' WHERE anime_title='旧版新负缓存'"); err != nil {
		t.Fatal(err)
	}
	enqueueMissingMeta()
	queued := map[string]bool{}
	for {
		select {
		case v := <-metaQueue:
			queued[v] = true
			continue
		default:
		}
		break
	}
	if len(queued) != 3 || !queued["旧负缓存"] || !queued["没查过的动画"] || !queued["旧版新负缓存"] {
		t.Fatalf("queued lookups: %v", queued)
	}
}

func TestSyncTriggersMetaEnqueueWithoutBlocking(t *testing.T) {
	testDatabase(t)
	withMetaTestEnv(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<rss><channel><item><title>[组] 测试动画 01 [1080p]</title><link>https://example.test/1.mp4</link></item></channel></rss>`)
	}))
	defer server.Close()
	result, err := db.Exec("INSERT INTO feeds(name,url) VALUES('订阅',?)", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	feedID, _ := result.LastInsertId()
	n, err := syncFeed(int(feedID))
	if err != nil || n != 1 {
		t.Fatalf("sync: %d %v", n, err)
	}
	select {
	case title := <-metaQueue:
		if title != "测试动画" {
			t.Fatalf("queued title: %q", title)
		}
	case <-time.After(time.Second):
		t.Fatal("sync did not enqueue a cover lookup")
	}
}

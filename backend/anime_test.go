package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// putAnime 通过 PUT /api/animes/{id} 更新番剧配置。
func putAnime(t *testing.T, id int64, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", fmt.Sprintf("/api/animes/%d", id), strings.NewReader(body))
	animeEntryAPI(rec, req)
	return rec
}

func animeIDByTitle(t *testing.T, title string) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow("SELECT id FROM animes WHERE title=?", title).Scan(&id); err != nil {
		t.Fatalf("anime %q missing: %v", title, err)
	}
	return id
}

func TestSyncMergesItemsByAnimeRules(t *testing.T) {
	testDatabase(t)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<rss><channel>
			<item><title>[灵屏组] 星海物语 第01话 [1080p][简中]</title><enclosure url="%s/e01.mp4"/></item>
			<item><title>[银河组] 星海物语 第02话 [1080p][繁中]</title><enclosure url="%s/e02.mp4"/></item>
			<item><title>[灵屏组] 别的作品 第03话</title><enclosure url="%s/e03.mp4"/></item>
		</channel></rss>`, server.URL, server.URL, server.URL)
	}))
	defer server.Close()
	r, err := db.Exec("INSERT INTO feeds(name,url) VALUES('混合订阅',?)", server.URL+"/rss")
	if err != nil {
		t.Fatal(err)
	}
	feedID, _ := r.LastInsertId()
	// 先同步：三条都按本地识别标题落库。
	if n, err := syncFeed(int(feedID)); err != nil || n != 3 {
		t.Fatalf("first sync: %d %v", n, err)
	}
	// 建立番剧 + 多来源规则，"应用规则"应把两个字幕组的星海物语都归并进来。
	if _, err := db.Exec(`INSERT INTO animes(title,include_rules,exclude_rules) VALUES('星海物语','星海物语','')`); err != nil {
		t.Fatal(err)
	}
	id := animeIDByTitle(t, "星海物语")
	rec := httptest.NewRecorder()
	animeEntryAPI(rec, httptest.NewRequest("POST", fmt.Sprintf("/api/animes/%d/apply-rules", id), nil))
	if rec.Code != 200 {
		t.Fatalf("apply-rules: %d %s", rec.Code, rec.Body.String())
	}
	var matched int
	json.Unmarshal(rec.Body.Bytes(), &matched)
	_ = matched
	var owned int
	if err := db.QueryRow("SELECT COUNT(*) FROM items WHERE anime_id=?", id).Scan(&owned); err != nil || owned != 2 {
		t.Fatalf("rule merge: owned=%d err=%v", owned, err)
	}
	// 规则命中后新同步的条目直接归入该番剧（模拟其它订阅源发布同一部番）。
	r2, err := db.Exec("INSERT INTO feeds(name,url) VALUES('另一来源','https://example.test/other')")
	if err != nil {
		t.Fatal(err)
	}
	_ = r2
	if _, err := db.Exec("INSERT OR IGNORE INTO items(feed_id,title,link,anime_title,episode) VALUES(2,'[银河组] 星海物语 第03话 [简中]','https://example.test/other/03','银河组 星海物语','03')"); err != nil {
		t.Fatal(err)
	}
	// 重新应用规则：手动纠错过的条目不动，其余按规则搬家。
	if _, err := db.Exec("UPDATE items SET manual=1 WHERE link='https://example.test/other/03'"); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	animeEntryAPI(rec, httptest.NewRequest("POST", fmt.Sprintf("/api/animes/%d/apply-rules", id), nil))
	if rec.Code != 200 {
		t.Fatalf("apply-rules 2: %d %s", rec.Code, rec.Body.String())
	}
	var manualOwner int64
	db.QueryRow("SELECT COALESCE(anime_id,0) FROM items WHERE link='https://example.test/other/03'").Scan(&manualOwner)
	if manualOwner == id {
		t.Fatal("manually corrected item must not be moved by rules")
	}
}

func TestSyncRoutesNewItemByRule(t *testing.T) {
	testDatabase(t)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<rss><channel><item><title>[新组] 星海物语 第04话 [1080p]</title><enclosure url="%s/e04.mp4"/></item></channel></rss>`, server.URL)
	}))
	defer server.Close()
	if _, err := db.Exec(`INSERT INTO animes(title,include_rules,exclude_rules) VALUES('星海物语','星海物语','')`); err != nil {
		t.Fatal(err)
	}
	id := animeIDByTitle(t, "星海物语")
	r, err := db.Exec("INSERT INTO feeds(name,url) VALUES('来源B',?)", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	feedID, _ := r.LastInsertId()
	if _, err := syncFeed(int(feedID)); err != nil {
		t.Fatal(err)
	}
	var animeID int64
	var animeTitle string
	if err := db.QueryRow("SELECT COALESCE(anime_id,0),COALESCE(anime_title,'') FROM items WHERE feed_id=?", feedID).Scan(&animeID, &animeTitle); err != nil {
		t.Fatal(err)
	}
	if animeID != id || animeTitle != "星海物语" {
		t.Fatalf("rule routing at sync: anime_id=%d title=%q", animeID, animeTitle)
	}
}

func TestAnimeListAndDetailGrouping(t *testing.T) {
	testDatabase(t)
	if _, err := db.Exec(`INSERT INTO animes(title,season) VALUES(' grouping番',0)`); err != nil {
		t.Fatal(err)
	}
	id := animeIDByTitle(t, " grouping番")
	for _, it := range []struct{ ep, group, season string }{
		{"01", "A组", "0"},
		{"01", "B组", "0"},
		{"10", "A组", "0"},
		{"2", "A组", "0"},
		{"", "A组", "2"},
	} {
		if _, err := db.Exec(`INSERT INTO items(title,link,anime_title,anime_id,episode,fansub_group,season,status)
			VALUES('x',?,' grouping番',?,?,?,?, '已保存')`,
			"https://example.test/"+it.ep+it.group, id, it.ep, it.group, it.season); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO anime_meta(anime_title,source,rating,summary) VALUES(' grouping番','bangumi',8.65,'群组番剧的测试简介。')`); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	animesAPI(rec, httptest.NewRequest("GET", "/api/animes", nil))
	if rec.Code != 200 {
		t.Fatalf("animes list: %d", rec.Code)
	}
	var list []struct {
		ID      int64   `json:"id"`
		Title   string  `json:"title"`
		Items   int     `json:"item_count"`
		Epis    int     `json:"episode_count"`
		Saved   int     `json:"saved_count"`
		Season  int     `json:"season"`
		Rating  float64 `json:"rating"`
		Summary string  `json:"summary"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 1 {
		t.Fatalf("list payload: %s %v", rec.Body.String(), err)
	}
	if list[0].Items != 5 || list[0].Epis != 3 || list[0].Saved != 5 || list[0].Season != 0 {
		t.Fatalf("list counters: %+v", list[0])
	}
	if list[0].Rating != 8.65 || list[0].Summary != "群组番剧的测试简介。" {
		t.Fatalf("list rating/summary: %+v", list[0])
	}

	rec = httptest.NewRecorder()
	animeEntryAPI(rec, httptest.NewRequest("GET", fmt.Sprintf("/api/animes/%d", id), nil))
	if rec.Code != 200 {
		t.Fatalf("detail: %d %s", rec.Code, rec.Body.String())
	}
	var detail struct {
		Seasons []struct {
			Season   int `json:"season"`
			Episodes []struct {
				Episode string           `json:"episode"`
				Items   []map[string]any `json:"items"`
			} `json:"episodes"`
			Loose []map[string]any `json:"loose"`
		} `json:"seasons"`
		Sources []string `json:"sources"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.Seasons) != 2 {
		t.Fatalf("seasons: %+v", detail.Seasons)
	}
	s1 := detail.Seasons[0]
	if s1.Season != 1 || len(s1.Episodes) != 3 {
		t.Fatalf("season 1 grouping: %+v", s1)
	}
	// 集数按数值排序：01, 02, 10；同集多来源合并进一个集。
	if s1.Episodes[0].Episode != "01" || s1.Episodes[1].Episode != "2" || s1.Episodes[2].Episode != "10" {
		t.Fatalf("episode order: %s %s %s", s1.Episodes[0].Episode, s1.Episodes[1].Episode, s1.Episodes[2].Episode)
	}
	if len(s1.Episodes[0].Items) != 2 {
		t.Fatalf("multi-source merge: %+v", s1.Episodes[0])
	}
	if len(detail.Seasons[1].Loose) != 1 {
		t.Fatalf("loose resources: %+v", detail.Seasons[1])
	}
	if len(detail.Sources) != 0 {
		t.Fatalf("sources should skip empty feed names: %v", detail.Sources)
	}
}

func TestAnimeUpdateRenameAndRules(t *testing.T) {
	testDatabase(t)
	if _, err := db.Exec(`INSERT INTO animes(title) VALUES('旧名')`); err != nil {
		t.Fatal(err)
	}
	id := animeIDByTitle(t, "旧名")
	if _, err := db.Exec("INSERT INTO items(title,link,anime_title,anime_id) VALUES('旧名 01','https://example.test/r1','旧名',?)", id); err != nil {
		t.Fatal(err)
	}
	rec := putAnime(t, id, `{"title":"新名","search_name":"新名 搜索词","season":2,"include_rules":"新名","exclude_rules":"广告"}`)
	if rec.Code != 200 {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	var title string
	var season int
	var searchName, include, exclude string
	if err := db.QueryRow(`SELECT title,COALESCE(season,0),COALESCE(search_name,''),COALESCE(include_rules,''),COALESCE(exclude_rules,'') FROM animes WHERE id=?`, id).
		Scan(&title, &season, &searchName, &include, &exclude); err != nil {
		t.Fatal(err)
	}
	if title != "新名" || season != 2 || searchName != "新名 搜索词" || include != "新名" || exclude != "广告" {
		t.Fatalf("update result: %q %d %q %q %q", title, season, searchName, include, exclude)
	}
	// 重命名级联到条目。
	var itemTitle string
	db.QueryRow("SELECT COALESCE(anime_title,'') FROM items WHERE anime_id=?", id).Scan(&itemTitle)
	if itemTitle != "新名" {
		t.Fatalf("rename cascade: %q", itemTitle)
	}
	// 同名冲突被拒绝。
	if _, err := db.Exec(`INSERT INTO animes(title) VALUES('占位')`); err != nil {
		t.Fatal(err)
	}
	other := animeIDByTitle(t, "占位")
	rec = putAnime(t, other, `{"title":"新名"}`)
	if rec.Code != 409 {
		t.Fatalf("duplicate title: %d", rec.Code)
	}
	// 非法季号被拒绝。
	rec = putAnime(t, id, `{"season":1000}`)
	if rec.Code != 400 {
		t.Fatalf("bad season: %d", rec.Code)
	}
	// 有条目的番剧不能删除。
	rec = httptest.NewRecorder()
	animeEntryAPI(rec, httptest.NewRequest("DELETE", fmt.Sprintf("/api/animes/%d", id), nil))
	if rec.Code != 409 {
		t.Fatalf("delete with items: %d", rec.Code)
	}
	// 无条目可以删除。
	rec = httptest.NewRecorder()
	animeEntryAPI(rec, httptest.NewRequest("DELETE", fmt.Sprintf("/api/animes/%d", other), nil))
	if rec.Code != 200 {
		t.Fatalf("delete empty anime: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAnimeMetadataBinding(t *testing.T) {
	testDatabase(t)
	_, _ = startMetaTestServer(t, "测试动画")
	if _, err := db.Exec(`INSERT INTO animes(title,bangumi_id) VALUES('测试动画',0)`); err != nil {
		t.Fatal(err)
	}
	id := animeIDByTitle(t, "测试动画")
	// 绑定：按 ID 直接拉取条目并缓存封面，来源标记 manual。
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", fmt.Sprintf("/api/animes/%d/metadata", id), strings.NewReader(`{"bangumi_id":4242}`))
	animeEntryAPI(rec, req)
	if rec.Code != 200 {
		t.Fatalf("bind: %d %s", rec.Code, rec.Body.String())
	}
	var source, coverFile, nameCN string
	var subjectID int64
	if err := db.QueryRow(`SELECT COALESCE(source,''),COALESCE(cover_file,''),COALESCE(name_cn,''),COALESCE(subject_id,0) FROM anime_meta WHERE anime_title='测试动画'`).
		Scan(&source, &coverFile, &nameCN, &subjectID); err != nil {
		t.Fatalf("meta row: %v", err)
	}
	if source != "manual" || coverFile == "" || subjectID != 4242 {
		t.Fatalf("bound meta: source=%s cover=%q subject=%d", source, coverFile, subjectID)
	}
	if nameCN != "测试动画" {
		t.Fatalf("name_cn: %q", nameCN)
	}
	var bangumiID int64
	db.QueryRow("SELECT COALESCE(bangumi_id,0) FROM animes WHERE id=?", id).Scan(&bangumiID)
	if bangumiID != 4242 {
		t.Fatalf("animes.bangumi_id: %d", bangumiID)
	}
	// 绑定同时保存评分、简介与声优列表，媒体库卡片直接可用。
	var rating float64
	var summary, voices string
	if err := db.QueryRow(`SELECT COALESCE(rating,0),COALESCE(summary,''),COALESCE(voices,'[]') FROM anime_meta WHERE anime_title='测试动画'`).
		Scan(&rating, &summary, &voices); err != nil {
		t.Fatalf("bound rating/summary: %v", err)
	}
	if rating != 8.1 || summary == "" || !json.Valid([]byte(voices)) || voices == "[]" {
		t.Fatalf("bound rating/summary/voices: %g %q %s", rating, summary, voices)
	}
	// 已绑定后自动查找不再覆盖 manual 记录。
	lookupAndCacheMeta("测试动画")
	db.QueryRow("SELECT COALESCE(source,'') FROM anime_meta WHERE anime_title='测试动画'").Scan(&source)
	if source != "manual" {
		t.Fatalf("manual meta overwritten: %s", source)
	}
	// 解绑：清空元数据并重新入队自动识别。
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("PUT", fmt.Sprintf("/api/animes/%d/metadata", id), strings.NewReader(`{"bangumi_id":0}`))
	animeEntryAPI(rec, req)
	if rec.Code != 200 {
		t.Fatalf("unbind: %d %s", rec.Code, rec.Body.String())
	}
	var rows int
	db.QueryRow("SELECT COUNT(*) FROM anime_meta WHERE anime_title='测试动画'").Scan(&rows)
	if rows != 0 {
		t.Fatalf("unbind should clear meta, %d rows", rows)
	}
	select {
	case title := <-metaQueue:
		if title != "测试动画" {
			t.Fatalf("re-queued title: %q", title)
		}
	default:
		t.Fatal("unbind should re-enqueue automatic lookup")
	}
}

func TestMetaSearchAPIReturnsCandidates(t *testing.T) {
	testDatabase(t)
	startMetaTestServer(t, "星海物语")
	rec := httptest.NewRecorder()
	metaSearchAPI(rec, httptest.NewRequest("GET", "/api/meta/search?q="+"%E6%98%9F%E6%B5%B7%E7%89%A9%E8%AF%AD", nil))
	if rec.Code != 200 {
		t.Fatalf("search: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Items []struct {
			ID     int64  `json:"id"`
			NameCN string `json:"name_cn"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || len(out.Items) != 1 || out.Items[0].ID != 4242 {
		t.Fatalf("search payload: %s %v", rec.Body.String(), err)
	}
	// 过短关键词拒绝。
	rec = httptest.NewRecorder()
	metaSearchAPI(rec, httptest.NewRequest("GET", "/api/meta/search?q=%E6%98%9F", nil))
	if rec.Code != 400 {
		t.Fatalf("short query: %d", rec.Code)
	}
}

func TestCorrectItemReallocatesAndMovesFile(t *testing.T) {
	testDatabase(t)
	server := mediaServer(t)
	// 保存一集到默认位置。
	result, _ := db.Exec("INSERT INTO items(title,link,anime_title,anime_id,episode) VALUES(?,?,?,?,?)",
		"[组] 某动画 [07]", server.URL+"/ep07.mp4", "某动画", 0, "07")
	id, _ := result.LastInsertId()
	savedPath, err := fetchArchive(int(id), server.URL+"/ep07.mp4")
	if err != nil {
		t.Fatal(err)
	}
	// 真实下载流程在归档成功后写回状态与路径；这里等价模拟。
	if _, err := db.Exec("UPDATE items SET status='已保存',file_path=? WHERE id=?", savedPath, id); err != nil {
		t.Fatal(err)
	}
	// 目标番剧：第二季。
	if _, err := db.Exec(`INSERT INTO animes(title,season) VALUES('另一部动画',2)`); err != nil {
		t.Fatal(err)
	}
	target := animeIDByTitle(t, "另一部动画")
	oldPath := filepath.Join(downloadDir, "某动画", "Season 01", "某动画 - S01E07.mp4")
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("saved file missing: %v", err)
	}
	body := fmt.Sprintf(`{"anime_id":%d,"episode":"07","rearchive":true}`, target)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/items/%d", id), strings.NewReader(body))
	correctItem(rec, req, id)
	if rec.Code != 200 {
		t.Fatalf("correct: %d %s", rec.Code, rec.Body.String())
	}
	newPath := filepath.Join(downloadDir, "另一部动画", "Season 02", "另一部动画 - S02E07.mp4")
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("file not moved: %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatal("old file still exists")
	}
	var file_path, animeTitle string
	var animeID, manual int64
	var season int
	if err := db.QueryRow(`SELECT COALESCE(file_path,''),COALESCE(anime_title,''),COALESCE(anime_id,0),COALESCE(manual,0),COALESCE(season,0) FROM items WHERE id=?`, id).
		Scan(&file_path, &animeTitle, &animeID, &manual, &season); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(filepath.ToSlash(file_path), "另一部动画/Season 02/另一部动画 - S02E07.mp4") {
		t.Fatalf("file_path: %q", file_path)
	}
	if animeID != target || animeTitle != "另一部动画" || manual != 1 || season != 0 {
		t.Fatalf("item row: %q %d manual=%d season=%d", animeTitle, animeID, manual, season)
	}
	// 旧目录被清理，不留下空 Season 01。
	if _, err := os.Stat(filepath.Join(downloadDir, "某动画", "Season 01")); !os.IsNotExist(err) {
		t.Fatal("old season dir not cleaned up")
	}
	// 新位置带同名 NFO / info.json，且番剧根有 tvshow.nfo。
	for _, name := range []string{"另一部动画 - S02E07.nfo", "另一部动画 - S02E07-info.json"} {
		if _, err := os.Stat(filepath.Join(filepath.Dir(newPath), name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	nfo, err := os.ReadFile(filepath.Join(downloadDir, "另一部动画", "tvshow.nfo"))
	if err != nil || !strings.Contains(string(nfo), "<tvshow>") || !strings.Contains(string(nfo), "另一部动画") {
		t.Fatalf("tvshow.nfo: %q %v", nfo, err)
	}
}

func TestCorrectItemRejectsInvalidInput(t *testing.T) {
	testDatabase(t)
	if _, err := db.Exec(`INSERT INTO animes(title) VALUES('目标')`); err != nil {
		t.Fatal(err)
	}
	target := animeIDByTitle(t, "目标")
	result, _ := db.Exec("INSERT INTO items(title,link,anime_title,status) VALUES('t','https://example.test/x','源','待下载')")
	id, _ := result.LastInsertId()
	for _, body := range []string{
		`{"anime_id":999}`,   // 不存在的番剧
		`{"season":1000}`,    // 非法季号
		`{}`,                 // 没有任何修改
		`{"rearchive":true}`, // 未保存的条目不能迁移
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/items/%d", id), strings.NewReader(body))
		correctItem(rec, req, id)
		if rec.Code != 400 && rec.Code != 404 {
			t.Fatalf("case %s accepted: %d %s", body, rec.Code, rec.Body.String())
		}
	}
	// 目标番剧字段未变化（用 anime_title 新建），但集数改了：允许。
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/items/%d", id), strings.NewReader(`{"anime_title":"新建番剧","episode":"5"}`))
	correctItem(rec, req, id)
	if rec.Code != 200 {
		t.Fatalf("rename+episode: %d %s", rec.Code, rec.Body.String())
	}
	var animeID int64
	var episode string
	db.QueryRow(`SELECT COALESCE(anime_id,0),COALESCE(episode,'') FROM items WHERE id=?`, id).Scan(&animeID, &episode)
	if animeID != target && animeID == 0 {
		t.Fatal("anime row not created")
	}
	if episode != "5" {
		t.Fatalf("episode: %q", episode)
	}
}

func TestAnimeDetailSeriesNFOAndSeasonDefault(t *testing.T) {
	testDatabase(t)
	server := mediaServer(t)
	if _, err := db.Exec(`INSERT INTO animes(title,season,storage_dir) VALUES('NFO动画',3,'')`); err != nil {
		t.Fatal(err)
	}
	id := animeIDByTitle(t, "NFO动画")
	result, _ := db.Exec("INSERT INTO items(title,link,anime_title,anime_id,episode) VALUES(?,?,?,?,?)",
		"[组] NFO动画 [05]", server.URL+"/e05.mp4", "NFO动画", id, "5")
	itemID, _ := result.LastInsertId()
	path, err := fetchArchive(int(itemID), server.URL+"/e05.mp4")
	if err != nil {
		t.Fatal(err)
	}
	// 番剧季号 3 生效：归档进 Season 03。
	want := filepath.Join(downloadDir, "NFO动画", "Season 03", "NFO动画 - S03E05.mp4")
	if filepath.Clean(path) != filepath.Clean(want) {
		t.Fatalf("series season default: %s", path)
	}
	nfo, err := os.ReadFile(filepath.Join(downloadDir, "NFO动画", "tvshow.nfo"))
	if err != nil || !strings.Contains(string(nfo), "<tvshow>") {
		t.Fatalf("tvshow.nfo: %q %v", nfo, err)
	}
	// 条目级季号覆盖番剧默认值。
	result, _ = db.Exec("INSERT INTO items(title,link,anime_title,anime_id,episode,season) VALUES(?,?,?,?,?,?)",
		"[组] NFO动画 SP [01]", server.URL+"/sp01.mp4", "NFO动画", id, "01", 0)
	itemID, _ = result.LastInsertId()
	db.Exec("UPDATE items SET season=1 WHERE id=?", itemID)
	if _, err := fetchArchive(int(itemID), server.URL+"/sp01.mp4"); err != nil {
		t.Fatal(err)
	}
	sp := filepath.Join(downloadDir, "NFO动画", "Season 01", "NFO动画 - S01E01.mp4")
	if _, err := os.Stat(sp); err != nil {
		t.Fatalf("item season override: %v", err)
	}
}

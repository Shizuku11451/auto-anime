package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	titleparse "auto-anime/internal/title"
)

// mediaServer answers every request with a small MP4 payload.
func mediaServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("media-" + r.URL.Path))
	}))
	t.Cleanup(server.Close)
	return server
}

func putSeriesStorage(t *testing.T, title, dir string, season int) *httptest.ResponseRecorder {
	t.Helper()
	body := fmt.Sprintf(`{"storage_dir":%q,"season":%d}`, dir, season)
	rec := httptest.NewRecorder()
	animeStorageAPI(rec, httptest.NewRequest("PUT", "/api/anime/"+title+"/storage", strings.NewReader(body)))
	return rec
}

func TestEpisodeArchiveUsesSeasonAndStandardName(t *testing.T) {
	testDatabase(t)
	server := mediaServer(t)
	result, _ := db.Exec("INSERT INTO items(title,link,anime_title,episode) VALUES(?,?,?,?)", "[组] 夏日动画 第07话", server.URL+"/ep07.mp4", "夏日动画", "7")
	id, _ := result.LastInsertId()
	path, err := fetchArchiveContext(context.Background(), int(id), server.URL+"/ep07.mp4")
	if err != nil {
		t.Fatal(err)
	}
	rel := filepath.ToSlash(path)
	trim := filepath.ToSlash(downloadDir) + "/"
	rel = strings.TrimPrefix(rel, trim)
	if rel != "夏日动画/Season 01/夏日动画 - S01E07.mp4" {
		t.Fatalf("episode archive layout: %s", rel)
	}
	for _, name := range []string{"夏日动画 - S01E07.nfo", "夏日动画 - S01E07-info.json"} {
		if _, err := os.Stat(filepath.Join(filepath.Dir(path), name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "info.json")); !os.IsNotExist(err) {
		t.Fatal("legacy info.json must not be created anymore")
	}
}

func TestCustomStorageDirAndSeason(t *testing.T) {
	testDatabase(t)
	server := mediaServer(t)
	custom := filepath.Join(t.TempDir(), "anime库")
	if rec := putSeriesStorage(t, "夏日动画", custom, 2); rec.Code != 200 {
		t.Fatalf("put storage: %d %s", rec.Code, rec.Body.String())
	}
	rec := httptest.NewRecorder()
	animeStorageAPI(rec, httptest.NewRequest("GET", "/api/anime/夏日动画/storage", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"effective_path":`) {
		t.Fatalf("get storage: %d %s", rec.Code, rec.Body.String())
	}

	result, _ := db.Exec("INSERT INTO items(title,link,anime_title,episode) VALUES(?,?,?,?)", "[组] 夏日动画 第05话", server.URL+"/ep05.mp4", "夏日动画", "05")
	id, _ := result.LastInsertId()
	path, err := fetchArchiveContext(context.Background(), int(id), server.URL+"/ep05.mp4")
	if err != nil {
		t.Fatal(err)
	}
	rel := strings.TrimPrefix(filepath.ToSlash(path), filepath.ToSlash(custom)+"/")
	if rel != "Season 02/夏日动画 - S02E05.mp4" {
		t.Fatalf("custom dir layout: %s", rel)
	}

	// 清除自定义目录后回到默认位置。
	if rec := putSeriesStorage(t, "夏日动画", "", 1); rec.Code != 200 {
		t.Fatalf("clear storage: %d %s", rec.Code, rec.Body.String())
	}
	result, _ = db.Exec("INSERT INTO items(title,link,anime_title,episode) VALUES(?,?,?,?)", "[组] 夏日动画 第06话", server.URL+"/ep06.mp4", "夏日动画", "6")
	id, _ = result.LastInsertId()
	path, err = fetchArchiveContext(context.Background(), int(id), server.URL+"/ep06.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(filepath.ToSlash(path), filepath.ToSlash(downloadDir)+"/夏日动画/Season 01/") {
		t.Fatalf("default layout after clearing override: %s", path)
	}
}

func TestStorageAPIRejectsUnsafeDirs(t *testing.T) {
	testDatabase(t)
	for _, dir := range []string{"relative/path", "../escape", ".", "/data/../.."} {
		if rec := putSeriesStorage(t, "某番", dir, 1); rec.Code != 400 {
			t.Fatalf("unsafe dir %q accepted: %d %s", dir, rec.Code, rec.Body.String())
		}
	}
	// Clean 折叠掉 .. 后不再是绝对子路径穿越；但相对路径必须全部拒绝。
	if rec := putSeriesStorage(t, "某番", "/data/动画", 1000); rec.Code != 400 {
		t.Fatalf("season 1000 accepted: %d", rec.Code)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM animes").Scan(&count); err != nil || count != 0 {
		t.Fatalf("invalid requests must not save anything: %d %v", count, err)
	}
}

func TestSameEpisodeConflictGetsUniqueSuffix(t *testing.T) {
	testDatabase(t)
	server := mediaServer(t)
	for _, link := range []string{server.URL + "/v1.mp4", server.URL + "/v2.mp4"} {
		result, _ := db.Exec("INSERT INTO items(title,link,anime_title,episode) VALUES(?,?,?,?)", "[组] 夏日动画 [09]", link, "夏日动画", "09")
		id, _ := result.LastInsertId()
		if _, err := fetchArchiveContext(context.Background(), int(id), link); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(downloadDir, "夏日动画", "Season 01"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, "夏日动画 - S01E09.mp4") || !strings.Contains(joined, "夏日动画 - S01E09 (2).mp4") {
		t.Fatalf("conflict handling: %s", joined)
	}
}

func TestMultiFileResourceLandsInEpisodeFolder(t *testing.T) {
	testDatabase(t)
	local := t.TempDir()
	if err := os.MkdirAll(filepath.Join(local, "EP07", "Subs"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{filepath.Join(local, "EP07", "ep07.mp4"), filepath.Join(local, "EP07", "Subs", "chs.ass")} {
		if err := os.WriteFile(f, []byte("bt"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	result, _ := db.Exec("INSERT INTO items(title,link,anime_title,episode) VALUES(?,?,?,?)", "[组] 夏日动画 [07][BT]", "magnet:?xt=urn:btih:multi", "夏日动画", "07")
	id, _ := result.LastInsertId()
	files := []archiveFile{
		{Rel: "EP07/ep07.mp4", Src: filepath.Join(local, "EP07", "ep07.mp4")},
		{Rel: "EP07/Subs/chs.ass", Src: filepath.Join(local, "EP07", "Subs", "chs.ass")},
	}
	path, err := archiveFromFiles(int(id), files)
	if err != nil {
		t.Fatal(err)
	}
	// 主视频在集目录内按媒体库名重命名，字幕等附属文件保留原名。
	rel := strings.TrimPrefix(filepath.ToSlash(path), filepath.ToSlash(downloadDir)+"/")
	if rel != "夏日动画/Season 01/夏日动画 - S01E07/EP07/夏日动画 - S01E07.mp4" {
		t.Fatalf("multi-file layout: %s", rel)
	}
	folder := filepath.Join(downloadDir, "夏日动画", "Season 01", "夏日动画 - S01E07")
	for _, name := range []string{
		filepath.Join("EP07", "夏日动画 - S01E07.mp4"),
		filepath.Join("EP07", "Subs", "chs.ass"),
		"夏日动画 - S01E07.nfo",
		"夏日动画 - S01E07-info.json",
	} {
		if _, err := os.Stat(filepath.Join(folder, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	// 迁移识别必须能按清单找回改名后的主视频。
	if _, videoRel, _, err := migrationSource(path, id); err != nil || filepath.ToSlash(videoRel) != "EP07/夏日动画 - S01E07.mp4" {
		t.Fatalf("renamed main video not recognized: %q %v", videoRel, err)
	}
}

func TestArchiveTargetUsesLibraryDir(t *testing.T) {
	testDatabase(t)
	old := libraryDir
	t.Cleanup(func() { libraryDir = old })
	libraryDir = filepath.Join(t.TempDir(), "番剧")

	root, seasonDir, base := archiveTarget("夏日动画", "7", "[组] 夏日动画 [07]", 0, 0)
	if seasonDir != "Season 01" || base != "夏日动画 - S01E07" {
		t.Fatalf("target names: %q %q", seasonDir, base)
	}
	if !strings.HasPrefix(filepath.ToSlash(root), filepath.ToSlash(libraryDir)+"/") {
		t.Fatalf("library root ignored: %s", root)
	}

	// 番剧自定义存储位置优先于媒体库目录。
	custom := filepath.Join(t.TempDir(), "custom")
	if rec := putSeriesStorage(t, "夏日动画", custom, 2); rec.Code != 200 {
		t.Fatalf("put storage: %d %s", rec.Code, rec.Body.String())
	}
	if root, _, _ = archiveTarget("夏日动画", "7", "x", 0, 0); filepath.ToSlash(root) != filepath.ToSlash(custom) {
		t.Fatalf("override ignored: %s", root)
	}

	// 清空媒体库目录后回退 DOWNLOAD_DIR（用没有自定义位置的番剧验证）。
	libraryDir = ""
	if root, _, _ = archiveTarget("其他故事", "7", "x", 0, 0); !strings.HasPrefix(filepath.ToSlash(root), filepath.ToSlash(downloadDir)+"/") {
		t.Fatalf("fallback to downloadDir failed: %s", root)
	}
}

func TestSeriesPosterMirrorsCover(t *testing.T) {
	testDatabase(t)
	oldDir := coversDir
	coversDir = t.TempDir()
	t.Cleanup(func() { coversDir = oldDir })
	if err := os.WriteFile(filepath.Join(coversDir, "0123456789abcdef.jpg"), []byte("cover-bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO anime_meta(anime_title,cover_file,source,updated_at) VALUES('夏日动画','0123456789abcdef.jpg','bangumi','2026-09-09T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	writeTvshowNFO(&animeRow{Title: "夏日动画"})
	poster := filepath.Join(downloadDir, "夏日动画", "poster.jpg")
	if data, err := os.ReadFile(poster); err != nil || string(data) != "cover-bytes" {
		t.Fatalf("poster not mirrored: %v", err)
	}
	// 封面缓存清空（如解绑元数据）后，过期海报一并移除。
	if _, err := db.Exec("DELETE FROM anime_meta WHERE anime_title='夏日动画'"); err != nil {
		t.Fatal(err)
	}
	writeTvshowNFO(&animeRow{Title: "夏日动画"})
	if _, err := os.Stat(poster); !os.IsNotExist(err) {
		t.Fatal("stale poster must be removed")
	}
}

func TestBackupCarriesSeriesLibrary(t *testing.T) {
	testDatabase(t)
	seriesDir := filepath.ToSlash(t.TempDir())
	if rec := putSeriesStorage(t, "夏日动画", seriesDir, 3); rec.Code != 200 {
		t.Fatalf("put storage: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := db.Exec("UPDATE animes SET include_rules='夏日',bangumi_id=4242 WHERE title='夏日动画'"); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	backup(rec, httptest.NewRequest("GET", "/api/backup", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"animes":[{`) {
		t.Fatalf("backup export: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := db.Exec("DELETE FROM animes"); err != nil {
		t.Fatal(err)
	}
	result := httptest.NewRecorder()
	restore(result, httptest.NewRequest("POST", "/api/restore", strings.NewReader(rec.Body.String())))
	if result.Code != 200 {
		t.Fatalf("restore: %d %s", result.Code, result.Body.String())
	}
	var dir, rules string
	var season int
	var bangumiID int64
	if err := db.QueryRow(`SELECT COALESCE(storage_dir,''),COALESCE(season,0),COALESCE(include_rules,''),COALESCE(bangumi_id,0) FROM animes WHERE title='夏日动画'`).Scan(&dir, &season, &rules, &bangumiID); err != nil {
		t.Fatal(err)
	}
	if dir != seriesDir || season != 3 || rules == "" || bangumiID != 4242 {
		t.Fatalf("restored series: %q season=%d rules=%q bangumi=%d", dir, season, rules, bangumiID)
	}
}

func TestRestoreAcceptsLegacyV2StorageBackup(t *testing.T) {
	testDatabase(t)
	legacyDir := filepath.ToSlash(t.TempDir())
	body := `{"version":2,"feeds":[],"anime_storages":[{"anime_title":"旧番剧","storage_dir":"` + legacyDir + `","season":2}]}`
	rec := httptest.NewRecorder()
	restore(rec, httptest.NewRequest("POST", "/api/restore", strings.NewReader(body)))
	if rec.Code != 200 {
		t.Fatalf("restore v2: %d %s", rec.Code, rec.Body.String())
	}
	var dir string
	var season int
	if err := db.QueryRow(`SELECT COALESCE(storage_dir,''),COALESCE(season,0) FROM animes WHERE title='旧番剧'`).Scan(&dir, &season); err != nil || dir != legacyDir || season != 2 {
		t.Fatalf("v2 storage restore: %q %d %v", dir, season, err)
	}
}

func TestMigrateAnimeDataUnifiesLegacyRows(t *testing.T) {
	testDatabase(t)
	now := "2026-09-07T00:00:00Z"
	if _, err := db.Exec("INSERT INTO items(title,link,anime_title) VALUES('旧标题 01','https://example.test/legacy-1','遗留动画')"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO anime_storage(anime_title,storage_dir,season,updated_at) VALUES('遗留动画','/volume1/anime/legacy',2,?)", now); err != nil {
		t.Fatal(err)
	}
	if err := migrateAnimeData(); err != nil {
		t.Fatal(err)
	}
	var animeID int64
	var dir string
	var season int
	if err := db.QueryRow(`SELECT COALESCE(anime_id,0) FROM items WHERE link='https://example.test/legacy-1'`).Scan(&animeID); err != nil || animeID == 0 {
		t.Fatalf("item anime_id backfill: %d %v", animeID, err)
	}
	if err := db.QueryRow(`SELECT COALESCE(storage_dir,''),COALESCE(season,0) FROM animes WHERE title='遗留动画'`).Scan(&dir, &season); err != nil || dir != "/volume1/anime/legacy" || season != 2 {
		t.Fatalf("storage override migrated: %q %d %v", dir, season, err)
	}
	var storageCount int
	db.QueryRow("SELECT COUNT(*) FROM anime_storage").Scan(&storageCount)
	if storageCount != 0 {
		t.Fatalf("legacy storage table should be consumed, %d rows left", storageCount)
	}
	// 幂等：重复迁移不产生重复行或错误。
	if err := migrateAnimeData(); err != nil {
		t.Fatal(err)
	}
	var animes, items int
	db.QueryRow("SELECT COUNT(*) FROM animes").Scan(&animes)
	db.QueryRow("SELECT COUNT(*) FROM items").Scan(&items)
	if animes != 1 || items != 1 {
		t.Fatalf("migration not idempotent: animes=%d items=%d", animes, items)
	}
}

// 标题里解析出的季（items.season_auto）决定归档目录与 NFO；番剧设置能覆盖它。
func TestArchiveUsesParsedSeason(t *testing.T) {
	testDatabase(t)
	server := mediaServer(t)
	result, _ := db.Exec("INSERT INTO items(title,link,anime_title,episode,season_auto) VALUES(?,?,?,?,?)",
		"[LoliHouse] 无职转生 3期 / Mushoku Tensei S3 - 01", server.URL+"/e01.mp4", "无职转生 3期", "01", 3)
	id, _ := result.LastInsertId()
	path, err := fetchArchiveContext(context.Background(), int(id), server.URL+"/e01.mp4")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.ToSlash(downloadDir) + "/无职转生 3期/Season 03/无职转生 3期 - S03E01.mp4"
	if filepath.ToSlash(path) != want {
		t.Fatalf("archive path: %s", path)
	}
	nfo, err := os.ReadFile(filepath.Join(filepath.Dir(path), "无职转生 3期 - S03E01.nfo"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(nfo), "<season>3</season>") || !strings.Contains(string(nfo), "<episode>01</episode>") {
		t.Fatalf("NFO 未写入解析出的季集：%s", nfo)
	}
	// 番剧设置指定季号后压过标题解析结果，但用户在该条目上的指定仍然最优先。
	if _, err := ensureAnime("无职转生 3期"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE animes SET season=5 WHERE title='无职转生 3期'"); err != nil {
		t.Fatal(err)
	}
	result, _ = db.Exec("INSERT INTO items(title,link,anime_title,episode,season_auto) VALUES(?,?,?,?,?)",
		"[LoliHouse] 无职转生 3期 / Mushoku Tensei S3 - 02", server.URL+"/e02.mp4", "无职转生 3期", "02", 3)
	id, _ = result.LastInsertId()
	path, err = fetchArchiveContext(context.Background(), int(id), server.URL+"/e02.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if got := filepath.ToSlash(path); got != filepath.ToSlash(downloadDir)+"/无职转生 3期/Season 05/无职转生 3期 - S05E02.mp4" {
		t.Fatalf("番剧设置未压过标题解析季：%s", got)
	}
}

// 标题解析结果直接决定归档目录与季集目录：噪声标签不进番剧文件夹名，
// 英文序数季写进 Season 目录。
func TestArchiveUsesParsedTitleTags(t *testing.T) {
	testDatabase(t)
	server := mediaServer(t)
	cases := []struct {
		title string
		want  string
	}{
		{
			"[Moozzi2] 呪術廻戦 - 01 (BD 1920x1080 x265-10Bit FLAC)",
			"呪術廻戦/Season 01/呪術廻戦 - S01E01.mp4",
		},
		{
			"[sakurato.sub] Tensei shitara Slime Datta Ken 3rd Season - 02 [1080P][CHS]",
			"Tensei shitara Slime Datta Ken/Season 03/Tensei shitara Slime Datta Ken - S03E02.mp4",
		},
	}
	for i, c := range cases {
		info := titleparse.Extract(c.title)
		link := fmt.Sprintf("%s/case%d.mp4", server.URL, i)
		result, err := db.Exec("INSERT INTO items(title,link,anime_title,episode,season_auto) VALUES(?,?,?,?,?)",
			c.title, link, info.Title, info.Episode, info.Season)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := result.LastInsertId()
		path, err := fetchArchiveContext(context.Background(), int(id), link)
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.ToSlash(downloadDir) + "/" + c.want; filepath.ToSlash(path) != want {
			t.Fatalf("archive path = %s, want %s", path, want)
		}
	}
}

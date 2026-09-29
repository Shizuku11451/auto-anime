package main

import (
	"os"
	"path/filepath"
	"testing"
)

// resolveSeason 的优先级：用户指定单条 > 番剧设置 > 标题解析 > 第 1 季。
func TestResolveSeasonPriority(t *testing.T) {
	testDatabase(t)
	id, err := ensureAnime("测试番")
	if err != nil {
		t.Fatal(err)
	}
	check := func(item, auto, want int) {
		t.Helper()
		if got := resolveSeason("测试番", item, auto); got != want {
			t.Errorf("resolveSeason(item=%d, auto=%d) = %d, want %d", item, auto, got, want)
		}
	}
	// 番剧设置为自动：没有标题解析结果时落在第 1 季。
	check(0, 0, 1)
	check(0, 3, 3)
	check(1, 3, 1)
	// 番剧设置指定了季：压过标题解析结果，但不压过用户在该条目上的指定。
	if _, err := db.Exec("UPDATE animes SET season=2 WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	check(0, 0, 2)
	check(0, 5, 2)
	check(3, 5, 3)
}

func mergeSeriesRename(t *testing.T, oldTitle, newTitle string) {
	t.Helper()
	id, err := ensureAnime(oldTitle)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO items(title,link,anime_title,anime_id,episode) VALUES(?,?,?,?,?)",
		"资源一", "https://example.test/e1", oldTitle, id, "01"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO feeds(name,url,target_title) VALUES(?,?,?)",
		"订阅", "https://example.test/rss", oldTitle); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO anime_meta(anime_title,subject_id,name,name_cn,source,rating) VALUES(?,?,?,?,?,?)",
		oldTitle, 501963, "無職転生Ⅲ ～異世界行ったら本気だす～", "无职转生 第三季 ～到了异世界就拿出真本事～", "bangumi", 8.0); err != nil {
		t.Fatal(err)
	}
	if err := renameAnime(oldTitle, newTitle); err != nil {
		t.Fatal(err)
	}
	var title, itemTitle, targetTitle, metaTitle string
	if err := db.QueryRow("SELECT title FROM animes WHERE id=?", id).Scan(&title); err != nil || title != newTitle {
		t.Fatalf("animes.title = %q (%v)", title, err)
	}
	if err := db.QueryRow("SELECT anime_title FROM items WHERE anime_id=?", id).Scan(&itemTitle); err != nil || itemTitle != newTitle {
		t.Fatalf("items.anime_title = %q (%v)", itemTitle, err)
	}
	if err := db.QueryRow("SELECT target_title FROM feeds").Scan(&targetTitle); err != nil || targetTitle != newTitle {
		t.Fatalf("feeds.target_title = %q (%v)", targetTitle, err)
	}
	if err := db.QueryRow("SELECT anime_title FROM anime_meta").Scan(&metaTitle); err != nil || metaTitle != newTitle {
		t.Fatalf("anime_meta.anime_title = %q (%v)", metaTitle, err)
	}
}

func TestRenameAnimeSyncsRelatedRows(t *testing.T) {
	testDatabase(t)
	mergeSeriesRename(t, "无职转生 3期", "无职转生 ～到了异世界就拿出真本事～")
}

// 目录里只有海报/NFO 这类本应用产物时，番剧改名会带着目录一起改。
func TestRenameAnimeMovesDisposableSeriesDir(t *testing.T) {
	testDatabase(t)
	base := t.TempDir()
	oldDir := filepath.Join(base, "无职转生 3期")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"poster.jpg", "tvshow.nfo"} {
		if err := os.WriteFile(filepath.Join(oldDir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mergeSeriesRename(t, "无职转生 3期", "无职转生")
	id := animeIDByTitle(t, "无职转生")
	var storageDir string
	if err := db.QueryRow("SELECT storage_dir FROM animes WHERE id=?", id).Scan(&storageDir); err != nil {
		t.Fatal(err)
	}
	if storageDir != "" {
		t.Fatalf("storage_dir 未设置时不应改写：%q", storageDir)
	}
}

func TestRenameAnimeRelocatesEmptySeriesDir(t *testing.T) {
	testDatabase(t)
	base := t.TempDir()
	oldDir := filepath.Join(base, "无职转生 3期")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "poster.jpg"), []byte("cover"), 0o644); err != nil {
		t.Fatal(err)
	}
	id, err := ensureAnime("无职转生 3期")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE animes SET storage_dir=? WHERE id=?", oldDir, id); err != nil {
		t.Fatal(err)
	}
	newTitle := "无职转生 ～到了异世界就拿出真本事～"
	if err := renameAnime("无职转生 3期", newTitle); err != nil {
		t.Fatal(err)
	}
	newDir := filepath.Join(base, safeComponent(newTitle))
	if data, err := os.ReadFile(filepath.Join(newDir, "poster.jpg")); err != nil || string(data) != "cover" {
		t.Fatalf("海报没有跟着目录迁移: %v", err)
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatalf("旧目录仍然存在: %v", err)
	}
	var storageDir string
	if err := db.QueryRow("SELECT storage_dir FROM animes WHERE id=?", id).Scan(&storageDir); err != nil || !samePath(storageDir, newDir) {
		t.Fatalf("storage_dir = %q (%v), want %q", storageDir, err, newDir)
	}
}

// 目录里已经有媒体文件时不搬动，避免媒体库扫描期间移动数据；storage_dir 保持原值。
func TestRenameAnimeKeepsSeriesDirWithMedia(t *testing.T) {
	testDatabase(t)
	base := t.TempDir()
	oldDir := filepath.Join(base, "无职转生 3期")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "无职转生 3期 - S01E01.mkv"), []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	id, err := ensureAnime("无职转生 3期")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE animes SET storage_dir=? WHERE id=?", oldDir, id); err != nil {
		t.Fatal(err)
	}
	if err := renameAnime("无职转生 3期", "无职转生"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(oldDir, "无职转生 3期 - S01E01.mkv")); err != nil {
		t.Fatalf("媒体文件被移动了: %v", err)
	}
	var storageDir string
	if err := db.QueryRow("SELECT storage_dir FROM animes WHERE id=?", id).Scan(&storageDir); err != nil || storageDir != oldDir {
		t.Fatalf("storage_dir = %q (%v), want %q", storageDir, err, oldDir)
	}
}

// 资料匹配成功后番剧名自动规范成官方名，目录（只有海报）跟着改。
func TestStandardizeAnimeTitleUsesOfficialName(t *testing.T) {
	testDatabase(t)
	base := t.TempDir()
	oldDir := filepath.Join(base, "无职转生 3期")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	id, err := ensureAnime("无职转生 3期")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE animes SET storage_dir=? WHERE id=?", oldDir, id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO anime_meta(anime_title,subject_id,name,name_cn,source,rating) VALUES(?,?,?,?,?,?)`,
		"无职转生 3期", 501963, "無職転生Ⅲ ～異世界行ったら本気だす～", "无职转生 第三季 ～到了异世界就拿出真本事～", "bangumi", 8.0); err != nil {
		t.Fatal(err)
	}
	standardizeAnimeTitle("无职转生 3期")
	want := "无职转生 ～到了异世界就拿出真本事～"
	if got := animeTitleByID(t, id); got != want {
		t.Fatalf("番剧名 = %q, want %q", got, want)
	}
	var metaTitle string
	if err := db.QueryRow("SELECT anime_title FROM anime_meta").Scan(&metaTitle); err != nil || metaTitle != want {
		t.Fatalf("anime_meta.anime_title = %q (%v)", metaTitle, err)
	}
	if _, err := os.Stat(filepath.Join(base, safeComponent(want))); err != nil {
		t.Fatalf("番剧目录没有跟着规范化: %v", err)
	}
	// 没有资料时不做任何改动。
	standardizeAnimeTitle(want)
	if got := animeTitleByID(t, id); got != want {
		t.Fatalf("无资料时不应改名: %q", got)
	}
}

func TestRetargetSeriesDirPath(t *testing.T) {
	if got := retargetSeriesDirPath("/vol1/1000/番剧/无职转生 3期", "无职转生 3期", "无职转生"); got != "/vol1/1000/番剧/无职转生" {
		t.Fatalf("retarget = %q", got)
	}
	// 用户自定义过的目录名与番剧名不一致时不动。
	if got := retargetSeriesDirPath("/vol1/1000/番剧/我的收藏", "无职转生 3期", "无职转生"); got != "" {
		t.Fatalf("自定义目录不应改写: %q", got)
	}
	if got := retargetSeriesDirPath("", "无职转生 3期", "无职转生"); got != "" {
		t.Fatalf("空路径不应改写: %q", got)
	}
}

// 数据库部分失败时目录必须挪回原名：库和目录不能各留一个名字。
func TestRenameAnimeRestoresSeriesDirWhenDatabaseFails(t *testing.T) {
	testDatabase(t)
	base := t.TempDir()
	oldDir := filepath.Join(base, "无职转生 3期")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "poster.jpg"), []byte("cover"), 0o644); err != nil {
		t.Fatal(err)
	}
	id, err := ensureAnime("无职转生 3期")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE animes SET storage_dir=? WHERE id=?", oldDir, id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER fail_rename BEFORE UPDATE OF title ON animes BEGIN SELECT RAISE(ABORT,'test rollback'); END`); err != nil {
		t.Fatal(err)
	}
	if err := renameAnime("无职转生 3期", "无职转生"); err == nil {
		t.Fatal("数据库失败时改名应当报错")
	}
	if data, err := os.ReadFile(filepath.Join(oldDir, "poster.jpg")); err != nil || string(data) != "cover" {
		t.Fatalf("目录没有挪回原名: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, safeComponent("无职转生"))); !os.IsNotExist(err) {
		t.Fatalf("失败后仍留下新名字的目录: %v", err)
	}
	var storageDir string
	if err := db.QueryRow("SELECT storage_dir FROM animes WHERE id=?", id).Scan(&storageDir); err != nil || storageDir != oldDir {
		t.Fatalf("storage_dir = %q (%v), want %q", storageDir, err, oldDir)
	}
	if got := animeTitleByID(t, id); got != "无职转生 3期" {
		t.Fatalf("番剧名被改动: %q", got)
	}
}

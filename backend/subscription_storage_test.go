package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

// samePath 比较两个路径是否指向同一位置：容器内是 POSIX 路径，Windows 本机
// 可能是盘符，比较前统一为斜杠形式。
func samePath(a, b string) bool {
	return path.Clean(filepath.ToSlash(a)) == path.Clean(filepath.ToSlash(b))
}

func postFeed(t *testing.T, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	feeds(rec, httptest.NewRequest("POST", "/api/feeds", strings.NewReader(string(raw))))
	return rec
}

func animeStorage(t *testing.T, title string) string {
	t.Helper()
	var dir string
	if err := db.QueryRow("SELECT COALESCE(storage_dir,'') FROM animes WHERE title=?", title).Scan(&dir); err != nil {
		t.Fatalf("读取番剧存储位置失败 %q: %v", title, err)
	}
	return dir
}

// 找番添加订阅时选择保存位置：目录写入订阅与番剧记录，并预建番剧文件夹，
// 归档位置随之改变。
func TestSubscriptionStorageCreatesSeriesFolder(t *testing.T) {
	testDatabase(t)
	base := filepath.ToSlash(t.TempDir())
	rec := postFeed(t, map[string]any{
		"name": "夏日动画 · 目标组", "url": "https://example.test/rss",
		"target_title": "夏日动画", "storage_dir": base,
	})
	var created struct {
		ID             int    `json:"id"`
		StoragePath    string `json:"storage_path"`
		StorageWarning string `json:"storage_warning"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.ID == 0 {
		t.Fatalf("create: %s", rec.Body.String())
	}
	want := filepath.Join(filepath.FromSlash(base), "夏日动画")
	if created.StorageWarning != "" || !samePath(created.StoragePath, want) {
		t.Fatalf("storage path=%q warning=%q want=%q", created.StoragePath, created.StorageWarning, want)
	}
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Fatalf("番剧文件夹没有建立: %v", err)
	}
	var feedDir string
	if err := db.QueryRow("SELECT COALESCE(storage_dir,'') FROM feeds WHERE id=?", created.ID).Scan(&feedDir); err != nil || !samePath(feedDir, base) {
		t.Fatalf("订阅保存位置=%q want=%q err=%v", feedDir, base, err)
	}
	if got := animeStorage(t, "夏日动画"); !samePath(got, want) {
		t.Fatalf("番剧存储位置=%q want=%q", got, want)
	}
	// 归档位置与选择一致：<所选目录>/<番剧名>/Season XX/…
	if got := archiveRootFor("夏日动画"); !samePath(got, want) {
		t.Fatalf("归档目录=%q want=%q", got, want)
	}
	seriesRoot, seasonDir, fileBase := archiveTarget("夏日动画", "7", "[目标组] 夏日动画 [07] [1080p]", 0)
	if !samePath(seriesRoot, want) || seasonDir != "Season 01" || fileBase != "夏日动画 - S01E07" {
		t.Fatalf("归档目标=%q %q %q", seriesRoot, seasonDir, fileBase)
	}
}

// 非法保存位置必须被拒绝，且不能留下半个订阅。
func TestSubscriptionStorageRejectsUnsafePaths(t *testing.T) {
	testDatabase(t)
	for i, raw := range []string{"番剧", "vol1/anime", "../etc", "/vol1/../etc", "C:"} {
		rec := postFeed(t, map[string]any{
			"name": "订阅", "url": fmt.Sprintf("https://example.test/rss/%d", i), "storage_dir": raw,
		})
		if rec.Code != 400 || !strings.Contains(rec.Body.String(), "保存位置") {
			t.Fatalf("保存位置 %q 应当被拒绝: %d %s", raw, rec.Code, rec.Body.String())
		}
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM feeds").Scan(&count)
	if count != 0 {
		t.Fatalf("非法保存位置不应写入订阅，实际 %d 条", count)
	}
}

// 番剧名里的分隔符不能把文件带出所选目录。
func TestSubscriptionStorageKeepsTitleInsideBase(t *testing.T) {
	testDatabase(t)
	root := filepath.ToSlash(t.TempDir())
	rec := postFeed(t, map[string]any{
		"name": "订阅", "url": "https://example.test/rss",
		"target_title": "../逃逸/夏日动画", "storage_dir": root,
	})
	if rec.Code != 200 {
		t.Fatalf("create: %s", rec.Body.String())
	}
	got := animeStorage(t, "../逃逸/夏日动画")
	if !strings.HasPrefix(filepath.ToSlash(got), path.Clean(root)+"/") {
		t.Fatalf("番剧目录逃出所选根目录: %q", got)
	}
	if strings.Contains(path.Base(filepath.ToSlash(got)), "..") {
		t.Fatalf("番剧目录仍含 ..: %q", got)
	}
}

// 编辑订阅：清空保存位置回到默认位置；未携带该字段时保持原值。
func TestSubscriptionStorageEditClearsAndKeeps(t *testing.T) {
	testDatabase(t)
	base := filepath.ToSlash(t.TempDir())
	rec := postFeed(t, map[string]any{
		"name": "订阅", "url": "https://example.test/rss",
		"target_title": "夏日动画", "storage_dir": base,
	})
	var created struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.ID == 0 {
		t.Fatalf("create: %s", rec.Body.String())
	}
	update := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		out := httptest.NewRecorder()
		updateFeed(out, httptest.NewRequest("PUT", fmt.Sprintf("/api/feeds/%d", created.ID), strings.NewReader(body)))
		return out
	}
	// 只改订阅名：不携带保存位置，番剧目录保持不动。
	if out := update(`{"name":"订阅改名","target_title":"夏日动画"}`); out.Code != 200 {
		t.Fatalf("rename: %s", out.Body.String())
	}
	if got := animeStorage(t, "夏日动画"); !samePath(got, filepath.Join(filepath.FromSlash(base), "夏日动画")) {
		t.Fatalf("改名后番剧目录被改写: %q", got)
	}
	seriesDir := filepath.Join(filepath.FromSlash(base), "夏日动画")
	// 清空保存位置：番剧回到默认位置（媒体库目录）。
	if out := update(`{"name":"订阅改名","target_title":"夏日动画","storage_dir":""}`); out.Code != 200 {
		t.Fatalf("clear: %s", out.Body.String())
	}
	if got := animeStorage(t, "夏日动画"); got != "" {
		t.Fatalf("清空保存位置后应回到默认位置，实际 %q", got)
	}
	// 重新指定保存位置后，用户在番剧库里单独设置的位置不能被本订阅清空。
	if out := update(fmt.Sprintf(`{"name":"订阅改名","target_title":"夏日动画","storage_dir":%q}`, base)); out.Code != 200 {
		t.Fatalf("set: %s", out.Body.String())
	}
	if got := animeStorage(t, "夏日动画"); !samePath(got, seriesDir) {
		t.Fatalf("重新指定保存位置失败: %q", got)
	}
	manual := filepath.ToSlash(filepath.Join(t.TempDir(), "手动位置"))
	if _, err := db.Exec("UPDATE animes SET storage_dir=? WHERE title='夏日动画'", manual); err != nil {
		t.Fatal(err)
	}
	if out := update(`{"name":"订阅改名","target_title":"夏日动画","storage_dir":""}`); out.Code != 200 {
		t.Fatalf("clear: %s", out.Body.String())
	}
	if got := animeStorage(t, "夏日动画"); !samePath(got, manual) {
		t.Fatalf("清空订阅保存位置时覆盖了手动设置: %q", got)
	}
}

// 换保存位置时提示原位置，并说明已归档文件不会自动迁移。
func TestSubscriptionStorageReportsReplacedLocation(t *testing.T) {
	testDatabase(t)
	first := filepath.ToSlash(t.TempDir())
	second := filepath.ToSlash(t.TempDir())
	if rec := postFeed(t, map[string]any{"name": "订阅", "url": "https://example.test/rss", "target_title": "夏日动画", "storage_dir": first}); rec.Code != 200 {
		t.Fatalf("create: %s", rec.Body.String())
	}
	rec := postFeed(t, map[string]any{"name": "订阅2", "url": "https://example.test/rss2", "target_title": "夏日动画", "storage_dir": second})
	var result struct {
		StoragePath    string `json:"storage_path"`
		StorageWarning string `json:"storage_warning"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !samePath(result.StoragePath, filepath.Join(filepath.FromSlash(second), "夏日动画")) || !strings.Contains(result.StorageWarning, "不会自动迁移") {
		t.Fatalf("path=%q warning=%q", result.StoragePath, result.StorageWarning)
	}
	if got := animeStorage(t, "夏日动画"); !samePath(got, filepath.Join(filepath.FromSlash(second), "夏日动画")) {
		t.Fatalf("番剧存储位置=%q", got)
	}
}

// 无固定番剧的订阅（按标题自动识别）同步时套用保存位置，但不覆盖已有设置。
func TestSyncAppliesSubscriptionStorage(t *testing.T) {
	testDatabase(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<rss><channel>
<item><title>[目标组] 夏日动画 [07]</title><link>https://example.test/7</link></item>
<item><title>[目标组] 秋日动画 [03]</title><link>https://example.test/3</link></item>
</channel></rss>`)
	}))
	defer server.Close()
	base := filepath.ToSlash(t.TempDir())
	if _, err := db.Exec("INSERT INTO feeds(id,name,url,include_words,storage_dir) VALUES(1,'订阅',?,'',?)", server.URL, base); err != nil {
		t.Fatal(err)
	}
	manual := filepath.ToSlash(filepath.Join(t.TempDir(), "手动位置"))
	if _, err := db.Exec("INSERT INTO animes(title,storage_dir) VALUES('秋日动画',?)", manual); err != nil {
		t.Fatal(err)
	}
	if n, err := syncFeed(1); err != nil || n != 2 {
		t.Fatalf("sync: %d %v", n, err)
	}
	if got := animeStorage(t, "夏日动画"); !samePath(got, filepath.Join(filepath.FromSlash(base), "夏日动画")) {
		t.Fatalf("新识别番剧没有套用订阅保存位置: %q", got)
	}
	if got := animeStorage(t, "秋日动画"); !samePath(got, manual) {
		t.Fatalf("同步覆盖了手动设置的番剧位置: %q", got)
	}
}

// 备份与恢复保留订阅保存位置，非法值只丢弃该字段。
func TestSubscriptionStorageBackupRoundTrip(t *testing.T) {
	testDatabase(t)
	base := filepath.ToSlash(t.TempDir())
	if rec := postFeed(t, map[string]any{"name": "订阅", "url": "https://example.test/rss", "storage_dir": base}); rec.Code != 200 {
		t.Fatalf("create: %s", rec.Body.String())
	}
	backupRec := httptest.NewRecorder()
	backup(backupRec, httptest.NewRequest("GET", "/api/backup", nil))
	if _, err := db.Exec("UPDATE feeds SET storage_dir=''"); err != nil {
		t.Fatal(err)
	}
	out := httptest.NewRecorder()
	restore(out, httptest.NewRequest("POST", "/api/restore", strings.NewReader(backupRec.Body.String())))
	if out.Code != 200 {
		t.Fatalf("restore: %s", out.Body.String())
	}
	var got string
	if err := db.QueryRow("SELECT COALESCE(storage_dir,'') FROM feeds").Scan(&got); err != nil || !samePath(got, base) {
		t.Fatalf("恢复后保存位置=%q want=%q err=%v", got, base, err)
	}
}

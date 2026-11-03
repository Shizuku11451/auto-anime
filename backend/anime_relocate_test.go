package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func animeIDOf(t *testing.T, itemID int64) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow("SELECT anime_id FROM items WHERE id=?", itemID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func animeRequest(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	animeEntryAPI(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func waitRelocation(t *testing.T, animeID int64) relocationStatus {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if st := relocationSnapshot(animeID); !st.Running {
			return st
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("文件迁移没有结束")
	return relocationStatus{}
}

func savedPath(t *testing.T, itemID int64) string {
	t.Helper()
	var p string
	if err := db.QueryRow("SELECT file_path FROM items WHERE id=?", itemID).Scan(&p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRelocationPreviewAndSeasonMove(t *testing.T) {
	settingsFixture(t)
	itemID, oldPath := savedFixture(t, "Relo", 0, false)
	animeID := animeIDOf(t, itemID)

	rec := animeRequest(t, "POST", fmt.Sprintf("/api/animes/%d/relocation-preview", animeID), `{"season":2}`)
	var plan relocationPlan
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &plan) != nil || plan.Saved != 1 || plan.Moving != 1 || len(plan.From) != 1 {
		t.Fatalf("preview: %d %s", rec.Code, rec.Body.String())
	}
	// 预览不改动任何内容。
	if savedPath(t, itemID) != oldPath {
		t.Fatal("预览不应修改记录")
	}
	if rec := animeRequest(t, "POST", fmt.Sprintf("/api/animes/%d/relocation-preview", animeID), `{}`); !strings.Contains(rec.Body.String(), `"moving":0`) {
		t.Fatalf("当前设置下不应有需要移动的文件: %s", rec.Body.String())
	}

	if rec := animeRequest(t, "PUT", fmt.Sprintf("/api/animes/%d", animeID), `{"season":2}`); rec.Code != 200 {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	if rec := animeRequest(t, "POST", fmt.Sprintf("/api/animes/%d/relocation", animeID), ""); rec.Code != 200 {
		t.Fatalf("relocate: %d %s", rec.Code, rec.Body.String())
	}
	st := waitRelocation(t, animeID)
	if st.Moved != 1 || len(st.Failed) != 0 {
		t.Fatalf("relocation status: %+v", st)
	}
	newPath := savedPath(t, itemID)
	if !strings.Contains(filepath.ToSlash(newPath), "/Season 02/") {
		t.Fatalf("文件应移到 Season 02: %s", newPath)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("旧文件应被清理: %v", err)
	}
	// 再次执行时没有需要移动的文件。
	animeRequest(t, "POST", fmt.Sprintf("/api/animes/%d/relocation", animeID), "")
	if st := waitRelocation(t, animeID); st.Total != 0 {
		t.Fatalf("文件已在新位置时不应再次移动: %+v", st)
	}
}

func TestRelocationAfterRenameRetiresOldFolder(t *testing.T) {
	settingsFixture(t)
	itemID, oldPath := savedFixture(t, "OldName", 0, true)
	animeID := animeIDOf(t, itemID)
	oldRoot := seriesRoot("OldName", "")
	if err := os.WriteFile(filepath.Join(oldRoot, "poster.jpg"), []byte("poster"), 0644); err != nil {
		t.Fatal(err)
	}

	if rec := animeRequest(t, "PUT", fmt.Sprintf("/api/animes/%d", animeID), `{"title":"NewName"}`); rec.Code != 200 {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body.String())
	}
	animeRequest(t, "POST", fmt.Sprintf("/api/animes/%d/relocation", animeID), "")
	if st := waitRelocation(t, animeID); st.Moved != 1 || len(st.Failed) != 0 {
		t.Fatalf("relocation status: %+v", st)
	}
	newRoot := seriesRoot("NewName", "")
	if newPath := savedPath(t, itemID); !pathWithin(newPath, newRoot) {
		t.Fatalf("文件应移到新番剧文件夹: %s", newPath)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("旧文件应被清理: %v", err)
	}
	if content, err := os.ReadFile(filepath.Join(newRoot, "poster.jpg")); err != nil || string(content) != "poster" {
		t.Fatalf("海报应随番剧文件夹迁移: %v", err)
	}
	if _, err := os.Stat(oldRoot); !os.IsNotExist(err) {
		t.Fatalf("只剩本应用产物的旧文件夹应删除: %v", err)
	}
}

func TestRetireSeriesDirKeepsForeignContent(t *testing.T) {
	settingsFixture(t)
	oldDir, newDir := filepath.Join(downloadDir, "A"), filepath.Join(downloadDir, "B")
	for _, dir := range []string{oldDir, newDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(oldDir, "poster.jpg"), []byte("p"), 0644)
	os.WriteFile(filepath.Join(oldDir, "notes.txt"), []byte("user"), 0644)
	retireSeriesDir(oldDir, newDir)
	if _, err := os.Stat(filepath.Join(oldDir, "notes.txt")); err != nil {
		t.Fatal("含其他文件的旧目录必须保持不动")
	}
	retireSeriesDir(downloadDir, newDir)
	if _, err := os.Stat(downloadDir); err != nil {
		t.Fatal("下载目录本身不能被清理")
	}
}

func TestDeleteItemWithFiles(t *testing.T) {
	settingsFixture(t)
	keepID, keepPath := savedFixture(t, "Keep", 0, false)
	rec := httptest.NewRecorder()
	itemsEntryAPI(rec, httptest.NewRequest("DELETE", fmt.Sprintf("/api/items/%d", keepID), nil))
	if rec.Code != 200 {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(keepPath); err != nil {
		t.Fatal("未选择删除文件时必须保留媒体")
	}

	id, p := savedFixture(t, "Drop", 0, false)
	info := strings.TrimSuffix(p, filepath.Ext(p)) + "-info.json"
	rec = httptest.NewRecorder()
	itemsEntryAPI(rec, httptest.NewRequest("DELETE", fmt.Sprintf("/api/items/%d?delete_files=1", id), nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"deleted_files":1`) {
		t.Fatalf("delete files: %d %s", rec.Code, rec.Body.String())
	}
	for _, f := range []string{p, info} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Fatalf("%s 应被删除: %v", f, err)
		}
	}

	// 媒体目录之外的文件即使被记录引用也不删除。
	outside := filepath.Join(t.TempDir(), "outside.mkv")
	os.WriteFile(outside, []byte("x"), 0644)
	result, err := db.Exec("INSERT INTO items(title,link,status,file_path) VALUES('Outside','magnet:?xt=urn:btih:outside','已保存',?)", outside)
	if err != nil {
		t.Fatal(err)
	}
	outsideID, _ := result.LastInsertId()
	rec = httptest.NewRecorder()
	itemsEntryAPI(rec, httptest.NewRequest("DELETE", fmt.Sprintf("/api/items/%d?delete_files=1", outsideID), nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "file_errors") {
		t.Fatalf("outside delete: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("媒体目录之外的文件必须保留")
	}
}

func TestManageAnimeDeleteFilesAndMergeMove(t *testing.T) {
	settingsFixture(t)
	itemID, p := savedFixture(t, "Gone", 0, true)
	animeID := animeIDOf(t, itemID)
	out, code, err := manageAnime(animeManageInput{Action: "delete", IDs: []int64{animeID}, DeleteItems: true, DeleteFiles: true})
	if err != nil || code != 200 || out.Deleted != 1 {
		t.Fatalf("delete anime: %d %v %+v", code, err, out)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("选择删除文件后媒体应删除: %v", err)
	}

	srcItem, srcPath := savedFixture(t, "MergeSource", 0, false)
	source := animeIDOf(t, srcItem)
	target, err := ensureAnime("MergeTarget")
	if err != nil {
		t.Fatal(err)
	}
	out, code, err = manageAnime(animeManageInput{Action: "merge", IDs: []int64{source}, TargetID: target, MoveFiles: true})
	if err != nil || code != 200 || out.Relocation == nil || out.Relocation.Total != 1 {
		t.Fatalf("merge: %d %v %+v", code, err, out)
	}
	if st := waitRelocation(t, target); st.Moved != 1 {
		t.Fatalf("merge relocation: %+v", st)
	}
	if newPath := savedPath(t, srcItem); !pathWithin(newPath, seriesRoot("MergeTarget", "")) {
		t.Fatalf("合并后文件应移到目标番剧文件夹: %s", newPath)
	}
	if _, err := os.Stat(srcPath); !os.IsNotExist(err) {
		t.Fatalf("旧文件应被清理: %v", err)
	}
}

package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// seedSavedItem 插入一条已保存条目并把内容写到临时文件。
func seedSavedItem(t *testing.T, path string, status string) int64 {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("0123456789", 100)), 0644); err != nil {
		t.Fatal(err)
	}
	r, err := db.Exec("INSERT INTO items(title,link,status,file_path) VALUES(?,?,?,?)",
		"测试动画 第01话", "http://example.test/"+filepath.Base(path), status, path)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := r.LastInsertId()
	return id
}

func streamRequest(id int64, method string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	streamItem(rec, httptest.NewRequest(method, "/api/items/"+strconv.FormatInt(id, 10)+"/stream", nil), id)
	return rec
}

func TestStreamServesRangeRequests(t *testing.T) {
	testDatabase(t)
	path := filepath.Join(t.TempDir(), "lib", "测试动画", "Season 01", "测试动画 - S01E01.mp4")
	id := seedSavedItem(t, path, "已保存")

	rec := streamRequest(id, "GET")
	if rec.Code != 200 {
		t.Fatalf("full GET status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "video/mp4" {
		t.Fatalf("Content-Type = %q", ct)
	}
	if ar := rec.Header().Get("Accept-Ranges"); ar != "bytes" {
		t.Fatalf("Accept-Ranges = %q", ar)
	}
	if body := rec.Body.String(); len(body) != 1000 {
		t.Fatalf("full body length = %d", len(body))
	}

	// 区间请求返回 206 与对应字节段，支持拖动进度。
	req := httptest.NewRequest("GET", "/api/items/1/stream", nil)
	req.Header.Set("Range", "bytes=100-199")
	rec = httptest.NewRecorder()
	streamItem(rec, req, id)
	if rec.Code != 206 {
		t.Fatalf("range GET status = %d", rec.Code)
	}
	if cr := rec.Header().Get("Content-Range"); cr != "bytes 100-199/1000" {
		t.Fatalf("Content-Range = %q", cr)
	}
	if got := rec.Body.String(); got != strings.Repeat("0123456789", 10) {
		t.Fatalf("range body mismatch: %q", got[:min(20, len(got))])
	}

	// 开区间与后缀区间。
	for _, tc := range []struct{ rng, want string }{
		{"bytes=990-", "0123456789"},
		{"bytes=-25", "56789" + strings.Repeat("0123456789", 2)},
	} {
		req := httptest.NewRequest("GET", "/api/items/1/stream", nil)
		req.Header.Set("Range", tc.rng)
		rec = httptest.NewRecorder()
		streamItem(rec, req, id)
		if rec.Code != 206 {
			t.Fatalf("%s status = %d", tc.rng, rec.Code)
		}
		if got := rec.Body.String(); got != tc.want {
			t.Fatalf("%s body mismatch", tc.rng)
		}
	}

	// HEAD 不带正文但响应头一致。
	rec = streamRequest(id, "HEAD")
	if rec.Code != 200 || rec.Body.Len() != 0 {
		t.Fatalf("HEAD status = %d body = %d", rec.Code, rec.Body.Len())
	}
}

func TestStreamContentTypeByExtension(t *testing.T) {
	testDatabase(t)
	dir := t.TempDir()
	for _, ext := range []string{".mkv", ".webm"} {
		id := seedSavedItem(t, filepath.Join(dir, "ep"+ext), "已保存")
		rec := streamRequest(id, "GET")
		want := map[string]string{".mkv": "video/x-matroska", ".webm": "video/webm"}[ext]
		if rec.Code != 200 || rec.Header().Get("Content-Type") != want {
			t.Fatalf("%s: status=%d Content-Type=%q", ext, rec.Code, rec.Header().Get("Content-Type"))
		}
	}
}

func TestStreamRejectsUnplayable(t *testing.T) {
	testDatabase(t)
	path := filepath.Join(t.TempDir(), "lib", "ep01.mp4")

	if rec := streamRequest(12345, "GET"); rec.Code != 404 {
		t.Fatalf("unknown id status = %d", rec.Code)
	}

	// 未保存条目不可播。
	id := seedSavedItem(t, path, "待下载")
	if rec := streamRequest(id, "GET"); rec.Code != 404 || !strings.Contains(rec.Body.String(), "已保存") {
		t.Fatalf("unsaved status = %d body = %q", rec.Code, rec.Body.String())
	}

	// 文件被移走后返回 404 而不是崩溃。
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if rec := streamRequest(id, "GET"); rec.Code != 404 {
		t.Fatalf("missing file status = %d", rec.Code)
	}

	if rec := streamRequest(id, "POST"); rec.Code != 405 {
		t.Fatalf("POST status = %d", rec.Code)
	}
}

// 旧的嵌套归档记录 file_path 指向集文件夹：自动挑其中最大的视频文件。
func TestStreamPicksMainVideoFromDirectory(t *testing.T) {
	testDatabase(t)
	dir := filepath.Join(t.TempDir(), "lib", "ep01")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "subs.chs.srt"), []byte("sub"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "small.mp4"), []byte("sm"), 0644); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(dir, "测试动画 - S01E01.mkv")
	if err := os.WriteFile(main, []byte("main-video-bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	r, err := db.Exec("INSERT INTO items(title,link,status,file_path) VALUES(?,?,?,?)",
		"测试动画 第01话", "http://example.test/ep01", "已保存", dir)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := r.LastInsertId()

	rec := streamRequest(id, "GET")
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "video/x-matroska" {
		t.Fatalf("Content-Type = %q", ct)
	}
	if got := rec.Body.String(); got != "main-video-bytes" {
		t.Fatalf("served wrong file: %q", got)
	}
}

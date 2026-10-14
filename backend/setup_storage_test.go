package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSetupStorageProbeCleansTemporaryFile(t *testing.T) {
	useMountInfo(t, "")
	dir := t.TempDir()
	out := probeSetupStorage(dir)
	if runtime.GOOS == "windows" {
		// Windows 本机没有容器卷映射：探测只验证目录存在与可写。
		if !out.Writable || out.MountStatus != "not_required" || !strings.Contains(out.Message, "无需容器映射") {
			t.Fatalf("unexpected result: %+v", out)
		}
	} else if !out.Writable || out.MountStatus != "unknown" || !strings.Contains(out.Message, "无法确认挂载") {
		t.Fatalf("unexpected result: %+v", out)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("probe left files: %v %v", entries, err)
	}
}

func TestSetupStorageDoesNotCreateMissingDirectory(t *testing.T) {
	useMountInfo(t, "")
	dir := filepath.Join(t.TempDir(), "missing")
	out := probeSetupStorage(dir)
	if out.Writable || !strings.Contains(out.Message, "不存在") {
		t.Fatalf("unexpected result: %+v", out)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("probe created a directory")
	}
	file := filepath.Join(t.TempDir(), "not-directory")
	if err := os.WriteFile(file, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if probeSetupStorage(file).Writable {
		t.Fatal("file reported writable directory")
	}
	if data, _ := os.ReadFile(file); string(data) != "keep" {
		t.Fatal("existing file changed")
	}
}

func TestSetupStorageUsesOnlyDeploymentRootAndClosesAfterSetup(t *testing.T) {
	settingsFixture(t)
	setWebAuth(t, "", "")
	setupDone = false
	old := libraryDir
	libraryDir = t.TempDir()
	t.Cleanup(func() { libraryDir = old })
	useMountInfo(t, "")
	r := httptest.NewRequest("POST", "http://nas.test/api/setup/storage", strings.NewReader(`{"path":"/unexpected"}`))
	r.Header.Set("Origin", "http://nas.test")
	rec := httptest.NewRecorder()
	auth(http.HandlerFunc(setupStorageHandler)).ServeHTTP(rec, r)
	var out setupStorageResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || !samePath(out.Path, libraryDir) || !out.Writable {
		t.Fatalf("result: %d %+v", rec.Code, out)
	}
	setupDone = true
	rec = httptest.NewRecorder()
	setupStorageHandler(rec, r)
	if rec.Code != 403 {
		t.Fatalf("initialized status: %d", rec.Code)
	}
}

func TestSetupStorageMountChecksBeforeWriting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("容器挂载检测仅适用于容器/Linux 环境")
	}
	dir := t.TempDir()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	root := "36 25 0:32 / / rw - overlay overlay rw\n"
	useMountInfo(t, root)
	out := probeSetupStorage(dir)
	if out.Writable || out.MountStatus != "unmapped" {
		t.Fatalf("unmapped: %+v", out)
	}
	escaped := strings.NewReplacer("\\", "\\134", " ", "\\040").Replace(filepath.ToSlash(resolved))
	useMountInfo(t, root+"123 36 8:1 / "+escaped+" rw - ext4 /dev/sda1 rw\n")
	out = probeSetupStorage(dir)
	if !out.Writable || out.MountStatus != "mapped" {
		t.Fatalf("mapped: %+v", out)
	}
	useMountInfo(t, root+"123 36 0:1 / "+escaped+" rw - tmpfs tmpfs rw\n")
	out = probeSetupStorage(dir)
	if out.Writable || out.MountStatus != "unmapped" {
		t.Fatalf("tmpfs: %+v", out)
	}
}

func TestSetupWritesRejectCrossOriginBeforeChangingSettings(t *testing.T) {
	settingsFixture(t)
	setWebAuth(t, "", "")
	setupDone = false
	for _, origin := range []string{"http://evil.test", "null", "http://nas.test:9999", "http://nas.test/path", "ftp://nas.test"} {
		for _, handler := range []func(http.ResponseWriter, *http.Request){setupHandler, setupStorageHandler} {
			r := httptest.NewRequest("POST", "http://nas.test/api/setup", strings.NewReader(`{}`))
			r.Header.Set("Origin", origin)
			rec := httptest.NewRecorder()
			handler(rec, r)
			if rec.Code != 403 || setupDone {
				t.Fatalf("origin %q: %d", origin, rec.Code)
			}
		}
	}
}

func TestSettingsStorageRequiresAuthAndCleansProbe(t *testing.T) {
	settingsFixture(t)
	useMountInfo(t, "")
	dir := filepath.ToSlash(t.TempDir())
	body, _ := json.Marshal(map[string]string{"path": dir})
	handler := auth(http.HandlerFunc(settingsStorageHandler))
	request := func(authorized bool, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://nas.test/api/settings/storage", strings.NewReader(string(body)))
		if authorized {
			r.SetBasicAuth("admin", "test-password")
		}
		r.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, r)
		return rec
	}
	if rec := request(false, "http://nas.test"); rec.Code != 401 {
		t.Fatalf("anonymous: %d", rec.Code)
	}
	if rec := request(true, "http://evil.test"); rec.Code != 403 {
		t.Fatalf("cross origin: %d", rec.Code)
	}
	rec := request(true, "http://nas.test")
	var out setupStorageResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || !out.Writable {
		t.Fatalf("probe: %d %+v", rec.Code, out)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("probe leaked files: %v %v", entries, err)
	}
}

func TestSettingsMissingLibraryDoesNotPartiallySave(t *testing.T) {
	settingsFixture(t)
	useMountInfo(t, "")
	dir := filepath.Join(t.TempDir(), "missing")
	body, _ := json.Marshal(map[string]any{"library_dir": dir, "poll_minutes": 45})
	before := runtimeConfig()
	rec := settingsPUT(t, string(body))
	if rec.Code != 400 || runtimeConfig().libraryDir != before.libraryDir || runtimeConfig().pollMinutes != before.pollMinutes {
		t.Fatalf("partial save: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("missing directory was created")
	}
}

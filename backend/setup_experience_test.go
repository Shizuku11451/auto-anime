package main

import (
	"encoding/json"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
)

func TestSetupStatusPathsOnlyBeforeInitialization(t *testing.T) {
	settingsFixture(t)
	setWebAuth(t, "", "")
	setupDone = false
	oldLibrary := libraryDir
	libraryDir = "/media"
	t.Cleanup(func() { libraryDir = oldLibrary })
	read := func() map[string]any {
		t.Helper()
		rec := httptest.NewRecorder()
		setupStatusHandler(rec, httptest.NewRequest("GET", "/api/setup/status", nil))
		if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("status: %d %s", rec.Code, rec.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	out := read()
	if out["required"] != true || out["library_dir"] != "/media" || out["download_dir"] != downloadDir || out["poll_minutes"] != float64(pollMinutes) || out["proxy_enabled"] != false {
		t.Fatalf("missing defaults: %+v", out)
	}
	setupDone = true
	out = read()
	if out["required"] != false || out["library_dir"] != nil || out["download_dir"] != nil {
		t.Fatalf("public paths after setup: %+v", out)
	}
	if out["poll_minutes"] != nil || out["proxy_enabled"] != nil || out["proxy_url"] != nil {
		t.Fatalf("public network defaults after setup: %+v", out)
	}
	setupDone = false
	webUser, webPassword = "admin", "test-password"
	out = read()
	if out["required"] != false || out["library_dir"] != nil || out["download_dir"] != nil {
		t.Fatalf("public paths with environment credentials: %+v", out)
	}
}

// Windows 上 "/media"、"/unmapped/media" 这类没有盘符的路径会按当前盘解析，必须按
// 绝对路径校验拒绝，而不是像容器那样当作 /media 挂载点。
func TestWindowsRejectsContainerStyleLibrary(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("仅 Windows 校验无盘符路径")
	}
	settingsFixture(t)
	setWebAuth(t, "", "")
	setupDone = false
	rec := httptest.NewRecorder()
	setupHandler(rec, httptest.NewRequest("POST", "/api/setup", strings.NewReader(`{"library_dir":"/unmapped/media","download_mode":"direct"}`)))
	if rec.Code != 400 || setupDone {
		t.Fatalf("container path accepted: %d %s", rec.Code, rec.Body.String())
	}
}

func TestSetupReportsUnmappedArchiveRootAndPreservesSettings(t *testing.T) {
	for _, library := range []string{"/unmapped/media", ""} {
		if runtime.GOOS == "windows" && library != "" {
			continue // 容器未映射提示只适用于容器/Linux，Windows 由上一个用例验证拒绝。
		}
		t.Run(library, func(t *testing.T) {
			settingsFixture(t)
			setWebAuth(t, "", "")
			setupDone = false
			oldLibrary, oldDownload := libraryDir, downloadDir
			libraryDir, downloadDir = "", "/unmapped/downloads"
			t.Cleanup(func() { libraryDir, downloadDir = oldLibrary, oldDownload })
			useMountInfo(t, containerMountTable)
			body, _ := json.Marshal(map[string]any{"library_dir": library, "download_mode": "direct"})
			rec := httptest.NewRecorder()
			setupHandler(rec, httptest.NewRequest("POST", "/api/setup", strings.NewReader(string(body))))
			var out struct {
				OK      bool   `json:"ok"`
				Warning string `json:"library_warning"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			if rec.Code != 200 || !out.OK || out.Warning == "" || !setupDone || libraryDir != library {
				t.Fatalf("setup: %d %+v library=%q", rec.Code, out, libraryDir)
			}
			if storedSettings()["library_dir"] != library {
				t.Fatal("library setting not persisted")
			}
		})
	}
}

func TestInvalidSetupLibraryDoesNotFinishInitialization(t *testing.T) {
	settingsFixture(t)
	setWebAuth(t, "", "")
	setupDone = false
	rec := httptest.NewRecorder()
	setupHandler(rec, httptest.NewRequest("POST", "/api/setup", strings.NewReader(`{"library_dir":"relative/media","poll_minutes":15}`)))
	if rec.Code != 400 || setupDone || storedSettings()["setup_done"] == "1" || pollMinutes != 30 {
		t.Fatalf("partial setup: %d done=%v poll=%d", rec.Code, setupDone, pollMinutes)
	}
}

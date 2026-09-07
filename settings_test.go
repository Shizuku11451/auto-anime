package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func settingsPUT(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	settingsHandler(rec, httptest.NewRequest("PUT", "/api/settings", strings.NewReader(body)))
	return rec
}

func TestSetupRequiredAndWizardFlow(t *testing.T) {
	testDatabase(t)
	setWebAuth(t, "", "")
	setupDone = false
	t.Cleanup(func() { setupDone = false; webHash = "" })

	if !setupRequired() {
		t.Fatal("a fresh instance without credentials must require setup")
	}
	// 引导完成前，settings API 不可用（中间件拦截由 auth 测试覆盖）。
	rec := httptest.NewRecorder()
	setupHandler(rec, httptest.NewRequest("POST", "/api/setup", strings.NewReader(`{"web_user":"admin","web_password":"secret123","download_mode":"aria2","aria2_url":"http://127.0.0.1:6800/jsonrpc","poll_minutes":15,"proxy_url":"socks5://127.0.0.1:1080"}`)))
	if rec.Code != 200 {
		t.Fatalf("setup: %d %s", rec.Code, rec.Body.String())
	}
	if setupRequired() {
		t.Fatal("setup must not be required after the wizard finished")
	}
	if webUser != "admin" || webHash == "" || downloadMode != "aria2" || aria2URL != "http://127.0.0.1:6800/jsonrpc" || pollMinutes != 15 {
		t.Fatalf("setup did not apply settings live: %s %q %s %s %d", webUser, webHash, downloadMode, aria2URL, pollMinutes)
	}
	if !verifyWebCredentials("admin", "secret123") || verifyWebCredentials("admin", "wrong") {
		t.Fatal("credentials from the wizard do not verify")
	}
	// 完成后 setup 接口锁定，防止匿名重跑。
	rec = httptest.NewRecorder()
	setupHandler(rec, httptest.NewRequest("POST", "/api/setup", strings.NewReader(`{}`)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("setup rerun must be locked: %d", rec.Code)
	}
	// 重启后设置仍然生效。
	setupDone, webUser, webHash, downloadMode, pollMinutes = false, "", "", "direct", 30
	loadSettings()
	if !setupDone || webUser != "admin" || webHash == "" || downloadMode != "aria2" || pollMinutes != 15 {
		t.Fatalf("settings did not survive restart: %+v", storedSettings())
	}
	if !verifyWebCredentials("admin", "secret123") {
		t.Fatal("credentials lost after reload")
	}
}

func storedSettings() map[string]string {
	out := map[string]string{}
	rows, err := db.Query("SELECT key,value FROM settings")
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if rows.Scan(&k, &v) == nil {
			out[k] = v
		}
	}
	return out
}

func TestSettingsUpdateTakesEffectImmediately(t *testing.T) {
	testDatabase(t)
	setWebAuth(t, "", "")
	setupDone = true
	t.Cleanup(func() { setupDone = false; webHash = "" })
	if rec := settingsPUT(t, `{"download_mode":"transmission","transmission_url":"http://127.0.0.1:9091/","poll_minutes":10,"proxy_url":"http://127.0.0.1:7890"}`); rec.Code != 200 {
		t.Fatalf("put settings: %d %s", rec.Code, rec.Body.String())
	}
	if downloadMode != "transmission" || transmissionURL != "http://127.0.0.1:9091" || pollMinutes != 10 || proxyURL == nil {
		t.Fatalf("settings not applied live: %s %s %d %v", downloadMode, transmissionURL, pollMinutes, proxyURL)
	}
	// 清空代理回到直连。
	if rec := settingsPUT(t, `{"proxy_url":""}`); rec.Code != 200 {
		t.Fatalf("clear proxy: %d %s", rec.Code, rec.Body.String())
	}
	if proxyURL != nil {
		t.Fatalf("proxy was not cleared: %v", proxyURL)
	}
	// 非法值整体拒绝。
	if rec := settingsPUT(t, `{"download_mode":"ftp"}`); rec.Code != 400 {
		t.Fatalf("invalid mode accepted: %d", rec.Code)
	}
	if rec := settingsPUT(t, `{"poll_minutes":1}`); rec.Code != 400 {
		t.Fatalf("small poll accepted: %d", rec.Code)
	}
	if rec := settingsPUT(t, `{"proxy_url":"ftp://x"}`); rec.Code != 400 {
		t.Fatalf("invalid proxy accepted: %d", rec.Code)
	}
	if downloadMode != "transmission" {
		t.Fatalf("failed requests changed the mode: %s", downloadMode)
	}
}

func TestMiddlewareRoutesFreshInstanceToSetup(t *testing.T) {
	testDatabase(t)
	setWebAuth(t, "", "")
	setupDone = false
	t.Cleanup(func() { setupDone = false; webHash = "" })
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") })
	handler := auth(next)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/setup" {
		t.Fatalf("page redirect: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/api/items", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("api before setup: %d", rec.Code)
	}
	for _, tc := range []struct{ method, path string }{
		{"GET", "/setup"}, {"GET", "/api/setup/status"}, {"POST", "/api/setup"},
	} {
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}")))
		if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusFound {
			t.Fatalf("setup path %s was blocked: %d", tc.path, rec.Code)
		}
	}

	// 跳过引导后恢复完全开放。
	setupDone = true
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("open instance after skipping setup: %d", rec.Code)
	}
}

func TestPasswordChangeIsHashedAndSecretsNotExposed(t *testing.T) {
	testDatabase(t)
	setWebAuth(t, "", "")
	setupDone = true
	t.Cleanup(func() { setupDone = false; webHash = "" })
	if rec := settingsPUT(t, `{"web_user":"boss","web_password":"newpass123"}`); rec.Code != 200 {
		t.Fatalf("set credentials: %d %s", rec.Code, rec.Body.String())
	}
	if !verifyWebCredentials("boss", "newpass123") {
		t.Fatal("new credentials do not verify")
	}
	stored := storedSettings()
	if stored["web_hash"] == "" || strings.Contains(stored["web_hash"], "newpass123") || stored["web_hash"] == "newpass123" {
		t.Fatalf("password must be stored hashed only: %q", stored["web_hash"])
	}

	rec := httptest.NewRecorder()
	settingsHandler(rec, httptest.NewRequest("GET", "/api/settings", nil))
	if rec.Code != 200 {
		t.Fatalf("get settings: %d", rec.Code)
	}
	body := rec.Body.String()
	for _, secret := range []string{"newpass123", `"web_hash"`, `"aria2_token"`, `"qbit_password"`, `"transmission_password"`} {
		if strings.Contains(body, secret) {
			t.Fatalf("settings response leaks %q: %s", secret, body)
		}
	}
	var out struct {
		WebPasswordSet bool `json:"web_password_set"`
		Aria2TokenSet  bool `json:"aria2_token_set"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !out.WebPasswordSet || out.Aria2TokenSet {
		t.Fatalf("set flags wrong: %+v", out)
	}

	// 只改密码：用户名保持不变。用户名带空格被拒绝（必须同时给密码时才校验长度）。
	if rec := settingsPUT(t, `{"web_password":"another-pass"}`); rec.Code != 200 {
		t.Fatalf("password-only change: %d %s", rec.Code, rec.Body.String())
	}
	if !verifyWebCredentials("boss", "another-pass") || verifyWebCredentials("boss", "newpass123") {
		t.Fatal("password rotation failed")
	}
	if rec := settingsPUT(t, fmt.Sprintf(`{"web_user":%q,"web_password":"short"}`, "boss")); rec.Code != 400 {
		t.Fatalf("short password accepted: %d", rec.Code)
	}
}

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 桌面标题栏主题接口：只接受 dark/light，服务模式或非 Windows 返回 applied=false。
func TestDesktopThemeHandler(t *testing.T) {
	post := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		desktopThemeHandler(rec, httptest.NewRequest("POST", "/api/desktop/theme", strings.NewReader(body)))
		return rec
	}
	for _, theme := range []string{"dark", "light", "DARK", " dark "} {
		rec := post(`{"theme":"` + theme + `"}`)
		if rec.Code != 200 {
			t.Fatalf("%q: %d %s", theme, rec.Code, rec.Body.String())
		}
		var out struct {
			OK      bool `json:"ok"`
			Applied bool `json:"applied"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || !out.OK {
			t.Fatalf("%q: %s", theme, rec.Body.String())
		}
	}
	for _, body := range []string{`{"theme":"blue"}`, `{}`, `{`, `{"theme":""}`} {
		if rec := post(body); rec.Code != 400 {
			t.Fatalf("%s: %d %s", body, rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	desktopThemeHandler(rec, httptest.NewRequest("GET", "/api/desktop/theme", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: %d", rec.Code)
	}
}

// 登录页也要能同步标题栏主题，因此未登录时接口放行（只设置窗口配色，不读数据）。
func TestDesktopThemeReachableWithoutSession(t *testing.T) {
	testDatabase(t)
	setWebAuth(t, "admin", "secret")
	handler := auth(http.HandlerFunc(desktopThemeHandler))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("POST", "/api/desktop/theme", strings.NewReader(`{"theme":"dark"}`)))
	if rec.Code != 200 {
		t.Fatalf("anonymous theme sync: %d %s", rec.Code, rec.Body.String())
	}
}

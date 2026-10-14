package main

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
)

// Windows 本机没有容器和卷映射，/install 部署配置页直接 404。
func TestInstallPageUnavailableOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows 专用行为")
	}
	rec := httptest.NewRecorder()
	installPage(rec, httptest.NewRequest("GET", "/install", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("install page on windows: %d %s", rec.Code, rec.Body.String())
	}
}

// 安装配置页必须在未初始化、未登录时也能打开：它是容器启动后更换媒体目录或端口的入口。
func TestInstallPageServesGeneratorWithoutAuth(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Compose 生成页仅适用于容器/Linux 部署")
	}
	rec := httptest.NewRecorder()
	installPage(rec, httptest.NewRequest("GET", "/install", nil))
	if rec.Code != 200 {
		t.Fatalf("install page: %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"Auto-Anime", "Compose", "番剧"} {
		if !strings.Contains(body, want) {
			t.Fatalf("install page missing %q", want)
		}
	}
	// 离线页面的占位符必须在构建时被替换，否则浏览器里是一个不能生成配置的空壳。
	if strings.Contains(body, "/* INSTALL_SCRIPT */") || strings.Contains(body, "/* DESIGN_TOKENS */") {
		t.Fatal("install page placeholders were not replaced")
	}
}

func TestInstallPageRejectsWrites(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Compose 生成页仅适用于容器/Linux 部署")
	}
	rec := httptest.NewRecorder()
	installPage(rec, httptest.NewRequest("POST", "/install", strings.NewReader("{}")))
	if rec.Code != 405 {
		t.Fatalf("write to install page: %d", rec.Code)
	}
}

// 通过鉴权中间件访问时也必须直接返回页面，而不是跳转到登录或首次向导。
func TestInstallPageReachableThroughAuthMiddleware(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Compose 生成页仅适用于容器/Linux 部署")
	}
	testDatabase(t)
	setWebAuth(t, "admin", "secret")
	handler := auth(http.HandlerFunc(installPage))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/install", nil))
	if rec.Code != 200 {
		t.Fatalf("anonymous /install through auth: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	if !strings.Contains(rec.Body.String(), "复制完整配置") {
		t.Fatal("install page did not render through the auth middleware")
	}
}

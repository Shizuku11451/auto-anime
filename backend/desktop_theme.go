package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

// desktopThemeHandler 让桌面窗口的原生标题栏跟随应用主题：前端切换亮暗或跟随系统
// 变化时调用一次。服务模式、非 Windows 或窗口尚未创建时返回 applied=false，前端
// 忽略即可；接口不读取任何数据，因此初始化前也允许调用（仅做同源校验）。
func desktopThemeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		http.Error(w, "方法不支持", http.StatusMethodNotAllowed)
		return
	}
	if !allowSetupWrite(w, r) {
		return
	}
	var in struct {
		Theme string `json:"theme"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in) != nil {
		http.Error(w, "请求参数无效", http.StatusBadRequest)
		return
	}
	theme := strings.ToLower(strings.TrimSpace(in.Theme))
	if theme != "dark" && theme != "light" {
		http.Error(w, "主题必须是 dark 或 light", http.StatusBadRequest)
		return
	}
	jsonOut(w, map[string]any{"ok": true, "applied": applyDesktopTheme(theme)})
}

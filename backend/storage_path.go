package main

import (
	"path/filepath"
	"runtime"
	"strings"
)

// filepath.Clean preserves Windows UNC volumes; path.Clean would collapse
// //server/share into /server/share and silently target the current drive.
func cleanStoragePath(value string) string {
	return filepath.ToSlash(filepath.Clean(value))
}

// storageExampleDir 是给用户看的绝对路径示例：容器与 Linux 用 NAS 挂载路径，
// Windows 本机用盘符或 UNC 共享（"/番剧"、"\\番剧" 会按当前盘解析，不是完整路径）。
func storageExampleDir() string {
	if runtime.GOOS == "windows" {
		return `D:\番剧 或 \\NAS\共享\番剧`
	}
	return "/vol1/1000/番剧"
}

// windowsAbsolutePath 判断 Windows 上的完整绝对路径：盘符路径（D:\番剧）或 UNC
// 共享（\\NAS\动画）。"/番剧"、"\\番剧" 这类「当前盘根相对」的写法不算——它们会随
// 进程当前目录所在的盘变化，用户很难发现文件实际写到了哪里。
func windowsAbsolutePath(value string) bool {
	if len(value) >= 3 && value[1] == ':' && (value[2] == '\\' || value[2] == '/') {
		c := value[0]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			return true
		}
	}
	if strings.HasPrefix(value, `\\`) || strings.HasPrefix(value, "//") {
		rest := strings.TrimLeft(value, `/\`)
		parts := strings.SplitN(filepath.ToSlash(rest), "/", 3)
		return len(parts) >= 2 && parts[0] != "" && parts[1] != ""
	}
	return false
}

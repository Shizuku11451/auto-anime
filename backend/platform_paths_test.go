package main

import (
	"path/filepath"
	"testing"
)

// normalizeRemotePath 让不同平台的下载器路径可比较：统一分隔符、去掉尾部斜杠，
// 但保留 "C:/" 与 "//server/share" 这类根形式。
func TestNormalizeRemotePath(t *testing.T) {
	cases := map[string]string{
		`C:\Downloads\Anime\`: "C:/Downloads/Anime",
		`C:\`:                 "C:/",
		`/data/downloads/`:    "/data/downloads",
		`\\NAS\媒体\番剧\`:        "//NAS/媒体/番剧",
		"/":                   "/",
		`  C:\a  `:            "C:/a",
		"":                    "",
	}
	for input, want := range cases {
		if got := normalizeRemotePath(input); got != want {
			t.Errorf("normalizeRemotePath(%q) = %q, want %q", input, got, want)
		}
	}
}

// Windows 下载器用反斜杠报路径（Aria2/qBittorrent/Transmission），用户配置的映射
// 前缀也可能用另一种分隔符；两种情况都要能算出本地路径。
func TestMapRemotePathHandlesWindowsSeparators(t *testing.T) {
	local := `D:\媒体`
	remote := `C:\Downloads`
	want := filepath.Join(local, filepath.FromSlash("Show/01.mkv"))
	for _, path := range []string{`C:\Downloads\Show\01.mkv`, `C:/Downloads/Show/01.mkv`, `C:\Downloads/Show/01.mkv`} {
		if got := mapRemotePath(path, remote, local); got != want {
			t.Errorf("mapRemotePath(%q, %q) = %q, want %q", path, remote, got, want)
		}
	}
	if got := mapRemotePath(`C:\Downloads\Show\01.mkv`, `C:/Downloads`, local); got != want {
		t.Errorf("mixed separators in base: %q, want %q", got, want)
	}
	// 无法映射时保留原值，让归档报出具体缺失文件，而不是静默复制错文件。
	if got := mapRemotePath(`E:\Other\01.mkv`, remote, local); got != "E:/Other/01.mkv" {
		t.Errorf("unmapped path = %q", got)
	}
	// 映射根与本地根相同（容器或同机）时不做搬运。
	if got := mapRemotePath(`C:\Downloads\Show\01.mkv`, remote, remote); got != "C:/Downloads/Show/01.mkv" {
		t.Errorf("identical roots = %q", got)
	}
	if got := mapRemotePath(`c:\downloads\show\01.mkv`, remote, local); got != "c:/downloads/show/01.mkv" {
		t.Errorf("case-sensitive host must not map: %q", got)
	}
}

// 存储位置沿用 NAS 挂载路径。
func TestSanitizeStorageDirKeepsMountPath(t *testing.T) {
	clean, err := sanitizeStorageDir("/vol1/1000/番剧")
	if err != nil || clean != "/vol1/1000/番剧" {
		t.Fatalf("容器路径：%q %v", clean, err)
	}
}

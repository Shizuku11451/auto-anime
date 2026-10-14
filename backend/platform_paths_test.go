package main

import (
	"bytes"
	"log"
	"path/filepath"
	"runtime"
	"strings"
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
	if runtime.GOOS == "windows" {
		// Windows 文件系统大小写不敏感：盘符大小写不同也要匹配。
		if got := mapRemotePath(`c:\downloads\Show\01.mkv`, remote, local); got != want {
			t.Errorf("case-insensitive map = %q, want %q", got, want)
		}
	} else if got := mapRemotePath(`c:\downloads\show\01.mkv`, remote, local); got != "c:/downloads/show/01.mkv" {
		t.Errorf("case-sensitive host must not map: %q", got)
	}
}

// Windows 的完整绝对路径只有盘符路径和 UNC 共享两种；"/番剧"、"\\番剧" 会按当前盘
// 解析，必须被识别出来。
func TestWindowsAbsolutePath(t *testing.T) {
	valid := []string{`D:\番剧`, `d:/media`, `\\NAS\动画`, `//NAS/动画/番剧`, `\\?\D:\番剧`}
	for _, value := range valid {
		if !windowsAbsolutePath(value) {
			t.Errorf("%q 应当是完整绝对路径", value)
		}
	}
	invalid := []string{``, `番剧`, `/番剧`, `\番剧`, `C:`, `D:番剧`, `1:\番剧`, `\\NAS`, `//NAS`, `\\NAS\`}
	for _, value := range invalid {
		if windowsAbsolutePath(value) {
			t.Errorf("%q 不应当是完整绝对路径", value)
		}
	}
}

// 存储位置校验按平台区分：Windows 必须有盘符或 UNC，容器/Linux 沿用挂载路径。
func TestSanitizeStorageDirFollowsPlatform(t *testing.T) {
	if runtime.GOOS == "windows" {
		for _, raw := range []string{`/番剧`, `\番剧`, `/vol1/1000/番剧`, `C:`, `D:番剧`} {
			if _, err := sanitizeStorageDir(raw); err == nil {
				t.Errorf("Windows 上应拒绝没有盘符或 UNC 的路径：%q", raw)
			}
		}
		clean, err := sanitizeStorageDir(`D:\番剧\夏日`)
		if err != nil || clean != "D:/番剧/夏日" {
			t.Fatalf("盘符路径：%q %v", clean, err)
		}
		return
	}
	clean, err := sanitizeStorageDir("/vol1/1000/番剧")
	if err != nil || clean != "/vol1/1000/番剧" {
		t.Fatalf("容器路径：%q %v", clean, err)
	}
}

// 内置下载器的源文件在私有根的 downloads\ 下，源文件清理必须按同一位置校验边界。
func TestBuiltinSourceRootMatchesDownloaderDirectory(t *testing.T) {
	root := t.TempDir()
	t.Setenv("QBITTORRENT_DATA_DIR", root)
	want := filepath.Join(root, "downloads")
	if got := embeddedQbitDownloadDir(); got != want {
		t.Fatalf("embeddedQbitDownloadDir = %q, want %q", got, want)
	}
	if got := sourceRootFor("builtin"); got != want {
		t.Fatalf("sourceRootFor(builtin) = %q, want %q", got, want)
	}
}

// 从 Linux/容器数据库搬到 Windows 的历史保存位置要在启动时提示，且不能静默改写数据。
func TestWarnStoredPathsForPlatformKeepsData(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows 专用提示")
	}
	testDatabase(t)
	if _, err := db.Exec("INSERT INTO feeds(name,url,storage_dir) VALUES('旧订阅','https://example.test/rss','/vol1/1000/番剧')"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO animes(title,storage_dir) VALUES('旧番剧','/volume1/anime/old')"); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(old)
	warnStoredPathsForPlatform()
	out := buf.String()
	for _, want := range []string{"旧订阅", "旧番剧", "当前盘根"} {
		if !strings.Contains(out, want) {
			t.Fatalf("启动提示缺少 %q：%q", want, out)
		}
	}
	var stored string
	if err := db.QueryRow("SELECT storage_dir FROM feeds WHERE name='旧订阅'").Scan(&stored); err != nil || stored != "/vol1/1000/番剧" {
		t.Fatalf("不应改写数据库：%q %v", stored, err)
	}
}

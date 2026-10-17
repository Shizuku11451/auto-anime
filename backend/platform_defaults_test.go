package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// 默认数据目录按平台取值：容器/Linux 沿用 /data，Windows 本机用程序同级的 data\，
// 不再落到 <盘根>:\data（盘根可能不可写，用户也找不到数据）。
func TestDefaultDataDirFollowsPlatform(t *testing.T) {
	testDatabase(t)
	dir := defaultDataDir()
	if runtime.GOOS != "windows" {
		if dir != "/data" {
			t.Fatalf("defaultDataDir = %q, want /data", dir)
		}
	} else {
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		exeDir := filepath.Dir(exe)
		// 允许两个父目录：程序同级（发布 exe）与当前目录（go run/go test 的临时构建），
		// 但不能落回盘根。
		parent := filepath.Dir(dir)
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Base(dir) != "data" || (!samePath(parent, exeDir) && !samePath(parent, cwd)) {
			t.Fatalf("defaultDataDir = %q, want data\\ under %q or %q", dir, exeDir, cwd)
		}
		if root := filepath.VolumeName(exeDir) + `\`; !samePath(exeDir, root) && samePath(dir, filepath.Join(root, "data")) {
			t.Fatalf("defaultDataDir 落回盘根: %q", dir)
		}
	}
	if got, want := defaultDBPath(), filepath.Join(dir, "auto-anime.db"); !samePath(got, want) {
		t.Fatalf("defaultDBPath = %q, want %q", got, want)
	}
	// 启动日志按 dbFile 说明实际使用的库，initDB 必须把它设成生效路径。
	if got, want := dbFile, env("DB_PATH", defaultDBPath()); !samePath(got, want) {
		t.Fatalf("dbFile = %q, want %q", got, want)
	}
}

// 下载方式默认值按平台取值：Windows 自带进程内 BT 下载器，容器/Linux 仍由
// 部署配置选择 builtin，源码运行走 direct。
func TestDefaultDownloadModeFollowsPlatform(t *testing.T) {
	want := "direct"
	if runtime.GOOS == "windows" {
		want = "native"
	}
	if got := defaultDownloadMode(); got != want {
		t.Fatalf("defaultDownloadMode = %q, want %q", got, want)
	}
}

// 媒体库默认值按平台取值：容器/Linux 留空（归档落下载目录，模板显式给 /media）；
// Windows 用程序同级的 media\，与下载器源文件所在的 data\ 分开，媒体服务器可直接扫描。
func TestDefaultLibraryDirFollowsPlatform(t *testing.T) {
	dir := defaultLibraryDir()
	if runtime.GOOS != "windows" {
		if dir != "" {
			t.Fatalf("defaultLibraryDir = %q, want empty", dir)
		}
		return
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(dir)
	if filepath.Base(dir) != "media" || (!samePath(parent, filepath.Dir(exe)) && !samePath(parent, cwd)) {
		t.Fatalf("defaultLibraryDir = %q, want media\\ under %q or %q", dir, filepath.Dir(exe), cwd)
	}
	// 归档目录不能和下载目录是同一个：否则源文件、暂存与已归档媒体混在一起。
	if samePath(dir, downloadDir) {
		t.Fatalf("媒体库默认值不应等于下载目录 %q", downloadDir)
	}
	if !samePath(filepath.Dir(dir), filepath.Dir(defaultDataDir())) {
		t.Fatalf("媒体库 %q 与数据目录 %q 应为同级", dir, defaultDataDir())
	}
}

// 当前模式不是 builtin、但库里有旧的内置条目时，Windows 上只告警，不改写当前模式。
func TestEmbeddedQBittorrentKeepsModeWithBuiltinItems(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("仅 Windows Release 不带内置 qBittorrent")
	}
	testDatabase(t)
	t.Setenv("QBITTORRENT_PATH", "")
	oldMode := downloadMode
	t.Cleanup(func() { downloadMode = oldMode })

	// 当前模式不是 builtin、但库里有旧的内置条目：只告警，不改写当前模式。
	if _, err := db.Exec("INSERT INTO items(title,link,downloader) VALUES('x','https://example.test/bt','builtin')"); err != nil {
		t.Fatal(err)
	}
	downloadMode = "qbittorrent"
	if err := startEmbeddedQBittorrent(); err != nil {
		t.Fatalf("库里的内置条目不应导致启动失败: %v", err)
	}
	if downloadMode != "qbittorrent" {
		t.Fatalf("非 builtin 模式不应被改写: %q", downloadMode)
	}
}

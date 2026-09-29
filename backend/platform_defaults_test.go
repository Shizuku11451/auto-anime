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

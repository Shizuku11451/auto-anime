package main

import (
	"path/filepath"
	"testing"
)

func TestDefaultDataDir(t *testing.T) {
	testDatabase(t)
	dir := defaultDataDir()
	if dir != "/data" {
		t.Fatalf("defaultDataDir = %q, want /data", dir)
	}
	if got, want := defaultDBPath(), filepath.Join(dir, "auto-anime.db"); !samePath(got, want) {
		t.Fatalf("defaultDBPath = %q, want %q", got, want)
	}
	// 启动日志按 dbFile 说明实际使用的库，initDB 必须把它设成生效路径。
	if got, want := dbFile, env("DB_PATH", defaultDBPath()); !samePath(got, want) {
		t.Fatalf("dbFile = %q, want %q", got, want)
	}
}

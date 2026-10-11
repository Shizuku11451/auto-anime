//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDesktopModeSelection(t *testing.T) {
	oldArgs, oldDefault := os.Args, desktopDefault
	t.Cleanup(func() { os.Args, desktopDefault = oldArgs, oldDefault })
	for _, tc := range []struct {
		defaults, arg string
		want          bool
	}{
		{"false", "", false}, {"true", "", true}, {"true", "--server", false}, {"false", "--desktop", true},
	} {
		desktopDefault, os.Args = tc.defaults, []string{"app", tc.arg}
		if desktopRequested() != tc.want {
			t.Fatalf("%+v", tc)
		}
	}
}

func TestBundledDownloaderPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "中文 空格")
	if got := bundledQbitExecutable(root); got != "" {
		t.Fatal(got)
	}
	dir := filepath.Join(root, "qbittorrent")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "qbittorrent.exe")
	if err := os.WriteFile(exe, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := bundledQbitExecutable(root); got != exe {
		t.Fatal(got)
	}
}

func TestWindowsApplicationLock(t *testing.T) {
	database := filepath.Join(t.TempDir(), "中文", "state.db")
	release, err := acquireApplicationLock(database)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if second, err := acquireApplicationLock(database); err == nil {
		second()
		t.Fatal("duplicate database lock")
	}
	release()
	third, err := acquireApplicationLock(database)
	if err != nil {
		t.Fatal(err)
	}
	third()
}

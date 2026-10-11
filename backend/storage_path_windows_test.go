//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsUNCStorageSurvivesArchiveAndRename(t *testing.T) {
	root := `\\nas\动画共享\媒体库`
	clean, err := sanitizeStorageDir(root)
	if err != nil || clean != "//nas/动画共享/媒体库" {
		t.Fatalf("UNC: %q %v", clean, err)
	}
	series := subscriptionSeriesRoot(clean, "测试动画")
	if series != "//nas/动画共享/媒体库/测试动画" {
		t.Fatal(series)
	}
	if renamed := retargetSeriesDirPath(series, "测试动画", "新动画"); renamed != "//nas/动画共享/媒体库/新动画" {
		t.Fatal(renamed)
	}
	if _, err := sanitizeStorageDir(`\\nas\动画共享\..\逃逸`); err == nil {
		t.Fatal("accepted traversal")
	}
	if !filepath.IsAbs(series) {
		t.Fatal("UNC lost absolute volume")
	}
}

func TestWindowsReservedArchiveNames(t *testing.T) {
	for _, name := range []string{"CON", "NUL.mp4", "CON .mp4", "COM¹", "LPT².mkv", "CONIN$", "CONOUT$"} {
		if !strings.HasPrefix(safeComponent(name), "_") {
			t.Errorf("device name: %q", name)
		}
	}
}

func TestWindowsArchiveFallbackPreservesExistingFile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "中文 归档")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	source, destination := filepath.Join(root, "临时.mp4"), filepath.Join(root, "番剧.mp4")
	if err := os.WriteFile(source, []byte("new video"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("original video"), 0600); err != nil {
		t.Fatal(err)
	}
	err := commitWithoutHardlink(source, destination, errors.New("hardlinks unavailable"))
	if !os.IsExist(err) {
		t.Fatalf("collision: %v", err)
	}
	for name, want := range map[string]string{source: "new video", destination: "original video"} {
		data, err := os.ReadFile(name)
		if err != nil || string(data) != want {
			t.Fatalf("%s: %q %v", name, data, err)
		}
	}
	if err := os.Remove(destination); err != nil {
		t.Fatal(err)
	}
	if err := commitWithoutHardlink(source, destination, errors.New("hardlinks unavailable")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("staging retained: %v", err)
	}
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != "new video" {
		t.Fatalf("committed: %q %v", data, err)
	}
}

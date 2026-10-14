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

// 桌面窗口必须在创建前声明 DPI 感知：否则 Windows 会按缩放比例拉伸整个窗口位图，
// 在 125%/150% 的屏幕上界面发虚。
func TestDesktopDpiAwarenessEnabled(t *testing.T) {
	level := enableDesktopDpiAwareness()
	if level == "unaware" {
		t.Fatalf("DPI 感知未生效：%s", level)
	}
	if current := currentDpiAwareness(); current < 1 {
		t.Fatalf("当前 DPI 感知级别异常：%d", current)
	}
	// 重复声明（例如实例被重复初始化）不能把级别降回去，也不能误报 unaware。
	if again := enableDesktopDpiAwareness(); again == "unaware" {
		t.Fatalf("重复声明后级别丢失：%s", again)
	}
}

// 设计尺寸按 DPI 换算成物理像素：100% 原样，150% 放大 1.5 倍。
func TestScaleForDpi(t *testing.T) {
	for _, tc := range []struct{ value, dpi, want int }{
		{desktopWindowWidth, 96, desktopWindowWidth},
		{desktopWindowWidth, 144, 1650},
		{desktopWindowHeight, 120, 950},
		{desktopWindowMinWidth, 192, 840},
	} {
		if got := scaleForDpi(tc.value, tc.dpi); got != tc.want {
			t.Fatalf("scaleForDpi(%d, %d) = %d, want %d", tc.value, tc.dpi, got, tc.want)
		}
	}
}

// 初始窗口尺寸必须为正、不低于 100% 基准，并且不超出主屏工作区。
func TestDesktopWindowSizeFitsWorkArea(t *testing.T) {
	width, height, dpi := desktopWindowSize()
	if dpi < desktopDpiBaseline || width <= 0 || height <= 0 {
		t.Fatalf("尺寸异常：%dx%d dpi=%d", width, height, dpi)
	}
	if width < scaleForDpi(desktopWindowMinWidth, dpi) || height < scaleForDpi(desktopWindowMinHeight, dpi) {
		t.Fatalf("窗口小于最小尺寸：%dx%d", width, height)
	}
	if area, ok := primaryWorkArea(); ok {
		if width > int(area.Right-area.Left) || height > int(area.Bottom-area.Top) {
			t.Fatalf("窗口 %dx%d 超出工作区 %dx%d", width, height, area.Right-area.Left, area.Bottom-area.Top)
		}
	}
}

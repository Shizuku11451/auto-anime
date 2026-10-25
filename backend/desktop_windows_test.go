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

// 图标资源必须真的嵌进可执行文件：标题栏、任务栏与托盘都靠它，取不到就退回默认图标。
func TestAppIconResourceLoaded(t *testing.T) {
	if handle := moduleHandle(); handle == 0 {
		t.Fatal("模块句柄不可用")
	}
	for _, size := range []int{16, 32} {
		if loadAppIcon(size) == 0 {
			t.Fatalf("%dx%d 图标加载失败（rsrc_windows_amd64.syso 是否随包提交？）", size, size)
		}
	}
	if trayIcon() == 0 {
		t.Fatal("托盘图标不可用")
	}
}

// 没有桌面窗口（服务模式、测试进程）时标题栏主题同步必须安全返回 false。
func TestDesktopThemeWithoutWindow(t *testing.T) {
	if applyTitleBarTheme(0, true) {
		t.Fatal("空窗口句柄不应报告成功")
	}
	if applyDesktopTheme("dark") || applyDesktopTheme("light") {
		t.Fatal("没有桌面窗口时不应报告已应用")
	}
	// 读不到系统主题时按深色处理，不能 panic。
	_ = systemPrefersDarkAppTheme()
}

// .ico 需要包含标题栏（16）、任务栏（32）等尺寸，否则 Windows 会缩放导致发虚。
func TestIconAssetContainsRequiredSizes(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("assets", "auto-anime.ico"))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 6 || data[0] != 0 || data[1] != 0 || data[2] != 1 || data[3] != 0 {
		t.Fatal("不是有效的 ICO 文件头")
	}
	count := int(data[4]) | int(data[5])<<8
	if count < 4 || len(data) < 6+16*count {
		t.Fatalf("ICO 目录异常：count=%d len=%d", count, len(data))
	}
	sizes := map[int]bool{}
	for i := 0; i < count; i++ {
		entry := data[6+16*i:]
		width, height := int(entry[0]), int(entry[1])
		if width == 0 {
			width = 256
		}
		if height == 0 {
			height = 256
		}
		sizes[width] = sizes[width] && width == height || !sizes[width]
		if width != height {
			t.Fatalf("第 %d 个图标不是正方形：%dx%d", i, width, height)
		}
	}
	for _, want := range []int{16, 32, 48, 256} {
		if !sizes[want] {
			t.Fatalf("缺少 %dx%d 图标：%v", want, want, sizes)
		}
	}
}

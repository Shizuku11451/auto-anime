//go:build windows

package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	webview "github.com/jchv/go-webview2"
	"github.com/jchv/go-webview2/webviewloader"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Release sets this with -X; source runs retain their console/server behavior.
var desktopDefault = "false"

func desktopRequested() bool {
	for _, arg := range os.Args[1:] {
		if arg == "--server" {
			return false
		}
		if arg == "--desktop" {
			return true
		}
	}
	return desktopDefault == "true"
}

var desktopUser32 = windows.NewLazySystemDLL("user32.dll")

func desktopText(text string) *uint16 { return windows.StringToUTF16Ptr(text) }

func desktopError(message string) {
	log.Print(message)
	desktopUser32.NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(desktopText(message))), uintptr(unsafe.Pointer(desktopText("Auto-Anime"))), 0x10)
}

// ---- 窗口 DPI ----
//
// Go 程序默认没有 DPI 感知声明，Windows 会把整个窗口位图按系统缩放比例拉伸，
// 在 125%/150% 的屏幕上文字和界面都会发虚。桌面入口在创建窗口之前声明按显示器感知
// （Per-Monitor V2，逐级回退到系统感知），并把设计尺寸换算成物理像素、再限制在
// 主屏工作区内，让界面在任意缩放下都清晰且不会超出屏幕。

const (
	dpiAwarenessContextPerMonitorAwareV2 = ^uintptr(3) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = -4
	processPerMonitorDpiAware            = 2           // PROCESS_PER_MONITOR_DPI_AWARE
	spiGetWorkArea                       = 0x0030
	desktopDpiBaseline                   = 96 // 100% 缩放
	desktopWindowWidth                   = 1100
	desktopWindowHeight                  = 760
	desktopWindowMinWidth                = 420
	desktopWindowMinHeight               = 480
	desktopWindowMargin                  = 40 // 工作区留白，避免窗口顶满屏幕
)

var (
	desktopShcore = windows.NewLazySystemDLL("shcore.dll")

	procSetProcessDpiAwarenessContext       = desktopUser32.NewProc("SetProcessDpiAwarenessContext")
	procSetProcessDPIAware                  = desktopUser32.NewProc("SetProcessDPIAware")
	procGetDpiForSystem                     = desktopUser32.NewProc("GetDpiForSystem")
	procSystemParametersInfoW               = desktopUser32.NewProc("SystemParametersInfoW")
	procGetThreadDpiAwarenessContext        = desktopUser32.NewProc("GetThreadDpiAwarenessContext")
	procGetAwarenessFromDpiAwarenessContext = desktopUser32.NewProc("GetAwarenessFromDpiAwarenessContext")
	procSetProcessDpiAwareness              = desktopShcore.NewProc("SetProcessDpiAwareness")
)

// callOptional 只在系统确实提供该函数时调用；LazyProc 对缺失的函数会直接 panic，
// 这里用于兼容没有 shcore.dll / GetDpiForSystem 的旧系统。
func callOptional(proc *windows.LazyProc, args ...uintptr) (uintptr, bool) {
	if err := proc.Find(); err != nil {
		return 0, false
	}
	value, _, _ := proc.Call(args...)
	return value, true
}

// enableDesktopDpiAwareness 声明进程 DPI 感知，返回实际生效的级别（写进日志便于排查）。
// 重复调用或清单已经声明过时，改为报告当前生效级别，不误报成 unaware。
func enableDesktopDpiAwareness() string {
	if ok, _ := callOptional(procSetProcessDpiAwarenessContext, dpiAwarenessContextPerMonitorAwareV2); ok != 0 {
		return "per-monitor v2"
	}
	// shcore 的 SetProcessDpiAwareness 成功时返回 S_OK(0)。
	if hr, found := callOptional(procSetProcessDpiAwareness, processPerMonitorDpiAware); found && hr == 0 {
		return "per-monitor"
	}
	if ok, _ := callOptional(procSetProcessDPIAware); ok != 0 {
		return "system"
	}
	return dpiAwarenessLabel(currentDpiAwareness())
}

// currentDpiAwareness 读取当前生效的 DPI 感知级别：0 不感知、1 系统感知、2 按显示器感知；
// 旧系统读不到时返回 -1。
func currentDpiAwareness() int {
	context, found := callOptional(procGetThreadDpiAwarenessContext)
	if !found || context == 0 {
		return -1
	}
	level, found := callOptional(procGetAwarenessFromDpiAwarenessContext, context)
	if !found {
		return -1
	}
	return int(level)
}

func dpiAwarenessLabel(level int) string {
	switch level {
	case 0:
		return "unaware"
	case 1:
		return "system"
	case 2:
		return "per-monitor"
	}
	return "unaware"
}

func themeLabel(dark bool) string {
	if dark {
		return "深色"
	}
	return "浅色"
}

// systemDpi 返回系统 DPI；取不到时按 96（100%）。
func systemDpi() uintptr {
	if dpi, found := callOptional(procGetDpiForSystem); found && dpi >= desktopDpiBaseline {
		return dpi
	}
	return desktopDpiBaseline
}

type desktopRect struct{ Left, Top, Right, Bottom int32 }

// primaryWorkArea 返回主屏工作区（不含任务栏）；读不到时 ok=false。
func primaryWorkArea() (desktopRect, bool) {
	var rect desktopRect
	ok, found := callOptional(procSystemParametersInfoW, spiGetWorkArea, 0, uintptr(unsafe.Pointer(&rect)), 0)
	if !found || ok == 0 || rect.Right <= rect.Left || rect.Bottom <= rect.Top {
		return desktopRect{}, false
	}
	return rect, true
}

// scaleForDpi 把设计尺寸（逻辑像素）换算成给定 DPI 下的物理像素。
func scaleForDpi(value, dpi int) int {
	return value * dpi / desktopDpiBaseline
}

// desktopWindowSize 把设计尺寸换算成物理像素，并限制在主屏工作区内。
func desktopWindowSize() (width, height, dpi int) {
	dpi = int(systemDpi())
	width, height = scaleForDpi(desktopWindowWidth, dpi), scaleForDpi(desktopWindowHeight, dpi)
	if area, ok := primaryWorkArea(); ok {
		if maxWidth := int(area.Right-area.Left) - desktopWindowMargin; maxWidth > 0 && width > maxWidth {
			width = maxWidth
		}
		if maxHeight := int(area.Bottom-area.Top) - desktopWindowMargin; maxHeight > 0 && height > maxHeight {
			height = maxHeight
		}
	}
	return width, height, dpi
}

// ---- 窗口图标与标题栏配色 ----
//
// 发布包通过 rsrc_windows_amd64.syso 内置图标资源（ID 1）与 Per-Monitor V2 清单：
// 图标用于标题栏、任务栏与托盘；标题栏配色跟随应用主题（前端切换后调用
// /api/desktop/theme），初始值取系统应用主题，避免先亮后暗的闪烁。
//
// 图标与清单由 go-winres 预生成并提交（Linux 交叉编译不需要额外工具）：
//
//go:generate go run github.com/tc-hib/go-winres@v0.3.3 simply --arch amd64 --icon assets/auto-anime.ico --manifest gui --product-name Auto-Anime --file-description "Auto-Anime 自动追番" --out rsrc

const (
	desktopIconID       = 1
	dwmwaImmersiveDark  = 20 // DWMWA_USE_IMMERSIVE_DARK_MODE（Windows 10 20H1+）
	dwmwaImmersiveDark2 = 19 // 更早的 1809/1903 使用的属性号
	iconSmall           = 0
	iconBig             = 1
	wmSetIcon           = 0x0080
	smCxIcon            = 11
	smCxSmIcon          = 49
	imageIcon           = 1
	idiApplication      = 32512
)

var (
	procSendMessageW     = desktopUser32.NewProc("SendMessageW")
	procLoadImageW       = desktopUser32.NewProc("LoadImageW")
	procLoadIconW        = desktopUser32.NewProc("LoadIconW")
	procGetSystemMetrics = desktopUser32.NewProc("GetSystemMetrics")
)

// desktopWindowHandle 是当前桌面窗口（服务模式或非 Windows 为 0），供主题接口更新标题栏。
var desktopWindowHandle windows.HWND

// moduleHandle 返回当前可执行文件的模块句柄，用于按 ID 读取内置图标资源。
func moduleHandle() windows.Handle {
	var handle windows.Handle
	if err := windows.GetModuleHandleEx(0, nil, &handle); err != nil {
		return 0
	}
	return handle
}

// loadAppIcon 按目标尺寸加载内置应用图标；失败返回 0，调用方回退到系统默认图标。
func loadAppIcon(size int) uintptr {
	handle := moduleHandle()
	if handle == 0 || size <= 0 {
		return 0
	}
	icon, found := callOptional(procLoadImageW, uintptr(handle), desktopIconID, imageIcon, uintptr(size), uintptr(size), 0)
	if !found || icon == 0 {
		return 0
	}
	return icon
}

func systemIconSize(metric uintptr, fallback int) int {
	if size, found := callOptional(procGetSystemMetrics, metric); found && size > 0 {
		return int(size)
	}
	return fallback
}

// applyWindowIcon 给标题栏（小图标）和任务栏/Alt+Tab（大图标）分别设置合适尺寸的图标，
// 避免 Windows 缩放图标导致发虚。
func applyWindowIcon(hwnd windows.HWND) {
	if hwnd == 0 {
		return
	}
	if small := loadAppIcon(systemIconSize(smCxSmIcon, 16)); small != 0 {
		procSendMessageW.Call(uintptr(hwnd), wmSetIcon, iconSmall, small)
	}
	if big := loadAppIcon(systemIconSize(smCxIcon, 32)); big != 0 {
		procSendMessageW.Call(uintptr(hwnd), wmSetIcon, iconBig, big)
	}
}

// trayIcon 返回托盘图标：优先内置应用图标，取不到时回退系统默认图标。
func trayIcon() uintptr {
	if icon := loadAppIcon(systemIconSize(smCxSmIcon, 16)); icon != 0 {
		return icon
	}
	icon, _, _ := procLoadIconW.Call(0, idiApplication)
	return icon
}

// systemPrefersDarkAppTheme 读取系统「应用模式」是否为深色；读不到时按深色处理，
// 与前端 prefers-color-scheme 的默认值保持一致。
func systemPrefersDarkAppTheme() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return true
	}
	defer key.Close()
	value, _, err := key.GetIntegerValue("AppsUseLightTheme")
	if err != nil {
		return true
	}
	return value == 0
}

// applyTitleBarTheme 设置窗口标题栏亮暗（Windows 10 1809+；旧系统忽略失败）。
func applyTitleBarTheme(hwnd windows.HWND, dark bool) bool {
	if hwnd == 0 {
		return false
	}
	value := int32(0)
	if dark {
		value = 1
	}
	size := uint32(unsafe.Sizeof(value))
	if err := windows.DwmSetWindowAttribute(hwnd, dwmwaImmersiveDark, unsafe.Pointer(&value), size); err == nil {
		return true
	}
	return windows.DwmSetWindowAttribute(hwnd, dwmwaImmersiveDark2, unsafe.Pointer(&value), size) == nil
}

// applyDesktopTheme 由 /api/desktop/theme 调用：把应用主题同步到原生标题栏。
// 返回 false 表示当前没有桌面窗口（服务模式）或系统不支持。
func applyDesktopTheme(theme string) bool {
	dark := theme != "light"
	if theme != "light" && theme != "dark" {
		dark = systemPrefersDarkAppTheme()
	}
	return applyTitleBarTheme(desktopWindowHandle, dark)
}

func runDesktop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// 必须在创建任何窗口之前声明 DPI 感知，否则系统会拉伸整个窗口位图导致界面发虚。
	dpiAwareness := enableDesktopDpiAwareness()
	root := filepath.Dir(env("DB_PATH", defaultDBPath()))
	if err := os.MkdirAll(root, 0700); err != nil {
		desktopError("数据目录不可写，请将程序解压到有写权限的目录：" + err.Error())
		return
	}
	// A per-database mutex prevents repeated launches from opening one SQLite
	// database twice. It does not claim a different user-selected database.
	abs, err := filepath.Abs(env("DB_PATH", defaultDBPath()))
	if err != nil {
		desktopError(err.Error())
		return
	}
	name := fmt.Sprintf(`Local\AutoAnime-%x`, sha256.Sum256([]byte(strings.ToLower(abs))))
	mutex, err := windows.CreateMutex(nil, false, desktopText(name))
	if mutex != 0 {
		defer windows.CloseHandle(mutex)
	}
	if err == windows.ERROR_ALREADY_EXISTS {
		desktopError("Auto-Anime 已在运行，请从系统托盘打开窗口。")
		return
	}
	if err != nil {
		desktopError("无法创建实例锁：" + err.Error())
		return
	}
	logFile, err := os.OpenFile(filepath.Join(root, "desktop.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		desktopError("无法写入启动日志：" + err.Error())
		return
	}
	defer logFile.Close()
	previousLog := log.Writer()
	log.SetOutput(logFile)
	defer log.SetOutput(previousLog)
	windowWidth, windowHeight, dpi := desktopWindowSize()
	darkTitleBar := systemPrefersDarkAppTheme()
	log.Printf("窗口 DPI 感知 %s，系统 DPI %d（%d%% 缩放），初始尺寸 %dx%d，标题栏初始%s主题", dpiAwareness, dpi, dpi*100/desktopDpiBaseline, windowWidth, windowHeight, themeLabel(darkTitleBar))
	if _, err := webviewloader.GetInstalledVersion(); err != nil {
		desktopError("未检测到 Microsoft Edge WebView2 Runtime，请安装运行时后重试：" + err.Error())
		return
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", port))
	if err != nil {
		desktopError("本地端口不可用，请退出已运行的实例或设置 PORT 后重试：" + err.Error())
		return
	}
	defer listener.Close()
	url := "http://" + listener.Addr().String()
	if os.Getenv("DOWNLOAD_MODE") == "" {
		if exe, err := os.Executable(); err == nil && bundledQbitExecutable(filepath.Dir(exe)) != "" {
			downloadMode = "builtin"
		}
	}
	w := webview.NewWithOptions(webview.WebViewOptions{
		DataPath: filepath.Join(root, "webview2"), AutoFocus: true,
		WindowOptions: webview.WindowOptions{Title: "Auto-Anime 自动追番", Width: uint(windowWidth), Height: uint(windowHeight), Center: true, IconId: desktopIconID},
	})
	if w == nil {
		desktopError("无法创建桌面窗口，请安装 Microsoft Edge WebView2 Runtime 后重试。日志位于数据目录的 desktop.log。")
		return
	}
	defer w.Destroy()
	w.SetSize(scaleForDpi(desktopWindowMinWidth, dpi), scaleForDpi(desktopWindowMinHeight, dpi), webview.HintMin)
	// 记录窗口句柄供 /api/desktop/theme 更新标题栏；同时按尺寸设置标题栏与任务栏图标。
	hwnd := windows.HWND(uintptr(w.Window()))
	desktopWindowHandle = hwnd
	defer func() { desktopWindowHandle = 0 }()
	applyWindowIcon(hwnd)
	applyTitleBarTheme(hwnd, darkTitleBar)
	w.SetHtml(`<!doctype html><meta charset="utf-8"><style>body{font:18px system-ui;padding:48px;color:#333}@media(prefers-color-scheme:dark){body{background:#18181b;color:#eee}}</style><h1>Auto-Anime</h1><p>正在启动服务与下载器，请稍候…</p><p>关闭窗口后继续在系统托盘运行；右键托盘图标可退出。</p>`)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	exiting := false
	removeTray, err := installDesktopTray(uintptr(w.Window()), func() {
		if !exiting {
			exiting = true
			w.SetTitle("Auto-Anime · 正在停止下载器，请稍候")
			cancel()
		}
	})
	if err != nil {
		desktopError(err.Error())
		return
	}
	defer removeTray()
	stopped := make(chan struct{})
	defer func() {
		cancel()
		<-stopped
	}()
	go func() {
		defer close(stopped)
		err := runApplicationContext(ctx, listener, func() { w.Dispatch(func() { w.Navigate(url) }) })
		w.Dispatch(func() {
			if err != nil && !exiting {
				desktopError("服务启动或运行失败：" + err.Error() + "\n请查看数据目录的 desktop.log。")
			}
			w.Terminate()
		})
	}()
	w.Run()
}

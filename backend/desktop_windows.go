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

func runDesktop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
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
		WindowOptions: webview.WindowOptions{Title: "Auto-Anime 自动追番", Width: 1100, Height: 760, Center: true},
	})
	if w == nil {
		desktopError("无法创建桌面窗口，请安装 Microsoft Edge WebView2 Runtime 后重试。日志位于数据目录的 desktop.log。")
		return
	}
	defer w.Destroy()
	w.SetSize(420, 480, webview.HintMin)
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

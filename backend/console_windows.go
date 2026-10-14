//go:build windows

package main

import (
	"log"
	"os"

	"golang.org/x/sys/windows"
)

// Windows 发布包用 `-H windowsgui` 构建（桌面版双击不弹控制台）。GUI 子系统进程
// 默认不连接到父进程的控制台，所以 `Auto-Anime.exe --server` 或 `-version` 从
// 终端启动时看不到日志。这里附着到「已经存在」的父控制台并重开标准流，不会新开
// 窗口；没有父控制台（双击、服务方式）时保持静默。

var (
	consoleKernel32   = windows.NewLazySystemDLL("kernel32.dll")
	procAttachConsole = consoleKernel32.NewProc("AttachConsole")
	procGetConsoleWin = consoleKernel32.NewProc("GetConsoleWindow")
)

const attachParentProcess = ^uintptr(0) // ATTACH_PARENT_PROCESS

func attachParentConsole() {
	if window, _, _ := procGetConsoleWin.Call(); window != 0 {
		return // 已经连着控制台（控制台子系统构建、或从终端直接运行）。
	}
	if ok, _, _ := procAttachConsole.Call(attachParentProcess); ok == 0 {
		return // 父进程没有控制台：桌面双击或作为服务运行，保持无控制台。
	}
	reopened := false
	if file, ok := reopenConsoleStream(windows.STD_INPUT_HANDLE, "CONIN$", os.O_RDONLY); ok {
		os.Stdin = file
	}
	if file, ok := reopenConsoleStream(windows.STD_OUTPUT_HANDLE, "CONOUT$", os.O_WRONLY); ok {
		os.Stdout = file
		reopened = true
	}
	if file, ok := reopenConsoleStream(windows.STD_ERROR_HANDLE, "CONOUT$", os.O_WRONLY); ok {
		os.Stderr = file
		reopened = true
	}
	if reopened {
		// log 包在初始化时就记住了旧的 os.Stderr，必须显式换掉输出目标。
		log.SetOutput(os.Stderr)
	}
}

// reopenConsoleStream 只在标准句柄无效或本来就是控制台句柄时重开控制台设备；
// 已经重定向到文件或管道的句柄保持原样，避免覆盖 `Auto-Anime.exe --server > log.txt`。
func reopenConsoleStream(stdHandle uint32, name string, flag int) (*os.File, bool) {
	handle, err := windows.GetStdHandle(stdHandle)
	if err == nil && handle != 0 && handle != windows.InvalidHandle {
		if fileType, err := windows.GetFileType(handle); err == nil && fileType != windows.FILE_TYPE_CHAR {
			return nil, false
		}
	}
	file, err := os.OpenFile(name, flag, 0)
	if err != nil {
		return nil, false
	}
	return file, true
}

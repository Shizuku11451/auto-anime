//go:build windows

package main

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// NOTIFYICONDATAW, including the union uTimeout/uVersion and GUID.
type desktopNotifyIcon struct {
	Size                uint32
	Window              uintptr
	ID, Flags, Callback uint32
	Icon                uintptr
	Tip                 [128]uint16
	State, StateMask    uint32
	Info                [256]uint16
	Version             uint32
	InfoTitle           [64]uint16
	InfoFlags           uint32
	GUID                windows.GUID
	BalloonIcon         uintptr
}

func installDesktopTray(hwnd uintptr, quit func()) (func(), error) {
	const trayMessage = 0x8002
	const exitCommand = 0xa010
	shell := windows.NewLazySystemDLL("shell32.dll")
	controls := windows.NewLazySystemDLL("comctl32.dll")
	notify := shell.NewProc("Shell_NotifyIconW")
	// 托盘用内置应用图标（与标题栏、任务栏一致），取不到时回退系统默认图标。
	icon := trayIcon()
	data := desktopNotifyIcon{Window: hwnd, ID: 1, Flags: 1 | 2 | 4, Callback: trayMessage, Icon: icon}
	data.Size = uint32(unsafe.Sizeof(data))
	copy(data.Tip[:], windows.StringToUTF16("Auto-Anime · 双击打开，右键退出"))
	add := func() bool { ok, _, _ := notify.Call(0, uintptr(unsafe.Pointer(&data))); return ok != 0 }
	if !add() {
		return nil, fmt.Errorf("无法创建系统托盘图标，未启动后台服务")
	}
	trayAvailable := true
	// Keep an explicit exit reachable with Alt+Space even if Explorer's tray
	// is hidden. Closing the window retains its background behavior.
	systemMenu, _, _ := desktopUser32.NewProc("GetSystemMenu").Call(hwnd, 0)
	if systemMenu != 0 {
		desktopUser32.NewProc("AppendMenuW").Call(systemMenu, 0x0800, 0, 0)
		desktopUser32.NewProc("AppendMenuW").Call(systemMenu, 0, exitCommand, uintptr(unsafe.Pointer(desktopText("退出 Auto-Anime"))))
	}
	taskbarCreated, _, _ := desktopUser32.NewProc("RegisterWindowMessageW").Call(uintptr(unsafe.Pointer(desktopText("TaskbarCreated"))))
	show := func() {
		desktopUser32.NewProc("ShowWindow").Call(hwnd, 9)
		desktopUser32.NewProc("SetForegroundWindow").Call(hwnd)
	}
	callback := windows.NewCallback(func(window uintptr, msg uint32, wp, lp, id, ref uintptr) uintptr {
		switch {
		case msg == 0x0112 && wp&0xfff0 == exitCommand: // WM_SYSCOMMAND
			quit()
			return 0
		case msg == 0x0010: // WM_CLOSE hides; explicit tray exit shuts down.
			if trayAvailable {
				desktopUser32.NewProc("ShowWindow").Call(hwnd, 0)
			} else {
				quit()
			}
			return 0
		case msg == 0x0011: // WM_QUERYENDSESSION
			return 1
		case msg == 0x0016 && wp != 0: // WM_ENDSESSION
			quit()
			return 0
		case uintptr(msg) == taskbarCreated:
			trayAvailable = add()
			if !trayAvailable {
				show()
			} // Never strand an invisible process after Explorer restart.
		case msg == trayMessage:
			if lp == 0x0203 {
				show()
			} // WM_LBUTTONDBLCLK
			if lp == 0x0205 { // WM_RBUTTONUP
				menu, _, _ := desktopUser32.NewProc("CreatePopupMenu").Call()
				if menu == 0 {
					show()
					return 0
				}
				defer desktopUser32.NewProc("DestroyMenu").Call(menu)
				desktopUser32.NewProc("AppendMenuW").Call(menu, 0, 1, uintptr(unsafe.Pointer(desktopText("打开 Auto-Anime"))))
				desktopUser32.NewProc("AppendMenuW").Call(menu, 0, 2, uintptr(unsafe.Pointer(desktopText("退出并停止下载器"))))
				var point struct{ X, Y int32 }
				desktopUser32.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&point)))
				desktopUser32.NewProc("SetForegroundWindow").Call(hwnd)
				choice, _, _ := desktopUser32.NewProc("TrackPopupMenu").Call(menu, 0x0102, uintptr(point.X), uintptr(point.Y), 0, hwnd, 0)
				if choice == 1 {
					show()
				}
				if choice == 2 {
					quit()
				}
				desktopUser32.NewProc("PostMessageW").Call(hwnd, 0, 0, 0)
			}
			return 0
		}
		value, _, _ := controls.NewProc("DefSubclassProc").Call(window, uintptr(msg), wp, lp)
		return value
	})
	ok, _, _ := controls.NewProc("SetWindowSubclass").Call(hwnd, callback, 1, 0)
	if ok == 0 {
		notify.Call(2, uintptr(unsafe.Pointer(&data)))
		return nil, fmt.Errorf("无法安装桌面窗口事件处理器")
	}
	return func() {
		controls.NewProc("RemoveWindowSubclass").Call(hwnd, callback, 1)
		notify.Call(2, uintptr(unsafe.Pointer(&data)))
	}, nil
}

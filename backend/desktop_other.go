//go:build !windows

package main

func desktopRequested() bool { return false }
func runDesktop()            {}

func acquireApplicationLock(string) (func(), error) { return func() {}, nil }

// 非 Windows 没有桌面窗口与原生标题栏，主题同步请求直接返回未应用。
func applyDesktopTheme(string) bool { return false }

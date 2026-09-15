//go:build windows

package main

import "errors"

// execReplace 在 Windows 上无法替换运行中的进程映像；重新启动由调用方通过
// 启动新进程完成（见 startReplacementProcess）。
func execReplace(path string, args []string) error {
	return errors.New("Windows 上不支持就地替换进程映像")
}

// inPlaceExec 为 false：Windows 无法覆盖正在运行的可执行文件，更新包只会
// 暂存并由下次启动的引导步骤应用。
func inPlaceExec() bool { return false }

// restartInPlace 为 false：通过启动新进程再退出当前进程完成重启。
func restartInPlace() bool { return false }

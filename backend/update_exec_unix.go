//go:build !windows

package main

import (
	"os"
	"syscall"
)

// execReplace 用新程序替换当前进程映像：PID 与容器不变，端口不会被两个
// 进程同时占用，也不需要额外的进程管理（Linux/Unix）。
func execReplace(path string, args []string) error {
	argv := append([]string{path}, args[1:]...)
	return syscall.Exec(path, argv, os.Environ())
}

// inPlaceExec 报告当前平台能否就地替换运行中的可执行文件。
func inPlaceExec() bool { return true }

// restartInPlace 报告重启是否应通过替换进程映像完成。
func restartInPlace() bool { return true }

//go:build !windows

package main

// 非 Windows 平台的标准输出本来就是终端、管道或文件，无需额外处理。
func attachParentConsole() {}

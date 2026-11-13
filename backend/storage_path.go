package main

import "path/filepath"

// cleanStoragePath 统一成正斜杠并清理多余分隔符。
func cleanStoragePath(value string) string {
	return filepath.ToSlash(filepath.Clean(value))
}

// storageExampleDir 是给用户看的绝对路径示例（NAS 挂载路径）。
const storageExampleDir = "/vol1/1000/番剧"

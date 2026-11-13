package main

import "os"

func archivePathIsLink(_ string, info os.FileInfo) bool {
	return info.Mode()&os.ModeSymlink != 0
}

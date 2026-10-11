//go:build !windows

package main

import "os"

func commitWithoutHardlink(_, _ string, linkError error) error { return linkError }

func archivePathIsLink(_ string, info os.FileInfo) bool {
	return info.Mode()&os.ModeSymlink != 0
}

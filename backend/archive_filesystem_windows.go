//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// Staging and destination share the archive root. MoveFileW supports exFAT
// and SMB without a hardlink and refuses existing destinations, unlike Rename.
func commitWithoutHardlink(source, destination string, _ error) error {
	src, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	dst, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	if err := windows.MoveFile(src, dst); err != nil {
		return &os.LinkError{Op: "move", Old: source, New: destination, Err: err}
	}
	return nil
}

func archivePathIsLink(path string, info os.FileInfo) bool {
	if info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return true
	}
	attrs, err := windows.GetFileAttributes(p)
	return err != nil || attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
}

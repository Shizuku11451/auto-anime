//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// Share mode zero uses the filesystem's identity/case rules and releases on
// process death. Both --server and the desktop entry acquire the same lock.
func acquireApplicationLock(database string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(database), 0700); err != nil {
		return nil, err
	}
	name, err := windows.UTF16PtrFromString(database + ".lock")
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, fmt.Errorf("数据库目录不可写或已被另一个实例使用：%w", err)
	}
	return func() {
		if handle != windows.InvalidHandle {
			windows.CloseHandle(handle)
			handle = windows.InvalidHandle
		}
	}, nil
}

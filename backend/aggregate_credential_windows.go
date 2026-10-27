//go:build windows

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

func localAggregateSearchToken() (string, error) {
	return readAggregateSearchToken(filepath.Join(windowsProgramDir(), "secrets", "aggregate-token.dpapi"))
}

func readAggregateSearchToken(path string) (string, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 16385))
	if err != nil || len(data) == 0 || len(data) > 16384 {
		return "", fmt.Errorf("invalid protected credential")
	}
	in := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return "", fmt.Errorf("cannot decrypt protected credential")
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	plain := unsafe.Slice(out.Data, int(out.Size))
	defer clear(plain)
	if len(plain) < 32 || len(plain) > 4096 {
		return "", fmt.Errorf("invalid credential length")
	}
	return string(plain), nil
}

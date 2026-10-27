//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestProtectedAggregateSearchToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credential.dpapi")
	if token, err := readAggregateSearchToken(path); token != "" || err != nil {
		t.Fatal("missing credential must leave aggregation disabled")
	}
	want := []byte("test-protected-aggregate-token-not-a-real-secret")
	in := windows.DataBlob{Size: uint32(len(want)), Data: &want[0]}
	var out windows.DataBlob
	if err := windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		t.Fatal(err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	if err := os.WriteFile(path, unsafe.Slice(out.Data, int(out.Size)), 0600); err != nil {
		t.Fatal(err)
	}
	if token, err := readAggregateSearchToken(path); err != nil || token != string(want) {
		t.Fatal("protected credential round trip failed")
	}
	for _, data := range []string{"", "invalid", strings.Repeat("x", 16385)} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if token, err := readAggregateSearchToken(path); token != "" || err == nil {
			t.Fatal("invalid protected credential accepted")
		}
	}
}

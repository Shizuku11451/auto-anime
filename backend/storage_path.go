package main

import "path/filepath"

// filepath.Clean preserves Windows UNC volumes; path.Clean would collapse
// //server/share into /server/share and silently target the current drive.
func cleanStoragePath(value string) string {
	return filepath.ToSlash(filepath.Clean(value))
}

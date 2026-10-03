package main

import (
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// Only retire GIDs recorded by the removed private Aria2 instance. An
// explicitly configured external endpoint or a different download directory
// is not evidence of ownership, even if an old private session still exists.
func legacyEmbeddedAria2Tasks(values map[string]string) ([]string, error) {
	cfg := runtimeConfig()
	setting := func(key, fallback string) string {
		if value, ok := values[key]; ok {
			return strings.TrimSpace(value)
		}
		return strings.TrimSpace(fallback)
	}
	if setting("download_mode", cfg.downloadMode) != "builtin" || strings.TrimSpace(os.Getenv("ARIA2_URL")) != "" {
		return nil, nil
	}
	root, err := filepath.Abs(filepath.Join(downloadDir, ".aria2"))
	if err != nil {
		return nil, err
	}
	if endpoint := setting("aria2_url", cfg.aria2URL); endpoint != "" {
		u, err := url.Parse(endpoint)
		if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.Path != "/jsonrpc" || u.RawQuery != "" || u.User != nil {
			return nil, nil
		}
		for key, fallback := range map[string]string{"aria2_dir": cfg.aria2Dir, "aria2_local_dir": cfg.aria2LocalDir} {
			dir := setting(key, fallback)
			absolute, err := filepath.Abs(dir)
			if dir == "" || err != nil || absolute != root {
				return nil, nil
			}
		}
	}
	file, err := os.Open(filepath.Join(root, "session.txt"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取旧内置 Aria2 会话失败: %w", err)
	}
	defer file.Close()
	const maxSession = 8 << 20
	raw, err := io.ReadAll(io.LimitReader(file, maxSession+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxSession || !utf8.Valid(raw) {
		return nil, fmt.Errorf("旧内置 Aria2 会话不是有效的 UTF-8 文件或超过大小限制，未修改旧任务")
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || key != "gid" || len(value) != 16 {
			continue
		}
		if _, err := hex.DecodeString(value); err == nil {
			seen[strings.ToLower(value)] = true
		}
	}
	gids := make([]string, 0, len(seen))
	for gid := range seen {
		gids = append(gids, gid)
	}
	sort.Strings(gids)
	return gids, nil
}

// Run in the same transaction as the mode change or startup recovery. Keep
// the old identity, progress and files for diagnosis and a deliberate retry.
func retireLegacyAria2Tasks(tx *sql.Tx, gids []string) error {
	for _, gid := range gids {
		if _, err := tx.Exec("UPDATE items SET status='下载失败',task_error=? WHERE downloader='aria2' AND task_id=? AND status IN ("+activeTaskStatusSQL+")",
			"内置下载器已升级为 qBittorrent，请点击下载重新提交；原 Aria2 文件已保留", gid); err != nil {
			return err
		}
	}
	return nil
}

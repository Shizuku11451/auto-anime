package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type sourceCleanup struct {
	taskRef
	Archive    string
	Files      []archiveFile
	Root       string
	Stopped    bool
	Connection string
}

func cleanupConnection(downloader string) string {
	cfg := runtimeConfig()
	var fields []string
	switch downloader {
	case "builtin":
		fields = []string{downloadDir, env("QBITTORRENT_DATA_DIR", "")}
	case "qbittorrent":
		fields = []string{cfg.qbitURL, cfg.qbitUser, cfg.qbitPassword, cfg.qbitSavePath, cfg.qbitLocalDir}
	case "aria2":
		fields = []string{cfg.aria2URL, cfg.aria2Token, cfg.aria2Dir, cfg.aria2LocalDir}
	case "transmission":
		fields = []string{cfg.transmissionURL, cfg.transmissionUser, cfg.transmissionPassword, cfg.transmissionDownloadDir, cfg.transmissionLocalDir}
	}
	data, _ := json.Marshal(fields)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func sourceRootFor(downloader string) string {
	cfg := runtimeConfig()
	switch downloader {
	case "builtin":
		return downloadDir
	case "qbittorrent":
		if cfg.qbitLocalDir != "" {
			return cfg.qbitLocalDir
		}
		return cfg.qbitSavePath
	case "aria2":
		if cfg.aria2LocalDir != "" {
			return cfg.aria2LocalDir
		}
		return cfg.aria2Dir
	case "transmission":
		if cfg.transmissionLocalDir != "" {
			return cfg.transmissionLocalDir
		}
		return cfg.transmissionDownloadDir
	}
	return ""
}

func queueSourceCleanup(t taskRef, archive string, files []archiveFile) error {
	item, err := readControlledItem(t.ID)
	if err != nil {
		return err
	}
	if item.Downloader != t.Downloader || item.SubmittedAt != t.SubmittedAt {
		return fmt.Errorf("下载任务已变化")
	}
	t = item.taskRef
	manifest := append([]archiveFile(nil), files...)
	for i := range manifest {
		info, err := os.Lstat(manifest[i].Src)
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("下载源文件不可访问，保留原文件")
		}
		manifest[i].CleanupSize = info.Size()
		manifest[i].CleanupModTime = info.ModTime().UnixNano()
	}
	data, err := json.Marshal(struct {
		Files      []archiveFile
		Connection string
	}{manifest, cleanupConnection(t.Downloader)})
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO source_cleanup(item_id,downloader,task_id,submitted_at,archive_path,files,source_root)
		VALUES(?,?,?,?,?,?,?) ON CONFLICT(item_id) DO NOTHING`, t.ID, t.Downloader, t.TaskID, t.SubmittedAt, archive, string(data), sourceRootFor(t.Downloader))
	return err
}

// Read and close rows before network/file work: SQLite has one connection.
func processSourceCleanup() {
	rows, err := db.Query(`SELECT item_id,downloader,task_id,submitted_at,archive_path,files,source_root,stopped FROM source_cleanup LIMIT 100`)
	if err != nil {
		return
	}
	var pending []sourceCleanup
	for rows.Next() {
		var c sourceCleanup
		var data string
		var manifest struct {
			Files      []archiveFile
			Connection string
		}
		if rows.Scan(&c.ID, &c.Downloader, &c.TaskID, &c.SubmittedAt, &c.Archive, &data, &c.Root, &c.Stopped) == nil && json.Unmarshal([]byte(data), &manifest) == nil {
			c.Files, c.Connection = manifest.Files, manifest.Connection
			pending = append(pending, c)
		}
	}
	rows.Close()
	for _, c := range pending {
		l, release := downloadLock(c.ID)
		l.operation.Lock()
		animeCleanupMu.RLock()
		if err := cleanupSource(c); err != nil {
			db.Exec(`UPDATE items SET task_error=? WHERE id=? AND status='已保存' AND downloader=? AND task_id=?`, "媒体已保存，源文件清理待重试："+err.Error(), c.ID, c.Downloader, c.TaskID)
		}
		animeCleanupMu.RUnlock()
		l.operation.Unlock()
		release()
	}
}

func cleanupSource(c sourceCleanup) error {
	item, err := readControlledItem(c.ID)
	if err != nil {
		var action *downloadActionError
		if !errors.As(err, &action) || action.Code != 404 {
			return err
		}
		_, err = db.Exec("DELETE FROM source_cleanup WHERE item_id=?", c.ID)
		return err
	}
	if item.Downloader != c.Downloader || item.TaskID != c.TaskID || item.SubmittedAt != c.SubmittedAt {
		_, err = db.Exec("DELETE FROM source_cleanup WHERE item_id=?", c.ID)
		return err
	}
	if item.Status != "已保存" {
		return nil
	} // Archive commit/recovery has not succeeded yet.
	var archive string
	if err := db.QueryRow("SELECT file_path FROM items WHERE id=?", c.ID).Scan(&archive); err != nil {
		return err
	}
	if runtimeConfig().keepSourceFiles {
		_, err := db.Exec("DELETE FROM source_cleanup WHERE item_id=?", c.ID)
		return err
	}
	if c.Connection != cleanupConnection(c.Downloader) {
		return fmt.Errorf("下载器连接或映射已变化，保留原文件")
	}
	if cleanStoragePath(archive) != cleanStoragePath(c.Archive) {
		return fmt.Errorf("归档位置已变化，保留原文件")
	}
	if _, err := os.Stat(archive); err != nil {
		return fmt.Errorf("归档媒体不可访问，保留原文件")
	}
	if c.TaskID == "" {
		return fmt.Errorf("尚未确认下载任务标识，保留原文件")
	}
	var shared int
	if err := db.QueryRow(`SELECT COUNT(*) FROM items WHERE id<>? AND downloader=? AND task_id=?`, c.ID, c.Downloader, c.TaskID).Scan(&shared); err != nil {
		return err
	}
	if shared != 0 {
		return fmt.Errorf("其他条目共用此任务，保留原文件")
	}
	paths, err := cleanupPaths(c)
	if err != nil {
		return err
	}
	if !c.Stopped {
		if err := stopSourceTask(c); err != nil {
			return err
		}
		if _, err := db.Exec("UPDATE source_cleanup SET stopped=1 WHERE item_id=?", c.ID); err != nil {
			return err
		}
	}
	// Revalidate after potentially slow downloader RPCs before touching sources.
	paths, err = cleanupPaths(c)
	if err != nil {
		return err
	}
	for _, file := range paths {
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("源文件删除失败，请检查映射和权限")
		}
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM source_cleanup WHERE item_id=?", c.ID); err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE items SET task_error='' WHERE id=? AND status='已保存' AND downloader=? AND task_id=?", c.ID, c.Downloader, c.TaskID); err != nil {
		return err
	}
	return tx.Commit()
}

// Validate the whole set before removing anything. Never delete directories,
// symlinked paths, the archive itself, or sources outside the configured root.
func cleanupPaths(c sourceCleanup) ([]string, error) {
	if c.Root == "" {
		return nil, fmt.Errorf("归档时未配置下载器本地映射根目录，已保留原文件，请手动处理")
	}
	root, err := filepath.EvalSymlinks(c.Root)
	if err != nil {
		return nil, fmt.Errorf("下载器源目录不可访问")
	}
	root, err = filepath.Abs(root)
	if err != nil || filepath.Dir(root) == root {
		return nil, fmt.Errorf("下载器源目录不能是文件系统根目录")
	}
	archive, err := os.Stat(c.Archive)
	if err != nil {
		return nil, fmt.Errorf("归档媒体不可访问")
	}
	var paths []string
	for _, file := range c.Files {
		info, err := os.Lstat(file.Src)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() || archivePathIsLink(file.Src, info) {
			return nil, fmt.Errorf("源文件不是普通文件，保留原文件")
		}
		if file.CleanupModTime != 0 && (file.CleanupSize != info.Size() || file.CleanupModTime != info.ModTime().UnixNano()) {
			return nil, fmt.Errorf("源文件已被替换或修改，保留原文件")
		}
		resolved, err := filepath.EvalSymlinks(file.Src)
		if err != nil {
			return nil, fmt.Errorf("源文件不可访问")
		}
		resolved, err = filepath.Abs(resolved)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(root, resolved)
		if err != nil || rel == "." || !filepath.IsLocal(rel) {
			return nil, fmt.Errorf("源文件超出下载器映射目录，保留原文件")
		}
		archiveRel, relErr := filepath.Rel(filepath.Dir(c.Archive), resolved)
		if os.SameFile(info, archive) || (relErr == nil && filepath.IsLocal(archiveRel)) {
			return nil, fmt.Errorf("源文件与归档媒体重叠，保留原文件")
		}
		paths = append(paths, resolved)
	}
	return paths, nil
}

var aria2CleanupGID = regexp.MustCompile(`^[0-9a-fA-F]{16}$`)

func stopSourceTask(c sourceCleanup) error {
	switch c.Downloader {
	case "builtin", "qbittorrent":
		_, err := controlDownloaderTask(c.taskRef, true)
		return err
	case "transmission":
		_, err := controlDownloaderTask(c.taskRef, true)
		if err != nil {
			return err
		}
		var out struct {
			Result    string `json:"result"`
			Arguments struct {
				Torrents []struct {
					ID     int `json:"id"`
					Status int `json:"status"`
				} `json:"torrents"`
			} `json:"arguments"`
		}
		id, err := strconv.Atoi(c.TaskID)
		if err != nil || id < 0 {
			return fmt.Errorf("Transmission 任务标识无效")
		}
		if err := (&transmissionClient{}).call(map[string]any{"method": "torrent-get", "arguments": map[string]any{"ids": []int{id}, "fields": []string{"id", "status"}}}, &out); err != nil || out.Result != "success" || len(out.Arguments.Torrents) != 1 || out.Arguments.Torrents[0].ID != id || out.Arguments.Torrents[0].Status != 0 {
			return fmt.Errorf("下载器尚未确认停止，保留原文件")
		}
		return nil
	case "aria2":
		if !aria2CleanupGID.MatchString(c.TaskID) {
			return fmt.Errorf("Aria2 任务标识无效")
		}
		var out struct {
			Status string `json:"status"`
			GID    string `json:"gid"`
		}
		if err := aria2RPC("aria2.tellStatus", []any{c.TaskID, []string{"gid", "status"}}, &out); err != nil {
			return fmt.Errorf("无法确认 Aria2 任务状态")
		}
		if !strings.EqualFold(out.GID, c.TaskID) {
			return fmt.Errorf("Aria2 任务身份不一致")
		}
		if out.Status == "complete" || out.Status == "removed" || out.Status == "paused" {
			return nil
		}
		var gid string
		if err := aria2RPC("aria2.pause", []any{c.TaskID}, &gid); err != nil || gid != c.TaskID {
			return fmt.Errorf("Aria2 未确认暂停")
		}
		if err := aria2RPC("aria2.tellStatus", []any{c.TaskID, []string{"gid", "status"}}, &out); err != nil || out.Status != "paused" || !strings.EqualFold(out.GID, c.TaskID) {
			return fmt.Errorf("Aria2 暂停尚未生效")
		}
		return nil
	}
	return fmt.Errorf("下载器不支持源文件清理")
}

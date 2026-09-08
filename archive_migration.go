package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var archiveMigrationMu sync.Mutex

// Source media is retained until new media, sidecars and the item update have
// all succeeded. A crash before the DB update may leave an extra copy, but
// never invalidates the path stored in the DB. Only files owned by this
// operation are removed on rollback; old files are cleaned up after commit.
func rearchiveSavedItem(id int64, oldPath string, animeID int64, title, episode string, season int, persist func(string) error) (string, error) {
	archiveMigrationMu.Lock()
	defer archiveMigrationMu.Unlock()
	m, err := archiveInfo(int(id))
	if err != nil {
		return "", err
	}
	source, rel, sidecars, err := migrationSource(oldPath, id)
	if err != nil {
		return "", err
	}
	root, seasonDir, base := archiveTarget(title, episode, m.Title, season)
	finalDir := filepath.Join(root, seasonDir)
	if err := checkMigrationParents(finalDir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(finalDir, 0755); err != nil {
		return "", err
	}
	// Never copy a resource directory into itself.
	if rel != "" {
		if relative, err := filepath.Rel(source, finalDir); err == nil && (relative == "." || filepath.IsLocal(relative)) {
			return "", fmt.Errorf("目标目录不能位于原资源目录内")
		}
	}
	stage, err := os.MkdirTemp(finalDir, ".rearchive-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage) // Freshly allocated staging directory only.
	created := []string{}
	committed := false
	defer func() {
		if !committed {
			for i := len(created) - 1; i >= 0; i-- {
				os.Remove(created[i])
			}
		}
	}()
	oldFiles, oldDirs := []string{}, []string{}
	m.AnimeID, m.AnimeTitle, m.Episode = animeID, title, episode
	m.Season, m.SavedAt = resolveSeason(title, season), time.Now().UTC().Format(time.RFC3339)
	metaDir, finalBase, newPath := finalDir, base, ""
	if rel == "" {
		work := filepath.Join(stage, "media")
		m.Size, err = copyArchiveFile(work, source)
		if err != nil {
			return "", err
		}
		for i := 1; ; i++ {
			finalBase = migrationName(base, i)
			newPath = filepath.Join(finalDir, finalBase+filepath.Ext(source))
			if err = publishMigrationFile(work, newPath); err == nil {
				created = append(created, newPath)
				break
			} else if !os.IsExist(err) {
				return "", err
			}
		}
		m.File = filepath.Base(newPath)
		oldFiles = append(oldFiles, source)
		oldFiles = append(oldFiles, sidecars...)
	} else {
		// A dedicated directory is reserved atomically, including for conflicts.
		for i := 1; ; i++ {
			finalBase = migrationName(base, i)
			metaDir = filepath.Join(finalDir, finalBase)
			if err = os.Mkdir(metaDir, 0755); err == nil {
				created = append(created, metaDir)
				break
			} else if !os.IsExist(err) {
				return "", err
			}
		}
		m.Size = 0
		err = filepath.WalkDir(source, func(p string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relative, err := filepath.Rel(source, p)
			if err != nil {
				return err
			}
			if entry.IsDir() {
				oldDirs = append(oldDirs, p)
				if relative == "." {
					return nil
				}
				dst := filepath.Join(metaDir, relative)
				if err := os.Mkdir(dst, 0755); err != nil {
					return err
				}
				created = append(created, dst)
				return nil
			}
			if !entry.Type().IsRegular() {
				return fmt.Errorf("不支持的文件类型：%s", entry.Name())
			}
			oldFiles = append(oldFiles, p)
			for _, sidecar := range sidecars {
				if p == sidecar {
					return nil // Generate corrected sidecars, never reuse old metadata.
				}
			}
			dst := filepath.Join(metaDir, relative)
			if err := publishMigrationFile(p, dst); err != nil {
				return err
			}
			created = append(created, dst)
			info, err := entry.Info()
			if err != nil {
				return err
			}
			m.Size += info.Size()
			return nil
		})
		if err != nil {
			return "", err
		}
		m.File, newPath = filepath.ToSlash(rel), filepath.Join(metaDir, rel)
	}
	if err := writeArchiveMeta(stage, m, finalBase); err != nil {
		return "", err
	}
	for _, suffix := range []string{".nfo", "-info.json"} {
		dst := filepath.Join(metaDir, finalBase+suffix)
		if err := publishMigrationFile(filepath.Join(stage, finalBase+suffix), dst); err != nil {
			return "", err
		}
		created = append(created, dst)
	}
	if err := persist(newPath); err != nil {
		return "", err
	}
	committed = true
	// Remove only the source files observed during preparation. New/unrelated
	// files and nonempty directories are retained, even if cleanup fails.
	warning := ""
	for _, p := range oldFiles {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			warning = "文件已迁移，部分旧文件未能清理，请检查原目录"
		}
	}
	for i := len(oldDirs) - 1; i >= 0; i-- {
		os.Remove(oldDirs[i])
	}
	cleanupEmptyDirs(filepath.Dir(source), root)
	writeTvshowNFO(&animeRow{Title: title})
	return warning, nil
}

func migrationName(base string, attempt int) string {
	if attempt == 1 {
		return base
	}
	return fmt.Sprintf("%s (%d)", base, attempt)
}

// Link immutable media when possible; across filesystems copy without removing
// the source. O_EXCL ensures a racing creator can never be overwritten/deleted.
func publishMigrationFile(src, dst string) (err error) {
	if err = os.Link(src, dst); err == nil || os.IsExist(err) {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := out.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			os.Remove(dst) // This call created dst exclusively.
		}
	}()
	if _, err = io.Copy(out, in); err == nil {
		err = out.Sync()
	}
	return err
}

func checkMigrationParents(p string) error {
	for {
		info, err := os.Lstat(p)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("迁移路径不可包含符号链接")
		}
		parent := filepath.Dir(p)
		if parent == p {
			return nil
		}
		p = parent
	}
}

// Recognize dedicated BT/legacy resource directories by their own manifest ID
// and relative file path, including nested torrents and " (2)" conflicts.
// A direct download's adjacent manifest takes precedence over ancestor scans.
func migrationSource(oldPath string, id int64) (root, rel string, sidecars []string, err error) {
	oldPath = filepath.Clean(oldPath)
	if !filepath.IsAbs(oldPath) {
		return "", "", nil, fmt.Errorf("原文件路径必须是绝对路径")
	}
	if err := checkMigrationParents(oldPath); err != nil {
		return "", "", nil, err
	}
	info, err := os.Lstat(oldPath)
	if err != nil {
		return "", "", nil, err
	}
	readMeta := func(p string) (*archiveMetadata, error) {
		info, err := os.Lstat(p)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("归档记录必须是普通文件")
		}
		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		var saved archiveMetadata
		if err := json.NewDecoder(io.LimitReader(f, 1<<20)).Decode(&saved); err != nil {
			return nil, err
		}
		if int64(saved.ID) != id {
			return nil, fmt.Errorf("归档记录不匹配")
		}
		return &saved, nil
	}
	base := strings.TrimSuffix(oldPath, filepath.Ext(oldPath))
	if info.Mode().IsRegular() {
		if _, err := readMeta(base + "-info.json"); err == nil {
			return oldPath, "", []string{base + ".nfo", base + "-info.json"}, nil
		}
	}
	for dir := filepath.Dir(oldPath); ; dir = filepath.Dir(dir) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return "", "", nil, err
		}
		for _, entry := range entries {
			if !entry.Type().IsRegular() || !(strings.HasSuffix(entry.Name(), "-info.json") || entry.Name() == "info.json") {
				continue
			}
			metaPath := filepath.Join(dir, entry.Name())
			saved, err := readMeta(metaPath)
			if err != nil || !filepath.IsLocal(filepath.FromSlash(saved.File)) || saved.File == "." || filepath.Clean(filepath.Join(dir, saved.File)) != oldPath {
				continue
			}
			metaBase := strings.TrimSuffix(entry.Name(), "-info.json")
			if entry.Name() == "info.json" {
				metaBase = strings.TrimSuffix(saved.File, filepath.Ext(saved.File))
			} else if filepath.Base(dir) != metaBase && !strings.HasPrefix(filepath.Base(dir), metaBase+" (") {
				continue
			}
			return dir, filepath.FromSlash(saved.File), []string{metaPath, filepath.Join(dir, metaBase+".nfo")}, nil
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	if info.Mode().IsRegular() {
		return oldPath, "", nil, nil
	}
	return "", "", nil, fmt.Errorf("无法确认原资源目录的归档记录，请先检查 info.json")
}

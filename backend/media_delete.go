package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// 删除记录时可选地一并删除已保存的媒体文件。只有用户明确选择时才会调用，
// 且只删除位于媒体库根目录、下载目录或番剧自定义存储位置之内、并能确认归属于该条目的文件：
// 归档时写下的 -info.json 能确认的整个资源（视频与 .nfo/-info.json，或专属资源目录），
// 否则只删除记录指向的那一个普通文件。下载器自己目录里的源文件不在此列。

type savedMedia struct {
	ID   int64
	Path string
}

type mediaDeleteResult struct {
	Deleted int      `json:"deleted_files"`
	Failed  []string `json:"file_errors,omitempty"`
}

func mediaRoots() []string {
	roots := []string{filepath.Clean(archiveBaseRoot()), filepath.Clean(downloadDir)}
	rows, err := db.Query("SELECT DISTINCT storage_dir FROM animes WHERE COALESCE(storage_dir,'')<>''")
	if err != nil {
		return roots
	}
	defer rows.Close()
	for rows.Next() {
		var dir string
		if rows.Scan(&dir) == nil {
			roots = append(roots, filepath.Clean(cleanStoragePath(dir)))
		}
	}
	return roots
}

func containingRoot(p string, roots []string) string {
	for _, root := range roots {
		if pathWithin(p, root) {
			return root
		}
	}
	return ""
}

// collectSavedMedia 读取已保存条目的文件路径；q 可以是事务，以便在删除记录前读取。读完立即关闭结果集。
func collectSavedMedia(q interface {
	Query(string, ...any) (*sql.Rows, error)
}, query string, args ...any) ([]savedMedia, error) {
	rows, err := q.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []savedMedia
	for rows.Next() {
		var media savedMedia
		if err := rows.Scan(&media.ID, &media.Path); err != nil {
			return nil, err
		}
		out = append(out, media)
	}
	return out, rows.Err()
}

func deleteSavedMedia(list []savedMedia) mediaDeleteResult {
	out := mediaDeleteResult{}
	if len(list) == 0 {
		return out
	}
	roots := mediaRoots()
	for _, media := range list {
		if err := deleteOneMedia(media, roots); err != nil {
			out.Failed = append(out.Failed, fmt.Sprintf("%s：%v", filepath.Base(media.Path), err))
			continue
		}
		out.Deleted++
	}
	return out
}

func deleteOneMedia(media savedMedia, roots []string) error {
	p := filepath.Clean(media.Path)
	if !filepath.IsAbs(p) {
		return fmt.Errorf("路径无效")
	}
	if _, err := os.Lstat(p); os.IsNotExist(err) {
		return nil // 文件已经不存在，视为已删除。
	}
	root := containingRoot(p, roots)
	if root == "" {
		return fmt.Errorf("不在媒体目录内，已保留")
	}
	source, rel, sidecars, err := migrationSource(p, media.ID)
	if err != nil {
		info, statErr := os.Lstat(p)
		if statErr != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("无法确认文件归属，已保留")
		}
		source, rel, sidecars = p, "", nil
	}
	if !pathWithin(source, root) || isStorageBase(source) {
		return fmt.Errorf("不在媒体目录内，已保留")
	}
	if rel != "" {
		if err := os.RemoveAll(source); err != nil {
			return err
		}
	} else {
		if err := os.Remove(source); err != nil && !os.IsNotExist(err) {
			return err
		}
		for _, sidecar := range sidecars {
			if err := os.Remove(sidecar); err != nil && !os.IsNotExist(err) {
				log.Printf("删除附属文件失败 %q: %v", sidecar, err)
			}
		}
	}
	cleanupEmptyDirs(filepath.Dir(source), root)
	return nil
}

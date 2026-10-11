package main

import (
	"database/sql"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ---- 番剧命名 ----
//
// 番剧库、番剧文件夹与归档文件名用同一个规范名，来源是 Bangumi 资料里的官方名字：
// 优先中文名（name_cn），其次日文原名（name），并剥掉季标记——季由 Season XX 目录与
// NFO 承载。资源标题里的字幕组、分辨率、编码、字幕语言标签都不会进番剧名和目录名。
//
// 已经有媒体内容的番剧目录不自动迁移：只有空目录或只含 poster/tvshow.nfo 这类本应用
// 产物时才跟着改名，避免在媒体库扫描期间搬动媒体文件（用户可在番剧库里手动迁移）。

// keepableSeriesEntries 是本应用写在番剧目录里的产物；目录只有它们时可以安全改名。
var keepableSeriesEntries = map[string]bool{
	"poster.jpg": true, "poster.png": true, "poster.webp": true, "poster.gif": true,
	"tvshow.nfo": true,
}

// standardizeAnimeTitle 用 Bangumi 资料把番剧名规范成官方名，并同步番剧记录、条目、
// 订阅目标名、元数据键与空目录。没有资料或已经规范时什么都不做。
func standardizeAnimeTitle(animeTitle string) {
	animeTitle = strings.TrimSpace(animeTitle)
	if animeTitle == "" {
		return
	}
	var name, nameCN string
	if err := db.QueryRow(`SELECT COALESCE(name,''),COALESCE(name_cn,'') FROM anime_meta WHERE anime_title=?`, animeTitle).
		Scan(&name, &nameCN); err != nil {
		if err != sql.ErrNoRows {
			log.Printf("番剧规范化读取资料失败 %q: %v", animeTitle, err)
		}
		return
	}
	standard := standardSeriesName(nameCN, name)
	if standard == animeTitle {
		return
	}
	if standard == "" || len([]rune(standard)) > titleMaxLength {
		log.Printf("番剧规范化跳过 %q：官方名不可用或超出长度限制", animeTitle)
		return
	}
	if err := renameAnime(animeTitle, standard); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// 有资料但番剧库里没有这一行：没有可改的名字，静默跳过。
			return
		}
		log.Printf("番剧规范化改名失败 %q → %q: %v", animeTitle, standard, err)
		return
	}
	log.Printf("番剧名已规范为官方名：%q → %q", animeTitle, standard)
	refreshSeriesPoster(standard)
}

// renameAnime 同步改名：番剧记录、条目归属、订阅目标名、元数据键，以及可安全改名的番剧目录。
func renameAnime(oldTitle, newTitle string) error {
	oldTitle, newTitle = strings.TrimSpace(oldTitle), strings.TrimSpace(newTitle)
	if oldTitle == "" || newTitle == "" || oldTitle == newTitle {
		return nil
	}
	// 改名会动 animes/items/feeds 与番剧目录，必须和 RSS 同步、归档互斥，所以取写锁；
	// 调用方不得已经持有 animeCleanupMu，否则这里会等自己。
	animeCleanupMu.Lock()
	defer animeCleanupMu.Unlock()
	var id int64
	var storageDir string
	if err := db.QueryRow("SELECT id,COALESCE(storage_dir,'') FROM animes WHERE title=?", oldTitle).Scan(&id, &storageDir); err != nil {
		return err
	}
	var taken int
	if err := db.QueryRow("SELECT COUNT(*) FROM animes WHERE title=? AND id!=?", newTitle, id).Scan(&taken); err != nil {
		return err
	}
	if taken > 0 {
		// 已经存在同名番剧：改名会合并两条记录，交给用户决定，不自动做。
		log.Printf("番剧改名跳过 %q → %q：已存在同名番剧", oldTitle, newTitle)
		return nil
	}
	// 目录先改名：只有确实移动成功才把新位置写回数据库，失败就沿用原目录。
	movedFrom, newStorageDir := "", storageDir
	if target := retargetSeriesDirPath(storageDir, oldTitle, newTitle); target != "" {
		if moveSeriesDirIfDisposable(storageDir, target) {
			movedFrom, newStorageDir = storageDir, target
		}
	}
	// 数据库没写成就把目录挪回去，避免库和目录两个名字。
	saved := false
	if movedFrom != "" {
		defer func() {
			if saved {
				return
			}
			if backErr := os.Rename(newStorageDir, movedFrom); backErr != nil {
				log.Printf("番剧目录回滚失败 %q → %q: %v", newStorageDir, movedFrom, backErr)
			}
		}()
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := renameAnimeTx(tx, id, oldTitle, newTitle); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE animes SET storage_dir=?,updated_at=? WHERE id=?`,
		newStorageDir, time.Now().UTC().Format(time.RFC3339), id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	saved = true
	return nil
}

// renameAnimeTx 做数据库侧的改名（番剧、条目、订阅目标名、元数据键）；由调用方提交事务。
func renameAnimeTx(tx *sql.Tx, id int64, oldTitle, newTitle string) error {
	for _, update := range []struct {
		query string
		args  []any
	}{
		{"DELETE FROM anime_meta WHERE anime_title=?", []any{newTitle}},
		{"UPDATE anime_meta SET anime_title=? WHERE anime_title=?", []any{newTitle, oldTitle}},
		{"UPDATE animes SET title=? WHERE id=?", []any{newTitle, id}},
		{"UPDATE items SET anime_title=? WHERE anime_id=?", []any{newTitle, id}},
		{"UPDATE feeds SET target_title=? WHERE target_title=?", []any{newTitle, oldTitle}},
	} {
		if _, err := tx.Exec(update.query, update.args...); err != nil {
			return err
		}
	}
	return nil
}

// retargetSeriesDirPath 把番剧目录路径的最后一段换成新番剧名；目录名与旧番剧名不一致
// （用户自定义过路径）时返回空字符串，表示不动。
func retargetSeriesDirPath(dir, oldTitle, newTitle string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return ""
	}
	clean := cleanStoragePath(dir)
	if filepath.Base(clean) != safeComponent(oldTitle) {
		return ""
	}
	target := cleanStoragePath(filepath.Join(filepath.Dir(clean), safeComponent(newTitle)))
	if target == clean {
		return ""
	}
	return target
}

// moveSeriesDirIfDisposable 只在目录为空或只含本应用产物时改名。
func moveSeriesDirIfDisposable(oldDir, newDir string) bool {
	if oldDir == "" || newDir == "" || oldDir == newDir {
		return false
	}
	entries, err := os.ReadDir(oldDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() || !keepableSeriesEntries[strings.ToLower(entry.Name())] {
			return false // 目录里有媒体文件或子目录，留给用户手动迁移。
		}
	}
	if err := os.Rename(oldDir, newDir); err != nil {
		log.Printf("番剧目录改名失败 %q → %q: %v", oldDir, newDir, err)
		return false
	}
	return true
}

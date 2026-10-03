package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
)

// Persist the publication intent before exposing files. A failed final UPDATE
// must be retried against this receipt, not by copying the media again.
func archiveSignature(id int) (string, error) {
	var signature string
	err := db.QueryRow(`SELECT json_array(link,COALESCE(downloader,''),COALESCE(task_id,''),
		COALESCE(task_submitted_at,0),COALESCE(anime_id,0),COALESCE(anime_title,''),
		COALESCE(episode,''),COALESCE(season,0),COALESCE(season_auto,0),COALESCE(manual,0)) FROM items WHERE id=?`, id).Scan(&signature)
	return signature, err
}

func prepareArchiveCommit(id int, path, marker, savedAt string) error {
	signature, err := archiveSignature(id)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO archive_commits(item_id,signature,file_path,marker,saved_at) VALUES(?,?,?,?,?)
		ON CONFLICT(item_id) DO UPDATE SET signature=excluded.signature,file_path=excluded.file_path,marker=excluded.marker,saved_at=excluded.saved_at`, id, signature, path, marker, savedAt)
	return err
}

func pendingArchivePath(id int) (string, error) {
	var signature, path, marker, savedAt string
	err := db.QueryRow("SELECT signature,file_path,marker,saved_at FROM archive_commits WHERE item_id=?", id).Scan(&signature, &path, &marker, &savedAt)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	current, err := archiveSignature(id)
	if err != nil {
		return "", err
	}
	if current != signature {
		_, err = db.Exec("DELETE FROM archive_commits WHERE item_id=?", id)
		return "", err // A correction or new task supersedes the old receipt; preserve its files.
	}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		_, err = db.Exec("DELETE FROM archive_commits WHERE item_id=?", id)
		return "", err
	}
	// Unnumbered multi-file resources may publish a directory as their main path.
	if err != nil || (!info.Mode().IsRegular() && !info.IsDir()) {
		return "", fmt.Errorf("归档文件暂不可用，请检查存储位置")
	}
	data, err := os.ReadFile(marker)
	var m archiveMetadata
	if err != nil || json.Unmarshal(data, &m) != nil || m.ID != id || m.SavedAt != savedAt {
		return "", fmt.Errorf("归档文件已存在但提交标记不完整，请检查文件；不会重复复制")
	}
	return path, nil
}

func commitSavedArchive(id int, path string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE items SET status='已保存',file_path=?,task_error='',progress=100 WHERE id=?
		AND EXISTS (SELECT 1 FROM archive_commits c WHERE c.item_id=items.id AND c.file_path=? AND
		c.signature=json_array(link,COALESCE(downloader,''),COALESCE(task_id,''),COALESCE(task_submitted_at,0),
		COALESCE(anime_id,0),COALESCE(anime_title,''),COALESCE(episode,''),COALESCE(season,0),COALESCE(season_auto,0),COALESCE(manual,0)))`, path, id, path)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return fmt.Errorf("归档条目已不存在")
	}
	if _, err = tx.Exec("DELETE FROM archive_commits WHERE item_id=?", id); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	notify("download_saved", id, "已保存")
	return nil
}

func recoverArchiveCommits() error {
	rows, err := db.Query("SELECT item_id FROM archive_commits WHERE item_id IN (SELECT id FROM items)")
	if err != nil {
		return err
	}
	var ids []int
	for rows.Next() {
		var id int
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		l, release := downloadLock(id)
		if !l.operation.TryLock() {
			release()
			continue
		}
		e := recoverArchiveCommit(id)
		l.operation.Unlock()
		release()
		if e != nil {
			return e
		}
	}
	return nil
}

func recoverArchiveCommit(id int) error {
	animeCleanupMu.RLock()
	defer animeCleanupMu.RUnlock()
	item, err := readControlledItem(id)
	if err != nil {
		return nil
	}
	if !activeDownloadStatus(item.Status) {
		return nil
	}
	path, e := pendingArchivePath(id)
	if e != nil {
		return nil
	} // Keep incomplete publications for explicit inspection/retry.
	if path != "" {
		if e = commitSavedArchive(id, path); e != nil {
			return e
		}
	}
	return nil
}

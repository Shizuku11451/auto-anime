package main

import (
	"database/sql"
	"net/http"
	"time"
)

// The caller holds animeCleanupMu, as for binding and other series edits.
func animeMetadataRematch(w http.ResponseWriter, r *http.Request, id int64) {
	tx, err := db.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "重新识别失败，请稍后重试", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	var title, source string
	var bangumiID int64
	err = tx.QueryRowContext(r.Context(), `SELECT animes.title,COALESCE(animes.bangumi_id,0),COALESCE(anime_meta.source,'')
		FROM animes LEFT JOIN anime_meta ON anime_meta.anime_title=animes.title WHERE animes.id=?`, id).
		Scan(&title, &bangumiID, &source)
	if err == sql.ErrNoRows {
		http.Error(w, "番剧不存在", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "重新识别失败，请稍后重试", http.StatusInternalServerError)
		return
	}
	if source == "manual" || bangumiID > 0 {
		http.Error(w, "该番剧已手动绑定元数据，请先在「元数据绑定」中解除绑定", http.StatusConflict)
		return
	}
	if !bangumiEnabled {
		http.Error(w, "自动元数据识别已关闭，请先启用 Bangumi 服务", http.StatusConflict)
		return
	}
	if _, err = tx.ExecContext(r.Context(), "DELETE FROM anime_meta WHERE anime_title=?", title); err == nil {
		// Nanosecond precision also distinguishes consecutive rematches within
		// one second, without adding a persistent job or changing user settings.
		_, err = tx.ExecContext(r.Context(), "UPDATE animes SET updated_at=? WHERE id=?", time.Now().UTC().Format(time.RFC3339Nano), id)
	}
	if err != nil {
		http.Error(w, "重新识别失败，原匹配已保留", http.StatusInternalServerError)
		return
	}
	select {
	case metaQueue <- title:
		// The worker cannot read until this transaction releases the single
		// database connection. If commit fails it observes the original cache.
	default:
		http.Error(w, "元数据队列繁忙，请稍后重试", http.StatusServiceUnavailable)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "重新识别失败，原匹配已保留", http.StatusInternalServerError)
		return
	}
	refreshSeriesPoster(title)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	jsonOut(w, map[string]any{"ok": true, "queued": true})
}

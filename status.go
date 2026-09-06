package main

import (
	"context"
	"net/http"
	"time"
)

func statusMethod(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "方法不支持", 405)
		return false
	}
	return true
}

func healthStatus(w http.ResponseWriter, r *http.Request) {
	if !statusMethod(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		http.Error(w, "数据库不可用", 503)
		return
	}
	jsonOut(w, map[string]bool{"ok": true})
}

func runtimeStatus(w http.ResponseWriter, r *http.Request) {
	if !statusMethod(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	rows, err := db.QueryContext(ctx, "SELECT COALESCE(status,'待下载'),COUNT(*) FROM items GROUP BY COALESCE(status,'待下载')")
	if err != nil {
		http.Error(w, "任务统计暂不可用", 503)
		return
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var status string
		var count int
		if err = rows.Scan(&status, &count); err != nil {
			http.Error(w, "任务统计读取失败", 503)
			return
		}
		counts[status] = count
	}
	if err = rows.Err(); err != nil {
		http.Error(w, "任务统计读取失败", 503)
		return
	}
	jsonOut(w, map[string]any{"poll_minutes": pollMinutes, "download_mode": downloadMode, "counts": counts})
}

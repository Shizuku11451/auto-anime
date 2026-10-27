package main

import (
	"net/http"
	"strconv"
	"strings"
)

var bangumiSubjectSlots = make(chan struct{}, 4)

// Read public metadata without creating local anime records or changing bindings.
func bangumiSubjectAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		http.Error(w, "方法不支持", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/bangumi/subjects/"), 10, 32)
	if err != nil || id <= 0 {
		http.Error(w, "Bangumi 条目 ID 无效", http.StatusBadRequest)
		return
	}
	select {
	case bangumiSubjectSlots <- struct{}{}:
		defer func() { <-bangumiSubjectSlots }()
	default:
		http.Error(w, "详情请求繁忙，请稍后重试", http.StatusTooManyRequests)
		return
	}
	subject, err := fetchBangumiSubjectContext(r.Context(), id)
	if err != nil {
		http.Error(w, "Bangumi 详情暂不可用，请稍后重试", http.StatusBadGateway)
		return
	}
	jsonOut(w, map[string]any{
		"id": subject.ID, "name": subject.Name, "name_cn": subject.NameCN,
		"summary": subject.Summary, "cover": preferredCoverURL(subject),
		"rating": subject.Rating.Score, "rating_total": subject.Rating.Total,
		"rank": subject.Rating.Rank, "eps": subject.Episodes, "date": subject.Date,
	})
}

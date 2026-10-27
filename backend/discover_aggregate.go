package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"auto-anime/internal/aggregate"
)

var aggregateSearchURL = strings.TrimRight(env("AGGREGATE_SEARCH_URL", ""), "/")
var aggregateSearchToken = env("AGGREGATE_SEARCH_TOKEN", "")
var discoverAggregateSlots = make(chan struct{}, 4)

func aggregateConfigured() bool {
	u, err := url.Parse(aggregateSearchURL)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(aggregateSearchToken) < 32 {
		return false
	}
	return u.Scheme == "https" || (u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"))
}

func discoverSubjectsAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		http.Error(w, "方法不支持", http.StatusMethodNotAllowed)
		return
	}
	keyword := strings.TrimSpace(r.URL.Query().Get("keyword"))
	if !utf8.ValidString(keyword) || len([]rune(keyword)) == 0 || len([]rune(keyword)) > discoverKeywordMax {
		http.Error(w, "请输入 1 至 100 字符的番剧名", http.StatusBadRequest)
		return
	}
	select {
	case discoverAggregateSlots <- struct{}{}:
		defer func() { <-discoverAggregateSlots }()
	default:
		http.Error(w, "搜索请求较多，请稍后重试", http.StatusTooManyRequests)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	subjects, err := searchBangumiListContext(ctx, keyword)
	if err != nil {
		http.Error(w, "Bangumi 搜索暂不可用，可重试或改用 RSS 搜索", http.StatusBadGateway)
		return
	}
	items := []map[string]any{}
	for i := range subjects {
		s := &subjects[i]
		if s.ID <= 0 {
			continue
		}
		items = append(items, map[string]any{"id": s.ID, "name": s.Name, "name_cn": s.NameCN,
			"cover": preferredCoverURL(s), "summary": s.Summary, "date": s.Date, "rating": s.Rating.Score})
	}
	jsonOut(w, map[string]any{"keyword": keyword, "items": items, "resources_enabled": aggregateConfigured()})
}

func discoverResourcesAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		http.Error(w, "方法不支持", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.ParseInt(r.URL.Query().Get("subject_id"), 10, 32)
	if err != nil || id <= 0 {
		http.Error(w, "Bangumi 条目 ID 无效", http.StatusBadRequest)
		return
	}
	if !aggregateConfigured() {
		http.Error(w, "尚未配置在线聚合服务，请联系实例管理员；现有 RSS 搜索仍可使用", http.StatusServiceUnavailable)
		return
	}
	select {
	case discoverAggregateSlots <- struct{}{}:
		defer func() { <-discoverAggregateSlots }()
	default:
		http.Error(w, "搜索请求较多，请稍后重试", http.StatusTooManyRequests)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, aggregateSearchURL+"/v1/search?subject_id="+strconv.FormatInt(id, 10), nil)
	if err != nil {
		http.Error(w, "聚合服务配置无效", http.StatusServiceUnavailable)
		return
	}
	req.Header.Set("Authorization", "Bearer "+aggregateSearchToken)
	client := *httpClient
	client.Timeout = 45 * time.Second
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := client.Do(req)
	if err != nil {
		http.Error(w, "在线聚合服务暂不可用，请稍后重试", http.StatusBadGateway)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		status := http.StatusBadGateway
		message := "在线聚合服务请求失败，请联系管理员或稍后重试"
		if res.StatusCode == http.StatusTooManyRequests {
			status, message = http.StatusTooManyRequests, "在线聚合服务繁忙，请稍后重试"
			w.Header().Set("Retry-After", "60")
		}
		http.Error(w, message, status)
		return
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, (2<<20)+1))
	var result aggregate.Result
	if err != nil || len(data) > 2<<20 || !utf8.Valid(data) || json.Unmarshal(data, &result) != nil || result.SubjectID != int(id) || len(result.Items) > 300 {
		http.Error(w, "在线聚合服务响应无效", http.StatusBadGateway)
		return
	}
	if result.Items == nil {
		result.Items = []aggregate.Resource{}
	}
	if result.Warnings == nil {
		result.Warnings = []string{}
	}
	jsonOut(w, result)
}

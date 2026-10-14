package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	titleparse "auto-anime/internal/title"
)

// 找番：在内置或自定义的资源站搜索关键词，直接拿到可订阅的 RSS 地址。
// 搜索结果本身就是一个 RSS（按关键词过滤的资源列表），把它添加为订阅后
// 复用现有的同步、过滤和下载流程。搜索属于公网内容访问，走 httpClient
// 的统一传输配置（PROXY_URL 生效），并对响应大小设上限，防止异常站点
// 返回超大内容拖垮内存。

var discoverSearchTemplate string // 设置页配置的自定义搜索模板；空表示未启用

// 模板占位符同时支持中英文写法；搜索词会做 URL 转义后再替换。
const (
	discoverPlaceholderEN = "{keyword}"
	discoverPlaceholderCN = "{关键词}"
	discoverTimeout       = 60 * time.Second
	discoverKeywordMax    = 100
)

// discoverMaxBytes 是 var 而非常量，测试可以调小以验证大小限制。
var discoverMaxBytes = 8 << 20

type discoverSource struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Hint        string `json:"hint"`
	Template    string `json:"-"`
	Custom      bool   `json:"custom"`
	PageParam   string `json:"-"`
	PageSize    int    `json:"-"`
	ResultLimit int    `json:"-"`
}

// 2026-09-10 核验公开 RSS：正常关键词返回相关条目，不存在的关键词返回空结果。
// 分页能力与源站的单次返回上限分别记录，不能把最新列表当成全部历史。
var discoverBuiltinSources = []discoverSource{
	{ID: "acgrip", Name: "ACG.RIP", Hint: "公开搜索 RSS，支持自动翻页查找早期集数", Template: "https://acg.rip/.xml?term={keyword}", PageParam: "page", PageSize: 30},
	{ID: "mikan", Name: "蜜柑计划", Hint: "公开搜索 RSS，最多返回最近 100 条；可加上字幕组缩小搜索范围", Template: "https://mikanani.me/RSS/Search?searchstr={keyword}", ResultLimit: 100},
	{ID: "dmhy", Name: "动漫花园", Hint: "公开关键词 RSS，最多返回最近 500 条", Template: "https://share.dmhy.org/topics/rss/rss.xml?keyword={keyword}", ResultLimit: 500},
}

// validateDiscoverTemplate ensures a custom search template is an http(s) RSS
// address with a keyword placeholder, so a saved template can never point at
// file:// or miss the keyword entirely.
func validateDiscoverTemplate(raw string) (string, error) {
	t := strings.TrimSpace(raw)
	if t == "" {
		return "", nil
	}
	if !strings.Contains(t, discoverPlaceholderEN) && !strings.Contains(t, discoverPlaceholderCN) {
		return "", fmt.Errorf("搜索模板必须包含 %s 占位符", discoverPlaceholderEN)
	}
	u, err := url.Parse(t)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", fmt.Errorf("搜索模板必须是 http(s) 地址")
	}
	return t, nil
}

func discoverSourceList() []discoverSource {
	cfg := runtimeConfig()
	sources := append([]discoverSource{}, discoverBuiltinSources...)
	if t, err := validateDiscoverTemplate(cfg.discoverSearchTemplate); err == nil && t != "" {
		sources = append(sources, discoverSource{ID: "custom", Name: "自定义搜索源", Hint: "设置页配置的 RSS 搜索模板", Template: t, Custom: true})
	}
	return sources
}

func discoverSourceByID(id string) (discoverSource, bool) {
	for _, s := range discoverSourceList() {
		if s.ID == id {
			return s, true
		}
	}
	return discoverSource{}, false
}

func discoverSearchURL(template, keyword string) string {
	escaped := url.QueryEscape(keyword)
	return strings.ReplaceAll(strings.ReplaceAll(template, discoverPlaceholderEN, escaped), discoverPlaceholderCN, escaped)
}

// ---- GET /api/discover/sources ----

func discoverSourcesAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "方法不支持", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	jsonOut(w, map[string]any{"sources": discoverSourceList()})
}

// ---- GET /api/discover/search?source=&keyword= ----

type discoverItem struct {
	Title      string `json:"title"`
	AnimeTitle string `json:"anime_title"`
	Group      string `json:"fansub_group"`
	Episode    string `json:"episode"`
	Quality    string `json:"quality"`
	Link       string `json:"link"`
	Torrent    string `json:"torrent"`
	Published  string `json:"published"`
	Season     int    `json:"season"`
	FeedURL    string `json:"feed_url,omitempty"`
	Query      string `json:"query,omitempty"`
}

func discoverSearchAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "方法不支持", http.StatusMethodNotAllowed)
		return
	}
	keyword := sanitizeText(r.URL.Query().Get("keyword"), discoverKeywordMax)
	if keyword == "" {
		http.Error(w, "请输入搜索关键词", http.StatusBadRequest)
		return
	}
	source, ok := discoverSourceByID(strings.TrimSpace(r.URL.Query().Get("source")))
	if !ok {
		http.Error(w, "搜索源不存在", http.StatusBadRequest)
		return
	}
	base, season := titleparse.DiscoverIntent(keyword)
	if r.URL.Query().Get("literal") == "1" {
		base, season = keyword, 0
	}
	ctx, cancel := context.WithTimeout(r.Context(), discoverTimeout)
	defer cancel()
	budget := newFeedReadBudget()
	items := []discoverItem{}
	seen := map[string]bool{}
	warnings := []string{}
	queries := []string{}
	pages, successes := 0, 0
	history, limited := false, false
	for i, query := range titleparse.DiscoverQueries(keyword, base, season) {
		if ctx.Err() != nil || budget.pages <= 0 || budget.items <= 0 || budget.bytes <= 0 {
			warnings = append(warnings, "部分扩展查询未完成：达到超时或读取预算。")
			limited = true
			break
		}
		feedURL := discoverSearchURL(source.Template, query)
		maxPages := feedMaxPages
		if season > 0 && query != base {
			maxPages = 3
			if i == 0 {
				maxPages = 1
			}
		}
		queries = append(queries, query)
		queryContext := ctx
		queryCancel := func() {}
		if season > 0 && query != base {
			queryContext, queryCancel = context.WithTimeout(ctx, 12*time.Second)
		}
		result, err := fetchRSSBudget(queryContext, feedURL, &budget, maxPages)
		queryCancel()
		pages += result.Pages
		history = history || result.History
		limited = limited || result.Limited
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("查询 %d 失败：%v", i+1, err))
			limited = true
			continue
		}
		successes++
		if result.Warning != "" {
			warnings = append(warnings, "查询 "+strconv.Itoa(i+1)+"："+result.Warning)
		}
		for _, item := range discoverItemsFromEntries(result.Entries) {
			key := item.Torrent
			if key == "" {
				key = item.Link
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			item.FeedURL, item.Query = feedURL, query
			items = append(items, item)
		}
	}
	if successes == 0 {
		http.Error(w, "搜索失败："+strings.Join(warnings, "；"), http.StatusBadGateway)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	jsonOut(w, map[string]any{
		"source": source.ID, "source_name": source.Name,
		"keyword": keyword, "feed_url": discoverSearchURL(source.Template, keyword),
		"base_keyword": base, "requested_season": season, "queries": queries,
		"count": len(items), "items": items,
		"pages": pages, "history": history,
		"limited": limited, "warning": strings.Join(warnings, "；"),
	})
}

func resolveDiscoverLink(base *url.URL, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	parsed, err := url.Parse(ref)
	if err != nil || parsed.Scheme != "" {
		return ref
	}
	return base.ResolveReference(parsed).String()
}

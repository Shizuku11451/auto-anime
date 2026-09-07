package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
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
	discoverMaxItems      = 60
	discoverTimeout       = 20 * time.Second
	discoverKeywordMax    = 100
)

// discoverMaxBytes 是 var 而非常量，测试可以调小以验证大小限制。
var discoverMaxBytes = 8 << 20

type discoverSource struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Hint     string `json:"hint"`
	Template string `json:"-"`
	Custom   bool   `json:"custom"`
}

// 内置源只收录实测可直接匿名搜索的站点；蜜柑计划、动漫花园的匿名搜索
// 在 2026-09-07 实测不可用（返回空结果/不按关键词过滤），故未内置。
var discoverBuiltinSources = []discoverSource{
	{ID: "acgrip", Name: "ACG.RIP", Hint: "动漫资源站，一般可国内直连", Template: "https://acg.rip/.xml?term={keyword}"},
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
	sources := append([]discoverSource{}, discoverBuiltinSources...)
	if t, err := validateDiscoverTemplate(discoverSearchTemplate); err == nil && t != "" {
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
}

// parseDiscoverFeed 解码 RSS/RDF/Atom 并保留资源页与种子两条链接；
// parseFeed 会把 enclosure 合并进 Link，这里需要分开展示。
func parseDiscoverFeed(data []byte) ([]discoverItem, error) {
	var doc rss
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.XMLName.Local != "rss" && doc.XMLName.Local != "RDF" && doc.XMLName.Local != "feed" {
		return nil, fmt.Errorf("搜索源返回的不是 RSS 文档")
	}
	out := make([]discoverItem, 0, len(doc.Items)+len(doc.RootItems))
	appendItem := func(title, link, torrent, pub string) {
		title = strings.TrimSpace(title)
		if title == "" || (link == "" && torrent == "") {
			return
		}
		info := extractAnimeInfo(title)
		out = append(out, discoverItem{
			Title: title, AnimeTitle: info.Title, Group: info.Group,
			Episode: info.Episode, Quality: info.Quality,
			Link: link, Torrent: torrent, Published: strings.TrimSpace(pub),
		})
	}
	for _, item := range doc.Items {
		appendItem(item.Title, item.Link, item.Enclosure.URL, item.Pub)
	}
	for _, item := range doc.RootItems {
		appendItem(item.Title, item.Link, item.Enclosure.URL, item.Pub)
	}
	for _, entry := range doc.Entries {
		link, torrent := "", ""
		for _, candidate := range entry.Links {
			switch {
			case candidate.Rel == "enclosure":
				torrent = candidate.Href
			case candidate.Rel == "alternate" || link == "":
				link = candidate.Href
			}
		}
		pub := entry.Pub
		if pub == "" {
			pub = entry.Updated
		}
		appendItem(entry.Title, link, torrent, pub)
	}
	return out, nil
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
	feedURL := discoverSearchURL(source.Template, keyword)
	ctx, cancel := context.WithTimeout(r.Context(), discoverTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		http.Error(w, "搜索地址无效："+err.Error(), http.StatusBadRequest)
		return
	}
	req.Header.Set("User-Agent", "Auto-Anime/1.0")
	res, err := httpClient.Do(req)
	if err != nil {
		http.Error(w, "搜索源连接失败："+err.Error(), http.StatusBadGateway)
		return
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		http.Error(w, fmt.Sprintf("搜索源返回 HTTP %d", res.StatusCode), http.StatusBadGateway)
		return
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, int64(discoverMaxBytes)+1))
	if err != nil {
		http.Error(w, "读取搜索结果失败："+err.Error(), http.StatusBadGateway)
		return
	}
	if len(data) > discoverMaxBytes {
		http.Error(w, fmt.Sprintf("搜索响应超过 %d MB，已放弃解析", discoverMaxBytes>>20), http.StatusBadGateway)
		return
	}
	items, err := parseDiscoverFeed(data)
	if err != nil {
		http.Error(w, "解析搜索结果失败："+err.Error(), http.StatusBadGateway)
		return
	}
	if len(items) > discoverMaxItems {
		items = items[:discoverMaxItems]
	}
	// 资源页与种子链接可能是相对路径，按搜索地址补全。
	if base, _ := url.Parse(feedURL); base != nil {
		for i := range items {
			items[i].Link = resolveDiscoverLink(base, items[i].Link)
			items[i].Torrent = resolveDiscoverLink(base, items[i].Torrent)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	jsonOut(w, map[string]any{
		"source": source.ID, "source_name": source.Name,
		"keyword": keyword, "feed_url": feedURL,
		"count": len(items), "items": items,
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

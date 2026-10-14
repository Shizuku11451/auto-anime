package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	titleparse "auto-anime/internal/title"
)

const (
	feedMaxPages     = 20
	feedMaxItems     = 1000
	feedFetchTimeout = 60 * time.Second
	feedPageTimeout  = 20 * time.Second
)

var feedMaxTotalBytes = 32 << 20
var feedPageDelay = 200 * time.Millisecond
var feedRequestSlots = make(chan struct{}, 3)

// 搜索、预览和同步必须读取同一批资源，否则预览中的早期集数不会真正入库。
type feedEntry struct {
	Title, Link, Torrent, Published, Description string
}

func (e feedEntry) resourceURL() string {
	if e.Torrent != "" {
		return e.Torrent
	}
	return e.Link
}

type feedPage struct {
	Entries []feedEntry
	Next    string
}

type feedFetchResult struct {
	Entries    []feedEntry
	Pages      int
	History    bool
	Limited    bool
	Warning    string
	Incomplete bool // 本次读取中断；来源自身的最新列表上限仅提示，不视为同步失败。
}

func (f *feedFetchResult) limit(message string) {
	f.Limited = true
	f.Warning = message
	f.Incomplete = true
}

func decodeFeedPage(reader io.Reader) (feedPage, error) {
	var doc rss
	if err := xml.NewDecoder(reader).Decode(&doc); err != nil {
		return feedPage{}, fmt.Errorf("解析 RSS 失败：%w", err)
	}
	if doc.XMLName.Local != "rss" && doc.XMLName.Local != "RDF" && doc.XMLName.Local != "feed" {
		return feedPage{}, fmt.Errorf("不是 RSS 或 Atom 文档")
	}
	page := feedPage{Entries: make([]feedEntry, 0, len(doc.Items)+len(doc.RootItems)+len(doc.Entries))}
	for _, item := range append(doc.Items, doc.RootItems...) {
		page.Entries = append(page.Entries, feedEntry{strings.TrimSpace(item.Title), strings.TrimSpace(item.Link), strings.TrimSpace(item.Enclosure.URL), strings.TrimSpace(item.Pub), item.Description})
	}
	for _, item := range doc.Entries {
		entry := feedEntry{Title: strings.TrimSpace(item.Title), Published: strings.TrimSpace(item.Pub), Description: item.Description}
		for _, link := range item.Links {
			if link.Rel == "enclosure" {
				if entry.Torrent == "" {
					entry.Torrent = strings.TrimSpace(link.Href)
				}
			} else if link.Rel == "alternate" || (entry.Link == "" && link.Rel == "") {
				entry.Link = strings.TrimSpace(link.Href)
			}
		}
		if entry.Published == "" {
			entry.Published = strings.TrimSpace(item.Updated)
		}
		page.Entries = append(page.Entries, entry)
	}
	for _, link := range append(doc.ChannelLinks, doc.Links...) {
		if strings.EqualFold(strings.TrimSpace(link.Rel), "next") {
			page.Next = strings.TrimSpace(link.Href)
			break
		}
	}
	return page, nil
}

func feedItemsFromEntries(entries []feedEntry) []feedItem {
	items := make([]feedItem, 0, len(entries))
	for _, entry := range entries {
		items = append(items, feedItem{
			Title: entry.Title, Link: entry.resourceURL(), Pub: entry.Published,
			Description: entry.Description, PageURL: entry.Link,
		})
	}
	return items
}

func backfillItemPageURL(link, page string) error {
	if page == "" {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec("UPDATE items SET page_url=? WHERE link=? AND COALESCE(page_url,'')=''", page, link)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	u, parseErr := url.Parse(page)
	if changed > 0 && parseErr == nil && mikanPageAllowed(u) && mikanEpisodePath.MatchString(u.Path) {
		// A miss recorded before the page was available is no longer useful.
		// Invalidate in-flight lookups too, while preserving successful matches
		// and explicit user corrections. The sync's normal scan will requeue it.
		result, err = tx.Exec(`UPDATE animes SET updated_at=? WHERE title=(SELECT anime_title FROM items WHERE link=?)
			AND COALESCE(search_name,'')='' AND COALESCE(bangumi_id,0)=0
			AND NOT EXISTS (SELECT 1 FROM anime_meta WHERE anime_title=animes.title AND COALESCE(source,'') NOT IN ('','none'))`,
			time.Now().UTC().Format(time.RFC3339Nano), link)
		if err != nil {
			return err
		}
		changed, err = result.RowsAffected()
		if err != nil {
			return err
		}
		if changed > 0 {
			if _, err := tx.Exec(`DELETE FROM anime_meta WHERE source='none' AND anime_title=(SELECT anime_title FROM items WHERE link=?)`, link); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func discoverItemsFromEntries(entries []feedEntry) []discoverItem {
	items := make([]discoverItem, 0, len(entries))
	for _, entry := range entries {
		if entry.Title == "" || entry.resourceURL() == "" {
			continue
		}
		info := titleparse.Extract(entry.Title)
		items = append(items, discoverItem{
			Title: entry.Title, AnimeTitle: info.Title, Group: info.Group,
			Season:  titleparse.ResourceSeason(entry.Title),
			Episode: info.Episode, Quality: info.Quality,
			Link: entry.Link, Torrent: entry.Torrent, Published: entry.Published,
		})
	}
	return items
}

// 仅匹配已核验的搜索 RSS（包括固定查询参数）；不能对任意订阅猜测 page 参数。
// 使用注册表匹配，也使旧订阅无需改地址就能读取历史页。
func feedSource(address *url.URL) discoverSource {
	const marker = "autoanimekeywordplaceholder"
	for _, source := range discoverSourceList() {
		template, err := url.Parse(discoverSearchURL(source.Template, marker))
		if err != nil || !strings.EqualFold(address.Host, template.Host) || !strings.EqualFold(address.Path, template.Path) {
			continue
		}
		matched := true
		for key, values := range template.Query() {
			for _, value := range values {
				actual := address.Query().Get(key)
				if strings.Contains(value, marker) {
					if strings.TrimSpace(actual) == "" {
						matched = false
					}
				} else if actual != value {
					matched = false
				}
			}
		}
		if matched {
			return source
		}
	}
	return discoverSource{}
}

func sameFeedOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}

func readFeedPage(ctx context.Context, address, origin *url.URL, remaining int) (feedPage, *url.URL, int, error) {
	ctx, cancel := context.WithTimeout(ctx, feedPageTimeout)
	defer cancel()
	select {
	case feedRequestSlots <- struct{}{}:
		defer func() { <-feedRequestSlots }()
	case <-ctx.Done():
		return feedPage{}, nil, 0, fmt.Errorf("RSS 读取超时或已取消")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address.String(), nil)
	if err != nil {
		return feedPage{}, nil, 0, fmt.Errorf("RSS 地址无效")
	}
	req.Header.Set("User-Agent", "Auto-Anime/1.0")
	// 沿用统一代理配置。翻页重定向也必须留在首个响应的来源内。
	client := *httpClient
	previousRedirect := client.CheckRedirect
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if origin != nil && (!sameFeedOrigin(next.URL, origin) || next.URL.User != nil) {
			return fmt.Errorf("RSS 历史页重定向到其他来源，已停止")
		}
		if previousRedirect != nil {
			return previousRedirect(next, via)
		}
		if len(via) >= 10 {
			return fmt.Errorf("RSS 重定向次数过多")
		}
		return nil
	}
	response, err := client.Do(req)
	if err != nil {
		// url.Error 包含完整 URL（可能有 RSS 令牌），不能直接写入日志或界面。
		if ctx.Err() != nil {
			return feedPage{}, nil, 0, fmt.Errorf("RSS 读取超时或已取消")
		}
		if detail, ok := err.(*url.Error); ok {
			err = detail.Err
		}
		return feedPage{}, nil, 0, fmt.Errorf("RSS 连接失败：%v", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return feedPage{}, nil, 0, fmt.Errorf("RSS 返回 HTTP %d", response.StatusCode)
	}
	limit := min(discoverMaxBytes, remaining)
	data, err := io.ReadAll(io.LimitReader(response.Body, int64(limit)+1))
	if err != nil {
		return feedPage{}, nil, len(data), fmt.Errorf("读取 RSS 响应失败")
	}
	if len(data) > limit {
		return feedPage{}, nil, len(data), fmt.Errorf("RSS 响应超过读取大小上限")
	}
	page, err := decodeFeedPage(bytes.NewReader(data))
	return page, response.Request.URL, len(data), err
}

func fetchRSS(ctx context.Context, feedURL string) (feedFetchResult, error) {
	budget := newFeedReadBudget()
	return fetchRSSBudget(ctx, feedURL, &budget, feedMaxPages)
}

type feedReadBudget struct {
	pages, items, bytes int
}

func newFeedReadBudget() feedReadBudget {
	return feedReadBudget{feedMaxPages, feedMaxItems, feedMaxTotalBytes}
}

func fetchRSSBudget(ctx context.Context, feedURL string, budget *feedReadBudget, maxPages int) (feedFetchResult, error) {
	result := feedFetchResult{Entries: []feedEntry{}}
	address, err := url.Parse(strings.TrimSpace(feedURL))
	if err != nil || (address.Scheme != "http" && address.Scheme != "https") || address.Hostname() == "" {
		return result, fmt.Errorf("RSS 地址无效")
	}
	ctx, cancel := context.WithTimeout(ctx, feedFetchTimeout)
	defer cancel()
	source := feedSource(address)
	result.History = source.PageParam != ""
	seenPages := map[string]bool{}
	seenEntries := map[string]bool{}
	var origin *url.URL
	for {
		if budget.pages <= 0 || budget.items <= 0 || budget.bytes <= 0 || result.Pages >= maxPages {
			result.limit("达到本次读取预算；结果可能不完整，请缩小搜索范围。")
			break
		}
		if seenPages[address.String()] {
			result.limit("来源的下一页链接重复，已停止读取；结果可能不完整。")
			break
		}
		seenPages[address.String()] = true
		if result.Pages > 0 {
			timer := time.NewTimer(feedPageDelay)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				result.limit(fmt.Sprintf("已读取 %d 页，历史读取超时或已取消；请重试。", result.Pages))
				return result, nil
			}
		}
		// Failed requests consume a page slot as well; expansions share one budget.
		budget.pages--
		page, base, size, err := readFeedPage(ctx, address, origin, budget.bytes)
		budget.bytes -= size
		if err != nil {
			if result.Pages == 0 {
				return result, err
			}
			result.limit(fmt.Sprintf("已读取 %d 页，第 %d 页读取失败：%v；结果可能不完整，请重试。", result.Pages, result.Pages+1, err))
			break
		}
		result.Pages++
		if origin == nil {
			origin = base
		}
		added := 0
		for _, entry := range page.Entries {
			entry.Link = resolveDiscoverLink(base, entry.Link)
			entry.Torrent = resolveDiscoverLink(base, entry.Torrent)
			key := entry.resourceURL()
			if entry.Title == "" || key == "" || seenEntries[key] {
				continue
			}
			if budget.items <= 0 {
				result.limit(fmt.Sprintf("已读取 %d 条资源，达到本次读取上限；请加上番名或字幕组缩小搜索范围。", feedMaxItems))
				return result, nil
			}
			seenEntries[key] = true
			budget.items--
			result.Entries = append(result.Entries, entry)
			added++
		}
		if len(page.Entries) == 0 {
			break
		}
		if result.Pages > 1 && added == 0 {
			result.limit("来源重复返回已读取的资源，可能未支持翻页；结果可能不完整。")
			break
		}
		var next *url.URL
		if page.Next != "" {
			result.History = true
			ref, parseErr := url.Parse(page.Next)
			if parseErr != nil {
				result.limit("来源的下一页地址无效；结果可能不完整。")
				break
			}
			next = base.ResolveReference(ref)
			if !sameFeedOrigin(next, origin) || next.User != nil {
				result.limit("来源的下一页指向其他来源，已停止读取；结果可能不完整。")
				break
			}
		} else if source.PageParam != "" && len(page.Entries) >= source.PageSize {
			copyURL := *base
			query := copyURL.Query()
			number, _ := strconv.Atoi(query.Get(source.PageParam))
			query.Set(source.PageParam, strconv.Itoa(max(1, number)+1))
			copyURL.RawQuery = query.Encode()
			next = &copyURL
		}
		if next == nil {
			if source.ResultLimit > 0 && len(page.Entries) >= source.ResultLimit {
				result.Limited = true
				result.Warning = fmt.Sprintf("此来源只返回最近 %d 条搜索结果，可能遗漏早期集数；请加上字幕组缩小搜索范围，或改用支持历史翻页的 ACG.RIP。", source.ResultLimit)
			}
			break
		}
		if result.Pages >= maxPages || budget.pages <= 0 || budget.items <= 0 || budget.bytes <= 0 {
			result.limit(fmt.Sprintf("已读取 %d 页、%d 条资源，达到本次读取上限；请加上番名或字幕组缩小搜索范围。", result.Pages, len(result.Entries)))
			break
		}
		next.Fragment = ""
		address = next
	}
	return result, nil
}

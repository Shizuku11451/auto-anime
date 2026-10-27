package main

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	keywordfilter "auto-anime/internal/filter"
	titleparse "auto-anime/internal/title"
)

var aggregateMagnet = regexp.MustCompile(`(?i)^magnet:\?xt=urn:btih:[a-f0-9]{40}$`)
var aggregateLanguage = regexp.MustCompile(`(?i)简繁|簡繁|简日|簡日|繁日|简中|簡中|繁中|简体|簡體|繁体|繁體|\bCHS\b|\bCHT\b|\bGB\b|\bBIG5\b`)
var aggregateResultCap = regexp.MustCompile(`^(?:[a-z0-9][a-z0-9-]{0,63} 达到单次读取上限，结果可能不完整|已达到 300 条结果上限)$`)

// The address stores public selection rules, not an upstream URL or credential.
func aggregateSubscription(raw string) (int, url.Values, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "aggregate" || u.Host != "subject" || u.User != nil || u.Fragment != "" || u.Opaque != "" || len(raw) > 8192 {
		return 0, nil, fmt.Errorf("聚合订阅地址无效")
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(u.Path, "/"), 10, 32)
	if err != nil || id <= 0 || u.Path != "/"+strconv.FormatInt(id, 10) {
		return 0, nil, fmt.Errorf("Bangumi 条目 ID 无效")
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(q) != 5 {
		return 0, nil, fmt.Errorf("聚合订阅规则无效")
	}
	for _, key := range []string{"title", "group", "quality", "season", "language"} {
		values, ok := q[key]
		if !ok || len(values) != 1 || !utf8.ValidString(values[0]) || len([]rune(values[0])) > 500 || strings.ContainsAny(values[0], "\r\n\x00") {
			return 0, nil, fmt.Errorf("聚合订阅规则无效")
		}
	}
	season, err := strconv.Atoi(q.Get("season"))
	if strings.TrimSpace(q.Get("title")) == "" || err != nil || season < 0 || season > 999 {
		return 0, nil, fmt.Errorf("聚合订阅版本无效")
	}
	return int(id), q, nil
}

func fetchSubscription(ctx context.Context, address string) (feedFetchResult, error) {
	if !strings.HasPrefix(address, "aggregate:") {
		return fetchRSS(ctx, address)
	}
	id, rules, err := aggregateSubscription(address)
	if err != nil {
		return feedFetchResult{}, err
	}
	resources, err := fetchAggregateResources(ctx, id)
	if err != nil {
		return feedFetchResult{}, err
	}
	result := feedFetchResult{Entries: []feedEntry{}, Pages: 1}
	season, _ := strconv.Atoi(rules.Get("season"))
	seasonFilter := season
	if seasonFilter == 0 {
		seasonFilter = -1
	}
	for _, resource := range resources.Items {
		if !aggregateMagnet.MatchString(resource.Magnet) || len(resource.Title) > 8192 || !titleparse.AllowsSeason(resource.Title, seasonFilter, false) {
			continue
		}
		info := titleparse.Extract(resource.Title)
		matches := true
		for key, actual := range map[string]string{"title": info.Title, "group": info.Group, "quality": info.Quality, "language": aggregateLanguage.FindString(resource.Title)} {
			wanted := rules.Get(key)
			if !keywordfilter.Equivalent(actual, wanted) {
				matches = false
			}
		}
		if matches {
			page := ""
			if u, err := url.Parse(resource.DetailsURL); err == nil && u.Hostname() != "" && u.User == nil && (u.Scheme == "https" || u.Scheme == "http") {
				page = resource.DetailsURL
			}
			result.Entries = append(result.Entries, feedEntry{Title: resource.Title, Link: page, Torrent: strings.ToLower(resource.Magnet), Published: resource.Published})
		}
	}
	if len(resources.Warnings) > 0 {
		result.Limited = true
		result.Warning = strings.Join(resources.Warnings, "；")
		// shortcut: 在线 API 仅提供警告文本，增加机器可读错误类型后改用类型判断。
		for _, warning := range resources.Warnings {
			if !aggregateResultCap.MatchString(warning) {
				result.Incomplete = true
			}
		}
	}
	return result, nil
}

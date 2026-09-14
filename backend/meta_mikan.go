package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const mikanPageMaxBytes = 2 << 20

var mikanEpisodePath = regexp.MustCompile(`^/Home/Episode/[a-zA-Z0-9]+/?$`)
var mikanBangumiPath = regexp.MustCompile(`^/Home/Bangumi/[1-9][0-9]*/?$`)
var bangumiSubjectPath = regexp.MustCompile(`^/subject/([1-9][0-9]*)/?$`)

// Page scraping has its own allowlist. It must never widen coverHostAllowed.
func mikanPageAllowed(u *url.URL) bool {
	if u == nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || !strings.EqualFold(u.Hostname(), "mikanani.me") {
		return false
	}
	return u.Port() == "" || (u.Scheme == "https" && u.Port() == "443") || (u.Scheme == "http" && u.Port() == "80")
}

// Select one episode only; a failed page does not trigger a crawl of the other
// resources. Legacy rows whose download link was itself a page still work.
func mikanEpisodeURL(animeTitle string) (*url.URL, error) {
	rows, err := db.Query(`SELECT COALESCE(page_url,''),COALESCE(link,'') FROM items
		WHERE anime_title=? ORDER BY id DESC`, animeTitle)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var page, link string
		if err := rows.Scan(&page, &link); err != nil {
			return nil, err
		}
		for _, candidate := range []string{page, link} {
			u, err := url.Parse(strings.TrimSpace(candidate))
			if err == nil && mikanPageAllowed(u) && mikanEpisodePath.MatchString(u.Path) {
				u.Fragment = ""
				return u, nil
			}
		}
	}
	return nil, rows.Err()
}

func fetchMikanPage(ctx context.Context, u *url.URL) ([]byte, error) {
	if !mikanPageAllowed(u) {
		return nil, fmt.Errorf("蜜柑资源页地址无效")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", metaUserAgent)
	// Reuse the configured transport/proxy and timeout, without allowing a
	// redirect to escape the allowlist or exceed the two-page request budget.
	client := *metaClient
	if client.Timeout <= 0 || client.Timeout > 15*time.Second {
		client.Timeout = 15 * time.Second
	}
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("蜜柑资源页返回 HTTP %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, mikanPageMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > mikanPageMaxBytes {
		return nil, fmt.Errorf("蜜柑资源页超过 2 MB")
	}
	return data, nil
}

// Read real anchor attributes, ignoring lookalikes in comments, scripts and
// text. Multiple distinct matches are ambiguous and must fall back to search.
func uniquePageLink(data []byte, base *url.URL, accept func(*url.URL) string) (string, error) {
	tokens := html.NewTokenizer(bytes.NewReader(data))
	tokens.SetMaxBuf(mikanPageMaxBytes)
	found := ""
	for {
		switch tokens.Next() {
		case html.ErrorToken:
			if err := tokens.Err(); err != io.EOF {
				return "", err
			}
			if found == "" {
				return "", fmt.Errorf("页面未包含可识别的条目链接")
			}
			return found, nil
		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokens.Token()
			if token.Data != "a" {
				continue
			}
			for _, attr := range token.Attr {
				if attr.Key != "href" {
					continue
				}
				u, err := url.Parse(strings.TrimSpace(attr.Val))
				if err != nil {
					break
				}
				candidate := accept(base.ResolveReference(u))
				if candidate != "" {
					if found != "" && found != candidate {
						return "", fmt.Errorf("页面包含多个不同条目链接")
					}
					found = candidate
				}
				break
			}
		}
	}
}

func mikanBangumiID(ctx context.Context, episode *url.URL) (int64, error) {
	data, err := fetchMikanPage(ctx, episode)
	if err != nil {
		return 0, err
	}
	series, err := uniquePageLink(data, episode, func(u *url.URL) string {
		if !mikanPageAllowed(u) || !mikanBangumiPath.MatchString(u.Path) {
			return ""
		}
		// Tracking parameters and anchors do not identify different series.
		u.RawQuery, u.Fragment = "", ""
		u.Path = strings.TrimRight(u.Path, "/")
		return u.String()
	})
	if err != nil {
		return 0, err
	}
	seriesURL, err := url.Parse(series)
	if err != nil {
		return 0, err
	}
	data, err = fetchMikanPage(ctx, seriesURL)
	if err != nil {
		return 0, err
	}
	id, err := uniquePageLink(data, seriesURL, func(u *url.URL) string {
		if (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || !strings.EqualFold(u.Hostname(), "bgm.tv") ||
			(u.Port() != "" && !(u.Scheme == "https" && u.Port() == "443") && !(u.Scheme == "http" && u.Port() == "80")) {
			return ""
		}
		match := bangumiSubjectPath.FindStringSubmatch(u.Path)
		if match == nil {
			return ""
		}
		return match[1]
	})
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(id, 10, 64)
}

func lookupMikanSubject(episode *url.URL) (*bangumiSubject, error) {
	ctx, cancel := context.WithTimeout(context.Background(), metaLookupTimeout)
	defer cancel()
	id, err := mikanBangumiID(ctx, episode)
	if err != nil {
		return nil, err
	}
	return fetchBangumiSubjectContext(ctx, id)
}

package main

import (
	"net/url"
	"strings"
)

const (
	discoverPlaceholderEN = "{keyword}"
	discoverPlaceholderCN = "{关键词}"
	discoverKeywordMax    = 100
)

var discoverMaxBytes = 8 << 20

type discoverSource struct {
	ID          string `json:"id"`
	Template    string `json:"-"`
	PageParam   string `json:"-"`
	PageSize    int    `json:"-"`
	ResultLimit int    `json:"-"`
}

func discoverSearchURL(template, keyword string) string {
	escaped := url.QueryEscape(keyword)
	return strings.ReplaceAll(strings.ReplaceAll(template, discoverPlaceholderEN, escaped), discoverPlaceholderCN, escaped)
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

package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// PROXY_URL keeps the default explicit-direct behavior; when set, outbound
// public-internet traffic (RSS fetch/preview, direct downloads, .torrent
// bytes, Bangumi covers, webhook) goes through it. Supported schemes are
// http, https, socks5, and socks5h, with credentials inside the URL. A value
// without a scheme is normalized to http. Downloader RPC (Aria2 /
// qBittorrent / Transmission) stays direct: those endpoints are usually on
// the user's own LAN and a remote proxy cannot reach them.

var proxyURL *url.URL

// newTransport is the single place building outbound transports, so every
// proxy-aware client shares the same configuration.
func newTransport() *http.Transport {
	transport := &http.Transport{
		Proxy:               nil,
		MaxIdleConns:        32,
		MaxIdleConnsPerHost: 16,
		MaxConnsPerHost:     16,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
		ForceAttemptHTTP2:   true,
	}
	if proxyURL != nil {
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	return transport
}

func newClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: newTransport()}
}

// newMediaTransport caps the wait for response headers so a stalled proxy or
// server fails fast, while the client timeout still allows multi-hour media
// downloads.
func newMediaTransport() *http.Transport {
	transport := newTransport()
	transport.ResponseHeaderTimeout = 30 * time.Second
	return transport
}

func parseProxyURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("代理地址无法解析: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, fmt.Errorf("不支持的代理协议 %q：支持 http、https、socks5、socks5h", u.Scheme)
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("代理地址缺少主机，例如 http://192.168.1.10:7890")
	}
	return u, nil
}

// applyProxyConfig parses PROXY_URL and rebuilds every proxy-aware client.
func applyProxyConfig(raw string) error {
	u, err := parseProxyURL(raw)
	if err != nil {
		return err
	}
	proxyURL = u
	httpClient.Transport = newTransport()
	mediaClient.Transport = newMediaTransport()
	torrentClient.Transport = newTransport()
	metaClient.Transport = newTransport()
	return nil
}

// redactProxy drops credentials before the address is logged or shown.
func redactProxy(u *url.URL) string {
	if u == nil {
		return ""
	}
	out := *u
	out.User = nil
	return out.String()
}

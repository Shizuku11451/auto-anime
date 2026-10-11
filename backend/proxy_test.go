package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestParseProxyURL(t *testing.T) {
	if u, err := parseProxyURL(""); err != nil || u != nil {
		t.Fatalf("empty proxy: %v %v", u, err)
	}
	u, err := parseProxyURL("127.0.0.1:7890")
	if err != nil || u.Scheme != "http" || u.Host != "127.0.0.1:7890" {
		t.Fatalf("scheme-less proxy should default to http: %v %v", u, err)
	}
	for _, raw := range []string{"http://10.0.0.1:8080", "https://10.0.0.1:8443", "socks5://10.0.0.1:1080", "socks5h://10.0.0.1:1080", "SOCKS5://10.0.0.1:1080"} {
		u, err := parseProxyURL(raw)
		if err != nil || u.Hostname() != "10.0.0.1" {
			t.Fatalf("proxy %q: %v %v", raw, u, err)
		}
	}
	u, err = parseProxyURL("socks5://alice:secret@10.0.0.1:1080")
	if err != nil || u.User == nil || u.User.Username() != "alice" {
		t.Fatalf("proxy credentials dropped: %v %v", u, err)
	}
	for _, raw := range []string{"ftp://10.0.0.1:21", "http://", "://7890", "http://:0"} {
		if u, err := parseProxyURL(raw); err == nil {
			t.Fatalf("invalid proxy %q accepted: %v", raw, u)
		}
	}
}

// The fake proxy answers without dialing the target, so an unresolvable
// target host proves the request was routed through the proxy.
func TestProxyRoutesOutboundContentRequests(t *testing.T) {
	oldDownloadDir := downloadDir
	downloadDir = t.TempDir()
	t.Cleanup(func() { downloadDir = oldDownloadDir })
	var viaProxy atomic.Bool
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		viaProxy.Store(true)
		if r.URL.Host == "" {
			t.Errorf("expected absolute target URI, got %q", r.RequestURI)
		}
		if strings.HasSuffix(r.URL.Path, ".torrent") {
			fmt.Fprint(w, "d4:infod4:name6:test.ee")
			return
		}
		fmt.Fprint(w, `<rss><channel><item><title>第一话</title><link>http://target.example/1.mp4</link></item></channel></rss>`)
	}))
	defer proxy.Close()
	t.Cleanup(func() {
		if err := applyProxyConfig(""); err != nil {
			t.Fatalf("reset proxy: %v", err)
		}
	})
	if err := applyProxyConfig(proxy.URL); err != nil {
		t.Fatal(err)
	}

	result, err := fetchRSS(context.Background(), "http://target.example/rss.xml")
	items := feedItemsFromEntries(result.Entries)
	if err != nil || len(items) != 1 {
		t.Fatalf("rss via proxy: %#v %v", items, err)
	}
	if !viaProxy.Load() {
		t.Fatal("RSS request did not reach the proxy")
	}
	viaProxy.Store(false)
	data, err := fetchTorrentBytes("http://target.example/a.torrent")
	if err != nil || len(data) == 0 {
		t.Fatalf("torrent via proxy: %q %v", data, err)
	}
	if !viaProxy.Load() {
		t.Fatal("torrent request did not reach the proxy")
	}
}

func TestApplyProxyConfigRejectsInvalidAndKeepsPrevious(t *testing.T) {
	t.Cleanup(func() {
		if err := applyProxyConfig(""); err != nil {
			t.Fatalf("reset proxy: %v", err)
		}
	})
	if err := applyProxyConfig("socks5://10.0.0.1:1080"); err != nil {
		t.Fatal(err)
	}
	if err := applyProxyConfig("ftp://10.0.0.1:21"); err == nil {
		t.Fatal("invalid proxy scheme accepted")
	}
	if proxyURL == nil || proxyURL.Scheme != "socks5" {
		t.Fatalf("failed apply changed the proxy: %v", proxyURL)
	}
}

func TestRedactProxyHidesCredentials(t *testing.T) {
	u, err := parseProxyURL("socks5://alice:secret@10.0.0.1:1080")
	if err != nil {
		t.Fatal(err)
	}
	if got := redactProxy(u); got != "socks5://10.0.0.1:1080" {
		t.Fatalf("redacted proxy: %q", got)
	}
	if got := redactProxy(nil); got != "" {
		t.Fatalf("redacted nil proxy: %q", got)
	}
}

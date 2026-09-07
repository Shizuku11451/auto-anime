package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// overrideDiscoverSource replaces the builtin source registry with one entry
// pointing at a local template; the registry is shared global state so tests
// must restore it.
func overrideDiscoverSource(t *testing.T, template string) {
	t.Helper()
	old := discoverBuiltinSources
	discoverBuiltinSources = []discoverSource{{ID: "acgrip", Name: "ACG.RIP", Template: template}}
	t.Cleanup(func() { discoverBuiltinSources = old })
}

func overrideDiscoverMaxBytes(t *testing.T, limit int) {
	t.Helper()
	old := discoverMaxBytes
	discoverMaxBytes = limit
	t.Cleanup(func() { discoverMaxBytes = old })
}

func discoverGet(t *testing.T, url string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	discoverSearchAPI(rec, httptest.NewRequest("GET", url, nil))
	return rec
}

func TestDiscoverSourceList(t *testing.T) {
	oldTemplate := discoverSearchTemplate
	t.Cleanup(func() { discoverSearchTemplate = oldTemplate })

	rec := httptest.NewRecorder()
	discoverSourcesAPI(rec, httptest.NewRequest("GET", "/api/discover/sources", nil))
	if rec.Code != 200 {
		t.Fatalf("sources: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Sources []discoverSource `json:"sources"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Sources) != 1 || out.Sources[0].ID != "acgrip" || out.Sources[0].Custom {
		t.Fatalf("builtin sources: %+v", out.Sources)
	}

	// 配置了合法模板后出现自定义源；非法模板被忽略，不进入列表。
	discoverSearchTemplate = "https://example.test/rss?kw={keyword}"
	out.Sources = nil
	rec = httptest.NewRecorder()
	discoverSourcesAPI(rec, httptest.NewRequest("GET", "/api/discover/sources", nil))
	json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Sources) != 2 || out.Sources[1].ID != "custom" || !out.Sources[1].Custom {
		t.Fatalf("sources with custom template: %+v", out.Sources)
	}
	discoverSearchTemplate = "https://example.test/rss?kw=missing"
	rec = httptest.NewRecorder()
	discoverSourcesAPI(rec, httptest.NewRequest("GET", "/api/discover/sources", nil))
	json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Sources) != 1 {
		t.Fatalf("invalid template must not appear: %+v", out.Sources)
	}
}

func TestDiscoverTemplateValidation(t *testing.T) {
	if _, err := validateDiscoverTemplate(""); err != nil {
		t.Fatalf("empty template must stay empty: %v", err)
	}
	if got, err := validateDiscoverTemplate("  https://x.test/s?k={keyword}  "); err != nil || got != "https://x.test/s?k={keyword}" {
		t.Fatalf("trim: %q %v", got, err)
	}
	if _, err := validateDiscoverTemplate("https://x.test/s?k=missing"); err == nil {
		t.Fatal("missing placeholder accepted")
	}
	if _, err := validateDiscoverTemplate("ftp://x.test/{keyword}"); err == nil {
		t.Fatal("non-http scheme accepted")
	}
	if discoverSearchURL("https://x.test/s?k={关键词}&q={keyword}", "芙 莉莲") != "https://x.test/s?k=%E8%8A%99+%E8%8E%89%E8%8E%B2&q=%E8%8A%99+%E8%8E%89%E8%8E%B2" {
		t.Fatalf("placeholder substitution: %s", discoverSearchURL("https://x.test/s?k={关键词}&q={keyword}", "芙 莉莲"))
	}
}

func TestDiscoverSearchParsesResults(t *testing.T) {
	var seenTerm string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenTerm = r.URL.Query().Get("term")
		fmt.Fprintf(w, `<rss version="2.0"><channel>
			<item><title>[字幕组] 夏日番剧 - 07 [1080p]</title><link>https://example.test/t/1</link>
				<enclosure url="/dl/1.torrent" type="application/x-bittorrent"/><pubDate>Mon, 07 Sep 2026 10:00:00 +0800</pubDate></item>
			<item><title>完结番剧 全一季</title><link>/t/2</link></item>
			<item><title></title><link>/t/3</link></item>
		</channel></rss>`)
	}))
	defer server.Close()
	overrideDiscoverSource(t, server.URL+"/search.xml?term={keyword}")

	rec := discoverGet(t, "/api/discover/search?source=acgrip&keyword="+"葬送的芙莉莲")
	if rec.Code != 200 {
		t.Fatalf("search: %d %s", rec.Code, rec.Body.String())
	}
	if seenTerm != "葬送的芙莉莲" {
		t.Fatalf("keyword was not forwarded escaped: %q", seenTerm)
	}
	var out struct {
		Source     string         `json:"source"`
		SourceName string         `json:"source_name"`
		Keyword    string         `json:"keyword"`
		FeedURL    string         `json:"feed_url"`
		Count      int            `json:"count"`
		Items      []discoverItem `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Source != "acgrip" || out.Keyword != "葬送的芙莉莲" {
		t.Fatalf("meta: %+v", out)
	}
	if out.FeedURL != server.URL+"/search.xml?term="+"%E8%91%AC%E9%80%81%E7%9A%84%E8%8A%99%E8%8E%89%E8%8E%B2" {
		t.Fatalf("feed url: %s", out.FeedURL)
	}
	if out.Count != 2 || len(out.Items) != 2 {
		t.Fatalf("items: %d %+v", out.Count, out.Items)
	}
	first := out.Items[0]
	if first.Group != "字幕组" || first.Episode != "07" || first.Quality != "1080P" {
		t.Fatalf("parsed info: %+v", first)
	}
	if first.Link != "https://example.test/t/1" {
		t.Fatalf("absolute link: %s", first.Link)
	}
	if first.Torrent != server.URL+"/dl/1.torrent" {
		t.Fatalf("relative enclosure resolved: %s", first.Torrent)
	}
	if out.Items[1].Episode != "" || out.Items[1].Torrent != "" || out.Items[1].Link != server.URL+"/t/2" {
		t.Fatalf("second item: %+v", out.Items[1])
	}
}

func TestDiscoverSearchInputValidation(t *testing.T) {
	overrideDiscoverSource(t, "https://example.test/?kw={keyword}")
	if rec := discoverGet(t, "/api/discover/search?source=acgrip&keyword="); rec.Code != 400 {
		t.Fatalf("empty keyword: %d", rec.Code)
	}
	if rec := discoverGet(t, "/api/discover/search?source=ghost&keyword=x"); rec.Code != 400 {
		t.Fatalf("unknown source: %d", rec.Code)
	}
	rec := httptest.NewRecorder()
	discoverSearchAPI(rec, httptest.NewRequest("POST", "/api/discover/search?source=acgrip&keyword=x", nil))
	if rec.Code != 405 {
		t.Fatalf("post: %d", rec.Code)
	}
}

func TestDiscoverSearchUpstreamFailures(t *testing.T) {
	t.Run("http error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()
		overrideDiscoverSource(t, server.URL+"/?kw={keyword}")
		rec := discoverGet(t, "/api/discover/search?source=acgrip&keyword=x")
		if rec.Code != 502 || !strings.Contains(rec.Body.String(), "500") {
			t.Fatalf("http error surfaced: %d %s", rec.Code, rec.Body.String())
		}
	})
	t.Run("not rss", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "<html><body>维护中</body></html>")
		}))
		defer server.Close()
		overrideDiscoverSource(t, server.URL+"/?kw={keyword}")
		rec := discoverGet(t, "/api/discover/search?source=acgrip&keyword=x")
		if rec.Code != 502 || !strings.Contains(rec.Body.String(), "RSS") {
			t.Fatalf("html response rejected: %d %s", rec.Code, rec.Body.String())
		}
	})
	t.Run("oversized", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, strings.Repeat("a", 128))
		}))
		defer server.Close()
		overrideDiscoverSource(t, server.URL+"/?kw={keyword}")
		overrideDiscoverMaxBytes(t, 64)
		rec := discoverGet(t, "/api/discover/search?source=acgrip&keyword=x")
		if rec.Code != 502 || !strings.Contains(rec.Body.String(), "超过") {
			t.Fatalf("size limit enforced: %d %s", rec.Code, rec.Body.String())
		}
	})
	t.Run("connection refused", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		base := server.URL
		server.Close()
		overrideDiscoverSource(t, base+"/?kw={keyword}")
		rec := discoverGet(t, "/api/discover/search?source=acgrip&keyword=x")
		if rec.Code != 502 || !strings.Contains(rec.Body.String(), "连接失败") {
			t.Fatalf("connection failure surfaced: %d %s", rec.Code, rec.Body.String())
		}
	})
}

func TestDiscoverCustomTemplateSetting(t *testing.T) {
	testDatabase(t)
	setWebAuth(t, "", "")
	setupDone = true
	oldTemplate := discoverSearchTemplate
	t.Cleanup(func() { discoverSearchTemplate = oldTemplate; setupDone = false; webHash = "" })

	// 非法模板整体拒绝且不落库。
	if rec := settingsPUT(t, `{"discover_search_template":"https://x.test/?kw=missing"}`); rec.Code != 400 {
		t.Fatalf("missing placeholder accepted: %d %s", rec.Code, rec.Body.String())
	}
	if discoverSearchTemplate != "" || storedSettings()["discover_search_template"] != "" {
		t.Fatalf("invalid template was stored: %q", discoverSearchTemplate)
	}

	// 合法模板保存后立即生效，接口回读，自定义源出现在列表里。
	if rec := settingsPUT(t, `{"discover_search_template":"https://x.test/rss?kw={keyword}"}`); rec.Code != 200 {
		t.Fatalf("valid template rejected: %d %s", rec.Code, rec.Body.String())
	}
	if discoverSearchTemplate != "https://x.test/rss?kw={keyword}" {
		t.Fatalf("template not applied live: %q", discoverSearchTemplate)
	}
	rec := httptest.NewRecorder()
	settingsHandler(rec, httptest.NewRequest("GET", "/api/settings", nil))
	if !strings.Contains(rec.Body.String(), `"discover_search_template":"https://x.test/rss?kw={keyword}"`) {
		t.Fatalf("settings response: %s", rec.Body.String())
	}
	var out struct {
		Sources []discoverSource `json:"sources"`
	}
	json.Unmarshal(discoverSourcesRecorder(t).Body.Bytes(), &out)
	if len(out.Sources) != 2 || out.Sources[1].ID != "custom" {
		t.Fatalf("custom source missing: %+v", out.Sources)
	}

	// 清空模板回到仅内置源；重启后 loadSettings 恢复同样的状态。
	if rec := settingsPUT(t, `{"discover_search_template":""}`); rec.Code != 200 {
		t.Fatalf("clear template: %d %s", rec.Code, rec.Body.String())
	}
	if discoverSearchTemplate != "" {
		t.Fatalf("template not cleared: %q", discoverSearchTemplate)
	}
	discoverSearchTemplate = "stale"
	loadSettings()
	if discoverSearchTemplate != "" {
		t.Fatalf("loadSettings did not restore cleared template: %q", discoverSearchTemplate)
	}

	// 自定义模板真实生效：搜索请求打到模板展开后的地址。
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("kw") == "测试" {
			hits.Add(1)
		}
		fmt.Fprint(w, `<rss version="2.0"><channel></channel></rss>`)
	}))
	defer server.Close()
	if rec := settingsPUT(t, fmt.Sprintf(`{"discover_search_template":%q}`, server.URL+"/find?kw={keyword}")); rec.Code != 200 {
		t.Fatalf("custom template for search: %d %s", rec.Code, rec.Body.String())
	}
	rec = discoverGet(t, "/api/discover/search?source=custom&keyword=测试")
	if rec.Code != 200 || hits.Load() != 1 {
		t.Fatalf("custom search: %d %s hits=%d", rec.Code, rec.Body.String(), hits.Load())
	}
	var searchOut struct {
		SourceName string `json:"source_name"`
	}
	json.Unmarshal(rec.Body.Bytes(), &searchOut)
	if searchOut.SourceName != "自定义搜索源" {
		t.Fatalf("custom source name: %q", searchOut.SourceName)
	}
}

func discoverSourcesRecorder(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	discoverSourcesAPI(rec, httptest.NewRequest("GET", "/api/discover/sources", nil))
	return rec
}

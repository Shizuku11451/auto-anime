package main

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const mikanTestEpisode = "https://mikanani.me/Home/Episode/abc123"
const mikanTestSeriesHTML = `<a href="https://bgm.tv/subject/4242">Bangumi</a>`
const mikanTestEpisodeHTML = `<a href="/Home/Bangumi/123">番剧主页</a>`

type mikanTestRoundTripper func(*http.Request) (*http.Response, error)

func (f mikanTestRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type mikanTestScenario struct {
	episodeHTML, seriesHTML string
	searchHit               bool
	networkError            bool
	pageStatus              int
	subjectStatus           int
	blockPage               bool
}

type mikanTestRequests struct {
	pages, searches, subjects atomic.Int32
}

// All HTTP requests, including production-looking Mikan URLs, terminate at
// this local fake server. Production host validation stays enabled in tests.
func startMikanLookupTest(t *testing.T, scenario mikanTestScenario) *mikanTestRequests {
	t.Helper()
	withMetaTestEnv(t)
	counts := &mikanTestRequests{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != metaUserAgent {
			t.Errorf("missing metadata user agent on %s", r.URL.Path)
		}
		switch {
		case strings.HasPrefix(r.URL.Path, "/Home/"):
			counts.pages.Add(1)
			if scenario.blockPage {
				<-r.Context().Done()
				return
			}
			if scenario.pageStatus != 0 {
				w.Header().Set("Location", "http://127.0.0.1/private")
				w.WriteHeader(scenario.pageStatus)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if strings.HasPrefix(r.URL.Path, "/Home/Episode/") {
				fmt.Fprint(w, scenario.episodeHTML)
			} else {
				fmt.Fprint(w, scenario.seriesHTML)
			}
		case strings.HasPrefix(r.URL.Path, "/search/subject/"):
			counts.searches.Add(1)
			list := []bangumiSubject{}
			if scenario.searchHit {
				subject := bangumiSubject{ID: 4343, NameCN: "测试动画", Summary: "文本搜索结果"}
				subject.Images.Large = "http://" + r.Host + "/cover.jpg"
				list = append(list, subject)
			}
			json.NewEncoder(w).Encode(map[string]any{"list": list})
		case strings.HasSuffix(r.URL.Path, "/characters"):
			fmt.Fprint(w, `[]`)
		case r.URL.Path == "/v0/subjects/4242":
			counts.subjects.Add(1)
			if scenario.subjectStatus != 0 {
				w.WriteHeader(scenario.subjectStatus)
				return
			}
			subject := bangumiSubject{ID: 4242, NameCN: "蜜柑精确作品", Summary: "页面提供的精确条目"}
			subject.Images.Large = "http://" + r.Host + "/cover.jpg"
			json.NewEncoder(w).Encode(subject)
		case r.URL.Path == "/cover.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			fmt.Fprint(w, "test-cover")
		default:
			t.Errorf("unexpected local request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	base, _ := url.Parse(server.URL)
	oldAPI, oldClient, oldHost := bangumiAPI, metaClient, coverHostAllowed
	bangumiAPI = server.URL
	transport := server.Client().Transport
	metaClient = &http.Client{Timeout: 15 * time.Second, Transport: mikanTestRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Hostname() == "mikanani.me" {
			if scenario.networkError {
				return nil, errors.New("simulated page connection failure")
			}
			copy := r.Clone(r.Context())
			copy.URL.Scheme, copy.URL.Host = base.Scheme, base.Host
			return transport.RoundTrip(copy)
		}
		if r.URL.Host != base.Host {
			return nil, fmt.Errorf("unexpected outbound host")
		}
		return transport.RoundTrip(r)
	})}
	coverHostAllowed = func(host string) bool { return host == base.Host }
	t.Cleanup(func() {
		server.Close()
		bangumiAPI, metaClient, coverHostAllowed = oldAPI, oldClient, oldHost
	})
	return counts
}

func seedMikanAnime(t *testing.T) int64 {
	t.Helper()
	id, err := ensureAnime("测试动画")
	if err != nil {
		t.Fatal(err)
	}
	for n := 1; n <= 3; n++ {
		if _, err := db.Exec(`INSERT INTO items(title,link,page_url,anime_title,anime_id) VALUES(?,?,?,'测试动画',?)`,
			fmt.Sprintf("测试动画 %02d", n), fmt.Sprintf("https://mikanani.me/Download/%d.torrent", n), mikanTestEpisode, id); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func TestMikanSubjectLinkPrecedesTextSearch(t *testing.T) {
	testDatabase(t)
	counts := startMikanLookupTest(t, mikanTestScenario{
		episodeHTML: `<!-- <a href="/Home/Bangumi/999">wrong</a> -->
			<script>var text = '<a href="/Home/Bangumi/888">wrong</a>';</script>
			<A data-note=">" HREF='/Home/Bangumi/123?from=rss&amp;v=1'>番剧</A>
			<a href="/Home/Bangumi/123#episodes">重复链接</a>`,
		seriesHTML: mikanTestSeriesHTML + `<a href="//bgm.tv/subject/4242?from=mikan">同一条目</a>`, searchHit: true,
	})
	seedMikanAnime(t)
	lookupAndCacheMeta("测试动画")
	var id, binding int64
	var source, key string
	if err := db.QueryRow(`SELECT subject_id,source,match_key,animes.bangumi_id FROM anime_meta JOIN animes ON animes.title=anime_meta.anime_title`).
		Scan(&id, &source, &key, &binding); err != nil {
		t.Fatal(err)
	}
	if id != 4242 || source != "bangumi" || binding != 0 || key != "mikan-link:"+metaLookupKey("测试动画", "") {
		t.Fatalf("wrong exact match: id=%d source=%q key=%q binding=%d", id, source, key, binding)
	}
	lookupAndCacheMeta("测试动画")
	if counts.pages.Load() != 2 || counts.subjects.Load() != 1 || counts.searches.Load() != 0 {
		t.Fatalf("request budget/priority: pages=%d subjects=%d searches=%d", counts.pages.Load(), counts.subjects.Load(), counts.searches.Load())
	}
}

func TestMikanFailureFallsBackWithoutNegativeCache(t *testing.T) {
	for _, kind := range []string{"episode-structure", "series-structure", "network", "http", "subject", "oversize", "ambiguous", "redirect"} {
		for _, searchHit := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/searchHit=%v", kind, searchHit), func(t *testing.T) {
				testDatabase(t)
				scenario := mikanTestScenario{episodeHTML: mikanTestEpisodeHTML, seriesHTML: mikanTestSeriesHTML, searchHit: searchHit}
				switch kind {
				case "episode-structure":
					scenario.episodeHTML = `<div>页面结构已变化</div>`
				case "series-structure":
					scenario.seriesHTML = `<a href="https://bgm.tv.evil.test/subject/4242">假链接</a>`
				case "network":
					scenario.networkError = true
				case "http":
					scenario.pageStatus = http.StatusServiceUnavailable
				case "subject":
					scenario.subjectStatus = http.StatusServiceUnavailable
				case "oversize":
					scenario.episodeHTML = mikanTestEpisodeHTML + strings.Repeat(" ", mikanPageMaxBytes)
				case "ambiguous":
					scenario.seriesHTML += `<a href="https://bgm.tv/subject/4343">另一个条目</a>`
				case "redirect":
					scenario.pageStatus = http.StatusFound
				}
				counts := startMikanLookupTest(t, scenario)
				seedMikanAnime(t)
				old := time.Now().Add(-8 * 24 * time.Hour).UTC().Format(time.RFC3339)
				if kind == "network" {
					if _, err := db.Exec(`INSERT INTO anime_meta(anime_title,source,updated_at,match_key) VALUES('测试动画','none',?,?)`, old, metaLookupKey("测试动画", "")); err != nil {
						t.Fatal(err)
					}
				}
				lookupAndCacheMeta("测试动画")
				if counts.searches.Load() != 1 || counts.pages.Load() > 2 {
					t.Fatalf("fallback/budget: searches=%d pages=%d", counts.searches.Load(), counts.pages.Load())
				}
				var id int64
				var source, updated string
				err := db.QueryRow(`SELECT subject_id,source,updated_at FROM anime_meta WHERE anime_title='测试动画'`).Scan(&id, &source, &updated)
				if searchHit {
					if err != nil || id != 4343 || source != "bangumi" {
						t.Fatalf("text fallback missing: id=%d source=%q err=%v", id, source, err)
					}
				} else if kind == "network" {
					if err != nil || source != "none" || updated != old {
						t.Fatalf("failure renewed old miss: source=%q updated=%q err=%v", source, updated, err)
					}
				} else if !errors.Is(err, sql.ErrNoRows) {
					t.Fatalf("page failure wrote a negative cache: %q %v", source, err)
				}
			})
		}
	}
}

func TestMikanSkippedForSearchNameAndKnownSubject(t *testing.T) {
	for _, mode := range []string{"search-name", "existing-auto", "manual"} {
		t.Run(mode, func(t *testing.T) {
			testDatabase(t)
			counts := startMikanLookupTest(t, mikanTestScenario{episodeHTML: mikanTestEpisodeHTML, seriesHTML: mikanTestSeriesHTML, searchHit: true})
			id := seedMikanAnime(t)
			wantID, wantSource := 4242, "bangumi"
			if mode == "search-name" {
				if _, err := db.Exec(`UPDATE animes SET search_name='测试动画' WHERE id=?`, id); err != nil {
					t.Fatal(err)
				}
				wantID = 4343
			} else {
				if mode == "manual" {
					wantSource = "manual"
					if _, err := db.Exec(`UPDATE animes SET bangumi_id=4242 WHERE id=?`, id); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := db.Exec(`INSERT INTO anime_meta(anime_title,subject_id,source) VALUES('测试动画',4242,?)`, wantSource); err != nil {
					t.Fatal(err)
				}
			}
			lookupAndCacheMeta("测试动画")
			var subjectID int
			var source string
			if err := db.QueryRow(`SELECT subject_id,source FROM anime_meta WHERE anime_title='测试动画'`).Scan(&subjectID, &source); err != nil {
				t.Fatal(err)
			}
			if subjectID != wantID || source != wantSource || counts.pages.Load() != 0 {
				t.Fatalf("priority: id=%d source=%s pages=%d", subjectID, source, counts.pages.Load())
			}
		})
	}
}

func TestMikanLookupRetriesV1NegativeCache(t *testing.T) {
	testDatabase(t)
	startMikanLookupTest(t, mikanTestScenario{episodeHTML: mikanTestEpisodeHTML, seriesHTML: mikanTestSeriesHTML})
	seedMikanAnime(t)
	oldHash := sha1.Sum([]byte("bracket-titles-v1\x00测试动画\x00"))
	if _, err := db.Exec(`INSERT INTO anime_meta(anime_title,source,updated_at,match_key) VALUES('测试动画','none',?,?)`, time.Now().UTC().Format(time.RFC3339), hex.EncodeToString(oldHash[:])); err != nil {
		t.Fatal(err)
	}
	enqueueMissingMeta()
	select {
	case title := <-metaQueue:
		lookupAndCacheMeta(title)
	default:
		t.Fatal("fresh v1 miss was not queued after matcher upgrade")
	}
	var source string
	if err := db.QueryRow(`SELECT source FROM anime_meta WHERE anime_title='测试动画'`).Scan(&source); err != nil || source != "bangumi" {
		t.Fatalf("legacy negative not repaired: %q %v", source, err)
	}
}

func TestMikanPageAllowlistAndCancellation(t *testing.T) {
	for _, address := range []string{
		"https://mikanani.me.evil.test/Home/Episode/abc", "https://evil.mikanani.me/Home/Episode/abc",
		"https://user@mikanani.me/Home/Episode/abc", "https://mikanani.me:9999/Home/Episode/abc",
		"file://mikanani.me/Home/Episode/abc", "http://127.0.0.1/Home/Episode/abc",
	} {
		u, _ := url.Parse(address)
		if mikanPageAllowed(u) {
			t.Errorf("unsafe page allowed: %s", address)
		}
	}
	if isBangumiHost("mikanani.me") {
		t.Fatal("cover host allowlist widened to Mikan")
	}
	counts := startMikanLookupTest(t, mikanTestScenario{blockPage: true})
	u, _ := url.Parse(mikanTestEpisode)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := mikanBangumiID(ctx, u)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second || counts.pages.Load() != 1 {
		t.Fatalf("page cancellation: err=%v elapsed=%s pages=%d", err, time.Since(start), counts.pages.Load())
	}
}

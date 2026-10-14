package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	titleparse "auto-anime/internal/title"
)

const bracketedMushokuTitle = "[7月新番][无职转生 第三季 ～到了异世界就拿出真本事～][Mushoku Tensei III: Isekai Ittara Honki Dasu] [1080P][GB][MP4][简中]"

func TestMetaQueriesKeepBracketedNames(t *testing.T) {
	cases := []struct {
		title string
		want  []string
	}{
		{bracketedMushokuTitle, []string{"无职转生 第三季 ～到了异世界就拿出真本事～", "Mushoku Tensei III: Isekai Ittara Honki Dasu"}},
		{"[爱恋字幕社]" + bracketedMushokuTitle + "[01]", []string{"无职转生 第三季 ～到了异世界就拿出真本事～", "Mushoku Tensei III: Isekai Ittara Honki Dasu"}},
		{"【我推的孩子】 第二季 [WebRip][1080p HEVC AAC][简繁日内封]", []string{"我推的孩子 第二季"}},
		{"[葬送的芙莉莲][Sousou no Frieren][1080P][简中", []string{"葬送的芙莉莲", "Sousou no Frieren"}},
		{"Fate/stay night [Unlimited Blade Works] [BDRip][1080p]", []string{"Fate/stay night Unlimited Blade Works"}},
		{"86 -不存在的战区- [1080p]", []string{"86 -不存在的战区-"}},
		{"【我推的孩子】 II [1080p]", []string{"我推的孩子 II"}},
		{"测试动画 1080p HEVC AAC", []string{"测试动画"}},
		{"[1080p][x265][10bit][AAC][A1B2C3D4][简日双语]", nil},
	}
	for _, c := range cases {
		t.Run(c.title, func(t *testing.T) {
			if got := titleparse.QueryTitles(c.title); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("queries: got %#v, want %#v", got, c.want)
			}
		})
	}
}

func metaSearchTestServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	oldAPI, oldClient := bangumiAPI, metaClient
	bangumiAPI, metaClient = server.URL, server.Client()
	t.Cleanup(func() {
		server.Close()
		bangumiAPI, metaClient = oldAPI, oldClient
	})
}

func TestCoverLookupRepairsOldBracketedMiss(t *testing.T) {
	testDatabase(t)
	const nameCN = "无职转生 第三季 ～到了异世界就拿出真本事～"
	_, imageHits := startMetaTestServer(t, nameCN)
	id, err := ensureAnime(bracketedMushokuTitle)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a fresh miss made by the old matcher, which dropped both names.
	if _, err := db.Exec("INSERT INTO anime_meta(anime_title,source,updated_at) VALUES(?,'none',?)",
		bracketedMushokuTitle, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO items(title,link,anime_title,anime_id,episode) VALUES(?,?,?,?,?)",
		"[爱恋字幕社]"+bracketedMushokuTitle+"[01]", "https://example.test/01.torrent", bracketedMushokuTitle, id, "01"); err != nil {
		t.Fatal(err)
	}
	enqueueMissingMeta()
	select {
	case title := <-metaQueue:
		lookupAndCacheMeta(title)
	default:
		t.Fatal("legacy miss was not queued")
	}
	var title, name, source, cover string
	var season int
	if err := db.QueryRow(`SELECT animes.title,animes.season,anime_meta.name_cn,anime_meta.source,anime_meta.cover_file
		FROM animes JOIN anime_meta ON animes.title=anime_meta.anime_title WHERE animes.id=?`, id).
		Scan(&title, &season, &name, &source, &cover); err != nil {
		t.Fatal(err)
	}
	// 资料匹配成功后番剧名会被规范成官方中文名（剥掉季标记），元数据键随之改名。
	wantTitle := "无职转生 ～到了异世界就拿出真本事～"
	if title != wantTitle || season != 0 || name != nameCN || source != "bangumi" || !coverFilePattern.MatchString(cover) {
		t.Fatalf("规范化后的番剧名或资料不符: %q %d %q %q %q", title, season, name, source, cover)
	}
	rec := httptest.NewRecorder()
	animeEntryAPI(rec, httptest.NewRequest("GET", fmt.Sprintf("/api/animes/%d", id), nil))
	var detail struct {
		Cover string `json:"cover"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil || detail.Cover != coverPath(cover) {
		t.Fatalf("detail did not expose cached cover: %s (%v)", rec.Body.String(), err)
	}
	// 番剧名已规范成官方名，再次查询用新名字验证封面只下载一次。
	lookupAndCacheMeta(animeTitleByID(t, id))
	if *imageHits != 1 {
		t.Fatalf("cover downloaded %d times", *imageHits)
	}
}

func TestMetaSearchFallbackPreservesOriginalSeason(t *testing.T) {
	for _, target := range []string{"Mushoku Tensei III: Isekai Ittara Honki Dasu", "无职转生"} {
		t.Run(target, func(t *testing.T) {
			var queries []string
			metaSearchTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				query := strings.TrimPrefix(r.URL.Path, "/search/subject/")
				queries = append(queries, query)
				list := []bangumiSubject{{ID: 1, NameCN: "无职转生 第二季 ～到了异世界就拿出真本事～"}}
				if query == target {
					list = append(list,
						bangumiSubject{ID: 2, NameCN: "无职转生～到了异世界就拿出真本事～"},
						bangumiSubject{ID: 3, Name: "無職転生Ⅲ ～異世界行ったら本気だす～", NameCN: "无职转生 第三季 ～到了异世界就拿出真本事～"})
				}
				json.NewEncoder(w).Encode(map[string]any{"list": list})
			})
			subject, err := searchBangumiAliases(context.Background(), titleparse.QueryTitles(bracketedMushokuTitle))
			if err != nil || subject == nil || subject.ID != 3 {
				t.Fatalf("fallback chose %+v (%v), queries=%v", subject, err, queries)
			}
			if queries[len(queries)-1] != target || len(queries) > titleparse.MaxMetaQueries {
				t.Fatalf("unexpected queries: %v", queries)
			}
		})
	}
}

func TestMetaCandidateOrderAndAmbiguity(t *testing.T) {
	aliases := []string{"魔法少女小圆"}
	exact := bangumiSubject{ID: 1, NameCN: "魔法少女小圆"}
	other := bangumiSubject{ID: 2, NameCN: "魔法少女育成计划"}
	for _, list := range [][]bangumiSubject{{other, exact}, {exact, other}} {
		if got, _ := bestMetaCandidate(aliases, list); got == nil || got.ID != 1 {
			t.Fatalf("wrong best match: %+v", got)
		}
	}
	sameName := bangumiSubject{ID: 3, NameCN: "魔法少女小圆"}
	for _, list := range [][]bangumiSubject{{exact, sameName}, {sameName, exact}} {
		if got, _ := bestMetaCandidate(aliases, list); got != nil {
			t.Fatalf("ambiguous match selected by API order: %+v", got)
		}
	}
}

func TestMetaNegativeCacheRenewsAndSkipsDuplicateLookups(t *testing.T) {
	testDatabase(t)
	var requests atomic.Int32
	metaSearchTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		fmt.Fprint(w, `{"list":[]}`)
	})
	old := time.Now().Add(-8 * 24 * time.Hour).UTC().Format(time.RFC3339)
	if _, err := db.Exec("INSERT INTO anime_meta(anime_title,source,updated_at,match_key) VALUES('测试动画','none',?,?)",
		old, metaLookupKey("测试动画", "")); err != nil {
		t.Fatal(err)
	}
	lookupAndCacheMeta("测试动画")
	lookupAndCacheMeta("测试动画")
	var updated, key string
	if err := db.QueryRow("SELECT updated_at,match_key FROM anime_meta WHERE anime_title='测试动画'").Scan(&updated, &key); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 || updated == old || key != metaLookupKey("测试动画", "") {
		t.Fatalf("negative cache not renewed: requests=%d updated=%q key=%q", requests.Load(), updated, key)
	}
}

func TestMetaSearchNameEditRetriesMiss(t *testing.T) {
	testDatabase(t)
	_, imageHits := startMetaTestServer(t, "测试动画")
	id, err := ensureAnime("旧搜索名")
	if err != nil {
		t.Fatal(err)
	}
	lookupAndCacheMeta("旧搜索名")
	rec := putAnime(t, id, `{"search_name":"测试动画"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("search name update: %d %s", rec.Code, rec.Body.String())
	}
	select {
	case title := <-metaQueue:
		lookupAndCacheMeta(title)
	default:
		t.Fatal("updated search name did not queue a retry")
	}
	if *imageHits != 1 {
		t.Fatalf("corrected search name did not obtain cover: %d", *imageHits)
	}
}

func TestMetaFailureDoesNotRenewMiss(t *testing.T) {
	for _, kind := range []string{"http", "json", "oversize"} {
		t.Run(kind, func(t *testing.T) {
			testDatabase(t)
			metaSearchTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch kind {
				case "http":
					http.Error(w, "rate limited", http.StatusTooManyRequests)
				case "json":
					fmt.Fprint(w, "{")
				case "oversize":
					fmt.Fprint(w, `{"list":[]}`+strings.Repeat(" ", 2<<20))
				}
			})
			old := time.Now().Add(-8 * 24 * time.Hour).UTC().Format(time.RFC3339)
			if _, err := db.Exec("INSERT INTO anime_meta(anime_title,source,updated_at,match_key) VALUES('测试动画','none',?,?)",
				old, metaLookupKey("测试动画", "")); err != nil {
				t.Fatal(err)
			}
			lookupAndCacheMeta("测试动画")
			var updated string
			if err := db.QueryRow("SELECT updated_at FROM anime_meta WHERE anime_title='测试动画'").Scan(&updated); err != nil || updated != old {
				t.Fatalf("failure extended negative cache: %q %v", updated, err)
			}
		})
	}
}

func TestMetaSearchBudgetAndCancellation(t *testing.T) {
	aliases := []string{"第一动画 ～副标题～", "Second Anime: Subtitle", "Third Anime", "Fourth Anime", "Fifth Anime", "Sixth Anime", "Seventh Anime"}
	queries := titleparse.SearchQueries(append(aliases, aliases...))
	if len(queries) != titleparse.MaxMetaQueries {
		t.Fatalf("search limit: %v", queries)
	}
	seen := map[string]bool{}
	for _, q := range queries {
		if seen[q] {
			t.Fatalf("duplicate query: %q", q)
		}
		seen[q] = true
	}
	var requests atomic.Int32
	metaSearchTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		fmt.Fprint(w, `{"list":[]}`)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := searchBangumiAliases(ctx, aliases)
	if !errors.Is(err, context.DeadlineExceeded) || requests.Load() > 1 {
		t.Fatalf("cancellation: err=%v requests=%d", err, requests.Load())
	}
}

func TestLateAutomaticMatchDoesNotReplaceManualBinding(t *testing.T) {
	testDatabase(t)
	server, _ := startMetaTestServer(t, "测试动画")
	if _, err := db.Exec("INSERT INTO anime_meta(anime_title,source,subject_id,name_cn) VALUES('测试动画','manual',999,'手动选择')"); err != nil {
		t.Fatal(err)
	}
	subject := &bangumiSubject{ID: 4242, NameCN: "测试动画"}
	subject.Images.Large = server.URL + "/pic/cover/l/x.jpg"
	if err := storeMetaSubject("测试动画", subject, "bangumi", "", nil); err != nil {
		t.Fatal(err)
	}
	var id int
	var name, source string
	if err := db.QueryRow("SELECT subject_id,name_cn,source FROM anime_meta WHERE anime_title='测试动画'").Scan(&id, &name, &source); err != nil {
		t.Fatal(err)
	}
	if id != 999 || name != "手动选择" || source != "manual" {
		t.Fatalf("manual binding replaced: %d %q %q", id, name, source)
	}
}

func TestMetaSearchNameCanCorrectSeason(t *testing.T) {
	testDatabase(t)
	const searchName = "测试动画 第三季"
	startMetaTestServer(t, searchName)
	if _, err := db.Exec("INSERT INTO animes(title,search_name) VALUES('测试动画 第二季',?)", searchName); err != nil {
		t.Fatal(err)
	}
	lookupAndCacheMeta("测试动画 第二季")
	var name string
	if err := db.QueryRow("SELECT name_cn FROM anime_meta").Scan(&name); err != nil || name != searchName {
		t.Fatalf("explicit season correction ignored: %q %v", name, err)
	}
	aliases := titleparse.LookupAliases(bracketedMushokuTitle, "无职转生")
	if titleparse.AliasesScore(aliases, "", "无职转生 第二季") != 0 {
		t.Fatal("short custom keyword lost the original season")
	}
}

func TestLateAutomaticMatchDoesNotUseOldSearchName(t *testing.T) {
	testDatabase(t)
	server, _ := startMetaTestServer(t, "测试动画")
	if _, err := db.Exec("INSERT INTO animes(title,search_name) VALUES('测试动画','用户纠正后的搜索词')"); err != nil {
		t.Fatal(err)
	}
	// The network lookup captured an empty search name before the user edit.
	subject := &bangumiSubject{ID: 4242, NameCN: "测试动画"}
	subject.Images.Large = server.URL + "/pic/cover/l/x.jpg"
	expected := metaLookupState{ID: animeIDByTitle(t, "测试动画")}
	if err := storeMetaSubject("测试动画", subject, "bangumi", "", &expected); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM anime_meta WHERE anime_title='测试动画'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("stale search result persisted: %d %v", count, err)
	}
}

func TestMetaSearchEscapesQuery(t *testing.T) {
	query := "Fate/stay night"
	metaSearchTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/search/subject/"+url.PathEscape(query) || r.URL.Query().Get("type") != "2" {
			t.Errorf("invalid search URL: %s", r.URL.String())
		}
		fmt.Fprint(w, `{"list":[]}`)
	})
	if _, err := searchBangumiAliases(context.Background(), titleparse.QueryTitles(query)); err != nil {
		t.Fatal(err)
	}
}

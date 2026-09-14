package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---- bencode 测试夹具 ----

func bencStr(s string) []byte { return append([]byte(strconv.Itoa(len(s))+":"), s...) }
func bencInt(v int64) []byte  { return []byte("i" + strconv.FormatInt(v, 10) + "e") }

func bencDict(pairs ...[]byte) []byte {
	out := []byte("d")
	for _, p := range pairs {
		out = append(out, p...)
	}
	return append(out, 'e')
}
func bencList(items ...[]byte) []byte {
	out := []byte("l")
	for _, item := range items {
		out = append(out, item...)
	}
	return append(out, 'e')
}

func buildTorrent(t *testing.T, announce string, announceList [][]string) []byte {
	t.Helper()
	pairs := [][]byte{bencStr("info"), bencDict(bencStr("length"), bencInt(100), bencStr("name"), bencStr("test.mkv"))}
	if announce != "" {
		pairs = append(pairs, bencStr("announce"), bencStr(announce))
	}
	if announceList != nil {
		list := []byte("l")
		for _, group := range announceList {
			strs := make([][]byte, 0, len(group))
			for _, u := range group {
				strs = append(strs, bencStr(u))
			}
			list = append(list, bencList(strs...)...)
		}
		pairs = append(pairs, bencStr("announce-list"), append(list, 'e'))
	}
	return bencDict(pairs...)
}

// ---- 假 tracker ----

// startFakeHTTPTracker 返回地址；seeds/leechers 为应答值；failReason 非空时返回 failure reason。
func startFakeHTTPTracker(t *testing.T, seeds, leechers int, failReason string, capture *sync.Map) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if capture != nil {
			capture.Store("query", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "text/plain")
		if failReason != "" {
			fmt.Fprintf(w, "d14:failure reason%d:%se", len(failReason), failReason)
			return
		}
		fmt.Fprintf(w, "d8:completei%de10:incompletei%de8:intervali1800e5:peers0:e", seeds, leechers)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// torrentServingHost 用本地 HTTP 服务提供种子字节。
func torrentServingHost(t *testing.T, torrent []byte) string {
	t.Helper()
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(torrent)
	}))
	t.Cleanup(host.Close)
	return host.URL + "/file.torrent"
}

// startFakeUDPTracker 实现 BEP 15 connect+announce 最小应答。
func startFakeUDPTracker(t *testing.T, seeds, leechers int) string {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("启动 UDP tracker 失败: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if n < 16 {
				continue
			}
			action := binary.BigEndian.Uint32(buf[8:12])
			tid := binary.BigEndian.Uint32(buf[12:16])
			var reply []byte
			if action == 0 {
				reply = make([]byte, 16)
				binary.BigEndian.PutUint32(reply[0:4], 0)
				binary.BigEndian.PutUint32(reply[4:8], tid)
				binary.BigEndian.PutUint64(reply[8:16], 0x1122334455667788)
			} else if action == 1 && n >= 98 {
				reply = make([]byte, 20)
				binary.BigEndian.PutUint32(reply[0:4], 1)
				binary.BigEndian.PutUint32(reply[4:8], tid)
				binary.BigEndian.PutUint32(reply[8:12], 1800)
				binary.BigEndian.PutUint32(reply[12:16], uint32(leechers))
				binary.BigEndian.PutUint32(reply[16:20], uint32(seeds))
			} else {
				continue
			}
			_, _ = conn.WriteToUDP(reply, addr)
		}
	}()
	return "udp://" + conn.LocalAddr().String() + "/announce"
}

// ---- 纯函数测试 ----

func TestTorrentTrackerListParsesAnnounceAndList(t *testing.T) {
	data := buildTorrent(t, "http://a.example/announce", [][]string{
		{"http://b.example/announce", "udp://c.example:6969/announce"},
		{"http://b.example/announce"},
		{"wss://skip.example/announce"},
	})
	got := torrentTrackerList(data)
	// torrentTrackerList 只做提取与去重，协议过滤在 filterTrackerURLs 中
	want := []string{"http://a.example/announce", "http://b.example/announce", "udp://c.example:6969/announce", "wss://skip.example/announce"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("tracker 列表不符: %v", got)
	}
	filtered := filterTrackerURLs([]string{
		"http://a.example/announce", "ws://x.example", "udp://c.example:6969/announce", "http://a.example/announce",
	})
	if len(filtered) != 2 || filtered[0] != "http://a.example/announce" {
		t.Fatalf("过滤结果不符: %v", filtered)
	}
}

func TestTorrentTotalLengthSingleAndMultiFile(t *testing.T) {
	single := bencDict(
		bencStr("info"),
		bencDict(bencStr("length"), bencInt(12345), bencStr("name"), bencStr("a.mkv")),
	)
	if got := torrentTotalLength(single); got != 12345 {
		t.Fatalf("单文件长度不符: %d", got)
	}
	files := bencList(
		bencDict(bencStr("length"), bencInt(100), bencStr("path"), bencList(bencStr("a.mkv"))),
		bencDict(bencStr("length"), bencInt(250), bencStr("path"), bencList(bencStr("b.srt"))),
	)
	multi := bencDict(bencStr("info"), bencDict(bencStr("files"), files, bencStr("name"), bencStr("dir")))
	if got := torrentTotalLength(multi); got != 350 {
		t.Fatalf("多文件长度不符: %d", got)
	}
}

// ---- 可用性检测行为 ----

func overrideAvailability(t *testing.T) {
	t.Helper()
	oldDir := downloadDir
	downloadDir = t.TempDir()
	t.Cleanup(func() { downloadDir = oldDir })
}

func availabilityFromHandler(t *testing.T, urls []string) (int, []availabilityResult) {
	t.Helper()
	body, _ := json.Marshal(availabilityInput{URLs: urls})
	rec := httptest.NewRecorder()
	discoverAvailabilityAPI(rec, httptest.NewRequest("POST", "/api/discover/availability", bytes.NewReader(body)))
	if rec.Code != 200 {
		return rec.Code, nil
	}
	var out struct {
		Results []availabilityResult `json:"results"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	return rec.Code, out.Results
}

func TestAvailabilityDetectsSeedsViaHTTPTracker(t *testing.T) {
	overrideAvailability(t)
	var capture sync.Map
	tracker := startFakeHTTPTracker(t, 3, 5, "", &capture)
	link := torrentServingHost(t, buildTorrent(t, tracker, nil))

	code, results := availabilityFromHandler(t, []string{link})
	if code != 200 || len(results) != 1 {
		t.Fatalf("响应异常: code=%d results=%v", code, results)
	}
	r := results[0]
	if r.Verdict != "available" || r.Seeds != 3 || r.Leechers != 5 {
		t.Fatalf("应检测到做种: %+v", r)
	}
	if r.TrackersAlive != 1 || r.TrackersTotal != 1 {
		t.Fatalf("tracker 计数不符: %+v", r)
	}
	if len(r.InfoHash) != 40 {
		t.Fatalf("infohash 缺失: %q", r.InfoHash)
	}
	q, _ := capture.Load("query")
	if !strings.Contains(q.(string), "left=100") {
		t.Fatalf("announce 应携带种子长度 left=100: %v", q)
	}
}

func TestAvailabilityNoSeedsWhenTrackerAlive(t *testing.T) {
	overrideAvailability(t)
	tracker := startFakeHTTPTracker(t, 0, 2, "", nil)
	link := torrentServingHost(t, buildTorrent(t, tracker, nil))

	_, results := availabilityFromHandler(t, []string{link})
	if results[0].Verdict != "no_seeds" || results[0].TrackersAlive != 1 || results[0].Seeds != 0 {
		t.Fatalf("应判定无做种: %+v", results[0])
	}
}

func TestAvailabilityUnreachableWhenAllTrackersFail(t *testing.T) {
	overrideAvailability(t)
	tracker := startFakeHTTPTracker(t, 0, 0, "torrent not registered", nil)
	link := torrentServingHost(t, buildTorrent(t, tracker, nil))

	_, results := availabilityFromHandler(t, []string{link})
	if results[0].Verdict != "unreachable" || results[0].TrackersAlive != 0 {
		t.Fatalf("tracker 全失败应判 unreachable: %+v", results[0])
	}
	if results[0].Trackers[0].Error == "" {
		t.Fatalf("应包含 tracker 错误信息: %+v", results[0])
	}
}

func TestAvailabilityUnknownWithoutTrackers(t *testing.T) {
	overrideAvailability(t)
	link := torrentServingHost(t, buildTorrent(t, "", nil))

	_, results := availabilityFromHandler(t, []string{link})
	if results[0].Verdict != "unknown" || results[0].Error == "" {
		t.Fatalf("无 tracker 应判 unknown: %+v", results[0])
	}
}

func TestAvailabilityMagnetUsesTRTrackers(t *testing.T) {
	overrideAvailability(t)
	udp := startFakeUDPTracker(t, 7, 1)
	httpTracker := startFakeHTTPTracker(t, 1, 1, "", nil)
	magnet := "magnet:?xt=urn:btih:7135d830b60eda66853eb780a94e53aec0528d83" +
		"&tr=" + queryEscapeForTest(udp) + "&tr=" + queryEscapeForTest(httpTracker)

	_, results := availabilityFromHandler(t, []string{magnet})
	r := results[0]
	if r.Kind != "magnet" || r.Verdict != "available" {
		t.Fatalf("磁力应检测到做种: %+v", r)
	}
	if r.Seeds != 7 {
		t.Fatalf("应取 tracker 汇报的最大做种数 7: %+v", r)
	}
	if r.TrackersTotal != 2 || r.TrackersAlive != 2 {
		t.Fatalf("应同时探测 udp 与 http tracker: %+v", r)
	}
	if r.InfoHash != "7135d830b60eda66853eb780a94e53aec0528d83" {
		t.Fatalf("infohash 不符: %q", r.InfoHash)
	}
}

func TestAvailabilityUDPTrackerProtocol(t *testing.T) {
	overrideAvailability(t)
	udp := startFakeUDPTracker(t, 9, 0)
	u, err := url.Parse(udp)
	if err != nil {
		t.Fatalf("解析 %s: %v", udp, err)
	}
	seeds, leechers, err := udpTrackerAnnounce(context.Background(), u,
		"7135d830b60eda66853eb780a94e53aec0528d83", 391151608)
	if err != nil {
		t.Fatalf("UDP announce 失败: %v", err)
	}
	if seeds != 9 || leechers != 0 {
		t.Fatalf("UDP 应答不符: seeds=%d leechers=%d", seeds, leechers)
	}
}

func TestAvailabilityHandlerValidatesInput(t *testing.T) {
	overrideAvailability(t)
	rec := httptest.NewRecorder()
	discoverAvailabilityAPI(rec, httptest.NewRequest("GET", "/api/discover/availability", nil))
	if rec.Code != 405 {
		t.Fatalf("GET 应 405: %d", rec.Code)
	}
	urls := make([]string, 21)
	for i := range urls {
		urls[i] = "magnet:?xt=urn:btih:" + strings.Repeat("a", 40)
	}
	if code, _ := availabilityFromHandler(t, urls); code != 400 {
		t.Fatalf("21 条应 400: %d", code)
	}
	if code, results := availabilityFromHandler(t, []string{"ftp://example.com/a.torrent", "magnet:?xt=urn:btih:" + strings.Repeat("a", 40)}); code != 200 {
		t.Fatalf("混合输入应 200: %d", code)
	} else if len(results) != 2 || results[0].Verdict != "unknown" || results[1].Verdict != "unknown" {
		t.Fatalf("非法输入应逐条 unknown: %+v", results)
	} else if results[0].Error == "" || results[1].Error == "" {
		t.Fatalf("非法输入应带错误说明: %+v", results)
	}
}

func TestAvailabilitySlowTrackerBoundedByTimeout(t *testing.T) {
	overrideAvailability(t)
	oldTimeout := availabilityHTTPTimeout
	availabilityHTTPTimeout = 300 * time.Millisecond
	t.Cleanup(func() { availabilityHTTPTimeout = oldTimeout })

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		fmt.Fprint(w, "d8:completei1e10:incompletei0e5:peers0:e")
	}))
	defer slow.Close()
	link := torrentServingHost(t, buildTorrent(t, slow.URL, nil))

	start := time.Now()
	_, results := availabilityFromHandler(t, []string{link})
	elapsed := time.Since(start)
	if results[0].Verdict != "unreachable" {
		t.Fatalf("慢 tracker 应超时判 unreachable: %+v", results[0])
	}
	if elapsed > 8*time.Second {
		t.Fatalf("检测应在预算内结束，实际 %v", elapsed)
	}
}

func queryEscapeForTest(s string) string {
	return url.QueryEscape(s)
}

func TestAvailabilityRejectsInvalidTrackerResponses(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"html", "<html>Access denied</html>"},
		{"empty", ""},
		{"missing counts", "d8:intervali1800e5:peers0:e"},
		{"string count", "d8:complete4:oops10:incompletei0ee"},
		{"negative count", "d8:completei-1e10:incompletei0ee"},
		{"integer overflow", "d8:completei9223372036854775808ee"},
		{"invalid leechers", "d8:completei4e10:incomplete3:bade"},
		{"truncated", "d8:completei0e"},
		{"trailing data", "d8:completei0ee<html>"},
		{"non dictionary", "l8:completei0ee"},
		{"string length overflow", "d9223372036854775808:xe"},
		{"nested length overflow", "d8:completei0e5:peersl18446744073709551616:xee"},
		{"deep nesting", "d5:peers" + strings.Repeat("l", 1000) + "0:" + strings.Repeat("e", 1000) + "e"},
		{"oversized", "d8:completei0ee" + strings.Repeat(" ", 64<<10)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tracker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, tc.body)
			}))
			defer tracker.Close()
			// Exercise the real worker goroutines: a parser panic would kill the test process.
			code, results := availabilityFromHandler(t, []string{
				"magnet:?xt=urn:btih:7135d830b60eda66853eb780a94e53aec0528d83&tr=" + url.QueryEscape(tracker.URL),
			})
			if code != http.StatusOK || len(results) != 1 {
				t.Fatalf("invalid response: status=%d results=%v", code, results)
			}
			result := results[0]
			if result.Verdict != "unreachable" || result.TrackersAlive != 0 || result.Trackers[0].Error == "" {
				t.Fatalf("invalid response must not establish seeding status: %+v", result)
			}
		})
	}
}

func TestAvailabilityCompleteCountWithoutOptionalLeechers(t *testing.T) {
	for _, seeds := range []int{0, 4} {
		tracker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "d8:completei%dee", seeds)
		}))
		result := checkResourceAvailability(context.Background(),
			"magnet:?xt=urn:btih:7135d830b60eda66853eb780a94e53aec0528d83&tr="+url.QueryEscape(tracker.URL))
		tracker.Close()
		want := "no_seeds"
		if seeds > 0 {
			want = "available"
		}
		if result.Verdict != want || result.Seeds != seeds || result.TrackersAlive != 1 {
			t.Fatalf("valid complete count rejected: %+v", result)
		}
	}
}

func TestAvailabilityBudgetIncludesTorrentFetch(t *testing.T) {
	overrideAvailability(t)
	oldBudget := availabilityBudget
	availabilityBudget = 75 * time.Millisecond
	t.Cleanup(func() { availabilityBudget = oldBudget })
	cancelled := make(chan struct{})
	torrent := buildTorrent(t, "", nil)
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			close(cancelled)
		case <-time.After(time.Second):
			w.Write(torrent)
		}
	}))
	defer host.Close()
	start := time.Now()
	result := checkResourceAvailability(context.Background(), host.URL+"/slow.torrent")
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond || result.Verdict != "unknown" || result.Error == "" {
		t.Fatalf("torrent fetch exceeded budget: elapsed=%v result=%+v", elapsed, result)
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("torrent HTTP request was not cancelled")
	}
}

func TestAvailabilityCancellationStopsQueuedTorrentFetches(t *testing.T) {
	overrideAvailability(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var requests atomic.Int32
	started := make(chan struct{})
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == availabilityURLWorkers {
			close(started)
		}
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer host.Close()
	urls := make([]string, availabilityMaxURLs)
	for i := range urls {
		urls[i] = fmt.Sprintf("%s/%d.torrent", host.URL, i)
	}
	body, _ := json.Marshal(availabilityInput{URLs: urls})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/discover/availability", bytes.NewReader(body)).WithContext(ctx)
	done := make(chan struct{})
	go func() { discoverAvailabilityAPI(rec, req); close(done) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		cancel()
		<-done
		t.Fatal("initial workers did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		<-done
		t.Fatal("cancelled batch did not stop promptly")
	}
	if got := requests.Load(); got != availabilityURLWorkers {
		t.Fatalf("cancelled batch started additional torrent fetches: %d", got)
	}
	result := checkResourceAvailability(ctx, host.URL+"/cancelled.torrent")
	if requests.Load() != availabilityURLWorkers || result.Verdict != "unknown" {
		t.Fatalf("already-cancelled request performed network I/O: %+v", result)
	}
}

func TestAvailabilityUDPCancellationClosesConnection(t *testing.T) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	started := make(chan struct{})
	go func() {
		buf := make([]byte, 128)
		if _, _, err := conn.ReadFromUDP(buf); err == nil {
			close(started)
		}
	}()
	u, _ := url.Parse("udp://" + conn.LocalAddr().String() + "/announce")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, _, err := udpTrackerAnnounce(ctx, u, "7135d830b60eda66853eb780a94e53aec0528d83", 1)
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("UDP connect did not start")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled announce succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("UDP announce ignored cancellation")
	}
}

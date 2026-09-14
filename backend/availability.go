package main

// 找番资源可用性检测：对种子/磁力链接直接向其自带 tracker 发起 announce，
// 用返回的做种数判断资源当前是否有人做种，帮助在添加订阅前发现死种。
// 只读探测，不提交下载任务、不落库；UDP tracker 直连，HTTP tracker 走统一出站代理。

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	availabilityMaxURLs      = 20 // 与预览截断一致
	availabilityMaxTrackers  = 12 // 单个资源最多探测的 tracker 数
	availabilityURLWorkers   = 5  // 同时检测的资源数
	availabilityTrackerLimit = 8  // 单资源并发 tracker 数
)

// 超时用 var 以便测试注入更短预算。
var (
	availabilityBudget      = 15 * time.Second
	availabilityHTTPTimeout = 5 * time.Second
	availabilityUDPTTimeout = 4 * time.Second
)

type availabilityInput struct {
	URLs []string `json:"urls"`
}

type availabilityTracker struct {
	URL      string `json:"url"`
	Seeds    int    `json:"seeds"`
	Leechers int    `json:"leechers"`
	Error    string `json:"error,omitempty"`
}

type availabilityResult struct {
	URL           string                `json:"url"`
	Kind          string                `json:"kind"`
	InfoHash      string                `json:"info_hash,omitempty"`
	Seeds         int                   `json:"seeds"`
	Leechers      int                   `json:"leechers"`
	TrackersTotal int                   `json:"trackers_total"`
	TrackersAlive int                   `json:"trackers_alive"`
	Trackers      []availabilityTracker `json:"trackers,omitempty"`
	Verdict       string                `json:"verdict"`
	Error         string                `json:"error,omitempty"`
}

func discoverAvailabilityAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "方法不支持", http.StatusMethodNotAllowed)
		return
	}
	var in availabilityInput
	if json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in) != nil {
		http.Error(w, "请求参数无效", http.StatusBadRequest)
		return
	}
	urls := make([]string, 0, len(in.URLs))
	for _, raw := range in.URLs {
		if link := strings.TrimSpace(raw); link != "" {
			urls = append(urls, link)
		}
	}
	if len(urls) > availabilityMaxURLs {
		http.Error(w, "单次最多检测 20 条资源，请减少后重试", http.StatusBadRequest)
		return
	}
	results := make([]availabilityResult, len(urls))
	var wg sync.WaitGroup
	sem := make(chan struct{}, availabilityURLWorkers)
	for i, link := range urls {
		wg.Add(1)
		go func(i int, link string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-r.Context().Done():
				results[i] = availabilityResult{URL: link, Verdict: "unknown", Error: "检测已取消"}
				return
			}
			defer func() { <-sem }()
			results[i] = checkResourceAvailability(r.Context(), link)
		}(i, link)
	}
	wg.Wait()
	jsonOut(w, map[string]any{"results": results})
}

func checkResourceAvailability(ctx context.Context, raw string) availabilityResult {
	ctx, cancel := context.WithTimeout(ctx, availabilityBudget)
	defer cancel()
	res := availabilityResult{URL: raw}
	if err := ctx.Err(); err != nil {
		res.Verdict, res.Error = "unknown", "检测已取消或超时"
		return res
	}
	link := strings.TrimSpace(raw)
	lower := strings.ToLower(link)
	var infohash string
	var trackers []string
	left := int64(1)
	switch {
	case strings.HasPrefix(lower, "magnet:"):
		res.Kind = "magnet"
		infohash = magnetInfoHash(link)
		if infohash == "" {
			res.Verdict = "unknown"
			res.Error = "磁力链接缺少 BTIH 信息哈希，无法检测"
			return res
		}
		if u, err := url.Parse(link); err == nil {
			trackers = append(trackers, u.Query()["tr"]...)
		}
	case strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://"):
		res.Kind = "torrent"
		data, _, err := fetchTorrentAttemptContext(ctx, link)
		if err != nil {
			res.Verdict = "unknown"
			res.Error = err.Error()
			return res
		}
		infohash = torrentInfoHash(data)
		trackers = torrentTrackerList(data)
		if size := torrentTotalLength(data); size > 0 {
			left = size
		}
	default:
		res.Verdict = "unknown"
		res.Error = "仅支持 http(s) 种子链接或磁力链接"
		return res
	}
	res.InfoHash = infohash
	trackers = filterTrackerURLs(trackers)
	if len(trackers) == 0 {
		res.Verdict = "unknown"
		res.Error = "资源没有可检测的 tracker，无法判断做种状态"
		return res
	}
	res.TrackersTotal = len(trackers)

	perTracker := make([]availabilityTracker, len(trackers))
	sem := make(chan struct{}, availabilityTrackerLimit)
	var wg sync.WaitGroup
	for i, tracker := range trackers {
		wg.Add(1)
		go func(i int, tracker string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				perTracker[i] = availabilityTracker{URL: tracker, Error: "检测已取消或超时"}
				return
			}
			defer func() { <-sem }()
			seeds, leechers, err := announceTracker(ctx, tracker, infohash, left)
			t := availabilityTracker{URL: tracker, Seeds: seeds, Leechers: leechers}
			if err != nil {
				t.Error = err.Error()
			}
			perTracker[i] = t
		}(i, tracker)
	}
	wg.Wait()
	res.Trackers = perTracker
	for _, t := range perTracker {
		if t.Error == "" {
			res.TrackersAlive++
			if t.Seeds > res.Seeds {
				res.Seeds = t.Seeds
			}
			if t.Leechers > res.Leechers {
				res.Leechers = t.Leechers
			}
		}
	}
	switch {
	case res.Seeds > 0:
		res.Verdict = "available"
	case res.TrackersAlive > 0:
		res.Verdict = "no_seeds"
	default:
		res.Verdict = "unreachable"
		res.Error = "所有 tracker 均未返回有效的做种信息，无法判断（不代表没有做种者）"
	}
	return res
}

// filterTrackerURLs 去重、只保留 http(s)/udp tracker 并限制数量，保持原有顺序。
func filterTrackerURLs(raw []string) []string {
	seen := make(map[string]bool, len(raw))
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		link := strings.TrimSpace(item)
		if link == "" || seen[link] {
			continue
		}
		u, err := url.Parse(link)
		if err != nil {
			continue
		}
		scheme := strings.ToLower(u.Scheme)
		if scheme != "http" && scheme != "https" && scheme != "udp" {
			continue
		}
		seen[link] = true
		out = append(out, link)
		if len(out) >= availabilityMaxTrackers {
			break
		}
	}
	return out
}

// torrentTrackerList 提取根字典的 announce 与 announce-list（BEP 12）。
func torrentTrackerList(data []byte) []string {
	var out []string
	seen := make(map[string]bool)
	add := func(v []byte) {
		s := strings.TrimSpace(string(v))
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	i := 1
	for i < len(data) && data[i] != 'e' {
		key, n, ok := bencodeString(data[i:])
		if !ok {
			break
		}
		i += n
		start := i
		if !bencodeSkip(data, &i) {
			break
		}
		value := data[start:i]
		switch string(key) {
		case "announce":
			if len(value) > 0 && value[0] >= '0' && value[0] <= '9' {
				if s, _, ok := bencodeString(value); ok {
					add(s)
				}
			}
		case "announce-list":
			if len(value) == 0 || value[0] != 'l' {
				continue
			}
			j := 1
			for j < len(value) && value[j] != 'e' {
				elemStart := j
				if !bencodeSkip(value, &j) {
					break
				}
				elem := value[elemStart:j]
				if len(elem) > 0 && elem[0] == 'l' {
					k := 1
					for k < len(elem) && elem[k] != 'e' {
						strStart := k
						if !bencodeSkip(elem, &k) {
							break
						}
						if elem[strStart] >= '0' && elem[strStart] <= '9' {
							if s, _, ok := bencodeString(elem[strStart:k]); ok {
								add(s)
							}
						}
					}
				} else if len(elem) > 0 && elem[0] >= '0' && elem[0] <= '9' {
					if s, _, ok := bencodeString(elem); ok {
						add(s)
					}
				}
			}
		}
	}
	return out
}

// torrentTotalLength 汇总 info 字典的 length 或 files 内各文件长度。
func torrentTotalLength(data []byte) int64 {
	info, ok := bencodeFindInfo(data)
	if !ok {
		return 0
	}
	var total int64
	i := 1
	for i < len(info) && info[i] != 'e' {
		key, n, ok := bencodeString(info[i:])
		if !ok {
			return total
		}
		i += n
		start := i
		if !bencodeSkip(info, &i) {
			return total
		}
		value := info[start:i]
		switch string(key) {
		case "length":
			total += bencodeIntValue(value)
		case "files":
			if len(value) > 0 && value[0] == 'l' {
				j := 1
				for j < len(value) && value[j] != 'e' {
					fileStart := j
					if !bencodeSkip(value, &j) {
						return total
					}
					total += bencodeDictInt(value[fileStart:j], "length")
				}
			}
		}
	}
	return total
}

func bencodeIntValue(b []byte) int64 {
	if len(b) < 2 || b[0] != 'i' {
		return 0
	}
	v, err := strconv.ParseInt(string(b[1:len(b)-1]), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

func bencodeDictInt(dict []byte, key string) int64 {
	i := 1
	for i < len(dict) && dict[i] != 'e' {
		k, n, ok := bencodeString(dict[i:])
		if !ok {
			return 0
		}
		i += n
		start := i
		if !bencodeSkip(dict, &i) {
			return 0
		}
		if string(k) == key {
			return bencodeIntValue(dict[start:i])
		}
	}
	return 0
}

// announceTracker 向单个 tracker 发送 announce，返回 (做种数, 下载者数, 错误)。
func announceTracker(ctx context.Context, tracker, infohashHex string, left int64) (int, int, error) {
	u, err := url.Parse(tracker)
	if err != nil {
		return 0, 0, fmt.Errorf("tracker 地址无效")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return httpTrackerAnnounce(ctx, u, infohashHex, left)
	case "udp":
		return udpTrackerAnnounce(ctx, u, infohashHex, left)
	default:
		return 0, 0, fmt.Errorf("不支持的 tracker 协议")
	}
}

func httpTrackerAnnounce(ctx context.Context, u *url.URL, infohashHex string, left int64) (int, int, error) {
	infohash, err := hex.DecodeString(infohashHex)
	if err != nil || len(infohash) != 20 {
		return 0, 0, fmt.Errorf("信息哈希无效")
	}
	query := u.Query()
	query.Set("info_hash", string(infohash))
	query.Set("peer_id", string(randomPeerID()))
	query.Set("port", "6881")
	query.Set("uploaded", "0")
	query.Set("downloaded", "0")
	query.Set("left", strconv.FormatInt(left, 10))
	query.Set("compact", "1")
	query.Set("numwant", "10")
	query.Set("event", "started")
	target := *u
	target.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return 0, 0, fmt.Errorf("构造 announce 请求失败")
	}
	req.Header.Set("User-Agent", "auto-anime/1.0")
	client := *torrentClient
	client.Timeout = availabilityHTTPTimeout
	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, fmt.Errorf("连接失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return 0, 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	const responseLimit = 64 << 10
	body, err := io.ReadAll(io.LimitReader(resp.Body, responseLimit+1))
	if err != nil {
		return 0, 0, fmt.Errorf("读取响应失败")
	}
	if len(body) > responseLimit {
		return 0, 0, fmt.Errorf("tracker 响应超过 64 KB")
	}
	i := 0
	if len(body) == 0 || body[0] != 'd' || !bencodeSkip(body, &i) || i != len(body) {
		return 0, 0, fmt.Errorf("tracker 响应不是有效的 bencode 字典")
	}
	if value, ok := bencodeDictValue(body, "failure reason"); ok {
		reason, _, valid := bencodeString(value)
		if !valid {
			return 0, 0, fmt.Errorf("tracker 错误信息无效")
		}
		return 0, 0, fmt.Errorf("tracker 拒绝：%s", reason)
	}
	value, _ := bencodeDictValue(body, "complete")
	seeds, err := trackerCount(value)
	if err != nil {
		return 0, 0, fmt.Errorf("tracker 未返回有效的做种数")
	}
	leechers := 0
	// incomplete is optional; a missing complete cannot establish availability.
	if value, ok := bencodeDictValue(body, "incomplete"); ok {
		leechers, err = trackerCount(value)
		if err != nil {
			return 0, 0, fmt.Errorf("tracker 下载者数无效")
		}
	}
	return seeds, leechers, nil
}

func trackerCount(value []byte) (int, error) {
	if len(value) < 3 || value[0] != 'i' || value[len(value)-1] != 'e' {
		return 0, fmt.Errorf("计数不是整数")
	}
	n, err := strconv.ParseInt(string(value[1:len(value)-1]), 10, strconv.IntSize)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("计数超出范围")
	}
	return int(n), nil
}

// udpTrackerAnnounce 按 BEP 15 直连 UDP tracker（不经过代理）。
func udpTrackerAnnounce(ctx context.Context, u *url.URL, infohashHex string, left int64) (int, int, error) {
	infohash, err := hex.DecodeString(infohashHex)
	if err != nil || len(infohash) != 20 {
		return 0, 0, fmt.Errorf("信息哈希无效")
	}
	host := u.Host
	if u.Port() == "" {
		host = net.JoinHostPort(u.Hostname(), "80")
	}
	dialer := net.Dialer{Timeout: availabilityUDPTTimeout}
	conn, err := dialer.DialContext(ctx, "udp", host)
	if err != nil {
		return 0, 0, fmt.Errorf("连接失败")
	}
	defer conn.Close()
	stopCancel := context.AfterFunc(ctx, func() { conn.Close() })
	defer stopCancel()
	deadline := time.Now().Add(availabilityUDPTTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetDeadline(deadline)

	tid := randomUint32()
	connect := make([]byte, 16)
	binary.BigEndian.PutUint64(connect[0:8], 0x41727101980)
	binary.BigEndian.PutUint32(connect[8:12], 0)
	binary.BigEndian.PutUint32(connect[12:16], tid)
	if _, err := conn.Write(connect); err != nil {
		return 0, 0, fmt.Errorf("发送失败")
	}
	resp := make([]byte, 16)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return 0, 0, fmt.Errorf("无响应")
	}
	if action := binary.BigEndian.Uint32(resp[0:4]); action != 0 {
		return 0, 0, fmt.Errorf("connect 被拒绝")
	}
	connectionID := binary.BigEndian.Uint64(resp[8:16])

	tid2 := randomUint32()
	announce := make([]byte, 98)
	binary.BigEndian.PutUint64(announce[0:8], connectionID)
	binary.BigEndian.PutUint32(announce[8:12], 1)
	binary.BigEndian.PutUint32(announce[12:16], tid2)
	copy(announce[16:36], infohash)
	copy(announce[36:56], randomPeerID())
	binary.BigEndian.PutUint64(announce[56:64], 0)            // downloaded
	binary.BigEndian.PutUint64(announce[64:72], uint64(left)) // left
	binary.BigEndian.PutUint64(announce[72:80], 0)            // uploaded
	binary.BigEndian.PutUint32(announce[80:84], 2)            // event=started
	binary.BigEndian.PutUint32(announce[84:88], 0)            // ip
	binary.BigEndian.PutUint32(announce[88:92], tid)          // key
	binary.BigEndian.PutUint32(announce[92:96], 10)           // numwant
	binary.BigEndian.PutUint16(announce[96:98], 6881)
	if _, err := conn.Write(announce); err != nil {
		return 0, 0, fmt.Errorf("发送失败")
	}
	result := make([]byte, 20)
	if _, err := io.ReadFull(conn, result); err != nil {
		return 0, 0, fmt.Errorf("无响应")
	}
	switch action := binary.BigEndian.Uint32(result[0:4]); action {
	case 1:
		leechers := int(binary.BigEndian.Uint32(result[12:16]))
		seeds := int(binary.BigEndian.Uint32(result[16:20]))
		return seeds, leechers, nil
	case 3:
		message := bytes.TrimRight(result[8:], "\x00")
		if len(message) > 0 {
			return 0, 0, fmt.Errorf("tracker 拒绝：%s", message)
		}
		return 0, 0, fmt.Errorf("tracker 拒绝")
	default:
		return 0, 0, fmt.Errorf("异常响应")
	}
}

func randomPeerID() []byte {
	id := make([]byte, 20)
	if _, err := rand.Read(id); err != nil {
		for i := range id {
			id[i] = byte(i)
		}
	}
	return id
}

func randomUint32() uint32 {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return uint32(time.Now().UnixNano())
	}
	return binary.BigEndian.Uint32(b[:])
}

// bencodeDictValue reads a field from a dictionary already checked by bencodeSkip.
func bencodeDictValue(data []byte, key string) ([]byte, bool) {
	i := 1
	for i < len(data) && data[i] != 'e' {
		k, n, ok := bencodeString(data[i:])
		if !ok {
			return nil, false
		}
		i += n
		start := i
		if !bencodeSkip(data, &i) {
			return nil, false
		}
		if string(k) == key {
			return data[start:i], true
		}
	}
	return nil, false
}

package aggregate

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	titleparse "auto-anime/internal/title"
)

type Config struct {
	Token, JackettURL, JackettKey, BangumiURL string
	Indexers                                  []string
	Public                                    bool
}

type cacheEntry struct {
	result Result
	until  time.Time
}

type pending struct {
	done   chan struct{}
	result Result
	err    error
}

type Server struct {
	// shortcut: Shared token and in-memory quotas; add per-client credentials before multi-tenant operation.
	config Config
	client *http.Client
	slots  chan struct{}
	mu     sync.Mutex
	cache  map[int]cacheEntry
	active map[int]*pending
	minute int64
	count  int
}

var indexerID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

func New(config Config) (*Server, error) {
	if len(config.Token) < 32 || strings.TrimSpace(config.JackettKey) == "" {
		return nil, errors.New("SEARCH_API_TOKEN 至少 32 字符，且必须配置 JACKETT_API_KEY")
	}
	if config.BangumiURL == "" {
		config.BangumiURL = "https://api.bgm.tv"
	}
	for _, raw := range []string{config.JackettURL, config.BangumiURL} {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return nil, errors.New("上游地址必须为不含凭据、查询参数和片段的 HTTP(S) 根地址")
		}
	}
	if len(config.Indexers) == 0 || len(config.Indexers) > 8 {
		return nil, errors.New("请显式配置 1 至 8 个 JACKETT_INDEXERS")
	}
	seen := map[string]bool{}
	for _, id := range config.Indexers {
		if !indexerID.MatchString(id) || id == "all" || seen[id] {
			return nil, errors.New("索引器 ID 无效、重复或使用了 all")
		}
		seen[id] = true
	}
	config.Indexers = append([]string(nil), config.Indexers...)
	return &Server{
		config: config,
		// Direct by default; credentials must not follow an upstream redirect.
		client: &http.Client{Timeout: 12 * time.Second, Transport: &http.Transport{Proxy: nil, MaxConnsPerHost: 4, ResponseHeaderTimeout: 10 * time.Second},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		slots: make(chan struct{}, 4), cache: map[int]cacheEntry{}, active: map[int]*pending{},
	}, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "方法不支持", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path == "/healthz" {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
		return
	}
	expected, supplied := sha256.Sum256([]byte("Bearer "+s.config.Token)), sha256.Sum256([]byte(r.Header.Get("Authorization")))
	if !(s.config.Public && r.Header.Get("Authorization") == "") && subtle.ConstantTimeCompare(expected[:], supplied[:]) != 1 {
		http.Error(w, "认证失败", http.StatusUnauthorized)
		return
	}
	if r.URL.Path != "/v1/search" {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(r.URL.Query().Get("subject_id"), 10, 32)
	if err != nil || id <= 0 || len(r.URL.RawQuery) > 128 {
		http.Error(w, "Bangumi 条目 ID 无效", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	minute := time.Now().Unix() / 60
	if minute != s.minute {
		s.minute, s.count = minute, 0
	}
	s.count++
	limited := s.count > 60
	s.mu.Unlock()
	if limited {
		w.Header().Set("Retry-After", "60")
		http.Error(w, "搜索过于频繁，请稍后重试", http.StatusTooManyRequests)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	result, err := s.searchCached(ctx, int(id))
	if err != nil {
		http.Error(w, "聚合搜索暂不可用，请稍后重试", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) searchCached(ctx context.Context, id int) (Result, error) {
	s.mu.Lock()
	if entry, ok := s.cache[id]; ok && time.Now().Before(entry.until) {
		s.mu.Unlock()
		return entry.result, nil
	}
	if p := s.active[id]; p != nil {
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		case <-p.done:
			return p.result, p.err
		}
	}
	if len(s.active) >= 4 {
		s.mu.Unlock()
		return Result{}, errors.New("搜索并发已满")
	}
	p := &pending{done: make(chan struct{})}
	s.active[id] = p
	s.mu.Unlock()
	p.result, p.err = s.search(ctx, id)
	s.mu.Lock()
	if p.err == nil && len(p.result.Warnings) == 0 && ctx.Err() == nil {
		if len(s.cache) >= 128 {
			for key := range s.cache {
				delete(s.cache, key)
				break
			}
		}
		s.cache[id] = cacheEntry{result: p.result, until: time.Now().Add(5 * time.Minute)}
	}
	delete(s.active, id)
	close(p.done)
	s.mu.Unlock()
	return p.result, p.err
}

func (s *Server) get(ctx context.Context, address string, limit int64) ([]byte, error) {
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, errors.New("上游地址无效")
	}
	req.Header.Set("User-Agent", "Auto-Anime-Search/1.0")
	res, err := s.client.Do(req)
	if err != nil {
		return nil, errors.New("上游请求失败")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, errors.New("上游返回失败状态")
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil || int64(len(data)) > limit || !utf8.Valid(data) {
		return nil, errors.New("上游响应无效或过大")
	}
	return data, nil
}

type subject struct {
	ID      int    `json:"id"`
	Type    int    `json:"type"`
	Name    string `json:"name"`
	NameCN  string `json:"name_cn"`
	Infobox []struct {
		Key   string          `json:"key"`
		Value json.RawMessage `json:"value"`
	} `json:"infobox"`
}

func (s *Server) search(ctx context.Context, id int) (Result, error) {
	result := Result{SubjectID: id, Items: []Resource{}, Warnings: []string{}}
	data, err := s.get(ctx, strings.TrimRight(s.config.BangumiURL, "/")+"/v0/subjects/"+strconv.Itoa(id), 2<<20)
	if err != nil {
		return result, err
	}
	var anime subject
	if json.Unmarshal(data, &anime) != nil || anime.ID != id || anime.Type != 2 {
		return result, errors.New("不是有效的动画条目")
	}
	aliases := []string{anime.NameCN, anime.Name}
	for _, box := range anime.Infobox {
		if box.Key != "别名" {
			continue
		}
		var values []struct {
			Value string `json:"v"`
		}
		if json.Unmarshal(box.Value, &values) == nil {
			for _, v := range values {
				if len(aliases) < 16 {
					aliases = append(aliases, v.Value)
				}
			}
		}
	}
	for i, alias := range aliases {
		runes := []rune(alias)
		aliases[i] = string(runes[:min(len(runes), 200)])
	}
	queries := titleparse.SearchQueries(aliases)
	if len(queries) == 0 {
		return result, errors.New("条目缺少可搜索名称")
	}
	queries = queries[:min(len(queries), 3)]
	type sourceResult struct {
		items     []Resource
		failed    bool
		succeeded bool
		limited   bool
	}
	results := make([]sourceResult, len(s.config.Indexers))
	var wg sync.WaitGroup
	for i, source := range s.config.Indexers {
		wg.Add(1)
		go func(i int, source string) {
			defer wg.Done()
			for _, query := range queries {
				items, limited, err := s.searchSource(ctx, source, query, aliases)
				if err != nil {
					results[i].failed = true
					continue
				}
				results[i].succeeded = true
				results[i].limited = results[i].limited || limited
				results[i].items = append(results[i].items, items...)
			}
		}(i, source)
	}
	wg.Wait()
	seen := map[string]int{}
	succeeded := false
	for i, source := range results {
		succeeded = succeeded || source.succeeded
		if source.failed {
			result.Warnings = append(result.Warnings, s.config.Indexers[i]+" 部分查询失败，结果可能不完整")
		}
		if source.limited {
			result.Warnings = append(result.Warnings, s.config.Indexers[i]+" 达到单次读取上限，结果可能不完整")
		}
		for _, item := range source.items {
			key := item.Magnet
			if key == "" {
				key = item.DetailsURL + "\n" + item.Title
			}
			if previous, ok := seen[key]; ok {
				old := &result.Items[previous]
				if !contains(old.Sources, item.Sources[0]) {
					old.Sources = append(old.Sources, item.Sources[0])
				}
				continue
			}
			if len(result.Items) == 300 {
				result.Warnings = append(result.Warnings, "已达到 300 条结果上限")
				return result, nil
			}
			seen[key] = len(result.Items)
			result.Items = append(result.Items, item)
		}
	}
	if !succeeded {
		return result, errors.New("所有来源均失败")
	}
	return result, nil
}

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

type torznabItem struct {
	Title     string `xml:"title"`
	Comments  string `xml:"comments"`
	GUID      string `xml:"guid"`
	PubDate   string `xml:"pubDate"`
	Size      int64  `xml:"size"`
	Enclosure struct {
		URL string `xml:"url,attr"`
	} `xml:"enclosure"`
	Attrs []struct {
		Name  string `xml:"name,attr"`
		Value string `xml:"value,attr"`
	} `xml:"attr"`
}

func (s *Server) searchSource(ctx context.Context, source, query string, aliases []string) ([]Resource, bool, error) {
	u := strings.TrimRight(s.config.JackettURL, "/") + "/api/v2.0/indexers/" + source + "/results/torznab/api?" +
		url.Values{"apikey": {s.config.JackettKey}, "t": {"search"}, "q": {query}, "cat": {"5070"}, "limit": {"100"}}.Encode()
	data, err := s.get(ctx, u, 8<<20)
	if err != nil {
		return nil, false, err
	}
	var feed struct {
		XMLName xml.Name      `xml:"rss"`
		Items   []torznabItem `xml:"channel>item"`
	}
	if xml.Unmarshal(data, &feed) != nil || feed.XMLName.Local != "rss" {
		return nil, false, errors.New("无效的 Torznab 响应")
	}
	items := []Resource{}
	for _, raw := range feed.Items[:min(len(feed.Items), 100)] {
		if len(raw.Title) > 2048 || strings.TrimSpace(raw.Title) == "" {
			continue
		}
		info := titleparse.Extract(raw.Title)
		info.Season = titleparse.ResourceSeason(raw.Title)
		if info.Season < 0 {
			continue
		}
		match := false
		for _, name := range titleparse.QueryTitles(info.Title) {
			base, _, _ := titleparse.TitleParts(name)
			for _, alias := range aliases {
				other, _, _ := titleparse.TitleParts(alias)
				if base != "" && base == other {
					match = true
				}
			}
		}
		if !match {
			continue
		}
		// A shared series name does not prove a sequel/part belongs to this subject.
		item := Resource{Title: raw.Title, AnimeTitle: info.Title, Group: info.Group, Episode: info.Episode,
			Quality: info.Quality, Season: info.Season, Size: max(raw.Size, 0), Sources: []string{source}, NeedsReview: true}
		for _, attr := range raw.Attrs {
			switch attr.Name {
			case "infohash":
				if magnet := magnetForHash(attr.Value); magnet != "" {
					item.Magnet = magnet
				}
			case "magneturl":
				if magnet := publicMagnet(attr.Value); item.Magnet == "" {
					item.Magnet = magnet
				}
			case "size":
				if size, err := strconv.ParseInt(attr.Value, 10, 64); err == nil && size >= 0 {
					item.Size = size
				}
			}
		}
		if item.Magnet == "" {
			item.Magnet = publicMagnet(raw.Enclosure.URL)
		}
		for _, address := range []string{raw.Comments, raw.GUID} {
			if item.DetailsURL == "" {
				item.DetailsURL = s.publicDetails(address)
			}
		}
		if item.Magnet == "" && item.DetailsURL == "" {
			continue
		}
		if date, err := http.ParseTime(raw.PubDate); err == nil {
			item.Published = date.UTC().Format(time.RFC3339)
		}
		items = append(items, item)
	}
	return items, len(feed.Items) >= 100, nil
}

func magnetForHash(hash string) string {
	hash = strings.TrimSpace(hash)
	if len(hash) == 32 {
		data, err := base32.StdEncoding.DecodeString(strings.ToUpper(hash))
		if err != nil {
			return ""
		}
		hash = hex.EncodeToString(data)
	}
	data, err := hex.DecodeString(hash)
	if err != nil || len(data) != 20 {
		return ""
	}
	return "magnet:?xt=urn:btih:" + strings.ToLower(hash)
}

func publicMagnet(raw string) string {
	if len(raw) > 8192 {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "magnet" {
		return ""
	}
	for _, xt := range u.Query()["xt"] {
		if strings.HasPrefix(strings.ToLower(xt), "urn:btih:") {
			return magnetForHash(xt[len("urn:btih:"):])
		}
	}
	return ""
}

func (s *Server) publicDetails(raw string) string {
	if len(raw) > 2048 || strings.Contains(raw, s.config.JackettKey) || strings.Contains(raw, s.config.Token) {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil {
		return ""
	}
	upstream, _ := url.Parse(s.config.JackettURL)
	if strings.EqualFold(u.Hostname(), upstream.Hostname()) {
		return ""
	}
	if host := strings.ToLower(u.Hostname()); host == "localhost" || !strings.Contains(host, ".") {
		return ""
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && (!ip.IsGlobalUnicast() || ip.IsPrivate()) {
		return ""
	}
	if strings.Contains(u.Path, s.config.JackettKey) || strings.Contains(u.Path, s.config.Token) {
		return ""
	}
	// No upstream query credentials may escape through a resource-page link.
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return ""
	}
	for key, values := range query {
		if key != "id" && key != "tid" {
			return ""
		}
		for _, value := range values {
			if strings.Contains(value, s.config.JackettKey) || strings.Contains(value, s.config.Token) {
				return ""
			}
		}
	}
	u.Fragment = ""
	return u.String()
}

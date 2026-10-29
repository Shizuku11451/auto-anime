package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var bangumiWishMu sync.Mutex
var bangumiWishClient = &http.Client{Timeout: 15 * time.Second, Transport: &proxyTransport{}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

type bangumiWish struct {
	ID     int64   `json:"id"`
	Name   string  `json:"name"`
	NameCN string  `json:"name_cn"`
	Cover  string  `json:"cover,omitempty"`
	Type   int     `json:"type"`
	Rating float64 `json:"rating"`
	// Summary 来自收藏接口的简短简介，供未下载番剧的卡片预览；旧缓存没有该字段。
	Summary string `json:"summary,omitempty"`
	// BroadcastWeekday 来自条目放送星期或每日放送表；BroadcastEnd 是「播放结束」日期。
	// 两者由后台逐个读取条目补全，同步刚完成时可能为空。
	BroadcastWeekday int    `json:"broadcast_weekday,omitempty"`
	BroadcastEnd     string `json:"broadcast_end,omitempty"`
}
type bangumiWishState struct {
	Username string        `json:"username"`
	LastSync string        `json:"last_sync"`
	Error    string        `json:"error"`
	Items    []bangumiWish `json:"items"`
}

func bangumiWishRequest(ctx context.Context, token, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bangumiAPI+path, nil)
	if err != nil {
		return errors.New("Bangumi 请求配置无效")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", metaUserAgent)
	res, err := bangumiWishClient.Do(req)
	if err != nil {
		return errors.New("无法连接 Bangumi，请检查网络或代理")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("Bangumi 返回 HTTP %d，请检查令牌和网络", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, (2<<20)+1))
	if err != nil || len(data) > 2<<20 || json.Unmarshal(data, out) != nil {
		return errors.New("Bangumi 响应无效或过大")
	}
	return nil
}

func saveBangumiWishSettings(values map[string]string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for key, value := range values {
		if _, err = tx.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// bangumiWishSummaryLimit caps each cached summary so the settings row stays small.
const bangumiWishSummaryLimit = 300

// Serialize account changes and sync so old results cannot replace a new account.
func syncBangumiWishes(ctx context.Context) error {
	token, username := getSetting("bangumi_token"), getSetting("bangumi_username")
	if token == "" || username == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	items := []bangumiWish{}
	seen := map[int64]bool{}
	total := -1
	for offset := 0; ; offset += 50 {
		var page struct {
			Total *int `json:"total"`
			Data  []struct {
				SubjectID int64 `json:"subject_id"`
				Type      int   `json:"type"`
				Subject   struct {
					Name    string  `json:"name"`
					NameCN  string  `json:"name_cn"`
					Score   float64 `json:"score"`
					Summary string  `json:"short_summary"`
					Images  struct {
						Large  string `json:"large"`
						Medium string `json:"medium"`
					} `json:"images"`
				} `json:"subject"`
			} `json:"data"`
		}
		path := fmt.Sprintf("/v0/users/%s/collections?subject_type=2&limit=50&offset=%d", url.PathEscape(username), offset)
		if err := bangumiWishRequest(ctx, token, path, &page); err != nil {
			return err
		}
		if page.Total == nil || page.Data == nil || *page.Total < 0 || *page.Total > 10000 || (total >= 0 && total != *page.Total) || (len(page.Data) == 0 && offset < *page.Total) || len(page.Data) > 50 || offset+len(page.Data) > *page.Total {
			return errors.New("Bangumi 收藏分页不完整")
		}
		total = *page.Total
		for _, row := range page.Data {
			if err := ctx.Err(); err != nil {
				return err
			}
			if row.SubjectID <= 0 || row.Type < 1 || row.Type > 5 || (strings.TrimSpace(row.Subject.Name) == "" && strings.TrimSpace(row.Subject.NameCN) == "") || seen[row.SubjectID] {
				return errors.New("Bangumi 收藏数据不完整，请稍后重试")
			}
			seen[row.SubjectID] = true
			cover := row.Subject.Images.Large
			if cover == "" {
				cover = row.Subject.Images.Medium
			}
			score := row.Subject.Score
			if score < 0 || score > 10 {
				score = 0
			}
			summary := strings.TrimSpace(row.Subject.Summary)
			if r := []rune(summary); len(r) > bangumiWishSummaryLimit {
				summary = string(r[:bangumiWishSummaryLimit]) + "…"
			}
			items = append(items, bangumiWish{ID: row.SubjectID, Name: row.Subject.Name, NameCN: row.Subject.NameCN, Cover: cover, Type: row.Type, Rating: score, Summary: summary, BroadcastWeekday: calendarWeekdayLookup(ctx, row.SubjectID)})
		}
		if offset+len(page.Data) >= total {
			break
		}
		if len(page.Data) != 50 {
			return errors.New("Bangumi 收藏分页不完整")
		}
	}
	data, err := json.Marshal(items)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return saveBangumiWishSettings(map[string]string{"bangumi_wishes": string(data), "bangumi_last_sync": time.Now().UTC().Format(time.RFC3339), "bangumi_sync_error": ""})
}

func bangumiWishSync(ctx context.Context) error {
	bangumiWishMu.Lock()
	defer bangumiWishMu.Unlock()
	err := syncBangumiWishes(ctx)
	if err != nil {
		if saveErr := saveBangumiWishSettings(map[string]string{"bangumi_sync_error": err.Error()}); saveErr != nil {
			return errors.New("同步失败，且无法保存错误状态")
		}
	}
	return err
}

func bangumiWishAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet && !allowSetupWrite(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		bangumiWishMu.Lock()
		defer bangumiWishMu.Unlock()
		state := bangumiWishState{Username: getSetting("bangumi_username"), LastSync: getSetting("bangumi_last_sync"), Error: getSetting("bangumi_sync_error"), Items: []bangumiWish{}}
		if raw := getSetting("bangumi_wishes"); raw != "" {
			if json.Unmarshal([]byte(raw), &state.Items) != nil {
				http.Error(w, "读取收藏失败", 500)
				return
			}
		}
		// Caches written before collection sync contain only wishes, without a type.
		for i := range state.Items {
			if state.Items[i].Type == 0 {
				state.Items[i].Type = 1
			}
		}
		if err := applySubjectSchedules(state.Items); err != nil {
			http.Error(w, "读取收藏失败", 500)
			return
		}
		jsonOut(w, state)
	case http.MethodPost:
		var input struct {
			Token string `json:"token"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input) != nil {
			http.Error(w, "请求参数无效", 400)
			return
		}
		token := strings.TrimSpace(input.Token)
		if token == "" || strings.ContainsAny(token, "\r\n") {
			http.Error(w, "请输入有效令牌", 400)
			return
		}
		var user struct {
			Username string `json:"username"`
		}
		if err := bangumiWishRequest(r.Context(), token, "/v0/me", &user); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		if strings.TrimSpace(user.Username) == "" {
			http.Error(w, "Bangumi 账户信息无效", http.StatusBadGateway)
			return
		}
		bangumiWishMu.Lock()
		err := saveBangumiWishSettings(map[string]string{"bangumi_token": token, "bangumi_username": user.Username, "bangumi_wishes": "[]", "bangumi_last_sync": "", "bangumi_sync_error": ""})
		bangumiWishMu.Unlock()
		if err != nil {
			http.Error(w, "保存账户失败", 500)
			return
		}
		jsonOut(w, map[string]bool{"ok": true})
	case http.MethodDelete:
		bangumiWishMu.Lock()
		err := saveBangumiWishSettings(map[string]string{"bangumi_token": "", "bangumi_username": "", "bangumi_wishes": "[]", "bangumi_last_sync": "", "bangumi_sync_error": ""})
		bangumiWishMu.Unlock()
		if err != nil {
			http.Error(w, "解除连接失败", 500)
			return
		}
		jsonOut(w, map[string]bool{"ok": true})
	default:
		http.Error(w, "方法不支持", 405)
	}
}
func bangumiWishSyncAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "方法不支持", 405)
		return
	}
	if !allowSetupWrite(w, r) {
		return
	}
	if err := bangumiWishSync(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	go refreshWishSchedules(wishScheduleCtx)
	jsonOut(w, map[string]bool{"ok": true})
}
func startBangumiWishSync(ctx context.Context) {
	wishScheduleCtx = ctx
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for {
			_ = bangumiWishSync(ctx)
			refreshWishSchedules(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// wishScheduleCtx 随服务退出取消；手动同步触发的补全也使用它，而不是请求上下文。
var wishScheduleCtx = context.Background()
var wishScheduleMu sync.Mutex

// 补全间隔，避免短时间内大量请求 Bangumi；测试中缩短。
var wishScheduleDelay = 300 * time.Millisecond

// 已完结的放送信息基本不再变化，缓存 30 天；其余沿用放送星期的缓存规则。
func subjectScheduleFresh(checked string, weekday int, airEnd string) bool {
	if airEnd != "" && airEnd < time.Now().Format("2006-01-02") {
		at, err := time.Parse(time.RFC3339, checked)
		return err == nil && at.After(time.Now().Add(-30*24*time.Hour))
	}
	return broadcastWeekdayFresh(checked, weekday)
}

// refreshWishSchedules 逐个读取收藏条目，补全放送星期与播放结束日期。
// 单个条目失败只跳过，下次同步再试；同一时间只运行一轮。
func refreshWishSchedules(ctx context.Context) {
	if !bangumiEnabled || !wishScheduleMu.TryLock() {
		return
	}
	defer wishScheduleMu.Unlock()
	var items []bangumiWish
	bangumiWishMu.Lock()
	raw := getSetting("bangumi_wishes")
	bangumiWishMu.Unlock()
	if raw == "" || json.Unmarshal([]byte(raw), &items) != nil {
		return
	}
	failed := 0
	for _, item := range items {
		if ctx.Err() != nil {
			return
		}
		var weekday int
		var airEnd, checked string
		if err := db.QueryRow(`SELECT weekday,air_end,checked_at FROM subject_schedules WHERE subject_id=?`, item.ID).Scan(&weekday, &airEnd, &checked); err == nil && subjectScheduleFresh(checked, weekday, airEnd) {
			continue
		}
		subject, err := fetchBangumiSubjectContext(ctx, item.ID)
		if err != nil {
			if ctx.Err() == nil {
				failed++
			}
			continue
		}
		weekday = subjectBroadcastWeekday(subject)
		if weekday == 0 {
			weekday = calendarWeekdayLookup(ctx, item.ID)
		}
		if ctx.Err() != nil {
			return
		}
		if _, err := db.Exec(`INSERT INTO subject_schedules(subject_id,weekday,air_end,checked_at) VALUES(?,?,?,?)
			ON CONFLICT(subject_id) DO UPDATE SET weekday=excluded.weekday,air_end=excluded.air_end,checked_at=excluded.checked_at`,
			item.ID, weekday, subjectAirEnd(subject), time.Now().UTC().Format(time.RFC3339)); err != nil {
			log.Printf("收藏放送信息保存失败: %v", err)
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wishScheduleDelay):
		}
	}
	if failed > 0 {
		log.Printf("收藏放送信息有 %d 个条目获取失败，下次同步重试", failed)
	}
}

// applySubjectSchedules 把缓存的放送信息并入收藏列表；条目没有星期时保留同步时放送表给出的值。
func applySubjectSchedules(items []bangumiWish) error {
	if len(items) == 0 {
		return nil
	}
	index := make(map[int64]int, len(items))
	for i, item := range items {
		index[item.ID] = i
	}
	rows, err := db.Query(`SELECT subject_id,weekday,air_end FROM subject_schedules`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var weekday int
		var airEnd string
		if err := rows.Scan(&id, &weekday, &airEnd); err != nil {
			return err
		}
		i, ok := index[id]
		if !ok {
			continue
		}
		if weekday >= 1 && weekday <= 7 {
			items[i].BroadcastWeekday = weekday
		}
		items[i].BroadcastEnd = airEnd
	}
	return rows.Err()
}

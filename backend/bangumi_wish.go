package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var bangumiWishMu sync.Mutex
var bangumiWishClient = &http.Client{Timeout: 15 * time.Second, Transport: &proxyTransport{}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

type bangumiWish struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	NameCN string `json:"name_cn"`
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
				Subject   struct {
					Name   string `json:"name"`
					NameCN string `json:"name_cn"`
				} `json:"subject"`
			} `json:"data"`
		}
		path := fmt.Sprintf("/v0/users/%s/collections?subject_type=2&type=1&limit=50&offset=%d", url.PathEscape(username), offset)
		if err := bangumiWishRequest(ctx, token, path, &page); err != nil {
			return err
		}
		if page.Total == nil || page.Data == nil || *page.Total < 0 || *page.Total > 10000 || (total >= 0 && total != *page.Total) || (len(page.Data) == 0 && offset < *page.Total) || len(page.Data) > 50 || offset+len(page.Data) > *page.Total {
			return errors.New("Bangumi 收藏分页不完整")
		}
		total = *page.Total
		for _, row := range page.Data {
			if row.SubjectID <= 0 || (strings.TrimSpace(row.Subject.Name) == "" && strings.TrimSpace(row.Subject.NameCN) == "") || seen[row.SubjectID] {
				return errors.New("Bangumi 收藏数据不完整，请稍后重试")
			}
			seen[row.SubjectID] = true
			items = append(items, bangumiWish{row.SubjectID, row.Subject.Name, row.Subject.NameCN})
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
	jsonOut(w, map[string]bool{"ok": true})
}
func startBangumiWishSync(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for {
			_ = bangumiWishSync(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

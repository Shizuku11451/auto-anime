package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Browser sessions for the web login page. The account itself still comes
// from WEB_USER / WEB_PASSWORD (both must be set, matching the documented
// behavior); Basic Auth stays valid for scripts and API clients.

const sessionCookieName = "auto_anime_session"
const sessionLifetime = 30 * 24 * time.Hour

func authEnabled() bool { return webHash != "" || (webUser != "" && webPassword != "") }

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Only the token hash is stored, so a leaked database yields no usable session.
func createSession(user string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	_, err := db.Exec("INSERT INTO web_sessions(token_hash,user,created_at,expires_at) VALUES(?,?,?,?)",
		hashToken(token), user, time.Now().UTC().Format(time.RFC3339), time.Now().Add(sessionLifetime).UTC().Format(time.RFC3339))
	if err != nil {
		return "", err
	}
	return token, nil
}

// sessionUser validates the request's session cookie and returns the account,
// or "" when the cookie is missing, unknown, or expired.
func sessionUser(r *http.Request) string {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return ""
	}
	hash := hashToken(cookie.Value)
	var user, expires string
	if db.QueryRow("SELECT user,expires_at FROM web_sessions WHERE token_hash=?", hash).Scan(&user, &expires) != nil {
		return ""
	}
	deadline, err := time.Parse(time.RFC3339, expires)
	if err != nil || time.Now().After(deadline) {
		db.Exec("DELETE FROM web_sessions WHERE token_hash=?", hash)
		return ""
	}
	return user
}

func deleteSession(r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		db.Exec("DELETE FROM web_sessions WHERE token_hash=?", hashToken(cookie.Value))
	}
}

func cleanupSessions() {
	db.Exec("DELETE FROM web_sessions WHERE expires_at < ?", time.Now().UTC().Format(time.RFC3339))
}

// ---- login throttling: repeated failures from one source lock it out ----

type loginAttempt struct {
	count int
	until time.Time
}

type loginThrottle struct {
	mu      sync.Mutex
	fails   map[string]*loginAttempt
	max     int
	lockout time.Duration
}

func newLoginThrottle() *loginThrottle {
	return &loginThrottle{fails: map[string]*loginAttempt{}, max: 5, lockout: 15 * time.Minute}
}

var loginLimiter = newLoginThrottle()

func (t *loginThrottle) blocked(ip string) (time.Duration, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if a, ok := t.fails[ip]; ok && time.Now().Before(a.until) {
		return time.Until(a.until).Round(time.Second), true
	}
	return 0, false
}

func (t *loginThrottle) fail(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	a := t.fails[ip]
	if a == nil {
		a = &loginAttempt{}
		t.fails[ip] = a
	}
	a.count++
	if a.count >= t.max {
		a.count = 0
		a.until = time.Now().Add(t.lockout)
		log.Printf("登录连续失败，来自 %s 的请求被临时锁定 15 分钟", ip)
	}
}

func (t *loginThrottle) reset(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.fails, ip)
	if len(t.fails) > 4096 {
		t.fails = map[string]*loginAttempt{}
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ---- HTTP handlers ----

type loginInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "方法不支持", http.StatusMethodNotAllowed)
		return
	}
	var in loginInput
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in) != nil || in.Username == "" || in.Password == "" {
		http.Error(w, "请输入用户名和密码", http.StatusBadRequest)
		return
	}
	// Decode before taking the lock so a slow request body cannot block saves.
	// Do not issue a session against credentials being replaced by settings.
	settingsMu.Lock()
	defer settingsMu.Unlock()
	if !authEnabled() {
		http.Error(w, "未启用登录：需要同时设置 WEB_USER 和 WEB_PASSWORD", http.StatusBadRequest)
		return
	}
	ip := clientIP(r)
	if wait, locked := loginLimiter.blocked(ip); locked {
		http.Error(w, fmt.Sprintf("失败次数过多，请 %d 秒后再试", int(wait.Seconds())+1), http.StatusTooManyRequests)
		return
	}
	if !verifyWebCredentials(in.Username, in.Password) {
		loginLimiter.fail(ip)
		http.Error(w, "用户名或密码错误", http.StatusUnauthorized)
		return
	}
	loginLimiter.reset(ip)
	token, err := createSession(webUser)
	if err != nil {
		http.Error(w, "会话创建失败", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: token, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: int(sessionLifetime.Seconds()),
	})
	jsonOut(w, map[string]bool{"ok": true})
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "方法不支持", http.StatusMethodNotAllowed)
		return
	}
	deleteSession(r)
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	jsonOut(w, map[string]bool{"ok": true})
}

// safeNext only allows in-site paths so the login redirect cannot leave the app.
func safeNext(next string) string {
	if strings.HasPrefix(next, "/") && !strings.HasPrefix(next, "//") && !strings.Contains(next, `\`) {
		return next
	}
	return "/"
}

func loginPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "方法不支持", http.StatusMethodNotAllowed)
		return
	}
	if !authEnabled() {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	if sessionUser(r) != "" {
		http.Redirect(w, r, safeNext(r.URL.Query().Get("next")), http.StatusFound)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, "static/login.html")
}

// The login and setup pages, their assets, and the health probe stay
// reachable without a session; everything else requires one.
func isPublicPath(p string) bool {
	return p == "/api/health" || p == "/login" || p == "/api/login" ||
		p == "/setup" || p == "/api/setup" || p == "/api/setup/status" ||
		strings.HasPrefix(p, "/static/")
}

func checkBasicAuth(r *http.Request) bool {
	user, password, ok := r.BasicAuth()
	if !ok {
		return false
	}
	return verifyWebCredentials(user, password)
}

func auth(next http.Handler) http.Handler {
	// API requests get a plain 401 without WWW-Authenticate so the browser
	// shows our login page instead of the native Basic Auth prompt; page
	// requests are redirected to /login. A brand-new instance without any
	// credentials is routed to the first-run setup wizard instead.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authEnabled() {
			if setupRequired() && !isPublicPath(r.URL.Path) {
				if strings.HasPrefix(r.URL.Path, "/api/") {
					w.Header().Set("Cache-Control", "no-store")
					http.Error(w, "请先完成首次安装引导", http.StatusUnauthorized)
					return
				}
				http.Redirect(w, r, "/setup", http.StatusFound)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if sessionUser(r) != "" || checkBasicAuth(r) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.Error(w, "登录已过期，请重新登录", http.StatusUnauthorized)
			return
		}
		target := "/login"
		if r.URL.Path != "/" {
			target += "?next=" + url.QueryEscape(r.URL.Path)
		}
		http.Redirect(w, r, target, http.StatusFound)
	})
}

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func setWebAuth(t *testing.T, user, password string) {
	t.Helper()
	oldUser, oldPassword := webUser, webPassword
	webUser, webPassword = user, password
	loginLimiter = newLoginThrottle()
	t.Cleanup(func() { webUser, webPassword = oldUser, oldPassword })
}

func loginRequest(user, password string) *httptest.ResponseRecorder {
	body := fmt.Sprintf(`{"username":%q,"password":%q}`, user, password)
	recorder := httptest.NewRecorder()
	handleLogin(recorder, httptest.NewRequest("POST", "/api/login", strings.NewReader(body)))
	return recorder
}

func TestLoginCreatesTrackedSession(t *testing.T) {
	testDatabase(t)
	setWebAuth(t, "admin", "secret")

	recorder := loginRequest("admin", "wrong")
	if recorder.Code != http.StatusUnauthorized || !strings.Contains(recorder.Body.String(), "用户名或密码错误") {
		t.Fatalf("wrong password: %d %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Result().Cookies() != nil && len(recorder.Result().Cookies()) != 0 {
		t.Fatal("failed login must not set a session cookie")
	}

	recorder = loginRequest("admin", "secret")
	if recorder.Code != http.StatusOK {
		t.Fatalf("login: %d %s", recorder.Code, recorder.Body.String())
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Value == "" || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("session cookie: %#v", cookies)
	}
	token := cookies[0].Value
	var stored string
	if err := db.QueryRow("SELECT token_hash FROM web_sessions").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == token {
		t.Fatal("sessions must store the token hash, not the raw token")
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") })
	handler := auth(next)
	request := httptest.NewRequest("GET", "/api/items", nil)
	request.AddCookie(cookies[0])
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("session cookie did not authenticate: %d", recorder.Code)
	}

	// 登出删除服务端会话，重放旧 Cookie 立即失效。
	logoutRequest := httptest.NewRequest("POST", "/api/logout", nil)
	logoutRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	recorder = httptest.NewRecorder()
	handleLogout(recorder, logoutRequest)
	if recorder.Code != http.StatusOK {
		t.Fatalf("logout: %d", recorder.Code)
	}
	request = httptest.NewRequest("GET", "/api/items", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session still works: %d", recorder.Code)
	}
}

func TestAuthRedirectsPagesAndKeepsLoginAssetsPublic(t *testing.T) {
	testDatabase(t)
	setWebAuth(t, "admin", "secret")
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") })
	handler := auth(next)

	request := httptest.NewRequest("GET", "/some/page", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusFound || recorder.Header().Get("Location") != "/login?next=%2Fsome%2Fpage" {
		t.Fatalf("page redirect: %d %s", recorder.Code, recorder.Header().Get("Location"))
	}

	for _, path := range []string{"/login", "/static/app.js", "/static/style.css", "/api/health", "/api/login"} {
		recorder = httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest("GET", path, nil))
		if recorder.Code == http.StatusUnauthorized || recorder.Code == http.StatusFound {
			t.Fatalf("public path %s was blocked: %d", path, recorder.Code)
		}
	}
}

func TestLoginPageAndSafeNext(t *testing.T) {
	testDatabase(t)
	setWebAuth(t, "admin", "secret")

	recorder := httptest.NewRecorder()
	loginPage(recorder, httptest.NewRequest("GET", "/login", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "登录") {
		t.Fatalf("login page: %d %.80s", recorder.Code, recorder.Body.String())
	}

	for next, want := range map[string]string{"//evil.com": "/", "/\\evil": "/", "https://evil.com": "/", "/items": "/items", "": "/"} {
		if got := safeNext(next); got != want {
			t.Fatalf("safeNext(%q) = %q, want %q", next, got, want)
		}
	}

	// 已登录访问 /login 直接跳回站内目标，不再显示表单。
	token, err := createSession("admin")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/login?next=/items", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	recorder = httptest.NewRecorder()
	loginPage(recorder, request)
	if recorder.Code != http.StatusFound || recorder.Header().Get("Location") != "/items" {
		t.Fatalf("logged-in /login: %d %s", recorder.Code, recorder.Header().Get("Location"))
	}
}

func TestExpiredSessionRejectedAndRemoved(t *testing.T) {
	testDatabase(t)
	setWebAuth(t, "admin", "secret")
	token, err := createSession("admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE web_sessions SET expires_at=?", time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") })
	request := httptest.NewRequest("GET", "/api/items", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	recorder := httptest.NewRecorder()
	auth(next).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expired session accepted: %d", recorder.Code)
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM web_sessions").Scan(&count)
	if count != 0 {
		t.Fatal("expired session was not removed on access")
	}
}

func TestSessionCleanupKeepsLiveSessions(t *testing.T) {
	testDatabase(t)
	if _, err := db.Exec("INSERT INTO web_sessions(token_hash,user,created_at,expires_at) VALUES('dead','u','','2000-01-01T00:00:00Z')"); err != nil {
		t.Fatal(err)
	}
	if _, err := createSession("admin"); err != nil {
		t.Fatal(err)
	}
	cleanupSessions()
	var count int
	db.QueryRow("SELECT COUNT(*) FROM web_sessions").Scan(&count)
	if count != 1 {
		t.Fatalf("cleanup removed %d sessions, want only the expired one gone", count)
	}
}

func TestLoginThrottleBlocksRepeatedFailures(t *testing.T) {
	testDatabase(t)
	setWebAuth(t, "admin", "secret")
	for i := 0; i < 5; i++ {
		if recorder := loginRequest("admin", "bad"); recorder.Code != http.StatusUnauthorized {
			t.Fatalf("failure %d: %d", i, recorder.Code)
		}
	}
	recorder := loginRequest("admin", "secret")
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("correct password during lockout: %d %s", recorder.Code, recorder.Body.String())
	}
	ip := clientIP(httptest.NewRequest("POST", "/api/login", nil))
	loginLimiter.reset(ip)
	if recorder := loginRequest("admin", "secret"); recorder.Code != http.StatusOK {
		t.Fatalf("login after reset: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestLoginEndpointRequiresCompleteConfig(t *testing.T) {
	testDatabase(t)
	setWebAuth(t, "admin", "")
	recorder := loginRequest("admin", "secret")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("login without full config: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestStatusReportsAuthEnabled(t *testing.T) {
	testDatabase(t)
	var out struct {
		AuthEnabled bool `json:"auth_enabled"`
	}
	setWebAuth(t, "admin", "secret")
	recorder := httptest.NewRecorder()
	runtimeStatus(recorder, httptest.NewRequest("GET", "/api/status", nil))
	if err := json.Unmarshal(recorder.Body.Bytes(), &out); err != nil || !out.AuthEnabled {
		t.Fatalf("auth_enabled with config: %+v %v", out, err)
	}
	setWebAuth(t, "", "")
	recorder = httptest.NewRecorder()
	runtimeStatus(recorder, httptest.NewRequest("GET", "/api/status", nil))
	out.AuthEnabled = true
	if err := json.Unmarshal(recorder.Body.Bytes(), &out); err != nil || out.AuthEnabled {
		t.Fatalf("auth_enabled without config: %+v %v", out, err)
	}
}

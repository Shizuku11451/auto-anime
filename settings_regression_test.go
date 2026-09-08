package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// Restore the configuration and clients touched by these tests as well as the
// temporary DB. No workers run here and no requests leave loopback servers.
func settingsFixture(t *testing.T) {
	t.Helper()
	testDatabase(t)
	setWebAuth(t, "admin", "test-password")
	oldDone, oldHash, oldPoll := setupDone, webHash, pollMinutes
	oldProxy, oldHTTP, oldMedia, oldTorrent, oldMeta := proxyURL, httpClient.Transport, mediaClient.Transport, torrentClient.Transport, metaClient.Transport
	setupDone, webHash, pollMinutes = true, "", 30
	if err := applyProxyConfig(""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		setupDone, webHash, pollMinutes = oldDone, oldHash, oldPoll
		proxyURL, httpClient.Transport, mediaClient.Transport, torrentClient.Transport, metaClient.Transport = oldProxy, oldHTTP, oldMedia, oldTorrent, oldMeta
	})
}

func authenticatedStatus(cookie *http.Cookie, user, password string) int {
	req := httptest.NewRequest("GET", "/api/items", nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if user != "" {
		req.SetBasicAuth(user, password)
	}
	rec := httptest.NewRecorder()
	auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })).ServeHTTP(rec, req)
	return rec.Code
}

func TestSettingsOrdinarySavePreservesLoginAndSetup(t *testing.T) {
	settingsFixture(t)
	login := loginRequest("admin", "test-password")
	if login.Code != 200 {
		t.Fatal(login.Body.String())
	}
	cookie := login.Result().Cookies()[0]
	if rec := settingsPUT(t, `{"web_user":"admin","web_password":"","poll_minutes":10}`); rec.Code != 200 {
		t.Fatalf("ordinary save: %d %s", rec.Code, rec.Body.String())
	}
	if pollMinutes != 10 || !setupDone || authenticatedStatus(cookie, "", "") != 200 {
		t.Fatal("ordinary save changed login or setup state")
	}
	webUser, webPassword = "", ""
	if err := setSetting("setup_done", "1"); err != nil {
		t.Fatal(err)
	}
	if rec := settingsPUT(t, `{"web_user":"","poll_minutes":12}`); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if setupRequired() || authenticatedStatus(nil, "", "") != 200 {
		t.Fatal("skipped setup reopened")
	}
	setupDone = false
	loadSettings()
	if setupRequired() || pollMinutes != 12 {
		t.Fatal("skipped setup lost on restart")
	}
}

func TestSettingsRejectedUpdateIsAtomic(t *testing.T) {
	settingsFixture(t)
	for _, body := range []string{
		`{"web_user":"renamed","poll_minutes":11,"download_mode":"aria2"}`,
		`{"web_password":"short","poll_minutes":11,"download_mode":"aria2"}`,
		`{"proxy_url":"ftp://invalid","poll_minutes":11,"download_mode":"aria2"}`,
	} {
		if rec := settingsPUT(t, body); rec.Code != 400 {
			t.Fatalf("expected rejection: %s", rec.Body.String())
		}
		if pollMinutes != 30 || downloadMode != "direct" || len(storedSettings()) != 0 || !setupDone {
			t.Fatal("rejected request partially changed settings")
		}
	}
}

func TestSettingsSQLFailureRollsBackConfigurationAndSessions(t *testing.T) {
	for _, trigger := range []string{
		`CREATE TRIGGER fail_setting BEFORE INSERT ON settings WHEN NEW.key='web_user' BEGIN SELECT RAISE(ABORT,'injected settings failure'); END`,
		`CREATE TRIGGER fail_revoke BEFORE DELETE ON web_sessions BEGIN SELECT RAISE(ABORT,'injected revocation failure'); END`,
	} {
		t.Run(trigger, func(t *testing.T) {
			settingsFixture(t)
			login := loginRequest("admin", "test-password")
			if login.Code != 200 {
				t.Fatal(login.Body.String())
			}
			cookie := login.Result().Cookies()[0]
			before := storedSettings()
			if _, err := db.Exec(trigger); err != nil {
				t.Fatal(err)
			}
			rec := settingsPUT(t, `{"web_user":"renamed","web_password":"new-test-password","poll_minutes":11,"download_mode":"aria2"}`)
			if rec.Code == 200 {
				t.Fatal("SQL failure accepted")
			}
			if !reflect.DeepEqual(before, storedSettings()) || pollMinutes != 30 || downloadMode != "direct" || !verifyWebCredentials("admin", "test-password") || authenticatedStatus(cookie, "", "") != 200 {
				t.Fatal("SQL failure changed configuration or revoked the old session")
			}
		})
	}
}

func TestSetupSQLFailureLeavesFreshInstance(t *testing.T) {
	settingsFixture(t)
	webUser, webPassword, setupDone = "", "", false
	if _, err := db.Exec(`CREATE TRIGGER fail_setup BEFORE INSERT ON settings WHEN NEW.key='setup_done' BEGIN SELECT RAISE(ABORT,'injected setup failure'); END`); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	setupHandler(rec, httptest.NewRequest("POST", "/api/setup", strings.NewReader(`{"poll_minutes":11,"download_mode":"aria2","web_user":"boss","web_password":"test-password"}`)))
	if rec.Code == 200 || !setupRequired() || authEnabled() || pollMinutes != 30 || downloadMode != "direct" || len(storedSettings()) != 0 {
		t.Fatal("failed setup left partial configuration")
	}
}

func TestProxyRedactedRoundTripAndExplicitClear(t *testing.T) {
	settingsFixture(t)
	const original = "http://test-user:test-password@127.0.0.1:8099"
	if err := applyProxyConfig(original); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	settingsHandler(rec, httptest.NewRequest("GET", "/api/settings", nil))
	var current struct {
		ProxyURL    string `json:"proxy_url"`
		Credentials bool   `json:"proxy_credentials_set"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	if !current.Credentials || strings.Contains(rec.Body.String(), "test-password") || strings.Contains(rec.Body.String(), "test-user") {
		t.Fatal("proxy response exposes credentials or wrong flag")
	}
	if rec := settingsPUT(t, fmt.Sprintf(`{"proxy_url":%q,"poll_minutes":10}`, current.ProxyURL)); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if proxyURL.String() != original || getSetting("proxy_url") != original {
		t.Fatal("redacted save lost credentials")
	}
	loadSettings()
	if proxyURL.String() != original {
		t.Fatal("credentials lost on reload")
	}
	if rec := settingsPUT(t, `{"proxy_clear_credentials":true}`); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if proxyURL.User != nil || getSetting("proxy_url") != current.ProxyURL {
		t.Fatal("explicit clear did not persist")
	}
	if err := applyProxyConfig(original); err != nil {
		t.Fatal(err)
	}
	if rec := settingsPUT(t, `{"proxy_url":"http://127.0.0.1:8100"}`); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if proxyURL.User != nil {
		t.Fatal("credentials leaked to a changed endpoint")
	}
	if rec := settingsPUT(t, `{"proxy_url":""}`); rec.Code != 200 || proxyURL != nil {
		t.Fatal("direct mode failed")
	}
}

func TestCredentialRotationRevokesEverySession(t *testing.T) {
	for _, renamed := range []bool{false, true} {
		t.Run(fmt.Sprint(renamed), func(t *testing.T) {
			settingsFixture(t)
			var cookies []*http.Cookie
			for i := 0; i < 2; i++ {
				login := loginRequest("admin", "test-password")
				if login.Code != 200 {
					t.Fatal(login.Body.String())
				}
				cookies = append(cookies, login.Result().Cookies()[0])
			}
			user := "admin"
			if renamed {
				user = "renamed"
			}
			rec := settingsPUT(t, fmt.Sprintf(`{"web_user":%q,"web_password":"new-test-password"}`, user))
			if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"reauth_required":true`) {
				t.Fatalf("rotation: %s", rec.Body.String())
			}
			for _, cookie := range cookies {
				if authenticatedStatus(cookie, "", "") != 401 {
					t.Fatal("old session still valid")
				}
			}
			if authenticatedStatus(nil, "admin", "test-password") != 401 || authenticatedStatus(nil, user, "new-test-password") != 200 {
				t.Fatal("Basic Auth was not rotated")
			}
			login := loginRequest(user, "new-test-password")
			if login.Code != 200 || authenticatedStatus(login.Result().Cookies()[0], "", "") != 200 {
				t.Fatal("new login failed")
			}
		})
	}
}

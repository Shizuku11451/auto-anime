package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestQBittorrentLoginResponseCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		ok     bool
	}{
		{"legacy", http.StatusOK, "Ok.", true},
		{"legacy whitespace", http.StatusOK, "Ok.\n", true},
		{"v5.2", http.StatusNoContent, "", true},
		{"wrong password", http.StatusOK, "Fails.", false},
		{"empty legacy response", http.StatusOK, "", false},
		{"forbidden", http.StatusForbidden, "", false},
		{"server failure", http.StatusInternalServerError, "Ok.", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/api/v2/auth/login" {
					t.Errorf("unexpected login request: %s %s", r.Method, r.URL.Path)
				}
				http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test-session", Path: "/"})
				w.WriteHeader(tc.status)
				if tc.status != http.StatusNoContent {
					io.WriteString(w, tc.body)
				}
			}))
			defer server.Close()
			oldURL := qbitURL
			qbitURL = server.URL
			t.Cleanup(func() { qbitURL = oldURL })
			client, err := qbitLogin()
			if !tc.ok {
				if err == nil {
					t.Fatal("failed login was accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			u, _ := url.Parse(server.URL)
			cookies := client.http.Jar.Cookies(u)
			if len(cookies) != 1 || cookies[0].Name != "SID" || cookies[0].Value != "test-session" {
				t.Fatalf("login session not retained: %v", cookies)
			}
		})
	}
}

func TestQBittorrentCompletionWaitsForVerification(t *testing.T) {
	for _, state := range []string{"checkingUP", "checkingDL", "checkingResumeData", "moving", "metaDL", "downloading", "unknown"} {
		if qbitFinishedState(state) {
			t.Errorf("unfinished state accepted: %s", state)
		}
	}
	for _, state := range []string{"uploading", "completed", "pausedUP", "stoppedUP", "queuedUP", "stalledUP", "forcedUP"} {
		if !qbitFinishedState(state) {
			t.Errorf("complete state rejected: %s", state)
		}
	}
}

func TestQBittorrentAddResponseCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		ok     bool
	}{
		{"legacy", http.StatusOK, "Ok.", true},
		{"legacy empty", http.StatusNoContent, "", true},
		{"v5.2 added", http.StatusOK, `{"success_count":1,"failure_count":0,"pending_count":0,"added_torrent_ids":["0123456789abcdef0123456789abcdef01234567"]}`, true},
		{"v5.2 pending", http.StatusAccepted, `{"success_count":0,"failure_count":0,"pending_count":1,"added_torrent_ids":[]}`, true},
		{"rejected", http.StatusOK, `{"success_count":0,"failure_count":1,"pending_count":0}`, false},
		{"partial failure", http.StatusOK, `{"success_count":1,"failure_count":1,"pending_count":0}`, false},
		{"nothing accepted", http.StatusOK, `{"success_count":0,"failure_count":0,"pending_count":0}`, false},
		{"missing counts", http.StatusOK, `{"success_count":1}`, false},
		{"invalid counts", http.StatusOK, `{"success_count":1,"failure_count":0,"pending_count":-1}`, false},
		{"malformed", http.StatusOK, `{"success_count":`, false},
		{"legacy failed", http.StatusOK, "Fails.", false},
		{"conflict", http.StatusConflict, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v2/auth/login" {
					http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test-session", Path: "/"})
					w.WriteHeader(http.StatusNoContent)
					return
				}
				if r.Method != http.MethodPost || r.URL.Path != "/api/v2/torrents/add" {
					t.Errorf("unexpected addition request: %s %s", r.Method, r.URL.Path)
				}
				cookie, err := r.Cookie("SID")
				if err != nil || cookie.Value != "test-session" {
					t.Error("addition did not reuse login session")
				}
				w.WriteHeader(tc.status)
				if tc.status != http.StatusNoContent {
					io.WriteString(w, tc.body)
				}
			}))
			defer server.Close()
			oldURL := qbitURL
			qbitURL = server.URL
			t.Cleanup(func() { qbitURL = oldURL })
			client, err := qbitLogin()
			if err != nil {
				t.Fatalf("login: %v", err)
			}
			err = client.add("magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567", "auto-anime-1")
			if (err == nil) != tc.ok {
				t.Fatalf("addition success=%v, expected=%v: %v", err == nil, tc.ok, err)
			}
		})
	}
}

func TestQBittorrentRecheckDoesNotArchiveAndErrorFails(t *testing.T) {
	for _, state := range []string{"checkingUP", "error", "errored", "missingFiles"} {
		t.Run(state, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v2/torrents/info" {
					t.Errorf("unfinished task fetched files: %s", r.URL.Path)
				}
				io.WriteString(w, `[{"hash":"task","progress":1,"state":"`+state+`"}]`)
			}))
			defer server.Close()
			oldURL := qbitURL
			qbitURL = server.URL
			t.Cleanup(func() { qbitURL = oldURL })
			result, err := qbitPoll(&qbitClient{http: server.Client(), config: qbitConnection{URL: server.URL}}, taskRef{TaskID: "task"})
			if err != nil || len(result.Files) != 0 {
				t.Fatalf("unfinished result: %#v %v", result, err)
			}
			if (result.Fail != "") != (state != "checkingUP") {
				t.Fatalf("unexpected failure for %s: %q", state, result.Fail)
			}
		})
	}
}

func TestQBittorrentAsyncSubmissionSurvivesPollAndRestart(t *testing.T) {
	testDatabase(t)
	id := insertItem(t, "https://example.test/async.torrent")
	hash := "0123456789abcdef0123456789abcdef01234567"
	visible := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !visible {
			io.WriteString(w, `[]`)
			return
		}
		io.WriteString(w, `[{"hash":"`+hash+`","progress":0.25,"state":"downloading"}]`)
	}))
	defer server.Close()
	oldURL := qbitURL
	qbitURL = server.URL
	t.Cleanup(func() { qbitURL = oldURL })
	client := &qbitClient{http: server.Client(), config: qbitConnection{URL: server.URL}}
	for _, initialHash := range []string{hash, ""} {
		setSubmitted(id, "qbittorrent", initialHash, "已发送到 qBittorrent")
		refs, err := activeTaskRefs()
		if err != nil || len(refs) != 1 || refs[0].SubmittedAt == 0 {
			t.Fatalf("submission time not persisted: %#v %v", refs, err)
		}
		state, err := qbitPoll(client, refs[0])
		if err != nil || !state.Pending || state.Fail != "" {
			t.Fatalf("async addition failed prematurely: %#v %v", state, err)
		}
		applyTaskUpdateLocked(refs[0], state)
		if _, err := recoverDownloads(db); err != nil {
			t.Fatal(err)
		}
		var status string
		db.QueryRow("SELECT status FROM items WHERE id=?", id).Scan(&status)
		if status != "已发送到 qBittorrent" {
			t.Fatalf("pending submission lost across recovery: %s", status)
		}
		refs, err = activeTaskRefs()
		if err != nil || len(refs) != 1 {
			t.Fatalf("pending task missing after recovery: %#v %v", refs, err)
		}
		visible = true
		state, err = qbitPoll(client, refs[0])
		if err != nil || state.Pending || state.Progress != 25 {
			t.Fatalf("async addition did not appear: %#v %v", state, err)
		}
		applyTaskUpdateLocked(refs[0], state)
		db.QueryRow("SELECT status FROM items WHERE id=?", id).Scan(&status)
		if status != "下载中" {
			t.Fatalf("visible task not tracked: %s", status)
		}
		visible = false
		refs[0].SubmittedAt = time.Now().Add(-91 * time.Second).Unix()
		state, err = qbitPoll(client, refs[0])
		if err != nil || state.Pending || state.Fail == "" {
			t.Fatalf("missing task did not expire: %#v %v", state, err)
		}
	}
}

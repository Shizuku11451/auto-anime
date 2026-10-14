package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRuntimeCountsIncludeAllRowsAndNullStatus(t *testing.T) {
	testDatabase(t)
	for i := 0; i < 105; i++ {
		if _, err := db.Exec("INSERT INTO items(title,status) VALUES('动画','待下载')"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec("INSERT INTO items(title,status) VALUES('旧条目',NULL),('视频','已保存'),('BT','已发送到 qBittorrent')"); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	runtimeStatus(recorder, httptest.NewRequest("GET", "/api/status", nil))
	var result struct {
		Counts  map[string]int `json:"counts"`
		Mode    string         `json:"download_mode"`
		Minutes int            `json:"poll_minutes"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != 200 || result.Counts["待下载"] != 106 || result.Counts["已保存"] != 1 || result.Counts["已发送到 qBittorrent"] != 1 || result.Mode != downloadMode || result.Minutes != pollMinutes {
		t.Fatalf("unexpected status: %d %+v", recorder.Code, result)
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("status was cacheable")
	}
}

func TestRuntimeAuthenticationAndPublicHealth(t *testing.T) {
	testDatabase(t)
	resetEmbeddedQbitTest(t)
	t.Cleanup(func() { resetEmbeddedQbitTest(t) })
	oldUser, oldPassword := webUser, webPassword
	webUser, webPassword = "tester", "test-secret"
	defer func() { webUser, webPassword = oldUser, oldPassword }()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", healthStatus)
	mux.HandleFunc("/api/status", runtimeStatus)
	handler := auth(mux)
	for _, tc := range []struct {
		path  string
		login bool
		want  int
	}{
		{"/api/status", false, 401}, {"/api/status", true, 200}, {"/api/health", false, 200},
	} {
		req := httptest.NewRequest("GET", tc.path, nil)
		if tc.login {
			req.SetBasicAuth(webUser, webPassword)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("%s returned %d", tc.path, rec.Code)
		}
		if tc.path == "/api/health" {
			var fields map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &fields); err != nil {
				t.Fatal(err)
			}
			if len(fields) != 1 || fields["ok"] != true {
				t.Fatalf("public health leaked details: %v", fields)
			}
		}
	}
}

func TestRuntimeQueryFailureAndMethods(t *testing.T) {
	testDatabase(t)
	rec := httptest.NewRecorder()
	runtimeStatus(rec, httptest.NewRequest("POST", "/api/status", nil))
	if rec.Code != 405 {
		t.Fatalf("method: %d", rec.Code)
	}
	if _, err := db.Exec("DROP TABLE items"); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	runtimeStatus(rec, httptest.NewRequest("GET", "/api/status", nil))
	if rec.Code != 503 {
		t.Fatalf("failed query reported success: %d %s", rec.Code, rec.Body.String())
	}
}

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// Opt in with TEST_ARIA2_PATH; uses a private port, database and download root.
func TestEmbeddedRealSessionRestart(t *testing.T) {
	exe := os.Getenv("TEST_ARIA2_PATH")
	if exe == "" {
		t.Skip("set TEST_ARIA2_PATH for the real Aria2 lifecycle test")
	}
	testDatabase(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	t.Setenv("ARIA2_PATH", exe)
	t.Setenv("ARIA2_URL", "")
	t.Setenv("EMBEDDED_ARIA2_PORT", fmt.Sprint(port))
	oldEnabled, oldURL, oldToken, oldMode := embeddedAria2, aria2URL, aria2Token, downloadMode
	oldDir, oldLocal := aria2Dir, aria2LocalDir
	embeddedAria2, aria2URL, downloadMode = true, "", "direct"
	t.Cleanup(func() {
		stopEmbeddedAria2()
		embeddedAria2, aria2URL, aria2Token, downloadMode = oldEnabled, oldURL, oldToken, oldMode
		aria2Dir, aria2LocalDir = oldDir, oldLocal
	})
	if err := startEmbeddedAria2(); err != nil {
		t.Fatal(err)
	}
	var gid string
	if err := aria2RPC("aria2.addUri", []any{[]string{"http://127.0.0.1:1/resume.mp4"}, map[string]string{"pause": "true"}}, &gid); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO items(link,downloader,task_id,status) VALUES('http://127.0.0.1:1/resume.mp4','aria2',?,'下载中')", gid); err != nil {
		t.Fatal(err)
	}
	stopEmbeddedAria2()
	aria2URL = ""
	if err := startEmbeddedAria2(); err != nil {
		t.Fatal(err)
	}
	var state struct {
		GID string `json:"gid"`
	}
	if err := aria2RPC("aria2.tellStatus", []any{gid, []string{"gid"}}, &state); err != nil {
		t.Fatalf("task lost across restart: %v", err)
	}
	if state.GID != gid {
		t.Fatalf("GID changed: %q -> %q", gid, state.GID)
	}
}

func TestEmbeddedOrphansRPCAndOwnership(t *testing.T) {
	testDatabase(t)
	root := t.TempDir()
	if _, err := db.Exec("INSERT INTO items(link,downloader,task_id) VALUES('https://example.test/keep','aria2','keep')"); err != nil {
		t.Fatal(err)
	}
	oldURL, oldToken := aria2URL, aria2Token
	t.Cleanup(func() { aria2URL, aria2Token = oldURL, oldToken })
	removed := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string
			Params []json.RawMessage
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		switch req.Method {
		case "aria2.tellActive":
			if len(req.Params) != 1 {
				t.Errorf("tellActive expects keys only, got %s", req.Params)
				fmt.Fprint(w, `{"error":{"message":"bad params"}}`)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"result": []map[string]string{{"gid": "orphan", "dir": root}, {"gid": "keep", "dir": root}, {"gid": "foreign", "dir": filepath.Join(root, "other")}}})
		case "aria2.tellWaiting":
			if len(req.Params) != 3 {
				t.Errorf("invalid waiting params: %s", req.Params)
			}
			fmt.Fprint(w, `{"result":[]}`)
		case "aria2.remove":
			var gid string
			json.Unmarshal(req.Params[0], &gid)
			removed[gid] = true
			fmt.Fprint(w, `{"result":"ok"}`)
		default:
			t.Errorf("unexpected RPC %s", req.Method)
		}
	}))
	defer server.Close()
	aria2URL, aria2Token = server.URL, ""
	cleanupEmbeddedAria2Orphans(root)
	if !removed["orphan"] || len(removed) != 1 {
		t.Fatalf("removed = %v", removed)
	}
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func useAria2RPCServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	oldURL, oldToken := aria2URL, aria2Token
	aria2URL, aria2Token = server.URL, ""
	t.Cleanup(func() {
		server.Close()
		aria2URL, aria2Token = oldURL, oldToken
	})
	return server
}

func aria2RequestedGID(t *testing.T, r *http.Request) string {
	t.Helper()
	var req struct {
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Method != "aria2.tellStatus" || len(req.Params) != 2 {
		t.Errorf("unexpected RPC request: %+v, %v", req, err)
		return ""
	}
	var gid string
	if err := json.Unmarshal(req.Params[0], &gid); err != nil {
		t.Error(err)
	}
	return gid
}

func TestAria2MagnetFollowsPayloadAndArchives(t *testing.T) {
	testDatabase(t)
	local := t.TempDir()
	oldRemote, oldLocal := aria2Dir, aria2LocalDir
	aria2Dir, aria2LocalDir = "/downloads", local
	t.Cleanup(func() { aria2Dir, aria2LocalDir = oldRemote, oldLocal })
	media := filepath.Join(local, "episode.mp4")
	if err := os.WriteFile(media, []byte("bt-media"), 0600); err != nil {
		t.Fatal(err)
	}
	var phase atomic.Int32
	useAria2RPCServer(t, func(w http.ResponseWriter, r *http.Request) {
		gid := aria2RequestedGID(t, r)
		switch gid {
		case "metadata":
			if phase.Load() == 0 {
				fmt.Fprint(w, `{"result":{"status":"active","completedLength":"50","totalLength":"50","bittorrent":{},"files":[{"path":"[METADATA]","length":"50"}]}}`)
			} else if phase.Load() == 1 {
				fmt.Fprint(w, `{"result":{"status":"complete","completedLength":"50","totalLength":"50","followedBy":["payload"],"files":[{"path":"[METADATA]","length":"50"}]}}`)
			} else {
				t.Error("queried metadata after the payload GID was persisted")
				fmt.Fprint(w, `{"error":{"message":"GID metadata is not found"}}`)
			}
		case "payload":
			if phase.Load() == 1 {
				fmt.Fprint(w, `{"result":{"status":"active","completedLength":"4","totalLength":"8","downloadSpeed":"128"}}`)
			} else {
				fmt.Fprint(w, `{"result":{"status":"active","completedLength":"8","totalLength":"8","dir":"/downloads","bittorrent":{"info":{"name":"episode.mp4"}},"files":[{"path":"/downloads/episode.mp4","length":"8"}]}}`)
			}
		default:
			t.Errorf("unexpected GID %q", gid)
		}
	})
	id := insertItem(t, "magnet:?xt=urn:btih:0123456789012345678901234567890123456789")
	setSubmitted(id, "aria2", "metadata", "下载中")
	if _, err := db.Exec("UPDATE items SET task_error='归档失败：旧元数据路径' WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	pollDownloaderTasks()
	var status, gid, taskError, archived string
	var progress int
	read := func() {
		t.Helper()
		if err := db.QueryRow("SELECT status,task_id,task_error,progress,COALESCE(file_path,'') FROM items WHERE id=?", id).Scan(&status, &gid, &taskError, &progress, &archived); err != nil {
			t.Fatal(err)
		}
	}
	read()
	if status != "下载中" || gid != "metadata" || progress != 0 || taskError != "" || archived != "" {
		t.Fatalf("metadata treated as payload: %s %s %d %q %q", status, gid, progress, taskError, archived)
	}
	phase.Store(1)
	pollDownloaderTasks()
	read()
	if gid != "payload" || status != "下载中" || progress != 50 || taskError != "" || archived != "" {
		t.Fatalf("payload tracking: %s %s %d %q %q", status, gid, progress, taskError, archived)
	}
	// Restart recovery keeps the payload identity and does not resubmit it.
	if ids, err := recoverDownloads(db); err != nil || len(ids) != 0 {
		t.Fatalf("recovery resubmitted a tracked payload: %v %v", ids, err)
	}
	phase.Store(2)
	pollDownloaderTasks()
	read()
	if status != "已保存" || gid != "payload" || progress != 100 || taskError != "" {
		t.Fatalf("payload completion: %s %s %d %q", status, gid, progress, taskError)
	}
	for _, path := range []string{media, archived} {
		if data, err := os.ReadFile(path); err != nil || string(data) != "bt-media" {
			t.Fatalf("payload bytes at %q: %q, %v", path, data, err)
		}
	}
}

func TestAria2MagnetPayloadFailuresAreReported(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"error", `{"result":{"status":"error","errorMessage":"existing file needs integrity check"}}`, "existing file needs integrity check"},
		{"removed", `{"result":{"status":"removed"}}`, "被移除"},
		{"missing", `{"error":{"message":"Invalid GID payload"}}`, "不存在"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testDatabase(t)
			useAria2RPCServer(t, func(w http.ResponseWriter, r *http.Request) {
				if aria2RequestedGID(t, r) == "metadata" {
					fmt.Fprint(w, `{"result":{"status":"complete","followedBy":["payload"],"files":[{"path":"[METADATA]","length":"50"}]}}`)
				} else {
					fmt.Fprint(w, tc.body)
				}
			})
			id := insertItem(t, "magnet:?xt=urn:btih:0123456789012345678901234567890123456789")
			setSubmitted(id, "aria2", "metadata", "下载中")
			pollDownloaderTasks()
			var status, gid, taskError string
			if err := db.QueryRow("SELECT status,task_id,task_error FROM items WHERE id=?", id).Scan(&status, &gid, &taskError); err != nil {
				t.Fatal(err)
			}
			if status != "下载失败" || gid != "payload" || !strings.Contains(taskError, tc.want) {
				t.Fatalf("payload failure: %s %s %q", status, gid, taskError)
			}
		})
	}
}

func TestAria2MagnetTimeoutPreservesTaskAndRecovers(t *testing.T) {
	testDatabase(t)
	var healthy atomic.Bool
	server := useAria2RPCServer(t, func(w http.ResponseWriter, r *http.Request) {
		if aria2RequestedGID(t, r) == "metadata" {
			fmt.Fprint(w, `{"result":{"status":"complete","followedBy":["payload"]}}`)
			return
		}
		if !healthy.Load() {
			<-r.Context().Done()
			return
		}
		fmt.Fprint(w, `{"result":{"status":"active","completedLength":"13","totalLength":"100"}}`)
	})
	oldClient := downloaderClient
	downloaderClient = &http.Client{Timeout: 100 * time.Millisecond, Transport: server.Client().Transport}
	t.Cleanup(func() { downloaderClient = oldClient })
	id := insertItem(t, "magnet:?xt=urn:btih:0123456789012345678901234567890123456789")
	setSubmitted(id, "aria2", "metadata", "下载中")
	if _, err := db.Exec("UPDATE items SET progress=13 WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	setTransfer(id, 2048, 13, time.Minute)
	pollDownloaderTasks()
	var status, gid, taskError string
	var progress int
	read := func() {
		t.Helper()
		if err := db.QueryRow("SELECT status,task_id,task_error,progress FROM items WHERE id=?", id).Scan(&status, &gid, &taskError, &progress); err != nil {
			t.Fatal(err)
		}
	}
	read()
	if status != "下载中" || gid != "metadata" || progress != 13 || !strings.Contains(taskError, "超时") {
		t.Fatalf("timeout lost task: %s %s %d %q", status, gid, progress, taskError)
	}
	if speed, _ := transferSnapshot(id, status, progress); speed != 0 {
		t.Fatalf("timeout retained stale speed: %d", speed)
	}
	healthy.Store(true)
	pollDownloaderTasks()
	read()
	if status != "下载中" || gid != "payload" || progress != 13 || taskError != "" {
		t.Fatalf("recovery retained error with unchanged progress: %s %s %d %q", status, gid, progress, taskError)
	}
}

func TestAria2MagnetRejectsAmbiguousAndCyclicChains(t *testing.T) {
	for _, tc := range []struct {
		name string
		next func(string) []string
	}{
		{"self", func(gid string) []string { return []string{gid} }},
		{"cycle", func(gid string) []string {
			if gid == "root" {
				return []string{"child"}
			}
			return []string{"root"}
		}},
		{"deep", func(gid string) []string { return []string{gid + "x"} }},
		{"multiple", func(string) []string { return []string{"one", "two"} }},
		{"empty", func(string) []string { return []string{""} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			useAria2RPCServer(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				gid := aria2RequestedGID(t, r)
				json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"status": "complete", "followedBy": tc.next(gid)}})
			})
			state, err := aria2Poll("root")
			if err == nil || len(state.Files) != 0 || calls.Load() > 8 {
				t.Fatalf("invalid chain accepted: %+v err=%v calls=%d", state, err, calls.Load())
			}
		})
	}
}

func TestAria2MagnetLateResultsCannotReplaceNewTask(t *testing.T) {
	testDatabase(t)
	id := insertItem(t, "magnet:?xt=urn:btih:0123456789012345678901234567890123456789")
	setSubmitted(id, "aria2", "replacement", "下载中")
	old := taskRef{ID: id, Downloader: "aria2", TaskID: "metadata"}
	applyTaskUpdate(old, taskState{TaskID: "payload", Fail: "old failure"})
	recordTaskQueryError(old, context.DeadlineExceeded)
	var status, gid, taskError string
	if err := db.QueryRow("SELECT status,task_id,task_error FROM items WHERE id=?", id).Scan(&status, &gid, &taskError); err != nil {
		t.Fatal(err)
	}
	if status != "下载中" || gid != "replacement" || taskError != "" {
		t.Fatalf("late result changed new task: %s %s %q", status, gid, taskError)
	}
}

func TestAria2MagnetFailedHandoffDoesNotArchive(t *testing.T) {
	testDatabase(t)
	id := insertItem(t, "magnet:?xt=urn:btih:0123456789012345678901234567890123456789")
	setSubmitted(id, "aria2", "metadata", "下载中")
	if _, err := db.Exec("CREATE TRIGGER reject_handoff BEFORE UPDATE OF task_id ON items BEGIN SELECT RAISE(ABORT,'handoff failed'); END"); err != nil {
		t.Fatal(err)
	}
	media := filepath.Join(t.TempDir(), "episode.mp4")
	if err := os.WriteFile(media, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	applyTaskUpdate(taskRef{ID: id, Downloader: "aria2", TaskID: "metadata"}, taskState{TaskID: "payload", Progress: 100, Files: []archiveFile{{Src: media, Rel: "episode.mp4"}}})
	var status, gid, archived string
	if err := db.QueryRow("SELECT status,task_id,COALESCE(file_path,'') FROM items WHERE id=?", id).Scan(&status, &gid, &archived); err != nil {
		t.Fatal(err)
	}
	if status != "下载中" || gid != "metadata" || archived != "" {
		t.Fatalf("failed handoff applied completion: %s %s %q", status, gid, archived)
	}
	entries, err := os.ReadDir(downloadDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed handoff wrote archive: %v %v", entries, err)
	}
}

func TestAria2WaitsForMetadataAndIntegrityCheck(t *testing.T) {
	for _, extra := range []string{
		`"files":[{"path":"[METADATA]","length":"10"}]`,
		`"files":[{"path":"[METADATA]0123456789abcdef","length":"10"}]`,
		`"files":[{"path":"[MEMORY][METADATA]","length":"10"}]`,
		`"verifyIntegrityPending":"true","files":[{"path":"/downloads/episode.mp4","length":"10"}]`,
		`"verifiedLength":"5","files":[{"path":"/downloads/episode.mp4","length":"10"}]`,
	} {
		for _, status := range []string{"active", "complete"} {
			var st aria2Status
			body := fmt.Sprintf(`{"status":%q,"completedLength":"10","totalLength":"10","bittorrent":{},%s}`, status, extra)
			if err := json.Unmarshal([]byte(body), &st); err != nil {
				t.Fatal(err)
			}
			if state := aria2PayloadState(&st); len(state.Files) != 0 || state.Fail != "" || state.Progress != 0 {
				t.Fatalf("unverified payload completed: %s %+v", body, state)
			}
		}
	}
}

func TestAria2BTSubmissionChecksExistingFiles(t *testing.T) {
	testDatabase(t)
	var submitted atomic.Int32
	server := useAria2RPCServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/episode.torrent" {
			fmt.Fprint(w, "d4:infod6:lengthi5e4:name3:abcee")
			return
		}
		var req struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		var options map[string]string
		if err := json.Unmarshal(req.Params[len(req.Params)-1], &options); err != nil {
			t.Error(err)
		}
		if options["check-integrity"] != "true" || options["continue"] != "true" || options["allow-overwrite"] == "true" {
			t.Errorf("unsafe BT resume options: %v", options)
		}
		submitted.Add(1)
		fmt.Fprint(w, `{"result":"new-task"}`)
	})
	for _, link := range []string{server.URL + "/episode.torrent", "magnet:?xt=urn:btih:0123456789012345678901234567890123456789"} {
		if gid, err := aria2Add(link); err != nil || gid != "new-task" {
			t.Fatalf("submit: gid=%s err=%v", gid, err)
		}
	}
	if submitted.Load() != 2 {
		t.Fatalf("submitted %d BT tasks", submitted.Load())
	}
}

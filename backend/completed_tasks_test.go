package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompletedQbitTaskPreservesFilesAndWaitsForSeedGoal(t *testing.T) {
	testDatabase(t)
	hash := strings.Repeat("a", 40)
	state, ratio := "uploading", 0.5
	removed := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			fmt.Fprint(w, "Ok.")
		case "/api/v2/torrents/info":
			fmt.Fprintf(w, `[{"hash":%q,"progress":1,"state":%q,"ratio":%v,"ratio_limit":1,"seeding_time_limit":-1}]`, hash, state, ratio)
		case "/api/v2/torrents/delete":
			r.ParseForm()
			if r.Form.Get("hashes") != hash || r.Form.Get("deleteFiles") != "false" {
				t.Error("unsafe deletion", r.Form)
			}
			removed++
		default:
			t.Error("unexpected request", r.URL.Path)
		}
	}))
	defer s.Close()
	old := qbitURL
	qbitURL = s.URL
	defer func() { qbitURL = old }()
	archive := filepath.Join(t.TempDir(), "saved.mp4")
	if err := os.WriteFile(archive, []byte("media"), 0600); err != nil {
		t.Fatal(err)
	}
	id := insertItem(t, "https://example.test/complete")
	if _, err := db.Exec("UPDATE items SET downloader='qbittorrent',task_id=?,task_submitted_at=1,status='已保存',file_path=? WHERE id=?", hash, archive, id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO completed_tasks VALUES(?,?,?,?,?)", id, "qbittorrent", hash, 1, cleanupConnection("qbittorrent")); err != nil {
		t.Fatal(err)
	}
	// Completed tasks remain protected while their archived copy is unavailable.
	if _, err := db.Exec("UPDATE items SET file_path=? WHERE id=?", archive+".missing", id); err != nil {
		t.Fatal(err)
	}
	processCompletedTasks()
	if _, err := db.Exec("UPDATE items SET file_path=? WHERE id=?", archive, id); err != nil {
		t.Fatal(err)
	}
	processCompletedTasks()
	state = "pausedUP" // A manual pause before the ratio target must not delete anything.
	processCompletedTasks()
	if removed != 0 {
		t.Fatal("removed before seed goal")
	}
	ratio = 1
	processCompletedTasks()
	processCompletedTasks()
	if removed != 1 {
		t.Fatalf("deletions: %d", removed)
	}
	if data, err := os.ReadFile(archive); err != nil || string(data) != "media" {
		t.Fatal("archive changed", err)
	}
}

func TestCompletedTaskReceiptCreatedOnlyOnArchiveCommit(t *testing.T) {
	testDatabase(t)
	old := keepSourceFiles
	keepSourceFiles = true
	defer func() { keepSourceFiles = old }()
	id := insertItem(t, "https://example.test/receipt")
	if _, err := db.Exec("UPDATE items SET downloader='aria2',task_id='0123456789abcdef',status='下载中',task_submitted_at=123 WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	if err := prepareArchiveCommit(id, "saved.mp4", "marker", "now"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM completed_tasks").Scan(&count); err != nil || count != 0 {
		t.Fatalf("before commit: %d %v", count, err)
	}
	if err := commitSavedArchive(id, "saved.mp4"); err != nil {
		t.Fatal(err)
	}
	var got taskRef
	var connection string
	if err := db.QueryRow("SELECT item_id,downloader,task_id,submitted_at,connection FROM completed_tasks").Scan(&got.ID, &got.Downloader, &got.TaskID, &got.SubmittedAt, &connection); err != nil {
		t.Fatal(err)
	}
	if got != (taskRef{ID: id, Downloader: "aria2", TaskID: "0123456789abcdef", SubmittedAt: 123}) || connection != cleanupConnection("aria2") {
		t.Fatalf("receipt: %+v", got)
	}
	if _, err := db.Exec("UPDATE items SET task_id='fedcba9876543210' WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	processCompletedTasks()
	if err := db.QueryRow("SELECT COUNT(*) FROM completed_tasks").Scan(&count); err != nil || count != 0 {
		t.Fatalf("stale receipt: %d %v", count, err)
	}
}

func TestFinishedAria2OnlyRemovesCompletedPayloadHistory(t *testing.T) {
	testDatabase(t)
	status := "active"
	removed := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		switch in.Method {
		case "aria2.tellStatus":
			fmt.Fprintf(w, `{"result":{"status":%q}}`, status)
		case "aria2.removeDownloadResult":
			removed++
			fmt.Fprint(w, `{"result":"OK"}`)
		default:
			t.Error("unsafe RPC", in.Method)
		}
	}))
	defer s.Close()
	aria2URL = s.URL
	ref := taskRef{TaskID: "0123456789abcdef", Downloader: "aria2"}
	if done, err := removeFinishedTask(ref); done || err != nil {
		t.Fatalf("active: %v %v", done, err)
	}
	status = "paused"
	if done, err := removeFinishedTask(ref); done || err != nil {
		t.Fatalf("paused: %v %v", done, err)
	}
	status = "complete"
	if done, err := removeFinishedTask(ref); !done || err != nil {
		t.Fatalf("complete: %v %v", done, err)
	}
	if removed != 1 {
		t.Fatal(removed)
	}
}

func TestFinishedTaskRejectsGlobalIdentifiers(t *testing.T) {
	for _, downloader := range []string{"native", "aria2", "qbittorrent", "transmission"} {
		for _, id := range []string{"", "all", "-1", "1|2"} {
			if done, err := removeFinishedTask(taskRef{Downloader: downloader, TaskID: id}); done || err == nil {
				t.Errorf("accepted %s %q", downloader, id)
			}
		}
	}
}

func TestFinishedTransmissionRequiresSeedCompletion(t *testing.T) {
	testDatabase(t)
	old := transmissionURL
	defer func() { transmissionURL = old }()
	finished, status, removed := false, 6, 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Method    string                     `json:"method"`
			Arguments map[string]json.RawMessage `json:"arguments"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Error(err)
		}
		switch in.Method {
		case "torrent-get":
			fmt.Fprintf(w, `{"result":"success","arguments":{"torrents":[{"id":7,"status":%d,"percentDone":1,"isFinished":%t}]}}`, status, finished)
		case "torrent-remove":
			if string(in.Arguments["ids"]) != "[7]" || string(in.Arguments["delete-local-data"]) != "false" {
				t.Error("unsafe remove", in.Arguments)
			}
			removed++
			fmt.Fprint(w, `{"result":"success"}`)
		default:
			t.Error(in.Method)
		}
	}))
	defer s.Close()
	transmissionURL = s.URL
	ref := taskRef{Downloader: "transmission", TaskID: "7"}
	for _, tc := range []struct {
		finished bool
		status   int
		want     bool
	}{{false, 6, false}, {false, 0, false}, {true, 6, false}, {true, 0, true}} {
		finished, status = tc.finished, tc.status
		if done, err := removeFinishedTask(ref); err != nil || done != tc.want {
			t.Fatalf("%+v: %v %v", tc, done, err)
		}
	}
	if removed != 1 {
		t.Fatal(removed)
	}
}

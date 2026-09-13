package main

import (
	"crypto/sha1"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestTorrentInfoHashAndMagnetExtraction(t *testing.T) {
	torrent := []byte(`d8:announce3:foo4:infod6:lengthi5e4:name3:abc12:piece lengthi16384e6:pieces3:aaae5:other3:xse`)
	info, ok := bencodeFindInfo(torrent)
	if !ok || string(info) != `d6:lengthi5e4:name3:abc12:piece lengthi16384e6:pieces3:aaae` {
		t.Fatalf("info slice: %q ok=%v", string(info), ok)
	}
	want := sha1.Sum(info)
	if got := torrentInfoHash(torrent); got != hex.EncodeToString(want[:]) {
		t.Fatalf("info hash: %s", got)
	}
	if torrentInfoHash([]byte(`d4:infod4:name3:abe`)) != "" {
		t.Fatal("truncated info accepted")
	}
	if torrentInfoHash([]byte(`i5e`)) != "" {
		t.Fatal("non-dict accepted")
	}
	if got := magnetInfoHash("magnet:?xt=urn:btih:0123456789ABCDEF0123456789ABCDEF01234567&dn=x"); got != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("hex magnet: %s", got)
	}
	raw, _ := hex.DecodeString("0123456789abcdef0123456789abcdef01234567")
	if got := magnetInfoHash("magnet:?xt=urn:btih:" + strings.ToLower(base32.StdEncoding.EncodeToString(raw))); got != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("base32 magnet: %s", got)
	}
	if magnetInfoHash("magnet:?dn=only-name") != "" {
		t.Fatal("hash extracted from magnet without xt")
	}
	if torrentLink("magnet:?xt=urn:btih:0123456789ABCDEF0123456789ABCDEF01234567") {
		t.Fatal("magnet treated as torrent URL")
	}
	if !torrentLink("https://example.test/download/EP01.TORRENT?token=x") {
		t.Fatal(".torrent URL not detected")
	}
}

func TestAria2SubmitsTorrentFileThroughAddTorrent(t *testing.T) {
	testDatabase(t)
	torrentBytes := []byte(`d4:infod6:lengthi5e4:name3:abcee`)
	var rpcBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/t.torrent" {
			w.Write(torrentBytes)
			return
		}
		rpcBody, _ = io.ReadAll(r.Body)
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":"auto-anime","result":"gid-torrent"}`)
	}))
	defer server.Close()
	downloadMode, aria2URL = "aria2", server.URL
	t.Cleanup(func() { aria2URL = "" })
	id := insertItem(t, server.URL+"/t.torrent")
	downloadItem(id)
	var status, taskID string
	db.QueryRow("SELECT status,COALESCE(task_id,'') FROM items WHERE id=?", id).Scan(&status, &taskID)
	if status != "已发送到 Aria2" || taskID != "gid-torrent" {
		t.Fatalf("torrent submit: %s %s", status, taskID)
	}
	var rpc struct {
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(rpcBody, &rpc); err != nil || rpc.Method != "aria2.addTorrent" {
		t.Fatalf("rpc: %s %v", rpc.Method, err)
	}
	var encoded string
	if err := json.Unmarshal(rpc.Params[0], &encoded); err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || string(decoded) != string(torrentBytes) {
		t.Fatalf("torrent payload: %q %v", decoded, err)
	}
}

func TestTorrentBytesUsesCacheAfterFirstFetch(t *testing.T) {
	testDatabase(t)
	root := t.TempDir()
	old := downloadDir
	downloadDir = root
	defer func() { downloadDir = old }()
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write([]byte("d4:infod6:lengthi5e4:name3:abcee"))
	}))
	defer s.Close()
	if _, err := fetchTorrentBytes(s.URL + "/x.torrent"); err != nil {
		t.Fatal(err)
	}
	if _, err := fetchTorrentBytes(s.URL + "/x.torrent"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("fetch calls=%d", calls)
	}
}

func TestAria2TracksProgressAndArchivesCompletion(t *testing.T) {
	testDatabase(t)
	local := t.TempDir()
	tells := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Method string `json:"method"`
		}
		json.Unmarshal(body, &req)
		switch req.Method {
		case "aria2.addUri":
			fmt.Fprint(w, `{"jsonrpc":"2.0","id":"auto-anime","result":"gid-1"}`)
		case "aria2.tellStatus":
			tells++
			if tells == 1 {
				fmt.Fprint(w, `{"jsonrpc":"2.0","id":"auto-anime","result":{"status":"active","completedLength":"50","totalLength":"100","dir":"/downloads","files":[{"path":"/downloads/EP01.mp4","length":"100"}]}}`)
				return
			}
			fmt.Fprint(w, `{"jsonrpc":"2.0","id":"auto-anime","result":{"status":"complete","completedLength":"100","totalLength":"100","dir":"/downloads","files":[{"path":"/downloads/EP01.mp4","length":"8"}]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	oldMode, oldURL := downloadMode, aria2URL
	downloadMode, aria2URL = "aria2", server.URL
	aria2Dir, aria2LocalDir = "/downloads", local
	t.Cleanup(func() { downloadMode, aria2URL = oldMode, oldURL; aria2Dir, aria2LocalDir = "", "" })
	if err := os.MkdirAll(filepath.Join(local, "ep"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(local, "EP01.mp4"), []byte("bt-media"), 0644); err != nil {
		t.Fatal(err)
	}
	id := insertItem(t, server.URL+"/episode.mp4")
	downloadItem(id)
	var status, taskID, downloader string
	if err := db.QueryRow("SELECT status,COALESCE(task_id,''),COALESCE(downloader,'') FROM items WHERE id=?", id).Scan(&status, &taskID, &downloader); err != nil {
		t.Fatal(err)
	}
	if status != "已发送到 Aria2" || taskID != "gid-1" || downloader != "aria2" {
		t.Fatalf("submit: %s %s %s", status, taskID, downloader)
	}
	pollDownloaderTasks()
	var progress int
	db.QueryRow("SELECT status,progress FROM items WHERE id=?", id).Scan(&status, &progress)
	if status != "下载中" || progress != 50 {
		t.Fatalf("first poll: %s %d", status, progress)
	}
	pollDownloaderTasks()
	var path string
	db.QueryRow("SELECT status,COALESCE(file_path,'') FROM items WHERE id=?", id).Scan(&status, &path)
	if status != "已保存" {
		t.Fatalf("second poll status: %s", status)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "bt-media" {
		t.Fatalf("archived bytes: %q %v", data, err)
	}
	// The item has no recognized anime title, so it archives under the
	// fallback series folder; the remote /downloads prefix maps to local.
	if !strings.Contains(filepath.ToSlash(path), "未分类番剧/") || !strings.HasSuffix(filepath.ToSlash(path), "EP01.mp4") {
		t.Fatalf("archive path layout: %s", path)
	}
}

func TestAria2ErrorStateFailsItem(t *testing.T) {
	testDatabase(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":"auto-anime","result":{"status":"error","errorMessage":"disk full","dir":"/downloads","files":[]}}`)
	}))
	defer server.Close()
	oldMode, oldURL := downloadMode, aria2URL
	downloadMode, aria2URL = "aria2", server.URL
	t.Cleanup(func() { downloadMode, aria2URL = oldMode, oldURL })
	id := insertItem(t, "magnet:?xt=urn:btih:0123456789012345678901234567890123456789")
	setSubmitted(id, "aria2", "gid-err", "已发送到 Aria2")
	pollDownloaderTasks()
	var status, taskError string
	db.QueryRow("SELECT status,COALESCE(task_error,'') FROM items WHERE id=?", id).Scan(&status, &taskError)
	if status != "下载失败" || !strings.Contains(taskError, "disk full") {
		t.Fatalf("error state: %s %s", status, taskError)
	}
}

func TestAria2UnknownTaskFailsInsteadOfHanging(t *testing.T) {
	testDatabase(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":"auto-anime","error":{"code":1,"message":"GID abc is not found"}}`)
	}))
	defer server.Close()
	oldMode, oldURL := downloadMode, aria2URL
	downloadMode, aria2URL = "aria2", server.URL
	t.Cleanup(func() { downloadMode, aria2URL = oldMode, oldURL })
	id := insertItem(t, "magnet:?xt=urn:btih:0123456789012345678901234567890123456789")
	setSubmitted(id, "aria2", "gid-gone", "已发送到 Aria2")
	pollDownloaderTasks()
	var status, taskError string
	db.QueryRow("SELECT status,COALESCE(task_error,'') FROM items WHERE id=?", id).Scan(&status, &taskError)
	if status != "下载失败" || !strings.Contains(taskError, "不存在") {
		t.Fatalf("gone task: %s %s", status, taskError)
	}
}

func TestQBittorrentHashTrackingAndArchive(t *testing.T) {
	testDatabase(t)
	local := t.TempDir()
	hash := "0123456789abcdef0123456789abcdef01234567"
	var infoCalls atomic.Int32
	var queriedTag, queriedHash, addedTags string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/auth/login", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Ok.")
	})
	mux.HandleFunc("/api/v2/torrents/add", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		addedTags = r.FormValue("tags")
		fmt.Fprint(w, "Ok.")
	})
	mux.HandleFunc("/api/v2/torrents/info", func(w http.ResponseWriter, r *http.Request) {
		if queriedHash = r.URL.Query().Get("hashes"); queriedHash != "" {
			fmt.Fprintf(w, `[{"hash":%q,"name":"夏日动画","save_path":"/downloads/qbit/","progress":1,"state":"uploading"}]`, hash)
			return
		}
		queriedTag = r.URL.Query().Get("tag")
		if infoCalls.Add(1) == 1 {
			fmt.Fprintf(w, `[{"hash":%q,"name":"夏日动画","save_path":"/downloads/qbit/","progress":0.4,"state":"downloading"}]`, strings.ToUpper(hash))
			return
		}
		fmt.Fprint(w, `[]`)
	})
	mux.HandleFunc("/api/v2/torrents/files", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"name":"夏日动画/01.mkv"}]`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	oldMode, oldURL, oldSave, oldLocal := downloadMode, qbitURL, qbitSavePath, qbitLocalDir
	downloadMode, qbitURL, qbitSavePath, qbitLocalDir = "qbittorrent", server.URL, "/downloads/qbit", local
	t.Cleanup(func() { downloadMode, qbitURL, qbitSavePath, qbitLocalDir = oldMode, oldURL, oldSave, oldLocal })
	if err := os.MkdirAll(filepath.Join(local, "夏日动画"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(local, "夏日动画", "01.mkv"), []byte("qbit-media"), 0644); err != nil {
		t.Fatal(err)
	}
	// A .torrent URL the origin no longer serves: infohash lookup fails, so
	// tracking must fall back to discovering the hash by tag.
	id := insertItem(t, server.URL+"/missing.torrent")
	downloadItem(id)
	var status, taskID string
	db.QueryRow("SELECT status,COALESCE(task_id,'') FROM items WHERE id=?", id).Scan(&status, &taskID)
	if status != "已发送到 qBittorrent" || taskID != "" {
		t.Fatalf("submit: %s %q", status, taskID)
	}
	if !strings.Contains(addedTags, "auto-anime-") {
		t.Fatalf("submit tag: %q", addedTags)
	}
	pollDownloaderTasks()
	db.QueryRow("SELECT status,COALESCE(task_id,'') FROM items WHERE id=?", id).Scan(&status, &taskID)
	if status != "下载中" || taskID != hash {
		t.Fatalf("tag discovery: %s %q", status, taskID)
	}
	if queriedTag != qbitTagFor(id) {
		t.Fatalf("discovery tag: %q", queriedTag)
	}
	pollDownloaderTasks()
	var path string
	db.QueryRow("SELECT status,COALESCE(file_path,'') FROM items WHERE id=?", id).Scan(&status, &path)
	if status != "已保存" {
		t.Fatalf("completion: %s", status)
	}
	if queriedHash != hash {
		t.Fatalf("hash query: %q", queriedHash)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "qbit-media" {
		t.Fatalf("archived bytes: %q %v", data, err)
	}
	if !strings.Contains(filepath.ToSlash(path), "未分类番剧/") || !strings.HasSuffix(filepath.ToSlash(path), "夏日动画/01.mkv") {
		t.Fatalf("unexpected archive layout: %s", path)
	}
}

func TestQBittorrentMagnetHashUsedDirectly(t *testing.T) {
	testDatabase(t)
	local := t.TempDir()
	hash := "0123456789abcdef0123456789abcdef01234567"
	var queriedHash string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/auth/login", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Ok.")
	})
	mux.HandleFunc("/api/v2/torrents/add", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Ok.")
	})
	mux.HandleFunc("/api/v2/torrents/info", func(w http.ResponseWriter, r *http.Request) {
		queriedHash = r.URL.Query().Get("hashes")
		if queriedHash != "" {
			fmt.Fprintf(w, `[{"hash":%q,"name":"x","save_path":"/downloads/qbit/","progress":1,"state":"stalledUP"}]`, hash)
			return
		}
		fmt.Fprint(w, `[]`)
	})
	mux.HandleFunc("/api/v2/torrents/files", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"name":"01.mkv"}]`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	oldMode, oldURL, oldLocal := downloadMode, qbitURL, qbitLocalDir
	downloadMode, qbitURL, qbitLocalDir = "qbittorrent", server.URL, local
	t.Cleanup(func() { downloadMode, qbitURL, qbitLocalDir = oldMode, oldURL, oldLocal })
	os.WriteFile(filepath.Join(local, "01.mkv"), []byte("m"), 0644)
	id := insertItem(t, "magnet:?xt=urn:btih:0123456789ABCDEF0123456789ABCDEF01234567")
	downloadItem(id)
	var taskID string
	db.QueryRow("SELECT COALESCE(task_id,'') FROM items WHERE id=?", id).Scan(&taskID)
	if taskID != hash {
		t.Fatalf("magnet hash task: %q", taskID)
	}
	pollDownloaderTasks()
	var path, status string
	db.QueryRow("SELECT status,COALESCE(file_path,'') FROM items WHERE id=?", id).Scan(&status, &path)
	if status != "已保存" || queriedHash != hash {
		t.Fatalf("hash poll: %s %q", status, queriedHash)
	}
}

func TestTransmissionTracksAndArchives(t *testing.T) {
	testDatabase(t)
	local := t.TempDir()
	var getCalls int
	var requestedIDs []int
	mux := http.NewServeMux()
	mux.HandleFunc("/transmission/rpc", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Transmission-Session-Id") == "" {
			w.Header().Set("X-Transmission-Session-Id", "sess-1")
			w.WriteHeader(http.StatusConflict)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Method    string `json:"method"`
			Arguments struct {
				IDs []int `json:"ids"`
			} `json:"arguments"`
		}
		json.Unmarshal(body, &req)
		switch req.Method {
		case "torrent-add":
			fmt.Fprint(w, `{"result":"success","arguments":{"torrent-added":{"id":7}}}`)
		case "torrent-get":
			getCalls++
			requestedIDs = req.Arguments.IDs
			if getCalls == 1 {
				fmt.Fprint(w, `{"result":"success","arguments":{"torrents":[{"id":7,"status":4,"percentDone":0.5,"downloadDir":"/downloads/tr/","files":[{"name":"夏日动画/01.mkv"}]}]}}`)
				return
			}
			fmt.Fprint(w, `{"result":"success","arguments":{"torrents":[{"id":7,"status":6,"percentDone":1,"isFinished":true,"downloadDir":"/downloads/tr/","files":[{"name":"夏日动画/01.mkv"}]}]}}`)
		default:
			http.NotFound(w, r)
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	oldMode, oldURL, oldDir, oldLocal := downloadMode, transmissionURL, transmissionDownloadDir, transmissionLocalDir
	downloadMode, transmissionURL, transmissionDownloadDir, transmissionLocalDir = "transmission", server.URL, "/downloads/tr", local
	t.Cleanup(func() {
		downloadMode, transmissionURL, transmissionDownloadDir, transmissionLocalDir = oldMode, oldURL, oldDir, oldLocal
	})
	if err := os.MkdirAll(filepath.Join(local, "夏日动画"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(local, "夏日动画", "01.mkv"), []byte("tr-media"), 0644); err != nil {
		t.Fatal(err)
	}
	id := insertItem(t, "magnet:?xt=urn:btih:0123456789012345678901234567890123456789")
	downloadItem(id)
	var status, taskID string
	db.QueryRow("SELECT status,COALESCE(task_id,'') FROM items WHERE id=?", id).Scan(&status, &taskID)
	if status != "已发送到 Transmission" || taskID != "7" {
		t.Fatalf("submit: %s %s", status, taskID)
	}
	pollDownloaderTasks()
	var progress int
	db.QueryRow("SELECT status,progress FROM items WHERE id=?", id).Scan(&status, &progress)
	if status != "下载中" || progress != 50 {
		t.Fatalf("first poll: %s %d", status, progress)
	}
	pollDownloaderTasks()
	var path string
	db.QueryRow("SELECT status,COALESCE(file_path,'') FROM items WHERE id=?", id).Scan(&status, &path)
	if status != "已保存" {
		t.Fatalf("completion: %s", status)
	}
	if len(requestedIDs) != 1 || requestedIDs[0] != 7 {
		t.Fatalf("torrent-get ids: %v", requestedIDs)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "tr-media" {
		t.Fatalf("archived bytes: %q %v", data, err)
	}
}

func TestTorrentPayloadNeverArchivesAsEpisode(t *testing.T) {
	testDatabase(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if strings.HasPrefix(r.URL.Path, "/dynamic") {
			w.Header().Set("Content-Type", "application/x-bittorrent")
			fmt.Fprint(w, "d4:infod4:name3:abcee")
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	t.Run("static-torrent-url", func(t *testing.T) {
		id := insertItem(t, server.URL+"/EP01.torrent")
		downloadItem(id)
		var status, taskError, path string
		db.QueryRow("SELECT status,COALESCE(task_error,''),COALESCE(file_path,'') FROM items WHERE id=?", id).Scan(&status, &taskError, &path)
		if status != "等待下载器" || !strings.Contains(taskError, "种子") || path != "" {
			t.Fatalf("static torrent link: %s %q %q", status, taskError, path)
		}
	})
	t.Run("dynamic-torrent-response", func(t *testing.T) {
		id := insertItem(t, server.URL+"/dynamic?id=2")
		downloadItem(id)
		var status, taskError string
		db.QueryRow("SELECT status,COALESCE(task_error,'') FROM items WHERE id=?", id).Scan(&status, &taskError)
		if status != "等待下载器" || !strings.Contains(taskError, "种子") {
			t.Fatalf("dynamic torrent response: %s %q", status, taskError)
		}
	})
	entries, _ := os.ReadDir(downloadDir)
	if len(entries) != 0 {
		t.Fatalf("torrent payload was archived: %v", entries)
	}
}

func TestMagnetWithoutDownloaderWaitsWithReason(t *testing.T) {
	testDatabase(t)
	id := insertItem(t, "magnet:?xt=urn:btih:0123456789012345678901234567890123456789")
	downloadItem(id)
	var status, taskError string
	db.QueryRow("SELECT status,COALESCE(task_error,'') FROM items WHERE id=?", id).Scan(&status, &taskError)
	if status != "等待下载器" || !strings.Contains(taskError, "下载器") {
		t.Fatalf("magnet wait: %s %q", status, taskError)
	}
}

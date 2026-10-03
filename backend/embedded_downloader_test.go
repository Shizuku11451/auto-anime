package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestEmbeddedQbitConfigurationPersistsCredentials(t *testing.T) {
	root := filepath.Join(t.TempDir(), "配置,目录")
	first, err := prepareEmbeddedQbit(root, 18080, 16881)
	if err != nil {
		t.Fatal(err)
	}
	second, err := prepareEmbeddedQbit(root, 18081, 16882)
	if err != nil {
		t.Fatal(err)
	}
	if first.Password != second.Password || len(first.Password) < 32 || second.URL != "http://127.0.0.1:18081" {
		t.Fatal("credentials or endpoint did not survive reconfiguration")
	}
	ext := ".conf"
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		ext = ".ini"
	}
	path := filepath.Join(root, "profile", "qBittorrent", "config", "qBittorrent"+ext)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`WebUI\Address=127.0.0.1`, `WebUI\Port=18081`, `WebUI\LocalHostAuth=true`, `Session\QueueingSystemEnabled=false`, `Session\Port=16882`, `WebUI\Password_PBKDF2="@ByteArray(`} {
		if !bytes.Contains(raw, []byte(expected)) {
			t.Errorf("missing managed setting %s", expected)
		}
	}
	if bytes.Contains(raw, []byte(first.Password)) {
		t.Fatal("plaintext credential entered qBittorrent configuration")
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(filepath.Join(root, "credentials.json"))
		if info.Mode().Perm() != 0600 {
			t.Fatal("private credentials are not mode 0600")
		}
	}
}

func TestEmbeddedQbitPreservesSettingsAndRejectsInvalidFiles(t *testing.T) {
	values := map[string]map[string]string{"Preferences": {`WebUI\Address`: "127.0.0.1", `WebUI\Port`: "8080"}}
	raw := []byte("[Preferences]\nWebUI\\Address=*\nWebUI\\Address=0.0.0.0\nCustom=中文\n[BitTorrent]\nSession\\GlobalUPSpeedLimit=123\n")
	got, err := updateQbitINI(raw, values)
	if err != nil || strings.Count(string(got), `WebUI\Address=127.0.0.1`) != 2 || !bytes.Contains(got, []byte("Custom=中文")) || !bytes.Contains(got, []byte(`Session\GlobalUPSpeedLimit=123`)) {
		t.Fatalf("settings not preserved: %s %v", got, err)
	}
	if _, err := updateQbitINI([]byte{0xff}, values); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
	root := t.TempDir()
	path := filepath.Join(root, "credentials.json")
	os.WriteFile(path, []byte("corrupt"), 0600)
	if _, err := prepareEmbeddedQbit(root, 8080, 6881); err == nil {
		t.Fatal("corrupt credentials silently replaced")
	}
	if raw, _ := os.ReadFile(path); string(raw) != "corrupt" {
		t.Fatal("old credential file overwritten")
	}
}

func TestEmbeddedQbitLeavesExternalDownloadersAlone(t *testing.T) {
	testDatabase(t)
	oldMode, oldURL, oldUser, oldPass := downloadMode, qbitURL, qbitUser, qbitPassword
	t.Cleanup(func() { downloadMode, qbitURL, qbitUser, qbitPassword = oldMode, oldURL, oldUser, oldPass })
	t.Setenv("QBITTORRENT_PATH", filepath.Join(t.TempDir(), "missing-qbit"))
	qbitURL, qbitUser, qbitPassword = "http://external.invalid", "external", "external-test-value"
	for _, mode := range []string{"direct", "aria2", "qbittorrent", "transmission"} {
		downloadMode = mode
		if err := startEmbeddedQBittorrent(); err != nil {
			t.Fatal(err)
		}
		if qbitURL != "http://external.invalid" || qbitUser != "external" || qbitPassword != "external-test-value" {
			t.Fatal("external configuration overwritten")
		}
	}
	downloadMode = "builtin"
	err := startEmbeddedQBittorrent()
	if runtime.GOOS == "windows" {
		// Windows Release 不带 qBittorrent：降级到 direct 并告警，不能启动即退出，
		// 否则界面进不去、设置也改不回来（见 platform_defaults_test.go）。
		if err != nil {
			t.Fatalf("Windows 上应降级而不是报错: %v", err)
		}
		if downloadMode != "direct" {
			t.Fatalf("downloadMode = %q, want direct", downloadMode)
		}
		return
	}
	if err == nil {
		t.Fatal("missing executable silently accepted")
	}
}

func TestEmbeddedQbitOrphansRespectOwnershipAndPreserveFiles(t *testing.T) {
	testDatabase(t)
	root := t.TempDir()
	for _, row := range []struct {
		id               int
		downloader, hash string
	}{{1, "builtin", "keep"}, {2, "qbittorrent", "external"}, {3, "builtin", ""}} {
		if _, err := db.Exec("INSERT INTO items(id,link,downloader,task_id) VALUES(?,?,?,?)", row.id, fmt.Sprint(row.id), row.downloader, row.hash); err != nil {
			t.Fatal(err)
		}
	}
	removed := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/torrents/info":
			json.NewEncoder(w).Encode([]qbitTorrent{
				{Hash: "orphan", SavePath: root, Tags: "auto-anime-9"},
				{Hash: "keep", SavePath: root, Tags: "auto-anime-8, auto-anime-1"},
				{Hash: "pending", SavePath: root, Tags: "auto-anime-3"},
				{Hash: "foreign-dir", SavePath: filepath.Dir(root), Tags: "auto-anime-9"},
				{Hash: "foreign-tag", SavePath: root, Tags: "manual"},
			})
		case "/api/v2/torrents/delete":
			r.ParseForm()
			if r.FormValue("deleteFiles") != "false" {
				t.Error("attempted to delete media")
			}
			removed[r.FormValue("hashes")] = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client := &qbitClient{http: server.Client(), config: qbitConnection{URL: server.URL, SavePath: root}}
	if err := cleanupEmbeddedQbitOrphans(client); err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || !removed["orphan"] {
		t.Fatalf("removed wrong tasks: %v", removed)
	}
}

func resetEmbeddedQbitTest(t *testing.T) {
	t.Helper()
	stopEmbeddedQBittorrent()
	embeddedQbit.Lock()
	embeddedQbit.process, embeddedQbit.stopping = nil, false
	embeddedQbit.requested, embeddedQbit.failures, embeddedQbit.retryAt = false, 0, time.Time{}
	embeddedQbit.Unlock()
}

// Optional real lifecycle coverage uses a private profile, local torrent,
// random ports and temporary database; it never touches an installed profile.
func TestEmbeddedQbitRealLifecycle(t *testing.T) {
	exe := os.Getenv("TEST_QBITTORRENT_PATH")
	if exe == "" {
		t.Skip("set TEST_QBITTORRENT_PATH for the real qBittorrent lifecycle test")
	}
	testDatabase(t)
	resetEmbeddedQbitTest(t)
	t.Cleanup(func() { resetEmbeddedQbitTest(t) })
	freePort := func() string {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer listener.Close()
		return fmt.Sprint(listener.Addr().(*net.TCPAddr).Port)
	}
	root := filepath.Join(t.TempDir(), "private qbit")
	t.Setenv("QBITTORRENT_PATH", exe)
	t.Setenv("QBITTORRENT_DATA_DIR", root)
	t.Setenv("EMBEDDED_QBIT_PORT", freePort())
	t.Setenv("QBITTORRENT_BT_PORT", freePort())
	oldMode := downloadMode
	downloadMode = "builtin"
	t.Cleanup(func() { downloadMode = oldMode })
	if err := startEmbeddedQBittorrent(); err != nil {
		t.Fatal(err)
	}
	config, err := embeddedQbitConnection()
	if err != nil {
		t.Fatal(err)
	}
	oldExternalURL := qbitURL
	t.Cleanup(func() { qbitURL = oldExternalURL })
	externalURL, mode := "http://external.invalid", "builtin"
	if err := applySettings(&settingsInput{DownloadMode: &mode, QbitURL: &externalURL}); err != nil {
		t.Fatal(err)
	}
	if after, err := embeddedQbitConnection(); err != nil || after != config || qbitURL != externalURL {
		t.Fatal("settings save changed the private downloader connection")
	}
	for path, handler := range map[string]http.HandlerFunc{"/api/settings": settingsHandler, "/api/status": runtimeStatus, "/api/backup": backup} {
		rec := httptest.NewRecorder()
		handler(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), config.Password) || strings.Contains(rec.Body.String(), config.URL) {
			t.Fatalf("private connection exposed or endpoint failed: %s", path)
		}
	}
	client, err := qbitLoginFor("builtin")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.post("app/setPreferences", url.Values{"json": {`{"dht":false,"pex":false,"lsd":false}`}}); err != nil {
		t.Fatal(err)
	}
	content := []byte("local qBittorrent verified media")
	piece := sha1.Sum(content)
	info := append([]byte(fmt.Sprintf("d6:lengthi%de4:name11:episode.mp412:piece lengthi16384e6:pieces20:", len(content))), piece[:]...)
	info = append(info, []byte("7:privatei1ee")...)
	torrent := append(append([]byte("d4:info"), info...), 'e')
	hashBytes := sha1.Sum(info)
	hash := hex.EncodeToString(hashBytes[:])
	source := filepath.Join(config.SavePath, "episode.mp4")
	if err := os.WriteFile(source, content, 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(torrent) }))
	defer server.Close()
	id := insertItem(t, server.URL+"/test.torrent")
	downloadItem(id)
	waitFor := func(check func() bool) {
		t.Helper()
		deadline := time.Now().Add(25 * time.Second)
		for time.Now().Before(deadline) {
			if check() {
				return
			}
			time.Sleep(150 * time.Millisecond)
		}
		t.Fatal("real qBittorrent condition timed out")
	}
	var archive string
	waitFor(func() bool {
		pollDownloaderTasks()
		var status string
		db.QueryRow("SELECT status,COALESCE(file_path,'') FROM items WHERE id=?", id).Scan(&status, &archive)
		return status == "已保存"
	})
	if raw, _ := os.ReadFile(archive); !bytes.Equal(raw, content) {
		t.Fatal("archive content changed")
	}
	if raw, _ := os.ReadFile(source); !bytes.Equal(raw, content) {
		t.Fatal("source content changed")
	}
	// A trackerless magnet with DHT/PEX/LSD disabled exercises real controls
	// without contacting peers or downloading public media.
	pausedID := insertItem(t, "magnet:?xt=urn:btih:"+controlTestHash)
	downloadItem(pausedID)
	waitFor(func() bool {
		rows, err := client.info(url.Values{"hashes": {controlTestHash}})
		return err == nil && len(rows) == 1
	})
	if changed, err := pauseItemDownload(pausedID); err != nil || !changed {
		t.Fatal("real pause failed", changed, err)
	}
	if item := controlItem(t, pausedID); item.Status != "已暂停" || item.TaskID != controlTestHash {
		t.Fatal("real pause lost task identity", item)
	}
	resetEmbeddedQbitTest(t)
	// Saved builtin jobs still need their private instance for seeding even
	// when new downloads have switched to another mode.
	downloadMode = "direct"
	if err := startEmbeddedQBittorrent(); err != nil {
		t.Fatal(err)
	}
	restored, err := embeddedQbitConnection()
	if err != nil || restored.Password != config.Password {
		t.Fatal("credentials not restored")
	}
	client, err = qbitLoginFor("builtin")
	if err != nil {
		t.Fatal(err)
	}
	waitFor(func() bool {
		rows, err := client.info(url.Values{"hashes": {hash}})
		return err == nil && len(rows) == 1 && rows[0].Hash == hash
	})
	waitFor(func() bool {
		rows, err := client.info(url.Values{"hashes": {controlTestHash}})
		return err == nil && len(rows) == 1 && qbitPausedState(rows[0].State)
	})
	if changed, err := queueItemDownload(pausedID, true); err != nil || !changed {
		t.Fatal("real resume did not queue", changed, err)
	}
	runQueuedDownload(pausedID)
	if item := controlItem(t, pausedID); item.Status != "下载中" || item.Downloader != "builtin" || item.TaskID != controlTestHash {
		t.Fatal("real resume changed downloader identity", item)
	}
	rows, err := client.info(url.Values{"hashes": {controlTestHash}})
	if err != nil || len(rows) != 1 || qbitPausedState(rows[0].State) {
		t.Fatal("real qBittorrent did not resume its original task", rows, err)
	}
	removeDownloaderTask("builtin", controlTestHash)
	removeDownloaderTask("builtin", hash)
	waitFor(func() bool {
		rows, err := client.info(url.Values{"hashes": {hash}})
		return err == nil && len(rows) == 0
	})
	if raw, _ := os.ReadFile(source); !bytes.Equal(raw, content) {
		t.Fatal("task deletion removed source file")
	}
}

func TestEmbeddedQbitTagDiscoveryCannotChangeAnotherDownloader(t *testing.T) {
	for _, original := range []string{"builtin", "qbittorrent"} {
		t.Run(original, func(t *testing.T) {
			testDatabase(t)
			id := insertItem(t, "https://example.test/test.torrent")
			if _, err := db.Exec("UPDATE items SET downloader=?,task_id='',status='已发送到 qBittorrent' WHERE id=?", original, id); err != nil {
				t.Fatal(err)
			}
			replacement := "builtin"
			if original == replacement {
				replacement = "qbittorrent"
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if _, err := db.Exec("UPDATE items SET downloader=? WHERE id=?", replacement, id); err != nil {
					t.Error(err)
				}
				json.NewEncoder(w).Encode([]qbitTorrent{{Hash: controlTestHash, State: "downloading", Progress: 0.5}})
			}))
			defer server.Close()
			client := &qbitClient{http: server.Client(), config: qbitConnection{URL: server.URL}}
			state, err := qbitPoll(client, taskRef{ID: id, Downloader: original})
			if err != nil || !state.Pending {
				t.Fatalf("stale result was accepted: %+v %v", state, err)
			}
			var hash string
			if err := db.QueryRow("SELECT task_id FROM items WHERE id=?", id).Scan(&hash); err != nil || hash != "" {
				t.Fatal("stale hash replaced the new downloader task")
			}
		})
	}
}

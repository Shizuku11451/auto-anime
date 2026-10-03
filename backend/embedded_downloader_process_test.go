package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Re-exec only in an explicitly selected helper process. This exercises the
// real launcher and OS process cleanup without a qBittorrent installation,
// public peers or a user's running downloader/profile.
func TestMain(m *testing.M) {
	if os.Getenv("AUTO_ANIME_QBIT_TEST_HELPER") == "1" && len(os.Args) > 1 && strings.HasPrefix(os.Args[1], "--profile=") {
		os.Exit(runQbitTestHelper())
	}
	os.Exit(m.Run())
}

func runQbitTestHelper() int {
	var credentials struct{ Username, Password string }
	raw, err := os.ReadFile(filepath.Join(os.Getenv("QBITTORRENT_DATA_DIR"), "credentials.json"))
	if err != nil || json.Unmarshal(raw, &credentials) != nil {
		return 2
	}
	mux := http.NewServeMux()
	server := &http.Server{Addr: "127.0.0.1:" + os.Getenv("EMBEDDED_QBIT_PORT"), Handler: mux, ReadHeaderTimeout: time.Second}
	mux.HandleFunc("/api/v2/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("username") != credentials.Username || r.FormValue("password") != credentials.Password {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/api/v2/torrents/info", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "[]") })
	mux.HandleFunc("/api/v2/app/shutdown", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			server.Shutdown(ctx)
		}()
	})
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		return 3
	}
	return 0
}

func TestEmbeddedQbitProcessCrashRestartsPrivateProfile(t *testing.T) {
	testDatabase(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	freePort := func() string {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer listener.Close()
		return fmt.Sprint(listener.Addr().(*net.TCPAddr).Port)
	}
	root := filepath.Join(t.TempDir(), "独立配置")
	t.Setenv("AUTO_ANIME_QBIT_TEST_HELPER", "1")
	t.Setenv("QBITTORRENT_PATH", exe)
	t.Setenv("QBITTORRENT_DATA_DIR", root)
	t.Setenv("EMBEDDED_QBIT_PORT", freePort())
	t.Setenv("QBITTORRENT_BT_PORT", freePort())
	var manager embeddedQBittorrentManager
	t.Cleanup(manager.stop)
	first, err := manager.connection(launchEmbeddedQBittorrent)
	if err != nil {
		t.Fatal(err)
	}
	media := filepath.Join(first.config.SavePath, "source.mkv")
	if err := os.WriteFile(media, []byte("preserved source"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := first.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	awaitQbitTest(t, first.done)
	if manager.healthy() {
		t.Fatal("crashed child remained healthy")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); manager.supervise(ctx, 20*time.Millisecond, launchEmbeddedQBittorrent) }()
	t.Cleanup(func() { cancel(); manager.stop(); awaitQbitTest(t, done) })
	deadline := time.Now().Add(8 * time.Second)
	var second *managedQBittorrent
	for time.Now().Before(deadline) {
		manager.Lock()
		second = manager.process
		manager.Unlock()
		if second != first && manager.healthy() {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if second == first || !manager.healthy() {
		t.Fatal("real child process did not restart")
	}
	if second.config != first.config || second.cmd.Process.Pid == first.cmd.Process.Pid {
		t.Fatal("restart did not reuse the private connection with a new process")
	}
	manager.stop()
	awaitQbitTest(t, second.done)
	if manager.healthy() {
		t.Fatal("stopped child remained healthy")
	}
	if raw, err := os.ReadFile(media); err != nil || string(raw) != "preserved source" {
		t.Fatal("restart or shutdown changed source media")
	}
}

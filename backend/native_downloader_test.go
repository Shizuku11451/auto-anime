package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	raintorrent "github.com/cenkalti/rain/torrent"
)

func TestValidateNativePath(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{
		"episode.mkv",
		"Show/Season 01/episode.mkv",
		"番剧/第01集.mkv",
		"folder\\episode.mkv",
	} {
		if err := validateNativePath(root, name); err != nil {
			t.Errorf("safe path %q rejected: %v", name, err)
		}
	}
	for _, name := range []string{
		"../evil",
		"a/../../evil",
		"a\\..\\evil",
		`C:\evil`,
		"C:/evil",
		"/absolute",
		"CON",
		"folder/CON.txt",
		"a:b",
		"a/b.",
		"a/b ",
		"a/b\x00c",
		"",
	} {
		if err := validateNativePath(root, name); err == nil {
			t.Errorf("unsafe path %q accepted", name)
		}
	}
}

func TestNativeSettingsFromEnv(t *testing.T) {
	root := t.TempDir()
	t.Setenv("NATIVE_BT_DATA_DIR", root)
	t.Setenv("NATIVE_BT_PORT", "7001")
	t.Setenv("NATIVE_BT_PORT_RANGE", "3")
	t.Setenv("NATIVE_DHT_PORT", "7002")
	settings, err := nativeSettingsFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if settings.Root == "" || settings.DataDir != filepath.Join(settings.Root, "downloads") || settings.SessionDB != filepath.Join(settings.Root, "session.db") {
		t.Fatalf("unexpected settings paths: %+v", settings)
	}
	if settings.BTPort != 7001 || settings.PortEnd != 7004 || settings.DHTPort != 7002 || settings.PortRange != 3 {
		t.Fatalf("unexpected settings ports: %+v", settings)
	}
	t.Setenv("NATIVE_BT_PORT_RANGE", "101")
	if _, err := nativeSettingsFromEnv(); err == nil {
		t.Fatal("out-of-range native port range accepted")
	}
	t.Setenv("NATIVE_BT_PORT_RANGE", "10")
	t.Setenv("NATIVE_BT_PORT", "65530")
	if _, err := nativeSettingsFromEnv(); err == nil {
		t.Fatal("out-of-range native port end accepted")
	}
}

func TestNativeSettingsValidationAndSourceRoot(t *testing.T) {
	testDatabase(t)
	mode := "native"
	if err := validateSettings(&settingsInput{DownloadMode: &mode}); err != nil {
		t.Fatalf("native mode rejected by settings: %v", err)
	}
	oldMode := downloadMode
	t.Cleanup(func() { downloadMode = oldMode })
	downloadMode = "native"
	if err := validateDownloadTarget(); err != nil {
		t.Fatalf("native mode rejected by download target: %v", err)
	}
	root := t.TempDir()
	t.Setenv("NATIVE_BT_DATA_DIR", root)
	want := filepath.Join(root, "downloads")
	if got := nativeSourceRoot(); got != want {
		t.Fatalf("nativeSourceRoot = %q, want %q", got, want)
	}
	if got := sourceRootFor("native"); got != want {
		t.Fatalf("sourceRootFor(native) = %q, want %q", got, want)
	}
	connection := cleanupConnection("native")
	if connection == "" {
		t.Fatal("native cleanup connection is empty")
	}
	t.Setenv("NATIVE_BT_PORT", "7101")
	if cleanupConnection("native") == connection {
		t.Fatal("native cleanup connection does not include port changes")
	}
}

func TestNativeRealPeerDownloadPauseResume(t *testing.T) {
	if testing.Short() {
		t.Skip("real BT transfer")
	}
	testDatabase(t)
	downloadMode = "native"
	base := t.TempDir()
	t.Setenv("NATIVE_BT_DATA_DIR", filepath.Join(base, "native"))
	t.Setenv("NATIVE_BT_PORT", fmt.Sprint(freeTCPPort(t)))
	t.Setenv("NATIVE_BT_PORT_RANGE", "1")
	t.Setenv("NATIVE_DHT_PORT", fmt.Sprint(freeUDPPort(t)))

	name := "native-test.bin"
	data := bytes.Repeat([]byte("0123456789abcdef"), 4096) // 64 KiB
	torrentData := buildNativeTestTorrent(name, data)
	hash := torrentInfoHash(torrentData)
	if !nativeInfoHashPattern.MatchString(hash) {
		t.Fatalf("invalid test info hash %q", hash)
	}

	seederDir := filepath.Join(base, "seeder")
	if err := os.MkdirAll(seederDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seederDir, name), data, 0600); err != nil {
		t.Fatal(err)
	}
	seeder := newNativeTestSession(t, seederDir, freeTCPPort(t), false)
	defer seeder.Close()
	seederTorrent, err := seeder.AddTorrent(bytes.NewReader(torrentData), &raintorrent.AddTorrentOptions{})
	if err != nil {
		t.Fatal(err)
	}
	waitNativeStatus(t, seederTorrent, raintorrent.Seeding, 10*time.Second)
	if seederTorrent.Port() == 0 {
		t.Fatal("seeder did not open a listening port")
	}

	result, err := db.Exec("INSERT INTO items(title,link,downloader,task_id,status,progress) VALUES(?,?,?,?,?,0)",
		"测试动画", "magnet:?xt=urn:btih:"+hash, "native", hash, "下载中")
	if err != nil {
		t.Fatal(err)
	}
	itemID, _ := result.LastInsertId()
	ref := taskRef{ID: int(itemID), Downloader: "native", TaskID: hash}

	download := &nativeDownloader{}
	defer download.stop()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := download.add(ctx, ref, "", torrentData); err != nil {
		t.Fatal(err)
	}
	var torrent *raintorrent.Torrent
	if err := download.withSession(ctx, func(session *raintorrent.Session) error {
		torrent = session.GetTorrent(hash)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if torrent == nil {
		t.Fatal("native torrent not found after add")
	}
	if err := torrent.AddPeer(fmt.Sprintf("127.0.0.1:%d", seederTorrent.Port())); err != nil {
		t.Fatal(err)
	}

	var completed []archiveFile
	deadline := time.Now().Add(20 * time.Second)
	for {
		state, err := download.poll(ctx, ref)
		if err != nil {
			t.Fatal(err)
		}
		if state.Fail != "" {
			t.Fatal(state.Fail)
		}
		if len(state.Files) > 0 {
			if len(state.Files) != 1 {
				t.Fatalf("unexpected archive file count: %+v", state.Files)
			}
			got, err := os.ReadFile(state.Files[0].Src)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, data) {
				t.Fatal("downloaded data mismatch")
			}
			completed = state.Files
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("download did not complete: %+v", torrent.Stats())
		}
		time.Sleep(100 * time.Millisecond)
	}

	if _, err := download.control(ctx, ref, true); err != nil {
		t.Fatal(err)
	}
	if status := torrent.Stats().Status; status != raintorrent.Stopped {
		t.Fatalf("pause left status %s", status)
	}
	if _, err := download.control(ctx, ref, false); err != nil {
		t.Fatal(err)
	}
	if status := torrent.Stats().Status; status == raintorrent.Stopped {
		t.Fatalf("resume did not leave stopped state")
	}

	// 源文件清理必须通过 native 身份停止任务并只删除下载根内的普通文件。
	oldNative, oldKeep := nativeDownloads, keepSourceFiles
	nativeDownloads, keepSourceFiles = download, false
	t.Cleanup(func() { nativeDownloads, keepSourceFiles = oldNative, oldKeep })
	// 归档目录与下载目录独立，避免触发保护归档目录内源文件的清理规则。
	archive := filepath.Join(t.TempDir(), "archive.mkv")
	if err := os.WriteFile(archive, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE items SET status='已保存',file_path=?,task_submitted_at=0 WHERE id=?", archive, itemID); err != nil {
		t.Fatal(err)
	}
	if err := queueSourceCleanup(ref, archive, completed); err != nil {
		t.Fatal(err)
	}
	processSourceCleanup()
	if _, err := os.Stat(completed[0].Src); !os.IsNotExist(err) {
		var warning string
		if readErr := db.QueryRow("SELECT task_error FROM items WHERE id=?", itemID).Scan(&warning); readErr != nil {
			t.Fatal(readErr)
		}
		t.Fatalf("source file was not cleaned: %v; task_error=%q", err, warning)
	}
	if got, err := os.ReadFile(archive); err != nil || !bytes.Equal(got, data) {
		t.Fatalf("archive changed during source cleanup: %v", err)
	}
	var pending int
	if err := db.QueryRow("SELECT COUNT(*) FROM source_cleanup WHERE item_id=?", itemID).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Fatal("completed source cleanup was not retired")
	}
}

func newNativeTestSession(t *testing.T, dataDir string, port int, includeTorrentID bool) *raintorrent.Session {
	t.Helper()
	cfg := raintorrent.DefaultConfig
	cfg.Database = filepath.Join(t.TempDir(), "session.db")
	cfg.DataDir = dataDir
	cfg.DataDirIncludesTorrentID = includeTorrentID
	cfg.ResumeOnStartup = false
	cfg.RPCEnabled = false
	cfg.DHTEnabled = false
	cfg.PortBegin = uint16(port)
	cfg.PortEnd = uint16(port + 1)
	cfg.MaxOpenFiles = 0
	cfg.HealthCheckInterval = 24 * time.Hour
	cfg.HealthCheckTimeout = 24 * time.Hour
	cfg.FilePermissions = 0700
	session, err := raintorrent.NewSession(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func waitNativeStatus(t *testing.T, torrent *raintorrent.Torrent, want raintorrent.Status, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		stats := torrent.Stats()
		if stats.Status == want {
			return
		}
		if stats.Error != nil {
			t.Fatalf("torrent error: %v", stats.Error)
		}
		if time.Now().After(deadline) {
			t.Fatalf("status = %s, want %s", stats.Status, want)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func freeUDPPort(t *testing.T) int {
	t.Helper()
	c, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	port := c.LocalAddr().(*net.UDPAddr).Port
	c.Close()
	return port
}

func buildNativeTestTorrent(name string, data []byte) []byte {
	const pieceLength = 16 * 1024
	var pieces bytes.Buffer
	for offset := 0; offset < len(data); offset += pieceLength {
		end := min(offset+pieceLength, len(data))
		sum := sha1.Sum(data[offset:end])
		pieces.Write(sum[:])
	}
	var b strings.Builder
	b.WriteString("d4:infod")
	fmt.Fprintf(&b, "6:lengthi%de", len(data))
	fmt.Fprintf(&b, "4:name%d:%s", len(name), name)
	fmt.Fprintf(&b, "12:piece lengthi%de", pieceLength)
	fmt.Fprintf(&b, "6:pieces%d:", pieces.Len())
	b.Write(pieces.Bytes())
	b.WriteString("ee")
	return []byte(b.String())
}

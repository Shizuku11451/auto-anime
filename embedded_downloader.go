package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var embeddedAria2Done chan error

// startEmbeddedAria2 owns a private Aria2 instance when no external RPC is
// configured. This keeps task lifecycle inside Auto-Anime and works in Docker
// where the aria2 package is installed by the image.
func startEmbeddedAria2() error {
	// Preserve configured external downloaders, including database settings.
	if !embeddedAria2 || strings.TrimSpace(env("ARIA2_URL", "")) != "" || downloadMode == "qbittorrent" || downloadMode == "transmission" {
		return nil
	}
	root := filepath.Join(downloadDir, ".aria2")
	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}
	port := 6800
	if raw := strings.TrimSpace(env("EMBEDDED_ARIA2_PORT", "6800")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("内置下载器端口无效: %s", raw)
		}
		port = n
	}
	probe, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("内置下载器端口 %d 已被占用", port)
	}
	_ = probe.Close()
	exe := strings.TrimSpace(env("ARIA2_PATH", ""))
	if exe == "" {
		exe, _ = exec.LookPath("aria2c")
	}
	if exe == "" {
		for _, candidate := range []string{filepath.Join(os.Getenv("LOCALAPPDATA"), "Auto-Anime", "tools", "aria2-1.37.0-win-64bit-build1", "aria2c.exe"), "/usr/bin/aria2c", "/usr/local/bin/aria2c"} {
			if _, statErr := os.Stat(candidate); statErr == nil {
				exe = candidate
				break
			}
		}
	}
	if exe == "" {
		return fmt.Errorf("找不到 aria2c，请安装 Aria2 或设置 ARIA2_PATH")
	}
	// Session state retains GIDs and partial downloads between process restarts.
	session := filepath.Join(root, "session.txt")
	f, err := os.OpenFile(session, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("创建下载会话失败: %w", err)
	}
	if err := f.Close(); err != nil {
		return err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return err
	}
	aria2Dir, aria2LocalDir = root, root
	aria2Token = hex.EncodeToString(secret)
	aria2URL = fmt.Sprintf("http://127.0.0.1:%d/jsonrpc", port)
	downloadMode = "aria2"
	args := []string{"--enable-rpc=true", "--rpc-listen-all=false", "--rpc-listen-port=" + strconv.Itoa(port), "--rpc-secret=" + aria2Token, "--dir=" + root, "--continue=true", "--enable-dht=true", "--bt-enable-lpd=true", "--max-concurrent-downloads=" + fmt.Sprint(downloadLimit()), "--follow-torrent=true"}
	args = append(args, "--no-conf=true", "--input-file="+session, "--save-session="+session, "--save-session-interval=10", "--force-save=true", "--auto-save-interval=10", "--stop-with-process="+strconv.Itoa(os.Getpid()), "--dht-file-path="+filepath.Join(root, "dht.dat"), "--dht-file-path6="+filepath.Join(root, "dht6.dat"))
	if trackers := strings.TrimSpace(env("ARIA2_BT_TRACKER", "")); trackers != "" {
		args = append(args, "--bt-tracker="+trackers)
	}
	embeddedAria2Cmd = exec.Command(exe, args...)
	embeddedAria2Cmd.Stdout, embeddedAria2Cmd.Stderr = os.Stdout, os.Stderr
	if err := embeddedAria2Cmd.Start(); err != nil {
		return fmt.Errorf("启动内置下载器失败: %w", err)
	}
	embeddedAria2Done = make(chan error, 1)
	cmd, done := embeddedAria2Cmd, embeddedAria2Done
	go func() { done <- cmd.Wait() }()
	for i := 0; i < 20; i++ {
		if aria2RPC("aria2.getVersion", nil, &struct{}{}) == nil {
			cleanupEmbeddedAria2Orphans(root)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	stopEmbeddedAria2()
	return fmt.Errorf("内置下载器 RPC 未就绪")
}

func cleanupEmbeddedAria2Orphans(root string) {
	for _, method := range []string{"aria2.tellActive", "aria2.tellWaiting"} {
		var rows []struct {
			GID string `json:"gid"`
			Dir string `json:"dir"`
		}
		params := []any{0, 100, []string{"gid", "dir"}}
		if method == "aria2.tellActive" {
			params = []any{[]string{"gid", "dir"}}
		}
		if err := aria2RPC(method, params, &rows); err != nil {
			continue
		}
		for _, row := range rows {
			if filepath.Clean(row.Dir) != filepath.Clean(root) {
				continue
			}
			var count int
			if db.QueryRow("SELECT COUNT(*) FROM items WHERE downloader='aria2' AND task_id=?", row.GID).Scan(&count) == nil && count == 0 {
				if err := aria2RPC("aria2.remove", []any{row.GID}, nil); err != nil {
					_ = aria2RPC("aria2.removeDownloadResult", []any{row.GID}, nil)
				}
			}
		}
	}
}

func stopEmbeddedAria2() {
	if embeddedAria2Cmd == nil || embeddedAria2Cmd.Process == nil {
		return
	}
	select {
	case <-embeddedAria2Done:
	default:
		if err := aria2RPC("aria2.saveSession", nil, nil); err != nil {
			log.Printf("保存内置下载会话失败: %v", err)
		}
		_ = aria2RPC("aria2.shutdown", nil, nil)
		select {
		case <-embeddedAria2Done:
		case <-time.After(5 * time.Second):
			_ = embeddedAria2Cmd.Process.Kill()
			<-embeddedAria2Done
		}
	}
	embeddedAria2Cmd, embeddedAria2Done = nil, nil
}

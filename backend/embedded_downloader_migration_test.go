package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func legacyAria2Fixture(t *testing.T) string {
	t.Helper()
	testDatabase(t)
	t.Setenv("ARIA2_URL", "")
	oldDir, oldLocal := aria2Dir, aria2LocalDir
	t.Cleanup(func() { aria2Dir, aria2LocalDir = oldDir, oldLocal })
	root := filepath.Join(downloadDir, ".aria2")
	aria2URL, aria2Dir, aria2LocalDir = "", root, root
	downloadMode = "aria2"
	if err := writePrivateFile(filepath.Join(root, "session.txt"), []byte("magnet:?xt=urn:btih:test\n gid=1111111111111111\nmagnet:?xt=urn:btih:second\n gid=2222222222222222\n")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "partial.mp4"), []byte("partial media"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		downloader, gid, status string
	}{
		{"aria2", "1111111111111111", "下载中"},
		{"aria2", "2222222222222222", "已发送到 Aria2"},
		{"aria2", "1111111111111111", "已保存"},
		{"qbittorrent", "1111111111111111", "下载中"},
		{"aria2", "3333333333333333", "下载中"},
	} {
		id := insertItem(t, fmt.Sprint(row))
		if _, err := db.Exec("UPDATE items SET downloader=?,task_id=?,status=?,progress=23,task_error='old' WHERE id=?", row.downloader, row.gid, row.status, id); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func assertLegacyAria2Upgrade(t *testing.T, root string) {
	t.Helper()
	for id, want := range []string{"下载失败", "下载失败", "已保存", "下载中", "下载中"} {
		var status, taskID, message string
		var progress int
		if err := db.QueryRow("SELECT status,task_id,task_error,progress FROM items WHERE id=?", id+1).Scan(&status, &taskID, &message, &progress); err != nil {
			t.Fatal(err)
		}
		if status != want || taskID == "" || progress != 23 || (id < 2 && !strings.Contains(message, "重新提交")) {
			t.Fatalf("item %d: status=%s progress=%d error=%s", id+1, status, progress, message)
		}
	}
	if data, err := os.ReadFile(filepath.Join(root, "partial.mp4")); err != nil || string(data) != "partial media" {
		t.Fatal("old source file was changed")
	}
	// A late response from the retired downloader cannot revive the task.
	applyTaskUpdateLocked(taskRef{ID: 1, Downloader: "aria2", TaskID: "1111111111111111"}, taskState{Progress: 99})
	var status string
	if err := db.QueryRow("SELECT status FROM items WHERE id=1").Scan(&status); err != nil || status != "下载失败" {
		t.Fatal("late poll revived a retired task")
	}
}

func TestLegacyEmbeddedAria2UpgradeOnSettingsAndRestart(t *testing.T) {
	for _, entry := range []string{"settings", "restart", "stored private endpoint"} {
		t.Run(entry, func(t *testing.T) {
			root := legacyAria2Fixture(t)
			if entry == "stored private endpoint" {
				aria2URL = "http://127.0.0.1:6800/jsonrpc"
			}
			if entry == "restart" {
				downloadMode = "builtin"
				if _, err := recoverDownloads(db); err != nil {
					t.Fatal(err)
				}
			} else {
				if rec := settingsPUT(t, `{"download_mode":"builtin"}`); rec.Code != 200 {
					t.Fatal(rec.Body.String())
				}
				if downloadMode != "builtin" || getSetting("download_mode") != "builtin" {
					t.Fatal("mode change not applied")
				}
			}
			assertLegacyAria2Upgrade(t, root)
		})
	}
}

func TestLegacyEmbeddedAria2UpgradePreservesExternalTasks(t *testing.T) {
	for _, mode := range []string{"external URL", "explicit env", "different directory", "external mode", "missing session"} {
		t.Run(mode, func(t *testing.T) {
			root := legacyAria2Fixture(t)
			downloadMode = "builtin"
			switch mode {
			case "external URL":
				aria2URL = "http://aria2:6800/jsonrpc"
			case "explicit env":
				t.Setenv("ARIA2_URL", "http://127.0.0.1:6800/jsonrpc")
			case "different directory":
				aria2URL, aria2Dir = "http://127.0.0.1:6800/jsonrpc", t.TempDir()
			case "external mode":
				downloadMode = "qbittorrent"
			case "missing session":
				if err := os.Remove(filepath.Join(root, "session.txt")); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := recoverDownloads(db); err != nil {
				t.Fatal(err)
			}
			var status string
			if err := db.QueryRow("SELECT status FROM items WHERE id=1").Scan(&status); err != nil || status != "下载中" {
				t.Fatal("unrelated external task was retired")
			}
		})
	}
}

func TestLegacyEmbeddedAria2UpgradeRollsBackWithSettings(t *testing.T) {
	legacyAria2Fixture(t)
	if _, err := db.Exec(`CREATE TRIGGER fail_retire BEFORE UPDATE ON items WHEN NEW.id=2 BEGIN SELECT RAISE(ABORT,'injected upgrade failure'); END`); err != nil {
		t.Fatal(err)
	}
	if rec := settingsPUT(t, `{"download_mode":"builtin"}`); rec.Code == 200 {
		t.Fatal("failed upgrade was accepted")
	}
	var status string
	if err := db.QueryRow("SELECT status FROM items WHERE id=1").Scan(&status); err != nil || status != "下载中" || downloadMode != "aria2" || getSetting("download_mode") != "" {
		t.Fatal("failed upgrade left partially changed settings or tasks")
	}
}

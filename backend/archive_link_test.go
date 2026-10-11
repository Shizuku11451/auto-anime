package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func makeArchiveDirectoryLink(t *testing.T, destination, link string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, destination).CombinedOutput(); err != nil {
			t.Fatalf("create temporary junction: %v %s", err, out)
		}
	} else if err := os.Symlink(destination, link); err != nil {
		t.Fatal(err)
	}
}

func TestArchiveRejectsLinkedSeasonForDirectAndDownloaderFiles(t *testing.T) {
	testDatabase(t)
	server := mediaServer(t)
	series := filepath.Join(downloadDir, "链接测试")
	if err := os.MkdirAll(series, 0755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	makeArchiveDirectoryLink(t, outside, filepath.Join(series, "Season 01"))
	result, err := db.Exec("INSERT INTO items(title,link,anime_title,episode) VALUES(?,?,?,?)", "链接测试 01", server.URL+"/ep01.mp4", "链接测试", "1")
	if err != nil {
		t.Fatal(err)
	}
	id, _ := result.LastInsertId()
	if _, err = fetchArchive(int(id), server.URL+"/ep01.mp4"); err == nil || !strings.Contains(err.Error(), "符号链接") {
		t.Fatalf("direct archive accepted linked season: %v", err)
	}
	source := filepath.Join(t.TempDir(), "episode.mp4")
	if err := os.WriteFile(source, []byte("fake-video"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = archiveFromFiles(int(id), []archiveFile{{Src: source, Rel: "episode.mp4"}}); err == nil || !strings.Contains(err.Error(), "符号链接") {
		t.Fatalf("downloader archive accepted linked season: %v", err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("archive escaped to linked target: %v %v", entries, err)
	}
	if data, _ := os.ReadFile(source); string(data) != "fake-video" {
		t.Fatal("source was changed")
	}
}

func TestArchiveRejectsLinkedSeriesRoot(t *testing.T) {
	link := filepath.Join(t.TempDir(), "series")
	makeArchiveDirectoryLink(t, t.TempDir(), link)
	if err := checkSeriesRoot(link); err == nil {
		t.Fatal("linked series root accepted")
	}
}

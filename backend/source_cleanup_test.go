package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSourceCleanupProtectsOutsideDirectoriesAndArchive(t *testing.T) {
	dir := t.TempDir()
	keep := filepath.Join(dir, "keep.txt")
	remove := filepath.Join(dir, "remove.mkv")
	if err := os.WriteFile(keep, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(remove, []byte("remove"), 0600); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "archive.mp4")
	if err := os.WriteFile(archive, []byte("media"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{dir, archive} {
		if _, err := cleanupPaths(sourceCleanup{Root: dir, Archive: archive, Files: []archiveFile{{Src: src}}}); err == nil {
			t.Fatalf("unsafe source accepted: %s", src)
		}
	}
	paths, err := cleanupPaths(sourceCleanup{Root: dir, Archive: archive, Files: []archiveFile{{Src: remove}}})
	if err != nil || len(paths) != 1 {
		t.Fatalf("valid source: %v %v", paths, err)
	}
	if data, err := os.ReadFile(keep); err != nil || string(data) != "keep" {
		t.Fatalf("unrelated file changed: %v %q", err, data)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("source directory removed: %v", err)
	}
}

func TestSourceCleanupWaitsForCommitAndStopsOnlyItsTask(t *testing.T) {
	testDatabase(t)
	oldKeep := keepSourceFiles
	keepSourceFiles = false
	t.Cleanup(func() { keepSourceFiles = oldKeep })
	fake := newControlQbit(t, false)
	root := t.TempDir()
	old := qbitLocalDir
	qbitLocalDir = root
	t.Cleanup(func() { qbitLocalDir = old })
	source := filepath.Join(root, "source.mp4")
	archive := filepath.Join(t.TempDir(), "archive.mp4")
	for _, file := range []string{source, archive} {
		if err := os.WriteFile(file, []byte("media"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := db.Exec(`INSERT INTO items(title,status,downloader,task_id,task_submitted_at,file_path) VALUES('test','下载中','qbittorrent',?,123,?)`, controlTestHash, archive)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := result.LastInsertId()
	ref := taskRef{ID: int(id), Downloader: "qbittorrent", TaskID: controlTestHash, SubmittedAt: 123}
	if err := queueSourceCleanup(ref, archive, []archiveFile{{Src: source}}); err != nil {
		t.Fatal(err)
	}
	processSourceCleanup()
	if _, err := os.Stat(source); err != nil {
		t.Fatal("source removed before archive commit")
	}
	fake.Lock()
	calls := len(fake.calls)
	fake.Unlock()
	if calls != 0 {
		t.Fatal("download task changed before commit")
	}
	db.Exec("UPDATE items SET status='已保存' WHERE id=?", id)
	fake.Lock()
	fake.failure = 500
	fake.Unlock()
	processSourceCleanup()
	if _, err := os.Stat(source); err != nil {
		t.Fatal("source removed despite failed stop")
	}
	var warning string
	db.QueryRow("SELECT task_error FROM items WHERE id=?", id).Scan(&warning)
	if warning == "" {
		t.Fatal("cleanup failure not visible")
	}
	fake.Lock()
	fake.failure = 0
	fake.Unlock()
	processSourceCleanup()
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("source not removed: %v", err)
	}
	if _, err := os.Stat(archive); err != nil {
		t.Fatal("archive removed")
	}
	var pending int
	db.QueryRow("SELECT COUNT(*) FROM source_cleanup").Scan(&pending)
	if pending != 0 {
		t.Fatal("completed cleanup not retired")
	}
	processSourceCleanup()
}

func TestKeepSourceSettingPersistsAndDefaultsToPreserve(t *testing.T) {
	settingsFixture(t)
	old := keepSourceFiles
	keepSourceFiles = true
	t.Cleanup(func() { keepSourceFiles = old })
	if !runtimeConfig().keepSourceFiles {
		t.Fatal("default must preserve")
	}
	keep := false
	if err := applySettings(&settingsInput{KeepSourceFiles: &keep}); err != nil {
		t.Fatal(err)
	}
	if runtimeConfig().keepSourceFiles || storedSettings()["keep_source_files"] != "0" {
		t.Fatal("false not persisted")
	}
	keepSourceFiles = true
	loadSettings()
	if runtimeConfig().keepSourceFiles {
		t.Fatal("setting lost on reload")
	}
}

func TestSourceCleanupArchiveCommitFailureAndRecovery(t *testing.T) {
	for _, keep := range []bool{true, false} {
		t.Run(map[bool]string{true: "preserve", false: "cleanup"}[keep], func(t *testing.T) {
			testDatabase(t)
			fake := newControlQbit(t, false)
			oldKeep, oldRoot := keepSourceFiles, qbitLocalDir
			keepSourceFiles = keep
			qbitLocalDir = t.TempDir()
			t.Cleanup(func() { keepSourceFiles, qbitLocalDir = oldKeep, oldRoot })
			source := filepath.Join(qbitLocalDir, "video.mp4")
			if err := os.WriteFile(source, []byte("video"), 0600); err != nil {
				t.Fatal(err)
			}
			result, err := db.Exec(`INSERT INTO items(title,link,anime_title,episode,status,downloader,task_id,task_submitted_at) VALUES('动画01','https://test.invalid/video','动画','1','下载中','qbittorrent',?,123)`, controlTestHash)
			if err != nil {
				t.Fatal(err)
			}
			id, _ := result.LastInsertId()
			ref := taskRef{ID: int(id), Downloader: "qbittorrent", TaskID: controlTestHash, SubmittedAt: 123}
			if _, err := db.Exec(`CREATE TRIGGER fail_source_commit BEFORE UPDATE OF status ON items WHEN NEW.status='已保存' BEGIN SELECT RAISE(FAIL,'simulated'); END`); err != nil {
				t.Fatal(err)
			}
			applyTaskUpdate(ref, taskState{Progress: 100, Files: []archiveFile{{Src: source, Rel: "video.mp4"}}})
			processSourceCleanup()
			if _, err := os.Stat(source); err != nil {
				t.Fatal("failed commit removed source")
			}
			fake.Lock()
			calls := len(fake.calls)
			fake.Unlock()
			if calls != 0 {
				t.Fatal("failed commit stopped task")
			}
			if _, err := db.Exec("DROP TRIGGER fail_source_commit"); err != nil {
				t.Fatal(err)
			}
			if err := recoverArchiveCommits(); err != nil {
				t.Fatal(err)
			}
			processSourceCleanup()
			_, err = os.Stat(source)
			if keep && err != nil {
				t.Fatal("default retention removed source")
			}
			if !keep && !os.IsNotExist(err) {
				t.Fatalf("recovery failed cleanup: %v", err)
			}
			var archive, status string
			db.QueryRow("SELECT file_path,status FROM items WHERE id=?", id).Scan(&archive, &status)
			if status != "已保存" {
				t.Fatalf("status: %s", status)
			}
			if data, err := os.ReadFile(archive); err != nil || string(data) != "video" {
				t.Fatalf("archive lost: %v %q", err, data)
			}
		})
	}
}

func TestSourceCleanupChangedConnectionOrSharedTaskKeepsSource(t *testing.T) {
	testDatabase(t)
	oldKeep, oldRoot := keepSourceFiles, qbitLocalDir
	keepSourceFiles = false
	qbitLocalDir = t.TempDir()
	t.Cleanup(func() { keepSourceFiles, qbitLocalDir = oldKeep, oldRoot })
	source := filepath.Join(qbitLocalDir, "source.mp4")
	archive := filepath.Join(t.TempDir(), "saved.mp4")
	for _, p := range []string{source, archive} {
		if err := os.WriteFile(p, []byte("media"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := db.Exec(`INSERT INTO items(status,downloader,task_id,file_path) VALUES('已保存','qbittorrent',?,?)`, controlTestHash, archive)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := result.LastInsertId()
	if err := queueSourceCleanup(taskRef{ID: int(id), Downloader: "qbittorrent", TaskID: controlTestHash}, archive, []archiveFile{{Src: source}}); err != nil {
		t.Fatal(err)
	}
	savedRoot := qbitLocalDir
	qbitLocalDir = t.TempDir()
	processSourceCleanup()
	if _, err := os.Stat(source); err != nil {
		t.Fatal("changed connection deleted source")
	}
	qbitLocalDir = savedRoot
	db.Exec(`INSERT INTO items(status,downloader,task_id) VALUES('下载中','qbittorrent',?)`, controlTestHash)
	processSourceCleanup()
	if _, err := os.Stat(source); err != nil {
		t.Fatal("shared task deleted source")
	}
	keepSourceFiles = true
	processSourceCleanup()
	var n int
	db.QueryRow("SELECT COUNT(*) FROM source_cleanup").Scan(&n)
	if n != 0 {
		t.Fatal("reenabling preservation did not cancel pending cleanup")
	}
}

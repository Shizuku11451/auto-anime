package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	raintorrent "github.com/cenkalti/rain/torrent"
)

func nativeLifecycleSession(t *testing.T, capacity int) (*nativeDownloader, raintorrent.Config) {
	t.Helper()
	raintorrent.DisableLogging()
	root := t.TempDir()
	t.Setenv("NATIVE_BT_DATA_DIR", root)
	cfg := raintorrent.DefaultConfig
	cfg.Database = filepath.Join(root, "session.db")
	cfg.DataDir = filepath.Join(root, "downloads")
	cfg.DataDirIncludesTorrentID = true
	cfg.ResumeOnStartup = false
	cfg.RPCEnabled = false
	cfg.DHTEnabled = false
	cfg.Host = "127.0.0.1"
	cfg.PortBegin = uint16(freeTCPPort(t))
	cfg.PortEnd = cfg.PortBegin + uint16(capacity)
	cfg.MaxOpenFiles = 0
	cfg.HealthCheckInterval = 24 * time.Hour
	cfg.HealthCheckTimeout = 24 * time.Hour
	session, err := raintorrent.NewSession(cfg)
	if err != nil {
		t.Fatal(err)
	}
	n := &nativeDownloader{session: session}
	old := nativeDownloads
	nativeDownloads = n
	t.Cleanup(func() { n.stop(); nativeDownloads = old })
	return n, cfg
}

func nativeLifecycleItem(t *testing.T, id int, hash, status string) taskRef {
	t.Helper()
	ref := taskRef{ID: id, Downloader: "native", TaskID: nativeTaskID(hash, id)}
	if _, err := db.Exec("UPDATE items SET status=?,downloader='native',task_id=? WHERE id=?", status, ref.TaskID, id); err != nil {
		t.Fatal(err)
	}
	return ref
}

func nativeLifecycleSeed(t *testing.T, n *nativeDownloader, ref taskRef, meta, data []byte) string {
	t.Helper()
	src := filepath.Join(nativeSourceRoot(), ref.TaskID, "episode.mkv")
	if err := os.MkdirAll(filepath.Dir(src), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := n.add(context.Background(), ref, "", meta); err != nil {
		t.Fatal(err)
	}
	waitNativeStatus(t, n.session.GetTorrent(ref.TaskID), raintorrent.Seeding, 5*time.Second)
	return src
}

func TestNativeCancelledTasksReleasePortsAndPreserveFiles(t *testing.T) {
	testDatabase(t)
	n, _ := nativeLifecycleSession(t, nativeDefaultPortRange)
	for i := 1; i <= nativeDefaultPortRange+2; i++ {
		hash := fmt.Sprintf("%040x", i)
		ref := taskRef{TaskID: nativeTaskID(hash, i)}
		if err := n.add(context.Background(), ref, "magnet:?xt=urn:btih:"+hash, nil); err != nil {
			t.Fatalf("task %d: %v", i, err)
		}
		file := filepath.Join(n.session.GetTorrent(ref.TaskID).Dir(), "keep.mkv")
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := n.remove(context.Background(), ref.TaskID); err != nil {
			t.Fatal(err)
		}
		if n.session.GetTorrent(ref.TaskID) != nil || !n.session.HasFreePort() {
			t.Fatal("cancelled task retained runtime resources")
		}
		if restored, err := n.session.LoadTorrent(ref.TaskID); err != nil || restored != nil {
			t.Fatalf("cancelled task retained resume state: %v", err)
		}
		if got, err := os.ReadFile(file); err != nil || string(got) != "keep" {
			t.Fatalf("cancel changed source: %q %v", got, err)
		}
	}
}

func TestNativeSameHashItemsPauseAndDeleteIndependently(t *testing.T) {
	testDatabase(t)
	n, _ := nativeLifecycleSession(t, 2)
	meta := buildNativeTestTorrent("episode.mkv", bytes.Repeat([]byte("x"), 16*1024))
	hash := torrentInfoHash(meta)
	refs := make([]taskRef, 2)
	for i := range refs {
		id := insertItem(t, fmt.Sprintf("https://source%d.test/episode.torrent", i))
		refs[i] = nativeLifecycleItem(t, id, hash, "下载中")
		if err := n.add(context.Background(), refs[i], "", meta); err != nil {
			t.Fatal(err)
		}
		waitNativeStatus(t, n.session.GetTorrent(refs[i].TaskID), raintorrent.Downloading, 5*time.Second)
	}
	if _, err := pauseItemDownload(refs[0].ID); err != nil {
		t.Fatal(err)
	}
	pollDownloaderTasks()
	if n.session.GetTorrent(refs[0].TaskID).Stats().Status != raintorrent.Stopped {
		t.Fatal("another item restarted paused torrent")
	}
	if _, err := n.control(context.Background(), refs[0], false); err != nil {
		t.Fatal(err)
	}
	if err := deleteItemRecords([]int{refs[0].ID}); err != nil {
		t.Fatal(err)
	}
	if n.session.GetTorrent(refs[0].TaskID) != nil || n.session.GetTorrent(refs[1].TaskID).Stats().Status != raintorrent.Downloading {
		t.Fatal("delete affected another item's torrent")
	}
}

func TestNativeCleanedSourceDoesNotBecomeSavedZeroFile(t *testing.T) {
	testDatabase(t)
	n, _ := nativeLifecycleSession(t, 1)
	oldKeep := keepSourceFiles
	keepSourceFiles = false
	t.Cleanup(func() { keepSourceFiles = oldKeep })
	data := bytes.Repeat([]byte("x"), 16*1024)
	meta := buildNativeTestTorrent("episode.mkv", data)
	hash := torrentInfoHash(meta)
	id := insertItem(t, "https://source.test/episode.torrent")
	ref := nativeLifecycleItem(t, id, hash, "下载中")
	src := nativeLifecycleSeed(t, n, ref, meta, data)
	pollDownloaderTasks()
	processSourceCleanup()
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source not cleaned: %v", err)
	}
	if n.session.GetTorrent(ref.TaskID) != nil {
		t.Fatal("cleanup retained completed torrent")
	}
	// Reuse the exact same ID to cover old tasks and retry paths, not just new IDs.
	if _, err := db.Exec("UPDATE items SET status='下载中' WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	if err := n.add(context.Background(), ref, "", meta); err != nil {
		t.Fatal(err)
	}
	waitNativeStatus(t, n.session.GetTorrent(ref.TaskID), raintorrent.Downloading, 5*time.Second)
	state, err := n.poll(context.Background(), ref)
	if err != nil || len(state.Files) != 0 || state.Progress != 0 {
		t.Fatalf("cleaned payload reported complete: %+v %v", state, err)
	}
	pollDownloaderTasks()
	item, err := readControlledItem(id)
	if err != nil || item.Status != "下载中" {
		t.Fatalf("invalid archive status: %+v %v", item, err)
	}
}

func TestNativeMissingOrResizedPayloadInvalidatesCompletion(t *testing.T) {
	for _, resize := range []bool{false, true} {
		t.Run(fmt.Sprint(resize), func(t *testing.T) {
			testDatabase(t)
			n, _ := nativeLifecycleSession(t, 1)
			data := bytes.Repeat([]byte("x"), 16*1024)
			meta := buildNativeTestTorrent("episode.mkv", data)
			ref := taskRef{TaskID: nativeTaskID(torrentInfoHash(meta), 1)}
			src := nativeLifecycleSeed(t, n, ref, meta, data)
			torrent := n.session.GetTorrent(ref.TaskID)
			if err := nativePause(torrent); err != nil {
				t.Fatal(err)
			}
			var err error
			if resize {
				err = os.Truncate(src, 0)
			} else {
				err = os.Remove(src)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := nativeStartTorrent(torrent); err != nil {
				t.Fatal(err)
			}
			waitNativeStatus(t, torrent, raintorrent.Downloading, 5*time.Second)
			if state := nativeTorrentState(torrent); len(state.Files) != 0 || state.Progress != 0 {
				t.Fatalf("recreated source reported complete: %+v", state)
			}
		})
	}
}

func TestNativePortPressurePreservesResumeAndRestart(t *testing.T) {
	testDatabase(t)
	n, cfg := nativeLifecycleSession(t, 1)
	data := bytes.Repeat([]byte("x"), 16*1024)
	meta := buildNativeTestTorrent("episode.mkv", data)
	hash := torrentInfoHash(meta)
	id := insertItem(t, "https://source.test/a.torrent")
	ref := nativeLifecycleItem(t, id, hash, "下载中")
	src := nativeLifecycleSeed(t, n, ref, meta, data)
	if err := nativePause(n.session.GetTorrent(ref.TaskID)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE items SET status='已暂停' WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	other := taskRef{TaskID: nativeTaskID(hash, id+1)}
	if err := n.add(context.Background(), other, "", meta); err != nil {
		t.Fatal(err)
	}
	if n.session.GetTorrent(ref.TaskID) != nil {
		t.Fatal("paused torrent retained port under pressure")
	}
	if got, err := os.ReadFile(src); err != nil || !bytes.Equal(got, data) {
		t.Fatal("unloading changed payload")
	}
	if err := n.remove(context.Background(), other.TaskID); err != nil {
		t.Fatal(err)
	}
	if err := n.session.Close(); err != nil {
		t.Fatal(err)
	}
	n.session = nil
	s, err := raintorrent.NewSession(cfg)
	if err != nil {
		t.Fatal(err)
	}
	n.session = s
	reconcileNativeSession(s)
	if s.GetTorrent(ref.TaskID) == nil || s.GetTorrent(ref.TaskID).Stats().Status != raintorrent.Stopped {
		t.Fatal("paused state not restored after restart")
	}
	if _, err := n.control(context.Background(), ref, false); err != nil {
		t.Fatal(err)
	}
	waitNativeStatus(t, s.GetTorrent(ref.TaskID), raintorrent.Seeding, 5*time.Second)
}

func TestNativeActiveCapacityWaitsWithoutEvicting(t *testing.T) {
	testDatabase(t)
	n, _ := nativeLifecycleSession(t, 1)
	meta := buildNativeTestTorrent("episode.mkv", bytes.Repeat([]byte("x"), 16*1024))
	hash := torrentInfoHash(meta)
	id := insertItem(t, "magnet:?xt=urn:btih:"+hash)
	ref := nativeLifecycleItem(t, id, hash, "下载中")
	if err := n.add(context.Background(), ref, "", meta); err != nil {
		t.Fatal(err)
	}
	otherID := insertItem(t, "magnet:?xt=urn:btih:"+hash+"&dn=other")
	other := nativeLifecycleItem(t, otherID, hash, "下载中")
	if err := n.add(context.Background(), other, "", meta); !errors.Is(err, raintorrent.ErrNoFreePort) {
		t.Fatalf("capacity error: %v", err)
	}
	state, err := n.poll(context.Background(), other)
	if err != nil || !state.Pending || n.session.GetTorrent(ref.TaskID) == nil {
		t.Fatalf("active task evicted or waiting task failed: %+v %v", state, err)
	}
	if _, err := pauseItemDownload(id); err != nil {
		t.Fatal(err)
	}
	if _, err := n.poll(context.Background(), other); err != nil || n.session.GetTorrent(other.TaskID) == nil {
		t.Fatalf("waiting task did not acquire released capacity: %v", err)
	}
}

func TestNativeSavedHistoryExceedsPortPoolAndRestarts(t *testing.T) {
	testDatabase(t)
	n, cfg := nativeLifecycleSession(t, 1)
	oldKeep := keepSourceFiles
	keepSourceFiles = true
	t.Cleanup(func() { keepSourceFiles = oldKeep })
	data := bytes.Repeat([]byte("x"), 16*1024)
	meta := buildNativeTestTorrent("episode.mkv", data)
	hash := torrentInfoHash(meta)
	var refs []taskRef
	var sources []string
	for i := 0; i < nativeDefaultPortRange+2; i++ {
		id := insertItem(t, fmt.Sprintf("https://source.test/%d.torrent", i))
		ref := nativeLifecycleItem(t, id, hash, "下载中")
		sources = append(sources, nativeLifecycleSeed(t, n, ref, meta, data))
		if _, err := db.Exec("UPDATE items SET status='已保存' WHERE id=?", id); err != nil {
			t.Fatal(err)
		}
		refs = append(refs, ref)
	}
	if err := n.session.Close(); err != nil {
		t.Fatal(err)
	}
	n.session = nil
	s, err := raintorrent.NewSession(cfg)
	if err != nil {
		t.Fatal(err)
	}
	n.session = s
	if len(s.ListTorrents()) != 1 {
		t.Fatal("restart exceeded runtime capacity")
	}
	// More resume records than ports must not mark excess records corrupt.
	if err := s.CleanDatabase(); err != nil {
		t.Fatal(err)
	}
	for i, ref := range refs {
		for _, loaded := range s.ListTorrents() {
			if err := s.UnloadTorrent(loaded.ID()); err != nil {
				t.Fatal(err)
			}
		}
		torrent, err := s.LoadTorrent(ref.TaskID)
		if err != nil || torrent == nil {
			t.Fatalf("resume record lost: %s %v", ref.TaskID, err)
		}
		if got, err := os.ReadFile(sources[i]); err != nil || !bytes.Equal(got, data) {
			t.Fatalf("saved source changed: %v", err)
		}
	}
}

func TestNativeLegacyPausedOwnerIsNotRestartedByPoll(t *testing.T) {
	testDatabase(t)
	n, _ := nativeLifecycleSession(t, 1)
	meta := buildNativeTestTorrent("episode.mkv", bytes.Repeat([]byte("x"), 16*1024))
	hash := torrentInfoHash(meta)
	paused := insertItem(t, "https://one.test/a.torrent")
	active := insertItem(t, "https://two.test/a.torrent")
	for _, id := range []int{paused, active} {
		if _, err := db.Exec("UPDATE items SET status='下载中',downloader='native',task_id=? WHERE id=?", hash, id); err != nil {
			t.Fatal(err)
		}
	}
	ref := taskRef{ID: active, Downloader: "native", TaskID: hash}
	if err := n.add(context.Background(), ref, "", meta); err != nil {
		t.Fatal(err)
	}
	if err := nativePause(n.session.GetTorrent(hash)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE items SET status='已暂停' WHERE id=?", paused); err != nil {
		t.Fatal(err)
	}
	if _, err := n.poll(context.Background(), ref); err == nil {
		t.Fatal("ambiguous paused owner was not reported")
	}
	pollDownloaderTasks()
	var message string
	if err := db.QueryRow("SELECT task_error FROM items WHERE id=?", active).Scan(&message); err != nil || message != errNativeSharedTask.Error() {
		t.Fatalf("actionable legacy warning not persisted: %q %v", message, err)
	}
	if n.session.GetTorrent(hash).Stats().Status != raintorrent.Stopped {
		t.Fatal("poll restarted paused shared torrent")
	}
}

func TestNativeSubmitPersistsDistinctIdentityWhileAtCapacity(t *testing.T) {
	testDatabase(t)
	n, _ := nativeLifecycleSession(t, 1)
	hash := fmt.Sprintf("%040x", 123)
	link := "magnet:?xt=urn:btih:" + hash
	for i := range 2 {
		id := insertItem(t, fmt.Sprintf("%s&dn=%d", link, i))
		submitNativeDownload(id, link)
		item, err := readControlledItem(id)
		if err != nil || item.Status != "下载中" || item.Downloader != "native" || item.TaskID != nativeTaskID(hash, id) {
			t.Fatalf("submitted identity lost: %+v %v", item, err)
		}
	}
	if len(n.session.ListTorrents()) != 1 {
		t.Fatal("submission evicted an active task")
	}
}

func TestNativeLegacySharedTaskRefusesIndependentControl(t *testing.T) {
	testDatabase(t)
	n, _ := nativeLifecycleSession(t, 1)
	meta := buildNativeTestTorrent("episode.mkv", bytes.Repeat([]byte("x"), 16*1024))
	hash := torrentInfoHash(meta)
	ids := []int{insertItem(t, "https://one.test/a.torrent"), insertItem(t, "https://two.test/a.torrent")}
	for _, id := range ids {
		if _, err := db.Exec("UPDATE items SET status='下载中',downloader='native',task_id=? WHERE id=?", hash, id); err != nil {
			t.Fatal(err)
		}
	}
	ref := taskRef{ID: ids[0], Downloader: "native", TaskID: hash}
	if err := n.add(context.Background(), ref, "", meta); err != nil {
		t.Fatal(err)
	}
	if _, err := pauseItemDownload(ids[0]); err == nil {
		t.Fatal("ambiguous legacy control accepted")
	}
	if err := deleteItemRecords([]int{ids[0]}); err != nil {
		t.Fatal(err)
	}
	if n.session.GetTorrent(hash) == nil {
		t.Fatal("removing a duplicate stopped the remaining owner")
	}
	ref.ID = ids[1]
	if _, err := n.control(context.Background(), ref, true); err != nil {
		t.Fatal(err)
	}
}

func TestNativeConcurrentDuplicateAddSerializesSession(t *testing.T) {
	testDatabase(t)
	n, _ := nativeLifecycleSession(t, 1)
	meta := buildNativeTestTorrent("episode.mkv", bytes.Repeat([]byte("x"), 16*1024))
	ref := taskRef{TaskID: nativeTaskID(torrentInfoHash(meta), 1)}
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := n.add(context.Background(), ref, "", meta); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if len(n.session.ListTorrents()) != 1 {
		t.Fatal("duplicate runtime torrent")
	}
}

func TestNativeFinishedRequiresCompletedBytes(t *testing.T) {
	stats := raintorrent.Stats{Status: raintorrent.Seeding}
	stats.Bytes.Total = 100
	if nativeFinished(stats) {
		t.Fatal("stale Seeding latch treated as complete")
	}
	stats.Bytes.Completed = 100
	if !nativeFinished(stats) {
		t.Fatal("verified complete torrent rejected")
	}
}

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func awaitQbitTest(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for qBittorrent lifecycle")
	}
}

func TestEmbeddedQbitStartupIsSingleAndCancellable(t *testing.T) {
	var manager embeddedQBittorrentManager
	entered := make(chan struct{})
	finished := make(chan struct{})
	var starts atomic.Int32
	launch := func(ctx context.Context) (*managedQBittorrent, error) {
		starts.Add(1)
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	go func() {
		defer close(finished)
		if _, err := manager.connection(launch); !errors.Is(err, context.Canceled) {
			t.Errorf("startup was not cancelled: %v", err)
		}
	}()
	awaitQbitTest(t, entered)
	t.Cleanup(manager.stop)
	checked := make(chan struct{})
	go func() {
		defer close(checked)
		if manager.healthy() {
			t.Error("starting process reported healthy")
		}
		for i := 0; i < 10; i++ {
			if _, err := manager.connection(launch); err == nil {
				t.Error("concurrent caller received an unready connection")
			}
		}
	}()
	awaitQbitTest(t, checked)
	stopped := make(chan struct{})
	go func() { manager.stop(); close(stopped) }()
	awaitQbitTest(t, stopped)
	awaitQbitTest(t, finished)
	if _, err := manager.connection(launch); err == nil || starts.Load() != 1 {
		t.Fatal("shutdown allowed a second launch")
	}
}

func TestEmbeddedQbitSupervisorRestartsWithBackoff(t *testing.T) {
	var manager embeddedQBittorrentManager
	var starts atomic.Int32
	launched := make(chan *managedQBittorrent, 2)
	launch := func(context.Context) (*managedQBittorrent, error) {
		starts.Add(1)
		p := &managedQBittorrent{done: make(chan struct{})}
		launched <- p
		return p, nil
	}
	first, err := manager.connection(launch)
	if err != nil || !manager.healthy() {
		t.Fatalf("initial launch: %v", err)
	}
	<-launched
	close(first.done)
	if manager.healthy() {
		t.Fatal("dead process reported healthy")
	}
	if _, err := manager.connection(launch); err == nil || starts.Load() != 1 {
		t.Fatal("crash skipped restart backoff")
	}
	manager.Lock()
	if manager.failures != 1 || time.Until(manager.retryAt) <= 0 {
		t.Error("missing crash backoff")
	}
	manager.retryAt = time.Time{}
	manager.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	supervised := make(chan struct{})
	go func() { defer close(supervised); manager.supervise(ctx, time.Millisecond, launch) }()
	var second *managedQBittorrent
	select {
	case second = <-launched:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("supervisor did not restart the process")
	}
	cancel()
	awaitQbitTest(t, supervised)
	if second == first || !manager.healthy() || starts.Load() != 2 {
		t.Fatal("restart did not replace the process")
	}
	close(second.done)
	if _, err := manager.connection(launch); err == nil {
		t.Fatal("second crash skipped backoff")
	}
	manager.Lock()
	if manager.failures != 2 {
		t.Error("brief successful login reset crash count")
	}
	// A long stable run should restart the backoff sequence at two seconds.
	second.readyAt = time.Now().Add(-6 * time.Minute)
	second.ready.Store(true)
	manager.observeExit(time.Now())
	if manager.failures != 1 {
		t.Error("stable uptime did not reset crash count")
	}
	manager.Unlock()
	manager.stop()
	if _, err := manager.connection(launch); err == nil || starts.Load() != 2 {
		t.Fatal("stopped manager restarted")
	}
}

func TestEmbeddedQbitFailedLaunchesRemainBounded(t *testing.T) {
	var manager embeddedQBittorrentManager
	var starts int
	launch := func(context.Context) (*managedQBittorrent, error) {
		starts++
		return nil, errors.New("test startup failure")
	}
	for i, delay := range []time.Duration{2, 4, 8, 16, 32, 60, 60, 60} {
		manager.retryAt = time.Time{}
		before := time.Now()
		if _, err := manager.connection(launch); err == nil {
			t.Fatal("failed startup accepted")
		}
		if manager.healthy() || manager.retryAt.Before(before.Add(delay*time.Second)) || embeddedQbitRetryDelay(manager.failures) != delay*time.Second {
			t.Fatalf("attempt %d has invalid health or backoff", i+1)
		}
		if _, err := manager.connection(launch); err == nil || starts != i+1 {
			t.Fatal("failed launch bypassed backoff")
		}
	}
	manager.stop()
}

func TestEmbeddedQbitRestoreGracePreservesTaskUntilVisible(t *testing.T) {
	testDatabase(t)
	var visible atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if visible.Load() {
			fmt.Fprintf(w, `[{"hash":%q,"state":"downloading","progress":0.42}]`, controlTestHash)
		} else {
			fmt.Fprint(w, `[]`)
		}
	}))
	defer server.Close()
	p := &managedQBittorrent{done: make(chan struct{}), readyAt: time.Now()}
	p.ready.Store(true)
	client := &qbitClient{http: server.Client(), config: qbitConnection{URL: server.URL}, embedded: p}
	for _, hash := range []string{controlTestHash, ""} {
		id := insertItem(t, fmt.Sprintf("https://example.test/%d.torrent", len(hash)))
		if _, err := db.Exec("UPDATE items SET status='下载中',downloader='builtin',task_id=?,progress=37,task_submitted_at=? WHERE id=?", hash, time.Now().Add(-time.Hour).Unix(), id); err != nil {
			t.Fatal(err)
		}
		ref := controlItem(t, id).taskRef
		state, err := qbitPoll(client, ref)
		if err != nil || !state.Pending || state.Fail != "" {
			t.Fatalf("restoring task failed early: %+v %v", state, err)
		}
		applyTaskUpdateLocked(ref, state)
		item := controlItem(t, id)
		if item.Status != "下载中" || item.Progress != 37 || item.TaskID != hash || item.Downloader != "builtin" {
			t.Fatal("restore changed persisted identity or progress", item)
		}
		visible.Store(true)
		state, err = qbitPoll(client, ref)
		if err != nil || state.Pending || state.Progress != 42 {
			t.Fatalf("visible task did not recover: %+v %v", state, err)
		}
		applyTaskUpdateLocked(ref, state)
		if item = controlItem(t, id); item.TaskID != controlTestHash || item.Progress != 42 {
			t.Fatal("recovery failed to track original task", item)
		}
		visible.Store(false)
	}
	old := taskRef{TaskID: controlTestHash, SubmittedAt: time.Now().Add(-time.Hour).Unix()}
	p.readyAt = time.Now().Add(-embeddedQbitRestoreGrace - time.Second)
	if state, err := qbitPoll(client, old); err != nil || state.Fail == "" {
		t.Fatalf("missing task never failed after grace: %+v %v", state, err)
	}
	p.readyAt = time.Now()
	external := &qbitClient{http: server.Client(), config: client.config}
	if state, err := qbitPoll(external, old); err != nil || state.Fail == "" {
		t.Fatalf("builtin grace leaked into external downloader: %+v %v", state, err)
	}
	close(p.done)
	if _, err := qbitPoll(client, old); err == nil {
		t.Fatal("query from an exited process was accepted")
	}
}

func TestQbitTagRecoveryRejectsAmbiguity(t *testing.T) {
	testDatabase(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[{"hash":%q,"state":"downloading"},{"hash":%q,"state":"downloading"}]`, controlTestHash, controlTestHash)
	}))
	defer server.Close()
	id := insertItem(t, "https://example.test/ambiguous.torrent")
	setSubmitted(id, "builtin", "", "已发送到 qBittorrent")
	client := &qbitClient{http: server.Client(), config: qbitConnection{URL: server.URL}}
	if _, err := qbitPoll(client, controlItem(t, id).taskRef); err == nil {
		t.Fatal("ambiguous tag was bound to an arbitrary task")
	}
	if item := controlItem(t, id); item.TaskID != "" || item.Status != "已发送到 qBittorrent" {
		t.Fatal("ambiguous query changed the task", item)
	}
}

func TestRecoverDownloadsKeepsQbitWithoutHashAndPausedTasks(t *testing.T) {
	testDatabase(t)
	for _, downloader := range []string{"builtin", "qbittorrent", "direct"} {
		for _, status := range []string{"下载中", "已暂停", "继续中"} {
			id := insertItem(t, "https://example.test/"+downloader+status)
			if _, err := db.Exec("UPDATE items SET downloader=?,task_id='',status=?,progress=37 WHERE id=?", downloader, status, id); err != nil {
				t.Fatal(err)
			}
			if _, err := recoverDownloads(db); err != nil {
				t.Fatal(err)
			}
			want := status
			if downloader == "direct" && status == "下载中" {
				want = "下载失败"
			}
			if item := controlItem(t, id); item.Status != want || item.Downloader != downloader || item.Progress != 37 || item.TaskID != "" {
				t.Fatal("startup recovery changed a tracked or paused task", item)
			}
		}
	}
}

func TestQbitStartupRequestsHonorCancellation(t *testing.T) {
	for _, operation := range []string{"login", "cleanup"} {
		t.Run(operation, func(t *testing.T) {
			entered := make(chan struct{})
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.Copy(io.Discard, r.Body)
				close(entered)
				select {
				case <-r.Context().Done():
				case <-release:
				}
			}))
			defer server.Close()
			defer close(release)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				config := qbitConnection{URL: server.URL}
				var err error
				if operation == "login" {
					_, err = loginQBittorrentContext(ctx, config, time.Minute)
				} else {
					err = cleanupEmbeddedQbitOrphansContext(ctx, &qbitClient{http: server.Client(), config: config})
				}
				if !errors.Is(err, context.Canceled) {
					t.Errorf("request ignored cancellation: %v", err)
				}
			}()
			awaitQbitTest(t, entered)
			cancel()
			awaitQbitTest(t, finished)
		})
	}
}

package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const controlTestHash = "1234567890123456789012345678901234567890"

type controlQbitFake struct {
	sync.Mutex
	server                 *httptest.Server
	state                  string
	legacy                 bool
	visible                bool
	duplicates             bool
	failure                int
	applyBeforeFailure     bool
	adds                   int
	calls                  []string
	addStarted, addRelease chan struct{}
}

func newControlQbit(t *testing.T, legacy bool) *controlQbitFake {
	t.Helper()
	f := &controlQbitFake{state: "downloading", visible: true, legacy: legacy}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		f.Lock()
		defer f.Unlock()
		f.calls = append(f.calls, r.URL.Path)
		switch r.URL.Path {
		case "/api/v2/auth/login":
			w.WriteHeader(http.StatusNoContent)
		case "/api/v2/torrents/info":
			if value := r.URL.Query().Get("hashes"); value != "" && value != controlTestHash {
				t.Errorf("unexpected hash filter: %q", value)
			}
			list := []map[string]any{}
			if f.visible {
				list = append(list, map[string]any{"hash": controlTestHash, "state": f.state, "progress": 0.37, "tags": "auto-anime-1"})
			}
			if f.duplicates {
				list = append(list, list[0])
			}
			json.NewEncoder(w).Encode(list)
		case "/api/v2/torrents/add":
			f.adds++
			if f.addStarted != nil {
				close(f.addStarted)
				wait := f.addRelease
				f.Unlock()
				<-wait
				f.Lock()
			}
			fmt.Fprint(w, `{"success_count":1,"failure_count":0,"pending_count":0}`)
		case "/api/v2/torrents/stop", "/api/v2/torrents/start", "/api/v2/torrents/pause", "/api/v2/torrents/resume":
			if r.Method != http.MethodPost || r.Form.Get("hashes") != controlTestHash {
				t.Errorf("unsafe control: %s %v", r.Method, r.Form)
			}
			if f.legacy && (strings.HasSuffix(r.URL.Path, "/stop") || strings.HasSuffix(r.URL.Path, "/start")) {
				w.WriteHeader(404)
				return
			}
			if f.failure != 0 && !f.applyBeforeFailure {
				w.WriteHeader(f.failure)
				return
			}
			f.state = "downloading"
			if strings.HasSuffix(r.URL.Path, "/stop") || strings.HasSuffix(r.URL.Path, "/pause") {
				f.state = "stoppedDL"
			}
			if f.failure != 0 {
				w.WriteHeader(f.failure)
				return
			}
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected qbit request: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	oldURL, oldUser, oldPassword := qbitURL, qbitUser, qbitPassword
	qbitURL, qbitUser, qbitPassword = f.server.URL, "test-user", "test-password"
	t.Cleanup(func() { f.server.Close(); qbitURL, qbitUser, qbitPassword = oldURL, oldUser, oldPassword })
	return f
}

func controlItem(t *testing.T, id int) controlledItem {
	t.Helper()
	item, err := readControlledItem(id)
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func waitDownloadDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(4 * time.Second):
		t.Fatal("download worker did not finish")
	}
}

func TestPausePendingTasksSurvivesReopen(t *testing.T) {
	testDatabase(t)
	db.Exec("INSERT INTO feeds(id,name,url,enabled,auto_download) VALUES(1,'自动订阅','https://example.test/rss',1,1)")
	var ids []int
	for _, status := range []string{"待下载", "等待下载", "等待下载器", "等待继续"} {
		id := insertItem(t, "https://example.test/"+status)
		db.Exec("UPDATE items SET feed_id=1,status=? WHERE id=?", status, id)
		if status == "等待继续" {
			db.Exec("UPDATE items SET downloader='qbittorrent',task_id=?,progress=37 WHERE id=?", controlTestHash, id)
		}
		if changed, err := pauseItemDownload(id); err != nil || !changed {
			t.Fatalf("pause %s: %v %v", status, changed, err)
		}
		if changed, err := pauseItemDownload(id); err != nil || changed {
			t.Fatal("pause not idempotent", err)
		}
		ids = append(ids, id)
	}
	db.Close()
	initDB()
	recovered, err := recoverDownloads(db)
	if err != nil || len(recovered) != 0 {
		t.Fatalf("paused tasks auto-resumed: %v %v", recovered, err)
	}
	for _, id := range ids {
		downloadItem(id) // RSS / scheduled startup must not undo a manual pause.
		if item := controlItem(t, id); item.Status != "已暂停" {
			t.Fatal(item)
		}
	}
	last := controlItem(t, ids[len(ids)-1])
	if last.TaskID != controlTestHash || last.Progress != 37 {
		t.Fatal("pause reset resume identity", last)
	}
}

func TestPauseDirectCleansPartialAndResumeDownloadsOnce(t *testing.T) {
	testDatabase(t)
	started, cancelled := make(chan struct{}), make(chan struct{})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		if requests.Add(1) == 1 {
			w.Header().Set("Content-Length", "1048576")
			fmt.Fprint(w, strings.Repeat("x", 32768))
			w.(http.Flusher).Flush()
			close(started)
			<-r.Context().Done()
			close(cancelled)
			return
		}
		fmt.Fprint(w, "completed media")
	}))
	defer server.Close()
	id := insertItem(t, server.URL+"/episode.mp4")
	done := make(chan struct{})
	go func() { downloadItem(id); close(done) }()
	waitDownloadDone(t, started)
	if changed, err := pauseItemDownload(id); err != nil || !changed {
		t.Fatal(changed, err)
	}
	waitDownloadDone(t, done)
	waitDownloadDone(t, cancelled)
	if item := controlItem(t, id); item.Status != "已暂停" || item.Progress != 0 {
		t.Fatal(item)
	}
	filepath.WalkDir(downloadDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(entry.Name(), ".partial-") || strings.HasSuffix(path, ".mp4") {
			t.Errorf("incomplete archive remains: %s", path)
		}
		return nil
	})
	applyTaskUpdateLocked(taskRef{ID: id}, taskState{Fail: "late failure"})
	if item := controlItem(t, id); item.Status != "已暂停" {
		t.Fatal("late update overwrote pause", item)
	}
	if changed, err := queueItemDownload(id, true); err != nil || !changed {
		t.Fatal(changed, err)
	}
	runQueuedDownload(id)
	if item := controlItem(t, id); item.Status != "已保存" {
		t.Fatal(item)
	}
	var path string
	db.QueryRow("SELECT file_path FROM items WHERE id=?", id).Scan(&path)
	body, err := os.ReadFile(path)
	if err != nil || string(body) != "completed media" || requests.Load() != 2 {
		t.Fatal(string(body), requests.Load(), err)
	}
}

func TestQbitPauseResumePreservesTaskAndRespectsCapacity(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(fmt.Sprint("legacy=", legacy), func(t *testing.T) {
			testDatabase(t)
			fake := newControlQbit(t, legacy)
			oldLimit := configuredDownloads.Load()
			configuredDownloads.Store(1)
			t.Cleanup(func() { configuredDownloads.Store(oldLimit) })
			id := insertItem(t, "magnet:?xt=urn:btih:"+controlTestHash)
			db.Exec("UPDATE items SET status='下载中',downloader='qbittorrent',task_id=?,progress=37 WHERE id=?", controlTestHash, id)
			setTransfer(id, 1024, 37, time.Minute)
			if changed, err := pauseItemDownload(id); err != nil || !changed {
				t.Fatal(changed, err)
			}
			item := controlItem(t, id)
			if item.Status != "已暂停" || item.TaskID != controlTestHash || item.Progress != 37 {
				t.Fatal(item)
			}
			if speed, _ := transferSnapshot(id, item.Status, item.Progress); speed != 0 {
				t.Fatal("paused speed", speed)
			}
			applyTaskUpdateLocked(item.taskRef, taskState{Progress: 70, DownloadSpeed: 1234})
			if item := controlItem(t, id); item.Status != "已暂停" || item.Progress != 37 {
				t.Fatal("late poll changed paused task", item)
			}
			blocker := insertItem(t, "https://example.test/blocker")
			db.Exec("UPDATE items SET status='下载中',downloader='aria2',task_id='other' WHERE id=?", blocker)
			if changed, err := queueItemDownload(id, true); err != nil || !changed {
				t.Fatal(changed, err)
			}
			if changed, err := queueItemDownload(id, true); err != nil || changed {
				t.Fatal("resume was duplicated", err)
			}
			if claimed, waiting, err := claimDownload(id); err != nil || claimed || !waiting {
				t.Fatal(claimed, waiting, err)
			}
			item = controlItem(t, id)
			if item.Status != "等待继续" || item.TaskID != controlTestHash || item.Progress != 37 {
				t.Fatal(item)
			}
			downloadMode = "direct" // Default-mode changes cannot redirect an existing task.
			db.Exec("UPDATE items SET status='已保存' WHERE id=?", blocker)
			runQueuedDownload(id)
			item = controlItem(t, id)
			if item.Status != "下载中" || item.TaskID != controlTestHash || item.Downloader != "qbittorrent" || item.Progress != 37 {
				t.Fatal(item)
			}
			fake.Lock()
			defer fake.Unlock()
			if fake.adds != 0 || fake.state != "downloading" {
				t.Fatal("resume submitted another task", fake.adds, fake.state)
			}
			if legacy && !strings.Contains(strings.Join(fake.calls, " "), "/torrents/resume") {
				t.Fatal("missing legacy fallback", fake.calls)
			}
		})
	}
}

func TestPauseDuringQbitSubmissionControlsAcceptedTask(t *testing.T) {
	testDatabase(t)
	fake := newControlQbit(t, false)
	fake.addStarted, fake.addRelease = make(chan struct{}), make(chan struct{})
	downloadMode = "qbittorrent"
	id := insertItem(t, "magnet:?xt=urn:btih:"+controlTestHash)
	done := make(chan struct{})
	go func() { downloadItem(id); close(done) }()
	waitDownloadDone(t, fake.addStarted)
	paused := make(chan struct{})
	var pauseErr error
	go func() { _, pauseErr = pauseItemDownload(id); close(paused) }()
	close(fake.addRelease)
	waitDownloadDone(t, done)
	waitDownloadDone(t, paused)
	if pauseErr != nil {
		t.Fatal(pauseErr)
	}
	item := controlItem(t, id)
	if item.Status != "已暂停" || item.TaskID != controlTestHash {
		t.Fatal(item)
	}
	fake.Lock()
	defer fake.Unlock()
	if fake.adds != 1 || fake.state != "stoppedDL" {
		t.Fatal(fake.adds, fake.state)
	}
}

func TestQbitControlFailuresDoNotLieOrResubmit(t *testing.T) {
	testDatabase(t)
	fake := newControlQbit(t, false)
	id := insertItem(t, "magnet:?xt=urn:btih:"+controlTestHash)
	db.Exec("UPDATE items SET status='下载中',downloader='qbittorrent',task_id=?,progress=37 WHERE id=?", controlTestHash, id)
	for _, code := range []int{403, 500} {
		fake.failure = code
		if changed, err := pauseItemDownload(id); err == nil || changed {
			t.Fatal("failed pause reported success", code)
		}
		if item := controlItem(t, id); item.Status != "下载中" || item.Progress != 37 || item.TaskID != controlTestHash {
			t.Fatal(item)
		}
	}
	if strings.Contains(strings.Join(fake.calls, " "), "/torrents/pause") {
		t.Fatal("auth/server failure used legacy fallback")
	}
	fake.failure = 0
	if _, err := pauseItemDownload(id); err != nil {
		t.Fatal(err)
	}
	fake.failure = 500
	if _, err := queueItemDownload(id, true); err != nil {
		t.Fatal(err)
	}
	runQueuedDownload(id)
	if item := controlItem(t, id); item.Status != "继续中" {
		t.Fatal("unknown resume outcome freed capacity", item)
	}
	pollDownloaderTasks()
	if item := controlItem(t, id); item.Status != "已暂停" || item.TaskID != controlTestHash || item.Progress != 37 {
		t.Fatal(item)
	}
	var message string
	db.QueryRow("SELECT task_error FROM items WHERE id=?", id).Scan(&message)
	if !strings.HasPrefix(message, "继续请求未确认") || fake.adds != 0 {
		t.Fatal(message, fake.adds)
	}
}

func TestLostResumeResponseKeepsSlotUntilStateConfirmed(t *testing.T) {
	testDatabase(t)
	fake := newControlQbit(t, false)
	fake.state, fake.failure, fake.applyBeforeFailure = "stoppedDL", 500, true
	oldLimit := configuredDownloads.Load()
	configuredDownloads.Store(1)
	t.Cleanup(func() { configuredDownloads.Store(oldLimit) })
	id := insertItem(t, "magnet:?xt=urn:btih:"+controlTestHash)
	db.Exec("UPDATE items SET status='已暂停',downloader='qbittorrent',task_id=?,progress=37 WHERE id=?", controlTestHash, id)
	if _, err := queueItemDownload(id, true); err != nil {
		t.Fatal(err)
	}
	runQueuedDownload(id)
	if item := controlItem(t, id); item.Status != "继续中" {
		t.Fatal(item)
	}
	next := insertItem(t, "https://example.test/waiting")
	db.Exec("UPDATE items SET status='等待下载' WHERE id=?", next)
	if claimed, waiting, err := claimDownload(next); claimed || !waiting || err != nil {
		t.Fatal("uncertain resume released capacity", claimed, waiting, err)
	}
	pollDownloaderTasks()
	if item := controlItem(t, id); item.Status != "下载中" || item.TaskID != controlTestHash {
		t.Fatal("accepted resume was reported paused", item)
	}
	if fake.adds != 0 {
		t.Fatal("resume duplicated a task")
	}
}

func TestQbitControlDiscoversOnlyUnambiguousTask(t *testing.T) {
	testDatabase(t)
	fake := newControlQbit(t, false)
	id := insertItem(t, "https://example.test/unknown.torrent")
	db.Exec("UPDATE items SET status='已发送到 qBittorrent',downloader='qbittorrent',task_submitted_at=? WHERE id=?", time.Now().Unix(), id)
	fake.visible = false
	if _, err := pauseItemDownload(id); err == nil || !strings.Contains(err.Error(), "仍在接收") {
		t.Fatal(err)
	}
	fake.visible, fake.duplicates = true, true
	if _, err := pauseItemDownload(id); err == nil {
		t.Fatal("ambiguous tag controlled")
	}
	fake.duplicates = false
	if _, err := pauseItemDownload(id); err != nil {
		t.Fatal(err)
	}
	if item := controlItem(t, id); item.TaskID != controlTestHash || item.Status != "已暂停" {
		t.Fatal(item)
	}
	db.Exec("UPDATE items SET status='下载中',task_id='all' WHERE id=?", id)
	before := len(fake.calls)
	if _, err := pauseItemDownload(id); err == nil || len(fake.calls) != before+1 { // Login only; no global info/control.
		t.Fatal("invalid hash reached control", err, fake.calls[before:])
	}
}

func TestAria2PauseFollowsAndPersistsPayloadGID(t *testing.T) {
	testDatabase(t)
	state := "active"
	var controls, queries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		var gid string
		json.Unmarshal(in.Params[0], &gid)
		if in.Method == "aria2.tellStatus" {
			queries = append(queries, gid)
			if gid == "parent" {
				fmt.Fprint(w, `{"result":{"status":"complete","followedBy":["payload"]}}`)
				return
			}
			fmt.Fprintf(w, `{"result":{"status":%q,"completedLength":"37","totalLength":"100"}}`, state)
			return
		}
		controls = append(controls, in.Method+":"+gid)
		if in.Method == "aria2.pause" {
			state = "paused"
		} else if in.Method == "aria2.unpause" {
			state = "active"
		} else {
			t.Errorf("unexpected method %s", in.Method)
		}
		json.NewEncoder(w).Encode(map[string]any{"result": gid})
	}))
	defer server.Close()
	aria2URL, aria2Token = server.URL, ""
	id := insertItem(t, "magnet:?xt=urn:btih:"+controlTestHash)
	db.Exec("UPDATE items SET status='下载中',downloader='aria2',task_id='parent',progress=37 WHERE id=?", id)
	if _, err := pauseItemDownload(id); err != nil {
		t.Fatal(err)
	}
	if item := controlItem(t, id); item.Status != "已暂停" || item.TaskID != "payload" {
		t.Fatal(item)
	}
	if _, err := queueItemDownload(id, true); err != nil {
		t.Fatal(err)
	}
	runQueuedDownload(id)
	if !reflect.DeepEqual(controls, []string{"aria2.pause:payload", "aria2.unpause:payload"}) || !reflect.DeepEqual(queries, []string{"parent", "payload", "payload"}) {
		t.Fatal(controls, queries)
	}
}

func TestTransmissionControlsValidateRPCResult(t *testing.T) {
	testDatabase(t)
	state, fail := 4, true
	var controls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Transmission-Session-Id") != "test-session" {
			w.Header().Set("X-Transmission-Session-Id", "test-session")
			w.WriteHeader(409)
			return
		}
		var in struct {
			Method    string `json:"method"`
			Arguments struct {
				IDs []int `json:"ids"`
			} `json:"arguments"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		if !reflect.DeepEqual(in.Arguments.IDs, []int{12}) {
			t.Errorf("unsafe torrent IDs: %v", in.Arguments.IDs)
		}
		if in.Method == "torrent-get" {
			fmt.Fprintf(w, `{"result":"success","arguments":{"torrents":[{"id":12,"status":%d,"percentDone":0.37}]}}`, state)
			return
		}
		controls = append(controls, in.Method)
		if fail {
			fmt.Fprint(w, `{"result":"permission denied"}`)
			return
		}
		if in.Method == "torrent-stop" {
			state = 0
		} else if in.Method == "torrent-start" {
			state = 4
		} else {
			t.Errorf("unexpected method: %s", in.Method)
		}
		fmt.Fprint(w, `{"result":"success"}`)
	}))
	defer server.Close()
	oldURL := transmissionURL
	transmissionURL = server.URL
	t.Cleanup(func() { transmissionURL = oldURL })
	id := insertItem(t, "magnet:?xt=urn:btih:"+controlTestHash)
	db.Exec("UPDATE items SET status='下载中',downloader='transmission',task_id='12',progress=37 WHERE id=?", id)
	if _, err := pauseItemDownload(id); err == nil {
		t.Fatal("RPC failure ignored")
	}
	if item := controlItem(t, id); item.Status != "下载中" {
		t.Fatal(item)
	}
	fail = false
	if _, err := pauseItemDownload(id); err != nil {
		t.Fatal(err)
	}
	if _, err := queueItemDownload(id, true); err != nil {
		t.Fatal(err)
	}
	runQueuedDownload(id)
	if item := controlItem(t, id); item.Status != "下载中" || item.TaskID != "12" || item.Progress != 37 {
		t.Fatal(item)
	}
	if !reflect.DeepEqual(controls, []string{"torrent-stop", "torrent-stop", "torrent-start"}) {
		t.Fatal(controls)
	}
}

func TestResumeQueueRecoveryRetainsIdentityAndPausedStaysPaused(t *testing.T) {
	testDatabase(t)
	fake := newControlQbit(t, false)
	fake.state = "stoppedDL"
	db.Exec("INSERT INTO feeds(id,name,url,enabled,auto_download) VALUES(1,'暂停订阅','https://example.test/feed',0,1)")
	first := insertItem(t, "https://example.test/resuming")
	second := insertItem(t, "https://example.test/paused")
	for _, id := range []int{first, second} {
		db.Exec("UPDATE items SET feed_id=1,status='已暂停',downloader='qbittorrent',task_id=?,progress=37 WHERE id=?", controlTestHash, id)
	}
	if _, err := queueItemDownload(first, true); err != nil {
		t.Fatal(err)
	}
	db.Close()
	initDB()
	ids, err := recoverDownloads(db)
	if err != nil || !reflect.DeepEqual(ids, []int{first}) {
		t.Fatal(ids, err)
	}
	resumeDownloads(ids)
	if item := controlItem(t, first); item.Status != "下载中" || item.TaskID != controlTestHash || item.Progress != 37 {
		t.Fatal(item)
	}
	if item := controlItem(t, second); item.Status != "已暂停" {
		t.Fatal(item)
	}
	if fake.adds != 0 {
		t.Fatal("recovery submitted a duplicate")
	}
}

func TestDownloadControlAPIValidationAndPartialBatchResults(t *testing.T) {
	testDatabase(t)
	first := insertItem(t, "https://example.test/first")
	paused := insertItem(t, "https://example.test/paused")
	saved := insertItem(t, "https://example.test/saved")
	db.Exec("UPDATE items SET status='已暂停' WHERE id=?", paused)
	db.Exec("UPDATE items SET status='已保存' WHERE id=?", saved)
	for _, c := range []struct {
		method, path, body string
		code               int
	}{
		{"GET", "/api/items/1/pause", "", 405},
		{"POST", "/api/items/1x/pause", "", 404},
		{"POST", "/api/items/1/pause/extra", "", 404},
		{"POST", "/api/items/999/resume", "", 404},
		{"POST", "/api/items/3/pause", "", 409},
		{"POST", "/api/items/manage", `{"action":"pause","ids":[1,1]}`, 400},
		{"POST", "/api/items/manage", `{"action":"pause","ids":[0]}`, 400},
		{"POST", "/api/items/manage", `{"action":"pause","ids":[]}`, 400},
		{"POST", "/api/items/manage", `{"action":"all","ids":[1]}`, 400},
		{"POST", "/api/items/manage", `{"action":"pause","ids":[1]} {}`, 400},
	} {
		rec := httptest.NewRecorder()
		itemsEntryAPI(rec, httptest.NewRequest(c.method, c.path, strings.NewReader(c.body)))
		if rec.Code != c.code {
			t.Errorf("%s %s: %d %s", c.method, c.path, rec.Code, rec.Body)
		}
	}
	rec := httptest.NewRecorder()
	itemsEntryAPI(rec, httptest.NewRequest("POST", "/api/items/manage", strings.NewReader(fmt.Sprintf(`{"action":"pause","ids":[%d,%d,%d,999]}`, first, paused, saved))))
	var out itemBatchResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || out.OK || out.Count != 1 || out.Skipped != 1 || len(out.Failed) != 2 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	setWebAuth(t, "control-test", "test-password")
	rec = httptest.NewRecorder()
	auth(http.HandlerFunc(itemsEntryAPI)).ServeHTTP(rec, httptest.NewRequest("POST", fmt.Sprintf("/api/items/%d/resume", first), nil))
	if rec.Code != 401 || controlItem(t, first).Status != "已暂停" {
		t.Fatal("unauthenticated control succeeded", rec.Code)
	}
}

func TestPausedDownloaderStateReleasesCapacity(t *testing.T) {
	testDatabase(t)
	fake := newControlQbit(t, false)
	fake.state = "stoppedDL"
	id := insertItem(t, "magnet:?xt=urn:btih:"+controlTestHash)
	db.Exec("UPDATE items SET status='下载中',downloader='qbittorrent',task_id=? WHERE id=?", controlTestHash, id)
	pollDownloaderTasks()
	if item := controlItem(t, id); item.Status != "已暂停" || item.Progress != 37 {
		t.Fatal(item)
	}
	refs, err := activeTaskRefs()
	if err != nil || len(refs) != 0 {
		t.Fatal(refs, err)
	}
	if _, err := db.Exec(`CREATE TRIGGER reject_pause BEFORE UPDATE ON items WHEN NEW.status='已暂停' BEGIN SELECT RAISE(FAIL,'blocked'); END`); err != nil {
		t.Fatal(err)
	}
	id2 := insertItem(t, "https://example.test/pending")
	if changed, err := pauseItemDownload(id2); changed || err == nil {
		t.Fatal("failed DB transition reported success")
	}
	if item := controlItem(t, id2); item.Status != "待下载" {
		t.Fatal(item)
	}
}

func TestPauseAdmitsNextQueuedDownload(t *testing.T) {
	testDatabase(t)
	newControlQbit(t, false)
	oldLimit := configuredDownloads.Load()
	configuredDownloads.Store(1)
	t.Cleanup(func() { configuredDownloads.Store(oldLimit) })
	active := insertItem(t, "magnet:?xt=urn:btih:"+controlTestHash)
	db.Exec("UPDATE items SET status='下载中',downloader='qbittorrent',task_id=? WHERE id=?", controlTestHash, active)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); fmt.Fprint(w, "complete media") }))
	defer server.Close()
	next := insertItem(t, server.URL+"/next.mp4")
	if _, err := queueItemDownload(next, false); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { runQueuedDownload(next); close(done) }()
	if claimed, waiting, err := claimDownload(next); claimed || !waiting || err != nil {
		t.Fatal(claimed, waiting, err)
	}
	if requests.Load() != 0 {
		t.Fatal("queue exceeded capacity")
	}
	if _, err := pauseItemDownload(active); err != nil {
		t.Fatal(err)
	}
	waitDownloadDone(t, done)
	if requests.Load() != 1 || controlItem(t, next).Status != "已保存" {
		t.Fatal("pause did not release capacity")
	}
}

func TestDeleteInterruptsDirectTransferAndRollsBackRecords(t *testing.T) {
	for _, failDelete := range []bool{false, true} {
		t.Run(fmt.Sprint("rollback=", failDelete), func(t *testing.T) {
			testDatabase(t)
			started := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Length", "1000000")
				fmt.Fprint(w, "partial media")
				w.(http.Flusher).Flush()
				close(started)
				<-r.Context().Done()
			}))
			defer server.Close()
			id := insertItem(t, server.URL+"/partial.mp4")
			other := insertItem(t, "https://example.test/saved")
			archive := filepath.Join(downloadDir, "keep.mp4")
			if err := os.WriteFile(archive, []byte("existing archive"), 0600); err != nil {
				t.Fatal(err)
			}
			db.Exec("UPDATE items SET status='已保存',file_path=? WHERE id=?", archive, other)
			if failDelete {
				if _, err := db.Exec(fmt.Sprintf(`CREATE TRIGGER reject_delete BEFORE DELETE ON items WHEN OLD.id=%d BEGIN SELECT RAISE(FAIL,'blocked'); END`, other)); err != nil {
					t.Fatal(err)
				}
			}
			done := make(chan struct{})
			go func() { downloadItem(id); close(done) }()
			waitDownloadDone(t, started)
			err := deleteItemRecords([]int{id, other})
			waitDownloadDone(t, done)
			if (err != nil) != failDelete {
				t.Fatal(err)
			}
			var count int
			db.QueryRow("SELECT COUNT(*) FROM items").Scan(&count)
			if failDelete {
				if count != 2 || controlItem(t, id).Status != "已暂停" {
					t.Fatal("failed removal lost records or left an orphan worker", count)
				}
			} else if count != 0 {
				t.Fatal(count)
			}
			if raw, err := os.ReadFile(archive); err != nil || string(raw) != "existing archive" {
				t.Fatal("removal changed saved media", err)
			}
			filepath.WalkDir(downloadDir, func(path string, entry fs.DirEntry, err error) error {
				if err != nil {
					t.Fatal(err)
				}
				if strings.HasPrefix(entry.Name(), ".partial-") {
					t.Errorf("temporary file remains: %s", path)
				}
				return nil
			})
		})
	}
}

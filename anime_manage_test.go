package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func managementRequest(method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	w := httptest.NewRecorder()
	if path == "/api/animes" {
		animesAPI(w, r)
	} else {
		animeEntryAPI(w, r)
	}
	return w
}

func managementExec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

func managementCount(t *testing.T, query string, expected int) {
	t.Helper()
	var n int
	if err := db.QueryRow(query).Scan(&n); err != nil || n != expected {
		t.Fatalf("%s: got %d want %d (%v)", query, n, expected, err)
	}
}

func TestAnimeManagementCreate(t *testing.T) {
	testDatabase(t)
	for _, body := range []string{`{`, `{"title":" "}`, `{"title":"无效","season":-1}`, `{"title":"无效","season":0}`, `{"title":"无效","season":1000}`, `{"title":"无效","storage_dir":"relative"}`} {
		if w := managementRequest("POST", "/api/animes", body); w.Code != 400 {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body.String())
		}
	}
	managementCount(t, "SELECT COUNT(*) FROM animes", 0)
	w := managementRequest("POST", "/api/animes", `{"title":" 星海物语 ","search_name":"星海","season":2,"storage_dir":"/library","include_rules":"星海","exclude_rules":"预告"}`)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var a animeRow
	if err := json.Unmarshal(w.Body.Bytes(), &a); err != nil || a.Title != "星海物语" || a.Season != 2 || a.StorageDir != "/library" {
		t.Fatal(a, err)
	}
	if w = managementRequest("POST", "/api/animes", `{"title":"星海物语","season":3}`); w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	managementCount(t, "SELECT COUNT(*) FROM animes WHERE season=2", 1)
	managementExec(t, "INSERT INTO settings(key,value) VALUES('auto_cleanup_empty_anime','1')")
	if w = managementRequest("POST", "/api/animes", `{"title":"手动新建"}`); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if n, err := cleanupEmptyAnime(); err != nil || n != 0 {
		t.Fatalf("manual creation cleaned: %d %v", n, err)
	}
}

func seedManagement(t *testing.T) string {
	t.Helper()
	managementExec(t, "INSERT INTO animes(id,title,season,storage_dir,include_rules) VALUES(1,'来源甲',2,'/source','甲'),(2,'来源乙',0,'','乙'),(3,'目标',4,'/target','目标')")
	media := filepath.Join(downloadDir, "keep.mp4")
	if err := os.WriteFile(media, []byte("preserved media"), 0600); err != nil {
		t.Fatal(err)
	}
	managementExec(t, `INSERT INTO items(id,anime_id,anime_title,episode,season,status,file_path,link) VALUES
		(1,1,'来源甲','07',0,'已保存',?,'https://test/1'),
		(2,1,'来源甲','08',3,'待下载','','https://test/2'),
		(3,0,'来源乙','09',0,'待下载','','https://test/3'),
		(4,3,'目标','01',0,'待下载','','https://test/4')`, media)
	managementExec(t, "INSERT INTO feeds(id,name,url,target_title,enabled) VALUES(1,'甲','https://feed/1','来源甲',1),(2,'乙','https://feed/2','来源乙',0),(3,'其他','https://feed/3','目标',1)")
	managementExec(t, "INSERT INTO anime_meta(anime_title,source) VALUES('来源甲','manual'),('来源乙','bangumi'),('目标','manual')")
	managementExec(t, "INSERT INTO anime_storage(anime_title,storage_dir) VALUES('来源甲','/legacy')")
	return media
}

func TestAnimeManagementMerge(t *testing.T) {
	testDatabase(t)
	media := seedManagement(t)
	w := managementRequest("POST", "/api/animes/manage", `{"action":"merge","ids":[1,2],"target_id":3}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	managementCount(t, "SELECT COUNT(*) FROM animes", 1)
	managementCount(t, "SELECT COUNT(*) FROM items WHERE anime_id=3 AND anime_title='目标'", 4)
	managementCount(t, "SELECT COUNT(*) FROM items WHERE (id=1 AND season=2) OR (id=2 AND season=3) OR (id=3 AND season=1)", 3)
	managementCount(t, "SELECT COUNT(*) FROM items WHERE manual=1", 3)
	managementCount(t, "SELECT COUNT(*) FROM feeds WHERE target_title='目标'", 3)
	managementCount(t, "SELECT COUNT(*) FROM feeds WHERE enabled=0", 1)
	managementCount(t, "SELECT COUNT(*) FROM animes WHERE storage_dir='/target' AND include_rules='目标' AND season=4", 1)
	managementCount(t, "SELECT COUNT(*) FROM anime_meta", 1)
	managementCount(t, "SELECT COUNT(*) FROM anime_storage", 0)
	var path, episode string
	db.QueryRow("SELECT file_path,episode FROM items WHERE id=1").Scan(&path, &episode)
	if path != media || episode != "07" {
		t.Fatal(path, episode)
	}
	if data, err := os.ReadFile(media); err != nil || string(data) != "preserved media" {
		t.Fatal("media changed", err)
	}
}

func TestAnimeManagementDelete(t *testing.T) {
	testDatabase(t)
	media := seedManagement(t)
	if w := managementRequest("DELETE", "/api/animes/1", ""); w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	w := managementRequest("POST", "/api/animes/manage", `{"action":"delete","ids":[1,2],"delete_items":true}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	managementCount(t, "SELECT COUNT(*) FROM animes", 1)
	managementCount(t, "SELECT COUNT(*) FROM items", 1)
	managementCount(t, "SELECT COUNT(*) FROM feeds WHERE enabled=0 AND target_title=''", 2)
	managementCount(t, "SELECT COUNT(*) FROM feeds WHERE enabled=1 AND target_title='目标'", 1)
	managementCount(t, "SELECT COUNT(*) FROM anime_meta", 1)
	managementCount(t, "SELECT COUNT(*) FROM anime_storage", 0)
	if data, err := os.ReadFile(media); err != nil || string(data) != "preserved media" {
		t.Fatal("media changed", err)
	}
}

func TestAnimeManagementBatchRollback(t *testing.T) {
	for _, action := range []string{"merge", "delete"} {
		t.Run(action, func(t *testing.T) {
			testDatabase(t)
			seedManagement(t)
			managementExec(t, "CREATE TRIGGER fail_manage BEFORE DELETE ON anime_meta WHEN OLD.anime_title='来源乙' BEGIN SELECT RAISE(ABORT,'test'); END")
			w := managementRequest("POST", "/api/animes/manage", fmt.Sprintf(`{"action":%q,"ids":[1,2],"target_id":3,"delete_items":true}`, action))
			if w.Code != 500 {
				t.Fatal(w.Code, w.Body.String())
			}
			managementCount(t, "SELECT COUNT(*) FROM animes", 3)
			managementCount(t, "SELECT COUNT(*) FROM items", 4)
			managementCount(t, "SELECT COUNT(*) FROM items WHERE anime_id=1 AND anime_title='来源甲' AND manual=0", 2)
			managementCount(t, "SELECT COUNT(*) FROM anime_meta", 3)
			managementCount(t, "SELECT COUNT(*) FROM feeds WHERE target_title='来源甲' AND enabled=1", 1)
		})
	}
}

func TestAnimeManagementRejectsActiveAndInvalidBatches(t *testing.T) {
	testDatabase(t)
	seedManagement(t)
	for _, status := range []string{"等待下载", "下载中", "已发送到 Aria2", "已发送到 qBittorrent", "已发送到 Transmission"} {
		managementExec(t, "UPDATE items SET status=? WHERE id=1", status)
		for _, action := range []string{"merge", "delete"} {
			w := managementRequest("POST", "/api/animes/manage", fmt.Sprintf(`{"action":%q,"ids":[2,1],"target_id":3,"delete_items":true}`, action))
			if w.Code != 409 {
				t.Fatal(status, w.Code, w.Body.String())
			}
			managementCount(t, "SELECT COUNT(*) FROM animes", 3)
			managementCount(t, "SELECT COUNT(*) FROM items", 4)
		}
	}
	managementExec(t, "UPDATE items SET status='待下载' WHERE id=1")
	for _, body := range []string{`{"action":"delete","ids":[]}`, `{"action":"merge","ids":[1,1],"target_id":3}`, `{"action":"merge","ids":[1,3],"target_id":3}`, `{"action":"delete","ids":[-1]}`, `{"action":"other","ids":[1]}`} {
		if w := managementRequest("POST", "/api/animes/manage", body); w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	for _, body := range []string{`{"action":"merge","ids":[1],"target_id":99}`, `{"action":"delete","ids":[1,99],"delete_items":true}`} {
		if w := managementRequest("POST", "/api/animes/manage", body); w.Code != 404 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	managementCount(t, "SELECT COUNT(*) FROM items", 4)
	managementCount(t, "SELECT COUNT(*) FROM animes", 3)
}

func TestAnimeManagementStaleDownloaderResult(t *testing.T) {
	testDatabase(t)
	media := seedManagement(t)
	managementExec(t, "UPDATE items SET downloader='aria2',task_id='old-task' WHERE id=1")
	w := managementRequest("POST", "/api/animes/manage", `{"action":"delete","ids":[1],"delete_items":true}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	applyTaskUpdate(taskRef{ID: 1, Downloader: "aria2", TaskID: "old-task"}, taskState{Progress: 100, Files: []archiveFile{{Src: media, Rel: "keep.mp4"}}})
	managementCount(t, "SELECT COUNT(*) FROM items WHERE id=1", 0)
	managementCount(t, "SELECT COUNT(*) FROM animes WHERE id=1", 0)
	if data, err := os.ReadFile(media); err != nil || string(data) != "preserved media" {
		t.Fatal("media changed", err)
	}
}

func TestAnimeRenameRollsBackAllAssociations(t *testing.T) {
	testDatabase(t)
	seedManagement(t)
	managementExec(t, "CREATE TRIGGER fail_rename BEFORE UPDATE ON feeds BEGIN SELECT RAISE(ABORT,'test'); END")
	w := managementRequest("PUT", "/api/animes/1", `{"title":"新名称","season":5}`)
	if w.Code != 500 {
		t.Fatalf("expected rollback error: %d %s", w.Code, w.Body.String())
	}
	managementCount(t, "SELECT COUNT(*) FROM animes WHERE id=1 AND title='来源甲' AND season=2", 1)
	managementCount(t, "SELECT COUNT(*) FROM items WHERE anime_id=1 AND anime_title='来源甲'", 2)
	managementCount(t, "SELECT COUNT(*) FROM anime_meta WHERE anime_title='来源甲'", 1)
	managementCount(t, "SELECT COUNT(*) FROM feeds WHERE target_title='来源甲'", 1)
}

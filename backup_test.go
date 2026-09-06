package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBackupPendingAndSavedRoundTrip(t *testing.T) {
	testDatabase(t)
	if _, err := db.Exec(`INSERT INTO feeds(id,name,url,include_words,auto_download,enabled) VALUES(1,'夏日动画','https://example.test/rss','简中',1,0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO items(feed_id,title,link) VALUES(1,'待下载的一话','https://example.test/1')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO items(feed_id,title,link,status,file_path) VALUES(1,'已保存的一话','https://example.test/2','已保存','/data/动画/02.mp4')`); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	backup(rec, httptest.NewRequest("GET", "/api/backup", nil))
	if rec.Code != 200 {
		t.Fatalf("export: %d %s", rec.Code, rec.Body.String())
	}
	var data backupData
	if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Feeds) != 1 || len(data.Items) != 2 || data.Items[0]["file_path"] != "" || data.Items[1]["file_path"] != "/data/动画/02.mp4" {
		t.Fatalf("export incomplete: %+v", data)
	}
	// Restore into an existing subscription twice, preserving its ID and items.
	if _, err := db.Exec("UPDATE feeds SET name='旧名称',include_words='',auto_download=0,enabled=1 WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		result := httptest.NewRecorder()
		restore(result, httptest.NewRequest("POST", "/api/restore", strings.NewReader(rec.Body.String())))
		if result.Code != 200 {
			t.Fatalf("restore: %d %s", result.Code, result.Body.String())
		}
	}
	var name, include string
	var auto, enabled, feedCount, itemCount int
	if err := db.QueryRow("SELECT name,include_words,auto_download,enabled FROM feeds WHERE id=1").Scan(&name, &include, &auto, &enabled); err != nil {
		t.Fatal(err)
	}
	db.QueryRow("SELECT COUNT(*) FROM feeds").Scan(&feedCount)
	db.QueryRow("SELECT COUNT(*) FROM items").Scan(&itemCount)
	if name != "夏日动画" || include != "简中" || auto != 1 || enabled != 0 || feedCount != 1 || itemCount != 2 {
		t.Fatalf("round trip changed data: %s %s %d %d %d %d", name, include, auto, enabled, feedCount, itemCount)
	}
}

func TestRestoreRejectsInvalidDocumentsAtomically(t *testing.T) {
	valid := `{"name":"新订阅","url":"https://example.test/new","enabled":true}`
	for _, body := range []string{
		`null`, `{}`, `{"feeds":null}`, `{"version":99,"feeds":[]}`,
		`{"feeds":[` + valid + `]} {"unexpected":true}`,
		`{"feeds":[` + valid + `,{"name":"错误","url":"http://"}]}`,
		`{"feeds":[` + valid + `,{"name":"错误","url":"https://example.test:bad/rss"}]}`,
	} {
		t.Run(fmt.Sprintf("case-%d", len(body)), func(t *testing.T) {
			testDatabase(t)
			rec := httptest.NewRecorder()
			restore(rec, httptest.NewRequest("POST", "/api/restore", strings.NewReader(body)))
			if rec.Code != 400 {
				t.Fatalf("accepted malformed backup: %d %s", rec.Code, body)
			}
			var count int
			if err := db.QueryRow("SELECT COUNT(*) FROM feeds").Scan(&count); err != nil || count != 0 {
				t.Fatalf("partial restore: %d %v", count, err)
			}
		})
	}
}

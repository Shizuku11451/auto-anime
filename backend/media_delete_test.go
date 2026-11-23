package main

import (
	"os"
	"path/filepath"
	"testing"
)

// 自定义 storage_dir 下的文件可删除（#17），但旁边的归档记录属于别的条目时必须保留（#6）。
func TestDeleteMediaRespectsCustomRootAndForeignArchive(t *testing.T) {
	testDatabase(t)
	storage := t.TempDir()
	if _, err := db.Exec("INSERT INTO animes(title,storage_dir) VALUES('星海',?)", storage); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(storage, "星海", "S01E01.mkv")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(file, "video")
	write(filepath.Join(storage, "星海", "S01E01-info.json"), `{"id":99}`)
	media := []savedMedia{{ID: 1, Path: file}}
	if out := deleteSavedMedia(media, mediaRoots()); out.Deleted != 0 || len(out.Failed) != 1 {
		t.Fatalf("foreign archive: %+v", out)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("file owned by another item was deleted: %v", err)
	}
	write(filepath.Join(storage, "星海", "S01E01-info.json"), `{"id":1}`)
	if out := deleteSavedMedia(media, mediaRoots()); out.Deleted != 1 || len(out.Failed) != 0 {
		t.Fatalf("own archive: %+v", out)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("file in custom storage_dir was kept: %v", err)
	}
}

// 番剧行在事务里被删掉后再算媒体根目录，就会丢掉它的 storage_dir（#17）。
func TestAnimeDeleteRemovesFilesInCustomStorage(t *testing.T) {
	testDatabase(t)
	storage := t.TempDir()
	file := filepath.Join(storage, "S01E01.mkv")
	if err := os.WriteFile(file, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO animes(id,title,storage_dir) VALUES(1,'星海',?)", storage); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO items(id,title,link,anime_id,anime_title,status,file_path) VALUES(1,'星海 01','l1',1,'星海','已保存',?)", file); err != nil {
		t.Fatal(err)
	}
	w := managementRequest("POST", "/api/animes/manage", `{"action":"delete","ids":[1],"delete_items":true,"delete_files":true}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("file kept: %v %s", err, w.Body.String())
	}
}

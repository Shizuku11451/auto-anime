package main

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func savedFixture(t *testing.T, title string, season int, nested bool) (int64, string) {
	t.Helper()
	a, err := ensureAnime(title)
	if err != nil {
		t.Fatal(err)
	}
	server := mediaServer(t)
	result, err := db.Exec("INSERT INTO items(title,link,anime_title,anime_id,episode,season) VALUES(?,?,?,?,?,?)", title+" [07]", server.URL+"/07.mp4", title, a, "07", season)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := result.LastInsertId()
	var saved string
	if nested {
		local := t.TempDir()
		var files []archiveFile
		for _, rel := range []string{"Pack/Video/07.mp4", "Pack/Subs/07.ass"} {
			src := filepath.Join(local, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(src), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(src, []byte("media:"+rel), 0644); err != nil {
				t.Fatal(err)
			}
			files = append(files, archiveFile{Src: src, Rel: rel})
		}
		saved, err = archiveFromFiles(int(id), files)
	} else {
		saved, err = fetchArchive(int(id), server.URL+"/07.mp4")
	}
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE items SET status='已保存',file_path=? WHERE id=?", saved, id); err != nil {
		t.Fatal(err)
	}
	return id, saved
}

func correctSaved(t *testing.T, id, animeID int64, season int) *httptest.ResponseRecorder {
	t.Helper()
	body := fmt.Sprintf(`{"anime_id":%d,"season":%d,"episode":"07","rearchive":true}`, animeID, season)
	rec := httptest.NewRecorder()
	correctItem(rec, httptest.NewRequest("PATCH", fmt.Sprintf("/api/items/%d", id), strings.NewReader(body)), id)
	return rec
}

func snapshotFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		files[rel] = string(content)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestMigrationFailurePreservesSourceAndDatabase(t *testing.T) {
	for _, failure := range []string{"metadata", "database", "nested-database"} {
		t.Run(failure, func(t *testing.T) {
			settingsFixture(t)
			id, oldPath := savedFixture(t, "Source", 0, failure == "nested-database")
			target, err := ensureAnime("Target")
			if err != nil {
				t.Fatal(err)
			}
			root, season, base := archiveTarget("Target", "07", "unused", 2)
			conflict := filepath.Join(root, season, base+"-info.json")
			if failure == "metadata" {
				if err := os.MkdirAll(conflict, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(conflict, "keep"), []byte("unrelated"), 0644); err != nil {
					t.Fatal(err)
				}
			} else if _, err := db.Exec(`CREATE TRIGGER fail_migration BEFORE UPDATE ON items BEGIN SELECT RAISE(ABORT,'injected item failure'); END`); err != nil {
				t.Fatal(err)
			}
			before := snapshotFiles(t, downloadDir)
			rec := correctSaved(t, id, target, 2)
			if rec.Code != 500 {
				t.Fatalf("expected failure: %d %s", rec.Code, rec.Body.String())
			}
			var recorded, title string
			var manual, s int
			if err := db.QueryRow("SELECT file_path,anime_title,manual,season FROM items WHERE id=?", id).Scan(&recorded, &title, &manual, &s); err != nil {
				t.Fatal(err)
			}
			if recorded != oldPath || title != "Source" || manual != 0 || s != 0 {
				t.Fatal("failed migration changed DB")
			}
			if !reflect.DeepEqual(before, snapshotFiles(t, downloadDir)) {
				t.Fatal("failed migration lost originals or left newly published files")
			}
		})
	}
}

func TestNestedMigrationRetainsAllFilesAcrossConflicts(t *testing.T) {
	settingsFixture(t)
	id, oldPath := savedFixture(t, "Source", 0, true)
	source, _, _, err := migrationSource(oldPath, id)
	if err != nil {
		t.Fatal(err)
	}
	oldFiles := snapshotFiles(t, source)
	target, err := ensureAnime("Target")
	if err != nil {
		t.Fatal(err)
	}
	root, season, base := archiveTarget("Target", "07", "unused", 2)
	conflict := filepath.Join(root, season, base)
	if err := os.MkdirAll(conflict, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(conflict, "keep"), []byte("unrelated"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, s := range []int{2, 3} {
		rec := correctSaved(t, id, target, s)
		if rec.Code != 200 {
			t.Fatalf("migration: %d %s", rec.Code, rec.Body.String())
		}
		var saved string
		if err := db.QueryRow("SELECT file_path FROM items WHERE id=?", id).Scan(&saved); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
			t.Fatal("committed source not cleaned up")
		}
		newRoot, rel, _, err := migrationSource(saved, id)
		if err != nil || rel == "" {
			t.Fatalf("nested archive not recognized: %s %v", saved, err)
		}
		if s == 2 && !strings.HasSuffix(newRoot, " (2)") {
			t.Fatal("existing archive overwritten")
		}
		newFiles := snapshotFiles(t, newRoot)
		newContents := map[string]int{}
		for _, content := range newFiles {
			newContents[content]++
		}
		for name, content := range oldFiles {
			if strings.HasSuffix(name, ".ass") {
				if newFiles[name] != content {
					t.Fatalf("lost nested file %s", name)
				}
			}
			if strings.HasSuffix(name, ".mp4") {
				// 主视频随番剧/季纠错改名，但内容必须仍在。
				if newContents[content] == 0 {
					t.Fatalf("lost video content from %s", name)
				}
			}
		}
		// 集文件夹名与主视频文件名一致（含冲突后缀），始终跟随媒体库命名。
		renamed := filepath.Join("Pack", "Video", filepath.Base(newRoot)+".mp4")
		if _, ok := newFiles[renamed]; !ok {
			t.Fatalf("main video not renamed to media-library name: %v", newFiles)
		}
		assertArchiveMetadata(t, newRoot, filepath.Base(newRoot), id, target, s, filepath.ToSlash(rel))
		oldPath = saved
	}
	if b, err := os.ReadFile(filepath.Join(conflict, "keep")); err != nil || string(b) != "unrelated" {
		t.Fatal("conflicting folder changed")
	}
}

func assertArchiveMetadata(t *testing.T, dir, base string, id, animeID int64, season int, file string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, base+"-info.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m archiveMetadata
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if int64(m.ID) != id || m.AnimeID != animeID || m.Season != season || m.File != file || m.Size <= 0 {
		t.Fatalf("incorrect manifest: %+v", m)
	}
	data, err = os.ReadFile(filepath.Join(dir, base+".nfo"))
	if err != nil {
		t.Fatal(err)
	}
	var nfo episodeNFO
	if err := xml.Unmarshal(data, &nfo); err != nil {
		t.Fatal(err)
	}
	if nfo.Season != season || nfo.Episode != "07" {
		t.Fatalf("incorrect NFO: %+v", nfo)
	}
}

func TestArchiveMetadataUsesEffectiveSeason(t *testing.T) {
	for _, nested := range []bool{false, true} {
		for _, override := range []int{0, 3} {
			t.Run(fmt.Sprintf("nested=%v/override=%d", nested, override), func(t *testing.T) {
				settingsFixture(t)
				a, err := ensureAnime("Series")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec("UPDATE animes SET season=2 WHERE id=?", a); err != nil {
					t.Fatal(err)
				}
				id, saved := savedFixture(t, "Series", override, nested)
				want := 2
				if override > 0 {
					want = override
				}
				if !strings.Contains(saved, fmt.Sprintf("Season %02d", want)) {
					t.Fatal("wrong season directory")
				}
				root, rel, _, err := migrationSource(saved, id)
				if err != nil {
					t.Fatal(err)
				}
				base := filepath.Base(root)
				if rel == "" {
					root = filepath.Dir(saved)
					base = strings.TrimSuffix(filepath.Base(saved), filepath.Ext(saved))
					rel = filepath.Base(saved)
				}
				assertArchiveMetadata(t, root, base, id, a, want, filepath.ToSlash(rel))
			})
		}
	}
}

func TestMigrationCommitConflictKeepsSource(t *testing.T) {
	settingsFixture(t)
	id, oldPath := savedFixture(t, "Source", 0, false)
	target, err := ensureAnime("Target")
	if err != nil {
		t.Fatal(err)
	}
	before := snapshotFiles(t, downloadDir)
	_, err = rearchiveSavedItem(id, oldPath, target, "Target", "07", 2, func(newPath string) error {
		if _, err := os.Stat(oldPath); err != nil {
			t.Fatal("source removed before DB commit")
		}
		if _, err := os.Stat(newPath); err != nil {
			t.Fatal("destination not ready at commit")
		}
		return errors.New("concurrent correction or deletion")
	})
	if err == nil || !reflect.DeepEqual(before, snapshotFiles(t, downloadDir)) {
		t.Fatal("commit conflict failed to roll back")
	}
}

func TestMigrationRejectsSymlinkPaths(t *testing.T) {
	settingsFixture(t)
	id, oldPath := savedFixture(t, "Source", 0, false)
	link := filepath.Join(t.TempDir(), "linked.mp4")
	if err := os.Symlink(oldPath, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, _, _, err := migrationSource(link, id); err == nil {
		t.Fatal("symlink source accepted")
	}
	dirLink := filepath.Join(t.TempDir(), "linked-dir")
	if err := os.Symlink(filepath.Dir(oldPath), dirLink); err != nil {
		t.Fatal(err)
	}
	if err := checkMigrationParents(filepath.Join(dirLink, "child")); err == nil {
		t.Fatal("symlink destination ancestor accepted")
	}
}

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// 番剧级文件迁移：修改名称、存储位置或季号后，把已保存的集数移到新位置。
// 每一集复用 rearchiveSavedItem（先复制、再有条件地更新数据库、最后清理旧文件），
// 所以任何一集失败都不会丢失原文件。迁移在后台逐集进行，每集只短暂持有
// animeCleanupMu 读锁，跨盘复制大文件时不会阻塞番剧库的读写。

type savedItem struct {
	ID         int64
	Title      string
	FilePath   string
	Episode    string
	Season     int
	AutoSeason int
}

type relocationFailure struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Error string `json:"error"`
}

type relocationStatus struct {
	Running bool                `json:"running"`
	Total   int                 `json:"total"`
	Moved   int                 `json:"moved"`
	Skipped int                 `json:"skipped"`
	Failed  []relocationFailure `json:"failed"`
	Warning string              `json:"warning,omitempty"`
}

type relocationPlan struct {
	Saved  int      `json:"saved"`
	Moving int      `json:"moving"`
	From   []string `json:"from"`
	To     string   `json:"to"`
}

var relocations = struct {
	sync.Mutex
	jobs map[int64]*relocationStatus
}{jobs: map[int64]*relocationStatus{}}

var errRelocationRunning = errors.New("该番剧正在移动文件，请等待完成")

var seasonDirName = regexp.MustCompile(`^Season \d+$`)

func savedItemsOf(animeID int64) ([]savedItem, error) {
	rows, err := db.Query(`SELECT id,COALESCE(title,''),file_path,COALESCE(episode,''),COALESCE(season,0),COALESCE(season_auto,0)
		FROM items WHERE anime_id=? AND status='已保存' AND COALESCE(file_path,'')<>'' ORDER BY id`, animeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []savedItem
	for rows.Next() {
		var it savedItem
		if err := rows.Scan(&it.ID, &it.Title, &it.FilePath, &it.Episode, &it.Season, &it.AutoSeason); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// targetDir 与 archiveTarget 的目录规则一致：识别出集数的放进 Season XX，其余留在番剧根目录。
func (it savedItem) targetDir(root string, animeSeason int) string {
	if strings.TrimSpace(it.Episode) == "" {
		return root
	}
	return filepath.Join(root, fmt.Sprintf("Season %02d", pickSeason(it.Season, animeSeason, it.AutoSeason)))
}

func pathWithin(p, dir string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(p))
	return err == nil && rel != "." && filepath.IsLocal(rel)
}

// itemSeriesDir 推断已保存文件所在的番剧文件夹：Season XX 的上一级，没有季目录时为文件所在目录。
func itemSeriesDir(p string) string {
	dir := filepath.Dir(filepath.Clean(p))
	for d := dir; ; d = filepath.Dir(d) {
		if seasonDirName.MatchString(filepath.Base(d)) {
			return filepath.Dir(d)
		}
		if filepath.Dir(d) == d {
			return dir
		}
	}
}

func planRelocation(items []savedItem, root string, animeSeason int) (moving []savedItem, from []string) {
	seen := map[string]bool{}
	for _, it := range items {
		if pathWithin(it.FilePath, it.targetDir(root, animeSeason)) {
			continue
		}
		moving = append(moving, it)
		if dir := filepath.Dir(it.FilePath); !seen[dir] {
			seen[dir] = true
			from = append(from, dir)
		}
	}
	sort.Strings(from)
	return moving, from
}

// POST /api/animes/{id}/relocation-preview：按尚未保存的名称/位置/季号预估要移动的文件，不改动任何内容。
func animeRelocationPreview(w http.ResponseWriter, r *http.Request, id int64) {
	var in struct {
		Title      *string `json:"title"`
		StorageDir *string `json:"storage_dir"`
		Season     *int    `json:"season"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in) != nil {
		http.Error(w, "请求参数无效", 400)
		return
	}
	a, err := getAnime(id)
	if err != nil {
		http.Error(w, "番剧不存在", 404)
		return
	}
	title, storageDir, season := a.Title, a.StorageDir, a.Season
	if in.Title != nil && strings.TrimSpace(*in.Title) != "" {
		if title = sanitizeText(*in.Title, titleMaxLength); title == "" {
			http.Error(w, "番剧名称无效", 400)
			return
		}
	}
	if in.StorageDir != nil {
		if storageDir, err = sanitizeStorageDir(*in.StorageDir); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
	}
	if in.Season != nil {
		if *in.Season < 0 || *in.Season > 999 {
			http.Error(w, "季号必须是 0-999（0 表示自动）", 400)
			return
		}
		season = *in.Season
	}
	items, err := savedItemsOf(id)
	if err != nil {
		http.Error(w, "读取资源失败", 500)
		return
	}
	root := seriesRoot(title, storageDir)
	moving, from := planRelocation(items, root, season)
	if len(from) > 3 {
		from = from[:3]
	}
	w.Header().Set("Cache-Control", "no-store")
	jsonOut(w, relocationPlan{Saved: len(items), Moving: len(moving), From: append([]string{}, from...), To: root})
}

func relocationSnapshot(animeID int64) relocationStatus {
	relocations.Lock()
	defer relocations.Unlock()
	if st := relocations.jobs[animeID]; st != nil {
		out := *st
		out.Failed = append([]relocationFailure{}, st.Failed...)
		return out
	}
	return relocationStatus{Failed: []relocationFailure{}}
}

// startRelocation 按番剧当前设置移动已保存的文件；only 非空时只处理这些条目（合并时用）。
func startRelocation(animeID int64, only map[int64]bool) (relocationStatus, error) {
	a, err := getAnime(animeID)
	if err != nil {
		return relocationStatus{}, err
	}
	items, err := savedItemsOf(animeID)
	if err != nil {
		return relocationStatus{}, err
	}
	if only != nil {
		filtered := items[:0]
		for _, it := range items {
			if only[it.ID] {
				filtered = append(filtered, it)
			}
		}
		items = filtered
	}
	moving, _ := planRelocation(items, seriesRoot(a.Title, a.StorageDir), a.Season)
	relocations.Lock()
	if st := relocations.jobs[animeID]; st != nil && st.Running {
		relocations.Unlock()
		return relocationStatus{}, errRelocationRunning
	}
	st := &relocationStatus{Running: len(moving) > 0, Total: len(moving), Failed: []relocationFailure{}}
	relocations.jobs[animeID] = st
	relocations.Unlock()
	if len(moving) > 0 {
		go runRelocation(animeID, moving)
	}
	return relocationSnapshot(animeID), nil
}

func runRelocation(animeID int64, moving []savedItem) {
	oldDirs := map[string]bool{}
	warning := ""
	for _, it := range moving {
		oldDir := itemSeriesDir(it.FilePath)
		moved, skipped, warn, err := relocateItem(animeID, it)
		relocations.Lock()
		st := relocations.jobs[animeID]
		switch {
		case err != nil:
			st.Failed = append(st.Failed, relocationFailure{ID: it.ID, Title: it.Title, Error: err.Error()})
		case skipped:
			st.Skipped++
		case moved:
			st.Moved++
			oldDirs[oldDir] = true
		}
		relocations.Unlock()
		if warn != "" {
			warning = warn
		}
	}
	if a, err := getAnime(animeID); err == nil {
		root := seriesRoot(a.Title, a.StorageDir)
		for dir := range oldDirs {
			retireSeriesDir(dir, root)
		}
	}
	relocations.Lock()
	st := relocations.jobs[animeID]
	st.Running, st.Warning = false, warning
	movedCount, skippedCount, failedCount := st.Moved, st.Skipped, len(st.Failed)
	relocations.Unlock()
	log.Printf("番剧 %d 文件迁移完成：移动 %d，跳过 %d，失败 %d", animeID, movedCount, skippedCount, failedCount)
}

// relocateItem 重新读取条目与番剧，确认仍需移动后迁移一集；期间被修改或已在新位置的条目跳过。
func relocateItem(animeID int64, planned savedItem) (moved, skipped bool, warning string, err error) {
	animeCleanupMu.RLock()
	defer animeCleanupMu.RUnlock()
	a, err := getAnime(animeID)
	if err != nil {
		return false, false, "", fmt.Errorf("番剧已不存在")
	}
	var it savedItem
	var status string
	var owner int64
	err = db.QueryRow(`SELECT id,COALESCE(title,''),COALESCE(file_path,''),COALESCE(episode,''),COALESCE(season,0),COALESCE(season_auto,0),COALESCE(status,''),COALESCE(anime_id,0)
		FROM items WHERE id=?`, planned.ID).Scan(&it.ID, &it.Title, &it.FilePath, &it.Episode, &it.Season, &it.AutoSeason, &status, &owner)
	if err != nil || owner != animeID || status != "已保存" || it.FilePath == "" {
		return false, true, "", nil
	}
	if pathWithin(it.FilePath, it.targetDir(seriesRoot(a.Title, a.StorageDir), a.Season)) {
		return false, true, "", nil
	}
	if _, err := os.Stat(it.FilePath); err != nil {
		return false, false, "", fmt.Errorf("已保存的文件不存在")
	}
	persist := func(newPath string) error {
		result, err := db.Exec(`UPDATE items SET file_path=? WHERE id=? AND anime_id=? AND status='已保存' AND file_path=?
			AND COALESCE(episode,'')=? AND COALESCE(season,0)=?`, newPath, it.ID, animeID, it.FilePath, it.Episode, it.Season)
		if err != nil {
			return err
		}
		if n, err := result.RowsAffected(); err != nil || n != 1 {
			return fmt.Errorf("条目已被其他操作修改")
		}
		return nil
	}
	warning, err = rearchiveSavedItem(it.ID, it.FilePath, animeID, a.Title, it.Episode, it.Season, persist)
	if err != nil {
		return false, false, "", err
	}
	return true, false, warning, nil
}

// retireSeriesDir 在旧番剧文件夹只剩本应用产物（海报、tvshow.nfo）时，把它们挪到新文件夹
// （新文件夹已有同名文件则直接删除旧的）并删除旧文件夹；有任何其他内容都保持不动。
func retireSeriesDir(oldDir, newDir string) {
	oldDir, newDir = filepath.Clean(oldDir), filepath.Clean(newDir)
	if oldDir == newDir || pathWithin(newDir, oldDir) || pathWithin(oldDir, newDir) || isStorageBase(oldDir) {
		return
	}
	entries, err := os.ReadDir(oldDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !keepableSeriesEntries[strings.ToLower(entry.Name())] {
			return
		}
	}
	for _, entry := range entries {
		src, dst := filepath.Join(oldDir, entry.Name()), filepath.Join(newDir, entry.Name())
		if _, err := os.Lstat(dst); os.IsNotExist(err) {
			if os.Rename(src, dst) != nil {
				return
			}
		} else if os.Remove(src) != nil {
			return
		}
	}
	if err := os.Remove(oldDir); err != nil {
		log.Printf("旧番剧目录未能删除 %q: %v", oldDir, err)
	}
}

// isStorageBase 判断目录是否为媒体库根目录或下载目录本身，这类目录永远不能被当作番剧文件夹清理。
func isStorageBase(dir string) bool {
	dir = filepath.Clean(dir)
	return dir == filepath.Clean(archiveBaseRoot()) || dir == filepath.Clean(downloadDir) || filepath.Dir(dir) == dir
}

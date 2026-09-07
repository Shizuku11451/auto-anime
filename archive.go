package main

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// RSS descriptions remain inert JSON text; they are never rendered as HTML.
type archiveMetadata struct {
	ID          int    `json:"id"`
	FeedID      int    `json:"feed_id"`
	FeedName    string `json:"feed_name"`
	Title       string `json:"original_title"`
	Published   string `json:"published"`
	Description string `json:"description"`
	AnimeTitle  string `json:"anime_title"`
	AnimeID     int64  `json:"anime_id"`
	Season      int    `json:"season"`
	Episode     string `json:"episode"`
	Quality     string `json:"quality"`
	Group       string `json:"fansub_group"`
	File        string `json:"file"`
	Size        int64  `json:"size_bytes"`
	SavedAt     string `json:"saved_at"`
}
type episodeNFO struct {
	XMLName   xml.Name `xml:"episodedetails"`
	Title     string   `xml:"title,omitempty"`
	ShowTitle string   `xml:"showtitle,omitempty"`
	Season    int      `xml:"season,omitempty"`
	Episode   string   `xml:"episode,omitempty"`
	Studio    string   `xml:"studio,omitempty"`
	Video     struct {
		Quality string `xml:"quality,omitempty"`
	} `xml:"video"`
	Plot      string `xml:"plot,omitempty"`
	DateAdded string `xml:"dateadded,omitempty"`
}

// tvshowUniqueID links the series NFO to its metadata source; only written
// when the series is bound to a Bangumi subject.
type tvshowUniqueID struct {
	Type string `xml:"type,attr"`
	ID   int64  `xml:",chardata"`
}

type tvshowNFO struct {
	XMLName  xml.Name        `xml:"tvshow"`
	Title    string          `xml:"title"`
	UniqueID *tvshowUniqueID `xml:"uniqueid,omitempty"`
}

// errTorrentResource marks a download that turned out to be a .torrent file:
// storing it must never count as a saved episode.
var errTorrentResource = errors.New("资源是种子文件")

func archiveInfo(id int) (archiveMetadata, error) {
	m := archiveMetadata{ID: id}
	err := db.QueryRow(`SELECT COALESCE(items.feed_id,0),COALESCE(feeds.name,''),items.title,
        COALESCE(published,''),COALESCE(description,''),COALESCE(anime_title,''),COALESCE(items.anime_id,0),COALESCE(items.season,0),COALESCE(episode,''),COALESCE(quality,''),COALESCE(fansub_group,'')
        FROM items LEFT JOIN feeds ON feeds.id=items.feed_id WHERE items.id=?`, id).
		Scan(&m.FeedID, &m.FeedName, &m.Title, &m.Published, &m.Description, &m.AnimeTitle, &m.AnimeID, &m.Season, &m.Episode, &m.Quality, &m.Group)
	return m, err
}

func safeComponent(raw string) string {
	name := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`/\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, raw)
	name = strings.Trim(name, " .")
	// Bound by UTF-8 bytes without splitting a Chinese code point.
	for len(name) > 120 {
		_, n := utf8.DecodeLastRuneInString(name)
		name = name[:len(name)-n]
	}
	name = strings.Trim(name, " .")
	if name == "" {
		return "未命名资源"
	}
	base := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || (len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9') {
		name = "_" + name
	}
	return name
}

// safeRelPath turns a downloader-provided relative path into a safe
// archive-relative path; it returns "" when the path cannot be trusted.
func safeRelPath(rel string) string {
	var parts []string
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			return ""
		}
		parts = append(parts, safeComponent(part))
	}
	return strings.Join(parts, "/")
}

// ---- 媒体库布局：一部番一个文件夹，集数用 Season XX / SXXEYY 命名 ----

// episodeTag renders "S01E07"; the episode number is zero-padded when numeric.
func episodeTag(season int, episode string) string {
	ep := strings.TrimSpace(episode)
	if n, err := strconv.Atoi(ep); err == nil && n >= 0 {
		ep = fmt.Sprintf("%02d", n)
	}
	return fmt.Sprintf("S%02dE%s", season, ep)
}

// queryAnimeStorage returns the per-series override saved on the animes row
// (migrated from the legacy anime_storage table); ok is false when the series
// is unknown, and season 0 means "automatic".
func queryAnimeStorage(animeTitle string) (dir string, season int, ok bool) {
	err := db.QueryRow("SELECT COALESCE(storage_dir,''),COALESCE(season,0) FROM animes WHERE title=?", animeTitle).Scan(&dir, &season)
	return dir, season, err == nil
}

// resolveSeason picks the effective season: an explicit item override wins,
// then the series setting, then Season 1.
func resolveSeason(animeTitle string, override int) int {
	if override >= 1 && override <= 999 {
		return override
	}
	if _, season, ok := queryAnimeStorage(animeTitle); ok && season >= 1 && season <= 999 {
		return season
	}
	return 1
}

// archiveTarget computes where a resource belongs: the series folder (a saved
// override wins over DOWNLOAD_DIR), the Season NN subfolder for episodes, and
// the media-library main name "番剧名 - S01E07". Resources without a
// recognized episode stay in the series root under their original name so
// media servers do not mistake them for specials.
func archiveTarget(animeTitle, episode, rawTitle string, seasonOverride int) (seriesRoot, seasonDir, base string) {
	series := strings.TrimSpace(animeTitle)
	if series == "" {
		series = "未分类番剧"
	}
	season := resolveSeason(series, seasonOverride)
	seriesRoot = filepath.Join(downloadDir, safeComponent(series))
	if dir, _, ok := queryAnimeStorage(series); ok {
		if d := strings.TrimSpace(dir); d != "" {
			seriesRoot = path.Clean(filepath.ToSlash(d))
		}
	}
	ep := strings.TrimSpace(episode)
	if ep == "" {
		base = safeComponent(rawTitle)
		return seriesRoot, "", base
	}
	seasonDir = fmt.Sprintf("Season %02d", season)
	base = safeComponent(safeComponent(series) + " - " + episodeTag(season, ep))
	return seriesRoot, seasonDir, base
}

// checkSeriesRoot rejects a symlinked series root so archives cannot be
// redirected out of the expected location.
func checkSeriesRoot(root string) error {
	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}
	if info, err := os.Lstat(root); err != nil || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("归档目录不可用或是符号链接")
	}
	return nil
}

// uniqueChildName picks base, base (2), base (3)… for a directory target that
// does not exist yet.
func uniqueChildName(dir, base string) string {
	candidate := base
	for i := 2; ; i++ {
		if _, err := os.Lstat(filepath.Join(dir, candidate)); os.IsNotExist(err) {
			return candidate
		}
		candidate = fmt.Sprintf("%s (%d)", base, i)
	}
}

// commitLinked moves one staged file into place via a hard link so an
// existing archive file is never overwritten; it returns os.ErrExist when the
// target name is taken.
func commitLinked(workPath, dstPath string) error {
	if err := os.Link(workPath, dstPath); err != nil {
		return err
	}
	return os.Remove(workPath)
}

// archiveParent creates the per-series directory after checking it is a real
// directory, not a symlink out of the archive root.
func archiveParent(root, series string) (string, error) {
	parent := filepath.Join(root, safeComponent(series))
	if err := os.MkdirAll(parent, 0755); err != nil {
		return "", err
	}
	if info, err := os.Lstat(parent); err != nil || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("归档目录不可用或是符号链接")
	}
	return parent, nil
}

func resourceExtension(res *http.Response) (string, error) {
	kind, _, _ := mime.ParseMediaType(res.Header.Get("Content-Type"))
	if kind == "text/html" || kind == "application/xhtml+xml" {
		return "", fmt.Errorf("下载地址返回网页，请使用资源直链或 RSS enclosure")
	}
	name := ""
	if _, params, err := mime.ParseMediaType(res.Header.Get("Content-Disposition")); err == nil {
		name = params["filename"]
	}
	if name == "" && res.Request != nil {
		name = res.Request.URL.Path
	}
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".mp4", ".mkv", ".webm", ".avi", ".mov", ".m4v", ".ts", ".torrent", ".ass", ".srt":
		return ext, nil
	}
	switch kind {
	case "application/x-bittorrent":
		return ".torrent", nil
	case "video/mp4":
		return ".mp4", nil
	case "video/x-matroska":
		return ".mkv", nil
	case "video/webm":
		return ".webm", nil
	}
	return ".bin", nil
}

// writeArchiveMeta writes info.json and the episode NFO next to the media
// using the shared base name, so several episodes can live in one folder.
func writeArchiveMeta(dir string, m archiveMetadata, baseName string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(dir, baseName+"-info.json"), append(data, '\n'), 0644); err != nil {
		return err
	}
	nfo := episodeNFO{Title: m.AnimeTitle, ShowTitle: m.AnimeTitle, Season: m.Season, Episode: m.Episode, Studio: m.Group, Plot: m.Description, DateAdded: m.SavedAt}
	nfo.Video.Quality = m.Quality
	nfoData, err := xml.MarshalIndent(nfo, "", "  ")
	if err != nil {
		return err
	}
	nfoPath := filepath.Join(dir, baseName+".nfo")
	return os.WriteFile(nfoPath, append([]byte(xml.Header), append(nfoData, '\n')...), 0644)
}

// writeTvshowNFO refreshes the series-level NFO in the series root. Metadata
// is only what this app knows (title, Bangumi binding); it never pretends to
// be verified TMDB data. Failures are logged and never fail the archive.
func writeTvshowNFO(a *animeRow) {
	if a == nil || strings.TrimSpace(a.Title) == "" {
		return
	}
	seriesRoot, _, _ := archiveTarget(a.Title, "", "", 0)
	var nameCN string
	var bangumiID int64
	db.QueryRow(`SELECT COALESCE(animes.bangumi_id,0),COALESCE(anime_meta.name_cn,'') FROM animes
		LEFT JOIN anime_meta ON anime_meta.anime_title=animes.title WHERE animes.title=?`, a.Title).Scan(&bangumiID, &nameCN)
	nfo := tvshowNFO{Title: strings.TrimSpace(nameCN)}
	if nfo.Title == "" {
		nfo.Title = a.Title
	}
	if bangumiID > 0 {
		nfo.UniqueID = &tvshowUniqueID{Type: "bangumi", ID: bangumiID}
	}
	data, err := xml.MarshalIndent(nfo, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(seriesRoot, 0755); err != nil {
		log.Printf("tvshow.nfo 写入失败 %q: %v", seriesRoot, err)
		return
	}
	if err := os.WriteFile(filepath.Join(seriesRoot, "tvshow.nfo"), append([]byte(xml.Header), append(data, '\n')...), 0644); err != nil {
		log.Printf("tvshow.nfo 写入失败 %q: %v", seriesRoot, err)
	}
}

// Commit an entire resource only after the media and metadata have been fully
// written into a staging directory. Existing archive files are linked into
// place under a unique name, never overwritten.
func saveArchive(m archiveMetadata, res *http.Response) (string, error) {
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("资源返回 HTTP %d", res.StatusCode)
	}
	ext, err := resourceExtension(res)
	if err != nil {
		return "", err
	}
	if ext == ".torrent" {
		return "", errTorrentResource
	}
	seriesRoot, seasonDir, base := archiveTarget(m.AnimeTitle, m.Episode, m.Title, m.Season)
	if err := checkSeriesRoot(seriesRoot); err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(seriesRoot, fmt.Sprintf(".partial-%d-", m.ID))
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage) // Only the freshly allocated temporary directory.
	work := filepath.Join(stage, seasonDir)
	if seasonDir != "" {
		if err := os.MkdirAll(work, 0755); err != nil {
			return "", err
		}
	}
	m.File = base + ext
	f, err := os.OpenFile(filepath.Join(work, m.File), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return "", err
	}
	m.Size, err = io.Copy(f, res.Body)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	if res.ContentLength >= 0 && m.Size != res.ContentLength {
		return "", fmt.Errorf("资源长度不完整")
	}
	m.SavedAt = time.Now().UTC().Format(time.RFC3339)
	if err = writeArchiveMeta(work, m, base); err != nil {
		return "", err
	}
	// Pick a free "base", "base (2)"… name and link the media in first; the
	// metadata follows under the same final base name.
	finalDir := filepath.Join(seriesRoot, seasonDir)
	if err := os.MkdirAll(finalDir, 0755); err != nil {
		return "", err
	}
	final := base
	for i := 2; ; i++ {
		if err := commitLinked(filepath.Join(work, m.File), filepath.Join(finalDir, final+ext)); err == nil {
			break
		} else if !os.IsExist(err) {
			return "", err
		}
		final = fmt.Sprintf("%s (%d)", base, i)
	}
	for _, suffix := range []string{".nfo", "-info.json"} {
		if err := commitLinked(filepath.Join(work, base+suffix), filepath.Join(finalDir, final+suffix)); err != nil {
			os.Remove(filepath.Join(finalDir, final+ext))
			return "", err
		}
	}
	writeTvshowNFO(&animeRow{Title: m.AnimeTitle})
	return filepath.Join(finalDir, final+ext), nil
}

// archiveMainName picks the metadata "file" value: the single file itself, or
// the top-level folder of a multi-file torrent.
func archiveMainName(files []archiveFile) string {
	first := files[0].Rel
	if len(files) == 1 {
		return first
	}
	if dir := filepath.ToSlash(filepath.Dir(first)); dir != "." && dir != "" {
		return dir
	}
	return first
}

func copyArchiveFile(dst, src string) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return 0, err
	}
	size, err := io.Copy(out, in)
	if err == nil {
		err = out.Sync()
	}
	closeErr := out.Close()
	if err == nil {
		err = closeErr
	}
	return size, err
}

// archiveFromFiles copies files that a BT downloader has already completed into
// the same archive layout as direct downloads. The downloader keeps its own
// copy for seeding. Multi-file resources land in a per-episode folder named
// like the media files, so media servers still match them.
func archiveFromFiles(id int, files []archiveFile) (string, error) {
	if len(files) == 0 {
		return "", fmt.Errorf("下载器未返回文件列表")
	}
	m, err := archiveInfo(id)
	if err != nil {
		return "", err
	}
	for _, f := range files {
		if safeRelPath(f.Rel) == "" {
			return "", fmt.Errorf("文件名无效：%s", f.Rel)
		}
		if !filepath.IsAbs(f.Src) {
			return "", fmt.Errorf("下载器文件路径不是绝对路径：%s", f.Src)
		}
		if _, err := os.Stat(f.Src); err != nil {
			return "", fmt.Errorf("下载器文件不可读：%s", f.Src)
		}
	}
	seriesRoot, seasonDir, base := archiveTarget(m.AnimeTitle, m.Episode, m.Title, m.Season)
	if err := checkSeriesRoot(seriesRoot); err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(seriesRoot, fmt.Sprintf(".partial-%d-", m.ID))
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	workDir := filepath.Join(stage, seasonDir, base)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return "", err
	}
	for _, f := range files {
		rel := safeRelPath(f.Rel)
		dst := filepath.Join(workDir, filepath.FromSlash(rel))
		if !strings.HasPrefix(dst, workDir+string(os.PathSeparator)) {
			return "", fmt.Errorf("文件路径越界：%s", f.Rel)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return "", err
		}
		size, err := copyArchiveFile(dst, f.Src)
		m.Size += size
		if err != nil {
			return "", err
		}
	}
	m.File = archiveMainName(files)
	m.SavedAt = time.Now().UTC().Format(time.RFC3339)
	if err := writeArchiveMeta(workDir, m, base); err != nil {
		return "", err
	}
	finalDir := filepath.Join(seriesRoot, seasonDir)
	if err := os.MkdirAll(finalDir, 0755); err != nil {
		return "", err
	}
	name := uniqueChildName(finalDir, base)
	if err := os.Rename(workDir, filepath.Join(finalDir, name)); err != nil {
		return "", err
	}
	writeTvshowNFO(&animeRow{Title: m.AnimeTitle})
	return filepath.Join(finalDir, name, m.File), nil
}

// ---- 手动纠错后的文件迁移 ----

// rearchiveSavedItem moves an already-saved resource (single file or a
// multi-file episode folder) to the location implied by corrected metadata.
// The move happens before the item row is updated, so a failure leaves the
// database untouched.
func rearchiveSavedItem(id int64, newTitle, newEpisode string, newSeason int) (string, error) {
	var oldPath string
	if err := db.QueryRow("SELECT COALESCE(file_path,'') FROM items WHERE id=?", id).Scan(&oldPath); err != nil {
		return "", err
	}
	m, err := archiveInfo(int(id))
	if err != nil {
		return "", err
	}
	// 元数据按纠错后的值重写；原始标题等字段保持不变。
	m.AnimeTitle, m.Episode, m.Season = newTitle, newEpisode, resolveSeason(newTitle, newSeason)
	m.SavedAt = time.Now().UTC().Format(time.RFC3339)
	seriesRoot, seasonDir, base := archiveTarget(newTitle, newEpisode, m.Title, newSeason)
	oldParent := filepath.Dir(oldPath)
	oldBase := strings.TrimSuffix(filepath.Base(oldPath), filepath.Ext(oldPath))
	// 多文件资源落在与集数同名的文件夹里，文件夹内包含 base.nfo。
	isFolder := false
	if info, err := os.Stat(filepath.Join(oldParent, filepath.Base(oldParent)+".nfo")); err == nil && !info.IsDir() {
		isFolder = true
	}
	if err := os.MkdirAll(filepath.Join(seriesRoot, seasonDir), 0755); err != nil {
		return "", err
	}
	if isFolder {
		newName := uniqueChildName(filepath.Join(seriesRoot, seasonDir), base)
		dst := filepath.Join(seriesRoot, seasonDir, newName)
		if err := moveTree(oldParent, dst); err != nil {
			return "", err
		}
		if oldBase != base {
			os.Remove(filepath.Join(dst, oldBase+".nfo"))
			os.Remove(filepath.Join(dst, oldBase+"-info.json"))
		}
		if err := writeArchiveMeta(dst, m, base); err != nil {
			return "", err
		}
		cleanupEmptyDirs(filepath.Dir(oldParent), seriesRoot)
		writeTvshowNFO(&animeRow{Title: newTitle})
		return filepath.Join(dst, filepath.Base(oldPath)), nil
	}
	final := base
	for i := 2; ; i++ {
		if _, err := os.Lstat(filepath.Join(seriesRoot, seasonDir, final+filepath.Ext(oldPath))); os.IsNotExist(err) {
			break
		}
		final = fmt.Sprintf("%s (%d)", base, i)
	}
	dst := filepath.Join(seriesRoot, seasonDir, final+filepath.Ext(oldPath))
	if err := movePath(oldPath, dst); err != nil {
		return "", err
	}
	os.Remove(filepath.Join(oldParent, oldBase+".nfo"))
	os.Remove(filepath.Join(oldParent, oldBase+"-info.json"))
	if err := writeArchiveMeta(filepath.Join(seriesRoot, seasonDir), m, final); err != nil {
		return "", err
	}
	cleanupEmptyDirs(oldParent, seriesRoot)
	writeTvshowNFO(&animeRow{Title: newTitle})
	return dst, nil
}

// movePath renames a file, falling back to copy+delete across devices.
func movePath(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if _, err := copyArchiveFile(dst, src); err != nil {
		os.Remove(dst)
		return err
	}
	return os.Remove(src)
}

// moveTree copies a directory tree, then removes the source. Used when the
// old and the corrected series root live on different volumes.
func moveTree(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	for _, entry := range entries {
		s := filepath.Join(src, entry.Name())
		d := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := moveTree(s, d); err != nil {
				return err
			}
			continue
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("不支持的文件类型：%s", entry.Name())
		}
		if err := movePath(s, d); err != nil {
			return err
		}
	}
	return os.Remove(src)
}

// cleanupEmptyDirs removes the empty directories left behind by a move, but
// never removes the archive root itself or a directory that is not empty.
func cleanupEmptyDirs(dir, stopAt string) {
	for dir != "" && dir != stopAt && dir != downloadDir {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		if os.Remove(dir) != nil {
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		dir = parent
	}
}

func fetchArchive(id int, link string) (string, error) {
	u, err := url.Parse(link)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("资源地址无效")
	}
	m, err := archiveInfo(id)
	if err != nil {
		return "", err
	}
	res, err := mediaClient.Get(link)
	if err != nil {
		return "", fmt.Errorf("资源连接失败")
	}
	defer res.Body.Close()
	return saveArchive(m, res)
}

// RSS/RPC calls remain short; large media downloads may take several hours.
var mediaClient = &http.Client{Timeout: 24 * time.Hour, Transport: newMediaTransport()}

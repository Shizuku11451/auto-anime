package main

import (
	"context"
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
	// AutoSeason 是标题分析出的季（items.season_auto）；用户没在条目上指定季时用它归档。
	AutoSeason int    `json:"-"`
	Episode    string `json:"episode"`
	Quality    string `json:"quality"`
	Group      string `json:"fansub_group"`
	File       string `json:"file"`
	Size       int64  `json:"size_bytes"`
	SavedAt    string `json:"saved_at"`
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
        COALESCE(published,''),COALESCE(description,''),COALESCE(anime_title,''),COALESCE(items.anime_id,0),COALESCE(items.season,0),COALESCE(items.season_auto,0),COALESCE(episode,''),COALESCE(quality,''),COALESCE(fansub_group,'')
        FROM items LEFT JOIN feeds ON feeds.id=items.feed_id WHERE items.id=?`, id).
		Scan(&m.FeedID, &m.FeedName, &m.Title, &m.Published, &m.Description, &m.AnimeTitle, &m.AnimeID, &m.Season, &m.AutoSeason, &m.Episode, &m.Quality, &m.Group)
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
	base := strings.ToUpper(strings.TrimRight(strings.SplitN(name, ".", 2)[0], " "))
	device := strings.NewReplacer("¹", "1", "²", "2", "³", "3").Replace(base)
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || base == "CONIN$" || base == "CONOUT$" || (len(device) == 4 && (strings.HasPrefix(device, "COM") || strings.HasPrefix(device, "LPT")) && device[3] >= '1' && device[3] <= '9') {
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

// resolveSeason 选出生效的季号，优先级：
//  1. 用户在条目上显式指定的季（items.season，0 表示跟随番剧设置）；
//  2. 番剧设置里的季（animes.season）；
//  3. 标题分析解析出的季（items.season_auto，例如「3期」「S3」「第三季」）；
//  4. 第 1 季。
func resolveSeason(animeTitle string, itemSeason, autoSeason int) int {
	if itemSeason >= 1 && itemSeason <= 999 {
		return itemSeason
	}
	if _, season, ok := queryAnimeStorage(animeTitle); ok && season >= 1 && season <= 999 {
		return season
	}
	if autoSeason >= 1 && autoSeason <= 999 {
		return autoSeason
	}
	return 1
}

// archiveBaseRoot is the default parent folder of all series folders: the
// configured media library directory (媒体库目录, for fnOS/Emby/Jellyfin to
// scan) when set, otherwise DOWNLOAD_DIR.
func archiveBaseRoot() string {
	cfg := runtimeConfig()
	if d := strings.TrimSpace(cfg.libraryDir); d != "" {
		return cleanStoragePath(d)
	}
	return downloadDir
}

// archiveRootFor resolves where a series folder lives: a saved per-series
// override wins, then the media library directory, then DOWNLOAD_DIR.
func archiveRootFor(series string) string {
	root := filepath.Join(archiveBaseRoot(), safeComponent(series))
	if dir, _, ok := queryAnimeStorage(series); ok {
		if d := strings.TrimSpace(dir); d != "" {
			root = cleanStoragePath(d)
		}
	}
	return root
}

// archiveTarget computes where a resource belongs: the series folder (a saved
// override wins over the media library directory), the Season NN subfolder for
// episodes, and the media-library main name "番剧名 - S01E07". Resources
// without a recognized episode stay in the series root under their original
// name so media servers do not mistake them for specials.
func archiveTarget(animeTitle, episode, rawTitle string, itemSeason, autoSeason int) (seriesRoot, seasonDir, base string) {
	series := strings.TrimSpace(animeTitle)
	if series == "" {
		series = "未分类番剧"
	}
	season := resolveSeason(series, itemSeason, autoSeason)
	seriesRoot = archiveRootFor(series)
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
	if info, err := os.Lstat(root); err != nil || archivePathIsLink(root, info) {
		return fmt.Errorf("归档目录不可用或是符号链接")
	}
	return nil
}

func checkArchiveChild(root, child string) error {
	if child == "" {
		return nil
	}
	dir := filepath.Join(root, child)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if archivePathIsLink(dir, info) {
		return fmt.Errorf("归档季目录不可用或是符号链接")
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
		return commitWithoutHardlink(workPath, dstPath, err)
	}
	return os.Remove(workPath)
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
	seriesRoot, _, _ := archiveTarget(a.Title, "", "", 0, 0)
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
	refreshSeriesPoster(a.Title)
}

// refreshSeriesPoster mirrors the cached Bangumi cover into the series root as
// poster.jpg/poster.png so fnOS, Kodi and friends show artwork from local
// files instead of scraping. A series without a cover gets its poster removed.
// Failures are logged and never fail the caller.
func refreshSeriesPoster(animeTitle string) {
	if strings.TrimSpace(animeTitle) == "" {
		return
	}
	seriesRoot, _, _ := archiveTarget(animeTitle, "", "", 0, 0)
	var coverFile string
	db.QueryRow(`SELECT COALESCE(cover_file,'') FROM anime_meta WHERE anime_title=?`, animeTitle).Scan(&coverFile)
	coverFile = strings.TrimSpace(coverFile)
	for _, ext := range []string{".jpg", ".png", ".webp", ".gif"} {
		stale := filepath.Join(seriesRoot, "poster"+ext)
		if coverFile == "" || !coverFilePattern.MatchString(coverFile) || ext != strings.ToLower(filepath.Ext(coverFile)) {
			os.Remove(stale)
		}
	}
	if coverFile == "" || !coverFilePattern.MatchString(coverFile) {
		return
	}
	src := filepath.Join(coversDir, coverFile)
	if _, err := os.Stat(src); err != nil {
		return
	}
	if err := os.MkdirAll(seriesRoot, 0755); err != nil {
		log.Printf("poster 写入失败 %q: %v", seriesRoot, err)
		return
	}
	dst := filepath.Join(seriesRoot, "poster"+filepath.Ext(coverFile))
	os.Remove(dst)
	if err := os.Link(src, dst); err != nil {
		if _, copyErr := copyArchiveFile(dst, src); copyErr != nil && !os.IsExist(copyErr) {
			log.Printf("poster 写入失败 %q: %v", seriesRoot, copyErr)
		}
	}
}

// Commit an entire resource only after the media and metadata have been fully
// written into a staging directory. Existing archive files are linked into
// place under a unique name, never overwritten.
func saveArchiveContext(ctx context.Context, m archiveMetadata, res *http.Response) (string, error) {
	m.Season = resolveSeason(m.AnimeTitle, m.Season, m.AutoSeason)
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
	seriesRoot, seasonDir, base := archiveTarget(m.AnimeTitle, m.Episode, m.Title, m.Season, m.AutoSeason)
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
	reader := &transferReader{reader: res.Body, id: m.ID, total: res.ContentLength, windowStart: time.Now()}
	m.Size, err = io.Copy(f, reader)
	clearTransfer(m.ID)
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
	if err := ctx.Err(); err != nil {
		return "", err
	}
	m.SavedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err = writeArchiveMeta(work, m, base); err != nil {
		return "", err
	}
	// Pick a free "base", "base (2)"… name and link the media in first; the
	// metadata follows under the same final base name.
	finalDir := filepath.Join(seriesRoot, seasonDir)
	if err := checkArchiveChild(seriesRoot, seasonDir); err != nil {
		return "", err
	}
	final := base
	for i := 2; ; i++ {
		if err := prepareArchiveCommit(m.ID, filepath.Join(finalDir, final+ext), filepath.Join(finalDir, final+"-info.json"), m.SavedAt); err != nil {
			return "", err
		}
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

// videoExtensions lists the container names media servers recognize; subtitles
// and extras keep their original names.
var videoExtensions = map[string]bool{".mp4": true, ".mkv": true, ".webm": true, ".avi": true, ".mov": true, ".m4v": true, ".ts": true}

// pickMainVideo returns the index of the largest video file in a torrent so its
// archived copy can carry the media-library name; -1 when there is no video.
func pickMainVideo(files []archiveFile) int {
	best, bestSize := -1, int64(-1)
	for i, f := range files {
		if !videoExtensions[strings.ToLower(filepath.Ext(f.Src))] {
			continue
		}
		info, err := os.Stat(f.Src)
		if err != nil || info.Size() <= bestSize {
			continue
		}
		best, bestSize = i, info.Size()
	}
	return best
}

// archiveFromFiles copies files that a BT downloader has already completed into
// the same archive layout as direct downloads. The downloader keeps its own
// copy for seeding. Multi-file resources land in a per-episode folder named
// like the media files, and the largest video is renamed to the media-library
// name: fnOS and friends match episodes by file name only, never by folders.
func archiveFromFiles(id int, files []archiveFile) (string, error) {
	if len(files) == 0 {
		return "", fmt.Errorf("下载器未返回文件列表")
	}
	m, err := archiveInfo(id)
	if err != nil {
		return "", err
	}
	m.Season = resolveSeason(m.AnimeTitle, m.Season, m.AutoSeason)
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
	seriesRoot, seasonDir, base := archiveTarget(m.AnimeTitle, m.Episode, m.Title, m.Season, m.AutoSeason)
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
	// The main video keeps its place in the torrent layout but takes the
	// "番剧名 - SxxEyy" name, so the episode is recognizable by file name.
	mainRel := ""
	if seasonDir != "" {
		if idx := pickMainVideo(files); idx >= 0 {
			mainRel = safeRelPath(files[idx].Rel)
		}
	}
	for _, f := range files {
		rel := safeRelPath(f.Rel)
		dst := filepath.Join(workDir, filepath.FromSlash(rel))
		if !strings.HasPrefix(dst, workDir+string(os.PathSeparator)) {
			return "", fmt.Errorf("文件路径越界：%s", f.Rel)
		}
		if rel == mainRel {
			dst = filepath.Join(filepath.Dir(dst), base+filepath.Ext(dst))
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
	if mainRel != "" {
		m.File = filepath.ToSlash(filepath.Join(filepath.Dir(mainRel), base+filepath.Ext(mainRel)))
	} else {
		m.File = archiveMainName(files)
	}
	m.SavedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := writeArchiveMeta(workDir, m, base); err != nil {
		return "", err
	}
	finalDir := filepath.Join(seriesRoot, seasonDir)
	if err := checkArchiveChild(seriesRoot, seasonDir); err != nil {
		return "", err
	}
	name := uniqueChildName(finalDir, base)
	if err := prepareArchiveCommit(m.ID, filepath.Join(finalDir, name, m.File), filepath.Join(finalDir, name, base+"-info.json"), m.SavedAt); err != nil {
		return "", err
	}
	if err := os.Rename(workDir, filepath.Join(finalDir, name)); err != nil {
		return "", err
	}
	writeTvshowNFO(&animeRow{Title: m.AnimeTitle})
	return filepath.Join(finalDir, name, m.File), nil
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

func fetchArchiveContext(ctx context.Context, id int, link string) (string, error) {
	u, err := url.Parse(link)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("资源地址无效")
	}
	m, err := archiveInfo(id)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return "", err
	}
	res, err := mediaClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("资源连接失败")
	}
	defer res.Body.Close()
	return saveArchiveContext(ctx, m, res)
}

// RSS/RPC calls remain short; large media downloads may take several hours.
var mediaClient = &http.Client{Timeout: 24 * time.Hour, Transport: newMediaTransport()}

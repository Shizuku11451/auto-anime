package main

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
	Episode   string   `xml:"episode,omitempty"`
	Studio    string   `xml:"studio,omitempty"`
	Video     struct {
		Quality string `xml:"quality,omitempty"`
	} `xml:"video"`
	Plot      string `xml:"plot,omitempty"`
	DateAdded string `xml:"dateadded,omitempty"`
}

// errTorrentResource marks a download that turned out to be a .torrent file:
// storing it must never count as a saved episode.
var errTorrentResource = errors.New("资源是种子文件")

func archiveInfo(id int) (archiveMetadata, error) {
	m := archiveMetadata{ID: id}
	err := db.QueryRow(`SELECT COALESCE(items.feed_id,0),COALESCE(feeds.name,''),items.title,
        COALESCE(published,''),COALESCE(description,''),COALESCE(anime_title,''),COALESCE(episode,''),COALESCE(quality,''),COALESCE(fansub_group,'')
        FROM items LEFT JOIN feeds ON feeds.id=items.feed_id WHERE items.id=?`, id).
		Scan(&m.FeedID, &m.FeedName, &m.Title, &m.Published, &m.Description, &m.AnimeTitle, &m.Episode, &m.Quality, &m.Group)
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

func writeArchiveMeta(stage string, m archiveMetadata, mainName string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(stage, "info.json"), append(data, '\n'), 0644); err != nil {
		return err
	}
	nfo := episodeNFO{Title: m.AnimeTitle, ShowTitle: m.AnimeTitle, Episode: m.Episode, Studio: m.Group, Plot: m.Description, DateAdded: m.SavedAt}
	nfo.Video.Quality = m.Quality
	nfoData, err := xml.MarshalIndent(nfo, "", "  ")
	if err != nil {
		return err
	}
	nfoPath := filepath.Join(stage, strings.TrimSuffix(mainName, filepath.Ext(mainName))+".nfo")
	return os.WriteFile(nfoPath, append([]byte(xml.Header), append(nfoData, '\n')...), 0644)
}

// commitStage renames the fully written temporary directory into place under a
// fresh random name, so independent resources never collide.
func commitStage(parent, stage string) (string, error) {
	final := filepath.Join(parent, strings.TrimPrefix(filepath.Base(stage), ".partial-"))
	if _, err := os.Lstat(final); !os.IsNotExist(err) {
		return "", fmt.Errorf("归档目标已存在或不可访问")
	}
	if err := os.Rename(stage, final); err != nil {
		return "", err
	}
	return final, nil
}

// Commit an entire resource directory only after the media and metadata have
// been fully written. Existing archives are never opened for writing.
func saveArchive(root string, m archiveMetadata, res *http.Response) (string, error) {
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
	series := m.AnimeTitle
	if strings.TrimSpace(series) == "" {
		series = "未分类番剧"
	}
	parent, err := archiveParent(root, series)
	if err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(parent, fmt.Sprintf(".partial-%d-", m.ID))
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage) // Only the freshly allocated temporary directory.
	m.File = safeComponent(m.Title) + ext
	f, err := os.OpenFile(filepath.Join(stage, m.File), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
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
	if err = writeArchiveMeta(stage, m, m.File); err != nil {
		return "", err
	}
	final, err := commitStage(parent, stage)
	if err != nil {
		return "", err
	}
	return filepath.Join(final, m.File), nil
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
// copy for seeding.
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
	series := m.AnimeTitle
	if strings.TrimSpace(series) == "" {
		series = "未分类番剧"
	}
	parent, err := archiveParent(downloadDir, series)
	if err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(parent, fmt.Sprintf(".partial-%d-", m.ID))
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	for _, f := range files {
		rel := safeRelPath(f.Rel)
		dst := filepath.Join(stage, filepath.FromSlash(rel))
		if !strings.HasPrefix(dst, stage+string(os.PathSeparator)) {
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
	if err := writeArchiveMeta(stage, m, m.File); err != nil {
		return "", err
	}
	final, err := commitStage(parent, stage)
	if err != nil {
		return "", err
	}
	return filepath.Join(final, m.File), nil
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
	return saveArchive(downloadDir, m, res)
}

// RSS/RPC calls remain short; large media downloads may take several hours.
var mediaClient = &http.Client{Timeout: 24 * time.Hour, Transport: &http.Transport{Proxy: nil, ResponseHeaderTimeout: 30 * time.Second}}

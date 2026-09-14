package main

// 内置播放器的媒体流接口：按 HTTP Range 提供已归档视频，
// 浏览器只拉取需要的字节段，拖动进度与起播都不必下载整个文件。

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// streamContentTypes 显式指定视频 MIME，不依赖运行平台的 mime 数据库
// （Windows 会读注册表，NAS 容器内可能没有扩展名映射）；
// ServeContent 只在响应未带 Content-Type 时才自行推断。
var streamContentTypes = map[string]string{
	".mp4":  "video/mp4",
	".m4v":  "video/x-m4v",
	".mkv":  "video/x-matroska",
	".webm": "video/webm",
	".mov":  "video/quicktime",
	".avi":  "video/x-msvideo",
	".ts":   "video/mp2t",
	".m2ts": "video/mp2t",
	".flv":  "video/x-flv",
	".wmv":  "video/x-ms-wmv",
}

// streamMediaPath 解析条目当前可播放的媒体文件：
// 仅已保存条目可播；file_path 必须存在且是文件。
// 旧的嵌套归档记录可能仍指向集文件夹，此时挑选其中最大的视频文件。
func streamMediaPath(id int64) (string, error) {
	var status, filePath string
	err := db.QueryRow("SELECT COALESCE(status,''),COALESCE(file_path,'') FROM items WHERE id=?", id).
		Scan(&status, &filePath)
	if err != nil {
		return "", errNotPlayable("条目不存在")
	}
	if status != "已保存" || filePath == "" {
		return "", errNotPlayable("资源还没有已保存的文件")
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return "", errNotPlayable("文件不存在或不可读")
	}
	if !info.IsDir() {
		return filePath, nil
	}
	entries, err := os.ReadDir(filePath)
	if err != nil {
		return "", errNotPlayable("目录不可读")
	}
	files := make([]archiveFile, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		files = append(files, archiveFile{Rel: e.Name(), Src: filepath.Join(filePath, e.Name())})
	}
	if i := pickMainVideo(files); i >= 0 {
		return files[i].Src, nil
	}
	return "", errNotPlayable("目录中没有视频文件")
}

type streamError struct{ msg string }

func errNotPlayable(msg string) error { return streamError{msg} }

func (e streamError) Error() string { return e.msg }

func streamItem(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "方法不支持", http.StatusMethodNotAllowed)
		return
	}
	path, err := streamMediaPath(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		http.Error(w, "文件无法读取", http.StatusNotFound)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		http.Error(w, "文件无法读取", http.StatusNotFound)
		return
	}
	if ct := streamContentTypes[strings.ToLower(filepath.Ext(path))]; ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	// ServeContent 处理 Range/If-Range/HEAD，以 io.Seeker 定位读取，
	// 不把文件读进内存；ModTime 让浏览器可做条件请求。
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), f)
}

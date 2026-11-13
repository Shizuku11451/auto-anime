package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// 由全局 auth 中间件保护；初始化前不允许探测任意目录。
func settingsStorageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		http.Error(w, "方法不支持", http.StatusMethodNotAllowed)
		return
	}
	if !allowSetupWrite(w, r) {
		return
	}
	if setupRequired() {
		http.Error(w, "请先完成首次安装引导", http.StatusForbidden)
		return
	}
	var in struct {
		Path string `json:"path"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil {
		http.Error(w, "请求参数无效", http.StatusBadRequest)
		return
	}
	dir, err := validateLibraryDir(in.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if dir == "" {
		dir = downloadDir
	}
	jsonOut(w, probeSetupStorage(dir))
}

// Setup is unauthenticated, so only same-host browser requests may write.
// CLI clients without browser headers remain supported.
func allowSetupWrite(w http.ResponseWriter, r *http.Request) bool {
	allowed := r.Header.Get("Sec-Fetch-Site") != "cross-site"
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		u, err := url.Parse(origin)
		allowed = allowed && err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == "" && strings.EqualFold(u.Host, r.Host)
	}
	if !allowed {
		http.Error(w, "请求来源不被允许", http.StatusForbidden)
	}
	return allowed
}

type setupStorageResult struct {
	Path        string `json:"path"`
	MountStatus string `json:"mount_status"`
	Writable    bool   `json:"writable"`
	Message     string `json:"message"`
}

// Probe only an already-existing directory. Do not silently create NAS paths
// inside the container writable layer or accept arbitrary anonymous paths.
func probeSetupStorage(dir string) setupStorageResult {
	out := setupStorageResult{Path: dir, MountStatus: "unknown"}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		out.Message = "媒体目录不存在或无法访问。请在 NAS 创建文件夹，检查目录映射与权限后重新部署。"
		return out
	}
	mounts := readMountPoints()
	root, hasRoot := longestMount("/", mounts)
	if hasRoot && !containerRootFS(root.fsType) {
		out.MountStatus = "not_required"
	} else if hasRoot {
		target, found := longestMount(filepath.ToSlash(resolved), mounts)
		if found && target.path != "/" && !containerRootFS(target.fsType) && (target.fsType == "ext4" || target.fsType == "btrfs" || target.fsType == "xfs" || target.fsType == "zfs" || target.fsType == "nfs" || target.fsType == "nfs4" || target.fsType == "cifs" || target.fsType == "fuseblk" || target.fsType == "exfat" || target.fsType == "fuse.mergerfs") {
			out.MountStatus = "mapped"
		} else if !found || target.path == "/" || containerRootFS(target.fsType) || target.fsType == "tmpfs" {
			out.MountStatus = "unmapped"
			out.Message = "媒体目录未映射到持久化存储。请修改 Compose 的 volumes，将 NAS 番剧目录映射到 /media，再重新部署。"
			return out
		}
	}
	f, err := os.CreateTemp(resolved, ".auto-anime-check-*")
	if err != nil {
		out.Message = "媒体目录无法写入，请检查 NAS 文件夹权限、只读挂载与剩余空间。"
		return out
	}
	name := f.Name()
	_, writeErr := f.WriteString("auto-anime storage check\n")
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	removeErr := os.Remove(name)
	if removeErr != nil {
		out.Message = "临时文件无法删除，请检查目录删除权限；检测文件以 .auto-anime-check- 开头。"
		return out
	}
	if writeErr != nil || closeErr != nil {
		out.Message = "媒体目录写入失败，请检查目录权限与剩余空间。"
		return out
	}
	out.Writable = true
	switch out.MountStatus {
	case "mapped":
		out.Message = "媒体目录已映射，可以写入。"
	case "not_required":
		out.Message = "媒体目录可以写入；当前环境无需 Docker 挂载检查。"
	default:
		out.Message = "媒体目录可以写入，但无法确认挂载。Docker 用户仍需核对 Compose 的 volumes，确保文件保存在 NAS 上。"
	}
	return out
}

func setupStorageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		http.Error(w, "方法不支持", http.StatusMethodNotAllowed)
		return
	}
	if !allowSetupWrite(w, r) {
		return
	}
	settingsMu.Lock()
	defer settingsMu.Unlock()
	if !setupRequired() {
		http.Error(w, "已完成初始化，请登录后管理目录", http.StatusForbidden)
		return
	}
	dir := strings.TrimSpace(runtimeConfig().libraryDir)
	if dir == "" {
		dir = downloadDir
	}
	jsonOut(w, probeSetupStorage(dir))
}

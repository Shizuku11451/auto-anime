package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

// 用户自定义设置：settings 表为 KV 存储；数据库值优先，环境变量回退。
// 保存后立即更新对应的全局变量（热生效，无需重启容器）。

var setupDone bool
var webHash string        // v2 bcrypt；兼容旧 v1，非空时优先于环境变量
var settingsMu sync.Mutex // Serialize settings writes and login session issuance.

func getSetting(key string) string {
	var value string
	db.QueryRow("SELECT value FROM settings WHERE key=?", key).Scan(&value)
	return value
}

// loadSettings applies stored overrides over the env-initialized globals.
func loadSettings() {
	rows, err := db.Query("SELECT key,value FROM settings")
	if err != nil {
		log.Println("读取用户设置失败:", err)
		return
	}
	stored := map[string]string{}
	for rows.Next() {
		var key, value string
		if rows.Scan(&key, &value) == nil {
			stored[key] = value
		}
	}
	rows.Close()
	if _, ok := stored["setup_done"]; !ok {
		stored["setup_done"] = "0"
	}
	applyStoredSettings(stored)
}

// applyStoredSettings maps persisted settings onto the global configuration.
// Regular fields take effect whenever the key exists in the table (so clearing
// one in the UI reverts to the env/default); secrets only ever move upwards.
func applyStoredSettings(stored map[string]string) {
	runtimeConfigMu.Lock()
	defer runtimeConfigMu.Unlock()
	if v, ok := stored["max_concurrent_downloads"]; ok {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= maximumConcurrentDownloads {
			configuredDownloads.Store(int64(n))
		}
	}
	if v, ok := stored["download_mode"]; ok && v != "" {
		downloadMode = v
	}
	if downloadMode == "builtin" {
		// 环境变量和数据库旧值仅在本进程回退；活跃任务由启动恢复统一退役。
		downloadMode = "native"
		log.Println("配置的下载方式是已移除的内置 qBittorrent（builtin），本次运行改用内置 BT 下载器")
	}
	if v, ok := stored["aria2_url"]; ok {
		aria2URL = strings.TrimRight(v, "/")
	}
	if v, ok := stored["aria2_dir"]; ok {
		aria2Dir = v
	}
	if v, ok := stored["aria2_local_dir"]; ok {
		aria2LocalDir = v
	}
	if v, ok := stored["qbit_url"]; ok {
		qbitURL = strings.TrimRight(v, "/")
	}
	if v, ok := stored["qbit_user"]; ok {
		qbitUser = v
	}
	if v, ok := stored["qbit_save_path"]; ok {
		qbitSavePath = v
	}
	if v, ok := stored["qbit_local_dir"]; ok {
		qbitLocalDir = v
	}
	if v, ok := stored["transmission_url"]; ok {
		transmissionURL = strings.TrimRight(v, "/")
	}
	if v, ok := stored["transmission_user"]; ok {
		transmissionUser = v
	}
	if v, ok := stored["transmission_download_dir"]; ok {
		transmissionDownloadDir = v
	}
	if v, ok := stored["transmission_local_dir"]; ok {
		transmissionLocalDir = v
	}
	if v, ok := stored["keep_source_files"]; ok {
		keepSourceFiles = v == "1"
	}
	// 只写不读的敏感字段：仅在有值时更新，无法通过设置页清除。
	for key, target := range map[string]*string{
		"aria2_token":           &aria2Token,
		"qbit_password":         &qbitPassword,
		"transmission_password": &transmissionPassword,
	} {
		if v := stored[key]; v != "" {
			*target = v
		}
	}
	if v, ok := stored["proxy_url"]; ok {
		if err := applyProxyConfig(v); err != nil && v != "" {
			log.Printf("忽略无效的代理设置: %v", err)
		}
	}
	if v := stored["poll_minutes"]; v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 5 {
			pollMinutes = n
		}
	}
	// 媒体库目录：键存在即生效（含清空回退 env），非法历史值忽略。
	if v, ok := stored["library_dir"]; ok {
		if clean, err := sanitizeStorageDir(v); err == nil {
			libraryDir = clean
		} else {
			// 例如从 Linux/容器数据库搬到 Windows 的 "/media"：不静默套用，也不覆盖
			// 启动默认值，明确提示用户改成本机路径。
			log.Printf("忽略数据库中不适用于当前系统的媒体库目录 %q：%v", v, err)
		}
	}
	if v := stored["web_user"]; v != "" {
		if hash := stored["web_hash"]; hash != "" {
			webUser, webHash = v, hash
		}
	}
	if v, ok := stored["setup_done"]; ok {
		setupDone = v == "1"
	}
}

// setupRequired reports whether the first-run wizard should run: the service
// has no usable credentials at all and the wizard was not skipped.
func setupRequired() bool {
	cfg := runtimeConfig()
	return !cfg.setupDone && cfg.webHash == "" && (cfg.webUser == "" || cfg.webPassword == "")
}

// v1 SHA-256 is read-only legacy support; successful authentication upgrades it.

func hashWebPassword(password, salt string) string {
	sum := sha256.Sum256([]byte(salt + ":" + password))
	return hex.EncodeToString(sum[:])
}

func newPasswordHash(password string) (string, error) {
	// Prehash to preserve support for passwords longer than bcrypt's 72 bytes.
	sum := sha256.Sum256([]byte(password))
	hash, err := bcrypt.GenerateFromPassword([]byte(hex.EncodeToString(sum[:])), bcrypt.DefaultCost)
	return "v2$" + string(hash), err
}

// verifyWebCredentials checks a login against the hashed DB credentials when
// present, then falls back to the WEB_USER/WEB_PASSWORD environment pair.
func verifyWebCredentials(user, password string) bool {
	cfg := runtimeConfig()
	if cfg.webHash != "" {
		if strings.HasPrefix(cfg.webHash, "v2$") {
			sum := sha256.Sum256([]byte(password))
			return subtle.ConstantTimeCompare([]byte(user), []byte(cfg.webUser)) == 1 &&
				bcrypt.CompareHashAndPassword([]byte(strings.TrimPrefix(cfg.webHash, "v2$")), []byte(hex.EncodeToString(sum[:]))) == nil
		}
		parts := strings.SplitN(cfg.webHash, "$", 3)
		if len(parts) != 3 || parts[0] != "v1" {
			return false
		}
		okUser := subtle.ConstantTimeCompare([]byte(user), []byte(cfg.webUser))
		okHash := subtle.ConstantTimeCompare([]byte(hashWebPassword(password, parts[1])), []byte(parts[2]))
		return okUser == 1 && okHash == 1
	}
	okUser := subtle.ConstantTimeCompare([]byte(user), []byte(cfg.webUser))
	okPass := subtle.ConstantTimeCompare([]byte(password), []byte(cfg.webPassword))
	return okUser == 1 && okPass == 1
}

// Commit the entire configuration and session revocation before publishing it.
// A validation or SQL failure must leave both persisted and live settings intact.
func persistSettings(values map[string]string, revokeSessions bool) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, err := tx.Exec("INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, values[key]); err != nil {
			return err
		}
	}
	if revokeSessions {
		if _, err := tx.Exec("DELETE FROM web_sessions"); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	applyStoredSettings(values)
	return nil
}

// ---- HTTP API ----

// settingsInput uses pointers so an absent field means "leave unchanged".
// Sensitive fields (passwords, tokens) can only be set, never read back or
// cleared through this API.
type settingsInput struct {
	MaxConcurrentDownloads  *int    `json:"max_concurrent_downloads"`
	WebUser                 *string `json:"web_user"`
	WebPassword             *string `json:"web_password"`
	DownloadMode            *string `json:"download_mode"`
	Aria2URL                *string `json:"aria2_url"`
	Aria2Token              *string `json:"aria2_token"`
	Aria2Dir                *string `json:"aria2_dir"`
	Aria2LocalDir           *string `json:"aria2_local_dir"`
	QbitURL                 *string `json:"qbit_url"`
	QbitUser                *string `json:"qbit_user"`
	QbitPassword            *string `json:"qbit_password"`
	QbitSavePath            *string `json:"qbit_save_path"`
	QbitLocalDir            *string `json:"qbit_local_dir"`
	TransmissionURL         *string `json:"transmission_url"`
	TransmissionUser        *string `json:"transmission_user"`
	TransmissionPassword    *string `json:"transmission_password"`
	TransmissionDownloadDir *string `json:"transmission_download_dir"`
	TransmissionLocalDir    *string `json:"transmission_local_dir"`
	ProxyURL                *string `json:"proxy_url"`
	ProxyClearCredentials   bool    `json:"proxy_clear_credentials"`
	PollMinutes             *int    `json:"poll_minutes"`
	LibraryDir              *string `json:"library_dir"`
	AutoCleanupEmptyAnime   *bool   `json:"auto_cleanup_empty_anime"`
	KeepSourceFiles         *bool   `json:"keep_source_files"`
}

func decodeSettings(w http.ResponseWriter, r *http.Request) (*settingsInput, bool) {
	var in settingsInput
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in) != nil {
		http.Error(w, "请求参数无效", 400)
		return nil, false
	}
	return &in, true
}

// validateSettings checks mode, poll interval and proxy before anything is saved.
func validateSettings(in *settingsInput) error {
	cfg := runtimeConfig()
	if n := in.MaxConcurrentDownloads; n != nil && (*n < 1 || *n > maximumConcurrentDownloads) {
		return fmt.Errorf("同时下载数量必须是 1-32 的整数")
	}
	user := cfg.webUser
	if in.WebUser != nil && strings.TrimSpace(*in.WebUser) != "" {
		user = strings.TrimSpace(*in.WebUser)
	}
	if user != cfg.webUser && (in.WebPassword == nil || *in.WebPassword == "") {
		return fmt.Errorf("修改用户名时必须同时填写新密码")
	}
	if in.WebPassword != nil && *in.WebPassword != "" {
		if user == "" {
			return fmt.Errorf("设置密码前请先填写用户名")
		}
		if len([]rune(*in.WebPassword)) < 6 {
			return fmt.Errorf("密码至少 6 位")
		}
	}
	if in.DownloadMode != nil {
		switch *in.DownloadMode {
		case "native", "direct", "aria2", "qbittorrent", "transmission":
		default:
			return fmt.Errorf("下载方式无效")
		}
	}
	if in.PollMinutes != nil && (*in.PollMinutes < 5 || *in.PollMinutes > 1440) {
		return fmt.Errorf("同步周期必须是 5-1440 分钟")
	}
	if in.ProxyURL != nil {
		if _, err := parseProxyURL(*in.ProxyURL); err != nil {
			return err
		}
	}
	if in.LibraryDir != nil {
		if _, err := validateLibraryDir(*in.LibraryDir); err != nil {
			return err
		}
	}
	return nil
}

// validateLibraryDir checks the media library root with the same rules as a
// per-series storage dir; empty means "fall back to DOWNLOAD_DIR".
func validateLibraryDir(raw string) (string, error) {
	clean, err := sanitizeStorageDir(raw)
	if err != nil {
		return "", fmt.Errorf("媒体库目录%s", strings.TrimPrefix(err.Error(), "存储位置"))
	}
	return clean, nil
}

// applySettings persists the provided fields and updates the live globals.
func applySettings(in *settingsInput) error {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	return saveSettings(in, false)
}

// settingsMu must be held, including when called by the setup handler.
func saveSettings(in *settingsInput, finishSetup bool) error {
	cfg := runtimeConfig()
	proxyURL := currentProxy()
	if err := validateSettings(in); err != nil {
		return err
	}
	values := map[string]string{}
	if in.MaxConcurrentDownloads != nil {
		values["max_concurrent_downloads"] = strconv.Itoa(*in.MaxConcurrentDownloads)
	}
	apply := func(key, value string) error {
		values[key] = value
		return nil
	}
	setTrimmed := func(field *string, key string) error {
		if field == nil {
			return nil
		}
		return apply(key, strings.TrimSpace(*field))
	}
	setSecret := func(field *string, key string) error {
		if field == nil || strings.TrimSpace(*field) == "" {
			return nil // sensitive values are write-only: absent/empty keeps the current one
		}
		return apply(key, strings.TrimSpace(*field))
	}
	if err := setTrimmed(in.DownloadMode, "download_mode"); err != nil {
		return err
	}
	if err := setTrimmed(in.Aria2URL, "aria2_url"); err != nil {
		return err
	}
	if err := setSecret(in.Aria2Token, "aria2_token"); err != nil {
		return err
	}
	if err := setTrimmed(in.Aria2Dir, "aria2_dir"); err != nil {
		return err
	}
	if err := setTrimmed(in.Aria2LocalDir, "aria2_local_dir"); err != nil {
		return err
	}
	if err := setTrimmed(in.QbitURL, "qbit_url"); err != nil {
		return err
	}
	if err := setTrimmed(in.QbitUser, "qbit_user"); err != nil {
		return err
	}
	if err := setSecret(in.QbitPassword, "qbit_password"); err != nil {
		return err
	}
	if err := setTrimmed(in.QbitSavePath, "qbit_save_path"); err != nil {
		return err
	}
	if err := setTrimmed(in.QbitLocalDir, "qbit_local_dir"); err != nil {
		return err
	}
	if err := setTrimmed(in.TransmissionURL, "transmission_url"); err != nil {
		return err
	}
	if err := setTrimmed(in.TransmissionUser, "transmission_user"); err != nil {
		return err
	}
	if err := setSecret(in.TransmissionPassword, "transmission_password"); err != nil {
		return err
	}
	if err := setTrimmed(in.TransmissionDownloadDir, "transmission_download_dir"); err != nil {
		return err
	}
	if err := setTrimmed(in.TransmissionLocalDir, "transmission_local_dir"); err != nil {
		return err
	}
	if in.ProxyURL != nil || in.ProxyClearCredentials {
		raw := redactProxy(proxyURL)
		if in.ProxyURL != nil {
			raw = strings.TrimSpace(*in.ProxyURL)
		}
		u, err := parseProxyURL(raw)
		if err != nil {
			return err
		}
		if u != nil {
			if in.ProxyClearCredentials {
				u.User = nil
			} else if u.User == nil && proxyURL != nil && redactProxy(u) == redactProxy(proxyURL) {
				// An unchanged, redacted form value is not a credential reset.
				u.User = proxyURL.User
			}
			values["proxy_url"] = u.String()
		} else {
			values["proxy_url"] = ""
		}
	}
	if in.PollMinutes != nil {
		if err := apply("poll_minutes", strconv.Itoa(*in.PollMinutes)); err != nil {
			return err
		}
	}
	if err := setTrimmed(in.LibraryDir, "library_dir"); err != nil {
		return err
	}
	if in.AutoCleanupEmptyAnime != nil {
		values["auto_cleanup_empty_anime"] = strconv.Itoa(boolInt(*in.AutoCleanupEmptyAnime))
	}
	if in.KeepSourceFiles != nil {
		values["keep_source_files"] = strconv.Itoa(boolInt(*in.KeepSourceFiles))
	}
	revokeSessions := false
	if in.WebPassword != nil && *in.WebPassword != "" {
		user := cfg.webUser
		if in.WebUser != nil && strings.TrimSpace(*in.WebUser) != "" {
			user = strings.TrimSpace(*in.WebUser)
		}
		hash, err := newPasswordHash(*in.WebPassword)
		if err != nil {
			return err
		}
		values["web_user"], values["web_hash"] = user, hash
		revokeSessions = true
	}
	if finishSetup {
		values["setup_done"] = "1"
	}
	return persistSettings(values, revokeSessions)
}

func settingsHandler(w http.ResponseWriter, r *http.Request) {
	cfg := runtimeConfig()
	proxyURL := currentProxy()
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Cache-Control", "no-store")
		jsonOut(w, map[string]any{
			// 运行平台供前端切换 Windows/Docker 文案（不是敏感信息）。
			"os":       runtime.GOOS,
			"web_user": cfg.webUser, "web_password_set": cfg.webHash != "" || cfg.webPassword != "",
			"download_mode":            cfg.downloadMode,
			"max_concurrent_downloads": downloadLimit(),
			"aria2_url":                cfg.aria2URL, "aria2_token_set": cfg.aria2Token != "",
			"aria2_dir": cfg.aria2Dir, "aria2_local_dir": cfg.aria2LocalDir,
			"qbit_url": cfg.qbitURL, "qbit_user": cfg.qbitUser, "qbit_password_set": cfg.qbitPassword != "",
			"qbit_save_path": cfg.qbitSavePath, "qbit_local_dir": cfg.qbitLocalDir,
			"transmission_url": cfg.transmissionURL, "transmission_user": cfg.transmissionUser, "transmission_password_set": cfg.transmissionPassword != "",
			"transmission_download_dir": cfg.transmissionDownloadDir, "transmission_local_dir": cfg.transmissionLocalDir,
			"proxy_url":                redactProxy(proxyURL),
			"proxy_credentials_set":    proxyURL != nil && proxyURL.User != nil,
			"poll_minutes":             cfg.pollMinutes,
			"library_dir":              cfg.libraryDir,
			"auto_cleanup_empty_anime": getSetting("auto_cleanup_empty_anime") == "1",
			"keep_source_files":        cfg.keepSourceFiles,
			"download_dir":             downloadDir,
			"setup_done":               cfg.setupDone,
		})
	case http.MethodPut:
		in, ok := decodeSettings(w, r)
		if !ok {
			return
		}
		// 目录变更在写数据库前检测，失败时整份设置不落库。
		if in.LibraryDir != nil && strings.TrimSpace(*in.LibraryDir) != cfg.libraryDir {
			dir, err := validateLibraryDir(*in.LibraryDir)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if dir == "" {
				dir = downloadDir
			}
			if result := probeSetupStorage(dir); !result.Writable {
				http.Error(w, result.Message, http.StatusBadRequest)
				return
			}
		}
		if err := applySettings(in); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		cleaned := 0
		warning := ""
		if in.AutoCleanupEmptyAnime != nil && *in.AutoCleanupEmptyAnime {
			var err error
			cleaned, err = cleanupEmptyAnime()
			if err != nil {
				log.Printf("设置已保存，清理空番剧失败: %v", err)
				warning = "设置已保存，但本次清理失败，将在后台重试"
			}
		}
		// 媒体库目录同样可能落在容器未映射的路径上，保存时一并提示。
		libraryWarning := ""
		if in.LibraryDir != nil {
			libraryWarning = unmappedPathWarning(runtimeConfig().libraryDir)
		}
		jsonOut(w, map[string]any{"ok": true, "reauth_required": in.WebPassword != nil && *in.WebPassword != "", "cleaned_anime": cleaned, "cleanup_warning": warning, "library_warning": libraryWarning})
	default:
		http.Error(w, "方法不支持", 405)
	}
}

func setupStatusHandler(w http.ResponseWriter, r *http.Request) {
	cfg := runtimeConfig()
	if r.Method != http.MethodGet {
		http.Error(w, "方法不支持", 405)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response := map[string]any{"required": setupRequired(), "download_mode": cfg.downloadMode, "os": runtime.GOOS}
	// Only the uninitialized wizard needs paths. This endpoint is public.
	if response["required"] == true {
		response["library_dir"] = cfg.libraryDir
		response["download_dir"] = downloadDir
		response["poll_minutes"] = cfg.pollMinutes
		response["proxy_enabled"] = currentProxy() != nil
		response["keep_source_files"] = cfg.keepSourceFiles
	}
	jsonOut(w, response)
}

// setupHandler runs the first-run wizard. It stays reachable without
// credentials only while the instance is uninitialized; afterwards the normal
// login is required and this endpoint refuses to run again.
func setupHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "方法不支持", 405)
		return
	}
	if !allowSetupWrite(w, r) {
		return
	}
	in, ok := decodeSettings(w, r)
	if !ok {
		return
	}
	settingsMu.Lock()
	defer settingsMu.Unlock()
	cfg := runtimeConfig()
	if cfg.setupDone || authEnabled() {
		http.Error(w, "已完成初始化，请登录后在设置页修改", http.StatusForbidden)
		return
	}
	if err := saveSettings(in, true); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	log.Println("首次安装引导已完成")
	root := strings.TrimSpace(runtimeConfig().libraryDir)
	if root == "" {
		root = downloadDir
	}
	jsonOut(w, map[string]any{"ok": true, "library_warning": unmappedPathWarning(root)})
}

func setupPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "方法不支持", http.StatusMethodNotAllowed)
		return
	}
	if !setupRequired() {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	serveWebPage(w, r, "setup.html")
}

// installPage 提供与离线安装配置页相同的 Compose 生成器。容器启动后可在局域网任意设备
// 访问 /install，用于更换 NAS 媒体目录或端口时重新生成配置，无需联网下载文件；
// Windows 本机没有这套部署结构，访问时返回 404。
func installPage(w http.ResponseWriter, r *http.Request) {
	// Windows 本机没有容器和卷映射，Compose 生成器不适用，直接 404。
	if runtime.GOOS == "windows" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "方法不支持", http.StatusMethodNotAllowed)
		return
	}
	serveWebPage(w, r, "install.html")
}

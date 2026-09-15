package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

// 版本检查与一键更新。
//
// 发布方式：GitHub Releases 上每个 tag 附带 linux/amd64、linux/arm64 与
// windows/amd64 三个可执行文件，以及一份 sha256 校验文件。客户端读取
// Release API 拿到最新 tag、更新说明与附件地址，下载与当前平台匹配的文件，
// 校验 SHA-256 后替换自身可执行文件并重启。
//
// Docker 场景：容器内进程可能无权改写镜像里的 /app/auto-anime，因此更新包
// 还会写入数据目录（/data）的暂存位置；下次启动时 bootstrapStagedUpdate 在
// 最早的时机完成替换。这样即使容器被重建（回到旧镜像里的可执行文件），
// 只要 /data 卷还在，重启一次就会回到新版本。

const (
	// updateDefaultSource 是内置默认更新源；发布工作流会用仓库名覆盖它
	// （-ldflags "-X main.defaultUpdateSource=用户名/仓库"），便于仓库改名或 fork。
	updateDefaultSource = "Shizuku11451/auto-anime"
	updateStageFileName = "auto-anime.update"
	updateBackupSuffix  = ".old"
	// 单次下载上限，避免异常响应占满磁盘（正常可执行文件约 20-40 MB）。
	updateMaxBinaryBytes = int64(160 << 20)
	updateCheckTimeout   = 30 * time.Second
	updateDownloadTime   = 10 * time.Minute
	updateCheckInterval  = 6 * time.Hour
	updateNotesLimit     = 48 << 10
)

// defaultUpdateSource 由构建注入，留空时使用上面的常量。
var defaultUpdateSource = ""

func builtinUpdateSource() string {
	if value := strings.TrimSpace(defaultUpdateSource); value != "" {
		return value
	}
	return updateDefaultSource
}

var (
	// UPDATE_CHECK=0/false/off 关闭联网检查新版本。
	updateEnabled = envBool("UPDATE_CHECK", true)
	// updateSource 是版本检查与更新包的来源：留空使用默认仓库；也支持
	// 用户名/仓库、GitHub 仓库地址，或自定义更新源前缀（读取 <前缀>/latest.json）。
	updateSource = strings.TrimSpace(env("UPDATE_SOURCE", ""))
	// GITHUB_TOKEN：私有仓库或需要提高 API 限额时使用，不写入日志与接口。
	githubToken = strings.TrimSpace(env("GITHUB_TOKEN", ""))
	// UPDATE_PROXY：仅更新流量使用的代理；留空沿用 PROXY_URL。
	updateProxyRaw = strings.TrimSpace(env("UPDATE_PROXY", ""))

	updateStateMu  sync.Mutex
	updateStarting bool
	// updateTransportOverride 仅用于测试中把 GitHub 请求指向本地服务。
	updateTransportOverride http.RoundTripper

	versionStateMu sync.RWMutex
	versionState   versionCheck
	latestRelease  *releaseInfo
)

type releaseInfo struct {
	Tag       string `json:"tag"`
	Notes     string `json:"notes"`
	HTMLURL   string `json:"html_url"`
	Published string `json:"published_at"`
	AssetName string `json:"asset_name"`
	// AssetURL 是实际下载地址：GitHub 源优先用 Release Assets API
	// （/releases/assets/<id>），私有仓库的 browser_download_url 会返回 404。
	AssetURL string `json:"asset_url"`
	// PublicURL 是 browser_download_url，自定义更新源与公开仓库的备用地址。
	PublicURL string `json:"public_url"`
	// AssetUseAPI 表示下载地址是 GitHub Release Assets API，需要
	// application/octet-stream 请求头才会返回文件内容。
	AssetUseAPI bool   `json:"asset_use_api"`
	SumName     string `json:"sum_name"`
	SumURL      string `json:"sum_url"`
	// SumUseAPI 表示校验文件也要走 Assets API 下载。
	SumUseAPI bool   `json:"sum_use_api"`
	SHA256    string `json:"sha256"`
	// InstallOK 表示该版本在本平台可自动安装（与“是否有新版本”无关）。
	InstallOK  bool   `json:"install_ok"`
	UpdateHint string `json:"update_hint"`
}

type versionCheck struct {
	CheckedAt       string `json:"checked_at"`
	Source          string `json:"source"`
	State           string `json:"state"`
	Error           string `json:"error"`
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	ReleaseURL      string `json:"release_url"`
	Notes           string `json:"notes"`
	PublishedAt     string `json:"published_at"`
	UpdateAvailable bool   `json:"update_available"`
	CanInstall      bool   `json:"can_install"`
	InstallHint     string `json:"install_hint"`
	// Pending 表示已有更新包写好、等待下次启动生效（容器内无法替换运行中的文件时）。
	Pending bool `json:"pending"`
	// Status/Message 反映一键更新的实时进度（idle/downloading/installing/restarting/failed）。
	Status  string `json:"status"`
	Message string `json:"message"`
}

// ---- 版本号比较 ----

// versionParts parses "v1.2.3-rc1+build" into its numeric core.
func versionParts(v string) ([]int, error) {
	core := strings.TrimSpace(v)
	core = strings.TrimPrefix(core, "v")
	core = strings.TrimPrefix(core, "V")
	if i := strings.IndexAny(core, "-+"); i >= 0 {
		core = core[:i]
	}
	if core == "" {
		return nil, errors.New("版本号为空")
	}
	fields := strings.Split(core, ".")
	parts := make([]int, 0, len(fields))
	for _, field := range fields {
		n, err := strconv.Atoi(field)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("版本号 %q 无法解析", v)
		}
		parts = append(parts, n)
	}
	return parts, nil
}

// compareVersions returns -1, 0 or 1; a parse failure yields 0 so an
// unparsable tag never triggers an update.
func compareVersions(a, b string) int {
	pa, errA := versionParts(a)
	pb, errB := versionParts(b)
	if errA != nil || errB != nil {
		return 0
	}
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// ---- 更新源解析 ----

var repoSourcePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// resolveUpdateSource normalizes the configured update source into an HTTP
// base URL plus a human-readable label.
func resolveUpdateSource(raw string) (base, label string, err error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		value = builtinUpdateSource()
	}
	value = strings.TrimSuffix(strings.TrimSuffix(value, "/"), "/")
	if strings.HasPrefix(value, "github:") {
		value = strings.TrimPrefix(value, "github:")
	} else if rest, ok := strings.CutPrefix(value, "https://github.com/"); ok {
		value = rest
	} else if rest, ok := strings.CutPrefix(value, "http://github.com/"); ok {
		value = rest
	}
	if !strings.Contains(value, "://") {
		value = strings.TrimSuffix(value, ".git")
		for _, suffix := range []string{"/releases/latest", "/releases", "/latest.json", "/latest"} {
			value = strings.TrimSuffix(value, suffix)
		}
		if !repoSourcePattern.MatchString(value) {
			return "", "", fmt.Errorf("更新源 %q 无效：可填写 用户名/仓库、GitHub 仓库地址或更新源前缀", raw)
		}
		return "https://api.github.com/repos/" + value + "/releases", "github.com/" + value, nil
	}
	return strings.TrimSuffix(value, "/"), redactURL(value), nil
}

func isGitHubAPI(base string) bool {
	return strings.Contains(base, "api.github.com")
}

// githubDownloadPrefix is the only place a checksum-less download may come from.
func githubDownloadPrefix() string {
	_, label, err := resolveUpdateSource(updateSource)
	if err != nil {
		return ""
	}
	return "https://github.com/" + strings.TrimPrefix(label, "github.com/")
}

func trimmedString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// ---- 检查更新 ----

// runUpdateCheck queries the release source and publishes the result. When the
// version is unchanged the previous notes are kept so the UI does not blank.
func runUpdateCheck() {
	check := versionCheck{CheckedAt: time.Now().UTC().Format(time.RFC3339), Current: strings.TrimSpace(version)}
	if !updateEnabled {
		check.State = "disabled"
		setVersionState(check)
		return
	}
	// 已有的更新包与版本检查无关：即使更新源不可达也要提示重启生效。
	pendingHint := applyPendingState(&check, !inPlaceExec())
	base, label, err := resolveUpdateSource(updateSource)
	if err != nil {
		check.State = "error"
		check.Error = err.Error()
		check.InstallHint = pendingHint
		setVersionState(check)
		return
	}
	check.Source = label
	rel, err := fetchLatestRelease(base)
	if err != nil {
		check.State = "failed"
		check.Error = err.Error()
		check.InstallHint = pendingHint
		setVersionState(check)
		return
	}
	check.State = "ok"
	check.Latest = rel.Tag
	check.ReleaseURL = rel.HTMLURL
	check.PublishedAt = rel.Published
	if strings.TrimSpace(rel.Notes) == "" {
		rel.Notes = fetchReleaseNotes(rel.HTMLURL)
	}
	check.Notes = rel.Notes
	if prevLatest, prevNotes := previousVersionInfo(); rel.Tag == prevLatest && check.Notes == "" {
		check.Notes = prevNotes
	}
	if devVersion() {
		// 源码运行/本地构建：只提示有更新，不自动替换。
		rel.InstallOK = false
		rel.UpdateHint = "当前是源码或本地构建（" + strings.TrimSpace(version) + "），不支持自动更新；请 git pull 后重新构建。"
	} else {
		check.UpdateAvailable = compareVersions(version, rel.Tag) < 0
		rel.InstallOK = rel.AssetURL != ""
		if !rel.InstallOK {
			rel.UpdateHint = fmt.Sprintf("该版本没有提供 %s/%s 的更新包，请在 Release 页面手动更新。", runtime.GOOS, runtime.GOARCH)
		}
	}
	if rel.InstallOK {
		if dir := updateStageDir(); dir == "" {
			rel.InstallOK = false
			rel.UpdateHint = "当前环境不允许自动替换程序文件，请手动更新镜像或可执行文件。"
		}
	}
	check.Pending = imageVersionDrift() || (!inPlaceExec() && installReady())
	check.CanInstall = rel.InstallOK && check.UpdateAvailable
	if check.InstallHint == "" {
		check.InstallHint = rel.UpdateHint
	}
	if check.Pending && !check.CanInstall {
		check.InstallHint = pendingUpdateHint()
	}
	versionStateMu.Lock()
	latestRelease = &rel
	versionStateMu.Unlock()
	setVersionState(check)
}

// applyPendingState marks an already staged update and returns the hint that
// belongs with it. A package counts as pending when the running binary is not
// the one the image shipped (a container rebuild brought the old layer back),
// or when this platform needs a start to apply it (Windows). After a
// successful in-place update the staged file is just a rebuild-proof cache.
func applyPendingState(check *versionCheck, stagedOnStart bool) string {
	if !installReady() {
		return ""
	}
	check.Pending = imageVersionDrift() || stagedOnStart
	if !check.Pending {
		return ""
	}
	return pendingUpdateHint()
}

func pendingUpdateHint() string {
	return "已下载的新版本需要重启后生效：重启服务（容器）即可运行新版本。"
}

// imageVersionDrift reports that the running binary is not the one the image
// shipped, i.e. an earlier one-click update replaced the file but a container
// rebuild brought the old image layer back.
func imageVersionDrift() bool {
	dir := dataDir()
	if dir == "" {
		return false
	}
	data, err := os.ReadFile(filepath.Join(dir, "image-version"))
	if err != nil {
		return false
	}
	recorded := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(data)), "version="))
	running := strings.TrimSpace(version)
	return recorded != "" && running != "" && recorded != running
}

// reportPendingUpdate logs an update package that could not be applied on this
// start (for example the executable lives in a read-only image layer).
func reportPendingUpdate() {
	if !installReady() {
		return
	}
	logUpdate("检测到待生效的更新包；若本次启动未能替换程序文件，请检查 %s 是否可写或重建容器", executableDisplay())
}

func executableDisplay() string {
	exe, err := executableFile()
	if err != nil {
		return "程序目录"
	}
	return filepath.Dir(exe)
}

func previousVersionInfo() (string, string) {
	versionStateMu.RLock()
	defer versionStateMu.RUnlock()
	return versionState.Latest, versionState.Notes
}

func setVersionState(next versionCheck) {
	versionStateMu.Lock()
	versionState = next
	versionStateMu.Unlock()
}

func currentVersionCheck() versionCheck {
	versionStateMu.RLock()
	defer versionStateMu.RUnlock()
	return versionState
}

func currentRelease() *releaseInfo {
	versionStateMu.RLock()
	defer versionStateMu.RUnlock()
	return latestRelease
}

// startUpdateChecker runs one check at startup and then on an interval.
func startUpdateChecker() {
	if !updateEnabled {
		setVersionState(versionCheck{Current: strings.TrimSpace(version), State: "disabled"})
		logUpdate("已关闭版本检查：需要时把 UPDATE_CHECK 设为 1")
		return
	}
	logUpdate("当前版本 %s（%s/%s），更新源 %s", versionLabel(), runtime.GOOS, runtime.GOARCH, sourceHost())
	reportPendingUpdate()
	go func() {
		runUpdateCheck()
		if check := currentVersionCheck(); check.UpdateAvailable {
			logUpdate("发现新版本 %s，可在设置页一键更新", check.Latest)
		} else if check.Error != "" {
			logUpdate("版本检查失败：%s", check.Error)
		}
		ticker := time.NewTicker(updateCheckInterval)
		defer ticker.Stop()
		for range ticker.C {
			runUpdateCheck()
		}
	}()
}

func logUpdate(format string, args ...any) {
	log.Printf("[更新] "+format, args...)
}

// envBool treats 0/false/off/no as false and everything else as true.
func envBool(key string, fallback bool) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch raw {
	case "":
		return fallback
	case "0", "false", "off", "no":
		return false
	default:
		return true
	}
}

// ---- Release 读取 ----

func fetchLatestRelease(base string) (releaseInfo, error) {
	if isGitHubAPI(base) {
		return fetchGitHubRelease(base)
	}
	return fetchGenericRelease(base)
}

func updateHTTPClient() *http.Client {
	transport := newTransport()
	transport.DisableCompression = false
	// UPDATE_PROXY=direct/off 时更新流量强制直连（不跟随 PROXY_URL）。
	if updateProxyRaw != "" {
		if directProxyValue(updateProxyRaw) {
			transport.Proxy = nil
		} else if u, err := parseProxyURL(updateProxyRaw); err == nil && u != nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}
	if updateTransportOverride != nil {
		return &http.Client{Timeout: updateCheckTimeout, Transport: updateTransportOverride}
	}
	return &http.Client{Timeout: updateCheckTimeout, Transport: transport}
}

// directProxyValue recognizes the explicit "no proxy" spellings for UPDATE_PROXY.
func directProxyValue(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "direct", "none", "off", "0", "false":
		return true
	default:
		return false
	}
}

// currentPlatform is the platform key used in asset names and manifest keys.
func currentPlatform() string {
	return runtime.GOOS + "_" + runtime.GOARCH
}

func updateBodyRequest(ctx context.Context, method, target string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "auto-anime-updater")
	if githubToken != "" {
		req.Header.Set("Authorization", "Bearer "+githubToken)
	}
	return req, nil
}

// updateDownloadClient follows the release-asset redirect and keeps the
// Authorization header only for GitHub hosts (S3 rejects foreign headers).
func updateDownloadClient() *http.Client {
	client := *updateHTTPClient()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("重定向次数过多")
		}
		if len(via) > 0 && isGitHubHost(req.URL.Hostname()) {
			req.Header.Set("Authorization", via[0].Header.Get("Authorization"))
		}
		return nil
	}
	return &client
}

func isGitHubHost(host string) bool {
	host = strings.ToLower(host)
	return host == "github.com" || host == "api.github.com" || host == "codeload.github.com" ||
		strings.HasSuffix(host, ".github.com") || strings.HasSuffix(host, ".githubusercontent.com")
}

// downloadURL is the address a one-click update actually fetches.
func downloadURL(rel *releaseInfo) string {
	if rel.AssetURL != "" {
		return rel.AssetURL
	}
	return rel.PublicURL
}

func updateAPIRequest(ctx context.Context, method, target string) (*http.Response, error) {
	req, err := updateBodyRequest(ctx, method, target)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json, application/json")
	return updateHTTPClient().Do(req)
}

func fetchGitHubRelease(base string) (releaseInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), updateCheckTimeout)
	defer cancel()
	res, err := updateAPIRequest(ctx, http.MethodGet, strings.TrimSuffix(base, "/")+"/latest")
	if err != nil {
		return releaseInfo{}, friendlyUpdateError(err)
	}
	defer res.Body.Close()
	if err := githubStatusError(res); err != nil {
		return releaseInfo{}, err
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return releaseInfo{}, fmt.Errorf("读取更新信息失败：%w", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return releaseInfo{}, fmt.Errorf("更新信息格式无法解析：%w", err)
	}
	if draft, ok := payload["draft"].(bool); ok && draft {
		return releaseInfo{}, errors.New("最新 Release 仍是草稿")
	}
	tag := trimmedString(payload, "tag_name")
	if tag == "" {
		tag = trimmedString(payload, "name")
	}
	if tag == "" {
		return releaseInfo{}, errors.New("更新信息缺少版本号")
	}
	rel := releaseInfo{
		Tag:       tag,
		Notes:     strings.TrimSpace(trimmedString(payload, "body")),
		HTMLURL:   trimmedString(payload, "html_url"),
		Published: trimmedString(payload, "published_at"),
	}
	assets, _ := payload["assets"].([]any)
	names := make([]string, 0, len(assets))
	for _, raw := range assets {
		asset, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name := trimmedString(asset, "name")
		names = append(names, name)
		if isChecksumAsset(name) && rel.SumName == "" {
			rel.SumName = name
		}
	}
	rel.AssetName, rel.PublicURL = pickAssetForPlatform(tag, names, func(name string) string {
		return assetFieldByName(assets, name, "browser_download_url")
	})
	// 私有仓库的 browser_download_url 会返回 404，改用 Release Assets API 地址；
	// 公开仓库两者都可用，API 地址同样有效，因此统一优先 API。
	if apiURL := assetFieldByName(assets, rel.AssetName, "url"); apiURL != "" {
		rel.AssetURL, rel.AssetUseAPI = apiURL, true
	} else {
		rel.AssetURL = rel.PublicURL
	}
	if apiURL := assetFieldByName(assets, rel.SumName, "url"); apiURL != "" {
		rel.SumURL, rel.SumUseAPI = apiURL, true
	} else {
		rel.SumURL = assetFieldByName(assets, rel.SumName, "browser_download_url")
	}
	return rel, nil
}

func githubStatusError(res *http.Response) error {
	switch res.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusNotFound:
		return errors.New("更新源还没有发布任何 Release")
	case http.StatusForbidden, http.StatusTooManyRequests:
		return errors.New("GitHub 接口访问受限（可能触发限额）；可稍后重试或配置 GITHUB_TOKEN")
	case http.StatusUnauthorized:
		return errors.New("GitHub 接口鉴权失败，请检查 GITHUB_TOKEN")
	default:
		return fmt.Errorf("更新源返回 HTTP %d", res.StatusCode)
	}
}

func isChecksumAsset(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	return strings.HasSuffix(lower, ".sha256sum") || strings.HasSuffix(lower, ".sha256") ||
		lower == "sha256sums.txt" || strings.HasSuffix(lower, "checksums.txt")
}

func assetFieldByName(assets []any, name, key string) string {
	for _, raw := range assets {
		asset, ok := raw.(map[string]any)
		if !ok || trimmedString(asset, "name") != name {
			continue
		}
		return trimmedString(asset, key)
	}
	return ""
}

// pickAssetForPlatform chooses this platform's binary from the release asset
// names. `lookup` resolves the download URL for a chosen name.
func pickAssetForPlatform(tag string, names []string, lookup func(string) string) (string, string) {
	candidates := map[string]bool{}
	for _, name := range []string{
		fmt.Sprintf("auto-anime_%s_%s_%s", tag, runtime.GOOS, runtime.GOARCH),
		fmt.Sprintf("auto-anime_%s_%s_%s", strings.TrimPrefix(tag, "v"), runtime.GOOS, runtime.GOARCH),
		fmt.Sprintf("auto-anime-%s-%s-%s", tag, runtime.GOOS, runtime.GOARCH),
		fmt.Sprintf("auto-anime-%s-%s-%s", strings.TrimPrefix(tag, "v"), runtime.GOOS, runtime.GOARCH),
	} {
		candidates[name] = true
		candidates[name+".exe"] = true
	}
	for _, name := range names {
		if !candidates[name] {
			continue
		}
		if url := lookup(name); url != "" {
			return name, url
		}
	}
	// 旧版本发布可能没有带版本号的文件名。
	fallback := fmt.Sprintf("auto-anime-%s-%s", runtime.GOOS, runtime.GOARCH)
	for _, name := range names {
		if name == fallback || name == fallback+".exe" {
			if url := lookup(name); url != "" {
				return name, url
			}
		}
	}
	return "", ""
}

// fetchGenericRelease reads a JSON manifest from a self-hosted update source:
//
//	{"version":"1.2.0","notes":"...","published_at":"...","url":"...",
//	 "assets":{"linux-amd64":"...","linux-amd64.sha256":"...","sha256":"..."}}
func fetchGenericRelease(base string) (releaseInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), updateCheckTimeout)
	defer cancel()
	res, err := updateAPIRequest(ctx, http.MethodGet, strings.TrimSuffix(base, "/")+"/latest.json")
	if err != nil {
		return releaseInfo{}, friendlyUpdateError(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return releaseInfo{}, fmt.Errorf("更新源返回 HTTP %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return releaseInfo{}, fmt.Errorf("读取更新信息失败：%w", err)
	}
	var payload struct {
		Version     string            `json:"version"`
		Notes       string            `json:"notes"`
		PublishedAt string            `json:"published_at"`
		HTMLURL     string            `json:"html_url"`
		URL         string            `json:"url"`
		Assets      map[string]string `json:"assets"`
		SHA256      string            `json:"sha256"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return releaseInfo{}, fmt.Errorf("更新源格式无法解析：%w", err)
	}
	rel := releaseInfo{
		Tag:       strings.TrimSpace(payload.Version),
		Notes:     strings.TrimSpace(payload.Notes),
		HTMLURL:   strings.TrimSpace(payload.HTMLURL),
		Published: strings.TrimSpace(payload.PublishedAt),
	}
	if rel.Tag == "" {
		return releaseInfo{}, errors.New("更新信息缺少版本号")
	}
	platform := runtime.GOOS + "-" + runtime.GOARCH
	rel.AssetURL = strings.TrimSpace(payload.Assets[platform])
	if rel.AssetURL == "" {
		rel.AssetURL = strings.TrimSpace(payload.URL)
	}
	if rel.AssetURL != "" {
		rel.AssetName = filepath.Base(strings.TrimSuffix(rel.AssetURL, "/"))
	}
	for _, key := range []string{platform + ".sha256", platform + "-sha256", "sha256"} {
		if value := strings.TrimSpace(payload.Assets[key]); value != "" {
			rel.SHA256 = strings.ToLower(value)
			break
		}
	}
	if rel.SHA256 == "" {
		rel.SHA256 = strings.ToLower(strings.TrimSpace(payload.SHA256))
	}
	if rel.SHA256 == "" && rel.AssetURL != "" {
		if sum := strings.TrimSpace(payload.Assets["sha256:"+platform]); sum != "" {
			rel.SHA256 = strings.ToLower(sum)
		}
	}
	if rel.SHA256 == "" && rel.AssetURL != "" {
		rel.SumURL = strings.TrimSuffix(rel.AssetURL, "/") + ".sha256"
	}
	return rel, nil
}

// friendlyUpdateError keeps credentials out of the surfaced message while
// still naming the host that failed.
func friendlyUpdateError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return fmt.Errorf("连接更新源失败（%s）：%v", redactURL(urlErr.URL), urlErr.Err)
	}
	return fmt.Errorf("连接更新源失败：%v", err)
}

// redactURL returns just the host, so update URLs never reach logs or the UI.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "更新源"
	}
	return u.Host
}

// ---- 更新说明 ----

// fetchReleaseNotes reads the rendered release page; used when the release
// body is empty (some projects only fill the page description).
func fetchReleaseNotes(releaseURL string) string {
	if strings.TrimSpace(releaseURL) == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), updateCheckTimeout)
	defer cancel()
	res, err := updateAPIRequest(ctx, http.MethodGet, releaseURL)
	if err != nil {
		return ""
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return ""
	}
	return notesToText(extractReleaseNotes(body))
}

const notesPayloadTag = `<script type="application/json" data-target="react-app.embeddedData">`

func extractReleaseNotes(body []byte) string {
	raw := string(body)
	start := strings.Index(raw, notesPayloadTag)
	if start < 0 {
		return ""
	}
	start += len(notesPayloadTag)
	end := strings.Index(raw[start:], "</script>")
	if end < 0 {
		return ""
	}
	var payload map[string]any
	if json.Unmarshal([]byte(raw[start:start+end]), &payload) != nil {
		return ""
	}
	release, _ := payload["payload"].(map[string]any)
	if release == nil {
		return ""
	}
	if nested, ok := release["release"].(map[string]any); ok {
		release = nested
	}
	return strings.TrimSpace(trimmedString(release, "bodyHTML"))
}

// notesToText converts GitHub's rendered release HTML into readable text.
func notesToText(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "<") {
		return truncateNotes(value)
	}
	doc, err := html.Parse(strings.NewReader(value))
	if err != nil {
		return truncateNotes(value)
	}
	var out strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "br":
				out.WriteString("\n")
			case "li":
				out.WriteString("\n• ")
			case "p", "div", "section", "article", "ul", "ol", "pre", "blockquote", "table", "h1", "h2", "h3", "h4", "h5", "h6":
				out.WriteString("\n")
			case "td", "th":
				out.WriteString("  ")
			}
		}
		if n.Type == html.TextNode {
			if text := strings.TrimSpace(n.Data); text != "" {
				out.WriteString(text)
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "p", "div", "section", "article", "ul", "ol", "pre", "blockquote", "table", "h1", "h2", "h3", "h4", "h5", "h6":
				out.WriteString("\n")
			}
		}
	}
	walk(doc)
	return truncateNotes(out.String())
}

func truncateNotes(text string) string {
	cleaned := strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	lines := strings.Split(cleaned, "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if line == "" {
			blank++
			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, line)
	}
	cleaned = strings.TrimSpace(strings.Join(out, "\n"))
	if len(cleaned) > updateNotesLimit {
		cleaned = strings.ToValidUTF8(cleaned[:updateNotesLimit], "") + "\n…（更新说明过长，已截断）"
	}
	return cleaned
}

// ---- 校验文件 ----

// parseChecksumList reads "sha256  filename" lines into a name → hash map.
func parseChecksumList(text string) map[string]string {
	sums := map[string]string{}
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		hash := strings.ToLower(fields[0])
		if len(hash) != 64 {
			continue
		}
		if _, err := hex.DecodeString(hash); err != nil {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		name = strings.TrimPrefix(name, "./")
		sums[name] = hash
	}
	return sums
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// ---- 一键更新 ----

// applyUpdate downloads and verifies the pending release, then installs it.
// The staged copy in the data directory is always written first: it is what
// makes the update survive a container rebuild. Returns whether the running
// executable was replaced right away.
func applyUpdate(ctx context.Context) (bool, error) {
	rel := currentRelease()
	if rel == nil || strings.TrimSpace(rel.Tag) == "" {
		return false, errors.New("尚无可用更新，请先检查更新")
	}
	if devVersion() {
		return false, errors.New("当前是源码或本地构建，不支持自动更新")
	}
	if rel.AssetURL == "" {
		return false, errors.New("该版本没有提供当前平台的更新包")
	}
	stageDir := updateStageDir()
	if stageDir == "" {
		return false, errors.New("当前环境不允许自动替换程序文件")
	}
	exe, err := executableFile()
	if err != nil {
		return false, fmt.Errorf("无法定位当前可执行文件：%w", err)
	}

	tmp, err := os.CreateTemp(stageDir, ".auto-anime-download-*")
	if err != nil {
		return false, fmt.Errorf("无法写入更新临时文件：%w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	if err := downloadFile(ctx, rel, tmpPath); err != nil {
		return false, err
	}
	sum, err := verifyDownload(rel, tmpPath)
	if err != nil {
		return false, err
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return false, fmt.Errorf("无法设置可执行权限：%w", err)
	}

	staged := filepath.Join(stageDir, updateStageFileName)
	if err := copyFile(tmpPath, staged); err != nil {
		return false, fmt.Errorf("无法写入更新包：%w", err)
	}
	// 一旦新文件就位，旧校验记录必须消失，避免下次启动按旧值校验新文件。
	removeFileIfExists(staged + ".sha256")
	if err := os.WriteFile(staged+".sha256", []byte(sum+"\n"), 0o644); err != nil {
		logUpdate("写入校验记录失败：%v", err)
	}

	if !inPlaceExec() {
		return false, nil
	}
	if err := replaceRunningExecutable(exe, tmpPath); err != nil {
		logUpdate("未能替换运行中的程序（%v），更新包已暂存，将在下次启动时生效", err)
		return false, nil
	}
	logUpdate("已替换可执行文件 %s", exe)
	return true, nil
}

// replaceRunningExecutable swaps the file of the running process.
func replaceRunningExecutable(exe, fresh string) error {
	backup := exe + updateBackupSuffix
	removeFileIfExists(backup)
	if err := os.Rename(exe, backup); err != nil {
		return err
	}
	if err := renameInto(fresh, exe); err != nil {
		restoreBackup(backup, exe)
		return err
	}
	return nil
}

func removeFileIfExists(path string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		logUpdate("清理 %s 失败：%v", path, err)
	}
}

func downloadFile(ctx context.Context, rel *releaseInfo, target string) error {
	targetURL := downloadURL(rel)
	// GitHub 源的附件地址需要 Accept: application/octet-stream；
	// 自定义更新源的普通 URL 用默认 Accept 即可。
	withBinaryAccept := rel.AssetUseAPI
	res, err := downloadRequest(ctx, targetURL, withBinaryAccept)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	// 公开仓库的 browser_download_url 会 302 到 objects.githubusercontent.com，
	// 由 updateDownloadClient 跟随；这里只处理最终状态。
	if res.StatusCode != http.StatusOK && rel.PublicURL != "" && rel.PublicURL != targetURL {
		res.Body.Close()
		if res, err = downloadRequest(ctx, rel.PublicURL, false); err != nil {
			return err
		}
		defer res.Body.Close()
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("下载更新包失败：更新源返回 HTTP %d", res.StatusCode)
	}
	if res.ContentLength > updateMaxBinaryBytes {
		return fmt.Errorf("更新包大小异常（%d 字节），已中止", res.ContentLength)
	}
	file, err := os.Create(target)
	if err != nil {
		return err
	}
	defer file.Close()
	written, err := io.Copy(file, io.LimitReader(res.Body, updateMaxBinaryBytes+1))
	if err != nil {
		return fmt.Errorf("下载中断：%w", err)
	}
	if written > updateMaxBinaryBytes {
		return errors.New("更新包超过大小上限，已中止")
	}
	if written == 0 {
		return errors.New("更新包为空，已中止")
	}
	return file.Sync()
}

func downloadRequest(ctx context.Context, target string, binaryAccept bool) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "auto-anime-updater")
	if binaryAccept {
		req.Header.Set("Accept", "application/octet-stream")
	}
	if githubToken != "" {
		req.Header.Set("Authorization", "Bearer "+githubToken)
	}
	res, err := updateDownloadClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载更新包失败：%w", err)
	}
	return res, nil
}

// verifyDownload confirms the download belongs to the release and returns its
// SHA-256. A published checksum is authoritative; if the release has none, only
// a download directly from the configured GitHub repository is accepted.
func verifyDownload(rel *releaseInfo, path string) (string, error) {
	sum, err := hashFile(path)
	if err != nil {
		return "", fmt.Errorf("校验更新包失败：%w", err)
	}
	expected := strings.ToLower(strings.TrimSpace(rel.SHA256))
	if expected == "" && rel.SumURL != "" {
		expected = lookupChecksum(rel.SumURL, rel.AssetName, rel.SumUseAPI)
	}
	if expected != "" {
		if !strings.EqualFold(expected, sum) {
			return "", fmt.Errorf("更新包校验失败（期望 %s，实际 %s），已中止", shortHash(expected), shortHash(sum))
		}
		return sum, nil
	}
	// 没有校验文件时，只接受来自配置仓库/更新源的下载。
	if prefix := githubDownloadPrefix(); prefix != "" && strings.HasPrefix(rel.PublicURL, prefix) {
		logUpdate("该 Release 未提供校验文件，按更新源仓库地址接受 %s", filepath.Base(rel.PublicURL))
		return sum, nil
	}
	base, _, err := resolveUpdateSource(updateSource)
	if err != nil {
		return "", fmt.Errorf("更新源无效：%w", err)
	}
	if isGitHubAPI(base) {
		// API 附件地址由 api.github.com 提供，仓库归属已在检查阶段确认。
		if rel.AssetUseAPI {
			logUpdate("该 Release 未提供校验文件，按更新源仓库地址接受 %s", rel.AssetName)
			return sum, nil
		}
		return "", errors.New("更新源未提供 SHA-256 校验值，为安全起见已中止更新")
	}
	if !strings.HasPrefix(rel.AssetURL, base) {
		return "", fmt.Errorf("更新包来自 %s，不在更新源 %s 下，已中止", redactURL(rel.AssetURL), sourceHost())
	}
	return "", errors.New("更新源未提供 SHA-256 校验值，为安全起见已中止更新")
}

func lookupChecksum(sumURL, assetName string, sumUseAPI bool) string {
	ctx, cancel := context.WithTimeout(context.Background(), updateCheckTimeout)
	defer cancel()
	var res *http.Response
	var err error
	if sumUseAPI {
		res, err = downloadRequest(ctx, sumURL, true)
	} else {
		res, err = updateAPIRequest(ctx, http.MethodGet, sumURL)
	}
	if err != nil {
		return ""
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return ""
	}
	sums := parseChecksumList(string(body))
	if sum := sums[assetName]; sum != "" {
		return sum
	}
	// 有些发布只写一个校验和，不带文件名。
	if len(sums) == 1 {
		for _, sum := range sums {
			return sum
		}
	}
	return ""
}

func shortHash(hash string) string {
	if len(hash) <= 12 {
		return hash
	}
	return hash[:12]
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}

// ---- 路径与重启 ----

var (
	executablePath = os.Executable
	resolveSymlink = filepath.EvalSymlinks
)

func executableFile() (string, error) {
	exe, err := executablePath()
	if err != nil {
		return "", err
	}
	if resolved, err := resolveSymlink(exe); err == nil && resolved != "" {
		exe = resolved
	}
	return exe, nil
}

// updateStageDir returns the writable place for the update package. The data
// directory comes first because it is the only path a container is guaranteed
// to persist: a package written next to the executable can disappear when the
// container is recreated from the image. Bare-metal runs without a writable
// data directory fall back to the executable's directory.
func updateStageDir() string {
	if dir := dataDir(); dir != "" && writableDir(dir) == nil {
		return dir
	}
	if exe, err := executableFile(); err == nil {
		if writableDir(filepath.Dir(exe)) == nil {
			return filepath.Dir(exe)
		}
	}
	return ""
}

// dataDir mirrors the DB location, the one path a container always persists.
func dataDir() string {
	dir := filepath.Dir(env("DB_PATH", "/data/auto-anime.db"))
	if dir == "" || dir == "." {
		return ""
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	return dir
}

func writableDir(dir string) error {
	probe, err := os.CreateTemp(dir, ".auto-anime-write-*")
	if err != nil {
		return err
	}
	name := probe.Name()
	probe.Close()
	os.Remove(name)
	return nil
}

// installReady reports whether an update package is already waiting to be
// applied by the next start. Only the data directory matters: the mirror next
// to the executable is a convenience for the current run, not a durable place.
func installReady() bool {
	dir := updateStageDir()
	if dir == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(dir, updateStageFileName))
	return err == nil && !info.IsDir()
}

// bootstrapStagedUpdate applies an update package left by an earlier run. It
// runs before the HTTP server starts, so a container rebuilt from an older
// image still lands on the new version.
func bootstrapStagedUpdate() {
	for _, dir := range updateStageDirs() {
		staged := filepath.Join(dir, updateStageFileName)
		expected := ""
		if data, err := os.ReadFile(staged + ".sha256"); err == nil {
			expected = strings.TrimSpace(string(data))
		}
		swapped, err := adoptStagedUpdate(staged, expected)
		if err != nil {
			logUpdate("启动升级失败：%v", err)
			continue
		}
		if !swapped {
			continue
		}
		logUpdate("已应用启动升级包，重新启动以运行新版本")
		if err := startReplacementProcess(os.Args); err != nil {
			logUpdate("自动重启失败：%v（已替换程序文件，下次启动即为新版本）", err)
			return
		}
		os.Exit(0)
	}
}

func updateStageDirs() []string {
	dirs := []string{}
	seen := map[string]bool{}
	for _, dir := range []string{updateStageDir(), dataDir()} {
		if dir == "" || seen[dir] {
			continue
		}
		seen[dir] = true
		dirs = append(dirs, dir)
	}
	return dirs
}

// adoptStagedUpdate replaces the running executable with a staged file. It
// refuses anything that does not match the recorded checksum, and keeps the
// previous file as <executable>.old.
func adoptStagedUpdate(staged, expected string) (bool, error) {
	info, err := os.Stat(staged)
	if err != nil || info.IsDir() || info.Size() < 1<<20 {
		return false, nil
	}
	sum, err := hashFile(staged)
	if err != nil {
		return false, err
	}
	if expected != "" && !strings.EqualFold(expected, sum) {
		os.Remove(staged)
		os.Remove(staged + ".sha256")
		return false, fmt.Errorf("启动升级包校验失败（%s）", shortHash(sum))
	}
	exe, err := executableFile()
	if err != nil {
		return false, err
	}
	if err := replaceExecutable(exe, staged); err != nil {
		return false, err
	}
	// 只清理确实被采用的那份暂存文件。
	if filepath.Dir(staged) != filepath.Dir(exe) {
		os.Remove(staged)
	}
	os.Remove(staged + ".sha256")
	return true, nil
}

func replaceExecutable(exe, staged string) error {
	backup := exe + updateBackupSuffix
	os.Remove(backup)
	if err := os.Rename(exe, backup); err != nil {
		return fmt.Errorf("无法备份旧程序：%w", err)
	}
	if filepath.Dir(staged) == filepath.Dir(exe) {
		if err := renameInto(staged, exe); err != nil {
			restoreBackup(backup, exe)
			return err
		}
		return nil
	}
	if err := copyFile(staged, exe); err != nil {
		restoreBackup(backup, exe)
		return fmt.Errorf("无法写入新程序：%w", err)
	}
	return nil
}

func restoreBackup(backup, exe string) {
	if _, err := os.Stat(exe); err == nil {
		os.Remove(exe)
	}
	os.Rename(backup, exe)
}

// renameInto retries briefly: Linux keeps the old inode busy until the running
// process lets go (ETXTBSY).
func renameInto(src, dst string) error {
	var err error
	for attempt := 0; attempt < 6; attempt++ {
		if err = os.Rename(src, dst); err == nil {
			return nil
		}
		os.Remove(dst)
		time.Sleep(400 * time.Millisecond)
	}
	return fmt.Errorf("无法替换 %s：%w", dst, err)
}

// reExecAfterUpdate starts the freshly installed binary in place of this
// process. Unix replaces the process image; Windows cannot, so the caller
// reports the pending state and the next container start applies it.
func reExecAfterUpdate() error {
	exe, err := executableFile()
	if err != nil {
		return err
	}
	target := strings.TrimSuffix(exe, updateBackupSuffix)
	logUpdate("正在重启以运行 %s", versionLabel())
	if restartInPlace() {
		if err := execReplace(target, os.Args); err != nil {
			return fmt.Errorf("重启失败：%w", err)
		}
		return errors.New("重启失败：进程替换未生效")
	}
	return startProcess(target, os.Args)
}

func startReplacementProcess(args []string) error {
	exe, err := executableFile()
	if err != nil {
		return err
	}
	return startProcess(exe, args)
}

func startProcess(exe string, args []string) error {
	cmd := exec.Command(exe, args[1:]...)
	cmd.Env = os.Environ()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	configureChildProcess(cmd)
	return cmd.Start()
}

func versionLabel() string {
	label := strings.TrimSpace(version)
	if label == "" {
		label = "dev"
	}
	if c := strings.TrimSpace(commit); c != "" {
		if len(c) > 7 {
			c = c[:7]
		}
		label += " (" + c + ")"
	}
	return label
}

// ---- HTTP 接口 ----

// updateHandler serves the version state and starts a one-click update.
func updateHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	switch r.Method {
	case http.MethodGet:
		check := currentVersionCheck()
		if check.Current == "" {
			check.Current = strings.TrimSpace(version)
			check.State = "unknown"
		}
		jsonOut(w, updatePayload(check))
	case http.MethodPost:
		startUpdate()
		jsonOut(w, map[string]any{"started": true, "status": "downloading", "message": "正在下载更新包"})
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "方法不支持", http.StatusMethodNotAllowed)
	}
}

type updateResponse struct {
	versionCheck
	Version   string `json:"version"` // 当前运行版本
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
	Platform  string `json:"platform"`
	Enabled   bool   `json:"enabled"`
	CanUpdate bool   `json:"can_update"`
	// Status（idle | downloading | installing | restarting | failed）与 Message
	// 由 versionCheck 提供，随附在响应中。
}

func updatePayload(check versionCheck) updateResponse {
	progress := currentVersionCheck()
	check.Status, check.Message = updateProgress()
	pending := check.Pending || progress.Pending
	check.Pending = pending
	installHint := check.InstallHint
	if installHint == "" {
		if rel := currentRelease(); rel != nil {
			installHint = rel.UpdateHint
		}
	}
	// 一键更新失败时，具体原因优先于其它提示。
	if check.Status == "failed" && check.Message != "" {
		installHint = check.Message
	}
	check.InstallHint = installHint
	return updateResponse{
		versionCheck: check,
		Version:      strings.TrimSpace(version),
		Commit:       strings.TrimSpace(commit),
		BuildTime:    strings.TrimSpace(buildTime),
		Platform:     runtime.GOOS + "/" + runtime.GOARCH,
		Enabled:      updateEnabled,
		CanUpdate:    check.CanInstall,
	}
}

// startUpdate launches the install in the background; the HTTP handler answers
// immediately so the UI can follow progress through GET /api/update.
func startUpdate() {
	updateStateMu.Lock()
	if updateStarting {
		updateStateMu.Unlock()
		return
	}
	updateStarting = true
	updateStateMu.Unlock()
	setUpdateProgress("downloading", "正在下载更新包", false)
	logUpdate("开始一键更新，当前 %s", versionLabel())
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), updateDownloadTime)
		defer cancel()
		inPlace, err := applyUpdate(ctx)
		updateStateMu.Lock()
		updateStarting = false
		updateStateMu.Unlock()
		if err != nil {
			setUpdateProgress("failed", err.Error(), false)
			logUpdate("更新失败：%v", err)
			return
		}
		if !inPlace {
			// 更新包已写好：等待重启（Windows 与只读镜像层由启动引导替换）
			setUpdateProgress("installing", "更新包已就绪，重启服务后生效", true)
			logUpdate("更新包已写入暂存位置，重启后生效")
			return
		}
		setUpdateProgress("installing", "更新包已就绪，正在替换程序文件", false)
		// 留出时间让状态接口回包、并让在途的数据库写入结束。
		time.Sleep(1200 * time.Millisecond)
		setUpdateProgress("restarting", "正在重启服务", false)
		if err := reExecAfterUpdate(); err != nil {
			setUpdateProgress("failed", "新版本已就绪，但自动重启失败，请手动重启容器："+err.Error(), false)
			logUpdate("自动重启失败：%v", err)
			return
		}
		os.Exit(0)
	}()
}

// setUpdateProgress 记录一键更新的实时进度；staged 为 true 表示更新包已写好、
// 等待重启生效（Windows 与只读镜像层）。
func setUpdateProgress(status, message string, staged bool) {
	versionStateMu.Lock()
	versionState.Status = status
	versionState.Message = message
	versionState.Pending = versionState.Pending || staged
	versionStateMu.Unlock()
}

func updateProgress() (string, string) {
	versionStateMu.RLock()
	defer versionStateMu.RUnlock()
	status := versionState.Status
	if status == "" {
		status = "idle"
	}
	return status, versionState.Message
}

// sourceHost is used in logs instead of the full update source URL.
func sourceHost() string {
	base, label, err := resolveUpdateSource(updateSource)
	if err != nil {
		return label
	}
	return redactURL(base)
}

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.2.0", "1.2.0", 0},
		{"v1.2.0", "1.2.0", 0},
		{"1.2.0", "1.3.0", -1},
		{"1.3.0", "1.2.9", 1},
		{"1.2", "1.2.0", 0},
		{"1.2.0.1", "1.2.0", 1},
		{"2.0.0", "10.0.0", -1},
		{"1.2.0-rc1", "1.2.0", 0}, // 预发布只比较数字主体
		{"1.2.0+build5", "1.2.0", 0},
		{"dev", "1.2.0", 0}, // 无法解析时不触发更新
		{"", "1.2.0", 0},
		{"nightly", "1.0.0", 0},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q,%q)=%d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestLooksLikeReleaseTag(t *testing.T) {
	for _, tag := range []string{"1.2.0", "v1.2.0", "1.2.0-rc1", "0.0.1+build", "1.0"} {
		if !looksLikeReleaseTag(tag) {
			t.Errorf("%q 应视为发布版本号", tag)
		}
	}
	for _, tag := range []string{"dev", "", "nightly", "release-1", "1.2.x"} {
		if looksLikeReleaseTag(tag) {
			t.Errorf("%q 不应视为发布版本号", tag)
		}
	}
}

func TestResolveUpdateSource(t *testing.T) {
	cases := []struct {
		raw       string
		wantBase  string
		wantLabel string
		wantErr   bool
	}{
		{raw: "", wantBase: "https://api.github.com/repos/" + updateDefaultSource + "/releases", wantLabel: "github.com/" + updateDefaultSource},
		{raw: "owner/repo", wantBase: "https://api.github.com/repos/owner/repo/releases", wantLabel: "github.com/owner/repo"},
		{raw: "github:owner/repo", wantBase: "https://api.github.com/repos/owner/repo/releases", wantLabel: "github.com/owner/repo"},
		{raw: "https://github.com/owner/repo", wantBase: "https://api.github.com/repos/owner/repo/releases", wantLabel: "github.com/owner/repo"},
		{raw: "https://github.com/owner/repo/releases/latest/", wantBase: "https://api.github.com/repos/owner/repo/releases", wantLabel: "github.com/owner/repo"},
		{raw: "https://github.com/owner/repo.git", wantBase: "https://api.github.com/repos/owner/repo/releases", wantLabel: "github.com/owner/repo"},
		{raw: "https://dl.example.com/anime/", wantBase: "https://dl.example.com/anime", wantLabel: "dl.example.com"},
		{raw: "not a source", wantErr: true},
	}
	for _, c := range cases {
		base, label, err := resolveUpdateSource(c.raw)
		if c.wantErr {
			if err == nil {
				t.Errorf("resolveUpdateSource(%q) 期望报错", c.raw)
			}
			continue
		}
		if err != nil || base != c.wantBase {
			t.Errorf("resolveUpdateSource(%q)=%q,%v want %q", c.raw, base, err, c.wantBase)
		}
		if label != c.wantLabel {
			t.Errorf("resolveUpdateSource(%q) label=%q want %q", c.raw, label, c.wantLabel)
		}
	}
}

func TestParseChecksumList(t *testing.T) {
	text := strings.Join([]string{
		"# comment line without hash",
		strings.Repeat("a", 64) + "  auto-anime_1.2.0_linux_amd64",
		strings.Repeat("B", 64) + " *auto-anime_1.2.0_linux_arm64",
		"not-a-hash  file.bin",
		strings.Repeat("c", 63) + "  short.txt",
	}, "\r\n")
	sums := parseChecksumList(text)
	if len(sums) != 2 {
		t.Fatalf("解析到 %d 条校验和：%#v", len(sums), sums)
	}
	if sums["auto-anime_1.2.0_linux_amd64"] != strings.Repeat("a", 64) {
		t.Errorf("linux/amd64 校验和错误：%v", sums)
	}
	if sums["auto-anime_1.2.0_linux_arm64"] != strings.Repeat("b", 64) {
		t.Errorf("应接受大写哈希并去掉星号：%v", sums)
	}
}

func TestPickAssetForPlatform(t *testing.T) {
	tag := "v1.2.0"
	names := []string{"auto-anime_v1.2.0_linux_amd64", "auto-anime_v1.2.0_linux_arm64", "auto-anime_v1.2.0_windows_amd64.exe", "checksums.txt"}
	lookup := func(name string) string { return "https://example.test/" + name }
	chosen, url := pickAssetForPlatform(tag, names, lookup)
	if chosen == "" || url == "" {
		t.Fatalf("未选到当前平台附件：%q %q", chosen, url)
	}
	if !strings.Contains(chosen, "_"+platformPair()+".") && !strings.HasSuffix(chosen, "_"+platformPair()) {
		t.Fatalf("选中的附件不是当前平台：%s", chosen)
	}
	// 不带 v 前缀的文件命名也要能命中。
	chosen, _ = pickAssetForPlatform("v1.2.0", []string{"auto-anime_1.2.0_" + platformPair()}, lookup)
	if chosen == "" {
		t.Fatal("无 v 前缀的附件命名未命中")
	}
	// 缺少当前平台附件时返回空，由调用方提示手动更新。
	if chosen, _ := pickAssetForPlatform(tag, []string{"auto-anime_v1.2.0_plan9_mips"}, lookup); chosen != "" {
		t.Fatalf("不应选中其他平台附件：%s", chosen)
	}
}

func platformPair() string { return currentPlatform() }

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func TestNotesToText(t *testing.T) {
	html := `<h2>新增</h2><ul><li>一键更新</li><li>版本检查</li></ul><p>修复：<code>size</code> 计算</p>`
	text := notesToText(html)
	for _, want := range []string{"新增", "一键更新", "版本检查", "修复", "size"} {
		if !strings.Contains(text, want) {
			t.Errorf("转换结果缺少 %q：%q", want, text)
		}
	}
	if strings.Contains(text, "<") {
		t.Errorf("转换结果仍含标签：%q", text)
	}
	if plain := notesToText("纯文本说明"); plain != "纯文本说明" {
		t.Errorf("纯文本被改写：%q", plain)
	}
	if empty := notesToText("   "); empty != "" {
		t.Errorf("空说明应返回空串：%q", empty)
	}
}

func TestExtractReleaseNotes(t *testing.T) {
	body := []byte(`<html><script type="application/json" data-target="react-app.embeddedData">{"payload":{"release":{"bodyHTML":"<p>修复了 <b>BUG</b></p>"}}}</script></html>`)
	if got := extractReleaseNotes(body); !strings.Contains(got, "BUG") {
		t.Fatalf("未提取到更新说明：%q", got)
	}
	if got := extractReleaseNotes([]byte(`<html>no payload</html>`)); got != "" {
		t.Fatalf("无 payload 时应返回空：%q", got)
	}
}

// privateReleaseServer 模拟私有仓库的 Release：只提供 /releases/assets/<id>
// 下载（browser_download_url 会 404，与 GitHub 私有仓库行为一致）。
func privateReleaseServer(t *testing.T, repo, tag string, payload []byte, checksumText string) *httptest.Server {
	t.Helper()
	release := map[string]any{
		"tag_name":     tag,
		"body":         "私有仓库测试",
		"html_url":     "https://github.com/" + repo + "/releases/tag/" + tag,
		"published_at": "2026-01-02T03:04:05Z",
		"draft":        false,
	}
	server := httptest.NewServer(nil)
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			assetName := "auto-anime_" + tag + "_" + platformPair() + exeSuffix()
			sumName := "checksums.txt"
			release["assets"] = []map[string]any{
				{
					"name":                 assetName,
					"url":                  server.URL + "/api/uploads/assets/1",
					"browser_download_url": server.URL + "/downloads/" + tag + "/" + assetName,
				},
				{
					"name":                 sumName,
					"url":                  server.URL + "/api/uploads/assets/2",
					"browser_download_url": server.URL + "/downloads/" + tag + "/" + sumName,
				},
			}
			json.NewEncoder(w).Encode(release)
		case strings.Contains(r.URL.Path, "/assets/1"):
			if got := r.Header.Get("Accept"); got != "application/octet-stream" {
				t.Errorf("下载附件必须带 octet-stream Accept，实际 %q", got)
			}
			w.Write(payload)
		case strings.Contains(r.URL.Path, "/assets/2"):
			fmt.Fprint(w, checksumText)
		case strings.Contains(r.URL.Path, "/downloads/"):
			// 私有仓库：浏览器下载地址对匿名/API 客户端返回 404
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	})
	return server
}

func TestDownloadUsesReleaseAssetsAPI(t *testing.T) {
	const repo = "owner/private-repo"
	dir := t.TempDir()
	t.Setenv("DB_PATH", filepath.Join(dir, "auto-anime.db"))
	exe := filepath.Join(dir, "auto-anime"+exeSuffix())
	if err := os.WriteFile(exe, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldPath := executablePath
	executablePath = func() (string, error) { return exe, nil }
	defer func() { executablePath = oldPath }()

	payload := make([]byte, 2<<20)
	copy(payload, []byte("private release binary"))
	sum := sha256.Sum256(payload)
	assetName := "auto-anime_v1.2.0_" + platformPair() + exeSuffix()
	server := privateReleaseServer(t, repo, "v1.2.0", payload, hex.EncodeToString(sum[:])+"  "+assetName+"\n")
	defer server.Close()
	useRewriteTransport(t, server)
	withUpdateGlobals(t, "", "1.1.0")
	runUpdateCheck()

	rel := currentRelease()
	if rel == nil || rel.AssetName != assetName {
		t.Fatalf("未选到当前平台附件：%+v", rel)
	}
	if !rel.AssetUseAPI || !rel.SumUseAPI {
		t.Fatalf("私有仓库应通过 Release Assets API 下载更新包与校验文件：%+v", rel)
	}
	if !strings.Contains(rel.AssetURL, "/assets/") {
		t.Fatalf("下载地址应为附件 API 地址，实际 %q", rel.AssetURL)
	}
	if rel.PublicURL == "" {
		t.Fatal("应保留 browser_download_url 作为兜底")
	}

	inPlace, err := applyUpdate(context.Background())
	if err != nil {
		t.Fatalf("通过 Assets API 更新失败：%v", err)
	}
	_ = inPlace
	staged, err := os.ReadFile(filepath.Join(dir, updateStageFileName))
	if err != nil || len(staged) != len(payload) {
		t.Fatalf("暂存更新包不正确：%v %d", err, len(staged))
	}
	recorded, err := os.ReadFile(filepath.Join(dir, updateStageFileName+".sha256"))
	if err != nil || strings.TrimSpace(string(recorded)) != hex.EncodeToString(sum[:]) {
		t.Fatalf("校验记录不正确：%q %v", recorded, err)
	}
}

// withUpdateGlobals swaps the package-level update configuration for one test.
func withUpdateGlobals(t *testing.T, source string, versionValue string) {
	t.Helper()
	oldSource, oldVersion, oldCommit, oldProxy := updateSource, version, commit, updateProxyRaw
	oldRelease := currentRelease()
	updateSource, version, commit, updateProxyRaw = source, versionValue, "", ""
	t.Cleanup(func() {
		updateSource, version, commit, updateProxyRaw = oldSource, oldVersion, oldCommit, oldProxy
		versionStateMu.Lock()
		latestRelease = oldRelease
		versionState = versionCheck{}
		versionStateMu.Unlock()
		updateStateMu.Lock()
		updateStarting = false
		updateStateMu.Unlock()
	})
}

func releasePath(base string) string {
	return strings.TrimPrefix(strings.TrimPrefix(base, "https://github.com/"), "http://github.com/")
}

// githubReleaseServer pretends to be both api.github.com and github.com for one repo.
func githubReleaseServer(t *testing.T, repo, tag, body string, assets map[string]string) *httptest.Server {
	t.Helper()
	release := map[string]any{
		"tag_name":     tag,
		"body":         body,
		"html_url":     "https://github.com/" + repo + "/releases/tag/" + tag,
		"published_at": "2026-01-02T03:04:05Z",
		"draft":        false,
	}
	list := make([]map[string]any, 0, len(assets))
	for name := range assets {
		list = append(list, map[string]any{
			"name":                 name,
			"url":                  "https://api.github.com/repos/" + repo + "/releases/assets/" + name,
			"browser_download_url": "https://github.com/" + repo + "/releases/download/" + tag + "/" + name,
		})
	}
	release["assets"] = list
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			json.NewEncoder(w).Encode(release)
		default:
			for name, content := range assets {
				if strings.HasSuffix(r.URL.Path, "/"+name) {
					fmt.Fprint(w, content)
					return
				}
			}
			http.NotFound(w, r)
		}
	}))
}

// rewriteTransport sends api.github.com and github.com requests to the test server.
type rewriteTransport struct {
	target string
	base   http.RoundTripper
}

func (rt rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = "http"
	clone.URL.Host = strings.TrimPrefix(rt.target, "http://")
	base := rt.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(clone)
}

func useRewriteTransport(t *testing.T, server *httptest.Server) {
	t.Helper()
	old := updateTransportOverride
	updateTransportOverride = rewriteTransport{target: server.URL}
	t.Cleanup(func() { updateTransportOverride = old })
}

func TestRunUpdateCheckFindsNewRelease(t *testing.T) {
	const repo = "owner/repo"
	tag := "v1.2.0"
	assets := map[string]string{
		"auto-anime_v1.2.0_" + platformPair() + exeSuffix(): "binary",
		"checksums.txt": "sums",
	}
	server := githubReleaseServer(t, repo, tag, "### 更新内容\n- 支持一键更新", assets)
	defer server.Close()
	withUpdateGlobals(t, "", "1.1.0")
	useRewriteTransport(t, server)

	runUpdateCheck()
	check := currentVersionCheck()
	if check.State != "ok" || check.Latest != tag {
		t.Fatalf("检查结果异常：%+v", check)
	}
	if !check.UpdateAvailable || !check.CanInstall {
		t.Fatalf("应识别到可安装的新版本：%+v", check)
	}
	if !strings.Contains(check.Notes, "一键更新") {
		t.Fatalf("更新说明缺失：%q", check.Notes)
	}
	rel := currentRelease()
	if rel == nil || rel.AssetURL == "" || rel.SumURL == "" {
		t.Fatalf("release 信息不完整：%+v", rel)
	}
}

func TestRunUpdateCheckSameVersionKeepsQuiet(t *testing.T) {
	server := githubReleaseServer(t, "owner/repo", "v1.2.0", "", map[string]string{
		"auto-anime_v1.2.0_" + platformPair() + exeSuffix(): "binary",
	})
	defer server.Close()
	withUpdateGlobals(t, "", "1.2.0")
	useRewriteTransport(t, server)

	runUpdateCheck()
	check := currentVersionCheck()
	if check.UpdateAvailable || check.CanInstall {
		t.Fatalf("版本相同时不应提示更新：%+v", check)
	}
}

func TestRunUpdateCheckWithoutPlatformAsset(t *testing.T) {
	server := githubReleaseServer(t, "owner/repo", "v9.9.9", "", map[string]string{
		"auto-anime_v9.9.9_plan9_mips": "binary",
	})
	defer server.Close()
	withUpdateGlobals(t, "", "1.0.0")
	useRewriteTransport(t, server)

	runUpdateCheck()
	check := currentVersionCheck()
	if !check.UpdateAvailable {
		t.Fatalf("应提示有新版本：%+v", check)
	}
	if check.CanInstall || check.InstallHint == "" {
		t.Fatalf("缺少当前平台附件时必须给出说明且不可自动安装：%+v", check)
	}
}

func TestRunUpdateCheckDevVersionNeverInstalls(t *testing.T) {
	server := githubReleaseServer(t, "owner/repo", "v1.2.0", "notes", map[string]string{
		"auto-anime_v1.2.0_" + platformPair() + exeSuffix(): "binary",
	})
	defer server.Close()
	withUpdateGlobals(t, "", "dev")
	useRewriteTransport(t, server)

	runUpdateCheck()
	check := currentVersionCheck()
	if check.CanInstall || check.UpdateAvailable {
		t.Fatalf("dev 版本不应自动更新：%+v", check)
	}
	if !strings.Contains(check.InstallHint, "源码") {
		t.Fatalf("dev 版本应说明原因：%q", check.InstallHint)
	}
}

func TestRunUpdateCheckSourceFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	withUpdateGlobals(t, server.URL, "1.0.0")

	runUpdateCheck()
	check := currentVersionCheck()
	if check.State != "failed" || check.Error == "" {
		t.Fatalf("更新源 404 应记录失败：%+v", check)
	}
	if check.UpdateAvailable {
		t.Fatalf("失败时不应提示更新：%+v", check)
	}
}

func TestVerifyDownloadWithChecksumFile(t *testing.T) {
	payload := []byte("auto-anime binary payload")
	sum := sha256.Sum256(payload)
	dir := t.TempDir()
	file := filepath.Join(dir, "download")
	if err := os.WriteFile(file, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, hex.EncodeToString(sum[:])+"  auto-anime_v1.2.0_"+platformPair()+exeSuffix()+"\n")
	}))
	defer server.Close()

	rel := &releaseInfo{
		AssetName: "auto-anime_v1.2.0_" + platformPair() + exeSuffix(),
		AssetURL:  server.URL + "/auto-anime",
		SumURL:    server.URL + "/checksums.txt",
	}
	got, err := verifyDownload(rel, file)
	if err != nil || got != hex.EncodeToString(sum[:]) {
		t.Fatalf("校验应通过：%v %q", err, got)
	}

	// 内容不符必须中止。
	if err := os.WriteFile(file, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyDownload(rel, file); err == nil {
		t.Fatal("校验和不匹配时必须报错")
	}
}

func TestVerifyDownloadRequiresTrust(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "download")
	if err := os.WriteFile(file, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	// 自定义更新源且既无校验值、地址又不在更新源下：拒绝。
	withUpdateGlobals(t, "https://dl.example.com/anime/", "1.0.0")
	rel := &releaseInfo{AssetURL: "https://evil.example.com/auto-anime", AssetName: "auto-anime"}
	if _, err := verifyDownload(rel, file); err == nil {
		t.Fatal("来源不可信时必须拒绝")
	}
	// 自定义更新源下但没有校验值：同样拒绝。
	rel = &releaseInfo{AssetURL: "https://dl.example.com/anime/auto-anime", AssetName: "auto-anime"}
	if _, err := verifyDownload(rel, file); err == nil {
		t.Fatal("缺少校验值时必须拒绝")
	}
}

func TestAdoptStagedUpdateSwapsExecutable(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "auto-anime")
	if err := os.WriteFile(exe, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	staged := filepath.Join(dir, updateStageFileName)
	payload := make([]byte, 2<<20)
	copy(payload, []byte("new binary"))
	if err := os.WriteFile(staged, payload, 0o755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)

	oldPath := executablePath
	executablePath = func() (string, error) { return exe, nil }
	defer func() { executablePath = oldPath }()

	// 校验和不匹配：拒绝替换并清理暂存文件。
	swapped, err := adoptStagedUpdate(staged, strings.Repeat("0", 64))
	if err == nil || swapped {
		t.Fatalf("校验和不匹配时应拒绝：%v %v", swapped, err)
	}
	if _, err := os.Stat(exe); err != nil {
		t.Fatalf("拒绝后原文件应保留：%v", err)
	}

	if err := os.WriteFile(staged, payload, 0o755); err != nil {
		t.Fatal(err)
	}
	swapped, err = adoptStagedUpdate(staged, hex.EncodeToString(sum[:]))
	if err != nil || !swapped {
		t.Fatalf("校验通过时应完成替换：%v %v", swapped, err)
	}
	got, err := os.ReadFile(exe)
	if err != nil || len(got) != len(payload) {
		t.Fatalf("替换后的可执行文件不正确：%v %d", err, len(got))
	}
	if _, err := os.Stat(exe + updateBackupSuffix); err != nil {
		t.Fatalf("应保留旧程序备份：%v", err)
	}
}

func TestAdoptStagedUpdateIgnoresGarbage(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "auto-anime")
	if err := os.WriteFile(exe, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	staged := filepath.Join(dir, updateStageFileName)
	if err := os.WriteFile(staged, []byte("too small"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldPath := executablePath
	executablePath = func() (string, error) { return exe, nil }
	defer func() { executablePath = oldPath }()

	swapped, err := adoptStagedUpdate(staged, "")
	if err != nil || swapped {
		t.Fatalf("过小的暂存文件应被忽略：%v %v", swapped, err)
	}
	if _, err := os.Stat(exe + updateBackupSuffix); err == nil {
		t.Fatal("忽略时不应产生备份文件")
	}
}

func TestUpdateHandlerReportsState(t *testing.T) {
	withUpdateGlobals(t, "", "1.1.0")
	setVersionState(versionCheck{
		Current: "1.1.0", Latest: "1.2.0", State: "ok",
		UpdateAvailable: true, CanInstall: true, Notes: "内容",
	})
	recorder := httptest.NewRecorder()
	updateHandler(recorder, httptest.NewRequest(http.MethodGet, "/api/update", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("状态码 %d", recorder.Code)
	}
	var payload updateResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Version != "1.1.0" || payload.Latest != "1.2.0" || !payload.UpdateAvailable || !payload.CanUpdate {
		t.Fatalf("响应内容不正确：%+v", payload)
	}
	if payload.Status != "idle" {
		t.Fatalf("空闲状态应为 idle：%q", payload.Status)
	}
	if payload.Platform == "" || !payload.Enabled {
		t.Fatalf("缺少平台或启用标记：%+v", payload)
	}

	// 失败信息要出现在提示里，供界面直接展示。
	setUpdateProgress("failed", "更新包校验失败", false)
	recorder = httptest.NewRecorder()
	updateHandler(recorder, httptest.NewRequest(http.MethodGet, "/api/update", nil))
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status != "failed" || !strings.Contains(payload.InstallHint, "校验失败") {
		t.Fatalf("失败状态未透出：%+v", payload)
	}
}

func TestUpdateHandlerRejectsUnsupportedMethod(t *testing.T) {
	recorder := httptest.NewRecorder()
	updateHandler(recorder, httptest.NewRequest(http.MethodDelete, "/api/update", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("应拒绝 DELETE：%d", recorder.Code)
	}
}

func TestStartUpdateReportsFailure(t *testing.T) {
	// dev 版本会拒绝自动更新，用来验证 POST → 后台任务 → 失败状态的接线。
	withUpdateGlobals(t, "", "dev")
	versionStateMu.Lock()
	latestRelease = &releaseInfo{Tag: "v1.2.0", AssetURL: "https://github.com/owner/repo/releases/download/v1.2.0/auto-anime"}
	versionStateMu.Unlock()
	recorder := httptest.NewRecorder()
	updateHandler(recorder, httptest.NewRequest(http.MethodPost, "/api/update", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("POST 应立即返回：%d %s", recorder.Code, recorder.Body.String())
	}
	var started map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &started); err != nil || started["started"] != true {
		t.Fatalf("响应内容不正确：%v %v", started, err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if status, message := updateProgress(); status == "failed" {
			if !strings.Contains(message, "源码") {
				t.Fatalf("失败原因未透出：%q", message)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	status, message := updateProgress()
	t.Fatalf("后台更新未进入失败状态：%s %q", status, message)
}

func TestApplyUpdateRefusesWithoutRelease(t *testing.T) {
	withUpdateGlobals(t, "", "1.0.0")
	if _, err := applyUpdate(context.Background()); err == nil {
		t.Fatal("没有可用发布时必须报错")
	}
	withUpdateGlobals(t, "", "dev")
	versionStateMu.Lock()
	latestRelease = &releaseInfo{Tag: "v1.1.0", AssetURL: "https://github.com/owner/repo/releases/download/v1.1.0/x"}
	versionStateMu.Unlock()
	if _, err := applyUpdate(context.Background()); err == nil {
		t.Fatal("dev 版本必须拒绝自动更新")
	}
}

func TestEnvBool(t *testing.T) {
	cases := map[string]bool{
		"": true, "0": false, "false": false, "off": false, "no": false, " FALSE ": false,
		"1": true, "true": true, "yes": true, "anything": true,
	}
	for raw, want := range cases {
		t.Setenv("UPDATE_CHECK", raw)
		if got := envBool("UPDATE_CHECK", true); got != want {
			t.Errorf("envBool(%q)=%v want %v", raw, got, want)
		}
	}
	if !envBool("AUTO_ANIME_MISSING_KEY_FOR_TEST", true) {
		t.Error("未设置时应返回默认值")
	}
	if envBool("AUTO_ANIME_MISSING_KEY_FOR_TEST", false) {
		t.Error("未设置时应返回传入的默认值")
	}
}

func TestRunUpdateCheckDisabled(t *testing.T) {
	withUpdateGlobals(t, "", "1.0.0")
	oldEnabled := updateEnabled
	updateEnabled = false
	defer func() { updateEnabled = oldEnabled }()

	runUpdateCheck()
	check := currentVersionCheck()
	if check.State != "disabled" {
		t.Fatalf("关闭检查后应为 disabled：%+v", check)
	}
	if check.UpdateAvailable || check.CanInstall {
		t.Fatalf("关闭检查后不应提示更新：%+v", check)
	}
}

func TestRunUpdateCheckKeepsNotesWhenVersionUnchanged(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DB_PATH", filepath.Join(dir, "auto-anime.db"))
	server := githubReleaseServer(t, "owner/repo", "v1.2.0", "", map[string]string{
		"auto-anime_v1.2.0_" + platformPair() + exeSuffix(): "binary",
	})
	defer server.Close()
	withUpdateGlobals(t, "", "1.1.0")
	useRewriteTransport(t, server)

	runUpdateCheck()
	// 第二次检查时 Release 正文为空，说明应沿用上一次的内容而不是清空。
	setVersionState(versionCheck{Current: "1.1.0", Latest: "v1.2.0", Notes: "上一次的说明", State: "ok"})
	versionStateMu.Lock()
	latestRelease = nil
	versionStateMu.Unlock()
	runUpdateCheck()
	if notes := currentVersionCheck().Notes; notes != "上一次的说明" {
		t.Fatalf("未沿用上次的更新说明：%q", notes)
	}
}

func TestInstallReadyDetectsStagedPackage(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DB_PATH", filepath.Join(dir, "auto-anime.db"))
	staged := filepath.Join(dir, updateStageFileName)
	if installReady() {
		t.Fatal("没有暂存文件时不应报告就绪")
	}
	if err := os.WriteFile(staged, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !installReady() {
		t.Fatal("存在暂存文件时应报告就绪")
	}
}

func TestImageVersionDrift(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DB_PATH", filepath.Join(dir, "auto-anime.db"))
	withUpdateGlobals(t, "", "1.2.0")
	if imageVersionDrift() {
		t.Fatal("没有镜像版本记录时不应报告不一致")
	}
	if err := os.WriteFile(filepath.Join(dir, "image-version"), []byte("version=1.2.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if imageVersionDrift() {
		t.Fatal("版本相同时不应报告不一致")
	}
	if err := os.WriteFile(filepath.Join(dir, "image-version"), []byte("version=1.1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !imageVersionDrift() {
		t.Fatal("运行版本与镜像版本不同时应报告不一致")
	}
}

func TestRunUpdateCheckReportsPendingUpdate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DB_PATH", filepath.Join(dir, "auto-anime.db"))
	if err := os.WriteFile(filepath.Join(dir, updateStageFileName), []byte("staged"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 更新源不可达也要能提示"已有更新包等待生效"。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	withUpdateGlobals(t, server.URL, "1.0.0")

	runUpdateCheck()
	check := currentVersionCheck()
	if !check.Pending {
		t.Fatalf("存在暂存更新包时应标记 pending：%+v", check)
	}
	if !strings.Contains(check.InstallHint, "重启") {
		t.Fatalf("pending 提示未给出重启说明：%q", check.InstallHint)
	}
}

func TestApplyUpdateDownloadsAndStages(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DB_PATH", filepath.Join(dir, "auto-anime.db"))
	exe := filepath.Join(dir, "auto-anime")
	if err := os.WriteFile(exe, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldPath := executablePath
	executablePath = func() (string, error) { return exe, nil }
	defer func() { executablePath = oldPath }()

	payload := make([]byte, 2<<20)
	copy(payload, []byte("new release binary"))
	sum := sha256.Sum256(payload)
	assetName := "auto-anime_v1.2.0_" + platformPair() + exeSuffix()
	server := githubReleaseServer(t, "owner/repo", "v1.2.0", "notes", map[string]string{
		assetName:       string(payload),
		"checksums.txt": hex.EncodeToString(sum[:]) + "  " + assetName + "\n",
	})
	defer server.Close()
	withUpdateGlobals(t, "", "1.1.0")
	useRewriteTransport(t, server)
	runUpdateCheck()
	if check := currentVersionCheck(); !check.CanInstall || check.Latest != "v1.2.0" {
		t.Fatalf("前置检查未通过：%+v", check)
	}

	inPlace, err := applyUpdate(context.Background())
	if err != nil {
		t.Fatalf("一键更新失败：%v", err)
	}
	// 更新包必须落到数据目录，容器重建后仍能生效。
	staged := filepath.Join(dir, updateStageFileName)
	stagedBytes, err := os.ReadFile(staged)
	if err != nil {
		t.Fatalf("未写入暂存更新包：%v", err)
	}
	if len(stagedBytes) != len(payload) {
		t.Fatalf("暂存更新包大小异常：%d", len(stagedBytes))
	}
	recorded, err := os.ReadFile(staged + ".sha256")
	if err != nil || strings.TrimSpace(string(recorded)) != hex.EncodeToString(sum[:]) {
		t.Fatalf("校验记录不正确：%q %v", recorded, err)
	}
	if inPlace {
		// Unix：运行中的可执行文件已被替换，并留有备份。
		if got, err := os.ReadFile(exe); err != nil || len(got) != len(payload) {
			t.Fatalf("就地替换失败：%v %d", err, len(got))
		}
		if _, err := os.Stat(exe + updateBackupSuffix); err != nil {
			t.Fatalf("应保留旧程序备份：%v", err)
		}
	}
}

func TestApplyUpdateRejectsChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DB_PATH", filepath.Join(dir, "auto-anime.db"))
	exe := filepath.Join(dir, "auto-anime")
	if err := os.WriteFile(exe, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldPath := executablePath
	executablePath = func() (string, error) { return exe, nil }
	defer func() { executablePath = oldPath }()

	payload := make([]byte, 2<<20)
	copy(payload, []byte("new release binary"))
	assetName := "auto-anime_v1.2.0_" + platformPair() + exeSuffix()
	server := githubReleaseServer(t, "owner/repo", "v1.2.0", "notes", map[string]string{
		assetName:       string(payload),
		"checksums.txt": strings.Repeat("0", 64) + "  " + assetName + "\n",
	})
	defer server.Close()
	withUpdateGlobals(t, "", "1.1.0")
	useRewriteTransport(t, server)
	runUpdateCheck()

	if _, err := applyUpdate(context.Background()); err == nil {
		t.Fatal("校验和不匹配时必须中止更新")
	}
	if _, err := os.Stat(filepath.Join(dir, updateStageFileName)); err == nil {
		t.Fatal("校验失败时不应留下更新包")
	}
}

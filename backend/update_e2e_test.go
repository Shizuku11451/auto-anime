package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// 端到端演练：编译两个“版本”的真实主程序，用本地假更新源让 1.0.0 实例
// 通过 /api/update 升级到 1.2.0，并验证暂存更新包与启动引导替换。
// 只有 Go 工具链，不依赖外部服务；用 -short 跳过。
func TestEndToEndOneClickUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("端到端演练在 -short 模式下跳过")
	}
	if _, err := exec.LookPath("go"); err != nil {
		if _, statErr := os.Stat(goToolPath()); statErr != nil {
			t.Skipf("未找到 go 工具链：%v", err)
		}
	}
	goTool := resolveGoTool(t)

	work := t.TempDir()
	buildConfig := t.TempDir()
	sourceDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// 两个可执行文件都来自当前源码，只有版本号不同。
	build := func(ldflags, output string) {
		t.Helper()
		cmd := exec.Command(goTool, "build", "-trimpath", "-ldflags", ldflags, "-o", output, ".")
		cmd.Dir = sourceDir // 必须在模块根目录构建
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOCACHE="+buildConfig)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("构建 %s 失败：%v\n%s", output, err, out)
		}
	}
	build("-X main.version=1.0.0 -X main.commit=e2e0001", filepath.Join(work, "app-old"+exeSuffix()))
	build("-X main.version=1.2.0 -X main.commit=e2e0002", filepath.Join(work, "app-new"+exeSuffix()))

	appOld := filepath.Join(work, "auto-anime"+exeSuffix())
	if err := copyFile(filepath.Join(work, "app-old"+exeSuffix()), appOld); err != nil {
		t.Fatal(err)
	}
	payload := filepath.Join(work, "payload.bin")
	if err := copyFile(filepath.Join(work, "app-new"+exeSuffix()), payload); err != nil {
		t.Fatal(err)
	}
	payloadBytes, err := os.ReadFile(payload)
	if err != nil {
		t.Fatal(err)
	}
	payloadSum := sha256.Sum256(payloadBytes)

	// 假更新源：latest.json + 更新包（清单里直接给出 sha256）
	assetName := "auto-anime-1.2.0-" + runtime.GOOS + "-" + runtime.GOARCH + exeSuffix()
	var release *httptest.Server
	release = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/latest.json"):
			json.NewEncoder(w).Encode(map[string]any{
				"version":      "1.2.0",
				"notes":        "### 更新内容\n- 支持一键更新\n- 修复下载队列",
				"published_at": "2026-09-15T00:00:00Z",
				"html_url":     "https://example.test/releases/1.2.0",
				"assets": map[string]string{
					runtime.GOOS + "-" + runtime.GOARCH: release.URL + "/assets/" + assetName,
					// 约定：assets["sha256"] 是更新包的 SHA-256 校验值
					"sha256": hex.EncodeToString(payloadSum[:]),
				},
			})
		case strings.HasPrefix(r.URL.Path, "/assets/"):
			w.Write(payloadBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer release.Close()

	dataDir := filepath.Join(work, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	port := freePort(t)
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	env := append(os.Environ(),
		"DB_PATH="+filepath.Join(dataDir, "auto-anime.db"),
		"DOWNLOAD_DIR="+filepath.Join(dataDir, "downloads"),
		"COVERS_DIR="+filepath.Join(dataDir, "covers"),
		"PORT="+fmt.Sprint(port),
		"UPDATE_SOURCE="+release.URL,
		"UPDATE_PROXY=direct", // 忽略环境里的系统代理
		"UPDATE_CHECK=1",
		"DOWNLOAD_MODE=direct",
		// 重启后的新进程延后监听，避免与旧进程抢占端口
		"AUTO_ANIME_STARTUP_DELAY_SECONDS=3",
	)
	appLog := filepath.Join(work, "app.log")
	logFile, err := os.OpenFile(appLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()

	startApp := func(binary string) *exec.Cmd {
		t.Helper()
		cmd := exec.Command(binary)
		cmd.Dir = work
		cmd.Env = env
		cmd.Stdout, cmd.Stderr = logFile, logFile
		if err := cmd.Start(); err != nil {
			t.Fatalf("启动实例失败：%v", err)
		}
		return cmd
	}
	stop := func(cmd *exec.Cmd) {
		if cmd == nil || cmd.Process == nil {
			return
		}
		cmd.Process.Kill()
		cmd.Wait()
	}

	app := startApp(appOld)
	defer stop(app)

	if !waitForHealth(baseURL, 40*time.Second) {
		t.Fatalf("实例未就绪，日志：\n%s", readFileTail(appLog, 40))
	}
	// 完成首次安装引导，之后用 Basic Auth 调用接口
	if _, err := http.Post(baseURL+"/api/setup", "application/json",
		strings.NewReader(`{"web_user":"admin","web_password":"e2e-pass"}`)); err != nil {
		t.Fatalf("初始化失败：%v", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	versionOf := func() string {
		body, err := authedGet(client, baseURL+"/api/version")
		if err != nil {
			return ""
		}
		var payload struct {
			Version string `json:"version"`
		}
		if json.Unmarshal(body, &payload) != nil {
			return ""
		}
		return payload.Version
	}
	if got := versionOf(); got != "1.0.0" {
		t.Fatalf("初始版本应为 1.0.0，实际 %q", got)
	}

	body, err := authedGet(client, baseURL+"/api/update")
	if err != nil {
		t.Fatalf("读取更新状态失败：%v", err)
	}
	var check map[string]any
	if err := json.Unmarshal(body, &check); err != nil {
		t.Fatal(err)
	}
	if check["latest"] != "1.2.0" || check["update_available"] != true {
		t.Fatalf("未识别到新版本：%s", body)
	}
	if notes, _ := check["notes"].(string); !strings.Contains(notes, "一键更新") {
		t.Fatalf("更新说明未透出：%s", body)
	}
	t.Logf("版本检查：latest=%v can_update=%v", check["latest"], check["can_update"])

	// 一键更新
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/update", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetBasicAuth("admin", "e2e-pass")
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /api/update 失败：%v", err)
	}
	res.Body.Close()

	// 等暂存更新包落盘（两种平台都会先写入数据目录）。应用先复制更新包、删掉旧记录，
	// 再写新校验记录：只等更新包本身会出现「包已完整、记录还没写」的窗口，CI 上文件
	// 系统慢时足以让下面的读取失败，所以这里等最后写入的校验记录。
	staged := filepath.Join(dataDir, updateStageFileName)
	record := staged + ".sha256"
	if !waitForFile(record, 60*time.Second) {
		st := updateStatus(client, baseURL)
		t.Fatalf("未写入暂存更新包校验记录（status=%q message=%q），日志：\n%s", st.Status, st.Message, readFileTail(appLog, 60))
	}

	// Unix 上会就地替换并自动重启；Windows 无法替换运行中的文件，只会暂存。
	if runtime.GOOS == "windows" {
		st := updateStatus(client, baseURL)
		if st.Status == "failed" {
			t.Fatalf("更新失败：%s，日志：\n%s", st.Message, readFileTail(appLog, 60))
		}
		t.Logf("已暂存更新包（等待重启生效）：status=%q pending=%v message=%q", st.Status, st.Pending, st.Message)
	} else if !waitForVersion(versionOf, "1.2.0", 90*time.Second) {
		t.Fatalf("未自动重启到 1.2.0（当前 %q），日志：\n%s", versionOf(), readFileTail(appLog, 60))
	} else {
		t.Logf("就地更新后版本：%s", versionOf())
	}

	stagedBytes, err := os.ReadFile(staged)
	if err != nil {
		t.Fatalf("读取暂存更新包失败：%v", err)
	}
	if len(stagedBytes) != len(payloadBytes) {
		t.Fatalf("暂存更新包大小异常：%d", len(stagedBytes))
	}
	recorded, err := os.ReadFile(record)
	if err != nil || strings.TrimSpace(string(recorded)) != hex.EncodeToString(payloadSum[:]) {
		t.Fatalf("校验记录不正确：%q %v", recorded, err)
	}

	// 模拟容器重建：可执行文件回到旧版本，重新启动后引导步骤应换成新版本
	stop(app)
	app = nil
	if err := os.Rename(staged, appOld); err != nil {
		t.Fatalf("模拟容器重建失败：%v", err)
	}
	app = startApp(appOld)
	defer stop(app)
	if !waitForHealth(baseURL, 40*time.Second) {
		t.Fatalf("引导重启后实例未就绪，日志：\n%s", readFileTail(appLog, 40))
	}
	if !waitForVersion(versionOf, "1.2.0", 30*time.Second) {
		t.Fatalf("启动引导未应用更新包（当前 %q），日志：\n%s", versionOf(), readFileTail(appLog, 40))
	}
	t.Logf("启动引导后版本：%s", versionOf())

	// 新进程应已清理暂存文件
	if _, err := os.Stat(staged); err == nil {
		t.Log("提示：引导后暂存文件仍存在（同目录替换时属预期）")
	}
	if out := readFileTail(appLog, 200); !strings.Contains(out, "[更新]") {
		t.Errorf("日志缺少更新过程记录：\n%s", out)
	}
}

func authedGet(client *http.Client, target string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth("admin", "e2e-pass")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	buf := make([]byte, 0, 4096)
	chunk := make([]byte, 4096)
	for {
		n, err := res.Body.Read(chunk)
		buf = append(buf, chunk[:n]...)
		if err != nil {
			break
		}
	}
	return buf, nil
}

func waitForHealth(baseURL string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		res, err := client.Get(baseURL + "/api/health")
		if err == nil {
			res.Body.Close()
			if res.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false
}

func waitForVersion(versionOf func() string, want string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if versionOf() == want {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

type updateSnapshot struct {
	Status     string `json:"status"`
	Message    string `json:"message"`
	Pending    bool   `json:"pending"`
	State      string `json:"state"`
	Version    string `json:"version"`
	CanInstall bool   `json:"can_install"`
}

func updateStatus(client *http.Client, baseURL string) updateSnapshot {
	var snapshot updateSnapshot
	body, err := authedGet(client, baseURL+"/api/update")
	if err != nil {
		return snapshot
	}
	json.Unmarshal(body, &snapshot)
	return snapshot
}

func waitForFile(path string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(path); err == nil && info.Size() > 0 {
			return true
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func readFileTail(path string, lines int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "(无法读取日志)"
	}
	all := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	return strings.Join(all, "\n")
}

func resolveGoTool(t *testing.T) string {
	t.Helper()
	if path, err := exec.LookPath("go"); err == nil {
		return path
	}
	if path := goToolPath(); path != "" {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	t.Skip("未找到 go 工具链，跳过端到端演练")
	return ""
}

// goToolPath 覆盖本机常见安装位置（Windows 上 go 不在 PATH 时）。
func goToolPath() string {
	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Go", "bin", "go"+exeSuffix()),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Go", "bin", "go"+exeSuffix()),
		`E:\go-sdk\go\bin\go.exe`,
		"C:\\Go\\bin\\go.exe",
	}
	for _, path := range candidates {
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

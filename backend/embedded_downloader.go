package main

import (
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/pbkdf2"
)

type managedQBittorrent struct {
	cmd    *exec.Cmd
	config qbitConnection
	done   chan struct{}
	ready  atomic.Bool
}

var embeddedQbit struct {
	sync.Mutex
	process  *managedQBittorrent
	stopping bool
}

var embeddedQbitFailure = make(chan error, 1)

func embeddedQbitRoot() (string, error) {
	root := strings.TrimSpace(env("QBITTORRENT_DATA_DIR", ""))
	if root == "" {
		root = filepath.Join(filepath.Dir(env("DB_PATH", "/data/auto-anime.db")), "qbittorrent")
	}
	return filepath.Abs(root)
}

func embeddedQbitPort(key string, fallback int) (int, error) {
	n, err := strconv.Atoi(env(key, strconv.Itoa(fallback)))
	if err != nil || n < 1 || n > 65535 {
		return 0, fmt.Errorf("%s 必须是 1-65535 的端口", key)
	}
	return n, nil
}

func embeddedQbitExecutable() (string, error) {
	name := strings.TrimSpace(env("QBITTORRENT_PATH", ""))
	if name == "" {
		name = "qbittorrent-nox"
	}
	exe, err := exec.LookPath(name)
	if err != nil {
		if runtime.GOOS == "windows" && strings.TrimSpace(env("QBITTORRENT_PATH", "")) == "" {
			return "", fmt.Errorf("Windows Release 程序不包含 qBittorrent；请安装 qBittorrent 并设置 QBITTORRENT_PATH，或改用 direct/外部下载器模式")
		}
		return "", fmt.Errorf("找不到 qbittorrent-nox，请使用包含内置下载器的镜像，或设置 QBITTORRENT_PATH")
	}
	return exe, nil
}

// The private profile and credentials are independent of external downloaders.
// They never enter settings/status APIs or the public backup format.
func prepareEmbeddedQbit(root string, port, btPort int) (qbitConnection, error) {
	config := qbitConnection{URL: fmt.Sprintf("http://127.0.0.1:%d", port), SavePath: filepath.Join(root, "downloads")}
	config.LocalDir = config.SavePath
	if err := os.MkdirAll(config.SavePath, 0700); err != nil {
		return config, err
	}
	credentialPath := filepath.Join(root, "credentials.json")
	var credentials struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	raw, err := os.ReadFile(credentialPath)
	if errors.Is(err, os.ErrNotExist) {
		secret := make([]byte, 32)
		if _, err = rand.Read(secret); err != nil {
			return config, err
		}
		credentials.Username, credentials.Password = "auto-anime", hex.EncodeToString(secret)
		raw, err = json.Marshal(credentials)
		if err == nil {
			err = writePrivateFile(credentialPath, append(raw, '\n'))
		}
	} else if err == nil {
		if len(raw) > 64<<10 || json.Unmarshal(raw, &credentials) != nil || credentials.Username != "auto-anime" || len(credentials.Password) < 32 {
			return config, fmt.Errorf("内置 qBittorrent 凭据文件无效，未覆盖原文件")
		}
	}
	if err != nil {
		return config, err
	}
	config.User, config.Password = credentials.Username, credentials.Password
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return config, err
	}
	hash := base64.StdEncoding.EncodeToString(salt) + ":" + base64.StdEncoding.EncodeToString(pbkdf2.Key([]byte(config.Password), salt, 100000, 64, sha512.New))
	settings := map[string]map[string]string{
		"LegalNotice": {"Accepted": "true"},
		"Preferences": {
			`WebUI\Enabled`: "true", `WebUI\Address`: "127.0.0.1", `WebUI\Port`: strconv.Itoa(port),
			`WebUI\Username`: config.User, `WebUI\Password_PBKDF2`: `"@ByteArray(` + hash + `)"`,
			`WebUI\LocalHostAuth`: "true", `WebUI\AuthSubnetWhitelistEnabled`: "false",
			`WebUI\CSRFProtection`: "true", `WebUI\HostHeaderValidation`: "true", `WebUI\UseUPnP`: "false",
			`General\StartMinimized`: "true", `General\Locale`: "zh_CN",
		},
		"BitTorrent": {
			`Session\DefaultSavePath`: qbitINIString(filepath.ToSlash(config.SavePath)),
			`Session\Port`:            strconv.Itoa(btPort), `Session\DHTEnabled`: "true", `Session\PeXEnabled`: "true",
			`Session\LSDEnabled`: "true", `Session\QueueingSystemEnabled`: "false",
		},
	}
	ext := ".conf"
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		ext = ".ini"
	}
	path := filepath.Join(root, "profile", "qBittorrent", "config", "qBittorrent"+ext)
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return config, err
	}
	updated, err := updateQbitINI(old, settings)
	if err != nil {
		return config, err
	}
	return config, writePrivateFile(path, updated)
}

func qbitINIString(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`).Replace(value) + `"`
}

// Preserve qBittorrent's own settings while enforcing the managed keys.
func updateQbitINI(raw []byte, values map[string]map[string]string) ([]byte, error) {
	if len(raw) > 2<<20 || !utf8.Valid(raw) {
		return nil, fmt.Errorf("内置 qBittorrent 配置不是有效的 UTF-8 文件或超过大小限制")
	}
	seen := map[string]map[string]bool{}
	var out []string
	section := ""
	flush := func() {
		keys := make([]string, 0, len(values[section]))
		for key := range values[section] {
			if !seen[section][key] {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			out = append(out, key+"="+values[section][key])
			seen[section][key] = true
		}
	}
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			flush()
			section = strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]")
			if seen[section] == nil {
				seen[section] = map[string]bool{}
			}
		} else if key, _, ok := strings.Cut(trimmed, "="); ok {
			key = strings.TrimSpace(key)
			if value, managed := values[section][key]; managed {
				line = key + "=" + value
				seen[section][key] = true
			}
		}
		out = append(out, line)
	}
	flush()
	sections := make([]string, 0, len(values))
	for name := range values {
		if seen[name] == nil {
			sections = append(sections, name)
		}
	}
	sort.Strings(sections)
	for _, name := range sections {
		section = name
		seen[name] = map[string]bool{}
		out = append(out, "", "["+name+"]")
		flush()
	}
	return []byte(strings.TrimSpace(strings.Join(out, "\n")) + "\n"), nil
}

func writePrivateFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".auto-anime-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}

func startEmbeddedQBittorrent() error {
	var count int
	if downloadMode != "builtin" {
		if err := db.QueryRow("SELECT COUNT(*) FROM items WHERE downloader='builtin'").Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			return nil
		}
	}
	_, err := embeddedQbitConnection()
	return err
}

func embeddedQbitConnection() (qbitConnection, error) {
	embeddedQbit.Lock()
	defer embeddedQbit.Unlock()
	if embeddedQbit.stopping {
		return qbitConnection{}, fmt.Errorf("内置 qBittorrent 正在关闭")
	}
	if p := embeddedQbit.process; p != nil {
		select {
		case <-p.done:
			return qbitConnection{}, fmt.Errorf("内置 qBittorrent 已退出，请重启服务")
		default:
			return p.config, nil
		}
	}
	exe, err := embeddedQbitExecutable()
	if err != nil {
		return qbitConnection{}, err
	}
	root, err := embeddedQbitRoot()
	if err != nil {
		return qbitConnection{}, err
	}
	port, err := embeddedQbitPort("EMBEDDED_QBIT_PORT", 8080)
	if err != nil {
		return qbitConnection{}, err
	}
	btPort, err := embeddedQbitPort("QBITTORRENT_BT_PORT", 6881)
	if err != nil {
		return qbitConnection{}, err
	}
	probe, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return qbitConnection{}, fmt.Errorf("内置 qBittorrent WebUI 端口 %d 已被占用", port)
	}
	probe.Close()
	config, err := prepareEmbeddedQbit(root, port, btPort)
	if err != nil {
		return config, err
	}
	logFile, err := os.OpenFile(filepath.Join(root, "process.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return config, err
	}
	args := []string{"--profile=" + filepath.Join(root, "profile"), "--confirm-legal-notice", "--webui-port=" + strconv.Itoa(port), "--torrenting-port=" + strconv.Itoa(btPort)}
	if runtime.GOOS == "windows" {
		args = append(args, "--no-splash")
	}
	p := &managedQBittorrent{cmd: exec.Command(exe, args...), config: config, done: make(chan struct{})}
	p.cmd.Stdout, p.cmd.Stderr = logFile, logFile
	configureChildProcess(p.cmd)
	if err := p.cmd.Start(); err != nil {
		logFile.Close()
		return config, fmt.Errorf("启动内置 qBittorrent 失败: %w", err)
	}
	go func() {
		err := p.cmd.Wait()
		logFile.Close()
		close(p.done)
		if p.ready.Load() {
			select {
			case embeddedQbitFailure <- fmt.Errorf("内置 qBittorrent 已退出: %v", err):
			default:
			}
		}
	}()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-p.done:
			return config, fmt.Errorf("内置 qBittorrent 启动失败，请检查 %s", filepath.Join(root, "process.log"))
		default:
		}
		client, err := loginQBittorrent(config, 2*time.Second)
		if err == nil {
			embeddedQbit.process = p
			p.ready.Store(true)
			select {
			case <-p.done:
				return config, fmt.Errorf("内置 qBittorrent 在启动后退出")
			default:
			}
			if err := cleanupEmbeddedQbitOrphans(client); err != nil {
				log.Printf("清理内置下载器已移除任务失败: %v", err)
			}
			log.Println("内置 qBittorrent 已就绪")
			return config, nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	p.cmd.Process.Kill()
	<-p.done
	return config, fmt.Errorf("内置 qBittorrent WebUI 未就绪，请检查 %s", filepath.Join(root, "process.log"))
}

func cleanupEmbeddedQbitOrphans(client *qbitClient) error {
	torrents, err := client.info(nil)
	if err != nil {
		return err
	}
	for _, torrent := range torrents {
		rel, err := filepath.Rel(client.config.SavePath, torrent.SavePath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			continue
		}
		owned, keep := false, false
		for _, tag := range strings.Split(torrent.Tags, ",") {
			tag = strings.TrimSpace(tag)
			if !strings.HasPrefix(tag, "auto-anime-") {
				continue
			}
			id, err := strconv.Atoi(strings.TrimPrefix(tag, "auto-anime-"))
			if err != nil || id <= 0 {
				continue
			}
			owned = true
			var count int
			if err := db.QueryRow("SELECT COUNT(*) FROM items WHERE id=? AND downloader='builtin' AND (COALESCE(task_id,'')='' OR task_id=?)", id, torrent.Hash).Scan(&count); err != nil {
				return err
			}
			keep = keep || count > 0
		}
		if owned && !keep {
			if err := client.remove(torrent.Hash); err != nil {
				return err
			}
		}
	}
	return nil
}

func stopEmbeddedQBittorrent() {
	embeddedQbit.Lock()
	embeddedQbit.stopping = true
	p := embeddedQbit.process
	embeddedQbit.Unlock()
	if p == nil {
		return
	}
	p.ready.Store(false)
	select {
	case <-p.done:
		return
	default:
	}
	if client, err := loginQBittorrent(p.config, 2*time.Second); err == nil {
		if err := client.post("app/shutdown", nil); err != nil {
			log.Println("内置 qBittorrent 关闭请求失败，等待进程退出")
		}
	}
	select {
	case <-p.done:
	case <-time.After(15 * time.Second):
		log.Println("内置 qBittorrent 未及时退出，终止进程")
		p.cmd.Process.Kill()
		<-p.done
	}
}

func embeddedQbitHealthy() bool {
	embeddedQbit.Lock()
	defer embeddedQbit.Unlock()
	p := embeddedQbit.process
	if p == nil {
		// A source install may switch modes before submitting its first BT job.
		return true
	}
	select {
	case <-p.done:
		return false
	default:
		return p.ready.Load()
	}
}

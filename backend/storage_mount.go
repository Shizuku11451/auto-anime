package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// 归档目录必须真正落在映射进容器的目录上。Docker 允许向未映射的绝对路径写入
// （文件落在容器的可写层），任务会显示「已保存」，但 NAS 文件管理里看不到文件，
// 容器重建后还会丢失。这里在用户保存目录时就检测并提示，而不是等文件不见了
// 才发现。检测只针对容器环境：读不到挂载表或根文件系统是真实文件系统
// （源码运行、本机 Linux）时不提示，避免误报。

// mountInfoPath 可在测试中替换；默认为内核挂载表。
var mountInfoPath = "/proc/self/mountinfo"

type mountPoint struct {
	path   string
	fsType string
}

// parseMountInfo 解析 /proc/self/mountinfo：第 5 列是挂载点，文件系统类型在
// 分隔符 "-" 之后的第一列；路径里的空格、制表符、换行与反斜杠以八进制转义。
func parseMountInfo(r io.Reader) []mountPoint {
	var mounts []mountPoint
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 6 {
			continue
		}
		sep := -1
		for i := 5; i < len(fields)-1; i++ {
			if fields[i] == "-" {
				sep = i
				break
			}
		}
		if sep < 0 {
			continue
		}
		mounts = append(mounts, mountPoint{path: unescapeMountPath(fields[4]), fsType: fields[sep+1]})
	}
	return mounts
}

// unescapeMountPath 还原内核为挂载点做的八进制转义（\040 空格、\011 制表符、
// \012 换行、\134 反斜杠）。
func unescapeMountPath(raw string) string {
	if !strings.Contains(raw, `\`) {
		return raw
	}
	var b strings.Builder
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\\' && i+3 < len(raw) {
			if n, err := strconv.ParseUint(raw[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(raw[i])
	}
	return b.String()
}

func readMountPoints() []mountPoint {
	f, err := os.Open(mountInfoPath)
	if err != nil {
		return nil
	}
	defer f.Close()
	return parseMountInfo(f)
}

// longestMount 返回覆盖 cleanPath 的最长挂载点前缀（包含 "/" 本身）。
func longestMount(cleanPath string, mounts []mountPoint) (mountPoint, bool) {
	best, found := mountPoint{}, false
	for _, m := range mounts {
		if m.path != "/" && m.path != cleanPath && !strings.HasPrefix(cleanPath, strings.TrimSuffix(m.path, "/")+"/") {
			continue
		}
		if !found || len(m.path) > len(best.path) {
			best, found = m, true
		}
	}
	return best, found
}

// containerRootFS 识别容器自己的（临时）根文件系统。普通 Linux 主机的 "/" 是
// 真实文件系统，写在它下面同样是持久的，不应提示。
func containerRootFS(fsType string) bool {
	return fsType == "overlay" || fsType == "fuse-overlayfs"
}

// unmappedPathWarning 在目录没有映射进容器时返回给用户看的提示；无法判断或
// 无需提示时返回空字符串。目录为空表示「沿用默认位置」，由调用方自行处理。
func unmappedPathWarning(dir string) string {
	clean := path.Clean(filepath.ToSlash(strings.TrimSpace(dir)))
	// 容器内是 POSIX 路径（/vol1/...）；Windows 源码运行时盘符路径也接受，
	// 但那里读不到挂载表，不会提示。
	if clean == "." || (!strings.HasPrefix(clean, "/") && !filepath.IsAbs(filepath.FromSlash(clean))) {
		return ""
	}
	mounts := readMountPoints()
	if len(mounts) == 0 {
		return ""
	}
	root, ok := longestMount("/", mounts)
	if !ok || root.path != "/" || !containerRootFS(root.fsType) {
		return ""
	}
	target, ok := longestMount(clean, mounts)
	if !ok || target.path != "/" {
		return ""
	}
	return fmt.Sprintf("%s 没有映射进容器：归档会写进容器自身的文件系统，NAS 上看不到，容器重建后会丢失。"+
		"请在 docker-compose.yml 的 volumes 里加入 “%s:%s”（或改用已挂载目录下的路径），再重启容器",
		clean, clean, clean)
}

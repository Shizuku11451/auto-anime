package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// 模拟容器内的挂载表：根是 overlay（容器可写层），/data 与 /mnt/my media 是
// 映射进来的宿主机目录。
const containerMountTable = `36 25 0:32 / / rw,relatime - overlay overlay rw,lowerdir=/var/lib/docker/overlay2/l
26 25 0:25 / /proc rw,nosuid,nodev,noexec,relatime shared:6 - proc proc rw
1234 36 8:1 / /data rw,relatime - ext4 /dev/sda1 rw
1236 36 8:3 / /mnt/my\040media rw,relatime - ext4 /dev/sdc1 rw
`

// 源码运行或本机 Linux：根是真实文件系统，写在它下面同样是持久的。
const hostMountTable = `36 25 8:1 / / rw,relatime - ext4 /dev/sda1 rw
1234 36 8:2 / /mnt/disk rw,relatime - btrfs /dev/sdb1 rw
`

// useMountInfo 让检测读取给定的挂载表内容；content 为空时模拟读不到（Windows
// 源码运行）。测试结束恢复原值。
func useMountInfo(t *testing.T, content string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mountinfo")
	if content != "" {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	old := mountInfoPath
	mountInfoPath = path
	t.Cleanup(func() { mountInfoPath = old })
}

func TestParseMountInfoReadsEscapedPaths(t *testing.T) {
	mounts := parseMountInfo(strings.NewReader(containerMountTable))
	want := map[string]string{"/": "overlay", "/data": "ext4", "/mnt/my media": "ext4", "/proc": "proc"}
	if len(mounts) != len(want) {
		t.Fatalf("解析出 %d 个挂载点，期望 %d：%+v", len(mounts), len(want), mounts)
	}
	for _, m := range mounts {
		if want[m.path] != m.fsType {
			t.Fatalf("挂载点 %q 的文件系统是 %q", m.path, m.fsType)
		}
	}
}

func TestUnmappedPathWarning(t *testing.T) {
	cases := []struct {
		name  string
		table string
		dir   string
		warn  bool
	}{
		{name: "容器内未映射的绝对路径", table: containerMountTable, dir: "/vol1/1000/番剧", warn: true},
		{name: "容器根目录", table: containerMountTable, dir: "/", warn: true},
		{name: "已映射的下载目录", table: containerMountTable, dir: "/data/library", warn: false},
		{name: "已映射且路径含空格", table: containerMountTable, dir: "/mnt/my media/番剧", warn: false},
		{name: "非容器环境不提示", table: hostMountTable, dir: "/srv/番剧", warn: false},
		{name: "读不到挂载表不提示", table: "", dir: "/vol1/1000/番剧", warn: false},
		{name: "空目录沿用默认位置", table: containerMountTable, dir: "", warn: false},
		{name: "相对路径交由校验拒绝", table: containerMountTable, dir: "relative/番剧", warn: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			useMountInfo(t, c.table)
			got := unmappedPathWarning(c.dir)
			if c.warn && !strings.Contains(got, "没有映射进容器") {
				t.Fatalf("%q 应当提示未映射，实际 %q", c.dir, got)
			}
			if !c.warn && got != "" {
				t.Fatalf("%q 不应提示，实际 %q", c.dir, got)
			}
		})
	}
}

// 保存订阅时选择未映射的目录：订阅照常保存、目录照常预建，但响应必须带上
// 「NAS 上看不到」的提示，避免用户等到找不到文件才发现。
func TestSubscriptionStorageWarnsUnmappedPath(t *testing.T) {
	testDatabase(t)
	useMountInfo(t, containerMountTable)
	base := filepath.ToSlash(t.TempDir())
	rec := postFeed(t, map[string]any{
		"name": "未映射订阅", "url": "https://example.test/unmapped",
		"target_title": "未映射动画", "storage_dir": base,
	})
	var created struct {
		StoragePath    string `json:"storage_path"`
		StorageWarning string `json:"storage_warning"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("create: %s", rec.Body.String())
	}
	if created.StoragePath == "" {
		t.Fatalf("保存位置应当写入，响应为 %s", rec.Body.String())
	}
	if !strings.Contains(created.StorageWarning, "没有映射进容器") || !strings.Contains(created.StorageWarning, "volumes") {
		t.Fatalf("缺少未映射提示：%q", created.StorageWarning)
	}
}

// 设置页拒绝尚未挂载的媒体目录。
func TestLibraryDirRejectsWhenUnmapped(t *testing.T) {
	testDatabase(t)
	useMountInfo(t, containerMountTable)
	setWebAuth(t, "", "")
	setupDone = true
	old := libraryDir
	t.Cleanup(func() { setupDone = false; webHash = ""; libraryDir = old })
	base := filepath.ToSlash(t.TempDir())
	rec := settingsPUT(t, `{"library_dir":"`+base+`"}`)
	if runtime.GOOS == "windows" {
		// Windows 本机没有容器卷映射，目录可写即可用。
		if rec.Code != 200 || libraryDir == old {
			t.Fatalf("windows local directory rejected: %d %s", rec.Code, rec.Body.String())
		}
		return
	}
	if rec.Code != 400 || libraryDir != old || !strings.Contains(rec.Body.String(), "未映射") {
		t.Fatalf("unmapped directory accepted: %d %s", rec.Code, rec.Body.String())
	}
}

// 番剧库里的存储位置走同一套检测。
func TestAnimeStorageWarnsWhenUnmapped(t *testing.T) {
	testDatabase(t)
	useMountInfo(t, containerMountTable)
	base := filepath.ToSlash(t.TempDir())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/anime/未映射番剧/storage", strings.NewReader(`{"storage_dir":"`+base+`","season":1}`))
	animeStorageAPI(rec, req)
	if rec.Code != 200 {
		t.Fatalf("put anime storage: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		OK      bool   `json:"ok"`
		Warning string `json:"storage_warning"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !out.OK || !strings.Contains(out.Warning, "没有映射进容器") {
		t.Fatalf("缺少存储位置提示：%s", rec.Body.String())
	}
}

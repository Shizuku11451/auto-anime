package main

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"
)

// 订阅保存位置：找番页添加订阅时选择的「番剧根目录」。选中的目录不会直接堆放
// 集数，而是在其下按番剧名建立子文件夹（<根目录>/<番剧名>/Season XX/…），
// 一个订阅对应一部番剧，换源或分季也不会把文件散落到媒体库根目录。
// 留空表示沿用默认位置（媒体库目录，未设置时为下载目录），归档布局见
// archiveRootFor / archiveTarget。

// subscriptionSeriesRoot 返回某个番剧在所选根目录下的实际文件夹。
// 根目录或番剧名为空时返回空字符串，表示"未指定，沿用默认位置"。
// 番剧名经 safeComponent 清理，标题里的 / 、.. 或 Windows 保留名不会逃出所选目录；
// 路径统一用正斜杠，与 sanitizeStorageDir 写入的存储位置保持同一种形式。
func subscriptionSeriesRoot(base, title string) string {
	base = strings.TrimSpace(base)
	title = strings.TrimSpace(title)
	if base == "" || title == "" {
		return ""
	}
	return cleanStoragePath(filepath.Join(base, safeComponent(title)))
}

// appendWarning 拼接多条给用户看的提示，避免后一条覆盖前一条；空提示忽略。
func appendWarning(existing, message string) string {
	if message == "" {
		return existing
	}
	if existing == "" {
		return message
	}
	return existing + "；" + message
}

// applySubscriptionStorage 把订阅选择的保存位置写到番剧记录上并预建番剧文件夹，
// 让用户在添加订阅后立刻能看到目录。文件仍按媒体库布局归档（Season XX 子目录 +
// "番剧名 - SXXEYY" 文件名），与番剧库里的存储设置共用 animes.storage_dir 字段。
//
// 返回实际目录与一条可展示的提示。目录创建失败只提示、不阻断订阅保存：挂载可能
// 稍后才补齐，归档时会按同一路径重试。previousBase 是该订阅此前的选择，用于在
// 用户清空保存位置时回收本订阅写入的目录。
func applySubscriptionStorage(base, title, previousBase string) (string, string) {
	now := time.Now().UTC().Format(time.RFC3339)
	title = strings.TrimSpace(title)
	if title == "" {
		return "", ""
	}
	root := subscriptionSeriesRoot(base, title)
	if root == "" {
		// 清空保存位置＝回到默认位置，只回收本订阅写入的目录，
		// 不覆盖用户在番剧库里单独设置过的位置。
		old := subscriptionSeriesRoot(previousBase, title)
		if old == "" {
			return "", ""
		}
		if _, err := db.Exec(`UPDATE animes SET storage_dir='',updated_at=? WHERE title=? AND storage_dir=?`, now, title, old); err != nil {
			return "", "保存位置已清空，但番剧记录更新失败：" + err.Error()
		}
		return "", ""
	}
	if _, err := ensureAnime(title); err != nil {
		return root, "保存位置已保存，但番剧记录写入失败：" + err.Error()
	}
	var current string
	db.QueryRow("SELECT COALESCE(storage_dir,'') FROM animes WHERE title=?", title).Scan(&current)
	warning := ""
	if current != "" && current != root {
		warning = fmt.Sprintf("该番剧原保存位置为 %s，已改为 %s；此前归档的文件不会自动迁移", current, root)
	}
	if _, err := db.Exec(`UPDATE animes SET storage_dir=?,updated_at=? WHERE title=?`, root, now, title); err != nil {
		return root, "保存位置已保存，但番剧记录更新失败：" + err.Error()
	}
	if err := checkSeriesRoot(root); err != nil {
		warning = appendWarning(warning, "保存位置暂时不可用："+err.Error()+"；下载归档时会重试")
	}
	warning = appendWarning(warning, unmappedPathWarning(root))
	return root, warning
}

// applyFeedStorageOnSync 让订阅保存位置覆盖同步后新识别出的番剧：只在番剧还没有
// 指定位置时写入，绝不覆盖用户在番剧库里的存储设置，也不覆盖其它订阅先写入的值。
func applyFeedStorageOnSync(base, title string) {
	root := subscriptionSeriesRoot(base, title)
	if root == "" {
		return
	}
	if _, err := db.Exec(`UPDATE animes SET storage_dir=?,updated_at=? WHERE title=? AND COALESCE(storage_dir,'')=''`,
		root, time.Now().UTC().Format(time.RFC3339), title); err != nil {
		log.Printf("订阅保存位置写入失败 %q: %v", title, err)
	}
}

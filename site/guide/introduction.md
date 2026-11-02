# 介绍

Auto-Anime 是一个为 NAS 打造的自动追番服务，后端使用 Go，界面使用 Vue 3。它把「找资源 → 订阅 → 下载 → 整理归档 → 观看」串成一条自动化流程：订阅一次，之后每一集都会按同步周期自动下载，并整理成媒体服务器可以直接识别的目录结构。

## 工作流程

1. **搜索**：按 Bangumi 条目搜索番剧，展开即可看到多个来源聚合的候选资源。
2. **订阅**：选中合适的字幕组和清晰度，核对预览、季数与保存位置后订阅。也可以直接添加 RSS / Atom 地址或导入 OPML。
3. **下载**：使用内置 BT 下载器，或交给 qBittorrent、Aria2、Transmission、直链下载。
4. **归档**：识别番名与季集，按 `<媒体根目录>/<番剧名>/Season XX/` 归档，并生成 NFO 与封面。
5. **观看**：在番剧库里浏览、播放，或让 Jellyfin、Emby、飞牛影视等媒体服务器扫描归档目录。

## 主要特性

- **Bangumi 集成**：条目搜索、资料与封面、放送星期、已完结判断、收藏同步。
- **多来源聚合搜索**：默认公共聚合服务覆盖蜜柑计划、动漫花园、ACG.RIP、Nyaa 和 Bangumi Moe，也可[自建](/advanced/search-server)或关闭。
- **可靠的下载管理**：排队、并发、暂停与继续，重启后自动恢复；任务身份核对防止迟到的结果覆盖新任务。
- **识别与纠错**：简繁等价关键词过滤、季集与合集区间解析、集数偏移推断，识别错误可手动纠正并迁移文件。
- **文件安全**：只有归档成功才标记「已保存」；删除订阅或条目默认保留媒体；迁移失败保护原记录和文件。

完整的已实现与未实现范围见[功能边界](/guide/features)。

## 平台适配范围

| 平台 | 安装方式 |
| --- | --- |
| **飞牛 fnOS** · 主要维护平台（x86-64） | [FPK 应用包](/install/fnos-app)（推荐）/ [Docker Compose](/install/fnos-compose) |
| **Linux** · amd64 / arm64 | [Docker Compose / 独立服务端程序](/install/docker) |
| **Windows 10/11** · x64 | [单文件程序或便携桌面 ZIP](/install/windows)，需要 WebView2 Runtime |

其他 NAS 与 macOS 暂不专门适配。各版本提供的制品以 [Release 页面](https://github.com/Shizuku11451/auto-anime/releases)为准。

## 使用须知

::: warning 只在可信局域网使用
不要把管理端口直接暴露到公网。
:::

- 播放器不转码，也不渲染外挂字幕；浏览器无法解码的格式请用外部媒体服务器。
- 删除订阅或条目默认保留媒体文件，删除媒体需要单独确认。
- JSON 备份只包含配置和订阅，不含历史任务和媒体文件。完整备份方法见[维护与备份](/guide/maintenance)。
- 请仅下载你有权访问和保存的内容。

## 许可

Auto-Anime 基于 [MIT License](https://github.com/Shizuku11451/auto-anime/blob/main/LICENSE) 发布。第三方软件保留各自许可，见 [THIRD-PARTY-NOTICES](https://github.com/Shizuku11451/auto-anime/blob/main/THIRD-PARTY-NOTICES.md)。

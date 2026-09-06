<div align="center">

# Auto-Anime · 自动追番

**一个使用 Go 编写、适合 NAS Docker 部署的 RSS 自动追番服务**

订阅番剧 RSS → 自动识别集数 → 提交 Aria2 / qBittorrent / Transmission 下载 → 归档到 NAS 媒体库

[![Go](https://img.shields.io/badge/Go-1.23-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Docker](https://img.shields.io/badge/Docker-ready-2496ED?logo=docker&logoColor=white)](docker-compose.yml)
[![CGO](https://img.shields.io/badge/CGO-disabled-success)](Dockerfile)

</div>

---

## ✨ 功能特性

**订阅与解析**
- 支持 RSS 2.0、RSS 1.0/RDF、Atom 格式，优先读取 enclosure 资源链接
- 包含 / 排除关键词双重过滤，按资源地址自动去重
- 添加订阅后立即执行首次同步；同步失败会保留订阅并显示错误原因
- 添加前可「预览 RSS」检查地址可达性（最多显示 20 条，不写入数据库）
- 支持从其他 RSS 阅读器导入 OPML（含嵌套目录）

**番剧识别**
- 从资源标题自动提取字幕组、集数与清晰度
- 兼容 `[07]`、`S01E09`、`- 09`、`第08话`、`[09v2]` 等常见命名
- 「第三季」「S01」「3期」等季标记不会误判为集数
- 始终保留原始标题，识别字段为本地规则解析结果

**下载与归档**
- 三种外部下载器联动：**Aria2 / qBittorrent / Transmission**，提交后持久化任务 ID 并轮询进度
- 直链 HTTP(S) 下载：先写临时目录，完整后一次性归档，中断不会被误标为已保存
- `.torrent` 链接自动以种子方式提交给 Aria2，不会把种子文件本身当成视频
- 下载完成后**复制**归档到 `{DOWNLOAD_DIR}/{番剧名}/`，下载器任务保持做种不受影响
- 自动生成 `info.json` 与同名 `.nfo`，便于 Emby / Jellyfin 刮削
- 容器重启后自动恢复排队任务与正在跟踪的下载器任务

**管理界面**
- 深色管理面板，左侧导航切换「最近条目 / 订阅管理」
- 状态筛选徽章（计数来自全库）、关键词搜索、下载实时进度条、失败原因展示
- 添加 / 编辑 / 删除 / RSS 预览均使用弹窗与右下角通知交互
- 样式与脚本随应用本地提供，**零公共 CDN 依赖**

**运维能力**
- Docker healthcheck 健康检查；公开 `/api/health` 供探针使用
- 可选 Web Basic Auth 登录保护
- JSON 备份 / 恢复订阅，Webhook 事件通知

## 🚀 快速开始

### Docker Compose（推荐）

```bash
git clone https://github.com/sssshj114/auto-anime.git
cd auto-anime

# 编辑 docker-compose.yml，把数据目录映射到 NAS 实际路径
# 例如 /volume1/docker/auto-anime:/data

docker compose up -d --build
```

启动后访问 `http://NAS地址:8000`，等待容器管理器显示 `healthy` 即可开始添加订阅。

### 从源码构建

需要 Go 1.23+，无需 CGO：

```bash
CGO_ENABLED=0 go build -o auto-anime .
./auto-anime
```

SQLite 驱动为 `modernc.org/sqlite`（纯 Go 实现），可直接交叉编译 Linux amd64 / arm64。

## ⚙️ 环境变量

### 基础配置

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `DOWNLOAD_MODE` | `direct` | 下载方式：`direct` 直链 / `aria2` / `qbittorrent` / `transmission` |
| `DOWNLOAD_DIR` | `/data/downloads` | 归档根目录 |
| `DB_PATH` | `/data/auto-anime.db` | SQLite 数据库路径 |
| `POLL_MINUTES` | `30` | RSS 定时同步周期（分钟，最小 5） |
| `DOWNLOADER_POLL_SECONDS` | `30` | 下载器任务进度轮询间隔（秒，最小 5） |

### Aria2

| 变量 | 说明 |
| --- | --- |
| `ARIA2_URL` | RPC 地址，如 `http://NAS地址:6800/jsonrpc` |
| `ARIA2_TOKEN` | RPC secret（可选） |
| `ARIA2_DIR` | Aria2 容器内的下载目录 |
| `ARIA2_LOCAL_DIR` | 同一目录映射到本容器的路径；两侧路径相同时可省略 |

HTTP(S) 直链走 `addUri`；`.torrent` 链接由本程序读取后经 `addTorrent` 提交。

### qBittorrent（需要 4.4+）

| 变量 | 说明 |
| --- | --- |
| `QBIT_URL` | WebUI 地址 |
| `QBIT_USER` / `QBIT_PASSWORD` | 登录账号，密码只从环境变量读取 |
| `QBIT_SAVE_PATH` | qBittorrent 容器内的保存目录 |
| `QBIT_LOCAL_DIR` | 同一目录映射到本容器的路径 |

磁力链接直接提取 infohash 跟踪；种子链接通过 `auto-anime-<条目ID>` 标签发现任务。

### Transmission

| 变量 | 说明 |
| --- | --- |
| `TRANSMISSION_URL` | RPC 地址 |
| `TRANSMISSION_USER` / `TRANSMISSION_PASSWORD` | 登录账号，自动处理 RPC session ID |
| `TRANSMISSION_DOWNLOAD_DIR` | Transmission 容器内的下载目录 |
| `TRANSMISSION_LOCAL_DIR` | 同一目录映射到本容器的路径 |

### Web 与通知

| 变量 | 说明 |
| --- | --- |
| `WEB_USER` / `WEB_PASSWORD` | **同时设置两者**才启用 Basic Auth；只设置其一不启用 |
| `WEBHOOK_URL` | 事件通知地址（可选） |

## 📥 下载器路径映射（重要）

> 这是外部下载器联动**必须理解**的配置。

下载器把文件存放在**下载器容器自己的目录**里，自动追番容器必须能读到同一位置才能归档：

- `QBIT_SAVE_PATH`、`ARIA2_DIR`、`TRANSMISSION_DOWNLOAD_DIR` 描述**下载器一侧**的路径
- `QBIT_LOCAL_DIR`、`ARIA2_LOCAL_DIR`、`TRANSMISSION_LOCAL_DIR` 描述同一目录映射到**本容器内**的路径

两个容器必须把**同一 NAS 目录**分别挂载到这两个路径上。只配置下载器一侧会导致归档一直失败，并会在条目上显示具体原因。

## 📖 使用说明

**条目状态**

| 状态 | 含义 |
| --- | --- |
| 待下载 | 已通过过滤，等待下载 |
| 下载中 | 直链下载进行中（显示进度条） |
| 已发送到下载器 | 已提交外部下载器并持续跟踪进度 |
| 已保存 | 文件已完整归档到 NAS 目录 |
| 下载失败 | 任务出错或被删除，显示具体原因 |
| 等待下载器 | 直链模式下的磁力 / 种子链接，需切换下载模式 |

> 提交到下载器只代表任务已受理；文件复制进归档目录后才标记「已保存」。归档失败时保留进度与原因，问题解决后下一轮自动重试，无需重新下载。

**备份与恢复**
- `GET /api/backup` 下载 JSON 备份，**不包含**下载器密码和 Webhook 地址
- 「恢复订阅」按 RSS 地址去重合并，不覆盖历史条目、不删除下载文件

**删除行为**
- 删除订阅或条目只删除记录，**不会**删除 NAS 上已保存的媒体文件

**做种与空间**
- 归档是**复制**而非移动，下载器中的任务和文件保持原样继续做种；磁盘占用会翻倍，请规划好容量
- 下载器中的任务不会被自动删除，是否停止做种由你决定

**安全**
- RSS 地址、下载器密码、Webhook 凭据不会出现在日志、状态接口和备份中
- 若 NAS 端口对局域网外可见，建议设置 `WEB_USER` 和 `WEB_PASSWORD`；`/api/health` 始终公开供 Docker healthcheck 使用

**Webhook 事件**

配置 `WEBHOOK_URL` 后，以下事件会以 JSON POST 推送：

`new_item` · `download_submitted` · `download_saved` · `download_failed`

## 🗂️ 项目结构

```
auto-anime/
├── main.go           # 启动、HTTP API、订阅同步、解析、下载调度
├── downloader.go     # 下载器任务提交、进度轮询、路径映射
├── archive.go        # 文件归档、文件名处理、NFO/info.json 生成
├── recovery.go       # 重启恢复及下载器配置校验
├── status.go         # 健康检查和运行状态
├── static/           # 管理界面（原生 HTML/CSS/JS，无 CDN）
├── Dockerfile        # 两阶段构建，CGO_ENABLED=0
└── docker-compose.yml
```

## 🗺️ 路线图

- [ ] 标准季目录整理（`Season 01/S01E07`），合并同季不同字幕组
- [ ] Bangumi / TMDB 元数据接入与手动修正
- [ ] 任务卡死检测与超时告警
- [ ] 下载器任务详情页与条目分页

## ⚠️ 说明

- 仅使用合法、公开授权的 RSS 内容和下载来源
- 所有出站请求（RSS、下载器、Webhook）均**显式直连**，不读取系统代理设置
- 若 RSS 地址在国内网络不可达，程序无法绕过网络限制，请更换为 NAS 可访问的来源
- 一个数据库只运行一个服务实例

## 📄 许可证

本项目基于 [MIT License](LICENSE) 开源发布。

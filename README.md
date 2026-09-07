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
- **封面识别**：按识别出的番剧名查询 Bangumi（bgm.tv），封面下载到本地缓存后在条目列表显示；名称不匹配的搜索结果不会被采用，查不到的番剧显示占位图，一周后自动重试；Bangumi 不可达只影响封面，不影响订阅和下载

**下载与归档**
- 三种外部下载器联动：**Aria2 / qBittorrent / Transmission**，提交后持久化任务 ID 并轮询进度
- 直链 HTTP(S) 下载：先写临时目录，完整后一次性归档，中断不会被误标为已保存
- `.torrent` 链接自动以种子方式提交给 Aria2，不会把种子文件本身当成视频
- **媒体库友好归档**：一部番一个独立文件夹，识别到集数的资源按 `番剧名/Season 01/番剧名 - S01E01.mp4` 存放（附同名 `.nfo` 与 `-info.json`），Emby / Jellyfin / Plex 可直接刮削；同番不同字幕组自动合并到同一目录，重名冲突自动加序号
- **自定义存储位置**：在条目上点「位置」可为单部番指定存储目录和季号（如第二季改 `Season 02`），留空恢复默认；配置随备份导出
- 下载完成后**复制**归档到媒体库，下载器任务保持做种不受影响
- 自动生成 `info.json` 与同名 `.nfo`，便于 Emby / Jellyfin 刮削
- 容器重启后自动恢复排队任务与正在跟踪的下载器任务

**管理界面**
- **首次安装引导**：全新部署后自动进入初始设置页，一步步完成管理员账户、下载方式、代理与同步周期，跳过也可之后再配置
- **设置页**：侧栏「设置」可随时修改管理员账户、下载模式与下载器地址凭据、路径映射、出站代理、同步周期，保存后立即生效，无需重启
- **账户登录**：设置 `WEB_USER` 和 `WEB_PASSWORD`（或通过引导 / 设置页配置）后启用独立登录页，支持亮 / 暗主题；登录会话 30 天有效，右上角可一键退出
- 亮色 / 暗色主题一键切换：未手动选择时跟随系统偏好，选择保存在浏览器本地
- 管理面板左侧导航切换「最近条目 / 订阅管理 / 设置」
- 状态筛选徽章（计数来自全库）、关键词搜索、下载实时进度条、失败原因展示
- 添加 / 编辑 / 删除 / RSS 预览均使用弹窗与右下角通知交互
- 样式与脚本随应用本地提供，**零公共 CDN 依赖**

**运维能力**
- Docker healthcheck 健康检查；公开 `/api/health` 供探针使用
- 可选账户登录保护：登录失败连续 5 次临时锁定来源 IP 15 分钟；脚本调用仍可使用 Basic Auth
- **出站代理**：RSS 不可达或 Bangumi 访问慢时，配置 `PROXY_URL` 让公网请求走代理，支持 HTTP / HTTPS / SOCKS5
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
| `PORT` | `8000` | Web 服务监听端口 |
| `COVERS_DIR` | `/data/covers` | 封面本地缓存目录 |
| `PROXY_URL` | 空（直连） | 出站代理地址，支持 `http://` `https://` `socks5://` `socks5h://`，见下方说明 |

### 出站代理（PROXY_URL）

默认**显式直连**，不读取系统代理设置；只有配置了 `PROXY_URL` 才走代理。地址写法：

```
http://192.168.1.10:7890
socks5://192.168.1.10:1080
http://用户名:密码@192.168.1.10:7890     # 需要认证的代理
```

- **走代理的流量**：RSS 同步与预览、直链 HTTP(S) 下载、`.torrent` 文件下载、Bangumi 封面识别、Webhook 通知
- **始终直连**：Aria2 / qBittorrent / Transmission 的 RPC 调用——它们通常在 NAS 本机或局域网，远程代理无法访问，走代理反而会失败
- 代理只影响本程序发起的请求；BT 任务在下载器内部下载，流量路径由下载器自身决定
- 代理地址会在日志和管理界面「运行状态」卡片中显示，但**账号密码会被隐藏**
- 配置了无法解析或不支持的协议时服务拒绝启动并给出原因，避免静默绕过代理造成"看起来能用其实没走代理"

### 封面识别（Bangumi）

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `BANGUMI_ENABLED` | `true` | 设为 `false` 关闭封面识别，条目只显示占位图 |
| `BANGUMI_API` | `https://api.bgm.tv` | Bangumi API 地址，一般无需修改 |

封面请求显式直连（不读取系统代理），每次查询间隔 0.5 秒；封面只在首次识别时下载一次并缓存到 `COVERS_DIR`，浏览器加载的是本地缓存副本。封面缓存不包含在备份中，删除缓存目录后会按需重新下载。

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
| `WEB_USER` / `WEB_PASSWORD` | **同时设置两者**才启用登录；只设置其一不启用。也可通过首次安装引导或设置页配置（数据库优先） |
| `WEBHOOK_URL` | 事件通知地址（可选） |

启用后浏览器访问任何页面都会先跳转到登录页，登录成功写入会话 Cookie（HttpOnly，30 天），侧栏「退出登录」可随时注销；`curl` 等脚本可继续使用 `-u 用户:密码` 的 Basic Auth 方式调用 API。登录会话保存在数据库中，服务重启后仍然有效。

### 用户设置（数据库）

除环境变量外，以下设置保存在数据库（`/data` 内）并可随时在**设置页**修改，数据库值优先、立即生效：

- 管理员用户名与密码（密码只存盐值哈希，页面上只能重设、不能查看）
- 下载模式与 Aria2 / qBittorrent / Transmission 的地址、凭据、路径映射
- 出站代理 `PROXY_URL`、RSS 同步周期 `POLL_MINUTES`

首次部署且未配置任何凭据时，访问面板会自动进入**初始设置引导**（可跳过）；这些设置随 `/data` 中的数据库持久化，重启不丢失。环境变量适合容器编排，页面设置适合运行中调整——两者都未提供时使用默认值。

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
- `GET /api/backup` 下载 JSON 备份，**不包含**下载器密码和 Webhook 地址；v2 备份额外包含每部番的自定义存储位置
- 「恢复订阅」按 RSS 地址去重合并，不覆盖历史条目、不删除下载文件

**媒体库结构**

识别到集数的资源按下列结构归档，Emby / Jellyfin / Plex 直接指向 `DOWNLOAD_DIR` 即可刮削：

```
{DOWNLOAD_DIR}/
└── 夏日动画/
    └── Season 01/
        ├── 夏日动画 - S01E07.mp4
        ├── 夏日动画 - S01E07.nfo
        └── 夏日动画 - S01E07-info.json
```

- 识别不到集数的资源（剧场版、NCOP 等）放在番剧文件夹根目录，保留原始文件名
- BT 多文件资源以 `SXXEYY` 集文件夹存放，内部保留下载器原始目录结构
- 同一部番的第二季：在条目上点「位置」把季号改为 2，文件名会变成 `S02Exx`
- 更改只影响之后归档的集数；已归档文件不会被移动，删除条目记录后重新下载即可套用新结构

**删除行为**
- 删除订阅或条目只删除记录，**不会**删除 NAS 上已保存的媒体文件

**做种与空间**
- 归档是**复制**而非移动，下载器中的任务和文件保持原样继续做种；磁盘占用会翻倍，请规划好容量
- 下载器中的任务不会被自动删除，是否停止做种由你决定

**安全**
- RSS 地址、下载器密码、Webhook 凭据不会出现在日志、状态接口和备份中
- 登录会话在数据库中只存随机令牌的 SHA-256 哈希；登录接口按来源 IP 限速，连续失败 15 分钟内拒绝再试
- 若 NAS 端口对局域网外可见，建议设置 `WEB_USER` 和 `WEB_PASSWORD`；`/api/health` 始终公开供 Docker healthcheck 使用

**Webhook 事件**

配置 `WEBHOOK_URL` 后，以下事件会以 JSON POST 推送：

`new_item` · `download_submitted` · `download_saved` · `download_failed`

## 🗂️ 项目结构

```
auto-anime/
├── main.go           # 启动、HTTP API、订阅同步、解析、下载调度
├── auth.go           # 登录页、会话管理、登录限速与访问控制
├── settings.go       # 用户设置、首次安装引导与凭据哈希
├── proxy.go          # PROXY_URL 解析、统一出站传输配置
├── downloader.go     # 下载器任务提交、进度轮询、路径映射
├── archive.go        # 文件归档、文件名处理、NFO/info.json 生成
├── recovery.go       # 重启恢复及下载器配置校验
├── status.go         # 健康检查和运行状态
├── static/           # 管理界面、登录页与初始设置引导（原生 HTML/CSS/JS，无 CDN）
├── Dockerfile        # 两阶段构建，CGO_ENABLED=0
└── docker-compose.yml
```

## 🗺️ 路线图

- [x] 标准季目录整理（`Season 01/S01E07`），合并同季不同字幕组，支持每番自定义存储位置与季号
- [ ] Bangumi / TMDB 元数据接入与手动修正
- [ ] 任务卡死检测与超时告警
- [ ] 下载器任务详情页与条目分页

## ⚠️ 说明

- 仅使用合法、公开授权的 RSS 内容和下载来源
- 所有出站请求**默认显式直连**，不读取系统代理设置；需要代理时显式配置 `PROXY_URL`（见「出站代理」说明）
- 若 RSS 地址在国内网络不可达，程序无法绕过网络限制，请更换为 NAS 可访问的来源，或配置 `PROXY_URL`
- 一个数据库只运行一个服务实例

## 📄 许可证

本项目基于 [MIT License](LICENSE) 开源发布。

<div align="center">
  <img src="web/public/logo.svg" alt="Auto-Anime" width="360">
  <p><strong>面向 NAS 的 RSS 自动追番与媒体库整理服务</strong></p>
  <p>订阅 RSS · 自动下载 · 识别归档 · 浏览播放</p>
  <p>
    <a href="https://github.com/Shizuku11451/auto-anime/actions/workflows/release.yml">构建与发布</a>
    · <a href="docs/CONFIGURATION.md">配置参考</a>
    · <a href="LICENSE">MIT License</a>
  </p>
</div>

Auto-Anime 把追番过程中重复的工作交给服务完成：从 RSS 或搜索结果订阅资源，按规则过滤并交给下载器，下载完成后整理成媒体服务器可以直接识别的目录、NFO 和封面。

## 适合什么场景

- 想在 NAS 上长期订阅番剧，不想手动盯 RSS 和搬运文件。
- 使用飞牛影视、Emby、Jellyfin 等媒体服务器管理本地收藏。
- 需要一个中文、手机可用，并且能在浏览器里查看下载状态的管理界面。

## 功能一览

| 订阅与发现 | 下载与归档 | 管理体验 |
| --- | --- | --- |
| ACG.RIP、蜜柑计划、动漫花园搜索 | 内置 qBittorrent（Docker） | 首次设置向导 |
| RSS / Atom、OPML 导入 | 直链、Aria2、qBittorrent、Transmission | 手机适配与亮暗主题 |
| 包含 / 排除关键词与定时同步 | 自动识别番剧、季、集并整理文件 | 内置播放器与断点续播 |
| 资源去重、可用性检测 | NFO、封面和标准 Season 目录 | 备份、Webhook 与版本更新 |

## Docker 部署

需要 Docker 和 Docker Compose。默认数据目录为当前仓库下的 `docker-data/`，会持久化数据库、下载器状态、源文件和封面。

```bash
git clone https://github.com/Shizuku11451/auto-anime.git
cd auto-anime
docker compose up -d --build
```

打开 `http://NAS地址:8000`，按设置向导创建账户。Docker 模式默认使用内置 qBittorrent，不需要填写下载器地址或密码。

部署到 NAS 时，把 `docker-compose.yml` 里的数据卷改成 NAS 上的持久化目录，例如：

```yaml
volumes:
  - /volume1/docker/auto-anime:/data
  # 归档/媒体库目录：也要在这里映射进容器，且容器内路径与设置页填写的一致
  # 飞牛 fnOS（存储空间1 的用户目录）示例：- /vol1/1000/番剧:/vol1/1000/番剧
```

| 端口 | 用途 |
| --- | --- |
| `8000/tcp` | Web 管理页面 |
| `6881/tcp`、`6881/udp` | BT / DHT |

> 使用发布镜像时，可执行 `docker compose pull && docker compose up -d`。本地构建和发布镜像的差异见[配置参考](docs/CONFIGURATION.md)。

## 三步开始追番

1. 在「找番」搜索番名，或在「订阅管理」添加 RSS；设置字幕组、关键词和保存位置（归档目录需已映射进容器，见[文件归档](#文件归档)）。
2. 开启自动下载，在「最近条目」查看队列、进度和失败原因。
3. 在「番剧库」查看已归档的季集；需要时重新识别、绑定 Bangumi 或迁移文件。

默认每 30 分钟同步一次 RSS，同时下载 2 个任务。两项都可以在网页「设置」或「最近条目 → 下载控制」中调整。

## 文件归档

默认媒体库位于容器内的 `/data/downloads`。识别到集数的资源会整理为：

```text
番剧名/
├── tvshow.nfo
├── poster.jpg
└── Season 01/
    ├── 番剧名 - S01E01.mp4
    └── 番剧名 - S01E01.nfo
```

资源标题会先做一次标题分析：字幕组、片源、编码、分辨率、字幕语言（`[WebRip][1080p HEVC AAC][简繁内封]`）与校验码（`[ABCD1234]`）这类标签既不进番剧名也不进目录名——无论写在方括号、圆括号（`(BD 1920x1080 x265)`）还是裸写在标题里（`摇曳百合 第01话 GB 1080P MP4`）。季与集分开解析（`3期`、`S3`、`第三季`、`3rd Season`、`II` → 季号，`S03E01` → 第 3 季第 1 集），`[01-12合集]`、`01~24` 这类合集区间不会被当成某一集。标题是「中文名 / 原名」这种双语形式时，番剧名取中文那段，不同字幕组的同一部番更容易归并到一起。

标题里写在前面的方括号如果其实是番剧名（`[咒术回战][01][1080p]`、`[无职转生] 第01话`）会当名字用；`無職転生Ⅲ` 这类罗马数字、以及挡在集数前面的年份（`无职转生 2023 - 01`）分别按季号与噪声处理。中文技术词只在与作品名之间有分隔（空格、括号）时才清理，「我们的重制人生」这类名字不会被啃掉一块；代价是 `无职转生简体内嵌第01话` 这种完全连写的标签不再清理。

**番剧名与番剧文件夹用同一个规范名**：匹配到 Bangumi 资料后自动改成官方名（优先中文名，其次日文原名，并剥掉「第三季」这类季标记，季由 `Season XX` 目录承载）。改名会同步条目、订阅目标名、NFO 与元数据；**已有媒体文件的目录不会被搬动**，只有空目录或只含 `poster.jpg`/`tvshow.nfo` 的目录才会跟着改名。季号优先级：条目上手动指定的季 > 番剧设置里的季 > 标题解析出的季 > 第 1 季。

不带序号的季标记（`进击的巨人 最终季`、`… 完`）没法确定季号，会保留在番剧名里。标题解析出的季在识别资源时写进条目，**升级前已收录的条目没有这份信息**，归档时按番剧设置或第 1 季处理。

BT 下载器会保留源文件用于做种，再复制一份到媒体库，请预留足够空间。媒体服务器扫描媒体库根目录即可读取本地 NFO 和封面。

### 归档目录必须映射进容器

上面的根目录（设置页「媒体库目录」、单部番剧存储位置、订阅保存位置）都是**容器内路径**。Docker 不会阻止程序写入未映射的绝对路径，但文件会落在容器自己的文件系统里：任务照样显示「已保存」，NAS 文件管理里却看不到，容器重建后还会丢失。

想归档到 NAS 的 `/vol1/1000/番剧`（飞牛 fnOS 存储空间 1 的用户目录），先在 `docker-compose.yml` 的 volumes 里映射：

```yaml
volumes:
  - /volume1/docker/auto-anime:/data
  - /vol1/1000/番剧:/vol1/1000/番剧
```

容器内路径与设置页/订阅里填写的路径必须一致；也可以映射成别的名字，此时要填容器内的那个路径。映射后重启容器。保存「媒体库目录」「保存位置」或番剧存储位置时程序会检测目录是否已映射，未映射会明确提示。已有条目的文件不会自动搬迁，需要迁移时用条目「纠错」勾选迁移。

## 下载模式

Docker 默认使用 `builtin`，适合直接开箱使用。也可以在设置页或环境变量中切换：

| 模式 | 适用情况 |
| --- | --- |
| `builtin` | 使用镜像内置 qBittorrent，支持磁力和种子 |
| `direct` | 只由 Auto-Anime 下载 HTTP(S) 文件 |
| `qbittorrent` | 连接外部 qBittorrent |
| `aria2` | 连接外部 Aria2 |
| `transmission` | 连接外部 Transmission |

Windows Release 不包含 qBittorrent；Windows 上请选择 `direct` 或外部下载器。若配置里仍是 `builtin` 且没有 `QBITTORRENT_PATH`，Windows 版会**告警并临时降级成 `direct`**（不再启动即退出），可在设置页改成 `direct` 或外部下载器。Windows 直接运行 Release `.exe` 时默认数据目录是程序同级的 `data\`。完整环境变量、外部下载目录映射、代理、备份和更新源说明见[配置参考](docs/CONFIGURATION.md)。

## 本地开发

后端需要 Go 1.23+。前端使用 Vue 3、Vite 和 TypeScript，建议 Node.js 22.12+。

```bash
# 终端 1：启动后端，默认使用 direct 下载模式
cd backend
go run .
```

```bash
# 终端 2：启动前端开发服务器
cd web
npm ci
npm run dev
```

访问 <http://127.0.0.1:5173/>。前端开发服务器会把 API 转发到本机 `8000` 端口。构建前端资源：

```bash
cd web
npm run build
```

构建产物输出到 `backend/static/`。后端数据默认写入 `backend/data/`，与 Docker 的 `docker-data/` 相互独立。

## 数据与维护

- 完整备份：停止容器后备份挂载到 `/data` 的整个目录。
- 网页 JSON 备份：用于恢复订阅、规则、存储设置和元数据绑定，不包含历史条目、下载任务和媒体文件。
- 健康检查：`GET /api/health`。
- 查看状态：`docker compose ps`；查看日志：`docker compose logs --tail=100 -f`。

更细的迁移、旧版升级和一键更新说明，请查看[配置参考](docs/CONFIGURATION.md)。

## 许可证

[MIT License](LICENSE)

请仅订阅和下载你有权访问、保存的内容。

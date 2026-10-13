<div align="center">
  <img src="web/public/logo.svg" alt="Auto-Anime" width="360">
  <p><strong>面向 NAS 的 RSS 自动追番与媒体库整理服务</strong></p>
  <p>订阅 RSS · 自动下载 · 识别归档 · 浏览播放</p>
</div>

Auto-Anime 将 RSS 资源交给下载器，跟踪下载进度，完成后整理到媒体库，并提供中文管理页面。后端使用 Go，前端资源随程序提供；Docker 默认内置 qBittorrent，Windows 提供独立桌面打包方式。

> 本文描述当前工作区实现，不保证远端 Release 已包含这些功能。安装前请检查对应版本的发布说明和附件。正式发布制品、不同 NAS 的权限与存储兼容性需要分别验收。

## 选择安装方式

| 环境 | 入口 | 默认数据与媒体位置 |
| --- | --- | --- |
| 通用 Docker / NAS | [Docker 快速安装](#docker-快速安装) | `./docker-data` → `/data`；媒体在 `/data/downloads` |
| 飞牛 fnOS Compose | [飞牛安装指南](docs/FNOS.md) | 项目下 `data` → `/data`，`media` → `/media` |
| 飞牛实验性应用包 | [FPK 安装说明](docs/FNOS_APP.md) | 应用数据目录与共享媒体目录；需要同版本公开镜像 |
| Windows 10/11 x64 | [Windows 桌面版](docs/WINDOWS.md) | 完整解压目录下的 `data`，需要 WebView2 Runtime |
| 源码开发 | [贡献指南](CONTRIBUTING.md) | 显式配置独立开发数据目录 |

完整导航见[文档索引](docs/README.md)。

## 当前功能

- 订阅 RSS / RDF / Atom，关键词过滤、OPML 导入与定时同步；找番支持 ACG.RIP、蜜柑计划、动漫花园及自定义搜索模板。
- Docker 内置 qBittorrent；支持 HTTP(S) 直链及外部 Aria2、qBittorrent、Transmission。
- 下载排队、并发限制、进度展示、单条与批量暂停/继续，重启恢复原下载器任务身份。
- 识别番名、季集和技术标签，按标准目录归档，生成本地 NFO 与封面；支持人工纠错及单条媒体迁移。
- 番剧库、Bangumi 资料绑定与每周更新日、浏览器播放与断点续播、亮暗主题及手机布局。
- 管理员登录、目录检测、配置备份、Webhook 和版本检查；符合条件的发布程序支持一键更新。

## Docker 快速安装

前提：Docker Compose 可用，NAS 能获取 GHCR 发布镜像，端口可用，数据目录持久化且可写。如果目标版本尚未发布，请使用[源码构建](CONTRIBUTING.md#源码镜像构建)，不要将本地构建成功理解为远端镜像可用。

```bash
git clone https://github.com/Shizuku11451/auto-anime.git
cd auto-anime
docker compose pull
docker compose up -d
```

打开 `http://NAS地址:8000`，创建账户，确认下载方式与保存目录。默认直连、每 30 分钟同步、同时下载 2 个任务；Docker 保留内置 qBittorrent，无需填写连接密码。

- 网页端口冲突：将 `8000:8000` 改为 `18000:8000`，访问新端口。
- BT 端口冲突：同时改 TCP、UDP 映射两侧与 `QBITTORRENT_BT_PORT`。无需公开内置 WebUI 的 8080 端口。
- 将媒体归档到其他 NAS 目录：先配置 volume，再在网页填写容器内路径。仅修改网页路径不会增加挂载。

例如在原 `/data` 挂载之外增加：

```yaml
volumes:
  - ./docker-data:/data
  - /volume1/media/anime:/media
```

随后将媒体库目录设为 `/media`。飞牛模板已单独映射媒体目录，见[平台指南](docs/FNOS.md)。不要改变旧部署的 `/data` 来源，否则会表现为全新安装。

镜像获取依赖 GHCR，源码克隆依赖 GitHub；网页代理不能帮助 Docker 拉取镜像。管理页面不使用运行时公共 CDN，RSS、元数据和版本检查仍取决于相应服务的可达性。

## 开始追番

1. 在「找番」选择来源并搜索，或在「订阅管理」直接添加 RSS。
2. 预览资源，确认季数、关键词和保存位置，再开启自动下载。
3. 在「最近条目」查看进度与错误；只有「已保存」表示已完成归档。
4. 在「番剧库」管理资料、纠错或播放，也可让飞牛影视、Emby、Jellyfin 扫描媒体目录。

搜索来源返回的窗口不等于全集；未标明季数不自动视为第一季。详细操作见[使用指南](docs/USAGE.md)。

识别到单集编号时，典型目录为：

```text
<媒体根目录>/<番剧名>/
├── tvshow.nfo
├── poster.jpg
└── Season 01/
    ├── 番剧名 - S01E01.mp4
    └── 番剧名 - S01E01.nfo
```

无法识别单集的资源不会被强行命名为某个剧集，可能保留原名或作为合集目录保存。更改番剧名或默认根目录不会自动搬动已有媒体。

## 使用边界

- 应用层主要按资源 URL 去重，未实现统一的同季同集版本择优、自动洗版、缺集补全或媒体库扫描。
- 暂停直链后继续从头下载，不承诺 HTTP Range 续传；播放器不转码、不渲染外挂字幕。
- 每周更新日是安排，不证明某天实际到片、仍在连载或已收齐。
- 删除记录默认保留媒体；网页 JSON 恢复不恢复历史任务或媒体。完整备份须包含数据库、下载器状态与媒体，见[维护指南](docs/MAINTENANCE.md)。
- 一个数据库只运行一个实例。请在可信局域网使用，完成账户初始化；不要直接将管理端口暴露到公网。

## 开发与贡献

Go 模块在 `backend/`，Vue 源码在 `web/`。前端构建生成 `backend/static/` 与安装配置页，产物随源码交付并嵌入 Go 程序，不手工编辑。目录、启动、测试与构建命令见[贡献指南](CONTRIBUTING.md)，发布过程见[维护者发布说明](docs/RELEASING.md)。

## 许可证

项目使用 [MIT License](LICENSE)。打包的第三方软件保留各自许可，Windows 桌面包包含相应许可与源码获取说明。请仅订阅和下载有权访问、保存的内容。

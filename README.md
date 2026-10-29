<div align="center">
  <br>
  <img src="web/public/logo.svg" alt="Auto-Anime" width="420">
  <br>
  <br>
  <p><strong>让追番自动发生，让媒体库井井有条。</strong></p>
  <p>为 NAS 打造的自动追番服务，使用 Go 与 Vue 构建。<br>
    番剧搜索 · 订阅追更 · 内置下载 · 季集归档 · 番剧库与播放</p>
  <br>
  <p>
    <a href="https://github.com/Shizuku11451/auto-anime/releases"><img src="https://img.shields.io/github/v/release/Shizuku11451/auto-anime?style=for-the-badge&label=Release&color=0284c7" alt="GitHub Release"></a>
    <a href="#平台适配范围"><img src="https://img.shields.io/badge/Platform-fnOS%20%7C%20Linux%20%7C%20Windows-0284c7?style=for-the-badge" alt="平台：fnOS、Linux、Windows"></a>
    <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-65a30d?style=for-the-badge" alt="MIT License"></a>
  </p>
  <p>
    <a href="https://github.com/Shizuku11451/auto-anime/releases/latest"><img src="https://img.shields.io/badge/Download-最新发布-65a30d?style=for-the-badge&logo=github&logoColor=white" alt="下载最新发布"></a>
  </p>
  <br>
  <p>
    <a href="#快速安装">快速安装</a> ·
    <a href="docs/USAGE.md">使用指南</a> ·
    <a href="docs/CONFIGURATION.md">配置参考</a> ·
    <a href="docs/README.md">全部文档</a>
  </p>
  <br>
</div>

---

## 能做什么

<table>
  <tr>
    <td width="50%" valign="top"><strong>先找作品，再挑版本</strong><br><br>按 Bangumi 条目搜索番剧，展开即可看到多个来源聚合的候选资源，按字幕组、清晰度筛选后一键订阅追更。也支持手动 RSS 与 OPML 导入。</td>
    <td width="50%" valign="top"><strong>下载不用另装软件</strong><br><br>内置纯 Go 的 BT 下载器，也可接入 qBittorrent、Aria2、Transmission 或直链。排队、并发、暂停与继续，重启后自动恢复。</td>
  </tr>
  <tr>
    <td width="50%" valign="top"><strong>归档自动整理</strong><br><br>识别番名与季集，按 <code>番剧名/Season XX/</code> 归档并生成 NFO 与封面，媒体服务器直接扫描。集齐全季后自动停订。</td>
    <td width="50%" valign="top"><strong>一个页面追番</strong><br><br>番剧库按星期、已完结、收藏状态筛选，同步 Bangumi 收藏，浏览器直接播放。中文界面，亮暗主题与手机布局。</td>
  </tr>
</table>

## 平台适配范围

| 平台 | 安装方式 |
| --- | --- |
| **飞牛 fnOS** · 主要维护平台（x86-64） | [FPK 应用包](docs/FNOS_APP.md)（推荐）/ [Docker Compose](docs/FNOS.md) |
| **Linux** · amd64 / arm64 | Docker Compose / 独立服务端程序 |
| **Windows 10/11** · x64 | [单文件程序或便携桌面 ZIP](docs/WINDOWS.md)，需要 WebView2 Runtime |

其他 NAS 与 macOS 暂不专门适配。各版本提供的制品以 [Release 页面](https://github.com/Shizuku11451/auto-anime/releases)为准。

## 快速安装

**飞牛 fnOS**：从 Release 下载 `.fpk`，在「应用中心 → 手动安装」导入。FPK 自带镜像，安装时无需联网拉取，但要先装好 Docker。详见 [FPK 安装说明](docs/FNOS_APP.md)。

**Windows**：下载单文件 `.exe` 或 `-desktop.zip`，放到有写权限的目录后双击运行。数据保存在程序旁的 `data\`，归档媒体在 `media\`。详见 [Windows 桌面版](docs/WINDOWS.md)。

**Docker Compose**（Linux 或其他支持 Docker 的环境）：

```bash
git clone https://github.com/Shizuku11451/auto-anime.git
cd auto-anime
docker compose up -d
```

打开 `http://NAS地址:8000`，创建管理员账户，确认下载方式、保存目录和同步周期即可开始使用。

默认值：内置 BT 下载器，每 30 分钟同步，同时下载 2 个任务。数据保存在 `./docker-data`，容器内媒体目录为 `/data/downloads`；BT 占用 6881–6890 共 10 个端口。

> 想把媒体归档到 NAS 上的其他文件夹，要先在 Compose 里添加 volume 挂载，再在网页里填写**容器内**的路径；只改网页设置不会新增挂载。详见[存储位置与目录映射](docs/CONFIGURATION.md#存储位置与目录映射)。目标版本没有发布镜像时，可以[从源码构建](CONTRIBUTING.md#源码镜像构建)。

## 开始追番

1. **搜索**：在「搜索」输入番名，先确认 Bangumi 条目，再展开查看候选资源。
2. **订阅**：选中合适的版本，点击「订阅此版本」，核对预览、季数和保存位置后确认。之后按同步周期自动追更新集。
3. **跟进**：在「最近条目」查看下载进度。只有「已保存」才表示文件已归档完成。
4. **观看**：在「番剧库」浏览和播放，或让 Jellyfin、Emby、飞牛影视等媒体服务器扫描归档目录。

搜索候选资源默认使用公共聚合服务 `https://search.115415.xyz`，无需令牌，目前覆盖蜜柑计划、动漫花园、ACG.RIP、Nyaa 和 Bangumi Moe。使用时请求者 IP 和条目 ID 会发送给该服务。也可以[自建服务](docs/SEARCH_SERVER.md)，或用 `AGGREGATE_SEARCH_URL=""` 关闭。聚合服务或 Bangumi 不可用时，已有的 RSS 订阅和下载不受影响。

更多操作（关键词规则、集数偏移、识别纠错、批量管理、Bangumi 收藏同步）见[使用指南](docs/USAGE.md)。

## 使用须知

- **只在可信局域网使用**，不要把管理端口直接暴露到公网。
- 播放器不转码，也不渲染外挂字幕；浏览器无法解码的格式请用外部媒体服务器。
- 删除订阅或条目默认保留媒体文件，删除媒体需要单独确认。
- JSON 备份只包含配置和订阅，不含历史任务和媒体文件。完整备份方法见[维护指南](docs/MAINTENANCE.md)。
- 已实现与未实现的范围见[功能边界](docs/FEATURE_COMPARISON.md)。

## 文档

| 想了解 | 查看 |
| --- | --- |
| 搜索、订阅、番剧库与播放 | [使用指南](docs/USAGE.md) |
| 环境变量、下载器、目录与代理 | [配置参考](docs/CONFIGURATION.md) |
| 更新、备份与故障排查 | [维护指南](docs/MAINTENANCE.md) |
| 飞牛 FPK / Compose 安装 | [FPK 应用包](docs/FNOS_APP.md) · [Compose 图形安装](docs/FNOS.md) |
| Windows 桌面版 | [Windows 桌面版](docs/WINDOWS.md) |
| 自建聚合搜索服务 | [在线聚合搜索服务](docs/SEARCH_SERVER.md) |
| 源码开发与参与贡献 | [贡献指南](CONTRIBUTING.md) |

---

[MIT License](LICENSE) · 第三方软件保留各自许可，见 [THIRD-PARTY-NOTICES](THIRD-PARTY-NOTICES.md) · 请仅下载你有权访问和保存的内容。

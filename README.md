<div align="center">
  <img src="web/public/logo.svg" alt="Auto-Anime" width="360">
  <p><strong>让追番自动发生，让媒体库井井有条。</strong></p>
  <p>为飞牛 fnOS 与 Linux NAS 打造的自动追番服务：搜索、订阅、下载、归档、播放，一个容器搞定。</p>
  <p>
    <a href="https://github.com/Shizuku11451/auto-anime/releases"><img src="https://img.shields.io/github/v/release/Shizuku11451/auto-anime?color=0284c7" alt="GitHub Release"></a>
    <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-65a30d" alt="MIT License"></a>
  </p>
  <p>
    <a href="https://autoanime.top">官网文档</a> ·
    <a href="https://autoanime.top/guide/quick-start">快速开始</a> ·
    <a href="https://github.com/Shizuku11451/auto-anime/releases/latest">下载</a>
  </p>
</div>

<!-- TODO: 补一张番剧库截图，例如：
<p align="center"><img src="docs/images/library.png" alt="番剧库" width="860"></p>
-->

## 为什么用它

- **先选作品，再挑版本**：按 Bangumi 条目搜索，聚合蜜柑计划、动漫花园、Nyaa 等多个来源，按字幕组和清晰度挑好再订阅，不用手写 RSS 规则。
- **不用另装下载器**：内置 BT 下载器，重启自动恢复；也能接 qBittorrent、Aria2、Transmission。
- **归档即可被媒体服务器识别**：按 `番剧名/Season XX/` 整理并生成 NFO 与封面，Jellyfin、Emby、飞牛影视直接扫描；集齐全季自动停订。
- **自带番剧库**：按星期、完结、收藏筛选，同步 Bangumi 收藏，浏览器直接播放，适配手机。

## 平台适配范围

| 平台 | 安装方式 |
| --- | --- |
| **飞牛 fnOS**（x86-64，主要维护） | [FPK 应用包](https://autoanime.top/install/fnos-app)（推荐）· [Docker Compose](https://autoanime.top/install/fnos-compose) |
| **Linux**（amd64 / arm64） | [Docker Compose / 独立程序](https://autoanime.top/install/docker) |

## 快速安装

**飞牛 fnOS**：从 [Release](https://github.com/Shizuku11451/auto-anime/releases/latest) 下载 `.fpk`，在「应用中心 → 手动安装」导入（需先装好 Docker）。

**Docker Compose**：

```bash
mkdir auto-anime && cd auto-anime
curl -O https://raw.githubusercontent.com/Shizuku11451/auto-anime/main/docker-compose.yml
docker compose up -d
```

打开 `http://NAS地址:8000`，创建管理员账户，按向导完成设置。挂载其他媒体目录、换端口、接外部下载器见[配置参考](https://autoanime.top/guide/configuration)。

## 使用须知

- **只在可信局域网使用**，不要把管理端口暴露到公网。
- 搜索默认使用公共聚合服务 `search.115415.xyz`，**你的 IP 和搜索的条目 ID 会发给该服务**。可以[自建](https://autoanime.top/advanced/search-server)，或设置 `AGGREGATE_SEARCH_URL=""` 关闭；关闭后 RSS 订阅和下载照常工作。
- 播放器不转码、不渲染外挂字幕，复杂格式请交给媒体服务器。
- 删除订阅默认保留媒体文件。已实现与未实现的功能见[功能边界](https://autoanime.top/guide/features)。

## 文档与贡献

使用、配置、维护和故障排查见 **[autoanime.top](https://autoanime.top)**，原文在 [`docs/`](docs/)。欢迎提交 [Issue](https://github.com/Shizuku11451/auto-anime/issues) 和 PR，开始前请读[贡献指南](https://autoanime.top/develop/contributing)。

---

[MIT License](LICENSE) · 第三方许可见 [THIRD-PARTY-NOTICES](THIRD-PARTY-NOTICES.md) · 请仅下载你有权访问和保存的内容。

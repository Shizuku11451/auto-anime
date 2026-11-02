---
layout: home

hero:
  name: Auto-Anime
  text: 让追番自动发生
  tagline: 为 NAS 打造的自动追番服务。番剧搜索、订阅追更、内置下载、季集归档、番剧库与播放。
  image:
    src: /favicon.svg
    alt: Auto-Anime
  actions:
    - theme: brand
      text: 快速开始
      link: /guide/quick-start
    - theme: alt
      text: 下载最新版本
      link: https://github.com/Shizuku11451/auto-anime/releases/latest
    - theme: alt
      text: GitHub
      link: https://github.com/Shizuku11451/auto-anime

features:
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m21 21-4.34-4.34"/><circle cx="11" cy="11" r="8"/></svg>'
    title: 先找作品，再挑版本
    details: 按 Bangumi 条目搜索番剧，聚合多个来源的候选资源，按字幕组、清晰度筛选后一键订阅追更。也支持手动 RSS 与 OPML 导入。
    link: /guide/usage
    linkText: 使用指南
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 15V3"/><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="m7 10 5 5 5-5"/></svg>'
    title: 下载不用另装软件
    details: 内置纯 Go 的 BT 下载器，也可接入 qBittorrent、Aria2、Transmission 或直链。排队、并发、暂停与继续，重启后自动恢复。
    link: /guide/configuration#下载模式
    linkText: 下载模式
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20 10a1 1 0 0 0 1-1V6a1 1 0 0 0-1-1h-2.5a1 1 0 0 1-.8-.4l-.9-1.2A1 1 0 0 0 15 3h-2a1 1 0 0 0-1 1v5a1 1 0 0 0 1 1Z"/><path d="M20 21a1 1 0 0 0 1-1v-3a1 1 0 0 0-1-1h-2.9a1 1 0 0 1-.88-.55l-.42-.85a1 1 0 0 0-.92-.6H13a1 1 0 0 0-1 1v5a1 1 0 0 0 1 1Z"/><path d="M3 5a2 2 0 0 0 2 2h3"/><path d="M3 3v13a2 2 0 0 0 2 2h3"/></svg>'
    title: 归档自动整理
    details: 识别番名与季集，按「番剧名/Season XX/」归档并生成 NFO 与封面，Jellyfin、Emby、飞牛影视可直接扫描。集齐全季后自动停订。
    link: /guide/configuration#存储位置与目录映射
    linkText: 存储与目录
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m17 2-5 5-5-5"/><rect width="20" height="15" x="2" y="7" rx="2"/></svg>'
    title: 一个页面追番
    details: 番剧库按星期、已完结、收藏状态筛选，同步 Bangumi 收藏，浏览器直接播放。中文界面，亮暗主题与手机布局。
    link: /guide/usage#番剧库
    linkText: 番剧库
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect width="20" height="8" x="2" y="2" rx="2" ry="2"/><rect width="20" height="8" x="2" y="14" rx="2" ry="2"/><line x1="6" x2="6.01" y1="6" y2="6"/><line x1="6" x2="6.01" y1="18" y2="18"/></svg>'
    title: 为 NAS 设计
    details: 飞牛 fnOS 为主要维护平台，提供自带镜像的 FPK 应用包；Linux amd64/arm64 Docker 镜像与 Windows 桌面版同步发布。
    link: /install/fnos-app
    linkText: 安装方式
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/><path d="m9 12 2 2 4-4"/></svg>'
    title: 安全可恢复
    details: 只在归档成功后标记完成；删除订阅默认保留媒体，迁移失败保护原文件；暂停任务跨重启保留进度。
    link: /guide/maintenance
    linkText: 维护与备份
---

<div class="home-section">

## 三步开始

```bash
git clone https://github.com/Shizuku11451/auto-anime.git
cd auto-anime
docker compose up -d
```

打开 `http://NAS地址:8000`，创建管理员账户，确认下载方式、保存目录和同步周期即可开始使用。飞牛用户推荐使用 [FPK 应用包](/install/fnos-app)，Windows 用户请看 [Windows 桌面版](/install/windows)。

## 平台

| 平台 | 安装方式 |
| --- | --- |
| **飞牛 fnOS** · 主要维护平台（x86-64） | [FPK 应用包](/install/fnos-app)（推荐）/ [Docker Compose](/install/fnos-compose) |
| **Linux** · amd64 / arm64 | [Docker Compose / 独立服务端程序](/install/docker) |
| **Windows 10/11** · x64 | [单文件程序或便携桌面 ZIP](/install/windows) |

</div>

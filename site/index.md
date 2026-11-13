---
layout: home

hero:
  name: Auto-Anime
  text: NAS 自动追番工具
  tagline: 搜索番剧，订阅更新，自动下载并整理到媒体库。
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

features:
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m21 21-4.34-4.34"/><circle cx="11" cy="11" r="8"/></svg>'
    title: 搜索与订阅
    details: 搜索番剧、选择版本，自动追更新集。支持 RSS 订阅与 Bangumi 收藏同步。
    link: /guide/usage
    linkText: 使用指南
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 15V3"/><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="m7 10 5 5 5-5"/></svg>'
    title: 自动下载
    details: 内置 BT 下载器，也可接入 qBittorrent、Aria2 或 Transmission。
    link: /guide/configuration#下载模式
    linkText: 下载模式
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20 10a1 1 0 0 0 1-1V6a1 1 0 0 0-1-1h-2.5a1 1 0 0 1-.8-.4l-.9-1.2A1 1 0 0 0 15 3h-2a1 1 0 0 0-1 1v5a1 1 0 0 0 1 1Z"/><path d="M20 21a1 1 0 0 0 1-1v-3a1 1 0 0 0-1-1h-2.9a1 1 0 0 1-.88-.55l-.42-.85a1 1 0 0 0-.92-.6H13a1 1 0 0 0-1 1v5a1 1 0 0 0 1 1Z"/><path d="M3 5a2 2 0 0 0 2 2h3"/><path d="M3 3v13a2 2 0 0 0 2 2h3"/></svg>'
    title: 归档与播放
    details: 按番名和季集整理文件，在浏览器播放，或接入 Jellyfin、Emby、飞牛影视。
    link: /guide/usage#番剧库
    linkText: 番剧库
---

<div class="home-section">

## 选择安装方式

| 平台 | 安装方式 |
| --- | --- |
| 飞牛 fnOS · x86-64 | [FPK 应用包](/install/fnos-app)（推荐） |
| Linux · amd64 / arm64 | [Docker / 独立程序](/install/docker) |

</div>

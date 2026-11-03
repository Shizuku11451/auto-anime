# 介绍

Auto-Anime 是一个为 NAS 打造的自动追番工具。订阅番剧后，自动下载新集并按番名、季集归档，可在浏览器播放，也可交给 Jellyfin、Emby 或飞牛影视管理。

## 主要功能

- 搜索番剧、筛选版本，支持 RSS / Atom 订阅与 OPML 导入。
- 内置 BT 下载器，也可接入 qBittorrent、Aria2 或 Transmission。
- 自动整理文件、生成封面与 NFO，支持手动纠正识别结果。
- 同步 Bangumi 收藏，按星期浏览番剧库。

支持飞牛 fnOS、Linux 和 Windows 10/11，安装方法见[快速开始](/guide/quick-start)。详细支持范围见[功能边界](/guide/features)。

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

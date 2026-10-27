<div align="center">
  <br>
  <img src="web/public/logo.svg" alt="Auto-Anime" width="420">
  <br>
  <br>
  <p><strong>让追番自动发生，让媒体库井井有条。</strong></p>
  <p>为 NAS 打造的 RSS 自动追番服务，使用 Go 与 Vue 构建。<br>
    RSS 订阅 · 自动下载 · 季集归档 · 番剧资料 · 浏览器播放</p>
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
    <td width="50%" valign="top"><strong>找番，不必来回切换</strong><br><br>配置自己的 RSS 搜索源后搜索番名，支持 RSS 订阅、关键词过滤与 OPML 导入。</td>
    <td width="50%" valign="top"><strong>下载，不必一直盯着</strong><br><br>Windows 与 Docker 都内置 BT 下载器，不需要额外装 qBittorrent；支持外部下载器与直链，排队、进度、暂停与继续。</td>
  </tr>
  <tr>
    <td width="50%" valign="top"><strong>归档，不必手动整理</strong><br><br>识别番名与季集，按季归档并生成 NFO 与封面，方便媒体服务扫描。</td>
    <td width="50%" valign="top"><strong>追番，一个页面就够</strong><br><br>番剧资料、更新日与浏览器播放；中文界面，支持亮暗主题与手机布局。</td>
  </tr>
</table>

应用页面资源随包提供，不依赖运行时公共 CDN；默认直连，元数据服务不可用不影响基础订阅与下载。

## 平台适配范围

| 平台 | 安装方式 |
| --- | --- |
| **飞牛 fnOS** · 主要维护平台 | [FPK 应用包](docs/FNOS_APP.md) / [Docker Compose](docs/FNOS.md) |
| **Linux** · amd64 / arm64 | Docker Compose / 独立服务端程序 |
| **Windows 10/11** · x64 | [单文件程序或便携桌面 ZIP](docs/WINDOWS.md)，需要 WebView2 Runtime |

其他 NAS 品牌与 macOS 暂不承诺专门适配。功能以所安装版本为准；构建通过不等于目标设备验收，安装前请查看[对应版本的发布说明与附件](https://github.com/Shizuku11451/auto-anime/releases)。

Windows 与飞牛桌面采用统一的圆角矩形星芒图标，网页横版 Logo 不变；已有安装需使用包含新图标的 Windows 程序或飞牛 FPK，单独更新容器镜像不会改变飞牛桌面图标。

## 快速安装

飞牛 FPK 自 v0.0.2 起自带 x86-64 镜像，仍需 Docker，安装阶段无需访问 GHCR。Windows 单文件已自带 BT 下载器；桌面包在解压目录下使用 `data` 保存数据、`media` 保存归档媒体。

飞牛推荐先看 [FPK 安装说明](docs/FNOS_APP.md) 或 [Compose 图形安装指南](docs/FNOS.md)。已有 Docker Compose 的环境可直接运行：

```bash
git clone https://github.com/Shizuku11451/auto-anime.git
cd auto-anime
docker compose pull
docker compose up -d
```

打开 `http://NAS地址:8000`，创建管理员账户并完成初始化。向导提供可选的 RSS 搜索模板配置；应用不内置或推荐资源来源，未配置时仍可手动添加 RSS。

Docker 与 Windows 默认都使用内置 BT 下载器；每 30 分钟同步、同时下载 2 个任务。Docker 数据保存在 `./docker-data`，容器内默认媒体目录为 `/data/downloads`。

内置 BT 任务按条目独立控制，即使种子相同也不会互相暂停或取消。默认 10 个 BT 端口限制同时驻留的任务数，不限制累计下载数量；端口不足时暂停任务和历史做种让出资源，源文件与续传信息仍保留，优先保障新下载。详见[下载器配置](docs/CONFIGURATION.md#内置-bt-下载器默认)。

内置 BT 的分片选择使用单次扫描，减少多分片种子的排序与内存分配开销；无需新增配置，并发、端口和缓存默认值不变。实际下载速度仍取决于可用做种者、网络与磁盘。

> **安装前留意**：需要能访问 GitHub / GHCR，且数据目录可写、端口可用。使用其他媒体目录时，先添加 volume 挂载，再填写容器内路径；仅修改网页路径不会增加挂载。详见[配置参考](docs/CONFIGURATION.md)。目标版本没有发布镜像时，请使用[源码构建](CONTRIBUTING.md#源码镜像构建)。

## 开始追番

1. **添加订阅** — 配置自己的搜索源后在「找番」搜索资源，或在「订阅管理」直接填写 RSS。
2. **确认规则** — 预览资源，核对季数、关键词和保存位置，再开启自动下载。
3. **查看进度** — 在「最近条目」查看任务；只有「已保存」表示归档完成。
4. **浏览播放** — 在「番剧库」管理与播放，或用自己的媒体服务扫描归档目录。

添加订阅时优先显示番剧归属、自动下载与匹配预览；关键词和季数收在「筛选规则」，RSS 与目录收在「来源与保存位置」。手动添加仍直接填写 RSS，默认目录与下载行为不变。

搜索模板需为可返回 RSS 的 HTTP(S) 地址，并包含 `{keyword}`（或 `{关键词}`），例如 `https://example.com/search/rss?term={keyword}`（仅示意，不是可用来源）。安装后可在「设置 → 网络与同步」修改或清空，已有订阅、历史条目与媒体不会因此删除。请自行确认来源的使用条款与内容授权；用户自行配置并不等于内容获得授权。

季数识别支持中文、英文、罗马数字及全角/分隔符写法。未写季号的版本可参考同一 RSS 内同名且至少两集重合的明确季号资源，显示「推测第 N 季」；推测仍需人工确认，默认关闭自动下载，不自动改写归档季号。

关键词过滤统一英文大小写、简繁文字与全角英数字（如 `ＡＩ` / `AI`、`１０８０ｐ` / `1080p`），不改写原始标题和已存规则；包含词仍须全部命中，排除词任一命中即排除。

番剧库采用带滑动选中效果的紧凑星期筛选条；桌面端将搜索、结果数量、排序与视图切换集中在同一工具栏，手机端自动换行。

请在可信局域网使用，不要直接暴露管理端口到公网。播放器不转码、不渲染外挂字幕；JSON 备份不包含历史任务或媒体。更多限制见[功能边界](docs/FEATURE_COMPARISON.md)。

## 下载任务自动清理

新归档的下载器任务在完成做种后自动移除，保留下载器源文件与媒体库文件。外部下载器沿用自身做种限制；内置 BT 在分享率达到 1 后移除任务。无限做种、未完成或无法确认身份的任务不会删除。原有「清理源文件」设置仍是独立的显式删除选择，详见[配置参考](docs/CONFIGURATION.md#做种与源文件清理)。

历史做种清理在活动任务轮询之后限时处理；下载器故障时保留登记、后续重试。混合 RSS 的合集和特别篇也参与同番同季校验，不据此误推断集数偏移；纠正搜索词后，旧自动匹配的总集数不再用于停订，手动绑定仍优先。

添加 RSS 可自动推断集数偏移：过滤后资源最小集数为 0 时偏移 +1、为 12 时偏移 -11，后续新增条目按偏移归档。预览显示推断结果，可在「筛选规则」手动纠正；请确认来源含起始集，滚动 RSS 的最小集数不一定是真正首集。已有订阅默认保持 0，已有文件不重编号。详见[集数偏移](docs/CONFIGURATION.md#集数偏移)。

Bangumi 提供有效正篇总集数后，同番剧、同季的第 1～N 集全部归档且文件存在时，自动禁用该订阅，卡片标记「已集齐」并显示原因。不同季单独查询，不改番剧库已有的封面绑定；未知总数、缺集、混合番剧/多季订阅不会误停。详见[集齐自动停订](docs/CONFIGURATION.md#集齐自动停订)。

可在「设置 → 网络与同步」使用 Bangumi 个人令牌连接账号，只读同步动画「想看」；在「番剧库」的想看列表中选择条目搜索自己的 RSS 来源。想看列表与本地番剧分开显示，不会自动订阅、下载或回写观看状态，详见[配置参考](docs/CONFIGURATION.md#bangumi-想看)。

## 更多文档

| 想了解 | 查看 |
| --- | --- |
| 找番、订阅、纠错与播放 | [使用指南](docs/USAGE.md) |
| 保存目录、下载器与代理 | [配置参考](docs/CONFIGURATION.md) |
| 更新、备份与故障处理 | [维护指南](docs/MAINTENANCE.md) |
| 源码开发与参与贡献 | [贡献指南](CONTRIBUTING.md) |

---

[MIT License](LICENSE) · 第三方软件保留各自许可 · 请仅下载有权访问与保存的内容。

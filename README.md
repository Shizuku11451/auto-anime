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
    <td width="50%" valign="top"><strong>搜索，先确认作品</strong><br><br>搜索 Bangumi 番剧条目，展开查看在线服务聚合的候选资源；支持手动 RSS 订阅、关键词过滤与 OPML 导入。</td>
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

打开 `http://NAS地址:8000`，创建管理员账户并完成初始化。向导确认下载方式、保存目录与同步周期；RSS 订阅在「订阅管理」手动添加。

Docker 与 Windows 默认都使用内置 BT 下载器；每 30 分钟同步、同时下载 2 个任务。Docker 数据保存在 `./docker-data`，容器内默认媒体目录为 `/data/downloads`。

内置 BT 任务按条目独立控制，即使种子相同也不会互相暂停或取消。默认 10 个 BT 端口限制同时驻留的任务数，不限制累计下载数量；端口不足时暂停任务和历史做种让出资源，源文件与续传信息仍保留，优先保障新下载。详见[下载器配置](docs/CONFIGURATION.md#内置-bt-下载器默认)。

内置 BT 的分片选择使用单次扫描，减少多分片种子的排序与内存分配开销；无需新增配置，并发、端口和缓存默认值不变。实际下载速度仍取决于可用做种者、网络与磁盘。

> **安装前留意**：需要能访问 GitHub / GHCR，且数据目录可写、端口可用。使用其他媒体目录时，先添加 volume 挂载，再填写容器内路径；仅修改网页路径不会增加挂载。详见[配置参考](docs/CONFIGURATION.md)。目标版本没有发布镜像时，请使用[源码构建](CONTRIBUTING.md#源码镜像构建)。

## 开始追番

1. **查找番剧** — 「搜索」先显示 Bangumi 条目；配置独立在线聚合服务后，展开条目查看多个来源的候选资源，点击「订阅此版本」确认后持续追更。也可在「订阅管理」直接填写 RSS。
2. **确认规则** — 预览资源，核对季数、关键词和保存位置，再开启自动下载。
3. **查看进度** — 在「最近条目」查看任务；只有「已保存」表示归档完成。
4. **浏览播放** — 在「番剧库」管理与播放，或用自己的媒体服务扫描归档目录。

添加订阅时优先显示番剧归属、自动下载与匹配预览；关键词和季数收在「筛选规则」，RSS 与目录收在「来源与保存位置」。手动添加仍直接填写 RSS，默认目录与下载行为不变。

旧的 RSS 搜索模式与模板设置已移除，已有订阅、历史条目及媒体保留。请自行确认来源的使用条款与内容授权；用户自行配置并不等于内容获得授权。

在线聚合服务独立部署，通过 Jackett 查询管理员明确启用且有权使用的来源。默认服务地址为 `https://search.115415.xyz`，不内置访问令牌或站点凭据；Windows 会读取程序同级 `secrets/aggregate-token.dpapi` 中由当前 Windows 用户加密的已有凭据，直接启动即可接入。其他机器或 NAS 仍须配置 `AGGREGATE_SEARCH_TOKEN`，自建服务可用 `AGGREGATE_SEARCH_URL` 覆盖地址，显式空地址可禁用。缺少凭据仍可搜索 Bangumi 和使用原有 RSS。候选种子可点击「订阅此版本」，核对匹配预览、自动下载和保存位置后确认，后续按现有同步周期通过在线服务追更，无需填写 RSS。订阅按解析番名、字幕组、清晰度、已识别的字幕语言和标题季号匹配，不自动验证 Bangumi 绑定；密钥不发到浏览器，既有 RSS 同步和下载不依赖该服务。部署、隐私与限制见[在线聚合搜索服务](docs/SEARCH_SERVER.md)。

聚合搜索达到每查询 100 条或合并 300 条的上限时，订阅仍同步已返回且符合规则的资源，不再误报同步失败；搜索和订阅预览保留截断提示，不保证历史资源齐全。来源查询失败、网络错误仍报告同步失败。

季数识别支持中文、英文、罗马数字及全角/分隔符写法。聚合订阅保留所选资源的标题季号条件；未标明季号的版本需人工核对，不根据其他候选推测归档季号。

关键词过滤统一英文大小写、简繁文字与全角英数字（如 `ＡＩ` / `AI`、`１０８０ｐ` / `1080p`），不改写原始标题和已存规则；包含词仍须全部命中，排除词任一命中即排除。

番剧库采用带滑动选中效果的紧凑星期筛选条；桌面端将搜索、结果数量、排序与视图切换集中在同一工具栏，手机端自动换行。

请在可信局域网使用，不要直接暴露管理端口到公网。播放器不转码、不渲染外挂字幕；JSON 备份不包含历史任务或媒体。更多限制见[功能边界](docs/FEATURE_COMPARISON.md)。

## 下载任务自动清理

新归档的下载器任务在完成做种后自动移除，保留下载器源文件与媒体库文件。外部下载器沿用自身做种限制；内置 BT 在分享率达到 1 后移除任务。无限做种、未完成或无法确认身份的任务不会删除。原有「清理源文件」设置仍是独立的显式删除选择，详见[配置参考](docs/CONFIGURATION.md#做种与源文件清理)。

历史做种清理在活动任务轮询之后限时处理；下载器故障时保留登记、后续重试。混合 RSS 的合集和特别篇也参与同番同季校验，不据此误推断集数偏移；纠正搜索词后，旧自动匹配的总集数不再用于停订，手动绑定仍优先。

添加 RSS 可自动推断集数偏移：过滤后资源最小集数为 0 时偏移 +1、为 12 时偏移 -11，后续新增条目按偏移归档。预览显示推断结果，可在「筛选规则」手动纠正；请确认来源含起始集，滚动 RSS 的最小集数不一定是真正首集。已有订阅默认保持 0，已有文件不重编号。详见[集数偏移](docs/CONFIGURATION.md#集数偏移)。

Bangumi 提供有效正篇总集数后，同番剧、同季的第 1～N 集全部归档且文件存在时，自动禁用该订阅，卡片标记「已集齐」并显示原因。不同季单独查询，不改番剧库已有的封面绑定；未知总数、缺集、混合番剧/多季订阅不会误停。详见[集齐自动停订](docs/CONFIGURATION.md#集齐自动停订)。

可在「设置 → 网络与同步」使用 Bangumi 个人令牌连接账号，单向只读同步动画的「想看、在看、看过、搁置、抛弃」收藏状态与封面。收藏直接显示在番剧库中，可按状态筛选；已绑定同一 Bangumi 条目的本地番剧只增加状态标签，不重复显示。点击番剧先展开详情，查看收藏状态与应用内关联的本地资源，再通过「找资源并选择下载」进入 Bangumi 搜索并查看聚合资源；确认版本后可订阅，打开详情本身不会自动订阅、下载或回写状态。移除收藏不删除本地番剧、订阅或媒体，详见[配置参考](docs/CONFIGURATION.md#bangumi-收藏同步)。

封面显示 Bangumi 社区评分（不是个人打分），无有效评分时不显示数字。展开详情按需获取简介、放送日期、正篇集数、评分人数和排名，并提供原条目链接；资料失败可单独重试，不影响本地资源查询和下载操作。

## 更多文档

| 想了解 | 查看 |
| --- | --- |
| 搜索、订阅、纠错与播放 | [使用指南](docs/USAGE.md) |
| 保存目录、下载器与代理 | [配置参考](docs/CONFIGURATION.md) |
| 更新、备份与故障处理 | [维护指南](docs/MAINTENANCE.md) |
| 源码开发与参与贡献 | [贡献指南](CONTRIBUTING.md) |

---

[MIT License](LICENSE) · 第三方软件保留各自许可 · 请仅下载有权访问与保存的内容。

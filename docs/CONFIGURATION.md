# 配置参考

[文档索引](README.md) · [使用指南](USAGE.md) · [维护与备份](MAINTENANCE.md)

## 配置方式与优先级

环境变量提供启动值，网页可编辑的对应设置保存到数据库后优先于环境变量。修改 Compose 环境变量需重建容器；仅重启不会更新环境变量，也不会覆盖数据库中的旧选择。

设置页统一保存下载器、媒体库、代理、同步周期、账户等设置。媒体库目录发生变化时先检测写入，失败整份设置不保存。下载目录、数据库路径及内置下载器端口等启动参数不在网页中修改。

首次向导沿用部署默认值；完整环境账户可跳过向导。代理输入留空沿用部署代理，关闭已有代理请在初始化后的设置页操作。通用、飞牛及 Windows 安装步骤分别见对应[安装文档](README.md)。

## 基础环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `DB_PATH` | Linux `/data/auto-anime.db`；Windows 程序同级 `data/auto-anime.db` | SQLite 路径，一个数据库只运行一个实例 |
| `DOWNLOAD_DIR` | 默认数据目录下 `downloads` | 默认下载/归档目录，媒体库留空时使用 |
| `LIBRARY_DIR` | Windows 程序同级 `media\`；其它平台空 | 媒体根目录（番剧文件夹的父目录）；飞牛模板显式设为 `/media`，Windows 清空界面设置时回退下载目录 |
| `COVERS_DIR` | 数据库同级 `covers` | 本地封面缓存 |
| `PORT` | `8000` | HTTP 监听端口；桌面模式仅回环，服务模式不限定回环 |
| `DOWNLOAD_MODE` | Windows `native`；其它源码 `direct`；Docker 显式 `builtin` | Windows 单文件默认内置 BT 下载器；桌面包可切换 `builtin` 回退，数据库值优先 |
| `POLL_MINUTES` | `30` | RSS 同步间隔，分钟，最小 5 |
| `DOWNLOADER_POLL_SECONDS` | `10` | 下载器轮询间隔，秒，最小 5 |
| `KEEP_SOURCE_FILES` | `true` | 归档成功后保留下载器源文件做种 |
| `PROXY_URL` | 空 | 显式出站代理，留空直连 |
| `BANGUMI_ENABLED` | `true` | 仅值为 `true` 时开启自动元数据获取；关闭可用 `false` |
| `BANGUMI_API` | `https://api.bgm.tv` | Bangumi API 根地址 |
| `WEB_USER` / `WEB_PASSWORD` | 空 | 同时配置启用认证，缺一项不算完整账户 |
| `WEBHOOK_URL` | 空 | 事件 JSON POST 接收地址 |
| `UPDATE_CHECK` | `1` | `0` / `false` / `off` / `no` 关闭联网版本检查和一键更新 |
| `UPDATE_SOURCE` | 空 | 默认发布仓库，也可填仓库标识、GitHub 地址或自建前缀 |
| `GITHUB_TOKEN` | 空 | 可选 GitHub API 令牌，提高额度或读取私有仓库 |
| `UPDATE_PROXY` | 空 | 更新流量专用代理，留空沿用 `PROXY_URL` |

Linux 源码运行同样默认 `/data`，不是 `backend/data`；普通用户开发应显式设置独立目录。Windows 可执行文件默认程序同级 `data`（媒体库默认同级的 `media`），`go run` / `go test` 退回当前工作目录。程序打印实际数据库、下载、封面与媒体库路径。早期盘根数据不自动迁移，继续使用需显式配置旧路径。Windows 上的存储位置只接受盘符路径或 UNC 共享，`/vol1/...` 这类容器路径会被拒绝，已在数据库里的旧值会在启动日志提示。

Docker Compose 专用的 `/install` 部署配置页只在容器/Linux 提供；Windows 上返回 404，设置页不显示入口。

## 下载模式

| 值 | 用途 |
| --- | --- |
| `native` | 进程内纯 Go BT 引擎；Windows 默认，无需 qBittorrent |
| `builtin` | 私有内置 qBittorrent 处理 BT；Docker/Linux 默认，Windows 可作为旧版回退 |
| `direct` | HTTP(S) 文件，磁力和种子进入等待下载器状态 |
| `qbittorrent` | 外部 qBittorrent WebUI API |
| `aria2` | 外部 Aria2 JSON-RPC |
| `transmission` | 外部 Transmission RPC |

切换默认模式只影响后续提交，已有任务按原下载器身份跟踪。内置与外部 qBittorrent 配置相互独立，不接管其他客户端任务；`native` 与 `builtin` 也使用独立任务身份，旧 `builtin` 任务不会自动迁移。

### 内置 BT 下载器（Windows 默认）

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `NATIVE_BT_DATA_DIR` | 数据库同级 `bt` | 会话库 `session.db` 与源文件根目录 |
| `NATIVE_BT_PORT` | `6881` | BT 监听起始端口（TCP） |
| `NATIVE_BT_PORT_RANGE` | `10` | 监听端口数量，1–100；限制同时驻留的 BT 任务数，不限制累计任务数 |
| `NATIVE_DHT_PORT` | 同 `NATIVE_BT_PORT` | DHT UDP 端口 |

Windows 未设置 `DOWNLOAD_MODE` 时默认 `native`。BT 流量直连，不套用 `PROXY_URL`；种子文件名会在开始写文件前校验，拒绝 `..`、反斜杠、盘符和 Windows 保留名。新任务源文件在 `data\bt\downloads\<infohash-条目ID>\`，相同种子的不同条目独立下载和控制；旧 `<infohash>` 目录继续兼容，不自动搬迁。归档后默认保留做种，关闭 `KEEP_SOURCE_FILES` 后释放会话状态，再按安全条件清理源文件，避免旧完成状态误用到重新创建的文件。

暂停保留任务身份与续传信息；取消/移除记录会释放会话和端口，但不删除源文件。端口不足时，已暂停任务和已归档的做种任务可卸载到 `session.db`，源文件不变；活动下载不会被挤出，超出容量的新任务等待端口。暂停任务继续时从会话恢复，不必重新访问种子 URL。被卸载的历史做种不会在本轮运行中自动抢回端口，下次启动会在容量允许时恢复。旧版若多个条目共用同一任务，独立暂停/继续会被拒绝并提示移除重复记录（保留媒体）；存在已暂停共用条目时，后台轮询也不会自动重启该任务。

### 内置 qBittorrent

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `QBITTORRENT_DATA_DIR` | 数据库同级 `qbittorrent` | 配置、私有凭据、会话与源文件；镜像显式 `/data/qbittorrent` |
| `QBITTORRENT_PATH` | 自动查找 | 显式可执行路径；Windows 优先程序同级 `qbittorrent/`，再查 PATH 的 `qbittorrent-nox` |
| `EMBEDDED_QBIT_PORT` | `8080` | 私有 WebUI，仅回环监听，Docker 无需映射 |
| `QBITTORRENT_BT_PORT` | `6881` | BT / DHT 端口，应与 TCP、UDP 映射一致 |

Windows 可使用 `qbittorrent.exe` 或 `qbittorrent-nox.exe`。找不到可执行文件时告警，当前 `builtin` 新任务临时改用 `native`，不改写旧任务归属；旧 `builtin` 任务仍需要 qBittorrent 才能继续跟踪。`profile/` 保存恢复配置，`downloads/` 保存源文件；凭据不进入设置响应或 JSON 备份。

运行中退出后按 2、4、8、16、32、60 秒退避恢复，连续运行 5 分钟重置退避。WebUI 就绪后给予旧任务 90 秒恢复等待，保留任务 ID、暂停与进度；超时不存在才报错，不自动重提。健康检查在内置下载器恢复过程中可能返回 503，管理页面仍可访问。Compose 为正常退出留出 45 秒。

### 外部下载器

| 下载器 | 地址与认证 | 下载器路径 → 本服务可读路径 |
| --- | --- | --- |
| qBittorrent | `QBIT_URL`、`QBIT_USER`、`QBIT_PASSWORD` | `QBIT_SAVE_PATH` → `QBIT_LOCAL_DIR` |
| Aria2 | `ARIA2_URL`、可选 `ARIA2_TOKEN` | `ARIA2_DIR` → `ARIA2_LOCAL_DIR` |
| Transmission | `TRANSMISSION_URL`、`TRANSMISSION_USER`、`TRANSMISSION_PASSWORD` | `TRANSMISSION_DOWNLOAD_DIR` → `TRANSMISSION_LOCAL_DIR` |

地址与认证默认空。在网页填写或通过环境配置。地址必须能从服务所在容器访问；容器的 `127.0.0.1` 不是另一个容器或 NAS。

两个容器必须挂载同一批文件，同名路径不足以证明映射正确。例如 NAS `/volume1/downloads` 在下载器中挂到 `/downloads`，在 Auto-Anime 中挂到 `/source-downloads`：

```yaml
# 添加到 Auto-Anime 原有 volumes 中
volumes:
  - ./docker-data:/data
  - /volume1/downloads:/source-downloads
# 添加到 environment 中
# QBIT_SAVE_PATH: /downloads
# QBIT_LOCAL_DIR: /source-downloads
```

归档媒体库与源目录映射是两回事。映射错误会使下载完成后无法归档；修正后再核对任务与文件。外部下载器的会话和源文件由用户另行持久化、备份。

## 存储位置与目录映射

实际归档优先使用番剧记录中的存储位置；没有指定时使用 `<LIBRARY_DIR>/<规范番名>`，媒体库留空则使用 `<DOWNLOAD_DIR>/<规范番名>`。

订阅保存位置填根目录，保存时转换成该番剧文件夹并写入番剧记录；番剧存储位置填完整番剧文件夹。同一番剧共用位置，因此不是“订阅路径始终优先于手动番剧设置”的两套独立层级。保存订阅且位置变化时可更新番剧位置；同步只填空，不覆盖现有设置。清空订阅位置只回收仍与该订阅旧落点相同的值，不清除其他手动位置。

所有标题路径经过安全规范化，不能用番名逃出所选目录。识别单集使用 `Season XX` 子目录；无明确单集编号保留原名或合集布局。改变默认根目录、订阅路径或番名不自动迁移旧媒体。

Docker 内填写容器路径，NAS 文件夹必须先映射。例如 `/volume1/media/anime:/media` 后网页填 `/media`。保存目录时会检查 overlay 容器挂载表，未映射则提示；读不到挂载表、Windows 或非 overlay 环境不能当作已经完成 NAS 挂载确认。挂载提示不替代写权限、剩余空间或实机下载检查。

首次向导仅检测部署根目录；登录后的设置页可检测填写的目录。检测创建、写入、同步并删除临时文件，不创建缺失目录，不添加挂载，也不移动媒体。

## 做种与源文件清理

`KEEP_SOURCE_FILES=true` 默认保留下载器源文件，再复制归档，请预留两份空间。设为 `false` 后，只在媒体归档及数据库提交成功后停止对应任务并清理本次普通源文件，不批量清理已有条目。直链没有独立做种副本，不受该开关影响。

清理队列持久化并重试；重新开启取消尚未处理的清理，不恢复已删文件。支持内置/外部 qBittorrent、Aria2、Transmission；外部模式需明确源目录映射。超出映射、与归档重叠、文件被替换、任务共用或连接发生变化时保护源文件。不会递归删除目录或控制文件，任务记录可能保留。关闭做种仍需满足归档时的空间需求。

## 网络与元数据

默认不读取系统代理，显式直连。`PROXY_URL` 支持 http、https、socks5、socks5h。

- 统一代理用于 RSS、HTTP(S) 文件/种子、HTTP tracker、Bangumi、Webhook 和更新；更新可用专用代理覆盖。
- 下载器 RPC / WebUI 与 UDP tracker 始终直连；BT 流量由下载器自己的设置决定。
- 代理凭据展示脱敏，保存未变地址保留凭据，更换地址不沿用旧凭据，清空恢复直连。
- 元数据不可达不阻塞基础下载；封面本地缓存，手动绑定与纠正优先于自动匹配。

资源可用性探测只看 tracker，未实现 DHT 探测，不保证资源一定可下载。管理页面的脚本、样式、图标不从公共 CDN 加载。

Windows 与飞牛桌面的圆角矩形应用图标随各自程序/FPK 提供，无需配置；不改变默认目录或网页横版 Logo。更换飞牛图标需要升级 FPK，而不是只更新容器镜像；Windows 需要完全退出旧程序再启动新版本，任务栏固定项可能需要重新固定以刷新缓存。

## 网页专有设置与接口兼容

| 设置 | 规则 |
| --- | --- |
| 最大同时下载数 | 默认 2，1–32，在最近条目的下载控制中修改 |
| 自定义搜索模板 | 含 `{keyword}`，清空后保留内置来源 |
| 自动清理空番 | 默认关闭，按页面提示使用，不等同删除媒体 |
| 管理员账户 | 修改用户名需同时设新密码；改密撤销会话 |
| 每周更新日 | 0 自动，1–7 手动；自动资料缓存 7 天 |
| 订阅季数条件 | `season_filter=0` 不限、`-1` 仅未标明、1–999 指定季；`allow_unknown_season=false` 默认 |

订阅季数条件与归档季号独立，旧库自动迁移为不限，旧备份缺字段按默认恢复。使用流程见[使用指南](USAGE.md)，不需要新增环境变量。

搜索读取有预算，不保证完整历史：普通 RSS 最多 20 页、1000 条；ACG.RIP 每页 30 条，20 页最多 600 条。季数扩展共享 60 秒、20 页、1000 条与 32 MiB，单页最多 8 MiB；原词最多 1 页，季标记变体各最多 3 页、各限时 12 秒，余量用于番名搜索。失败和达到上限时保留成功结果并提示不完整。

`GET /api/items` 默认仍返回最多 100 条数组；`page=1` 返回 `{items,next_before}`，下一页传 `before`，0 表示结束；`q`、`status`、`group` 用于服务端筛选。手动同步使用 `POST /api/sync/<订阅ID>`。

脚本可使用管理员 HTTP Basic Auth。登录与 Basic Auth 共用失败限流：连续 5 次失败锁定 15 分钟，429 表示限流；同一反向代理后可能共享额度。健康接口 `/api/health` 公开但不返回私人配置。新密码使用 bcrypt，旧哈希登录后升级；旧程序不支持新哈希，回退前恢复相应数据库备份。

Webhook 事件为 `new_item`、`download_submitted`、`download_saved`、`download_failed`，不等于多渠道通知中心或可靠事件队列。

## 更新源

默认源是项目 GitHub Release；发布工作流将默认仓库注入程序，fork 自行发布使用自己的仓库。本地可用 `UPDATE_SOURCE` 覆盖。发布程序启动后检查，之后每 6 小时复查；检查不可达不阻塞下载。

- **自建更新源**：把 `UPDATE_SOURCE` 设为前缀（例如 `https://dl.example.com/auto-anime`），服务读取 `<前缀>/latest.json`：
  ```json
  {
    "version": "1.2.0",
    "notes": "更新说明",
    "published_at": "2026-01-01T00:00:00Z",
    "html_url": "https://example.com/releases/1.2.0",
    "assets": {
      "linux-amd64": "https://dl.example.com/auto-anime/auto-anime-1.2.0-linux-amd64",
      "linux-arm64": "https://dl.example.com/auto-anime/auto-anime-1.2.0-linux-arm64",
      "sha256": "更新包的 SHA-256 十六进制值"
    }
  }
  ```
  也可以把 `sha256` 写成 `<更新包地址>.sha256`，内容按 `sha256sum` 的「哈希 文件名」格式。
- **校验**：优先使用更新源提供的 SHA-256；GitHub 源缺少校验文件时只接受直接来自配置仓库地址的下载，自建更新源没有校验值会拒绝更新。

  如果更新源已声明校验文件，但文件请求失败、内容无效或不包含当前平台附件的校验值，会中止更新并允许重试，不降级为无校验安装。

版本说明纯文本展示，浏览器可忽略某版本。不同部署的更新与备份操作见[维护指南](MAINTENANCE.md#版本更新)。

## 兼容旧部署

旧内置 Aria2 已移除，`EMBEDDED_ARIA2`、`EMBEDDED_ARIA2_PORT`、`ARIA2_PATH`、`ARIA2_BT_TRACKER` 不再生效，外部 Aria2 继续支持。旧配置、分片与媒体保留；能确认属于旧私有实例且不能继续运行的未完成任务标记失败，手动重试提交到当前下载器，不迁移 GID 或旧分片。

旧 Windows 盘根数据、Docker 数据卷和已有存储位置不会自动搬迁。升级与回退前停止实例并备份，不要以重新初始化代替数据恢复。

## Docker 部署的更新行为

此内容统一维护在[维护与备份](MAINTENANCE.md#版本更新)，包括非 root 飞牛运行副本、只读目录和 `dev` 构建限制。

## 维护者发布流程

见[维护者发布说明](RELEASING.md)，包含版本标签、匿名镜像验证与 FPK 分发条件。自 v0.0.2 起正式 FPK 自带同版本 x86-64 镜像，安装无需访问 GHCR，仍需预装飞牛 Docker；Compose 和手工在线包仍需拉取镜像。

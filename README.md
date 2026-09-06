# 自动追番

一个使用 Go 编写、适合 NAS Docker 部署的 RSS 资源收集器：定时读取 RSS、按关键词过滤并去重保存条目。

## 部署

编辑 `docker-compose.yml`，将 `./data:/data` 改为 NAS 的实际目录，例如 `/volume1/docker/auto-anime:/data`，然后运行：

```bash
docker compose up -d --build
```

容器提供 Docker healthcheck；NAS 的容器管理器显示 `healthy` 后再打开页面。`.dockerignore` 会排除参考代码、数据库和本地缓存，避免把它们发送到构建上下文。

访问 `http://NAS地址:8000`。RSS 源由你自行配置；程序不会访问代理服务。下载器联动、文件整理、TMDB/Bangumi 刮削和通知将按模块继续加入。

添加订阅前可以在弹窗中点击"预览 RSS"检查地址是否可访问和是否能解析出条目；预览最多显示 20 条，不会写入数据库。

访问 `/api/backup` 可以下载 JSON 备份，内容不包含下载器密码和 Webhook 地址；备份适合迁移订阅规则与条目元数据，已保存文件仍由 NAS volume 管理。

页面的“恢复订阅”只导入订阅规则，会按 RSS 地址去重更新，不会覆盖历史条目或删除下载文件。

页面的“导入 OPML”支持从其他 RSS 阅读器迁移订阅，递归读取嵌套目录中的 `xmlUrl`，不会导入历史条目或下载任务。

点击“保存并获取”后会立即执行首次同步，不必等待定时周期；同步失败时订阅仍会保留，并在订阅列表显示错误原因，之后可以编辑、刷新或暂停它。

条目可以移除记录；此操作只删除 SQLite 记录，不删除 NAS 上已保存的文件。

条目列表支持按番剧名、原始标题或字幕组关键词搜索，也可以通过 API 的 `status` 参数按下载状态过滤。条目状态区分"待下载、下载中、已发送到下载器、已保存、下载失败、等待下载器"：提交到下载器只代表任务已受理，轮询显示进度百分比，文件复制进归档目录后才标记"已保存"；归档失败会显示具体原因，问题解决后下一轮自动重试归档，无需重新下载。

容器重启时恢复尚未执行的排队任务，以及启用自动下载订阅中的待下载条目。暂停订阅不自动恢复；原来处于"下载中"的本地直链任务改为"下载失败"，可手动重试。已保存的任务不会重新提交。已发送到下载器、正在被跟踪的任务在重启后继续跟踪，不会丢失。直链重试重新下载，不续传旧的临时文件。一个数据库只运行一个服务实例。

可访问 `/api/health` 检查服务状态。RSS、直链下载、Aria2 和 qBittorrent 请求都使用显式直连传输，不读取系统代理设置。

页面为深色管理面板：左侧导航切换"最近条目"和"订阅管理"，侧栏底部显示当前下载器、同步周期和各状态条目数量；条目列表支持按状态筛选（数量统计来自全库）、关键词搜索，下载中的条目显示实时进度条，失败原因直接显示在条目上。添加和编辑订阅在弹窗中完成，删除等操作有确认提示，操作结果以右下角通知展示。样式和脚本随应用本地提供，不依赖公共 CDN。`/api/status` 遵循 Web 登录设置，返回全库统计（不受列表搜索或 100 条限制影响），不返回密码或资源地址；公开的 `/api/health` 仅返回数据库连接是否可用。Docker 健康状态为 unhealthy 本身不会触发 `restart: unless-stopped` 自动重启。

如果 NAS 端口对局域网外可见，建议设置 `WEB_USER` 和 `WEB_PASSWORD` 启用 Web Basic Auth；只设置其中一个不会启用认证，`/api/health` 始终保持公开供 Docker healthcheck 使用。

同步时会从资源标题中提取字幕组、集数与清晰度，保留原始标题；`/api/items` 同时返回 `anime_title`、`episode`、`quality`、`fansub_group`。这些字段来自本地标题规则，属于初步识别，不等同于 Bangumi/TMDB 的人工校验元数据。

直链模式会把资源放到 `/data/downloads/{番剧名}/` 下的临时目录中，下载完整后一次性提交归档，并生成 `info.json` 保存 RSS 和识别信息；中断或长度不完整的文件不会被标记为已保存。

归档同时生成与资源同名的 `.nfo` 文件，便于 Emby/Jellyfin 读取番剧名、集数、字幕组、清晰度和简介；这些信息只来自 RSS 和本地标题解析。

## 下载器联动（Aria2 / qBittorrent / Transmission）

BT 来源是番剧 RSS 的主流形式。设置 `DOWNLOAD_MODE=aria2`、`qbittorrent` 或 `transmission` 后，条目会自动提交给对应下载器并持续跟踪，完成后自动整理进 `DOWNLOAD_DIR`：

- **Aria2**：填写 `ARIA2_URL`（如 `http://NAS地址:6800/jsonrpc`）和可选的 `ARIA2_TOKEN`。HTTP(S) 直链走 `addUri`；`.torrent` 链接由本程序读取后通过 `addTorrent` 提交，避免把种子文件本身当成下载内容。
- **qBittorrent**：填写 `QBIT_URL`、`QBIT_USER`、`QBIT_PASSWORD`，密码只从环境变量读取。磁力链接直接提取 infohash 跟踪；种子链接通过 `auto-anime-<条目ID>` 标签发现任务（需要 qBittorrent 4.4+）。
- **Transmission**：填写 `TRANSMISSION_URL`、`TRANSMISSION_USER`、`TRANSMISSION_PASSWORD`，程序自动处理 RPC session ID 并持久化任务 ID。

**路径映射是必须理解的配置**：下载器把文件存放在下载器容器自己的目录里，自动追番容器必须能读到同一个位置才能归档。`QBIT_SAVE_PATH`、`ARIA2_DIR`、`TRANSMISSION_DOWNLOAD_DIR` 描述下载器一侧的路径；`ARIA2_LOCAL_DIR`、`QBIT_LOCAL_DIR`、`TRANSMISSION_LOCAL_DIR` 描述同一目录映射到本容器内的路径。两个容器必须把同一 NAS 目录分别挂载到这两个路径上，只配置下载器一侧会导致归档一直失败并在条目上显示原因。两侧路径相同时可以不填 `*_LOCAL_DIR`。

下载完成后程序把文件**复制**进 `{DOWNLOAD_DIR}/{番剧名}/` 并生成 `info.json` 与 `.nfo`，下载器中的任务和文件保持原样继续做种，因此不会破坏做种；占用空间会翻倍，请规划好 NAS 容量。归档完成后条目标记"已保存"，下载器中的任务不会被自动删除，是否停止做种由你在下载器中决定。

`DOWNLOADER_POLL_SECONDS` 可选（默认 30，最小 5），控制任务进度轮询间隔。任务在下载器中被删除或报错时，条目会标记"下载失败"并显示原因；手动重试会重新提交一个新任务，不会清理下载器中的旧任务。

直链模式（`DOWNLOAD_MODE=direct`）不执行 BT 下载：磁力链接和 `.torrent` 链接会标记为"等待下载器"并说明原因，不会被误当作已保存的视频。

`WEBHOOK_URL` 可选。配置后会以 JSON POST 发送 `new_item`、`download_submitted`、`download_saved` 和 `download_failed` 事件，适合接入 NAS 内网脚本或通知服务；不配置则不会发出通知。

## 说明

仅使用合法、公开授权的 RSS 内容和下载来源。若 RSS 地址在国内网络不可达，程序无法绕过网络限制，应更换为 NAS 可访问的来源。

## 许可证

本项目基于 [MIT License](LICENSE) 开源发布。

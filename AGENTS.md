# 项目协作说明

## 项目目标

- 使用 Go 开发适合 NAS 的 RSS 自动追番服务，通过 Docker 部署。
- 核心流程：订阅 RSS/Atom、获取条目、识别番剧信息、过滤去重、下载、整理到 NAS 文件夹、展示状态。
- 面向中国大陆网络，默认直接连接，不依赖代理、境外 CDN 或必须访问境外服务才能完成的初始化。
- 除“开发进度快照”外，本文件描述后续开发约束，不代表其中提及的功能已经实现；实际能力以代码和验证结果为准。

## 开发进度快照（2026-09-09）

当前阶段：外部下载器任务跟踪闭环已打通，并用真实 RSS（anibt.net）+ 本机 Aria2 完成端到端实测（识别 → BT 下载 → 归档 → 可播放 → 集数正确）。以下记录来自当前源码及本轮实际验证，后续完成相关工作时应更新本节，不要直接将规划项标记完成。

### 番剧库管理（2026-09-09）

- 提交前审查修复：番剧改名的名称、资源、元数据、订阅归属及配置更新统一事务提交，任一步失败完整回滚；`TestAnimeRenameRollsBackAllAssociations` 用订阅更新触发器复现并验证。前端操作后刷新等待旧轮询结束再重新读取，避免沿用操作前快照；Edge 延迟旧轮询响应验证新建番剧在下一次定时轮询前显示。全量 Go 测试、vet、构建、前端构建及订阅过滤测试通过。

- 番剧库增加新建、卡片直接编辑/合并/删除、批量选择（含全选当前筛选结果）、范围筛选和排序；详情页同样提供合并与删除。新建复用编辑表单，默认季号为 1，避免被空番剧自动清理；同名创建返回 409，不覆盖已有配置。
- 新增 `anime_manage.go`：`POST /api/animes` 新建，`POST /api/animes/manage` 按 `action=merge|delete` 处理最多 500 个来源 ID。整批事务提交或回滚；合并保留有效季号/集数/文件路径，资源标为手动归属，定向订阅改指目标，目标配置保持原样。来源番剧配置/元数据及旧存储记录被删除。删除含资源需显式 `delete_items=true`，并暂停、解绑定向订阅；旧 DELETE 接口仍拒绝删除有资源的番剧。
- 管理操作不删除或迁移磁盘文件、不重写已有 NFO、不取消外部下载器任务；存在排队/下载中资源时拒绝操作。管理写锁与同步/纠错/恢复及下载入队协作，外部下载器应用轮询结果时校验活动任务，兼容 qBittorrent 首次哈希发现。普通非定向订阅仍可能重新收录被删除的资源。
- 验证：`anime_manage_test.go` 覆盖创建校验/重名/自动清理保留、合并有效季号/手动标记/订阅与目标配置、删除媒体保留、整批 SQL 失败回滚、活动任务与非法批次拒绝、过期下载器结果；Go 全量测试、vet、构建及前端构建通过。8087 隔离数据库 + 本地 RSS/媒体 + Edge 实测新建/编辑/重复名称、两部番剧合并、取消/确认删除与批量删除，验证归档媒体内容未变、季号与订阅归属正确；1280/768/390/320 宽度亮暗主题无横向越界，手机合并/删除弹窗正常，无页面脚本错误。未修改用户运行数据库，未做 Docker/NAS 实机验证。

### 找番订阅范围修复（2026-09-09）

- 新增可选空番剧清理（默认关闭）：设置键 `auto_cleanup_empty_anime`，位于设置页「媒体库」，保存启用后立即清理，启动与每分钟自动检查；从数据库读取开关，无额外环境变量。`anime_cleanup.go` 事务删除无资源（同时查 ID/旧标题关联）、无订阅 target_title 关联（含暂停）、无 manual 元数据/bangumi_id/搜索名/季号/存储目录/包含排除规则/旧 anime_storage 的空记录及对应自动元数据，不操作文件。共享读写锁保护同步首集入库、纠错、番剧编辑/绑定、旧存储接口和恢复期间的多条 SQL，清理失败回滚并记录日志。设置保存返回清理数量或重试提示，前端刷新番剧库。
- 清理验证：`anime_cleanup_test.go` 覆盖默认关闭、启停/省略/非法设置、立即清理、关联与各配置保留、重复执行、解除关联后清理、事务失败回滚、媒体不变与首资源并发保护。Go 全量测试、vet、构建和前端构建通过；隔离临时实例 + Edge 验证默认关闭、开启即清除一部空番剧、暂停订阅/季号配置保留、刷新持久化、关闭后保留新空记录及 1280/390 亮暗主题，无页面错误。未对用户当前数据库启用清理，未做 Docker/NAS 实机验证。

- 后续订阅卡片布局修复：`FeedCard.vue` 内部网格明确使用 `minmax(0, 1fr)`，卡片允许收缩，状态徽章不压缩；长标题/地址/归属/错误/关键词在卡片内截断并可查看完整提示。`FeedsView.vue` 网格最小宽度受容器限制，避免窄屏横向溢出。前端构建通过；本地模拟三种订阅状态，在 Edge 的 1280/768/390/320 宽度、亮暗主题下检查长中日英标题、长 URL 和关键词均无越界，编辑弹窗保留完整名称。纯前端变更，未修改后端。

- 原因：逐条「添加订阅」实际订阅同一个搜索 RSS，只预填搜索词，未限制所选字幕组和版本；标题精确建档又把不同别名/标签拆成多张卡片。
- `web/src/utils/discover.ts` 与 `DiscoverView.vue` 按清理后的番名、字幕组、清晰度和已识别语言折叠多集；移除订阅全部结果入口，选择版本后预填过滤条件；相同 RSS 已存在时打开原订阅调整，不新增重复订阅。
- `FeedDialog.vue` 自动预览过滤结果；修改规则后原预览失效，找番入口在预览失败或零匹配时不能保存。`/api/preview` 复用同步的包含/排除逻辑，先过滤再截取前 20 条，返回总数和匹配数。
- `feeds.target_title` 保存显式的「归入番剧」，同步时优先于多来源规则统一后续条目归属；空值保留旧自动识别逻辑，编辑省略字段保留原值，番剧改名同步更新引用，备份 v3 增加可选字段并兼容旧备份。已存在的资源不会因调整归属额外创建空番剧。未合并或删除用户已有记录/文件，未实现同集择优下载或替换，仍按资源 URL 去重。
- 验证：新增 `discover_subscription_test.go` 覆盖过滤先于截断、定向同步/同番别名归一/重复同步、归属备份恢复与编辑省略/清空；前端 `node --test web/src/utils/discover.test.mjs` 三项通过。`gofmt`、`CGO_ENABLED=0 go test ./... -count=1 -timeout=240s`、`go vet ./...`、Go 构建与 `web/npm run build` 通过。隔离临时数据库 + 本地 RSS + Edge 浏览器实测：5 条资源显示 4 个版本，选定版本只收录 07/08 两集且只有 1 部番剧；预览失效/零匹配禁止保存、已有订阅编辑、空结果/搜索失败、1280/390 视口亮暗主题与手机弹窗均通过，无页面脚本错误。未运行真实资源站下载、Docker 或 NAS 验证。

### 已实现

| 模块 | 当前能力 | 实现位置 |
| --- | --- | --- |
| 订阅管理 | 添加、编辑名称和关键词、暂停/启用、删除、RSS 预览、手动和定时同步；页面添加后触发首次同步；同步条目写入错误会记录到订阅错误信息 | `main.go`、`web/src/views/FeedsView.vue`、`web/src/dialogs/FeedDialog.vue` |
| RSS 解析 | RSS、RSS 1.0/RDF、Atom；优先 enclosure，支持相对资源链接 | `main.go` |
| 过滤去重 | 包含关键词全部匹配、排除关键词任一匹配；按资源 URL 去重 | `main.go` |
| 番剧识别 | 从标题提取字幕组、集数和清晰度：支持 `[07]`、`S01E09`、`- 09`、`第08话`、`[09v2]`；季标记（第三季/S01/3期）不误判为集数；已用 anibt RSS 50 条真实标题验证 | `main.go` |
| 封面识别 | 后台 worker 按识别标题查询 Bangumi（名称双向包含 + 长度下限的启发式匹配，先剥离 `[标签]`、拆分 `中文名 / Romaji`）；封面下载到 `COVERS_DIR`（内容寻址文件名、10MB 上限、仅 bgm.tv 域名），浏览器只加载本地副本 `/api/covers/{hash}`，并镜像为番剧根目录 `poster.jpg`（扩展名随封面格式，覆盖元数据/解绑后同步更新或清除，失败只记日志）；无匹配记负缓存一周后重试，网络失败不写记录下次重试；查询间隔 0.5 秒，失败不阻塞同步下载 | `meta.go`、`archive.go`、`main.go`、`static/` |
| 直链下载 | 自动或手动下载 HTTP/HTTPS 资源，限制两个同时下载；临时目录写入后提交归档；`.torrent`/磁力不再被误存为视频 | `main.go`、`archive.go` |
| 媒体库归档 | 一部番一个文件夹：识别到集数的资源按 `{存储位置}/{番剧名}/Season NN/{番剧名} - SxxEyy.ext` 归档（集数补零、同名 `.nfo` 与 `-info.json`、番剧根目录 `tvshow.nfo`），无集数资源放番剧根目录保留原始文件名，BT 多文件资源放 `番剧名 - SxxEyy/` 集文件夹且最大的视频文件原位重命名为 `番剧名 - SxxEyy`（飞牛影视等按文件名识别集数；纠错迁移时集文件夹与主视频同步改名），字幕等附属文件保留原名；同番不同字幕组落到同一目录，重名冲突自动加 ` (n)`；归档根目录优先级 = 番剧 `storage_dir` > 媒体库目录 `library_dir`（settings 键/env `LIBRARY_DIR`）> `DOWNLOAD_DIR`；存储目录与季号存在 `animes` 行上（旧 `anime_storage` 启动时迁入并清空），条目级季号覆盖番剧默认值；旧接口 `/api/anime/{title}/storage` 兼容读写 animes；配置随 v3 备份导出/恢复 | `archive.go`、`anime.go`、`main.go`、`static/` |
| 番剧库 | `animes` 一等实体统一番剧/季/集/资源：识别标题自动入库、items 以 `anime_id` 关联；番剧库列表（封面、资源/集数统计）与详情（按季分组、同集聚合多来源版本、来源订阅列表、缺号排序）；编辑支持改名（级联 items 与 `anime_meta`）、默认季号、存储位置；多来源包含/排除规则：同步时按原始标题归并、`/apply-rules` 回溯收编现有条目（`manual=1` 的纠错条目不动）；元数据绑定：`/api/meta/search` 搜 Bangumi 候选、`PUT /api/animes/{id}/metadata` 按 ID 直连 v0 接口绑定/解绑（source=manual，自动查找不覆盖），`tvshow.nfo` 写入 `<uniqueid type="bangumi">`；条目纠错 `PATCH /api/items/{id}` 改判番剧（可新建）/季/集并置 `manual=1`，`rearchive` 把已保存文件（单文件或多文件集目录）迁移到新位置（先准备目标文件与 NFO/JSON、条件更新记录后清理旧文件，失败保留源文件）；备份 v3 含 `animes`，恢复兼容 v1/v2/v3 | `anime.go`、`archive.go`、`meta.go`、`main.go`、`static/` |
| 外部下载器跟踪 | Aria2/qBittorrent/Transmission 提交并持久化任务 ID（aria2 GID、qbit infohash+标签发现、Transmission ID）；轮询进度与错误；`*_DIR`→`*_LOCAL_DIR` 路径映射；完成后复制归档（不影响做种，aria2 做种中即归档）；任务被删/报错标记失败并显示原因 | `downloader.go`、`main.go` |
| 种子语义 | 直链模式下磁力与 `.torrent` 链接标记"等待下载器"并显示原因；aria2 对 `.torrent` 链接自动改用 `addTorrent`（含 bencode infohash 计算） | `main.go`、`downloader.go` |
| 重启恢复 | 重启后恢复符合条件的排队/自动下载条目；带任务 ID 的下载器任务继续跟踪；无任务 ID 的"下载中"改为失败待手动重试 | `recovery.go` |
| 数据迁移 | JSON 导出订阅及条目元数据；恢复只按 URL 合并订阅；支持嵌套 OPML 导入 | `main.go`、`backup_test.go` |
| 运行信息 | 条目搜索、API 状态筛选、全库状态统计、公开健康检查；管理面板：Vue 3 单页应用（hash 路由），token 化亮暗双主题、Lucide 图标（`@lucide/vue` 组件随构建产物本地打包，禁表情符号）、左侧导航（手机端为顶栏 + 底部标签栏）、状态筛选徽章、下载进度条、失败原因展示、弹窗（含右上角关闭、Esc 只关最上层）与通知交互、页面/列表/弹窗过渡动画（`prefers-reduced-motion` 降级）、亮/暗主题切换（手动选择优先，否则跟随系统，存浏览器本地） | `status.go`、`main.go`、`web/` |
| 账户登录 | `WEB_USER` 与 `WEB_PASSWORD` 齐备时启用：浏览器走 `/login` 登录页（沿用亮暗主题），登录成功发 HttpOnly、SameSite=Lax 会话 Cookie（30 天）；`web_sessions` 表只存令牌 SHA-256 哈希，注销即时作废，启动清理过期会话；`/api/login` 按来源 IP 限速（连续 5 次失败锁 15 分钟）；脚本仍可用 Basic Auth；API 未认证返回 401（不带 WWW-Authenticate），页面请求 302 到 `/login?next=`（仅站内路径）；`/api/health`、`/static/`、登录页保持公开；未配置或只配置其一不启用 | `auth.go`、`main.go`、`web/src/login/`、`web/` |
| 出站代理 | `PROXY_URL`（默认空=显式直连）支持 http/https/socks5/socks5h（无前缀按 http，凭据写在 URL）；代理作用于 RSS 同步/预览、直链下载、`.torrent` 字节下载（新增 `torrentClient`）、Bangumi、Webhook；下载器 RPC（`downloaderClient`/`qbitClient`）始终直连；`newTransport()`/`newClient()` 统一传输配置，日志与 `/api/status` 只输出脱敏地址；无效配置启动即失败并给出原因 | `proxy.go`、`main.go`、`meta.go`、`archive.go`、`downloader.go` |
| 用户设置与引导 | `settings` 表 KV 存用户自定义配置，数据库值优先、env 回退，保存后热更新全局变量（无需重启）；可改：管理员账户（盐值哈希 `v1$salt$hash` 存库，页面只能重设不能查看）、下载模式、三个下载器的地址/凭据/路径映射、出站代理、同步周期、媒体库目录 `library_dir`（绝对路径校验，清空回退 `DOWNLOAD_DIR`）；首次安装（无凭据且未跳过引导）强制进入 `/setup` 引导页（`/api/setup` 初始化后 403 锁定，可跳过）；设置页在侧栏「设置」视图；敏感字段接口只回传 `*_set` 布尔不回明文 | `settings.go`、`auth.go`、`main.go`、`static/` |
| 找番搜索 | 侧栏「找番」：`GET /api/discover/sources` 列出内置与自定义搜索源；`GET /api/discover/search` 按关键词拉取资源站搜索 RSS（20 秒超时、8MB 响应上限、最多 60 条、资源页与种子链接分开保留、相对链接补全），逐条复用标题识别（番剧名/字幕组/集数/清晰度）；内置 ACG.RIP（2026-09-07 实测可国内直连匿名搜索；蜜柑计划/动漫花园的匿名搜索当时不可用，未内置）；设置页可配自定义 RSS 搜索模板（`{keyword}`/`{关键词}` 占位符、http(s) 与主机校验，存 settings 表热生效、清空即回退）；前端结果列表一键预填订阅弹窗（名称=识别番剧名、包含=搜索词），摘要条展示并复制本次搜索的 RSS 地址；搜索走统一出站代理配置 | `discover.go`、`settings.go`、`main.go`、`static/` |
| 基础运维 | Webhook 事件、Dockerfile（已复制 go.sum）、Compose、健康检查及 `/data` 持久化 | `main.go`、部署文件 |

### 修复与验证（2026-09-08）

- 修复七类问题：迁移中途失败导致记录指向已移走文件；设置页未改用户名仍要求密码及失败部分生效；保存脱敏代理地址丢失认证；普通保存重开已跳过的安装引导；改密未注销旧会话；`7/07/007` 同集拆组；NFO/JSON 未采用番剧默认季号。
- 设置先完整校验，再事务写入配置与会话撤销，提交后热更新；用户名不变可空密码保存，更改密码会要求重新登录。`setup_done` 只在明确提供时更新。代理地址未变时保留凭据，新增显式清除选项，换地址不继承旧凭据。
- 迁移拆入 `archive_migration.go`：保留源文件，准备目标媒体与元数据，再以原记录条件更新数据库，成功后清理原文件；元数据冲突或数据库失败回滚本次目标文件。嵌套 BT 资源按归档清单识别并迁移全部内容，重写 anime_id、文件路径及有效季号；清理失败向页面返回提示。进程崩溃仍可能留下重复目标副本，未实现持久化迁移日志与自动清理。
- 新增 `settings_regression_test.go`、`archive_migration_test.go`、`anime_episode_test.go`，覆盖非法输入、SQL 失败事务回滚、两个旧会话失效、代理凭据往返/清除、初始化保持、迁移元数据冲突/DB 失败/嵌套目录重名/重复迁移、默认与覆盖季号、数字集数聚合且保留资源原始编号。已通过 `gofmt`、`CGO_ENABLED=0 go test ./... -count=1 -timeout=240s`、`go vet ./...`、`go build`；符号链接测试因 Windows 缺少创建权限而跳过，未作真实跨设备、Docker、NAS 验证。
- 浏览器使用 8058 端口隔离实例（独立临时数据库和目录、关闭 Bangumi）：登录、空番剧库、设置页及新增代理提示正常；未重填密码修改同步周期后提示保存成功，运行卡片立即更新且登录保留。已通过 `node --check static/app.js`；本轮未完成手机视口、亮暗主题、页面改密跳转和迁移交互的浏览器复测，对应后端行为由上述回归测试覆盖。

### 飞牛影视（fnOS）识别支持（2026-09-09）

- 调研结论（公开资料：飞牛官方帮助 `help.fnnas.com` 影视篇、官方论坛 `club.fnnas.com`、知乎《飞牛影视刮削的文件命名技巧V2》）：飞牛影视刮削匹配**只依据文件名**，与文件夹名和目录层级无关；推荐结构 `剧名/Season 01/剧名.S01E01.mkv`，季文件夹须全称 `Season 01`（不可缩写 S01），OVA/特别篇放 `Season 00`；媒体库只能建在一个根文件夹上；支持「优先读取本地 NFO 和图片」（「其他视频」类型只读本地 NFO 不联网刮削），本地图片按 Kodi 惯例读番剧根 `poster.jpg`。
- 据此四项改动：① 新增全局「媒体库目录」：settings 键 `library_dir`、env `LIBRARY_DIR` 回退、设置页「下载方式」分组底部字段、`/api/settings` 读写、`/api/status` 与运行卡片显示；归档根优先级 = 番剧 `storage_dir` > `library_dir` > `DOWNLOAD_DIR`，校验复用 `sanitizeStorageDir`（绝对路径、拒绝 `..`，错误文案改写为「媒体库目录…」）。② BT 多文件资源归档时把最大的视频文件在种子目录内原位重命名为 `番剧名 - SxxEyy`（`pickMainVideo` 按扩展名+大小选取，字幕等附属文件保留原名、保持与主视频同目录相邻），`items.file_path` 与 `info.json` 的 `file` 记录改名后的相对路径；纠错迁移（`rearchiveSavedItem` 多文件分支）同步把主视频改名为目标 `finalBase`，迁移识别按清单匹配不受影响。③ 番剧根目录自动镜像封面为 `poster.jpg`（扩展名随封面格式 png/webp/gif）：`refreshSeriesPoster` 挂在 `writeTvshowNFO` 尾部（归档/改名/纠错迁移/绑定/解绑均触发）并在 `storeMetaSubject` 成功后调用；无封面或解绑后移除全部 `poster.*`；跨设备回退复制，失败只记日志。④ README 新增「飞牛影视（fnOS）接入步骤」与媒体库结构说明，docker-compose.yml 增加 `LIBRARY_DIR: ""`。
- 验证：`gofmt` 等价校验（仓库为 CRLF 检出，用「去 CR 后 gofmt 比对」方式验证改动文件，未跑 `gofmt -w` 以避免整文件行尾重写）、`go vet ./...`、`go build`、`CGO_ENABLED=0 go test ./... -count=1 -timeout=240s` 全部通过；新增 `TestArchiveTargetUsesLibraryDir`（优先级/覆盖/回退）、`TestSeriesPosterMirrorsCover`（镜像与过期清除）、`TestLibraryDirSetting`（保存/非法拒绝/清空回退/重启恢复/GET 回显），并按新布局更新 `TestMultiFileResourceLandsInEpisodeFolder`（断言主视频改名 + 迁移识别）与 `TestNestedMigrationRetainsAllFilesAcrossConflicts`（内容级保留 + 改名断言，兼容冲突后缀 ` (2)`）。`node --check static/app.js` 通过；`docker compose config` 未重跑（本机无 docker，仅向 compose 增加一个空环境变量行）。
- 浏览器端到端（8068 隔离实例 + 本地 RSS/媒体/假 Bangumi 服务，真实 lain.bgm.tv 封面）：登录→设置页「媒体库目录」回填与「当前下载目录」提示；保存后运行卡片立即更新；相对路径保存被拒并提示「媒体库目录必须是绝对路径」；第 07 话归档到默认 `downloads/番名/Season 01/番名 - S01E07.mp4`，封面识别成功后番剧根出现 216KB `poster.jpg` 与含译名的 `tvshow.nfo`；改媒体库目录后同步的第 08/09 话落到新根目录（`番名/Season 01/番名 - S01E0x.mp4` + NFO/JSON + poster），旧根目录已有文件保持不动；实例重启后 `library_dir` 从数据库恢复。过程中发现并修复：媒体库目录字段最初被误放进 Transmission 专属显隐区块（非 transmission 模式下不可见），已移到分组底部并验证切换下载模式时始终可见。测试坑：Git Bash 控制台 GBK 编码会污染 curl `-d` 中的中文（落库成替换符），中文路径验证需经浏览器 UI 或 UTF-8 文件提交。
- 未验证：真实飞牛 NAS 实例上创建媒体库并确认识别/刮削效果（本机无 fnOS）；BT 多文件归档的浏览器端到端（仅 Go 测试覆盖）；Docker 容器内运行；亮色主题与手机视口本轮未复测（沿用既有 token 化组件，无新增样式类）。

### 前端重构为 Vue 3 + Vite（2026-09-09）

- 范围：新增 `web/` 前端工程（Vue 3.5、Vite 8、TypeScript 6、vue-router 5 hash 路由、@vueuse/core、@lucide/vue），`npm run build` 先跑 `vue-tsc` 类型检查再把三个入口（`index.html`/`login.html`/`setup.html`）与哈希命名的 `assets/` 输出到 `static/`；Go 代码、Dockerfile、docker-compose、start-dev.bat 零改动（服务端仍按 `static/index.html`、`static/login.html`、`static/setup.html` 与 `/static/` 前缀提供文件）。旧的 `static/app.js`、`style.css`、`icons.svg` 已删除，`favicon.svg` 由 `web/public/` 复制产出。构建产物随仓库提交，容器构建与 NAS 部署不需要 Node；`.dockerignore` 排除 `web/node_modules/`。
- 结构：`src/api`（与 Go json tag 对应的类型、fetch 封装，401 只跳转一次并把 hash 路由带回登录页）、`src/stores`（全局数据 5 秒轮询且页面不可见时暂停、主题、toast、确认框、弹窗状态、找番状态）、`src/components/ui`（按钮、弹窗、徽章、空状态、封面、进度条、骨架屏、数字滚动、搜索框等）、`src/components/layout`（侧栏、运行状态卡片、备份/恢复/OPML/退出操作、手机顶栏与底部标签栏）、`src/views`（番剧库/最近条目/订阅管理/找番/设置）、`src/dialogs`（订阅、存储位置、番剧详情、编辑、元数据绑定、纠错）、`src/login`、`src/setup`（三步向导，按方向滑动切换）。设计令牌在 `src/styles/tokens.css`（紫罗兰主色，暗色默认、`html[data-theme="light"]` 覆盖），通用原语在 `components.css`，过渡类与关键帧在 `base.css`。
- 动效：路由切换淡入上浮、列表交错入场与 FLIP 位移、弹窗弹性入场（≤640px 改为底部抽屉）、侧栏/底栏/下载模式分段控件的滑动指示器、主题切换圆形揭开（View Transitions API，不支持时直接切换）、封面淡入与悬停缩放、进度条流光、运行状态计数数字滚动、登录失败抖动；`prefers-reduced-motion` 全部降级。
- 弹窗栈：`src/composables/dialogStack.ts` 记录打开顺序，Esc 只关闭最上层，避免详情内二级弹窗一次 Esc 全关；打开期间锁定页面滚动。
- 已知取舍：每次构建 `assets/` 文件名哈希会变化，提交时 `static/assets` 会整体替换；这是为了让浏览器缓存自动失效。`@lucide/vue` 的函数式图标组件不能作为 `withDefaults` 默认值（会被当作工厂函数调用导致整页渲染失败），需要用 `computed` 兜底。

- 范围：仅 `static/` 与文档，Go 代码零改动。`style.css` 重写为 token 化设计系统（表面/线条/文字分层、语义色、圆角、阴影、缓动变量；暗色默认、亮色经 `html[data-theme="light"]` 覆盖），组件统一规格：按钮（primary/danger/small/icon）、输入聚焦环、状态徽章（带图标）、卡片、弹窗（毛玻璃背景 + 入场动画 + 右上角关闭）、toast（图标 + 退场动画）、进度条（渐变 + 流光）、空状态（圆形图标底）；另有 focus-visible 焦点环、细化滚动条、`prefers-reduced-motion` 降级、背景环境光渐变；运行状态卡片改为图标 + 脉冲点 + 键值行结构。
- 图标：新增 `static/icons.svg`（由 lucide-static v0.544.0 本地生成的 48 图标 sprite，文件头注明来源与 ISC 许可）与 `static/favicon.svg`；静态页面直接 `<use>` 引用，动态渲染统一走 `app.js` 的 `icon()` 产出。界面全程不使用表情符号与 Unicode 几何符号（◈▤☰⚙☀ 等），已全量扫描 static/ 非 ASCII 字符清单确认只剩中文排版符号与密码占位 `•`；零 CDN、零新增运行时依赖。
- 结构：侧栏导航包进 `.nav-group`（手机端横向滑动、隐藏滚动条）；各弹窗统一 `dialog-head` + 关闭按钮（确认弹窗右上角关闭等同取消）；番剧详情标题移到弹窗头；设置分组加图标头；找番关键词输入框内嵌搜索图标；登录页与 `/setup` 引导页统一品牌方块 logo、步骤编号 chip；`index.html` 增加 SVG favicon。
- 验证：`node --check static/app.js`；全部静态文件 UTF-8 无 BOM、无 `�`；引用图标与 sprite 定义做过一致性核对。浏览器实测（8058 隔离实例 + 本地 RSS/媒体测试服务制造真实条目状态，8059 全新实例验证引导页）：亮暗两套主题下登录页、番剧库卡片与详情（Season 分组/资源行）、条目三态徽章（已保存/下载中含进度条/失败含原因）、确认弹窗、订阅卡片、找番初始/结果/失败/自定义源、设置分组、toast、1280px 与 390px 视口；过程中修复侧栏导航被按钮居中样式影响、手机端导航竖向堆叠占满屏两个问题。未实测：真实封面（本轮关闭 Bangumi）、外部下载器模式下的徽章形态、Docker 容器内运行。

### 历史完成与验证（2026-09-06 至 2026-09-07）

- 外部下载器闭环：`items` 表新增 `downloader/task_id/task_error/progress`；提交捕获任务 ID；每 30 秒（`DOWNLOADER_POLL_SECONDS` 可调）轮询；完成后从映射路径复制进归档目录并生成 `info.json`+NFO；归档失败保留进度与原因，下轮自动重试。
- 端到端实测（Windows 本机，Aria2 1.37.0 RPC + anibt.net 真实 RSS 订阅）：订阅"绿茶字幕组 与你相恋到生命尽头"过滤出 4 条（07/08/08v2/09），全部正确识别集数并提交 aria2 BT 下载；09、08 两集已完成并自动归档，文件与 RSS enclosure 声明大小完全一致（399331889/391943072 字节），MP4 box 链（ftyp>mdat>moov）与 MKV EBML+Segment 结构均校验为完整可播放；`info.json`/NFO 集数字段与 RSS 描述互证。
- 管理界面 2026-09-06 重构为深色管理面板（原生 CSS/JS，无 CDN 依赖，styles: `static/style.css`，逻辑: `static/app.js`，`forms.js` 已移除）：左侧导航切换条目/订阅视图，状态筛选徽章带全库计数，下载中条目显示进度条，添加/编辑订阅改用弹窗（含 RSS 预览），删除等操作使用确认弹窗与通知，替代浏览器原生 prompt/alert。已用桌面 1280px 和手机 390px 视口浏览器实测：条目/订阅渲染、状态筛选、确认弹窗、RSS 预览弹窗、空状态均正常。
- 主题切换（2026-09-06 晚新增）：CSS 颜色全部收敛为 `:root`（暗色默认）与 `html[data-theme="light"]`（亮色，文字色加深保证对比度）两组变量，硬编码颜色清零；`index.html` 头部内联脚本按"手动选择优先、否则跟随 `prefers-color-scheme`"在首帧前写入 `data-theme`，避免刷新闪色；切换按钮在侧栏操作区（`#btn-theme`），选择存 localStorage，未手动选择时监听系统亮暗变化。已在运行中的本机实例浏览器实测：1280px/390px 视口下亮暗两套主题的条目/订阅渲染、添加订阅弹窗、搜索空状态、刷新持久化（刷新后首帧即恢复所选主题）均正常；纯静态文件改动，未触及 Go 代码。
- 封面识别（2026-09-06 深夜新增）：新增 `meta.go`，`anime_meta` 表缓存标题→封面；同步完成与启动时把缺元数据的识别标题放入队列，单 worker 每 0.5 秒一次查询 Bangumi 旧版搜索（`type=2&format=json`，已用真实接口确认返回结构与 lain.bgm.tv 图片链），封面下载到本地后条目 API 输出 `/api/covers/{hash}`；`main()` 同时新增 `PORT` 环境变量（默认 8000）用于多实例/测试。已通过 `gofmt`/`go vet`/`go build` 与 `go test ./... -count=1`（新增 `meta_test.go`：查询词拆分、匹配启发式、缓存与复用、负缓存、失败不落记录、`/api/covers` 路径安全、同步触发入队）；并用本地 RSS + 真实 Bangumi 在 8010 端口隔离实例浏览器实测：芙莉莲两集自动获得同一封面并本地缓存（216KB JPEG），“完全架空的动画作品XYZ”显示占位图，亮暗主题与手机 390px 视口渲染正常，`/api/covers/..%2f` 路径穿越返回 404，应用日志无异常。
- 本轮已执行并通过 `go vet ./...`、`go test ./... -count=1 -timeout=120s`、`go build ./...`；`docker compose config` 尚未在本轮重跑。
- 修复编辑订阅会悄悄停用的问题（2026-09-07）：此前编辑弹窗的 `PUT /api/feeds/{id}` 请求不携带 `enabled`，后端按零值把启用中的订阅写成暂停。`FeedInput.Enabled` 改为 `*bool`，未携带时用 `COALESCE(?, enabled)` 保留数据库原值；前端编辑弹窗保存时显式携带打开弹窗时的启用状态，暂停/启用开关的显式切换路径不变。新增 `TestUpdateFeedPreservesEnabledWhenOmitted`（临时数据库 + 直接调用 handler）：不带字段保持启用、不带字段保持暂停、显式携带生效三条路径。
- 2026-09-07 修复后已执行并通过 `gofmt`、`go vet ./...`、`go build ./...`、`go test ./... -count=1 -timeout=120s`；未做浏览器端到端复测（纯字段传递修复，界面操作路径与 2026-09-06 实测一致）。
- 账户登录（2026-09-07 新增）：新增 `auth.go`，原 Basic Auth 中间件升级为"会话 Cookie 为主、Basic Auth 兼容脚本"；`web_sessions` 表随 `initDB` 创建；`static/login.html` 登录页复用 `style.css` 变量（新增 `.login-*` 样式与 `input[type="password"]` 外观）；`/api/status` 新增 `auth_enabled` 字段控制侧栏"退出登录"按钮显隐；`app.js` 的 `api()` 遇 401 只跳转一次 `/login?next=`。验证：新增 `auth_test.go`（登录成功/失败、Cookie 属性、库内只存哈希、会话认证、注销后旧 Cookie 失效、过期会话拒绝并清理、清理保留有效会话、限速锁定与重置、配置不完整时接口 400、`safeNext` 防开放重定向、`/api/status` 输出），并更新 `TestWebAuth`（页面 302、API 401 无 WWW-Authenticate、Basic Auth 仍通、health 公开）；`gofmt`/`go vet`/`go build` 与 `go test ./... -count=1 -timeout=180s` 全部通过。另起 8020 端口隔离实例（`WEB_USER=admin`）浏览器实测：`/` 未登录 302 到 `/login`、错误密码提示"用户名或密码错误"、正确登录跳回首页且"退出登录"按钮出现、点击退出回登录页后再访问 `/` 重新跳转、刷新保持会话、1280px/390px 视口与亮暗主题渲染正常；另起 8021 端口未配置凭据实例确认：`/` 直接 200、`/login` 302 回首页、`/api/status` `auth_enabled:false`、`/api/login` 400。`docker compose config` 未在本轮重跑（未改容器配置）。
- 出站代理（2026-09-07 新增）：新增 `proxy.go`，`PROXY_URL` 默认空保持显式直连；`parseProxyURL` 支持 http/https/socks5/socks5h（大小写不敏感、无前缀自动补 `http://`、凭据写 URL、`Hostname()` 校验缺失主机）；`applyProxyConfig` 重建 4 个内容客户端（httpClient、mediaClient 保留 30s ResponseHeaderTimeout、torrentClient 新增替代 downloaderClient 抓 `.torrent`、metaClient），`downloaderClient`/`qbitClient` 仍显式直连只用于下载器 RPC；`/api/status` 新增 `proxy` 字段（`redactProxy` 去除账号密码）并在运行卡片显示。验证：新增 `proxy_test.go`（解析边界、假 HTTP 代理证明 RSS 与 `.torrent` 请求以绝对 URI 经过代理、非法配置不改变原配置、脱敏）；`gofmt`/`go vet`/`go build` 与 `go test ./... -count=1 -timeout=180s` 全部通过。另起 8022 端口实例配置 `PROXY_URL=http://alice:secret@127.0.0.1:8099`（本地假代理）：应用日志与 `/api/status` 均只显示 `http://127.0.0.1:8099`，RSS 预览请求在假代理日志中留下 `GET http://target.example/rss.xml` 绝对 URI；8023 端口实例配置 `ftp://` 前缀启动即失败并输出"不支持的代理协议"；浏览器实测运行卡片显示"代理 http://127.0.0.1:8099"且亮色/暗色布局正常。`docker compose config` 未在本轮重跑（仅新增注释与空变量行，未改结构）。SOCKS5 仅通过解析与 http.Transport 原生支持验证，未起真实 SOCKS5 服务端实测。
- 媒体库归档与自定义存储位置（2026-09-07 新增）：重写 `archive.go` 归档布局，弃用 `{番剧名}/{随机ID}/` 结构；`archiveTarget` 计算 `{存储位置}/{番剧名}/Season NN/{番剧名} - SxxEyy`（集数 Atoi 补零，无集数放番剧根保留原始文件名），存储位置 = `anime_storage` 表自定义目录（优先）或 `DOWNLOAD_DIR/{safeComponent(番剧名)}`；提交从目录级 rename 改为 `commitLinked` 硬链接（O_EXCL 语义，冲突自动 ` (n)` 后缀，失败回退 rename），BT 多文件资源放 `SxxEyy/` 集目录、元数据写在集目录内；`info.json` 更名为 `{基准名}-info.json` 支持多集同目录。新增 `anime_storage` 表与 `PUT/GET /api/anime/{title}/storage`（`sanitizeStorageDir` 要求绝对路径、拒绝 `..`、`path.Clean` 保持 `/` 分隔符跨平台稳定），items API 输出 `file_path`，前端条目卡片新增「位置」弹窗与归档路径展示，备份升级 v2 含 `anime_storages`（restore 接受 v1/v2）。验证：新增 `archive_test.go`（Season/SXXEYY 布局、自定义目录+季号、清除恢复默认、非法路径拒绝且不落库、同集冲突 ` (2)` 后缀、BT 多文件集目录、备份往返 anime_storages），更新 `TestAutomaticDownloadAndDeduplication` 断言；`gofmt`/`go vet`/`go build` 与 `go test ./... -count=1 -timeout=180s` 全部通过。8024 端口实例 + 本地 RSS/媒体服务端到端实测：添加订阅自动下载两集落 `downloads/夏日动画   [1080p]/Season 01/夏日动画   [1080p] - S01E07.mp4`（NFO/info.json 同名），浏览器条目卡片显示归档路径与「位置」按钮，弹窗设置自定义目录+季 2 保存后删除第 8 集重新同步，新集落 `my-anime/Season 02/...S02E08.mp4`，第 7 集保持默认位置不变。未实测：Docker 容器内运行、真实 NAS 媒体服务器（Emby/Jellyfin）实际刮削效果。
- 用户设置与首次安装引导（2026-09-07 新增）：新增 `settings.go` 与 `settings` 表（KV），`applyStoredSettings` 把库值映射到全局变量（普通字段键存在即生效——包括清空回退 env；token/密码只增不清；proxy 走 `applyProxyConfig` 热更新），`loadSettings()` 在 `main()` 中 env 代理之后调用（数据库优先）；`authEnabled`/`verifyWebCredentials` 支持哈希凭据（`webHash` 优先于 env 明文对），登录/Basic Auth/引导共用的 `setPasswordCredentials` 只存 `v1$salt$hash`；`setupRequired() = !setupDone && !authEnabled()`，中间件对未初始化实例把页面重定向 `/setup`、其它 API 返回 401（setup/status/static/health 公开），`POST /api/setup` 完成或跳过后锁定 403；`GET/PUT /api/settings` 指针字段区分"未提供/清空"，敏感字段只写不读（响应仅 `*_set` 布尔），密码至少 6 位、改用户名必须同给密码。前端：`static/setup.html` 三步引导（账户/下载方式/网络同步，按模式显隐下载器字段，完成后自动登录），主面板侧栏新增「设置」视图与 `#btn-save-settings`。验证：新增 `settings_test.go`（全新实例判定与向导流程、重启后 loadSettings 恢复、设置热生效与清空回退、非法模式/周期/代理拒绝且不改变原值、密码只存哈希、响应不含明文与哈希、setup 完成后 403、中间件未初始化重定向与跳过后开放），更新 `TestAuthRequiresCompleteConfig`（setupDone 隔离）；`gofmt`/`go vet`/`go build` 与 `go test ./... -count=1 -timeout=180s` 全部通过。8025 端口全新实例浏览器端到端：`/` 302 到 `/setup`，填管理员+aria2+socks5 代理完成引导后自动登录进面板，`/api/setup` 重放 403，设置页回填正确、改代理与同步周期保存后运行卡片立即显示"每 10 分钟同步 · 代理 http://127.0.0.1:7890"，重启实例后凭据/代理/周期全部保留且 `/setup` 跳回首页。未实测：Docker 容器内运行。
- 番剧订阅管理（2026-09-07 深夜新增）：新增 `anime.go` 与 `animes` 表（title 唯一，含 search_name/bangumi_id/season/storage_dir/include_rules/exclude_rules），`items` 新增 `anime_id/season/manual` 列；`migrateAnimeData()` 启动时把识别标题建库、`anime_storage` 迁入 animes（迁移后清空旧表避免复活）、回填 items.anime_id（幂等）。`syncFeed` 先按番剧规则匹配原始标题（`matchAnimeRule`，include 全匹配且最长者胜），命中归并、否则 `ensureAnime` 建库。归档 `archiveTarget` 接受条目级季号覆盖（`resolveSeason`：条目 > 番剧 > 1），番剧根写 `tvshow.nfo`（绑定后含 Bangumi uniqueid）；`rearchiveSavedItem` 支持已保存文件迁移（区分单文件与多文件集目录、跨设备回退复制、`cleanupEmptyDirs` 清理空目录但不删媒体）。API：`GET /api/animes`、`GET/PUT/DELETE /api/animes/{id}`、`POST /api/animes/{id}/apply-rules`、`PUT /api/animes/{id}/metadata`、`GET /api/meta/search`、`PATCH /api/items/{id}`；`GET /api/animes/{id}` 输出季/集分组（同集聚合、数值排序、loose 资源、来源列表）。备份升级 v3 导出 animes，恢复兼容 v1/v2/v3（v2 storage 并入 animes）。验证：新增 `anime_test.go` 14 个用例（规则同步归并与回溯、manual 保护、详情分组与排序、改名级联与同名 409、绑定/解绑与 manual 不覆盖、搜索候选、纠错迁移落盘与 tvshow.nfo、非法输入拒绝、番剧季号默认与条目覆盖、v2 备份兼容、迁移幂等），更新 `archive_test.go`（storage 断言改 animes、备份测试改 v3）；`gofmt`/`go vet`/`go build` 与 `go test ./... -count=1 -timeout=240s` 全部通过。8030 端口实例（本地 RSS + 真实 Bangumi）浏览器端到端：添加订阅自动下载 4 条，番剧库出现 3 张卡片；同一部番因标题带清晰度标签拆成两个条目，编辑改名「星海物语」+包含规则保存后点「应用到现有条目」归并 1 条，详情第 07 集显示"2 个版本"（绿茶组 1080p + 清风组 720p）；纠错 720p 版本到第 2 季并勾选迁移，文件从旧目录落到 `星海物语/Season 02/星海物语 - S02E07.mp4`（旧 Season 01 清空）；元数据绑定搜索"葬送的芙莉莲"绑定 #400602 后标题变译名、封面本地缓存（216KB JPEG）、tvshow.nfo 含 `<uniqueid type="bangumi">400602</uniqueid>`；1280px/390px 视口与亮暗主题渲染正常。本轮另修复：详情资源缺 `anime_id` 字段导致纠错弹窗无法预选、仅改季号被误拦（后端补字段+前端放行同番纠错）。分支：全部工作在 `feature/anime-library` 分支提交（基线 daf72ee），已随本轮合并进入 main；`docker compose config` 未在本轮重跑。
- 找番与搜索订阅（2026-09-07 新增，`feature/discover` 分支，已合并进入 main）：新增 `discover.go`，`GET /api/discover/sources` 与 `GET /api/discover/search`；搜索结果 RSS 用独立解析保留资源页与 enclosure 两条链接，逐条 `extractAnimeInfo` 识别；20 秒请求超时、8MB 响应上限、60 条截断；自定义搜索模板存 `settings` 表（键 `discover_search_template`，`{keyword}`/`{关键词}` 占位符，保存时校验 http(s)+占位符+主机，清空即回退，热生效）；前端侧栏新增「找番」视图（源下拉+关键词+摘要条展示/复制搜索 RSS 地址+结果卡片一键预填订阅弹窗）。验证：新增 `discover_test.go`（源列表与自定义源出现、非法模板忽略、模板校验与占位符替换转义、搜索解析含相对链接补全与空标题过滤、关键词转发、输入校验 400/405、上游 HTTP 错误/非 RSS 文档/超响应上限/连接失败均 502 带原因、设置非法拒绝且不落库、合法保存即生效、清空回退、loadSettings 重启恢复、自定义模板真实请求命中），更新 settings 集成；`gofmt`/`go vet`/`go build` 与 `go test ./... -count=1 -timeout=180s` 全部通过。8046 端口隔离实例（独立数据目录）浏览器端到端：登录后搜索「芙莉莲」（ACG.RIP 真实源，国内直连）返回 30 条并正确识别字幕组/集数/清晰度，结果「添加订阅」预填弹窗（标题「添加订阅」、地址可改）保存后自动首次同步，17 条入库且种子类资源进「等待下载器」，订阅卡片显示搜索 RSS 地址与同步时间；设置页非法模板（缺占位符）被拒绝、死端口自定义源搜索给出具体连接失败原因；1280px/390px 视口与亮暗主题渲染正常。过程中修复两处前端问题：`openFeedDialog` 按 `feed.id` 判断编辑态（预填伪对象此前被误判为编辑、地址被禁用），搜索源列表改为每次进入找番视图刷新（设置页修改模板后立即生效并保持选中项）。本轮开发在独立工作区进行（目录 `E:\项目\auto-anime-discover`，分支 `feature/discover`，基线为并行会话的 `feature/anime-library` 提交 daf72ee）；测试会话 Cookie 冲突教训：同主机多实例的登录 Cookie 按 host 共享、与端口无关，浏览器实测需用 127.0.0.2 等不同回环地址隔离。未实测：Docker 容器内运行、SOCKS 代理下找番、资源站 IPv6 环境。
- 测试仍使用本地 HTTP 服务及临时数据库；真实 BT 下载仅在本机 Windows 验证，NAS 挂载权限、Docker 容器内运行和国内 NAS 实机仍未验证。源码已托管在 GitHub（https://github.com/sssshj114/auto-anime，main 分支），`ani-rss-reference/`、数据库、下载文件、本地可执行文件经 `.gitignore` 排除。

### 已知缺口

- **媒体库整理的收尾**：季号识别仍是本地启发式（默认跟随番剧，条目可纠错但无自动季号解析）；已归档文件需逐条纠错迁移（无批量迁移工具）；同季同集不同字幕组会以 ` (n)` 后缀并存，不区分版本优先级；番剧改名不迁移已归档文件（旧目录保留，仅新归档用新名）。
- **元数据仍以封面为主**：Bangumi 封面已接入且支持手动绑定与解绑，但没有 TMDB、简介、评分；绑定后的简介/年份不展示；绑定搜索一次最多 8 条候选，无翻页。
- **番剧库细节**：番剧列表无分页（一次全量）；删除番剧要求先清空条目；规则匹配只在同步与手动应用时生效，不会自动撤销用户后续改错的规则；空番剧（条目被移走后遗留）需手动删除。
- **慢任务无告警**：做种少的旧种（如 8 月底发布、仅 2% 进度的任务）会长期停留在"下载中"，没有超时/卡死检测；手动重试会重复提交任务且不清理下载器中的旧任务。
- **同步和任务可靠性还需加强**：RSS 响应大小限制未实现；任务取消与删除期间的并发行为未专项测试；下载器临时故障只在日志中体现。
- **备份恢复不是完整灾备**：不恢复历史条目、下载器任务及媒体文件；备份含 RSS 地址令牌。
- **管理界面仍有缺口**：条目列表最多 100 条且无分页；没有每条任务的下载器详情页（速度/peer 数/做种状态）。
- **部署验收未完成**：未在 Docker 容器内运行本功能，未用用户 NAS 实机（挂载、权限、amd64/arm64 镜像）做国内直连验收；Windows 本机验证不能等同 NAS 验证。
- **找番依赖第三方站点可用性**：内置源只有 ACG.RIP；蜜柑计划搜索接口与动漫花园关键词 RSS 在 2026-09-07 对匿名请求不可用（分别返回空结果、不按关键词过滤），站点改版或不可达时找番不可用（订阅、同步、下载不受影响），可在设置页配置自定义 RSS 搜索模板更换来源；搜索结果按资源站返回顺序展示，无跨源聚合、排序与分页。

### 后续开发顺序

1. 元数据补全：TMDB 可选来源、简介/年份展示与绑定候选翻页；来源不可达时保留基础订阅下载能力。
2. 番剧库收尾：批量迁移工具（番剧改名/改季号后一键迁移全部已归档文件）、番剧列表分页、版本优先级规则。
3. 任务卡死检测/超时告警、重试时清理下载器旧任务、RSS 响应大小限制。
4. 管理界面任务详情与分页。
5. 构建 NAS 常用架构镜像并在容器内验证本功能，用实际 NAS 的挂载目录、RSS 和下载器完成国内直连验收。
6. 找番增强：跨源聚合与结果分页、蜜柑计划/动漫花园匿名搜索恢复可用后纳入内置源、搜索结果直接手动下载单条资源。

## 技术栈

- Go 1.23，使用标准库 HTTP、XML 和 SQL。
- SQLite 使用 `modernc.org/sqlite`，关闭 CGO，适合 NAS Docker 部署。
- Web 页面源码位于 `web/`（Vue 3 + Vite + TypeScript），`npm run build` 输出到 `static/` 并随仓库提交；Go 服务端只读取 `static/`，运行与 Docker 构建不需要 Node。界面文案默认使用简体中文。
- 优先沿用现有依赖和模块模式；按职责逐步拆分代码，不把无关重构混入功能修改。
- 新依赖必须有具体用途，并提交对应的 `go.mod`、`go.sum` 变更。不要用其他语言重写后端。

## 项目布局

- `main.go`：启动、HTTP API、订阅同步、解析、下载调度及数据库初始化。
- `anime.go`：番剧库实体与 API（animes 表、多来源规则、元数据绑定、条目纠错与数据迁移）。
- `auth.go`：登录页、会话管理、登录限速与访问控制中间件。
- `settings.go`：用户设置存储与应用、首次安装引导、管理员凭据哈希。
- `proxy.go`：`PROXY_URL` 解析、统一出站传输配置与代理地址脱敏。
- `discover.go`：找番搜索源注册表、搜索请求（大小/超时限制）与结果解析。
- `downloader.go`：下载器任务提交、任务 ID 持久化、进度轮询、路径映射与 infohash/种子处理。
- `meta.go`：Bangumi 封面识别、本地缓存与 `/api/covers/` 服务。
- `archive.go`：直链与下载器文件的归档、文件名处理和本地元数据生成。
- `archive_migration.go`：纠错迁移、源文件保护、目标准备与失败回滚、提交后旧文件清理。
- `recovery.go`：下载任务恢复及下载器配置校验。
- `status.go`：健康检查和运行状态。
- `*_test.go`：Go 测试；`web/`：管理界面源码（`src/api` 类型与请求封装、`src/stores` 全局状态、`src/components` 通用与布局组件、`src/views` 五个视图、`src/dialogs` 业务弹窗、`src/login` 与 `src/setup` 独立入口）；`static/`：`web/` 的构建产物，不要手工编辑。
- `Dockerfile`、`docker-compose.yml`：构建及 NAS 部署；`README.md`：用户配置与使用说明。
- 目录结构调整时同步维护本节。

## 编码与文件

- Windows 环境。代码、配置、Markdown、JSON、YAML、TOML、TXT、CSV、Go 及 Godot 文本文件统一按 UTF-8 无 BOM 读取和保存。
- 无法按 UTF-8 严格解码时立即停止修改并报告路径，不猜测编码，不在 GBK、CP936、UTF-8、UTF-16 之间反复转换。
- 修改中文文件后检查是否出现 `�`，确认没有无关的整文件变化。
- 不对图片、音频、视频、字体、二进制文件、`.godot/imported` 和生成缓存做编码转换。
- 小范围修改优先使用 `apply_patch`；不要使用 `>`、`>>`、`Out-File` 或未明确指定编码的命令写入项目文本文件。
- 优先使用 PowerShell 7（`pwsh`）。不可用时不要用 Windows PowerShell 5.1 重写中文文件；必要时使用 Python 并明确指定 `encoding="utf-8"`。
- 保留已有用户修改；有 Git 时检查差异，没有 Git 时对比修改前后内容，不声称已检查 Git diff。

## 参考项目与独立实现

- 参考项目：[ani-rss](https://github.com/wushuo894/ani-rss)。仅研究公开功能、用户流程及接口文档，再根据本项目需求独立设计实现。
- 不复制、翻译或移植参考项目的源代码、界面、素材和测试；不要将参考代码包装成自己的实现。
- `ani-rss-reference/` 仅作参考资料，不属于产品源码、编译输入或 Docker 构建上下文。
- 可研究订阅、过滤、定时同步、下载器联动、重命名整理、元数据和通知等功能。引用功能结论时注明来源，区分已核实事实与设计建议。

## 网络与外部服务

- RSS、下载器、通知和可选元数据服务使用用户配置的地址；HTTP 请求设置超时，并限制响应大小和并发量。
- 出站请求默认显式直连，不隐式读取系统代理设置；用户可通过 `PROXY_URL` 显式配置代理（http/https/socks5/socks5h），代理仅作用于公网内容访问，下载器 RPC 保持直连；新增客户端应沿用 `proxy.go` 的统一传输配置。
- 国内直连是设计要求，不是对第三方站点可达性的保证。RSS、元数据或下载源不可达时报告具体原因，允许更换地址或手动补充信息。
- 元数据服务失败不得阻塞 RSS 基础订阅和下载流程；保留原始标题与识别来源，不把推测的集数当作已确认信息。
- 管理页面的脚本、样式、字体和必要图标随应用提供，避免运行时依赖公共 CDN。
- 分别说明源码构建、镜像获取和应用运行需要的网络条件；不能因本机联网成功就声称国内 NAS 已验证。

## 数据与下载约束

- 使用参数化 SQL；数据库迁移兼容已有数据。批量导入使用事务，失败不得留下半份配置。
- SQLite 当前连接池仅有一个连接，必须及时关闭查询结果；事务中的查询使用同一事务，避免等待自己释放连接。
- 同步、定时任务、手动下载及重启恢复要考虑幂等性和并发，防止重复提交或重复保存同一资源。
- 区分待下载、排队、下载中、已提交下载器、已保存及失败等状态。外部下载器接受任务不等于文件已下载或已归档。
- 直链下载先写临时位置，校验可用的长度信息并完成归档后才能标记成功；中断时清理临时资源，不能把不完整文件当成完成。
- RSS 标题、URL 和外部文件名都是不可信输入。文件路径必须限制在配置目录内，处理路径穿越、Windows 保留名称、非法字符和重名冲突。
- 保留原始资源标题；本地生成的 NFO、JSON 信息不能冒充已通过 Bangumi/TMDB 校验的元数据。
- 删除订阅或条目默认仅删除记录。删除媒体文件必须是用户明确选择的独立操作。
- 备份恢复明确列出包含和排除的数据；恢复订阅不得误报为恢复文件或下载任务。RSS URL 可能含令牌，日志和界面注意脱敏。
- 下载器密码、令牌及 Webhook 凭据不得写入日志、状态 API、测试样例或提交到源码。
- 修改鉴权时验证未配置、完整配置及配置不完整的行为；健康检查不泄露地址、凭据或订阅详情。

## NAS 与 Docker

- 数据库和应用管理的下载文件默认持久化到 `/data`，用户通过 volume 映射 NAS 实际目录。
- 外部下载器的保存路径属于该下载器容器。配置说明必须解释两边的挂载关系，不能假设两个容器的相同路径指向相同文件。
- 保持 `CGO_ENABLED=0`；依赖变动需检查 Linux amd64 和 arm64 编译兼容性，不把交叉编译成功等同于 NAS 实机验证。
- Docker 构建使用 `go.mod` 和 `go.sum` 锁定依赖，并编译整个 Go 包，不能遗漏新增的 `.go` 文件。
- 排除参考项目、凭据、数据库、下载文件、本地可执行文件和缓存，避免进入镜像构建上下文。
- 说明 NAS 挂载目录的写权限、端口、健康检查和备份方式。一个数据库只运行一个服务实例，除非已实现并验证多实例协调。

## 验证

- 修改 Go 代码后对修改过的文件执行 `gofmt -w 文件名.go`。PowerShell 不会自动为所有程序展开 `*.go`，不要直接使用 `gofmt -w *.go`。
- 后端变更运行 `go test ./...`、`go vet ./...` 和 `go build ./...`；纯文档修改检查内容、编码和差异即可。
- 测试使用临时数据库、临时目录和本地模拟 HTTP 服务，不依赖真实 RSS 站点、NAS 凭据或公网下载。
- 触及解析、去重、重试、恢复、路径安全和事务时补充对应失败场景。共享全局数据库或配置的现有测试不能直接使用 `t.Parallel()`。
- 修改容器配置后执行 `docker compose config`；Docker 可用且不会影响现有部署时构建并启动容器验证。无法验证时如实说明原因。
- 修改界面时检查中文、加载、空状态、失败状态以及桌面和手机布局；使用浏览器验证实际操作流程。
- 前端改动后在 `web/` 执行 `npm run build`（含 `vue-tsc` 类型检查），把 `static/` 产物一并提交；不要手工编辑 `static/`。
- 只报告真正执行过的检查；编译通过不能替代下载、重启恢复或容器运行验证。

## 交付说明

- 新增或修改用户可见功能时同步更新 README 和配置示例，包括环境变量、默认值、目录及兼容性影响。
- 完成时简要说明改动、实际验证结果和仍未实现或无法验证的部分。
- 不将规划功能写成现有能力，不声称已完成未执行的国内网络、NAS 或下载器端到端验证。

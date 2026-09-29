# 配置参考

[返回 README](../README.md)

## 配置方式

日常设置可在网页「设置」中修改；同时下载数量及暂停、继续操作集中在「最近条目」。设置保存后立即生效并写入数据库。已有数据库中的对应设置优先于环境变量，因此修改 Compose 后，页面中的旧设置仍可能继续生效。

环境变量用于启动配置；修改后执行 `docker compose up -d` 重建容器。仅重启容器不会更新环境变量。首次访问未初始化的实例时会进入设置向导，已通过环境变量配置完整登录账户时直接进入登录页。

## 基础环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `DOWNLOAD_MODE` | Docker：`builtin`；源码：`direct` | 下载方式，见下表 |
| `DOWNLOAD_DIR` | `/data/downloads`（Windows：程序同级 `data\downloads`） | 默认归档目录 |
| `LIBRARY_DIR` | 空 | 媒体库根目录；空时使用 `DOWNLOAD_DIR`，单部番的存储位置优先 |
| `DB_PATH` | `/data/auto-anime.db`（Windows：程序同级 `data\auto-anime.db`） | SQLite 数据库路径 |
| `COVERS_DIR` | 数据库同级的 `covers` | 本地封面缓存目录 |
| `PORT` | `8000` | Web 监听端口；修改时同步调整端口映射和健康检查 |
| `POLL_MINUTES` | `30` | RSS 同步周期，单位分钟，最小 5 |
| `DOWNLOADER_POLL_SECONDS` | `10` | 下载器进度轮询间隔，单位秒，最小 5 |
| `PROXY_URL` | 空 | 默认直连，代理作用范围见下文 |
| `BANGUMI_ENABLED` | `true` | 设为 `false` 关闭自动封面识别 |
| `BANGUMI_API` | `https://api.bgm.tv` | Bangumi API 地址 |
| `WEB_USER` / `WEB_PASSWORD` | 空 | 同时配置才启用登录，也可在向导或设置页配置 |
| `WEBHOOK_URL` | 空 | 接收事件通知的 HTTP(S) 地址 |
| `UPDATE_CHECK` | `1` | 设为 `0` 关闭联网检查新版本（一键更新也随之不可用） |
| `UPDATE_SOURCE` | 空 | 更新源：留空用项目仓库；也可填 `用户名/仓库`、GitHub 仓库地址或自建更新源前缀 |
| `GITHUB_TOKEN` | 空 | 访问 GitHub API 的令牌，用于私有仓库或提高接口限额；不写入日志与接口 |
| `UPDATE_PROXY` | 空 | 仅更新流量使用的代理，留空沿用 `PROXY_URL` |

容器与 Linux 的数据目录默认是 `/data`，与镜像、compose 一致。**Windows 直接运行 Release `.exe` 时**默认用程序同级的 `data\`（数据库、`downloads\`、`covers\` 都在里面）；用 `go run` 调试时该目录在当前工作目录下，避免写进 `go run` 的临时构建目录。程序启动时会打印实际使用的数据库、下载与封面路径。

早期 Windows 版本把数据写在 `<盘根>:\data`（例如 `C:\data`、`E:\data`）：这些数据不会自动搬迁，想继续用就设置 `DB_PATH`（以及需要时的 `DOWNLOAD_DIR`、`COVERS_DIR`）指回原目录。

| `DOWNLOAD_MODE` | 行为 |
| --- | --- |
| `builtin` | 镜像内置 qBittorrent 处理磁力与种子，普通 HTTP(S) 文件由本程序下载 |
| `direct` | 本程序下载 HTTP(S) 文件，磁力与种子条目进入「等待下载器」 |
| `qbittorrent` | 连接外部 qBittorrent（4.4+） |
| `aria2` | 连接外部 Aria2 |
| `transmission` | 连接外部 Transmission |

切换默认下载器影响后续提交，已有任务仍按原下载器跟踪，不会自动转移。内置与外部 qBittorrent 使用独立配置；只要保留内置任务记录，重启后仍会启动内置下载器继续跟踪和做种。

## 内置 qBittorrent

Docker 部署无需填写下载器连接信息。源码运行时需自行安装下载器，程序会为它创建独立配置，不使用桌面客户端的默认配置。Windows Release `.exe` 不包含 qBittorrent 程序；Windows 上请选择 `direct` 或外部下载器，或者自行安装 qBittorrent 并设置 `QBITTORRENT_PATH`。如果数据库或环境变量里仍是 `builtin` 而没有 `QBITTORRENT_PATH`，Windows 版启动时会明确告警并临时降级成 `direct`（老的配置因此不再导致启动即退出，可在设置页改成 `direct`/外部下载器，或装好 qBittorrent 后重启）；库里已有的内置条目会提示重新提交。

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `QBITTORRENT_DATA_DIR` | 数据库同级的 `qbittorrent/`；镜像为 `/data/qbittorrent` | 配置、凭据、续传数据与下载源文件 |
| `QBITTORRENT_PATH` | 从 PATH 查找 `qbittorrent-nox` | 可指定完整程序路径，Windows 可指向 `qbittorrent.exe` |
| `EMBEDDED_QBIT_PORT` | `8080` | 仅监听容器回环地址的 WebUI 端口，无需映射到宿主机 |
| `QBITTORRENT_BT_PORT` | `6881` | BT / DHT 端口，需与 TCP、UDP 映射一致 |

数据目录中的 `profile/` 保存配置与恢复数据，`downloads/` 保存源文件。连接凭据自动生成并持久化，不包含在设置响应或 JSON 备份中。

Compose 为正常停止留出 45 秒，以便下载器保存会话；下载器意外退出时，服务随之退出并由容器重启策略恢复。

### 旧内置 Aria2 升级

镜像已移除内置 Aria2，旧的 `EMBEDDED_ARIA2`、`EMBEDDED_ARIA2_PORT`、`ARIA2_PATH`、`ARIA2_BT_TRACKER` 不再生效。外部 Aria2 接口继续支持。

旧数据库保留原模式，升级后在设置页选择「内置 qBittorrent」。仅对能通过旧私有 `.aria2/session.txt` 与连接配置确认归属、且无法继续运行的未完成任务标记失败，供手动重试；不会将 GID 转成 qBittorrent 任务。

旧配置、下载分片和归档文件保留，不自动迁移或重新下载。重试会向当前下载器提交新任务，不代表已迁移旧分片；独立外部下载器任务不受该升级处理影响。

## 外部下载器

先运行下载器并启用 RPC / WebUI，再在设置页或环境变量中填写连接信息：

| 下载器 | 地址 | 认证 |
| --- | --- | --- |
| qBittorrent | `QBIT_URL`：WebUI 地址 | `QBIT_USER`、`QBIT_PASSWORD` |
| Aria2 | `ARIA2_URL`：如 `http://192.168.1.100:6800/jsonrpc` | `ARIA2_TOKEN`：RPC secret，可选 |
| Transmission | `TRANSMISSION_URL`：如 `http://192.168.1.100:9091/transmission/rpc` | `TRANSMISSION_USER`、`TRANSMISSION_PASSWORD` |

地址必须能从 Auto-Anime 容器访问。容器内的 `127.0.0.1` 指当前容器，连接另一个容器时应使用同一 Docker 网络中的服务名，或 NAS 的局域网地址。

### 下载目录映射

下载器写入文件后，Auto-Anime 必须能读取**同一个 NAS 目录中的同一批文件**才能复制归档：

| 下载器 | 下载器内保存路径 | 同一目录在 Auto-Anime 内的路径 |
| --- | --- | --- |
| qBittorrent | `QBIT_SAVE_PATH` | `QBIT_LOCAL_DIR` |
| Aria2 | `ARIA2_DIR` | `ARIA2_LOCAL_DIR` |
| Transmission | `TRANSMISSION_DOWNLOAD_DIR` | `TRANSMISSION_LOCAL_DIR` |

例如，同一 NAS 目录 `/volume1/downloads` 分别挂载为：

| 容器 | 卷挂载 |
| --- | --- |
| qBittorrent | `/volume1/downloads:/downloads` |
| Auto-Anime | `/volume1/downloads:/source-downloads`（在原 `/data` 挂载之外添加） |

对应配置为：

```yaml
QBIT_SAVE_PATH: /downloads
QBIT_LOCAL_DIR: /source-downloads
```

即 qBittorrent 中的 `/downloads/某集.mp4`，在 Auto-Anime 中是 `/source-downloads/某集.mp4`。两侧路径相同时也必须确保挂载的是同一 NAS 目录。映射错误会导致下载完成后无法归档；修正配置后会再次尝试归档。

归档目标由 `LIBRARY_DIR` / `DOWNLOAD_DIR` 或番剧存储位置决定，与上述源目录映射分开设置。外部下载器的文件、会话和配置需自行持久化、另行备份。

### 番剧文件夹的根目录

归档时总是先在根目录下建立以番剧名命名的文件夹（`<根目录>/<番剧名>/Season XX/…`），不会把集数直接堆在根目录。根目录按以下优先级确定：

1. 添加订阅时在「保存位置」填写的番剧根目录（找番与订阅管理同一个弹窗，只影响该订阅对应的番剧）；
2. 番剧库里为该番单独设置的存储位置（填写的是番剧文件夹本身，不是根目录）；
3. 设置页的「媒体库目录」，即 `LIBRARY_DIR`；
4. `DOWNLOAD_DIR`。

订阅保存位置留空时按 3、4 的顺序回退，并跟随媒体库目录的设置变化；填写后在订阅弹窗中会直接显示实际落点。修改保存位置不会自动搬迁已归档的文件，已有文件需通过条目「纠错」勾选迁移。

### 目录必须映射进容器

上面的根目录（`LIBRARY_DIR`、番剧存储位置、订阅保存位置）都是**容器内路径**。Docker 不会阻止程序写入未映射的绝对路径，但文件会落在容器自己的文件系统里：任务显示「已保存」，NAS 文件管理里却没有，容器重建后还会丢失。

在飞牛 fnOS 上，存储空间挂载为 `/vol1`、`/vol2`…，第一个用户（UID 1000）的「我的文件」是 `/vol1/1000`；用户文件/共享文件使用 Windows ACL，Docker 目录仍用 POSIX ACL，因此容器能否写入取决于**目录是否被映射进来**，而不是 ACL。群晖等 NAS 同理使用 `/volume1/…`。

```yaml
volumes:
  - /volume1/docker/auto-anime:/data
  # 飞牛 fnOS：把归档目录按同样路径映射进容器
  - /vol1/1000/番剧:/vol1/1000/番剧
  # 也可以映射成别的名字，此时设置页要填容器内的路径
  - /vol1/1000/番剧:/library
```

保存「媒体库目录」「保存位置」或番剧存储位置时，程序会读取容器的挂载表检查该目录是否落在已映射的目录下；未映射会返回提示（设置页、订阅弹窗、番剧编辑与「存储位置」弹窗都会显示），要求改用已映射的路径或补上 volumes 后重启容器。源码运行（非容器）与 Windows 本机不做此提示。

修改映射后已归档文件不会自动搬迁，需要迁移时用条目「纠错」勾选迁移。

## 版本检查与一键更新

服务启动后会检查一次新版本，之后每 6 小时复查；发现新版本时侧边栏出现提醒，也可在「设置 → 版本更新」手动检查。点「一键更新」后客户端会：下载与当前平台匹配的更新包 → 校验 SHA-256 → 替换程序文件 → 重启服务（同一进程原地重启，容器不会因此重建）。

- **信息来源**：默认读取项目仓库的 GitHub Release（标签、更新说明、附件与 `checksums.txt`）。预发布版本标签的更新说明为空时，会回退读取 Release 页面正文。
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
- **更新内容**：Release 正文会显示在弹窗中，纯文本渲染（HTML 已转换为文本），不执行任何页面脚本。
- **忽略版本**：可以「忽略此版本」，仅保存在浏览器本地，出现更高版本时再次提醒。
- **关闭**：`UPDATE_CHECK=0` 后不再联网检查，接口返回 `enabled=false`。
- **私有仓库**：仓库为私有时匿名读取 Release 会返回 404，必须配置带 `repo` 读权限的 `GITHUB_TOKEN`（也可使用 `UPDATE_SOURCE` 指向自建更新源）。仓库公开后该变量可以去掉。
- **仓库改名与 fork**：默认更新源为 `Shizuku11451/auto-anime`；发布工作流会用实际仓库名覆盖它（`-ldflags -X main.defaultUpdateSource=<owner/repo>`），因此 fork 后自行发版不需要改代码，本机构建可用 `-ldflags` 或 `UPDATE_SOURCE` 覆盖。

### Docker 部署的更新行为

| 部署方式 | 一键更新后的结果 |
| --- | --- |
| 发布镜像（`ghcr.io/shizuku11451/auto-anime`），`/app` 可写 | 更新包直接替换容器内可执行文件，并写入 `/data` 暂存副本，重启后即为新版本 |
| 发布镜像，`/app` 只读（默认行为时通常可写，容器以 root 运行） | 更新包写入 `/data` 暂存；下次启动由容器入口与应用引导步骤替换程序文件并自动重启 |
| 本地构建镜像（`docker compose up -d --build`） | 可正常更新，但**下次构建镜像会覆盖为新镜像里的旧版本**；把源码更新并重新构建，或把 `/app/auto-anime` 做成挂载卷 |
| 源码运行 | 不提供自动更新（版本为 `dev`），设置页只提示有新版本 |

容器被重建（`docker compose up -d --force-recreate`）时，如果镜像是旧版本，程序文件会回到旧版本，但 `/data` 中保留的暂存更新包会在启动引导阶段重新替换，重启一次即恢复新版本。因此升级镜像与一键更新可以并存：任一路径都能把版本推到最新。

`/app` 不可写时，容器入口会把镜像版本写入 `/data/image-version`，应用据此提示「更新已下载但未能替换程序文件：请用新镜像重建容器」。

### 维护者发布流程

1. 提交并推送源码，打上语义化版本标签：`git tag -a v1.1.0 -m "更新说明" && git push origin v1.1.0`（标签注释会作为 Release 说明；留空则用两次 tag 之间的提交列表）。
2. GitHub Actions 的 `release` 工作流会交叉编译 `linux/amd64`、`linux/arm64`、`windows/amd64` 三个可执行文件，生成 `checksums.txt` 并创建 Release。
3. 同一工作流随后构建并推送 `ghcr.io/shizuku11451/auto-anime:<版本>` 与 `:latest` 镜像；工作流也支持手动触发（`workflow_dispatch`）并填写已存在的 tag。
4. 客户端的更新检查会立即看到该版本。本地构建镜像时可用 `--build-arg VERSION=v1.1.0` 写入版本号，否则镜像内版本为 `dev`。

## 网络与封面

本程序默认显式直连，不读取系统代理。可通过设置页或 `PROXY_URL` 配置 `http://`、`https://`、`socks5://`、`socks5h://` 代理，例如：

```text
http://192.168.1.10:7890
socks5://192.168.1.10:1080
http://用户名:密码@192.168.1.10:7890
```

- **使用代理**：RSS 搜索、预览与同步，HTTP(S) 文件和种子获取，HTTP(S) tracker 探测，Bangumi 请求、Webhook 以及版本检查与更新包下载。
- **始终直连**：下载器 RPC / WebUI API、UDP tracker 探测。BT 下载流量由下载器自己的网络设置决定。
- **认证信息**：界面隐藏代理账号密码；保存未变的地址会保留凭据，更换地址不会沿用旧凭据。清空地址恢复直连，清除凭据可使用页面中的专门选项。

Bangumi 不可达只影响元数据获取，不阻塞订阅和下载。封面缓存在本地；匹配错误可在番剧详情重新绑定，未匹配时可修改「元数据搜索词」重试。

## 其他网页设置与通知

以下设置通过网页保存到数据库：

| 设置 | 说明 |
| --- | --- |
| 最大同时下载数 | 在「最近条目 → 下载控制」设置，默认 2，范围 1–32；降低限制不打断已有任务，暂停释放名额，继续后按上限排队；下载器自身限额仍有效 |
| 自定义 RSS 搜索模板 | 必须含 `{keyword}` 占位符，如 `https://example.com/search?term={keyword}`；清空后仅保留内置源 |
| 管理员账户 | 密码只存哈希；普通保存无需重填密码，修改用户名须同时设置新密码，改密后旧会话失效 |

`WEBHOOK_URL` 接收 JSON POST，事件包括 `new_item`、`download_submitted`、`download_saved`、`download_failed`。脚本调用 API 可使用管理员账户的 HTTP Basic Auth；`/api/health` 始终公开供健康检查使用。

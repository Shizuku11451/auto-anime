# 配置参考

[返回 README](../README.md)

## 配置方式

日常设置可在网页「设置」中修改；同时下载数量及暂停、继续操作集中在「最近条目」。设置保存后立即生效并写入数据库。已有数据库中的对应设置优先于环境变量，因此修改 Compose 后，页面中的旧设置仍可能继续生效。

环境变量用于启动配置；修改后执行 `docker compose up -d` 重建容器。仅重启容器不会更新环境变量。首次访问未初始化的实例时会进入设置向导，已通过环境变量配置完整登录账户时直接进入登录页。

## 基础环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `DOWNLOAD_MODE` | Docker：`builtin`；源码：`direct` | 下载方式，见下表 |
| `DOWNLOAD_DIR` | `/data/downloads` | 默认归档目录 |
| `LIBRARY_DIR` | 空 | 媒体库根目录；空时使用 `DOWNLOAD_DIR`，单部番的存储位置优先 |
| `DB_PATH` | `/data/auto-anime.db` | SQLite 数据库路径 |
| `COVERS_DIR` | `/data/covers` | 本地封面缓存目录 |
| `PORT` | `8000` | Web 监听端口；修改时同步调整端口映射和健康检查 |
| `POLL_MINUTES` | `30` | RSS 同步周期，单位分钟，最小 5 |
| `DOWNLOADER_POLL_SECONDS` | `10` | 下载器进度轮询间隔，单位秒，最小 5 |
| `PROXY_URL` | 空 | 默认直连，代理作用范围见下文 |
| `BANGUMI_ENABLED` | `true` | 设为 `false` 关闭自动封面识别 |
| `BANGUMI_API` | `https://api.bgm.tv` | Bangumi API 地址 |
| `WEB_USER` / `WEB_PASSWORD` | 空 | 同时配置才启用登录，也可在向导或设置页配置 |
| `WEBHOOK_URL` | 空 | 接收事件通知的 HTTP(S) 地址 |

| `DOWNLOAD_MODE` | 行为 |
| --- | --- |
| `builtin` | 镜像内置 qBittorrent 处理磁力与种子，普通 HTTP(S) 文件由本程序下载 |
| `direct` | 本程序下载 HTTP(S) 文件，磁力与种子条目进入「等待下载器」 |
| `qbittorrent` | 连接外部 qBittorrent（4.4+） |
| `aria2` | 连接外部 Aria2 |
| `transmission` | 连接外部 Transmission |

切换默认下载器影响后续提交，已有任务仍按原下载器跟踪，不会自动转移。内置与外部 qBittorrent 使用独立配置；只要保留内置任务记录，重启后仍会启动内置下载器继续跟踪和做种。

## 内置 qBittorrent

Docker 部署无需填写下载器连接信息。源码运行时需自行安装下载器，程序会为它创建独立配置，不使用桌面客户端的默认配置。

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

## 网络与封面

本程序默认显式直连，不读取系统代理。可通过设置页或 `PROXY_URL` 配置 `http://`、`https://`、`socks5://`、`socks5h://` 代理，例如：

```text
http://192.168.1.10:7890
socks5://192.168.1.10:1080
http://用户名:密码@192.168.1.10:7890
```

- **使用代理**：RSS 搜索、预览与同步，HTTP(S) 文件和种子获取，HTTP(S) tracker 探测，Bangumi 请求及 Webhook。
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

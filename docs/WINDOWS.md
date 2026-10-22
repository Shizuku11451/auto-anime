# Windows 桌面版

[文档索引](README.md) · [使用指南](USAGE.md) · [维护与备份](MAINTENANCE.md)

Windows 是次要适配平台，保留桌面便携包、基础回归与打包检查；主要维护和发布验收重点为飞牛 NAS，同时提供 Linux 服务端部署。完整范围见 [README 平台适配范围](../README.md#平台适配范围)，具体版本的可安装性仍以实际制品验收为准。

适用于 Windows 10/11 x64。桌面版使用与容器版相同的 Go 服务、Vue 界面和下载归档逻辑，窗口由 Microsoft Edge WebView2 Runtime 渲染。系统未安装该运行时时，启动会提示安装；可使用微软提供的离线安装包，不要求使用代理。

## 启动与后台运行

1. 目标 Release 提供单文件 `auto-anime-<版本>-windows-amd64.exe` 和包含回退下载器的 `-desktop.zip`。单文件已自带纯 Go BT 下载器，直接放到有写权限的目录即可；使用 ZIP 时请完整解压，例如 `D:\Auto-Anime`。
2. 双击 `Auto-Anime.exe`，在独立窗口中完成初始化。首次运行默认使用「内置 BT 下载器」，不需要 `qbittorrent\` 目录；已保存的数据库设置优先。需要旧版内置 qBittorrent 时，选择「内置 qBittorrent」并确保程序目录存在 `qbittorrent\`，或设置 `QBITTORRENT_PATH`。
3. 关闭窗口后保留系统托盘图标并继续下载。双击托盘图标重新打开，右键选择「退出并停止下载器」才会结束程序和内置下载器。窗口菜单（`Alt+Space`）也提供「退出 Auto-Anime」。退出可能等待下载器保存会话。

桌面服务仅监听 `127.0.0.1`，默认端口 `8000`；也可用浏览器访问 `http://127.0.0.1:8000`。端口被占用时显示错误，可通过环境变量 `PORT` 更换。Windows 未设置 `DOWNLOAD_MODE` 时默认使用内置 BT 下载器；`Auto-Anime.exe --server` 使用原有服务入口，源码 `go run .` 默认使用服务入口，`go run . --desktop` 打开桌面窗口。

桌面版不提供开机启动或 Windows 服务注册。单文件 `.exe` 用于程序更新或已有部署，内置下载器随程序一起更新；`-desktop.zip` 额外保留 qBittorrent 回退，二者共用同一份 `data\`。

## 数据与文件系统

- 默认数据保存在程序同级的 `data\`：数据库 `auto-anime.db`、直链暂存与归档回退 `downloads\`、封面 `covers\`、窗口缓存 `webview2\`、桌面日志 `desktop.log`。
- 未设置 `LIBRARY_DIR`/界面「媒体库目录」时，归档使用程序同级的 `media\`（与 `data\` 分开），媒体服务器扫描这个目录即可；清空界面里的媒体库目录会回退到 `downloads\`。
- 默认「内置 BT 下载器」使用 `data\bt\`：`session.db` 保存会话与续传信息，新任务在 `downloads\<infohash-条目ID>\` 下保存种子内的文件，旧 `<infohash>` 目录继续兼容。相同种子的不同条目独立控制。端口默认从 TCP `6881` 起监听 10 个端口，DHT 使用 UDP `6881`；可在启动前用 `NATIVE_BT_PORT`、`NATIVE_BT_PORT_RANGE`、`NATIVE_DHT_PORT` 更换。首次运行时 Windows 防火墙可能询问，请允许专用网络。
- 旧版「内置 qBittorrent」使用 `data\qbittorrent\` 下的独立配置、凭据、下载源文件和会话，不接管电脑上其他 qBittorrent 的任务；它作为回退保留，不影响单文件版。不要删除这些目录后期待恢复下载任务。
- 媒体库和订阅保存位置只接受完整绝对路径：盘符路径，例如 `D:\番剧`，或 UNC 网络共享，例如 `\\NAS\动画\番剧`。`/番剧`、`\番剧` 这类「当前盘根相对」写法会被拒绝，避免文件落到用户没预期的盘符；从 Linux/容器数据库搬来的 `/vol1/...` 位置会在启动时提示，需要手动改成盘符或 UNC。共享必须已经可访问且当前 Windows 用户有写权限；程序不会代为保存共享登录密码。
- 归档沿用 `<根目录>\<番剧名>\Season XX\…`，清理非法字符、尾部空格/点与 Windows 设备名称。外部及内置下载器归档复制后默认保留源文件，关闭保留开关后按安全条件清理；已有媒体不会自动迁移。内置 BT 下载器会在开始写文件前拒绝含 `..`、反斜杠、盘符、Windows 保留名等不安全路径的种子。
- Windows 直链归档遇到不支持硬链接的文件系统时，回退为拒绝覆盖重名文件的原生移动；适用于 exFAT 和部分 SMB 共享，具体共享权限仍由系统决定。
- 长路径仍受 Windows、下载器和共享文件系统的实际限制；建议选择较短的根目录。不能仅凭本机测试保证 NAS、断网共享或长路径兼容。
- 外部 Aria2/qBittorrent/Transmission 在 Windows 上返回反斜杠路径；映射比较会先统一分隔符（Windows 上同时忽略大小写）再计算本地路径，用户填写的映射前缀可以混用 `/` 与 `\`。
- Docker 专用的 `/install` 部署配置页在 Windows 上返回 404，设置页与首次向导不显示该入口；修改媒体目录直接填盘符或 UNC 路径。
- 发布包是 GUI 子系统：`Auto-Anime.exe --server` 或 `-version` 从终端运行时程序会附着到已有控制台，让日志和错误可见；双击启动不会新开控制台窗口。输出重定向到文件或管道时保持原样。

BT 端口数限制同时驻留任务，不限制累计下载数量。取消会释放会话与端口但保留源文件；端口不足时暂停任务、历史做种可让出资源，续传信息不丢失，新下载优先。被卸载的历史做种本轮不会自动恢复；重启后在容量允许时恢复。旧版若多个条目共用一个任务，请按提示移除重复记录并保留媒体，再独立控制剩余任务。完整行为见[配置参考](CONFIGURATION.md#内置-bt-下载器默认)。

## 窗口外观与图标

- 桌面窗口声明 Per-Monitor V2 DPI 感知（`backend/rsrc_windows_amd64.syso` 里的清单，启动时另有兜底设置），窗口按系统 DPI 换算物理像素并限制在主屏工作区内：125%/150% 缩放下文字不发虚。`desktop.log` 会打印实际生效的 DPI 感知级别、系统 DPI 与初始尺寸，排查显示问题时先看这一行。
- 原生标题栏跟随应用主题：启动时取系统「应用模式」，之后由界面在切换亮暗或系统主题变化时调用 `/api/desktop/theme` 同步（服务模式返回 `applied=false`，不影响其他接口）。
- 程序使用与飞牛桌面一致的蓝紫渐变圆角矩形星芒图标，统一原图为 `backend/assets/app-icon.png`，网页横版 Logo 保持不变。Windows 多尺寸图标 `backend/assets/auto-anime.ico` 由 `go-winres` 预生成的 `backend/rsrc_windows_amd64.syso` 内嵌（资源 ID 1），同时用于 exe 文件图标、标题栏、任务栏与系统托盘；资源缺失时回退系统默认图标。更新图标（需要 Pillow 与联网下载 go-winres）：

```powershell
python tools/make_windows_icon.py   # 同时生成飞牛 PNG 与 Windows 多尺寸 ico
cd backend
go generate ./...                  # 重新生成 rsrc_windows_amd64.syso
```

更新图标后须重新构建程序并完全退出旧实例；已固定的任务栏快捷方式可能仍显示 Windows 缓存的旧图标，可取消固定后重新固定。不影响数据、媒体目录或启动配置。

`DB_PATH`、`DOWNLOAD_DIR`、`COVERS_DIR`、`LIBRARY_DIR`、`NATIVE_BT_DATA_DIR`、`QBITTORRENT_DATA_DIR` 均可通过环境变量覆盖。搬迁便携包前先正常退出；不自动搬迁早期版本的盘根 `C:\data` 或 `E:\data`。同一数据库只运行一个实例，新版 Windows 服务和桌面入口共用数据库锁；不要同时运行不带该锁的旧版本。

例如在 PowerShell 中指定独立媒体库后启动（已保存的界面设置优先）：

```powershell
$env:LIBRARY_DIR = 'D:\番剧'
$env:PORT = '8000'
& 'D:\Auto-Anime\Auto-Anime.exe'
```

## 构建与更新

先在 `web/` 执行 `npm ci` 和 `npm run build`，再在 `backend/` 构建，保持 `CGO_ENABLED=0`：

```powershell
$env:CGO_ENABLED = '0'
go build -ldflags '-H windowsgui -X main.desktopDefault=true' -o ..\dist\Auto-Anime.exe .
```

窗口使用 Go 依赖 `github.com/jchv/go-webview2`（窗口与 WebView2 接口）及间接依赖 `github.com/jchv/go-winloader`（加载 WebView2 loader）；内置 BT 引擎使用 `github.com/cenkalti/rain`（MIT，纯 Go）。版本均由 `go.mod`/`go.sum` 固定。Windows 文件由构建标签隔离，Linux/NAS 继续使用原服务入口。

发布脚本 `tools/package_windows.py <Windows exe>` 需要 Python 3 和 `7z`，下载固定版本 qBittorrent 官方安装包并校验 SHA-256，只提取程序文件，不执行安装器、不修改系统关联；这只用于桌面包的可选回退。压缩包同时携带说明、`THIRD-PARTY-NOTICES.md` 及第三方许可/源码资料。构建与打包需要访问 Go 模块源、GitHub 和 SourceForge；解压后的初始化和使用不需要再次下载 qBittorrent 或加载境外 CDN。

一键更新更新 Auto-Anime 主程序（含嵌入前端和内置 BT 下载器），Windows 暂存后在下次启动应用；它不更新桌面包里的 qBittorrent 回退。需要更新回退时请正常退出后完整解压新版桌面包，保留 `data\`。版本检查不可达不影响基础订阅和下载。

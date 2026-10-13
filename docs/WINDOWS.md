# Windows 桌面版

[文档索引](README.md) · [使用指南](USAGE.md) · [维护与备份](MAINTENANCE.md)

Windows 是次要适配平台，保留桌面便携包、基础回归与打包检查；主要维护和发布验收重点为飞牛 NAS，同时提供 Linux 服务端部署。完整范围见 [README 平台适配范围](../README.md#平台适配范围)，具体版本的可安装性仍以实际制品验收为准。

适用于 Windows 10/11 x64。桌面版使用与容器版相同的 Go 服务、Vue 界面和下载归档逻辑，窗口由 Microsoft Edge WebView2 Runtime 渲染。系统未安装该运行时时，启动会提示安装；可使用微软提供的离线安装包，不要求使用代理。

## 启动与后台运行

1. 先确认目标 Release 已包含桌面附件，再下载其中的 `auto-anime-<版本>-windows-amd64-desktop.zip`，完整解压到有写权限的目录，例如 `D:\Auto-Anime`。
2. 双击 `Auto-Anime.exe`，在独立窗口中完成初始化。随包的 `qbittorrent\` 必须与程序放在一起；首次运行默认使用内置下载器，已保存的数据库设置优先。
3. 关闭窗口后保留系统托盘图标并继续下载。双击托盘图标重新打开，右键选择「退出并停止下载器」才会结束程序和内置下载器。窗口菜单（`Alt+Space`）也提供「退出 Auto-Anime」。退出可能等待下载器保存会话。

桌面服务仅监听 `127.0.0.1`，默认端口 `8000`；也可用浏览器访问 `http://127.0.0.1:8000`。端口被占用时显示错误，可通过环境变量 `PORT` 更换。`Auto-Anime.exe --server` 使用原有服务入口；源码 `go run .` 默认使用服务入口，`go run . --desktop` 打开桌面窗口。

桌面版不提供开机启动或 Windows 服务注册。独立 `.exe` 下载项用于程序更新或已有部署；要获得随包下载器，请下载并完整解压 `-desktop.zip`。

## 数据与文件系统

- 默认数据保存在程序同级的 `data\`：数据库 `auto-anime.db`、归档 `downloads\`、封面 `covers\`、窗口缓存 `webview2\`、桌面日志 `desktop.log`。
- 内置下载器使用 `data\qbittorrent\` 下的独立配置、凭据、下载源文件和会话，不接管电脑上其他 qBittorrent 的任务。不要删除这个目录后期待恢复下载任务。
- 媒体库和订阅保存位置支持盘符绝对路径，例如 `D:\番剧`，以及 UNC 网络共享，例如 `\\NAS\动画\番剧`。共享必须已经可访问且当前 Windows 用户有写权限；程序不会代为保存共享登录密码。
- 归档沿用 `<根目录>\<番剧名>\Season XX\…`，清理非法字符、尾部空格/点与 Windows 设备名称。外部及内置下载器归档复制后默认保留源文件，关闭保留开关后按安全条件清理；已有媒体不会自动迁移。
- Windows 直链归档遇到不支持硬链接的文件系统时，回退为拒绝覆盖重名文件的原生移动；适用于 exFAT 和部分 SMB 共享，具体共享权限仍由系统决定。
- 长路径仍受 Windows、下载器和共享文件系统的实际限制；建议选择较短的根目录。不能仅凭本机测试保证 NAS、断网共享或长路径兼容。

`DB_PATH`、`DOWNLOAD_DIR`、`COVERS_DIR`、`LIBRARY_DIR`、`QBITTORRENT_DATA_DIR` 均可通过环境变量覆盖。搬迁便携包前先正常退出；不自动搬迁早期版本的盘根 `C:\data` 或 `E:\data`。同一数据库只运行一个实例，新版 Windows 服务和桌面入口共用数据库锁；不要同时运行不带该锁的旧版本。

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

窗口使用 Go 依赖 `github.com/jchv/go-webview2`（窗口与 WebView2 接口）及间接依赖 `github.com/jchv/go-winloader`（加载 WebView2 loader）；版本由 `go.mod`/`go.sum` 固定。Windows 文件由构建标签隔离，Linux/NAS 继续使用原服务入口。

发布脚本 `tools/package_windows.py <Windows exe>` 需要 Python 3 和 `7z`，下载固定版本 qBittorrent 官方安装包并校验 SHA-256，只提取程序文件，不执行安装器、不修改系统关联。压缩包携带说明及第三方许可/源码资料。构建与打包需要访问 Go 模块源、GitHub 和 SourceForge；解压后的初始化和使用不需要再次下载 qBittorrent 或加载境外 CDN。

一键更新仅更新 Auto-Anime 主程序（含嵌入前端），Windows 暂存后在下次启动应用；它不更新随包 qBittorrent。下载器升级请正常退出后完整解压新版桌面包，保留 `data\`。版本检查不可达不影响基础订阅和下载。

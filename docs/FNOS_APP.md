# 飞牛 fnOS 应用包（.fpk）

[返回 README](../README.md) · [飞牛 Compose 安装](FNOS.md) · [完整配置参考](CONFIGURATION.md)

飞牛 fnOS 是主适配平台，FPK 是优先交付方式，Compose 为备用部署方式。当前实机测试范围为 x86-64 飞牛；其他架构须单独验收，不因 Linux ARM64 制品存在而默认视为已适配。

应用包采用飞牛官方[应用开放平台](https://developer.fnnas.com/)格式，提供安装向导、桌面入口与启停控制。尚未上架应用中心；正式发布后从 GitHub Release 获取 `.fpk` 并手动安装，无需编译源码或 SSH 预先构建镜像。安装前检查对应 Release 是否确实包含 FPK 和公开镜像；工作流设计或本地测试包不代表已经发布。

从 v0.0.2 起，正式 FPK 自带同版本 Linux amd64 镜像，仅面向 x86-64 飞牛；安装时校验 SHA-256 并通过 `docker load` 导入，禁止联网拉取。前提是飞牛 Docker 已安装并启动、网页及 BT 端口可用、存储空间可写。获取 FPK 文件仍需访问 GitHub 或自行传输，解包和导入需要额外磁盘空间；RSS、元数据等运行功能仍需要网络。旧版及手工构建的在线 FPK 仍需 NAS 访问 GHCR。

## 安装与目录选择

1. 在「应用中心 → 手动安装」选择 `.fpk`，填写 BT 端口段（默认 `6881-6890`，必须是 10 个连续端口）和保留原文件做种（默认开启）。端口段不能包含网页端口 8000。
2. 安装回调校验并导入包内同版本镜像；再次启动也会从本地包导入，无需下载。镜像导入完成后，最多等待 120 秒确认容器健康启动，再点击桌面入口完成网页初始化。镜像校验与导入不包含在这 120 秒内。默认整理位置是应用共享目录 `auto-anime/media`，容器内为 `/media`。
3. 想用已有番剧目录，在「应用设置 → 访问权限」授权具体文件夹，例如 `/vol1/1000/ani`，选择读写权限。
4. 停止并启动应用，宿主脚本读取最新授权列表并重新创建容器挂载。
5. 在网页「设置 → 媒体库目录」填写同一个绝对路径，点击「检测保存目录」。检测会创建、写入、同步并删除临时文件；保存目录变更前也会检测，失败整份设置不保存。

归档仍使用 `<根目录>/<番剧名>/Season XX/…`。修改根目录只影响后续归档，不迁移旧媒体。订阅或番剧已指定的保存位置优先于媒体库默认目录。

支持存储空间下的具体授权目录，不接受卷根、飞牛系统/应用目录及软链路径。新增或撤销授权后应重启应用同步挂载；不会创建缺失的宿主目录。跨存储空间的真实授权仍需对应设备验收。

默认共享目录通过 `permission.ro: ["trim.photos"]` 给飞牛影视只读权限。自定义目录还需在影视自身的「访问权限」授权，Auto-Anime 的授权不会自动授予影视或其他媒体服务器。

网页设置可修改做种策略，没有 BT 端口修改入口。内置 BT 引擎一个任务占一个端口，默认连续监听 10 个（`NATIVE_BT_PORT_RANGE`），宿主机侧必须映射同样多的端口；Compose 手工部署改端口需同时改 TCP/UDP 两行端口映射与 `NATIVE_BT_PORT`。FPK 使用安装向导提供的端口段，不建议直接修改飞牛管理的内部模板，重新安装前必须备份并核对授权。

安装或启动失败时，应用中心会收到分类中文提示：镜像访问被拒绝、版本/架构缺失、仓库网络失败、端口占用、存储/权限异常、Docker 不可用或容器健康失败。完整 Docker 输出在 `/var/log/apps/auto-anime.log`，弹窗不复制原始输出或凭据。网络错误先检查 NAS 的 DNS 与镜像获取配置，网页代理不能帮助 Docker 拉取；不要通过删除数据库或媒体目录处理启动失败。分享日志前检查并脱敏。

## 权限与容器生命周期

飞牛通过应用用户 ACL 控制文件访问，并把授权路径放入 `TRIM_DATA_ACCESSIBLE_PATHS`。宿主生命周期脚本以 root 调用 Docker，在 `TRIM_PKGETC/authorized-mounts.json` 生成具体授权目录挂载；该文件不进入容器。基础挂载只有应用数据 `/data` 与默认媒体 `/media`。

容器使用 `${TRIM_UID}:${TRIM_GID}`，移除全部 capabilities，并启用 `no-new-privileges`。不挂载整个存储空间、宿主应用包、配置目录或 Docker socket。Docker 启停由 `cmd/main` 管理，不再声明 `docker-project` 让应用中心重复管理。

飞牛安装、升级可能将应用数据目录属主重置为 root。安装、升级以及运行中的配置回调会先将 `TRIM_PKGVAR` 的属主恢复为应用用户；无效用户、缺失目录或属主修改失败时，向应用中心错误日志报告中文原因并以非零状态退出，不继续启动或重建容器。遇到失败应先检查数据卷是否只读、数据目录权限及系统传入的应用用户配置，再重试；不会通过改为 root 容器绕过问题。

非 root 容器在 `/data/runtime/<镜像程序 SHA-256>` 建立权限 700 的私有运行目录，复制程序与静态资源，镜像 `/app` 保持只读。同一镜像重启保留自更新程序；换镜像使用新副本。历史运行目录暂不自动删除，备份整个数据目录时包含它们。

| 文件 | 用途 |
| --- | --- |
| `fpk/manifest` | 桌面入口、服务端口和显示「访问权限」 |
| `fpk/config/resource` | 声明默认媒体共享目录与影视只读权限 |
| `fpk/config/privilege` | 宿主脚本以 root 管理 Docker，服务在非 root 容器运行 |
| `fpk/app/docker/docker-compose.yaml` | 基础编排、非 root 身份、权限限制及数据/媒体挂载 |
| `fpk/cmd/storage` | 恢复应用数据目录属主、校验授权路径、生成 Compose 覆盖文件 |
| `fpk/cmd/main` | 创建、停止、检查容器 |
| `fpk/cmd/config_callback` | 运行中配置变更时同步挂载；用户也可停止再启动 |
| `fpk/wizard/install`、`fpk/cmd/install_init` | 安装向导与端口校验 |
| `scripts/docker-entrypoint.sh` | 两个镜像构建入口共用的非 root 运行目录初始化 |

## 构建与分发

桌面入口与应用中心使用同一套蓝紫渐变圆角矩形星芒图标，与 Windows 桌面版一致；网页横版 Logo 不变。维护者修改 `backend/assets/app-icon.png` 后，在仓库根目录执行 `python tools/make_windows_icon.py`（需要 Pillow），统一生成 `ICON.PNG`、`ICON_256.PNG` 和 `app/ui/images/icon_64.png`、`icon_256.png`，再重新打包。已有安装需升级含新图标的 FPK 并刷新飞牛桌面后才会使用新资源；单独更新服务端镜像不会替换 FPK 图标。

以下从仓库根目录运行，`1.2.0` 仅是示例，应替换为实际版本。

```powershell
pwsh deploy/fnos/build-fpk.ps1 -Version 1.2.0
```

```bash
deploy/fnos/build-fpk.sh 1.2.0
```

构建机需要 Python 3（仅维护者打包需要，NAS 用户不需要）。脚本下载官方 fnpack 1.2.3、校验 SHA-256，在临时目录替换版本与镜像标签并打包至 `dist/fnos/`，不修改源文件。随后校验官方包的 `app.tgz` MD5、路径与文本格式，统一生命周期脚本为 755、普通文件为 644、目录为 755，并重算包内 checksum，避免 Windows 打包丢失 Linux 执行权限。

发布流程先推同版本镜像并通过无凭据匿名拉取及双架构非 root 启动检查，再更新 latest，随后在 GitHub 构建机导出 amd64 镜像并使用同一脚本生成自带镜像的正式 FPK，最后公开 Release；首次 GHCR 包需维护者设置 Public，验证失败则停止发布，调整后重跑已有 tag。仅构建 FPK 不会发布镜像，完整流程见[配置参考](CONFIGURATION.md#维护者发布流程)。fork 发布自动使用自己的 GHCR 仓库；手动打包可用 PowerShell `-ImageRepository ghcr.io/owner/repo` 或 Bash 环境变量 `IMAGE_REPOSITORY` 覆盖默认仓库。

本地仅验证包安装与生命周期时可用 `-ImageTag latest`。这不会把未发布的 Go、前端或入口脚本送进旧镜像，不能据此宣称新功能已上线。

命令行安装需要 root：

```bash
printf 'wizard_bt_ports=6881-6890\nwizard_keep_source=true\n' > /tmp/aa-install.env
sudo appcenter-cli install-fpk --volume 1 --env /tmp/aa-install.env /vol1/1000/auto-anime-1.2.0-fnos.fpk
sudo appcenter-cli status auto-anime
sudo appcenter-cli start auto-anime
```

实测 `appcenter-cli 1.0.2 install-fpk` 对已安装应用直接跳过，连更高版本也不会更新。不能把返回 0 当成升级成功，必须核实 manifest 与实际容器挂载。不能据此推断图形界面升级结果。若选择保留数据的卸载重装，先备份数据库、下载器数据和媒体，并记录自定义目录授权，重装后重新检查。

## 验证边界

### 自带镜像试验

Actions 的 `FPK offline test` 手动流程生成独立应用 `auto-anime-offline`，网页端口 18015、BT 默认 16895-16904，仅针对 x86-64 飞牛。包内携带正式 `0.0.1` 的 amd64 镜像，不代表新程序版本；安装校验 SHA-256 后通过 `docker load` 导入，使用 `pull_policy: never` 禁止联网拉取。仍需预装并启动飞牛 Docker，包下载、解包及镜像导入需要额外空间。该流程不替换正式 FPK 或发布镜像，该独立试验包已构建成功并获用户安装反馈；正式新版本安装与升级仍需逐版本验收。

Linux 打包脚本可通过 `IMAGE_ARCHIVE` 指定对应镜像引用的 `docker save` gzip 文件；未指定时保持原在线包行为。当前 PowerShell 打包入口尚未提供该选项。自带镜像在应用目录内保留以供再次启动导入，不属于数据库或媒体备份。

正式包保持 `auto-anime` 标识、网页端口 8000、默认 BT 端口段 6881-6890。它与 `auto-anime-offline` 测试应用数据独立，不会自动迁移测试应用数据；两者共用媒体前须核实目录与写入任务。已有正式应用升级前备份数据，图形升级尚需验收。

生命周期脚本、包权限、checksum 和公开镜像门槛有自动化测试；这不代表正式 GitHub Release、无缓存图形安装或升级已验收。安装前检查实际 Release 的说明与附件，不使用内部 `installcheck` 或 `review` 测试包作为正式制品。

跨存储空间 ACL、图形升级、不同 NAS 架构与真实下载归档须在对应设备和版本验证。本页不保存现场端口、进程和逐轮操作日志。

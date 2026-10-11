# 飞牛 fnOS 应用包（.fpk）

[返回 README](../README.md) · [飞牛 Compose 安装](FNOS.md) · [完整配置参考](CONFIGURATION.md)

应用包采用飞牛官方[应用开放平台](https://developer.fnnas.com/)格式，提供安装向导、桌面入口与启停控制。尚未上架应用中心，手动安装适用于本地测试。镜像仓库必须公开：私有 GHCR 镜像会让安装以 `code 10266 unauthorized` 失败。

## 安装与目录选择

1. 在「应用中心 → 手动安装」选择 `.fpk`，填写 BT 监听端口（默认 6881）和保留原文件做种（默认开启）。BT 端口不能与网页端口 8000 相同。
2. 安装回调启动容器，点击桌面入口完成网页初始化。默认整理位置是应用共享目录 `auto-anime/media`，容器内为 `/media`。
3. 想用已有番剧目录，在「应用设置 → 访问权限」授权具体文件夹，例如 `/vol1/1000/ani`，选择读写权限。
4. 停止并启动应用，宿主脚本读取最新授权列表并重新创建容器挂载。
5. 在网页「设置 → 媒体库目录」填写同一个绝对路径，点击「检测保存目录」。检测会创建、写入、同步并删除临时文件；保存目录变更前也会检测，失败整份设置不保存。

归档仍使用 `<根目录>/<番剧名>/Season XX/…`。修改根目录只影响后续归档，不迁移旧媒体。订阅或番剧已指定的保存位置优先于媒体库默认目录。

支持存储空间下的具体授权目录，不接受卷根、飞牛系统/应用目录及软链路径。新增或撤销授权后应重启应用同步挂载；不会创建缺失的宿主目录。跨存储空间的真实授权仍需对应设备验收。

默认共享目录通过 `permission.ro: ["trim.photos"]` 给飞牛影视只读权限。自定义目录还需在影视自身的「访问权限」授权，Auto-Anime 的授权不会自动授予影视或其他媒体服务器。

网页设置可以修改做种策略，目前没有 BT 端口修改入口。后续改端口需同时更新 Compose 的端口映射与 `QBITTORRENT_BT_PORT`，再重新创建容器。

## 权限与容器生命周期

飞牛通过应用用户 ACL 控制文件访问，并把授权路径放入 `TRIM_DATA_ACCESSIBLE_PATHS`。宿主生命周期脚本以 root 调用 Docker，在 `TRIM_PKGETC/authorized-mounts.json` 生成具体授权目录挂载；该文件不进入容器。基础挂载只有应用数据 `/data` 与默认媒体 `/media`。

容器使用 `${TRIM_UID}:${TRIM_GID}`，移除全部 capabilities，并启用 `no-new-privileges`。不挂载整个存储空间、宿主应用包、配置目录或 Docker socket。Docker 启停由 `cmd/main` 管理，不再声明 `docker-project` 让应用中心重复管理。

飞牛安装、升级可能将应用数据目录属主重置为 root。安装、升级以及运行中的配置回调会先将 `TRIM_PKGVAR` 的属主恢复为应用用户；无效用户、缺失目录或属主修改失败时，向应用中心错误日志报告中文原因并以非零状态退出，不继续启动或重建容器。遇到失败应先检查数据卷是否只读、数据目录权限及系统传入的应用用户配置，再重试；不会通过改为 root 容器绕过问题。没有新增向导选项或环境变量。

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

```powershell
pwsh deploy/fnos/build-fpk.ps1 -Version 1.2.0
```

```bash
deploy/fnos/build-fpk.sh 1.2.0
```

脚本下载官方 fnpack 1.2.3、校验 SHA-256，在临时目录替换版本与镜像标签并打包至 `dist/fnos/`，不修改源文件。发布流程使用同一脚本，发布包应配合对应版本的公开镜像。

本地仅验证包安装与生命周期时可用 `-ImageTag latest`。这不会把未发布的 Go、前端或入口脚本送进旧镜像，不能据此宣称新功能已上线。

命令行安装需要 root：

```bash
printf 'wizard_bt_port=6881\nwizard_keep_source=true\n' > /tmp/aa-install.env
sudo appcenter-cli install-fpk --volume 1 --env /tmp/aa-install.env /vol1/1000/auto-anime-1.2.0-fnos.fpk
sudo appcenter-cli status auto-anime
sudo appcenter-cli start auto-anime
```

实测 `appcenter-cli 1.0.2 install-fpk` 对已安装应用直接跳过，连更高版本也不会更新。不能把返回 0 当成升级成功，必须核实 manifest 与实际容器挂载。图形界面升级尚未验收。若选择保留数据的卸载重装，先备份数据库、下载器数据和媒体，并记录自定义目录授权，重装后重新检查。

## 实际验证与边界（2026-10-11）

- 飞牛独立测试应用 `auto-anime-review` 安装成功，非 root 容器 healthy，内置 qBittorrent 启动正常。应用中心停止、启动命令正常，仅挂载自身 `/data` 与 `/media`，宿主 `@appcenter`、`@appconf` 不可见。测试后已卸载应用并移除容器。
- 授权目录 Compose 经飞牛 `docker compose config` 解析成功，生成具体目录挂载；拒绝系统应用目录，移除授权时覆盖文件不保留旧目录。自定义媒体目录实际 ACL 读写在此前的真机验证中通过，新实现保留此应用用户身份。
- 隔离容器采用新的入口脚本与本轮编译程序，镜像 `/app` 不可写，运行目录为 `/data/runtime/…`。校验过的暂存更新 `0.1.8-review → 0.1.9-review` 成功替换并重启，再次重启仍保留新版本。
- 设置页真实浏览器验证目录检测成功、缺失目录失败、非法目录保存返回 400，其他设置不变。自动化测试覆盖未登录和跨来源拒绝、检测文件清理、请求取消及旧响应隔离。
- 后续在飞牛通过 Dockerfile 源码构建本地镜像成功，Linux fnpack 打包成功；独立「Auto-Anime 本地新版」应用安装在 18010，容器 healthy、内置 qBittorrent 就绪，默认 /media 检测 mapped/writable，安装向导浏览器访问成功。两个镜像构建入口明确程序与静态资源可读、入口脚本 755，避免非 root 启动依赖构建上下文的原始权限。该本地包依赖同一飞牛设备上的本地镜像，不代表公开可分发的 Release。
- 正式 GitHub 镜像发布、飞牛图形界面升级、跨存储空间授权、真实下载归档完整闭环仍未验收。历史整卷挂载验证不代表当前安全方案。

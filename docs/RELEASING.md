# 维护者发布说明

[文档索引](README.md) · [贡献指南](../CONTRIBUTING.md)

本文描述 `.github/workflows/release.yml` 的当前设计，不代表该工作区已经发布。用户安装步骤见平台指南。

## 平台与维护重点

- 适配目标为飞牛 fnOS、Linux 和 Windows。飞牛 NAS 为主适配平台，FPK 为优先交付方式、飞牛 Compose 为备用；发布验收重点是安装、初始化、目录授权、下载归档、重启与升级后的数据保留。
- Linux 提供 amd64 / arm64 Docker 镜像与独立服务程序；分别记录构建、模拟启动和实机结果，不把 Linux 支持等同于所有 NAS 品牌已验收。
- Windows 10/11 x64 为次要适配平台，保留单文件内置 BT 下载器、桌面 ZIP 的 qBittorrent 回退及基础回归与打包检查，不取消现有 Windows 发布和检查任务。
- 其他 NAS 品牌、macOS 与其他架构不属于当前专门适配承诺。适配目标不代表本次 Release 已通过对应平台安装验收，发布说明须列明实际验证与未验证项。

## 发布前

- 检查待提交文件、完整 Git 历史、日志与附件中的凭据、私人路径及资源授权。忽略规则不会清除历史。
- 按贡献指南运行前后端及工具验证，交付生成的前端和安装配置页。更新说明描述真实功能与兼容性变化。
- 确认 GitHub Release 权限和 GHCR 包可见性。仓库公开不自动使容器包公开。
- 发布说明分别记录源码验证、制品验证和 NAS 实机验收，不将 mock 或打包成功当成正式安装成功。

## 标签与流水线

推送 `v*` 标签触发发布，手动运行须指定已存在标签；构建检出该标签提交，不使用分支上之后的修改。

```bash
git tag -a v1.2.0 -m "此处填写实际更新说明"
git push origin v1.2.0
```

以上版本只是示例。充分长的标签注释用于 Release 说明，否则从提交历史生成；当前脚本以注释长度至少 40 字符为判断条件。

1. 调用 checks：前端测试/构建、Go 测试/vet/build、Windows 检查、工具和 FPK 脚本测试，以及指定配置并发测试的 race 检测。并非全量 race 或 NAS 验收。
2. 构建 Linux amd64、Linux arm64、Windows amd64 程序，注入版本、提交、时间与实际发布仓库。Windows 额外打包 qBittorrent 桌面 ZIP。
3. 在独立 `image-context/` 中使用本次构建程序、静态资源和发布 Dockerfile 构建双架构镜像，先推版本标签。
4. 使用全新无登录凭据的 Docker 配置验证匿名拉取、双架构程序版本、隔离非 root 启动、向导内置模式和 qBittorrent 进程。通过后更新 `latest`。
5. 在构建机拉取已验证的同版本 Linux amd64 镜像，检查架构并导出 gzip 镜像归档，生成 x86-64 自带镜像 FPK（正式应用标识 `auto-anime`，`pull_policy: never`）。
6. 生成 `checksums.txt`，创建或上传 Release 附件。若 Release 已存在，当前流程上传覆盖附件，不自动重写原有正文。

镜像门槛失败不会执行后续 Release 发布，但已推送的版本镜像可能仍存在。修正公开权限或网络问题后重跑原标签；重试不是撤销已发生的推送。

## 制品

推送到 `main` 后，`.github/workflows/fpk-test.yml` 自动构建 FPK 打包测试版，也可在 Actions 的 `FPK test build` 中手动运行。产物以 `fnos-fpk-test-<运行编号>` 保存 14 天，包含 FPK、SHA-256 和构建说明；不创建 Release、不构建或推送镜像。测试包版本为 `0.0.0-ci.<运行编号>`，引用现有 `latest` 镜像，因此不代表该提交的程序已部署，不能替代正式发布或 NAS 安装验收，不建议覆盖现有应用。

| 文件 / 镜像 | 内容 |
| --- | --- |
| `auto-anime-<版本>-linux-amd64` / `-linux-arm64` | 带嵌入前端的程序，不含独立下载器 |
| `auto-anime-<版本>-windows-amd64.exe` | Windows 主程序，内置纯 Go BT 下载器，不需要额外 qBittorrent |
| `auto-anime-<版本>-windows-amd64-desktop.zip` | 桌面程序、qBittorrent 回退、第三方许可与源码资料 |
| `auto-anime-<版本>-fnos.fpk` | 飞牛模板、向导、生命周期脚本及同版本 amd64 镜像（自 v0.0.2 起）；仍需预装 Docker |
| `THIRD-PARTY-NOTICES.md` | 内置 BT 引擎等第三方许可说明 |
| `checksums.txt` | Release 附件 SHA-256 |
| `ghcr.io/<owner>/<repo>:<版本>` / `:latest` | Linux amd64/arm64 Docker 镜像，内置 qBittorrent |

具体以 Release 实际附件为准。桌面 ZIP 打包需 Python、7z 和固定版本官方安装包，用于可选 qBittorrent 回退；只提取、不执行安装器。单文件与 ZIP 的默认下载方式都是内置 BT 下载器。FPK 使用官方 fnpack 并规范化权限与 checksum，构建方法见[FPK 指南](FNOS_APP.md#构建与分发)。

fork 工作流使用自身仓库的全小写镜像名及更新源，手动 Compose 示例不会自动改成 fork 地址，应主动调整。

## 交付验收

正式公开前至少核对：干净机器匿名获取镜像、Release 校验、Windows 完整解压启动、飞牛无缓存图形安装与升级、目录授权、暂停重启恢复、真实受授权资源下载归档和播放器。测试应使用独立应用、数据库、端口及媒体目录，不升级已有实例来代替隔离验收。

CI 的双架构模拟启动不能替代 ARM NAS 实机和跨存储空间权限验证。已存在的测试应用健康也不能证明新的 Release 和 FPK 能被普通用户安装。

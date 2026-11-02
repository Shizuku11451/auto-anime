# 快速开始

## 1. 安装

按你的平台选择一种安装方式：

| 平台 | 推荐方式 | 说明 |
| --- | --- | --- |
| 飞牛 fnOS | [FPK 应用包](/install/fnos-app) | 从 Release 下载 `.fpk`，在「应用中心 → 手动安装」导入。FPK 自带镜像，需预装 Docker |
| Linux / 其他 Docker 环境 | [Docker Compose](/install/docker) | 使用发布镜像 `ghcr.io/shizuku11451/auto-anime` |
| Windows 10/11 | [桌面版](/install/windows) | 下载单文件 `.exe` 或 `-desktop.zip`，放到有写权限的目录后双击运行 |

以 Docker Compose 为例：

```bash
git clone https://github.com/Shizuku11451/auto-anime.git
cd auto-anime
docker compose up -d
```

默认值：内置 BT 下载器，每 30 分钟同步，同时下载 2 个任务。数据保存在 `./docker-data`，容器内媒体目录为 `/data/downloads`；BT 占用 6881–6890 共 10 个端口。

## 2. 首次设置

打开 `http://NAS地址:8000`，按向导：

1. 创建管理员账户。
2. 确认下载方式（默认内置 BT 下载器）。
3. 确认保存目录和同步周期。

::: tip 媒体目录要先挂载
想把媒体归档到 NAS 上的其他文件夹，要先在 Compose 里添加 volume 挂载，再在网页里填写**容器内**的路径；只改网页设置不会新增挂载。详见[存储位置与目录映射](/guide/configuration#存储位置与目录映射)。
:::

## 3. 开始追番

1. **搜索**：在「搜索」输入番名，先确认 Bangumi 条目，再展开查看候选资源。
2. **订阅**：选中合适的版本，点击「订阅此版本」，核对预览、季数和保存位置后确认。之后按同步周期自动追更新集。
3. **跟进**：在「最近条目」查看下载进度。只有「已保存」才表示文件已归档完成。
4. **观看**：在「番剧库」浏览和播放，或让媒体服务器扫描归档目录。

搜索候选资源默认使用公共聚合服务 `https://search.115415.xyz`，无需令牌。使用时请求者 IP 和条目 ID 会发送给该服务。也可以[自建服务](/advanced/search-server)，或设置 `AGGREGATE_SEARCH_URL=""` 关闭。聚合服务或 Bangumi 不可用时，已有的 RSS 订阅和下载不受影响。

## 下一步

- [使用指南](/guide/usage)：关键词规则、集数偏移、识别纠错、批量管理、Bangumi 收藏同步。
- [配置参考](/guide/configuration)：环境变量、外部下载器、目录与代理。
- [维护与备份](/guide/maintenance)：更新、备份与故障排查。

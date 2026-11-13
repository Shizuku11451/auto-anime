# 快速开始

## 1. 安装

| 平台 | 安装方式 |
| --- | --- |
| 飞牛 fnOS | [FPK 应用包](/install/fnos-app)，需预装 Docker |
| Linux / Docker | [Docker Compose](/install/docker) |

以 Docker Compose 为例：

```bash
git clone https://github.com/Shizuku11451/auto-anime.git
cd auto-anime
docker compose up -d
```

数据保存在 `./docker-data`。端口与目录设置见 [Docker 安装](/install/docker)。

## 2. 首次设置

打开 `http://NAS地址:8000`，按向导创建管理员账户，确认下载方式、保存目录和同步周期。

::: tip 媒体目录要先挂载
使用 Docker 时，先挂载媒体目录，再在网页中填写**容器内路径**。详见[目录映射](/guide/configuration#存储位置与目录映射)。
:::

## 3. 开始追番

1. 在「搜索」找到番剧，选择版本并订阅。
2. 在「最近条目」查看进度，「已保存」表示归档完成。
3. 在「番剧库」播放，或用媒体服务器扫描归档目录。

资源搜索默认使用公共服务 `https://search.115415.xyz`，会向其发送请求者 IP 和条目 ID。可[自建服务](/advanced/search-server)或设置 `AGGREGATE_SEARCH_URL=""` 关闭；不影响已有 RSS 订阅和下载。

## 下一步

- [使用指南](/guide/usage)
- [配置参考](/guide/configuration)
- [维护与备份](/guide/maintenance)

# Docker / Linux

Linux amd64 / arm64 提供 Docker 镜像与独立服务端程序。飞牛 fnOS 用户请优先使用 [FPK 应用包](/install/fnos-app)。

## Docker Compose（推荐）

仓库根目录的 [`docker-compose.yml`](https://github.com/Shizuku11451/auto-anime/blob/main/docker-compose.yml) 默认直接使用发布镜像，无需源码构建：

```bash
git clone https://github.com/Shizuku11451/auto-anime.git
cd auto-anime
docker compose up -d
```

也可以只下载这一个文件后在其所在目录执行 `docker compose up -d`。

启动后打开 `http://NAS地址:8000`，按向导创建管理员账户并确认下载方式、保存目录和同步周期。

| 项目 | 默认值 |
| --- | --- |
| 镜像 | `ghcr.io/shizuku11451/auto-anime:latest` |
| 网页端口 | `8000`（冲突时可改为 `"18000:8000"`） |
| BT 端口 | `6881-6890` TCP/UDP，起始端口须与 `NATIVE_BT_PORT` 一致 |
| 数据目录 | `./docker-data` → 容器内 `/data` |
| 下载方式 | `DOWNLOAD_MODE=native`（内置 BT 下载器） |
| 源文件 | `KEEP_SOURCE_FILES=true`，归档后保留源文件做种 |

### 挂载媒体目录

把媒体归档到 NAS 的其他文件夹时，先在 Compose 中添加 volume，再在网页设置里填写**容器内**路径：

```yaml
services:
  auto-anime:
    volumes:
      - ./docker-data:/data
      - /vol1/1000/Anime:/media   # 左侧为 NAS 路径，按实际修改
    environment:
      LIBRARY_DIR: /media
```

修改 Compose 环境变量后需要重建容器（`docker compose up -d`），仅重启不会更新环境变量。网页中保存的设置优先于环境变量。外部下载器的目录映射见[配置参考 · 外部下载器](/guide/configuration#外部下载器)。

### 更新

```bash
docker compose pull
docker compose up -d
```

程序内一键更新不会替换 Docker 镜像，建议按上面的方式更新镜像。更新前的备份建议见[维护与备份](/guide/maintenance)。

### 从源码构建

目标版本没有发布镜像时，可以使用 `docker-compose.build.yml` 从源码构建，详见[参与开发 · 源码镜像构建](/develop/contributing#源码镜像构建)。

## 独立服务端程序

Release 提供 `auto-anime-<版本>-linux-amd64` 与 `-linux-arm64` 单文件程序，已嵌入网页前端。源码与独立程序默认使用直链下载方式，需要内置 BT 下载器时显式指定：

```bash
chmod +x auto-anime-*-linux-amd64
DB_PATH=$HOME/auto-anime/data/auto-anime.db \
DOWNLOAD_DIR=$HOME/auto-anime/downloads \
DOWNLOAD_MODE=native \
./auto-anime-*-linux-amd64
```

Linux 下未设置时数据默认位于 `/data`，普通用户运行应显式设置可写目录。程序启动时会打印实际使用的数据库、下载、封面与媒体库路径。全部环境变量见[配置参考](/guide/configuration#基础环境变量)。

::: warning
一个数据库只运行一个实例；只在可信局域网使用，不要把管理端口直接暴露到公网。
:::

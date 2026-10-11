#!/bin/sh
# 容器启动引导：
#   1. 记录镜像版本（若 /app 不可写，则把版本写到数据目录，供应用提示"更新未生效"）
#   2. 非 root 容器使用数据目录内的私有程序副本，支持一键更新
#   3. 实际启动应用
set -e

# 非 root 容器在持久化私有目录运行；镜像程序保持只读。以镜像摘要分目录，
# 同一镜像重启保留自更新程序，更换镜像则使用新镜像的程序与静态资源。
if [ "$(id -u)" != 0 ] && [ "${1:-}" = "./auto-anime" ]; then
  runtime_base="$(dirname "${DB_PATH:-/data/auto-anime.db}")/runtime"
  image_key="$(sha256sum /app/auto-anime | cut -d ' ' -f 1)"
  runtime_dir="$runtime_base/$image_key"
  mkdir -p "$runtime_base"
  chmod 700 "$runtime_base"
  if [ ! -d "$runtime_dir" ]; then
    runtime_tmp="$(mktemp -d "$runtime_base/.prepare-XXXXXX")"
    trap 'rm -rf "$runtime_tmp"' EXIT HUP INT TERM
    cp /app/auto-anime "$runtime_tmp/auto-anime"
    cp -R /app/static "$runtime_tmp/static"
    if [ -f /app/meta.env ]; then cp /app/meta.env "$runtime_tmp/meta.env"; fi
    chmod 700 "$runtime_tmp" "$runtime_tmp/auto-anime"
    mv "$runtime_tmp" "$runtime_dir"
    trap - EXIT HUP INT TERM
  fi
  cd "$runtime_dir"
fi

APP_DIR="$(pwd)"
DATA_DIR="${DB_PATH:-/data/auto-anime.db}"
DATA_DIR="$(dirname "$DATA_DIR")"

image_version() {
  if [ -f /app/meta.env ]; then
    sed -n 's/^version=//p' /app/meta.env | head -n 1
  fi
}

current_version() {
  "$APP_DIR/auto-anime" -version 2>/dev/null | head -n 1
}

mirror_version() {
  # /app 不可写时把镜像版本写到数据目录：应用启动时对比即可发现
  # "更新已下载但没能替换程序文件"（例如容器被重建回旧镜像层）。
  if mkdir -p "$DATA_DIR" 2>/dev/null; then
    echo "version=$(image_version)" >"$DATA_DIR/image-version" 2>/dev/null || true
  fi
}

if [ -f /app/meta.env ]; then
  recorded="$(cat "$DATA_DIR/image-version" 2>/dev/null | sed -n 's/^version=//p' | head -n 1)"
  running="$(current_version)"
  if [ -n "$recorded" ] && [ -n "$running" ] && [ "$recorded" != "$running" ]; then
    echo "[update] 运行的版本 $running 与镜像版本 $recorded 不一致：上次一键更新已替换程序文件，镜像本身未更新" >&2
  fi
fi

# 运行目录可写时可直接替换，否则保留暂存包并明确提示需要调整权限。
if ! ( : >"$APP_DIR/.auto-anime-write-probe" ) 2>/dev/null; then
  echo "[update] 程序目录不可写，更新仅能暂存；请调整程序目录权限或更新容器镜像" >&2
  mirror_version
fi
rm -f "$APP_DIR/.auto-anime-write-probe" 2>/dev/null || true

exec "$@"

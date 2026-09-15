#!/bin/sh
# 容器启动引导：
#   1. 记录镜像版本（若 /app 不可写，则把版本写到数据目录，供应用提示"更新未生效"）
#   2. 尽量让 /app 可写，使应用的一键更新能在容器内替换自身并重启
#   3. 实际启动应用
set -e

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

if ! ( : >/app/.auto-anime-write-probe ) 2>/dev/null; then
  chmod -R a+rwX /app 2>/dev/null || true
fi
rm -f /app/.auto-anime-write-probe 2>/dev/null || true

# 应用在容器内能替换 /app/auto-anime 时走直接替换；否则退回数据目录暂存，
# 由下次启动的引导步骤完成替换。
if ! ( : >/app/.auto-anime-write-probe ) 2>/dev/null; then
  echo "[update] /app 不可写，一键更新将在数据目录暂存并在下次重启时生效" >&2
  mirror_version
fi
rm -f /app/.auto-anime-write-probe 2>/dev/null || true

exec "$@"

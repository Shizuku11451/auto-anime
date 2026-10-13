#!/usr/bin/env bash
# 发布门槛：全新无凭据配置、双架构拉取、非 root 隔离容器健康启动。
set -euo pipefail
image="${1:?image reference required}"
version="${2:?version required}"
uid="$(id -u)"
gid="$(id -g)"
[ "$uid" -ne 0 ] || { echo '请使用非 root 用户运行发布验收（需要 Docker 权限）。' >&2; exit 1; }
tmp="$(mktemp -d)"
container=''
cleanup() {
  if [ -n "$container" ]; then docker rm -f "$container" >/dev/null 2>&1 || true; fi
  rm -rf -- "$tmp"
}
trap cleanup EXIT
export DOCKER_CONFIG="$tmp/docker"
mkdir -p "$DOCKER_CONFIG"
printf '{"auths":{}}\n' > "$DOCKER_CONFIG/config.json"

if ! docker manifest inspect "$image" > "$tmp/manifest.json"; then
  echo '正式镜像无法匿名访问。请先在 GHCR 包设置中将镜像设为 Public，再重跑发布；不能先交付 FPK。' >&2
  exit 1
fi
python3 - "$tmp/manifest.json" <<'PY'
import json, sys
with open(sys.argv[1], encoding='utf-8') as source:
    manifest = json.load(source)
platforms = {(m.get('platform', {}).get('os'), m.get('platform', {}).get('architecture')) for m in manifest.get('manifests', [])}
assert {('linux', 'amd64'), ('linux', 'arm64')} <= platforms, 'Release image must contain linux/amd64 and linux/arm64'
PY

for arch in amd64 arm64; do
  docker pull --platform "linux/$arch" "$image"
  running="$(docker run --rm --platform "linux/$arch" --network none --entrypoint /app/auto-anime "$image" -version)"
  case "$running" in "$version "* | "v$version "*) ;; *) echo "镜像程序版本不一致：$running" >&2; exit 1 ;; esac
  data="$tmp/data-$arch"
  mkdir "$data"
  chmod 700 "$data"
  container="auto-anime-release-$arch-$$"
  docker run -d --name "$container" --platform "linux/$arch" --network none \
    --user "$uid:$gid" --cap-drop ALL --security-opt no-new-privileges \
    --mount "type=bind,source=$data,target=/data" \
    -e DB_PATH=/data/auto-anime.db -e UPDATE_CHECK=0 -e BANGUMI_ENABLED=false "$image"
  healthy=false
  for attempt in $(seq 1 45); do
    state="$(docker inspect --format '{{.State.Health.Status}}' "$container")"
    if [ "$state" = healthy ]; then healthy=true; break; fi
    if [ "$state" = unhealthy ]; then break; fi
    sleep 2
  done
  if [ "$healthy" != true ]; then
    echo "linux/$arch 非 root 容器未健康启动" >&2
    docker logs "$container" >&2 || true
    exit 1
  fi
  docker exec "$container" sh -c 'wget -qO- http://127.0.0.1:8000/api/setup/status' > "$tmp/setup.json"
  python3 - "$tmp/setup.json" <<'PY'
import json, sys
with open(sys.argv[1], encoding='utf-8') as source:
    status = json.load(source)
assert status['required'] is True and status['download_mode'] == 'builtin', 'Fresh setup must retain builtin downloader'
PY
  downloader_ready=false
  for attempt in $(seq 1 15); do
    if docker exec "$container" pgrep -f '[q]bittorrent-nox' >/dev/null; then downloader_ready=true; break; fi
    sleep 2
  done
  [ "$downloader_ready" = true ] || { echo "linux/$arch 内置下载器未启动" >&2; exit 1; }
  docker rm -f "$container" >/dev/null
  container=''
done
echo '正式镜像匿名获取、双架构版本及非 root 启动检查通过。'

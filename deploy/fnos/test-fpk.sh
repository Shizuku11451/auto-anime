#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"
for script in "$root"/deploy/fnos/fpk/cmd/* "$root/scripts/docker-entrypoint.sh"; do bash -n "$script"; done
for port in 8000 18000; do
  if TRIM_SERVICE_PORT="$port" wizard_bt_port="$port" wizard_keep_source=true bash "$root/deploy/fnos/fpk/cmd/install_init"; then
    echo "接受了冲突端口 $port" >&2; exit 1
  fi
done
for port in 6881 16889; do
  wizard_bt_port="$port" wizard_keep_source=false bash "$root/deploy/fnos/fpk/cmd/install_init"
done

# 生成器不需要 Docker，也不能默默接受整卷/系统目录或坏 UID。
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
export TRIM_PKGETC="$tmp" TRIM_UID=991 TRIM_GID=986
. "$root/deploy/fnos/fpk/cmd/storage"
. "$root/deploy/fnos/fpk/cmd/image"
# Offline payload failures must never silently fall back to a registry pull.
(
  export TRIM_APPDEST="$tmp/offline"
  mkdir -p "$TRIM_APPDEST/image"
  if load_bundled_image; then echo '接受了不完整离线镜像' >&2; exit 1; fi
  printf 'payload' > "$TRIM_APPDEST/image/image.tar.gz"
  printf 'ghcr.io/test/repo:0.0.1\n' > "$TRIM_APPDEST/image/reference"
  printf '%064d\n' 0 > "$TRIM_APPDEST/image/image.sha256"
  if load_bundled_image; then echo '接受了错误校验值' >&2; exit 1; fi
  sha256sum "$TRIM_APPDEST/image/image.tar.gz" | awk '{print $1}' > "$TRIM_APPDEST/image/image.sha256"
  docker() { [ "$1" = load ] || [ "$1 $2" = 'image inspect' ]; }
  load_bundled_image
  docker() { return 1; }
  if load_bundled_image; then echo '忽略了镜像导入失败' >&2; exit 1; fi
)
TRIM_DATA_ACCESSIBLE_PATHS='' prepare_mounts
grep -q '"volumes":\[\]' "$tmp/authorized-mounts.json"
for path in / /vol1 /vol1/@appcenter/auto-anime /vol1/@appconf/auto-anime; do
  if TRIM_DATA_ACCESSIBLE_PATHS="$path" prepare_mounts; then
    echo "接受了系统目录 $path" >&2; exit 1
  fi
done
if TRIM_UID=0 prepare_mounts; then echo '接受了 root UID' >&2; exit 1; fi

# 模拟属主修改与 Docker，失败回调不得启动容器，也不修改真实文件属主。
export TRIM_PKGVAR="$tmp" TRIM_APPDEST="$tmp" TRIM_TEMP_LOGFILE="$tmp/error.log"
mkdir -p "$tmp/docker"
cp "$root/deploy/fnos/fpk/app/docker/docker-compose.yaml" "$tmp/docker/docker-compose.yaml"
export TEST_CHOWN_RESULT=0
chown() {
  printf 'chown\n' >> "$TRIM_PKGETC/calls"
  if [ "$TEST_CHOWN_RESULT" != 0 ]; then printf 'mock read-only filesystem\n' >&2; fi
  return "$TEST_CHOWN_RESULT"
}
docker() {
  case "$1" in
    inspect) printf 'true\n' ;;
    compose)
      printf 'compose\n' >> "$TRIM_PKGETC/calls"
      printf '%s\n' "$*" >> "$TRIM_PKGETC/compose-args"
      if [ "${TEST_DOCKER_RESULT:-0}" != 0 ]; then printf '%s\n' "${TEST_DOCKER_ERROR:-mock Docker failure}" >&2; fi
      return "${TEST_DOCKER_RESULT:-0}"
      ;;
    *) return 1 ;;
  esac
}
export -f chown docker
for invalid in '' relative / "$tmp/missing"; do
  if TRIM_PKGVAR="$invalid" ensure_data_owner; then
    echo '接受了无效数据目录' >&2; exit 1
  fi
done
if TRIM_UID=0 ensure_data_owner; then echo '属主修复接受了 root UID' >&2; exit 1; fi
if TRIM_GID=invalid ensure_data_owner; then echo '属主修复接受了无效 GID' >&2; exit 1; fi
[ ! -f "$tmp/calls" ] || { echo '无效参数调用了 chown' >&2; exit 1; }
for callback in install_callback upgrade_callback config_callback; do
  : > "$tmp/calls"
  : > "$TRIM_TEMP_LOGFILE"
  if TEST_CHOWN_RESULT=1 bash "$root/deploy/fnos/fpk/cmd/$callback"; then
    echo "$callback 吞掉了属主修复失败" >&2; exit 1
  fi
  grep -q '无法恢复应用数据目录属主' "$TRIM_TEMP_LOGFILE"
  grep -q 'mock read-only filesystem' "$TRIM_TEMP_LOGFILE"
  if grep -q '^compose$' "$tmp/calls"; then echo "$callback 失败后仍启动容器" >&2; exit 1; fi
  : > "$tmp/calls"
  : > "$tmp/compose-args"
  bash "$root/deploy/fnos/fpk/cmd/$callback"
  [ "$(cat "$tmp/calls")" = $'chown\ncompose' ] || { echo "$callback 未先修复属主再启动" >&2; exit 1; }
  grep -q -- '--wait --wait-timeout 120' "$tmp/compose-args" || { echo "$callback 未等待健康启动" >&2; exit 1; }
done
grep -q -- '--wait --wait-timeout 120' "$tmp/compose-args"
grep -q 'pull_policy: missing' "$tmp/docker/docker-compose.yaml"

# Docker 失败须保留退出码、写入安装提示，且不能把输出中的凭据带入弹窗。
export TEST_DOCKER_RESULT=17
for case in 'No such image|镜像不存在' 'manifest unknown|镜像不存在' 'unauthorized|镜像访问被拒绝' 'TLS handshake timeout|网络连接失败' 'port is already allocated|端口已被占用' 'no space left on device|存储空间' 'container is unhealthy|健康启动' 'Cannot connect to the Docker daemon|Docker 服务不可用' 'mock failure secret-token|Docker 操作失败'; do
  export TEST_DOCKER_ERROR="${case%%|*}"
  expected="${case#*|}"
  : > "$TRIM_TEMP_LOGFILE"
  status=0
  bash "$root/deploy/fnos/fpk/cmd/main" start || status=$?
  [ "$status" = 17 ] || { echo 'Docker 失败退出码被吞掉' >&2; exit 1; }
  grep -q "$expected" "$TRIM_TEMP_LOGFILE"
  grep -q '退出码 17' "$TRIM_TEMP_LOGFILE"
  if grep -q 'secret-token' "$TRIM_TEMP_LOGFILE"; then echo '安装提示包含原始 Docker 输出' >&2; exit 1; fi
done
for callback in install_callback upgrade_callback config_callback; do
  export TEST_DOCKER_ERROR='manifest unknown'
  status=0
  bash "$root/deploy/fnos/fpk/cmd/$callback" || status=$?
  [ "$status" = 17 ] || { echo "$callback 未报告启动失败" >&2; exit 1; }
  grep -q '镜像不存在' "$TRIM_TEMP_LOGFILE"
done
unset TEST_DOCKER_RESULT TEST_DOCKER_ERROR
echo '飞牛包脚本检查通过'

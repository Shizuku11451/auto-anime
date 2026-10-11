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
TRIM_DATA_ACCESSIBLE_PATHS='' prepare_mounts
grep -q '"volumes":\[\]' "$tmp/authorized-mounts.json"
for path in / /vol1 /vol1/@appcenter/auto-anime /vol1/@appconf/auto-anime; do
  if TRIM_DATA_ACCESSIBLE_PATHS="$path" prepare_mounts; then
    echo "接受了系统目录 $path" >&2; exit 1
  fi
done
if TRIM_UID=0 prepare_mounts; then echo '接受了 root UID' >&2; exit 1; fi
echo '飞牛包脚本检查通过'

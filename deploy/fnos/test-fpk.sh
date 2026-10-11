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

# 模拟属主修改与 Docker，失败回调不得启动容器，也不修改真实文件属主。
export TRIM_PKGVAR="$tmp" TRIM_APPDEST="$tmp" TRIM_TEMP_LOGFILE="$tmp/error.log"
export TEST_CHOWN_RESULT=0
chown() {
  printf 'chown\n' >> "$TRIM_PKGETC/calls"
  if [ "$TEST_CHOWN_RESULT" != 0 ]; then printf 'mock read-only filesystem\n' >&2; fi
  return "$TEST_CHOWN_RESULT"
}
docker() {
  case "$1" in
    inspect) printf 'true\n' ;;
    compose) printf 'compose\n' >> "$TRIM_PKGETC/calls" ;;
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
  bash "$root/deploy/fnos/fpk/cmd/$callback"
  [ "$(cat "$tmp/calls")" = $'chown\ncompose' ] || { echo "$callback 未先修复属主再启动" >&2; exit 1; }
done
echo '飞牛包脚本检查通过'

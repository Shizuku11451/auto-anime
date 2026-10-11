#!/usr/bin/env bash
#
# 构建飞牛 fnOS 应用包（.fpk）。
#
# 用法：deploy/fnos/build-fpk.sh [版本号] [镜像标签]
# 环境变量：OUT_DIR 指定输出目录；FNPACK 指定已有的 fnpack 可执行文件；KEEP_STAGING=1 保留临时目录。
# 产物默认写到 dist/fnos/。
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
package_dir="$script_dir/fpk"

version="${1:-0.0.0}"
image_tag="${2:-}"
if [ -z "$image_tag" ]; then
  if [ "$version" = "0.0.0" ]; then image_tag="latest"; else image_tag="$version"; fi
fi
out_dir="${OUT_DIR:-$repo_root/dist/fnos}"

# 官方只发布了 windows-amd64 与 linux-amd64 两个 fnpack，版本与校验和需和 build-fpk.ps1 保持一致。
fnpack_version="1.2.3"
fnpack_sha256="54B97FA7B70968C4D05C79840F5DAEFF508957D0BB2062FDB0376D00D9615C93"

case "$(uname -s)/$(uname -m)" in
  Linux/x86_64 | Linux/amd64) fnpack_platform="linux-amd64" ;;
  *)
    echo "官方目前只发布了 fnpack 的 windows-amd64 与 linux-amd64，当前平台 $(uname -s)/$(uname -m) 请用 deploy/fnos/build-fpk.ps1 或指定 FNPACK。" >&2
    exit 1
    ;;
esac

if [ -z "${FNPACK:-}" ]; then
  cache_dir="${XDG_CACHE_HOME:-$HOME/.cache}/auto-anime-fpk"
  fnpack="$cache_dir/fnpack-$fnpack_version-$fnpack_platform"
  if [ ! -f "$fnpack" ]; then
    mkdir -p "$cache_dir"
    url="https://static2.fnnas.com/fnpack/fnpack-$fnpack_version-$fnpack_platform"
    echo "下载 fnpack $fnpack_version ..."
    if command -v curl > /dev/null 2>&1; then
      curl -fsSL "$url" -o "$fnpack"
    else
      wget -qO "$fnpack" "$url"
    fi
  fi
  chmod +x "$fnpack"
else
  fnpack="$FNPACK"
fi

actual_sha256="$(sha256sum "$fnpack" | awk '{print toupper($1)}')"
if [ "$actual_sha256" != "$fnpack_sha256" ]; then
  echo "fnpack 校验失败：期望 $fnpack_sha256，实际 $actual_sha256" >&2
  exit 1
fi

staging_parent="$(mktemp -d "${TMPDIR:-/tmp}/auto-anime-fpk-XXXXXXXX")"
staging="$staging_parent/auto-anime"
cleanup() {
  if [ "${KEEP_STAGING:-}" != "1" ]; then rm -rf "$staging_parent"; fi
}
trap cleanup EXIT
cp -R "$package_dir" "$staging"

# 生命周期脚本在飞牛的 Linux 环境里执行，统一成 LF 并去掉 BOM。
while IFS= read -r -d '' file; do
  sed -i 's/\r$//; 1s/^\xEF\xBB\xBF//' "$file"
done < <(find "$staging" -type f \( -path "$staging/cmd/*" -o -path "$staging/wizard/*" -o -name manifest -o -name docker-compose.yaml -o -name config -o -name resource -o -name privilege \) -print0)

sed -i "s|^version\s*=.*$|version               = $version|" "$staging/manifest"
sed -i "s|image:\s*ghcr\.io/shizuku11451/auto-anime:[^[:space:]]*|image: ghcr.io/shizuku11451/auto-anime:$image_tag|" "$staging/app/docker/docker-compose.yaml"

(cd "$staging" && "$fnpack" build)

if [ ! -f "$staging/auto-anime.fpk" ]; then
  echo "没有生成 auto-anime.fpk" >&2
  exit 1
fi

mkdir -p "$out_dir"
target="$out_dir/auto-anime-$version-fnos.fpk"
cp "$staging/auto-anime.fpk" "$target"

echo
echo "已生成：$target"
echo "版本：$version    镜像：ghcr.io/shizuku11451/auto-anime:$image_tag"
echo "安装：飞牛应用中心 → 手动安装，选择上面的 .fpk，或执行 appcenter-cli install-fpk <文件>"

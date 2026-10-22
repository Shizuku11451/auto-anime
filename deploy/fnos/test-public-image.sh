#!/usr/bin/env bash
# Mock Docker validates the release gate without contacting registries or a daemon.
set -euo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf -- "$tmp"' EXIT
export TEST_CALLS="$tmp/calls"
docker() {
  grep -q '"auths":{}' "$DOCKER_CONFIG/config.json" || return 98
  printf '%s\n' "$*" >> "$TEST_CALLS"
  case "$1" in
    manifest)
      [ "$TEST_FAILURE" != private ] || return 1
      if [ "$TEST_FAILURE" = architecture ]; then
        printf '{"manifests":[{"platform":{"os":"linux","architecture":"amd64"}}]}'
      else
        printf '{"manifests":[{"platform":{"os":"linux","architecture":"amd64"}},{"platform":{"os":"linux","architecture":"arm64"}}]}'
      fi ;;
    pull) [ "$TEST_FAILURE" != pull ] ;;
    run)
      if [ "$2" = --rm ]; then
        if [ "$TEST_FAILURE" = version ]; then printf 'dev (unknown)\n'; else printf '1.2.3 (test)\n'; fi
      else printf 'mock-container\n'; fi ;;
    inspect) if [ "$TEST_FAILURE" = health ]; then printf 'unhealthy\n'; else printf 'healthy\n'; fi ;;
    exec)
      if [ "$TEST_FAILURE" = setup ]; then printf '{"required":false,"download_mode":"direct"}'; else printf '{"required":true,"download_mode":"native"}'; fi ;;
    logs)
      [ "$TEST_FAILURE" != downloader ] || return 1
      printf '内置 BT 下载器已就绪，监听 TCP 6881-6890、UDP 6881\n' ;;
    rm) return 0 ;;
    *) return 99 ;;
  esac
}
# Git Bash may have an unusable Windows Store python3 alias.
python3() {
  case "$(uname -s)" in MINGW* | MSYS*) python "$@" ;; *) command python3 "$@" ;; esac
}
sleep() { :; }
# No container is created: provide a stable non-root identity even on root CI shells.
id() { case "${1:-}" in -u | -g) printf '1000\n' ;; *) command id "$@" ;; esac; }
export -f docker python3 sleep id
for scenario in none private architecture pull version health setup downloader; do
  export TEST_FAILURE="$scenario"
  : > "$TEST_CALLS"
  result=0
  bash "$root/scripts/verify-public-image.sh" ghcr.io/example/app:1.2.3 1.2.3 > "$tmp/output" 2>&1 || result=$?
  if [ "$scenario" = none ]; then
    [ "$result" = 0 ] || { cat "$tmp/output"; exit 1; }
    for arch in amd64 arm64; do
      grep -q "pull --platform linux/$arch" "$TEST_CALLS"
      grep -q "run -d .*--platform linux/$arch --network none --user .*--cap-drop ALL" "$TEST_CALLS"
    done
    [ "$(grep -c '^rm -f ' "$TEST_CALLS")" = 2 ]
  else
    [ "$result" != 0 ] || { echo "Gate accepted failure: $scenario" >&2; exit 1; }
    case "$scenario" in health | setup | downloader) grep -q '^rm -f ' "$TEST_CALLS" ;; esac
  fi
done
echo 'Public-image gate mock success and seven failure scenarios passed.'

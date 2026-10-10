#!/usr/bin/env bash
# Internal desktop build. No install, publishing, autostart or service changes.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
for arg in "$@"; do
  if [[ "$arg" == --help || "$arg" == -h || "$arg" == --check ]]; then
    exec python3 scripts/build-desktop.py "$@"
  fi
done
guard="${ANCHOR_RESOURCE_GUARD:-$HOME/.codex/skills/resource-guard/scripts/resource_guard.sh}"
if [[ -x "$guard" ]]; then
  trap '"$guard" status >&2' EXIT
  "$guard" run --expected-mib 2048 --max-mib 3072 --swap-max-mib 256 \
    --label anchor-desktop-build -- timeout 900 env ANCHOR_DESKTOP_GUARDED=1 python3 scripts/build-desktop.py "$@"
elif [[ -e /home/mosaki/.codex/skills/resource-guard/SKILL.md ]]; then
  printf 'Resource guard is required on this host.\n' >&2
  exit 2
else
  exec python3 scripts/build-desktop.py "$@"
fi

#!/usr/bin/env bash
# Build Windows + Ubuntu desktop installers, never install, execute or publish them.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
for arg in "$@"; do
  if [[ "$arg" == --help || "$arg" == -h ]]; then
    exec python3 scripts/package-desktop.py "$@"
  fi
done
guard="${ANCHOR_RESOURCE_GUARD:-$HOME/.codex/skills/resource-guard/scripts/resource_guard.sh}"
if [[ -x "$guard" ]]; then
  trap '"$guard" status >&2' EXIT
  "$guard" run --expected-mib 3072 --max-mib 3584 --swap-max-mib 256 \
    --label anchor-desktop-packages -- timeout --kill-after=30 1800 \
    env ANCHOR_DESKTOP_GUARDED=1 PYTHONDONTWRITEBYTECODE=1 python3 scripts/package-desktop.py "$@"
elif [[ -e /home/mosaki/.codex/skills/resource-guard/SKILL.md ]]; then
  printf 'Resource guard is required on this host.\n' >&2
  exit 2
else
  exec python3 scripts/package-desktop.py "$@"
fi

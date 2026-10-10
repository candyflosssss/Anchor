#!/usr/bin/env bash
# Development entrypoint: bound compilation and keep Go temporary data off RAM-backed /tmp.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
export GOMAXPROCS="${GOMAXPROCS:-2}"
export GOFLAGS="${GOFLAGS:-} -p=1"
guard="${ANCHOR_RESOURCE_GUARD:-$HOME/.codex/skills/resource-guard/scripts/resource_guard.sh}"
if [[ -x "$guard" ]]; then
  trap '"$guard" status >&2' EXIT
  # The guard writes its preflight to stdout; keep those diagnostics out of CLI JSON.
  "$guard" run --expected-mib 2048 --max-mib 3072 --swap-max-mib 256 \
    --label anchor-dev -- go run ./cmd/anchor "$@" | awk '
      /^(MemAvailableMiB|SwapUsedMiB|SwapFreeMiB|TmpUsedMiB|MemoryPSIAvg10)=/ || /^OK: resource preflight passed\.$/ {
        print > "/dev/stderr"; fflush("/dev/stderr"); next
      }
      { print; fflush() }
    '
  exit 0
fi
# Other machines can run directly; this host must retain the resource guard.
if [[ -e /home/mosaki/.codex/skills/resource-guard/SKILL.md ]]; then
  printf 'Resource guard is required on this host; set ANCHOR_RESOURCE_GUARD correctly.\n' >&2
  exit 2
fi
exec go run ./cmd/anchor "$@"

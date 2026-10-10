#!/usr/bin/env bash
# Local UI-design preview only. Never connects to Anchor's management listener.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
guard="/home/mosaki/.codex/skills/resource-guard/scripts/resource_guard.sh"
[[ -x "$guard" ]] || { echo 'resource-guard is required on this machine' >&2; exit 1; }
[[ -d "$root/desktop/frontend/node_modules" ]] || { echo 'Install locked UI dependencies first; see desktop/frontend/README.md' >&2; exit 1; }
trap '"$guard" status' EXIT
cd "$root/desktop/frontend"
"$guard" run --expected-mib 1024 --max-mib 1536 --swap-max-mib 256 --label anchor-ui-preview -- timeout 1200 npm run dev -- --port 14201

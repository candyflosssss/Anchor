#!/usr/bin/env bash
# Runs ONLY in the short-lived, memory-limited Ubuntu builder.
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
export PATH="$GOROOT/bin:$PATH"
export GOCACHE=/scratch/go-cache GOPATH=/scratch/go
export GOTOOLCHAIN=local GOWORK=off GOMAXPROCS=2 GOFLAGS='-p=2 -mod=readonly'
export GOPROXY=off GOSUMDB=off PYTHONDONTWRITEBYTECODE=1
# apt uses the container overlay; host scratch files are created only as BUILD_UID.
export TMPDIR=/var/tmp
# Package installation happens in the throwaway container, NEVER on the host.
apt-get -o Acquire::Retries=2 -o Acquire::http::Timeout=30 update
apt-get -o Acquire::Retries=2 -o Acquire::http::Timeout=30 install -y --no-install-recommends \
  ca-certificates build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev \
  nsis python3 dpkg-dev util-linux
export TMPDIR=/scratch/tmp HOME=/scratch/home
mkdir -p "$TMPDIR" "$HOME"
chown "$BUILD_UID:$BUILD_GID" /scratch "$TMPDIR" "$HOME"
cd /src
setpriv --reuid="$BUILD_UID" --regid="$BUILD_GID" --clear-groups \
  python3 scripts/desktop-package-worker.py --version "$APP_VERSION"
setpriv --reuid="$BUILD_UID" --regid="$BUILD_GID" --clear-groups \
  sh -c 'dpkg-query -W -f='"'"'${Package}\t${Version}\n'"'"' > /output/BUILD_ENVIRONMENT.txt'

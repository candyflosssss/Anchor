# mihomo v1.19.32

Copyright MetaCubeX and mihomo contributors. Licensed under GNU GPL version 3;
see LICENSE. Anchor bundles the unmodified compatible amd64 release executable
in Windows/Linux builds as a separate child process. It does not enable TUN,
change system settings, or expose a controller API.

Upstream: https://github.com/MetaCubeX/mihomo
Corresponding version source: https://github.com/MetaCubeX/mihomo/tree/v1.19.32
Corresponding source download: https://github.com/MetaCubeX/mihomo/archive/refs/tags/v1.19.32.tar.gz
Build/dependency instructions: the upstream source's README, go.mod and Makefile.
No Anchor patches to mihomo. Upstream release artifacts and archive SHA256 pins
are recorded in scripts/prepare-mihomo.py. This notice and complete license are
also embedded in the Anchor executable. Distributors must preserve notices and
GPLv3 corresponding-source access alongside each binary download; merely running
it as a subprocess does not waive redistribution obligations.

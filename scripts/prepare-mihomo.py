#!/usr/bin/env python3
"""Prepare pinned GPLv3 mihomo archives for Go embedding. No executable is run."""
from __future__ import annotations
import argparse
import hashlib
import os
from pathlib import Path
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
VERSION = "v1.19.32"
ASSETS = {
    "linux-amd64": ("mihomo-linux-amd64-compatible-v1.19.32.gz", "ba3ce607747a07f948fc35780e108a4a7c7f552a38b9bd4d115f313ebcb89c20", "linux/mihomo.gz"),
    "windows-amd64": ("mihomo-windows-amd64-compatible-v1.19.32.zip", "974a4d7ad69aed27aa2e8f91d61113573c14dadb14562c63e58effabf59816f0", "windows/mihomo.zip"),
}
MAX_BYTES = 32 << 20

def prepare(target: str) -> Path:
    asset, expected, relative = ASSETS[target]
    destination = ROOT / "internal/egress/payload" / relative
    if destination.is_file():
        if hashlib.sha256(destination.read_bytes()).hexdigest() != expected:
            raise RuntimeError(f"Pinned mihomo checksum mismatch: {target}; remove the damaged generated payload explicitly")
        return destination
    destination.parent.mkdir(parents=True, exist_ok=True)
    partial = destination.with_suffix(destination.suffix + ".partial")
    # Build downloads may use the builder network proxy; runtime routing never does.
    opener = urllib.request.build_opener()
    request = urllib.request.Request(f"https://github.com/MetaCubeX/mihomo/releases/download/{VERSION}/{asset}", headers={"User-Agent": "Anchor-build"})
    try:
        with opener.open(request, timeout=60) as response, partial.open("xb") as output:
            total = 0
            digest = hashlib.sha256()
            while chunk := response.read(256 << 10):
                total += len(chunk)
                if total > MAX_BYTES:
                    raise RuntimeError("mihomo archive exceeds size limit")
                output.write(chunk)
                digest.update(chunk)
        if digest.hexdigest() != expected:
            raise RuntimeError(f"Pinned mihomo checksum mismatch: {target}")
        os.replace(partial, destination)
    finally:
        partial.unlink(missing_ok=True)
    return destination

def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--target", action="append", choices=ASSETS, required=True)
    args = parser.parse_args()
    for target in args.target:
        print(f"Verified mihomo {VERSION} {target}: {prepare(target)}")

if __name__ == "__main__":
    main()

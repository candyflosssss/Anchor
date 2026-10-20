#!/usr/bin/env python3
"""Generate an update feed from final local artifacts. Never build or publish."""
from __future__ import annotations
import argparse
import hashlib
import ipaddress
import json
from pathlib import Path
import re
from urllib.parse import quote, urlsplit

VERSION = re.compile(r"^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z.-]+))?$")
CHANNELS = {"stable", "alpha", "beta", "rc", "dev"}
MAX_ARTIFACT = 256 << 20

def validate_version(version: str, channel: str) -> None:
    match = VERSION.fullmatch(version)
    if not match or any(int(match[i]) > 2**32-1 for i in (1, 2, 3)):
        raise ValueError("invalid version")
    pre = match[4].split(".") if match[4] else []
    if any(not part or (part.isdecimal() and len(part) > 1 and part.startswith("0")) for part in pre):
        raise ValueError("invalid prerelease")
    if channel not in CHANNELS or (pre[0] if pre else "stable") != channel:
        raise ValueError("version and channel differ")

def validate_base(base: str) -> None:
    url = urlsplit(base)
    if not url.netloc or url.username or url.password or url.query or url.fragment:
        raise ValueError("invalid base URL")
    loopback = False
    try:
        loopback = ipaddress.ip_address(url.hostname or "").is_loopback
    except ValueError:
        pass
    if url.scheme != "https" and not (url.scheme == "http" and loopback):
        raise ValueError("use HTTPS (HTTP only on literal loopback)")

def build_manifest(product: str, version: str, channel: str, base: str,
                   files: dict[str, Path], layout: str = "flat", runtimes: dict[str, Path] | None = None) -> dict:
    validate_version(version, channel)
    validate_base(base)
    if product not in {"cli", "desktop"} or layout not in {"flat", "target"} or not files:
        raise ValueError("invalid product, layout or empty artifacts")
    if runtimes and (product != "desktop" or set(runtimes) != set(files)):
        raise ValueError("runtime targets must match desktop installer targets")
    artifacts = []
    for target, path in sorted(files.items()):
        if target not in {"linux-amd64", "windows-amd64"} or path.is_symlink() or not path.is_file():
            raise ValueError("unsupported target or non-regular artifact")
        if any(c in path.name for c in "\\:\x00"):
            raise ValueError("invalid artifact name")
        size = path.stat().st_size
        if size < 1 or size > MAX_ARTIFACT:
            raise ValueError("artifact size out of range")
        kind = "binary" if product == "cli" else "nsis" if target.startswith("windows") else "deb"
        if kind == "nsis" and path.suffix != ".exe" or kind == "deb" and path.suffix != ".deb":
            raise ValueError("wrong installer extension")
        digest = hashlib.sha256()
        with path.open("rb") as stream:
            for block in iter(lambda: stream.read(1 << 20), b""):
                digest.update(block)
        relative = (target + "/" if layout == "target" else "") + quote(path.name)
        artifacts.append({"file": path.name, "target": target, "kind": kind,
                          "url": base.rstrip("/") + "/" + relative,
                          "sha256": digest.hexdigest(), "bytes": size})
    if runtimes:
        runtime_feed = build_manifest("cli", version, channel, base, runtimes, layout)
        by_target = {a["target"]: a for a in runtime_feed["artifacts"]}
        for artifact in artifacts:
            runtime = by_target[artifact["target"]]
            if artifact["target"] == "windows-amd64" and not runtime["file"].endswith(".exe"):
                raise ValueError("Windows runtime needs .exe extension")
            runtime["kind"] = "runtime"
            artifact["runtime"] = runtime
    return {"schema_version": 1, "product": product, "version": version,
            "channel": channel, "artifacts": artifacts}

def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--product", required=True, choices=["cli", "desktop"])
    parser.add_argument("--version", required=True)
    parser.add_argument("--channel", required=True, choices=sorted(CHANNELS))
    parser.add_argument("--base-url", required=True)
    parser.add_argument("--artifact", required=True, action="append", metavar="TARGET=FILE")
    parser.add_argument("--runtime", action="append", default=[], metavar="TARGET=FILE", help="optional same-version desktop executable; one per installer")
    parser.add_argument("--layout", default="flat", choices=["flat", "target"])
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    files = {}
    for item in args.artifact:
        target, sep, file = item.partition("=")
        if not sep or target in files:
            parser.error("artifact must be unique TARGET=FILE")
        files[target] = Path(file)
    runtimes = {}
    for item in args.runtime:
        target, sep, file = item.partition("=")
        if not sep or target in runtimes:
            parser.error("runtime must be unique TARGET=FILE")
        runtimes[target] = Path(file)
    try:
        result = build_manifest(args.product, args.version, args.channel, args.base_url, files, args.layout, runtimes)
    except ValueError as error:
        parser.error(str(error))
    # Never overwrite a live feed in place or silently replace an existing file.
    with args.output.open("x", encoding="utf-8") as stream:
        json.dump(result, stream, indent=2, ensure_ascii=False)
        stream.write("\n")
    print(f"Created {args.output}; no artifacts uploaded or feed published.")

if __name__ == "__main__":
    main()

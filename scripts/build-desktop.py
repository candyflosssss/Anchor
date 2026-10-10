#!/usr/bin/env python3
"""Build an internal desktop executable; no install, publish, service or autostart changes."""
from __future__ import annotations

import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import shutil
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
FRONTEND = ROOT / "desktop/frontend"
TARGETS = ("linux-amd64", "windows-amd64")


def run(args: list[str], *, cwd: Path = ROOT, env: dict[str, str] | None = None) -> None:
    print("+ " + " ".join(args), flush=True)
    subprocess.run(args, cwd=cwd, env=env, check=True, timeout=600)


def preflight(target: str) -> None:
    for tool in ("go", "node", "npm"):
        if not shutil.which(tool):
            raise RuntimeError(f"Required tool missing: {tool}")
    engines = json.loads((FRONTEND / "package.json").read_text())["engines"]
    for tool in ("node", "npm"):
        actual = subprocess.check_output([tool, "--version"], text=True).strip().lstrip("v")
        if actual != engines[tool]:
            raise RuntimeError(f"{tool} {engines[tool]} required (found {actual})")
    if not (FRONTEND / "node_modules").is_dir():
        raise RuntimeError("Install locked frontend dependencies with npm ci first; see desktop/frontend/README.md")
    if target == "linux-amd64":
        if platform.system() != "Linux" or platform.machine() not in ("x86_64", "AMD64"):
            raise RuntimeError("Build Linux on a native Linux AMD64 host with CGO; cross-Linux builds are not configured")
        if not shutil.which("pkg-config") or subprocess.run(
            ["pkg-config", "--exists", "gtk+-3.0", "webkit2gtk-4.1"], check=False
        ).returncode:
            raise RuntimeError("Linux desktop build requires GTK3 + WebKitGTK 4.1 development files (pkg-config gtk+-3.0 webkit2gtk-4.1). No system packages were installed.")


def verify_contract(env: dict[str, str]) -> None:
    # Build-time generated-source comparison, not a test/acceptance run.
    generated = subprocess.check_output(
        ["go", "run", "./cmd/desktop-contract"], cwd=ROOT, env=env, timeout=120,
    )
    if generated != (FRONTEND / "src/desktop/contract.generated.ts").read_bytes():
        raise RuntimeError("Desktop Go/TypeScript contract drift; regenerate and review the contract first")


def verify_assets() -> None:
    # Reject accidentally bundled synthetic fixtures / scenario UI before embedding.
    files = sorted((FRONTEND / "dist/assets").glob("*.js"))
    if not files:
        raise RuntimeError("Desktop frontend assets are missing")
    bundle = "\n".join(path.read_text(encoding="utf-8") for path in files)
    if "anchor-desktop-v1" not in bundle:
        raise RuntimeError("Desktop contract marker is missing from the frontend")
    for marker in ("https://backup.example.com/v1", "本地调试", "原型场景"):
        if marker in bundle:
            raise RuntimeError(f"Prototype-only content leaked into the desktop bundle: {marker}")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--target", required=True, choices=TARGETS)
    parser.add_argument("--version", default="0.1.0-dev.desktop.1")
    parser.add_argument("--check", action="store_true", help="only check toolchain/native build prerequisites")
    args = parser.parse_args()
    if not re.fullmatch(r"\d+\.\d+\.\d+-dev\.desktop\.\d+", args.version):
        parser.error("Internal builds require X.Y.Z-dev.desktop.N; this is not the public release pipeline")
    if not args.check and Path("/home/mosaki/.codex/skills/resource-guard/SKILL.md").exists() and os.environ.get("ANCHOR_DESKTOP_GUARDED") != "1":
        raise RuntimeError("Use scripts/build-desktop.sh on this host for bounded resources")
    preflight(args.target)
    if args.check:
        print("Desktop build prerequisites passed; native runtime has NOT been tested.")
        return 0
    output = ROOT / "dist/desktop" / args.version / args.target
    if output.exists():
        raise RuntimeError(f"Refusing to overwrite an existing desktop build: {output}")
    env = os.environ.copy()
    for key in ("GOOS", "GOARCH", "CGO_ENABLED"):
        env.pop(key, None)
    env.update(GOMAXPROCS="2", GOFLAGS="-p=2", GOWORK="off", GOTOOLCHAIN="local")
    # Drift is a build error, not silent regeneration of the reviewed contract.
    verify_contract(env)
    run(["npm", "run", "build"], cwd=FRONTEND, env=env)
    verify_assets()
    goos, goarch = args.target.split("-")
    env.update(GOOS=goos, GOARCH=goarch, CGO_ENABLED="1" if goos == "linux" else "0")
    tags = "desktop,production" + (",webkit2_41" if goos == "linux" else "")
    ldflags = f"-s -w -X github.com/candyflosssss/Anchor/internal/core.Version={args.version}"
    if goos == "windows":
        ldflags += " -H windowsgui"
    output.mkdir(parents=True)
    binary = output / ("anchor-desktop.exe" if goos == "windows" else "anchor-desktop")
    partial = binary.with_suffix(binary.suffix + ".partial")
    try:
        run(["go", "build", "-trimpath", "-tags", tags, "-ldflags", ldflags, "-o", str(partial), "./cmd/anchor-desktop"], env=env)
        partial.replace(binary)
    finally:
        partial.unlink(missing_ok=True)
    sha = hashlib.sha256(binary.read_bytes()).hexdigest()
    (output / "SHA256SUMS").write_text(f"{sha}  {binary.name}\n", encoding="utf-8")
    metadata = {
        "channel": "desktop-dev", "version": args.version, "target": args.target,
        "built_at": datetime.now(timezone.utc).isoformat(), "artifact": binary.name,
        "sha256": sha, "native_gui_verified": False, "public_release": False,
        "revision": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip(),
        "dirty": bool(subprocess.check_output(["git", "status", "--porcelain"], cwd=ROOT, text=True).strip()),
        "runtime": "WebView2" if goos == "windows" else "GTK3 + WebKitGTK 4.1 + graphical session",
    }
    (output / "BUILD.json").write_text(json.dumps(metadata, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(metadata, indent=2))
    print(f"Built {binary}; internal only. Native acceptance not performed. Write a release update record; do not equate compilation with acceptance.")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (RuntimeError, subprocess.SubprocessError) as error:
        print(f"Desktop build failed: {error}", file=sys.stderr)
        raise SystemExit(1)

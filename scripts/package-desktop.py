#!/usr/bin/env python3
"""Build local Windows NSIS / Ubuntu 24.04 amd64 .deb packages; no acceptance or publication."""
from __future__ import annotations

import argparse
import update_manifest
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import platform
import re
import shutil
import signal
import subprocess
import sys
import tempfile
import uuid

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("desktop_build", ROOT / "scripts/build-desktop.py")
build = importlib.util.module_from_spec(spec)
spec.loader.exec_module(build)


def capture(argv: list[str], *, env: dict[str, str] | None = None) -> str:
    return subprocess.check_output(argv, cwd=ROOT, env=env, text=True, timeout=60).strip()


def digest(path: Path) -> str:
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def inputs_digest() -> str:
    # Only build inputs, never arbitrary worktree state, credentials or node_modules.
    files = [ROOT / "go.mod", ROOT / "go.sum", ROOT / "LICENSE"]
    for folder in ("cmd", "internal", "desktop/branding", "desktop/packaging", "third_party/mihomo"):
        files.extend(p for p in (ROOT / folder).rglob("*") if p.is_file())
    files.extend(p for p in (ROOT / "desktop/frontend/src").rglob("*") if p.is_file())
    files.extend(p for p in (ROOT / "desktop/frontend").iterdir() if p.is_file())
    files.extend(ROOT / "scripts" / n for n in ("build-desktop.py", "package-desktop.py", "package-desktop.sh", "desktop-package-worker.py", "update_manifest.py", "prepare-mihomo.py"))
    result = hashlib.sha256()
    for path in sorted(set(files)):
        if path.suffix == ".pyc" or "__pycache__" in path.parts:
            continue
        if path.is_symlink():
            raise RuntimeError(f"Build input may not be a symlink: {path}")
        result.update(path.relative_to(ROOT).as_posix().encode() + b"\0" + path.read_bytes() + b"\0")
    return result.hexdigest()


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True, help="X.Y.Z-dev.desktop.N (never overwritten)")
    parser.add_argument("--update-base-url", help="optional HTTPS directory for update-desktop.json; no upload")
    args = parser.parse_args()
    if args.update_base_url:
        update_manifest.validate_base(args.update_base_url)
    match = re.fullmatch(r"(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)-dev\.desktop\.(0|[1-9]\d*)", args.version)
    if not match or any(int(n) > 65535 for n in match.groups()):
        parser.error("Use X.Y.Z-dev.desktop.N; all components must fit the Windows version resource (0..65535)")
    if Path("/home/mosaki/.codex/skills/resource-guard/SKILL.md").exists() and os.environ.get("ANCHOR_DESKTOP_GUARDED") != "1":
        raise RuntimeError("Use scripts/package-desktop.sh on this host for bounded resources")
    if platform.system() != "Linux" or platform.machine() != "x86_64":
        raise RuntimeError("This dual-platform packager requires a Linux AMD64 Docker build host")
    if not shutil.which("docker"):
        raise RuntimeError("Docker is required; no packages will be installed on the host")
    build.preflight("windows-amd64")
    if capture(["docker", "info", "--format", "{{.OSType}}/{{.Architecture}}"] ) not in ("linux/x86_64", "linux/amd64"):
        raise RuntimeError("A Linux AMD64 Docker daemon is required")
    output = ROOT / "dist/desktop" / args.version
    if output.exists():
        raise RuntimeError(f"Refusing to overwrite existing desktop artifacts: {output}")
    env = os.environ.copy()
    for key in ("GOOS", "GOARCH", "CGO_ENABLED"):
        env.pop(key, None)
    env.update(GOMAXPROCS="2", GOFLAGS="-p=2 -mod=readonly", GOWORK="off", GOTOOLCHAIN="local")
    goroot = capture(["go", "env", "GOROOT"], env=env)
    modcache = capture(["go", "env", "GOMODCACHE"], env=env)
    # Distribution Go packages split GOROOT across /usr/lib and /usr/share.
    # Preserve their original absolute layout rather than breaking relative links.
    go_links = sorted({str(p.resolve(strict=True)) for p in
                       [*Path(goroot).iterdir(), *(Path(goroot) / "pkg").iterdir()] if p.is_symlink()})
    go_notices = [p for p in Path(goroot).iterdir() if p.is_file() and
                  p.name.upper().startswith(("LICENSE", "COPYING", "COPYRIGHT", "NOTICE"))]
    if not go_notices:
        distro_notice = Path("/usr/share/doc") / ("golang-" + Path(goroot).name.removeprefix("go-") + "-go") / "copyright"
        if not distro_notice.is_file():
            raise RuntimeError("Go license/notice files are required for packaging")
        go_notices = [distro_notice]
    # Contract comparison + TypeScript/Vite compilation are build steps, not test suites.
    build.verify_contract(env)
    build.run(["npm", "run", "build"], cwd=build.FRONTEND, env=env)
    build.verify_assets()
    build.run([sys.executable, str(ROOT / "scripts/prepare-mihomo.py"), "--target", "linux-amd64", "--target", "windows-amd64"], env=env)
    output.mkdir(parents=True)
    for target in build.TARGETS:
        (output / target).mkdir()
    source = {
        "revision": capture(["git", "rev-parse", "HEAD"]),
        "dirty": bool(capture(["git", "status", "--porcelain"])),
        "inputs_sha256": inputs_digest(),
        "go": capture(["go", "version"], env=env),
        "node": capture(["node", "--version"]), "npm": capture(["npm", "--version"]),
    }
    (output / "SOURCE.json").write_text(json.dumps(source, indent=2) + "\n")
    windows = output / "windows-amd64/anchor-desktop.exe"
    partial = windows.with_suffix(".exe.partial")
    winenv = env | {"GOOS": "windows", "GOARCH": "amd64", "CGO_ENABLED": "0"}
    ldflags = f"-s -w -H windowsgui -X github.com/candyflosssss/Anchor/internal/core.Version={args.version}"
    try:
        build.run(["go", "build", "-trimpath", "-buildvcs=false", "-tags", "desktop,production", "-ldflags", ldflags,
                   "-o", str(partial), "./cmd/anchor-desktop"], env=winenv)
        partial.replace(windows)
    finally:
        partial.unlink(missing_ok=True)

    # The Docker daemon is outside the wrapper cgroup: bound the container too.
    # The source and module cache are read-only. Writable mounts are task-owned.
    name = "anchor-desktop-build-" + uuid.uuid4().hex[:12]
    def terminate(_signum, _frame):
        raise KeyboardInterrupt
    previous = signal.signal(signal.SIGTERM, terminate)
    try:
        with tempfile.TemporaryDirectory(prefix="anchor-packages-", dir=os.environ.get("TMPDIR")) as scratch:
            notices = Path(scratch) / "go-notices"
            notices.mkdir()
            for file in go_notices:
                shutil.copyfile(file, notices / file.name)
            # Pull before running so the selected immutable digest can be recorded.
            image = "ubuntu:24.04"
            build.run(["docker", "pull", image], env=env)
            image_id = capture(["docker", "image", "inspect", "--format", "{{.Id}}", image])
            source["container_image"] = capture(["docker", "image", "inspect", "--format", "{{json .RepoDigests}}", image])
            command = ["docker", "run", "--rm", "--init", "--name", name,
                       "--memory=2560m", "--memory-swap=2816m", "--cpus=2", "--pids-limit=256",
                       "--security-opt=no-new-privileges", "--workdir=/src",
                       "--mount", f"type=bind,src={ROOT},dst=/src,readonly",
                       "--mount", f"type=bind,src={goroot},dst={goroot},readonly",
                       *[arg for path in go_links for arg in ("--mount", f"type=bind,src={path},dst={path},readonly")],
                       "--mount", f"type=bind,src={modcache},dst={modcache},readonly",
                       "--mount", f"type=bind,src={output},dst=/output",
                       "--mount", f"type=bind,src={scratch},dst=/scratch",
                       "--env", f"GOROOT={goroot}", "--env", f"GOMODCACHE={modcache}", "--env", f"APP_VERSION={args.version}",
                       "--env", f"BUILD_UID={os.getuid()}", "--env", f"BUILD_GID={os.getgid()}",
                       image_id, "bash", "/src/desktop/packaging/build-container.sh"]
            print("+ bounded Ubuntu 24.04 container: compile Linux, then package both targets", flush=True)
            try:
                subprocess.run(command, check=True, timeout=1500)
            finally:
                subprocess.run(["docker", "rm", "-f", name], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=30, check=False)
    finally:
        signal.signal(signal.SIGTERM, previous)
    (output / "SOURCE.json").write_text(json.dumps(source, indent=2) + "\n")
    # Unique attachment names allow side-by-side runtime updates from a flat Release.
    runtime_files, cli_files = {}, {}
    for target in build.TARGETS:
        folder = output / target
        extension = ".exe" if target.startswith("windows") else ""
        runtime_file = folder / f"anchor-desktop-{args.version}-{target}{extension}"
        shutil.copy2(folder / ("anchor-desktop" + extension), runtime_file)
        runtime_files[target] = runtime_file
        cli_file = folder / f"anchor-{args.version}-{target}{extension}"
        cli_env = env | {"GOOS": target.split("-")[0], "GOARCH": "amd64", "CGO_ENABLED": "0"}
        build.run(["go", "build", "-trimpath", "-buildvcs=false", "-ldflags",
                   f"-s -w -X github.com/candyflosssss/Anchor/internal/core.Version={args.version}",
                   "-o", str(cli_file), "./cmd/anchor"], env=cli_env)
        cli_files[target] = cli_file
    for target in build.TARGETS:
        folder = output / target
        artifacts = {p.name: digest(p) for p in sorted(folder.iterdir()) if p.is_file()}
        metadata = {
            "channel": "desktop-dev", "version": args.version, "target": target,
            "built_at": datetime.now(timezone.utc).isoformat(), "source": source, "artifacts": artifacts,
            "native_gui_verified": False, "installation_verified": False, "public_release": False,
            "acceptance": "not run at user request; compilation and packaging only",
            "signed": False,
            "runtime": "Windows 10+ AMD64 / WebView2" if target.startswith("windows") else "Ubuntu 24.04 LTS AMD64 / GTK3 / WebKitGTK 4.1 / graphical session",
        }
        (folder / "BUILD.json").write_text(json.dumps(metadata, indent=2) + "\n")
        artifacts["BUILD.json"] = digest(folder / "BUILD.json")
        (folder / "SHA256SUMS").write_text("".join(f"{sha}  {filename}\n" for filename, sha in sorted(artifacts.items())))
    if args.update_base_url:
        packages = {}
        for target in build.TARGETS:
            matches = list((output / target).glob("*-setup.exe" if target.startswith("windows") else "*.deb"))
            if len(matches) != 1:
                raise RuntimeError("Expected exactly one installer for the update feed")
            packages[target] = matches[0]
        feed = update_manifest.build_manifest("desktop", args.version, "dev", args.update_base_url, packages, "target", runtime_files)
        (output / "update-desktop.json").write_text(json.dumps(feed, indent=2) + "\n")
        cli_feed = update_manifest.build_manifest("cli", args.version, "dev", args.update_base_url, cli_files, "target")
        (output / "update-cli.json").write_text(json.dumps(cli_feed, indent=2) + "\n")
    print(f"Created local desktop installers in {output}. No installation, application launch, tests, acceptance or publication performed.")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (ValueError, RuntimeError, OSError, subprocess.SubprocessError, KeyboardInterrupt) as error:
        print(f"Desktop packaging failed: {error}", file=sys.stderr)
        raise SystemExit(1)

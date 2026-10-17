#!/usr/bin/env python3
"""Build standalone Anchor executables and optional audit bundles; never publish."""
from __future__ import annotations

import argparse
from contextlib import contextmanager
from datetime import datetime, timezone
import gzip
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
import time
import zipfile

ROOT = Path(__file__).resolve().parents[1]
TARGETS = {"linux-amd64": ("linux", "amd64"), "windows-amd64": ("windows", "amd64")}
VERSION = re.compile(r"(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)-(alpha|beta|rc)\.(?:0|[1-9][0-9]*)\Z")
MODULE = "github.com/candyflosssss/Anchor"


def arguments(argv: list[str] | None = None) -> argparse.Namespace:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--version", required=True, help="prerelease only, e.g. 0.1.0-alpha.1")
    p.add_argument("--targets", default="linux-amd64,windows-amd64", help="comma-separated build targets")
    p.add_argument("--internal", action="store_true", help="internal evaluation only; permits missing project LICENSE")
    p.add_argument("--allow-dirty", action="store_true", help="allow an uncommitted worktree, only with --internal")
    p.add_argument("--plan", action="store_true", help="print the plan without compiling, testing or creating files")
    p.add_argument("--check", action="store_true", help="run source checks only; create no distribution package")
    args = p.parse_args(argv)
    if not VERSION.fullmatch(args.version):
        p.error("this milestone accepts X.Y.Z-alpha.N, X.Y.Z-beta.N or X.Y.Z-rc.N only")
    args.targets = args.targets.split(",")
    if not args.targets or len(set(args.targets)) != len(args.targets) or any(t not in TARGETS for t in args.targets):
        p.error("targets must be unique values from linux-amd64,windows-amd64")
    if args.allow_dirty and not args.internal:
        p.error("--allow-dirty requires --internal; dirty worktrees are not public release inputs")
    if args.check and args.plan:
        p.error("--check and --plan are mutually exclusive")
    return args


def command(argv: list[str], *, env: dict[str, str] | None = None, timeout: int = 240, cwd: Path | None = None) -> str:
    print("+ " + " ".join(argv), file=sys.stderr, flush=True)
    result = subprocess.run(argv, cwd=cwd or ROOT, env=env, capture_output=True, text=True, timeout=timeout)
    if result.returncode:
        raise RuntimeError(f"command failed ({result.returncode}): {argv[0]}\n{result.stdout[-4000:]}{result.stderr[-4000:]}")
    if result.stderr:
        print(result.stderr, end="", file=sys.stderr)
    return result.stdout


def environment() -> dict[str, str]:
    env = os.environ.copy()
    # Do not inherit developer-specific build flags or a workspace outside this repository.
    env.update(GOMAXPROCS="2", GOFLAGS="-p=1", GOWORK="off", GOTOOLCHAIN="local", PYTHONDONTWRITEBYTECODE="1")
    env.pop("GOOS", None)
    env.pop("GOARCH", None)
    env.pop("CGO_ENABLED", None)
    return env


def source_info() -> dict[str, object]:
    return {
        "revision": command(["git", "rev-parse", "HEAD"]).strip(),
        "dirty": bool(command(["git", "status", "--porcelain", "--untracked-files=all"]).strip()),
        "inputs_sha256": input_digest(),
    }


def input_digest() -> str:
    inputs = [ROOT / "go.mod", ROOT / "go.sum", ROOT / "docs/cli/commands.md",
              ROOT / "docs/release/package-readme.md", ROOT / "scripts/smoke_test.py",
              ROOT / "scripts/release.py", ROOT / "scripts/release.sh"]
    inputs.extend((ROOT / "scripts").glob("*.py"))
    inputs.extend((ROOT / "cmd").rglob("*.go"))
    inputs.extend((ROOT / "internal").rglob("*.go"))
    if (ROOT / "LICENSE").exists():
        inputs.append(ROOT / "LICENSE")
    h = hashlib.sha256()
    for path in sorted(set(inputs)):
        if path.is_symlink():
            raise RuntimeError(f"release input must not be a symlink: {path.relative_to(ROOT)}")
        h.update(path.relative_to(ROOT).as_posix().encode() + b"\0")
        h.update(path.read_bytes())
        h.update(b"\0")
    return h.hexdigest()


def digest(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for block in iter(lambda: f.read(1 << 20), b""):
            h.update(block)
    return h.hexdigest()


def checks(env: dict[str, str]) -> list[str]:
    # Fresh CI caches need every module directory for bundled license/notice files,
    # including build-tool modules not imported by the executable or its tests.
    command(["go", "mod", "download", "all"], env=env)
    passed = []
    for label, cmd in [
        ("module_verify", ["go", "mod", "verify"]),
        ("go_race_tests", ["go", "test", "-race", "-p", "1", "-count=1", "-timeout=180s", "./..."]),
        ("go_vet", ["go", "vet", "-p", "1", "./..."]),
        ("release_tool_tests", [sys.executable, "-m", "unittest", "discover", "-s", "scripts", "-p", "test_release*.py"]),
    ]:
        print(command(cmd, env=env), end="", file=sys.stderr)
        passed.append(label)
    return passed


def dependencies(env: dict[str, str]) -> tuple[list[dict[str, object]], str]:
    raw = command(["go", "list", "-m", "-json", "all"], env=env)
    decoder = json.JSONDecoder()
    modules, notices = [], []
    while raw.strip():
        raw = raw.lstrip()
        module, end = decoder.raw_decode(raw)
        raw = raw[end:]
        if module.get("Main"):
            continue
        if module.get("Replace"):
            raise RuntimeError("replaced dependency modules are not supported by this release workflow")
        directory = Path(module["Dir"])
        files = sorted(p for p in directory.iterdir() if p.is_file() and
                       p.name.upper().startswith(("LICENSE", "COPYING", "COPYRIGHT", "NOTICE")))
        if not files:
            raise RuntimeError(f"dependency has no root license/notice file: {module['Path']}")
        modules.append({"path": module["Path"], "version": module["Version"], "notice_files": [p.name for p in files]})
        notices.append(f"\n{'=' * 72}\n{module['Path']}@{module['Version']}\n")
        for file in files:
            if file.is_symlink() or file.stat().st_size > 2 << 20:
                raise RuntimeError(f"invalid dependency notice: {module['Path']}/{file.name}")
            notices.append(f"\n--- {file.name} ---\n" + file.read_text(errors="replace"))
    notices.extend((ROOT / "third_party/mihomo" / n).read_text() for n in ("NOTICE.md", "LICENSE"))
    return modules, "Bundled module notices (not a legal completeness audit).\n" + "\n".join(notices)


def archive(payload: Path, output: Path, target: str, epoch: int) -> Path:
    paths = [payload, *sorted(payload.rglob("*"))]
    if any(p.is_symlink() for p in paths):
        raise RuntimeError("payload symlinks are prohibited")
    if target.startswith("windows-"):
        destination = output / (payload.name + ".zip")
        with zipfile.ZipFile(destination, "x", compression=zipfile.ZIP_DEFLATED) as z:
            for p in paths:
                if p.is_file():
                    info = zipfile.ZipInfo(p.relative_to(payload.parent).as_posix(),
                                           datetime.fromtimestamp(epoch, timezone.utc).timetuple()[:6])
                    info.compress_type = zipfile.ZIP_DEFLATED
                    info.external_attr = (0o100755 if p.name == "anchor.exe" else 0o100644) << 16
                    z.writestr(info, p.read_bytes())
        return destination
    destination = output / (payload.name + ".tar.gz")
    with destination.open("xb") as f, gzip.GzipFile(filename="", mode="wb", fileobj=f, mtime=epoch) as gz:
        with tarfile.open(fileobj=gz, mode="w") as t:
            for p in paths:
                info = t.gettarinfo(str(p), p.relative_to(payload.parent).as_posix())
                info.uid = info.gid = 0
                info.uname = info.gname = ""
                info.mtime = epoch
                info.mode = 0o755 if p.is_dir() or p.name == "anchor" else 0o644
                if p.is_file():
                    with p.open("rb") as source:
                        t.addfile(info, source)
                else:
                    t.addfile(info)
    return destination


def export_source(output: Path, scratch: Path, version: str, source: dict[str, object],
                  env: dict[str, str], epoch: int) -> Path:
    """Export committed sources and the exact compiled dependency sources, never local state."""
    payload = scratch / f"anchor_{version}_source"
    payload.mkdir()
    snapshot = scratch / "committed-source.tar"
    command(["git", "archive", "--format=tar", f"--output={snapshot}", str(source["revision"])], env=env)
    with tarfile.open(snapshot) as t:
        # No tracked symlinks or special files may escape the source payload.
        if any(not (m.isfile() or m.isdir()) for m in t.getmembers()):
            raise RuntimeError("source snapshot contains links or special files")
        t.extractall(payload, filter="data")
    command(["go", "mod", "vendor"], env=env, cwd=payload)
    (payload / "PUBLIC-BUILD.md").write_text(
        f"# Anchor {version} corresponding source\n\n"
        f"Revision: {source['revision']}\n\n"
        "Anchor is licensed under AGPL-3.0-only (see LICENSE). Dependencies retain their own licenses.\n"
        "vendor/ contains the dependency source selected by go.mod/go.sum; do not remove its notices.\n"
        "Use the Go version in release.json. From this directory on Linux AMD64:\n\n"
        "```sh\n"
        f"GOWORK=off CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=vendor -p=1 -trimpath -buildvcs=false -ldflags '-X {MODULE}/internal/core.Version={version}' -o anchor ./cmd/anchor\n"
        "./anchor version\n```\n\n"
        "The runtime needs only the executable. Secrets, state and Agent configurations are not included.\n")
    return archive(payload, output, "linux-amd64", epoch)


def standalone_name(version: str, target: str) -> str:
    goos, goarch = TARGETS[target]
    return f"anchor_{version}_{goos}_{goarch}" + (".exe" if goos == "windows" else "")


def export_standalone(executable: Path, output: Path, target: str, version: str) -> Path:
    if executable.is_symlink() or not executable.is_file():
        raise RuntimeError("standalone executable must be a regular, non-symlink file")
    destination = output / standalone_name(version, target)
    with executable.open("rb") as source, destination.open("xb") as result:
        shutil.copyfileobj(source, result)
    destination.chmod(0o755 if target.startswith("linux-") else 0o644)
    if digest(destination) != digest(executable):
        raise RuntimeError("standalone executable copy failed checksum validation")
    return destination


def check_standalone(path: Path, target: str, version: str, env: dict[str, str]) -> str:
    if not native(target):
        return "cross_compile_only; native runtime acceptance pending"
    print(command([sys.executable, "scripts/smoke_test.py", "--binary", str(path),
                   "--expected-version", version], env=env), file=sys.stderr)
    return "standalone_binary_smoke_passed"


def native(target: str) -> bool:
    return target == "linux-amd64" and sys.platform == "linux" and platform.machine().lower() in {"x86_64", "amd64"}


def check_archive(path: Path, target: str, version: str, env: dict[str, str], scratch: Path) -> str:
    if not native(target):
        return "cross_compile_only; native runtime acceptance pending"
    extracted = scratch / "extracted"
    extracted.mkdir()
    with tarfile.open(path, "r:gz") as t:
        t.extractall(extracted, filter="data")
    binary = extracted / path.name.removesuffix(".tar.gz") / "anchor"
    print(command([sys.executable, "scripts/smoke_test.py", "--binary", str(binary),
                   "--expected-version", version], env=env), file=sys.stderr)
    return "packaged_binary_smoke_passed"


def save_manifest(output: Path, manifest: dict[str, object]) -> None:
    (output / "release.json").write_text(json.dumps(manifest, indent=2, ensure_ascii=False) + "\n")
    files = sorted(p for p in output.iterdir() if p.is_file() and p.name != "SHA256SUMS")
    (output / "SHA256SUMS").write_text("".join(f"{digest(p)}  {p.name}\n" for p in files))


@contextmanager
def output_lock():
    dist = ROOT / "dist"
    if dist.is_symlink():
        raise RuntimeError("dist must not be a symlink")
    dist.mkdir(exist_ok=True)
    lock = dist / ".release-lock"
    try:
        lock.mkdir()
    except FileExistsError:
        raise RuntimeError("another release owns dist/.release-lock; inspect before removing a stale lock") from None
    try:
        yield dist
    finally:
        lock.rmdir()


def build(args: argparse.Namespace, env: dict[str, str], source: dict[str, object], passed: list[str]) -> Path:
    epoch = int(time.time())
    blockers = []
    if args.internal:
        blockers.append("internal evaluation package; not authorized for public publication")
    if source["dirty"]:
        blockers.append("source worktree is dirty; Git revision alone does not identify these inputs")
    if not (ROOT / "LICENSE").is_file():
        blockers.append("project distribution license has not been selected")
    manifest = {"schema_version": 1, "version": args.version, "channel": args.version.split("-")[1].split(".")[0],
                "built_at": datetime.fromtimestamp(epoch, timezone.utc).isoformat(), "source": source,
                "go_version": command(["go", "version"], env=env).strip(), "checks": passed,
                "status": "building", "public_publish_ready": False, "publish_blockers": blockers,
                "artifacts": [], "standalone_artifacts": [], "dependencies": []}
    with output_lock() as dist:
        final = dist / args.version
        if final.exists() or final.is_symlink():
            raise RuntimeError(f"refusing to replace existing release: {final}")
        stage = Path(tempfile.mkdtemp(prefix=f".{args.version}-", dir=dist))
        try:
            modules, notices = dependencies(env)
            manifest["dependencies"] = modules
            if not args.internal:
                shutil.copyfile(ROOT / "LICENSE", stage / "LICENSE")
                (stage / "THIRD_PARTY_NOTICES.txt").write_text(notices)
            with tempfile.TemporaryDirectory(prefix="anchor-package-") as tmp:
                scratch = Path(tmp)
                for target in args.targets:
                    command([sys.executable, str(ROOT / "scripts/prepare-mihomo.py"), "--target", target], env=env, timeout=240)
                    goos, goarch = TARGETS[target]
                    payload = scratch / f"anchor_{args.version}_{goos}_{goarch}"
                    payload.mkdir()
                    executable = payload / ("anchor.exe" if goos == "windows" else "anchor")
                    build_env = env | {"GOOS": goos, "GOARCH": goarch, "CGO_ENABLED": "0"}
                    command(["go", "build", "-p", "1", "-trimpath", "-buildvcs=false", "-ldflags",
                             f"-X {MODULE}/internal/core.Version={args.version}", "-o", str(executable), "./cmd/anchor"], env=build_env)
                    shutil.copyfile(ROOT / "docs/release/package-readme.md", payload / "README.md")
                    shutil.copyfile(ROOT / "docs/cli/commands.md", payload / "CLI-COMMANDS.md")
                    (payload / "THIRD_PARTY_NOTICES.txt").write_text(notices)
                    if (ROOT / "LICENSE").is_file():
                        shutil.copyfile(ROOT / "LICENSE", payload / "LICENSE")
                    else:
                        (payload / "LEGAL-NOTICE.txt").write_text("Internal evaluation only. Project distribution license pending.\nDo not publish this package as an authorized public release.\n")
                    (payload / "BUILD-INFO.json").write_text(json.dumps({"version": args.version, "target": target,
                        "built_at": manifest["built_at"], "source": source}, indent=2, ensure_ascii=False) + "\n")
                    standalone = export_standalone(executable, stage, target, args.version)
                    standalone_entry = {"file": standalone.name, "target": target, "sha256": digest(standalone),
                                        "bytes": standalone.stat().st_size, "verification": "pending"}
                    manifest["standalone_artifacts"].append(standalone_entry)
                    standalone_entry["verification"] = check_standalone(standalone, target, args.version, env)
                    artifact = archive(payload, stage, target, epoch)
                    entry = {"file": artifact.name, "target": target, "sha256": digest(artifact), "bytes": artifact.stat().st_size,
                             "verification": "pending"}
                    manifest["artifacts"].append(entry)
                    entry["verification"] = check_archive(artifact, target, args.version, env, scratch)
                    if entry["verification"] != "packaged_binary_smoke_passed":
                        blockers.append(f"{target}: native runtime acceptance pending")
                if not args.internal:
                    source_archive = export_source(stage, scratch, args.version, source, env, epoch)
                    manifest["corresponding_source"] = {
                        "file": source_archive.name, "sha256": digest(source_archive),
                        "bytes": source_archive.stat().st_size, "revision": source["revision"],
                        "dependencies": "vendored", "license": "AGPL-3.0-only"}
                if input_digest() != source["inputs_sha256"]:
                    raise RuntimeError("release inputs changed while building; discard this candidate")
            manifest["status"] = "packaged"
            manifest["public_publish_ready"] = not blockers
            (stage / "RELEASE_NOTES.md").write_text(f"# Anchor {args.version}\n\nSingle-executable CLI/headless Responses gateway with Codex base-config binding, model catalogs and conditional recovery.\nUse the standalone file for your OS; audit bundles with documentation and dependency notices are optional for runtime.\nNo desktop UI, built-in SSH connector or cross-protocol conversion.\n\nPublication blockers:\n" + "".join(f"- {b}\n" for b in blockers) + "\nSee release.json for source and validation evidence.\n" +
                (f"\nLicense: AGPL-3.0-only. Commercial use is allowed under its terms.\nCorresponding source (including vendored dependencies): anchor_{args.version}_source.tar.gz.\nSource revision: {source['revision']}.\nRetain LICENSE and THIRD_PARTY_NOTICES.txt when redistributing.\n" if not args.internal else ""))
            save_manifest(stage, manifest)
            stage.rename(final)
            return final
        except BaseException as error:
            if manifest["standalone_artifacts"] or list(stage.glob("*.tar.gz")) or list(stage.glob("*.zip")):
                manifest["status"] = "partial"
                manifest["error"] = str(error)[:4000]
                save_manifest(stage, manifest)
                failed = dist / f"{args.version}.failed-{stage.name.rsplit('-', 1)[-1]}"
                stage.rename(failed)
                print(f"Partial artifacts retained for inspection, NOT for distribution: {failed}", file=sys.stderr)
            else:
                shutil.rmtree(stage)
            raise


def main(argv: list[str] | None = None) -> int:
    args = arguments(argv)
    if args.plan:
        print(json.dumps({"version": args.version, "targets": args.targets, "output": str(ROOT / "dist" / args.version),
                          "standalone_files": [standalone_name(args.version, t) for t in args.targets],
                          "steps": ["source/license checks", "tests and vet", "build", "standalone smoke", "audit bundles", "native archive smoke", "checksums"],
                          "internal": args.internal, "uploads": False}, indent=2))
        return 0
    if Path("/home/mosaki/.codex/skills/resource-guard/SKILL.md").exists() and os.environ.get("ANCHOR_RELEASE_GUARDED") != "1":
        raise RuntimeError("use scripts/release.sh so heavy checks run under resource-guard")
    if sys.platform != "linux" or platform.machine().lower() not in {"x86_64", "amd64"}:
        raise RuntimeError("the current release runner requires Linux amd64; Windows is a cross-build target, not a validated runner")
    env = environment()
    source = source_info()
    if source["dirty"] and not args.allow_dirty:
        raise RuntimeError("source worktree is dirty; commit intended inputs, or explicitly use --internal --allow-dirty")
    if not (ROOT / "LICENSE").is_file() and not args.internal:
        raise RuntimeError("project LICENSE is missing; choose distribution terms first, or use --internal")
    if not args.check and (ROOT / "dist" / args.version).exists():
        raise RuntimeError("version output already exists; choose a new version, never overwrite an existing release")
    passed = checks(env)
    if args.check:
        print(json.dumps({"checks": passed, "status": "passed", "artifacts_created": False}))
        return 0
    result = build(args, env, source, passed)
    print(json.dumps({"output": str(result), "status": "packaged", "published": False}))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (RuntimeError, OSError, subprocess.TimeoutExpired) as error:
        print(f"anchor release: {error}", file=sys.stderr)
        raise SystemExit(1)

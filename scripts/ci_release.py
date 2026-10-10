#!/usr/bin/env python3
"""Validate CI inputs and downloaded packages; never publish by itself."""
from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
import re

from release import VERSION, standalone_name


def plan(event: str, ref: str, run_number: str) -> dict[str, str]:
    if event == "workflow_dispatch":
        if not re.fullmatch(r"[1-9][0-9]*", run_number):
            raise ValueError("invalid workflow run number")
        return {"version": f"0.0.0-alpha.{run_number}", "internal": "true", "draft": "false", "targets": "linux-amd64,windows-amd64"}
    if event != "push" or not ref.startswith("refs/tags/v"):
        raise ValueError("only manual runs or prerelease tag pushes are supported")
    version = ref.removeprefix("refs/tags/v")
    if not VERSION.fullmatch(version):
        raise ValueError("tag must be vX.Y.Z-alpha.N, vX.Y.Z-beta.N or vX.Y.Z-rc.N")
    return {"version": version, "internal": "false", "draft": "true", "targets": "linux-amd64"}


def verify(directory: Path, version: str, revision: str, targets: str = "linux-amd64,windows-amd64") -> None:
    if not VERSION.fullmatch(version) or not re.fullmatch(r"[0-9a-f]{40}", revision):
        raise ValueError("invalid version or source revision")
    if targets not in {"linux-amd64", "linux-amd64,windows-amd64"}:
        raise ValueError("unsupported release scope")
    linux_public = targets == "linux-amd64"
    names = {f"anchor_{version}_linux_amd64.tar.gz", f"anchor_{version}_windows_amd64.zip",
             standalone_name(version, "linux-amd64"), standalone_name(version, "windows-amd64"),
             "release.json", "RELEASE_NOTES.md"}
    if linux_public:
        names -= {f"anchor_{version}_windows_amd64.zip", standalone_name(version, "windows-amd64")}
        names |= {f"anchor_{version}_source.tar.gz", "LICENSE", "THIRD_PARTY_NOTICES.txt"}
    if directory.is_symlink() or {p.name for p in directory.iterdir()} != names | {"SHA256SUMS"}:
        raise ValueError("unexpected release bundle contents")
    if any(p.is_symlink() or not p.is_file() for p in directory.iterdir()):
        raise ValueError("release bundle must contain only regular files")
    sums = {}
    for line in (directory / "SHA256SUMS").read_text().splitlines():
        match = re.fullmatch(r"([0-9a-f]{64})  ([A-Za-z0-9_.-]+)", line)
        if not match or match[2] not in names or match[2] in sums:
            raise ValueError("invalid or duplicate checksum entry")
        sums[match[2]] = match[1]
    if set(sums) != names:
        raise ValueError("incomplete checksum list")
    for name, expected in sums.items():
        with (directory / name).open("rb") as f:
            if hashlib.file_digest(f, "sha256").hexdigest() != expected:
                raise ValueError(f"checksum mismatch: {name}")
    data = json.loads((directory / "release.json").read_text())
    source = data.get("source", {})
    if (data.get("schema_version") != 1 or data.get("version") != version
            or data.get("channel") != version.split("-")[1].split(".")[0]
            or data.get("status") != "packaged" or source.get("revision") != revision
            or source.get("dirty") is not False):
        raise ValueError("package is incomplete or does not match the tagged clean source")
    if not re.fullmatch(r"[0-9a-f]{64}", str(source.get("inputs_sha256", ""))):
        raise ValueError("missing source input fingerprint")
    pending = [] if linux_public else ["windows-amd64: native runtime acceptance pending"]
    if data.get("publish_blockers") != pending or data.get("public_publish_ready") is not linux_public:
        raise ValueError("unexpected publication blockers; refusing even a release draft")
    if set(data.get("checks", [])) != {"module_verify", "go_race_tests", "go_vet", "release_tool_tests"}:
        raise ValueError("source checks did not all pass")
    expected_artifacts = {
        "linux-amd64": (f"anchor_{version}_linux_amd64.tar.gz", "packaged_binary_smoke_passed"),
        "windows-amd64": (f"anchor_{version}_windows_amd64.zip", "cross_compile_only; native runtime acceptance pending"),
    }
    if linux_public:
        del expected_artifacts["windows-amd64"]
        corresponding = data.get("corresponding_source", {})
        name = f"anchor_{version}_source.tar.gz"
        if (corresponding.get("file") != name or corresponding.get("revision") != revision
                or corresponding.get("sha256") != sums[name]
                or corresponding.get("bytes") != (directory / name).stat().st_size
                or corresponding.get("dependencies") != "vendored"
                or corresponding.get("license") != "AGPL-3.0-only"):
            raise ValueError("missing or mismatched corresponding source")
    artifacts = data.get("artifacts", [])
    if len(artifacts) != len(expected_artifacts) or {a.get("target") for a in artifacts} != set(expected_artifacts):
        raise ValueError("missing or duplicate target artifacts")
    for artifact in artifacts:
        name, acceptance = expected_artifacts[artifact["target"]]
        if (artifact.get("file") != name or artifact.get("verification") != acceptance
                or artifact.get("sha256") != sums[name]
                or artifact.get("bytes") != (directory / name).stat().st_size):
            raise ValueError("artifact metadata mismatch")

    standalone = data.get("standalone_artifacts", [])
    if len(standalone) != len(expected_artifacts) or {a.get("target") for a in standalone} != set(expected_artifacts):
        raise ValueError("missing or duplicate standalone executables")
    for artifact in standalone:
        name = standalone_name(version, artifact["target"])
        acceptance = ("standalone_binary_smoke_passed" if artifact["target"] == "linux-amd64"
                      else "cross_compile_only; native runtime acceptance pending")
        if (artifact.get("file") != name or artifact.get("verification") != acceptance
                or artifact.get("sha256") != sums[name]
                or artifact.get("bytes") != (directory / name).stat().st_size):
            raise ValueError("standalone executable metadata mismatch")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    subs = parser.add_subparsers(dest="command", required=True)
    p = subs.add_parser("plan")
    p.add_argument("--event", required=True)
    p.add_argument("--ref", required=True)
    p.add_argument("--run-number", required=True)
    p.add_argument("--output", type=Path, required=True)
    v = subs.add_parser("verify")
    v.add_argument("--directory", type=Path, required=True)
    v.add_argument("--version", required=True)
    v.add_argument("--revision", required=True)
    v.add_argument("--targets", default="linux-amd64,windows-amd64")
    args = parser.parse_args()
    try:
        if args.command == "plan":
            result = plan(args.event, args.ref, args.run_number)
            with args.output.open("a") as f:
                f.write("".join(f"{key}={value}\n" for key, value in result.items()))
        else:
            verify(args.directory, args.version, args.revision, args.targets)
            print("Checksums, metadata and tagged source match; draft only; scope: " + args.targets)
    except (ValueError, KeyError, TypeError, OSError) as exc:
        parser.exit(1, f"CI release validation failed: {exc}\n")


if __name__ == "__main__":
    main()

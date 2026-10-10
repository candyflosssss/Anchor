#!/usr/bin/env python3
"""Container-only compiler / installer packager. Never execute an Anchor binary."""
from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import re
import shutil
import subprocess

ROOT = Path("/src")
OUTPUT = Path("/output")
SCRATCH = Path("/scratch")


def command(argv: list[str], *, cwd: Path = ROOT, capture: bool = False, env=None) -> str:
    print("+ " + " ".join(argv), flush=True)
    result = subprocess.run(argv, cwd=cwd, env=env, check=True, text=True,
                            stdout=subprocess.PIPE if capture else None, timeout=600)
    return result.stdout.strip() if capture else ""


def decode_objects(raw: str):
    decoder = json.JSONDecoder()
    while raw.strip():
        raw = raw.lstrip()
        item, end = decoder.raw_decode(raw)
        yield item
        raw = raw[end:]


def license_text(directory: Path, label: str) -> str:
    files = sorted(p for p in directory.iterdir() if p.is_file() and
                   p.name.upper().startswith(("LICENSE", "COPYING", "COPYRIGHT", "NOTICE")))
    if not files:
        raise RuntimeError(f"Missing dependency notice: {label}")
    parts = ["\n" + "=" * 72 + "\n" + label + "\n"]
    for file in files:
        if file.is_symlink() or file.stat().st_size > 2 << 20:
            raise RuntimeError(f"Invalid notice: {label}/{file.name}")
        parts.append(f"\n--- {file.name} ---\n" + file.read_text(errors="replace"))
    return "\n".join(parts)


def notices(binaries: list[Path]) -> str:
    modules = set()
    # Read build metadata, never launch a native/cross-platform executable.
    for binary in binaries:
        for line in command(["go", "version", "-m", str(binary)], capture=True).splitlines():
            fields = line.split()
            if fields and fields[0] == "dep":
                modules.add(fields[1])
    result = ["Anchor desktop dependency notices. Automatically collected; not a legal completeness audit.\n",
              license_text(SCRATCH / "go-notices", "Go standard library/toolchain")]
    raw = command(["go", "list", "-m", "-json", *sorted(modules)], capture=True)
    for module in decode_objects(raw):
        if module.get("Replace"):
            raise RuntimeError("Replaced dependency modules are not supported")
        result.append(license_text(Path(module["Dir"]), f"{module['Path']}@{module['Version']}"))
    lock = json.loads((ROOT / "desktop/frontend/package-lock.json").read_text())
    for relative, package in sorted(lock["packages"].items()):
        if not relative or package.get("dev"):
            continue
        directory = ROOT / "desktop/frontend" / relative
        if not directory.is_dir() and package.get("optional"):
            continue  # Not installed or bundled on this build host (e.g. macOS fsevents).
        metadata = json.loads((directory / "package.json").read_text())
        label = f"{metadata['name']}@{metadata['version']} ({metadata.get('license', 'see notice')})"
        supplement = ROOT / "desktop/packaging/notices" / (f"{metadata['name']}-{metadata['version']}.LICENSE")
        if supplement.is_file():
            result.append(f"\n{'=' * 72}\n{label} — supplementary notice\n" + supplement.read_text())
        else:
            result.append(license_text(directory, label))
    result.append((ROOT / "desktop/packaging/notices/README.md").read_text())
    result.append("\nMicrosoft Edge WebView2 Runtime bootstrapper: Copyright Microsoft Corporation.\n"
                  "Bundled from the locked Wails module; the runtime is a separate Microsoft product.\n"
                  "Distribution/installation terms: https://developer.microsoft.com/microsoft-edge/webview2/\n")
    return "\n".join(result)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True)
    args = parser.parse_args()
    match = re.fullmatch(r"(\d+)\.(\d+)\.(\d+)-dev\.desktop\.(\d+)", args.version)
    if not match or not (ROOT / "go.mod").is_file() or not SCRATCH.is_dir():
        parser.error("Run through package-desktop.sh in its isolated builder")
    version = args.version
    linux = OUTPUT / "linux-amd64"
    windows = OUTPUT / "windows-amd64"
    executable = linux / "anchor-desktop"
    env = os.environ | {"GOOS": "linux", "GOARCH": "amd64", "CGO_ENABLED": "1"}
    ldflags = f"-s -w -X github.com/candyflosssss/Anchor/internal/core.Version={version}"
    partial = executable.with_suffix(".partial")
    command(["go", "build", "-trimpath", "-buildvcs=false", "-tags", "desktop,production,webkit2_41",
             "-ldflags", ldflags, "-o", str(partial), "./cmd/anchor-desktop"], env=env)
    partial.replace(executable)

    docs = SCRATCH / "docs"
    docs.mkdir()
    shutil.copy2(ROOT / "LICENSE", docs / "LICENSE")
    shutil.copy2(ROOT / "desktop/packaging/README.txt", docs / "README.txt")
    (docs / "THIRD_PARTY_NOTICES.txt").write_text(notices([executable, windows / "anchor-desktop.exe"]))
    for folder in (windows, linux):
        for file in docs.iterdir():
            shutil.copy2(file, folder / file.name)

    # Derive runtime library dependencies from the actual ELF built on Ubuntu 24.04.
    stage = SCRATCH / "deb"
    stage.mkdir()
    (stage / "debian").mkdir()
    (stage / "debian/control").write_text("Source: anchor-desktop\nSection: utils\nPriority: optional\n"
        "Maintainer: Anchor contributors <noreply@invalid.example>\n\nPackage: anchor-desktop\n"
        "Architecture: amd64\nDescription: Anchor desktop development build\n")
    staged_binary = stage / "debian/anchor-desktop/usr/bin/anchor-desktop"
    staged_binary.parent.mkdir(parents=True)
    shutil.copy2(executable, staged_binary)
    deps = command(["dpkg-shlibdeps", "-O", "debian/anchor-desktop/usr/bin/anchor-desktop"], cwd=stage, capture=True)
    prefix = "shlibs:Depends="
    values = [line[len(prefix):] for line in deps.splitlines() if line.startswith(prefix)]
    if len(values) != 1 or "libgtk-3-0t64" not in values[0] or "libwebkit2gtk-4.1-0" not in values[0]:
        raise RuntimeError("Expected GTK3 / WebKitGTK 4.1 dependencies were not detected")
    dependencies = values[0]
    shutil.rmtree(stage / "debian")  # Only this freshly-created packaging scratch directory.
    payloads = {
        "usr/bin/anchor-desktop": executable,
        "usr/share/applications/anchor-desktop.desktop": ROOT / "desktop/packaging/linux/anchor-desktop.desktop",
        "usr/share/icons/hicolor/scalable/apps/anchor-desktop.svg": ROOT / "desktop/branding/anchor.svg",
        "usr/share/icons/hicolor/256x256/apps/anchor-desktop.png": ROOT / "desktop/branding/anchor.png",
        "usr/share/doc/anchor-desktop/copyright": docs / "LICENSE",
        "usr/share/doc/anchor-desktop/README.txt": docs / "README.txt",
        "usr/share/doc/anchor-desktop/THIRD_PARTY_NOTICES.txt": docs / "THIRD_PARTY_NOTICES.txt",
    }
    for relative, source in payloads.items():
        destination = stage / relative
        destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(source, destination)
        destination.chmod(0o755 if relative == "usr/bin/anchor-desktop" else 0o644)
    control = stage / "DEBIAN"
    control.mkdir()
    deb_version = version.replace("-dev.desktop.", "~dev.desktop.")
    size = sum((p.stat().st_size + 1023) // 1024 for p in stage.rglob("*") if p.is_file())
    (control / "control").write_text(
        f"Package: anchor-desktop\nVersion: {deb_version}\nArchitecture: amd64\n"
        "Maintainer: Anchor contributors <noreply@invalid.example>\nSection: utils\nPriority: optional\n"
        f"Installed-Size: {size}\nDepends: {dependencies}\n"
        "Homepage: https://github.com/candyflosssss/Anchor\n"
        "Description: Anchor local provider desktop (development build)\n"
        " Opens the graphical interface and automatically starts or reuses the local\n"
        " core. Closing the window leaves the core running. No system service or\n"
        " login autostart is installed; user state is retained when removing.\n"
        " This development package has not undergone native acceptance.\n")
    deb = linux / f"anchor-desktop_{version}_amd64.deb"
    command(["dpkg-deb", "--root-owner-group", "-Zxz", "-z6", "--threads-max=2", "--build", str(stage), str(deb)])
    (linux / "RUNTIME_DEPENDENCIES.txt").write_text(dependencies + "\n")

    wails = Path(command(["go", "list", "-m", "-f", "{{.Dir}}", "github.com/wailsapp/wails/v2"], capture=True))
    setup = windows / f"Anchor-Desktop-{version}-windows-amd64-setup.exe"
    command(["makensis", "-WX", "-V3", f"-DAPP_VERSION={version}", f"-DNUMERIC_VERSION={'.'.join(match.groups())}",
             f"-DOUTPUT_FILE={setup}", f"-DAPP_EXE={windows / 'anchor-desktop.exe'}",
             f"-DDOCS_DIR={docs}", f"-DICON_FILE={ROOT / 'desktop/branding/anchor.ico'}",
             f"-DWEBVIEW_BOOTSTRAPPER={wails / 'internal/webview2runtime/MicrosoftEdgeWebview2Setup.exe'}",
             str(ROOT / "desktop/packaging/windows/installer.nsi")])


if __name__ == "__main__":
    main()

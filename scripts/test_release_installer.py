#!/usr/bin/env python3
"""Offline installer regression tests; never change real HOME or fetch upstream."""
from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
VERSION = "0.1.0-alpha.9"
PAYLOAD = b"#!/bin/sh\nprintf 'test executable only\\n'\n"


@unittest.skipUnless(os.name == "posix" and shutil.which("sh"), "POSIX shell required")
class LinuxInstallerTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="anchor-install-test-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.home = self.root / "home"
        self.bin = self.root / "tools"
        self.bin.mkdir()
        self.home.mkdir()
        self.destination = self.home / ".local" / "bin"
        self.config = self.root / "fixture.json"
        self.log = self.root / "requests.jsonl"
        self.fixture = {
            "os": "Linux", "arch": "x86_64", "tag": "v" + VERSION,
            "sha": hashlib.sha256(PAYLOAD).hexdigest(), "payload": PAYLOAD.decode(),
        }
        self.env = dict(os.environ, HOME=str(self.home), PATH=str(self.bin) + os.pathsep + os.environ["PATH"],
                        INSTALL_FIXTURE=str(self.config), INSTALL_LOG=str(self.log))
        python = shutil.which("python3") or shutil.which("python")
        self.assertIsNotNone(python)
        self.make_tool("curl", f"#!{python}\n" + r'''
import json, os, sys
from pathlib import Path
args = sys.argv[1:]
f = json.loads(Path(os.environ["INSTALL_FIXTURE"]).read_text())
url = args[-1]
with open(os.environ["INSTALL_LOG"], "a") as log:
    log.write(json.dumps(args) + "\n")
assert args[0] == "-q"
assert args[args.index("--proto") + 1] == "=https"
assert args[args.index("--proto-redir") + 1] == "=https"
assert "--fail" in args and "--max-time" in args
if f.get("fail") and f["fail"] in url:
    sys.exit(22)
if url == "https://api.github.com/repos/candyflosssss/Anchor/releases?per_page=1":
    data = f.get("metadata", json.dumps([{"tag_name": f["tag"], "draft": False, "prerelease": True}]))
elif url.endswith("/SHA256SUMS"):
    name = "anchor_" + f["tag"][1:] + "_linux_amd64"
    data = f.get("checksums", f["sha"] + "  " + name + "\n")
else:
    expected = "https://github.com/candyflosssss/Anchor/releases/download/" + f["tag"] + "/anchor_" + f["tag"][1:] + "_linux_amd64"
    assert url == expected, (url, expected)
    data = f["payload"]
Path(args[args.index("--output") + 1]).write_bytes(data.encode())
''')
        self.make_tool("uname", f"#!{python}\n" + r'''
import json, os, sys
from pathlib import Path
f = json.loads(Path(os.environ["INSTALL_FIXTURE"]).read_text())
print(f["os"] if sys.argv[1] == "-s" else f["arch"])
''')
        self.sentinels = [self.home / ".codex" / "config.toml", self.home / ".config" / "anchor" / "state.db"]
        for path in self.sentinels:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(b"do not edit")

    def make_tool(self, name, text):
        path = self.bin / name
        path.write_text(text)
        path.chmod(0o755)

    def run_install(self, *args, pipe=False, success=True):
        self.config.write_text(json.dumps(self.fixture))
        if pipe:
            command = ["sh", "-s", "--", *args]
            text = (ROOT / "scripts/install.sh").read_text()
        else:
            command = ["sh", str(ROOT / "scripts/install.sh"), *args]
            text = None
        result = subprocess.run(command, input=text, capture_output=True, text=True, env=self.env, timeout=20)
        if success:
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        else:
            self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        for path in self.sentinels:
            self.assertEqual(path.read_bytes(), b"do not edit")
        if self.destination.exists():
            self.assertFalse(list(self.destination.glob(".anchor-install*")))
        return result

    def requests(self):
        return [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []

    def test_default_downloads_latest_published_prerelease_only_one_program(self):
        result = self.run_install()
        self.assertEqual((self.destination / "anchor").read_bytes(), PAYLOAD)
        self.assertEqual((self.destination / "anchor").stat().st_mode & 0o777, 0o755)
        self.assertEqual(list(self.destination.iterdir()), [self.destination / "anchor"])
        self.assertEqual(len(self.requests()), 3)
        self.assertIn("export PATH=", result.stdout)

    def test_pipe_input_and_pinned_version_skip_discovery(self):
        self.run_install("--version", "v" + VERSION, pipe=True)
        self.assertEqual(len(self.requests()), 2)
        self.assertEqual((self.destination / "anchor").read_bytes(), PAYLOAD)

    def test_pretty_metadata_and_uppercase_hash(self):
        self.fixture["metadata"] = json.dumps([{"tag_name": "v" + VERSION}], indent=2)
        self.fixture["sha"] = self.fixture["sha"].upper()
        self.run_install()

    def test_custom_path_spaces_and_quotes_path_hint_is_shell_safe(self):
        self.destination = self.root / "a dir ' with quote"
        result = self.run_install("--install-dir", str(self.destination))
        self.assertEqual((self.destination / "anchor").read_bytes(), PAYLOAD)
        hint = next(line.strip() for line in result.stdout.splitlines() if line.startswith("  export PATH="))
        check = subprocess.run(["sh", "-c", hint + '; printf "%s" "$PATH"'], env=self.env, capture_output=True, text=True, timeout=5)
        self.assertEqual(check.returncode, 0, check.stderr)
        self.assertTrue(check.stdout.startswith(str(self.destination) + ":"), check.stdout)

    def test_existing_file_is_not_implicitly_overwritten(self):
        self.destination.mkdir(parents=True)
        old = self.destination / "anchor"
        old.write_bytes(b"old")
        self.run_install(success=False)
        self.assertEqual(old.read_bytes(), b"old")
        self.assertFalse(self.requests())

    def test_explicit_replace_keeps_stable_path(self):
        self.destination.mkdir(parents=True)
        (self.destination / "anchor").write_bytes(b"old")
        self.run_install("--replace")
        self.assertEqual((self.destination / "anchor").read_bytes(), PAYLOAD)

    def test_hash_mismatch_keeps_existing_file(self):
        self.destination.mkdir(parents=True)
        old = self.destination / "anchor"
        old.write_bytes(b"old")
        self.fixture["sha"] = "0" * 64
        self.run_install("--replace", success=False)
        self.assertEqual(old.read_bytes(), b"old")

    def test_symlink_and_broken_symlink_destinations_are_rejected(self):
        self.destination.mkdir(parents=True)
        dest = self.destination / "anchor"
        victim = self.root / "victim"
        victim.write_bytes(b"untouched")
        for target in [victim, self.root / "missing"]:
            with self.subTest(target=target):
                dest.symlink_to(target)
                self.run_install("--replace", success=False)
                self.assertTrue(dest.is_symlink())
                dest.unlink()
                self.assertEqual(victim.read_bytes(), b"untouched")
        self.assertFalse(self.requests())

    def test_directory_destination_is_rejected(self):
        (self.destination / "anchor").mkdir(parents=True)
        self.run_install("--replace", success=False)
        self.assertFalse(self.requests())

    def test_preexisting_lock_is_not_removed(self):
        lock = self.destination / ".anchor-install.lock"
        lock.mkdir(parents=True)
        self.config.write_text(json.dumps(self.fixture))
        result = subprocess.run(["sh", str(ROOT / "scripts/install.sh")], env=self.env, capture_output=True, text=True, timeout=20)
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue(lock.is_dir())
        self.assertFalse(self.requests())

    def test_download_failures_install_nothing(self):
        for part in ["api.github.com", "SHA256SUMS", "linux_amd64"]:
            with self.subTest(part=part):
                self.fixture["fail"] = part
                self.run_install(success=False)
                self.assertFalse((self.destination / "anchor").exists())

    def test_unsupported_os_or_architecture_downloads_nothing(self):
        for system, arch in [("Darwin", "x86_64"), ("Linux", "aarch64"), ("Linux", "i686"), ("Linux", "armv7l")]:
            with self.subTest(system=system, arch=arch):
                self.fixture.update(os=system, arch=arch)
                self.run_install(success=False)
                self.assertFalse(self.destination.exists())
                self.assertFalse(self.requests())

    def test_invalid_versions_and_arguments_download_nothing(self):
        args_list = [("--version", v) for v in ["bad", "v", "../x", "0.1.0-alpha.x", "00.1.0", "1.0.0\n2.0.0", "1.0.0;touch x"]]
        args_list += [("--version",), ("--unknown",), ("--install-dir", "relative"), ("--install-dir", "/a:b")]
        for args in args_list:
            with self.subTest(args=args):
                self.run_install(*args, success=False)
                self.assertFalse(self.requests())
                self.assertFalse(self.destination.exists())

    def test_no_release_bad_tag_or_multiple_tags_fail_closed(self):
        for metadata in ["[]", "{}", 'not JSON', '{"tag_name":"bad"}', '{"tag_name":"v1.0.0;echo hi"}',
                         '[{"tag_name":"v1.0.0"},{"tag_name":"v2.0.0"}]',
                         '[\n{"tag_name":"v1.0.0"},\n{"tag_name":"v2.0.0"}\n]' ]:
            with self.subTest(metadata=metadata):
                self.fixture["metadata"] = metadata
                self.run_install(success=False)
                self.assertFalse((self.destination / "anchor").exists())

    def test_missing_duplicate_malformed_or_wrong_asset_checksum_fails(self):
        asset = f"anchor_{VERSION}_linux_amd64"
        sha = self.fixture["sha"]
        for sums in ["", f"{sha}  other\n", f"{sha}  {asset}\n{sha}  {asset}\n", f"bad  {asset}\n", f"{sha}  {asset} extra\n"]:
            with self.subTest(sums=sums):
                self.fixture["checksums"] = sums
                self.run_install(success=False)
                self.assertFalse((self.destination / "anchor").exists())

    def test_help_does_not_download_or_install(self):
        result = self.run_install("--help")
        self.assertIn("--replace", result.stdout)
        self.assertFalse(self.destination.exists())
        self.assertFalse(self.requests())


POWERSHELL = shutil.which("powershell.exe") or shutil.which("pwsh")


@unittest.skipUnless(os.name == "nt" and POWERSHELL, "native Windows PowerShell required")
class WindowsInstallerTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="anchor-win-install-test-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.destination = self.root / "program only"
        self.config = self.root / "fixture.json"
        self.log = self.root / "requests.log"
        self.fixture = {
            "tag": "v" + VERSION, "sha": hashlib.sha256(PAYLOAD).hexdigest(),
            "payload": PAYLOAD.decode(), "fail": "", "metadata": "", "checksums": "", "redirect": "",
        }
        self.env = dict(os.environ, INSTALL_TEST_FIXTURE=str(self.config), INSTALL_TEST_LOG=str(self.log))
        self.env["LOCALAPPDATA"] = str(self.root / "localappdata")
        self.env["APPDATA"] = str(self.root / "appdata")
        self.env["USERPROFILE"] = str(self.root / "profile")
        self.sentinel = Path(self.env["APPDATA"]) / "anchor" / "state.db"
        self.sentinel.parent.mkdir(parents=True)
        self.sentinel.write_bytes(b"unchanged")
        self.codex_config = Path(self.env["USERPROFILE"]) / ".codex" / "config.toml"
        self.codex_config.parent.mkdir(parents=True)
        self.codex_config.write_bytes(b"do not edit Codex")
        source = (ROOT / "scripts/install.ps1").read_text()
        # Replace exactly the network-construction seam in a test-owned copy.
        # All production download/redirect/checksum/install/cleanup logic remains.
        seam = "[Net.HttpWebRequest]::Create($uri)"
        self.assertEqual(source.count(seam), 1)
        self.patched = self.root / "installer.ps1"
        self.patched.write_text(source.replace(seam, "(New-TestRequest $uri)"), encoding="utf-8")
        self.helper = self.root / "mock-network.ps1"
        self.helper.write_text(r'''
function New-TestRequest([Uri] $Uri) {
    $request = [PSCustomObject]@{
        Uri = $Uri; AllowAutoRedirect = $false; Timeout = 0; ReadWriteTimeout = 0
        UserAgent = ''; Accept = ''
    }
    $request | Add-Member ScriptMethod GetResponse {
        $f = Get-Content -LiteralPath $env:INSTALL_TEST_FIXTURE -Raw | ConvertFrom-Json
        $url = $this.Uri.AbsoluteUri
        [IO.File]::AppendAllText($env:INSTALL_TEST_LOG, $url + "`n")
        if ($this.AllowAutoRedirect) { throw 'Automatic redirects must be disabled.' }
        if ($this.Uri.Scheme -ne 'https') { throw 'HTTPS required.' }
        if ($f.fail -and $url.Contains($f.fail)) { throw 'Simulated HTTP failure.' }
        $status = 200
        $headers = @{}
        if ($f.redirect) {
            $status = 302
            $headers['Location'] = $f.redirect
            $data = ''
        } elseif ($url -like 'https://api.github.com/*') {
            $data = $f.metadata
        } elseif ($url.EndsWith('/SHA256SUMS')) {
            $data = $f.checksums
        } else {
            $expected = 'https://github.com/candyflosssss/Anchor/releases/download/' + $f.tag + '/anchor_' + $f.tag.Substring(1) + '_windows_amd64.exe'
            if ($url -cne $expected) { throw "Unexpected asset: $url" }
            $data = $f.payload
        }
        $response = [PSCustomObject]@{ StatusCode = $status; Headers = $headers; Bytes = [Text.Encoding]::UTF8.GetBytes($data) }
        $response | Add-Member ScriptMethod GetResponseStream { [IO.MemoryStream]::new($this.Bytes, $false) }
        $response | Add-Member ScriptMethod Dispose { }
        return $response
    }
    return $request
}
''', encoding="utf-8")
        self.bootstrap = self.root / "run.ps1"
        def quote(path):
            return "'" + str(path).replace("'", "''") + "'"
        self.bootstrap.write_text(
            "param([string] $Version='', [string] $InstallDir='', [switch] $Replace, [switch] $Help)\n"
            f". {quote(self.helper)}\n"
            "$params = @{Version=$Version; InstallDir=$InstallDir; Replace=$Replace; Help=$Help}\n"
            f"try {{ & {quote(self.patched)} @params }} catch {{ Write-Error $_; exit 1 }}\n",
            encoding="utf-8",
        )

    def run_install(self, *args, success=True, default_dir=False):
        if not self.fixture["metadata"]:
            self.fixture["metadata"] = json.dumps([{"tag_name": self.fixture["tag"], "draft": False}])
        if not self.fixture["checksums"]:
            self.fixture["checksums"] = self.fixture["sha"] + "  anchor_" + self.fixture["tag"][1:] + "_windows_amd64.exe\n"
        self.config.write_text(json.dumps(self.fixture))
        command = [POWERSHELL, "-NoProfile", "-NonInteractive", "-File", str(self.bootstrap), *args]
        if not default_dir:
            command += ["-InstallDir", str(self.destination)]
        result = subprocess.run(command, capture_output=True, text=True, env=self.env, timeout=30)
        self.assertEqual(result.returncode == 0, success, result.stdout + result.stderr)
        self.assertEqual(self.sentinel.read_bytes(), b"unchanged")
        self.assertEqual(self.codex_config.read_bytes(), b"do not edit Codex")
        if self.destination.exists():
            self.assertFalse(list(self.destination.glob(".anchor-install*")))
        return result

    def test_production_script_parses_in_powershell(self):
        path = str(ROOT / "scripts/install.ps1").replace("'", "''")
        command = "$t=$null; $e=$null; [System.Management.Automation.Language.Parser]::ParseFile('" + path + "',[ref]$t,[ref]$e) | Out-Null; if ($e.Count) { $e | Out-String | Write-Error; exit 1 }"
        result = subprocess.run([POWERSHELL, "-NoProfile", "-NonInteractive", "-Command", command], capture_output=True, text=True, timeout=20)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_latest_published_install_and_pinned_replacement(self):
        self.run_install()
        self.assertEqual((self.destination / "anchor.exe").read_bytes(), PAYLOAD)
        self.assertEqual(list(self.destination.iterdir()), [self.destination / "anchor.exe"])
        self.run_install(success=False)
        self.run_install("-Version", "v" + VERSION, "-Replace")
        self.assertEqual((self.destination / "anchor.exe").read_bytes(), PAYLOAD)

    def test_default_user_directory_and_no_path_edits(self):
        self.destination = Path(self.env["LOCALAPPDATA"]) / "Anchor" / "bin"
        self.run_install(default_dir=True)
        self.assertEqual((self.destination / "anchor.exe").read_bytes(), PAYLOAD)

    def test_corrupt_download_leaves_existing_binary_unchanged(self):
        self.destination.mkdir()
        (self.destination / "anchor.exe").write_bytes(b"old")
        self.fixture["sha"] = "0" * 64
        self.run_install("-Replace", success=False)
        self.assertEqual((self.destination / "anchor.exe").read_bytes(), b"old")

    def test_missing_duplicate_and_invalid_checksums_fail(self):
        entry = self.fixture["sha"] + f"  anchor_{VERSION}_windows_amd64.exe\n"
        for sums in ["no match\n", entry * 2, f"bad  anchor_{VERSION}_windows_amd64.exe\n", entry.rstrip() + " extra\n"]:
            with self.subTest(sums=sums):
                self.fixture["checksums"] = sums
                self.run_install(success=False)
                self.assertFalse((self.destination / "anchor.exe").exists())

    def test_no_release_invalid_version_unsupported_arch_and_help(self):
        self.fixture["metadata"] = "[]"
        self.run_install(success=False)
        self.run_install("-Version", "../x", success=False)
        self.env["PROCESSOR_ARCHITECTURE"] = "ARM64"
        self.env.pop("PROCESSOR_ARCHITEW6432", None)
        self.run_install("-Version", VERSION, success=False)
        self.run_install("-Help")
        self.assertFalse((self.destination / "anchor.exe").exists())

    def test_https_downgrade_and_redirect_loops_are_rejected(self):
        for redirect in ["http://example.invalid/program", "https://example.invalid/loop"]:
            with self.subTest(redirect=redirect):
                self.fixture["redirect"] = redirect
                self.run_install(success=False)
                self.assertFalse((self.destination / "anchor.exe").exists())

    def test_download_failures_leave_no_staging_files(self):
        for part in ["api.github.com", "SHA256SUMS", "windows_amd64"]:
            with self.subTest(part=part):
                self.fixture["fail"] = part
                self.run_install(success=False)
                self.assertFalse((self.destination / "anchor.exe").exists())

    def test_existing_lock_is_not_touched(self):
        self.destination.mkdir()
        lock = self.destination / ".anchor-install.lock"
        lock.write_bytes(b"not ours")
        # Do not assert lock cleanup: this lock belongs to someone else.
        dest = self.destination
        self.destination = self.root / "unused"
        self.run_install("-InstallDir", str(dest), success=False, default_dir=True)
        self.assertEqual(lock.read_bytes(), b"not ours")


if __name__ == "__main__":
    unittest.main()

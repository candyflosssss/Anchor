"""Release-tool unit tests use fake binaries, never a distributable build."""
from __future__ import annotations

import argparse
from contextlib import redirect_stderr
import importlib.util
import io
import json
from pathlib import Path
import tarfile
import tempfile
import unittest
from unittest.mock import patch
import zipfile

spec = importlib.util.spec_from_file_location("anchor_release", Path(__file__).with_name("release.py"))
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)


class ReleaseTests(unittest.TestCase):
    def test_prerelease_version_and_targets(self):
        args = release.arguments(["--version", "0.1.0-alpha.1"])
        self.assertEqual(args.targets, ["linux-amd64", "windows-amd64"])
        for version in ["0.1.0", "v0.1.0-alpha.1", "0.1.0-alpha.1;echo", "01.0.0-alpha.1"]:
            with self.subTest(version=version), redirect_stderr(io.StringIO()), self.assertRaises(SystemExit):
                release.arguments(["--version", version])

    def test_dirty_requires_internal_and_targets_are_unique(self):
        for options in [["--allow-dirty"], ["--targets", "linux-amd64,linux-amd64"],
                        ["--targets", "darwin-arm64"], ["--plan", "--check"]]:
            with self.subTest(options=options), redirect_stderr(io.StringIO()), self.assertRaises(SystemExit):
                release.arguments(["--version", "0.1.0-alpha.1", *options])

    def test_plan_does_not_run_commands_or_write_files(self):
        with patch.object(release, "command", side_effect=AssertionError("unexpected command")), redirect_stderr(io.StringIO()):
            with patch("sys.stdout", new_callable=io.StringIO) as output:
                self.assertEqual(release.main(["--version", "0.1.0-alpha.1", "--plan"]), 0)
                plan = json.loads(output.getvalue())
                self.assertFalse(plan["uploads"])
                self.assertEqual(plan["standalone_files"], ["anchor_0.1.0-alpha.1_linux_amd64",
                                                          "anchor_0.1.0-alpha.1_windows_amd64.exe"])

    def test_archives_have_a_single_root_and_correct_permissions(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            payload = root / "anchor_0.1.0-alpha.1_linux_amd64"
            payload.mkdir()
            (payload / "anchor").write_bytes(b"fake-not-executable")
            (payload / "README.md").write_text("guide")
            archive = release.archive(payload, root, "linux-amd64", 1760000000)
            with tarfile.open(archive) as t:
                files = {m.name: m for m in t.getmembers()}
                self.assertEqual(set(files), {payload.name, payload.name + "/anchor", payload.name + "/README.md"})
                self.assertEqual(files[payload.name + "/anchor"].mode, 0o755)
                self.assertEqual(files[payload.name + "/README.md"].mode, 0o644)
                self.assertEqual(files[payload.name + "/anchor"].uid, 0)
            (payload / "anchor.exe").write_bytes(b"fake-not-executable")
            archive = release.archive(payload, root, "windows-amd64", 1760000000)
            with zipfile.ZipFile(archive) as z:
                self.assertIn(payload.name + "/anchor.exe", z.namelist())
                self.assertTrue(all(n.startswith(payload.name + "/") for n in z.namelist()))

    def test_symlink_payload_is_rejected(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            payload = root / "payload"
            payload.mkdir()
            (payload / "bad").symlink_to(root / "unowned-state")
            with self.assertRaises(RuntimeError):
                release.archive(payload, root, "linux-amd64", 1760000000)

    def test_checksums_match_manifest_and_change_when_tampered(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            artifact = root / "anchor.zip"
            artifact.write_bytes(b"test")
            release.save_manifest(root, {"status": "packaged"})
            for line in (root / "SHA256SUMS").read_text().splitlines():
                sha, name = line.split("  ")
                self.assertEqual(sha, release.digest(root / name))
            old = release.digest(artifact)
            artifact.write_bytes(b"tampered")
            self.assertNotEqual(old, release.digest(artifact))

    def test_checks_download_full_module_graph_before_verify(self):
        with patch.object(release, "command", return_value="") as cmd:
            self.assertEqual(release.checks({}),
                             ["module_verify", "go_race_tests", "go_vet", "release_tool_tests"])
        self.assertEqual(cmd.call_args_list[0].args[0], ["go", "mod", "download", "all"])
        self.assertEqual(cmd.call_args_list[1].args[0], ["go", "mod", "verify"])
        self.assertIn("test_release*.py", cmd.call_args_list[-1].args[0])

    def fixture(self, root: Path):
        (root / "docs/release").mkdir(parents=True)
        (root / "docs/cli").mkdir()
        (root / "docs/release/package-readme.md").write_text("guide")
        (root / "docs/cli/commands.md").write_text("command reference")
        (root / "owner.token").write_text("MUST_NOT_SHIP")
        (root / "state.db").write_text("MUST_NOT_SHIP")

    @staticmethod
    def fake_command(argv, **kwargs):
        if argv[:2] == ["go", "build"]:
            Path(argv[argv.index("-o") + 1]).write_bytes(b"FAKE_BINARY_NOT_FOR_DISTRIBUTION")
        return "go version test linux/amd64\n"

    def test_build_whitelists_payload_and_records_windows_as_unverified(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            self.fixture(root)
            args = release.arguments(["--version", "0.1.0-alpha.1", "--internal", "--allow-dirty"])
            source = {"revision": "test", "dirty": True, "inputs_sha256": "fixed"}
            with patch.object(release, "ROOT", root), patch.object(release, "command", side_effect=self.fake_command), \
                    patch.object(release, "dependencies", return_value=([], "notices")), \
                    patch.object(release, "input_digest", return_value="fixed"), \
                    patch.object(release, "check_archive", side_effect=["packaged_binary_smoke_passed", "cross_compile_only; native runtime acceptance pending"]):
                output = release.build(args, {}, source, ["test"])
                manifest = json.loads((output / "release.json").read_text())
                self.assertEqual(manifest["status"], "packaged")
                self.assertFalse(manifest["public_publish_ready"])
                self.assertEqual(len(manifest["standalone_artifacts"]), 2)
                for item in manifest["standalone_artifacts"]:
                    binary = output / item["file"]
                    self.assertEqual(binary.read_bytes(), b"FAKE_BINARY_NOT_FOR_DISTRIBUTION")
                    self.assertEqual(item["sha256"], release.digest(binary))
                    self.assertEqual(item["bytes"], binary.stat().st_size)
                    self.assertEqual(binary.stat().st_mode & 0o777,
                                     0o755 if item["target"] == "linux-amd64" else 0o644)
                self.assertEqual(manifest["standalone_artifacts"][0]["verification"], "standalone_binary_smoke_passed")
                self.assertEqual(manifest["standalone_artifacts"][1]["verification"],
                                 "cross_compile_only; native runtime acceptance pending")
                self.assertTrue(any("windows-amd64" in b for b in manifest["publish_blockers"]))
                with tarfile.open(next(output.glob("*.tar.gz"))) as t:
                    names = {Path(m.name).name for m in t.getmembers() if m.isfile()}
                    self.assertEqual(names, {"anchor", "README.md", "CLI-COMMANDS.md", "THIRD_PARTY_NOTICES.txt", "LEGAL-NOTICE.txt", "BUILD-INFO.json"})
                    self.assertFalse(any(b"MUST_NOT_SHIP" in t.extractfile(m).read() for m in t.getmembers() if m.isfile()))
                with self.assertRaises(RuntimeError):
                    release.build(args, {}, source, ["test"])
                self.assertFalse((root / "dist/.release-lock").exists())

    def test_public_linux_exports_license_notices_source_and_has_no_windows_blocker(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            self.fixture(root)
            (root / "LICENSE").write_text("AGPL fixture")
            args = release.arguments(["--version", "0.1.0-alpha.3", "--targets", "linux-amd64"])
            source = {"revision": "a" * 40, "dirty": False, "inputs_sha256": "fixed"}
            def fake_export(output, scratch, version, source, env, epoch):
                result = output / f"anchor_{version}_source.tar.gz"
                result.write_bytes(b"offline source fixture")
                return result
            with patch.object(release, "ROOT", root), patch.object(release, "command", side_effect=self.fake_command), \
                    patch.object(release, "dependencies", return_value=([], "notices")), \
                    patch.object(release, "input_digest", return_value="fixed"), \
                    patch.object(release, "export_source", side_effect=fake_export), \
                    patch.object(release, "check_archive", return_value="packaged_binary_smoke_passed"):
                output = release.build(args, {}, source, ["test"])
            manifest = json.loads((output / "release.json").read_text())
            self.assertTrue(manifest["public_publish_ready"])
            self.assertEqual(manifest["publish_blockers"], [])
            self.assertEqual((output / "LICENSE").read_text(), "AGPL fixture")
            self.assertEqual((output / "THIRD_PARTY_NOTICES.txt").read_text(), "notices")
            entry = manifest["corresponding_source"]
            self.assertEqual(entry["revision"], source["revision"])
            self.assertEqual(entry["sha256"], release.digest(output / entry["file"]))
            self.assertEqual(entry["dependencies"], "vendored")
            self.assertIn(entry["file"], (output / "SHA256SUMS").read_text())
            self.assertIn("AGPL-3.0-only", (output / "RELEASE_NOTES.md").read_text())
            self.assertFalse(list(output.glob("*windows*")))

    def test_source_export_uses_only_committed_snapshot_and_vendors_dependencies(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            scratch, output = root / "scratch", root / "output"
            scratch.mkdir(); output.mkdir()
            (root / "owner.token").write_text("MUST_NOT_SHIP")
            calls = []
            def fake(argv, **kwargs):
                calls.append((argv, kwargs))
                if argv[:2] == ["git", "archive"]:
                    snapshot = Path(next(a.split("=", 1)[1] for a in argv if a.startswith("--output=")))
                    with tarfile.open(snapshot, "w") as t:
                        for name in ("LICENSE", "go.mod", "cmd/anchor/main.go"):
                            content = b"committed source fixture"
                            info = tarfile.TarInfo(name); info.size = len(content)
                            t.addfile(info, io.BytesIO(content))
                elif argv == ["go", "mod", "vendor"]:
                    vendor = kwargs["cwd"] / "vendor"
                    vendor.mkdir()
                    (vendor / "modules.txt").write_text("dependency fixture")
                return ""
            with patch.object(release, "ROOT", root), patch.object(release, "command", side_effect=fake):
                source = release.export_source(output, scratch, "0.1.0-alpha.3", {"revision": "a" * 40}, {}, 1700000000)
            self.assertEqual(calls[0][0][-1], "a" * 40)
            with tarfile.open(source) as t:
                names = {m.name for m in t.getmembers()}
                self.assertTrue(any(n.endswith("vendor/modules.txt") for n in names))
                self.assertTrue(any(n.endswith("PUBLIC-BUILD.md") for n in names))
                self.assertFalse(any("owner.token" in n for n in names))

    def test_failed_native_smoke_retains_only_marked_partial_output(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            self.fixture(root)
            args = release.arguments(["--version", "0.1.0-alpha.2", "--internal", "--allow-dirty", "--targets", "linux-amd64"])
            with patch.object(release, "ROOT", root), patch.object(release, "command", side_effect=self.fake_command), \
                    patch.object(release, "dependencies", return_value=([], "notices")), \
                    patch.object(release, "check_archive", side_effect=RuntimeError("smoke failed")), redirect_stderr(io.StringIO()):
                with self.assertRaises(RuntimeError):
                    release.build(args, {}, {"dirty": True, "inputs_sha256": "fixed"}, [])
            self.assertFalse((root / "dist/0.1.0-alpha.2").exists())
            partial = list((root / "dist").glob("*.failed-*"))
            self.assertEqual(len(partial), 1)
            self.assertEqual(json.loads((partial[0] / "release.json").read_text())["status"], "partial")
            self.assertFalse((root / "dist/.release-lock").exists())

    def test_standalone_export_refuses_symlinks_and_existing_files(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            binary = root / "built"
            binary.write_bytes(b"fake")
            link = root / "link"
            link.symlink_to(binary)
            with self.assertRaises(RuntimeError):
                release.export_standalone(link, root, "linux-amd64", "0.1.0-alpha.1")
            exported = release.export_standalone(binary, root, "linux-amd64", "0.1.0-alpha.1")
            with self.assertRaises(FileExistsError):
                release.export_standalone(binary, root, "linux-amd64", "0.1.0-alpha.1")
            self.assertEqual(exported.read_bytes(), binary.read_bytes())

    def test_failed_standalone_smoke_preserves_partial_without_archive(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            self.fixture(root)
            args = release.arguments(["--version", "0.1.0-alpha.4", "--internal", "--allow-dirty",
                                      "--targets", "linux-amd64"])
            with patch.object(release, "ROOT", root), patch.object(release, "command", side_effect=self.fake_command), \
                    patch.object(release, "dependencies", return_value=([], "notices")), \
                    patch.object(release, "check_standalone", side_effect=RuntimeError("single-file smoke failed")), \
                    redirect_stderr(io.StringIO()):
                with self.assertRaises(RuntimeError):
                    release.build(args, {}, {"dirty": True, "inputs_sha256": "fixed"}, [])
            partial, = (root / "dist").glob("*.failed-*")
            manifest = json.loads((partial / "release.json").read_text())
            self.assertEqual(manifest["status"], "partial")
            self.assertEqual(manifest["standalone_artifacts"][0]["verification"], "pending")
            self.assertFalse(list(partial.glob("*.tar.gz")))
            self.assertTrue((partial / manifest["standalone_artifacts"][0]["file"]).is_file())
            self.assertFalse((root / "dist/.release-lock").exists())

    def test_prepackage_failure_removes_own_stage_and_lock(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            self.fixture(root)
            args = release.arguments(["--version", "0.1.0-alpha.3", "--internal", "--allow-dirty"])
            with patch.object(release, "ROOT", root), patch.object(release, "command", side_effect=self.fake_command), \
                    patch.object(release, "dependencies", side_effect=RuntimeError("license missing")):
                with self.assertRaises(RuntimeError):
                    release.build(args, {}, {"dirty": True, "inputs_sha256": "fixed"}, [])
            self.assertEqual(list((root / "dist").iterdir()), [])

    def test_environment_drops_ambient_cross_compile_and_flags(self):
        with patch.dict("os.environ", {"GOOS": "windows", "GOARCH": "arm64", "CGO_ENABLED": "0", "GOFLAGS": "-race"}):
            env = release.environment()
        self.assertNotIn("GOOS", env)
        self.assertNotIn("GOARCH", env)
        self.assertNotIn("CGO_ENABLED", env)
        self.assertEqual(env["GOFLAGS"], "-p=1")
        self.assertEqual(env["GOWORK"], "off")


if __name__ == "__main__":
    unittest.main()

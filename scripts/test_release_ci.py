"""Offline tests for CI trigger selection and draft publication gates."""
import hashlib
import json
from pathlib import Path
import tempfile
import unittest

import ci_release


class CIReleaseTests(unittest.TestCase):
    def test_manual_is_internal_even_when_selected_ref_is_a_tag(self):
        for ref in ("refs/heads/main", "refs/tags/v1.2.3-alpha.1"):
            self.assertEqual(ci_release.plan("workflow_dispatch", ref, "12"),
                             {"version": "0.0.0-alpha.12", "internal": "true", "draft": "false", "targets": "linux-amd64,windows-amd64"})

    def test_only_valid_prerelease_tags_can_create_drafts(self):
        for channel in ("alpha", "beta", "rc"):
            self.assertEqual(ci_release.plan("push", f"refs/tags/v1.2.3-{channel}.2", "12"),
                             {"version": f"1.2.3-{channel}.2", "internal": "false", "draft": "true", "targets": "linux-amd64,windows-amd64"})
        for event, ref, number in (("pull_request", "refs/tags/v1.2.3-alpha.1", "1"),
                                   ("push", "refs/heads/main", "1"),
                                   ("push", "refs/tags/v1.2.3", "1"),
                                   ("push", "refs/tags/v1.2.3-alpha.1\nevil=true", "1"),
                                   ("workflow_dispatch", "refs/heads/main", "1\nevil=true")):
            with self.subTest(ref=ref, number=number), self.assertRaises(ValueError):
                ci_release.plan(event, ref, number)

    def bundle(self, root):
        version, revision = "1.2.3-alpha.1", "a" * 40
        artifacts = []
        for target, ext, acceptance in (
                ("linux-amd64", "tar.gz", "packaged_binary_smoke_passed"),
                ("windows-amd64", "zip", "cross_compile_only; native runtime acceptance pending")):
            name = f"anchor_{version}_{target.replace('-', '_')}.{ext}"
            (root / name).write_bytes(b"fake archive for offline gate test")
            artifacts.append({"file": name, "target": target, "bytes": (root / name).stat().st_size,
                              "sha256": hashlib.sha256((root / name).read_bytes()).hexdigest(),
                              "verification": acceptance})
        standalone = []
        for target in ("linux-amd64", "windows-amd64"):
            name = ci_release.standalone_name(version, target)
            (root / name).write_bytes(b"fake executable for offline gate test")
            standalone.append({"file": name, "target": target, "bytes": (root / name).stat().st_size,
                               "sha256": hashlib.sha256((root / name).read_bytes()).hexdigest(),
                               "verification": "standalone_binary_smoke_passed" if target == "linux-amd64"
                               else "cross_compile_only; native runtime acceptance pending"})
        data = {"schema_version": 1, "version": version, "channel": "alpha", "status": "packaged",
                "source": {"revision": revision, "dirty": False, "inputs_sha256": "b" * 64},
                "public_publish_ready": False,
                "publish_blockers": ["windows-amd64: native runtime acceptance pending"],
                "checks": ["module_verify", "go_race_tests", "go_vet", "release_tool_tests"],
                "artifacts": artifacts, "standalone_artifacts": standalone}
        (root / "RELEASE_NOTES.md").write_text("Windows native acceptance pending\n")
        name = f"anchor_{version}_source.tar.gz"
        for file in (name, "LICENSE", "THIRD_PARTY_NOTICES.txt"):
            (root / file).write_bytes(b"offline source/notice fixture")
        data["corresponding_source"] = {
            "file": name, "revision": revision, "bytes": (root / name).stat().st_size,
            "sha256": hashlib.sha256((root / name).read_bytes()).hexdigest(),
            "dependencies": "vendored", "license": "AGPL-3.0-only"}
        self.write_manifest(root, data)
        return version, revision, data

    def write_manifest(self, root, data):
        (root / "release.json").write_text(json.dumps(data))
        files = sorted(p for p in root.iterdir() if p.name != "SHA256SUMS")
        (root / "SHA256SUMS").write_text("".join(
            f"{hashlib.sha256(p.read_bytes()).hexdigest()}  {p.name}\n" for p in files))

    def test_clean_pending_bundle_is_allowed_only_for_native_validation(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            version, revision, _ = self.bundle(root)
            ci_release.verify(root, version, revision, allow_pending=True)
            with self.assertRaises(ValueError):
                ci_release.verify(root, version, revision)

    def test_internal_dirty_partial_or_wrong_source_bundle_is_refused(self):
        edits = [lambda d: d.update(status="partial"),
                 lambda d: d.update(version="1.2.3-alpha.2"),
                 lambda d: d.update(channel="beta"),
                 lambda d: d["source"].update(dirty=True),
                 lambda d: d["source"].update(revision="c" * 40),
                 lambda d: d.update(publish_blockers=["project distribution license has not been selected"]),
                 lambda d: d.update(publish_blockers=[]),
                 lambda d: d.update(checks=[]),
                 lambda d: d["artifacts"][0].update(verification="not tested"),
                 lambda d: d["artifacts"][0].update(bytes=0),
                 lambda d: d["artifacts"][0].update(sha256="0" * 64),
                 lambda d: d.update(artifacts=[d["artifacts"][0], d["artifacts"][0]]),
                 lambda d: d.update(standalone_artifacts=[]),
                 lambda d: d.update(standalone_artifacts=[d["standalone_artifacts"][0]] * 2),
                 lambda d: d["standalone_artifacts"][0].update(file="other"),
                 lambda d: d["standalone_artifacts"][0].update(verification="not tested"),
                 lambda d: d["standalone_artifacts"][0].update(bytes=0),
                 lambda d: d["standalone_artifacts"][0].update(sha256="0" * 64)]
        for edit in edits:
            with self.subTest(edit=edit), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                version, revision, data = self.bundle(root)
                edit(data)
                self.write_manifest(root, data)
                with self.assertRaises(ValueError):
                    ci_release.verify(root, version, revision, allow_pending=True)

    def test_corruption_missing_and_extra_files_are_refused(self):
        for mutation in (lambda p: (p / "RELEASE_NOTES.md").write_text("corrupt"),
                         lambda p: (p / "SHA256SUMS").unlink(),
                         lambda p: (p / "anchor_1.2.3-alpha.1_linux_amd64").unlink(),
                         lambda p: (p / "anchor_1.2.3-alpha.1_windows_amd64.exe").write_bytes(b"corrupt"),
                         lambda p: (p / "unexpected").write_text("extra"),
                         lambda p: (p / "SHA256SUMS").write_text("0" * 64 + "  ../secret\n"),
                         lambda p: (p / "SHA256SUMS").write_text((p / "SHA256SUMS").read_text() * 2)):
            with tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                version, revision, _ = self.bundle(root)
                mutation(root)
                with self.assertRaises((ValueError, FileNotFoundError)):
                    ci_release.verify(root, version, revision, allow_pending=True)

    def linux_bundle(self, root):
        version, revision, data = self.bundle(root)
        for name in (f"anchor_{version}_windows_amd64.zip", ci_release.standalone_name(version, "windows-amd64")):
            (root / name).unlink()
        for field in ("artifacts", "standalone_artifacts"):
            data[field] = [a for a in data[field] if a["target"] == "linux-amd64"]
        data.update(public_publish_ready=True, publish_blockers=[])
        name = f"anchor_{version}_source.tar.gz"
        for file in (name, "LICENSE", "THIRD_PARTY_NOTICES.txt"):
            (root / file).write_bytes(b"offline source/notice fixture")
        data["corresponding_source"] = {
            "file": name, "revision": revision, "bytes": (root / name).stat().st_size,
            "sha256": hashlib.sha256((root / name).read_bytes()).hexdigest(),
            "dependencies": "vendored", "license": "AGPL-3.0-only"}
        self.write_manifest(root, data)
        return version, revision, data

    def test_public_linux_requires_ready_clean_source_and_correct_corresponding_source(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            version, revision, _ = self.linux_bundle(root)
            ci_release.verify(root, version, revision, "linux-amd64")
            with self.assertRaises(ValueError):
                ci_release.verify(root, version, revision, allow_pending=True)  # Cannot silently switch targets.
        edits = [lambda d: d.update(public_publish_ready=False),
                 lambda d: d.update(publish_blockers=["internal evaluation package"]),
                 lambda d: d.update(corresponding_source={}),
                 lambda d: d["corresponding_source"].update(revision="c" * 40),
                 lambda d: d["corresponding_source"].update(dependencies="missing"),
                 lambda d: d["corresponding_source"].update(license="MIT"),
                 lambda d: d["corresponding_source"].update(sha256="0" * 64),
                 lambda d: d["corresponding_source"].update(bytes=0),
                 lambda d: d["source"].update(dirty=True),
                 lambda d: d.update(artifacts=[])]
        for edit in edits:
            with self.subTest(edit=edit), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                version, revision, data = self.linux_bundle(root)
                edit(data)
                self.write_manifest(root, data)
                with self.assertRaises(ValueError):
                    ci_release.verify(root, version, revision, "linux-amd64")

    def native_evidence(self, root, version, revision):
        return {"schema_version": 1, "version": version, "revision": revision,
                "platform": "win32", "machine": "AMD64", "run_id": "123", "run_attempt": "1",
                "input_manifest_sha256": ci_release.digest(root / "release.json"),
                "standalone": {"sha256": ci_release.digest(root / ci_release.standalone_name(version, "windows-amd64")),
                               "checks": {k: "passed" for k in ci_release.NATIVE_CHECKS}},
                "archive": {"sha256": ci_release.digest(root / f"anchor_{version}_windows_amd64.zip"),
                            "checks": {k: "passed" for k in ci_release.NATIVE_CHECKS}}}

    def test_finalize_requires_same_run_native_evidence_and_retains_source(self):
        with tempfile.TemporaryDirectory() as temp:
            base = Path(temp)
            root = base / "bundle"
            root.mkdir()
            version, revision, _ = self.bundle(root)
            evidence = base / "native.json"
            evidence.write_text(json.dumps(self.native_evidence(root, version, revision)))
            ci_release.finalize(root, version, revision, evidence, "123", "1")
            ci_release.verify(root, version, revision)
            data = json.loads((root / "release.json").read_text())
            self.assertTrue(data["public_publish_ready"])
            self.assertEqual(data["publish_blockers"], [])
            self.assertEqual(data["corresponding_source"]["license"], "AGPL-3.0-only")
            data["windows_native"]["standalone"]["checks"].pop("windows_acl")
            self.write_manifest(root, data)
            with self.assertRaises(ValueError):
                ci_release.verify(root, version, revision)

    def test_wrong_native_platform_hash_revision_run_or_missing_check_is_rejected(self):
        edits = [lambda e: e.update(platform="linux"), lambda e: e.update(machine="arm64"),
                 lambda e: e.update(revision="c" * 40), lambda e: e.update(version="1.2.3-alpha.9"),
                 lambda e: e.update(run_id="999"), lambda e: e.update(run_attempt="2"),
                 lambda e: e.update(input_manifest_sha256="0" * 64),
                 lambda e: e["standalone"].update(sha256="0" * 64),
                 lambda e: e["archive"].update(sha256="0" * 64),
                 lambda e: e["archive"]["checks"].pop("windows_acl"),
                 lambda e: e["standalone"]["checks"].update(graceful_shutdown="failed")]
        for edit in edits:
            with self.subTest(edit=edit), tempfile.TemporaryDirectory() as temp:
                base = Path(temp)
                root = base / "bundle"
                root.mkdir()
                version, revision, _ = self.bundle(root)
                evidence = self.native_evidence(root, version, revision)
                edit(evidence)
                path = base / "native.json"
                path.write_text(json.dumps(evidence))
                original = (root / "release.json").read_bytes()
                with self.assertRaises(ValueError):
                    ci_release.finalize(root, version, revision, path, "123", "1")
                self.assertEqual(original, (root / "release.json").read_bytes())

    def test_symlink_bundle_refused(self):
        with tempfile.TemporaryDirectory() as temp:
            base = Path(temp)
            root = base / "real"
            root.mkdir()
            version, revision, _ = self.bundle(root)
            link = base / "link"
            link.symlink_to(root, target_is_directory=True)
            with self.assertRaises(ValueError):
                ci_release.verify(link, version, revision)


if __name__ == "__main__":
    unittest.main()

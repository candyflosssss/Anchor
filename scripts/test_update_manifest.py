import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from update_manifest import build_manifest, validate_version, validate_base

class UpdateManifestTests(unittest.TestCase):
    def test_desktop_and_cli_feeds(self):
        with tempfile.TemporaryDirectory() as root:
            root = Path(root)
            files = {"windows-amd64": root / "Anchor-setup.exe", "linux-amd64": root / "anchor.deb"}
            for file in files.values():
                file.write_bytes(b"synthetic artifact")
            result = build_manifest("desktop", "0.1.0-dev.desktop.10", "dev", "https://example.com/builds/10", files, "target")
            self.assertEqual(result["product"], "desktop")
            self.assertEqual([a["kind"] for a in result["artifacts"]], ["deb", "nsis"])
            self.assertEqual(result["artifacts"][0]["url"], "https://example.com/builds/10/linux-amd64/anchor.deb")
            cli = build_manifest("cli", "0.1.0-alpha.7", "alpha", "https://example.com", {"linux-amd64": files["linux-amd64"]})
            self.assertEqual(cli["artifacts"][0]["kind"], "binary")
            output = root / "feed.json"
            args = [sys.executable, str(Path(__file__).with_name("update_manifest.py")), "--product", "desktop", "--version", "0.1.0-dev.desktop.10", "--channel", "dev", "--base-url", "https://example.com", "--artifact", f"linux-amd64={files['linux-amd64']}", "--output", str(output)]
            subprocess.run(args, check=True, capture_output=True)
            self.assertEqual(json.loads(output.read_text())["channel"], "dev")
            self.assertNotEqual(subprocess.run(args, capture_output=True).returncode, 0)

    def test_desktop_runtime_compatibility(self):
        with tempfile.TemporaryDirectory() as root:
            root = Path(root)
            installer = root / "anchor.deb"
            runtime = root / "anchor-desktop"
            installer.write_bytes(b"installer")
            runtime.write_bytes(b"runtime executable")
            feed = build_manifest("desktop", "1.0.0-dev.1", "dev", "https://example.com", {"linux-amd64": installer}, runtimes={"linux-amd64": runtime})
            artifact = feed["artifacts"][0]
            self.assertEqual(artifact["kind"], "deb")  # Old clients keep their installer.
            self.assertEqual(artifact["runtime"]["kind"], "runtime")
            self.assertEqual(artifact["runtime"]["url"], "https://example.com/anchor-desktop")
            with self.assertRaises(ValueError):
                build_manifest("desktop", "1.0.0-dev.1", "dev", "https://example.com", {"linux-amd64": installer}, runtimes={"windows-amd64": runtime})

    def test_invalid_version_and_url(self):
        for version, channel in [("0.1.0-dev..1", "dev"), ("01.0.0", "stable"), ("0.1.0-beta.1", "alpha"), ("0.1.0-dev.01", "dev")]:
            with self.assertRaises(ValueError): validate_version(version, channel)
        for url in ["http://example.com", "http://localhost", "https://u:p@example.com", "https://example.com?token=x"]:
            with self.assertRaises(ValueError): validate_base(url)
        validate_base("http://127.0.0.1:8787")

    def test_invalid_artifacts(self):
        with tempfile.TemporaryDirectory() as root:
            root = Path(root)
            file = root / "empty.deb"
            file.write_bytes(b"")
            with self.assertRaises(ValueError): build_manifest("desktop", "1.0.0", "stable", "https://example.com", {"linux-amd64": file})
            file.write_bytes(b"x")
            with self.assertRaises(ValueError): build_manifest("desktop", "1.0.0", "stable", "https://example.com", {"windows-amd64": file})
            link = root / "linked.deb"
            link.symlink_to(file)
            with self.assertRaises(ValueError): build_manifest("desktop", "1.0.0", "stable", "https://example.com", {"linux-amd64": link})

if __name__ == "__main__": unittest.main()

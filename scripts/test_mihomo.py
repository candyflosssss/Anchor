"""No network or executable invocation: release pins and cache integrity checks."""
import hashlib
import importlib.util
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("prepare_mihomo", Path(__file__).with_name("prepare-mihomo.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

class MihomoTests(unittest.TestCase):
    def test_archive_pins_match_runtime(self):
        for target, (_, checksum, relative) in module.ASSETS.items():
            source = module.ROOT / "internal/egress" / ("bundle_" + target.split("-")[0] + ".go")
            self.assertIn(checksum, source.read_text())
            self.assertIn("payload/" + relative, source.read_text())
        self.assertEqual((module.ROOT / "third_party/mihomo/LICENSE").read_bytes(), (module.ROOT / "internal/egress/payload/LICENSE").read_bytes())
        self.assertEqual((module.ROOT / "third_party/mihomo/NOTICE.md").read_bytes(), (module.ROOT / "internal/egress/payload/NOTICE.md").read_bytes())

    def test_corrupt_cached_archive_fails_without_network(self):
        with tempfile.TemporaryDirectory() as temp, patch.object(module, "ROOT", Path(temp)), patch.object(module.urllib.request, "build_opener", side_effect=AssertionError("network")):
            path = Path(temp) / "internal/egress/payload/linux/mihomo.gz"
            path.parent.mkdir(parents=True)
            path.write_bytes(b"corrupted")
            with self.assertRaisesRegex(RuntimeError, "checksum mismatch"):
                module.prepare("linux-amd64")

    def test_verified_cache_does_not_download(self):
        content = b"synthetic archive"
        assets = {"linux-amd64": ("synthetic.gz", hashlib.sha256(content).hexdigest(), "linux/mihomo.gz")}
        with tempfile.TemporaryDirectory() as temp, patch.object(module, "ROOT", Path(temp)), patch.object(module, "ASSETS", assets), patch.object(module.urllib.request, "build_opener", side_effect=AssertionError("network")):
            path = Path(temp) / "internal/egress/payload/linux/mihomo.gz"
            path.parent.mkdir(parents=True)
            path.write_bytes(content)
            self.assertEqual(module.prepare("linux-amd64"), path)

if __name__ == "__main__":
    unittest.main()

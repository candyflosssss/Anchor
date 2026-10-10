import importlib.util
import io
import json
from pathlib import Path
import tempfile
import unittest
from contextlib import redirect_stderr
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("anchor_desktop_build", Path(__file__).with_name("build-desktop.py"))
build = importlib.util.module_from_spec(spec)
spec.loader.exec_module(build)


class DesktopBuildTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.frontend = self.root / "desktop/frontend"
        (self.frontend / "node_modules").mkdir(parents=True)
        (self.frontend / "package.json").write_text(json.dumps({"engines": {"node": "22.22.1", "npm": "9.2.0"}}))
        for key, value in (("ROOT", self.root), ("FRONTEND", self.frontend)):
            patcher = patch.object(build, key, value)
            patcher.start()
            self.addCleanup(patcher.stop)

    def test_windows_preflight_does_not_require_linux_libraries(self):
        with patch.object(build.shutil, "which", return_value="/synthetic/tool"), \
             patch.object(build.subprocess, "check_output", side_effect=["v22.22.1\n", "9.2.0\n"]), \
             patch.object(build.subprocess, "run", side_effect=AssertionError("no native check for Windows")):
            build.preflight("windows-amd64")

    def test_missing_tools_fail_closed(self):
        with patch.object(build.shutil, "which", return_value=None), self.assertRaisesRegex(RuntimeError, "missing: go"):
            build.preflight("windows-amd64")

    def test_unlocked_toolchain_is_rejected(self):
        with patch.object(build.shutil, "which", return_value="tool"), \
             patch.object(build.subprocess, "check_output", return_value="v99.0.0"), \
             self.assertRaisesRegex(RuntimeError, "node 22.22.1 required"):
            build.preflight("windows-amd64")

    def test_linux_requires_native_build_host(self):
        with patch.object(build.shutil, "which", return_value="tool"), \
             patch.object(build.subprocess, "check_output", side_effect=["v22.22.1", "9.2.0"]), \
             patch.object(build.platform, "system", return_value="Windows"), \
             self.assertRaisesRegex(RuntimeError, "native Linux AMD64"):
            build.preflight("linux-amd64")

    def test_linux_missing_libraries_does_not_install_anything(self):
        with patch.object(build.shutil, "which", return_value="tool"), \
             patch.object(build.subprocess, "check_output", side_effect=["v22.22.1", "9.2.0"]), \
             patch.object(build.platform, "system", return_value="Linux"), \
             patch.object(build.platform, "machine", return_value="x86_64"), \
             patch.object(build.subprocess, "run") as run:
            run.return_value.returncode = 1
            with self.assertRaisesRegex(RuntimeError, "No system packages were installed"):
                build.preflight("linux-amd64")
            run.assert_called_once_with(["pkg-config", "--exists", "gtk+-3.0", "webkit2gtk-4.1"], check=False)

    def test_assets_require_desktop_contract_and_exclude_mock(self):
        assets = self.frontend / "dist/assets"
        assets.mkdir(parents=True)
        with self.assertRaisesRegex(RuntimeError, "missing"):
            build.verify_assets()
        bundle = assets / "test.js"
        bundle.write_text("no contract")
        with self.assertRaisesRegex(RuntimeError, "contract marker"):
            build.verify_assets()
        for marker in ("https://backup.example.com/v1", "本地调试", "原型场景"):
            bundle.write_text("anchor-desktop-v1 " + marker)
            with self.subTest(marker=marker), self.assertRaisesRegex(RuntimeError, "Prototype-only"):
                build.verify_assets()
        bundle.write_text("anchor-desktop-v1")
        build.verify_assets()

    def test_public_version_is_rejected_before_preflight(self):
        with patch("sys.argv", ["build", "--target", "windows-amd64", "--version", "0.1.0-alpha.6"]), \
             patch.object(build, "preflight", side_effect=AssertionError("no public build")), \
             redirect_stderr(io.StringIO()), self.assertRaises(SystemExit):
            build.main()

    def test_existing_artifact_is_never_overwritten(self):
        folder = self.root / "dist/desktop/0.1.0-dev.desktop.1/windows-amd64"
        folder.mkdir(parents=True)
        binary = folder / "anchor-desktop.exe"
        binary.write_bytes(b"previous build")
        with patch("sys.argv", ["build", "--target", "windows-amd64"]), \
             patch.dict(build.os.environ, {"ANCHOR_DESKTOP_GUARDED": "1"}), \
             patch.object(build, "preflight"), \
             patch.object(build, "run", side_effect=AssertionError("must not build")), \
             self.assertRaisesRegex(RuntimeError, "Refusing to overwrite"):
            build.main()
        self.assertEqual(binary.read_bytes(), b"previous build")


if __name__ == "__main__":
    unittest.main()

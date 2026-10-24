"""Source-contract regressions; NSIS compilation/native install are separate checks."""
from pathlib import Path
import re
import unittest


ROOT = Path(__file__).resolve().parents[1]


class WindowsInstallerContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.source = (ROOT / "desktop/packaging/windows/installer.nsi").read_text()

    def function(self, name):
        return self.source.split(f"Function {name}\n", 1)[1].split("FunctionEnd", 1)[0]

    def main_section(self):
        return self.source.split('Section "Anchor（必需）" main\n', 1)[1].split("SectionEnd", 1)[0]

    def test_directory_page_is_before_install_and_has_validation(self):
        pages = re.findall(r"!insertmacro (MUI_PAGE_\w+)", self.source)
        self.assertLess(pages.index("MUI_PAGE_COMPONENTS"), pages.index("MUI_PAGE_DIRECTORY"))
        self.assertLess(pages.index("MUI_PAGE_DIRECTORY"), pages.index("MUI_PAGE_INSTFILES"))
        self.assertIn("!define MUI_PAGE_CUSTOMFUNCTION_LEAVE ValidateInstallDirectory", self.source)

    def test_default_and_previous_directory_do_not_override_nsis_command_line(self):
        self.assertIn('InstallDir "$LOCALAPPDATA\\Programs\\Anchor Desktop"', self.source)
        self.assertIn('InstallDirRegKey HKCU "${UNINSTALL_KEY}" "InstallLocation"', self.source)
        self.assertNotIn("StrCpy $INSTDIR", self.function(".onInit"))
        self.assertIn('WriteRegStr HKCU "${UNINSTALL_KEY}" "InstallLocation" "$INSTDIR"', self.source)

    def test_invalid_directories_are_rejected_also_in_silent_mode(self):
        validation = self.function("ValidateInstallDirectory")
        self.assertIn('StrCpy $0 "$INSTDIR" 2 1', validation)
        # Neither a new destination nor a stale remembered path may require
        # existing parents just to normalize its spelling.
        self.assertNotRegex(validation, r"(?m)^\s*GetFullPathName\s")
        for directory in ("$INSTDIR", "$PreviousInstallDir"):
            self.assertIn(f'kernel32::GetFullPathNameW(w "{directory}",', validation)
            self.assertIn(f'StrCpy {directory} "$0"', validation)
        self.assertEqual(validation.count("${If} $1 == 0"), 2)
        self.assertEqual(validation.count("${OrIf} $1 >= ${NSIS_MAX_STRLEN}"), 2)
        self.assertIn("${If} $0 <= 3", validation)
        self.assertIn("SetErrorLevel 2\n  Abort", validation)
        self.assertIn("Call ValidateInstallDirectory", self.main_section())

    def test_both_old_and_selected_programs_must_be_stopped_before_writes(self):
        section = self.main_section()
        writes = section.index('SetOutPath "$INSTDIR"')
        for call in ("ValidateInstallDirectory", "CheckPreviousStopped", "CheckStopped", "EnsureWebView2", "RelocateInstalledApp"):
            self.assertLess(section.index("Call " + call), writes)
        previous = self.function("CheckPreviousStopped")
        self.assertIn("Push $INSTDIR", previous)
        self.assertIn('StrCpy $INSTDIR "$PreviousInstallDir"', previous)
        self.assertIn("Call CheckStopped\n  Pop $INSTDIR", previous)

    def test_relocation_requires_consent_and_writability_before_old_uninstall(self):
        relocation = self.function("RelocateInstalledApp")
        self.assertIn('${OrIf} $PreviousInstallDir == $INSTDIR\n    Return', relocation)
        self.assertIn('/SD IDNO IDYES relocate\n  SetErrorLevel 2\n  Abort', relocation)
        uninstall = relocation.index("ExecWait")
        for step in ('IfFileExists "$PreviousInstallDir\\Uninstall.exe"', "MessageBox MB_YESNO", 'GetTempFileName $0 "$INSTDIR"', 'Delete "$0"', "Call CheckPreviousStopped", "Call CheckStopped"):
            self.assertLess(relocation.index(step), uninstall)
        self.assertIn(' /S _?=$PreviousInstallDir\' $0', relocation)

    def test_failed_or_incomplete_uninstall_aborts_relocation(self):
        relocation = self.function("RelocateInstalledApp")
        after_uninstall = relocation.split("ExecWait", 1)[1]
        self.assertIn("IfErrors relocation_failed", after_uninstall)
        self.assertIn("${If} $0 != 0\n    Goto relocation_failed", after_uninstall)
        self.assertIn('ReadRegStr $0 HKCU "${UNINSTALL_KEY}" "InstallLocation"', after_uninstall)
        self.assertIn('${If} $0 != ""\n    Goto relocation_failed', after_uninstall)
        self.assertTrue(relocation.rstrip().endswith("SetErrorLevel 2\n  Abort"))

    def test_removal_is_non_recursive_and_does_not_touch_user_state(self):
        self.assertNotRegex(self.source, r"(?im)^\s*(?:RMDir|Delete)\s+/[rR](?:\s|$)")
        deletes = re.findall(r'^\s*(?:Delete|RMDir) (.+)$', self.source, re.MULTILINE)
        for command in deletes:
            self.assertNotIn("$APPDATA", command)
            self.assertNotIn("$LOCALAPPDATA", command)
            self.assertNotIn("state.db", command)
        self.assertIn("ReadRegStr $1 HKCU", self.function("un.onInit"))
        self.assertIn("${If} $1 != $INSTDIR", self.function("un.onInit"))

    def test_bad_destination_cannot_write_to_the_previous_output_directory(self):
        section = self.main_section()
        self.assertIn('ClearErrors\n  SetOutPath "$INSTDIR"\n  IfErrors install_failed', section)
        after_output = section.split('SetOutPath "$INSTDIR"', 1)[1].split('File /oname=anchor-desktop.exe', 1)[0]
        self.assertNotIn("ClearErrors", after_output)

    def test_shortcuts_and_uninstaller_follow_selected_directory(self):
        self.assertIn('WriteUninstaller "$INSTDIR\\Uninstall.exe"', self.source)
        self.assertIn('CreateShortcut "$SMPROGRAMS\\Anchor\\Anchor.lnk" "$INSTDIR\\anchor-desktop.exe"', self.source)
        self.assertIn('CreateShortcut "$DESKTOP\\Anchor.lnk" "$INSTDIR\\anchor-desktop.exe"', self.source)
        self.assertIn('"UninstallString" \'"$INSTDIR\\Uninstall.exe"\'', self.source)
        self.assertIn("RequestExecutionLevel user", self.source)


if __name__ == "__main__":
    unittest.main()

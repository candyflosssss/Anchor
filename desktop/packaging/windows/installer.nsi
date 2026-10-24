; Current-user development installer. No service, Run key, PATH or user-state changes.
Unicode true
RequestExecutionLevel user
ManifestDPIAware true
SetCompressor /SOLID lzma
SetCompressorDictSize 16
!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "WinVer.nsh"
!include "x64.nsh"

!define APP_NAME "Anchor Desktop"
!define UNINSTALL_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\AnchorDesktop"
!define WEBVIEW_KEY "Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}"
Name "${APP_NAME} ${APP_VERSION}"
OutFile "${OUTPUT_FILE}"
InstallDir "$LOCALAPPDATA\Programs\Anchor Desktop"
; HKCU Software is shared between registry views. NSIS gives /D= precedence.
InstallDirRegKey HKCU "${UNINSTALL_KEY}" "InstallLocation"
Var PreviousInstallDir
VIProductVersion "${NUMERIC_VERSION}"
VIAddVersionKey /LANG=2052 "ProductName" "${APP_NAME}"
VIAddVersionKey /LANG=2052 "FileDescription" "Anchor 当前用户开发版安装程序"
VIAddVersionKey /LANG=2052 "FileVersion" "${APP_VERSION}"
VIAddVersionKey /LANG=2052 "ProductVersion" "${APP_VERSION}"
VIAddVersionKey /LANG=2052 "LegalCopyright" "Anchor contributors; AGPL-3.0-only"
!define MUI_ICON "${ICON_FILE}"
!define MUI_UNICON "${ICON_FILE}"
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_RUN "$INSTDIR\anchor-desktop.exe"
!define MUI_FINISHPAGE_RUN_TEXT "打开 Anchor（将自动启动或连接后台核心）"
!define MUI_FINISHPAGE_RUN_NOTCHECKED
!define MUI_FINISHPAGE_TEXT "开发版安装完成。打开应用会自动启动或连接本机核心；关闭窗口不会停服。开机自启动未启用。更新或卸载前，请在界面明确停止核心，等待退出后关闭所有窗口。此版本尚未完成原生安装和运行验收。"
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "${DOCS_DIR}/LICENSE"
!insertmacro MUI_PAGE_COMPONENTS
!define MUI_DIRECTORYPAGE_TEXT_TOP "选择当前用户有写入权限的本地文件夹，可更换盘符，支持中文和空格。升级默认沿用原目录；更换位置会先确认卸载旧程序，保留配置和数据库。"
!define MUI_PAGE_CUSTOMFUNCTION_LEAVE ValidateInstallDirectory
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "SimpChinese"

; Requesting an exclusive write-capable handle detects a running window OR
; detached --core from this install. Never write through the handle or kill it.
!macro STOPPED_CHECK PREFIX
Function ${PREFIX}CheckStopped
  retry:
  IfFileExists "$INSTDIR\anchor-desktop.exe" 0 done
  System::Call 'kernel32::CreateFileW(w "$INSTDIR\anchor-desktop.exe", i 0xC0000000, i 0, p 0, i 3, i 0x80, p 0) p .r0'
  ${If} $0 == -1
    MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "Anchor 仍在运行（界面或后台核心），暂时无法覆盖安装文件。$\r$\n$\r$\n请先在主窗口点击「停止核心」，再在系统托盘菜单选择「退出界面」；也可在终端执行 anchor stop --approve。$\r$\n$\r$\n完成后点击「重试」继续。安装器不会强制停止进程。" /SD IDCANCEL IDRETRY retry
    SetErrorLevel 2
    Abort
  ${EndIf}
  System::Call 'kernel32::CloseHandle(p r0)'
  done:
FunctionEnd
!macroend
!insertmacro STOPPED_CHECK ""
!insertmacro STOPPED_CHECK "un."

Function .onInit
  SetShellVarContext current
  SetRegView 64
  ${IfNot} ${RunningX64}
    MessageBox MB_OK|MB_ICONSTOP "此安装包需要 64 位 Windows。" /SD IDOK
    SetErrorLevel 2
    Abort
  ${EndIf}
  ${IfNot} ${AtLeastWin10}
    MessageBox MB_OK|MB_ICONSTOP "此开发版的目标平台为 Windows 10 或更新版本。" /SD IDOK
    SetErrorLevel 2
    Abort
  ${EndIf}
  ; Keep NSIS /D= or the remembered InstallLocation; never force the default.
  ReadRegStr $PreviousInstallDir HKCU "${UNINSTALL_KEY}" "InstallLocation"
  Call CheckPreviousStopped
  Call CheckStopped
FunctionEnd

Function ValidateInstallDirectory
  ; Run on directory-page leave AND in the install section (including /S).
  ; Reject relative/UNC paths and drive roots before creating or removing files.
  StrCpy $0 "$INSTDIR" 2 1
  ${If} $0 != ":\"
  ${AndIf} $0 != ":/"
    Goto invalid_directory
  ${EndIf}
  ; NSIS GetFullPathName requires existing parent directories. Use the Win32
  ; lexical API so first installs and new nested destinations can be created
  ; later by SetOutPath (or the relocation writability check).
  System::Call 'kernel32::GetFullPathNameW(w "$INSTDIR", i ${NSIS_MAX_STRLEN}, w .r0, p 0) i .r1'
  ${If} $1 == 0
  ${OrIf} $1 >= ${NSIS_MAX_STRLEN}
    Goto invalid_directory
  ${EndIf}
  StrCpy $INSTDIR "$0"
  StrLen $0 "$INSTDIR"
  ${If} $0 <= 3
    Goto invalid_directory
  ${EndIf}
  ; Strip a trailing separator so choosing the same folder is not relocation.
  StrCpy $0 "$INSTDIR" 1 -1
  ${If} $0 == "\"
  ${OrIf} $0 == "/"
    StrCpy $INSTDIR "$INSTDIR" -1
  ${EndIf}
  ; Re-read to avoid relying on a stale registry value after directory selection.
  ReadRegStr $PreviousInstallDir HKCU "${UNINSTALL_KEY}" "InstallLocation"
  ${If} $PreviousInstallDir != ""
    System::Call 'kernel32::GetFullPathNameW(w "$PreviousInstallDir", i ${NSIS_MAX_STRLEN}, w .r0, p 0) i .r1'
    ${If} $1 == 0
    ${OrIf} $1 >= ${NSIS_MAX_STRLEN}
      Goto invalid_directory
    ${EndIf}
    StrCpy $PreviousInstallDir "$0"
    StrCpy $0 "$PreviousInstallDir" 1 -1
    ${If} $0 == "\"
    ${OrIf} $0 == "/"
      StrCpy $PreviousInstallDir "$PreviousInstallDir" -1
    ${EndIf}
  ${EndIf}
  SetErrorLevel 0
  Return
  invalid_directory:
  MessageBox MB_OK|MB_ICONSTOP "请选择本地盘符下的完整文件夹，例如 D:\Anchor 或 E:\应用程序\Anchor Desktop。不能使用盘符根目录、相对路径或网络共享；所选文件夹须允许当前用户写入。" /SD IDOK
  SetErrorLevel 2
  Abort
FunctionEnd

Function CheckPreviousStopped
  ${If} $PreviousInstallDir == ""
  ${OrIf} $PreviousInstallDir == $INSTDIR
    Return
  ${EndIf}
  ; Check the old core/window as well as the selected destination. No killing.
  Push $INSTDIR
  StrCpy $INSTDIR "$PreviousInstallDir"
  Call CheckStopped
  Pop $INSTDIR
FunctionEnd

Function RelocateInstalledApp
  ${If} $PreviousInstallDir == ""
  ${OrIf} $PreviousInstallDir == $INSTDIR
    Return
  ${EndIf}
  IfFileExists "$PreviousInstallDir\Uninstall.exe" 0 relocation_failed
  ; Silent installs cannot approve a location change implicitly.
  MessageBox MB_YESNO|MB_ICONQUESTION|MB_DEFBUTTON2 "将安装位置从：$\r$\n$PreviousInstallDir$\r$\n更改为：$\r$\n$INSTDIR$\r$\n$\r$\n确认后先卸载旧程序，再安装到新位置。配置、凭据和数据库会保留在原用户数据目录，不随程序移动。如果新安装失败，请重新运行安装包。是否继续？" /SD IDNO IDYES relocate
  SetErrorLevel 2
  Abort
  relocate:
  ; Prove the destination is writable BEFORE uninstalling the working copy.
  ClearErrors
  CreateDirectory "$INSTDIR"
  IfErrors relocation_failed
  GetTempFileName $0 "$INSTDIR"
  IfErrors relocation_failed
  Delete "$0"
  IfErrors relocation_failed
  Call CheckPreviousStopped
  Call CheckStopped
  ClearErrors
  ; _?= must be last and unquoted, even with spaces: wait for the original
  ; uninstaller instead of a detached temporary copy. It retains all user state.
  ExecWait '"$PreviousInstallDir\Uninstall.exe" /S _?=$PreviousInstallDir' $0
  IfErrors relocation_failed
  ${If} $0 != 0
    Goto relocation_failed
  ${EndIf}
  ReadRegStr $0 HKCU "${UNINSTALL_KEY}" "InstallLocation"
  ${If} $0 != ""
    Goto relocation_failed
  ${EndIf}
  ; _?= leaves the uninstaller itself behind. Never recurse into the old folder.
  Delete "$PreviousInstallDir\Uninstall.exe"
  RMDir "$PreviousInstallDir"
  ClearErrors
  Return
  relocation_failed:
  MessageBox MB_OK|MB_ICONSTOP "无法更换安装位置：新目录可能不可写，或旧版卸载未完成。未强制结束任何进程，配置和数据库不会删除。请停止核心、退出界面，检查目录权限或通过「已安装的应用」卸载旧版后重试。" /SD IDOK
  SetErrorLevel 2
  Abort
FunctionEnd

Function HasWebView2
  ; On x64 Windows, the machine-wide registration is in the 32-bit view.
  SetRegView 32
  ReadRegStr $0 HKLM "${WEBVIEW_KEY}" "pv"
  ${If} $0 == ""
  ${OrIf} $0 == "0.0.0.0"
    ReadRegStr $0 HKCU "${WEBVIEW_KEY}" "pv"
  ${EndIf}
  SetRegView 64
  ${If} $0 == ""
  ${OrIf} $0 == "0.0.0.0"
    Push 0
  ${Else}
    Push 1
  ${EndIf}
FunctionEnd

Function EnsureWebView2
  Call HasWebView2
  Pop $0
  ${If} $0 == 1
    Return
  ${EndIf}
  ; Silent install must not imply permission to download a new runtime.
  IfSilent missing
  MessageBox MB_YESNO|MB_ICONQUESTION|MB_DEFBUTTON2 "Anchor 需要 Microsoft Edge WebView2 Runtime。是否运行 Microsoft 引导器联网安装？这需要网络，可能额外使用磁盘空间。选择否将取消 Anchor 安装。" IDYES install_runtime
  Goto missing
  install_runtime:
  InitPluginsDir
  SetOutPath "$PLUGINSDIR"
  File /oname=MicrosoftEdgeWebview2Setup.exe "${WEBVIEW_BOOTSTRAPPER}"
  ClearErrors
  ExecWait '"$PLUGINSDIR\MicrosoftEdgeWebview2Setup.exe" /silent /install' $1
  IfErrors missing
  ${If} $1 != 0
  ${AndIf} $1 != 3010
    Goto missing
  ${EndIf}
  Call HasWebView2
  Pop $0
  ${If} $0 == 1
    Return
  ${EndIf}
  missing:
  MessageBox MB_OK|MB_ICONSTOP "未检测到可用的 WebView2 Runtime。Anchor 安装已取消。请先安装 Microsoft WebView2 Runtime，必要时重启 Windows 后再试。" /SD IDOK
  SetErrorLevel 2
  Abort
FunctionEnd

Section "Anchor（必需）" main
  SectionIn RO
  Call ValidateInstallDirectory
  Call CheckPreviousStopped
  Call CheckStopped
  Call EnsureWebView2
  Call RelocateInstalledApp
  ClearErrors
  SetOutPath "$INSTDIR"
  IfErrors install_failed
  SetOverwrite on
  File /oname=anchor-desktop.exe "${APP_EXE}"
  IfErrors install_failed
  File /oname=anchor.ico "${ICON_FILE}"
  File "${DOCS_DIR}/LICENSE"
  File "${DOCS_DIR}/README.txt"
  File "${DOCS_DIR}/THIRD_PARTY_NOTICES.txt"
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  IfErrors install_failed
  CreateDirectory "$SMPROGRAMS\Anchor"
  CreateShortcut "$SMPROGRAMS\Anchor\Anchor.lnk" "$INSTDIR\anchor-desktop.exe" "" "$INSTDIR\anchor.ico"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayName" "${APP_NAME}"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayVersion" "${APP_VERSION}"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "Publisher" "Anchor contributors"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayIcon" "$INSTDIR\anchor.ico"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegStr HKCU "${UNINSTALL_KEY}" "QuietUninstallString" '"$INSTDIR\Uninstall.exe" /S'
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "NoRepair" 1
  IfErrors install_failed
  Goto install_done
  install_failed:
  MessageBox MB_OK|MB_ICONSTOP "安装文件写入失败。未强制关闭任何进程；请检查文件权限并在明确停止核心、退出所有窗口后重试。" /SD IDOK
  SetErrorLevel 2
  Abort
  install_done:
SectionEnd

Section /o "桌面快捷方式（可选）" desktop_shortcut
  CreateShortcut "$DESKTOP\Anchor.lnk" "$INSTDIR\anchor-desktop.exe" "" "$INSTDIR\anchor.ico"
SectionEnd

Function un.onInit
  SetShellVarContext current
  SetRegView 64
  ReadRegStr $1 HKCU "${UNINSTALL_KEY}" "InstallLocation"
  ${If} $1 != $INSTDIR
    MessageBox MB_OK|MB_ICONSTOP "卸载位置与当前用户注册记录不符；未删除文件。" /SD IDOK
    SetErrorLevel 2
    Abort
  ${EndIf}
  Call un.CheckStopped
FunctionEnd

Section "Uninstall"
  Call un.CheckStopped
  ClearErrors
  Delete "$INSTDIR\anchor-desktop.exe"
  IfErrors uninstall_failed
  Delete "$DESKTOP\Anchor.lnk"
  Delete "$SMPROGRAMS\Anchor\Anchor.lnk"
  RMDir "$SMPROGRAMS\Anchor"
  Delete "$INSTDIR\anchor.ico"
  Delete "$INSTDIR\LICENSE"
  Delete "$INSTDIR\README.txt"
  Delete "$INSTDIR\THIRD_PARTY_NOTICES.txt"
  Delete "$INSTDIR\Uninstall.exe"
  ; Never recurse: extra files and all per-user Anchor state are preserved.
  RMDir "$INSTDIR"
  DeleteRegKey HKCU "${UNINSTALL_KEY}"
  Goto uninstall_done
  uninstall_failed:
  MessageBox MB_OK|MB_ICONSTOP "程序文件仍在使用中或无法删除；卸载已停止，请明确停止核心并退出窗口后重试。" /SD IDOK
  SetErrorLevel 2
  Abort
  uninstall_done:
SectionEnd

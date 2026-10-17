Anchor Desktop — internal desktop-dev package (unsigned, not a public release)

Windows: install for the current user; open Anchor from the Start menu or the
optional desktop shortcut. Choose a writable local-drive folder on the directory
page (Unicode and spaces supported); the default is
%LOCALAPPDATA%\Programs\Anchor Desktop. Upgrades remember the registered install
folder. Selecting a different folder asks for confirmation, checks destination
write access, then waits for the old uninstaller before writing the new copy.
This preserves user state in its original location; it does not move the database.
Stop the core and close all windows first. If installation after removal fails,
rerun the installer; program-file rollback is not automatic.
NSIS /D=<absolute local folder> overrides the default/remembered location; put it
last and do not quote its value, even with spaces. Drive roots, relative paths and
UNC shares are rejected. /S cannot approve relocation (exit code 2); use the GUI
or explicitly uninstall first. No elevation is requested for protected folders.
If WebView2 is missing, the installer asks before running Microsoft's online
bootstrapper. Internet access is then required.

Linux: this .deb targets Ubuntu 24.04 LTS amd64 with a graphical session.
Install with the graphical package manager, or `sudo apt install ./<file>.deb`
to resolve the declared GTK3 / WebKitGTK 4.1 dependencies. Open Anchor from the
application menu. No frontend development server or manually started backend
is needed. A bare server without a graphical session should use the CLI.

Opening the application initializes missing per-user Anchor state safely,
starts the local core in the background or authenticates/reuses an existing
compatible core. Closing the window does NOT stop the core. Explicit stopping
requires confirmation in the GUI (or a compatible CLI: anchor stop --approve).
Existing approved Agent management policies are retained; installation itself
does not create or change them. No login autostart or system service is added.

UPGRADE / REMOVE: first explicitly stop the core, wait for it to exit, then
close every Anchor window. Install the new package. The installer never kills
processes or deletes per-user configuration, credentials, database or recovery
materials. Linux may retain the old running process if you skip stopping it;
installing a new file does not update that running process. Removing the package
alone is not a stop operation. Linux uses the package manager for uninstall;
Windows registers a per-user uninstaller in Installed apps.

This build only establishes that the sources compiled and packages were
created. Installation, real Windows/Linux windows, runtime dependencies,
supplier operations and background lifecycle have NOT been accepted for this
version. Do not treat this development package as production-ready.

License: AGPL-3.0-only, see LICENSE. Dependencies retain their respective
licenses; see THIRD_PARTY_NOTICES.txt. This local package has no public source
release, signing or publication approval. Use docs/release/desktop.md in the
source repository for build and release requirements.

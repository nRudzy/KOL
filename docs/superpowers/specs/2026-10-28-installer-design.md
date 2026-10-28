# KOL — installation Windows

The user wants a real installation rather than running a portable executable, reports an “unknown publisher / run this file” warning at login, and authorizes building an installer as the continuation of the Windows application. Current evidence: the live process runs from `\\wsl.localhost\Ubuntu\home\gabriel\Projects\Kol\dist\KOL.exe`; an earlier local copy exists but has no Windows uninstall registration.

Builder choice: a French Inno Setup installer, per-user and without elevation. Compared with MSIX it needs no signing certificate or developer-mode configuration; compared with a hand-written MSI it keeps installation and updates small and maintainable. Signature trust remains separate: do not disable Windows security checks, remove trust marks, install certificates, or claim all unsigned-software warnings disappear.

Requirements:
- Default executable location `%LOCALAPPDATA%\Programs\KOL`, with stable application ID, Start menu shortcut, optional desktop shortcut, Windows installed-apps registration, and uninstaller.
- Installer startup task is opt-in for a new installation and reflects an existing Run entry for upgrades. Configure startup and saved settings together; preserve the selected item-size limit.
- Installed app registration takes priority over a portable executable when configuring automatic startup. Refuse registering an executable on UNC/network/WSL paths if no installed local executable exists.
- Gracefully close KOL before replacing it, including the existing portable v0.1.0 process. Abort on failure instead of force-killing a writer. Silent setup must not launch the UI or hang on a hidden message.
- Never install/delete/replace `history.db`, `history.key`, or previews. Uninstall disables startup and removes installed binaries/shortcuts/registration, while retaining the user's history and preferences (with startup disabled).
- Preserve portable ZIP distribution. Build an installer in Windows CI, test install/update/uninstall against disposable data, and publish `KOL-Setup-0.2.0.exe` alongside ZIP and checksums.

Validation: native startup executable selection and command-line quoting tests; existing history/Windows/race checks; graceful exit E2E; real unattended installer lifecycle in CI proving shortcuts, uninstall registration, startup command and unchanged data. Compile checks for installer and PowerShell scripts. Update the installed copy only after artifact verification, preserving live data and existing startup preference.

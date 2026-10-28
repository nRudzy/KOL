# KOL Windows Installer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Provide and apply a genuine per-user Windows installation, preserving existing clipboard history.

**Architecture:** Inno Setup manages executable files, shortcuts, App Paths and uninstall registration. Headless application commands gracefully stop the old instance and update startup preferences through the existing Go platform layer.

**Tech Stack:** Go 1.26+, Win32, Inno Setup 6.7+, PowerShell, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-10-28-installer-design.md`

## Global Constraints
- Per-user, no elevation; default `%LOCALAPPDATA%\Programs\KOL`.
- Never replace or remove clipboard history/key; preserve settings.
- No security-policy changes or trust-warning bypasses.
- Existing public GitHub publication authorization continues to apply.

## Review Focus
- Installed executable must win over a portable/WSL launch for startup.
- Shutdown must flush storage before file replacement, with no forced kill.
- Installer task, registry and saved preference must agree after install/update/uninstall.
- Quiet installation must return errors without opening hidden modal dialogs.
- Real user data stays outside the files managed by the uninstaller.

### Task 1: Native installation lifecycle
**Files:** `internal/platform/startup_windows.go`, `internal/platform/platform_windows_test.go`, `internal/desktop/lifecycle_windows.go`, `internal/desktop/app_windows_test.go`, `cmd/kol/main_windows.go`.
- [x] Write failing tests for registered local executable preference, network startup refusal, and preserving the item limit while changing startup.
- [x] Implement `ConfigureStartup(dir,mode string)error`, `chooseStartupExecutable(current,registered string)(string,error)` and headless `--quit`, `--configure-startup=enable|disable` commands.
- [x] Add native app-window identification and graceful shutdown compatibility with v0.1.0.
- [x] Run native tests, Go vet and existing portable suite; commit verified implementation.

### Task 2: Installer and distribution
**Files:** `installer/KOL.iss`, `scripts/build-installer.ps1`, `scripts/test-installer.ps1`, `.github/workflows/ci.yml`, `README.md`, version defaults.
- [x] Create per-user French installer, safe preinstall shutdown, optional tasks, App Paths, shortcuts and uninstaller preserving data.
- [x] Add installer build and real lifecycle smoke test in Windows CI with disposable history/key/config.
- [x] Verify first install, update, disabling startup and uninstall; keep portable ZIP and add checksum-covered installer release asset.
- [x] Fresh reviewer checks lifecycle/ownership/data handling; fix material issues and rerun relevant tests.
- [x] Publish v0.2.0, verify downloaded artifacts, apply installation locally with preserved preferences and confirm Windows registration.

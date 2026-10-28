//go:build windows

package platform

import (
	"bytes"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStartupCommandPreservesSelectedDataDirectory(t *testing.T) {
	exe := `C:\Program Files\KOL\KOL.exe`
	dir := `D:\Mes copies\épingles`
	args, err := windows.DecomposeCommandLine(startupCommand(exe, dir))
	if err != nil || !reflect.DeepEqual(args, []string{exe, "--background", "--data-dir", dir}) {
		t.Fatalf("startup arguments changed: %q (%v)", args, err)
	}
}

func TestStartupUsesRegisteredInstallationInsteadOfNetworkPortable(t *testing.T) {
	installed := filepath.Join(t.TempDir(), "KOL.exe")
	if err := os.WriteFile(installed, []byte("installed executable"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, current := range []string{`\\wsl.localhost\Ubuntu\home\user\KOL.exe`, `C:\Downloads\KOL.exe`} {
		got, err := chooseStartupExecutable(current, installed)
		if err != nil || got != installed {
			t.Fatalf("installation must win: %q %v", got, err)
		}
	}
}

func TestNetworkExecutableCannotBeRegisteredForStartup(t *testing.T) {
	for _, path := range []string{`\\wsl.localhost\Ubuntu\home\user\KOL.exe`, `\\server\share\KOL.exe`, `//server/share/KOL.exe`, `KOL.exe`} {
		if _, err := chooseStartupExecutable(path, ""); err == nil {
			t.Fatalf("unsafe startup location accepted: %q", path)
		}
	}
	local := `C:\Users\Person\Applications\KOL.exe`
	if got, err := chooseStartupExecutable(local, `C:\missing\KOL.exe`); err != nil || got != local {
		t.Fatalf("local portable fallback failed: %q %v", got, err)
	}
}

func TestConfigureStartupPreservesOtherSettingsAndHistory(t *testing.T) {
	dir := t.TempDir()
	original := Settings{MaxItemMiB: 512, StartAtLogin: false}
	if err := SaveSettings(dir, original); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"history.db", "history.key"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("untouched data"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var calls []bool
	apply := func(enabled bool, dataDir string) error {
		if dataDir != dir {
			t.Fatal("wrong data directory")
		}
		calls = append(calls, enabled)
		return nil
	}
	if err := configureStartup(dir, "enable", apply); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSettings(dir)
	if err != nil || !s.StartAtLogin || s.MaxItemMiB != 512 {
		t.Fatal("preferences changed", s, err)
	}
	if err = configureStartup(dir, "disable", apply); err != nil {
		t.Fatal(err)
	}
	s, err = LoadSettings(dir)
	if err != nil || s.StartAtLogin || s.MaxItemMiB != 512 {
		t.Fatal("preferences changed", s, err)
	}
	if !reflect.DeepEqual(calls, []bool{true, false}) {
		t.Fatal("startup changes not applied", calls)
	}
	if err = configureStartup(dir, "invalid", apply); err == nil {
		t.Fatal("invalid mode accepted")
	}
	for _, name := range []string{"history.db", "history.key"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(b) != "untouched data" {
			t.Fatal("history was changed", name, err)
		}
	}
}

func TestDPAPIKeyPersistenceAndMissingKey(t *testing.T) {
	dir := t.TempDir()
	key, err := LoadKey(dir)
	if err != nil {
		t.Fatal(err)
	}
	again, err := LoadKey(dir)
	if err != nil || !bytes.Equal(key, again) {
		t.Fatal("DPAPI round trip failed", err)
	}
	onDisk, err := os.ReadFile(filepath.Join(dir, "history.key"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(key, onDisk) || bytes.Contains(onDisk, key) {
		t.Fatal("key stored in plaintext")
	}
	if err = os.Remove(filepath.Join(dir, "history.key")); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "history.db"), []byte("existing history"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadKey(dir); err == nil {
		t.Fatal("missing database key was silently replaced")
	}
}

func TestSettingsPersistenceAndValidation(t *testing.T) {
	dir := t.TempDir()
	s, err := LoadSettings(dir)
	if err != nil || s.MaxItemMiB != 64 || s.StartAtLogin {
		t.Fatal("incorrect defaults", err)
	}
	want := Settings{MaxItemMiB: 128, StartAtLogin: true}
	if err = SaveSettings(dir, want); err != nil {
		t.Fatal(err)
	}
	s, err = LoadSettings(dir)
	if err != nil || s != want {
		t.Fatal("settings not persisted", err)
	}
	if err = SaveSettings(dir, Settings{MaxItemMiB: 0}); err == nil {
		t.Fatal("invalid limit accepted")
	}
}

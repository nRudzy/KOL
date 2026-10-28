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

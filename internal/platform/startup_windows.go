//go:build windows

package platform

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const AppPathsKey = `Software\Microsoft\Windows\CurrentVersion\App Paths\KOL.exe`

func localExecutable(path string) bool {
	path = filepath.Clean(path)
	// UNC includes WSL's network share. A normal local extended-length path is
	// allowed, but its UNC equivalent is still a network path.
	if strings.HasPrefix(path, `\\?\`) {
		path = strings.TrimPrefix(path, `\\?\`)
		if strings.HasPrefix(strings.ToUpper(path), `UNC\`) {
			return false
		}
	}
	return filepath.IsAbs(path) && !strings.HasPrefix(path, `\\`)
}

func chooseStartupExecutable(current, registered string) (string, error) {
	if localExecutable(registered) {
		if info, err := os.Stat(registered); err == nil && !info.IsDir() {
			return registered, nil
		}
	}
	if !localExecutable(current) {
		return "", errors.New("pour démarrer avec Windows, installez KOL ou placez-le dans un dossier Windows local")
	}
	return current, nil
}

func registeredExecutable() (string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, AppPathsKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer k.Close()
	path, _, err := k.GetStringValue("")
	if errors.Is(err, registry.ErrNotExist) {
		return "", nil
	}
	return path, err
}

// ConfigureStartup is the noninteractive installer entry point. Clipboard data
// is not opened: only the existing preferences and the Run value are updated.
func ConfigureStartup(dir, mode string) error { return configureStartup(dir, mode, SetStartup) }

func configureStartup(dir, mode string, apply func(bool, string) error) error {
	if mode != "enable" && mode != "disable" {
		return errors.New("mode de démarrage attendu : enable ou disable")
	}
	s, err := LoadSettings(dir)
	if err != nil {
		return err
	}
	previous := s.StartAtLogin
	s.StartAtLogin = mode == "enable"
	if err = apply(s.StartAtLogin, dir); err != nil {
		return err
	}
	if err = SaveSettings(dir, s); err != nil {
		return errors.Join(err, apply(previous, dir))
	}
	return nil
}

//go:build windows

package platform

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

type Settings struct {
	StartAtLogin bool `json:"startAtLogin"`
	MaxItemMiB   int  `json:"maxItemMiB"`
}

func DataDir() (string, error) {
	root, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, "KOL")
	return dir, os.MkdirAll(dir, 0700)
}

func protect(data []byte, decrypt bool) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("clé vide")
	}
	in := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	var out windows.DataBlob
	var err error
	if decrypt {
		err = windows.CryptUnprotectData(&in, nil, nil, 0, nil, 1, &out)
	} else {
		err = windows.CryptProtectData(&in, nil, nil, 0, nil, 1, &out)
	}
	if err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, int(out.Size))...), nil
}

func LoadKey(dir string) ([]byte, error) {
	path := filepath.Join(dir, "history.key")
	sealed, err := os.ReadFile(path)
	if err == nil {
		key, err := protect(sealed, true)
		if err != nil {
			return nil, fmt.Errorf("déverrouiller la clé avec ce compte Windows : %w", err)
		}
		if len(key) != 32 {
			return nil, errors.New("clé KOL invalide")
		}
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(dir, "history.db")); err == nil {
		return nil, errors.New("la clé de l’historique est manquante ; restaurez history.key depuis votre sauvegarde")
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	sealed, err = protect(key, false)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	_, writeErr := f.Write(sealed)
	syncErr := f.Sync()
	closeErr := f.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return nil, err
	}
	return key, nil
}

func LoadSettings(dir string) (Settings, error) {
	s := Settings{MaxItemMiB: 64}
	b, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err = json.Unmarshal(b, &s); err != nil {
		return s, err
	}
	if s.MaxItemMiB < 1 || s.MaxItemMiB > 512 {
		return s, errors.New("limite d’élément invalide (1 à 512 Mo)")
	}
	return s, nil
}

func SaveSettings(dir string, s Settings) error {
	if s.MaxItemMiB < 1 || s.MaxItemMiB > 512 {
		return errors.New("choisissez une taille de 1 à 512 Mo")
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "settings.json")
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	from, _ := windows.UTF16PtrFromString(tmp)
	to, _ := windows.UTF16PtrFromString(path)
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}

func startupCommand(exe, dir string) string {
	return windows.ComposeCommandLine([]string{exe, "--background", "--data-dir", dir})
}

func SetStartup(enabled bool, dir string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	if !enabled {
		err = key.DeleteValue("KOL")
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	registered, err := registeredExecutable()
	if err != nil {
		return err
	}
	exe, err = chooseStartupExecutable(exe, registered)
	if err != nil {
		return err
	}
	return key.SetStringValue("KOL", startupCommand(exe, dir))
}

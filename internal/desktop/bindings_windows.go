//go:build windows

package desktop

import (
	"errors"
	"time"

	"github.com/nRudzy/KOL/internal/clipboard"
	"github.com/nRudzy/KOL/internal/history"
	"github.com/nRudzy/KOL/internal/platform"
)

type state struct {
	Entries   []history.Entry   `json:"entries"`
	Total     int               `json:"total"`
	Matched   int               `json:"matched"`
	Paused    bool              `json:"paused"`
	Settings  platform.Settings `json:"settings"`
	Version   string            `json:"version"`
	LastError string            `json:"lastError"`
}

func (a *app) state(query, kind string, limit int) state {
	list := a.store.List(query, kind)
	a.mu.RLock()
	s := state{Total: len(a.store.List("", "all")), Matched: len(list), Paused: a.paused, Settings: a.settings, Version: a.version, LastError: a.lastError}
	a.mu.RUnlock()
	if !a.hotkey && s.LastError == "" {
		s.LastError = "Le raccourci Ctrl+Alt+V est déjà utilisé. L’icône près de l’horloge reste disponible."
	}
	limit = max(1, min(limit, 10000))
	s.Entries = list[:min(limit, len(list))]
	for i := range s.Entries {
		if len(s.Entries[i].Text) > 1600 {
			r := []rune(s.Entries[i].Text)
			s.Entries[i].Text = string(r[:min(1200, len(r))])
		}
	}
	return s
}

func (a *app) bind(background bool) error {
	bindings := map[string]any{
		"kolState": a.state,
		"kolCopy": func(id string) error {
			clip, err := a.store.Get(id)
			if err != nil {
				return err
			}
			a.clipMu.Lock()
			_, err = clipboard.Write(a.hwnd, clip)
			a.lastWritten.Store(clipboard.Sequence())
			a.clipMu.Unlock()
			if err != nil {
				return err
			}
			if _, err = a.store.Add(clip, time.Now()); err != nil {
				return err
			}
			a.hide(true)
			return nil
		},
		"kolDetail": func(id string) (history.Entry, error) {
			clip, err := a.store.Get(id)
			if err != nil {
				return history.Entry{}, err
			}
			e := history.Summarize(clip)
			if len(e.Text) > 131072 {
				r := []rune(e.Text)
				e.Text = string(r[:min(100000, len(r))]) + "\n\n[Aperçu limité ; la copie intégrale reste conservée.]"
			}
			return e, nil
		},
		"kolPin":    func(id string, pinned bool) error { return a.store.Pin(id, pinned) },
		"kolDelete": a.store.Delete,
		"kolClear":  a.store.Clear,
		"kolPause":  func(paused bool) { a.mu.Lock(); a.paused = paused; a.mu.Unlock() },
		"kolSettings": func(next platform.Settings) error {
			if next.MaxItemMiB < 1 || next.MaxItemMiB > 512 {
				return errors.New("choisissez une taille de 1 à 512 Mo")
			}
			a.mu.RLock()
			previous := a.settings
			a.mu.RUnlock()
			if err := platform.SetStartup(next.StartAtLogin); err != nil {
				return err
			}
			if err := platform.SaveSettings(a.dir, next); err != nil {
				_ = platform.SetStartup(previous.StartAtLogin)
				return err
			}
			a.mu.Lock()
			a.settings = next
			a.mu.Unlock()
			return nil
		},
		"kolHide": func() { a.hide(true) },
		"kolQuit": a.quit,
		"kolReady": func() {
			if !background {
				a.show()
			}
		},
	}
	for name, fn := range bindings {
		if err := a.web.Bind(name, fn); err != nil {
			return err
		}
	}
	return nil
}

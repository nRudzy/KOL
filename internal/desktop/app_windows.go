//go:build windows

package desktop

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	webview "github.com/jchv/go-webview2"
	"github.com/nRudzy/KOL/internal/clipboard"
	"github.com/nRudzy/KOL/internal/history"
	"github.com/nRudzy/KOL/internal/platform"
	"github.com/nRudzy/KOL/internal/ui"
	"golang.org/x/sys/windows"
)

type app struct {
	mu                                           sync.RWMutex
	clipMu                                       sync.Mutex
	web                                          webview.WebView
	store                                        *history.Store
	dir, version                                 string
	settings                                     platform.Settings
	paused                                       bool
	lastError                                    string
	hwnd, originalProc, callback, icon, previous uintptr
	taskbarMessage, showMessage                  uint32
	hotkey, trayV4, menuOpen                     bool
	deactivatedAt                                time.Time
	closing                                      atomic.Bool
	signals                                      chan struct{}
	stop, done                                   chan struct{}
}

func Run(version, dataDir string, background bool) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// Per-monitor v2 awareness; the manifest sets this too on packaged builds.
	if p := user.NewProc("SetProcessDpiAwarenessContext"); p.Find() == nil {
		p.Call(^uintptr(3))
	}
	var err error
	if dataDir == "" {
		dataDir, err = platform.DataDir()
	} else {
		dataDir, err = filepath.Abs(dataDir)
		if err == nil {
			err = os.MkdirAll(dataDir, 0700)
		}
	}
	if err != nil {
		return err
	}
	hash := sha256.Sum256([]byte(strings.ToLower(dataDir)))
	mutex, err := windows.CreateMutex(nil, false, utf(fmt.Sprintf(`Local\KOL.%x`, hash[:16])))
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if mutex != 0 {
			windows.CloseHandle(mutex)
		}
		postMessage.Call(0xffff, uintptr(registerMessage(fmt.Sprintf("KOL.ShowPanel.v1.%x", hash[:16]))), 0, 0)
		return nil
	}
	if err != nil {
		return err
	}
	defer windows.CloseHandle(mutex)
	key, err := platform.LoadKey(dataDir)
	if err != nil {
		return err
	}
	store, err := history.Open(filepath.Join(dataDir, "history.db"), key)
	if err != nil {
		return err
	}
	defer store.Close()
	settings, err := platform.LoadSettings(dataDir)
	if err != nil {
		return fmt.Errorf("lire les préférences : %w", err)
	}
	if settings.StartAtLogin {
		if err = platform.SetStartup(true, dataDir); err != nil {
			return fmt.Errorf("démarrage Windows : %w", err)
		}
	}
	a := &app{dir: dataDir, version: version, settings: settings, store: store, signals: make(chan struct{}, 32), stop: make(chan struct{}), done: make(chan struct{}), showMessage: registerMessage(fmt.Sprintf("KOL.ShowPanel.v1.%x", hash[:16]))}
	a.web = webview.NewWithOptions(webview.WebViewOptions{DataPath: filepath.Join(dataDir, "WebView2"), AutoFocus: true, WindowOptions: webview.WindowOptions{Title: "KOL — Presse-papiers", Width: 440, Height: 680, IconId: 1}})
	if a.web == nil {
		return errors.New("le moteur Microsoft Edge WebView2 est nécessaire ; installez le WebView2 Evergreen Runtime puis relancez KOL")
	}
	a.hwnd = uintptr(a.web.Window())
	if err = a.configureWindow(); err != nil {
		user.NewProc("DestroyWindow").Call(a.hwnd)
		return fmt.Errorf("initialiser KOL : %w", err)
	}
	defer func() {
		user.NewProc("RemoveClipboardFormatListener").Call(a.hwnd)
		user.NewProc("UnregisterHotKey").Call(a.hwnd, 1)
		a.removeTray()
		a.closing.Store(true)
		user.NewProc("RemovePropW").Call(a.hwnd, uintptr(unsafe.Pointer(utf(appWindowProperty))))
		user.NewProc("DestroyWindow").Call(a.hwnd)
	}()
	if err = a.bind(background); err != nil {
		return err
	}
	a.web.SetHtml(ui.HTML())
	go a.captureLoop()
	defer func() { close(a.stop); <-a.done }()
	a.signalCapture()
	a.web.Run()
	return nil
}

func (a *app) signalCapture() {
	select {
	case a.signals <- struct{}{}:
	default:
	}
}
func (a *app) setError(message string) { a.mu.Lock(); a.lastError = message; a.mu.Unlock(); a.notify() }
func (a *app) notify() {
	if a.closing.Load() {
		return
	}
	a.web.Dispatch(func() {
		if visible, _, _ := isVisible.Call(a.hwnd); visible != 0 {
			a.web.Eval("window.dispatchEvent(new Event('kol:changed'))")
		}
	})
}

func (a *app) captureLoop() {
	defer close(a.done)
	var lastSeen uint32
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()
	var retry <-chan time.Time
	retryDelay := 250 * time.Millisecond
	scheduleRetry := func() { timer.Reset(retryDelay); retry = timer.C; retryDelay = min(2*time.Second, retryDelay*2) }
	for {
		select {
		case <-a.stop:
			return
		case <-a.signals:
		case <-retry:
		}
		retry = nil
		a.mu.RLock()
		paused := a.paused
		maxBytes := int64(a.settings.MaxItemMiB) << 20
		a.mu.RUnlock()
		if paused {
			continue
		}
		a.clipMu.Lock()
		seq := clipboard.Sequence()
		if seq == lastSeen {
			a.clipMu.Unlock()
			continue
		}
		clip, captured, err := clipboard.Read(a.hwnd, maxBytes)
		capturedAt := time.Now()
		a.clipMu.Unlock()
		if errors.Is(err, clipboard.ErrExcluded) {
			lastSeen = seq
			retryDelay = 250 * time.Millisecond
			continue
		}
		if err != nil {
			a.setError(err.Error())
			scheduleRetry()
			continue
		}
		// Observe ownership while locked; sequence numbers can change on close.
		if clip.Owner == a.hwnd {
			lastSeen = captured
			retryDelay = 250 * time.Millisecond
			continue
		}
		a.mu.RLock()
		paused = a.paused
		a.mu.RUnlock()
		if paused {
			continue
		}
		if len(clip.Formats) == 0 {
			lastSeen = captured
			if len(clip.Omitted) > 0 {
				a.setError("Cette copie ne peut pas être conservée. Essayez de copier son texte ou son image.")
			}
			continue
		}
		if _, err = a.store.Add(clip, capturedAt); err != nil {
			a.setError("Enregistrement impossible : " + err.Error())
			scheduleRetry()
			continue
		}
		lastSeen = captured
		retryDelay = 250 * time.Millisecond
		// Optional information retains unsupported formats without cluttering the list.
		a.setError("")
	}
}

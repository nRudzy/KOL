//go:build windows

package desktop

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/nRudzy/KOL/internal/clipboard"
	"github.com/nRudzy/KOL/internal/history"
	"github.com/nRudzy/KOL/internal/platform"
	"github.com/nRudzy/KOL/internal/wintest"
	"golang.org/x/sys/windows"
)

func captureTestApp(t *testing.T) *app {
	t.Helper()
	s, err := history.Open(filepath.Join(t.TempDir(), "history.db"), bytes.Repeat([]byte{2}, 32))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	a := &app{store: s, hwnd: wintest.Window(t), settings: platform.Settings{MaxItemMiB: 64}, signals: make(chan struct{}, 32), stop: make(chan struct{}), done: make(chan struct{}), closing: atomic.Bool{}}
	// Silence visual dispatches while exercising the actual capture worker.
	a.closing.Store(true)
	return a
}

func TestCaptureRetriesAfterReaderReleasesWithoutUpdate(t *testing.T) {
	wintest.IsolateClipboard(t)
	source := wintest.Window(t)
	a := captureTestApp(t)
	u := windows.StringToUTF16("capture after contention")
	b := make([]byte, len(u)*2)
	for i, v := range u {
		b[2*i] = byte(v)
		b[2*i+1] = byte(v >> 8)
	}
	if _, err := clipboard.Write(source, history.Snapshot{Formats: []history.Format{{ID: 13, Data: b}}}); err != nil {
		t.Fatal(err)
	}
	if ok, _, err := user.NewProc("OpenClipboard").Call(source); ok == 0 {
		t.Fatal(err)
	}
	go a.captureLoop()
	t.Cleanup(func() { close(a.stop); <-a.done })
	a.signalCapture()
	until := time.Now().Add(3 * time.Second)
	blocked := false
	for time.Now().Before(until) {
		a.mu.RLock()
		blocked = a.lastError != ""
		a.mu.RUnlock()
		if blocked {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	user.NewProc("CloseClipboard").Call()
	if !blocked {
		t.Fatal("test did not exercise an unavailable clipboard")
	}
	until = time.Now().Add(4 * time.Second)
	for time.Now().Before(until) {
		if len(a.store.List("", "all")) == 1 {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("current copy was lost after the reader released its lock without an update")
}

func TestCaptureIgnoresOnlyItsOwnClipboardOwner(t *testing.T) {
	wintest.IsolateClipboard(t)
	a := captureTestApp(t)
	u := windows.StringToUTF16("own replay")
	b := make([]byte, len(u)*2)
	for i, v := range u {
		b[2*i] = byte(v)
		b[2*i+1] = byte(v >> 8)
	}
	if _, err := clipboard.Write(a.hwnd, history.Snapshot{Formats: []history.Format{{ID: 13, Data: b}}}); err != nil {
		t.Fatal(err)
	}
	clip, _, err := clipboard.Read(a.hwnd, 64<<20)
	if err != nil {
		t.Fatal(err)
	}
	if clip.Owner != a.hwnd {
		t.Fatal("read did not capture the actual clipboard owner")
	}
	go a.captureLoop()
	t.Cleanup(func() { close(a.stop); <-a.done })
	a.signalCapture()
	time.Sleep(200 * time.Millisecond)
	if len(a.store.List("", "all")) != 0 {
		t.Fatal("KOL captured its own restoration")
	}
}

func TestStateLimitsPreviewWithoutChangingStoredCopy(t *testing.T) {
	s, err := history.Open(filepath.Join(t.TempDir(), "history.db"), bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	raw := strings.Repeat("é", 2000)
	data := windows.StringToUTF16(raw)
	b := make([]byte, len(data)*2)
	for i, v := range data {
		b[i*2] = byte(v)
		b[i*2+1] = byte(v >> 8)
	}
	e, err := s.Add(history.Snapshot{Formats: []history.Format{{ID: 13, Data: b}}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	a := app{store: s, settings: platform.Settings{MaxItemMiB: 64}, hotkey: true}
	state := a.state("é", "text", 200)
	if state.Matched != 1 || len([]rune(state.Entries[0].Text)) != 1200 {
		t.Fatal("preview not bounded")
	}
	got, err := s.Get(e.ID)
	if err != nil || !bytes.Equal(got.Formats[0].Data, b) {
		t.Fatal("preview changed raw payload", err)
	}
}

// Opt-in real executable test. It only sends messages to its own child window;
// no user clipboard writes or keyboard/mouse injection are performed.
func TestDesktopLifecycle(t *testing.T) {
	binaryPath := os.Getenv("KOL_E2E_BINARY")
	if binaryPath == "" {
		t.Skip("set KOL_E2E_BINARY to test the built Windows executable")
	}
	dir := t.TempDir()
	child := exec.Command(binaryPath, "--background", "--data-dir", dir)
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- child.Wait() }()
	var hwnd uintptr
	defer func() {
		if hwnd != 0 {
			postMessage.Call(hwnd, 0x0016, 1, 0)
		}
		select {
		case <-exited:
		case <-time.After(5 * time.Second):
			_ = child.Process.Kill()
			<-exited
		}
	}()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		callback := windows.NewCallback(func(w, unused uintptr) uintptr {
			var pid uint32
			user.NewProc("GetWindowThreadProcessId").Call(w, uintptr(unsafe.Pointer(&pid)))
			if pid == uint32(child.Process.Pid) {
				var title [128]uint16
				user.NewProc("GetWindowTextW").Call(w, uintptr(unsafe.Pointer(&title[0])), 128)
				if windows.UTF16ToString(title[:]) == "KOL — Presse-papiers" {
					hwnd = w
					return 0
				}
			}
			return 1
		})
		user.NewProc("EnumWindows").Call(callback, 0)
		if hwnd != 0 {
			break
		}
		select {
		case err := <-exited:
			exited <- err
			t.Fatalf("KOL exited before showing its window: %v", err)
		default:
		}
		time.Sleep(100 * time.Millisecond)
	}
	if hwnd == 0 {
		t.Fatal("KOL window was not created")
	}
	// Initial embedding is asynchronous. Wait for the registered message loop by
	// using WM_HOTKEY, then verify the resulting actual native window state.
	waitVisible := func(want bool) {
		t.Helper()
		until := time.Now().Add(10 * time.Second)
		for time.Now().Before(until) {
			v, _, _ := isVisible.Call(hwnd)
			if (v != 0) == want {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatalf("window visibility did not become %v", want)
	}
	// Wait until the background app has finished initializing and hidden itself.
	waitVisible(false)
	postMessage.Call(hwnd, 0x0312, 1, 0)
	waitVisible(true)
	var r rect
	user.NewProc("GetWindowRect").Call(hwnd, uintptr(unsafe.Pointer(&r)))
	if r.Right <= r.Left || r.Bottom <= r.Top {
		t.Fatal("invalid panel dimensions")
	}
	postMessage.Call(hwnd, 0x0010, 0, 0)
	waitVisible(false)
	second := exec.Command(binaryPath, "--background", "--data-dir", dir)
	if output, err := second.CombinedOutput(); err != nil {
		t.Fatalf("second instance failed: %v (%s)", err, output)
	}
	waitVisible(true)
	if _, err := os.Stat(filepath.Join(dir, "history.key")); err != nil {
		t.Fatal("DPAPI key was not created", err)
	}
}

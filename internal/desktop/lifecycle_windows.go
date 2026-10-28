//go:build windows

package desktop

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const appWindowProperty = "KOL.Application.v1"

// StopExisting asks this user's KOL windows to exit and waits for storage to
// close. The v0.1.0 fallback is limited to KOL's title, class and executable.
// It never terminates a process or modifies a clipboard/history database.
func StopExisting() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var processes []windows.Handle
	defer func() {
		for _, p := range processes {
			windows.CloseHandle(p)
		}
	}()
	seen := make(map[uint32]bool)
	var problem error
	callback := windows.NewCallback(func(hwnd, unused uintptr) uintptr {
		var title [128]uint16
		var class [64]uint16
		user.NewProc("GetWindowTextW").Call(hwnd, uintptr(unsafe.Pointer(&title[0])), uintptr(len(title)))
		if windows.UTF16ToString(title[:]) != "KOL — Presse-papiers" {
			return 1
		}
		user.NewProc("GetClassNameW").Call(hwnd, uintptr(unsafe.Pointer(&class[0])), uintptr(len(class)))
		if windows.UTF16ToString(class[:]) != "webview" {
			return 1
		}
		var pid uint32
		user.NewProc("GetWindowThreadProcessId").Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if pid == 0 || pid == windows.GetCurrentProcessId() || seen[pid] {
			return 1
		}
		p, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
		if err != nil {
			problem = err
			return 1
		}
		marker, _, _ := user.NewProc("GetPropW").Call(hwnd, uintptr(unsafe.Pointer(utf(appWindowProperty))))
		if marker != 1 {
			var image [32768]uint16
			n := uint32(len(image))
			if windows.QueryFullProcessImageName(p, 0, &image[0], &n) != nil || !strings.EqualFold(filepath.Base(windows.UTF16ToString(image[:n])), "KOL.exe") {
				windows.CloseHandle(p)
				return 1
			}
		}
		seen[pid] = true
		processes = append(processes, p)
		if ok, _, err := postMessage.Call(hwnd, 0x0016, 1, 0); ok == 0 {
			problem = err
		}
		return 1
	})
	user.NewProc("EnumWindows").Call(callback, 0)
	if problem != nil {
		return fmt.Errorf("fermez KOL depuis son icône avant de continuer : %w", problem)
	}
	for _, p := range processes {
		result, err := windows.WaitForSingleObject(p, 10000)
		if err != nil {
			return err
		}
		if result != windows.WAIT_OBJECT_0 {
			return errors.New("KOL est encore ouvert ; quittez-le depuis son icône, puis réessayez")
		}
	}
	return nil
}

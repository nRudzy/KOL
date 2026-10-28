//go:build windows

// Package wintest isolates native clipboard tests from the interactive session.
package wintest

import (
	"errors"
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

var user = windows.NewLazySystemDLL("user32.dll")

func IsolateClipboard(t *testing.T) {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	// Unnamed stations are named by logon session, so different go test package
	// processes share one. Serialize their native tests without needing admin.
	mutexName, _ := windows.UTF16PtrFromString(`Local\KOL.Clipboard.Tests`)
	mutex, err := windows.CreateMutex(nil, false, mutexName)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		t.Fatal(err)
	}
	t.Cleanup(func() { windows.CloseHandle(mutex) })
	result, err := windows.WaitForSingleObject(mutex, windows.INFINITE)
	if err != nil || (result != windows.WAIT_OBJECT_0 && result != 0x80) {
		t.Fatal("acquire clipboard test mutex", err)
	}
	t.Cleanup(func() { windows.ReleaseMutex(mutex) })
	oldStation, _, _ := user.NewProc("GetProcessWindowStation").Call()
	oldDesktop, _, _ := user.NewProc("GetThreadDesktop").Call(uintptr(windows.GetCurrentThreadId()))
	station, _, err := user.NewProc("CreateWindowStationW").Call(0, 0, 0x000f037f, 0)
	if station == 0 {
		t.Fatal(err)
	}
	t.Cleanup(func() { user.NewProc("CloseWindowStation").Call(station) })
	if ok, _, err := user.NewProc("SetProcessWindowStation").Call(station); ok == 0 {
		t.Fatal(err)
	}
	t.Cleanup(func() { user.NewProc("SetProcessWindowStation").Call(oldStation) })
	name, _ := windows.UTF16PtrFromString("KOLTests")
	desktop, _, err := user.NewProc("CreateDesktopW").Call(uintptr(unsafe.Pointer(name)), 0, 0, 0, 0x000f01ff, 0)
	if desktop == 0 {
		t.Fatal(err)
	}
	t.Cleanup(func() { user.NewProc("CloseDesktop").Call(desktop) })
	if ok, _, err := user.NewProc("SetThreadDesktop").Call(desktop); ok == 0 {
		t.Fatal(err)
	}
	t.Cleanup(func() { user.NewProc("SetThreadDesktop").Call(oldDesktop) })
}

func Window(t *testing.T) uintptr {
	t.Helper()
	class, _ := windows.UTF16PtrFromString("STATIC")
	hwnd, _, err := user.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(class)), 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	if hwnd == 0 {
		t.Fatal(err)
	}
	t.Cleanup(func() { user.NewProc("DestroyWindow").Call(hwnd) })
	return hwnd
}

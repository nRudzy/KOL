//go:build windows

package desktop

import (
	"encoding/binary"
	"errors"
	"time"
	"unsafe"

	"github.com/nRudzy/KOL/assets"
	"golang.org/x/sys/windows"
)

var user = windows.NewLazySystemDLL("user32.dll")
var shell = windows.NewLazySystemDLL("shell32.dll")
var dwm = windows.NewLazySystemDLL("dwmapi.dll")
var gdi = windows.NewLazySystemDLL("gdi32.dll")
var showWindow = user.NewProc("ShowWindow")
var setWindowPos = user.NewProc("SetWindowPos")
var setWindowLong = user.NewProc("SetWindowLongPtrW")
var callWindowProc = user.NewProc("CallWindowProcW")
var postMessage = user.NewProc("PostMessageW")
var isVisible = user.NewProc("IsWindowVisible")

const trayMessage = 0x8000 + 42

type point struct{ X, Y int32 }
type rect struct{ Left, Top, Right, Bottom int32 }
type monitorInfo struct {
	Size          uint32
	Monitor, Work rect
	Flags         uint32
}
type notifyIcon struct {
	Size                uint32
	Wnd                 uintptr
	ID, Flags, Callback uint32
	Icon                uintptr
	Tip                 [128]uint16
	State, StateMask    uint32
	Info                [256]uint16
	Version             uint32
	InfoTitle           [64]uint16
	InfoFlags           uint32
	GUID                windows.GUID
	BalloonIcon         uintptr
}

func utf(s string) *uint16 { p, _ := windows.UTF16PtrFromString(s); return p }

func ShowError(err error) {
	user.NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(utf(err.Error()))), uintptr(unsafe.Pointer(utf("KOL"))), 0x10)
}

func (a *app) configureWindow() error {
	showWindow.Call(a.hwnd, 0)
	setWindowLong.Call(a.hwnd, ^uintptr(15), 0x86000000) // GWL_STYLE: popup + clip children/siblings
	setWindowLong.Call(a.hwnd, ^uintptr(19), 0x88)       // GWL_EXSTYLE: tool window + topmost
	a.callback = windows.NewCallback(a.wndProc)
	old, _, err := setWindowLong.Call(a.hwnd, ^uintptr(3), a.callback)
	if old == 0 {
		return err
	}
	a.originalProc = old
	setWindowPos.Call(a.hwnd, ^uintptr(0), 0, 0, 440, 680, 0x0027) // TOPMOST, FRAMECHANGED, no move/size
	preference := uint32(2)
	dwm.NewProc("DwmSetWindowAttribute").Call(a.hwnd, 33, uintptr(unsafe.Pointer(&preference)), 4)
	border := uint32(0x00e8e3dc)
	dwm.NewProc("DwmSetWindowAttribute").Call(a.hwnd, 34, uintptr(unsafe.Pointer(&border)), 4)
	a.taskbarMessage = registerMessage("TaskbarCreated")
	if ok, _, err := user.NewProc("AddClipboardFormatListener").Call(a.hwnd); ok == 0 {
		return err
	}
	if ok, _, _ := user.NewProc("RegisterHotKey").Call(a.hwnd, 1, 0x4003, 0x56); ok != 0 {
		a.hotkey = true
	}
	return a.addTray()
}

func registerMessage(name string) uint32 {
	n, _, _ := user.NewProc("RegisterWindowMessageW").Call(uintptr(unsafe.Pointer(utf(name))))
	return uint32(n)
}

func (a *app) addTray() error {
	if a.icon == 0 {
		data := assets.Icon
		if len(data) < 22 {
			return errors.New("icône KOL invalide")
		}
		count := int(binary.LittleEndian.Uint16(data[4:]))
		best := 6
		for i := 0; i < count; i++ {
			offset := 6 + i*16
			if offset+16 > len(data) {
				break
			}
			if data[offset] == 32 {
				best = offset
				break
			}
		}
		size := int(binary.LittleEndian.Uint32(data[best+8:]))
		offset := int(binary.LittleEndian.Uint32(data[best+12:]))
		if offset < 0 || size < 1 || offset > len(data)-size {
			return errors.New("icône KOL invalide")
		}
		a.icon, _, _ = user.NewProc("CreateIconFromResourceEx").Call(uintptr(unsafe.Pointer(&data[offset])), uintptr(size), 1, 0x00030000, 32, 32, 0)
		if a.icon == 0 {
			return errors.New("impossible de charger l’icône KOL")
		}
	}
	n := notifyIcon{Wnd: a.hwnd, ID: 1, Flags: 0x7, Callback: trayMessage, Icon: a.icon}
	n.Size = uint32(unsafe.Sizeof(n))
	copy(n.Tip[:], windows.StringToUTF16("KOL — Le fil de vos copies"))
	if ok, _, err := shell.NewProc("Shell_NotifyIconW").Call(0, uintptr(unsafe.Pointer(&n))); ok == 0 {
		return err
	}
	n.Version = 4
	ok, _, _ := shell.NewProc("Shell_NotifyIconW").Call(4, uintptr(unsafe.Pointer(&n)))
	a.trayV4 = ok != 0
	return nil
}

func (a *app) removeTray() {
	n := notifyIcon{Wnd: a.hwnd, ID: 1}
	n.Size = uint32(unsafe.Sizeof(n))
	shell.NewProc("Shell_NotifyIconW").Call(2, uintptr(unsafe.Pointer(&n)))
	if a.icon != 0 {
		user.NewProc("DestroyIcon").Call(a.icon)
	}
}

func (a *app) wndProc(hwnd, msg, wp, lp uintptr) uintptr {
	switch uint32(msg) {
	case 0x031d:
		a.signalCapture()
		return 0
	case trayMessage:
		event := uint32(lp) & 0xffff
		if event == 0x400 || event == 0x401 || (!a.trayV4 && event == 0x202) {
			if time.Since(a.deactivatedAt) < 250*time.Millisecond {
				a.hide(false)
			} else {
				a.toggle()
			}
			return 0
		}
		if event == 0x7b || event == 0x205 {
			a.trayMenu()
			return 0
		}
	case 0x0312:
		if wp == 1 {
			a.toggle()
			return 0
		}
	case 0x0010:
		if !a.closing.Load() {
			a.hide(true)
			return 0
		}
	case 0x0006:
		if wp&0xffff == 0 && !a.menuOpen {
			a.deactivatedAt = time.Now()
			a.hide(false)
		}
	case 0x02e0:
		if ok, _, _ := isVisible.Call(a.hwnd); ok != 0 {
			a.position()
		}
		return 0
	case 0x0011:
		return 1 // WM_QUERYENDSESSION
	case 0x0016:
		if wp != 0 {
			a.quit()
		}
		return 0
	}
	if uint32(msg) == a.taskbarMessage {
		if err := a.addTray(); err != nil {
			a.setError("L’icône KOL n’a pas pu être rétablie.")
		}
		return 0
	}
	if uint32(msg) == a.showMessage {
		a.show()
		return 0
	}
	r, _, _ := callWindowProc.Call(a.originalProc, hwnd, msg, wp, lp)
	return r
}

func (a *app) position() {
	var cursor point
	user.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&cursor)))
	packed := uintptr(uint32(cursor.X)) | uintptr(uint32(cursor.Y))<<32
	monitor, _, _ := user.NewProc("MonitorFromPoint").Call(packed, 2)
	info := monitorInfo{}
	info.Size = uint32(unsafe.Sizeof(info))
	if ok, _, _ := user.NewProc("GetMonitorInfoW").Call(monitor, uintptr(unsafe.Pointer(&info))); ok == 0 {
		return
	}
	dpi := uint32(96)
	var yDPI uint32
	shcore := windows.NewLazySystemDLL("shcore.dll")
	if proc := shcore.NewProc("GetDpiForMonitor"); proc.Find() == nil {
		proc.Call(monitor, 0, uintptr(unsafe.Pointer(&dpi)), uintptr(unsafe.Pointer(&yDPI)))
	}
	if dpi < 72 || dpi > 480 {
		dpi = 96
	}
	margin := int32(12 * dpi / 96)
	width := min(int32(440*dpi/96), info.Work.Right-info.Work.Left-2*margin)
	height := min(int32(680*dpi/96), info.Work.Bottom-info.Work.Top-2*margin)
	x := info.Work.Right - width - margin
	y := info.Work.Bottom - height - margin
	// Top/left taskbars and tray popovers should keep the panel in the work area.
	if cursor.Y < info.Work.Top+margin {
		y = info.Work.Top + margin
	}
	if cursor.X < info.Work.Left+margin {
		x = info.Work.Left + margin
	}
	setWindowPos.Call(a.hwnd, ^uintptr(0), uintptr(x), uintptr(y), uintptr(width), uintptr(height), 0x0010)
	// A region also gives rounded corners on Windows 10, where DWM ignores the preference.
	region, _, _ := gdi.NewProc("CreateRoundRectRgn").Call(0, 0, uintptr(width+1), uintptr(height+1), uintptr(18*dpi/96), uintptr(18*dpi/96))
	if region != 0 {
		if ok, _, _ := user.NewProc("SetWindowRgn").Call(a.hwnd, region, 1); ok == 0 {
			gdi.NewProc("DeleteObject").Call(region)
		}
	}
}

func (a *app) show() {
	foreground, _, _ := user.NewProc("GetForegroundWindow").Call()
	if foreground != 0 && foreground != a.hwnd {
		a.previous = foreground
	}
	a.position()
	showWindow.Call(a.hwnd, 5)
	user.NewProc("SetForegroundWindow").Call(a.hwnd)
	a.web.Eval("window.dispatchEvent(new Event('kol:shown'))")
}
func (a *app) hide(restore bool) {
	showWindow.Call(a.hwnd, 0)
	if restore && a.previous != 0 {
		user.NewProc("SetForegroundWindow").Call(a.previous)
	}
}
func (a *app) toggle() {
	if visible, _, _ := isVisible.Call(a.hwnd); visible != 0 {
		a.hide(true)
	} else {
		a.show()
	}
}
func (a *app) quit() {
	if a.closing.CompareAndSwap(false, true) {
		a.web.Terminate()
	}
}

func (a *app) trayMenu() {
	menu, _, _ := user.NewProc("CreatePopupMenu").Call()
	if menu == 0 {
		return
	}
	defer user.NewProc("DestroyMenu").Call(menu)
	appendItem := func(flags, id uintptr, label string) {
		user.NewProc("AppendMenuW").Call(menu, flags, id, uintptr(unsafe.Pointer(utf(label))))
	}
	appendItem(0, 1, "Ouvrir KOL\tCtrl+Alt+V")
	a.mu.RLock()
	paused := a.paused
	a.mu.RUnlock()
	label := "Mettre la capture en pause"
	if paused {
		label = "Reprendre la capture"
	}
	appendItem(0, 2, label)
	appendItem(0x800, 0, "")
	appendItem(0, 3, "Quitter KOL")
	var p point
	user.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&p)))
	a.menuOpen = true
	defer func() { a.menuOpen = false }()
	user.NewProc("SetForegroundWindow").Call(a.hwnd)
	choice, _, _ := user.NewProc("TrackPopupMenu").Call(menu, 0x0182, uintptr(p.X), uintptr(p.Y), 0, a.hwnd, 0)
	postMessage.Call(a.hwnd, 0, 0, 0)
	switch choice {
	case 1:
		a.show()
	case 2:
		a.mu.Lock()
		a.paused = !a.paused
		a.mu.Unlock()
		a.signalCapture()
		a.notify()
	case 3:
		a.quit()
	}
}

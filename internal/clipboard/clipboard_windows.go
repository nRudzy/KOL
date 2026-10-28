//go:build windows

// Package clipboard serializes transferable Win32 clipboard formats. It never
// treats a GDI handle or an application-owned live pointer as a memory block.
package clipboard

import (
	"encoding/binary"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"github.com/nRudzy/KOL/internal/history"
	"golang.org/x/sys/windows"
)

var user32 = windows.NewLazySystemDLL("user32.dll")
var kernel32 = windows.NewLazySystemDLL("kernel32.dll")
var gdi32 = windows.NewLazySystemDLL("gdi32.dll")
var openClipboard = user32.NewProc("OpenClipboard")
var closeClipboard = user32.NewProc("CloseClipboard")
var emptyClipboard = user32.NewProc("EmptyClipboard")
var enumFormats = user32.NewProc("EnumClipboardFormats")
var getData = user32.NewProc("GetClipboardData")
var setData = user32.NewProc("SetClipboardData")
var registerFormat = user32.NewProc("RegisterClipboardFormatW")
var formatName = user32.NewProc("GetClipboardFormatNameW")
var globalSize = kernel32.NewProc("GlobalSize")
var globalLock = kernel32.NewProc("GlobalLock")
var globalUnlock = kernel32.NewProc("GlobalUnlock")
var globalAlloc = kernel32.NewProc("GlobalAlloc")
var globalFree = kernel32.NewProc("GlobalFree")
var moveMemory = kernel32.NewProc("RtlMoveMemory")
var getEMFBits = gdi32.NewProc("GetEnhMetaFileBits")
var setEMFBits = gdi32.NewProc("SetEnhMetaFileBits")
var deleteEMF = gdi32.NewProc("DeleteEnhMetaFile")

var ErrExcluded = errors.New("cette copie est exclue de l’historique par l’application source")
var ErrChanged = errors.New("le presse-papiers a changé pendant la capture")

func Sequence() uint32 {
	n, _, _ := user32.NewProc("GetClipboardSequenceNumber").Call()
	return uint32(n)
}

func open(owner uintptr) error {
	for i := 0; i < 10; i++ {
		if ok, _, _ := openClipboard.Call(owner); ok != 0 {
			return nil
		}
		time.Sleep(time.Duration(8+i*4) * time.Millisecond)
	}
	return errors.New("le presse-papiers est occupé ; réessayez dans un instant")
}

// Win32's clipboard lock must be released on its opening OS thread.
func acquireClipboard(owner uintptr) (func(), error) {
	runtime.LockOSThread()
	if err := open(owner); err != nil {
		runtime.UnlockOSThread()
		return nil, err
	}
	return func() { closeClipboard.Call(); runtime.UnlockOSThread() }, nil
}

func namedFormat(name string) (uint32, error) {
	u, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	id, _, err := registerFormat.Call(uintptr(unsafe.Pointer(u)))
	if id == 0 {
		return 0, err
	}
	return uint32(id), nil
}

func nameOf(id uint32) string {
	if id < 0xc000 {
		return ""
	}
	var buf [256]uint16
	n, _, _ := formatName.Call(uintptr(id), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:n])
}

func memory(handle uintptr, max int64) ([]byte, error) {
	size, _, _ := globalSize.Call(handle)
	if size == 0 {
		return nil, errors.New("aucune mémoire transférable")
	}
	if uint64(size) > uint64(max) {
		return nil, errors.New("limite de taille dépassée")
	}
	ptr, _, _ := globalLock.Call(handle)
	if ptr == 0 {
		return nil, errors.New("mémoire inaccessible")
	}
	defer globalUnlock.Call(handle)
	data := make([]byte, int(size))
	moveMemory.Call(uintptr(unsafe.Pointer(&data[0])), ptr, size)
	return data, nil
}

// Windows can expose allocator padding in text HGLOBALs. The first NUL is the
// format terminator, not user whitespace; preserve it and every preceding byte.
func trimAllocation(data []byte, id uint32) []byte {
	if id == 13 {
		for i := 0; i+1 < len(data); i += 2 {
			if data[i] == 0 && data[i+1] == 0 {
				return data[:i+2]
			}
		}
	}
	if id == 1 || id == 7 || id == 0x81 {
		for i, b := range data {
			if b == 0 {
				return data[:i+1]
			}
		}
	}
	return data
}

func Read(owner uintptr, maxBytes int64) (history.Snapshot, uint32, error) {
	var last error
	for i := 0; i < 3; i++ {
		s, seq, err := readOnce(owner, maxBytes)
		if !errors.Is(err, ErrChanged) {
			return s, seq, err
		}
		last = err
	}
	return history.Snapshot{}, 0, last
}

func readOnce(owner uintptr, maxBytes int64) (history.Snapshot, uint32, error) {
	var clip history.Snapshot
	release, err := acquireClipboard(owner)
	if err != nil {
		return clip, 0, err
	}
	defer release()
	seq := Sequence()
	clip.Owner, _, _ = user32.NewProc("GetClipboardOwner").Call()
	var formats []uint32
	for id := uint32(0); ; {
		next, _, _ := enumFormats.Call(uintptr(id))
		if next == 0 {
			break
		}
		id = uint32(next)
		formats = append(formats, id)
		if len(formats) > 4096 {
			return clip, seq, errors.New("trop de formats dans le presse-papiers")
		}
	}
	for _, id := range formats {
		name := nameOf(id)
		if name == "ExcludeClipboardContentFromMonitorProcessing" || name == "Clipboard Viewer Ignore" {
			return clip, seq, ErrExcluded
		}
		if name == "CanIncludeInClipboardHistory" {
			h, _, _ := getData.Call(uintptr(id))
			if b, err := memory(h, 4096); err == nil && len(b) >= 4 && binary.LittleEndian.Uint32(b) == 0 {
				return clip, seq, ErrExcluded
			}
		}
	}
	var used int64
	for _, id := range formats {
		name := nameOf(id)
		if history.TransportFormat(name) || name == "CanUploadToCloudClipboard" || name == "CanIncludeInClipboardHistory" {
			continue
		}
		label := name
		if label == "" {
			label = fmt.Sprintf("CF_%d", id)
		}
		// Windows synthesizes portable DIB/EMF variants for these handles.
		if id == 2 || id == 9 || id == 3 || id == 0x82 || id == 0x83 {
			continue
		}
		if id == 0x80 || (id >= 0x200 && id <= 0x2ff) || liveFormat(name) {
			clip.Omitted = append(clip.Omitted, label+" : format propre à l’application")
			continue
		}
		h, _, _ := getData.Call(uintptr(id))
		if h == 0 {
			clip.Omitted = append(clip.Omitted, label+" : données indisponibles")
			continue
		}
		f := history.Format{ID: id, Name: name}
		var err error
		if id == 14 || id == 0x8e {
			f.Kind = "emf"
			n, _, _ := getEMFBits.Call(h, 0, 0)
			if n == 0 || uint64(n) > uint64(max(0, maxBytes-used)) {
				err = errors.New("image vectorielle indisponible ou trop volumineuse")
			} else {
				f.Data = make([]byte, n)
				copied, _, _ := getEMFBits.Call(h, n, uintptr(unsafe.Pointer(&f.Data[0])))
				if copied != n {
					err = errors.New("image vectorielle incomplète")
				}
			}
		} else {
			f.Data, err = memory(h, max(0, maxBytes-used))
			if err == nil {
				f.Data = trimAllocation(f.Data, id)
			}
		}
		if err != nil {
			clip.Omitted = append(clip.Omitted, label+" : "+err.Error())
			continue
		}
		used += int64(len(f.Data))
		clip.Formats = append(clip.Formats, f)
	}
	clip.Source = clipboardSource()
	if Sequence() != seq {
		return history.Snapshot{}, seq, ErrChanged
	}
	return clip, seq, nil
}

func liveFormat(name string) bool {
	switch strings.ToLower(name) {
	case "dataobject", "ole private data", "filecontents", "filegroupdescriptor", "filegroupdescriptorw", "link source", "ownerlink", "objectlink":
		return true
	}
	return false
}

func clipboardSource() string {
	hwnd, _, _ := user32.NewProc("GetClipboardOwner").Call()
	var pid uint32
	user32.NewProc("GetWindowThreadProcessId").Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return ""
	}
	p, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(p)
	buf := make([]uint16, 1024)
	n := uint32(len(buf))
	if windows.QueryFullProcessImageName(p, 0, &buf[0], &n) != nil {
		return ""
	}
	return filepath.Base(windows.UTF16ToString(buf[:n]))
}

type prepared struct {
	id          uint32
	handle      uintptr
	emf         bool
	transferred bool
}

// Allocate and validate every transferable handle before clearing the clipboard.
// On success SetClipboardData transfers ownership of each handle to Windows.
func Write(owner uintptr, clip history.Snapshot) (uint32, error) {
	var items []prepared
	defer func() {
		for _, p := range items {
			if !p.transferred && p.handle != 0 {
				if p.emf {
					deleteEMF.Call(p.handle)
				} else {
					globalFree.Call(p.handle)
				}
			}
		}
	}()
	formats := append([]history.Format(nil), clip.Formats...)
	for _, f := range formats {
		if f.ID == 15 {
			formats = append(formats, history.Format{Name: "Preferred DropEffect", Data: []byte{1, 0, 0, 0}})
			break
		}
	}
	for _, f := range formats {
		if history.TransportFormat(f.Name) && f.Name != "Preferred DropEffect" {
			continue
		}
		if len(f.Data) == 0 {
			continue
		}
		id := f.ID
		if f.Name != "" {
			var err error
			id, err = namedFormat(f.Name)
			if err != nil {
				return 0, fmt.Errorf("enregistrer le format : %w", err)
			}
		}
		if id == 0 || id == 2 || id == 3 || id == 9 || id == 0x80 || (id >= 0x200 && id <= 0x2ff) || liveFormat(f.Name) {
			continue
		}
		p := prepared{id: id, emf: f.Kind == "emf"}
		if p.emf {
			p.handle, _, _ = setEMFBits.Call(uintptr(len(f.Data)), uintptr(unsafe.Pointer(&f.Data[0])))
		} else {
			p.handle, _, _ = globalAlloc.Call(0x42, uintptr(len(f.Data))) // MOVEABLE | ZEROINIT
			if p.handle != 0 {
				ptr, _, _ := globalLock.Call(p.handle)
				if ptr == 0 {
					globalFree.Call(p.handle)
					p.handle = 0
				} else {
					moveMemory.Call(ptr, uintptr(unsafe.Pointer(&f.Data[0])), uintptr(len(f.Data)))
					globalUnlock.Call(p.handle)
				}
			}
		}
		if p.handle == 0 {
			return 0, errors.New("mémoire insuffisante pour recopier cet élément")
		}
		items = append(items, p)
	}
	if len(items) == 0 {
		return 0, errors.New("cette copie ne contient aucun format restituable")
	}
	release, err := acquireClipboard(owner)
	if err != nil {
		return 0, err
	}
	defer release()
	if ok, _, _ := emptyClipboard.Call(); ok == 0 {
		return 0, errors.New("impossible de préparer le presse-papiers")
	}
	var failures int
	for i := range items {
		if ok, _, _ := setData.Call(uintptr(items[i].id), items[i].handle); ok == 0 {
			failures++
		} else {
			items[i].transferred = true
		}
	}
	seq := Sequence()
	if failures > 0 {
		return seq, fmt.Errorf("%d format(s) n’ont pas pu être remis dans le presse-papiers", failures)
	}
	return seq, nil
}

//go:build windows

package clipboard

import (
	"bytes"
	"encoding/binary"
	"errors"
	"runtime"
	"testing"
	"unicode/utf16"

	"github.com/nRudzy/KOL/internal/history"
	"github.com/nRudzy/KOL/internal/wintest"
	"golang.org/x/sys/windows"
)

func utf16Bytes(s string) []byte {
	u := utf16.Encode([]rune(s + "\x00"))
	b := make([]byte, len(u)*2)
	for i, v := range u {
		binary.LittleEndian.PutUint16(b[i*2:], v)
	}
	return b
}

// A separate window station owns a separate clipboard. Never replace the user's
// live clipboard merely to test capture/restore, including on developer machines.
func TestWindowsClipboardRoundTrip(t *testing.T) {
	wintest.IsolateClipboard(t)
	hwnd := wintest.Window(t)
	cases := []struct {
		name string
		clip history.Snapshot
	}{
		{"unicode-whitespace", history.Snapshot{Formats: []history.Format{{ID: 13, Data: utf16Bytes("  éléphant 🐘\r\n\t ")}}}},
		{"rich-and-binary", history.Snapshot{Formats: []history.Format{{Name: "HTML Format", Data: []byte("Version:1.0\r\n<html><b>raw</b></html>\x00")}, {Name: "Rich Text Format", Data: []byte("{\\rtf1\\b raw}\x00")}, {Name: "KOL.Test.Binary", Data: []byte{0, 1, 2, 0, 255}}, {ID: 13, Data: utf16Bytes("raw")}}}},
	}
	drop := make([]byte, 20)
	binary.LittleEndian.PutUint32(drop, 20)
	binary.LittleEndian.PutUint32(drop[16:], 1)
	drop = append(drop, utf16Bytes("C:\\Windows\\notepad.exe\x00")...)
	cases = append(cases, struct {
		name string
		clip history.Snapshot
	}{"file-list", history.Snapshot{Formats: []history.Format{{ID: 15, Data: drop}}}})
	dib := make([]byte, 40+16)
	binary.LittleEndian.PutUint32(dib, 40)
	binary.LittleEndian.PutUint32(dib[4:], 2)
	binary.LittleEndian.PutUint32(dib[8:], 2)
	binary.LittleEndian.PutUint16(dib[12:], 1)
	binary.LittleEndian.PutUint16(dib[14:], 32)
	binary.LittleEndian.PutUint32(dib[20:], 16)
	copy(dib[40:], []byte{0, 0, 255, 255, 0, 255, 0, 255, 255, 0, 0, 255, 255, 255, 255, 255})
	cases = append(cases, struct {
		name string
		clip history.Snapshot
	}{"dib-image", history.Snapshot{Formats: []history.Format{{ID: 8, Data: dib}}}})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seq, err := Write(hwnd, tc.clip)
			if err != nil {
				t.Fatal(err)
			}
			if seq == 0 {
				t.Fatal("missing clipboard sequence")
			}
			got, _, err := Read(hwnd, 64<<20)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.clip.Formats {
				found := false
				for _, f := range got.Formats {
					if (want.Name != "" && f.Name == want.Name) || (want.Name == "" && f.ID == want.ID) {
						found = true
						if !bytes.Equal(f.Data, want.Data) {
							t.Errorf("format %s raw bytes changed: got %d want %d", history.FormatKey(want), len(f.Data), len(want.Data))
						}
					}
				}
				if !found {
					t.Errorf("format missing: %s", history.FormatKey(want))
				}
			}
		})
	}
	t.Run("session-stays-on-opening-thread", func(t *testing.T) {
		release, err := acquireClipboard(0)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		thread := windows.GetCurrentThreadId()
		for i := 0; i < 2000; i++ {
			runtime.Gosched()
			if windows.GetCurrentThreadId() != thread {
				t.Fatal("clipboard session migrated to another OS thread")
			}
		}
	})
	t.Run("excluded-by-source", func(t *testing.T) {
		_, err := Write(hwnd, history.Snapshot{Formats: []history.Format{{ID: 13, Data: utf16Bytes("do not save")}, {Name: "CanIncludeInClipboardHistory", Data: []byte{0, 0, 0, 0}}}})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err = Read(hwnd, 64<<20); !errors.Is(err, ErrExcluded) {
			t.Fatal("source exclusion ignored", err)
		}
	})
	t.Run("oversized-format-is-reported", func(t *testing.T) {
		_, err := Write(hwnd, history.Snapshot{Formats: []history.Format{{Name: "KOL.Large", Data: make([]byte, 128)}}})
		if err != nil {
			t.Fatal(err)
		}
		clip, _, err := Read(hwnd, 16)
		if err != nil || len(clip.Omitted) == 0 || len(clip.Formats) != 0 {
			t.Fatal("oversized data was not reported", err)
		}
	})
}

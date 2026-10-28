package history

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

func textClip(text string) Snapshot {
	u := utf16.Encode([]rune(text + "\x00"))
	b := make([]byte, len(u)*2)
	for i, v := range u {
		binary.LittleEndian.PutUint16(b[i*2:], v)
	}
	return Snapshot{Formats: []Format{{ID: 13, Data: b}}}
}

func openTest(t *testing.T) (*Store, string, []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "history.db")
	key := bytes.Repeat([]byte{0x42}, 32)
	s, err := Open(path, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, path, key
}

func TestDuplicateMovesToTopWithoutChangingWhitespace(t *testing.T) {
	s, _, _ := openTest(t)
	a, err := s.Add(textClip("  première copie\r\n\t "), time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = s.Add(textClip("seconde"), time.Unix(2, 0))
	b, err := s.Add(textClip("  première copie\r\n\t "), time.Unix(3, 0))
	if err != nil {
		t.Fatal(err)
	}
	list := s.List("", "all")
	if len(list) != 2 || list[0].ID != a.ID || b.Copies != 2 {
		t.Fatalf("incorrect deduplication: %+v", list)
	}
	got, err := s.Get(a.ID)
	if err != nil || !bytes.Equal(got.Formats[0].Data, textClip("  première copie\r\n\t ").Formats[0].Data) {
		t.Fatal("raw whitespace changed", err)
	}
	_, _ = s.Add(textClip("première copie"), time.Unix(4, 0))
	if len(s.List("", "all")) != 3 {
		t.Fatal("trimmed text must be a different entry")
	}
}

func TestFingerprintUsesNamesAndBytesNotFormatOrder(t *testing.T) {
	a := textClip("identique")
	a.Formats = append(a.Formats, Format{ID: 49000, Name: "Rich Text Format", Data: []byte("{\\rtf1 bold}")})
	b := Snapshot{Formats: []Format{{ID: 51000, Name: "Rich Text Format", Data: []byte("{\\rtf1 bold}")}, a.Formats[0]}}
	if Fingerprint(a) != Fingerprint(b) {
		t.Fatal("registered IDs and enumeration order must not change identity")
	}
	b.Formats[0].Data = []byte("{\\rtf1 italic}")
	if Fingerprint(a) == Fingerprint(b) {
		t.Fatal("different rich formatting must remain distinct")
	}
}

func TestPersistenceSearchAndCiphertext(t *testing.T) {
	s, path, key := openTest(t)
	entry, err := s.Add(textClip("Secret searchable phrase — Éléphant 🐘"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Pin(entry.ID, true); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	onDisk, _ := os.ReadFile(path)
	if bytes.Contains(onDisk, []byte("Secret searchable")) {
		t.Fatal("plaintext leaked to disk")
	}
	s2, err := Open(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	got := s2.List("ÉLÉPHANT", "pinned")
	if len(got) != 1 || got[0].ID != entry.ID {
		t.Fatalf("search or persistence broken: %+v", got)
	}
	if !strings.Contains(got[0].Text, "🐘") {
		t.Fatal("Unicode changed")
	}
}

func TestPinClearAndDelete(t *testing.T) {
	s, _, _ := openTest(t)
	a, _ := s.Add(textClip("à garder"), time.Now())
	_, _ = s.Add(textClip("à effacer"), time.Now())
	if err := s.Pin(a.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
	if got := s.List("", "all"); len(got) != 1 || !got[0].Pinned {
		t.Fatal("clear must keep pinned entries")
	}
	if err := s.Delete(a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(a.ID); err == nil {
		t.Fatal("deleted entry still exists")
	}
	if len(s.List("", "all")) != 0 {
		t.Fatal("deleted summary still exists")
	}
}

func TestEmptyCaptureRejectedAndWhitespaceAccepted(t *testing.T) {
	s, _, _ := openTest(t)
	if _, err := s.Add(Snapshot{}, time.Now()); err == nil {
		t.Fatal("empty capture accepted")
	}
	e, err := s.Add(textClip("   \t\r\n"), time.Now())
	if err != nil || e.Text != "   \t\r\n" {
		t.Fatal("whitespace-only text must be preserved", err)
	}
}

func TestPartialAndSourceSearch(t *testing.T) {
	s, _, _ := openTest(t)
	clip := textClip("hello")
	clip.Source = "éditeur.exe"
	clip.Omitted = []string{"VirtualFile: flux non sérialisable"}
	_, err := s.Add(clip, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	got := s.List("ÉDITEUR", "text")
	if len(got) != 1 || len(got[0].Omitted) != 1 {
		t.Fatal("partial information lost")
	}
}

func TestCipherAuthenticatesAndWrongKeyDoesNotResetStore(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	c, err := NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := c.Seal([]byte("confidential"))
	if err != nil {
		t.Fatal(err)
	}
	sealed[len(sealed)-1] ^= 1
	if _, err := c.Open(sealed); err == nil {
		t.Fatal("modified ciphertext accepted")
	}
	if _, err := c.Open(nil); err == nil {
		t.Fatal("truncated ciphertext accepted")
	}
	s, path, _ := openTest(t)
	_, _ = s.Add(textClip("do not reset"), time.Now())
	_ = s.Close()
	if bad, err := Open(path, key); err == nil {
		bad.Close()
		t.Fatal("wrong key accepted")
	}
	if good, err := Open(path, bytes.Repeat([]byte{0x42}, 32)); err != nil {
		t.Fatal("original database damaged", err)
	} else {
		good.Close()
	}
}

func TestConcurrentCopiesAndSearch(t *testing.T) {
	s, _, _ := openTest(t)
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 10; j++ {
				_, err := s.Add(textClip("concurrent"), time.Now())
				if err != nil {
					t.Error(err)
				}
				_ = s.List("con", "all")
			}
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
	if list := s.List("", "all"); len(list) != 1 || list[0].Copies != 80 {
		t.Fatalf("lost updates: %+v", list)
	}
}

func TestFileListPreviewAndLinkClassification(t *testing.T) {
	u := textClip("C:\\Images\\photo.png\x00C:\\Docs\\idée.txt\x00").Formats[0].Data
	drop := make([]byte, 20)
	binary.LittleEndian.PutUint32(drop, 20)
	binary.LittleEndian.PutUint32(drop[16:], 1)
	s := Snapshot{Formats: []Format{{ID: 15, Data: append(drop, u...)}}}
	e := Summarize(s)
	if e.Kind != "files" || len(e.Files) != 2 || e.Files[1] != "C:\\Docs\\idée.txt" {
		t.Fatalf("bad DROPFILES: %+v", e)
	}
	if e := Summarize(textClip("https://example.com/path?a=1")); e.Kind != "link" {
		t.Fatalf("URL not recognized: %+v", e)
	}
	if e := Summarize(Snapshot{Formats: []Format{{ID: 15, Data: []byte{1, 2}}}}); e.Kind != "data" {
		t.Fatal("malformed DROPFILES accepted")
	}
}

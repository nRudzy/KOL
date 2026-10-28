package history

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Format holds a serialized clipboard representation. Registered formats use
// their name as identity; Windows assigns their numeric ID anew each session.
type Format struct {
	ID   uint32 `json:"id"`
	Name string `json:"name,omitempty"`
	Data []byte `json:"data"`
	Kind string `json:"kind,omitempty"` // "emf" for serialized GDI metafile bits
}

type Snapshot struct {
	Formats []Format `json:"formats"`
	Omitted []string `json:"omitted,omitempty"`
	Source  string   `json:"source,omitempty"`
	Owner   uintptr  `json:"-"` // captured HWND, valid only for this live read
}

type Entry struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	Title       string    `json:"title"`
	Text        string    `json:"text"`
	Files       []string  `json:"files,omitempty"`
	Preview     string    `json:"preview,omitempty"`
	Source      string    `json:"source,omitempty"`
	FormatNames []string  `json:"formatNames"`
	Omitted     []string  `json:"omitted,omitempty"`
	Size        int64     `json:"size"`
	FirstAt     time.Time `json:"firstAt"`
	LastAt      time.Time `json:"lastAt"`
	Copies      uint64    `json:"copies"`
	Pinned      bool      `json:"pinned"`
	Order       uint64    `json:"order"`
}

func FormatKey(f Format) string {
	if f.Name != "" {
		return "registered:" + f.Name
	}
	return "standard:" + strconv.FormatUint(uint64(f.ID), 10)
}

// Transport-only shell hints are neither content identity nor replayable moves.
func TransportFormat(name string) bool {
	switch strings.ToLower(name) {
	case "preferred dropeffect", "performed dropeffect", "logical performed dropeffect", "pastesucceeded", "inshelldragloop", "dragcontext", "dragimagebits", "isShowingLayered", "isshowinglayered", "dragwindow":
		return true
	}
	return false
}

func Fingerprint(s Snapshot) string {
	formats := append([]Format(nil), s.Formats...)
	sort.Slice(formats, func(i, j int) bool { return FormatKey(formats[i]) < FormatKey(formats[j]) })
	h := sha256.New()
	for _, f := range formats {
		if TransportFormat(f.Name) {
			continue
		}
		key := FormatKey(f)
		_ = binary.Write(h, binary.LittleEndian, uint64(len(key)))
		_, _ = h.Write([]byte(key))
		_ = binary.Write(h, binary.LittleEndian, uint64(len(f.Data)))
		_, _ = h.Write(f.Data)
	}
	return hex.EncodeToString(h.Sum(nil))
}

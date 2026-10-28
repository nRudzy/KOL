package history

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"net/url"
	"strings"
	"unicode/utf16"

	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
)

func UTF16Text(data []byte) string {
	u := make([]uint16, 0, len(data)/2)
	for i := 0; i+1 < len(data); i += 2 {
		v := binary.LittleEndian.Uint16(data[i:])
		if v == 0 {
			break
		}
		u = append(u, v)
	}
	return string(utf16.Decode(u))
}

func FilePaths(data []byte) []string {
	if len(data) < 20 {
		return nil
	}
	offset := int(binary.LittleEndian.Uint32(data))
	if offset < 20 || offset >= len(data) {
		return nil
	}
	wide := binary.LittleEndian.Uint32(data[16:]) != 0
	var paths []string
	for offset < len(data) {
		start := offset
		if wide {
			for offset+1 < len(data) && binary.LittleEndian.Uint16(data[offset:]) != 0 {
				offset += 2
			}
			if offset == start || offset+1 >= len(data) {
				break
			}
			paths = append(paths, UTF16Text(data[start:offset+2]))
			offset += 2
		} else {
			for offset < len(data) && data[offset] != 0 {
				offset++
			}
			if offset == start || offset >= len(data) {
				break
			}
			paths = append(paths, string(data[start:offset]))
			offset++
		}
	}
	return paths
}

func Summarize(s Snapshot) Entry {
	e := Entry{ID: Fingerprint(s), Kind: "data", Title: "Contenu copié", Source: s.Source, Omitted: append([]string(nil), s.Omitted...)}
	var imageData []byte
	for _, f := range s.Formats {
		e.Size += int64(len(f.Data))
		name := f.Name
		if name == "" {
			name = fmt.Sprintf("CF_%d", f.ID)
		}
		e.FormatNames = append(e.FormatNames, name)
		if f.ID == 13 {
			e.Text = UTF16Text(f.Data)
		}
		if f.ID == 15 {
			e.Files = FilePaths(f.Data)
		}
		if imageData == nil {
			switch strings.ToLower(f.Name) {
			case "png", "image/png", "jfif", "image/jpeg":
				imageData = f.Data
			}
		}
	}
	if imageData == nil {
		for _, f := range s.Formats {
			if f.ID == 8 || f.ID == 17 {
				imageData = dibBitmap(f.Data)
				if imageData != nil {
					break
				}
			}
		}
	}
	if imageData != nil {
		e.Kind = "image"
		e.Title = "Image copiée"
		e.Preview = thumbnail(imageData)
	}
	if len(e.Files) > 0 {
		e.Kind = "files"
		parts := strings.Split(strings.ReplaceAll(e.Files[0], "\\", "/"), "/")
		e.Title = parts[len(parts)-1]
		if len(e.Files) > 1 {
			e.Title += fmt.Sprintf(" et %d autre(s)", len(e.Files)-1)
		}
	} else if e.Kind != "image" && e.Text != "" {
		e.Kind = "text"
		for _, line := range strings.Split(e.Text, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				e.Title = line
				break
			}
		}
		if strings.TrimSpace(e.Text) == "" {
			e.Title = "Espaces et retours à la ligne"
		}
		if parsed, err := url.Parse(strings.TrimSpace(e.Text)); err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Host != "" && !strings.ContainsAny(strings.TrimSpace(e.Text), "\r\n ") {
			e.Kind = "link"
		}
	}
	if runes := []rune(e.Title); len(runes) > 150 {
		e.Title = string(runes[:150]) + "…"
	}
	return e
}

// Previews are derived data; the untouched DIB is the stored clipboard payload.
func dibBitmap(dib []byte) []byte {
	if len(dib) < 40 {
		return nil
	}
	header := uint64(binary.LittleEndian.Uint32(dib))
	if header < 40 || header > uint64(len(dib)) {
		return nil
	}
	bpp := binary.LittleEndian.Uint16(dib[14:])
	colors := uint64(binary.LittleEndian.Uint32(dib[32:]))
	if colors == 0 && bpp <= 8 {
		colors = 1 << bpp
	}
	offset := uint64(14) + header + colors*4
	if header == 40 && binary.LittleEndian.Uint32(dib[16:]) == 3 {
		offset += 12
	}
	if offset > uint64(len(dib))+14 {
		return nil
	}
	bmp := make([]byte, 14+len(dib))
	copy(bmp, "BM")
	binary.LittleEndian.PutUint32(bmp[2:], uint32(len(bmp)))
	binary.LittleEndian.PutUint32(bmp[10:], uint32(offset))
	copy(bmp[14:], dib)
	return bmp
}

func thumbnail(data []byte) string {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 32_000_000 {
		return ""
	}
	im, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return ""
	}
	w, h := config.Width, config.Height
	if w > 640 || h > 480 {
		scale := min(640.0/float64(w), 480.0/float64(h))
		w = max(1, int(float64(w)*scale))
		h = max(1, int(float64(h)*scale))
	}
	thumb := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.BiLinear.Scale(thumb, thumb.Bounds(), im, im.Bounds(), draw.Src, nil)
	var out bytes.Buffer
	if png.Encode(&out, thumb) != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(out.Bytes())
}

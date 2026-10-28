// Command icon derives Windows icon sizes from the generated production logo.
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"os"

	"golang.org/x/image/draw"
)

func main() {
	f, err := os.Open("assets/logo.png")
	must(err)
	im, err := png.Decode(f)
	must(err)
	f.Close()
	b := im.Bounds()
	box := image.Rectangle{Min: b.Max, Max: b.Min}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := im.At(x, y).RGBA()
			if a > 0x1000 {
				box.Min.X = min(box.Min.X, x)
				box.Min.Y = min(box.Min.Y, y)
				box.Max.X = max(box.Max.X, x+1)
				box.Max.Y = max(box.Max.Y, y+1)
			}
		}
	}
	side := max(box.Dx(), box.Dy())
	pad := side / 14
	side += pad * 2
	canvas := image.NewNRGBA(image.Rect(0, 0, side, side))
	offset := image.Pt((side-box.Dx())/2, (side-box.Dy())/2)
	draw.Draw(canvas, image.Rectangle{Min: offset, Max: offset.Add(box.Size())}, im, box.Min, draw.Src)
	var payloads [][]byte
	var sizes = []int{16, 20, 24, 32, 40, 48, 64, 128, 256}
	for _, size := range sizes {
		thumb := image.NewNRGBA(image.Rect(0, 0, size, size))
		draw.CatmullRom.Scale(thumb, thumb.Bounds(), canvas, canvas.Bounds(), draw.Src, nil)
		var pngData bytes.Buffer
		must(png.Encode(&pngData, thumb))
		payloads = append(payloads, pngData.Bytes())
	}
	var ico bytes.Buffer
	must(binary.Write(&ico, binary.LittleEndian, []uint16{0, 1, uint16(len(sizes))}))
	offsetBytes := 6 + 16*len(sizes)
	for i, size := range sizes {
		dimension := byte(size)
		ico.Write([]byte{dimension, dimension, 0, 0})
		must(binary.Write(&ico, binary.LittleEndian, []uint16{1, 32}))
		must(binary.Write(&ico, binary.LittleEndian, uint32(len(payloads[i]))))
		must(binary.Write(&ico, binary.LittleEndian, uint32(offsetBytes)))
		offsetBytes += len(payloads[i])
	}
	for _, data := range payloads {
		ico.Write(data)
	}
	must(os.WriteFile("assets/kol.ico", ico.Bytes(), 0644))
	mark := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	draw.CatmullRom.Scale(mark, mark.Bounds(), canvas, canvas.Bounds(), draw.Src, nil)
	markFile, err := os.Create("assets/mark.png")
	must(err)
	must(png.Encode(markFile, mark))
	must(markFile.Close())
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}

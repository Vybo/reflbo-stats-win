// Package icon draws the tray's icon (spec §8): a panel with bars, a warning mark while HWiNFO isn't "ok", a dot
// while a board is connected. PNG for macOS's tray, ICO (a PNG inside) for Windows'.
package icon

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
)

type State int

const (
	Normal State = iota
	Warning
	Connected
)

var (
	panel = color.RGBA{0x2f, 0x5d, 0x50, 0xff}
	ink   = color.RGBA{0xf4, 0xf5, 0xf2, 0xff}
	amber = color.RGBA{0xf2, 0xa9, 0x00, 0xff}
	green = color.RGBA{0x3d, 0xd6, 0x7a, 0xff}
)

func fill(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func disc(img *image.RGBA, cx, cy, r int, c color.RGBA) {
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			if (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func draw(s State) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	fill(img, 2, 4, 30, 28, panel) // the panel, its corners left clear
	for x := 2; x < 30; x += 27 {
		img.SetRGBA(x, 4, color.RGBA{})
		img.SetRGBA(x, 27, color.RGBA{})
	}
	for i, h := range []int{6, 12, 9, 16} { // bars, as a chart
		x := 6 + i*5
		fill(img, x, 24-h, x+3, 24, ink)
	}
	switch s {
	case Warning:
		disc(img, 26, 6, 5, amber)
		fill(img, 25, 3, 27, 7, panel)
		fill(img, 25, 8, 27, 10, panel)
	case Connected:
		disc(img, 26, 26, 5, green)
	}
	return img
}

func PNG(s State) []byte {
	var b bytes.Buffer
	png.Encode(&b, draw(s))
	return b.Bytes()
}

// ICO wraps the PNG in an icon file of one image, which Windows reads since Vista.
func ICO(s State) []byte {
	payload := PNG(s)
	var b bytes.Buffer
	le := binary.LittleEndian
	binary.Write(&b, le, [3]uint16{0, 1, 1}) // reserved, type icon, one image
	b.Write([]byte{32, 32, 0, 0})            // width, height, no palette, reserved
	binary.Write(&b, le, [2]uint16{1, 32})   // planes, bits a pixel
	binary.Write(&b, le, [2]uint32{uint32(len(payload)), 22})
	b.Write(payload)
	return b.Bytes()
}

package icon

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"testing"
)

func TestEachStateIsA32pxPNGAndTheyDiffer(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range []State{Normal, Warning, Connected} {
		b := PNG(s)
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatalf("%v: %v", s, err)
		}
		if r := img.Bounds(); r.Dx() != 32 || r.Dy() != 32 {
			t.Fatalf("%v: %v", s, r)
		}
		if seen[string(b)] {
			t.Fatalf("%v looks like another state", s)
		}
		seen[string(b)] = true
	}
}

func TestTheICOWrapsThePNGAsWindowsReadsIt(t *testing.T) {
	ico := ICO(Normal)
	le := binary.LittleEndian
	if le.Uint16(ico[0:]) != 0 || le.Uint16(ico[2:]) != 1 || le.Uint16(ico[4:]) != 1 {
		t.Fatalf("header % x", ico[:6])
	}
	if ico[6] != 32 || ico[7] != 32 || le.Uint16(ico[10:]) != 1 || le.Uint16(ico[12:]) != 32 {
		t.Fatalf("entry % x", ico[6:22])
	}
	size, off := le.Uint32(ico[14:]), le.Uint32(ico[18:])
	payload := PNG(Normal)
	if int(size) != len(payload) || off != 22 || !bytes.Equal(ico[22:], payload) {
		t.Fatalf("size %d off %d", size, off)
	}
}

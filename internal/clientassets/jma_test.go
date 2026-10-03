package clientassets

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// buildJMA assembles a one-sprite archive with one 3x2 frame.
func buildJMA() []byte {
	frame := []byte{1, 2, 0, 0, 0, 3, 0, 0}
	sprite := append([]byte("jm"), make([]byte, 12)...)
	binary.LittleEndian.PutUint32(sprite[10:], 1)
	rec := make([]byte, 40)
	rec[0] = 3
	copy(rec[1:], "a.b")
	binary.LittleEndian.PutUint16(rec[12:], 256)
	binary.LittleEndian.PutUint16(rec[14:], 256)
	binary.LittleEndian.PutUint16(rec[16:], 10)
	binary.LittleEndian.PutUint16(rec[18:], 20)
	binary.LittleEndian.PutUint32(rec[20:], uint32(len(frame)))
	binary.LittleEndian.PutUint32(rec[24:], 14+40+1024)
	binary.LittleEndian.PutUint32(rec[28:], 3)
	binary.LittleEndian.PutUint32(rec[32:], 2)
	sprite = append(sprite, rec...)
	pal := make([]byte, 1024)
	pal[4*3], pal[4*3+1], pal[4*3+2] = 0x10, 0x20, 0x30
	sprite = append(sprite, pal...)
	sprite = append(sprite, frame...)
	archive := append([]byte("ja"), make([]byte, 12)...)
	binary.LittleEndian.PutUint32(archive[10:], 1)
	entry := make([]byte, 28)
	entry[0] = 8
	copy(entry[1:], "1000.jmp")
	binary.LittleEndian.PutUint32(entry[20:], uint32(len(sprite)))
	binary.LittleEndian.PutUint32(entry[24:], 14+28)
	return append(append(archive, entry...), sprite...)
}

type bytesReaderAt []byte

func (b bytesReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(b)) {
		return 0, os.ErrInvalid
	}
	return copy(p, b[off:]), nil
}

func TestSpriteArchiveGolden(t *testing.T) {
	b := buildJMA()
	a, err := OpenSpriteArchive(bytesReaderAt(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	s, err := a.Sprite(a.Find("1000.jmp"))
	if err != nil {
		t.Fatal(err)
	}
	f := s.Frames[0]
	if f.Name != "a.b" || f.X != 10 || f.Y != 20 || f.Width != 3 || f.Height != 2 || f.Stride != 4 ||
		f.Pixels[1] != 2 || f.Pixels[5] != 3 || s.Palette[3] != [3]uint8{0x30, 0x20, 0x10} {
		t.Fatalf("got %+v palette %v", f, s.Palette[3])
	}
}

func TestSpriteArchiveRejectsTruncation(t *testing.T) {
	b := buildJMA()
	if _, err := OpenSpriteArchive(bytesReaderAt(b[:30]), 30); err == nil {
		t.Fatal("a truncated directory should fail")
	}
	if _, err := DecodeSprite(b[42 : len(b)-2]); err == nil {
		t.Fatal("truncated pixels should fail")
	}
}

// TestNativeSpriteArchive opens the installed weapon archive when present.
func TestNativeSpriteArchive(t *testing.T) {
	path := filepath.Join(os.Getenv("WONDERLAND_CLIENT_DATA"), "jma", "001w.jma")
	f, err := os.Open(path)
	if err != nil {
		t.Skip("set WONDERLAND_CLIENT_DATA to the original client directory")
	}
	defer f.Close()
	st, _ := f.Stat()
	a, err := OpenSpriteArchive(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	s, err := a.Sprite(a.Find("1001.jmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Frames) != 64 || s.Frames[0].Width != 8 || s.Frames[0].Height != 36 || s.Frames[0].X != 139 {
		t.Fatalf("frame 0: %+v", s.Frames[0])
	}
}

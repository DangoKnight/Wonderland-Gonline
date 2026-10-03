package clientassets

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// JMA sprite archives (jma\*.jma) as read by the client's archive class
// (aLogin 0x2fdc40-0x302cf8):
//
//	archive  14-byte header, "ja" ... entry count u32 at +10
//	         entries of 28 bytes: name as a length byte and 19 bytes,
//	         u32 size, u32 offset of the sprite (a "jm" block)
//	sprite   14-byte header, "jm" ... frame count u32 at +10
//	         frames of 40 bytes, then a 256-entry BGRA palette
//	frame    name (length byte and 11 bytes), canvas width and height
//	         (u16), anchor x and y (u16), pixel bytes, pixel offset from
//	         the sprite start, width, height (u32), and a u32 the decoder
//	         does not use
//
// Pixels are 8-bit palette indexes, rows top down, each row padded to four
// bytes. Index 0 is transparent.
const (
	jmaHeaderBytes      = 14
	jmaCountOffset      = 10
	jmaEntryBytes       = 28
	jmaEntryNameBytes   = 20
	jmaFrameBytes       = 40
	jmaFrameNameBytes   = 12
	jmaPaletteEntries   = 256
	jmaPaletteBytes     = 4 * jmaPaletteEntries
	jmaMaxEntries       = 1 << 16
	jmaMaxFrames        = 1 << 16
	jmaMaxFrameSide     = 4096
	jmaRowAlignment     = 4
	jmaArchiveSignature = "ja"
	jmaSpriteSignature  = "jm"
)

// SpriteArchive is an opened .jma file.
type SpriteArchive struct {
	r       io.ReaderAt
	size    int64
	Entries []SpriteEntry
	index   map[string]int
}

// SpriteEntry is one directory record.
type SpriteEntry struct {
	Name   string
	Size   uint32
	Offset uint32
}

// Sprite is a decoded "jm" block.
type Sprite struct {
	Frames  []SpriteFrame
	Palette [jmaPaletteEntries][3]uint8 // RGB from the BGRA palette
}

// SpriteFrame is one frame record with its pixels.
type SpriteFrame struct {
	Name                      string
	CanvasWidth, CanvasHeight int
	X, Y                      int // anchor of the bitmap on the canvas
	Width, Height             int
	Stride                    int
	Pixels                    []byte // palette indexes, Stride*Height
	Extra                     uint32
}

var errJMA = errors.New("malformed JMA archive")

// OpenSpriteArchive reads an archive's directory.
func OpenSpriteArchive(r io.ReaderAt, size int64) (*SpriteArchive, error) {
	var h [jmaHeaderBytes]byte
	if _, err := r.ReadAt(h[:], 0); err != nil {
		return nil, err
	}
	if string(h[:2]) != jmaArchiveSignature {
		return nil, fmt.Errorf("%w: signature", errJMA)
	}
	n := binary.LittleEndian.Uint32(h[jmaCountOffset:])
	if n > jmaMaxEntries || int64(jmaHeaderBytes)+int64(n)*jmaEntryBytes > size {
		return nil, fmt.Errorf("%w: entry count %d", errJMA, n)
	}
	dir := make([]byte, int(n)*jmaEntryBytes)
	if _, err := r.ReadAt(dir, jmaHeaderBytes); err != nil {
		return nil, err
	}
	a := &SpriteArchive{r: r, size: size, index: map[string]int{}}
	for i := range int(n) {
		rec := dir[i*jmaEntryBytes:][:jmaEntryBytes]
		l := min(int(rec[0]), jmaEntryNameBytes-1)
		e := SpriteEntry{
			Name:   string(rec[1 : 1+l]),
			Size:   binary.LittleEndian.Uint32(rec[jmaEntryNameBytes:]),
			Offset: binary.LittleEndian.Uint32(rec[jmaEntryNameBytes+4:]),
		}
		if int64(e.Offset)+int64(e.Size) > size {
			return nil, fmt.Errorf("%w: entry %q outside the file", errJMA, e.Name)
		}
		a.index[e.Name] = i
		a.Entries = append(a.Entries, e)
	}
	return a, nil
}

// Find returns the index of a named sprite, or -1.
func (a *SpriteArchive) Find(name string) int {
	if i, ok := a.index[name]; ok {
		return i
	}
	return -1
}

// Sprite decodes entry i.
func (a *SpriteArchive) Sprite(i int) (*Sprite, error) {
	if i < 0 || i >= len(a.Entries) {
		return nil, fmt.Errorf("%w: entry %d", errJMA, i)
	}
	e := a.Entries[i]
	b := make([]byte, e.Size)
	if _, err := a.r.ReadAt(b, int64(e.Offset)); err != nil {
		return nil, err
	}
	return DecodeSprite(b)
}

// DecodeSprite decodes a "jm" block.
func DecodeSprite(b []byte) (*Sprite, error) {
	if len(b) < jmaHeaderBytes || string(b[:2]) != jmaSpriteSignature {
		return nil, fmt.Errorf("%w: sprite signature", errJMA)
	}
	n := binary.LittleEndian.Uint32(b[jmaCountOffset:])
	palAt := jmaHeaderBytes + int64(n)*jmaFrameBytes
	if n > jmaMaxFrames || palAt+jmaPaletteBytes > int64(len(b)) {
		return nil, fmt.Errorf("%w: frame count %d", errJMA, n)
	}
	s := &Sprite{}
	pal := b[palAt:][:jmaPaletteBytes]
	for i := range s.Palette {
		s.Palette[i] = [3]uint8{pal[4*i+2], pal[4*i+1], pal[4*i]}
	}
	for i := range int(n) {
		rec := b[jmaHeaderBytes+i*jmaFrameBytes:][:jmaFrameBytes]
		l := min(int(rec[0]), jmaFrameNameBytes-1)
		f := SpriteFrame{
			Name:         string(rec[1 : 1+l]),
			CanvasWidth:  int(binary.LittleEndian.Uint16(rec[12:])),
			CanvasHeight: int(binary.LittleEndian.Uint16(rec[14:])),
			X:            int(binary.LittleEndian.Uint16(rec[16:])),
			Y:            int(binary.LittleEndian.Uint16(rec[18:])),
			Width:        int(binary.LittleEndian.Uint32(rec[28:])),
			Height:       int(binary.LittleEndian.Uint32(rec[32:])),
			Extra:        binary.LittleEndian.Uint32(rec[36:]),
		}
		size := int64(binary.LittleEndian.Uint32(rec[20:]))
		off := int64(binary.LittleEndian.Uint32(rec[24:]))
		if f.Width > jmaMaxFrameSide || f.Height > jmaMaxFrameSide {
			return nil, fmt.Errorf("%w: frame %q size", errJMA, f.Name)
		}
		f.Stride = (f.Width + jmaRowAlignment - 1) / jmaRowAlignment * jmaRowAlignment
		need := int64(f.Stride) * int64(f.Height)
		if need > size || off+need > int64(len(b)) {
			return nil, fmt.Errorf("%w: frame %q pixels", errJMA, f.Name)
		}
		f.Pixels = b[off : off+need]
		s.Frames = append(s.Frames, f)
	}
	return s, nil
}

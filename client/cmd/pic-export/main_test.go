package main

import (
	"encoding/binary"
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestExportArchiveAndLooseBitmap(t *testing.T) {
	// Independent 1x1, 24-bit BMP containing RGB(12,34,56), padded to four bytes.
	bmp := make([]byte, 58)
	copy(bmp, "BM")
	binary.LittleEndian.PutUint32(bmp[2:], 58)
	binary.LittleEndian.PutUint32(bmp[10:], 54)
	binary.LittleEndian.PutUint32(bmp[14:], 40)
	binary.LittleEndian.PutUint32(bmp[18:], 1)
	binary.LittleEndian.PutUint32(bmp[22:], 1)
	binary.LittleEndian.PutUint16(bmp[26:], 1)
	binary.LittleEndian.PutUint16(bmp[28:], 24)
	copy(bmp[54:], []byte{56, 34, 12, 0})
	archive := make([]byte, 34)
	binary.LittleEndian.PutUint16(archive, 1)
	archive[2] = 8
	copy(archive[3:], "1001.bmp")
	binary.LittleEndian.PutUint32(archive[26:], 34)
	binary.LittleEndian.PutUint32(archive[30:], uint32(len(bmp)))
	archive = append(archive, bmp...)
	root := t.TempDir()
	input := filepath.Join(root, "input")
	output := filepath.Join(root, "pictures")
	if err := os.Mkdir(input, 0755); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"item.BMg": archive, "Loading.BMP": bmp, "1.Bls": []byte("\r\n1001\r\n"), "Thumbs.db": {0}} {
		if err := os.WriteFile(filepath.Join(input, name), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := export(input, output); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(output, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc manifest
	if err = json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Files) != 4 {
		t.Fatal("source census", len(doc.Files))
	}
	count := 0
	for _, s := range doc.Files {
		for _, p := range s.Pictures {
			count++
			f, err := os.Open(filepath.Join(output, p.PNG))
			if err != nil {
				t.Fatal(err)
			}
			m, err := png.Decode(f)
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			r, g, b, a := m.At(0, 0).RGBA()
			if r != 12*257 || g != 34*257 || b != 56*257 || a != 65535 {
				t.Fatal("pixels changed", r, g, b, a)
			}
		}
		if s.Kind == "resource-list" && (len(s.Lines) != 3 || s.Lines[1] != "1001") {
			t.Fatal("BLS lines changed", s.Lines)
		}
	}
	if count != 2 {
		t.Fatal("image census", count)
	}
	if err := export(input, output); err == nil {
		t.Fatal("existing edits may be overwritten")
	}
}

func TestUndefinedPaletteColor(t *testing.T) {
	// One defined red entry; the pixel uses absent index 255.
	raw := make([]byte, 62)
	copy(raw, "BM")
	binary.LittleEndian.PutUint32(raw[2:], 62)
	binary.LittleEndian.PutUint32(raw[10:], 58)
	binary.LittleEndian.PutUint32(raw[14:], 40)
	binary.LittleEndian.PutUint32(raw[18:], 1)
	binary.LittleEndian.PutUint32(raw[22:], 1)
	binary.LittleEndian.PutUint16(raw[26:], 1)
	binary.LittleEndian.PutUint16(raw[28:], 8)
	binary.LittleEndian.PutUint32(raw[46:], 1)
	raw[56] = 255
	raw[58] = 255
	root := t.TempDir()
	p, err := writePicture(root, "", "missing.bmp", raw, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Note == "" {
		t.Fatal("fallback not documented")
	}
	f, err := os.Open(filepath.Join(root, p.PNG))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	m, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, a := m.At(0, 0).RGBA()
	if r != 0 || g != 0 || b != 0 || a != 65535 {
		t.Fatal("fallback color", r, g, b, a)
	}
	if len(raw) != 62 || raw[46] != 1 {
		t.Fatal("source bytes modified")
	}
}

func TestCoreBitmap(t *testing.T) {
	// Independent OS/2 1x1 24-bit BMP: BGR(56,34,12).
	raw := make([]byte, 30)
	copy(raw, "BM")
	binary.LittleEndian.PutUint32(raw[2:], 30)
	binary.LittleEndian.PutUint32(raw[10:], 26)
	binary.LittleEndian.PutUint32(raw[14:], 12)
	binary.LittleEndian.PutUint16(raw[18:], 1)
	binary.LittleEndian.PutUint16(raw[20:], 1)
	binary.LittleEndian.PutUint16(raw[22:], 1)
	binary.LittleEndian.PutUint16(raw[24:], 24)
	copy(raw[26:], []byte{56, 34, 12, 0})
	fixed, err := expandCoreBitmap(raw)
	if err != nil {
		t.Fatal(err)
	}
	m, err := decode(fixed)
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, a := m.At(0, 0).RGBA()
	if r != 12*257 || g != 34*257 || b != 56*257 || a != 65535 {
		t.Fatal("core pixels changed", r, g, b, a)
	}
}

func TestRGB555Orientation(t *testing.T) {
	raw := make([]byte, 62)
	copy(raw, "BM")
	binary.LittleEndian.PutUint32(raw[10:], 54)
	binary.LittleEndian.PutUint32(raw[14:], 40)
	binary.LittleEndian.PutUint32(raw[18:], 1)
	binary.LittleEndian.PutUint32(raw[22:], 2)
	binary.LittleEndian.PutUint16(raw[28:], 16)
	// Stored bottom-up: green bottom, red top, each with two padding bytes.
	copy(raw[54:], []byte{0xe0, 0x03, 0, 0, 0, 0x7c, 0, 0})
	m, err := decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := m.At(0, 0).RGBA()
	if r != 65535 || g != 0 || b != 0 {
		t.Fatal("top pixel", r, g, b)
	}
	r, g, b, _ = m.At(0, 1).RGBA()
	if r != 0 || g != 65535 || b != 0 {
		t.Fatal("bottom pixel", r, g, b)
	}
	if _, err = decode(raw[:61]); err == nil {
		t.Fatal("truncated pixels accepted")
	}
}

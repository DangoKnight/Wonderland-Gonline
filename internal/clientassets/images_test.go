package clientassets

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestImageArchiveBounds(t *testing.T) {
	b := make([]byte, 38)
	binary.LittleEndian.PutUint16(b, 1)
	b[2] = 5
	copy(b[3:], "a.bmp")
	binary.LittleEndian.PutUint32(b[26:], 34)
	binary.LittleEndian.PutUint32(b[30:], 4)
	copy(b[34:], "BMxx")
	a, err := ReadImageArchive(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	if i, ok := a.Find("A.BMP"); !ok || i != 0 {
		t.Fatal(i, ok)
	}
	got, err := a.Read(0)
	if err != nil || string(got) != "BMxx" {
		t.Fatal(got, err)
	}
	for n := 0; n < 34; n++ {
		if _, err := ReadImageArchive(bytes.NewReader(b[:n]), int64(n)); err == nil {
			t.Fatal("truncated directory", n)
		}
	}
	for _, offset := range []uint32{0, 33, 35, 0xffffffff} {
		bad := append([]byte(nil), b...)
		binary.LittleEndian.PutUint32(bad[26:], offset)
		if _, err := ReadImageArchive(bytes.NewReader(bad), int64(len(bad))); err == nil {
			t.Fatal("invalid offset", offset)
		}
	}
	b[3] = '\\'
	if _, err := ReadImageArchive(bytes.NewReader(b), int64(len(b))); err == nil {
		t.Fatal("path accepted as resource")
	}
}

func TestNativeImageArchiveDirectory(t *testing.T) {
	dir := os.Getenv("WONDERLAND_CLIENT_DATA")
	if dir == "" {
		t.Skip("set WONDERLAND_CLIENT_DATA to original client directory")
	}
	for _, tc := range []struct {
		name      string
		count     int
		signature []byte
	}{
		{"images.BMg", 269, []byte("BM")}, {"item.BMg", 2190, []byte("BM")}, {"map.JMG", 326, []byte{0xff, 0xd8}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := os.Open(filepath.Join(dir, "pic", tc.name))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			stat, err := f.Stat()
			if err != nil {
				t.Fatal(err)
			}
			a, err := ReadImageArchive(f, stat.Size())
			if err != nil {
				t.Fatal(err)
			}
			if len(a.Entries) != tc.count {
				t.Fatal("directory census", len(a.Entries))
			}
			for i := range a.Entries {
				b, err := a.Read(i)
				if err != nil || !bytes.HasPrefix(b, tc.signature) {
					t.Fatal(a.Entries[i].Name, err)
				}
			}
		})
	}
}

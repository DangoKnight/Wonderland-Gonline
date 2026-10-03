package protocol

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

type fragmentReader struct{ io.Reader }

func (r fragmentReader) Read(b []byte) (int, error) {
	if len(b) > 1 {
		b = b[:1]
	}
	return r.Reader.Read(b)
}
func TestGoldenFrame(t *testing.T) {
	// C# SendPacket(0x44f4), Pack8(1), Pack8(1), Pack8(1), then XOR all bytes.
	want := []byte{0x59, 0xe9, 0xae, 0xad, 0xac, 0xac, 0xac}
	got, e := Encode([]byte{1, 1, 1})
	if e != nil || !bytes.Equal(got, want) {
		t.Fatalf("%x %v", got, e)
	}
	stream := fragmentReader{bytes.NewReader(append(want, want...))}
	for range 2 {
		b, e := Read(stream)
		if e != nil || !bytes.Equal(b, []byte{1, 1, 1}) {
			t.Fatalf("%x %v", b, e)
		}
	}
}
func TestMalformed(t *testing.T) {
	for _, b := range [][]byte{{0, 0, 0, 0}, {0x59, 0xe9, 0xad, 0xad}, {0x59, 0xe9, 0xaf, 0xad, 0xac}} {
		if _, e := Read(bytes.NewReader(b)); e == nil {
			t.Fatalf("accepted %x", b)
		}
	}
	r := NewReader([]byte{4, 'a'})
	_ = r.String()
	if !errors.Is(r.Err(), io.ErrUnexpectedEOF) {
		t.Fatal(r.Err())
	}
}
func FuzzRead(f *testing.F) {
	b, _ := Encode([]byte{63, 4, 4, 't', 'e', 's', 't'})
	f.Add(b)
	f.Fuzz(func(t *testing.T, b []byte) {
		p, e := Read(bytes.NewReader(b))
		if e == nil {
			wire, e := Encode(p)
			if e != nil {
				t.Fatal(e)
			}
			again, e := Read(bytes.NewReader(wire))
			if e != nil || !bytes.Equal(p, again) {
				t.Fatal("roundtrip")
			}
		}
	})
}

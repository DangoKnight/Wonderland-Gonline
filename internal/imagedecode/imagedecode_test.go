package imagedecode

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"testing"
)

type boundedRead struct {
	reader    *bytes.Reader
	remaining int
}

func (r *boundedRead) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, errors.New("decoder consumed unrelated trailing data")
	}
	if len(p) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.reader.Read(p)
	r.remaining -= n
	return n, err
}
func TestDesktopDecodeRemainsStreaming(t *testing.T) {
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewNRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	data.Write(make([]byte, 64<<10))
	r := &boundedRead{reader: bytes.NewReader(data.Bytes()), remaining: 4096}
	got, err := Decode(r)
	if err != nil || got.Bounds() != image.Rect(0, 0, 2, 3) {
		t.Fatal("desktop decode eagerly consumed entire input", err)
	}
}
func TestOptionalDecodeHookReceivesEncodedBytes(t *testing.T) {
	want := []byte("encoded browser artwork")
	Hook = func(raw []byte) (image.Image, error) {
		if !bytes.Equal(raw, want) {
			t.Fatal("encoded input changed")
		}
		return image.NewNRGBA(image.Rect(0, 0, 3, 2)), nil
	}
	defer func() { Hook = nil }()
	for _, decode := range []func(*bytes.Reader) (image.Image, error){func(r *bytes.Reader) (image.Image, error) { return Decode(r) }, func(r *bytes.Reader) (image.Image, error) { return DecodePNG(r) }} {
		got, err := decode(bytes.NewReader(want))
		if err != nil || got.Bounds() != image.Rect(0, 0, 3, 2) {
			t.Fatal(got, err)
		}
	}
}

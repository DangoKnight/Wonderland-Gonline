// Package imagedecode decodes picture files. The desktop build uses the
// standard library. The browser build installs a Hook that lets the page
// decode with the browser's own (hardware-assisted) image decoder, which is far
// faster than decoding PNG in WebAssembly.
package imagedecode

import (
	"bytes"
	"image"
	"image/png"
	"io"
)

// Hook, when set, decodes a whole encoded image. Nil means the standard library.
var Hook func(raw []byte) (image.Image, error)

// Decode decodes one encoded picture (PNG, JPEG, ...) from r.
func Decode(r io.Reader) (image.Image, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if Hook != nil {
		return Hook(raw)
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	return img, err
}

// DecodePNG decodes a PNG from r, through the Hook when one is installed.
func DecodePNG(r io.Reader) (image.Image, error) {
	if Hook == nil {
		return png.Decode(r)
	}
	return Decode(r)
}

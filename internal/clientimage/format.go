// Package clientimage stores editable PNGs as lossless, independently readable
// image pages. Palette colors and native RGB565 values are prepared at build time.
package clientimage

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/binary"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
)

const (
	Version                = 1
	TileSide               = 256
	SmallImageSide         = 128
	DescriptorSuffix       = ".wli"
	ChunkSuffix            = ".wlt"
	descriptorMagic        = "WLIM\x01\x00\x00\x00"
	chunkMagic             = "WLIT\x01\x00\x00\x00"
	maximumImageSide       = 1 << 16
	maximumImagePixels     = 256 << 20
	maximumDescriptorBytes = 4 << 20
	maximumPaletteColors   = 256
	pixelBytes             = 4
	colorTableBytes        = 8  // RGBA plus prepared plain/keyed RGB565.
	chunkHeaderBytes       = 14 // Magic, width, height, palette length.
	maximumChunkBytes      = chunkHeaderBytes + maximumPaletteColors*colorTableBytes + TileSide*TileSide*pixelBytes
)

const (
	nativeGreenKey      = uint16(0x07e0)
	nativeGreenResidual = byte(0x18)
	nearBlackGreenBit   = uint16(1 << 5)
)

type Tile struct {
	Rect image.Rectangle // Position within the image's unchanged original canvas.
	Page image.Rectangle // Position within a shared page.
	File string          // Content-addressed page relative to the asset root.
}
type Descriptor struct {
	Version       int
	Source        string
	Width, Height int
	Tiles         []Tile
}
type Mode uint8

const (
	Plain Mode = iota // Premultiplied RGB565, matching surface.FromImage.
	Keyed             // Native UI key rules; zero alpha and pure green are transparent.
)

type chunk struct {
	width, height   int
	palette         []color.NRGBA
	keyedPalette    []uint16
	indices         []byte
	base, plain     []uint16
	residual, alpha []byte
}

// Cache compact source information alongside the prepared surface. Opaque
// true-color pages reuse base as plain; paletted pages retain one-byte indices.
func (c *chunk) size() int {
	bytes := len(c.indices) + len(c.palette)*4 + len(c.keyedPalette)*2 + len(c.base)*2 + len(c.residual) + len(c.alpha)
	if len(c.base) == 0 || len(c.alpha) > 0 {
		bytes += len(c.plain) * 2
	}
	return bytes
}
func (c *chunk) colorAt(i int) color.NRGBA {
	if len(c.palette) > 0 {
		return c.palette[c.indices[i]]
	}
	v, extra := c.base[i], c.residual[i]
	alpha := byte(255)
	if len(c.alpha) > 0 {
		alpha = c.alpha[i]
	}
	return color.NRGBA{R: byte(v>>11)<<3 | extra>>5, G: byte(v>>5&63)<<2 | extra>>3&3, B: byte(v&31)<<3 | extra&7, A: alpha}
}
func (c *chunk) keyedAt(i int) uint16 {
	if len(c.palette) > 0 {
		return c.keyedPalette[c.indices[i]]
	}
	v := c.base[i]
	if len(c.alpha) > 0 && c.alpha[i] == 0 || v == nativeGreenKey && c.residual[i] == nativeGreenResidual {
		return 0
	}
	if v&^nearBlackGreenBit == 0 {
		return v | 1
	} // All RGB channels below 8: preserve near-black.
	return v
}

func Hash(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }
func ChunkPath(group string, data []byte) string {
	return "runtime/images/" + group + "/" + Hash(data) + ChunkSuffix
}
func EncodeDescriptor(d Descriptor) ([]byte, error) {
	if err := validateDescriptor(d); err != nil {
		return nil, err
	}
	var b bytes.Buffer
	b.WriteString(descriptorMagic)
	err := gob.NewEncoder(&b).Encode(d)
	return b.Bytes(), err
}
func decodeDescriptor(r io.Reader) (Descriptor, error) {
	var d Descriptor
	h := make([]byte, len(descriptorMagic))
	if _, err := io.ReadFull(r, h); err != nil {
		return d, err
	}
	if string(h) != descriptorMagic {
		return d, fmt.Errorf("unsupported compiled image")
	}
	if err := gob.NewDecoder(io.LimitReader(r, maximumDescriptorBytes)).Decode(&d); err != nil {
		return d, err
	}
	return d, validateDescriptor(d)
}
func validateDescriptor(d Descriptor) error {
	if d.Version != Version || !fs.ValidPath(d.Source) || strings.ContainsAny(d.Source, "\\:") || d.Width <= 0 || d.Height <= 0 || d.Width > maximumImageSide || d.Height > maximumImageSide || int64(d.Width)*int64(d.Height) > maximumImagePixels {
		return fmt.Errorf("invalid compiled image descriptor")
	}
	canvas := image.Rect(0, 0, d.Width, d.Height)
	if len(d.Tiles) > (d.Width/TileSide+2)*(d.Height/TileSide+2) {
		return fmt.Errorf("too many image tiles")
	}
	for _, t := range d.Tiles {
		if t.Rect.Empty() || !t.Rect.In(canvas) || t.Rect.Dx() != t.Page.Dx() || t.Rect.Dy() != t.Page.Dy() || t.Page.Empty() || !t.Page.In(image.Rect(0, 0, TileSide, TileSide)) || !fs.ValidPath(t.File) || strings.ContainsAny(t.File, "\\:") || !strings.HasPrefix(t.File, "runtime/images/") || !strings.HasSuffix(t.File, ChunkSuffix) {
			return fmt.Errorf("invalid compiled image tile")
		}
	}
	return nil
}
func colors(p color.NRGBA) (plain, keyed uint16) {
	r, g, b, _ := p.RGBA()
	plain = uint16(r>>11)<<11 | uint16(g>>10)<<5 | uint16(b>>11)
	if p.A == 0 || p.R == 0 && p.G == 255 && p.B == 0 {
		return plain, 0
	}
	blue := p.B
	if p.R < 8 && p.G < 8 && blue < 8 {
		blue = 8
	}
	keyed = uint16(p.R>>3)<<11 | uint16(p.G>>2)<<5 | uint16(blue>>3)
	return
}

// EncodeChunk preserves every RGBA byte. For up to 256 colors it writes a palette
// with prepared RGB565 values. Other images split RGB into its RGB565 base and
// exact low-bit residual, so RGB565 can be loaded without per-pixel conversion.
func EncodeChunk(m *image.NRGBA) ([]byte, error) {
	b := m.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 || w > TileSide || h > TileSide {
		return nil, fmt.Errorf("invalid image page size")
	}
	rgba := make([]byte, w*h*pixelBytes)
	for y := 0; y < h; y++ {
		copy(rgba[y*w*pixelBytes:], m.Pix[y*m.Stride:y*m.Stride+w*pixelBytes])
	}
	palette := []color.NRGBA{}
	index := map[color.NRGBA]byte{}
	indices := make([]byte, w*h)
	for i := 0; i < w*h; i++ {
		p := color.NRGBA{R: rgba[i*4], G: rgba[i*4+1], B: rgba[i*4+2], A: rgba[i*4+3]}
		ix, ok := index[p]
		if !ok {
			if len(palette) == maximumPaletteColors {
				palette = nil
				break
			}
			ix = byte(len(palette))
			index[p] = ix
			palette = append(palette, p)
		}
		indices[i] = ix
	}
	var raw bytes.Buffer
	raw.WriteString(chunkMagic)
	write16 := func(v uint16) { var a [2]byte; binary.LittleEndian.PutUint16(a[:], v); raw.Write(a[:]) }
	write16(uint16(w))
	write16(uint16(h))
	write16(uint16(len(palette)))
	if palette != nil {
		for _, p := range palette {
			raw.Write([]byte{p.R, p.G, p.B, p.A})
			a, b := colors(p)
			write16(a)
			write16(b)
		}
		raw.Write(predict(indices, w, h))
	} else {
		// Independent byte planes keep the low-bit residual and alpha lossless and
		// let the premultiplied native surface bytes be reconstructed directly.
		planes := make([]byte, w*h*pixelBytes)
		for i := 0; i < w*h; i++ {
			r, g, b := rgba[i*4], rgba[i*4+1], rgba[i*4+2]
			v := uint16(r>>3)<<11 | uint16(g>>2)<<5 | uint16(b>>3)
			planes[i] = byte(v)
			planes[w*h+i] = byte(v >> 8)
			planes[2*w*h+i] = (r&7)<<5 | (g&3)<<3 | (b & 7)
			planes[3*w*h+i] = rgba[i*4+3]
		}
		for plane := 0; plane < pixelBytes; plane++ {
			raw.Write(predict(planes[plane*w*h:(plane+1)*w*h], w, h))
		}
	}
	var out bytes.Buffer
	z, err := zlib.NewWriterLevel(&out, zlib.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err = z.Write(raw.Bytes()); err != nil {
		return nil, err
	}
	if err = z.Close(); err != nil {
		return nil, err
	}
	if palette == nil {
		candidate, err := encodePNG(m)
		if err != nil {
			return nil, err
		}
		if len(candidate) < out.Len() {
			return candidate, nil
		}
	}
	return out.Bytes(), nil
}

func encodePNG(m *image.NRGBA) ([]byte, error) {
	var b bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestCompression}
	err := encoder.Encode(&b, m)
	return b.Bytes(), err
}

// Optimize repacks cached true-color pages only when lossless PNG is smaller.
// Palette pages already carry compact indices and prepared native colors.
func Optimize(data []byte) ([]byte, error) {
	if bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		return data, nil
	}
	c, err := decodeChunk(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if len(c.palette) > 0 {
		return data, nil
	}
	m := image.NewNRGBA(image.Rect(0, 0, c.width, c.height))
	for i := 0; i < c.width*c.height; i++ {
		p := c.colorAt(i)
		m.Pix[i*4], m.Pix[i*4+1], m.Pix[i*4+2], m.Pix[i*4+3] = p.R, p.G, p.B, p.A
	}
	candidate, err := encodePNG(m)
	if err != nil {
		return nil, err
	}
	if len(candidate) < len(data) {
		return candidate, nil
	}
	return data, nil
}

// predict uses reversible row differences; no pixel/color quantization occurs.
func predict(data []byte, w, h int) []byte {
	out := make([]byte, len(data))
	for y := 0; y < h; y++ {
		var previous byte
		for x := 0; x < w; x++ {
			i := y*w + x
			out[i] = data[i] - previous
			previous = data[i]
		}
	}
	return out
}
func unPredict(data []byte, w, h int) {
	for y := 0; y < h; y++ {
		var previous byte
		for x := 0; x < w; x++ {
			i := y*w + x
			data[i] += previous
			previous = data[i]
		}
	}
}
func decodeChunk(r io.Reader) (*chunk, error) {
	buffered := bufio.NewReader(r)
	signature, _ := buffered.Peek(8)
	if bytes.Equal(signature, []byte("\x89PNG\r\n\x1a\n")) {
		data, err := io.ReadAll(io.LimitReader(buffered, maximumChunkBytes+1))
		if err != nil {
			return nil, err
		}
		if len(data) > maximumChunkBytes {
			return nil, fmt.Errorf("image page too large")
		}
		cfg, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > TileSide || cfg.Height > TileSide {
			return nil, fmt.Errorf("invalid PNG page dimensions")
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		m := image.NewNRGBA(image.Rect(0, 0, cfg.Width, cfg.Height))
		draw.Draw(m, m.Bounds(), img, img.Bounds().Min, draw.Src)
		c := &chunk{width: cfg.Width, height: cfg.Height, base: make([]uint16, cfg.Width*cfg.Height), residual: make([]byte, cfg.Width*cfg.Height)}
		opaque := m.Opaque()
		if opaque {
			c.plain = c.base
		} else {
			c.alpha = make([]byte, cfg.Width*cfg.Height)
			c.plain = make([]uint16, cfg.Width*cfg.Height)
		}
		for i := range c.base {
			p := m.NRGBAAt(i%cfg.Width, i/cfg.Width)
			c.base[i] = uint16(p.R>>3)<<11 | uint16(p.G>>2)<<5 | uint16(p.B>>3)
			c.residual[i] = (p.R&7)<<5 | (p.G&3)<<3 | p.B&7
			if !opaque {
				c.alpha[i] = p.A
				c.plain[i], _ = colors(p)
			}
		}
		return c, nil
	}
	z, err := zlib.NewReader(buffered)
	if err != nil {
		return nil, err
	}
	defer z.Close()
	raw, err := io.ReadAll(io.LimitReader(z, maximumChunkBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) < chunkHeaderBytes || len(raw) > maximumChunkBytes || string(raw[:len(chunkMagic)]) != chunkMagic {
		return nil, fmt.Errorf("unsupported compiled image page")
	}
	w, h, count := int(binary.LittleEndian.Uint16(raw[8:])), int(binary.LittleEndian.Uint16(raw[10:])), int(binary.LittleEndian.Uint16(raw[12:]))
	if w <= 0 || h <= 0 || w > TileSide || h > TileSide || count > maximumPaletteColors {
		return nil, fmt.Errorf("invalid image page")
	}
	expected := chunkHeaderBytes + w*h*pixelBytes
	if count > 0 {
		expected = chunkHeaderBytes + count*colorTableBytes + w*h
	}
	if len(raw) != expected {
		return nil, fmt.Errorf("invalid image page payload")
	}

	c := &chunk{width: w, height: h}
	data := raw[chunkHeaderBytes:]
	if count > 0 {
		c.palette = make([]color.NRGBA, count)
		c.keyedPalette = make([]uint16, count)
		native := make([]uint16, count)
		for i := 0; i < count; i++ {
			p := data[i*colorTableBytes:]
			c.palette[i] = color.NRGBA{R: p[0], G: p[1], B: p[2], A: p[3]}
			native[i] = binary.LittleEndian.Uint16(p[4:])
			c.keyedPalette[i] = binary.LittleEndian.Uint16(p[6:])
		}
		c.indices = append([]byte(nil), data[count*colorTableBytes:]...)
		unPredict(c.indices, w, h)
		c.plain = make([]uint16, w*h)
		for i, ix := range c.indices {
			if int(ix) >= count {
				return nil, fmt.Errorf("invalid image palette index")
			}
			c.plain[i] = native[ix]
		}
	} else {
		for plane := 0; plane < pixelBytes; plane++ {
			unPredict(data[plane*w*h:(plane+1)*w*h], w, h)
		}
		c.base = make([]uint16, w*h)
		c.residual = append([]byte(nil), data[2*w*h:3*w*h]...)
		opaque := true
		for _, a := range data[3*w*h:] {
			if a != 255 {
				opaque = false
				break
			}
		}
		if opaque {
			c.plain = c.base
		} else {
			c.alpha = append([]byte(nil), data[3*w*h:]...)
			c.plain = make([]uint16, w*h)
		}
		for i := 0; i < w*h; i++ {
			c.base[i] = uint16(data[i]) | uint16(data[w*h+i])<<8
			if !opaque {
				c.plain[i], _ = colors(c.colorAt(i))
			}
		}
	}

	return c, nil
}
func assetRoot(path, source string) (string, error) {
	clean := filepath.Clean(path)
	if !strings.HasSuffix(strings.ToLower(filepath.ToSlash(clean)), strings.ToLower(source)) {
		return "", fmt.Errorf("compiled image source path mismatch")
	}
	root := clean
	for range strings.Split(source, "/") {
		root = filepath.Dir(root)
	}
	return root, nil
}

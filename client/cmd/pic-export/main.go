// Command pic-export converts native client picture archives and loose images to PNG.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/bmp"
	"wonderland-go/internal/clientassets"
)

const schemaVersion = 1
const maxImagePixels = 128_000_000
const bitmapHeaderBytes = 54
const bitmapPaletteEntryBytes = 4
const bitmapPaletteColors = 256
const bitmapPaletteCountOffset = 46
const bitmapPixelOffsetOffset = 10
const bitmapDepthOffset = 28
const bitmapInfoSizeOffset = 14
const bitmapInfoHeaderBytes = 40

const bitmapCoreHeaderBytes = 12
const bitmapFileHeaderBytes = 14
const bitmapCorePaletteEntryBytes = 3

// OS/2 BITMAPCOREHEADER stores dimensions as uint16 and palette colors as
// BGR triples. Expand it to BITMAPINFOHEADER without changing pixel values.
func expandCoreBitmap(raw []byte) ([]byte, error) {
	if len(raw) < bitmapFileHeaderBytes+bitmapCoreHeaderBytes || string(raw[:2]) != "BM" || binary.LittleEndian.Uint32(raw[bitmapInfoSizeOffset:]) != bitmapCoreHeaderBytes {
		return raw, nil
	}
	offset := int(binary.LittleEndian.Uint32(raw[bitmapPixelOffsetOffset:]))
	start := bitmapFileHeaderBytes + bitmapCoreHeaderBytes
	if offset < start || offset > len(raw) || (offset-start)%bitmapCorePaletteEntryBytes != 0 {
		return nil, fmt.Errorf("invalid core bitmap palette")
	}
	colors := (offset - start) / bitmapCorePaletteEntryBytes
	newOffset := bitmapHeaderBytes + colors*bitmapPaletteEntryBytes
	fixed := make([]byte, newOffset+len(raw)-offset)
	copy(fixed, raw[:bitmapFileHeaderBytes])
	binary.LittleEndian.PutUint32(fixed[2:], uint32(len(fixed)))
	binary.LittleEndian.PutUint32(fixed[bitmapPixelOffsetOffset:], uint32(newOffset))
	binary.LittleEndian.PutUint32(fixed[bitmapInfoSizeOffset:], bitmapInfoHeaderBytes)
	binary.LittleEndian.PutUint32(fixed[18:], uint32(binary.LittleEndian.Uint16(raw[18:])))
	binary.LittleEndian.PutUint32(fixed[22:], uint32(binary.LittleEndian.Uint16(raw[20:])))
	copy(fixed[26:30], raw[22:26])
	binary.LittleEndian.PutUint32(fixed[bitmapPaletteCountOffset:], uint32(colors))
	for i := 0; i < colors; i++ {
		copy(fixed[bitmapHeaderBytes+i*bitmapPaletteEntryBytes:], raw[start+i*bitmapCorePaletteEntryBytes:start+(i+1)*bitmapCorePaletteEntryBytes])
	}
	copy(fixed[newOffset:], raw[offset:])
	return fixed, nil
}

// Native bitmaps sometimes use an index outside their declared palette.
// Pad absent colors with black, matching Pillow's BMP conversion. This is a
// documented fallback for undefined source colors, not a recovered palette.
func normalizePalette(raw []byte) ([]byte, bool) {
	if len(raw) < bitmapHeaderBytes || string(raw[:2]) != "BM" || binary.LittleEndian.Uint32(raw[bitmapInfoSizeOffset:]) != bitmapInfoHeaderBytes || binary.LittleEndian.Uint16(raw[bitmapDepthOffset:]) != 8 {
		return raw, false
	}
	count := binary.LittleEndian.Uint32(raw[bitmapPaletteCountOffset:])
	offset := binary.LittleEndian.Uint32(raw[bitmapPixelOffsetOffset:])
	if count == 0 || count >= bitmapPaletteColors || offset != bitmapHeaderBytes+count*bitmapPaletteEntryBytes || int(offset) > len(raw) {
		return raw, false
	}
	padding := (bitmapPaletteColors - int(count)) * bitmapPaletteEntryBytes
	fixed := make([]byte, len(raw)+padding)
	copy(fixed, raw[:offset])
	copy(fixed[int(offset)+padding:], raw[offset:])
	binary.LittleEndian.PutUint32(fixed[bitmapPaletteCountOffset:], bitmapPaletteColors)
	binary.LittleEndian.PutUint32(fixed[bitmapPixelOffsetOffset:], offset+uint32(padding))
	binary.LittleEndian.PutUint32(fixed[2:], uint32(len(fixed)))
	return fixed, true
}

type picture struct {
	Resource    string                   `json:"resource"`
	PNG         string                   `json:"png"`
	LogicalPath string                   `json:"logical_path,omitempty"`
	Rect        *clientassets.SpriteRect `json:"rect,omitempty"`
	Width       int                      `json:"width"`
	Height      int                      `json:"height"`
	Note        string                   `json:"note,omitempty"`
}
type source struct {
	Name     string    `json:"name"`
	SHA256   string    `json:"sha256"`
	Kind     string    `json:"kind"`
	Pictures []picture `json:"pictures,omitempty"`
	Lines    []string  `json:"lines,omitempty"`
	Note     string    `json:"note,omitempty"`
}
type manifest struct {
	SchemaVersion int      `json:"schema_version"`
	Source        string   `json:"source"`
	Files         []source `json:"files"`
}

const bitmapWidthOffset = 18
const bitmapHeightOffset = 22
const bitmapCompressionOffset = 30
const bitmapRGB555Depth = 16
const bitmapRowAlignment = 4
const bitmapChannelMask = 31

func decodeRGB555(raw []byte) (image.Image, error) {
	width := int(int32(binary.LittleEndian.Uint32(raw[bitmapWidthOffset:])))
	height := int(int32(binary.LittleEndian.Uint32(raw[bitmapHeightOffset:])))
	topDown := height < 0
	if topDown {
		height = -height
	}
	if width <= 0 || height <= 0 || int64(width)*int64(height) > maxImagePixels {
		return nil, fmt.Errorf("invalid RGB555 dimensions")
	}
	offset := int(binary.LittleEndian.Uint32(raw[bitmapPixelOffsetOffset:]))
	stride := (width*2 + bitmapRowAlignment - 1) / bitmapRowAlignment * bitmapRowAlignment
	if offset < bitmapHeaderBytes || offset > len(raw) || int64(stride)*int64(height) > int64(len(raw)-offset) {
		return nil, fmt.Errorf("truncated RGB555 pixels")
	}
	m := image.NewNRGBA(image.Rect(0, 0, width, height))
	expand := func(v uint16) uint8 { return uint8(v<<3 | v>>2) }
	for y := 0; y < height; y++ {
		sourceY := height - 1 - y
		if topDown {
			sourceY = y
		}
		for x := 0; x < width; x++ {
			v := binary.LittleEndian.Uint16(raw[offset+sourceY*stride+x*2:])
			m.SetNRGBA(x, y, color.NRGBA{expand((v >> 10) & bitmapChannelMask), expand((v >> 5) & bitmapChannelMask), expand(v & bitmapChannelMask), 255})
		}
	}
	return m, nil
}

func decode(raw []byte) (image.Image, error) {
	if len(raw) >= bitmapHeaderBytes && string(raw[:2]) == "BM" && binary.LittleEndian.Uint32(raw[bitmapInfoSizeOffset:]) == bitmapInfoHeaderBytes && binary.LittleEndian.Uint16(raw[bitmapDepthOffset:]) == bitmapRGB555Depth && binary.LittleEndian.Uint32(raw[bitmapCompressionOffset:]) == 0 {
		return decodeRGB555(raw)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > maxImagePixels {
		return nil, fmt.Errorf("invalid or oversized image: %dx%d", cfg.Width, cfg.Height)
	}
	if len(raw) > 1 && raw[0] == 0xff && raw[1] == 0xd8 {
		return clientassets.DecodeJPEG(raw)
	}
	m, _, err := image.Decode(bytes.NewReader(raw))
	return m, err
}

func writePicture(root, dir, name string, raw []byte, used map[string]bool) (picture, error) {
	if filepath.Base(name) != name || strings.ContainsAny(name, "\\/") {
		return picture{}, fmt.Errorf("unsafe resource name %q", name)
	}
	path := filepath.Join(dir, strings.TrimSuffix(name, filepath.Ext(name))+".png")
	key := strings.ToLower(path)
	if used[key] {
		return picture{}, fmt.Errorf("duplicate output %s", path)
	}
	used[key] = true
	raw, err := expandCoreBitmap(raw)
	if err != nil {
		return picture{}, fmt.Errorf("%s: %w", name, err)
	}
	m, err := decode(raw)
	corrected := false
	if err != nil && strings.Contains(err.Error(), "invalid palette index") {
		var fixed []byte
		fixed, corrected = normalizePalette(raw)
		if corrected {
			m, err = decode(fixed)
		}
	}
	if err != nil {
		return picture{}, fmt.Errorf("%s: %w", name, err)
	}
	destination := filepath.Join(root, path)
	if err = os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return picture{}, err
	}
	f, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return picture{}, err
	}
	err = png.Encode(f, m)
	closeErr := f.Close()
	if err != nil {
		return picture{}, err
	}
	if closeErr != nil {
		return picture{}, closeErr
	}
	p := picture{Resource: name, PNG: filepath.ToSlash(path), Width: m.Bounds().Dx(), Height: m.Bounds().Dy()}
	if corrected {
		p.Note = "Source uses undefined palette indices; missing palette colors exported as black (Pillow-compatible fallback)."
	}
	return p, nil
}

func export(input, output string) error {
	// A new destination protects any hand-edited artwork from regeneration.
	if err := os.Mkdir(output, 0755); err != nil {
		return fmt.Errorf("output must be a new directory: %w", err)
	}
	entries, err := os.ReadDir(input)
	if err != nil {
		return err
	}
	absolute, err := filepath.Abs(input)
	if err != nil {
		return err
	}
	doc := manifest{SchemaVersion: schemaVersion, Source: absolute}
	used := map[string]bool{}
	total := 0
	for _, e := range entries {
		if e.IsDir() {
			return fmt.Errorf("unexpected source directory %s", e.Name())
		}
		raw, err := os.ReadFile(filepath.Join(input, e.Name()))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		info := source{Name: e.Name(), SHA256: hex.EncodeToString(sum[:])}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		switch ext {
		case ".bmg", ".jmg":
			info.Kind = "image-archive"
			a, err := clientassets.ReadImageArchive(bytes.NewReader(raw), int64(len(raw)))
			if err != nil {
				return fmt.Errorf("%s: %w", e.Name(), err)
			}
			dir := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			for i, entry := range a.Entries {
				payload, err := a.Read(i)
				if err != nil {
					return err
				}
				p, err := writePicture(output, dir, entry.Name, payload, used)
				if err != nil {
					return fmt.Errorf("%s: %w", e.Name(), err)
				}
				info.Pictures = append(info.Pictures, p)
			}
		case ".bmp", ".jpg", ".jpeg", ".png":
			info.Kind = "image"
			p, err := writePicture(output, "", e.Name(), raw, used)
			if err != nil {
				return err
			}
			info.Pictures = append(info.Pictures, p)
		case ".bls":
			info.Kind = "resource-list"
			info.Lines = strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
			info.Note = "Original line order and blank lines retained; numeric and name meanings are not inferred."
		default:
			info.Kind = "skipped"
			info.Note = "Not a supported game image; original file remains untouched."
		}
		total += len(info.Pictures)
		doc.Files = append(doc.Files, info)
		fmt.Printf("%s: %d PNGs (%s)\n", e.Name(), len(info.Pictures), info.Kind)
	}
	if err = atlasPictures(output, &doc); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(output, "manifest.json"), append(encoded, '\n'), 0644); err != nil {
		return err
	}
	fmt.Printf("Exported %d PNGs from %d source files to %s\n", total, len(doc.Files), output)
	return nil
}
func main() {
	input := flag.String("input", "", "original client pic directory")
	output := flag.String("output", "../data/pictures", "new output directory (must not exist)")
	optimize := flag.String("optimize", "", "atlas an existing editable picture export in place")
	flag.Parse()
	if *optimize != "" {
		if err := optimizePictures(*optimize); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if *input == "" {
		fmt.Fprintln(os.Stderr, "-input is required")
		os.Exit(1)
	}
	if err := export(*input, *output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

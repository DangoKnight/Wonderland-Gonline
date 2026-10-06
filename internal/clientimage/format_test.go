package clientimage

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestChunkLosslessAndPreparedColors(t *testing.T) {
	for _, palette := range []bool{true, false} {
		m := image.NewNRGBA(image.Rect(0, 0, 80, 70))
		for y := 0; y < 70; y++ {
			for x := 0; x < 80; x++ {
				p := color.NRGBA{R: byte(x*13 + y), G: byte(y*7 + x), B: byte(x * y), A: byte(x*31 + y*19)}
				if palette {
					p = []color.NRGBA{{}, {A: 255}, {G: 255, A: 255}, {R: 4, G: 7, B: 3, A: 255}, {R: 129, G: 151, B: 231, A: 127}, {R: 22, G: 88, B: 179, A: 255}}[(x+y)%6]
				}
				m.SetNRGBA(x, y, p)
			}
		}
		data, err := EncodeChunk(m)
		if err != nil {
			t.Fatal(err)
		}
		got, err := decodeChunk(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		exact := make([]byte, len(m.Pix))
		for i := 0; i < 80*70; i++ {
			p := got.colorAt(i)
			copy(exact[i*4:], []byte{p.R, p.G, p.B, p.A})
		}
		if !bytes.Equal(exact, m.Pix) {
			t.Fatal("RGBA changed")
		}
		for i := 0; i < 80*70; i++ {
			p := m.NRGBAAt(i%80, i/80)
			r, g, b, _ := p.RGBA()
			plain := uint16(uint8(r>>8)>>3)<<11 | uint16(uint8(g>>8)>>2)<<5 | uint16(uint8(b>>8)>>3)
			var key uint16
			if p.A != 0 && !(p.R == 0 && p.G == 255 && p.B == 0) {
				blue := p.B
				if p.R < 8 && p.G < 8 && p.B < 8 {
					blue = 8
				}
				key = uint16(p.R>>3)<<11 | uint16(p.G>>2)<<5 | uint16(blue>>3)
			}
			if got.plain[i] != plain || got.keyedAt(i) != key {
				t.Fatalf("converted colors differ at %d: %x/%x want %x/%x", i, got.plain[i], got.keyedAt(i), plain, key)
			}
		}
		// Subimages have a nonzero origin and parent stride; their rows must remain exact.
		sub := m.SubImage(image.Rect(11, 9, 44, 36)).(*image.NRGBA)
		data, err = EncodeChunk(sub)
		if err != nil {
			t.Fatal(err)
		}
		got, err = decodeChunk(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		for y := 0; y < 27; y++ {
			exact := make([]byte, 33*4)
			for x := 0; x < 33; x++ {
				p := got.colorAt(y*33 + x)
				copy(exact[x*4:], []byte{p.R, p.G, p.B, p.A})
			}
			if !bytes.Equal(exact, sub.Pix[y*sub.Stride:y*sub.Stride+33*4]) {
				t.Fatal("subimage offset/stride corrupted")
			}
		}
	}
}

func writeFixture(t *testing.T, root, name string, data []byte) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatal(err)
	}
}
func TestImageRegionsOnlyReadVisibleSharedPages(t *testing.T) {
	root := t.TempDir()
	source := "pictures/map/100.png"
	m := image.NewNRGBA(image.Rect(0, 0, 256, 256))
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			m.SetNRGBA(x, y, color.NRGBA{R: byte(x), G: byte(y), A: 255})
		}
	}
	data, err := EncodeChunk(m)
	if err != nil {
		t.Fatal(err)
	}
	file := ChunkPath("world", data)
	writeFixture(t, root, file, data)
	// The invisible tile deliberately points to a missing page. A region read must
	// not open it, even when the canvas is larger than the visible viewport.
	d := Descriptor{Version: Version, Source: source, Width: 1024, Height: 256, Tiles: []Tile{
		{Rect: image.Rect(0, 0, 256, 256), Page: image.Rect(0, 0, 256, 256), File: file},
		{Rect: image.Rect(768, 0, 1024, 256), Page: image.Rect(0, 0, 256, 256), File: "runtime/images/world/missing.wlt"},
	}}
	raw, err := EncodeDescriptor(d)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, source+DescriptorSuffix, raw)
	ClearCache()
	viewImage, err := Open(filepath.Join(root, filepath.FromSlash(source)))
	if err != nil {
		t.Fatal(err)
	}
	got, err := viewImage.NRGBA(image.Rect(10, 20, 30, 40))
	if err != nil {
		t.Fatal(err)
	}
	if got.NRGBAAt(0, 0) != m.NRGBAAt(10, 20) {
		t.Fatal("wrong image region")
	}
	stats := Stats()
	if stats.Misses != 1 || stats.Pages != 1 || stats.Bytes > CacheBytes {
		t.Fatalf("unexpected cache: %+v", stats)
	}
	view, err := viewImage.SubImage(image.Rect(8, 9, 50, 60))
	if err != nil {
		t.Fatal(err)
	}
	got, err = view.NRGBA(view.Bounds())
	if err != nil {
		t.Fatal(err)
	}
	if got.NRGBAAt(0, 0) != m.NRGBAAt(8, 9) {
		t.Fatal("view offset lost")
	}
	if Stats().Misses != 1 || Stats().Hits == 0 {
		t.Fatal("shared image page decoded twice")
	}
	if _, err = viewImage.NRGBA(viewImage.Bounds()); err == nil {
		t.Fatal("missing visible page ignored")
	}
}

func TestImageFormatRejectsInvalidData(t *testing.T) {
	for _, d := range []Descriptor{
		{Version: 99, Source: "image.png", Width: 1, Height: 1},
		{Version: Version, Source: "../image.png", Width: 1, Height: 1},
		{Version: Version, Source: "image.png", Width: -1, Height: 1},
		{Version: Version, Source: "image.png", Width: 1, Height: 1, Tiles: []Tile{{Rect: image.Rect(0, 0, 2, 2)}}},
	} {
		if _, err := EncodeDescriptor(d); err == nil {
			t.Fatal("invalid descriptor accepted")
		}
	}
	if _, err := decodeChunk(bytes.NewReader([]byte("not an image"))); err == nil {
		t.Fatal("invalid page accepted")
	}
	if _, err := decodeDescriptor(bytes.NewReader([]byte("not an image"))); err == nil {
		t.Fatal("invalid descriptor accepted")
	}
}

func TestImageCacheEvictsLeastRecentlyUsedByBytes(t *testing.T) {
	root := t.TempDir()
	m := image.NewNRGBA(image.Rect(0, 0, TileSide, TileSide))
	data, err := EncodeChunk(m)
	if err != nil {
		t.Fatal(err)
	}
	c, err := decodeChunk(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	count := CacheBytes / c.size()
	ClearCache()
	defer ClearCache()
	paths := make([]string, count+1)
	for i := range paths {
		name := fmt.Sprintf("%d.wlt", i)
		writeFixture(t, root, name, data)
		paths[i] = filepath.Join(root, name)
		if i < count {
			if _, err := page(paths[i]); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := page(paths[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := page(paths[count]); err != nil {
		t.Fatal(err)
	}
	pages.Lock()
	first := pages.entries[paths[0]] != nil
	second := pages.entries[paths[1]] != nil
	pages.Unlock()
	if !first || second || Stats().Bytes > CacheBytes {
		t.Fatal("byte budget or LRU policy violated")
	}
	before := Stats().Misses
	if _, err := page(paths[1]); err != nil {
		t.Fatal(err)
	}
	if Stats().Misses != before+1 {
		t.Fatal("evicted page not decoded again")
	}
}

func TestImageCacheConcurrentSharing(t *testing.T) {
	root := t.TempDir()
	m := image.NewNRGBA(image.Rect(0, 0, TileSide, TileSide))
	data, err := EncodeChunk(m)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, "shared.wlt", data)
	ClearCache()
	defer ClearCache()
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := page(filepath.Join(root, "shared.wlt")); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if Stats().Misses != 1 || Stats().Hits != 7 {
		t.Fatal("concurrent clients decoded separate page copies")
	}
}

func TestPageEncodingChoosesSmallerLosslessPNG(t *testing.T) {
	m := image.NewNRGBA(image.Rect(0, 0, TileSide, TileSide))
	for y := 0; y < TileSide; y++ {
		for x := 0; x < TileSide; x++ {
			m.SetNRGBA(x, y, color.NRGBA{R: byte(x), G: byte(y), B: byte((x + y) / 2), A: 255})
		}
	}
	data, err := EncodeChunk(m)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatal("smooth true-color page did not select smaller PNG representation")
	}
	got, err := decodeChunk(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < TileSide; y++ {
		for x := 0; x < TileSide; x++ {
			if got.colorAt(y*TileSide+x) != m.NRGBAAt(x, y) {
				t.Fatal("PNG page color changed")
			}
		}
	}
	same, err := Optimize(data)
	if err != nil || !bytes.Equal(same, data) {
		t.Fatal("already optimized PNG rewritten", err)
	}
}

func TestCachedLegacyPageOptimizationPreservesPixels(t *testing.T) {
	w, h := 80, 70
	m := image.NewNRGBA(image.Rect(0, 0, w, h))
	planes := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := color.NRGBA{R: byte(x), G: byte(y), B: byte((x + y) / 2), A: 255}
			m.SetNRGBA(x, y, p)
			i := y*w + x
			v := uint16(p.R>>3)<<11 | uint16(p.G>>2)<<5 | uint16(p.B>>3)
			planes[i] = byte(v)
			planes[w*h+i] = byte(v >> 8)
			planes[2*w*h+i] = (p.R&7)<<5 | (p.G&3)<<3 | p.B&7
			planes[3*w*h+i] = 255
		}
	}
	// Construct the previous wire representation independently from EncodeChunk.
	var raw bytes.Buffer
	raw.Write([]byte{'W', 'L', 'I', 'T', 1, 0, 0, 0, 80, 0, 70, 0, 0, 0})
	for plane := 0; plane < 4; plane++ {
		for y := 0; y < h; y++ {
			var previous byte
			for x := 0; x < w; x++ {
				v := planes[plane*w*h+y*w+x]
				raw.WriteByte(v - previous)
				previous = v
			}
		}
	}
	var legacy bytes.Buffer
	z := zlib.NewWriter(&legacy)
	if _, err := z.Write(raw.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	optimized, err := Optimize(legacy.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(optimized) > legacy.Len() {
		t.Fatal("optimization increased page size")
	}
	got, err := decodeChunk(bytes.NewReader(optimized))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < w*h; i++ {
		if got.colorAt(i) != m.NRGBAAt(i%w, i/w) {
			t.Fatal("cached page optimization changed pixels")
		}
	}
	large := image.NewNRGBA(image.Rect(0, 0, TileSide+1, 1))
	pngData, err := encodePNG(large)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeChunk(bytes.NewReader(pngData)); err == nil {
		t.Fatal("oversized PNG page accepted")
	}
}

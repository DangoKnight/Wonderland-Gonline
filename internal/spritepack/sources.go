package spritepack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"wonderland-gonline/internal/clientfs"

	"wonderland-gonline/internal/clientassets"
)

// Jxa indexes: a 14-byte "jx" header, then an 80-byte record per sprite
// holding each action's frame count.
const (
	jxaHeaderBytes = 14
	jxaRecordBytes = 80
)

// EditableFile is the editable export's index in an archive directory.
const EditableFile = "editable.json"

// ManifestFile is the editable export's source inventory, in the directory
// above the archive directories.
const ManifestFile = "sprite_manifest.json"

// editableSourceBytes reads an archive's original .jma size from the
// export manifest beside dir, or 0.
func editableSourceBytes(dir, archive string) int64 {
	data, err := clientfs.ReadFile(filepath.Join(filepath.Dir(dir), ManifestFile))
	if err != nil {
		return 0
	}
	var m struct {
		Sources []struct {
			Source string `json:"source"`
			Bytes  int64  `json:"bytes"`
		} `json:"sources"`
	}
	if json.Unmarshal(data, &m) != nil {
		return 0
	}
	for _, s := range m.Sources {
		base := s.Source[strings.LastIndexAny(s.Source, `/\`)+1:]
		if strings.EqualFold(base, archive+".jma") {
			return s.Bytes
		}
	}
	return 0
}

// EditablePack reads an editable export directory as a pack: its PNG sheets
// become the pages, so nothing is repacked.
func EditablePack(dir, archive string) (*Pack, error) {
	e, err := clientassets.OpenEditableSprites(filepath.Join(dir, EditableFile))
	if err != nil {
		return nil, err
	}
	p := &Pack{Version: Version, Archive: archive, FrameMS: DefaultFrameMS, SourceBytes: editableSourceBytes(dir, archive)}
	pages := map[string]int{}
	for _, src := range e.Sprites {
		s := Sprite{Index: src.Index, ID: SpriteID(src.Name), Name: src.Name}
		for _, f := range src.Frames {
			out := Frame{
				Name: f.Name, CanvasWidth: f.CanvasWidth, CanvasHeight: f.CanvasHeight,
				AnchorX: f.AnchorX, AnchorY: f.AnchorY,
				Rect: Rect{X: f.Rect.X, Y: f.Rect.Y, W: f.Rect.Width, H: f.Rect.Height},
			}
			out.OffsetX, out.OffsetY = Offsets(f.CanvasWidth, f.CanvasHeight, f.AnchorX, f.AnchorY)
			if out.Rect.W > 0 && out.Rect.H > 0 {
				i, ok := pages[f.Sheet]
				if !ok {
					i = len(p.Pages)
					pages[f.Sheet] = i
					p.Pages = append(p.Pages, f.Sheet)
				}
				out.Page = i
			}
			s.Frames = append(s.Frames, out)
		}
		for action, a := range src.Animations {
			s.Animations = append(s.Animations, Animation{Action: action, Name: a.Name, Frames: append([]int(nil), a.Frames...)})
		}
		p.Sprites = append(p.Sprites, s)
	}
	return p, p.Validate()
}

// BuildEditable repacks an editable export directory into atlas pages.
func BuildEditable(dir, archive string, sink PageSink) (*Pack, error) {
	e, err := clientassets.OpenEditableSprites(filepath.Join(dir, EditableFile))
	if err != nil {
		return nil, err
	}
	sprites := append([]clientassets.EditableSprite(nil), e.Sprites...)
	sort.Slice(sprites, func(i, j int) bool { return sprites[i].Index < sprites[j].Index })
	b := NewBuilder(archive, sink)
	b.SetSourceBytes(editableSourceBytes(dir, archive))
	native, nativeErr := clientassets.OpenSpriteJSON(filepath.Join(dir, NativeFile))
	if nativeErr != nil && !os.IsNotExist(nativeErr) {
		return nil, nativeErr
	}
	if native != nil {
		defer native.Close()
	}
	for _, src := range sprites {
		var decoded *clientassets.Sprite
		if native != nil && src.Index < len(native.Archive.Entries) {
			decoded, _ = native.Archive.Sprite(src.Index)
		}
		var palette *[256][3]uint8
		if decoded != nil {
			palette = &decoded.Palette
		}
		frames := make([]FrameImage, len(src.Frames))
		for i, f := range src.Frames {
			img, err := e.FrameImage(f)
			if err != nil {
				return nil, fmt.Errorf("%s frame %d: %w", src.Name, i, err)
			}
			frames[i] = FrameImage{Name: f.Name, Image: img, CanvasWidth: f.CanvasWidth, CanvasHeight: f.CanvasHeight, AnchorX: f.AnchorX, AnchorY: f.AnchorY}
			if decoded != nil && i < len(decoded.Frames) {
				frames[i].Indices = matchingIndices(img, decoded.Frames[i], palette)
			}
		}
		var anims []Animation
		for action, a := range src.Animations {
			anims = append(anims, Animation{Action: action, Name: a.Name, Frames: append([]int(nil), a.Frames...)})
		}
		if err = b.AddSprite(src.Index, SpriteID(src.Name), src.Name, frames, anims, palette); err != nil {
			return nil, err
		}
	}
	return b.Finish()
}

// MatchIndices is matchingIndices for loaders that derive indices at run
// time from an editable export (the client reading data/sprites directly).
func MatchIndices(img image.Image, f clientassets.SpriteFrame, palette *[256][3]uint8) *image.Gray {
	return matchingIndices(img, f, palette)
}

// NativeFile is the editable export's lossless original archive bytes, in
// each archive directory.
const NativeFile = "sprites.json"

// matchingIndices returns a native frame's palette indices when the
// editable frame still shows exactly those pixels; an edited frame gets
// none and keeps its painted colours.
func matchingIndices(img image.Image, f clientassets.SpriteFrame, palette *[256][3]uint8) *image.Gray {
	b := img.Bounds()
	if b.Dx() != f.Width || b.Dy() != f.Height {
		return nil
	}
	want, ix := nativeFrame(f, palette)
	got := pixelsOf(img)
	if !bytes.Equal(got.Pix, want.Pix) {
		return nil
	}
	return ix
}

// JMA is an original archive with its Jxa index.
type JMA struct {
	Archive *clientassets.SpriteArchive
	Jxa     []byte
	Size    int64
	file    *os.File
}

// OpenJMA opens path (a .jma file) and the .Jxa beside it.
func OpenJMA(jmaPath, jxaPath string) (*JMA, error) {
	f, err := os.Open(jmaPath)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	a, err := clientassets.OpenSpriteArchive(f, st.Size())
	if err != nil {
		f.Close()
		return nil, err
	}
	jxa, _ := clientfs.ReadFile(jxaPath)
	return &JMA{Archive: a, Jxa: jxa, Size: st.Size(), file: f}, nil
}

func (j *JMA) Close() error { return j.file.Close() }

// counts is a sprite's Jxa record: frames per action.
func (j *JMA) counts(index int) []byte {
	at := jxaHeaderBytes + index*jxaRecordBytes
	if at+jxaRecordBytes > len(j.Jxa) {
		return nil
	}
	return j.Jxa[at : at+jxaRecordBytes]
}

// frames converts a decoded sprite with the client's palette rules
// (clientassets.NativeSpriteColor) and its animations: each action's frames
// follow the previous actions' frames (FUN_002fe570).
func (j *JMA) frames(index int) ([]FrameImage, []Animation, string, *[256][3]uint8, error) {
	e := j.Archive.Entries[index]
	d, err := j.Archive.Sprite(index)
	if err != nil {
		return nil, nil, e.Name, nil, err
	}
	frames := make([]FrameImage, len(d.Frames))
	for i, f := range d.Frames {
		img, ix := nativeFrame(f, &d.Palette)
		frames[i] = FrameImage{Name: f.Name, Image: img, Indices: ix, CanvasWidth: f.CanvasWidth, CanvasHeight: f.CanvasHeight, AnchorX: f.X, AnchorY: f.Y}
	}
	var anims []Animation
	next := 0
	for action, c := range j.counts(index) {
		a := Animation{Action: action, Name: fmt.Sprintf("action_%02d", action), Frames: []int{}}
		for k := 0; k < int(c); k++ {
			if next < len(frames) {
				a.Frames = append(a.Frames, next)
			} else {
				a.Frames = append(a.Frames, -1)
			}
			next++
		}
		anims = append(anims, a)
	}
	return frames, anims, e.Name, &d.Palette, nil
}

// nativeFrame renders a frame with the client's palette rules
// (clientassets.NativeSpriteColor) and keeps its palette indices.
func nativeFrame(f clientassets.SpriteFrame, palette *[256][3]uint8) (*image.NRGBA, *image.Gray) {
	img := image.NewNRGBA(image.Rect(0, 0, f.Width, f.Height))
	ix := image.NewGray(image.Rect(0, 0, f.Width, f.Height))
	for y := 0; y < f.Height; y++ {
		for x := 0; x < f.Width; x++ {
			p := f.Pixels[y*f.Stride+x]
			r, g, b, a := clientassets.NativeSpriteColor(p, *palette)
			img.SetNRGBA(x, y, color.NRGBA{r, g, b, a})
			ix.Pix[y*ix.Stride+x] = p
		}
	}
	return img, ix
}

// BuildJMA packs every sprite of an original archive.
func BuildJMA(j *JMA, archive string, sink PageSink) (*Pack, error) {
	b := NewBuilder(archive, sink)
	b.SetSourceBytes(j.Size)
	for i := range j.Archive.Entries {
		frames, anims, name, palette, err := j.frames(i)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if err = b.AddSprite(i, SpriteID(name), name, frames, anims, palette); err != nil {
			return nil, err
		}
	}
	return b.Finish()
}

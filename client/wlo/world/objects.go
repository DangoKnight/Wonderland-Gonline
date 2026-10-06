package world

import (
	"encoding/hex"
	"fmt"
	"image"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"wonderland-gonline/internal/clientruntime"

	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/surface"
)

// Scene objects (TGroundObj). The scene loader (FUN_003f830c) reads the
// map record's object list (resource, X, Y) after its walk grid and
// creates one object per entry (FUN_003f9310), described by the resource's
// Wem.MMG record (FUN_003f5f1c). An object paints its picture with the
// top-left corner at its position (FUN_003f65cc); pictures with several
// frames are stacked vertically and advance every interval.
//
// The map paint (FUN_0041e0d8) draws them by kind (+0xd6):
//   - kinds 0..2 right after the background, under every character;
//   - kind 3 in the depth-sorted list with the characters (FUN_0041d13c),
//     keyed by its Y plus its depth line, against the characters' feet;
//   - kinds 4 and 5 after the characters: overhangs such as the deck's
//     railings, painted over the background's copy of themselves.
//
// Not ported: the cull test (FUN_003f5d70), translucent objects (+0xd4
// other than 0xff, and +0xe0 from the picture's first pixel), and the
// resources 11011..11015 and 11031 that the loader forces to kind 3.
type Object struct {
	Resource uint32
	X, Y     int
	Depth    int // pixels below Y where characters pass behind it
	Kind     byte
	Frames   int
	Interval time.Duration
	Image    *surface.Surface
}

// Object kinds (+0xd6).
const (
	ObjectSorted = 3 // drawn in the depth-sorted pass; lower kinds before it, higher after
)

const (
	wemExport = "wem_data.json"
	// Wem.MMG record layout (FUN_003f5f1c): resource u32, a u32, the depth
	// line in grid cells u32, frames u8, interval u16 (ms), width u16,
	// height u16, translucency u8, kind u8.
	wemDepth    = 8
	wemFrames   = 12
	wemInterval = 13
	wemKind     = 20
	wemBytes    = 21
)

// objectArchives are the picture archives searched for object pictures.
var objectArchives = []string{"images1_d01", "images1", "images3_c01", "images3_01", "images3", "images4_c01", "images4", "images_c01", "images"}

type wemRecord struct {
	depth    int
	frames   int
	interval time.Duration
	kind     byte
}

var wems struct {
	sync.Mutex
	path    string
	records map[uint32]wemRecord
}

// wemRecords reads the Wem.MMG export, once.
func wemRecords(a login.Assets) (map[uint32]wemRecord, error) {
	wems.Lock()
	defer wems.Unlock()
	path := a.DataPath(wemExport)
	if wems.path == path {
		return wems.records, nil
	}
	var doc struct {
		Entries []struct {
			Name       string `json:"name"`
			DecodedHex string `json:"decoded_hex"`
		} `json:"entries"`
	}
	var compiled map[uint32][]byte
	if err := clientruntime.Read(a.DataPath(clientruntime.ObjectsFile), &compiled); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		if err := readJSON(path, &doc); err != nil {
			return nil, err
		}
	}
	out := make(map[uint32]wemRecord, len(doc.Entries))
	if compiled == nil {
		compiled = make(map[uint32][]byte, len(doc.Entries))
		for _, e := range doc.Entries {
			id, err := strconv.ParseUint(strings.TrimSuffix(strings.ToLower(e.Name), ".wem"), 10, 32)
			if err != nil {
				continue
			}
			b, err := hex.DecodeString(e.DecodedHex)
			if err != nil {
				continue
			}
			compiled[uint32(id)] = b
		}
	}
	for id, b := range compiled {
		if len(b) < wemBytes {
			continue
		}
		le := func(i, n int) int {
			v := 0
			for k := n - 1; k >= 0; k-- {
				v = v<<8 | int(b[i+k])
			}
			return v
		}
		out[id] = wemRecord{
			depth:    le(wemDepth, 4) * cellSize,
			frames:   max(int(b[wemFrames]), 1),
			interval: time.Duration(le(wemInterval, 2)) * time.Millisecond,
			kind:     b[wemKind],
		}
	}
	wems.path, wems.records = path, out
	return out, nil
}

// objectPicture loads a resource's picture from the first archive that
// has it, green key made transparent.
func objectPicture(a login.Assets, id uint32) (*surface.Surface, error) {
	name := strconv.FormatUint(uint64(id), 10)
	for _, arc := range objectArchives {
		m, err := a.LoadPicture(arc, name)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", arc, name, err)
		}
		return keyed(m), nil
	}
	return nil, fmt.Errorf("object picture %s not found", name)
}

// keyed converts a picture with FUN_0047b274's key rules: pure green is
// the transparent key 0, and near-black keeps a blue of 8 so it stays
// visible.
func keyed(m *image.NRGBA) *surface.Surface {
	b := m.Bounds()
	s := surface.New(b.Dx(), b.Dy())
	for y := 0; y < b.Dy(); y++ {
		row := m.Pix[y*m.Stride:]
		for x := 0; x < b.Dx(); x++ {
			r, g, bl := row[x*4], row[x*4+1], row[x*4+2]
			var v uint16
			if r == 0 && g == 0xff && bl == 0 {
				v = 0
			} else {
				if r < 8 && g < 8 && bl < 8 {
					bl = 8
				}
				v = uint16(r>>3)<<11 | uint16(g>>2)<<5 | uint16(bl>>3)
			}
			s.Pix[y*s.W+x] = v
		}
	}
	return s
}

// loadObjects creates the scene's objects from its record. Objects whose
// description or picture is missing are skipped.
func (s *Scene) loadObjects(a login.Assets, list []groundObject) error {
	recs, err := wemRecords(a)
	if err != nil {
		return err
	}
	pics := map[uint32]*surface.Surface{}
	for _, o := range list {
		r, ok := recs[o.Resource]
		if !ok {
			continue
		}
		img, seen := pics[o.Resource]
		if !seen {
			img, _ = objectPicture(a, o.Resource)
			pics[o.Resource] = img
		}
		if img == nil {
			continue
		}
		s.Objects = append(s.Objects, Object{Resource: o.Resource, X: int(o.X), Y: int(o.Y), Depth: r.depth,
			Kind: r.kind, Frames: r.frames, Interval: r.interval, Image: img})
	}
	return nil
}

// Draw paints the object's current frame with the camera at (camX, camY).
func (o *Object) Draw(dst *surface.Surface, camX, camY int, now time.Time) {
	h := o.Image.H / o.Frames
	frame := 0
	if o.Frames > 1 && o.Interval > 0 {
		frame = int(now.UnixMilli()/o.Interval.Milliseconds()) % o.Frames
	}
	dst.DrawRect(o.X-camX, o.Y-camY, image.Rect(0, frame*h, o.Image.W, (frame+1)*h), o.Image, true)
}

// SortY is a kind-3 object's place among the characters' feet.
func (o *Object) SortY() int { return o.Y + o.Depth }

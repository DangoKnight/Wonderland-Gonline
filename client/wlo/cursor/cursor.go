// Package cursor ports the original's mouse cursors. At startup (0x3bb990)
// the client registers animated Windows cursors from cursor\*.ani in
// Screen.Cursors and selects ShapeNormal; later Tjo_Cursor (0x3bac58)
// switches the index as the pointer moves over the world. Ebitengine cannot
// install custom system cursors, so the client hides the system pointer and
// draws the exported ANI frames itself.
package cursor

import (
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	_ "image/png"
	"os"
	"path/filepath"
	"time"
)

// Shape is a Screen.Cursors index registered at startup (0x3bb990). Names
// describe the artwork; the game situations that select most of them are
// still untraced.
type Shape int

const (
	ShapeSword        Shape = 1  // cursor3.ani
	ShapePoint        Shape = 2  // cursor4.ani
	ShapeLook         Shape = 3  // cursor5.ani, binoculars
	ShapeSwordDown    Shape = 4  // cursor11.ani, sword with a red arrow
	ShapePointAlt     Shape = 5  // cursor4.ani again
	ShapeSwordAlt     Shape = 6  // cursor3.ani again
	ShapeMagic        Shape = 7  // cursor6.ani
	ShapeGrab         Shape = 8  // cursor8.ani
	ShapeNormalAlt    Shape = 9  // cursor2.ani, Tjo_Cursor's state 0 while DAT_004ca00c is set
	ShapeMushroom     Shape = 10 // cursor9.ani
	ShapeBusy         Shape = 11 // cursor10.ani, hourglass
	ShapeNormal       Shape = 12 // cursor1.ani, selected at startup
	ShapeHammer       Shape = 13 // cursor12.ani
	ShapeHammerTilted Shape = 14 // cursor13.ani
	ShapeHammerSide   Shape = 15 // cursor14.ani
	ShapeTarget       Shape = 16 // cursor15.ani
	ShapeInspect      Shape = 17 // cursor17.ani
	ShapeHeal         Shape = 18 // cursor18.ani
	ShapeHidden       Shape = 99 // cursor99.ani, a blank cursor
)

// Registered is the startup table: each shape and its file under cursor\.
var Registered = map[Shape]string{
	ShapeNormal: "cursor1", ShapeSword: "cursor3", ShapePoint: "cursor4",
	ShapeLook: "cursor5", ShapeSwordDown: "cursor11", ShapePointAlt: "cursor4",
	ShapeSwordAlt: "cursor3", ShapeMagic: "cursor6", ShapeGrab: "cursor8",
	ShapeNormalAlt: "cursor2", ShapeMushroom: "cursor9", ShapeBusy: "cursor10",
	ShapeHammer: "cursor12", ShapeHammerTilted: "cursor13", ShapeHammerSide: "cursor14",
	ShapeTarget: "cursor15", ShapeInspect: "cursor17", ShapeHeal: "cursor18",
	ShapeHidden: "cursor99",
}

// jiffy is the ANI rate unit.
const jiffy = time.Second / 60

// Frame is one cursor image and its hotspot.
type Frame struct {
	Image   *image.NRGBA
	Hotspot image.Point
}

// Animation is an exported ANI cursor: frames shown in Sequence order,
// each step lasting Rates[i] jiffies.
type Animation struct {
	Name     string
	Frames   []Frame
	Sequence []int
	Rates    []int
	period   time.Duration
}

type manifest struct {
	Format string `json:"format"`
	Frames []struct {
		PNG     string `json:"png"`
		Hotspot [2]int `json:"hotspot"`
	} `json:"frames"`
	Rates    []int `json:"rate_jiffies"`
	Sequence []int `json:"sequence"`
}

// Load reads an ANI export (media/cursor/<name>/manifest.json).
func Load(dir, name string) (*Animation, error) {
	base := filepath.Join(dir, name)
	raw, err := os.ReadFile(filepath.Join(base, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("cursor %s: %w", name, err)
	}
	if m.Format != "animated-cursor" || len(m.Frames) == 0 {
		return nil, fmt.Errorf("cursor %s: not an animated cursor export", name)
	}
	a := &Animation{Name: name, Sequence: m.Sequence, Rates: m.Rates}
	for _, f := range m.Frames {
		img, err := loadPNG(filepath.Join(base, filepath.FromSlash(f.PNG)))
		if err != nil {
			return nil, fmt.Errorf("cursor %s: %w", name, err)
		}
		a.Frames = append(a.Frames, Frame{img, image.Pt(f.Hotspot[0], f.Hotspot[1])})
	}
	if len(a.Sequence) == 0 {
		for i := range a.Frames {
			a.Sequence = append(a.Sequence, i)
		}
	}
	if len(a.Rates) != len(a.Sequence) {
		return nil, fmt.Errorf("cursor %s: %d rates for %d steps", name, len(a.Rates), len(a.Sequence))
	}
	for i, s := range a.Sequence {
		if s < 0 || s >= len(a.Frames) {
			return nil, fmt.Errorf("cursor %s: step %d shows missing frame %d", name, i, s)
		}
		a.period += time.Duration(max(a.Rates[i], 1)) * jiffy
	}
	return a, nil
}

func loadPNG(path string) (*image.NRGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	m, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	if n, ok := m.(*image.NRGBA); ok {
		return n, nil
	}
	n := image.NewNRGBA(m.Bounds())
	draw.Draw(n, n.Bounds(), m, m.Bounds().Min, draw.Src)
	return n, nil
}

// At is the frame shown d after the cursor was selected; the animation
// loops as Windows plays it.
func (a *Animation) At(d time.Duration) int {
	if len(a.Sequence) == 1 || a.period <= 0 {
		return a.Sequence[0]
	}
	d %= a.period
	for i, s := range a.Sequence {
		step := time.Duration(max(a.Rates[i], 1)) * jiffy
		if d < step {
			return s
		}
		d -= step
	}
	return a.Sequence[len(a.Sequence)-1]
}

// Cursors is Screen.Cursors with Screen.Cursor: the registered shapes and
// the one selected.
type Cursors struct {
	Shapes   map[Shape]*Animation
	Current  Shape
	selected time.Time
}

// LoadAll registers every shape from dir (media/cursor), as 0x3bb990 does,
// and selects ShapeNormal.
func LoadAll(dir string, now time.Time) (*Cursors, error) {
	c := &Cursors{Shapes: map[Shape]*Animation{}}
	loaded := map[string]*Animation{}
	for shape, name := range Registered {
		a := loaded[name]
		if a == nil {
			var err error
			if a, err = Load(dir, name); err != nil {
				return nil, err
			}
			loaded[name] = a
		}
		c.Shapes[shape] = a
	}
	c.Set(ShapeNormal, now)
	return c, nil
}

// Set is Screen.Cursor := s (0x5f368). Selecting the current shape again
// does not restart its animation.
func (c *Cursors) Set(s Shape, now time.Time) {
	if s == c.Current && !c.selected.IsZero() {
		return
	}
	c.Current, c.selected = s, now
}

// Frame is the image to draw now, or nil for an unregistered shape, which
// Windows shows as the arrow; the window then shows the system pointer.
func (c *Cursors) Frame(now time.Time) *Frame {
	a := c.Shapes[c.Current]
	if a == nil {
		return nil
	}
	return &a.Frames[a.At(now.Sub(c.selected))]
}

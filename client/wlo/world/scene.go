// Package world is the in-game view: the scene of the player's map, drawn
// around the player, with the characters on it.
//
// Only the static view is ported so far. The scene loader (FUN_003f830c)
// reads a Ground.MMG record whose first block (FUN_004121a8) gives the
// scene size, its background layers and a 20-pixel walk grid, followed by
// the sound zones (zones.go) and the scene objects (objects.go); the rest
// of the record is not used yet.
//
// The terrain comes from the extracted Ground.MMG (data/ground_data.json)
// and the background pictures from the PNG export of pic\ (data/pictures).
package world

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"wonderland-go/client/wlo/login"
	"wonderland-go/client/wlo/surface"
	"wonderland-go/internal/clientassets"
)

// Map background archives, searched in order (patches first).
var backgroundArchives = []string{"map_d01", "map_c01", "map"}

const groundExport = "ground_data.json"

// Scene is one map's terrain record and its loaded background layers.
type Scene struct {
	MapID         uint16
	Width, Height int
	Ground        clientassets.GroundPrefix
	Layers        []Layer
	Objects       []Object
	Zones         []SoundZone
}

// Layer is a background picture at a scene position.
type Layer struct {
	Resource uint16
	X, Y     int
	Image    *surface.Surface
}

// groundEntry is one record of the Ground.MMG export.
type groundEntry struct {
	Name    string `json:"name"`
	Terrain struct {
		Width      uint32 `json:"width"`
		Height     uint32 `json:"height"`
		Layers     []clientassets.GroundLayer
		GridWidth  uint16         `json:"grid_width"`
		GridHeight uint16         `json:"grid_height"`
		CellsHex   string         `json:"cells_hex"`
		Zones      []groundZone   `json:"unknown_triples"`
		Objects    []groundObject `json:"objects"`
	} `json:"terrain"`
}

// groundObject is one entry of the record's object list.
type groundObject struct {
	Resource uint32 `json:"resource"`
	X        uint16 `json:"x"`
	Y        uint16 `json:"y"`
}

var grounds struct {
	sync.Mutex
	path    string
	entries map[string]*groundEntry
}

// groundRecord finds <map>.map in the Ground.MMG export, read once.
func groundRecord(a login.Assets, mapID uint16) (*groundEntry, error) {
	grounds.Lock()
	defer grounds.Unlock()
	path := a.DataPath(groundExport)
	if grounds.path != path {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var doc struct {
			Entries []*groundEntry `json:"entries"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, fmt.Errorf("%s: %w", groundExport, err)
		}
		grounds.entries = map[string]*groundEntry{}
		for _, e := range doc.Entries {
			grounds.entries[strings.ToLower(e.Name)] = e
		}
		grounds.path = path
	}
	e, ok := grounds.entries[strconv.Itoa(int(mapID))+".map"]
	if !ok {
		return nil, fmt.Errorf("map %d has no terrain record", mapID)
	}
	return e, nil
}

// LoadScene reads the map's terrain and its background layers.
func LoadScene(a login.Assets, mapID uint16) (*Scene, error) {
	e, err := groundRecord(a, mapID)
	if err != nil {
		return nil, err
	}
	t := e.Terrain
	cells, err := hex.DecodeString(t.CellsHex)
	if err != nil {
		return nil, err
	}
	g := clientassets.GroundPrefix{Width: t.Width, Height: t.Height, Layers: t.Layers,
		GridWidth: t.GridWidth, GridHeight: t.GridHeight, Cells: cells}
	s := &Scene{MapID: mapID, Width: int(g.Width), Height: int(g.Height), Ground: g, Zones: zonesOf(t.Zones)}
	for _, l := range g.Layers {
		img, err := background(a, l.Resource)
		if err != nil {
			return nil, err
		}
		s.Layers = append(s.Layers, Layer{Resource: l.Resource, X: int(l.X), Y: int(l.Y), Image: img})
	}
	if err := s.loadObjects(a, t.Objects); err != nil {
		return nil, err
	}
	return s, nil
}

// background decodes the first exported picture <archive>/<resource>.png.
func background(a login.Assets, resource uint16) (*surface.Surface, error) {
	name := strconv.Itoa(int(resource))
	for _, arc := range backgroundArchives {
		m, err := a.LoadPicture(arc, name)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", arc, name, err)
		}
		return surface.FromImage(m), nil
	}
	return nil, fmt.Errorf("background %s not found", name)
}

// Draw draws the background layers with the camera's top-left corner at
// (camX, camY) in scene coordinates.
func (s *Scene) Draw(dst *surface.Surface, camX, camY int) {
	for _, l := range s.Layers {
		if l.Image != nil {
			dst.Draw(l.X-camX, l.Y-camY, l.Image, false)
		}
	}
}

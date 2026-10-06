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
	"wonderland-gonline/internal/clientfs"
	"wonderland-gonline/internal/clientruntime"

	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/internal/clientassets"
)

// Map background archives, searched in order (patches first).
var backgroundArchives = []string{"map_d01", "map_c01", "map"}

const groundExport = "ground_data.json"

// Scene is one map's terrain record and its loaded background layers.
type Scene struct {
	WaterTravel   bool // Set only after a server-confirmed water vehicle mount.
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
type groundEntry = clientruntime.Ground
type groundObject = clientruntime.Object

var grounds struct {
	sync.Mutex
	path    string
	entries map[string]*groundEntry
}

// groundRecord finds <map>.map in the Ground.MMG export, read once.
func groundRecord(a login.Assets, mapID uint16) (*groundEntry, error) {
	var compiled groundEntry
	err := clientruntime.Read(a.DataPath(clientruntime.MapPath(mapID)), &compiled)
	if err == nil {
		return &compiled, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	if _, e := clientfs.Stat(a.DataPath(clientruntime.GroundDir)); e == nil {
		return nil, fmt.Errorf("map %d has no compiled terrain record", mapID)
	}

	grounds.Lock()
	defer grounds.Unlock()
	path := a.DataPath(groundExport)
	if grounds.path != path {
		raw, err := clientfs.ReadFile(path)
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
func loadScene(a login.Assets, mapID uint16) (*Scene, error) {
	e, err := groundRecord(a, mapID)
	if err != nil {
		return nil, err
	}
	t := e.Terrain
	cells := t.Cells
	if cells == nil {
		cells, err = hex.DecodeString(t.CellsHex)
		if err != nil {
			return nil, err
		}
	}
	if len(cells) != int(t.GridWidth)*int(t.GridHeight) {
		return nil, fmt.Errorf("invalid terrain grid for map %d", mapID)
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
		if compiled, err := a.CompiledPicture(arc, name); err == nil {
			return surface.FromCompiled(compiled), nil
		} else if !os.IsNotExist(err) {
			return nil, err
		}
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

// sceneCache holds four recent immutable scene definitions. Each world receives
// its own descriptor; large terrain and decoded image buffers are shared.
const cachedScenes = 4

var sceneCache = struct {
	sync.Mutex
	entries map[string]*Scene
	order   []string
}{entries: map[string]*Scene{}}

func LoadScene(a login.Assets, mapID uint16) (*Scene, error) {
	key := fmt.Sprintf("%s:%d", a.Data, mapID)
	sceneCache.Lock()
	defer sceneCache.Unlock()
	s := sceneCache.entries[key]
	if s == nil {
		var err error
		s, err = loadScene(a, mapID)
		if err != nil {
			return nil, err
		}
		sceneCache.entries[key] = s
		sceneCache.order = append(sceneCache.order, key)
		if len(sceneCache.order) > cachedScenes {
			delete(sceneCache.entries, sceneCache.order[0])
			sceneCache.order = sceneCache.order[1:]
		}
	}
	out := *s
	out.Layers = append([]Layer(nil), s.Layers...)
	out.Objects = append([]Object(nil), s.Objects...)
	out.Zones = append([]SoundZone(nil), s.Zones...)
	return &out, nil
}

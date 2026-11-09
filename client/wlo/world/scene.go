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

import "wonderland-gonline/internal/localization"

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"os"
	"strconv"
	"strings"
	"sync"
	"wonderland-gonline/internal/clientfs"
	"wonderland-gonline/internal/clientruntime"

	"wonderland-gonline/client/wlo/housing"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/game"
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
	Tiles         []TerrainTile
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
		return nil, fmt.Errorf(localization.UI("ui.world.map_d_has_no_compiled_terrain_record_ca4bc533"), mapID)
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
		return nil, fmt.Errorf(localization.UI("ui.world.map_d_has_no_terrain_record_90688a4a"), mapID)
	}
	return e, nil
}

// LoadScene reads the map's terrain and its background layers.
func loadScene(a login.Assets, mapID uint16) (*Scene, error) {
	if game.IsTentMap(mapID) {
		return NewTentScene(mapID), nil
	}
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
		return nil, fmt.Errorf(localization.UI("ui.world.invalid_terrain_grid_for_map_d_f098539e"), mapID)
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
	if err := s.loadTerrainTiles(a, t.TileGrid); err != nil {
		return nil, err
	}
	return s, nil
}

// Tent interiors are synthesized by the native housing manager; Ground.MMG
// has no terrain record for either private floor.
func NewTentScene(mapID uint16) *Scene {
	w, h := housing.SceneWidth, housing.SceneHeight
	g := clientassets.GroundPrefix{Width: uint32(w), Height: uint32(h), GridWidth: uint16((w + cellSize - 1) / cellSize), GridHeight: uint16((h + cellSize - 1) / cellSize)}
	g.Cells = make([]byte, int(g.GridWidth)*int(g.GridHeight))
	s := &Scene{MapID: mapID, Width: w, Height: h, Ground: g}
	s.SetTentWalkGrid(housing.Room(housing.DefaultRoom), nil)
	return s
}

// AC65:12 selects the native room image directly. Those image identifiers
// are not ordinary Ground.MMG map templates (FUN_00404e4c).
func TentBackground(a login.Assets, kind byte) (*surface.Surface, error) {
	return background(a, housing.Background(kind))
}

// Furniture footprints use native isometric cells. The regular pathfinder
// receives an independent walk grid, so tentative drags cannot alter it.
func (s *Scene) SetTentWalkGrid(room image.Rectangle, blocked []image.Rectangle) {
	for x := 0; x < int(s.Ground.GridWidth); x++ {
		for y := 0; y < int(s.Ground.GridHeight); y++ {
			p := housing.Cell(x*cellSize+cellSize/2, y*cellSize+cellSize/2)
			value := byte(walkBlocked)
			if p.In(room) || image.Pt(x*cellSize+cellSize/2,y*cellSize+cellSize/2).In(housing.Doorway()) {
				value = 0
				for _, r := range blocked {
					if p.In(r) {
						value = byte(walkBlocked)
						break
					}
				}
			}
			s.Ground.Cells[x*int(s.Ground.GridHeight)+y] = value
		}
	}
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
	return nil, fmt.Errorf(localization.UI("ui.world.background_s_not_found_b882d253"), name)
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
	if game.IsTentMap(mapID) {
		return NewTentScene(mapID), nil
	}
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
	out.Tiles = append([]TerrainTile(nil), s.Tiles...)
	out.Zones = append([]SoundZone(nil), s.Zones...)
	return &out, nil
}

// WarmTerrain parses the terrain export ahead of the first map change, so the
// large parse happens while the login screens load rather than on entering a
// map. It uses the same cache as groundRecord.
func WarmTerrain(a login.Assets) error {
	// Compiled installations already load an independent record per map.
	if _, err := clientfs.Stat(a.DataPath(clientruntime.GroundDir)); err == nil {
		return nil
	}
	grounds.Lock()
	defer grounds.Unlock()
	path := a.DataPath(groundExport)
	if grounds.path == path {
		return nil
	}
	raw, err := clientfs.ReadFile(path)
	if err != nil {
		return err
	}
	var doc struct {
		Entries []*groundEntry `json:"entries"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("%s: %w", groundExport, err)
	}
	grounds.entries = map[string]*groundEntry{}
	for _, e := range doc.Entries {
		grounds.entries[strings.ToLower(e.Name)] = e
	}
	grounds.path = path
	return nil
}

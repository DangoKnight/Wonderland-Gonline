package app

import (
	"fmt"
	"path/filepath"
	"time"
	"wonderland-gonline/client/wlo/cursor"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/role"
	"wonderland-gonline/client/wlo/skills"
	"wonderland-gonline/client/wlo/sprites"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/client/wlo/text"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/clientfs"
	"wonderland-gonline/internal/clientimage"
)

// Resources belongs to a workspace. Sessions use its immutable definitions and
// shared decoded caches; all UI/state mutations run serially on the game thread.
// Only the sprite manager's native decoder works asynchronously, under its lock.
type Resources struct {
	root, spriteRoot string
	ready            bool
	pics             *picdb.DB
	font             *clientassets.Font
	textRenderer     *text.Renderer
	cursors          map[cursor.Shape]*cursor.Animation
	sprites          *sprites.Manager
	library          *role.Library
	items            map[uint16]assets.NativeItem
	formula          *login.Formula
	skillCatalog     *skills.Catalog
	background       *surface.Surface
	audio            *SoundCache
	scenes           bool
	sceneNames       map[uint16]string
	mapScenes        map[uint16]uint16
	sceneWeather     map[uint16]byte
	sceneMusic       map[uint16]string
	npcTemplates     map[uint32]world.NPCTemplate
	talks            map[uint16]string
}

func (r *Resources) prepare(a login.Assets, spriteRoot string) error {
	if r.ready {
		if r.root != a.Root || r.spriteRoot != spriteRoot {
			return fmt.Errorf("shared resources cannot mix asset roots")
		}
		return nil
	}
	r.root, r.spriteRoot = a.Root, spriteRoot
	r.pics = picdb.New()
	r.pics.LoadDir(a.MediaPath("menu", "skins", "default"), false, 0)
	r.pics.LoadDir(a.MediaPath("menu", "Skins", "white"), true, 0)
	var err error
	if r.font, err = clientassets.LoadFontAtlas(a.MediaPath("font", "TATPC1_TWN")); err != nil {
		return err
	}
	r.textRenderer = &text.Renderer{Font: r.font}
	cursors, err := cursor.LoadAll(a.MediaPath("cursor"), time.Now())
	if err != nil {
		return err
	}
	r.cursors = cursors.Shapes
	for _, name := range []string{world.ShadowPicture, world.MonsterShadowPicture, world.DoorLightPicture, world.SmallDoorLightPicture} {
		if compiled, err := a.CompiledPicture(shadowArchive, name); err == nil {
			if err = r.pics.AddCompiled(name, compiled); err != nil {
				return err
			}
			continue
		}
		m, err := a.LoadPicture(shadowArchive, name)
		if err != nil {
			return err
		}
		r.pics.Add(name, m)
	}
	if compiled, err := a.CompiledPicture("LogPic1"); err == nil {
		b := compiled.Bounds()
		pixels, e := compiled.Pixels(b, clientimage.Plain)
		if e != nil {
			return e
		}
		r.background = &surface.Surface{W: b.Dx(), H: b.Dy(), Pix: pixels}
	} else if m, err := a.LoadPicture("LogPic1"); err == nil {
		r.background = surface.FromImage(m)
	}
	if r.formula, err = login.LoadFormula(a); err != nil {
		return err
	}
	r.sprites = sprites.NewManager([]string{spriteRoot, filepath.Join(a.Data, "sprites")}, []string{spriteRoot, filepath.Join(a.Data, "sprites")})
	r.library = role.NewLibraryWith(r.sprites)
	if raw, err := clientfs.ReadFile(a.DataPath(itemExport)); err == nil {
		r.items, _ = assets.ParseItemCatalogJSON(raw)
	}
	r.skillCatalog, _ = skills.Load(a)
	r.audio = &SoundCache{}
	r.ready = true
	return nil
}
func (r *Resources) loadScenes(a login.Assets) {
	if r.scenes {
		return
	}
	r.scenes = true
	r.sceneNames, _ = world.SceneNames(a)
	r.mapScenes, _ = world.MapScenes(a)
	r.sceneWeather, _ = world.SceneWeather(a)
	r.sceneMusic, _ = world.SceneMusic(a)
}

// Close is called once after every session has stopped using the workspace.
func (r *Resources) Close() error {
	if r.sprites != nil {
		r.sprites.WaitNative()
		return r.sprites.Close()
	}
	return nil
}

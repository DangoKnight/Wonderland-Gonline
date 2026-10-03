package sprites

import (
	"image"
	"log"
	"os"
	"path/filepath"
	"time"

	"wonderland-go/internal/clientassets"
	"wonderland-go/internal/spritepack"
)

// Palette indices for the editable export. Its PNG sheets carry colours
// only, but each archive keeps its original decoded bytes in sprites.json;
// a frame whose pixels still match the original gets that frame's indices,
// so characters can be recoloured without built packs. An edited frame
// keeps its painted colours, as in packs built from the export.
//
// Opening sprites.json decompresses the whole archive (about 0.7 s for
// 001), so it happens in the background on the archive's first use; until
// it is ready frames draw as painted. Sprites are then matched one at a
// time when first asked for.

// nativeSource is an editable archive's sprites.json.
type nativeSource struct {
	path    string
	opening bool
	json    *clientassets.SpriteJSON
	failed  bool
}

// attachNative gives an editable archive its sprites.json, if present.
func (a *Archive) attachNative(dir string) {
	path := filepath.Join(dir, spritepack.NativeFile)
	if _, err := os.Stat(path); err == nil {
		a.native = &nativeSource{path: path}
	}
}

// derive matches s's frames against the original, once the archive's
// sprites.json is open. The manager's lock is held.
func (a *Archive) derive(s *Sprite) {
	n := a.native
	if n == nil || s.derived || n.failed {
		return
	}
	if n.json == nil {
		if !n.opening {
			n.opening = true
			go a.openNative(n)
		}
		return
	}
	s.derived = true
	if s.Index < 0 || s.Index >= len(n.json.Archive.Entries) {
		return
	}
	decoded, err := n.json.Archive.Sprite(s.Index)
	if err != nil || decoded == nil {
		return
	}
	pal := decoded.Palette
	indexed := false
	for i := range s.Frames {
		f := &s.Frames[i]
		if f.page == nil || f.index != nil || i >= len(decoded.Frames) {
			continue
		}
		img, err := a.m.loadPage(f.page)
		if err != nil {
			continue
		}
		sub, ok := img.(interface {
			SubImage(image.Rectangle) image.Image
		})
		if !ok || !f.rect.In(img.Bounds()) {
			continue
		}
		if ix := spritepack.MatchIndices(sub.SubImage(f.rect), decoded.Frames[i], &pal); ix != nil {
			f.indices, indexed = ix, true
		}
	}
	if indexed && s.Palette == nil {
		s.Palette = &pal
	}
}

func (a *Archive) openNative(n *nativeSource) {
	j, err := clientassets.OpenSpriteJSON(n.path)
	m := a.m
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		log.Printf("sprites %s: palette indices unavailable: %v", a.Name, err)
		n.failed = true
		return
	}
	if m.closed {
		j.Close()
		return
	}
	n.json = j
	m.natives = append(m.natives, n)
}

// Close releases the decoded originals opened for palette indices (their
// temporary files). Archives stay usable; frames not yet matched draw as
// painted.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	var first error
	for _, n := range m.natives {
		if err := n.json.Close(); err != nil && first == nil {
			first = err
		}
		n.json, n.failed = nil, true
	}
	m.natives = nil
	return first
}

// WaitNative blocks until every archive loaded so far has finished opening
// its sprites.json; tests and snapshots use it for deterministic colours.
func (m *Manager) WaitNative() {
	for {
		m.mu.Lock()
		busy := false
		for _, a := range m.archives {
			if n := a.native; n != nil && n.opening && n.json == nil && !n.failed {
				busy = true
			}
		}
		m.mu.Unlock()
		if !busy {
			return
		}
		sleepBriefly()
	}
}

func sleepBriefly() { time.Sleep(10 * time.Millisecond) }

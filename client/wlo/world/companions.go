package world

import (
	"image"
	"time"
)

const (
	// FUN_004245bc shifts five owner positions when either axis moves >19.
	companionTrailSamples = 5
	companionTrailSpacing = 20
	// FUN_00424a0c targets +0x220c/+0x2210, the third trail entry.
	companionTrailTarget      = 2
	companionTeleportDistance = walkReach * cellSize
)

type Companion struct {
	ID            uint32
	Name          []byte
	Painter       NPCPainter
	Info          NPCTemplate
	position      Player
	walker        Walker
	trail         [companionTrailSamples]image.Point
	ownerPosition image.Point
	initialized   bool
}

// SetCompanion is owner-based AC15:4/AC19:1 presentation, separate from map NPCs.
func (w *World) SetCompanion(owner, id uint32, name []byte, painter NPCPainter) {
	if w.Companions == nil {
		w.Companions = map[uint32]*Companion{}
	}
	c := w.Companions[owner]
	if c == nil || c.ID != id {
		c = &Companion{ID: id}
		w.Companions[owner] = c
		if p := w.companionOwner(owner); p != nil {
			c.reset(p)
		}
	}
	c.Name, c.Painter = append([]byte(nil), name...), painter
}

func (w *World) companionOwner(owner uint32) *Player {
	if owner == w.Player.ID {
		return &w.Player
	}
	if peer := w.Peers[owner]; peer != nil {
		return &peer.Player
	}
	return nil
}

func (c *Companion) reset(owner *Player) {
	c.position = Player{X: owner.X, Y: owner.Y, Direction: standingAction + owner.Direction%directionsCount}
	c.walker = Walker{}
	c.ownerPosition = image.Pt(owner.X, owner.Y)
	for i := range c.trail {
		c.trail[i] = c.ownerPosition
	}
	c.initialized = true
}

// step uses the native owner's short position history rather than a fixed
// screen offset. TFollowNpc (FUN_0041c5f0) moves with FUN_004126b8 and stops
// at the trailing point; it does not catch up to the owner's final position.
func (c *Companion) step(owner *Player, now time.Time) {
	at := image.Pt(owner.X, owner.Y)
	if !c.initialized || abs(at.X-c.ownerPosition.X) >= companionTeleportDistance || abs(at.Y-c.ownerPosition.Y) >= companionTeleportDistance {
		c.reset(owner)
	}
	c.ownerPosition = at
	c.walker.Step(&c.position, now)
	if abs(at.X-c.trail[0].X) >= companionTrailSpacing || abs(at.Y-c.trail[0].Y) >= companionTrailSpacing {
		for i := len(c.trail) - 1; i > 0; i-- {
			c.trail[i] = c.trail[i-1]
		}
		c.trail[0] = at
	}
	goal := c.trail[companionTrailTarget]
	if current, walking := c.walker.destination(); walking && current == goal {
		return
	}
	if c.position.X == goal.X && c.position.Y == goal.Y {
		c.walker.Stop(&c.position)
		return
	}
	c.walker.Start(&c.position, []image.Point{goal}, now)
}

func (w *World) companionPosition(owner uint32) (int, int, int, bool) {
	c := w.Companions[owner]
	p := w.companionOwner(owner)
	if c == nil || p == nil {
		return 0, 0, 0, false
	}
	if !c.initialized {
		c.reset(p)
	}
	return c.position.X, c.position.Y, int(c.position.Direction), true
}

// Following pets share FUN_004147f8's NPC name placement, using their own
// template and frame anchor, rather than the owner's human-name height.
func (c *Companion) nameTop() int {
	if c.Painter == nil {
		return nameTop(0)
	}
	anchor, ok := c.Painter.FirstAnchorY()
	if !ok {
		return nameTop(0)
	}
	return nameTop(nameHeight(c.Info, anchor))
}

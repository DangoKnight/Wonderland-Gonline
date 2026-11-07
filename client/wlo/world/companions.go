package world

import (
	"image"
	"time"

	"wonderland-gonline/internal/protocol"
)

const (
	// FUN_004245bc shifts five owner positions when either axis moves >19.
	companionTrailSamples = 5
	companionTrailSpacing = 20
	// FUN_00424a0c targets +0x220c/+0x2210, the third trail entry.
	companionTrailTarget      = 2
	companionTeleportDistance = walkReach * cellSize
)

// Use FUN_00430680's sitting fallbacks only when the sprite lacks the exact
// facing. Its MoveKind check concerns a rider's pose, not a following pet.
var companionSittingFallbacks = [...]int{47, 47, 49, 49, 51, 51, 51, 53}

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
	posed         bool
	facing        int // Preserve the pet's facing across reduced-direction poses.
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
	action := PoseAction(standingAction, int(owner.Direction))
	c.position = Player{X: owner.X, Y: owner.Y, Direction: int32(action)}
	c.facing = action - standingAction
	c.walker = Walker{}
	c.posed = false
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
	if owner.VehicleID != 0 {
		// Retain selection, but discard the sailing trail. On landing the pet
		// starts beside its owner instead of walking across the water to catch up.
		c.initialized = false
		return
	}
	at := image.Pt(owner.X, owner.Y)
	if !c.initialized || abs(at.X-c.ownerPosition.X) >= companionTeleportDistance || abs(at.Y-c.ownerPosition.Y) >= companionTeleportDistance {
		c.reset(owner)
	}
	c.ownerPosition = at
	// Mirror the owner's pose family through normal AC32:2 state, retaining
	// the pet's own facing. Owner-only turns must not rotate the pet.
	if !c.posed {
		c.facing = PoseAction(standingAction, int(c.position.Direction)) - standingAction
	}
	if pose := ActionBase(int(owner.Direction)); pose > standingAction {
		action := PoseAction(pose, standingAction+c.facing)
		supported := c.supportsPose(action)
		if !supported && action >= protocol.PoseSitFirst && action <= protocol.PoseSitLast {
			action = companionSittingFallbacks[action-protocol.PoseSitFirst]
			supported = c.supportsPose(action)
		}
		if supported {
			c.walker.Stop(&c.position)
			c.position.Direction = int32(action)
			c.posed = true
			return
		}
	}
	if c.posed {
		c.position.Direction = int32(standingAction + c.facing)
		c.posed = false
	}
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

func (c *Companion) supportsPose(action int) bool {
	if painter, ok := c.Painter.(interface{ FrameSize(int) (int, int) }); ok {
		w, h := painter.FrameSize(action)
		return w > 0 && h > 0
	}
	return false
}

func (w *World) companionPosition(owner uint32) (int, int, int, bool) {
	c := w.Companions[owner]
	p := w.companionOwner(owner)
	if c == nil || p == nil {
		return 0, 0, 0, false
	}
	// FUN_004274cc hides following NPCs for both party roles. Selection and
	// combat ownership remain independent of this world presentation rule.
	if p.VehicleID != 0 || w.inTeam(owner) || w.fighting(owner) {
		c.initialized = false
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

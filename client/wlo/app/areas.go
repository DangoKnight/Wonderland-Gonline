package app

import (
	"encoding/binary"
	"image"
	"time"

	"wonderland-gonline/internal/protocol"
)

// Areas (doors and walk-in regions). Once the map is ready (+0x133d0, set
// by 5/4), FUN_00303ff4 looks at the player's
// position every 200 ms, or sooner once it has moved 21 pixels; with no
// event in progress, FUN_0030410c sends 20/8 with the ID of the first area
// with events that holds the player, unless it is the area entered last
// (player +0x3aa6, forgotten once the player is outside every area). The
// step then waits for the server (+0x70e8, +0x7109): a teleport, or 20/8
// back when nothing happens. The player stops (+0x2350) and the pointer
// returns to the arrow (FUN_003bac58). Not ported: a door's FUN_003ba9d8
// (…, 10) and FUN_00173774.
const (
	areaCheckInterval = 200 * time.Millisecond
	areaCheckMove     = 0x15
)

// areaWatch is FUN_00303ff4's state.
type areaWatch struct {
	at   time.Time // +0x7158
	x, y int       // +0x7160, +0x7164
	last uint16    // player +0x3aa6
}

// areaTick is FUN_00303ff4's area part.
func (c *Client) areaTick() {
	if c.World == nil || c.movie != nil || !c.mapReady {
		return
	}
	now, p := c.Now(), c.World.Player
	w := &c.areas
	elapsed := now.Sub(w.at) >= areaCheckInterval
	if elapsed {
		w.at = now
	}
	if !elapsed && abs(p.X-w.x)+abs(p.Y-w.y) < areaCheckMove {
		return
	}
	w.x, w.y = p.X, p.Y
	if c.event.active {
		return
	}
	inside := false
	for _, a := range c.World.Areas {
		if a.Events == 0 || !image.Pt(p.X, p.Y).In(a.Rect) {
			continue
		}
		inside = true
		if a.ID == w.last {
			continue
		}
		w.last = a.ID
		c.enterArea(a.ID)
		return
	}
	if !inside {
		w.last = 0
	}
}

// enterArea sends 20/8 for an area and waits for the server.
func (c *Client) enterArea(id uint16) {
	c.World.StopWalk()
	c.pendingNPC = nil
	c.event = eventState{active: true}
	c.Net.Send(binary.LittleEndian.AppendUint16([]byte{protocol.CommandEvent, protocol.EventPortal}, id))
}

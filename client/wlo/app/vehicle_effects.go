package app

import (
	"encoding/binary"
	"image"
	"strconv"
	"time"
	"wonderland-gonline/client/wlo/minigame"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

// FUN_00449f80 -> FUN_003c9284: three rows, one playback at 70 ms.
const vehicleBreakFrameInterval = 70 * time.Millisecond
const vehicleBreakRows = 3
const defaultVehicleBreakItem = 48005

type vehicleBreakEffect struct {
	effect  minigame.Effect
	mapID   uint16
	started time.Time
}
type vehicleRecoveryPosition struct {
	point image.Point
	mapID uint16
	at    time.Time
}
type vehicleEffectState struct {
	effects []vehicleBreakEffect
	origins map[uint32]vehicleRecoveryPosition
}

// AC7 can rescue a mounted rider before AC15:15 arrives. Keep the old
// water position for that animation without moving the rescued character back.
func (c *Client) rememberVehiclePosition(owner uint32) {
	if c.World == nil {
		return
	}
	w := c.World
	var point image.Point
	if owner == w.Player.ID {
		if w.Player.VehicleID == 0 {
			return
		}
		point = image.Pt(w.Player.X, w.Player.Y)
	} else {
		peer := w.Peers[owner]
		if peer == nil || peer.VehicleID == 0 {
			return
		}
		point = image.Pt(peer.X, peer.Y)
	}
	if c.vehicleEffects.origins == nil {
		c.vehicleEffects.origins = map[uint32]vehicleRecoveryPosition{}
	}
	c.vehicleEffects.origins[owner] = vehicleRecoveryPosition{point, w.Player.Map, c.Now()}
}

func (c *Client) breakVehicle(owner uint32, id uint16) {
	w := c.World
	if w == nil || c.Pics == nil {
		return
	}
	point := image.Pt(w.Player.X, w.Player.Y)
	if owner != w.Player.ID {
		peer := w.Peers[owner]
		if peer == nil {
			return
		}
		point = image.Pt(peer.X, peer.Y)
	}
	if origin, ok := c.vehicleEffects.origins[owner]; ok {
		delete(c.vehicleEffects.origins, owner)
		if origin.mapID == w.Player.Map && c.Now().Sub(origin.at) <= waterVehicleReplyTimeout {
			point = origin.point
		}
	}
	// FUN_00449f80 normalizes capsules, maps Robinson's raft to 48010_B,
	// and falls back to 48005_B when the vehicle's strip is unavailable.
	id = game.BaseVehicleID(id)
	if id == game.DisposableRaftItemID {
		id = game.RaftItemID
	}
	name := strconv.Itoa(int(id)) + "_B"
	if !c.loadVehicleBreakPicture(name) {
		name = strconv.Itoa(defaultVehicleBreakItem) + "_B"
		if !c.loadVehicleBreakPicture(name) {
			return
		}
	}
	e := vehicleBreakEffect{mapID: w.Player.Map, started: c.Now()}
	e.effect.Play(name, vehicleBreakFrameInterval, minigame.LevelKeyed, point.X, point.Y, vehicleBreakRows, vehicleBreakRows, 1, c.Now())
	c.vehicleEffects.effects = append(c.vehicleEffects.effects, e)
	// Native receipt sent when destruction plays; the server accepts the owner ID.
	if c.Inventory != nil && c.Inventory.Send != nil && owner == w.Player.ID {
		p := make([]byte, 6)
		p[0], p[1] = protocol.CommandPetControl, protocol.PetControlVehicleNoOp
		binary.LittleEndian.PutUint32(p[2:], owner)
		c.Inventory.Send(p)
	}
}

func (c *Client) loadVehicleBreakPicture(name string) bool {
	if c.Pics.Find(name) >= 0 {
		return true
	}
	if compiled, err := c.Assets.CompiledPicture("images", name); err == nil {
		return c.Pics.AddCompiled(name, compiled) == nil
	}
	if img, err := c.Assets.LoadPicture("images", name); err == nil {
		c.Pics.Add(name, img)
		return true
	}
	return false
}

func (c *Client) drawVehicleEffects() {
	if c.World == nil {
		return
	}
	cx, cy := c.World.Camera()
	effects := c.vehicleEffects.effects[:0]
	for _, e := range c.vehicleEffects.effects {
		if e.mapID != c.World.Player.Map || c.Now().Sub(e.started) >= vehicleBreakRows*vehicleBreakFrameInterval {
			continue
		}
		e.effect.X -= cx
		e.effect.Y -= cy
		e.effect.Draw(c.Screen, c.Pics, c.Now())
		e.effect.X += cx
		e.effect.Y += cy
		if e.effect.Active() {
			effects = append(effects, e)
		}
	}
	c.vehicleEffects.effects = effects
	for owner, origin := range c.vehicleEffects.origins {
		if c.Now().Sub(origin.at) > waterVehicleReplyTimeout {
			delete(c.vehicleEffects.origins, owner)
		}
	}
}

package app

import (
	"encoding/binary"
	"time"

	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/role"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/protocol"
)

// 8/1 after its command byte: subcommand, stat ID, kind, value.
const statUpdateBytes = 7

// AC12 from the server places a character on a map: ID, map, X, Y, then
// the portal (receive case 0x2e20cf).
const warpPacketBytes = 10

// loadNPCs creates the map's NPC objects (FUN_003030ec). A map without
// event data simply has none.
func (c *Client) loadNPCs(w *world.World) {
	if c.npcTemplates == nil {
		if c.resources.npcTemplates == nil {
			c.resources.npcTemplates, _ = world.NPCTemplates(c.Assets)
		}
		c.npcTemplates = c.resources.npcTemplates
	}
	rec, err := world.MapRecord(c.Assets, w.Player.Map)
	if err != nil {
		w.NPCs = nil
		return
	}
	var paint func(world.NPCTemplate) world.NPCPainter
	if c.lib != nil {
		paint = func(t world.NPCTemplate) world.NPCPainter {
			npc := role.NewNPC(c.lib, t.Look, t.Colors)
			npc.SetHeightScale(t.HeightScale)
			return npc
		}
	}
	w.NPCs = world.MapNPCs(rec, c.npcTemplates, paint)
	w.Questions = world.MapQuestions(rec)
	w.Areas = world.MapAreas(rec)
}

// warp is AC12 for the player: the map loads (again, when it changed) at
// the given position, then FUN_002f2fd0 tells the server the map is
// loaded with 12/1.
func (c *Client) warp(p []byte) {
	if c.World == nil || len(p) < warpPacketBytes {
		return
	}
	if id := binary.LittleEndian.Uint32(p); id != c.World.Player.ID {
		// Another player placed on a map: gone unless it is ours; map 0
		// means the player left the game.
		if m := binary.LittleEndian.Uint16(p[4:]); m != c.World.Player.Map {
			c.World.RemovePeer(id)
			if m == 0 {
				delete(c.players, id)
			}
		} else {
			c.World.PlacePeer(id, int(binary.LittleEndian.Uint16(p[6:])), int(binary.LittleEndian.Uint16(p[8:])))
		}
		return
	}
	// Loading a map forgets the area last entered (FUN_00304898), and the
	// map is not ready until 5/4.
	c.areas.last = 0
	c.mapReady = false
	if c.Settings != nil {
		c.Settings.Hide()
	}
	c.waterTravel = waterTravelState{}
	c.vehicleEffects = vehicleEffectState{}
	pl := c.World.Player
	pl.Map = binary.LittleEndian.Uint16(p[4:])
	pl.X, pl.Y = int(binary.LittleEndian.Uint16(p[6:])), int(binary.LittleEndian.Uint16(p[8:]))
	if pl.Map != c.World.Player.Map {
		c.stopAmbience() // FUN_004057c8(-1, -1)
		var body login.RoleView
		if c.lib != nil {
			body = role.NewHuman(c.lib, c.items)
		}
		w, err := world.New(c.Env, c.Assets, pl, body, c.sceneNames, func(m uint16) uint16 { return c.mapScenes[m] })
		if err != nil {
			c.Notices.Show([]byte(err.Error()), 3*time.Second, c.Now())
			return
		}
		c.loadNPCs(w)
		c.attachWeather(w)
		w.OnLeg = c.sendLeg
		w.Now = func() time.Time { return c.Now() }
		c.World = w
		c.applyWaterVehicle()
		c.applyLocalSettings()
		c.playMapMusic()
		c.MainStatus.Portrait = w.Body
	} else {
		c.World.Player = pl
	}
	c.Net.Send([]byte{protocol.CommandMapAcknowledgment, protocol.MapAcknowledgmentMapLoaded})
}

// Movement requests: 6/1 is the facing and the waypoint, then 8 bytes the
// original fills with an anti-cheat checksum (the send case at 0x2c33cc,
// built from the role's timing table +0x3ae0). The server reads only the
// first five, so they are zero here.
const movementCheckBytes = 8

// Walk re-planning while a button or an arrow key is held: every 400 ms
// (FUN_004a4248's FUN_00403954(400)); an arrow key aims 0x50 pixels away
// on its axis.
const (
	walkRepeat   = 400 * time.Millisecond
	walkKeyReach = 0x50
)

// GroundClick is a left click that no control took: on the map it starts
// a walk (FUN_0043bc70), and holding the button keeps walking toward the
// pointer.
func (c *Client) GroundClick(x, y int) {
	if c.World == nil || c.sceneFrozen() || c.settingsPromptForm != nil {
		return
	}
	if c.movie != nil {
		// In a movie a click only closes the shown line.
		if c.Talk.Shown() {
			c.Talk.Hide()
		}
		return
	}
	if c.battleClick(x, y) {
		return
	}
	if c.sportClick(x, y) {
		return
	}
	if c.Talk.Choosing() {
		c.pickAnswer(x, y)
		return
	}
	if c.Talk.Shown() {
		c.advanceTalk()
		return
	}
	if c.held || c.event.active {
		return
	}
	if c.teamTargetAt(x, y) {
		return
	}
	if n := c.World.NPCAt(x, y); n != nil {
		c.waterTravel.goal = nil
		c.clickNPC(n)
		return
	}
	c.pendingNPC = nil
	c.groundHeld, c.groundSince = true, c.Now()
	c.walkTo(x, y)
}

// walkTo plans a walk to a screen point and restarts the repeat clock.
func (c *Client) walkTo(x, y int) {
	c.walkToward(x, y, true)
}

// walkToward plans the walk; mouse walks show the walk marker at its
// destination (0x4a1d60), arrow-key walks don't.
func (c *Client) walkToward(x, y int, marked bool) {
	cx, cy := c.World.Camera()
	now := c.Now()
	c.nextWalk = now.Add(walkRepeat)
	c.walkWaterAware(x+cx, y+cy, now)
	if marked {
		c.World.MarkWalk()
	}
}

// GroundHold is called while the left button stays down: a press that
// started a walk re-aims it at the pointer every 400 ms. Release ends it.
func (c *Client) GroundHold(held bool, x, y int) {
	if !held || c.battle.state.Active || c.sport != nil || c.settingsPromptForm != nil {
		c.groundHeld, c.groundSince = false, time.Time{}
		return
	}
	if c.groundHeld && c.World != nil && !c.held && !c.event.active && !c.Now().Before(c.nextWalk) {
		c.walkTo(x, y)
	}
}

// WalkKeys is FUN_004a4248: with the arrow keys held, and no text field
// focused, the player walks 0x50 pixels from its position on each held
// axis, re-planned every 400 ms.
func (c *Client) WalkKeys(left, up, right, down bool) {
	if c.battle.state.Active || c.settingsPromptForm != nil || c.sport != nil || c.World == nil || c.held || c.event.active || !(left || up || right || down) || c.Now().Before(c.nextWalk) {
		return
	}
	if _, typing := c.Input.Focused.(*seui.Editor); typing {
		return
	}
	cx, cy := c.World.Camera()
	x, y := c.World.Player.X-cx, c.World.Player.Y-cy
	if left {
		x -= walkKeyReach
	}
	if right {
		x += walkKeyReach
	}
	if up {
		y -= walkKeyReach
	}
	if down {
		y += walkKeyReach
	}
	c.walkToward(x, y, false)
}

// sendLeg is 6/1 for a leg of the player's walk.
func (c *Client) sendLeg(facing, x, y int) {
	p := []byte{protocol.CommandMovement, protocol.MovementMove, byte(facing)}
	p = binary.LittleEndian.AppendUint16(p, uint16(x))
	p = binary.LittleEndian.AppendUint16(p, uint16(y))
	c.Net.Send(append(p, make([]byte, movementCheckBytes)...))
}

// peerAppears is AC4: another player on the map, dressed like the player.
func (c *Client) peerAppears(s []byte) {
	if c.World == nil {
		return
	}
	p, err := world.ParseOther(s)
	if err != nil {
		return
	}
	c.rememberPlayer(p.ID, p.Name)
	var newBody func() login.RoleView
	if c.lib != nil {
		newBody = func() login.RoleView { return role.NewHuman(c.lib, c.items) }
	}
	c.World.AddPeer(p, newBody)
	c.rememberTeammate(p)
}

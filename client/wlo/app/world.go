package app

import (
	"encoding/binary"
	"image"
	"maps"
	"time"

	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/role"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/game"
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
	if c.lib != nil {
		w.BattleCloud = c.lib.DrawBattleCloud
	}
	if c.npcTemplates == nil {
		if c.resources.npcTemplates == nil {
			c.resources.npcTemplates, _ = world.NPCTemplates(c.Assets)
			c.resources.npcTemplates = maps.Clone(c.resources.npcTemplates)
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
	w.Mines = world.MapMines(rec)
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
	c.resetServices()
	c.closeMessageBoard()
	if c.Notebook != nil {
		c.Notebook.CloseAll()
	}
	if c.starBook != nil {
		c.starBook.Hide()
	}
	c.resetGroundHold()
	c.setPKTarget(false)
	c.World.StopWalk()
	c.World.Marker.Hide()
	// Loading a map forgets the area last entered (FUN_00304898), and the
	// map is not ready until 5/4.
	c.areas.last = 0
	c.vacuum = vacuumRuntime{}
	// Ground packets are sparse updates (including individual respawns). Never
	// carry a previous visit's items into the new authoritative map snapshot,
	// even when no ground packets arrive on the intervening map.
	c.floorItems = nil
	c.floorMap = 0
	c.floorPendingPickup, c.floorHovered = 0, 0
	c.worldItemFlights = nil
	c.acquisitionActor = 0
	c.mapReady = false
	if c.LuckyDraw != nil {
		c.LuckyDraw.Hide()
	}
	if c.WorldPK != nil {
		c.WorldPK.Hide()
	}
	if c.Mall != nil {
		c.Mall.Hide()
	}
	if c.Settings != nil {
		c.Settings.Hide()
	}
	c.waterTravel = waterTravelState{}
	c.placedVehicles = nil
	c.vehicleEffects = vehicleEffectState{}
	pl := c.World.Player
	mapChanged := binary.LittleEndian.Uint16(p[4:]) != pl.Map
	if mapChanged {
		c.closeTeamTransfer()
		c.startMapLoading()
	}
	pl.Map = binary.LittleEndian.Uint16(p[4:])
	c.resetTentScene()
	if game.IsTentMap(pl.Map) {
		c.tentWarpMetadata(p)
	}
	pl.X, pl.Y = int(binary.LittleEndian.Uint16(p[6:])), int(binary.LittleEndian.Uint16(p[8:]))
	if pl.Map != c.World.Player.Map || game.IsTentMap(pl.Map) {
		c.stopAmbience() // FUN_004057c8(-1, -1)
		var body login.RoleView
		if c.lib != nil {
			body = role.NewHuman(c.lib, c.items)
		}
		w, err := world.New(c.Env, c.Assets, pl, body, c.sceneNames, func(m uint16) uint16 { return c.mapScenes[m] })
		if err != nil {
			c.stopMapLoading()
			c.Notices.Show([]byte(err.Error()), 3*time.Second, c.Now())
			return
		}
		c.loadNPCs(w)
		c.attachWeather(w)
		w.OnLeg = c.sendLeg
		w.Now = func() time.Time { return c.Now() }
		c.World = w
		if c.TeamState != nil && len(c.TeamState.Members) > 1 {
			w.SetTeamRoster(c.TeamState.Members)
		}
		c.applyWaterVehicle()
		c.applyLocalSettings()
		c.eventMusic = "" // FUN_002f2fd0 clears the event's track
		c.playMapMusic()
		c.setupMiniMap()
		c.MainStatus.Portrait = w.Body
	} else {
		c.World.Player = pl
		c.World.Relocate(image.Pt(pl.X, pl.Y))
	}
	if mapChanged {
		c.queueAreaTitle()
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
	walkRepeat     = 400 * time.Millisecond
	walkHoldRepeat = 100 * time.Millisecond
	walkKeyReach   = 0x50
)

// GroundClick is a left click that no control took: on the map it starts
// a walk (FUN_0043bc70), and holding the button keeps walking toward the
// pointer.
func (c *Client) GroundClick(x, y int) {
	if c.afkClick(x, y) {
		return
	}
	if c.exitMovementMode() {
		return
	}
	if c.Notices.dismissAt(c.Env, c.Now(), x, y) {
		return
	}
	if c.World == nil || c.sceneFrozen() || !c.clinicUntil.IsZero() || c.Services != nil && c.Services.Visible || c.settingsPromptForm != nil {
		return
	}
	if c.movie != nil {
		// In a movie a click only closes the shown line.
		if c.Talk.Shown() {
			c.advanceTalk()
		}
		return
	}
	if c.battleClick(x, y) {
		return
	}
	if c.sportClick(x, y) {
		return
	}
	if c.Talk.Choosing() && c.pickConfirm(x, y) {
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
	x, y = c.World.ScenePoint(x, y)
	// Target modes own the click even over another kind of field object.
	if c.battleEntryTargetAt(x, y) || c.pkTargetAt(x, y) || c.teamTargetAt(x, y) || c.friendTargetAt(x, y) || c.tradeTargetAt(x, y) {
		return
	}
	if c.boardPlacedVehicle(x, y) {
		return
	}
	if c.clickTent(x, y) {
		return
	}
	if c.pickupFloorItem(x, y) {
		return
	}
	c.floorPendingPickup = 0
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
	now := c.Now()
	// Consume elapsed movement before replacing its route. Otherwise every
	// held replan discards one update's travel and produces a periodic hitch.
	c.World.Step(now)
	cx, cy := c.World.Camera()
	c.nextWalk = now.Add(walkRepeat)
	if c.groundHeld {
		c.nextWalk = now.Add(walkHoldRepeat)
	}
	c.walkWaterAware(x+cx, y+cy, now)
	if marked {
		c.World.MarkWalk()
		c.World.Marker.SetHeld(c.groundHeld, now)
	}
}

const movementModeHoldDuration = 3 * time.Second

// resetGroundHold also drops latched movement when another action takes control.
func (c *Client) resetGroundHold() {
	c.groundHeld, c.groundSince, c.movementMode = false, time.Time{}, false
	if c.World != nil {
		c.World.SyncHeldMarker()
		c.World.Marker.SetHeld(false, c.Now())
	}
}

// exitMovementMode consumes its cancelling click, including clicks over UI.
func (c *Client) exitMovementMode() bool {
	if !c.movementMode {
		return false
	}
	c.resetGroundHold()
	if c.World != nil {
		c.World.StopWalk()
	}
	return true
}

// GroundHold re-aims a ground walk. After three seconds it stays active on release;
// only movement is repeated, never the entity interaction under the pointer.
func (c *Client) GroundHold(held bool, x, y int) {
	if c.holdTentFurniture(held, x, y) {
		return
	}
	c.sportRelease(held, x, y)
	if c.mapLoading.active || c.battle.state.Active || c.sport != nil || c.settingsPromptForm != nil || c.World == nil || c.held || c.event.active || c.sceneFrozen() || c.Talk.Shown() || !c.clinicUntil.IsZero() || c.Services != nil && c.Services.Visible {
		c.resetGroundHold()
		return
	}
	if c.groundHeld && held && c.Now().Sub(c.groundSince) >= movementModeHoldDuration {
		c.movementMode = true
	}
	if !held && !c.movementMode {
		c.resetGroundHold()
		return
	}
	if c.groundHeld && c.World != nil && !c.held && !c.event.active {
		x, y = c.World.ScenePoint(x, y)
		if !c.Now().Before(c.nextWalk) {
			c.walkTo(x, y)
		}
		c.World.TrackHeldMarker(x, y)
	}
}

// WalkKeys is FUN_004a4248: with the arrow keys held, and no text field
// focused, the player walks 0x50 pixels from its position on each held
// axis, re-planned every 400 ms.
func (c *Client) WalkKeys(left, up, right, down bool) {
	if c.mapLoading.active || c.battle.state.Active || c.settingsPromptForm != nil || c.sport != nil || c.World == nil || c.held || c.event.active || !(left || up || right || down) || c.Now().Before(c.nextWalk) {
		return
	}
	if c.MailSend != nil && c.MailSend.Visible || c.MailCompose != nil && c.MailCompose.Visible {
		return
	}
	switch c.Input.Focused.(type) {
	case *seui.Editor, *seui.Memo:
		return
	}
	c.floorPendingPickup = 0
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
	if c.World != nil && c.World.FollowingTeam() {
		return
	}
	c.stopVacuum()
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

package app

import (
	"encoding/binary"
	"fmt"
	"image"
	"strconv"
	"time"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

const (
	remoteTickInterval       = 250 * time.Millisecond
	remoteWalkPause          = time.Second
	remoteRequestTimeout     = 5 * time.Second
	remoteWanderRadius       = 160 // Go fallback: native offset-table distances are not verified.
	remoteBasicAttack        = 10001
	remoteBattleAttack       = 1
	remoteFighterRecordBytes = 32
	remoteFighterPlayer      = 2
	remoteFighterPet         = 4
	remoteFighterMonster     = 7
	remoteBattleStatCommand  = 51
	remoteBattleStatValue    = 1
	remoteHPStatus           = 25
	remoteSPStatus           = 26
	remoteRecoveryBase       = 100
	remotePercentBase        = 100
	remoteControllerItem     = 34058
	remoteStatusX            = 5
	remoteStatusY            = 200
	remoteStatusLine         = 15
)

// remoteRuntime owns transient client automation. Every resource request waits
// for an authoritative receipt; missing receipts stop automation without replay.
type remoteRuntime struct {
	active                    bool
	options                   inventory.RemoteOptions
	start, nextTick, nextWalk time.Time
	anchor                    image.Point
	mapID                     uint16
	direction                 int
	playerDeaths, petDeaths   int
	pending                   *remoteRequest
	battle                    remoteBattle
	activePet                 uint32
	button                    *seui.FixedButton
}
type remoteRequest struct {
	at           time.Time
	slot, target byte
	item         game.Item
	equipment    bool
	supply, ack  bool
}
type remoteFighter struct {
	side, kind, x, y byte
	id, owner        uint32
	hp, maxHP        uint32
	countedDeath     bool
	sp, maxSP        uint16
}
type remoteBattle struct {
	active, ready, prompt bool
	fighters              []remoteFighter
}

func (c *Client) startRemote(o inventory.RemoteOptions) bool {
	if o.SupplyFirst < 1 || o.SupplyLast < o.SupplyFirst || o.SupplyLast > game.BagSize {
		return false
	}
	for _, percent := range o.Thresholds {
		if percent < 0 || percent > remotePercentBase {
			return false
		}
	}
	if (o.LeaveAfter && o.LeaveMinutes <= 0) || (o.LeavePlayerDeaths && o.PlayerDeaths <= 0) || (o.LeavePetDeaths && o.PetDeaths <= 0) {
		return false
	}
	if c.World == nil || !c.mapReady || c.sceneFrozen() || c.held || c.event.active || c.petAnnouncement || c.sport != nil || c.movie != nil || c.Stats.HP == 0 {
		return false
	}
	owned := false
	for _, item := range c.InventoryState.Bag {
		if item.ID == remoteControllerItem && !item.Empty() && !item.Locked {
			owned = true
			break
		}
	}
	if !owned {
		c.Chat.Notice("Remote Control is no longer available.")
		return false
	}
	if c.remote.pending != nil {
		c.Chat.Notice("Wait for the pending Remote request.")
		return false
	}
	battle, pet, button := c.remote.battle, c.remote.activePet, c.remote.button
	c.remote = remoteRuntime{active: true, options: o, start: c.Now(), anchor: image.Pt(c.World.Player.X, c.World.Player.Y), mapID: c.World.Player.Map, battle: battle, activePet: pet, button: button}
	c.groundHeld = false
	if c.World.Walking() {
		c.World.StopWalk()
	}
	c.remoteButton()
	return true
}
func (c *Client) stopRemote() {
	wasActive := c.remote.active
	c.remote.active = false
	if c.remote.button != nil {
		c.remote.button.Hide()
	}
	if wasActive && c.World != nil {
		c.World.StopWalk()
	}
}
func (c *Client) remoteButton() {
	if c.remote.button == nil {
		b := seui.NewFixedButton(c.Env, nil)
		b.Init("", ScreenWidth-42, 0, 0, 0, 0, false, 32, 32, ScreenHeight-110)
		b.OnTop = true
		b.OnClick = func() {
			for i, item := range c.InventoryState.Bag {
				if item.ID == remoteControllerItem {
					c.Inventory.OpenRemote(byte(i + 1))
					return
				}
			}
		}
		c.UI.Add(b)
		c.remote.button = b
	}
	b := c.remote.button
	name := strconv.Itoa(int(c.items[remoteControllerItem].Icon))
	c.loadPictures(name)
	// A single-frame animation paints at the button origin; native Icon
	// drawing adds Left/Top twice for controls away from their parent origin.
	b.SetAnimation(name, 0, 1, 1, time.Hour, 32, 32, 0)
	b.StartAnimation()
	b.Show()
}
func (c *Client) remoteTick() {
	r := &c.remote
	if c.World == nil {
		return
	}
	now := c.Now()
	if r.pending != nil {
		p := r.pending
		current := c.InventoryState.Bag[p.slot-1]
		if p.equipment {
			current = c.InventoryState.Equipment[p.slot-1]
			if p.target != 0 {
				current = c.InventoryState.Pets[p.target-1].Equipment[p.slot-1]
			}
		}
		if current != p.item && (!p.supply || p.ack) {
			r.pending = nil
		} else if now.Sub(p.at) >= remoteRequestTimeout {
			c.stopRemote()
			r.pending = nil
			c.Chat.Notice("Remote stopped: no item request receipt. No automatic retry was sent.")
			return
		}
	}

	if !r.active {
		return
	}
	owned := false
	for _, item := range c.InventoryState.Bag {
		if item.ID == remoteControllerItem && !item.Empty() && !item.Locked {
			owned = true
			break
		}
	}
	if !owned {
		c.stopRemote()
		c.Chat.Notice("Remote stopped: controller unavailable.")
		return
	}
	if c.World.Player.Map != r.mapID {
		c.stopRemote()
		c.Chat.Notice("Remote stopped after changing maps.")
		return
	}

	o := r.options
	if o.LeaveAfter && now.Sub(r.start) >= time.Duration(o.LeaveMinutes)*time.Minute {
		c.remoteLeave("Remote leave timer reached.")
		return
	}
	if (o.LeavePlayerDeaths && r.playerDeaths >= o.PlayerDeaths) || (o.LeavePetDeaths && r.petDeaths >= o.PetDeaths) {
		c.remoteLeave("Remote death limit reached.")
		return
	}
	if now.Before(r.nextTick) || c.sceneFrozen() || c.movie != nil || c.sport != nil || c.UI.Modal != nil {
		return
	}
	r.nextTick = now.Add(remoteTickInterval)
	if r.battle.active {
		if r.options.AutoFight && r.battle.ready {
			c.remoteAttack()
		}
		return
	}
	if !c.mapReady || c.held || c.event.active || c.petAnnouncement || c.settingsPromptForm != nil || r.pending != nil {
		return
	}
	if r.options.AutoSupply && c.remoteSupply() {
		return
	}
	if r.options.AutoUnequip && c.remoteUnequip() {
		return
	}
	if r.options.AutoDiscard && c.remoteDiscard() {
		return
	}
	if !r.options.AutoMove || c.Stats.HP == 0 || c.World.Walking() || now.Before(r.nextWalk) || c.pendingNPC != nil {
		return
	}
	// Rotate destinations around the starting position; existing player paths
	// enforce walkable terrain. Do not replace a path while it is running.
	offsets := [...]image.Point{{X: -remoteWanderRadius}, {Y: -remoteWanderRadius}, {X: remoteWanderRadius}, {Y: remoteWanderRadius}, {X: -remoteWanderRadius, Y: -remoteWanderRadius}, {X: remoteWanderRadius, Y: -remoteWanderRadius}, {X: remoteWanderRadius, Y: remoteWanderRadius}, {X: -remoteWanderRadius, Y: remoteWanderRadius}}
	for range offsets {
		point := r.anchor.Add(offsets[r.direction%len(offsets)])
		r.direction++
		if c.World.WalkTo(point.X, point.Y, now) {
			break
		}
	}
	r.nextWalk = now.Add(remoteWalkPause)
}
func (c *Client) remotePetSlot() byte {
	for i, p := range c.InventoryState.Pets {
		if p.ID != 0 && game.SamePet(uint32(p.ID), c.remote.activePet) {
			return byte(i + 1)
		}
	}
	return 0
}
func (c *Client) remoteSupply() bool {
	o := c.remote.options
	type need struct {
		stats     *world.Stats
		target    byte
		status    uint16
		threshold int
	}
	needs := []need{{c.Stats, 0, remoteHPStatus, o.Thresholds[0]}, {c.Stats, 0, remoteSPStatus, o.Thresholds[1]}}
	if slot := c.remotePetSlot(); slot != 0 {
		pet := &c.InventoryState.Pets[slot-1].Stats
		needs = append(needs, need{pet, slot, remoteHPStatus, o.Thresholds[2]}, need{pet, slot, remoteSPStatus, o.Thresholds[3]})
	}
	for _, n := range needs {
		current, maximum := uint64(n.stats.HP), uint64(n.stats.MaxHP)
		if n.status == remoteSPStatus {
			current, maximum = uint64(n.stats.SP), uint64(n.stats.MaxSP)
		}
		if n.stats.HP == 0 || maximum == 0 || current >= maximum || current*remotePercentBase >= maximum*uint64(n.threshold) {
			continue
		}
		for slot := int(o.SupplyFirst); slot <= int(o.SupplyLast); slot++ {
			item := c.InventoryState.Bag[slot-1]
			if item.Empty() || item.Locked || containsRemoteDiscard(o, item.ID) {
				continue
			}
			def := c.items[item.ID].Definition
			usable := false
			safe := true
			for i, status := range def.Status {
				if status != 0 && (status != remoteHPStatus && status != remoteSPStatus || def.Values[i] < remoteRecoveryBase) {
					safe = false
				}
				if status == n.status && def.Values[i] > remoteRecoveryBase {
					usable = true
				}
			}
			if !usable || !safe {
				continue
			}
			if err := c.Net.Send([]byte{protocol.CommandInventory, protocol.InventoryItemUse, byte(slot), 1, n.target, 0}); err != nil {
				c.stopRemote()
				c.Chat.Notice(err.Error())
				return true
			}
			c.remote.pending = &remoteRequest{at: c.Now(), slot: byte(slot), item: item, supply: true}
			return true
		}
		if o.LeaveNoSupplies {
			c.remoteLeave("Remote has no supplies for the required recovery.")
			return true
		}
	}
	return false
}
func (c *Client) remoteInformation() {
	r := &c.remote
	if !r.active || !r.options.Information || c.World == nil || c.movie != nil || c.sceneFrozen() {
		return
	}
	o := r.options
	on := func(v bool) string {
		if v {
			return "On"
		}
		return "Off"
	}
	lines := []string{"Auto Atk: " + on(o.AutoFight), "Adv Setting: Off", "Pets Adv Setting: Off", "Auto Walk: " + on(o.AutoMove), "Auto Heal: " + on(o.AutoSupply), "Auto Leave: " + on(o.LeaveNoSupplies), "On Player Die: " + on(o.LeavePlayerDeaths), "On Pet Die: " + on(o.LeavePetDeaths), fmt.Sprintf("Player HP: %d(%d%%)", uint64(c.Stats.MaxHP)*uint64(o.Thresholds[0])/remotePercentBase, o.Thresholds[0]), fmt.Sprintf("Player SP: %d(%d%%)", uint64(c.Stats.MaxSP)*uint64(o.Thresholds[1])/remotePercentBase, o.Thresholds[1])}
	var pet world.Stats
	if slot := c.remotePetSlot(); slot != 0 {
		pet = c.InventoryState.Pets[slot-1].Stats
	}
	lines = append(lines, fmt.Sprintf("Pet HP: %d(%d%%)", uint64(pet.MaxHP)*uint64(o.Thresholds[2])/remotePercentBase, o.Thresholds[2]), fmt.Sprintf("Pet SP: %d(%d%%)", uint64(pet.MaxSP)*uint64(o.Thresholds[3])/remotePercentBase, o.Thresholds[3]), fmt.Sprintf("Player Deaths: %d/%d", r.playerDeaths, o.PlayerDeaths), fmt.Sprintf("Pet Deaths: %d/%d", r.petDeaths, o.PetDeaths), "Leave Timer: "+on(o.LeaveAfter), "Auto Unequip: "+on(o.AutoUnequip), "Auto Discard: "+on(o.AutoDiscard))
	for i, line := range lines {
		c.Env.Text.Draw(remoteStatusX, remoteStatusY+i*remoteStatusLine, 0, false, true, c.Screen, []byte(line), remoteStatusLine, 240, 0, 0xffff, 2)
	}
}

// Track native fighter cells and round readiness even with Remote off. Auto
// attack never starts a battle and only serves an existing monster encounter.
func (c *Client) remoteBattlePacket(p []byte) bool {
	if c.World == nil || len(p) < 2 {
		return false
	}
	b := &c.remote.battle
	switch p[0] {
	case protocol.CommandBattlePet:
		switch p[1] {
		case protocol.BattlePetSelect:
			if len(p) != 6 {
				return false
			}
			c.remote.activePet = binary.LittleEndian.Uint32(p[2:])
			return true
		case protocol.BattlePetRest:
			if len(p) != 2 {
				return false
			}
			c.remote.activePet = 0
			return true
		}
	case protocol.CommandBattleState:
		switch p[1] {
		case protocol.BattleStateFormation, protocol.BattleStateParticipant:
			offset := 2
			if p[1] == protocol.BattleStateFormation {
				offset = 4
			}
			if len(p) != offset+remoteFighterRecordBytes {
				return false
			}
			v := p[offset:]
			f := remoteFighter{side: v[0], kind: v[1], id: binary.LittleEndian.Uint32(v[2:]), owner: binary.LittleEndian.Uint32(v[8:]), x: v[12], y: v[13], maxHP: binary.LittleEndian.Uint32(v[14:]), maxSP: binary.LittleEndian.Uint16(v[18:]), hp: binary.LittleEndian.Uint32(v[20:]), sp: binary.LittleEndian.Uint16(v[24:])}
			if f.x < 1 || f.x > 4 || f.y < 1 || f.y > 4 || f.hp > f.maxHP || f.sp > f.maxSP {
				return false
			}
			if p[1] == protocol.BattleStateFormation {
				if f.kind != remoteFighterPlayer || f.id != c.World.Player.ID {
					return false
				}
				*b = remoteBattle{active: true}
				c.hotbar.submitted = nil
				c.hotbar.turn++
				c.closeHotbarTarget()
				c.World.StopWalk()
			}
			for i, old := range b.fighters {
				if old.x == f.x && old.y == f.y {
					b.fighters[i] = f
					return true
				}
			}
			if len(b.fighters) >= remoteFighterLimit {
				return false
			}
			b.fighters = append(b.fighters, f)
			return true
		case protocol.BattleStateWireCode0:
			if len(p) == 8 && binary.LittleEndian.Uint32(p[2:]) == c.World.Player.ID {
				*b = remoteBattle{}
				c.closeHotbarTarget()
				c.remote.nextWalk = c.Now().Add(remoteWalkPause)
				return true
			}
		case protocol.BattleStateFinish:
			b.ready = false
			c.closeHotbarTarget()
			return true
		case protocol.BattleStateExit:
			return true
		}
	case protocol.CommandBattleAction:
		if !b.active {
			return false
		}
		if p[1] == protocol.BattleActionTurn && len(p) == 5 {
			for _, f := range b.fighters {
				if f.id == c.World.Player.ID && f.kind == remoteFighterPlayer && f.x == p[2] && f.y == p[3] {
					b.prompt = true
					break
				}
			}
			return true
		}
		if p[1] == protocol.BattleActionAnimation {
			return true
		}
	case protocol.CommandBattleReady:
		if len(p) == 2 && p[1] == protocol.BattleReadyReady && b.active {
			if b.prompt {
				b.ready = true
				c.hotbar.submitted = nil
				c.hotbar.turn++
				c.closeHotbarTarget()
				b.prompt = false
			}
			return true
		}
	case remoteBattleStatCommand:
		if len(p) == 9 && p[1] == remoteBattleStatValue && b.active {
			for i := range b.fighters {
				f := &b.fighters[i]
				if f.x == p[2] && f.y == p[3] {
					value := binary.LittleEndian.Uint32(p[5:])
					if p[4] == remoteHPStatus {
						f.hp = min(value, f.maxHP)
						if value > 0 {
							f.countedDeath = false
						}
					}
					if p[4] == remoteSPStatus {
						f.sp = uint16(min(value, uint32(f.maxSP)))
					}
				}
			}
			return true
		}
	case protocol.CommandBattleEffect:
		if len(p) != 4 || !b.active {
			return false
		}
		if p[1] == protocol.BattleEffectDefeated {
			for i := range b.fighters {
				f := &b.fighters[i]
				if f.x == p[2] && f.y == p[3] && !f.countedDeath {
					f.hp = 0
					f.countedDeath = true
					if f.kind == remoteFighterPlayer && f.id == c.World.Player.ID {
						c.remote.playerDeaths++
					}
					if f.kind == remoteFighterPet && f.owner == c.World.Player.ID {
						c.remote.petDeaths++
					}
				}
			}
			return true
		}
		if p[1] == protocol.BattleEffectEscape {
			return true
		} // action acknowledgement
	}
	return false
}
func (c *Client) remoteAttack() {
	b := &c.remote.battle
	b.ready = false
	var target *remoteFighter
	for i := range b.fighters {
		f := &b.fighters[i]
		if f.kind == remoteFighterMonster && f.hp > 0 {
			target = f
			break
		}
	}
	if target == nil {
		// An exhausted monster roster is normal at battle completion.
		for _, f := range b.fighters {
			if f.kind == remoteFighterPlayer && f.id != c.World.Player.ID && f.side != ownRemoteSide(b, c.World.Player.ID) {
				c.stopRemote()
				c.Chat.Notice("Remote auto-fight only supports monster battles.")
				break
			}
		}
		return
	}
	for _, f := range b.fighters {
		if f.hp == 0 || !((f.kind == remoteFighterPlayer && f.id == c.World.Player.ID) || (f.kind == remoteFighterPet && f.owner == c.World.Player.ID)) {
			continue
		}
		p := []byte{protocol.CommandBattleAction, remoteBattleAttack, f.x, f.y, target.x, target.y}
		p = binary.LittleEndian.AppendUint16(p, remoteBasicAttack)
		if err := c.Net.Send(p); err != nil {
			c.stopRemote()
			c.Chat.Notice(err.Error())
			return
		}
	}
}

const remoteWornOutDamage = 200

func (c *Client) remoteLeave(reason string) {
	c.stopRemote()
	c.disconnectText = []byte(reason)
	c.Net.Close()
	c.disconnected()
}
func containsRemoteDiscard(o inventory.RemoteOptions, id uint16) bool {
	if !o.AutoDiscard {
		return false
	}
	for _, v := range o.Discard {
		if v != 0 && v == id {
			return true
		}
	}
	return false
}
func (c *Client) remoteSend(p []byte, pending *remoteRequest) bool {
	if err := c.Net.Send(p); err != nil {
		c.stopRemote()
		c.Chat.Notice(err.Error())
		return false
	}
	c.remote.pending = pending
	return true
}
func (c *Client) remoteDiscard() bool {
	for i, item := range c.InventoryState.Bag {
		if item.Empty() || item.Locked || item.ID == remoteControllerItem || !containsRemoteDiscard(c.remote.options, item.ID) {
			continue
		}
		c.remoteSend([]byte{protocol.CommandInventory, protocol.InventoryDestroy, byte(i + 1), item.Count, 0}, &remoteRequest{at: c.Now(), slot: byte(i + 1), item: item})
		return true
	}
	return false
}
func (c *Client) remoteUnequip() bool {
	definitions := make(map[uint16]game.ItemDefinition, len(c.items))
	for id, item := range c.items {
		definitions[id] = item.Definition
	}
	targets := []byte{0}
	if slot := c.remotePetSlot(); slot != 0 {
		targets = append(targets, slot)
	}
	for _, target := range targets {
		equipment := c.InventoryState.Equipment
		if target != 0 {
			equipment = c.InventoryState.Pets[target-1].Equipment
		}
		for i, item := range equipment {
			if item.Empty() || item.Locked || item.Damage < remoteWornOutDamage {
				continue
			}
			bag := c.InventoryState.Bag
			additions, err := bag.Grant(item, 1, definitions[item.ID].StackLimit(), definitions)
			if err != nil || len(additions) == 0 {
				continue
			}
			destination := additions[0].Slot
			if !c.InventoryState.Bag[destination-1].Empty() {
				continue
			}
			p := []byte{protocol.CommandInventory, protocol.InventoryUnequip, byte(i + 1), destination}
			if target != 0 {
				p = []byte{protocol.CommandInventory, protocol.InventoryPetUnequip, target, byte(i + 1), destination}
			}
			c.remoteSend(p, &remoteRequest{at: c.Now(), slot: byte(i + 1), target: target, item: item, equipment: true})
			return true
		}
	}
	return false
}

const remoteFighterLimit = 16

func (c *Client) remoteInventoryReceipt(p []byte) {
	if len(p) == 2 && p[0] == protocol.CommandInventory && p[1] == protocol.InventoryItemUse && c.remote.pending != nil && c.remote.pending.supply {
		c.remote.pending.ack = true
	}
}
func (c *Client) resetRemoteRuntime() {
	c.stopRemote()
	c.remote = remoteRuntime{button: c.remote.button}
}

func ownRemoteSide(b *remoteBattle, id uint32) byte {
	for _, f := range b.fighters {
		if f.kind == remoteFighterPlayer && f.id == id {
			return f.side
		}
	}
	return 0
}

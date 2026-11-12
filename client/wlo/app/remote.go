package app

import "wonderland-gonline/internal/localization"

import (
	"encoding/binary"
	"fmt"
	"image"
	"time"
	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

const (
	remoteTickInterval   = 250 * time.Millisecond
	remoteWalkPause      = time.Second
	remoteRequestTimeout = 5 * time.Second
	remoteWanderRadius   = 160 // Go fallback: native offset-table distances are not verified.
	remoteBasicAttack    = 10001
	remoteBattleAttack   = 1
	remoteFighterPlayer  = clientbattle.Player
	remoteFighterPet     = clientbattle.Pet
	remoteFighterMonster = clientbattle.Monster
	remoteHPStatus       = 25
	remoteSPStatus       = 26
	remoteRecoveryBase   = 100
	remotePercentBase    = 100
	remoteControllerItem = 34058
	// FUN_0026348c: a 49-pixel shortcut panel uses AutoplayTool_1,
	// whose native artwork is 42 x 42 (not the inventory item icon).
	remoteShortcutSize    = 49
	remoteShortcutArtSize = 42
	remoteShortcutPicture = "AutoplayTool_1"
	remoteStatusX         = 5
	remoteStatusY         = 200
	remoteStatusLine      = 15
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
	sequence                  [2]int
	reservedItems             map[byte]uint16 // Choices in the current round; no optimistic bag edits.
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
	if !o.Valid() {
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
		c.Chat.Notice(localization.UI("ui.app.remote_control_is_no_longer_available_6b5fd856"))
		return false
	}
	if c.remote.pending != nil {
		c.Chat.Notice(localization.UI("ui.app.wait_for_the_pending_remote_request_be3bf337"))
		return false
	}
	if err := saveRemoteProfile(c.remoteProfilePath(), o); err != nil {
		c.Chat.Notice(localization.UI("ui.remote.save_failed") + err.Error())
		return false
	}
	c.Inventory.RestoreRemoteOptions(o)
	battle, pet, button := c.remote.battle, c.remote.activePet, c.remote.button
	c.remote = remoteRuntime{active: true, options: o, start: c.Now(), anchor: image.Pt(c.World.Player.X, c.World.Player.Y), mapID: c.World.Player.Map, battle: battle, activePet: pet, button: button}
	c.resetGroundHold()
	if c.World.Walking() {
		c.World.StopWalk()
	}
	c.remoteButton()
	if o.AutoFight && c.battle.state.Active && !c.battle.state.Spectating {
		c.battle.auto = true
		c.battleAutoCancel()
	}
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
		b.Init("", ScreenWidth-remoteShortcutSize, 0, 0, 0, 0, false, remoteShortcutSize, remoteShortcutSize, ScreenHeight-110)
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
	c.loadPictures(remoteShortcutPicture)
	// A single-frame animation paints at the button origin; native Icon
	// drawing adds Left/Top twice for controls away from their parent origin.
	b.SetAnimation(remoteShortcutPicture, 0, 1, 1, time.Hour, remoteShortcutArtSize, remoteShortcutArtSize, 0)
	b.StartAnimation()
	b.Show()
}
func (c *Client) remoteTick() {
	if c.agentViewerOwns() || c.replica != nil {
		return
	}
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
			c.Chat.Notice(localization.UI("ui.app.remote_stopped_no_item_request_receipt_n_d0450d74"))
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
		c.Chat.Notice(localization.UI("ui.app.remote_stopped_controller_unavailable_d76ece5b"))
		return
	}
	if c.World.Player.Map != r.mapID {
		c.stopRemote()
		c.Chat.Notice(localization.UI("ui.app.remote_stopped_after_changing_maps_f23c0ad4"))
		return
	}

	o := r.options
	if o.LeaveAfter && now.Sub(r.start) >= time.Duration(o.LeaveMinutes)*time.Minute {
		c.remoteLeave(localization.UI("ui.app.remote_leave_timer_reached_27ba47d8"))
		return
	}
	if (o.LeavePlayerDeaths && r.playerDeaths >= o.PlayerDeaths) || (o.LeavePetDeaths && r.petDeaths >= o.PetDeaths) {
		c.remoteLeave(localization.UI("ui.app.remote_death_limit_reached_15ce6459"))
		return
	}
	if now.Before(r.nextTick) || c.sceneFrozen() || c.movie != nil || c.sport != nil || c.UI.Modal != nil {
		return
	}
	r.nextTick = now.Add(remoteTickInterval)
	if c.battle.closing {
		return
	}
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
			c.remoteLeave(localization.UI("ui.app.remote_has_no_supplies_for_the_required__a186756e"))
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
			return localization.UI("ui.app.on_13001175")
		}
		return localization.UI("ui.app.off_ca7981b4")
	}
	lines := []string{localization.UI("ui.app.auto_atk_ae4d734e") + on(o.AutoFight), localization.UI("ui.remote.player_advanced") + on(o.Advanced[0].Active()), localization.UI("ui.remote.pet_advanced") + on(o.Advanced[1].Active()), localization.UI("ui.app.auto_walk_f3a24a8a") + on(o.AutoMove), localization.UI("ui.app.auto_heal_493a1915") + on(o.AutoSupply), localization.UI("ui.app.auto_leave_576dcd4a") + on(o.LeaveNoSupplies), localization.UI("ui.app.on_player_die_60376312") + on(o.LeavePlayerDeaths), localization.UI("ui.app.on_pet_die_f02bb024") + on(o.LeavePetDeaths), fmt.Sprintf(localization.UI("ui.app.player_hp_d_d_4c41c047"), uint64(c.Stats.MaxHP)*uint64(o.Thresholds[0])/remotePercentBase, o.Thresholds[0]), fmt.Sprintf(localization.UI("ui.app.player_sp_d_d_da6308b8"), uint64(c.Stats.MaxSP)*uint64(o.Thresholds[1])/remotePercentBase, o.Thresholds[1])}
	var pet world.Stats
	if slot := c.remotePetSlot(); slot != 0 {
		pet = c.InventoryState.Pets[slot-1].Stats
	}
	lines = append(lines, fmt.Sprintf(localization.UI("ui.app.pet_hp_d_d_1eb1bcf2"), uint64(pet.MaxHP)*uint64(o.Thresholds[2])/remotePercentBase, o.Thresholds[2]), fmt.Sprintf(localization.UI("ui.app.pet_sp_d_d_cda0675e"), uint64(pet.MaxSP)*uint64(o.Thresholds[3])/remotePercentBase, o.Thresholds[3]), fmt.Sprintf(localization.UI("ui.app.player_deaths_d_d_78040c4f"), r.playerDeaths, o.PlayerDeaths), fmt.Sprintf(localization.UI("ui.app.pet_deaths_d_d_b3a32054"), r.petDeaths, o.PetDeaths), localization.UI("ui.app.leave_timer_a06795ec")+on(o.LeaveAfter), localization.UI("ui.app.auto_unequip_0541b7f5")+on(o.AutoUnequip), localization.UI("ui.app.auto_discard_b96da116")+on(o.AutoDiscard))
	for i, line := range lines {
		c.Env.Text.Draw(remoteStatusX, remoteStatusY+i*remoteStatusLine, 0, false, true, c.Screen, []byte(line), remoteStatusLine, 240, 0, 0xffff, 2)
	}
}

// Track native fighter cells and round readiness even with Remote off. Auto
// attack never starts a battle and only serves an existing monster encounter.
func (c *Client) remoteAttack() {
	c.remoteRearmBattleAuto()
	b := &c.remote.battle
	c.remote.reservedItems = map[byte]uint16{}
	b.ready = false
	c.battle.state.Ready = false
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
				c.Chat.Notice(localization.UI("ui.app.remote_auto_fight_only_supports_monster__46147dd8"))
				break
			}
		}
		return
	}
	for _, f := range b.fighters {
		if f.hp == 0 || !((f.kind == remoteFighterPlayer && f.id == c.World.Player.ID) || (f.kind == remoteFighterPet && f.owner == c.World.Player.ID)) {
			continue
		}
		if native := c.battle.state.At(clientbattle.Cell{X: f.x, Y: f.y}); native == nil || native.Waiting || native.Removed {
			continue
		}
		if c.remoteAdvancedAction(f) {
			if !c.remote.active {
				return
			}
			continue
		}
		p := []byte{protocol.CommandBattleAction, remoteBattleAttack, f.x, f.y, target.x, target.y}
		p = binary.LittleEndian.AppendUint16(p, remoteBasicAttack)
		if err := c.sendCombatChoice(p); err != nil {
			c.stopRemote()
			c.Chat.Notice(err.Error())
			return
		}
	}
}

// Remote re-arms AUTO only while choices are being accepted. Cancelling AUTO
// during playback leaves it off until the next decision stage; this does not
// submit a command or consume AUTO's once-per-turn submission marker.
func (c *Client) remoteRearmBattleAuto() {
	v := &c.battle
	if !c.remote.active || !c.remote.options.AutoFight || !v.state.Active || !v.state.Ready || v.state.Spectating || v.state.Finished || v.closing || v.timedOut || c.battleAnimating() {
		return
	}
	v.auto = true
	c.battleAutoCancel()
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

func (c *Client) remoteInventoryReceipt(p []byte) {
	if len(p) == 2 && p[0] == protocol.CommandInventory && p[1] == protocol.InventoryItemUse && c.remote.pending != nil && c.remote.pending.supply {
		c.remote.pending.ack = true
	}
}
func (c *Client) resetRemoteRuntime() {
	c.areaTitle = areaTitle{}
	c.closeBattleView()
	c.battle.actionHistory = nil
	c.battle.state = clientbattle.State{}
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

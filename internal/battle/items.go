package battle

import (
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

const battleItemStatResultBytes = 6

// SubmitItem follows FUN_002c2394's AC50:2 branch at 0x002d4247:
// two cells, item U16, two opaque random bytes, bag slot, compatibility flag.
func (b *Battle) submitItem(r Rules, player uint32, data []byte) []byte {
	if len(data) != protocol.BattleItemRequestBytes-2 && len(data) != protocol.BattleItemLegacyRequestBytes-2 {
		return nil
	}
	actor := at(b.all(), data[0], data[1], true)
	if actor == nil || actor.Char == nil && actor.Pet == nil || actor.Absent || !actor.CanAct() || (actor.ID != player || actor.Kind != Player) && actor.Owner != player {
		return nil
	}
	if _, exists := b.Pending[actor.key()]; exists {
		return nil
	}
	id := uint16(data[4]) | uint16(data[5])<<8
	slot := data[8]
	info, ok := r.Items[id]
	if !ok || slot < 1 || slot > game.BagSize {
		return nil
	}
	var owner *game.Character
	for _, f := range b.all() {
		if f.Kind == Player && f.ID == player {
			owner = f.Char
			break
		}
	}
	if owner == nil {
		return nil
	}
	item := owner.Bag[slot-1]
	if item.ID != id || item.Empty() || item.Locked {
		return nil
	}
	reserved := 0
	for _, a := range b.Pending {
		if a.Player == player && a.Kind == "item" && a.ItemSlot == slot {
			reserved++
		}
	}
	if reserved >= int(item.Count) {
		return nil
	}
	a := Action{Actor: actor, Player: player, Kind: "item", TX: data[2], TY: data[3], ItemSlot: slot, Item: item}
	if _, ok := b.itemTarget(r, a, info); !ok {
		return nil
	}
	b.Pending[actor.key()] = a
	return []byte{protocol.CommandBattleEffect, protocol.BattleEffectSubmitted, actor.X, actor.Y}
}

func (b *Battle) itemTarget(r Rules, a Action, info assets.BattleItem) (*Fighter, bool) {
	if info.Skill != 0 {
		if _, ok := r.Skills[info.Skill]; !ok {
			return nil, false
		}
	}
	t := at(b.all(), a.TX, a.TY, false)
	if t == nil || t.Absent {
		return nil, false
	}
	if info.Skill == assets.BattleItemReviveSkill {
		return t, t.Side == a.Actor.Side && t.Dead() && info.HP > 0
	}
	if t.Dead() {
		return nil, false
	}
	harmful := info.HP < 0 || info.SP < 0
	for _, e := range r.Skills[info.Skill].Effects {
		if e.Target == assets.EffectSelf && t != a.Actor {
			return nil, false
		}
		harmful = harmful || e.Target == assets.EffectEnemy
	}
	if harmful {
		return t, t.Side != a.Actor.Side
	}
	if t.Side != a.Actor.Side {
		return nil, false
	}
	// Like FUN_0038972c, selection does not require missing vitals: a teammate
	// may take damage before this action executes later in the round.
	return t, info.HP > 0 || info.SP > 0 || len(r.Skills[info.Skill].Effects) > 0
}

func (b *Battle) useItem(r Rules, a Action) []Step {
	if a.Actor.Dead() || a.Actor.Absent || !a.Actor.CanAct() || r.CommitItem == nil {
		return nil
	}
	info, ok := r.Items[a.Item.ID]
	if !ok {
		return nil
	}
	t, ok := b.itemTarget(r, a, info)
	if !ok {
		return nil
	}
	hp, sp := min(t.MaxHP, max(0, t.HP+info.HP)), min(t.MaxSP, max(0, t.SP+info.SP))
	if !r.CommitItem(a, t, hp, sp) {
		return nil
	}
	beforeHP, beforeSP := t.HP, t.SP
	if info.HP < 0 {
		a.supporters = a.Actor.consumeSupport()
	}
	if hp < t.HP {
		if r.ExperienceFormula == game.BattleEXPParticipation {
			b.recordExperienceDamage(t, a.Actor, t.HP-hp)
		}
		t.hurt(-info.HP)
	} else {
		t.HP = hp
	}
	t.SP = sp
	for i, e := range r.Skills[info.Skill].Effects {
		t.ApplyEffect(info.Skill, i, e)
	}
	motion := info.Skill
	if motion == 0 {
		motion = assets.BattleItemHumanMotion
		if t.Kind != Player {
			motion = assets.BattleItemCreatureMotion
		}
	}
	var stats protocol.Builder
	add := func(stat byte, before, after int) {
		if before == after {
			return
		}
		mode := byte(protocol.BattleStatRecovery)
		amount := after - before
		if amount < 0 {
			mode = protocol.BattleStatDamage
			amount = -amount
		}
		stats = stats.U8(stat).U32(uint32(amount)).U8(mode)
	}
	if info.HP < 0 {
		add(game.StatCurrentHP, beforeHP, beforeHP+info.HP)
	} else {
		add(game.StatCurrentHP, beforeHP, t.HP)
	}
	add(game.StatCurrentSP, beforeSP, t.SP)
	// One target can carry both HP and SP results, including a zero-stat buff.
	record := protocol.Builder{}.U8(a.Actor.X).U8(a.Actor.Y).U16(motion).U8(0).U8(1).
		U8(t.X).U8(t.Y).U8(protocol.BattleHitLanded).U8(0).U8(byte(len(stats) / battleItemStatResultBytes)).Bytes(stats)
	packet := protocol.Builder{protocol.CommandBattleAction, protocol.BattleActionAnimation}.U16(uint16(len(record))).Bytes(record)
	step := Step{Packets: [][]byte{turnPacket(a.Actor), packet, StatSync(t.X, t.Y, game.StatCurrentHP, uint32(t.HP)), StatSync(t.X, t.Y, game.StatCurrentSP, uint32(t.SP))},
		OwnerPackets: map[uint32][][]byte{a.Player: {{protocol.CommandInventory, protocol.InventoryRemove, a.ItemSlot, 1}}}, Delay: r.Delay(motion, "item")}
	steps := []Step{step}
	steps = r.appendKillReward(b, steps, t, []Action{a})
	return b.appendOverkill(steps, t)
}

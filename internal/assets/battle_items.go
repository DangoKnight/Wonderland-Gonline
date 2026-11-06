package assets

const (
	battleItemType              = 24
	battleConsumableSlot        = 8
	nativeBattleItemSkillOffset = 134 // FUN_0039be38: object +0x8a, minus its four-byte prefix.
	battleRecoveryBase          = 100
	battleItemHPStatus          = 25
	battleItemSPStatus          = 26
	battleItemHPBonusStatus     = 207
	battleItemSPBonusStatus     = 208
	BattleItemHumanMotion       = 10024 // FUN_0038972c, human target.
	BattleItemCreatureMotion    = 10025
	BattleItemReviveSkill       = 15138 // Native Secular Feather; ordinary food cannot revive.
)

// BattleItem keeps the verified consumable values and optional native skill.
// Unsupported special items have no executable effect and cannot be consumed.
type BattleItem struct {
	ID, Skill uint16
	HP, SP    int
}

func (it NativeItem) BattleItem() (BattleItem, bool) {
	d := it.Definition
	if d.EquipSlot != battleConsumableSlot {
		return BattleItem{}, false
	}
	b := BattleItem{ID: d.ID}
	if d.Type == battleItemType {
		b.Skill = le.Uint16(it.Record[nativeBattleItemSkillOffset:])
		if b.Skill == 0 {
			return BattleItem{}, false
		}
	}
	for i, status := range d.Status {
		delta := int(d.Values[i]) - battleRecoveryBase
		switch status {
		case battleItemHPStatus, battleItemHPBonusStatus:
			b.HP += delta
		case battleItemSPStatus, battleItemSPBonusStatus:
			b.SP += delta
		}
	}
	return b, b.Skill != 0 || b.HP > 0 || b.SP > 0
}

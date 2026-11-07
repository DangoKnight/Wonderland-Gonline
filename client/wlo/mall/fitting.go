package mall

import (
	"fmt"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/game"
)

const fittingBonusInk = 0xf800

func (f *Form) fittingText(x, y int, value string) {
	f.Env.Text.Draw(f.Left+x, f.Top+y, 0, false, true, f.Env.Screen, clientassets.Big5Text(value), 15, 94, 0, fittingBonusInk, 0)
}
func (f *Form) fittingVital(x, y int, current, maximum uint32, delta int32) {
	if delta == 0 {
		f.text(x, y, 94, 1, fmt.Sprintf("%d/%d", current, maximum))
		return
	}
	prefix := fmt.Sprintf("%d/", current)
	f.text(x, y, 94, 1, prefix)
	f.fittingText(x+len(prefix)*8, y, fmt.Sprint(max(1, int64(maximum)+int64(delta))))
}

// TryOn changes only the mall's copied equipment. No inventory command is sent.
func (f *Form) TryOn(id uint16) bool {
	n, ok := f.Items[id]
	if !ok {
		return false
	}
	if linked := f.FittingItems[id]; linked != 0 {
		actual, known := f.Items[linked]
		if !known {
			return false
		}
		n, id = actual, linked
	}
	slot := int(n.Definition.EquipSlot) - 1
	if slot < 0 || slot >= len(f.Fitted) {
		return false
	}
	f.Fitted[slot] = game.Item{ID: id, Count: 1}
	return true
}
func (f *Form) ResetFitting() { f.Fitted = game.Equipment{}; f.Direction = previewDefaultDirection }

// Compare trial gear against server-confirmed equipment, including forging.
// The authoritative Stats and inventory never change.
func (f *Form) FittingDelta() game.Combat {
	if f.Fitted == (game.Equipment{}) {
		return game.Combat{}
	}
	var before game.Equipment
	if f.Equipment != nil {
		before = f.Equipment()
	}
	after := before
	definitions := map[uint16]game.ItemDefinition{}
	for i, item := range before {
		definitions[item.ID] = f.Items[item.ID].Definition
		if f.Fitted[i].ID != 0 {
			after[i] = f.Fitted[i]
		}
		definitions[after[i].ID] = f.Items[after[i].ID].Definition
	}
	a, b := before.Bonuses(definitions), after.Bonuses(definitions)
	return game.Combat{MaxHP: b.HP - a.HP, MaxSP: b.SP - a.SP, ATK: int32(int16(b.ATK)) - int32(int16(a.ATK)), DEF: int32(int16(b.DEF)) - int32(int16(a.DEF)), MAT: int32(int16(b.MAT)) - int32(int16(a.MAT)), MDF: int32(int16(b.MDF)) - int32(int16(a.MDF)), SPD: int32(int16(b.SPD)) - int32(int16(a.SPD))}
}

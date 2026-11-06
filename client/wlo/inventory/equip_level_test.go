package inventory

import (
	"testing"
	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestEquipMinimumLevelWarningPlayerAndPet(t *testing.T) {
	env := &seui.Env{Pics: picdb.New()}
	seui.NewManager(env, &seui.Input{})
	s := &State{Items: map[uint16]assets.NativeItem{34076: {Definition: game.ItemDefinition{ID: 34076, EquipSlot: 6, EquipLevel: 10}}}}
	s.Bag[0] = game.Item{ID: 34076, Count: 1}
	f := NewForm(env, s, &world.Stats{Level: 9})
	env.UI.Add(f)
	f.Show()
	s.Pets[0] = UsePet{ID: 14156, Stats: world.Stats{Level: 9}}
	var warning string
	sent := 0
	f.Warning = func(s string) { warning = s }
	f.Send = func([]byte) { sent++ }
	for _, slot := range []byte{0, 1} {
		f.Selected = slot
		warning = ""
		f.equipBag(1)
		if warning != "Insufficient level" || sent != 0 {
			t.Fatal(slot, warning, sent)
		}
	}
	f.Stats.Level = 10
	s.Pets[0].Stats.Level = 10
	for _, slot := range []byte{0, 1} {
		f.Selected = slot
		f.equipBag(1)
	}
	if sent != 2 {
		t.Fatal("qualified wearer blocked", sent)
	}
}

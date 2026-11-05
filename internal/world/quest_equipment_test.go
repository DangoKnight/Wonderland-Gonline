package world

import (
	"testing"
	"wonderland-gonline/internal/game"
)

func TestQuestPlayerEquipmentNativeConditions(t *testing.T) {
	// Fall in Love: native ring condition, independent of the condition encoder.
	raw := [21]byte{16, 7, 0, 9, 100, 0, 0, 5}
	c := &game.Character{}
	c.Bag[0] = game.Item{ID: 25609, Count: 1}
	w := &World{}
	if w.condition(c, nil, 11003, nil, raw) {
		t.Fatal("bag ring satisfied equipped-ring gate")
	}
	c.Equipment[5] = game.Item{ID: 25609, Count: 1}
	if !w.condition(c, nil, 11003, nil, raw) {
		t.Fatal("worn ring not recognized")
	}
	raw[7] = 6
	if w.condition(c, nil, 11003, nil, raw) {
		t.Fatal("not-equal inverted incorrectly")
	}
	c.Equipment[5] = game.Item{}
	if !w.condition(c, nil, 11003, nil, raw) {
		t.Fatal("empty special slot rejected")
	}
	for _, selector := range []byte{0, 1, 8, 255} {
		raw[1] = selector
		if w.condition(c, nil, 11003, nil, raw) {
			t.Fatal("unresolved selector authorized", selector)
		}
	}
	raw = [21]byte{16, 7, 0, 9, 100, 0, 0, 4}
	if w.condition(c, nil, 11003, nil, raw) {
		t.Fatal("unresolved comparison authorized")
	}
}

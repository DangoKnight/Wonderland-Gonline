package inventory

import (
	"bytes"
	"testing"

	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestTentInventoryOpenCloseAndFurnitureDrop(t *testing.T) {
	env := &seui.Env{Pics: picdb.New()}
	seui.NewManager(env, &seui.Input{})
	state := &State{Items: map[uint16]assets.NativeItem{
		36002: {Definition: game.ItemDefinition{ID: 36002, Type: game.ItemTypeTent}},
	}}
	state.Bag[0] = game.Item{ID: 36002, Count: 1}
	f := NewForm(env, state, &world.Stats{})
	env.UI.Add(f)
	f.Show()
	var sent [][]byte
	f.Send = func(p []byte) { sent = append(sent, append([]byte(nil), p...)) }
	f.useBag(1)
	if len(sent) != 1 || !bytes.Equal(sent[0], []byte{23, 96, 1}) || f.Visible {
		t.Fatal("tent must use the native inventory request and close the bag", sent)
	}
	f.Show()
	var closed uint16
	f.CloseTent = func(id uint16) { closed = id }
	f.Slots[0].RightUp(0, 0, 0)
	if closed != 36002 {
		t.Fatal("right click did not request tent retrieval")
	}
	item := game.Item{ID: 38049, Count: 1}
	state.Bag[1] = item
	f.PlaceFurniture = func(slot byte, got game.Item, x, y int) bool {
		if slot != 2 || got != item || x != -100 || y != -100 {
			t.Fatal("furniture drag lost its inventory snapshot")
		}
		return true
	}
	f.drag = &dragItem{slot: 2, item: item, moved: true}
	f.release(-100, -100)
	if len(sent) != 1 || state.Bag[1] != item || f.destroyWarning != nil {
		t.Fatal("tent placement fell through to floor drop/destruction")
	}
}

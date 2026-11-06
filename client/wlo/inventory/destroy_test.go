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

func TestDraggingProtectedItemsWarnsBeforeDestruction(t *testing.T) {
	// Native item categories: Notepad and Radio Set are type 25;
	// Bee Spine is an ordinary droppable material.
	for _, tc := range []struct {
		id      uint16
		kind    byte
		destroy bool
	}{
		{34038, 25, true}, {34076, 25, true}, {46062, 31, false},
	} {
		env := &seui.Env{Pics: picdb.New()}
		seui.NewManager(env, &seui.Input{})
		state := &State{Items: map[uint16]assets.NativeItem{tc.id: {Definition: game.ItemDefinition{ID: tc.id, Type: tc.kind}}}}
		item := game.Item{ID: tc.id, Count: 5}
		state.Bag[0] = item
		f := NewForm(env, state, &world.Stats{})
		env.UI.Add(f)
		f.Show()
		var sent [][]byte
		f.Send = func(p []byte) { sent = append(sent, p) }
		drag := func(split bool) {
			f.drag = &dragItem{slot: 1, item: item, moved: true, split: split}
			f.release(-100, -100)
		}
		drag(false)
		if f.Dialog != nil {
			t.Fatal("ordinary drag opened quantity")
		}
		code := byte(3)
		if tc.destroy {
			code = 124
			if f.destroyWarning == nil || len(sent) != 0 {
				t.Fatal("missing destruction warning")
			}
			f.destroyWarning.Hide()
			if len(sent) != 0 {
				t.Fatal("cancel sent a request")
			}
			drag(false)
			f.destroyWarning.Confirm.OnClick()
		}
		if len(sent) != 1 || !bytes.Equal(sent[0], []byte{23, code, 1, 1, 0}) {
			t.Fatal("ordinary drag must select one", sent)
		}
		drag(true)
		if f.Dialog == nil || !f.Dialog.Visible || len(sent) != 1 {
			t.Fatal("CTRL drag must ask quantity")
		}
		f.Dialog.Count.SetText([]byte("3"))
		f.Dialog.confirm()
		if tc.destroy {
			if f.destroyWarning == nil || len(sent) != 1 {
				t.Fatal("quantity bypassed destruction warning")
			}
			f.destroyWarning.Confirm.OnClick()
		}
		if len(sent) != 2 || !bytes.Equal(sent[1], []byte{23, code, 1, 3, 0}) {
			t.Fatal("wrong selected quantity", sent)
		}
		if state.Bag[0] != item {
			t.Fatal("inventory changed before server response")
		}
		if tc.destroy {
			drag(false)
			state.Bag[0] = game.Item{}
			f.destroyWarning.Confirm.OnClick()
			if len(sent) != 2 {
				t.Fatal("stale warning sent request")
			}
		}
	}
}

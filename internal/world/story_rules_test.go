package world

import (
	"reflect"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestStoryHandInPriorityAndForwardSelection(t *testing.T) {
	w := eventWorld()
	c := character()
	for _, tt := range []struct {
		m, e uint16
		b    byte
	}{{10071, 6, 6}, {11148, 2, 2}, {11148, 4, 2}} {
		ev := &assets.Event{ClickID: tt.e, Branches: []assets.Branch{{Index: 1, Operations: []assets.Operation{action(1, 2, 1, 0, 10000, 0)}}, {Index: tt.b, Condition: cond(2, 1, 2, 0, 4, 50), Operations: []assets.Operation{action(1, 5, 900, 1, 0, 1)}}}}
		c.Gold = 0
		if got := w.FindBranch(c, nil, tt.m, ev, 0, 0, 0, -1); got != 0 {
			t.Fatal("hand-in ignored gates", got)
		}
		c.Gold = 50
		if got := w.FindBranch(c, nil, tt.m, ev, 0, 0, 0, -1); got != 1 {
			t.Fatal("reminder shadowed hand-in", got)
		}
	}
	ev := &assets.Event{ClickID: 1, Branches: []assets.Branch{{Index: 1, Operations: []assets.Operation{action(1, 2, 1, 0, 10000, 0)}}, {Index: 2, Operations: []assets.Operation{action(1, 2, 1, 0, 10001, 0)}}}}
	if got := w.FindBranch(c, nil, 11157, ev, 0, 0, 0, 1); got != -1 {
		t.Fatal("sequence replay", got)
	}
	ev.ClickID = 8
	c.Quests[13086] = game.Quest{ID: 13086, State: game.InProgress, Step: 1}
	if got := w.FindBranch(c, nil, 12002, ev, 0, 0, 0, -1); got < 0 {
		t.Fatal("early fate blocked home")
	}
	c.Quests[13086] = game.Quest{ID: 13086, State: game.InProgress, Step: 2}
	if got := w.FindBranch(c, nil, 12002, ev, 0, 0, 0, -1); got != -1 {
		t.Fatal("fate recruited Xaolan", got)
	}
}
func TestStoryControllerCopiesAndRecoveryOrder(t *testing.T) {
	niss := &assets.Event{ClickID: 6, Branches: []assets.Branch{{Index: 1, Operations: []assets.Operation{action(1, 3, 2, 14081, 0, 0), action(2, 5, 13052, 1, 0, 1)}}, {Index: 12, Operations: []assets.Operation{action(1, 10, 1, 0, 0, 0)}}}}
	before := append([]assets.Operation(nil), niss.Branches[0].Operations...)
	intro := PrepareStory(12052, niss, 0)
	if len(intro.Branches[0].Operations) != 1 || !reflect.DeepEqual(before, niss.Branches[0].Operations) {
		t.Fatal("intro dismissal/catalog mutation")
	}
	rest := PrepareStory(12052, niss, 1)
	if len(rest.Branches[1].Operations) != 1 || DecodeOp(rest.Branches[1].Operations[0]).Code != 3 {
		t.Fatal("rest checkpoint", rest)
	}
	if PrepareStory(12053, niss, 1) != niss {
		t.Fatal("repair escaped authored map")
	}
	ev := &assets.Event{ClickID: 7, Branches: []assets.Branch{{Index: 5, Operations: []assets.Operation{action(1, 3, 2, 14156, 0, 0), action(2, 5, 13086, 1, 0, 1)}}}}
	got := PrepareStory(11077, ev, 0)
	if DecodeOp(got.Branches[0].Operations[0]).D1 != 13086 {
		t.Fatal("fate checkpoint follows dismissal")
	}
	ev.ClickID = 12
	ev.Branches[0].Index = 1
	ev.Branches[0].Operations = []assets.Operation{action(1, 2, 13, 11, 0, 0), action(2, 5, 13086, 2, 0, 0), action(3, 5, 13087, 1, 0, 1)}
	got = PrepareStory(11077, ev, 0)
	ops := got.Branches[0].Operations
	if len(ops) != 4 || DecodeOp(ops[0]).D2 != 2 || DecodeOp(ops[1]).D2 != 2 || DecodeOp(ops[2]).D1 != 13087 || DecodeOp(ops[3]).D1 != 13086 {
		t.Fatal("farewell recovery", ops)
	}
}

func TestStoryBattleOutcomesFollowActualFormation(t *testing.T) {
	w := New(&assets.Catalog{})
	c := &game.Character{Quests: map[uint32]game.Quest{}}
	callback := func(index byte) assets.Branch {
		return assets.Branch{Index: index, Condition: cond(4, 1, 1, 0, 0, 0), Operations: []assets.Operation{action(1, 5, 900, 1, 0, 1)}}
	}
	for _, tc := range []struct{ mapID, event uint16 }{{12380, 2}, {12523, 12}, {11149, 5}} {
		ev := &assets.Event{ClickID: tc.event, Branches: []assets.Branch{{Index: 1, Operations: []assets.Operation{action(1, 4, 1, 0, 0, 0)}}, callback(2), {Index: 3, Operations: []assets.Operation{action(1, 4, 1, 0, 0, 0)}}, callback(4)}}
		if got := w.OutcomeBranch(c, nil, tc.mapID, ev, 1, 1, 2); got != 3 {
			t.Fatal("second formation used first callback", tc, got)
		}
		if got := w.OutcomeBranch(c, nil, tc.mapID, ev, 1, 0, 2); got != -1 {
			t.Fatal("wrong result", got)
		}
		if got := w.OutcomeBranch(c, nil, tc.mapID, ev, 1, 1, -1); got != -1 {
			t.Fatal("missing origin accepted")
		}
		// Bounded sequences cannot cross another fight; Mooter explicitly can.
		expected := -1
		if tc.mapID == 11149 {
			expected = 3
		}
		ev.Branches[1].Condition = cond(4, 1, 0, 0, 0, 0)
		if got := w.OutcomeBranch(c, nil, tc.mapID, ev, 1, 1, 0); got != expected {
			t.Fatal("callback barrier", tc, got)
		}
	}
}

func TestElinReplacementWeaponGatesRequireEquippedPartyCompanion(t *testing.T) {
	w := eventWorld()
	c := character()
	elin := game.Pet{ID: 14230, Slot: 1, Equipment: game.Equipment{2: {ID: 20044, Count: 1}}}
	c.Pets = []game.Pet{elin}
	original := cond(17, 14230, 4, 20044, 5, 0)
	replacement := cond(17, 14230, 1, 20045, 5, 0)
	if !w.condition(c, nil, 11167, nil, original) || w.condition(c, nil, 11167, nil, replacement) {
		t.Fatal("original equipped weapon gate")
	}
	c.Pets[0].Equipment[2] = game.Item{ID: 20045, Count: 1}
	if w.condition(c, nil, 11167, nil, original) || !w.condition(c, nil, 11167, nil, replacement) {
		t.Fatal("replacement equipped weapon gate")
	}
	c.Pets = nil
	c.HotelPets = []game.Pet{elin}
	if w.condition(c, nil, 11167, nil, original) {
		t.Fatal("hotel Elin accepted")
	}
	c.ReservePets = []game.Pet{elin}
	if w.condition(c, nil, 11167, nil, original) {
		t.Fatal("reserved Elin accepted")
	}
	c.Pets = []game.Pet{elin}
	for _, invalid := range [][21]byte{cond(17, 14230, 2, 20044, 5, 0), cond(17, 14230, 4, 20045, 5, 0), cond(17, 14081, 4, 20044, 5, 0)} {
		if w.condition(c, nil, 11167, nil, invalid) {
			t.Fatal("unverified operand authorized", invalid)
		}
	}
	if w.condition(c, nil, 11168, nil, original) {
		t.Fatal("unverified map authorized")
	}
}

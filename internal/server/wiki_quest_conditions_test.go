package server

import (
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/world"
)

func TestInstalledQuestVehicleCapacityBranches(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	s, c, wire := eventFixture(t)
	s.Assets, s.World = catalog, world.New(catalog)
	ev, known := s.World.Event(12159, 3)
	if !known {
		t.Fatal("missing native event")
	}
	c.character.Map = 12159
	c.character.Quests[13008] = game.Quest{ID: 13008, State: game.InProgress, Step: 7}
	for _, scenario := range []struct {
		fragmented bool
		branch     byte
	}{{false, 20}, {true, 23}} {
		c.character.Bag = game.Inventory{}
		if scenario.fragmented {
			for i := range c.character.Bag {
				if i%game.BagColumns == 2 {
					c.character.Bag[i] = game.Item{ID: 30025, Count: 1}
				}
			}
		}
		branch := s.World.FindBranch(c.character, c.view, 12159, ev, world.TriggerEntry, 0, 0, -1)
		if branch < 0 || ev.Branches[branch].Index != scenario.branch {
			t.Fatal("incorrect native capacity branch", scenario, branch)
		}
		if scenario.fragmented {
			es := &eventSession{mapID: 12159, ev: ev, branch: branch}
			wire.Reset()
			if _, err := s.fallbackDialogue(c, es, 1, 0, 0, 11087); err != nil || wire.Len() == 0 {
				t.Fatal("valid full-item warning suppressed despite fragmented inventory", err)
			}
		}
	}
}

func TestInstalledSledgeWolfCaptureGate(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	w := world.New(catalog)
	ev, known := w.Event(12350, 19)
	if !known {
		t.Fatal("missing native wolf event")
	}
	for _, scenario := range []struct {
		name   string
		pets   []game.Pet
		branch byte
	}{
		{"capture_needed", nil, 4},
		{"wolf_already_captured", []game.Pet{{ID: 17759, Slot: 1}}, 8},
		{"full_party", []game.Pet{{ID: 12005, Slot: 1}, {ID: 12068, Slot: 2}, {ID: 14230, Slot: 3}, {ID: 14609, Slot: 4}}, 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			c := &game.Character{Map: 12350, Quests: map[uint32]game.Quest{12102: {ID: 12102, State: game.InProgress, Step: 1}}, Pets: scenario.pets}
			branch := w.FindBranch(c, nil, 12350, ev, world.TriggerEntry, 0, 0, -1)
			if branch < 0 || ev.Branches[branch].Index != scenario.branch {
				t.Fatal("wrong wolf encounter branch", branch)
			}
		})
	}
	// The same unresolved operand must not authorize another event or map.
	probe := *ev
	probe.ClickID = 99
	probe.Branches = []assets.Branch{{Index: 1, Condition: [21]byte{2, 2, 0, 4, 0, 95, 69}, Operations: []assets.Operation{{Data: [21]byte{10}}}}}
	if w.FindBranch(&game.Character{}, nil, 12350, &probe, world.TriggerEntry, 0, 0, -1) >= 0 {
		t.Fatal("scoped companion operand escaped its contract")
	}
}

func TestInstalledFeteBookCompanionBranches(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	w := world.New(catalog)
	for _, eid := range []uint16{13, 15} {
		if w.Disabled(12765, eid) {
			t.Fatal("obsolete companion gate still disables quest", eid)
		}
		ev, ok := w.Event(12765, eid)
		if !ok {
			t.Fatal("missing event")
		}
		for _, scenario := range []struct {
			step      int
			following bool
			branch    byte
		}{{6, false, 4}, {6, true, 9}, {7, false, 13}, {7, true, 17}} {
			c := &game.Character{Map: 12765, Quests: map[uint32]game.Quest{15144: {ID: 15144, State: game.InProgress, Step: scenario.step}}, Pets: []game.Pet{{ID: 14615, Slot: 1}}}
			c.Bag[0] = game.Item{ID: 30206, Count: 1}
			if scenario.following {
				c.ActivePet = 14615
			}
			branch := w.FindBranch(c, nil, 12765, ev, world.TriggerEntry, 0, 0, -1)
			if branch < 0 || ev.Branches[branch].Index != scenario.branch {
				t.Fatalf("event %d step %d following %v: branch %d", eid, scenario.step, scenario.following, branch)
			}
		}
		if len(w.Formation(12765, 15)) == 0 {
			t.Fatal("quest battle formation missing")
		}
	}
}

func TestInstalledNativePlayerEquipmentGates(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	w := world.New(catalog)
	count := 0
	for mid, m := range catalog.Maps {
		for _, ev := range m.Events {
			for _, branch := range ev.Branches {
				if branch.Condition[0] != world.ConditionPlayerEquipment {
					continue
				}
				count++
				d := branch.Condition
				selector := uint16(d[1]) | uint16(d[2])<<8
				id := uint16(d[3]) | uint16(d[4])<<8
				item, known := catalog.Items[id]
				if !known || selector < world.PlayerEquipmentFirstSelector || selector > world.PlayerEquipmentLastSelector || item.EquipSlot != selector-1 {
					t.Fatalf("unresolved selector in %d:%d:%d", mid, ev.ClickID, branch.Index)
				}
				// A standalone copy tests each authentic condition, including those used
				// as condition-only continuation rows in the wedding programs.
				probe := ev
				probe.Branches = []assets.Branch{{Index: 1, Condition: d, Operations: []assets.Operation{{Index: 1, Data: [21]byte{10}}}}}
				c := &game.Character{}
				c.Equipment[selector-world.PlayerEquipmentFirstSelector] = game.Item{ID: id, Count: 1}
				match := w.FindBranch(c, nil, mid, &probe, world.TriggerEntry, 0, 0, -1) >= 0
				if match != (d[7] == world.CompareEqual) {
					t.Fatalf("worn-item comparison failed in %d:%d:%d", mid, ev.ClickID, branch.Index)
				}
			}
		}
	}
	if count != 45 {
		t.Fatal("unexpected native equipment condition census", count)
	}
}

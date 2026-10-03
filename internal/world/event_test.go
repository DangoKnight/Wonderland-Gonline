package world

import (
	"bytes"
	"testing"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

// action packs an EVE action; value is spread across dialog4's high byte and dword1.
func action(index, code byte, d1, d2, d3 uint16, value uint32) assets.Operation {
	o := assets.Operation{Index: index}
	copy(o.Data[:], protocol.Builder{code}.U16(d1).U16(d2).U16(d3).U16(uint16(value&0xff)<<8).U32(value>>8))
	return o
}

func eventWorld() *World {
	ev := assets.Event{ClickID: 1, Branches: []assets.Branch{
		{Index: 1, Condition: cond(5, 900, 2, 0, 0, 0), Operations: []assets.Operation{action(1, 2, 5, 0, 10001, 0)}},
		{Index: 2, Condition: cond(7, 77, 30, 0, 0, 0), Operations: []assets.Operation{action(1, 5, 900, 1, 0, 1)}},
		{Index: 3, Condition: cond(5, 900, 1, 0, 4, 1), Operations: []assets.Operation{action(1, 2, 5, 0, 10003, 0)}},
		{Index: 4, Condition: cond(2, 1, 2, 0, 4, 50)}, // Branch 3 also needs 50 gold.
	}}
	ev.Branches[2].Condition[12] = 2 // W6 high byte: two branches form branch 3's rule.
	chest := assets.Event{ClickID: 9, Branches: []assets.Branch{{Index: 1, Condition: cond(5, 901, 1, 0, 0, 0), Operations: []assets.Operation{action(1, 2, 0, 5, 0, 0)}}}}
	m := assets.Map{ID: 12345, NPCs: []assets.MapNPC{{ClickID: 5, Flags: 1, X: 10, Y: 10, Events: []byte{1}}, {ClickID: 9, Flags: 1, X: 20, Y: 20}}, Events: []assets.Event{ev, chest}}
	return New(&assets.Catalog{Maps: map[uint16]assets.Map{m.ID: m}, Marks: map[uint16]uint16{900: 0, 901: 14}, DisabledEvents: map[uint32]string{}})
}

func TestFindBranch(t *testing.T) {
	w := eventWorld()
	ev, _ := w.Event(12345, 1)
	c := character()
	if got := w.FindBranch(c, nil, 12345, ev, 0, 0, 0, -1); got != 0 {
		t.Fatal("greeting", got)
	}
	if got := w.FindBranch(c, nil, 12345, ev, 7, 77, 30, -1); got != 1 {
		t.Fatal("choice callback", got)
	}
	if got := w.FindBranch(c, nil, 12345, ev, 7, 77, 31, -1); got != -1 {
		t.Fatal("unknown answer", got)
	}
	active := character(game.Quest{ID: 900, State: game.InProgress, Step: 1})
	if got := w.FindBranch(active, nil, 12345, ev, 0, 0, 0, -1); got != -1 {
		t.Fatal("matched without the trailing gold condition", got)
	}
	active.Gold = 50
	if got := w.FindBranch(active, nil, 12345, ev, 0, 0, 0, -1); got != 2 {
		t.Fatal("follow-up", got)
	}
	if evs := w.NPCEvents(12345, 5); len(evs) != 1 || evs[0].ClickID != 1 {
		t.Fatal("linked event", evs)
	}
	if evs := w.NPCEvents(12345, 9); len(evs) != 1 || evs[0].ClickID != 9 {
		t.Fatal("direct event", evs)
	}
}

func TestJournalAndQuestUpdates(t *testing.T) {
	w := eventWorld()
	v := NewView()
	q := game.Quest{ID: 900, State: game.InProgress, Step: 2}
	if got := w.QuestUpdate(v, 900, q); len(got) != 2 || !bytes.Equal(got[0], []byte{24, 4, 0x84, 3}) || !bytes.Equal(got[1], []byte{24, 1, 0x84, 3, 2}) || !v.Marks[900] {
		t.Fatal("journal mark", got)
	}
	if got := w.QuestUpdate(v, 901, game.Quest{ID: 901, State: game.InProgress, Step: 1}); len(got) != 1 || !bytes.Equal(got[0], []byte{24, 5, 14, 0, 1}) {
		t.Fatal("completion flag", got)
	}
	c := character(q, game.Quest{ID: 901, State: game.InProgress, Step: 1})
	got := w.Journal(c, v)
	if len(got) != 3 || !bytes.Equal(got[0], []byte{24, 4, 0x84, 3}) || !bytes.Equal(got[1], []byte{24, 6, 1, 0x84, 3, 2}) || len(got[2]) != 502 || got[2][2+2*1+1] != 0x20 {
		t.Fatal("journal", got)
	}
}

func TestSyncReplaysCompletedChestAndMarkers(t *testing.T) {
	w := eventWorld()
	c := character()
	v := NewView()
	w.MapInfo(c, v, nil)
	// Branch 1 of NPC 5 changes active mark 900: an unfinished quest shows icon 7.
	got := w.Sync(c, v, false)
	if len(got) != 2 || !bytes.Equal(got[0], []byte{22, 12, 1, 5, 0, 7}) || !bytes.Equal(got[1], []byte{22, 12, 1, 9, 0, 0}) {
		t.Fatal("markers", got)
	}
	if again := w.Sync(c, v, false); len(again) != 0 {
		t.Fatal("unchanged markers resent", again)
	}
	done := character(game.Quest{ID: 901, State: game.Completed})
	got = w.Sync(done, v, false)
	if len(got) == 0 || !bytes.Equal(got[0], []byte{22, 1, 9, 0, 1}) {
		t.Fatal("completed chest", got)
	}
}

func TestNativeEventCensus(t *testing.T) {
	w := nativeWorld(t)
	fresh := &game.Character{ID: 1, Level: 1, Quests: map[uint32]game.Quest{}}
	total, runnable, refused, idle := 0, 0, 0, 0
	codes := map[byte]int{}
	for id, m := range w.maps {
		fresh.Map = id
		v := NewView()
		w.MapInfo(fresh, v, nil)
		for _, n := range m.NPCs {
			evs := w.NPCEvents(id, n.ClickID)
			if len(evs) == 0 {
				continue
			}
			total++
			found := false
			for _, ev := range evs {
				i := w.FindBranch(fresh, v, id, ev, 0, 0, 0, -1)
				if i < 0 {
					continue
				}
				found = true
				bad := byte(0)
				for _, o := range ev.Branches[i].Operations {
					if op := DecodeOp(o); op.Unsupported() && bad == 0 {
						bad = op.Code
					}
				}
				if bad != 0 {
					refused++
					codes[bad]++
				} else {
					runnable++
				}
				break
			}
			if !found {
				idle++
			}
		}
		_ = w.Sync(fresh, v, false)
		_ = w.Journal(fresh, v)
	}
	t.Logf("NPCs with events=%d runnable=%d refused=%d no_branch=%d refused_by_opcode=%v", total, runnable, refused, idle, codes)
	if total == 0 || runnable == 0 {
		t.Fatal("no native events ran")
	}
}

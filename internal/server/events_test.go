package server

import (
	"bytes"
	"context"
	"testing"
	"time"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/world"
)

func evCond(kind byte, w1, w2, w3 uint16, op byte, value uint32) (c [21]byte) {
	copy(c[:], protocol.Builder{kind}.U16(w1).U16(w2).U16(w3).U16(uint16(op)|uint16(value&0xff)<<8).U16(uint16(value>>8)).U16(uint16(value>>24&0xff)))
	return c
}

func evOp(index, code byte, d1, d2, d3 uint16, value uint32) assets.Operation {
	o := assets.Operation{Index: index}
	copy(o.Data[:], protocol.Builder{code}.U16(d1).U16(d2).U16(d3).U16(uint16(value&0xff)<<8).U32(value>>8))
	return o
}

// eventFixture places quest NPCs beside Alice's spawn (1042, 1075) on map 10017.
func eventFixture(t *testing.T) (*Server, *Session, *captureConn) {
	s, players, wires := worldFixture(t)
	talk := assets.Event{ClickID: 1, Branches: []assets.Branch{
		{Index: 1, Condition: evCond(5, 900, 2, 0, 0, 0), Operations: []assets.Operation{
			evOp(1, 2, 5, 0, 10001, 0), // NPC line
			evOp(2, 2, 5, 6, 77, 0),    // NPC question 77
		}},
		{Index: 2, Condition: evCond(7, 77, 30, 0, 0, 0), Operations: []assets.Operation{
			evOp(1, 1, 1, 2, 0, 100),   // +100 gold
			evOp(2, 1, 1, 1, 32176, 2), // +2 items
			evOp(3, 5, 900, 1, 0, 1),   // mark 900 +1
			evOp(4, 1, 2, 10002, 0, 0), // player line
		}},
		{Index: 3, Condition: evCond(5, 900, 1, 0, 4, 1), Operations: []assets.Operation{evOp(1, 2, 5, 0, 10003, 0)}},
	}}
	unsupported := assets.Event{ClickID: 2, Branches: []assets.Branch{{Index: 1, Condition: evCond(0, 0, 0, 0, 0, 0), Operations: []assets.Operation{evOp(1, 2, 6, 0, 10004, 0), evOp(2, 12, 1, 0, 0, 0)}}}}
	disabled := assets.Event{ClickID: 3, Branches: []assets.Branch{{Index: 1, Condition: evCond(0, 0, 0, 0, 0, 0), Operations: []assets.Operation{evOp(1, 2, 7, 0, 10005, 0)}}}}
	gift := assets.Event{ClickID: 4, Branches: []assets.Branch{{Index: 1, Condition: evCond(3, 0, 0, 0, 5, 0), Operations: []assets.Operation{evOp(1, 1, 1, 1, 24001, 1)}}}}
	s.Assets.Maps[10017] = assets.Map{ID: 10017,
		NPCs: []assets.MapNPC{
			{ClickID: 5, Flags: 1, X: 1050, Y: 1080, Events: []byte{1}},
			{ClickID: 6, Flags: 1, X: 1050, Y: 1080, Events: []byte{2}},
			{ClickID: 7, Flags: 1, X: 1050, Y: 1080, Events: []byte{3}},
			{ClickID: 8, Flags: 1, X: 1050, Y: 1080, Events: []byte{4}},
			{ClickID: 9, Flags: 1, X: 3000, Y: 3000, Events: []byte{1}},
		},
		Events: []assets.Event{talk, unsupported, disabled, gift},
	}
	s.Assets.Marks = map[uint16]uint16{900: 0}
	s.Assets.DisabledEvents = map[uint32]string{assets.EventKey(10017, 3): "item:1"}
	s.World = world.New(s.Assets)
	c := players[0]
	if err := s.worldCommand(context.Background(), c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	return s, c, wires[0]
}

func TestNPCConversationRewardsAndMarks(t *testing.T) {
	s, c, wire := eventFixture(t)
	ctx := context.Background()
	send := func(p ...byte) [][]byte {
		t.Helper()
		if err := s.worldCommand(ctx, c, p); err != nil {
			t.Fatal(err)
		}
		return wire.packets(t)
	}
	got := send(20, 1, 5, 0)
	if len(got) != 2 || !bytes.Equal(got[0], []byte{6, 2, 1}) || !bytes.Equal(got[1], eventFrame(1, 3, 5, 1, 0, 10001, 1, 1)) {
		t.Fatal("greeting", got)
	}
	if got = send(20, 1, 5, 0); len(got) != 0 {
		t.Fatal("click during an event", got)
	}
	if got = send(20, 6); len(got) != 1 || !bytes.Equal(got[0], eventFrame(6, 3, 5, 0, 0, 77, 2, 1)) {
		t.Fatal("question", got)
	}
	if got = send(20, 6); len(got) != 0 {
		t.Fatal("acknowledgment closed an open question", got)
	}
	got = send(20, 9, 30)
	// Lock, gold, items with banner and fanfare, journal mark, then the player's line.
	want := [][]byte{{6, 2, 1}, protocol.Builder{26, 4}.U32(100), protocol.Builder{23, 5, 2}.U16(32176).U8(2).U8(0).Bytes(make([]byte, 26)),
		headBanner("Obtain Item #32176 x2"), {20, 10}, {24, 4, 0x84, 3}, {24, 1, 0x84, 3, 1}, eventFrame(1, 7, 0, 1, 0, 10002, 4, 2)}
	if len(got) != len(want) {
		t.Fatal("rewards", got)
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("reward packet %d: %v", i, got[i])
		}
	}
	got = send(20, 6)
	if len(got) < 2 || !bytes.Equal(got[0], []byte{6, 2, 0}) || !bytes.Equal(got[1], []byte{20, 8}) || c.event != nil {
		t.Fatal("finish", got)
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Gold != 100 || chars[0].Quests[900].Step != 1 || chars[0].Bag[1] != (game.Item{ID: 32176, Count: 2}) {
		t.Fatal("rewards not persisted", err)
	}
	// A repeated click right after closing is swallowed; later the follow-up runs.
	if got = send(20, 1, 5, 0); len(got) != 1 || !bytes.Equal(got[0], []byte{20, 8}) {
		t.Fatal("resume guard", got)
	}
	c.resumeAt = time.Time{}
	if got = send(20, 1, 5, 0); len(got) != 2 || !bytes.Equal(got[1], eventFrame(1, 3, 5, 1, 0, 10003, 1, 3)) {
		t.Fatal("follow-up", got)
	}
	if got = send(20, 9, 40); len(got) != 2 || c.event != nil {
		t.Fatal("close", got)
	}
}

func TestNPCEventRefusals(t *testing.T) {
	s, c, wire := eventFixture(t)
	ctx := context.Background()
	click := func(id byte) [][]byte {
		t.Helper()
		c.resumeAt = time.Time{}
		if err := s.worldCommand(ctx, c, []byte{20, 1, id, 0}); err != nil {
			t.Fatal(err)
		}
		return wire.packets(t)
	}
	// A branch that would transform the player is refused before its first line.
	// Finishing also sends the first minimap-marker refresh for this view.
	got := click(6)
	if len(got) < 4 || !bytes.Equal(got[1], headBanner("This quest action is not supported yet. Please report this quest.")) || !bytes.Equal(got[3], []byte{20, 8}) || !contains(got, []byte{22, 12, 1, 5, 0, 7}) {
		t.Fatal("unsupported", got)
	}
	if got = click(7); len(got) != 3 || !bytes.Equal(got[2], headBanner("This quest is disabled: required game data is unavailable.")) {
		t.Fatal("disabled", got)
	}
	if got = click(9); len(got) != 1 || !bytes.Equal(got[0], []byte{20, 8}) {
		t.Fatal("out of reach", got)
	}
	// A chest reward that cannot fit leaves the bag and chest untouched.
	for i := range c.character.Bag {
		c.character.Bag[i] = game.Item{ID: 24001, Count: 1}
	}
	before := c.character.Bag
	got = click(8)
	if len(got) < 2 || !bytes.Equal(got[1], headBanner("Cannot receive reward. Check materials, bag space, gold, party slots and Pet Hotel.")) || c.character.Bag != before {
		t.Fatal("full bag", got)
	}
	c.character.Bag[0] = game.Item{}
	got = click(8)
	if !contains(got, protocol.Builder{22, 1}.U16(4).U8(1)) || c.character.Quests[world.ChestKey(10017, 4)].State != game.Completed || c.character.Bag[0].ID != 24001 {
		t.Fatal("chest", got)
	}
	// The emptied chest no longer matches its "not opened" condition.
	if got = click(8); len(got) != 1 || !bytes.Equal(got[0], []byte{20, 8}) {
		t.Fatal("emptied chest reopened", got)
	}
}

func contains(packets [][]byte, want []byte) bool {
	for _, p := range packets {
		if bytes.Equal(p, want) {
			return true
		}
	}
	return false
}

// areaEntry builds an EVE entry record: cells (x1,y1)-(x2,y2) of the given kind.
func areaEntry(id uint16, kind byte, x1, y1, x2, y2 uint32, events ...byte) assets.AreaEntry {
	tail := make([]byte, 18)
	copy(tail[1:], protocol.Builder{}.U32(x2).U32(y2))
	tail[9] = kind
	return assets.AreaEntry{ClickID: id, X: x1, Y: y1, Events: events, Tail: tail}
}

func TestRegionsAndDoorEntries(t *testing.T) {
	s, players, wires := worldFixture(t)
	ctx := context.Background()
	say := func(id byte, talk uint16) assets.Event {
		return assets.Event{ClickID: uint16(id), Branches: []assets.Branch{{Index: 1, Condition: evCond(0, 0, 0, 0, 0, 0), Operations: []assets.Operation{evOp(1, 2, 0, 0, talk, 0)}}}}
	}
	// Alice stands at (1042, 1075): cell (53, 54), as EVE cells count from 1.
	// Region 1 covers cells 61..62, door 2 is around her, and region 3 lies at
	// cell 71.
	s.Assets.Maps[10017] = assets.Map{ID: 10017,
		Entries: []assets.AreaEntry{areaEntry(1, 1, 61, 54, 62, 54, 7), areaEntry(2, 2, 53, 54, 53, 54, 8), areaEntry(3, 1, 71, 54, 71, 54, 9)},
		Events:  []assets.Event{say(7, 30001), say(8, 30002), say(9, 30003)},
	}
	s.World = world.New(s.Assets)
	c, wire := players[0], wires[0]
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	wire.Reset()
	do := func(p ...byte) [][]byte {
		t.Helper()
		if err := s.worldCommand(ctx, c, p); err != nil {
			t.Fatal(err)
		}
		return wire.packets(t)
	}
	// Walking into region 1 starts event 7; region 3 already contained Alice.
	got := do(protocol.Builder{6, 1, 0}.U16(1210).U16(1075)...)
	if !contains(got, eventFrame(1, 3, 0, 1, 0, 30001, 1, 1)) || c.event == nil {
		t.Fatal("region", got)
	}
	// Movement during the event is ignored, not persisted.
	if got = do(protocol.Builder{6, 1, 0}.U16(1300).U16(1075)...); len(got) != 0 || c.character.X != 1210 {
		t.Fatal("moved during an event", got)
	}
	do(20, 6)
	do(protocol.Builder{6, 1, 0}.U16(1042).U16(1075)...)
	// The area the player lands in is reported before any move; it does not
	// run until the player has moved.
	c.arrived = true
	wire.Reset()
	if got = do(20, 8, 2, 0); len(got) != 1 || !bytes.Equal(got[0], []byte{20, 8}) || c.event != nil {
		t.Fatal("step on arrival", got)
	}
	do(protocol.Builder{6, 1, 0}.U16(1043).U16(1075)...)
	if c.arrived {
		t.Fatal("still arrived after a move")
	}
	wire.Reset()
	// A step reported as entry 2 runs the door's script instead of warping.
	if got = do(20, 8, 2, 0); !contains(got, eventFrame(1, 3, 0, 1, 0, 30002, 1, 1)) {
		t.Fatal("door entry", got)
	}
	do(20, 6)
	wire.Reset()
	// The client reports region 3 after a move the server already recorded.
	c.character.X = 1400
	if got = do(20, 4, 3, 0); !contains(got, eventFrame(1, 3, 0, 1, 0, 30003, 1, 1)) {
		t.Fatal("region request", got)
	}
	do(20, 6)
	wire.Reset()
	if got = do(20, 4, 1, 0); len(got) != 1 || !bytes.Equal(got[0], []byte{20, 8}) {
		t.Fatal("request outside the region", got)
	}
}

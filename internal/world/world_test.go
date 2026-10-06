package world

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/assetsql"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

// cond packs an EVE condition: kind, three operands, comparison and a 32-bit value.
func cond(kind byte, w1, w2, w3 uint16, op byte, value uint32) (c [21]byte) {
	copy(c[:], protocol.Builder{kind}.U16(w1).U16(w2).U16(w3).U16(uint16(op)|uint16(value&0xff)<<8).U16(uint16(value>>8)).U16(uint16(value>>24&0xff)))
	return c
}

func actor(click, action uint16) (o assets.Operation) {
	copy(o.Data[:], protocol.Builder{2}.U16(click).U16(action))
	return o
}

func fixture() *World {
	m := assets.Map{
		ID: 12345,
		NPCs: []assets.MapNPC{
			{ClickID: 3, Flags: 1, X: 300, Y: 400, Template: 100},
			{ClickID: 1, Flags: 0, X: 100, Y: 100, Template: 200},
			{ClickID: 0, Flags: 1, X: 5, Y: 5},
			{ClickID: 2, Flags: 1},
			{ClickID: 4, Flags: 1, X: 50, Y: 60, Template: 19001},
		},
		Items: []assets.GroundItem{
			{ClickID: 7, ItemID: 30001, X: 10, Y: 20, Unknown: [2]uint16{30, 0}},
			{ItemID: 0, X: 1, Y: 1},
			{ClickID: 300, ItemID: 30002, X: 11, Y: 21},
		},
		PreEvents: []assets.Event{{ClickID: 1, Branches: []assets.Branch{
			{Condition: cond(5, 500, 1, 0, 4, 1), Operations: []assets.Operation{actor(1, 3)}},
			{Condition: cond(2, 1, 2, 0, 4, 100)}, // ANDed into the rule above.
			{Condition: cond(0, 0, 0, 0, 0, 0), Operations: []assets.Operation{actor(3, 2)}},
			{Condition: cond(5, 501, 2, 0, 0, 0), Operations: []assets.Operation{actor(3, 3)}},
		}}},
		Events: []assets.Event{{ClickID: 4, Branches: []assets.Branch{{Condition: cond(5, 600, 1, 0, 5, 1)}}}},
	}
	return New(&assets.Catalog{Maps: map[uint16]assets.Map{m.ID: m}, NPCs: map[uint16]assets.NPC{100: {ID: 100, Type: 6}, 200: {ID: 200, Type: 1}}})
}

func character(quests ...game.Quest) *game.Character {
	c := &game.Character{ID: 0x01020304, Map: 12345, Quests: map[uint32]game.Quest{}}
	for _, q := range quests {
		c.Quests[q.ID] = q
	}
	return c
}

func TestMapSpawnsFollowNativeLoader(t *testing.T) {
	m, _ := fixture().Map(12345)
	if len(m.NPCs) != 3 || m.NPCs[0].ClickID != 1 || m.NPCs[1].ClickID != 3 || m.NPCs[2].ClickID != 4 {
		t.Fatalf("npcs: %+v", m.NPCs)
	}
	if len(m.Items) != 2 || m.Items[0].Slot != 7 || m.Items[0].Respawn.Seconds() != 30 || m.Items[1].Slot != 2 || m.Items[1].Respawn.Seconds() != 120 {
		t.Fatalf("items: %+v", m.Items)
	}
}

func TestPreEventVisibility(t *testing.T) {
	w := fixture()
	fresh := character()
	if w.Visible(fresh, 12345, 1) || !w.Visible(fresh, 12345, 3) {
		t.Fatal("fresh visibility")
	}
	active := character(game.Quest{ID: 500, State: game.InProgress, Step: 1})
	active.Gold = 99
	if w.Visible(active, 12345, 1) {
		t.Fatal("rule matched without its ANDed gold condition")
	}
	active.Gold = 100
	if !w.Visible(active, 12345, 1) {
		t.Fatal("matching rule did not show actor")
	}
	// The unconditional hide is overridden only while quest 501 is inactive.
	if w.Visible(character(game.Quest{ID: 501, State: game.InProgress}), 12345, 3) {
		t.Fatal("later rule order not applied")
	}
	if w.Visible(character(), 12000, 28) || !w.Visible(character(game.Quest{ID: 13046, State: game.Completed}), 12000, 28) {
		t.Fatal("story override for Lina's dog")
	}
}

func TestConditionValues(t *testing.T) {
	w := fixture()
	c := character(game.Quest{ID: 7, State: game.InProgress, Step: 3})
	c.Bag[0] = game.Item{ID: 48019, Count: 2} // Capsule variant of vehicle 48001.
	for _, tc := range []struct {
		cond [21]byte
		want bool
	}{
		{cond(5, 7, 1, 0, 5, 3), true},
		{cond(5, 7, 1, 0, 2, 3), false},
		{cond(5, 8, 2, 0, 0, 0), true},
		{cond(2, 1, 1, 48001, 4, 2), true},
		{cond(2, 1, 1, 48002, 4, 1), false},
		{cond(15, 1, 0, 0, 5, 49), true},
		{cond(2, 2, 2, 17162, 0, 0), true},
		{cond(3, 1, 3, 0, 5, 0), false},
		{cond(99, 0, 0, 0, 0, 0), false},
		{cond(5, 7, 1, 0, 1, 0xff000000), false}, // Negative packed values stay signed.
	} {
		if got := w.preEventCondition(c, 12345, tc.cond); got != tc.want {
			t.Errorf("%v: got %v", tc.cond[:13], got)
		}
	}
}

func TestMapInfoGolden(t *testing.T) {
	w := fixture()
	id := uint32(0x01020304)
	want := [][]byte{
		{23, 138},
		{22, 4, 1, 0, 0xff, 0xff, 100, 0, 100, 0, 2, 0x18, 0xfc, 0xe7, 0x03, 0, 3, 0, 0, 0, 0x2c, 1, 0x90, 1, 1, 0, 0, 0, 0, 0, 4, 0, 0xff, 0, 50, 0, 60, 0, 1, 0, 0, 0, 0, 0},
		{23, 4, 3, 7, 0, 0x31, 0x75, 0, 0, 10, 0, 20, 0, 0, 0, 0, 0, 3, 2, 0, 0x32, 0x75, 0, 0, 11, 0, 21, 0, 0, 0, 0, 0},
		protocol.Builder{23, 122}.U32(id), protocol.Builder{10, 3}.U32(id).U8(255), protocol.Builder{23, 76}.U32(id),
		{23, 102}, {20, 8},
	}
	got := w.MapInfo(character(), NewView(), []uint32{id})
	if len(got) != len(want) {
		t.Fatalf("got %d packets", len(got))
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("packet %d:\n got %v\nwant %v", i, got[i], want[i])
		}
	}
	// A completed linked quest shows the prop's opened frame.
	opened := w.MapInfo(character(game.Quest{ID: 600, State: game.Completed}), NewView(), nil)[1]
	if !bytes.Equal(opened[2+28:2+32], []byte{4, 0, 1, 0}) {
		t.Fatalf("opened prop: %v", opened[30:])
	}
}

func TestPortalLookupPriorities(t *testing.T) {
	maps := map[uint16]assets.Map{
		11000: {ID: 11000, Warps: []assets.Warp{{ClickID: 5, MapID: 11001, X: 100, Y: 200}, {ClickID: 9, MapID: 11002, X: 300, Y: 400}}},
		11001: {ID: 11001, Warps: []assets.Warp{{ClickID: 1, MapID: 11000, X: 1000, Y: 1000}}},
		11002: {ID: 11002, Warps: []assets.Warp{{ClickID: 1, MapID: 11000, X: 2000, Y: 2000}}},
		11003: {ID: 11003, Warps: []assets.Warp{{ClickID: 1}, {ClickID: 2, MapID: 11000, X: 7, Y: 8}}},
	}
	w := New(&assets.Catalog{Maps: maps})
	for _, tc := range []struct {
		name            string
		m, portal, x, y uint16
		want            Destination
		ok              bool
	}{
		{"reverse geometry", 11000, 99, 1010, 1000, Destination{11001, 100, 200}, true},
		{"click ID", 11000, 9, 5000, 5000, Destination{11002, 300, 400}, true},
		{"index", 11000, 2, 0, 0, Destination{11002, 300, 400}, true},
		{"gray code", 11000, 13, 0, 0, Destination{11002, 300, 400}, true},
		{"single exit", 11003, 50, 0, 0, Destination{11000, 7, 8}, true},
		{"carnie", 11094, 1, 0, 0, carnieFallback, true},
		{"test map", 500, 1, 0, 0, Destination{12000, 892, 734}, true},
		{"unknown", 11000, 50, 0, 0, Destination{}, false},
		{"missing map", 11004, 1, 0, 0, Destination{}, false},
	} {
		got, ok := w.Portal(tc.m, tc.portal, tc.x, tc.y)
		if got != tc.want || ok != tc.ok {
			t.Errorf("%s: got %+v %v", tc.name, got, ok)
		}
	}
}

// nativeWorld loads the sibling game data, or skips when it is not configured.
func nativeWorld(t *testing.T) *World {
	t.Helper()
	dir := os.Getenv("WONDERLAND_TEST_DATA")
	if dir == "" && os.Getenv("WONDERLAND_TEST_ASSETS_DB") == "" {
		t.Skip("WONDERLAND_TEST_ASSETS_DB or WONDERLAND_TEST_DATA not set")
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join("..", "..", dir)
	}
	var catalog *assets.Catalog
	var err error
	if os.Getenv("WONDERLAND_TEST_ASSETS_DB") == "" {
		catalog, err = assets.Load(dir, filepath.Join("../..", "data/item_data.json"))
	}
	if path := os.Getenv("WONDERLAND_TEST_ASSETS_DB"); path != "" {
		if !filepath.IsAbs(path) {
			path = filepath.Join("..", "..", path)
		}
		catalog, err = assetsql.LoadDatabase(path)
	}
	if err != nil {
		t.Fatal(err)
	}
	return New(catalog)
}

func TestNativeWorld(t *testing.T) {
	w := nativeWorld(t)
	fresh := &game.Character{ID: 1, Quests: map[uint32]game.Quest{}}
	npcs, hidden, items, portals := 0, 0, 0, 0
	for id, m := range w.maps {
		fresh.Map = id
		info := w.MapInfo(fresh, NewView(), []uint32{1})
		if !bytes.Equal(info[len(info)-1], []byte{20, 8}) {
			t.Fatalf("map %d info does not release the scene", id)
		}
		npcs += len(m.NPCs)
		items += len(m.Items)
		for _, n := range m.NPCs {
			if !w.Visible(fresh, id, n.ClickID) {
				hidden++
			}
		}
		for _, warp := range m.data.Warps {
			if dst, ok := w.Portal(id, warp.ClickID, 0, 0); ok {
				if _, loaded := w.Map(dst.Map); loaded {
					portals++
				}
			}
		}
	}
	t.Logf("maps=%d npcs=%d hidden_for_new_character=%d ground_items=%d resolvable_warps=%d", len(w.maps), npcs, hidden, items, portals)
	if len(w.maps) != 1119 || npcs == 0 || hidden == 0 || items == 0 || portals == 0 {
		t.Fatal("native world census failed")
	}
}

func TestRidingVehicleConditions(t *testing.T) {
	w := fixture()
	c := character()
	condition := cond(2, 1, 4, 48010, 5, 1)
	if w.preEventCondition(c, 12345, condition) {
		t.Fatal("bag ownership mistaken for riding")
	}
	c.Bag[0] = game.Item{ID: 48028, Count: 1}
	if w.preEventCondition(c, 12345, condition) {
		t.Fatal("unboarded capsule qualified")
	}
	c.ActiveVehicle = 48028
	c.VehicleSlot = 1
	if !w.preEventCondition(c, 12345, condition) {
		t.Fatal("boarded capsule alias did not qualify")
	}
	if w.preEventCondition(c, 12345, cond(2, 1, 4, 48016, 5, 1)) {
		t.Fatal("another raft qualified")
	}
	c.ActiveVehicle = 0
	if !w.preEventCondition(c, 12345, cond(2, 1, 4, 48010, 5, 0)) {
		t.Fatal("dismount condition did not qualify")
	}
}

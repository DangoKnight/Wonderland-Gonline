package server

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"testing"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/world"
)

func TestNativeSynchronizationGolden(t *testing.T) {
	s, c, wire, _ := mallFixture(t)
	c.ready = false // These queries must work during scene loading.
	for _, tc := range []struct {
		request []byte
		replies [][]byte
	}{
		{[]byte{183, 17}, [][]byte{{183, 17, 0}, {183, 11, 9, 2}}},
		{[]byte{186, 9}, [][]byte{{186, 9, 1, 0, 1, 0, 0, 0, 0}}},
		{[]byte{186, 9, 0x34, 0x12}, [][]byte{{186, 9, 0x34, 0x12, 1, 0, 0, 0, 0}}},
		{[]byte{226, 255}, [][]byte{{238, 183, 0, 255, 27, 0, 1, 29, 0, 2, 24, 0, 0}, {225, 252, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}}},
	} {
		if err := s.dispatch(context.Background(), c, tc.request); err != nil {
			t.Fatal(err)
		}
		got := wire.packets(t)
		if len(got) != len(tc.replies) {
			t.Fatalf("%v: %v", tc.request, got)
		}
		for i := range got {
			if !bytes.Equal(got[i], tc.replies[i]) {
				t.Fatalf("%v: %v", tc.request, got)
			}
		}
	}
	if err := s.dispatch(context.Background(), c, []byte{4, 1}); err != nil {
		t.Fatal(err)
	}
	packet, _ := c.character.AppearancePacket(true)
	if got := wire.packets(t); len(got) != 1 || !bytes.Equal(got[0], packet) {
		t.Fatal(got)
	}
	for _, p := range [][]byte{{183, 17, 0}, {186, 9, 1}, {226, 255, 1}, {4, 1, 0}, {21, 1, 0}, {21, 3, 0}} {
		if err := s.dispatch(context.Background(), c, p); !errors.Is(err, protocol.ErrMalformed) {
			t.Fatalf("%v: %v", p, err)
		}
		if wire.Len() != 0 {
			t.Fatal("malformed sync replied")
		}
	}
}

func TestNativeSceneAndGestureIsolation(t *testing.T) {
	s, players, wires := worldFixture(t)
	for _, c := range players {
		if err := s.dispatch(context.Background(), c, []byte{12, 1}); err != nil {
			t.Fatal(err)
		}
	}
	for _, w := range wires {
		w.Reset()
	}
	c := players[0]
	if err := s.dispatch(context.Background(), c, []byte{22, 8, 0x34, 0x12}); err != nil {
		t.Fatal(err)
	}
	if got := wires[0].packets(t); len(got) != 1 || !bytes.Equal(got[0], []byte{22, 8, 0x34, 0x12, 1}) {
		t.Fatal(got)
	}
	if err := s.dispatch(context.Background(), c, []byte{18, 2, 9, 0}); err != nil {
		t.Fatal(err)
	}
	want := protocol.Builder{18, 2}.U32(c.character.ID).U16(9)
	for _, w := range wires[:2] {
		got := w.packets(t)
		if len(got) != 1 || !bytes.Equal(got[0], want) {
			t.Fatal(got)
		}
	}
	if wires[2].Len() != 0 {
		t.Fatal("gesture leaked to another map")
	}
	if err := s.dispatch(context.Background(), c, []byte{18, 2, 9}); !errors.Is(err, protocol.ErrMalformed) {
		t.Fatal(err)
	}
	// Clearing callbacks must not depend on the current pose, or minigame gates.
	c.event = &eventSession{onMinigame: func(byte) error { return nil }}
	c.emote = 0
	if err := s.dispatch(context.Background(), c, []byte{32, 3}); err != nil || c.event != nil {
		t.Fatal("pose stop retained interaction", err)
	}
}

func TestNativeMallAliasesAndAtomicPurchase(t *testing.T) {
	s, c, wire, _ := mallFixture(t)
	ctx := context.Background()
	c.ready = false
	if err := s.dispatch(ctx, c, []byte{13, 238}); err != nil {
		t.Fatal(err)
	}
	packets := wire.packets(t)
	if len(packets) != 5 || !bytes.Equal(packets[0], protocol.Builder{13, 42}.U32(c.character.ID)) || packets[3][1] != 1 || packets[4][1] != 10 {
		t.Fatal(packets)
	}
	if err := s.dispatch(ctx, c, []byte{21, 1}); err != nil {
		t.Fatal(err)
	}
	packets = wire.packets(t)
	window := []byte{21, 1}
	for i := byte(1); i <= 21; i++ {
		window = append(window, i)
	}
	if len(packets) != 3 || !bytes.Equal(packets[2], window) {
		t.Fatal(packets)
	}
	c.ready = true
	if err := s.dispatch(ctx, c, []byte{21, 2, 1}); err != nil {
		t.Fatal(err)
	}
	balances, err := s.Store.MallBalances(ctx, c.account.ID)
	if err != nil || balances.Points != 90 {
		t.Fatal(balances, err)
	}
	// The bundle size is five; the slot purchase must not square it to 25.
	if c.character.Bag[0].ID != 32176 || c.character.Bag[0].Count != 5 {
		t.Fatal(c.character.Bag)
	}
	saved, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || saved[0].Bag != c.character.Bag {
		t.Fatal(saved, err)
	}
	wire.Reset()
	if err := s.dispatch(ctx, c, []byte{21, 2, 0}); err != nil {
		t.Fatal(err)
	}
	balances, _ = s.Store.MallBalances(ctx, c.account.ID)
	if balances.Points != 90 {
		t.Fatal("invalid slot charged")
	}
}

func TestNativeMetadataDurabilityAndRollback(t *testing.T) {
	s, c, wire, path := mallFixture(t)
	ctx := context.Background()
	setAutosaveBaseline(c)
	c.character.X++ // Metadata commits must preserve dirty walking.
	oldX := c.autosaveBaseline.X
	for _, p := range [][]byte{{44, 2, 0x34, 0x12}, {66, 11, 6}} {
		if err := s.dispatch(ctx, c, p); err != nil {
			t.Fatal(err)
		}
	}
	got := wire.packets(t)
	if len(got) != 3 || !bytes.Equal(got[0], []byte{44, 2, 0x34, 0x12}) || !bytes.Equal(got[1], protocol.Builder{44, 1}.U32(c.character.ID).U16(0x1234)) || !bytes.Equal(got[2], []byte{66, 11, 6, 1}) {
		t.Fatal(got)
	}
	saved, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || saved[0].Title != 0x1234 || saved[0].RebornJob != 6 || saved[0].X != oldX || saved[0].Reborn {
		t.Fatal(saved, err)
	}
	if err = s.autosaveSession(ctx, c); err != nil {
		t.Fatal("metadata conflicted with pending movement", err)
	}
	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err = raw.Exec("CREATE TRIGGER reject_native BEFORE UPDATE ON character_state BEGIN SELECT RAISE(ABORT,'reject native'); END"); err != nil {
		t.Fatal(err)
	}
	for _, p := range [][]byte{{44, 1, 9, 0}, {66, 11, 1}, {87, 1}} {
		if err = s.dispatch(ctx, c, p); err == nil {
			t.Fatal("failed SQL returned success")
		}
		if wire.Len() != 0 {
			t.Fatal("failed SQL emitted success")
		}
	}
	if c.character.Title != 0x1234 || c.character.RebornJob != 6 {
		t.Fatal("failed SQL altered session")
	}
}

func TestNativeBathAndFishingDisabledRewards(t *testing.T) {
	s, c, wire, _ := mallFixture(t)
	ctx := context.Background()
	c.character.HP, c.character.SP = 1, 0
	if err := s.dispatch(ctx, c, []byte{87, 1}); err != nil {
		t.Fatal(err)
	}
	saved, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || saved[0].HP != saved[0].MaxHP || saved[0].SP != saved[0].MaxSP {
		t.Fatal(saved, err)
	}
	got := wire.packets(t)
	if !bytes.Equal(got[len(got)-1], []byte{87, 1, 1}) {
		t.Fatal(got)
	}
	bag := c.character.Bag
	for _, p := range [][]byte{{90, 1}, {90, 2}, {90, 2}, {90, 3}} {
		if err := s.dispatch(ctx, c, p); err != nil {
			t.Fatal(err)
		}
	}
	if c.fishing != nil || c.character.Bag != bag {
		t.Fatal("pending fishing granted a reward")
	}
	got = wire.packets(t)
	if !contains(got, []byte{90, 1, 0}) || !bytes.Equal(got[len(got)-1], []byte{90, 3, 0}) {
		t.Fatal(got)
	}
}

func TestNativeWaypointBufferedMovement(t *testing.T) {
	s, c, wire, _ := mallFixture(t)
	ctx := context.Background()
	setAutosaveBaseline(c)
	if err := s.dispatch(ctx, c, []byte{7, 1, 0xb0, 4, 0x14, 5}); err != nil {
		t.Fatal(err)
	}
	if c.character.X != 1200 || c.character.Y != 1300 {
		t.Fatal(c.character.X, c.character.Y)
	}
	got := wire.packets(t)
	if len(got) != 2 || !bytes.Equal(got[1], []byte{7, 1, 0xb0, 4, 0x14, 5}) {
		t.Fatal(got)
	}
	saved, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || saved[0].X == 1200 {
		t.Fatal("waypoint saved per packet", err)
	}
	c.event = &eventSession{}
	if err := s.dispatch(ctx, c, []byte{7, 1, 100, 0, 100, 0}); err != nil {
		t.Fatal(err)
	}
	if c.character.X != 1200 {
		t.Fatal("waypoint bypassed event lock")
	}
	if err := s.dispatch(ctx, c, []byte{7, 1, 1}); !errors.Is(err, protocol.ErrMalformed) {
		t.Fatal(err)
	}
}

func TestNativeMetadataEntryReplay(t *testing.T) {
	s, c, _, _ := mallFixture(t)
	c.character.Title, c.character.RebornJob = 321, 6
	packets, err := s.worldEntryPackets(*c.character, c.view, c.pets)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(packets, []byte{44, 1, 65, 1}) || !contains(packets, []byte{66, 11, 6, 1}) {
		t.Fatal("native metadata missing from login")
	}
	peer, err := peerPackets(*c.character, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(peer, protocol.Builder{44, 1}.U32(c.character.ID).U16(321)) || peer[len(peer)-1][0] != 7 {
		t.Fatal("title replay broke position ordering", peer)
	}
}

func TestNativeWaypointTerrainAndMapValidation(t *testing.T) {
	s, c, wire, _ := mallFixture(t)
	ctx := context.Background()
	cells := make([]byte, 10000)
	cells[60*100+65] = 23
	s.Assets.Terrains = map[uint16]assets.Terrain{c.character.Map: {Width: 2000, Height: 2000, GridWidth: 100, GridHeight: 100, Cells: cells}}
	s.World = world.New(s.Assets)
	x, y := c.character.X, c.character.Y
	if err := s.dispatch(ctx, c, []byte{7, 1, 0xb0, 4, 0x14, 5}); err != nil {
		t.Fatal(err)
	}
	got := wire.packets(t)
	if c.character.X != x || c.character.Y != y || len(got) != 2 || !bytes.Equal(got[0], c.character.PositionPacket()) {
		t.Fatal("waypoint bypassed collision", got)
	}
	if err := s.dispatch(ctx, c, protocol.Builder{7, 1}.U16(20000).U16(1100).U16(1100)); err != nil {
		t.Fatal(err)
	}
	got = wire.packets(t)
	if c.character.X != x || len(got) != 1 || !bytes.Equal(got[0], c.character.PositionPacket()) {
		t.Fatal("waypoint accepted foreign map", got)
	}
	if err := s.dispatch(ctx, c, protocol.Builder{7, 1}.U16(c.character.Map).U16(1100).U16(1100)); err != nil {
		t.Fatal(err)
	}
	if c.character.X != 1100 || c.character.Y != 1100 {
		t.Fatal("map-hint waypoint rejected")
	}
}

func TestNativeMallFullBagDoesNotCharge(t *testing.T) {
	s, c, _, _ := mallFixture(t)
	ctx := context.Background()
	for i := range c.character.Bag {
		c.character.Bag[i] = game.Item{ID: 32177, Count: 50}
	}
	if err := s.Store.UpdateCharacter(ctx, c.account.ID, c.character.ID, func(stored *game.Character) error { stored.Bag = c.character.Bag; return nil }); err != nil {
		t.Fatal(err)
	}
	before := c.character.Bag
	if err := s.dispatch(ctx, c, []byte{21, 2, 1}); err != nil {
		t.Fatal(err)
	}
	balance, err := s.Store.MallBalances(ctx, c.account.ID)
	if err != nil || balance.Points != 100 || c.character.Bag != before {
		t.Fatal("full bag charged or changed", balance, err)
	}
	saved, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || saved[0].Bag != before {
		t.Fatal("full bag SQL changed", err)
	}
}

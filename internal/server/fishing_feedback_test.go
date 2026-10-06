package server

import (
	"bytes"
	"context"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func TestFishingNativeFeedbackAndCumulativeProgress(t *testing.T) {
	s, c, wire := fishingFixture(t)
	ctx := context.Background()
	item := s.Assets.Items[44001]
	item.Name = "Tilapia"
	s.Assets.Items[44001] = item
	s.Assets.Fishing.CatchRequirements = []uint32{2, 3}
	s.Assets.Skills[15993] = assets.Skill{ID: 15993, TableOrder: 321}
	// A real drop must remain unchanged and unavailable to a second grant.
	s.World.Drop(c.character.Map, []byte{1}, game.Item{ID: 44001, Count: 1}, 0, 0)
	beforeGround := s.World.GroundPacket(c.character.Map)
	peerWire := &captureConn{}
	peerChar := c.character.Clone()
	peerChar.ID++
	peer := &Session{conn: peerWire, info: SessionInfo{ID: 2}, character: &peerChar, ready: true}
	s.world[peer.info.ID] = peer
	if err := s.startFishing(ctx, c, 1); err != nil {
		t.Fatal(err)
	}
	peerWire.Reset()
	for i := 0; i < 3; i++ {
		if err := s.catchFishing(ctx, c, c.fishing.NextAt); err != nil {
			t.Fatal(err)
		}
	}
	packets := wire.packets(t)
	if !contains(packets, []byte{23, 51, 0, 225, 171, 1}) {
		t.Fatal("missing localized catch chat", packets)
	}
	// Independent wire expectation: AC2:16, zero speaker ID, raw notification.
	notice := append([]byte{2, 16, 0, 0, 0, 0}, []byte("Fishing: caught [Tilapia] x1.")...)
	notices := 0
	for _, p := range packets {
		if bytes.Equal(p, notice) {
			notices++
		}
	}
	if notices != 3 {
		t.Fatalf("got %d private catch notifications, want 3", notices)
	}
	// Native incremental ground presentation at the selected water-cell center,
	// followed immediately by animated removal. Slot 1 belongs to the real drop.
	if !contains(packets, []byte{23, 3, 225, 171, 10, 0, 30, 0, 0, 0, 0, 0}) || !contains(packets, []byte{23, 2, 2, 0, 1}) {
		t.Fatal("missing item flight", packets)
	}
	if !contains(packets, []byte{8, 1, 111, 1, 3, 0, 0, 0, 121, 62, 0, 0}) {
		t.Fatal("missing cumulative EXP update", packets)
	}
	for _, p := range packets {
		if p[0] == 5 && p[1] == 11 {
			t.Fatal("fishing sent unrelated AC5:11", p)
		}
	}
	if len(peerWire.packets(t)) != 0 {
		t.Fatal("catcher feedback leaked to observer")
	}
	if !bytes.Equal(beforeGround, s.World.GroundPacket(c.character.Map)) {
		t.Fatal("visual mutated shared ground")
	}
	if _, ok := s.World.GroundAt(c.character.Map, 2); ok {
		t.Fatal("visual became claimable")
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	saved := chars[0]
	if saved.Skills[0].Grade != 2 || saved.Skills[0].EXP != 1 || saved.Bag[1].Count != 3 {
		t.Fatal("incorrect durable progress", saved.Skills, saved.Bag)
	}
	native := s.nativeStatsSnapshot(saved)
	if native.Skills[0].EXP != 3 || saved.Skills[0].EXP != 1 {
		t.Fatal("reconnect conversion mutated persisted state")
	}
	snapshot, err := native.BaseStatsPacket(func(id uint16) (uint16, bool) { return s.Assets.Skills[id].TableOrder, true })
	if err != nil || !bytes.Contains(snapshot, []byte{65, 1, 2, 3, 0, 0, 0}) {
		t.Fatal("native reconnect snapshot", snapshot, err)
	}
}

func TestFishingPeerRodVisibilityAndCancellation(t *testing.T) {
	s, c, wire := fishingFixture(t)
	ctx := context.Background()
	native := assets.NativeItem{}
	native.Record[45] = 5
	s.Assets.NativeItems = map[uint16]assets.NativeItem{37105: native}
	var wires []*captureConn
	for i := 0; i < 3; i++ {
		char := c.character.Clone()
		char.ID += uint32(i + 1)
		peerWire := &captureConn{}
		peer := &Session{conn: peerWire, info: SessionInfo{ID: uint64(i + 2)}, character: &char, ready: true}
		if i == 1 {
			peer.character.Map++
		}
		if i == 2 {
			peer.tentOwner = char.ID
		}
		s.world[peer.info.ID] = peer
		wires = append(wires, peerWire)
	}
	if err := s.dispatch(ctx, c, []byte{23, 53, 1, 255}); err != nil {
		t.Fatal(err)
	}
	want := protocol.Builder{23, 123}.U32(c.character.ID).U8(5)
	if !contains(wires[0].packets(t), want) {
		t.Fatal("SQL rod presentation missing")
	}
	if len(wires[1].packets(t)) != 0 || len(wires[2].packets(t)) != 0 || len(wire.packets(t)) != 0 {
		t.Fatal("rod state leaked scenes or echoed owner's slot update")
	}
	// Stop is idempotent and peer-only; the native owner clears its slot locally.
	if err := s.dispatch(ctx, c, []byte{23, 54}); err != nil {
		t.Fatal(err)
	}
	if !contains(wires[0].packets(t), protocol.Builder{23, 122}.U32(c.character.ID)) {
		t.Fatal("missing peer stop")
	}
	s.stopFishing(c)
	if len(wires[0].packets(t)) != 0 || len(wire.packets(t)) != 0 {
		t.Fatal("duplicate stop")
	}
	if err := s.startFishing(ctx, c, 1); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	c.character.X++
	if err := s.catchFishing(ctx, c, c.fishing.NextAt); err != nil {
		t.Fatal(err)
	}
	if c.fishing != nil || !contains(wires[0].packets(t), protocol.Builder{23, 122}.U32(c.character.ID)) {
		t.Fatal("interruption left rod visible")
	}
}

func TestFishingLateArrivalReplaysRod(t *testing.T) {
	s, players, wires := worldFixture(t)
	if err := s.acknowledgeWorld(players[0]); err != nil {
		t.Fatal(err)
	}
	players[0].fishing = &fishingRun{Presentation: 3}
	if err := s.acknowledgeWorld(players[1]); err != nil {
		t.Fatal(err)
	}
	if !contains(wires[1].packets(t), protocol.Builder{23, 123}.U32(players[0].character.ID).U8(3)) {
		t.Fatal("late observer missing rod")
	}
}

func TestFishingFullBagFeedbackAndRollback(t *testing.T) {
	s, c, wire := fishingFixture(t)
	ctx := context.Background()
	for i := 1; i < len(c.character.Bag); i++ {
		c.character.Bag[i] = game.Item{ID: 32176, Count: 50}
	}
	if err := s.commit(ctx, c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	if err := s.startFishing(ctx, c, 1); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.catchFishing(canceled, c, c.fishing.NextAt); err == nil {
		t.Fatal("canceled transaction accepted")
	}
	if len(wire.packets(t)) != 0 {
		t.Fatal("uncommitted feedback")
	}
	if err := s.catchFishing(ctx, c, c.fishing.NextAt); err != nil {
		t.Fatal(err)
	}
	packets := wire.packets(t)
	for _, p := range packets {
		if bytes.Contains(p, []byte("Fishing: caught [")) {
			t.Fatal("discarded catch sent success notification", p)
		}
		if p[0] == 23 && (p[1] == 51 || p[1] == 3 || p[1] == 2) {
			t.Fatal("discarded catch falsely displayed as received", p)
		}
	}
	if !contains(packets, []byte{8, 1, 111, 1, 1, 0, 0, 0, 121, 62, 0, 0}) {
		t.Fatal("full bag lost proficiency", packets)
	}
}

func TestFishingVisualSkipsReservedClientSlot(t *testing.T) {
	s, c, _ := fishingFixture(t)
	ctx := context.Background()
	if err := s.startFishing(ctx, c, 1); err != nil {
		t.Fatal(err)
	}
	// All 255 real ground slots occupied: delivery still succeeds without a visual.
	slots := make([]byte, 255)
	for i := range slots {
		slots[i] = byte(i + 1)
	}
	s.World.Drop(c.character.Map, slots, game.Item{ID: 44001, Count: 1}, 0, 0)
	if got := s.fishingCatchAnimation(c, 44001); len(got) != 0 {
		t.Fatal("occupied ground replaced", got)
	}
	if err := s.catchFishing(ctx, c, c.fishing.NextAt); err != nil {
		t.Fatal(err)
	}
	if c.character.Bag[1].ID != 44001 {
		t.Fatal("visual availability prevented durable catch")
	}
}

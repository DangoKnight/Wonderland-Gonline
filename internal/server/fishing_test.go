package server

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
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

func fishingFixture(t *testing.T) (*Server, *Session, *captureConn) {
	t.Helper()
	s, c, wire, _ := mallFixture(t)
	s.Assets.Fishing = assets.FishingRules{Enabled: true, IntervalSeconds: 60, Skills: []uint16{15993}, CatchRequirements: []uint32{1, 35}, Maps: []uint16{c.character.Map}, Rods: []assets.FishingRod{{ItemID: 37105, MaxGrade: 6}}, Rewards: []assets.FishingReward{{ItemID: 44001, Grade: 1, Weight: 1}}}
	s.Assets.Items[37105] = game.ItemDefinition{ID: 37105, Type: 28}
	s.Assets.Items[44001] = game.ItemDefinition{ID: 44001, Type: 32}
	c.character.X, c.character.Y = 0, 0
	c.character.Bag = game.Inventory{}
	c.character.Bag[0] = game.Item{ID: 37105, Count: 1}
	c.character.Skills = []game.LearnedSkill{{ID: 15993, Grade: 1}}
	if s.Assets.Terrains == nil {
		s.Assets.Terrains = map[uint16]assets.Terrain{}
	}
	s.Assets.Terrains[c.character.Map] = assets.Terrain{Width: 40, Height: 40, GridWidth: 2, GridHeight: 2, Cells: []byte{0, 2, 0, 0}}
	if err := s.commit(context.Background(), c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	return s, c, wire
}
func TestFishingNativeStartStopAndTimedCatch(t *testing.T) {
	s, c, wire := fishingFixture(t)
	ctx := context.Background()
	if err := s.dispatch(ctx, c, []byte{23, 53}); err != nil || c.fishing == nil {
		t.Fatal(c.fishing, err)
	}
	due := c.fishing.NextAt
	if err := s.dispatch(ctx, c, []byte{90, 2}); err != nil || !c.character.Bag[1].Empty() {
		t.Fatal(err, c.character.Bag)
	}
	if err := s.catchFishing(ctx, c, due); err != nil {
		t.Fatal(err)
	}
	if c.character.Bag[1].ID != 44001 || c.character.Skills[0].Grade != 2 {
		t.Fatal(c.character)
	}
	if err := s.catchFishing(ctx, c, due); err != nil || c.character.Bag[1].Count != 1 {
		t.Fatal("replay", err)
	}
	packets := wire.packets(t)
	if !contains(packets, []byte{8, 1, 110, 1, 2, 0, 0, 0, 121, 62, 0, 0}) {
		t.Fatal("missing committed fishing grade", packets)
	}
	if err := s.dispatch(ctx, c, []byte{23, 54}); err != nil || c.fishing != nil {
		t.Fatal(err)
	}
	if err := s.dispatch(ctx, c, []byte{23, 53, 1}); err == nil {
		t.Fatal("malformed start accepted")
	}
}
func TestFishingFullBagProgressAndWalkingMerge(t *testing.T) {
	s, c, _ := fishingFixture(t)
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
	// Walking stays buffered while SQL validates the catch's inventory.
	c.character.X = 1
	c.fishing.X = 1
	if err := s.catchFishing(ctx, c, c.fishing.NextAt); err != nil {
		t.Fatal(err)
	}
	if c.character.X != 1 || c.character.Skills[0].Grade != 2 || c.fishing == nil {
		t.Fatal("full bag progress or pending movement lost")
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].X != 0 || chars[0].Bag != c.character.Bag {
		t.Fatal(chars, err)
	}
	c.character.X++
	if err := s.catchFishing(ctx, c, time.Now().Add(time.Hour)); err != nil || c.fishing != nil {
		t.Fatal("movement failed to cancel", err)
	}
}

func TestFishingUnlearnedSkillAndFailedTransactionRetry(t *testing.T) {
	s, c, wire := fishingFixture(t)
	ctx := context.Background()
	c.character.Skills = nil
	if err := s.commit(ctx, c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	if err := s.startFishing(ctx, c, 1); err != nil {
		t.Fatal(err)
	}
	due := c.fishing.NextAt
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.catchFishing(canceled, c, due); err == nil {
		t.Fatal("canceled transaction accepted")
	}
	if c.fishing == nil || !c.character.Bag[1].Empty() || len(wire.packets(t)) != 0 {
		t.Fatal("failed catch published state")
	}
	if err := s.catchFishing(ctx, c, due); err != nil {
		t.Fatal(err)
	}
	if c.character.Bag[1].ID != 44001 || len(c.character.Skills) != 0 {
		t.Fatal("unlearned skill fishing changed skill ownership")
	}
	c.character.Bag[0] = game.Item{}
	if err := s.catchFishing(ctx, c, due); err != nil || c.fishing != nil {
		t.Fatal("lost rod failed to cancel", err)
	}
}
func TestFishingStartRequiresRodShoreAndNoParty(t *testing.T) {
	s, c, _ := fishingFixture(t)
	ctx := context.Background()
	c.party = &party{}
	if err := s.startFishing(ctx, c, 0); err != nil || c.fishing != nil {
		t.Fatal("party fishing accepted", err)
	}
	c.party = nil
	c.character.X = 40
	if err := s.startFishing(ctx, c, 0); err != nil || c.fishing != nil {
		t.Fatal("offshore fishing accepted", err)
	}
	c.character.X = 0
	c.character.Bag[0] = game.Item{}
	if err := s.startFishing(ctx, c, 0); err != nil || c.fishing != nil {
		t.Fatal("rodless fishing accepted", err)
	}
}

func TestFishingLegacyCatchReceiptAfterCommit(t *testing.T) {
	s, c, wire := fishingFixture(t)
	ctx := context.Background()
	if err := s.dispatch(ctx, c, []byte{90, 1}); err != nil {
		t.Fatal(err)
	}
	if !contains(wire.packets(t), []byte{90, 1, 1}) {
		t.Fatal("missing cast ACK")
	}
	due := c.fishing.NextAt
	if err := s.catchFishing(ctx, c, due); err != nil {
		t.Fatal(err)
	}
	if !contains(wire.packets(t), []byte{90, 2, 225, 171, 1}) {
		t.Fatal("missing committed catch receipt")
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Bag[1].ID != 44001 {
		t.Fatal("catch receipt preceded delivery", err)
	}
}

func waterGatheringFixture(t *testing.T) (*Server, *Session, *captureConn, *assets.Event) {
	s, players, wires := worldFixture(t)
	c := players[0]
	c.character.Map = 60001
	s.Assets.Items[60001] = game.ItemDefinition{ID: 60001, Type: 10, Name: "Sea Water"}
	if s.Assets.Marks == nil {
		s.Assets.Marks = make(map[uint16]uint16)
	}
	s.Assets.Marks[11009] = 0
	ev := assets.Event{ClickID: 88, Branches: []assets.Branch{
		{Index: 1, Condition: evCond(14, 11009, 1, 0, 0, 0), Operations: []assets.Operation{evOp(1, 1, 2, 10001, 0, 0)}},
		{Index: 2, Condition: evCond(14, 11009, 2, 0, 0, 0), Operations: []assets.Operation{evOp(1, 1, 1, 6, 7, 1), evOp(2, 14, 11009, 1, 0, 180)}},
	}}
	s.Assets.Maps[60001] = assets.Map{ID: 60001, Events: []assets.Event{ev}}
	s.World = world.New(s.Assets)
	if err := s.Store.UpdateCharacter(context.Background(), c.account.ID, c.character.ID, func(stored *game.Character) error { *stored = c.character.Clone(); return nil }); err != nil {
		t.Fatal(err)
	}
	return s, c, wires[0], &ev
}

func TestEventGatheringMultiplePoolsAtomicCooldown(t *testing.T) {
	s, c, _, _ := waterGatheringFixture(t)
	s.Assets.Items[11101] = game.ItemDefinition{ID: 11101, Type: 1}
	s.Assets.EventGathering = assets.EventGatheringRules{Timers: []uint16{11008}, Pools: []assets.EventGatheringPool{
		{Type: 1, Variant: 1, Items: []uint16{32176}},
		{Type: 2, Variant: 2, Items: []uint16{11101}},
	}}
	ev := assets.Event{ClickID: 88, Branches: []assets.Branch{{Index: 1, Condition: evCond(14, 11008, 2, 0, 0, 0), Operations: []assets.Operation{
		evOp(1, 1, 1, 6, 1, 1), evOp(2, 1, 1, 6, 2, 2), evOp(3, 14, 11008, 1, 0, 180),
	}}}}
	m := s.Assets.Maps[c.character.Map]
	m.Events = []assets.Event{ev}
	s.Assets.Maps[c.character.Map] = m
	s.World = world.New(s.Assets)
	now := time.Now().UTC()
	handled, err := s.tryWaterGathering(context.Background(), c, 88, &ev, 0, now)
	if !handled || err != nil {
		t.Fatal(handled, err)
	}
	var food, sword int
	for _, it := range c.character.Bag {
		switch it.ID {
		case 32176:
			food += int(it.Count)
		case 11101:
			sword += int(it.Count)
		}
	}
	if food != 51 || sword != 1 || !c.character.EventTimers[11008].Equal(now.Add(180*time.Second)) {
		t.Fatal("incomplete multi-pool reward")
	}
	before := c.character.Bag
	if _, err := s.tryWaterGathering(context.Background(), c, 88, &ev, 0, now); err != nil || c.character.Bag != before {
		t.Fatal("replay granted", err)
	}
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || stored[0].Bag != before || stored[0].Quests[11008].Step != 1 {
		t.Fatal("multi-pool durability", err)
	}
}

func TestWaterGatheringNativePacketsAndCooldown(t *testing.T) {
	s, c, wire, ev := waterGatheringFixture(t)
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	if branch := s.World.FindBranchAt(c.character, c.view, 60001, ev, 0, 0, 0, -1, now); branch != 1 {
		t.Fatal(branch)
	}
	s.worldMu.Lock()
	handled, err := s.tryWaterGathering(context.Background(), c, 88, ev, 1, now)
	s.worldMu.Unlock()
	if !handled || err != nil {
		t.Fatal(handled, err)
	}
	packets := wire.packets(t)
	if !bytes.Equal(packets[0], []byte{6, 2, 1}) {
		t.Fatal(packets)
	}
	wantAdd := append([]byte{23, 5, 2, 0x61, 0xea, 1, 0}, make([]byte, 26)...)
	if !bytes.Equal(packets[1], wantAdd) {
		t.Fatal("native addition", packets[1])
	}
	if len(packets) < 6 || !bytes.Equal(packets[2], []byte{24, 4, 1, 43}) || !bytes.Equal(packets[3], []byte{24, 1, 1, 43, 1}) || !bytes.Equal(packets[4], []byte{6, 2, 0}) || !bytes.Equal(packets[5], []byte{20, 8}) {
		t.Fatal("native mark and interaction release", packets)
	}
	if c.event != nil || c.character.Bag[1].Count != 1 || c.character.Quests[11009].Step != 1 || !c.character.EventTimers[11009].Equal(now.Add(180*time.Second)) {
		t.Fatal(c.character)
	}
	if branch := s.World.FindBranchAt(c.character, c.view, 60001, ev, 0, 0, 0, -1, now); branch != 0 {
		t.Fatal("cooldown branch not selected", branch)
	}
	if branch := s.World.FindBranchAt(c.character, c.view, 60001, ev, 0, 0, 0, -1, now.Add(180*time.Second)); branch != 1 {
		t.Fatal("expired timer branch", branch)
	}
	// A stale/replayed branch cannot grant despite current durable timer state.
	s.worldMu.Lock()
	handled, err = s.tryWaterGathering(context.Background(), c, 88, ev, 1, now)
	s.worldMu.Unlock()
	if !handled || err != nil || c.character.Bag[1].Count != 1 {
		t.Fatal(handled, err)
	}
	for _, p := range wire.packets(t) {
		if len(p) > 1 && p[0] == 23 && p[1] == 5 {
			t.Fatal("duplicate grant")
		}
	}
	// Native AC20 entry uses the same interception before rejecting opcode 14.
	delete(c.character.EventTimers, 11009)
	if err := s.Store.UpdateCharacter(context.Background(), c.account.ID, c.character.ID, func(stored *game.Character) error { delete(stored.EventTimers, 11009); return nil }); err != nil {
		t.Fatal(err)
	}
	s.worldMu.Lock()
	err = s.startEvent(context.Background(), c, 88, ev, 1, true)
	s.worldMu.Unlock()
	if err != nil || c.character.Bag[1].Count != 2 || c.event != nil {
		t.Fatal("ordinary event entry did not gather", err)
	}
}

func TestWaterGatheringRejectsInvalidBranchAndStaleConditions(t *testing.T) {
	for _, kind := range []string{"malformed", "foreign map", "full bag", "stale map", "stale branch", "missing SQL item", "disabled event"} {
		t.Run(kind, func(t *testing.T) {
			s, c, wire, ev := waterGatheringFixture(t)
			ctx := context.Background()
			switch kind {
			case "malformed":
				ev.Branches[1].Operations = append(ev.Branches[1].Operations, evOp(3, 1, 1, 2, 0, 999))
			case "foreign map":
				c.character.Map = 10017
			case "full bag":
				if err := s.Store.UpdateCharacter(ctx, c.account.ID, c.character.ID, func(stored *game.Character) error {
					for i := range stored.Bag {
						stored.Bag[i] = game.Item{ID: 60001, Count: 1}
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			case "stale map":
				if err := s.Store.UpdateCharacter(ctx, c.account.ID, c.character.ID, func(stored *game.Character) error { stored.Map = 10017; return nil }); err != nil {
					t.Fatal(err)
				}
			case "stale branch":
				ev.Branches[1].Condition = evCond(5, 900, 1, 0, 5, 1)
			case "missing SQL item":
				delete(s.Assets.Items, 60001)
			case "disabled event":
				if s.Assets.DisabledEvents == nil {
					s.Assets.DisabledEvents = make(map[uint32]string)
				}
				s.Assets.DisabledEvents[assets.EventKey(60001, 88)] = "missing"
			}
			s.worldMu.Lock()
			err := s.startEvent(ctx, c, 88, ev, 1, true)
			s.worldMu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			chars, err := s.Store.Characters(ctx, c.account.ID)
			if err != nil || len(chars[0].EventTimers) != 0 || chars[0].Quests[11009].ID != 0 {
				t.Fatal("unexpected timer/mark change", err)
			}
			for _, p := range wire.packets(t) {
				if len(p) > 1 && p[0] == 23 && p[1] == 5 {
					t.Fatal("rejected branch granted reward")
				}
			}
		})
	}
}

func TestInstalledWaterGatheringBranches(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	w := world.New(catalog)
	for _, node := range []struct{ mapID, event, timer, item uint16 }{{60001, 88, 11009, 60001}, {60003, 48, 11016, 60001}, {12268, 2, 11021, 60002}, {60005, 12, 11035, 60002}} {
		ev, ok := w.Event(node.mapID, node.event)
		if !ok {
			t.Fatalf("missing event %d/%d", node.mapID, node.event)
		}
		found := 0
		for i, br := range ev.Branches {
			plan, involved := world.WaterGathering(node.mapID, ev, i)
			if !involved {
				continue
			}
			if plan.Timer != node.timer || plan.Item != node.item {
				t.Fatalf("unsupported native shape %d/%d branch %d: %+v", node.mapID, node.event, br.Index, plan)
			}
			found++
		}
		if found != 2 {
			t.Fatalf("expected both initial/repeat reward branches for %d, got %d", node.mapID, found)
		}

		s, players, wires := worldFixture(t)
		c := players[0]
		s.Assets, s.World = catalog, w
		c.character.Map = node.mapID
		if err := s.Store.UpdateCharacter(context.Background(), c.account.ID, c.character.ID, func(stored *game.Character) error { *stored = c.character.Clone(); return nil }); err != nil {
			t.Fatal(err)
		}
		now := time.Now().UTC()
		branch := w.FindBranchAt(c.character, c.view, node.mapID, ev, 0, 0, 0, -1, now)
		if branch < 0 || ev.Branches[branch].Index != 5 {
			t.Fatal("initial authored reward branch missing", node.mapID, branch)
		}
		s.worldMu.Lock()
		handled, err := s.tryWaterGathering(context.Background(), c, node.event, ev, branch, now)
		s.worldMu.Unlock()
		if !handled || err != nil || c.character.Bag[1].ID != node.item || c.character.Bag[1].Count != 1 {
			t.Fatal("native initial grant", node.mapID, handled, err)
		}
		wires[0].Reset()
		repeatAt := now.Add(180 * time.Second)
		branch = w.FindBranchAt(c.character, c.view, node.mapID, ev, 0, 0, 0, -1, repeatAt)
		if branch < 0 || ev.Branches[branch].Index != 2 {
			t.Fatal("repeat authored reward branch missing", node.mapID, branch)
		}
		s.worldMu.Lock()
		handled, err = s.tryWaterGathering(context.Background(), c, node.event, ev, branch, repeatAt)
		s.worldMu.Unlock()
		if !handled || err != nil || c.character.Bag[1].Count != 2 {
			t.Fatal("native repeat grant", node.mapID, handled, err)
		}
		def, ok := catalog.Items[node.item]
		if !ok || def.StackLimit() != 50 {
			t.Fatal("native water reward unavailable", def)
		}
	}
}

type gatheringFailedReceipt struct {
	captureConn
	writes int
}

func (c *gatheringFailedReceipt) Write(p []byte) (int, error) {
	c.writes++
	if c.writes == 2 {
		return 0, io.ErrClosedPipe
	}
	return c.captureConn.Write(p)
}

func TestWaterGatheringFailedReceiptRemainsDurable(t *testing.T) {
	s, c, _, ev := waterGatheringFixture(t)
	c.conn = &gatheringFailedReceipt{}
	now := time.Now().UTC()
	s.worldMu.Lock()
	handled, err := s.tryWaterGathering(context.Background(), c, 88, ev, 1, now)
	s.worldMu.Unlock()
	if !handled || err == nil {
		t.Fatal(handled, err)
	}
	chars, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || chars[0].Bag[1].ID != 60001 || chars[0].Bag[1].Count != 1 || !chars[0].EventTimerActive(11009, now) {
		t.Fatal("committed grant lost after receipt failure", err)
	}
	c.character = &chars[0]
	c.conn = &captureConn{}
	s.worldMu.Lock()
	_, err = s.tryWaterGathering(context.Background(), c, 88, ev, 1, now)
	s.worldMu.Unlock()
	if err != nil || c.character.Bag[1].Count != 1 {
		t.Fatal("receipt failure duplicated reward", err)
	}
}

func TestWaterGatheringCanceledCommitDoesNotPublishReward(t *testing.T) {
	s, c, wire, ev := waterGatheringFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.worldMu.Lock()
	handled, err := s.tryWaterGathering(ctx, c, 88, ev, 1, time.Now())
	s.worldMu.Unlock()
	if !handled || err == nil || c.event != nil || len(c.character.EventTimers) != 0 || !c.character.Bag[1].Empty() {
		t.Fatal("failed commit changed session", handled, err)
	}
	for _, p := range wire.packets(t) {
		if len(p) > 1 && ((p[0] == 23 && p[1] == 5) || p[0] == 24) {
			t.Fatal("failed commit published a reward or mark", p)
		}
	}
}

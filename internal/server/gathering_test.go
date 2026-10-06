package server

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/world"
)

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

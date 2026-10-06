package server

import (
	"context"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/world"
)

func minigameFixture(t *testing.T, gameType, seed uint16) (*Server, *Session, *captureConn, *assets.Event) {
	s, c, wire := eventFixture(t)
	ev := &assets.Event{ClickID: 10, Branches: []assets.Branch{
		{Index: 1, Operations: []assets.Operation{evOp(1, 9, gameType, seed, 0, 0), evOp(2, 1, 1, 2, 0, 999)}},
		{Index: 2, Condition: evCond(8, gameType, 0, 0, 5, 1), Operations: []assets.Operation{evOp(1, 2, 5, 0, 20363, 0), evOp(2, 1, 1, 1, 32176, 10)}},
		{Index: 3, Condition: evCond(8, gameType, 0, 0, 5, 0), Operations: []assets.Operation{evOp(1, 2, 5, 0, 20364, 0)}},
	}}
	if err := s.startEvent(context.Background(), c, 5, ev, 0, false); err != nil {
		t.Fatal(err)
	}
	return s, c, wire, ev
}

func TestMinigameNativeStartResultsAndReplay(t *testing.T) {
	for _, result := range []byte{0, 1} {
		s, c, wire, _ := minigameFixture(t, 3, 0)
		packets := wire.packets(t)
		if len(packets) != 3 || !contains(packets, []byte{57, 1, 3, 0xf8, 0x2a, 1}) || !contains(packets, []byte{20, 9}) {
			t.Fatal("native minigame start", packets)
		}
		original := c.character.Bag
		es := c.event
		stale := es.onMinigame
		tradeDo(t, s, c, []byte{20, 6})
		if wire.Len() != 0 || c.event != es || es.onMinigame == nil {
			t.Fatal("ACK advanced minigame")
		}
		for _, p := range [][]byte{{57, 1}, {57, 1, 2}, {57, 1, 1, 0}} {
			tradeDo(t, s, c, p)
		}
		if wire.Len() != 0 || es.onMinigame == nil {
			t.Fatal("malformed result consumed callback")
		}
		tradeDo(t, s, c, []byte{57, 1, result})
		p := wire.packets(t)
		talk := uint16(20364)
		branch := byte(3)
		if result == 1 {
			talk = 20363
			branch = 2
		}
		if len(p) != 3 || !contains(p, []byte{57, 2}) || !contains(p, eventFrame(1, 3, 5, 1, 0, talk, 1, branch)) {
			t.Fatal("outcome packets", p)
		}
		if c.character.Bag != original || c.character.Gold != 0 {
			t.Fatal("reward before outcome dialogue")
		}
		tradeDo(t, s, c, []byte{57, 1, result})
		if wire.Len() != 0 {
			t.Fatal("result replayed during dialogue")
		}
		// The client acknowledges its own result (FUN_003bdfa4 → 20/6)
		// before the outcome dialogue is answered.
		tradeDo(t, s, c, []byte{20, 6})
		if wire.Len() != 0 || c.event == nil {
			t.Fatal("the result's acknowledgment closed the outcome dialogue")
		}
		tradeDo(t, s, c, []byte{20, 6})
		if c.event != nil || c.character.Gold != 0 {
			t.Fatal("minigame fell through origin branch")
		}
		saved := c.character.Bag
		if err := stale(1); err != nil || c.character.Bag != saved {
			t.Fatal("stale callback rewarded", err)
		}
		tradeDo(t, s, c, []byte{57, 1, 1})
		if c.character.Bag != saved {
			t.Fatal("late result rewarded")
		}
		chars, err := s.Store.Characters(context.Background(), c.account.ID)
		if err != nil || chars[0].Bag != saved {
			t.Fatal("outcome not durable", err)
		}
		if result == 1 && saved == original {
			t.Fatal("win did not give reward")
		}
		if result == 0 && saved != original {
			t.Fatal("loss gave reward")
		}
	}
}

func TestMinigameCancellationAndLock(t *testing.T) {
	for _, action := range []string{"cancel", "warp", "disconnect"} {
		s, c, wire, _ := minigameFixture(t, 4, 0x2710)
		if !contains(wire.packets(t), []byte{57, 1, 4, 0x10, 0x27, 1}) {
			t.Fatal("authored seed ignored")
		}
		es := c.event
		callback := es.onMinigame
		bag := c.character.Bag
		x := c.character.X
		tradeDo(t, s, c, []byte{23, 124, 1, 1, 0})
		tradeDo(t, s, c, []byte{6, 1, 0, 1, 0, 1, 0})
		if c.character.Bag != bag || c.character.X != x || wire.Len() != 0 {
			t.Fatal("game allowed inventory/movement mutation")
		}
		switch action {
		case "cancel":
			tradeDo(t, s, c, []byte{20, 9, 40})
		case "warp":
			s.Assets.Maps[20000] = assets.Map{ID: 20000}
			s.World = world.New(s.Assets)
			if err := s.commandTeleport(context.Background(), c, world.Destination{Map: 20000, X: 1, Y: 1}); err != nil {
				t.Fatal(err)
			}
		case "disconnect":
			s.leaveWorld(c)
		}
		if c.event != nil {
			t.Fatal("cancel retained game", action)
		}
		if err := callback(1); err != nil || c.character.Bag != bag {
			t.Fatal("cancelled callback rewarded", action, err)
		}
	}
}

func TestMinigameMissingOutcomeAndFailedRewardSave(t *testing.T) {
	s, c, wire, ev := minigameFixture(t, 1, 0)
	wire.Reset()
	ev.Branches = ev.Branches[:1]
	tradeDo(t, s, c, []byte{57, 1, 1})
	if c.event != nil || !contains(wire.packets(t), []byte{20, 8}) {
		t.Fatal("missing outcome retained interaction")
	}
	s, c, wire, _ = minigameFixture(t, 1, 0)
	wire.Reset()
	bag := c.character.Bag
	tradeDo(t, s, c, []byte{57, 1, 1})
	tradeDo(t, s, c, []byte{20, 6}) // the result's own acknowledgment
	wire.Reset()
	s.Store.Close()
	if err := s.dispatch(context.Background(), c, []byte{20, 6}); err == nil {
		t.Fatal("failed reward save ignored")
	}
	if c.character.Bag != bag || contains(wire.packets(t), []byte{20, 10}) {
		t.Fatal("failed save published reward")
	}
}

func TestNativeRabbitMinigameRewards(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, actor := range catalog.Maps[12000].NPCs {
		for _, event := range actor.Events {
			if event == 41 {
				t.Logf("rabbit actor=%d x=%d y=%d", actor.ClickID, actor.X, actor.Y)
			}
		}
	}
	for _, result := range []byte{0, 1} {
		s, c, wire := eventFixture(t)
		s.Assets = catalog
		s.World = world.New(catalog)
		next := c.character.Clone()
		next.Map = 12000
		next.Bag = game.Inventory{}
		if err := s.commit(context.Background(), c, next); err != nil {
			t.Fatal(err)
		}
		ev, ok := s.World.Event(12000, 41)
		if !ok {
			t.Fatal("native rabbit event missing")
		}
		branch := s.World.FindBranch(c.character, c.view, 12000, ev, 0, 0, 0, -1)
		if branch < 0 {
			t.Fatal("rabbit entry missing")
		}
		if err := s.startEvent(context.Background(), c, 41, ev, branch, false); err != nil {
			t.Fatal(err)
		}
		tradeDo(t, s, c, []byte{20, 9, 30})
		if c.event == nil || c.event.onMinigame == nil {
			t.Fatal("rabbit failed to launch minigame", wire.packets(t))
		}
		tradeDo(t, s, c, []byte{57, 1, result})
		p := wire.packets(t)
		want := uint16(20364)
		if result == 1 {
			want = 20363
		}
		found := false
		for _, packet := range p {
			if len(packet) == 18 && packet[6] == 1 && uint16(packet[15])|uint16(packet[16])<<8 == want {
				found = true
			}
		}
		if !found {
			t.Fatal("wrong rabbit outcome dialogue", p)
		}
		for i := 0; i < 100 && c.event != nil; i++ {
			tradeDo(t, s, c, []byte{20, 6})
		}
		if c.event != nil {
			t.Fatal("rabbit did not finish")
		}
		carrots, vouchers := 0, 0
		for _, item := range c.character.Bag {
			if item.ID == 32102 {
				carrots += int(item.Count)
			}
			if item.ID == 30002 {
				vouchers += int(item.Count)
			}
		}
		expected := 0
		if result == 1 {
			expected = 10
		}
		if carrots != expected || vouchers != 0 {
			t.Fatal("wrong native reward", carrots, vouchers)
		}
		bag := c.character.Bag
		tradeDo(t, s, c, []byte{57, 1, 1})
		tradeDo(t, s, c, []byte{20, 6})
		if c.character.Bag != bag {
			t.Fatal("rabbit replay rewarded")
		}
		c.resumeAt = time.Time{}
	}
}

func TestMinigameOutcomeCanLaunchNextGame(t *testing.T) {
	s, c, wire, ev := minigameFixture(t, 3, 0)
	stale := c.event.onMinigame
	ev.Branches[1].Operations = []assets.Operation{evOp(1, 9, 4, 0x2710, 0, 0)}
	ev.Branches = append(ev.Branches, assets.Branch{Index: 4, Condition: evCond(8, 4, 0, 0, 5, 1), Operations: []assets.Operation{evOp(1, 1, 1, 2, 0, 20)}})
	wire.Reset()
	tradeDo(t, s, c, []byte{57, 1, 1})
	if c.event == nil || c.event.onMinigame == nil || !contains(wire.packets(t), []byte{57, 1, 4, 0x10, 0x27, 1}) {
		t.Fatal("new game's callback lost")
	}
	active := c.event
	if err := stale(1); err != nil || c.event != active || c.character.Gold != 0 {
		t.Fatal("old callback replaced new game", err)
	}
	tradeDo(t, s, c, []byte{57, 1, 1})
	if c.character.Gold != 20 || c.event != nil {
		t.Fatal("second game outcome did not run")
	}
}

func TestMinigameFullBagCannotPartiallyReward(t *testing.T) {
	s, c, wire, _ := minigameFixture(t, 3, 0)
	next := c.character.Clone()
	for i := range next.Bag {
		next.Bag[i] = game.Item{ID: 32176, Count: 50}
	}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	bag := c.character.Bag
	wire.Reset()
	tradeDo(t, s, c, []byte{57, 1, 1})
	tradeDo(t, s, c, []byte{20, 6}) // the result's own acknowledgment
	wire.Reset()
	tradeDo(t, s, c, []byte{20, 6})
	if c.event != nil || c.character.Bag != bag || contains(wire.packets(t), []byte{20, 10}) {
		t.Fatal("full bag partially rewarded or retained interaction")
	}
}

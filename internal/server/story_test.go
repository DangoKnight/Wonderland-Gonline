package server

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/world"
)

func TestSpiderStoryArrivalRecovery(t *testing.T) {
	for _, deathSaved := range []bool{false, true} {
		t.Run(map[bool]string{false: "farewell", true: "cleanup"}[deathSaved], func(t *testing.T) {
			s, c, wire := eventFixture(t)
			ctx := context.Background()
			farewell := assets.Event{ClickID: 12, Branches: []assets.Branch{{Index: 1,
				Condition:  evCond(5, 13086, 1, 0, 5, 2),
				Operations: []assets.Operation{evOp(1, 2, 5, 0, 10001, 0), evOp(2, 5, 13086, 2, 0, 0)},
			}}}
			s.Assets.Maps[11077] = assets.Map{ID: 11077, Events: []assets.Event{farewell}}
			s.World = world.New(s.Assets)
			next := c.character.Clone()
			next.Map = 11077
			next.Quests[13086] = game.Quest{ID: 13086, State: game.InProgress, Step: 2}
			if deathSaved {
				next.Quests[13087] = game.Quest{ID: 13087, State: game.InProgress, Step: 1}
			}
			if err := s.commit(ctx, c, next); err != nil {
				t.Fatal(err)
			}
			ran, err := s.storyArrival(ctx, c)
			if err != nil {
				t.Fatal(err)
			}
			if !deathSaved {
				if !ran || c.event == nil || c.event.click != 13 || c.event.transition || c.event.ev.ClickID != 12 {
					t.Fatal("farewell was not resumed", ran, c.event)
				}
				if !contains(wire.packets(t), eventFrame(1, 3, 5, 1, 0, 10001, 1, 1)) {
					t.Fatal("missing farewell dialogue")
				}
				if err := s.worldCommand(ctx, c, []byte{20, 6}); err != nil {
					t.Fatal(err)
				}
			} else if ran || c.event != nil {
				t.Fatal("cleanup replayed the farewell")
			}
			chars, err := s.Store.Characters(ctx, c.account.ID)
			if err != nil || chars[0].Quests[13086].State != game.Completed {
				t.Fatal("completion was not persisted", err)
			}
			wire.Reset()
			if ran, err := s.storyArrival(ctx, c); ran || err != nil || len(wire.packets(t)) != 0 {
				t.Fatal("completed story replayed", ran, err)
			}
		})
	}
}

func TestStormAndBeachRescue(t *testing.T) {
	beachIntroDelay, beachTimeout = 10*time.Millisecond, time.Hour
	defer func() { beachIntroDelay, beachTimeout = 800*time.Millisecond, 25*time.Second }()
	s, players, wires := worldFixture(t)
	ctx := context.Background()
	storm := evOp(2, 8, 1, 0, 0, 0)
	storm.Data[7], storm.Data[8] = 0x00, 0x7b // dialog4 = 0x7B00
	captain := assets.Event{ClickID: 11, Branches: []assets.Branch{{Index: 1, Condition: evCond(0, 0, 0, 0, 0, 0), Operations: []assets.Operation{evOp(1, 2, 10, 0, 30100, 0), storm}}}}
	robinson := assets.Event{ClickID: 1, Branches: []assets.Branch{{Index: 1, Condition: evCond(5, beachQuest, 1, 0, 5, 1), Operations: []assets.Operation{evOp(1, 2, 1, 0, 20001, 0)}}}}
	s.Assets.Maps[10017] = assets.Map{ID: 10017, NPCs: []assets.MapNPC{{ClickID: 10, Flags: 1, X: 1050, Y: 1080, Events: []byte{11}}}, Events: []assets.Event{captain}}
	s.Assets.Maps[beachMap] = assets.Map{ID: beachMap, NPCs: []assets.MapNPC{{ClickID: 1, Flags: 1, X: 1000, Y: 2200}}, Events: []assets.Event{robinson}}
	s.World = world.New(s.Assets)
	c, wire := players[0], wires[0]
	do := func(p ...byte) [][]byte {
		t.Helper()
		if err := s.worldCommand(ctx, c, p); err != nil {
			t.Fatal(err)
		}
		return wire.packets(t)
	}
	do(12, 1)
	do(20, 1, 10, 0)
	if got := do(20, 6); len(got) < 2 || !bytes.Equal(got[0], []byte{186, 12, 1, 0, 0, 0, 0}) || !c.storm {
		t.Fatal("storm", got)
	}
	got := do(20, 6)
	if !contains(got, protocol.Builder{12}.U32(c.character.ID).U16(beachMap).U16(1038).U16(2235).U16(0).U8(0)) || c.storm || !c.beachPending {
		t.Fatal("storm did not end at the beach", got)
	}
	got = do(12, 1)
	if !contains(got, protocol.Builder{32, 2}.U32(c.character.ID).U8(9)) || !contains(got, []byte{22, 11, 6, 0, 0xff, 0xff}) || c.beach == nil {
		t.Fatal("rescue start", got)
	}
	if got = do(20, 6); len(got) != 0 {
		t.Fatal("acknowledgment before the wake-up animation", got)
	}
	for deadline := time.Now().Add(5 * time.Second); ; {
		s.worldMu.Lock()
		step := c.beach.step
		s.worldMu.Unlock()
		if step == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("wake-up animation did not start")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got = wire.packets(t); len(got) != 1 || !bytes.Equal(got[0], eventFrame(5, 0, 0, 1, 12008, 0, 1, 0)) {
		t.Fatal("wake-up", got)
	}
	for _, want := range [][]byte{{24, 1, 8, 47, 1}, {20, 10}, {24, 5, 97, 0, 1}, {22, 12, 1, 1, 0, 6}} {
		if got = do(20, 6); !bytes.Equal(got[0], want) {
			t.Fatal("rescue step", got)
		}
	}
	got = do(20, 6)
	// The player is released, then Robinson speaks because mark 12040 is now set.
	if len(got) < 4 || !bytes.Equal(got[0], []byte{20, 8}) || !bytes.Equal(got[3], eventFrame(1, 3, 1, 1, 0, 20001, 1, 1)) || c.beach != nil || c.emote != 0 {
		t.Fatal("rescue end", got)
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Map != beachMap || chars[0].Quests[beachQuest].Step != 1 {
		t.Fatal("beach state not persisted", err)
	}
}

func TestStarterShipSceneTransitions(t *testing.T) {
	for _, tc := range []struct {
		name        string
		from, entry uint16
		destination world.Destination
		rescue      bool
	}{
		{"deck to arcade cabin", 10017, 2, world.Destination{Map: 10027, X: 674, Y: 1067}, false},
		{"cabin to deck", 10027, 1, world.Destination{Map: 10017, X: 1814, Y: 904}, false},
		{"shipwreck chapter", 10017, 1, world.Destination{Map: 10035, X: 1038, Y: 2235}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, players, wires := worldFixture(t)
			c := players[0]
			c.character.Map = tc.from
			warpTarget := tc.destination.Map
			if tc.rescue {
				warpTarget = 10037
			} // Native chapter destination is normalized to rescue map 10035.
			s.Assets.Maps[tc.from] = assets.Map{ID: tc.from, Warps: []assets.Warp{{ClickID: tc.entry, MapID: warpTarget, X: uint32(tc.destination.X), Y: uint32(tc.destination.Y)}}}
			s.Assets.Maps[tc.destination.Map] = assets.Map{ID: tc.destination.Map}
			s.World = world.New(s.Assets)
			// Independent opcode bytes: player scene transition, with the warp ID.
			op := assets.Operation{Index: 1, Data: [21]byte{1, 3, 0, byte(tc.entry), 0}}
			ev := assets.Event{ClickID: 1, Branches: []assets.Branch{{Index: 1, Operations: []assets.Operation{op}}}}
			s.worldMu.Lock()
			_, err := s.execute(context.Background(), c, &eventSession{ev: &ev}, world.DecodeOp(op))
			s.worldMu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			if c.character.Map != tc.destination.Map || c.character.X != tc.destination.X || c.character.Y != tc.destination.Y || c.beachPending != tc.rescue {
				t.Fatalf("transition went to %d (%d,%d), rescue=%v", c.character.Map, c.character.X, c.character.Y, c.beachPending)
			}
			stored, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil || len(stored) != 1 || stored[0].Map != tc.destination.Map {
				t.Fatalf("transition not persisted: %v %v", stored, err)
			}
			if got := wires[0].packets(t); !contains(got, protocol.Builder{12}.U32(c.character.ID).U16(tc.destination.Map).U16(tc.destination.X).U16(tc.destination.Y).U16(0).U8(0)) {
				t.Fatal("missing destination packet")
			}
		})
	}
}

func TestNativeStarterShipCabinTransitions(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ from, to, x, y uint16 }{{10017, 10027, 674, 1067}, {10027, 10017, 1814, 904}} {
		t.Run(fmt.Sprintf("%d_to_%d", tc.from, tc.to), func(t *testing.T) {
			s, players, _ := worldFixture(t)
			s.Assets, s.World = catalog, world.New(catalog)
			c := players[0]
			c.character.Map = tc.from
			ev, ok := s.World.Event(tc.from, 1)
			if !ok {
				t.Fatal("native door event missing")
			}
			s.worldMu.Lock()
			err := s.startEvent(context.Background(), c, 0, ev, 0, true)
			s.worldMu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			if c.character.Map != tc.to || c.character.X != tc.x || c.character.Y != tc.y || c.beachPending {
				t.Fatalf("native door went to %d (%d,%d), rescue=%v", c.character.Map, c.character.X, c.character.Y, c.beachPending)
			}
		})
	}
}

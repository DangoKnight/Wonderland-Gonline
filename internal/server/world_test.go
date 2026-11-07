package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/config"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

func TestWorldSnapshotIncludesFightingPeerPresence(t *testing.T) {
	s, players, wires := worldFixture(t)
	peer, observer := players[0], players[1]
	if err := s.acknowledgeWorld(peer); err != nil {
		t.Fatal(err)
	}
	peer.battle = &battleRun{}
	if err := s.acknowledgeWorld(observer); err != nil {
		t.Fatal(err)
	}
	id := peer.character.ID
	want := []byte{11, 4, 2, byte(id), byte(id >> 8), byte(id >> 16), byte(id >> 24), 0, 0, 1}
	packets := wires[1].packets(t)
	if !contains(packets, want) {
		t.Fatal("entrant missing fighting peer's native presence")
	}
	appearance, presence := -1, -1
	for i, p := range packets {
		if len(p) > 0 && p[0] == 4 {
			appearance = i
		}
		if bytes.Equal(p, want) {
			presence = i
		}
	}
	if appearance < 0 || presence <= appearance {
		t.Fatal("presence must follow peer appearance")
	}
	peer.battle = nil
}

func TestEventWarpRestoresUIOnMapAcknowledgment(t *testing.T) {
	for _, tc := range []struct {
		name            string
		event, newEvent bool
		unlock          bool
	}{
		{"event", true, false, true}, {"regular", false, false, false}, {"new_interaction", true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, players, wires := worldFixture(t)
			c, wire := players[0], wires[0]
			s.Assets.Maps[20000] = assets.Map{ID: 20000}
			s.World = world.New(s.Assets)
			s.world[c.info.ID] = c
			c.ready = true
			if tc.event {
				c.event = &eventSession{mapID: c.character.Map}
			}
			if err := s.commandTeleport(context.Background(), c, world.Destination{Map: 20000, X: 500, Y: 600}); err != nil {
				t.Fatal(err)
			}
			if c.restoreUIAfterMapLoad != tc.event || c.ready || c.event != nil {
				t.Fatal("event warp ownership not captured")
			}
			if contains(wire.packets(t), []byte{6, 2, 0}) {
				t.Fatal("UI released before map acknowledgment")
			}
			var active *eventSession
			if tc.newEvent {
				active = &eventSession{mapID: 20000}
				c.event = active
			}
			if err := s.worldCommand(context.Background(), c, []byte{12, 1}); err != nil {
				t.Fatal(err)
			}
			packets := wire.packets(t)
			if contains(packets, []byte{6, 2, 0}) != tc.unlock || c.restoreUIAfterMapLoad {
				t.Fatal("wrong map-load UI release", packets)
			}
			if tc.unlock {
				found := false
				for i := 0; i+2 < len(packets); i++ {
					if bytes.Equal(packets[i], []byte{5, 4}) && bytes.Equal(packets[i+1], []byte{6, 2, 0}) && bytes.Equal(packets[i+2], []byte{20, 8}) {
						found = true
					}
				}
				if !found {
					t.Fatal("UI release not ordered after native scene-load completion", packets)
				}
			}
			if tc.newEvent && c.event != active {
				t.Fatal("map acknowledgment cancelled the new interaction")
			}
			if err := s.worldCommand(context.Background(), c, []byte{12, 1}); err != nil {
				t.Fatal(err)
			}
			if wire.Len() != 0 {
				t.Fatal("duplicate map acknowledgment repeated release")
			}
		})
	}
}

func TestAreaEntryScriptsPrecedeCollidingWarpIDs(t *testing.T) {
	for _, entry := range []byte{1, 2} {
		t.Run(fmt.Sprint(entry), func(t *testing.T) {
			s, players, _ := worldFixture(t)
			c := players[0]
			s.Assets.Maps[65000] = assets.Map{ID: 65000,
				Entries: []assets.AreaEntry{areaEntry(1, 3, 53, 54, 53, 54, 7), areaEntry(2, 3, 53, 54, 53, 54, 8)},
				Warps:   []assets.Warp{{ClickID: 1, MapID: 65001, X: 1, Y: 1}, {ClickID: 2, MapID: 65001, X: 1, Y: 1}},
				Events: []assets.Event{
					{ClickID: 7, Branches: []assets.Branch{{Index: 1, Operations: []assets.Operation{evOp(1, 7, 65002, 100, 200, 0)}}}},
					{ClickID: 8, Branches: []assets.Branch{{Index: 1, Operations: []assets.Operation{evOp(1, 7, 65003, 300, 400, 0)}}}},
				},
			}
			for _, id := range []uint16{65001, 65002, 65003} {
				s.Assets.Maps[id] = assets.Map{ID: id}
			}
			s.World = world.New(s.Assets)
			c.character.Map = 65000
			c.ready = true
			s.world[c.info.ID] = c
			if err := s.commit(context.Background(), c, c.character.Clone()); err != nil {
				t.Fatal(err)
			}
			if err := s.worldCommand(context.Background(), c, []byte{20, 8, entry, 0}); err != nil {
				t.Fatal(err)
			}
			want := uint16(65001) + uint16(entry)
			if c.character.Map != want {
				t.Fatal("area entry fell through to the colliding warp table", c.character.Map, want)
			}
		})
	}
}

func TestLauraExitRewardAtomicReplayAndCapacity(t *testing.T) {
	s, c, wire, _ := mallFixture(t)
	ctx := context.Background()
	s.Assets.MapTransitions = []assets.MapTransition{{FromMap: 10001, Entry: 1, Destination: assets.TravelDestination{Map: 10000, X: 1892, Y: 1655}, Quest: 10035, Talk: 30126, Items: []assets.BoxedItem{{ItemID: 36002, Quantity: 1}, {ItemID: 34058, Quantity: 1}}, SkipIfOwned: []uint16{36002}}}
	s.Assets.Items[36002] = game.ItemDefinition{ID: 36002}
	s.Assets.Items[34058] = game.ItemDefinition{ID: 34058}
	s.Assets.Maps[10000] = assets.Map{ID: 10000}
	s.Assets.Maps[10001] = assets.Map{ID: 10001}
	s.World = world.New(s.Assets)
	c.character.Map = 10001
	c.character.Bag = game.Inventory{}
	for i := range c.character.Bag {
		c.character.Bag[i] = game.Item{ID: 32176, Count: 50}
	}
	c.character.Bag[0] = game.Item{}
	if err := s.commit(ctx, c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	if err := s.teleport(ctx, c, world.Destination{Map: 10000, X: 1892, Y: 1655}, 1); err != nil || c.character.Map != 10001 || c.character.Quests[10035].ID != 0 || !c.character.Bag[0].Empty() {
		t.Fatal("partial/full-bag gift", err)
	}
	c.character.Bag[1] = game.Item{}
	if err := s.commit(ctx, c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	wire.Reset()
	if err := s.teleport(ctx, c, world.Destination{Map: 10000, X: 1892, Y: 1655}, 1); err != nil || c.character.Quests[10035].State != game.Completed || c.character.Bag[0].ID != 36002 || c.character.Bag[1].ID != 34058 {
		t.Fatal(c.character.Bag, err)
	}
	if !contains(wire.packets(t), []byte{20, 1, 0, 0, 0, 1, 1, 3, 0, 0, 1, 0, 0, 0, 0, 0xae, 0x75, 0}) {
		t.Fatal("Laura dialogue")
	}
	saved, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || saved[0].Map != 10000 || saved[0].Quests[10035].State != game.Completed {
		t.Fatal("not durable", err)
	}
	c.character.Map = 10001
	c.character.Bag[0] = game.Item{}
	c.character.Bag[1] = game.Item{}
	if err = s.commit(ctx, c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	if err = s.teleport(ctx, c, world.Destination{Map: 10000}, 1); err != nil || !c.character.Bag[0].Empty() {
		t.Fatal("replayed gift", err)
	}
}

func TestStarterScenesHidePlayerSnapshotsAndBroadcasts(t *testing.T) {
	for _, tc := range []struct {
		name         string
		mapID, scene uint16
		private      bool
	}{
		{"ship", 10017, 10001, true},
		{"cabin", 10027, 10002, true},
		{"island", 10035, 10003, true},
		{"public", 11016, 11016, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, p, w := worldFixture(t)
			for _, c := range p[:2] {
				c.character.Map = tc.mapID
			}
			s.Assets.Maps[tc.mapID] = assets.Map{ID: tc.mapID, Scene: tc.scene}
			s.World = world.New(s.Assets)
			ctx := context.Background()
			if err := s.acknowledgeWorld(p[0]); err != nil {
				t.Fatal(err)
			}
			w[0].Reset()
			// A tent sign must not bypass the normal peer snapshot filter.
			p[0].openTent = &openTent{Map: tc.mapID, X: 1000, Y: 1000, Slot: 1}
			if err := s.acknowledgeWorld(p[1]); err != nil {
				t.Fatal(err)
			}
			arrival := w[0].packets(t)
			snapshot := w[1].packets(t)
			if tc.private {
				if len(arrival) != 0 || countPrefix(snapshot, 4) != 0 || hasPacket(snapshot, tentSign(p[0])) {
					t.Fatal("private entry exposed a peer", arrival, snapshot)
				}
				if s.mapPlayer(p[0], p[1].character.ID) != nil {
					t.Fatal("hidden player discoverable by local commands")
				}
			} else if countPrefix(arrival, 4) != 1 || countPrefix(snapshot, 4) != 1 || !hasPacket(snapshot, tentSign(p[0])) {
				t.Fatal("public entry lost peers", arrival, snapshot)
			}
			if tc.private {
				if err := s.enterTent(ctx, p[1], p[0].character.ID); err != nil {
					t.Fatal(err)
				}
				if p[1].tentOwner != 0 || p[1].character.Map != tc.mapID {
					t.Fatal("entered hidden player's tent")
				}
				w[1].Reset()
			}
			for _, packet := range [][]byte{
				protocol.Builder{6, 1}.U32(p[0].character.ID).U8(2).U16(1200).U16(1300),
				protocol.Builder{32, 1}.U32(p[0].character.ID).U8(1),
				protocol.Builder{11, 4}.U32(p[0].character.ID).U8(1),
				protocol.Builder{15, 4}.U32(p[0].character.ID).U32(12178),
			} {
				s.broadcastWorld(p[0], packet)
				got := w[1].packets(t)
				if tc.private && len(got) != 0 || !tc.private && (len(got) != 1 || !bytes.Equal(got[0], packet)) {
					t.Fatal("broadcast visibility", got)
				}
			}
			p[0].party = &party{members: []*Session{p[0], p[1]}}
			p[1].party = p[0].party
			s.partyArrival(p[0])
			if tc.private && countPrefix(w[1].packets(t), 13, 5) != 0 {
				t.Fatal("party formation exposed a peer")
			}
			if tc.private && len(s.battleTeam(p[0])) != 1 {
				t.Fatal("hidden teammate entered personal battle")
			}
			w[0].Reset()
			w[1].Reset()
			s.sendMap(tc.mapID, []byte{13, 5, 1, 0, 0, 0, 2, 0, 0, 0}, nil)
			if tc.private && (w[0].Len() != 0 || w[1].Len() != 0) {
				t.Fatal("map-wide formation exposed peers")
			}
			w[0].Reset()
			w[1].Reset()
			if err := s.closePlayerTent(ctx, p[0]); err != nil {
				t.Fatal(err)
			}
			if tc.private && w[1].Len() != 0 {
				t.Fatal("tent close exposed its owner")
			}
			w[0].Reset()
			w[1].Reset()
			s.leaveWorld(p[0])
			if tc.private && countPrefix(w[1].packets(t), 12) != 0 {
				t.Fatal("logout exposed a peer")
			}
		})
	}
}

func TestStarterPlayerPolicyPreservesSharedTent(t *testing.T) {
	s := &Server{World: world.New(&assets.Catalog{Story: assets.DefaultStoryDefinitions(), Maps: map[uint16]assets.Map{10017: {ID: 10017, Scene: 10001}}})}
	a := &Session{character: &game.Character{ID: 1, Map: 10017}, tentOwner: 1}
	b := &Session{character: &game.Character{ID: 2, Map: 10017}, tentOwner: 1}
	if !s.samePlayerScene(a, b) {
		t.Fatal("shared tent occupants hidden")
	}
	b.tentOwner = 2
	if s.samePlayerScene(a, b) {
		t.Fatal("different tent owners share visibility")
	}
	a.tentOwner = 0
	b.tentOwner = 0
	if s.samePlayerScene(a, b) || !s.samePlayerScene(a, a) {
		t.Fatal("starter visibility does not isolate other players")
	}
}

func TestStarterVisibilityChangesAtMapTransitions(t *testing.T) {
	s, players, wires := worldFixture(t)
	a, b := players[0], players[1]
	s.Assets.Maps[10017] = assets.Map{ID: 10017, Scene: 10001}
	s.Assets.Maps[20000] = assets.Map{ID: 20000, Scene: 20000}
	s.World = world.New(s.Assets)
	ctx := context.Background()
	for _, c := range []*Session{a, b} {
		if err := s.acknowledgeWorld(c); err != nil {
			t.Fatal(err)
		}
	}
	for _, w := range wires {
		w.Reset()
	}
	warp := func(c *Session, mapID uint16) {
		t.Helper()
		s.worldMu.Lock()
		err := s.teleport(ctx, c, world.Destination{Map: mapID, X: 1042, Y: 1075}, 0)
		s.worldMu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		if err = s.acknowledgeWorld(c); err != nil {
			t.Fatal(err)
		}
	}
	warp(a, 20000)
	for _, w := range wires {
		w.Reset()
	}
	warp(b, 20000)
	if countPrefix(wires[0].packets(t), 4) != 1 || countPrefix(wires[1].packets(t), 4) != 1 {
		t.Fatal("public map did not restore peer appearances")
	}
	warp(a, 10017)
	departure := wires[1].packets(t)
	if countPrefix(departure, 12) != 1 || countPrefix(departure, 4) != 0 {
		t.Fatal("public departure did not remove player", departure)
	}
	wires[0].Reset()
	wires[1].Reset()
	warp(b, 10017)
	if countPrefix(wires[0].packets(t), 4) != 0 || countPrefix(wires[1].packets(t), 4) != 0 {
		t.Fatal("return to private map exposed a player")
	}
}

func TestClinicRestHealsAllAccompanyingPetsOnly(t *testing.T) {
	s, players, wires := worldFixture(t)
	c := players[0]
	ctx := context.Background()
	c.ready = true
	c.character.Pets = []game.Pet{{ID: 17100, Level: 1, HP: 1, SP: 0, Base: game.Attributes{Constitution: 5, Wisdom: 5}}, {ID: 17101, Level: 1, HP: 1, SP: 0, Base: game.Attributes{Constitution: 5, Wisdom: 5}}}
	c.character.HotelPets = []game.Pet{{ID: 17102, Level: 1, HP: 1}}
	c.character.ReservePets = []game.Pet{{ID: 17103, Level: 1, HP: 1}}
	for _, pet := range c.character.Pets {
		c.pets.register(pet.ID)
	}
	if err := s.commit(ctx, c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	if err := s.openService(c, world.ServiceClinic); err != nil {
		t.Fatal(err)
	}
	if !contains(wires[0].packets(t), []byte{31, 2, 0, 0, 0, 0}) {
		t.Fatal("injured pet did not produce free rest offer")
	}
	wires[0].Reset()
	if err := s.worldCommand(ctx, c, []byte{31, 1}); err != nil {
		t.Fatal(err)
	}
	packets := wires[0].packets(t)
	for _, pet := range c.character.Pets {
		full := pet.Combat(s.Assets.Items)
		if pet.HP != full.MaxHP || pet.SP != full.MaxSP {
			t.Fatal("unhealed pet", pet)
		}
		if !contains(packets, game.PetStat(c.pets.slot(pet.ID), 25, int64(full.MaxHP))) {
			t.Fatal("pet HP not replayed")
		}
	}
	if c.character.HotelPets[0].HP != 1 || c.character.ReservePets[0].HP != 1 {
		t.Fatal("off-roster pets healed")
	}
	saved, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || saved[0].Pets[1].HP != c.character.Pets[1].HP {
		t.Fatal("clinic persistence", err)
	}
	wires[0].Reset()
	if err = s.worldCommand(ctx, c, []byte{31, 1}); err != nil || wires[0].Len() != 0 {
		t.Fatal("rest confirmation replay")
	}
}

func TestGenericNPCGreetingCallbackAndPrecedence(t *testing.T) {
	s, players, wires := worldFixture(t)
	c := players[0]
	ctx := context.Background()
	c.ready = true
	s.Assets.Maps[c.character.Map] = assets.Map{ID: c.character.Map, NPCs: []assets.MapNPC{{ClickID: 5, Template: 14000, Name: "Villager", Flags: 1, X: uint32(c.character.X), Y: uint32(c.character.Y)}}}
	s.Assets.Talks.ByID = map[uint32]string{0x21284c: "Welcome to Kelan Village."}
	s.World = world.New(s.Assets)
	if err := s.worldCommand(ctx, c, []byte{20, 1, 5, 0}); err != nil {
		t.Fatal(err)
	}
	want := []byte{20, 1, 0, 0, 0, 1, 1, 3, 5, 0, 1, 0, 0, 0, 0, 0x4c, 0x28, 0x21}
	if c.event == nil || !contains(wires[0].packets(t), want) {
		t.Fatal("missing native fallback greeting")
	}
	wires[0].Reset()
	if err := s.worldCommand(ctx, c, []byte{20, 6}); err != nil || c.event != nil || !contains(wires[0].packets(t), []byte{20, 8}) {
		t.Fatal("greeting callback did not release", err)
	}
	// Hidden actors cannot reach the fallback or start an interaction.
	c.view.Hidden[5] = true
	c.resumeAt = time.Time{}
	wires[0].Reset()
	if err := s.worldCommand(ctx, c, []byte{20, 1, 5, 0}); err != nil || c.event != nil || contains(wires[0].packets(t), want) {
		t.Fatal("hidden actor greeted", err)
	}
}

func TestScriptedSharedPropReplayResetAndQuestIsolation(t *testing.T) {
	s, players, wires := worldFixture(t)
	ctx := context.Background()
	c := players[0]
	mapID := c.character.Map
	s.Assets.Maps[mapID] = assets.Map{ID: mapID, NPCs: []assets.MapNPC{{ClickID: 9, Template: 12001, Name: "Scripted barrel", Flags: 1, X: uint32(c.character.X), Y: uint32(c.character.Y), Events: []byte{1}}}}
	s.World = world.New(s.Assets)
	for i, p := range players {
		p.ready = true
		s.world[p.info.ID] = p
		wires[i].Reset()
	}
	ev := &assets.Event{ClickID: 1, Branches: []assets.Branch{{Index: 1}}}
	es := &eventSession{mapID: mapID, click: 9, ev: ev}
	op := world.DecodeOp(evOp(1, 2, 9, 5, 1, 1))
	handled, err := s.actorAction(ctx, c, es, op)
	if err != nil || !handled {
		t.Fatal("shared script", err)
	}
	want := []byte{22, 1, 9, 0, 1}
	for _, i := range []int{0, 1} {
		if !contains(wires[i].packets(t), want) {
			t.Fatal("map frame not shared", i)
		}
	}
	if wires[2].Len() != 0 {
		t.Fatal("foreign scene received frame")
	}
	players[1].view = world.NewView()
	wires[1].Reset()
	if err = s.syncMapProps(ctx, players[1], time.Now()); err != nil || !contains(wires[1].packets(t), want) {
		t.Fatal("scripted frame lost at entry", err)
	}
	s.respawnMapProps(ctx, time.Now().Add(61*time.Second))
	if !contains(wires[1].packets(t), []byte{22, 1, 9, 0, 0}) {
		t.Fatal("missing 60-second reset")
	}
	wires[0].Reset()
	wires[1].Reset()
	ev.Branches[0].Condition = evCond(5, 900, 1, 0, 0, 0)
	if _, err = s.actorAction(ctx, c, es, op); err != nil {
		t.Fatal(err)
	}
	rows, err := s.Store.ActiveMapProps(ctx, mapID, time.Now())
	if err != nil || len(rows) != 0 || wires[1].Len() != 0 || !contains(wires[0].packets(t), want) {
		t.Fatal("quest prop leaked shared state", rows, err)
	}
}

func TestStarterBulkGrantOwnership(t *testing.T) {
	s, players, wires := worldFixture(t)
	ctx := context.Background()
	c := players[0]
	c.character.Bag = game.Inventory{}
	if err := s.commit(ctx, c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	next := c.character.Clone()
	// One recipient has room, one is full, and one is busy.
	for i, p := range players {
		p.ready = true
		s.world[p.info.ID] = p
		wires[i].Reset()
	}
	c.character = &next
	baseline := next.Clone()
	c.autosaveBaseline = &baseline
	c.character.X += 10
	other := players[1]
	s.Assets.StarterItems = []game.StarterGrant{{ID: 32177, Count: 1}, {ID: 32176, Count: 1}}
	s.Assets.Items[32177] = game.ItemDefinition{ID: 32177, Type: 23}
	for i := range other.character.Bag {
		other.character.Bag[i] = game.Item{ID: 32176, Count: 50}
	}
	if err := s.commit(ctx, other, other.character.Clone()); err != nil {
		t.Fatal(err)
	}
	players[2].event = &eventSession{}
	result, err := s.AdminGiveStarterPacks(ctx)
	if err != nil || len(result) != 3 || !result[0].Delivered || result[1].Delivered || result[2].Delivered {
		t.Fatal("bulk outcomes", result, err)
	}
	if c.character.X != baseline.X+10 || bagCount(c.character.Bag, 32177) != 1 || bagCount(other.character.Bag, 32177) != 0 {
		t.Fatal("partial grant or walking discarded")
	}
	if wires[1].Len() != 0 || wires[2].Len() != 0 {
		t.Fatal("failed recipients received success")
	}
	audit, err := s.Store.Audit(ctx)
	if err != nil || len(audit) == 0 || audit[len(audit)-1].Action != "starter pack grant" {
		t.Fatal("grant audit missing", audit, err)
	}
}

func TestBattleSubmissionACKBroadcastAndOwnerPetEntry(t *testing.T) {
	s, players, wires := combatSessions(t)
	a, d := players[0], players[1]
	a.character.Pets = []game.Pet{{ID: 17100, Name: "Pet", Level: 3, HP: 100, SP: 100, Base: game.Attributes{Constitution: 5, Wisdom: 5}}}
	a.character.Pets[0].Battle = true
	a.pets.register(17100)
	if err := s.worldCommand(context.Background(), a, pkRequest(d.character.ID)); err != nil {
		t.Fatal(err)
	}
	run := a.battle
	if run == nil {
		t.Fatal("battle missing")
	}
	own := a.character.Pets[0].ProgressionPackets(a.pets.slot(17100), s.Assets.Items)
	entry := wires[0].packets(t)
	for i, packet := range entry {
		if len(packet) == 2 && packet[0] == 20 && packet[1] == 12 && i < len(own) {
			t.Fatal("battle UI preceded owner pet progression")
		}
	}
	for _, p := range own {
		if !contains(entry, p) {
			t.Fatal("owner pet not refreshed before battle", p)
		}
	}
	for _, w := range wires {
		w.Reset()
	}
	// Defender stays pending so the ACK can be checked before animation execution.
	if err := s.worldCommand(context.Background(), a, []byte{50, 4, 4, 2, 1, 2}); err != nil {
		t.Fatal(err)
	}
	ack := []byte{53, 5, 4, 2}
	if !contains(wires[0].packets(t), ack) || !contains(wires[1].packets(t), ack) || wires[2].Len() != 0 {
		t.Fatal("battle ACK scope")
	}
	for _, w := range wires {
		w.Reset()
	}
	if err := s.worldCommand(context.Background(), a, []byte{50, 4, 4, 2, 1, 2}); err != nil {
		t.Fatal(err)
	}
	if contains(wires[1].packets(t), ack) {
		t.Fatal("duplicate action ACK")
	}
	s.worldMu.Lock()
	s.abandonBattle(a)
	s.worldMu.Unlock()
	settle(t, s, d)
}

func TestClinicFailedSaveRetainsOfferAndPetState(t *testing.T) {
	s, players, wires := worldFixture(t)
	c := players[0]
	c.ready = true
	c.character.HP = 1
	c.character.Pets = []game.Pet{{ID: 17100, Level: 1, HP: 1, SP: 0, Base: game.Attributes{Constitution: 5, Wisdom: 5}}}
	if err := s.openService(c, world.ServiceClinic); err != nil {
		t.Fatal(err)
	}
	before := c.character.Clone()
	wires[0].Reset()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.confirmRest(ctx, c); err == nil {
		t.Fatal("cancelled save succeeded")
	}
	if c.restMap != c.character.Map || c.character.HP != before.HP || c.character.Pets[0].HP != before.Pets[0].HP || wires[0].Len() != 0 {
		t.Fatal("failed rest consumed offer or changed state")
	}
	if err := s.confirmRest(context.Background(), c); err != nil || c.restMap != 0 {
		t.Fatal("rest retry failed", err)
	}
}

func TestWorldEntryRejectsInvalidCreationAllocation(t *testing.T) {
	for _, stats := range [][]byte{{0, 0, 0, 0, 0}, {1, 1, 1, 1, 0}, {5, 5, 5, 5, 5}, {255, 255, 255, 255, 255}} {
		t.Run(string(rune(stats[0]+65)), func(t *testing.T) {
			db, err := store.Open(filepath.Join(t.TempDir(), "db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			account, err := db.Register(context.Background(), "tester", "password", "")
			if err != nil {
				t.Fatal(err)
			}
			s := New(config.Default(), db, creationCatalog(), slog.New(slog.NewTextHandler(io.Discard, nil)))
			wire := &captureConn{}
			c := &Session{account: account, slot: 1, conn: wire, info: SessionInfo{ID: 1}}
			if _, err := s.reserveName(context.Background(), c, "BudgetHero"); err != nil {
				t.Fatal(err)
			}
			p := createPayload()
			copy(p[len(p)-5:], stats)
			if err := s.dispatch(context.Background(), c, p); err != nil {
				t.Fatal(err)
			}
			got := wire.packets(t)
			if len(got) != 1 || !bytes.Equal(got[0], []byte{0, 30}) {
				t.Fatal(got)
			}
			chars, err := db.Characters(context.Background(), account.ID)
			if err != nil || len(chars) != 0 || c.character != nil {
				t.Fatal("rejection persisted or entered world", err)
			}
			// Correcting the allocation on the same connection retains the reserved name.
			if err := s.dispatch(context.Background(), c, createPayload()); err != nil || c.character == nil {
				t.Fatal("valid retry failed", err)
			}
		})
	}
}

func TestWorldEntryStoryConstellations(t *testing.T) {
	s, _, _ := worldFixture(t)
	c := &game.Character{Quests: map[uint32]game.Quest{}}
	if got := s.storyConstellations(c); !bytes.Equal(got, []byte{15, 19, 0}) {
		t.Fatal(got)
	}
	for _, id := range []uint32{13087, 13173, 13203, 13151} {
		c.Quests[id] = game.Quest{State: game.InProgress, Step: 1}
	}
	if got := s.storyConstellations(c); !bytes.Equal(got, []byte{15, 19, 4, 1, 6, 8, 20}) {
		t.Fatal(got)
	}
	c.Quests[13087] = game.Quest{State: game.Completed, Step: 1}
	c.Quests[13173] = game.Quest{State: game.InProgress, Step: 0}
	if got := s.storyConstellations(c); !bytes.Equal(got, []byte{15, 19, 2, 8, 20}) {
		t.Fatal(got)
	}
}

func TestWorldEntrySnapshotRestoresOptionalState(t *testing.T) {
	s, players, _ := worldFixture(t)
	c := players[0].character
	s.Assets.NPCs = map[uint16]assets.NPC{42: {BookIndex: 1}, 43: {BookIndex: 5500}, 44: {BookIndex: 5501}, 45: {BookIndex: 0}}
	c.DiscoveredMonsters = []uint16{43, 42, 42, 44, 45, 60000}
	c.Quests[13087] = game.Quest{State: game.InProgress, Step: 1}
	packets, err := s.worldEntryPackets(*c, world.NewView(), newPetRoster())
	if err != nil {
		t.Fatal(err)
	}
	var discoveries [][]byte
	star, ready := -1, -1
	for i, p := range packets {
		if p[0] == 53 {
			discoveries = append(discoveries, p)
		}
		if bytes.Equal(p, []byte{15, 19, 1, 1}) {
			star = i
		}
		if bytes.Equal(p, []byte{1, 11}) {
			ready = i
		}
	}
	if len(discoveries) != 2 || !bytes.Equal(discoveries[0], []byte{53, 9, 42, 0, 0, 0}) || !bytes.Equal(discoveries[1], []byte{53, 9, 43, 0, 0, 0}) {
		t.Fatal(discoveries)
	}
	if star < 0 || ready < star {
		t.Fatal("story state absent or after ready marker")
	}
	// aLogin refuses Trade Items until 25/44 2 sets its online-time flag.
	if !contains(packets, []byte{25, 44, 2}) {
		t.Fatal("trade online-time flag missing")
	}
	if c.DiscoveredMonsters[0] != 43 {
		t.Fatal("snapshot mutated durable discovery order")
	}
	clone := c.Clone()
	clone.DiscoveredMonsters[0] = 99
	if c.DiscoveredMonsters[0] != 43 {
		t.Fatal("discovery state aliases clone")
	}
}

func TestWorldEntryMonsterDiscoveryVictoryPersistence(t *testing.T) {
	s, c, wire := battleFixture(t, 1)
	s.Assets.NPCs[10500] = assets.NPC{ID: 10500, BookIndex: 2}
	wire.Reset()
	if err := s.worldCommand(context.Background(), c, []byte{50, 1, 4, 2, 2, 2}); err != nil {
		t.Fatal(err)
	}
	settle(t, s, c)
	if !contains(wire.packets(t), []byte{53, 9, 4, 41, 0, 0}) {
		t.Fatal("missing committed monster discovery")
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || len(saved) != 1 || len(saved[0].DiscoveredMonsters) != 1 || saved[0].DiscoveredMonsters[0] != 10500 {
		t.Fatal("discovery not persisted", err)
	}
	run := &battleRun{b: &battle.Battle{Defenders: []*battle.Fighter{{Template: 10500}, {Template: 10500, Captured: true}, {Template: 70000}}}}
	if got := s.discoverBattleMonsters(c.character, run); len(got) != 0 {
		t.Fatal("repeat discovery", got)
	}
}

func TestWorldEntrySceneReadyGoldenAndWelcomeOnce(t *testing.T) {
	for _, first := range [][]byte{{89, 0}, {89, 0, 0x11, 0x27, 0, 0}, {89, 0, 0xff, 0xff, 0xff, 0xff}, {92, 1}, {92, 1, 0}, {92, 1, 1}} {
		s, players, wires := worldFixture(t)
		c := players[0]
		wire := wires[0]
		s.motd = "Welcome"
		// These sync requests are valid while loading, but cannot publish presence.
		if err := s.dispatch(context.Background(), c, first); err != nil {
			t.Fatal(err)
		}
		got := wire.packets(t)
		welcome := []byte{23, 57, 0, 7, 'W', 'e', 'l', 'c', 'o', 'm', 'e'}
		if !contains(got, welcome) {
			t.Fatal(got)
		}
		if first[0] == 89 && !contains(got, []byte{90, 1, 0, 1, 1, 3, 2, 3}) {
			t.Fatal(got)
		}
		if c.ready || len(s.world) != 0 || c.character.ID != 10001 || c.account.ID != 1 {
			t.Fatal("scene sync published map readiness")
		}
		for _, p := range [][]byte{{89, 0}, {89, 0, 0x11, 0x27, 0, 0}, {92, 1}, {92, 1, 0}, {92, 1, 1}} {
			if err := s.dispatch(context.Background(), c, p); err != nil {
				t.Fatal(err)
			}
		}
		if contains(wire.packets(t), welcome) {
			t.Fatal("welcome repeated")
		}
		char := c.character.Clone()
		view, pets := world.NewView(), newPetRoster()
		packets, err := s.worldEntryPackets(char, view, pets)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.enterWorld(c, char, view, pets, packets); err != nil {
			t.Fatal(err)
		}
		wire.Reset()
		if err := s.dispatch(context.Background(), c, []byte{92, 1}); err != nil {
			t.Fatal(err)
		}
		if !contains(wire.packets(t), welcome) {
			t.Fatal("new character login did not reset welcome")
		}
		for _, p := range [][]byte{{89}, {92}, {89, 0, 1}, {89, 0, 1, 2}, {89, 0, 1, 2, 3}, {89, 0, 1, 2, 3, 4, 5}, {92, 1, 2}, {92, 1, 0xff}, {92, 1, 0, 0}, {92, 1, 1, 2, 3, 4}} {
			if err := s.dispatch(context.Background(), c, p); !errors.Is(err, protocol.ErrMalformed) {
				t.Fatal(p, err)
			}
		}
		for _, p := range [][]byte{{89, 1}, {92, 0}} {
			if err := s.dispatch(context.Background(), c, p); !errors.Is(err, ErrUnsupported) {
				t.Fatal(p, err)
			}
		}
	}
}

func TestWorldEntryWelcomeByteLimit(t *testing.T) {
	s, _, _ := worldFixture(t)
	settings := s.RuntimeSettings()
	settings.MOTD = strings.Repeat("x", 255)
	if err := s.UpdateRuntimeSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	settings.MOTD = strings.Repeat("x", 256)
	if err := s.UpdateRuntimeSettings(context.Background(), settings); err == nil {
		t.Fatal("accepted unencodable welcome")
	}
	if len(s.RuntimeSettings().MOTD) != 255 {
		t.Fatal("invalid settings published")
	}
}

func TestWorldEntryKeepsAuthenticatedSocketOnBothServices(t *testing.T) {
	for _, service := range []string{"login", "world"} {
		t.Run(service, func(t *testing.T) {
			db, err := store.Open(filepath.Join(t.TempDir(), "db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			_, err = db.Register(context.Background(), "tester", "password", "")
			if err != nil {
				t.Fatal(err)
			}
			s := New(config.Default(), db, creationCatalog(), slog.New(slog.NewTextHandler(io.Discard, nil)))
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() { defer close(done); s.accept(ctx, service, listener) }()
			client, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
			if err != nil {
				cancel()
				listener.Close()
				<-done
				t.Fatal(err)
			}
			defer func() { cancel(); client.Close(); listener.Close(); <-done; s.wg.Wait() }()
			client.SetDeadline(time.Now().Add(10 * time.Second))
			send := func(p []byte) {
				t.Helper()
				if err := protocol.Write(client, p); err != nil {
					t.Fatal(err)
				}
			}
			read := func() []byte {
				t.Helper()
				p, err := protocol.Read(client)
				if err != nil {
					t.Fatal(err)
				}
				return p
			}
			send(loginPayload("tester"))
			for i := 0; i < 3; i++ {
				read()
			}
			send([]byte{63, 2, 1})
			if p := read(); !bytes.Equal(p, []byte{1, 3, 0}) {
				t.Fatal(p)
			}
			send([]byte{9, 2, 'S', 'o', 'c', 'k', 'e', 't', 'H', 'e', 'r', 'o'})
			if p := read(); !bytes.Equal(p, []byte{9, 3, 0}) {
				t.Fatal(p)
			}
			send(createPayload())
			for count := 0; ; count++ {
				if count > 100 {
					t.Fatal("entry snapshot did not finish")
				}
				p := read()
				if p[0] == 35 && p[1] == 12 {
					break
				}
			}
			// Native aLogin sends this six-byte sync while the map is loading.
			// Its acknowledgment must arrive without closing the authenticated stream.
			send([]byte{89, 0, 0x11, 0x27, 0, 0})
			if p := read(); !bytes.Equal(p, []byte{90, 1, 0, 1, 1, 3, 2, 3}) {
				t.Fatal(p)
			}
			send([]byte{92, 1, 0})
			send([]byte{12, 1})
			for {
				if bytes.Equal(read(), []byte{5, 4}) {
					break
				}
			}
			if p := read(); !bytes.Equal(p, []byte{56, 30, 0}) {
				t.Fatal(p)
			}
			// A world action on the original stream proves that no reconnect/ticket is required.
			send([]byte{89, 0})
			if p := read(); !bytes.Equal(p, []byte{90, 1, 0, 1, 1, 3, 2, 3}) {
				t.Fatal(p)
			}
		})
	}
}

func TestWorldEntryFailedDiscoverySavePublishesNothing(t *testing.T) {
	s, c, wire := battleFixture(t, 1000)
	s.Assets.NPCs[10500] = assets.NPC{ID: 10500, BookIndex: 1}
	wire.Reset()
	s.Store.Close()
	s.endBattle(c.battle, battle.Victory)
	if len(c.character.DiscoveredMonsters) != 0 || wire.Len() != 0 {
		t.Fatal("failed save published/adopted discovery")
	}
}

func TestWorldEntryStoryRewardSynchronizesAfterCommit(t *testing.T) {
	s, players, wires := worldFixture(t)
	c := players[0]
	wire := wires[0]
	s.Assets.Marks = map[uint16]uint16{13087: 1}
	op := world.Op{Code: world.ActionQuestMark, D1: 13087, D2: 1}
	// Value packs its low byte in the high byte of D4.
	op.D4 = 1 << 8
	ok, err := s.questMark(context.Background(), c, op)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if !contains(wire.packets(t), []byte{15, 19, 1, 1}) {
		t.Fatal("story reward not synchronized")
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].Quests[13087].Step != 1 {
		t.Fatal("story mark not committed", err)
	}
}

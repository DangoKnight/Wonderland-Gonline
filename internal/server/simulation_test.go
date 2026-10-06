package server

import (
	"bytes"
	"context"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
)

func TestWorldSimulationMovementRejectionPreservesState(t *testing.T) {
	s, players, wires := worldFixture(t)
	c := players[0]
	ctx := context.Background()
	s.Assets.Terrains = map[uint16]assets.Terrain{c.character.Map: {Width: 2000, Height: 2000, GridWidth: 100, GridHeight: 100, Cells: make([]byte, 10000)}}
	s.Assets.Terrains[c.character.Map].Cells[60*100+65] = 23
	s.World = world.New(s.Assets)
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	x, y := c.character.X, c.character.Y
	c.arrived = true
	request := protocol.Builder{6, 1, 2}.U16(1200).U16(1300)
	if err := s.worldCommand(ctx, c, request); err != nil {
		t.Fatal(err)
	}
	packets := wires[0].packets(t)
	if len(packets) != 1 || !bytes.Equal(packets[0], c.character.PositionPacket()) || c.character.X != x || c.character.Y != y || !c.arrived {
		t.Fatal("rejection changed session", packets)
	}
	saved, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved[0].X != x || saved[0].Y != y {
		t.Fatal("rejected coordinates persisted")
	}
	wires[0].Reset()
	if err := s.worldCommand(ctx, c, protocol.Builder{6, 1, 2}.U16(1100).U16(1100)); err != nil {
		t.Fatal(err)
	}
	if c.character.X != 1100 || c.character.Y != 1100 || c.arrived {
		t.Fatal("valid movement rejected")
	}
}

func TestWorldSimulationVisibilityAndInstanceIsolation(t *testing.T) {
	s, players, wires := worldFixture(t)
	mapID := players[0].character.Map
	s.Assets.Maps[mapID] = assets.Map{ID: mapID, NPCs: []assets.MapNPC{{ClickID: 7, Template: 17001, Name: "Rabbit", Flags: 1, X: 100, Y: 100, WalkBehavior: 2, WalkSteps: []assets.WalkStep{{X: 200, Y: 100, Delay: 2000}}}}}
	s.Assets.NPCs = map[uint16]assets.NPC{17001: {ID: 17001, Name: "Rabbit"}}
	s.World = world.New(s.Assets)
	for i, c := range players {
		c.ready = true
		s.world[c.info.ID] = c
		wires[i].Reset()
	}
	players[1].view.Hidden[7] = true
	now := time.Unix(1000, 0)
	s.simulateWorld(now)
	s.simulateWorld(now.Add(8 * time.Second))
	want := []byte{22, 2, 7, 0, 200, 0, 100, 0, 2}
	if got := wires[0].packets(t); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatal(got)
	}
	if wires[1].Len() != 0 || wires[2].Len() != 0 {
		t.Fatal("hidden or foreign-map viewer received movement")
	}
	wires[0].Reset()
	players[0].event = &eventSession{click: 7}
	s.simulateWorld(now.Add(11 * time.Second))
	if wires[0].Len() != 0 {
		t.Fatal("scripted actor moved")
	}
	players[0].event = nil
	players[0].tentOwner = players[0].character.ID
	s.simulateWorld(now.Add(20 * time.Second))
	if wires[0].Len() != 0 {
		t.Fatal("world update leaked into tent")
	}
	players[0].tentOwner = 0
	s.simulateWorld(now.Add(30 * time.Second))
	if len(wires[0].packets(t)) != 1 {
		t.Fatal("actor did not resume")
	}
}

func TestWorldSimulationFailedRecipientIsIsolated(t *testing.T) {
	s, players, wires := worldFixture(t)
	mapID := players[0].character.Map
	s.Assets.Maps[mapID] = assets.Map{ID: mapID, NPCs: []assets.MapNPC{{ClickID: 7, Template: 17001, Name: "Rabbit", Flags: 1, X: 100, Y: 100, WalkBehavior: 2, WalkSteps: []assets.WalkStep{{X: 200, Y: 100}}}}}
	s.Assets.NPCs = map[uint16]assets.NPC{17001: {ID: 17001, Name: "Rabbit"}}
	s.World = world.New(s.Assets)
	for i, c := range players {
		c.ready = true
		s.world[c.info.ID] = c
		wires[i].Reset()
	}
	failed := &failedWorldConn{}
	players[0].conn = failed
	now := time.Unix(1, 0)
	s.simulateWorld(now)
	s.simulateWorld(now.Add(8 * time.Second))
	if !failed.closed || len(wires[1].packets(t)) != 1 || !players[1].ready {
		t.Fatal("recipient failure stopped simulation")
	}
}

func TestWorldSimulationVehicleTravelKeepsSceneBounds(t *testing.T) {
	s, players, wires := worldFixture(t)
	c := players[0]
	ctx := context.Background()
	c.ready = true
	s.world[c.info.ID] = c
	c.character.Bag[0] = game.Item{ID: 48001, Count: 1}
	c.character.ActiveVehicle = 48001
	c.character.VehicleSlot = 1
	s.Assets.Items[48001] = game.ItemDefinition{ID: 48001, Type: game.VehicleType}
	cells := make([]byte, 10000)
	cells[60*100+65] = 23
	s.Assets.Terrains = map[uint16]assets.Terrain{c.character.Map: {Width: 2000, Height: 2000, GridWidth: 100, GridHeight: 100, Cells: cells}}
	s.World = world.New(s.Assets)
	if err := s.worldCommand(ctx, c, protocol.Builder{6, 1, 2}.U16(1200).U16(1300)); err != nil {
		t.Fatal(err)
	}
	if c.character.X != 1200 || c.character.Y != 1300 {
		t.Fatal("land mask blocked valid vehicle")
	}
	wires[0].Reset()
	if err := s.worldCommand(ctx, c, protocol.Builder{6, 1, 2}.U16(2000).U16(1300)); err != nil {
		t.Fatal(err)
	}
	if c.character.X != 1200 || len(wires[0].packets(t)) != 1 {
		t.Fatal("vehicle escaped bounds")
	}
	c.character.ActiveVehicle = 48002
	// An unowned riding flag does not bypass the blocked destination.
	cells[61*100+65] = 23
	if err := s.worldCommand(ctx, c, protocol.Builder{6, 1, 2}.U16(1220).U16(1300)); err != nil {
		t.Fatal(err)
	}
	if c.character.X != 1200 {
		t.Fatal("stale vehicle bypassed collision")
	}
}

func TestNativeMovementRetargetAcrossBlockedRouteDoesNotSnap(t *testing.T) {
	for _, code := range []byte{1, 2} {
		t.Run(map[byte]string{1: "destination", 2: "stop"}[code], func(t *testing.T) {
			testNativeMovementRetargetAcrossBlockedRouteDoesNotSnap(t, code)
		})
	}
}

func testNativeMovementRetargetAcrossBlockedRouteDoesNotSnap(t *testing.T, code byte) {
	s, players, wires := worldFixture(t)
	c := players[0]
	ctx := context.Background()
	terrain := assets.Terrain{Width: 2000, Height: 2000, GridWidth: 100, GridHeight: 100, Cells: make([]byte, 10000)}
	terrain.Cells[60*100+65] = 23
	s.Assets.Terrains = map[uint16]assets.Terrain{c.character.Map: terrain}
	s.World = world.New(s.Assets)
	for i, player := range players {
		if err := s.worldCommand(ctx, player, []byte{12, 1}); err != nil {
			t.Fatal(err)
		}
		wires[i].Reset()
	}
	for _, wire := range wires {
		wire.Reset()
	}
	native := func(x, y uint16) []byte { return protocol.Builder{6, code, 2}.U16(x).U16(y).Bytes(make([]byte, 8)) }
	// The client can reroute around a blocked cell; the old announced destination
	// does not describe the avatar's current location or the full walkable route.
	if err := s.dispatch(ctx, c, native(1400, 1500)); err != nil {
		t.Fatal(err)
	}
	for _, wire := range wires {
		wire.Reset()
	}
	if err := s.dispatch(ctx, c, native(1100, 1100)); err != nil {
		t.Fatal(err)
	}
	if c.character.X != 1100 || c.character.Y != 1100 {
		t.Fatal("native retarget rejected")
	}
	for _, packet := range wires[0].packets(t) {
		if packet[0] == 7 {
			t.Fatal("valid retarget snapped client", packet)
		}
	}
	for _, wire := range wires {
		wire.Reset()
	}
	for _, packet := range [][]byte{native(1200, 1300), native(2500, 2500)} {
		if err := s.dispatch(ctx, c, packet); err != nil {
			t.Fatal(err)
		}
	}
	if c.character.X != 1100 || c.character.Y != 1100 {
		t.Fatal("invalid destination accepted")
	}
	for _, wire := range wires {
		if wire.Len() != 0 {
			t.Fatal("invalid click snapped client or broadcast false movement")
		}
	}
}

func boundedEncounterFixture(t *testing.T) (*Server, *Session, *captureConn) {
	t.Helper()
	s, c, wire := fieldFixture(t)
	c.character.X, c.character.Y = 1200, 1000
	c.encounter.mapEnter = time.Now().Add(-time.Minute)
	s.Assets.Maps[30001] = assets.Map{ID: 30001, NPCs: []assets.MapNPC{{ClickID: 4, Flags: 1, X: 1000, Y: 1000, Template: 17100, WalkBehavior: 3, WalkSteps: []assets.WalkStep{{X: uint32(0xfffffed4), Y: uint32(0xfffffed4)}, {X: 300, Y: 300}}}}}
	s.World = world.New(s.Assets)
	t.Cleanup(func() { s.worldMu.Lock(); defer s.worldMu.Unlock(); s.abandonBattle(c) })
	return s, c, wire
}

func TestWorldSimulationBoundedAggressionScansAndContact(t *testing.T) {
	s, c, wire := boundedEncounterFixture(t)
	now := time.Now()
	s.simulateWorld(now)
	if s.actorPursuits[30001][4].target != c.character.ID || c.battle != nil {
		t.Fatal("target acquisition/contact", c.battle)
	}
	// A departed target is dropped immediately; reacquisition waits three seconds.
	c.character.X = 1400
	s.simulateWorld(now.Add(time.Second))
	if s.actorPursuits[30001][4].target != 0 {
		t.Fatal("pursued outside bounds")
	}
	c.character.X, c.character.Y = 1050, 1000
	s.simulateWorld(now.Add(2 * time.Second))
	if c.battle != nil || s.actorPursuits[30001][4].target != 0 {
		t.Fatal("searched too frequently")
	}
	s.simulateWorld(now.Add(4 * time.Second))
	// Random wandering may move the actor during the no-target interval; bring
	// the player into contact with its announced position for the next tick.
	if c.battle == nil {
		n, _ := s.World.NPC(30001, 4)
		c.character.X, c.character.Y = n.X, n.Y
		s.simulateWorld(now.Add(time.Minute))
	}
	if c.battle == nil || c.battle.encounter != 4 || !s.World.Defeated(30001, 4) || !c.view.Hidden[4] {
		t.Fatal("approaching actor did not start and reserve battle", c.battle)
	}
	if !contains(wire.packets(t), s.World.HideActor(world.NewView(), 30001, 4)) {
		t.Fatal("missing despawn packet")
	}
	if len(s.World.Revive(now.Add(time.Hour))) != 0 {
		t.Fatal("reserved actor revived during battle")
	}
	s.worldMu.Lock()
	s.abandonBattle(c)
	s.worldMu.Unlock()
	s.reviveMonsters(time.Now().Add(world.EncounterRespawn + time.Second))
	if s.World.Defeated(30001, 4) || c.view.Hidden[4] {
		t.Fatal("disconnect did not release short respawn")
	}
}

func TestWorldSimulationBoundedAggressionEligibility(t *testing.T) {
	for _, kind := range []string{"outside", "distant", "hidden", "ghost", "loading", "event", "trade", "fishing", "cooldown", "entry", "tent", "wall"} {
		t.Run(kind, func(t *testing.T) {
			s, c, _ := boundedEncounterFixture(t)
			now := time.Now()
			c.character.X = 1050
			switch kind {
			case "outside":
				c.character.X = 1400
			case "distant":
				c.character.X = 1300
			case "hidden":
				c.view.Hidden[4] = true
			case "ghost":
				c.invisible = true
			case "loading":
				c.ready = false
			case "event":
				c.event = &eventSession{}
			case "trade":
				c.trade = &tradeSession{}
			case "fishing":
				c.fishing = &fishingRun{}
			case "cooldown":
				c.encounter.battleEnd = now
				c.encounter.cooldown = time.Minute
			case "entry":
				c.encounter.mapEnter = now
			case "tent":
				c.tentOwner = c.character.ID
			case "wall":
				cells := make([]byte, 10000)
				cells[51*100+50] = assets.TerrainBlockedMask
				s.Assets.Terrains = map[uint16]assets.Terrain{30001: {Width: 2000, Height: 2000, GridWidth: 100, GridHeight: 100, Cells: cells}}
				s.World = world.New(s.Assets)
			}
			s.simulateWorld(now)
			if c.battle != nil {
				t.Fatal("ineligible target entered battle")
			}
			if kind != "wall" && s.actorPursuits[30001][4] != nil && s.actorPursuits[30001][4].target != 0 {
				t.Fatal("ineligible target acquired")
			}
		})
	}
}

func TestWorldSimulationNativeRoamingEncounter(t *testing.T) {
	for _, behavior := range []byte{3, 4, 5} {
		t.Run(string(rune('0'+behavior)), func(t *testing.T) {
			s, c, _ := boundedEncounterFixture(t)
			m := s.Assets.Maps[30001]
			m.NPCs[0].WalkBehavior = behavior
			m.NPCs[0].Flags = 5
			m.NPCs[0].Events = []byte{1}
			m.Events = []assets.Event{{ClickID: 1, Kind: 10, Branches: []assets.Branch{{Index: 1, Operations: []assets.Operation{evOp(1, world.ActionBattle, 1, 17100, 0, 0)}}}}}
			s.Assets.Maps[30001] = m
			s.World = world.New(s.Assets)
			c.character.X = 1050
			s.simulateWorld(time.Now())
			if c.battle == nil || c.battle.event == nil || c.battle.encounter != 4 || len(c.battle.b.Defenders) != 1 || !s.World.Defeated(30001, 4) {
				t.Fatal("native event formation/reservation not preserved", c.battle)
			}
		})
	}
}

func TestWorldSimulationClickedActorShortRespawnAfterEveryOutcome(t *testing.T) {
	for _, outcome := range []battle.Outcome{battle.Victory, battle.Defeat, battle.Fled} {
		t.Run(string(rune('0'+outcome)), func(t *testing.T) {
			s, c, wire := boundedEncounterFixture(t)
			c.character.X = 1050
			if err := s.worldCommand(context.Background(), c, []byte{20, 1, 4, 0}); err != nil || c.battle == nil {
				t.Fatal("clicked encounter", err)
			}
			if !s.World.Defeated(30001, 4) || !c.view.Hidden[4] {
				t.Fatal("clicked actor was not hidden immediately")
			}
			s.worldMu.Lock()
			s.endBattle(c.battle, outcome)
			s.worldMu.Unlock()
			settle(t, s, c)
			if c.battle != nil || !s.World.Defeated(30001, 4) {
				t.Fatal("battle did not finish with actor still cooling down")
			}
			wire.Reset()
			s.reviveMonsters(time.Now().Add(world.EncounterRespawn + time.Second))
			if s.World.Defeated(30001, 4) || c.view.Hidden[4] || len(wire.packets(t)) != 1 {
				t.Fatal("short respawn not published")
			}
		})
	}
}

func TestWorldSimulationEncounterObserverAndPartyReservation(t *testing.T) {
	s, c, wire := boundedEncounterFixture(t)
	observerWire := &captureConn{}
	observer := &Session{conn: observerWire, info: SessionInfo{ID: 90}, character: &game.Character{ID: 90000, Map: 30001}, view: world.NewView(), ready: true}
	s.world[observer.info.ID] = observer
	ctx := context.Background()
	account, err := s.Store.Register(ctx, "Nearby", "password", "")
	if err != nil {
		t.Fatal(err)
	}
	char, err := game.NewCharacter(account.CharacterID(1), 1, "Nearby", game.Appearance{Body: 1, Element: 3}, s.Assets.StarterItems, s.Assets.Items, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	char.Map, char.X, char.Y = 30001, 1050, 1000
	if err := s.Store.CreateCharacter(ctx, account, char); err != nil {
		t.Fatal(err)
	}
	teammate := &Session{conn: &captureConn{}, info: SessionInfo{ID: 91}, account: account, character: &char, view: world.NewView(), pets: newPetRoster(), ready: true}
	c.party = &party{members: []*Session{c, teammate}}
	teammate.party = c.party
	s.world[teammate.info.ID] = teammate
	t.Cleanup(func() { s.worldMu.Lock(); defer s.worldMu.Unlock(); s.abandonBattle(teammate) })
	c.character.X = 1050
	if err := s.worldCommand(context.Background(), c, []byte{20, 1, 4, 0}); err != nil {
		t.Fatal(err)
	}
	run := c.battle
	observerPackets := observerWire.packets(t)
	if run == nil || teammate.battle != run || !observer.view.Hidden[4] || !contains(observerPackets, s.World.HideActor(world.NewView(), 30001, 4)) || !contains(observerPackets, battleState(c.character.ID, true)) {
		t.Fatalf("observer did not see enemy despawn: battle=%v teammate=%v hidden=%v packets=%v", run != nil, teammate.battle == run, observer.view.Hidden[4], observerPackets)
	}
	// A second click must not start a concurrent encounter with this actor.
	if handled, err := s.wildClick(observer, world.NPC{ClickID: 4, Template: 17100}); !handled || err != nil || observer.battle != nil {
		t.Fatal("reserved actor accepted another click", err)
	}
	// The remaining party member keeps the reservation when its initiator disconnects.
	s.worldMu.Lock()
	s.abandonBattle(c)
	s.worldMu.Unlock()
	if !s.World.Defeated(30001, 4) || len(s.World.Revive(time.Now().Add(time.Hour))) != 0 {
		t.Fatal("first member released a shared battle reservation")
	}
	s.worldMu.Lock()
	s.abandonBattle(teammate)
	s.worldMu.Unlock()
	wire.Reset()
	observerWire.Reset()
	s.reviveMonsters(time.Now().Add(world.EncounterRespawn + time.Second))
	if c.view.Hidden[4] || observer.view.Hidden[4] || len(wire.packets(t)) != 1 || len(observerWire.packets(t)) != 1 {
		t.Fatal("respawn was not sent to both viewers")
	}
}

func TestWorldSimulationRoamingAndPatrolTerritory(t *testing.T) {
	for _, tt := range []struct {
		name     string
		behavior byte
		path     []assets.WalkStep
		playerX  uint16
		acquire  bool
	}{
		{"roamer inside", 4, nil, 1050, true}, {"roamer expanded area", 4, nil, 1140, true}, {"roamer outside", 4, nil, 1151, false},
		{"patrol nearby", 5, []assets.WalkStep{{X: 1200, Y: 1000}}, 1150, true},
		{"patrol distant", 5, []assets.WalkStep{{X: 1200, Y: 1000}}, 1250, false},
		{"pathless patrol inside", 5, nil, 1050, true}, {"pathless patrol outside", 5, nil, 1151, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, c, _ := boundedEncounterFixture(t)
			m := s.Assets.Maps[30001]
			m.NPCs[0].WalkBehavior = tt.behavior
			m.NPCs[0].WalkSteps = tt.path
			s.Assets.Maps[30001] = m
			s.World = world.New(s.Assets)
			c.character.X = tt.playerX
			now := time.Now()
			s.simulateWorld(now)
			pursuit := s.actorPursuits[30001][4]
			if pursuit == nil || (pursuit.target != 0) != tt.acquire {
				t.Fatal("incorrect acquisition", pursuit)
			}
			if !tt.acquire {
				if err := s.stepEncounter(c, c.character.X-1, c.character.Y); err != nil {
					t.Fatal(err)
				}
				if c.battle != nil {
					t.Fatal("outside player entered a proximity battle")
				}
			}
			if tt.acquire && c.battle == nil {
				// Move out of the designated area before the next search is due.
				c.character.X = 1400
				s.simulateWorld(now.Add(time.Second))
				if pursuit.target != 0 {
					t.Fatal("invalid target not discarded immediately")
				}
			}
		})
	}
}

func TestWorldSimulationRetainsPlayerTargetWhilePursuing(t *testing.T) {
	s, c, _ := boundedEncounterFixture(t)
	now := time.Now()
	s.simulateWorld(now)
	pursuit := s.actorPursuits[30001][4]
	if pursuit.target != c.character.ID {
		t.Fatal("first target not acquired")
	}
	observer := &Session{conn: &captureConn{}, info: SessionInfo{ID: 90}, character: &game.Character{ID: 90000, Map: 30001, X: 1100, Y: 1000}, view: world.NewView(), ready: true}
	s.world[observer.info.ID] = observer
	s.simulateWorld(now.Add(3 * time.Second))
	if pursuit.target != c.character.ID || !s.World.ActorMoving(30001, 4, now.Add(time.Second)) {
		t.Fatal("current target replaced at scan deadline")
	}
	s.simulateWorld(now.Add(4 * time.Second))
	if pursuit.target != c.character.ID || observer.battle != nil {
		t.Fatal("closer observer stole moving monster")
	}
	s.simulateWorld(now.Add(6 * time.Second))
	if pursuit.target != c.character.ID {
		t.Fatal("retained target lost after arrival")
	}
	c.invisible = true
	s.simulateWorld(now.Add(7 * time.Second))
	if pursuit.target != 0 || !pursuit.nextScan.Equal(now.Add(10*time.Second)) {
		t.Fatal("target loss did not reset search interval", pursuit)
	}
	s.simulateWorld(now.Add(8 * time.Second))
	if pursuit.target != 0 {
		t.Fatal("searched again before cooldown/arrival")
	}
	s.simulateWorld(now.Add(10 * time.Second))
	if pursuit.target != observer.character.ID {
		t.Fatal("new target not acquired after loss and arrival", pursuit)
	}
	t.Cleanup(func() { s.worldMu.Lock(); defer s.worldMu.Unlock(); s.abandonBattle(observer) })
}

func TestWorldSimulationAcquiresPlayerDuringPatrol(t *testing.T) {
	s, c, wire := boundedEncounterFixture(t)
	def := s.Assets.Maps[30001]
	def.NPCs[0].WalkBehavior = 5
	def.NPCs[0].WalkSteps = []assets.WalkStep{{X: 1300, Y: 1000}}
	s.Assets.Maps[30001] = def
	s.World = world.New(s.Assets)
	c.invisible = true
	now := time.Now()
	s.simulateWorld(now)
	s.simulateWorld(now.Add(3 * time.Second))
	if !hasPacket(wire.packets(t), []byte{22, 2, 4, 0, 20, 5, 232, 3, 2}) {
		t.Fatal("patrol did not begin")
	}
	c.invisible = false
	c.character.X = 950
	s.simulateWorld(now.Add(4 * time.Second))
	if s.actorPursuits[30001][4].target != 0 {
		t.Fatal("searched before three-second deadline")
	}
	s.simulateWorld(now.Add(6 * time.Second))
	if s.actorPursuits[30001][4].target != c.character.ID {
		t.Fatal("moving actor did not acquire nearby player")
	}
	// Current position is 1120; the old destination 1300 is outside detection range.
	npc, _ := s.World.NPCAt(30001, 4, now.Add(6*time.Second))
	if npc.X != 1120 || c.battle != nil {
		t.Fatal("interruption snapped position or started premature battle", npc)
	}
	if !hasPacket(wire.packets(t), []byte{22, 2, 4, 0, 252, 3, 232, 3, 2}) {
		t.Fatal("player did not interrupt patrol with a leg to 1020")
	}
}

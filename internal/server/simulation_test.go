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
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/world"
)

// fieldFixture puts Alice on field map 30001 with a one-HP wild boar (click 4) beside
// her and a second one (click 5) far away.
func fieldFixture(t *testing.T) (*Server, *Session, *captureConn) {
	battleSleep, turnTimeout = func(time.Duration) {}, time.Hour
	t.Cleanup(func() { battleSleep, turnTimeout = time.Sleep, defaultTurnTimeout })
	s, players, wires := worldFixture(t)
	// Finish playback and cancel turn timers before closing the database or
	// restoring global timing hooks. Otherwise an unfinished fixture can fire
	// its next round during a later test and race that test's hook cleanup.
	t.Cleanup(func() {
		for _, player := range players {
			settle(t, s, player)
			s.worldMu.Lock()
			s.abandonBattle(player)
			s.worldMu.Unlock()
		}
	})
	s.Assets.Maps[30001] = assets.Map{ID: 30001, NPCs: []assets.MapNPC{{ClickID: 4, Flags: 1, X: 1050, Y: 1080, Template: 17100}, {ClickID: 5, Flags: 1, X: 3000, Y: 3000, Template: 17100}}}
	s.Assets.NPCs = map[uint16]assets.NPC{17100: {ID: 17100, Name: "Wild Boar", Level: 2, HP: 1}}
	s.World = world.New(s.Assets)
	c := players[0]
	c.character.Map = 30001
	c.encounter = encounterState{next: firstEncounter}
	c.encounter.enterMap()
	if err := s.worldCommand(context.Background(), c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	return s, c, wires[0]
}

func TestWildMonsterClickDefeatAndRespawn(t *testing.T) {
	s, c, wire := fieldFixture(t)
	ctx := context.Background()
	if !s.World.Wild(30001, world.NPC{ClickID: 4, Template: 17100}) || s.World.SafeTown(30001) || !s.World.SafeTown(10017) {
		t.Fatal("classification")
	}
	if err := s.worldCommand(ctx, c, []byte{20, 1, 4, 0}); err != nil || c.battle == nil {
		t.Fatal("wild click did not start a battle", err)
	}
	if f := c.battle.b.Defenders[0]; f.HP != 1 || f.MaxHP != 1 || f.SP != 0 || f.MaxSP != 0 {
		t.Fatalf("wild monster did not use stored vitals: %+v", f)
	}
	// Attack until the clicked monster's stored HP is depleted.
	for round := 0; c.battle != nil; round++ {
		if round == 30 {
			t.Fatal("battle did not end")
		}
		s.worldMu.Lock()
		c.battle.members[0].self.HP = c.battle.members[0].self.MaxHP
		s.worldMu.Unlock()
		if err := s.worldCommand(ctx, c, []byte{50, 1, 4, 2, 2, 2}); err != nil {
			t.Fatal(err)
		}
		settle(t, s, c)
	}
	got := wire.packets(t)
	if !contains(got, protocol.Builder{22, 4}.Bytes(world.NPCRecord(4, 0, 1050, 1080, true))) || !s.World.Defeated(30001, 4) || !c.view.Hidden[4] {
		t.Fatal("defeated monster not removed", got)
	}
	c.resumeAt = time.Time{}
	if err := s.worldCommand(ctx, c, []byte{20, 1, 4, 0}); err != nil || c.battle != nil {
		t.Fatal("defeated monster fought again", err)
	}
	wire.Reset()
	s.reviveMonsters(time.Now().Add(world.MonsterRespawn + time.Second))
	if got = wire.packets(t); len(got) != 1 || got[0][0] != 22 || c.view.Hidden[4] {
		t.Fatal("respawn", got)
	}
}

func TestFieldEncounters(t *testing.T) {
	s, c, _ := fieldFixture(t)
	ctx := context.Background()
	move := func(x, y uint16) {
		t.Helper()
		if err := s.worldCommand(ctx, c, protocol.Builder{6, 1, 0}.U16(x).U16(y)); err != nil {
			t.Fatal(err)
		}
	}
	// The map-entry grace period suppresses encounters.
	c.encounter.next = 1
	move(1500, 1500)
	if c.battle != nil {
		t.Fatal("battle during the grace period")
	}
	c.encounter.mapEnter = time.Now().Add(-time.Minute)
	move(1510, 1500)
	if c.battle == nil || len(c.battle.b.Defenders) < 1 || len(c.battle.b.Defenders) > 4 || c.battle.wild {
		t.Fatal("random encounter", c.battle)
	}
	s.worldMu.Lock()
	s.endBattle(c.battle, 3) // Flee.
	s.worldMu.Unlock()
	settle(t, s, c)
	// After a battle, proximity re-arms only once the player is away from every monster.
	c.encounter.battleEnd, c.encounter.next = time.Time{}, 100
	move(1060, 1090)
	if c.battle != nil {
		t.Fatal("proximity encounter before re-arming")
	}
	move(1500, 1500)
	move(1060, 1090)
	if c.battle == nil || !c.battle.wild || c.battle.encounter != 4 {
		t.Fatal("proximity encounter", c.battle)
	}
	if !bytes.Equal(battleState(c.character.ID, true)[:2], []byte{11, 4}) {
		t.Fatal("state packet")
	}
}

func TestRestingPoses(t *testing.T) {
	for pose, want := range map[byte]bool{0: false, 9: false, 25: false, 26: true, 27: true, 28: false, 45: false, 46: true, 53: true, 54: false} {
		if restingPose(pose) != want {
			t.Errorf("pose %d resting = %v", pose, !want)
		}
	}
}

// TestRestingRecoversVitalsAndCheckpoints: sitting recovers HP and SP every
// interval with AC8 reports, the checkpoint saves them without a full
// rewrite, committed SQL results keep the buffered recovery, and walking
// ends the pose.
func TestRestingRecoversVitalsAndCheckpoints(t *testing.T) {
	s, players, wires := chatFixture(t)
	ctx := context.Background()
	c := players[0]
	s.world[c.info.ID] = c
	c.ready = true
	next := c.character.Clone()
	next.HP, next.SP = 1, 0
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	full := c.character.Combat(s.Assets.Items)
	maxHP, maxSP := uint32(full.MaxHP), uint32(full.MaxSP)
	stepHP, stepSP := max(1, maxHP*restRecoveryPercent/100), max(1, maxSP*restRecoveryPercent/100)
	t0 := time.Now()

	s.rest(c, t0)
	if c.character.HP != 1 || wires[0].Len() != 0 {
		t.Fatal("standing characters recover")
	}
	if err := s.worldCommand(ctx, c, []byte{protocol.CommandPose, protocol.PoseBroadcast, protocol.PoseSitFirst + 2}); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	s.rest(c, t0)
	s.rest(c, t0.Add(restRecoveryInterval-time.Millisecond))
	if c.character.HP != 1 || wires[0].Len() != 0 {
		t.Fatal("recovered before the interval")
	}
	s.rest(c, t0.Add(restRecoveryInterval))
	hp, sp := 1+stepHP, stepSP
	got := wires[0].packets(t)
	if c.character.HP != hp || c.character.SP != sp || len(got) != 2 ||
		!bytes.Equal(got[0], serverwire.VitalPacket(game.StatCurrentHP, hp)) || !bytes.Equal(got[1], serverwire.VitalPacket(game.StatCurrentSP, sp)) {
		t.Fatalf("HP %d SP %d packets %x", c.character.HP, c.character.SP, got)
	}

	s.autosaveCharacters(ctx)
	saved, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || saved[0].HP != hp || saved[0].SP != sp {
		t.Fatal("checkpoint", saved[0].HP, saved[0].SP, err)
	}

	// A transaction result that leaves the vitals at the checkpoint keeps
	// newer buffered recovery.
	s.rest(c, t0.Add(2*restRecoveryInterval))
	s.adoptSavedCharacter(c, saved[0])
	if c.character.HP != hp+stepHP {
		t.Fatal("committed result discarded buffered recovery", c.character.HP)
	}

	// Recovery stops at the maxima.
	c.character.HP, c.character.SP = maxHP, maxSP
	wires[0].Reset()
	s.rest(c, t0.Add(4*restRecoveryInterval))
	if c.character.HP != maxHP || wires[0].Len() != 0 {
		t.Fatal("recovered past the maximum")
	}

	// Walking ends the held pose.
	c.character.HP = 1
	if err := s.worldCommand(ctx, c, []byte{6, 1, 2, 0xb0, 4, 0x14, 5}); err != nil {
		t.Fatal(err)
	}
	s.rest(c, t0.Add(5*restRecoveryInterval))
	s.rest(c, t0.Add(6*restRecoveryInterval))
	if c.emote != 0 || c.character.HP != 1 {
		t.Fatal("walking kept the resting pose", c.emote, c.character.HP)
	}
}

func TestRestingRecoversActivePetAndPlayer(t *testing.T) {
	s, players, wires := chatFixture(t)
	c, ctx := players[0], context.Background()
	next := c.character.Clone()
	next.HP, next.SP = 1, 0
	pet := game.NewPet(14162, "Roca", 3, game.PetTemplate{}, s.Assets.Items)
	pet.HP, pet.SP, pet.Battle = 1, 0, true
	idle := game.NewPet(12032, "Robinson", 1, game.PetTemplate{}, s.Assets.Items)
	idle.HP, idle.SP = 1, 0
	next.Pets, next.ActivePet = []game.Pet{idle, pet}, pet.ID
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	c.pets = newPetRoster()
	c.pets.register(idle.ID)
	c.pets.register(pet.ID) // Client slot 2 differs from party slot 3.
	c.emote = 46
	t0 := time.Unix(100, 0)
	s.rest(c, t0)
	s.rest(c, t0.Add(restRecoveryInterval))
	full := pet.Combat(s.Assets.Items)
	hp, sp := 1+max(1, full.MaxHP*2/100), max(1, full.MaxSP*2/100)
	if c.character.Pets[1].HP != hp || c.character.Pets[1].SP != sp || c.character.HP <= 1 || c.character.SP == 0 {
		t.Fatalf("player/pet recovery: %+v %+v", c.character, c.character.Pets[1])
	}
	if c.character.Pets[0].HP != 1 || c.character.Pets[0].SP != 0 {
		t.Fatal("inactive pet recovered")
	}
	got := wires[0].packets(t)
	// Independent native AC8:2 records: collection 4, client slot 2, HP/SP.
	wantHP := protocol.Builder{8, 2, 4, 2, 0, 25, 1}.U32(uint32(hp)).U32(0)
	wantSP := protocol.Builder{8, 2, 4, 2, 0, 26, 1}.U32(uint32(sp)).U32(0)
	if len(got) != 4 || !contains(got, wantHP) || !contains(got, wantSP) {
		t.Fatal("player/pet vital packets", got)
	}
	saved, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || saved[0].Pets[1].HP != 1 || saved[0].HP != 1 {
		t.Fatal("rest recovery wrote before checkpoint", err)
	}
	s.autosaveCharacters(ctx)
	saved, err = s.Store.Characters(ctx, c.account.ID)
	if err != nil || saved[0].Pets[1].HP != hp || saved[0].Pets[1].SP != sp {
		t.Fatal("pet checkpoint", err)
	}
	// A full owner must not prevent the pet's last, capped recovery tick.
	stats := c.character.Combat(s.Assets.Items)
	c.character.HP, c.character.SP = uint32(stats.MaxHP), uint32(stats.MaxSP)
	c.character.Pets[1].HP, c.character.Pets[1].SP = full.MaxHP-1, full.MaxSP-1
	wires[0].Reset()
	s.rest(c, t0.Add(2*restRecoveryInterval))
	if c.character.Pets[1].HP != full.MaxHP || c.character.Pets[1].SP != full.MaxSP || len(wires[0].packets(t)) != 2 {
		t.Fatal("full player blocked pet recovery or pet exceeded maxima")
	}
	// Event, battle, loading and non-resting poses reset the recovery clock.
	for _, block := range []func(){
		func() { c.event = &eventSession{} },
		func() { c.battle = &battleRun{} },
		func() { c.ready = false },
		func() { c.emote = 8 },
	} {
		c.character.Pets[1].HP = 1
		block()
		s.rest(c, t0.Add(3*restRecoveryInterval))
		if c.character.Pets[1].HP != 1 || !c.restedAt.IsZero() {
			t.Fatal("blocked pet recovered")
		}
		c.event, c.battle, c.ready, c.emote = nil, nil, true, 26
	}
}

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
	if code == 1 && (!c.walk.active || c.walk.toX != 1100 || c.walk.toY != 1100) || code == 2 && (c.character.X != 1100 || c.character.Y != 1100) {
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
	if code == 1 && (c.walk.toX != 1100 || c.walk.toY != 1100) || code == 2 && (c.character.X != 1100 || c.character.Y != 1100) {
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
	t.Cleanup(func() { settle(t, s, c); s.worldMu.Lock(); defer s.worldMu.Unlock(); s.abandonBattle(c) })
	return s, c, wire
}

func TestWorldSimulationBoundedAggressionScansAndContact(t *testing.T) {
	s, c, wire := boundedEncounterFixture(t)
	now := time.Now()
	s.simulateWorld(now)
	if s.actorPursuits[30001][4].target != c.character.ID || c.battle != nil {
		t.Fatal("target acquisition/contact", c.battle)
	}
	// A departed target is dropped immediately; reacquisition waits half a second.
	c.character.X = 1400
	s.simulateWorld(now.Add(time.Second))
	if s.actorPursuits[30001][4].target != 0 {
		t.Fatal("pursued outside bounds")
	}
	c.character.X, c.character.Y = 1050, 1000
	s.simulateWorld(now.Add(1250 * time.Millisecond))
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

func TestWorldSimulationHalfSecondScanDoesNotMove(t *testing.T) {
	s, c, wire := boundedEncounterFixture(t)
	now := time.Now()
	c.invisible = true
	s.simulateWorldTick(now, false)
	c.invisible = false
	s.simulateWorldTick(now.Add(499*time.Millisecond), false)
	if s.actorPursuits[30001][4].target != 0 {
		t.Fatal("target scanned before half-second deadline")
	}
	s.simulateWorldTick(now.Add(500*time.Millisecond), false)
	if s.actorPursuits[30001][4].target != c.character.ID {
		t.Fatal("target not acquired on half-second scan")
	}
	for _, p := range wire.packets(t) {
		if len(p) >= 2 && p[0] == 22 && p[1] == 2 {
			t.Fatal("scan-only tick emitted movement", p)
		}
	}
	s.simulateWorld(now.Add(time.Second))
	if !hasPacket(wire.packets(t), []byte{22, 2, 4, 0, 76, 4, 232, 3, 2}) {
		t.Fatal("one-second movement tick did not pursue acquired target")
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

func TestWorldSimulationKind8EncounterPursuitAndRespawn(t *testing.T) {
	testFieldEncounterLifecycle(t, 8, 3)
}

func TestWorldSimulationMobileEncounterPursuitAndRespawn(t *testing.T) {
	for _, tt := range []struct {
		name           string
		kind, movement byte
	}{
		{"chicken patrol", 11, 5},
		{"chicken roamer", 11, 4},
		{"patrolling leader", 1, 2},
		{"bounded encounter", 2, 3},
	} {
		t.Run(tt.name, func(t *testing.T) { testFieldEncounterLifecycle(t, tt.kind, tt.movement) })
	}
}

func testFieldEncounterLifecycle(t *testing.T, kind, movement byte) {
	t.Helper()
	s, c, wire := boundedEncounterFixture(t)
	if movement == 4 {
		c.character.X = 1140 // Inside the roamer's 150-pixel spawn leash, outside contact.
	}
	m := s.Assets.Maps[30001]
	m.NPCs[0].Flags = 5
	m.NPCs[0].WalkBehavior = movement
	if movement == 2 || movement == 5 {
		m.NPCs[0].WalkSteps = []assets.WalkStep{{X: 1300, Y: 1000}, {X: 1000, Y: 1000}}
	}
	m.NPCs[0].Events = []byte{42}
	m.Events = []assets.Event{{ClickID: 42, Kind: kind, Branches: []assets.Branch{{Index: 1, Operations: []assets.Operation{evOp(1, world.ActionBattle, 1, 17100, 0, 0)}}}}}
	s.Assets.Maps[30001] = m
	s.World = world.New(s.Assets)
	now := time.Now()
	s.simulateWorldTick(now, false)
	if pursuit := s.actorPursuits[30001][4]; pursuit == nil || pursuit.target != c.character.ID {
		t.Fatal("field enemy did not acquire nearby player")
	}
	c.character.X = 1050
	s.simulateWorldTick(now.Add(actorTargetScanInterval), false)
	if c.battle == nil || c.battle.event == nil || c.battle.encounter != 4 || !s.World.Defeated(30001, 4) || !c.view.Hidden[4] {
		t.Fatal("native encounter did not reserve and hide its map actor")
	}
	s.worldMu.Lock()
	s.endBattle(c.battle, battle.Victory)
	s.worldMu.Unlock()
	settle(t, s, c)
	if c.battle != nil || !s.World.Defeated(30001, 4) || !c.view.Hidden[4] {
		t.Fatal("enemy reappeared immediately after victory")
	}
	wire.Reset()
	s.reviveMonsters(time.Now().Add(world.EncounterRespawn + time.Second))
	if s.World.Defeated(30001, 4) || c.view.Hidden[4] || len(wire.packets(t)) != 1 {
		t.Fatal("enemy did not respawn after its cooldown")
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
	// Keep the target ahead of the now faster pursuit, outside contact range.
	c.character.X = 1300
	s.simulateWorld(now.Add(6 * time.Second))
	if pursuit.target != c.character.ID {
		t.Fatal("retained target lost after arrival")
	}
	c.invisible = true
	s.simulateWorld(now.Add(7 * time.Second))
	if pursuit.target != 0 || !pursuit.nextScan.Equal(now.Add(7500*time.Millisecond)) {
		t.Fatal("target loss did not reset search interval", pursuit)
	}
	s.simulateWorld(now.Add(7250 * time.Millisecond))
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
	s.simulateWorld(now.Add(3250 * time.Millisecond))
	if s.actorPursuits[30001][4].target != 0 {
		t.Fatal("searched before half-second deadline")
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

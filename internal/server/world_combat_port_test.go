package server

import (
	"context"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/world"
)

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
	if err := s.openService(c, 6); err != nil {
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
	if err := s.openService(c, 6); err != nil {
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

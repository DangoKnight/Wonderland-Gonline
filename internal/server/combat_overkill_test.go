package server

import (
	"context"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
)

func overkillDestinations(s *Server) {
	s.Assets.Maps[11016] = assets.Map{ID: 11016}
	s.Assets.Maps[32000] = assets.Map{ID: 32000}
	s.Assets.Maps[30001] = assets.Map{ID: 30001, Scene: 30001}
	s.World = world.New(s.Assets)
}

func overkillStep(m *battleMember) battle.Step {
	m.self.HP, m.self.Overkilled = 0, true
	if m.pet != nil {
		m.pet.Withdrawn = true
	}
	return battle.Step{Packets: [][]byte{{53, 3, m.self.X, m.self.Y}}, Delay: battle.OverkillFlightDuration, Overkills: []*battle.Fighter{m.self}}
}

func TestOverkillPlayerReturnsToDurableSpawn(t *testing.T) {
	for _, record := range []bool{false, true} {
		t.Run(map[bool]string{false: "starter beach", true: "record point"}[record], func(t *testing.T) {
			s, c, wire := battleFixture(t, 1000)
			overkillDestinations(s)
			dst := world.Destination{Map: 11016, X: 1181, Y: 243}
			if record {
				c.character.RecordPoint = &game.Location{Map: 32000, X: 77, Y: 88}
				dst = world.Destination{Map: 32000, X: 77, Y: 88}
			}
			pet := game.Pet{ID: 10501, Slot: 1, Level: 1, Amity: 100, HP: 100, MaxHP: 500, SP: 40, MaxSP: 100, Base: game.Attributes{Constitution: 20, Wisdom: 5}}
			c.character.Pets = []game.Pet{pet}
			c.character.ActivePet = pet.ID
			c.pets.register(pet.ID)
			if err := s.commit(context.Background(), c, c.character.Clone()); err != nil {
				t.Fatal(err)
			}
			run := c.battle
			m := run.member(c)
			m.pet = battle.PetFighter(pet, c.character.ID, nil, 0)
			m.pet.HP, m.pet.SP, m.pet.Deaths = 37, 11, 1
			run.b.Attackers = append(run.b.Attackers, m.pet)
			wire.Reset()
			if s.play(run, []battle.Step{overkillStep(m), {Packets: [][]byte{{51, 1, 4, 2, 25, 99, 0, 0, 0}}}}) {
				t.Fatal("battle kept playing without participants")
			}
			if c.battle != nil || c.event != nil || c.character.Map != dst.Map || c.character.X != dst.X || c.character.Y != dst.Y || c.ready || !c.warped {
				t.Fatal("overkill did not return player", c.character.Map, c.character.X, c.character.Y)
			}
			stored, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil || stored[0].Map != dst.Map || stored[0].X != dst.X || stored[0].Y != dst.Y || stored[0].HP != c.character.HP || stored[0].HP == 0 || stored[0].Pets[0].HP != 37 || stored[0].Pets[0].SP != 11 || stored[0].Pets[0].Amity != 99 {
				t.Fatal("return/vitals not saved together", stored, err)
			}
			if c.autosaveBaseline == nil || c.autosaveBaseline.Map != dst.Map {
				t.Fatal("return checkpoint stale")
			}
			packets := wire.packets(t)
			if !contains(packets, []byte{53, 3, 4, 2}) || !contains(packets, protocol.Builder{11, 0}.U32(c.character.ID).U16(0)) || contains(packets, []byte{51, 1, 4, 2, 25, 99, 0, 0, 0}) {
				t.Fatal("overkill departure sequence", packets)
			}
			if err := s.worldCommand(context.Background(), c, []byte{12, 1}); err != nil {
				t.Fatal(err)
			}
			if !c.ready || c.restoreUIAfterMapLoad {
				t.Fatal("spawn acknowledgment left player locked")
			}
		})
	}
}

func TestOverkillRemovesOnlyThatPlayerMidRound(t *testing.T) {
	s, players, wires := teamBattle(t, 10000)
	overkillDestinations(s)
	alice, bobby := players[0], players[1]
	// Ship Deck intentionally hides map peers. Use a shared field to verify
	// publication of the old-map departure in addition to battle removal.
	alice.character.Map, bobby.character.Map = 30001, 30001
	if s.world[alice.info.ID] != alice || len(s.peers(alice)) == 0 {
		t.Fatalf("fixture lacks published peers: published=%v hidden=%v world=%v", s.world[alice.info.ID] == alice, s.World.HideOtherPlayers(30001), s.world)
	}
	run := alice.battle
	for _, w := range wires {
		w.Reset()
	}
	marker := []byte{51, 1, 4, 3, 25, 77, 0, 0, 0}
	if !s.play(run, []battle.Step{overkillStep(run.member(alice)), {Packets: [][]byte{marker}}}) {
		t.Fatal("teammate's round stopped")
	}
	if alice.battle != nil || bobby.battle != run || run.b.Finished || alice.character.Map != 11016 || bobby.character.Map != 30001 {
		t.Fatal("overkill ended/warped teammate")
	}
	ownerPackets, peerPackets := wires[0].packets(t), wires[1].packets(t)
	if contains(ownerPackets, marker) || !contains(peerPackets, marker) {
		t.Fatal("removed player received later battle step")
	}
	// Old-map peers see the committed departure even though the session has
	// already adopted its new destination from the full-state SQL commit.
	warp := protocol.Builder{12}.U32(alice.character.ID).U16(11016).U16(1181).U16(243).U16(0).U8(0)
	if !contains(peerPackets, warp) {
		t.Fatalf("old-map peer missed overkill warp: want=%v got=%v", warp, peerPackets)
	}
}

func TestOverkillFailedSaveDoesNotPublishReturn(t *testing.T) {
	s, c, wire := battleFixture(t, 1000)
	overkillDestinations(s)
	run := c.battle
	before := c.character.Clone()
	step := overkillStep(run.member(c))
	step.Packets = nil
	wire.Reset()
	s.Store.Close()
	s.play(run, []battle.Step{step})
	if wire.Len() != 0 || c.character.Map != before.Map || c.character.HP != before.HP || c.battle != nil {
		t.Fatal("failed save adopted/published return", wire.packets(t))
	}
}

func TestOverkillInvalidRecordPointFallsBackToBeach(t *testing.T) {
	s, c, _ := battleFixture(t, 1000)
	overkillDestinations(s)
	c.character.RecordPoint = &game.Location{Map: 65500, X: 1, Y: 2}
	run := c.battle
	s.play(run, []battle.Step{overkillStep(run.member(c))})
	if c.character.Map != 11016 {
		t.Fatal("invalid spawn did not fall back")
	}
}

func TestOverkillDisconnectDuringFlightKeepsSavedReturn(t *testing.T) {
	s, c, wire := battleFixture(t, 1000)
	overkillDestinations(s)
	run := c.battle
	run.b.Processing = true
	step := overkillStep(run.member(c))
	entered, release := make(chan struct{}), make(chan struct{})
	oldSleep := battleSleep
	battleSleep = func(time.Duration) { close(entered); <-release }
	defer func() { battleSleep = oldSleep }()
	done := make(chan bool, 1)
	go func() { done <- s.play(run, []battle.Step{step}) }()
	<-entered
	s.worldMu.Lock()
	s.abandonBattle(c)
	s.worldMu.Unlock()
	close(release)
	if <-done {
		t.Fatal("disconnected battle kept running")
	}
	if c.battle != nil || c.character.Map != 11016 {
		t.Fatal("disconnect bypassed overkill return")
	}
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || stored[0].Map != 11016 {
		t.Fatal("disconnected return not durable", err)
	}
	for _, p := range wire.packets(t) {
		if len(p) > 0 && p[0] == 12 {
			t.Fatal("warp sent to closing owner")
		}
	}
}

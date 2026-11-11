package server

import (
	"context"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
)

func nativeWalkRequest(x, y uint16) []byte {
	return protocol.Builder{6, 1, 0}.U16(x).U16(y).Bytes(make([]byte, 8))
}

func TestNativeWalkDoesNotAggroAtDestination(t *testing.T) {
	s, c, _ := boundedEncounterFixture(t)
	c.character.X = 1600
	c.encounter.next = 10000
	if err := s.dispatch(context.Background(), c, nativeWalkRequest(1050, 1000)); err != nil {
		t.Fatal(err)
	}
	if c.character.X != 1600 || c.battle != nil || !c.walk.active {
		t.Fatal("destination treated as arrival")
	}
	started := c.walk.started
	s.simulateWorldTick(started.Add(500*time.Millisecond), false)
	if c.character.X != 1520 || c.battle != nil || s.actorPursuits[30001][4].target != 0 {
		t.Fatal("distant player acquired", c.character.X)
	}
	if err := s.advancePlayerWalk(c, started.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	if c.battle == nil || c.character.X > 1072 || c.character.X < 1000 || c.walk.active {
		t.Fatal("actual contact failed", c.character.X, c.battle != nil)
	}
}

func TestRandomEncounterUsesNativeFieldPopulation(t *testing.T) {
	s, c, _ := boundedEncounterFixture(t)
	m := s.Assets.Maps[30001]
	m.NPCs[0].Flags = 5
	m.NPCs[0].Events = []byte{42}
	m.Events = []assets.Event{{ClickID: 42, Kind: 8, Branches: []assets.Branch{{Operations: []assets.Operation{{Index: 1, Data: [21]byte{4, 1}}}}}}}
	s.Assets.Maps[30001] = m
	s.World = world.New(s.Assets)
	if !s.World.RoamingBattle(30001, 4) {
		t.Fatal("fixture not a native field actor")
	}
	c.encounter.next = 1
	if err := s.dispatch(context.Background(), c, nativeWalkRequest(1240, 1000)); err != nil {
		t.Fatal(err)
	}
	if c.battle != nil || c.character.X != 1200 {
		t.Fatal("random roll occurred before walking")
	}
	if err := s.advancePlayerWalk(c, c.walk.started.Add(300*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if c.battle == nil || c.battle.encounter != 0 || c.event != nil {
		t.Fatal("native population did not create independent random battle")
	}
	if s.World.Defeated(30001, 4) {
		t.Fatal("random battle reserved visible actor")
	}
	for _, f := range c.battle.b.Defenders {
		if f.Template != 17100 {
			t.Fatal("wrong population", f.Template)
		}
	}
}

func TestNativeWalkRetargetAndStopUseReachedPosition(t *testing.T) {
	s, cs, _ := worldFixture(t)
	c := cs[0]
	c.ready = true
	c.character.X, c.character.Y = 1000, 1000
	if err := s.worldCommand(context.Background(), c, nativeWalkRequest(1500, 1000)); err != nil {
		t.Fatal(err)
	}
	c.walk.started = time.Now().Add(-time.Second)
	if err := s.worldCommand(context.Background(), c, nativeWalkRequest(1000, 1000)); err != nil {
		t.Fatal(err)
	}
	if c.walk.fromX < 1160 || c.walk.fromX > 1170 || c.character.X != c.walk.fromX {
		t.Fatal("retarget started at announced endpoint", c.walk)
	}
	stop := protocol.Builder{6, 2, 0}.U16(c.character.X).U16(1000).Bytes(make([]byte, 8))
	if err := s.worldCommand(context.Background(), c, stop); err != nil {
		t.Fatal(err)
	}
	x := c.character.X
	if err := s.advancePlayerWalk(c, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if c.walk.active || c.character.X != x {
		t.Fatal("stopped walk resumed")
	}
}

func TestRandomEncountersRespectTravelEligibility(t *testing.T) {
	for _, block := range []string{"dialogue", "town", "training expired"} {
		t.Run(block, func(t *testing.T) {
			s, c, _ := fieldFixture(t)
			c.encounter.mapEnter = time.Now().Add(-time.Minute)
			c.encounter.next = 1
			switch block {
			case "dialogue":
				c.event = &eventSession{}
			case "town":
				c.character.Map = 10017
			case "training expired":
				m := s.Assets.Maps[c.character.Map]
				m.TrainingArea = true
				s.Assets.Maps[c.character.Map] = m
			}
			if err := s.stepEncounter(c, c.character.X-1, c.character.Y); err != nil {
				t.Fatal(err)
			}
			if c.battle != nil {
				t.Fatal("ineligible random battle")
			}
		})
	}
}

func TestNativeWalkCancellationAndFollowerPosition(t *testing.T) {
	s, cs, _ := worldFixture(t)
	leader, follower := cs[0], cs[1]
	leader.ready, follower.ready = true, true
	startX, startY := leader.character.X, leader.character.Y
	p := &party{members: []*Session{leader, follower}}
	leader.party, follower.party = p, p
	if err := s.worldCommand(context.Background(), leader, nativeWalkRequest(startX+500, startY)); err != nil {
		t.Fatal(err)
	}
	if err := s.advancePlayerWalk(leader, leader.walk.started.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if leader.character.X != startX+160 || follower.character.X != startX+160 {
		t.Fatal("party advanced to endpoint", leader.character.X, follower.character.X)
	}
	if err := s.autosaveSession(context.Background(), leader); err != nil {
		t.Fatal(err)
	}
	stored, err := s.Store.Characters(context.Background(), leader.account.ID)
	if err != nil || len(stored) != 1 || stored[0].X != startX+160 {
		t.Fatal("checkpoint saved destination instead of reached position", err)
	}
	// External relocation invalidates the old leg; it must never drag the player
	// back or traverse events from the previous scene.
	leader.character.X = 800
	if err := s.advancePlayerWalk(leader, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if leader.walk.active || leader.character.X != 800 {
		t.Fatal("stale leg survived relocation")
	}
}

func TestInstalledNativeRandomEncounterPopulation(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	s, cs, _ := worldFixture(t)
	c := cs[0]
	c.ready = true
	c.character.Map = 60001
	c.character.X, c.character.Y = 2202, 1395
	s.Assets, s.World = catalog, world.New(catalog)
	m, ok := s.World.Map(60001)
	if !ok {
		t.Fatal("North Island missing")
	}
	if err := s.randomEncounter(c, m); err != nil {
		t.Fatal(err)
	}
	if c.battle == nil || c.battle.encounter != 0 || c.event != nil {
		t.Fatal("native North Island population could not roll a random encounter")
	}
	t.Cleanup(func() { s.worldMu.Lock(); defer s.worldMu.Unlock(); s.abandonBattle(c) })
}

package server

import (
	"bytes"
	"context"
	"testing"
	"time"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/world"
)

// fieldFixture puts Alice on field map 30001 with a one-HP wild boar (click 4) beside
// her and a second one (click 5) far away.
func fieldFixture(t *testing.T) (*Server, *Session, *captureConn) {
	battleSleep, turnTimeout = func(time.Duration) {}, time.Hour
	t.Cleanup(func() { battleSleep, turnTimeout = time.Sleep, 30*time.Second })
	s, players, wires := worldFixture(t)
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
	// A clicked monster has at least 50 HP, so attack until it falls.
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

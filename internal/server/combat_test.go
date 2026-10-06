package server

import (
	"bytes"
	"context"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
)

// battleFixture: NPC 5 next to Alice fights monster template 10500 (level 5, HP hp)
// and has victory (result 1) and defeat (result 2) callback branches.
func battleFixture(t *testing.T, hp uint32) (*Server, *Session, *captureConn) {
	battleSleep, turnTimeout = func(time.Duration) {}, time.Hour
	t.Cleanup(func() { battleSleep, turnTimeout = time.Sleep, 30*time.Second })
	s, players, wires := worldFixture(t)
	fight := evOp(2, 6, 10500, 0, 5, hp<<8)
	fight.Data[7], fight.Data[8] = 0, 0 // Value packs HP in dword1; dialog4 high byte 0.
	copy(fight.Data[9:13], []byte{byte(hp), byte(hp >> 8), byte(hp >> 16), byte(hp >> 24)})
	ev := assets.Event{ClickID: 1, Branches: []assets.Branch{
		{Index: 1, Condition: evCond(0, 0, 0, 0, 0, 0), Operations: []assets.Operation{evOp(1, 2, 5, 0, 10001, 0), fight}},
		{Index: 2, Condition: evCond(4, 10500, 1, 0, 0, 0), Operations: []assets.Operation{evOp(1, 2, 5, 0, 10002, 0)}},
		{Index: 3, Condition: evCond(4, 10500, 2, 0, 0, 0), Operations: []assets.Operation{evOp(1, 2, 5, 0, 10003, 0)}},
	}}
	s.Assets.Maps[10017] = assets.Map{ID: 10017, NPCs: []assets.MapNPC{{ClickID: 5, Flags: 1, X: 1050, Y: 1080, Events: []byte{1}}}, Events: []assets.Event{ev}}
	s.Assets.NPCs = map[uint16]assets.NPC{10500: {ID: 10500, Name: "Slime"}}
	s.World = world.New(s.Assets)
	c := players[0]
	ctx := context.Background()
	for _, p := range [][]byte{{12, 1}, {20, 1, 5, 0}, {20, 6}} {
		if err := s.worldCommand(ctx, c, p); err != nil {
			t.Fatal(err)
		}
	}
	return s, c, wires[0]
}

// settle waits for the background animation player to release the battle.
func settle(t *testing.T, s *Server, c *Session) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; {
		s.worldMu.Lock()
		done := c.battle == nil || (!c.battle.b.Processing && !c.battle.b.Finished)
		s.worldMu.Unlock()
		if done {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("battle did not settle")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestQuestBattleVictory(t *testing.T) {
	s, c, wire := battleFixture(t, 1)
	got := wire.packets(t)
	if !contains(got, []byte{20, 12}) || !contains(got, battleState(c.character.ID, true)) || !contains(got, []byte{52, 1}) || c.battle == nil {
		t.Fatal("battle start", got)
	}
	ctx := context.Background()
	// Movement and portals are ignored during battle.
	if err := s.worldCommand(ctx, c, protocol.Builder{6, 1, 0}.U16(1).U16(1)); err != nil || c.character.X != 1042 {
		t.Fatal("moved during battle", err)
	}
	if err := s.worldCommand(ctx, c, []byte{50, 1, 4, 2, 2, 2}); err != nil {
		t.Fatal(err)
	}
	settle(t, s, c)
	got = wire.packets(t)
	if !contains(got, []byte{53, 5, 4, 2}) || !contains(got, []byte{53, 3, 2, 2}) || !contains(got, []byte{11, 12, 1}) || !contains(got, battleState(c.character.ID, false)) {
		t.Fatal("victory", got)
	}
	// The victory callback branch speaks next.
	if !bytes.Equal(got[len(got)-1], eventFrame(1, 3, 5, 1, 0, 10002, 1, 2)) || c.battle != nil || c.event == nil {
		t.Fatal("victory branch", got[len(got)-1])
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	// Level 5 grants 75 EXP (level 3 at 81 total) and 40 gold.
	if err != nil || chars[0].EXP != 81 || chars[0].Level != 3 || chars[0].Gold != 40 || chars[0].StatPoints != 6 {
		t.Fatal("rewards", chars[0].EXP, chars[0].Level, chars[0].Gold, err)
	}
}

func TestQuestBattleDefeat(t *testing.T) {
	s, c, wire := battleFixture(t, 50000)
	wire.Reset()
	s.worldMu.Lock()
	c.battle.members[0].self.HP = 1
	s.worldMu.Unlock()
	if err := s.worldCommand(context.Background(), c, []byte{50, 4, 4, 2, 0, 0}); err != nil {
		t.Fatal(err)
	}
	settle(t, s, c)
	got := wire.packets(t)
	if !bytes.Equal(got[len(got)-1], eventFrame(1, 3, 5, 1, 0, 10003, 1, 3)) || c.character.HP < 10 {
		t.Fatal("defeat branch", got[len(got)-1], c.character.HP)
	}
}

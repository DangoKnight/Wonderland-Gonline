package server

import (
	"bytes"
	"context"
	"testing"
	"time"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

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
		!bytes.Equal(got[0], vitalPacket(game.StatCurrentHP, hp)) || !bytes.Equal(got[1], vitalPacket(game.StatCurrentSP, sp)) {
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

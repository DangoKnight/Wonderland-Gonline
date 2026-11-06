package server

import (
	"context"
	"testing"
)

func TestBattlePetSwitchRacesTurnTimeout(t *testing.T) {
	for i := 0; i < 8; i++ {
		t.Run(string(rune('a'+i)), func(t *testing.T) {
			s, c, _ := battlePetFixture(t)
			run := c.battle
			done := make(chan error, 2)
			go func() { done <- s.worldCommand(context.Background(), c, []byte{19, 1, 6, 41, 0, 0}) }()
			go func() {
				s.worldMu.Lock()
				run.b.Timeout()
				s.tryRound(run)
				s.worldMu.Unlock()
				done <- nil
			}()
			for range 2 {
				if err := <-done; err != nil {
					t.Fatal(err)
				}
			}
			settle(t, s, c)
			s.worldMu.Lock()
			m := run.member(c)
			if m.pet == nil || c.character.ActivePet != m.pet.ID {
				t.Error("racing selection split saved mode and battle roster")
			}
			// With native AC19, the request may acquire the lock after playback
			// has already opened the next turn. That is a valid new selection.
			if len(run.petChanges) != 0 && (!run.b.HasCommand(m.self) || !run.b.HasCommand(m.pet)) {
				t.Error("next-turn switch did not consume both commands")
			}
			s.worldMu.Unlock()
		})
	}
}

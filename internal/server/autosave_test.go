package server

import (
	"context"
	"sync"
	"testing"
	"time"
	"wonderland-go/internal/game"
)

func setAutosaveBaseline(c *Session) {
	baseline := c.character.Clone()
	c.autosaveBaseline = &baseline
}

func TestAutosaveActiveWarpAndFailureIsolation(t *testing.T) {
	s, players, wires := worldFixture(t)
	for _, c := range players {
		setAutosaveBaseline(c)
	}
	s.world[players[0].info.ID] = players[0]
	s.world[players[1].info.ID] = players[1]
	s.friendSessions = map[uint32]*Session{players[0].character.ID: players[0], players[2].character.ID: players[2]}
	// Warping characters remain in the presence map while absent from s.world.
	players[2].warped = true
	players[0].character.X = 1200
	players[1].character.X = 1300
	players[2].character.X = 1400
	// A newer durable reward conflicts with player one's unsaved movement only.
	if err := s.Store.UpdateCharacter(context.Background(), players[0].account.ID, players[0].character.ID, func(c *game.Character) error { c.Gold = 7; return nil }); err != nil {
		t.Fatal(err)
	}
	s.autosaveCharacters(context.Background())
	for i, c := range players {
		chars, err := s.Store.Characters(context.Background(), c.account.ID)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			if chars[0].Gold != 7 || chars[0].X == 1200 || c.autosaveBaseline.X == 1200 {
				t.Fatal("conflict overwrote state or advanced checkpoint")
			}
		} else if chars[0].X != c.character.X || c.autosaveBaseline.X != c.character.X {
			t.Fatal("other player's save lost", chars)
		}
		if wires[i].Len() != 0 {
			t.Fatal("autosave sent unsolicited packets")
		}
	}
	players[1].character.Y = 1500
	s.leaveWorld(players[1])
	chars, err := s.Store.Characters(context.Background(), players[1].account.ID)
	if err != nil || chars[0].Y != 1500 || players[1].autosaveBaseline != nil {
		t.Fatal("disconnect lost pending state", chars, err)
	}
}

func TestAutosaveWorkerAndSerializedMovement(t *testing.T) {
	s, players, _ := worldFixture(t)
	c := players[0]
	setAutosaveBaseline(c)
	s.world[c.info.ID] = c
	c.ready = true
	c.character.X = 1200
	s.Config.CharacterSaveSeconds = 1
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { s.runAutosave(ctx); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		chars, err := s.Store.Characters(context.Background(), c.account.ID)
		if err != nil {
			t.Fatal(err)
		}
		if chars[0].X == 1200 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker did not save pending movement")
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker ignored cancellation")
	}
	// World ownership serializes checkpoint reads against gameplay mutation.
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); s.autosaveCharacters(context.Background()) }()
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.worldCommand(context.Background(), c, []byte{6, 1, 2, 0xb0, 4, 0x14, 5}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	s.autosaveCharacters(context.Background())
	chars, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || chars[0].X != 1200 || chars[0].Y != 1300 {
		t.Fatal(chars, err)
	}
}

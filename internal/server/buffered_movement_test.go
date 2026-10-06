package server

import (
	"context"
	"database/sql"
	"testing"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
)

func TestBufferedMovementPurchaseAndDisconnect(t *testing.T) {
	s, c, _, path := mallFixture(t)
	setAutosaveBaseline(c)
	s.world[c.info.ID] = c
	old := c.character.Clone()
	// Walking succeeds even while SQL rejects all character writes.
	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err = raw.Exec("CREATE TRIGGER reject_walk BEFORE UPDATE ON character_state BEGIN SELECT RAISE(ABORT,'unexpected per-packet write'); END"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		if err = s.worldCommand(context.Background(), c, protocol.Builder{6, 1, 2}.U16(uint16(1100+i)).U16(1200)); err != nil {
			t.Fatal(err)
		}
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved[0].X != old.X || saved[0].Y != old.Y {
		t.Fatal("walking wrote SQL")
	}
	if err = s.autosaveSession(context.Background(), c); err == nil {
		t.Fatal("failed checkpoint not reported")
	}
	if c.autosaveBaseline.X != old.X {
		t.Fatal("failed checkpoint advanced baseline")
	}
	if _, err = raw.Exec("DROP TRIGGER reject_walk"); err != nil {
		t.Fatal(err)
	}
	x, y := c.character.X, c.character.Y
	tradeDo(t, s, c, mallCart(1, mallCartRow{item: 32176, category: 4, quantity: 1, order: 12}))
	if c.character.X != x || c.character.Y != y {
		t.Fatal("purchase rewound buffered walking")
	}
	balances, err := s.Store.MallBalances(context.Background(), c.account.ID)
	if err != nil || balances.Points != 90 {
		t.Fatal(balances, err)
	}
	saved, err = s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].Bag != c.character.Bag {
		t.Fatal("purchase not durable", err)
	}
	if saved[0].X != old.X {
		t.Fatal("purchase accidentally flushed walking")
	}
	bag := c.character.Bag
	s.leaveWorld(c)
	saved, err = s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].X != x || saved[0].Y != y || saved[0].Bag != bag {
		t.Fatal("disconnect lost position or committed inventory", err)
	}
}

func TestBufferedMovementWarpDoesNotRestoreOldPosition(t *testing.T) {
	s, players, _ := worldFixture(t)
	c := players[0]
	setAutosaveBaseline(c)
	c.character.X, c.character.Y = 123, 456
	s.Assets.Maps[20000] = s.Assets.Maps[c.character.Map]
	if err := s.teleport(context.Background(), c, world.Destination{Map: 20000, X: 500, Y: 600}, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.autosaveSession(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].Map != 20000 || saved[0].X != 500 || saved[0].Y != 600 {
		t.Fatal(saved, err)
	}
}

func TestSavedTransactionPreservesWalkingButAdoptsExplicitPosition(t *testing.T) {
	s, players, _ := worldFixture(t)
	c := players[0]
	setAutosaveBaseline(c)
	baseline := c.character.Clone()
	c.character.X = 123
	reward := baseline.Clone()
	reward.Gold = 7
	s.adoptSavedCharacter(c, reward)
	if c.character.X != 123 || c.character.Gold != 7 || c.autosaveBaseline.X != baseline.X {
		t.Fatal("invalid cache merge")
	}
	warp := reward.Clone()
	warp.X = 789
	s.adoptSavedCharacter(c, warp)
	if c.character.X != 789 || c.autosaveBaseline.X != 789 {
		t.Fatal("committed location ignored")
	}
}

func TestBufferedMovementTradePreservesBothPlayers(t *testing.T) {
	s, players, wires := tradeFixture(t)
	for i, c := range players[:2] {
		tradeDo(t, s, c, protocol.Builder{6, 1, 0}.U16(uint16(1100+i*10)).U16(1200))
	}
	openTrade(t, s, players, wires)
	a, b := players[0], players[1]
	tradeDo(t, s, a, []byte{25, 3, 1, 1, 3})
	tradeDo(t, s, b, protocol.Builder{25, 4}.U32(50))
	tradeDo(t, s, a, []byte{25, 6})
	tradeDo(t, s, b, []byte{25, 6})
	for i, c := range players[:2] {
		if c.character.X != uint16(1100+i*10) || c.character.Gold != 150 {
			t.Fatal("trade rewound position or failed to transfer", i)
		}
	}
	s.autosaveCharacters(context.Background())
	saved := tradeStored(t, s, players)
	if saved[0].X != 1100 || saved[1].X != 1110 || saved[0].Gold != 150 || saved[1].Gold != 150 || saved[0].Bag[0].Count != 7 || saved[1].Bag[1].Count != 3 {
		t.Fatal("checkpoint lost trade or position", saved)
	}
}

func TestCleanCheckpointSkipsSQL(t *testing.T) {
	s, players, _ := worldFixture(t)
	c := players[0]
	setAutosaveBaseline(c)
	// A closed store makes any attempted SQL transaction fail.
	if err := s.Store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.autosaveSession(context.Background(), c); err != nil {
		t.Fatal("clean session queried SQL", err)
	}
}

func TestBufferedMovementHealKeepsPositionDirty(t *testing.T) {
	s, players, _ := worldFixture(t)
	c := players[0]
	c.ready = true
	s.world[c.info.ID] = c
	if err := s.worldCommand(context.Background(), c, protocol.Builder{6, 1, 0}.U16(123).U16(456)); err != nil {
		t.Fatal(err)
	}
	s.worldMu.Lock()
	err := s.heal(context.Background(), c, []string{"heal"})
	s.worldMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	s.autosaveCharacters(context.Background())
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].X != 123 || saved[0].Y != 456 {
		t.Fatal("vitals-only commit hid dirty walking", saved, err)
	}
}

func TestBufferedMovementFullCommitAdoptsCheckpointCoordinates(t *testing.T) {
	s, players, _ := worldFixture(t)
	c := players[0]
	setAutosaveBaseline(c)
	destination := c.character.Clone()
	c.character.X = 123
	if err := s.commitState(context.Background(), c, destination); err != nil {
		t.Fatal(err)
	}
	if c.character.X != destination.X || c.autosaveBaseline.X != destination.X {
		t.Fatal("explicit saved destination was replaced by buffered walking")
	}
}

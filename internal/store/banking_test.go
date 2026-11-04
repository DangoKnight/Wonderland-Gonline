package store

import (
	"context"
	"sync"
	"testing"
	"wonderland-gonline/internal/game"
)

func TestBankBalancesPersistAcrossReopenWithoutSchemaUpgrade(t *testing.T) {
	db, refs := pairFixture(t)
	ctx := context.Background()
	if err := db.UpdateCharacter(ctx, refs[0].Account, refs[0].ID, func(c *game.Character) error {
		if !c.DepositGoldToBank(75) {
			t.Fatal("deposit rejected")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var sequence int
	var name, path string
	if err := db.db.QueryRow("PRAGMA database_list").Scan(&sequence, &name, &path); err != nil {
		t.Fatal(err)
	}
	db.Close()
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	characters, err := reopened.Characters(ctx, refs[0].Account)
	if err != nil || len(characters) != 1 || characters[0].Gold != 25 || characters[0].BankGold != 75 {
		t.Fatal(characters, err)
	}
	others, err := reopened.Characters(ctx, refs[1].Account)
	if err != nil || others[0].BankGold != 0 || others[0].Gold != 100 {
		t.Fatal("bank crossed character ownership", others, err)
	}
	var version int
	if err := reopened.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != schemaVersion {
		t.Fatal(version, err)
	}
	// Existing identities and unrelated state survive bank updates.
	if characters[0].Name != "Alice" || characters[0].ID != refs[0].ID {
		t.Fatal(characters[0])
	}
}

func TestBankTransferAcrossConnectionsAndOwnership(t *testing.T) {
	db, refs := pairFixture(t)
	ctx := context.Background()
	if err := db.UpdateCharacter(ctx, refs[0].Account, refs[0].ID, func(c *game.Character) error { c.Gold = 100; c.BankGold = 0; return nil }); err != nil {
		t.Fatal(err)
	}
	var path string
	if err := db.db.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&path); err != nil {
		t.Fatal(err)
	}
	other, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	wrong := CharacterRef{Account: refs[1].Account, ID: refs[0].ID}
	if _, changed, err := other.TransferBankGold(ctx, wrong, 100, true); err == nil || changed {
		t.Fatal("foreign account changed the wallet", changed, err)
	}
	var wg sync.WaitGroup
	results := make(chan bool, 12)
	errors := make(chan error, 12)
	for i := range 12 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			bank := db
			if i%2 != 0 {
				bank = other
			}
			_, changed, err := bank.TransferBankGold(ctx, refs[0], 100, true)
			results <- changed
			errors <- err
		}(i)
	}
	wg.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	wins := 0
	for changed := range results {
		if changed {
			wins++
		}
	}
	if wins != 1 {
		t.Fatal("stale/concurrent requests spent the same gold", wins)
	}
	saved, err := db.Characters(ctx, refs[0].Account)
	if err != nil || saved[0].Gold != 0 || saved[0].BankGold != 100 {
		t.Fatal("currency was not conserved", saved, err)
	}
}

func TestBankTransferRollbackDoesNotDebitEitherBalance(t *testing.T) {
	db, refs := pairFixture(t)
	ctx := context.Background()
	if err := db.UpdateCharacter(ctx, refs[0].Account, refs[0].ID, func(c *game.Character) error { c.Gold = 100; c.BankGold = 20; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`CREATE TRIGGER reject_bank_write BEFORE UPDATE OF bank_gold ON character_state BEGIN SELECT RAISE(ABORT,'injected bank failure'); END;`); err != nil {
		t.Fatal(err)
	}
	if _, changed, err := db.TransferBankGold(ctx, refs[0], 50, true); err == nil || changed {
		t.Fatal("SQL failure accepted", changed, err)
	}
	saved, err := db.Characters(ctx, refs[0].Account)
	if err != nil || saved[0].Gold != 100 || saved[0].BankGold != 20 {
		t.Fatal("failed transfer changed funds", saved, err)
	}
}

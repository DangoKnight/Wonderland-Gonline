package store

import (
	"context"
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

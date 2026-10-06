package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/legacyimport"
	"wonderland-gonline/internal/store"
)

func TestImportProcedureReviewVerifyAndCleanup(t *testing.T) {
	plan := legacyimport.Plan{Accounts: []store.LegacyAccount{{Account: store.Account{ID: 7, Username: "legacy"}, Password: "password", Characters: []game.Character{{ID: 10007, Slot: 1, Name: "Hero", Map: 12000, Level: 1, Quests: map[uint32]game.Quest{}}}}}, Report: legacyimport.Report{Accounts: 1, Characters: 1, Unmapped: map[string]int64{"Archive": 1}}}
	out := filepath.Join(t.TempDir(), "imported")
	if err := apply(plan, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := apply(plan, out, ""); err == nil {
		t.Fatal("unreviewed data omitted")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("unreviewed output created")
	}
	if err := apply(plan, out, "archive"); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(out, "wonderland.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Authenticate(context.Background(), "legacy", "password"); err != nil {
		t.Fatal(err)
	}
	if err := apply(plan, out, "Archive"); err == nil {
		t.Fatal("existing output overwritten")
	}
	failed := filepath.Join(t.TempDir(), "failed")
	plan.Accounts[0].Password = "bad"
	if err := apply(plan, failed, "Archive"); err == nil {
		t.Fatal("invalid credentials imported")
	}
	if _, err := os.Stat(failed); !os.IsNotExist(err) {
		t.Fatal("partial output survived")
	}
}

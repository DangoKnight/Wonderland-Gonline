package store

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"wonderland-gonline/internal/game"
)

func economyStoreFixture(t *testing.T) (*Store, [2]CharacterRef, map[uint16]game.ItemDefinition) {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "economy.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	var refs [2]CharacterRef
	for i, name := range []string{"Sender", "Receiver"} {
		a, err := s.Register(context.Background(), name, "secret12", "")
		if err != nil {
			t.Fatal(err)
		}
		c := game.Character{ID: a.CharacterID(1), Slot: 1, Name: name, Level: 30, Gold: 100000}
		c.Bag[0] = game.Item{ID: 32176, Count: 10, Damage: 3}
		c.Bag[0].Metadata[4] = 99
		if err = s.CreateCharacter(context.Background(), a, c); err != nil {
			t.Fatal(err)
		}
		refs[i] = CharacterRef{Account: a.ID, ID: c.ID}
	}
	return s, refs, map[uint16]game.ItemDefinition{32176: {ID: 32176, Type: 23}}
}
func TestParcelEscrowOwnershipRecoveryAndConcurrentClaim(t *testing.T) {
	s, refs, items := economyStoreFixture(t)
	ctx := context.Background()
	next, err := s.SendParcel(ctx, refs[0], refs[1].ID, "Subject", "Body", 100, 1, 3, items)
	if err != nil || next.Gold != 99900 || next.Bag[0].Count != 7 {
		t.Fatal(next, err)
	}
	rows, err := s.Parcels(ctx, refs[1].ID)
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	id := rows[0].ID
	if _, err = s.ReadParcel(ctx, refs[0].ID, id); err == nil {
		t.Fatal("cross-owner read")
	}
	if err = s.DeleteParcel(ctx, refs[1].ID, id); !errors.Is(err, ErrParcelPending) {
		t.Fatal("unclaimed mail discarded", err)
	}
	if err = s.UpdateCharacter(ctx, refs[1].Account, refs[1].ID, func(c *game.Character) error {
		for i := range c.Bag {
			c.Bag[i] = game.Item{ID: 32176, Count: 50, Damage: 8}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.ClaimParcel(ctx, refs[1], id, items); !errors.Is(err, game.ErrInventoryFull) {
		t.Fatal("full bag claim", err)
	}
	rows, _ = s.Parcels(ctx, refs[1].ID)
	if rows[0].Claimed {
		t.Fatal("failed claim consumed escrow")
	}
	if err = s.UpdateCharacter(ctx, refs[1].Account, refs[1].ID, func(c *game.Character) error { c.Bag = game.Inventory{}; return nil }); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := s.ClaimParcel(ctx, refs[1], id, items); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	chars, err := s.Characters(ctx, refs[1].Account)
	if err != nil || chars[0].Gold != 100100 || chars[0].Bag[0].Count != 3 || chars[0].Bag[0].Metadata[4] != 99 || chars[0].Bag[0].Damage != 3 {
		t.Fatal("claim duplicated/lost metadata", chars, err)
	}
	if err = s.DeleteParcel(ctx, refs[1].ID, id); err != nil {
		t.Fatal(err)
	}
}
func TestMarriageRollbackAndMembershipUniqueness(t *testing.T) {
	s, refs, items := economyStoreFixture(t)
	ctx := context.Background()
	rings := [2]uint16{32176, 32176}
	if err := s.UpdateCharacter(ctx, refs[1].Account, refs[1].ID, func(c *game.Character) error {
		for i := range c.Bag {
			c.Bag[i] = game.Item{ID: 32176, Count: 50, Damage: 8}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Marry(ctx, refs, 30, 60000, rings, items); !errors.Is(err, game.ErrInventoryFull) {
		t.Fatal(err)
	}
	if row, err := s.Marriage(ctx, refs[0].ID); err != nil || row != nil {
		t.Fatal("partial relationship", row, err)
	}
	chars, _ := s.Characters(ctx, refs[0].Account)
	if chars[0].Gold != 100000 || chars[0].Bag[0].Count != 10 {
		t.Fatal("failed marriage charged")
	}
	if err := s.UpdateCharacter(ctx, refs[1].Account, refs[1].ID, func(c *game.Character) error { c.Bag = game.Inventory{}; return nil }); err != nil {
		t.Fatal(err)
	}
	next, _, err := s.Marry(ctx, refs, 30, 60000, rings, items)
	if err != nil || next[0].Gold != 40000 || next[1].Bag[0].ID != 32176 {
		t.Fatal(next, err)
	}
	if _, _, err = s.Marry(ctx, refs, 30, 60000, rings, items); !errors.Is(err, ErrAlreadyMarried) {
		t.Fatal("duplicate marriage", err)
	}
	if partner, err := s.Divorce(ctx, refs[1]); err != nil || partner != refs[0].ID {
		t.Fatal(partner, err)
	}
	if row, err := s.Marriage(ctx, refs[0].ID); err != nil || row != nil {
		t.Fatal("divorce not durable", row, err)
	}
}
func TestGuildAdminEditPreservesRanksAndIcon(t *testing.T) {
	s, refs, _ := economyStoreFixture(t)
	ctx := context.Background()
	if err := s.CreateGuild(ctx, refs[0].ID, "Guild"); err != nil {
		t.Fatal(err)
	}
	if err := s.JoinGuild(ctx, refs[0].ID, refs[1].ID); err != nil {
		t.Fatal(err)
	}
	rank, icon := GuildRankViceLeader, uint32(1234)
	if err := s.EditGuild(ctx, refs[0].ID, nil, &icon, refs[1].ID, &rank); err != nil {
		t.Fatal(err)
	}
	rows, err := s.AdminGuilds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rows[0].Notice = "Updated"
	if err = s.SaveAdminGuild(ctx, rows[0]); err != nil {
		t.Fatal(err)
	}
	guild, err := s.GuildForCharacter(ctx, refs[1].ID)
	if err != nil || guild.Icon != icon || guild.Roster[1].Rank != GuildRankViceLeader {
		t.Fatal("admin erased roles", guild, err)
	}
	if err = s.CreateGuild(ctx, refs[1].ID, "Other"); err == nil {
		t.Fatal("joined second guild")
	}
	if err = s.LeaveGuild(ctx, refs[1].ID, refs[0].ID); !errors.Is(err, ErrGuildPermission) {
		t.Fatal("member dismissed leader", err)
	}
}

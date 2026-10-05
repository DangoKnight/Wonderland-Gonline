package store

import (
	"context"
	"path/filepath"
	"testing"
	"wonderland-go/internal/game"
)

func TestSettingsPartyInvitesPreservingUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	a, err := s.Register(ctx, "settings", "password", "")
	if err != nil {
		t.Fatal(err)
	}
	c := game.Character{ID: a.CharacterID(1), Slot: 1, Name: "Settings", Level: 1, Gold: 123, Map: 10017, X: 44, Y: 55, Settings: &game.ClientSettings{PKAllowed: true, TradeAllowed: true, Channels: 31}}
	if err = s.CreateCharacter(ctx, a, c); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("ALTER TABLE character_state DROP COLUMN party_invites_blocked; PRAGMA user_version=16"); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	chars, err := s.Characters(ctx, a.ID)
	if err != nil || len(chars) != 1 {
		t.Fatal(chars, err)
	}
	c.Settings.PartyInvitesBlocked = true
	if CharacterVersion(chars[0]) != CharacterVersion(c) {
		t.Fatal("upgrade lost state or enabled previously blocked invitations")
	}
	// Native Joining Battle is independent: store invitation enablement alongside it.
	if err = s.UpdateCharacter(ctx, a.ID, c.ID, func(c *game.Character) error { c.Settings.PartyInvitesBlocked = false; return nil }); err != nil {
		t.Fatal(err)
	}
	chars, err = s.Characters(ctx, a.ID)
	if err != nil || chars[0].Preferences().JoinAllowed || chars[0].Preferences().PartyInvitesBlocked {
		t.Fatal(chars, err)
	}
}

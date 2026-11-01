package server

import (
	"bytes"
	"context"
	"testing"
)

func TestCompactCreationAuthenticatedOwnedSlotAndAllocation(t *testing.T) {
	s, c, wire, _ := mallFixture(t)
	ctx := context.Background()
	account := c.account
	c.character = nil
	c.slot = 0
	c.ready = false
	payload := []byte{63, 3, 2, 6, 'S', 'e', 'c', 'o', 'n', 'd', 1, 3, 1, 1, 1, 1, 1}
	invalid := append([]byte(nil), payload...)
	invalid[len(invalid)-1] = 99
	if err := s.dispatch(ctx, c, invalid); err != nil || c.character != nil {
		t.Fatal("invalid allocation", err)
	}
	if got := wire.packets(t); len(got) != 1 || !bytes.Equal(got[0], []byte{1, 3, 1}) {
		t.Fatal(got)
	}
	if err := s.dispatch(ctx, c, payload); err != nil || c.character == nil || c.character.Slot != 2 || c.character.Name != "Second" {
		t.Fatal(c.character, err)
	}
	if got := wire.packets(t); len(got) == 0 || !bytes.Equal(got[0], []byte{1, 3, 0}) {
		t.Fatal("compact creation ACK", got)
	}
	chars, err := s.Store.Characters(ctx, account.ID)
	if err != nil || len(chars) != 2 {
		t.Fatal("owned character not committed", err)
	}
	if err = s.dispatch(ctx, c, payload); err == nil {
		t.Fatal("live session recreated character")
	}
}

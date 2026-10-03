package server

import (
	"bytes"
	"context"
	"testing"

	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

func TestLoginReturnReleasesAccountAndCreationName(t *testing.T) {
	s, players, wires := worldFixture(t)
	c, wire := players[0], wires[0]
	c.character = nil
	ctx := context.Background()
	old := c.account
	s.accounts[old.ID] = c.info.ID
	s.sessions[c.info.ID] = c
	c.info.Username = old.Username
	c.gmLevel.Store(1)
	if _, err := s.reserveName(ctx, c, "Reserved"); err != nil {
		t.Fatal(err)
	}
	c.slot = 2
	tradeDo(t, s, c, []byte{63, 0})
	if wire.Len() != 0 || c.account.ID != 0 || c.slot != 0 || c.pendingName != "" || c.gmLevel.Load() != 0 || c.info.Username != "" {
		t.Fatal("return retained authentication or emitted a reply")
	}
	if _, held := s.accounts[old.ID]; held {
		t.Fatal("old account reservation retained")
	}
	if _, held := s.names["reserved"]; held {
		t.Fatal("creation name retained")
	}
	tradeDo(t, s, c, loginPayload(old.Username))
	if c.account.ID != old.ID || !contains(wire.packets(t), protocol.Builder{63, 2}.U32(old.UserID())) {
		t.Fatal("same-connection reauthentication failed")
	}
	tradeDo(t, s, c, []byte{63, 0})
	tradeDo(t, s, c, loginPayload(players[1].account.Username))
	if c.account.ID != players[1].account.ID {
		t.Fatal("previous account leaked into new login")
	}
}
func TestLoginReturnRejectsWorldAndMalformedRequests(t *testing.T) {
	s, players, wires := worldFixture(t)
	c, wire := players[0], wires[0]
	old := c.account.ID
	if err := s.dispatch(context.Background(), c, []byte{63, 0}); err == nil {
		t.Fatal("world player returned without world cleanup")
	}
	if c.account.ID != old {
		t.Fatal("rejected request released account")
	}
	c.character = nil
	for _, p := range [][]byte{{63, 0, 1}, {63, 2}, {63, 2, 1, 0}} {
		if err := s.dispatch(context.Background(), c, p); err == nil {
			t.Fatal("malformed login transition accepted", p)
		}
	}
	if wire.Len() != 0 || c.account.ID != old {
		t.Fatal("malformed transition mutated state")
	}
}
func TestLoginNativeTailIsFullyDecodedBeforeAuthentication(t *testing.T) {
	s, players, wires := worldFixture(t)
	c, wire := players[0], wires[0]
	name, accountID := c.account.Username, c.account.ID
	c.account.ID = 0
	c.character = nil
	// Native token is a key plus XOR-encoded ASCII item-file size.
	p, _ := protocol.Builder{63, 4}.U16(1211).String(name)
	p, _ = p.String("password")
	for _, tail := range [][]byte{{0}, {2, 0, 1}, {0, 0, 99}} {
		if err := s.dispatch(context.Background(), c, append(append([]byte{}, p...), tail...)); err == nil {
			t.Fatal("malformed native tail accepted", tail)
		}
		if c.account.ID != 0 || wire.Len() != 0 {
			t.Fatal("malformed tail authenticated or emitted packets")
		}
	}
	tradeDo(t, s, c, append(p, 3, 0x55, '1'^0x55, '2'^0x55, '3'^0x55))
	if c.account.ID != accountID || wire.Len() == 0 {
		t.Fatal("native login tail rejected")
	}
}
func TestLoginRosterUsesNativeSelectionLayout(t *testing.T) {
	s, players, wires := worldFixture(t)
	c, wire := players[0], wires[0]
	name := c.account.Username
	if err := s.Store.UpdateCharacter(context.Background(), c.account.ID, c.character.ID, func(char *game.Character) error {
		char.HP = 12
		char.MaxHP = 100
		char.SP = 8
		char.MaxSP = 50
		for i := range char.Equipment {
			char.Equipment[i] = game.Item{ID: uint16(0x1001 + i), Count: 1}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	c.account.ID = 0
	c.character = nil
	tradeDo(t, s, c, loginPayload(name))
	packets := wire.packets(t)
	if len(packets) != 3 || !bytes.Equal(packets[1][:2], []byte{63, 1}) {
		t.Fatal(packets)
	}
	record := packets[1][2:]
	n := int(record[1])
	if len(record) != 54+n || !bytes.Equal(record[n+40:n+44], []byte{0, 0, 1, 0x10}) {
		t.Fatal("rebirth/job/equipment misaligned", record)
	}
}

// TestCancelCreationReleasesNameAndSlot covers 63/3, sent when the client's
// creation wizard is cancelled on its first step (0x2d8dcd).
func TestCancelCreationReleasesNameAndSlot(t *testing.T) {
	s, players, wires := worldFixture(t)
	c, wire := players[0], wires[0]
	c.character = nil
	if _, err := s.reserveName(context.Background(), c, "Reserved"); err != nil {
		t.Fatal(err)
	}
	c.slot = 2
	old := c.account.ID
	tradeDo(t, s, c, []byte{63, 3})
	if wire.Len() != 0 || c.slot != 0 || c.pendingName != "" || c.account.ID != old {
		t.Fatal("cancel kept the slot or name, logged out, or replied")
	}
	if _, held := s.names["reserved"]; held {
		t.Fatal("creation name retained")
	}
	for _, p := range [][]byte{{63, 3, 0}} {
		if err := s.dispatch(context.Background(), c, p); err == nil {
			t.Fatal("malformed cancel accepted", p)
		}
	}
}

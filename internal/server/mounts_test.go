package server

import (
	"bytes"
	"context"
	"testing"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

func mountFixture(t *testing.T) (*Server, *Session, []*captureConn) {
	s, players, wires := petFixture(t)
	c := players[0]
	next := c.character.Clone()
	next.Pets = []game.Pet{{ID: 14156, Slot: 2, Name: "Xaolan", Amity: 80}, {ID: 12032, Slot: 3, Name: "Robinson", Amity: 80}}
	c.pets.register(14156)
	c.pets.register(12032)
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	return s, c, wires
}

func TestMountPacketsPersistenceAndRest(t *testing.T) {
	s, c, wires := mountFixture(t)
	ctx := context.Background()
	command := protocol.Builder{15, 11, 2}.U32(12178)
	if err := s.worldCommand(ctx, c, command); err != nil {
		t.Fatal(err)
	}
	want := mountPacket(c.character.ID, 12178, 3)
	if len(want) != 37 || !bytes.Equal(want[:11], protocol.Builder{15, 16, 3}.U32(c.character.ID).U32(12178)) {
		t.Fatal("mount wire layout", want)
	}
	for _, b := range want[11:] {
		if b != 0 {
			t.Fatal("reserved bytes")
		}
	}
	if !contains(wires[0].packets(t), want) || !contains(wires[1].packets(t), want) {
		t.Fatal("mount not replicated")
	}
	stored, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || stored[0].ActiveMount != 12178 {
		t.Fatal("mount not durable", err)
	}
	if !contains(s.peerPetPackets(c.character), mountPacket(c.character.ID, 12178, 1)) {
		t.Fatal("peer snapshot mount missing")
	}
	if !contains(s.rosterPackets(c.character, newPetRoster()), want) {
		t.Fatal("owner snapshot mount missing")
	}
	if err = s.worldCommand(ctx, c, command); err != nil || wires[0].Len() != 0 {
		t.Fatal("mount replay", err)
	}
	if err = s.worldCommand(ctx, c, protocol.Builder{15, 12, 1}.U32(14156)); err != nil || c.character.ActiveMount != 12178 || wires[0].Len() != 0 {
		t.Fatal("stale rest unmounted another pet", err)
	}
	if err = s.worldCommand(ctx, c, protocol.Builder{15, 12, 2}.U32(12032)); err != nil || c.character.ActiveMount != 0 {
		t.Fatal("rest failed", err)
	}
	if !contains(wires[0].packets(t), protocol.Builder{15, 17}.U32(c.character.ID)) {
		t.Fatal("rest wire")
	}
	stored, err = s.Store.Characters(ctx, c.account.ID)
	if err != nil || stored[0].ActiveMount != 0 {
		t.Fatal("rest not durable", err)
	}
}

func TestMountRefusalsAndSaveFailure(t *testing.T) {
	s, c, wires := mountFixture(t)
	ctx := context.Background()
	for _, command := range [][]byte{protocol.Builder{15, 11, 1}.U32(12032), protocol.Builder{15, 11, 2}.U32(99999)} {
		if err := s.worldCommand(ctx, c, command); err != nil || c.character.ActiveMount != 0 || wires[0].Len() != 0 || wires[1].Len() != 0 {
			t.Fatal("invalid mount accepted", err)
		}
	}
	command := protocol.Builder{15, 11, 2}.U32(12032)
	for i := 2; i < len(command); i++ {
		if err := s.mountCommand(ctx, c, command[:i]); err == nil {
			t.Fatal("truncation accepted", i)
		}
	}
	s.Store.Close()
	if err := s.worldCommand(ctx, c, command); err == nil || c.character.ActiveMount != 0 || wires[0].Len() != 0 || wires[1].Len() != 0 {
		t.Fatal("failed mount save published state", err)
	}
}

func TestMountedPetHotelAndDismissCleanup(t *testing.T) {
	for _, hotel := range []bool{false, true} {
		s, c, wires := mountFixture(t)
		ctx := context.Background()
		if err := s.worldCommand(ctx, c, protocol.Builder{15, 11, 2}.U32(12032)); err != nil {
			t.Fatal(err)
		}
		for _, w := range wires {
			w.Reset()
		}
		if hotel {
			if err := s.hotelCommand(ctx, c, []byte{31, 3, 2}); err != nil {
				t.Fatal(err)
			}
		} else {
			if _, err := s.dismiss(ctx, c, 12032); err != nil {
				t.Fatal(err)
			}
		}
		if c.character.ActiveMount != 0 || !contains(wires[0].packets(t), protocol.Builder{15, 17}.U32(c.character.ID)) || !contains(wires[1].packets(t), protocol.Builder{15, 17}.U32(c.character.ID)) {
			t.Fatal("removed mount remained visible")
		}
	}
}

func TestMountRestoredOnWorldEntry(t *testing.T) {
	s, c, wires := mountFixture(t)
	ctx := context.Background()
	if err := s.worldCommand(ctx, c, protocol.Builder{15, 11, 2}.U32(12032)); err != nil {
		t.Fatal(err)
	}
	id := c.character.ID
	c.character = nil
	for _, w := range wires {
		w.Reset()
	}
	if err := s.dispatch(ctx, c, []byte{63, 2, 1}); err != nil {
		t.Fatal(err)
	}
	if c.character.ActiveMount != 12178 || !contains(wires[0].packets(t), mountPacket(id, 12178, 3)) {
		t.Fatal("mount not restored on login")
	}
}

func TestPetControlsReachWorldThroughDispatcher(t *testing.T) {
	s, c, wires := mountFixture(t)
	ctx := context.Background()
	if err := s.dispatch(ctx, c, protocol.Builder{15, 11, 1}.U32(14156)); err != nil {
		t.Fatal(err)
	}
	if c.character.ActiveMount != 14156 || !contains(wires[0].packets(t), mountPacket(c.character.ID, 14156, 2)) {
		t.Fatal("dispatcher did not route companion mount")
	}
	if err := s.dispatch(ctx, c, append([]byte{15, 6, 1}, []byte("New Name")...)); err != nil {
		t.Fatal(err)
	}
	if c.character.Pets[0].Name != "New Name" {
		t.Fatal("dispatcher did not route pet rename")
	}
	if err := s.dispatch(ctx, c, []byte{15, 2, 1}); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.character.Pet(14156); ok || c.character.ActiveMount != 0 {
		t.Fatal("dispatcher did not route release")
	}
}

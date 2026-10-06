package server

import (
	"context"
	"testing"
	"wonderland-gonline/internal/protocol"
)

func TestRenamePetRawASCIIAndReplication(t *testing.T) {
	s, c, wires := mountFixture(t)
	ctx := context.Background()
	c.character.ActivePet = 14156
	c.character.Pets[0].Battle = true
	command := append([]byte{15, 6, 1}, []byte("  My Companion Name That Is Long  \x00\x00")...)
	if err := s.worldCommand(ctx, c, command); err != nil {
		t.Fatal(err)
	}
	name := "My Companion Nam"
	if c.character.Pets[0].Name != name || !contains(wires[0].packets(t), petNamePacket(c.character.ID, 1, name)) || !contains(wires[1].packets(t), petMapPacket(c.character.ID, 14156, name)) {
		t.Fatal("rename wire or trimming", c.character.Pets[0].Name)
	}
	stored, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || stored[0].Pets[0].Name != name {
		t.Fatal("rename not durable", err)
	}
	if err = s.worldCommand(ctx, c, []byte{15, 6, 2, 'A', 255}); err != nil || c.character.Pets[1].Name != "A?" {
		t.Fatal("ASCII replacement", err)
	}
}

func TestReleaseMountedBattlePetIsPermanent(t *testing.T) {
	s, c, wires := mountFixture(t)
	ctx := context.Background()
	c.character.ActivePet = 12178
	c.character.Pets[1].Battle = true
	if err := s.worldCommand(ctx, c, protocol.Builder{15, 11, 2}.U32(12032)); err != nil {
		t.Fatal(err)
	}
	for _, w := range wires {
		w.Reset()
	}
	if err := s.worldCommand(ctx, c, []byte{15, 2, 2}); err != nil {
		t.Fatal(err)
	}
	stored, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || len(stored[0].Pets) != 1 || len(stored[0].ReservePets) != 0 || stored[0].ActiveMount != 0 || stored[0].ActivePet != 0 || c.pets.slot(12032) != 0 {
		t.Fatal("release not durable/permanent", err)
	}
	for _, wire := range wires[:2] {
		packets := wire.packets(t)
		if !contains(packets, protocol.Builder{15, 17}.U32(c.character.ID)) || !contains(packets, protocol.Builder{15, 2}.U32(c.character.ID).U8(2)) {
			t.Fatal("released pet not removed from client", packets)
		}
	}
}

func TestReleaseBattlePetPreservesAnotherMount(t *testing.T) {
	s, c, wires := mountFixture(t)
	ctx := context.Background()
	c.character.ActivePet = 14156
	c.character.Pets[0].Battle = true
	if err := s.worldCommand(ctx, c, protocol.Builder{15, 11, 2}.U32(12032)); err != nil {
		t.Fatal(err)
	}
	for _, w := range wires {
		w.Reset()
	}
	if err := s.worldCommand(ctx, c, []byte{15, 2, 1}); err != nil {
		t.Fatal(err)
	}
	if c.character.ActiveMount != 12178 {
		t.Fatal("another mount removed")
	}
	for _, wire := range wires[:2] {
		packets := wire.packets(t)
		if contains(packets, protocol.Builder{15, 17}.U32(c.character.ID)) || contains(packets, protocol.Builder{5, 8}.U32(c.character.ID).U32(0)) {
			t.Fatal("another mount's appearance erased")
		}
	}
}

func TestPetControlRefusalsAndSaveFailure(t *testing.T) {
	s, c, wires := mountFixture(t)
	ctx := context.Background()
	for _, command := range [][]byte{{15, 2, 0}, {15, 2, 4}, {15, 6, 0, 'A'}, {15, 6, 4, 'A'}, {15, 6, 1, ' ', ' '}, {15, 6, 1, 'A', 0, 'B'}} {
		if err := s.worldCommand(ctx, c, command); err != nil || len(c.character.Pets) != 2 || c.character.Pets[0].Name != "Xaolan" || wires[0].Len() != 0 || wires[1].Len() != 0 {
			t.Fatal("invalid pet control mutated state", command, err)
		}
	}
	for _, command := range [][]byte{{15, 2}, {15, 2, 1, 0}, {15, 6}, {15, 6, 1}} {
		if err := s.worldCommand(ctx, c, command); err == nil {
			t.Fatal("malformed pet control accepted", command)
		}
	}
	s.Store.Close()
	for _, command := range [][]byte{{15, 6, 1, 'A'}, {15, 2, 1}} {
		if err := s.worldCommand(ctx, c, command); err == nil || len(c.character.Pets) != 2 || c.pets.slot(14156) != 1 || c.character.Pets[0].Name != "Xaolan" || wires[0].Len() != 0 || wires[1].Len() != 0 {
			t.Fatal("failed pet control save published state", err)
		}
	}
}

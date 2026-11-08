package server

import (
	"context"
	"testing"

	"wonderland-gonline/internal/game"
)

func TestNativeTentManufactureStartWithPlacementTail(t *testing.T) {
	s, c, wire := nativeManufactureFixture(t)
	ctx := context.Background()
	c.character.TentReturn = &game.Location{Map: 12000, X: 10, Y: 20}
	if err := s.commit(ctx, c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	packet := []byte{64, 1, 1, 0, 12, 0, 1, 0, 0, 0, 0, 0, 0, 1, 10, 1, 0, 0, 0, 0, 0, 0, 0, 0}
	if err := s.dispatch(ctx, c, packet); err != nil {
		t.Fatal(err)
	}
	if c.manufacturing == nil || c.character.Bag[0].Count != 5 {
		t.Fatal("native start with tail did not consume the recipe", c.manufacturing)
	}
	if !contains(wire.packets(t), []byte{64, 1, 1, 177, 125, 10, 0, 0, 0, 1}) {
		t.Fatal("native start reply", wire.packets(t))
	}
}

func TestNativeTentManufactureControlPacket(t *testing.T) {
	s, c, wire := nativeManufactureFixture(t)
	ctx := context.Background()
	c.character.TentReturn = &game.Location{Map: 12000, X: 10, Y: 20}
	if err := s.commit(ctx, c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	if err := s.dispatch(ctx, c, []byte{64, 1, 1, 0, 12, 0, 1}); err != nil || c.manufacturing == nil {
		t.Fatal(err)
	}
	wire.Reset()
	if err := s.dispatch(ctx, c, []byte{64, 3, 1, 0, 3, 0, 0, 0, 0, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	if c.manufacturing != nil {
		t.Fatal("twelve-byte stop packet did not cancel the native job")
	}
	if c.character.Bag[0].Count != 10 {
		t.Fatalf("cancel refunded %d materials", c.character.Bag[0].Count)
	}
	if !contains(wire.packets(t), []byte{64, 3, 1}) {
		t.Fatal("cancel reply", wire.packets(t))
	}
}

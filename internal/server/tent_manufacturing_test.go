package server

import (
	"context"
	"testing"
	"time"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/store"
)

func nativeManufactureFixture(t *testing.T) (*Server, *Session, *captureConn) {
	t.Helper()
	s, c, wire, _ := mallFixture(t)
	ctx := context.Background()
	c.character.Map = game.TentMapID
	c.tentOwner = c.character.ID
	c.character.Bag = game.Inventory{}
	c.character.Bag[0] = game.Item{ID: 32176, Count: 10}
	s.Assets.Items[38004] = game.ItemDefinition{ID: 38004, Type: 29}
	if err := s.commit(ctx, c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.EnsureTent(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, 39062, 39064, []store.TentItem{{ItemID: 38004}}); err != nil {
		t.Fatal(err)
	}
	s.Assets.Manufacturing = map[uint16]assets.ManufacturingFormula{12: {ID: 12, ToolID: 38004, Output: assets.ManufacturingInput{ItemID: 32177, Count: 2}, DurationSeconds: 10, Inputs: [5]assets.ManufacturingInput{{ItemID: 32176, Count: 1}, {ItemID: 32176, Count: 1}, {ItemID: 32176, Count: 1}, {ItemID: 32176, Count: 1}, {ItemID: 32176, Count: 1}}}}
	s.world[c.info.ID] = c
	return s, c, wire
}
func TestNativeManufactureStartContinueStopAndDelivery(t *testing.T) {
	s, c, wire := nativeManufactureFixture(t)
	ctx := context.Background()
	if err := s.dispatch(ctx, c, []byte{64, 1, 0, 12, 0}); err != nil || c.manufacturing == nil || c.character.Bag[0].Count != 5 {
		t.Fatal(c.manufacturing, err)
	}
	packets := wire.packets(t)
	if !contains(packets, []byte{64, 1, 0, 177, 125, 10, 0, 0, 0, 1}) {
		t.Fatal("start layout", packets)
	}
	if err := s.dispatch(ctx, c, []byte{64, 1, 0, 12, 0}); err != nil || c.character.Bag[0].Count != 5 {
		t.Fatal("duplicate start debited again", err)
	}
	if err := s.dispatch(ctx, c, []byte{64, 3, 0}); err != nil || !c.manufacturing.Paused {
		t.Fatal(err)
	}
	if !contains(wire.packets(t), []byte{64, 4, 0}) {
		t.Fatal("stop ACK")
	}
	if err := s.dispatch(ctx, c, []byte{64, 2}); err != nil || c.manufacturing.Paused {
		t.Fatal(err)
	}
	due := time.UnixMilli(c.manufacturing.DueAt)
	// SQL-result adoption must not flush buffered walking during delivery.
	c.character.X++
	position := c.character.X
	if err := s.completeManufacture(ctx, c, due); err != nil || c.manufacturing != nil || c.character.Bag[1].Count != 2 || c.character.X != position {
		t.Fatal(c.character.Bag, err)
	}
	if !contains(wire.packets(t), []byte{64, 9}) {
		t.Fatal("missing bag completion")
	}
	if err := s.completeManufacture(ctx, c, due); err != nil || c.character.Bag[1].Count != 2 {
		t.Fatal("duplicate output", err)
	}
	if err := s.dispatch(ctx, c, []byte{64, 1, 0}); err == nil {
		t.Fatal("truncated formula accepted")
	}
}
func TestManufacturingChanceAndFeeAtomicSQL(t *testing.T) {
	s, c, _ := nativeManufactureFixture(t)
	ctx := context.Background()
	zero := float64(0)
	c.character.Gold = 100
	if err := s.commit(ctx, c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	s.Assets.Economy.Manufacturing = []assets.ManufacturingRecipe{{Workbench: "Forge", Inputs: [2]assets.ManufacturingInput{{ItemID: 32176, Count: 2}}, Output: assets.ManufacturingInput{ItemID: 32177, Count: 1}, Fee: 30, SuccessPercent: &zero}}
	inputs := [2]assets.ManufacturingInput{{ItemID: 32176, Count: 2}}
	ok, err := s.manufacture(ctx, c, "Forge", inputs)
	if err != nil || ok || c.character.Gold != 70 || c.character.Bag[0].Count != 8 || !c.character.Bag[1].Empty() {
		t.Fatal(ok, err, c.character)
	}
	hundred := float64(100)
	s.Assets.Economy.Manufacturing[0].SuccessPercent = &hundred
	s.Assets.Economy.Manufacturing[0].Fee = 80
	ok, err = s.manufacture(ctx, c, "Forge", inputs)
	if err != nil || ok || c.character.Gold != 70 || c.character.Bag[0].Count != 8 {
		t.Fatal("insufficient fee consumed resources", ok, err)
	}
	s.Assets.Economy.Manufacturing[0].Fee = 20
	ok, err = s.manufacture(ctx, c, "Forge", inputs)
	if err != nil || !ok || c.character.Gold != 50 || c.character.Bag[1].Count != 1 {
		t.Fatal(ok, err)
	}
}
func TestGemSocketIncompleteSourceDoesNotConsume(t *testing.T) {
	s, c, wire := nativeManufactureFixture(t)
	before := c.character.Bag
	if err := s.dispatch(context.Background(), c, []byte{37, 1, 1, 2}); err != nil {
		t.Fatal(err)
	}
	if c.character.Bag != before || !contains(wire.packets(t), []byte{37, 1, 1, 0}) {
		t.Fatal("incomplete socket handler claimed success")
	}
}

package server

import (
	"context"
	"testing"
	"time"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
)

func fishingFixture(t *testing.T) (*Server, *Session, *captureConn) {
	t.Helper()
	s, c, wire, _ := mallFixture(t)
	s.Assets.Fishing = assets.FishingRules{Enabled: true, IntervalSeconds: 60, Skills: []uint16{15993}, CatchRequirements: []uint32{1, 35}, Maps: []uint16{c.character.Map}, Rods: []assets.FishingRod{{ItemID: 37105, MaxGrade: 6}}, Rewards: []assets.FishingReward{{ItemID: 44001, Grade: 1, Weight: 1}}}
	s.Assets.Items[37105] = game.ItemDefinition{ID: 37105, Type: 28}
	s.Assets.Items[44001] = game.ItemDefinition{ID: 44001, Type: 32}
	c.character.X, c.character.Y = 0, 0
	c.character.Bag = game.Inventory{}
	c.character.Bag[0] = game.Item{ID: 37105, Count: 1}
	c.character.Skills = []game.LearnedSkill{{ID: 15993, Grade: 1}}
	if s.Assets.Terrains == nil {
		s.Assets.Terrains = map[uint16]assets.Terrain{}
	}
	s.Assets.Terrains[c.character.Map] = assets.Terrain{Width: 40, Height: 40, GridWidth: 2, GridHeight: 2, Cells: []byte{0, 2, 0, 0}}
	if err := s.commit(context.Background(), c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	return s, c, wire
}
func TestFishingNativeStartStopAndTimedCatch(t *testing.T) {
	s, c, wire := fishingFixture(t)
	ctx := context.Background()
	if err := s.dispatch(ctx, c, []byte{23, 53}); err != nil || c.fishing == nil {
		t.Fatal(c.fishing, err)
	}
	due := c.fishing.NextAt
	if err := s.dispatch(ctx, c, []byte{90, 2}); err != nil || !c.character.Bag[1].Empty() {
		t.Fatal(err, c.character.Bag)
	}
	if err := s.catchFishing(ctx, c, due); err != nil {
		t.Fatal(err)
	}
	if c.character.Bag[1].ID != 44001 || c.character.Skills[0].Grade != 2 {
		t.Fatal(c.character)
	}
	if err := s.catchFishing(ctx, c, due); err != nil || c.character.Bag[1].Count != 1 {
		t.Fatal("replay", err)
	}
	packets := wire.packets(t)
	if !contains(packets, []byte{8, 1, 110, 1, 2, 0, 0, 0, 121, 62, 0, 0}) {
		t.Fatal("missing committed fishing grade", packets)
	}
	if err := s.dispatch(ctx, c, []byte{23, 54}); err != nil || c.fishing != nil {
		t.Fatal(err)
	}
	if err := s.dispatch(ctx, c, []byte{23, 53, 1}); err == nil {
		t.Fatal("malformed start accepted")
	}
}
func TestFishingFullBagProgressAndWalkingMerge(t *testing.T) {
	s, c, _ := fishingFixture(t)
	ctx := context.Background()
	for i := 1; i < len(c.character.Bag); i++ {
		c.character.Bag[i] = game.Item{ID: 32176, Count: 50}
	}
	if err := s.commit(ctx, c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	if err := s.startFishing(ctx, c, 1); err != nil {
		t.Fatal(err)
	}
	// Walking stays buffered while SQL validates the catch's inventory.
	c.character.X = 1
	c.fishing.X = 1
	if err := s.catchFishing(ctx, c, c.fishing.NextAt); err != nil {
		t.Fatal(err)
	}
	if c.character.X != 1 || c.character.Skills[0].Grade != 2 || c.fishing == nil {
		t.Fatal("full bag progress or pending movement lost")
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].X != 0 || chars[0].Bag != c.character.Bag {
		t.Fatal(chars, err)
	}
	c.character.X++
	if err := s.catchFishing(ctx, c, time.Now().Add(time.Hour)); err != nil || c.fishing != nil {
		t.Fatal("movement failed to cancel", err)
	}
}

func TestFishingUnlearnedSkillAndFailedTransactionRetry(t *testing.T) {
	s, c, wire := fishingFixture(t)
	ctx := context.Background()
	c.character.Skills = nil
	if err := s.commit(ctx, c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	if err := s.startFishing(ctx, c, 1); err != nil {
		t.Fatal(err)
	}
	due := c.fishing.NextAt
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.catchFishing(canceled, c, due); err == nil {
		t.Fatal("canceled transaction accepted")
	}
	if c.fishing == nil || !c.character.Bag[1].Empty() || len(wire.packets(t)) != 0 {
		t.Fatal("failed catch published state")
	}
	if err := s.catchFishing(ctx, c, due); err != nil {
		t.Fatal(err)
	}
	if c.character.Bag[1].ID != 44001 || len(c.character.Skills) != 0 {
		t.Fatal("unlearned skill fishing changed skill ownership")
	}
	c.character.Bag[0] = game.Item{}
	if err := s.catchFishing(ctx, c, due); err != nil || c.fishing != nil {
		t.Fatal("lost rod failed to cancel", err)
	}
}
func TestFishingStartRequiresRodShoreAndNoParty(t *testing.T) {
	s, c, _ := fishingFixture(t)
	ctx := context.Background()
	c.party = &party{}
	if err := s.startFishing(ctx, c, 0); err != nil || c.fishing != nil {
		t.Fatal("party fishing accepted", err)
	}
	c.party = nil
	c.character.X = 40
	if err := s.startFishing(ctx, c, 0); err != nil || c.fishing != nil {
		t.Fatal("offshore fishing accepted", err)
	}
	c.character.X = 0
	c.character.Bag[0] = game.Item{}
	if err := s.startFishing(ctx, c, 0); err != nil || c.fishing != nil {
		t.Fatal("rodless fishing accepted", err)
	}
}

func TestFishingLegacyCatchReceiptAfterCommit(t *testing.T) {
	s, c, wire := fishingFixture(t)
	ctx := context.Background()
	if err := s.dispatch(ctx, c, []byte{90, 1}); err != nil {
		t.Fatal(err)
	}
	if !contains(wire.packets(t), []byte{90, 1, 1}) {
		t.Fatal("missing cast ACK")
	}
	due := c.fishing.NextAt
	if err := s.catchFishing(ctx, c, due); err != nil {
		t.Fatal(err)
	}
	if !contains(wire.packets(t), []byte{90, 2, 225, 171, 1}) {
		t.Fatal("missing committed catch receipt")
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Bag[1].ID != 44001 {
		t.Fatal("catch receipt preceded delivery", err)
	}
}

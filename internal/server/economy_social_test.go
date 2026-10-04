package server

import (
	"context"
	"errors"
	"testing"
	"time"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/store"
)

func openTestStall(t *testing.T, s *Server, c *Session) {
	t.Helper()
	p, _ := protocol.Builder{56, 1}.String("Test stall")
	p = p.U8(1).U8(1).U16(32176).U32(10).U8(5)
	tradeDo(t, s, c, p)
	if c.stall == nil {
		t.Fatal("stall did not open")
	}
}
func TestStallPurchasePreservesMetadataAndCannotOversell(t *testing.T) {
	s, players, wires := tradeFixture(t)
	seller, buyer := players[0], players[1]
	openTestStall(t, s, seller)
	sign, _ := protocol.Builder{56, 30}.U32(seller.character.ID).String("Test stall")
	if !contains(wires[1].packets(t), sign) {
		t.Fatal("native sign missing")
	}
	tradeDo(t, s, buyer, protocol.Builder{56, 4}.U32(seller.character.ID).U8(1).U8(3))
	stored := tradeStored(t, s, players)
	if seller.character.Gold != 130 || buyer.character.Gold != 170 || stored[0].Gold != 130 || stored[1].Bag[1].Count != 3 || stored[1].Bag[1].Metadata[6] != 7 || stored[1].Bag[1].Damage != 2 {
		t.Fatal("purchase or metadata incorrect", stored)
	}
	if seller.stall.Listings[0].Item.Count != 2 {
		t.Fatal("listed stock incorrect")
	}
	before := tradeStored(t, s, players)
	tradeDo(t, s, buyer, protocol.Builder{56, 4}.U32(seller.character.ID).U8(1).U8(3))
	after := tradeStored(t, s, players)
	if after[0].Gold != before[0].Gold || after[1].Bag != before[1].Bag {
		t.Fatal("oversale mutated state")
	}
	tradeDo(t, s, buyer, protocol.Builder{56, 4}.U32(seller.character.ID).U8(1).U8(2))
	if seller.stall != nil || seller.character.Gold != 150 || buyer.character.Gold != 150 {
		t.Fatal("sellout did not close")
	}
}
func TestStallRollbackAndStaleInventory(t *testing.T) {
	s, players, _ := tradeFixture(t)
	seller, buyer := players[0], players[1]
	next := buyer.character.Clone()
	for i := range next.Bag {
		next.Bag[i] = game.Item{ID: 32176, Count: 50, Damage: 99}
	}
	if err := s.commit(context.Background(), buyer, next); err != nil {
		t.Fatal(err)
	}
	openTestStall(t, s, seller)
	before := tradeStored(t, s, players)
	tradeDo(t, s, buyer, protocol.Builder{56, 4}.U32(seller.character.ID).U8(1).U8(1))
	after := tradeStored(t, s, players)
	if before[0].Bag != after[0].Bag || before[0].Gold != after[0].Gold || before[1].Gold != after[1].Gold {
		t.Fatal("full inventory partially purchased")
	}
	if err := s.Store.UpdateCharacter(context.Background(), seller.account.ID, seller.character.ID, func(c *game.Character) error { c.Bag[0].Damage++; return nil }); err != nil {
		t.Fatal(err)
	}
	next = buyer.character.Clone()
	next.Bag[1] = game.Item{}
	if err := s.commit(context.Background(), buyer, next); err != nil {
		t.Fatal(err)
	}
	tradeDo(t, s, buyer, protocol.Builder{56, 4}.U32(seller.character.ID).U8(1).U8(1))
	if seller.character.Gold != 100 || buyer.character.Gold != 200 || seller.stall.Listings[0].Item.Count != 5 {
		t.Fatal("stale record purchased")
	}
}
func TestGuildInviteAuthorizationChatAndPersistence(t *testing.T) {
	s, players, wires := tradeFixture(t)
	a, b := players[0], players[1]
	ctx := context.Background()
	if err := s.Store.CreateGuild(ctx, a.character.ID, "Migrated"); err != nil {
		t.Fatal(err)
	}
	// An acceptance without an invitation must not grant membership.
	tradeDo(t, s, b, protocol.Builder{39, 3}.U32(a.character.ID))
	if guild, err := s.Store.GuildForCharacter(ctx, b.character.ID); err != nil || guild != nil {
		t.Fatal("uninvited member", guild, err)
	}
	tradeDo(t, s, a, protocol.Builder{39, 2}.U32(b.character.ID))
	if !contains(wires[1].packets(t), protocol.Builder{39, 3}.U32(a.character.ID)) {
		t.Fatal("guild invitation missing")
	}
	tradeDo(t, s, b, protocol.Builder{39, 3}.U32(a.character.ID))
	guild, err := s.Store.GuildForCharacter(ctx, b.character.ID)
	if err != nil || guild == nil || len(guild.Members) != 2 {
		t.Fatal(guild, err)
	}
	rank := store.GuildRankViceLeader
	if err = s.Store.EditGuild(ctx, b.character.ID, nil, nil, a.character.ID, &rank); !errors.Is(err, store.ErrGuildPermission) {
		t.Fatal("nonleader permission", err)
	}
	tradeDo(t, s, a, protocol.Builder{39, 14}.U32(b.character.ID))
	guild, err = s.Store.GuildForCharacter(ctx, b.character.ID)
	if err != nil || guild.Roster[1].Rank != store.GuildRankViceLeader {
		t.Fatal("promotion not durable", guild, err)
	}
	wires[0].Reset()
	wires[1].Reset()
	tradeDo(t, s, b, append([]byte{2, 6}, []byte("Guild chat")...))
	expected := protocol.Builder{2, 6}.U32(b.character.ID).Bytes([]byte("Guild chat"))
	if !contains(wires[0].packets(t), expected) || !contains(wires[1].packets(t), expected) {
		t.Fatal("guild chat not routed")
	}
	tradeDo(t, s, a, []byte{39, 6})
	guild, err = s.Store.GuildForCharacter(ctx, b.character.ID)
	if err != nil || guild.LeaderID != b.character.ID {
		t.Fatal("leader departure did not repair", guild, err)
	}
}
func TestManufacturingAtomicCostsAndGatheringStops(t *testing.T) {
	s, players, wires := tradeFixture(t)
	c := players[0]
	ctx := context.Background()
	s.Assets.Economy.Manufacturing = []assets.ManufacturingRecipe{{Workbench: "Forge", Inputs: [2]assets.ManufacturingInput{{ItemID: 32176, Count: 2}, {ItemID: 32176, Count: 1}}, Output: assets.ManufacturingInput{ItemID: 32176, Count: 1}}}
	before := c.character.Bag[0].Count
	tradeDo(t, s, c, protocol.Builder{59, 1}.U16(32176).U8(2).U16(32176).U8(1))
	if c.character.Bag[0].Count != before-3 || c.character.Bag[1].Count != 1 || !contains(wires[0].packets(t), []byte{59, 1, 1}) {
		t.Fatal("manufacturing inventory/wire", c.character.Bag)
	}
	s.Assets.Economy.Gathering = []assets.GatheringPool{{Kind: 1, IntervalSeconds: 5, Items: []uint16{32176}}}
	tradeDo(t, s, c, append([]byte{2, 2}, []byte("/fish")...))
	if c.gathering == nil {
		t.Fatal("gathering did not start")
	}
	tick := c.gathering.NextAt
	s.worldMu.Lock()
	s.tickGathering(ctx, tick)
	s.worldMu.Unlock()
	if c.character.Bag[1].Count != 2 {
		t.Fatal("gathering did not grant")
	}
	s.worldMu.Lock()
	s.tickGathering(ctx, tick.Add(time.Second))
	s.worldMu.Unlock()
	if c.character.Bag[1].Count != 2 {
		t.Fatal("early tick granted")
	}
	c.character.X++
	s.worldMu.Lock()
	s.tickGathering(ctx, tick.Add(time.Hour))
	s.worldMu.Unlock()
	if c.gathering != nil || c.character.Bag[1].Count != 2 {
		t.Fatal("movement did not stop gathering")
	}
}

func TestSynthesisSQLRatesPreserveNativeCompound(t *testing.T) {
	s, players, wires := tradeFixture(t)
	c := players[0]
	ctx := context.Background()
	next := c.character.Clone()
	next.Bag[1] = next.Bag[0]
	next.Bag[1].Count = 5
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	s.Assets.AlchemyRecipes = []assets.AlchemyRecipe{{Input1: 32176, Input2: 32176, Output: 32176}}
	s.Assets.Economy.Synthesis = assets.SynthesisRules{FailureItemID: 32176, DefaultSuccessPercent: 100}
	tradeDo(t, s, c, append([]byte{2, 2}, []byte("/compound 1 2")...))
	if c.character.Bag[0].Count != 9 || c.character.Bag[1].Count != 4 || c.character.Bag[2].Count != 1 {
		t.Fatal("synthesis costs or result", c.character.Bag)
	}
	effect := protocol.Builder{5, 5}.U32(c.character.ID).U16(60020)
	if !contains(wires[0].packets(t), effect) {
		t.Fatal("synthesis effect missing")
	}
	s.Assets.Economy.Synthesis.DefaultSuccessPercent = 0
	wires[0].Reset()
	tradeDo(t, s, c, append([]byte{2, 2}, []byte("/compound 1 2")...))
	if contains(wires[0].packets(t), effect) || c.character.Bag[2].Count != 2 {
		t.Fatal("failure branch not honored")
	}
	// The native handler remains deterministic even when public synthesis is 0%.
	wires[0].Reset()
	tradeDo(t, s, c, []byte{40, 1, 1, 2})
	if !contains(wires[0].packets(t), protocol.Builder{40, 1, 1}.U16(32176)) {
		t.Fatal("native alchemy behavior changed")
	}
}
func TestPublicMarriageAndParcelCommands(t *testing.T) {
	s, players, wires := tradeFixture(t)
	a, b := players[0], players[1]
	ctx := context.Background()
	s.Assets.Economy.Marriage = assets.MarriageRules{MinimumLevel: 1, Fee: 50, Rings: [2]uint16{32176, 32176}}
	tradeDo(t, s, a, append([]byte{2, 2}, []byte("/marry "+b.character.Name)...))
	if b.marriageProposal == nil {
		t.Fatal("proposal missing")
	}
	tradeDo(t, s, b, append([]byte{2, 2}, []byte("/acceptmarry")...))
	marriage, err := s.Store.Marriage(ctx, b.character.ID)
	if err != nil || marriage == nil || a.character.Gold != 50 {
		t.Fatal("ceremony failed", marriage, err)
	}
	tradeDo(t, s, a, append([]byte{2, 2}, []byte("/divorce")...))
	marriage, err = s.Store.Marriage(ctx, b.character.ID)
	if err != nil || marriage != nil {
		t.Fatal("divorce failed", marriage, err)
	}
	// Store sends an owned damaged stack; native mailbox exposes both attachments.
	if _, err = s.Store.SendParcel(ctx, store.CharacterRef{Account: a.account.ID, ID: a.character.ID}, b.character.ID, "Letter", "Body", 10, 1, 2, s.Assets.Items); err != nil {
		t.Fatal(err)
	}
	rows, err := s.Store.Parcels(ctx, b.character.ID)
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	wires[1].Reset()
	tradeDo(t, s, b, append([]byte{2, 2}, []byte("/mail")...))
	expected := protocol.Builder{23, 76, 1}.U32(rows[0].ID).U32(a.character.ID)
	expected, _ = expected.String(a.character.Name)
	expected, _ = expected.String("Letter")
	expected = expected.U8(0).U8(0).U32(10).U16(32176).U8(2)
	if !contains(wires[1].packets(t), expected) {
		t.Fatal("native mailbox layout missing")
	}
}

func TestAdminGuildRefreshAndGatheringNativeSafety(t *testing.T) {
	s, players, wires := tradeFixture(t)
	ctx := context.Background()
	a, b := players[0], players[1]
	if err := s.Store.CreateGuild(ctx, a.character.ID, "Admin refresh"); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.JoinGuild(ctx, a.character.ID, b.character.ID); err != nil {
		t.Fatal(err)
	}
	guild, err := s.Store.GuildForCharacter(ctx, a.character.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, wire := range wires {
		wire.Reset()
	}
	if err = s.AdminDeleteGuild(ctx, guild.ID); err != nil {
		t.Fatal(err)
	}
	cleared, _ := protocol.Builder{39, 30}.U32(a.character.ID).U32(0).String("")
	if !contains(wires[0].packets(t), []byte{39, 1, 0}) || !contains(wires[1].packets(t), cleared) {
		t.Fatal("admin guild disband did not clear native UI")
	}
	s.Assets.Economy.Gathering = []assets.GatheringPool{{Kind: 1, IntervalSeconds: 5, Items: []uint16{32176}}}
	for _, wire := range wires {
		wire.Reset()
	}
	tradeDo(t, s, a, append([]byte{2, 2}, []byte("/fish")...))
	if a.gathering == nil {
		t.Fatal("gathering not started")
	}
	for _, wire := range wires {
		for _, p := range wire.packets(t) {
			if len(p) >= 2 && p[0] == 5 && (p[1] == 12 || p[1] == 14) {
				t.Fatal("unverified short reference animation sent", p)
			}
		}
	}
	// A numeric typo in a public command gives feedback without a protocol error.
	tradeDo(t, s, a, append([]byte{2, 2}, []byte("/manufacture Forge invalid 2 0 0")...))
}

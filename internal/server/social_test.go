package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/store"
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
	s.Assets.Economy.Gathering = []assets.GatheringPool{{Kind: assets.GatheringMining, IntervalSeconds: 5, Items: []uint16{32176}}}
	tradeDo(t, s, c, append([]byte{2, 2}, []byte("/mine")...))
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

func TestSynthesisUsesRankBasesAcrossCommands(t *testing.T) {
	s, players, wires := compoundFixture(t)
	c := players[0]
	s.Assets.Economy.Synthesis = assets.SynthesisRules{FailureItemID: 100, DefaultSuccessPercent: 0}
	tradeDo(t, s, c, append([]byte{2, 2}, []byte("/compound 5 9")...))
	if c.character.Bag[0].ID != 300 {
		t.Fatal("chat shortcut did not use rank/base engine")
	}
	wires[0].Reset()
	next := c.character.Clone()
	next.Bag = game.Inventory{}
	next.Bag[4] = game.Item{ID: 100, Count: 1}
	next.Bag[8] = game.Item{ID: 200, Count: 1}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	tradeDo(t, s, c, []byte{40, 1, 5, 9})
	if !contains(wires[0].packets(t), protocol.Builder{40, 1, 1}.U16(300)) {
		t.Fatal("AC40 did not share rank/base behavior")
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
	s.Assets.Economy.Gathering = []assets.GatheringPool{{Kind: assets.GatheringMining, IntervalSeconds: 5, Items: []uint16{32176}}}
	for _, wire := range wires {
		wire.Reset()
	}
	tradeDo(t, s, a, append([]byte{2, 2}, []byte("/mine")...))
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

func friendFixture(t *testing.T) (*Server, []*Session, []*captureConn) {
	s, players, wires := worldFixture(t)
	for _, c := range players {
		tradeDo(t, s, c, []byte{12, 1})
	}
	for _, w := range wires {
		w.Reset()
	}
	return s, players, wires
}

func TestFriendDispatcherRequestAcceptListsAndRemoval(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[2] // Different maps.
	tradeDo(t, s, b, protocol.Builder{14, 3}.U32(a.character.ID))
	if w[0].Len() != 0 || w[2].Len() != 0 {
		t.Fatal("unsolicited accept succeeded")
	}
	tradeDo(t, s, a, protocol.Builder{14, 2}.U32(b.character.ID))
	if !contains(w[2].packets(t), protocol.Builder{14, 2}.U32(a.character.ID)) {
		t.Fatal("cross-map friend request missing")
	}
	tradeDo(t, s, a, protocol.Builder{14, 2}.U32(b.character.ID))
	if w[2].Len() != 0 {
		t.Fatal("duplicate prompt")
	}
	tradeDo(t, s, b, protocol.Builder{14, 3}.U32(a.character.ID).U8(1))
	for _, pair := range []struct {
		owner, other *Session
		wire         *captureConn
	}{{a, b, w[0]}, {b, a, w[2]}} {
		packets := pair.wire.packets(t)
		id := pair.other.character.ID
		if !contains(packets, protocol.Builder{14, 3}.U32(id).U8(1)) || !contains(packets, protocol.Builder{14, 9}.U32(id).U8(0)) || !contains(packets, protocol.Builder{10, 3}.U32(id).U8(255)) {
			t.Fatal("native accept packets missing", packets)
		}
		if !contains(packets, protocol.Builder{14, 11}.Bytes(serverwire.FriendEntry(*pair.other.character, true, true))) || !contains(packets, protocol.Builder{14, 5}.Bytes(serverwire.FriendEntry(*pair.other.character, false, true))) {
			t.Fatal("native lists wrong", packets)
		}
		friends, err := s.Store.Friends(context.Background(), pair.owner.character.ID)
		if err != nil || len(friends) != 1 || friends[0].ID != id {
			t.Fatal("friend not durable", friends, err)
		}
	}
	if w[1].Len() != 0 {
		t.Fatal("friend packets leaked to unrelated player")
	}
	tradeDo(t, s, b, protocol.Builder{14, 3}.U32(a.character.ID))
	if w[0].Len() != 0 || w[2].Len() != 0 {
		t.Fatal("accept replayed")
	}
	tradeDo(t, s, a, protocol.Builder{14, 4}.U32(b.character.ID))
	if !contains(w[0].packets(t), protocol.Builder{14, 4}.U32(b.character.ID)) || !contains(w[2].packets(t), protocol.Builder{14, 4}.U32(a.character.ID)) {
		t.Fatal("remove not replicated")
	}
	for _, c := range []*Session{a, b} {
		f, err := s.Store.Friends(context.Background(), c.character.ID)
		if err != nil || len(f) != 0 {
			t.Fatal("removed friendship remained", err)
		}
	}
}

func TestFriendPresenceOfflineAndReconnect(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[2]
	if err := s.Store.AddFriend(context.Background(), a.character.ID, b.character.ID); err != nil {
		t.Fatal(err)
	}
	s.leaveWorld(b)
	packets := w[0].packets(t)
	if !contains(packets, protocol.Builder{14, 9}.U32(b.character.ID).U8(3)) || !contains(packets, protocol.Builder{14, 5}.Bytes(serverwire.FriendEntry(*b.character, false, false))) {
		t.Fatal("offline presence wrong", packets)
	}
	tradeDo(t, s, b, []byte{12, 1})
	online := protocol.Builder{14, 9}.U32(b.character.ID).U8(0)
	if !contains(w[0].packets(t), online) {
		t.Fatal("friend reconnect missing")
	}
	w[2].Reset()
	tradeDo(t, s, b, []byte{14, 2})
	if !contains(w[2].packets(t), protocol.Builder{14, 5}.Bytes(serverwire.FriendEntry(*a.character, false, true))) {
		t.Fatal("list query wrong")
	}
	// A map load is not logout, but disconnect during it still reports offline.
	s.depart(b, protocol.Builder{12}.U32(b.character.ID))
	w[0].Reset()
	tradeDo(t, s, a, []byte{14, 2})
	if !contains(w[0].packets(t), protocol.Builder{14, 5}.Bytes(serverwire.FriendEntry(*b.character, false, true))) {
		t.Fatal("map loading appeared offline")
	}
	s.leaveWorld(b)
	if !contains(w[0].packets(t), protocol.Builder{14, 9}.U32(b.character.ID).U8(3)) {
		t.Fatal("disconnect during warp failed to report offline")
	}
}

func TestFriendExpiredRequestsAndDeparture(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[2]
	for _, action := range []string{"expired", "departed"} {
		tradeDo(t, s, a, protocol.Builder{14, 2}.U32(b.character.ID))
		w[2].Reset()
		if action == "expired" {
			b.friendRequests[a.character.ID] = time.Now().Add(-2 * friendRequestTTL)
		} else {
			s.leaveWorld(a)
		}
		tradeDo(t, s, b, protocol.Builder{14, 3}.U32(a.character.ID))
		if w[2].Len() != 0 {
			t.Fatal("stale friend accepted")
		}
		f, err := s.Store.Friends(context.Background(), b.character.ID)
		if err != nil || len(f) != 0 {
			t.Fatal("stale friendship saved", err)
		}
	}
}

func TestFriendMalformedAndFailedSave(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[2]
	for _, packet := range [][]byte{{14}, {14, 2, 1}, {14, 2, 1, 2, 3}, {14, 3}, {14, 3, 1, 2, 3}, {14, 3, 1, 2, 3, 4, 5, 6}, {14, 4, 1}} {
		if err := s.dispatch(context.Background(), a, packet); err == nil {
			t.Fatal("malformed friend packet accepted", packet)
		}
	}
	tradeDo(t, s, a, protocol.Builder{14, 2}.U32(a.character.ID))
	tradeDo(t, s, a, protocol.Builder{14, 2}.U32(999999))
	if w[0].Len() != 0 {
		t.Fatal("invalid target acknowledged")
	}
	tradeDo(t, s, a, protocol.Builder{14, 2}.U32(b.character.ID))
	w[2].Reset()
	s.Store.Close()
	if err := s.dispatch(context.Background(), b, protocol.Builder{14, 3}.U32(a.character.ID)); err == nil {
		t.Fatal("failed save ignored")
	}
	if w[0].Len() != 0 || w[2].Len() != 0 {
		t.Fatal("failed save emitted success")
	}
}

func TestFriendEntryGoldenAppearanceWords(t *testing.T) {
	c := game.Character{ID: 0x01020304, Name: "Al", Level: 25, Element: 3, Body: 2, Head: 4, Color1: 0x33441122, Color2: 0x77885566}
	expected := []byte{4, 3, 2, 1, 2, 'A', 'l', 25, 0, 0, 3, 2, 4, 0x22, 0x11, 0x44, 0x33, 0x66, 0x55, 0x88, 0x77, 0}
	if !hasPacket([][]byte{serverwire.FriendEntry(c, true, true)}, expected) {
		t.Fatal("friend appearance words")
	}
	if !hasPacket([][]byte{serverwire.FriendEntry(c, false, true)}, append(expected, 0, 1)) {
		t.Fatal("friend online record")
	}
}

func TestFriendAccountDeletionRefreshesOnlinePeer(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[2]
	if err := s.Store.AddFriend(context.Background(), a.character.ID, b.character.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAccount(context.Background(), b.account.ID); err != nil {
		t.Fatal(err)
	}
	if !contains(w[0].packets(t), protocol.Builder{14, 4}.U32(b.character.ID)) {
		t.Fatal("deleted account retained friend UI")
	}
	friends, err := s.Store.Friends(context.Background(), a.character.ID)
	if err != nil || len(friends) != 0 {
		t.Fatal("deleted account retained relationship", err)
	}
}

// TestFriendRefusalAndIgnore: aLogin's No (2) and timeout (3) answers form
// no friendship; the asker hears the answer once.
func TestFriendRefusalAndIgnore(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[2]
	for _, answer := range []byte{protocol.FriendsAnswerRefuse, protocol.FriendsAnswerIgnore} {
		tradeDo(t, s, a, protocol.Builder{14, 2}.U32(b.character.ID))
		w[0].Reset()
		w[2].Reset()
		tradeDo(t, s, b, protocol.Builder{14, 3}.U32(a.character.ID).U8(answer))
		if got := w[0].packets(t); len(got) != 1 || !contains(got, protocol.Builder{14, 3}.U32(b.character.ID).U8(answer)) || w[2].Len() != 0 {
			t.Fatal("answer not reported to the asker", answer, got)
		}
		for _, c := range []*Session{a, b} {
			if f, err := s.Store.Friends(context.Background(), c.character.ID); err != nil || len(f) != 0 {
				t.Fatal("refused friendship saved", answer, f, err)
			}
		}
		tradeDo(t, s, b, protocol.Builder{14, 3}.U32(a.character.ID).U8(protocol.FriendsAnswerAccept))
		if w[0].Len() != 0 {
			t.Fatal("answered request accepted later")
		}
	}
}

func TestFriendInvalidAnswerKeepsInvitation(t *testing.T) {
	s, p, _ := friendFixture(t)
	a, b := p[0], p[1]
	tradeDo(t, s, a, protocol.Builder{14, 2}.U32(b.character.ID))
	if err := s.friendCommand(context.Background(), b, protocol.Builder{14, 3}.U32(a.character.ID).U8(255)); err != protocol.ErrMalformed {
		t.Fatal("invalid answer accepted", err)
	}
	if _, pending := b.friendRequests[a.character.ID]; !pending {
		t.Fatal("invalid answer consumed invitation")
	}
	tradeDo(t, s, b, protocol.Builder{14, 3}.U32(a.character.ID).U8(1))
	friends, err := s.Store.Friends(context.Background(), b.character.ID)
	if err != nil || len(friends) != 1 {
		t.Fatal(friends, err)
	}
}

func nativeMailRequest(recipient uint32, body []byte) []byte {
	return protocol.Builder{14, 1, 0}.U32(recipient).Bytes(body)
}

func TestTextMailLiveCrossMapAndDurableReceipt(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[2]
	ctx := context.Background()
	body := []byte{0xa4, 0xa4, 0xa4, 0xe5, '\n', 'H', 'i'}
	tradeDo(t, s, a, nativeMailRequest(b.character.ID, append(bytes.Clone(body), 0, 0)))
	ack := protocol.Builder{14, 9}.U32(b.character.ID).U8(0)
	if !contains(w[0].packets(t), ack) {
		t.Fatal("sender did not receive native receipt")
	}
	messages := w[2].packets(t)
	if len(messages) != 1 || len(messages[0]) != 14+len(body) || !bytes.Equal(messages[0][14:], body) || binary.LittleEndian.Uint32(messages[0][2:6]) != a.character.ID {
		t.Fatal(messages)
	}
	if w[1].Len() != 0 {
		t.Fatal("mail leaked")
	}
	pending, err := s.Store.PendingTextMail(ctx, b.character.ID, 10)
	if err != nil || len(pending) != 0 {
		t.Fatal(pending, err)
	}
	tradeDo(t, s, b, []byte{12, 1})
	if w[2].Len() != 0 {
		t.Fatal("mail replayed on duplicate acknowledgment")
	}
}

func TestTextMailDeferredUntilLoginAndWarpAcknowledgment(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[2]
	s.leaveWorld(b)
	w[0].Reset()
	w[2].Reset()
	tradeDo(t, s, a, nativeMailRequest(b.character.ID, []byte("offline")))
	if !contains(w[0].packets(t), protocol.Builder{14, 9}.U32(b.character.ID).U8(3)) {
		t.Fatal("offline mail incorrectly advertised online presence")
	}
	if w[2].Len() != 0 {
		t.Fatal("delivered offline")
	}
	tradeDo(t, s, b, []byte{12, 1})
	first := w[2].packets(t)
	count := 0
	for _, packet := range first {
		if len(packet) >= 15 && packet[0] == 14 && packet[1] == 1 {
			count++
		}
	}
	if count != 1 {
		t.Fatal("offline mail not replayed once", first)
	}
	s.depart(b, protocol.Builder{12}.U32(b.character.ID))
	w[2].Reset()
	tradeDo(t, s, a, nativeMailRequest(b.character.ID, []byte("loading")))
	if w[2].Len() != 0 {
		t.Fatal("delivered into loading snapshot")
	}
	tradeDo(t, s, b, []byte{12, 1})
	second := w[2].packets(t)
	count = 0
	for _, packet := range second {
		if len(packet) >= 14 && packet[0] == 14 && packet[1] == 1 && string(packet[14:]) == "loading" {
			count++
		}
	}
	if count != 1 {
		t.Fatal(second)
	}
}

func TestTextMailFailedRecipientStaysQueued(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[2]
	failed := &failedWorldConn{}
	b.conn = failed
	tradeDo(t, s, a, nativeMailRequest(b.character.ID, []byte("retry")))
	if !failed.closed {
		t.Fatal("failed recipient not closed")
	}
	if !contains(w[0].packets(t), protocol.Builder{14, 9}.U32(b.character.ID).U8(0)) {
		t.Fatal("recipient failure withheld accepted sender receipt")
	}
	pending, err := s.Store.PendingTextMail(context.Background(), b.character.ID, 10)
	if err != nil || len(pending) != 1 {
		t.Fatal(pending, err)
	}
	s.leaveWorld(b)
	b.conn = w[2]
	w[2].Reset()
	tradeDo(t, s, b, []byte{12, 1})
	pending, err = s.Store.PendingTextMail(context.Background(), b.character.ID, 10)
	if err != nil || len(pending) != 0 {
		t.Fatal(pending, err)
	}
}

func TestTextMailRejectsMalformedAndUnknownRecipient(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[2]
	for _, packet := range [][]byte{{14, 1}, {14, 1, 0, 1, 2, 3}, nativeMailRequest(b.character.ID, nil), nativeMailRequest(b.character.ID, bytes.Repeat([]byte{'a'}, 256)), nativeMailRequest(b.character.ID, []byte("a\x00b"))} {
		if err := s.dispatch(context.Background(), a, packet); !errors.Is(err, protocol.ErrMalformed) {
			t.Fatal(packet, err)
		}
	}
	tradeDo(t, s, a, nativeMailRequest(999999, []byte("unknown")))
	if contains(w[0].packets(t), protocol.Builder{14, 9}.U32(999999).U8(0)) || w[2].Len() != 0 {
		t.Fatal("unknown recipient accepted")
	}
	// Closed storage must produce neither a success acknowledgment nor delivery.
	s.Store.Close()
	if err := s.dispatch(context.Background(), a, nativeMailRequest(b.character.ID, []byte("closed"))); err == nil {
		t.Fatal("storage failure ignored")
	}
	if w[0].Len() != 0 || w[2].Len() != 0 {
		t.Fatal("failed persistence published success")
	}
}

func TestTextMailDeliveryBatchesKeepOrder(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[2]
	ctx := context.Background()
	for i := 0; i < 35; i++ {
		if _, err := s.Store.SendTextMail(ctx, store.CharacterRef{Account: a.account.ID, ID: a.character.ID}, b.character.ID, 0, []byte{byte('A' + i)}, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	s.worldMu.Lock()
	err := s.deliverPendingTextMail(ctx, b)
	s.worldMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	packets := w[2].packets(t)
	if len(packets) != 35 {
		t.Fatal(len(packets))
	}
	for i, packet := range packets {
		if len(packet) != 15 || packet[14] != byte('A'+i) {
			t.Fatal(i, packet)
		}
	}
}

func TestTextMailSenderWriteFailureKeepsAcceptedMessage(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[2]
	a.conn = &failedWorldConn{}
	if err := s.dispatch(context.Background(), a, nativeMailRequest(b.character.ID, []byte("accepted"))); err == nil {
		t.Fatal("sender write failure ignored")
	}
	pending, err := s.Store.PendingTextMail(context.Background(), b.character.ID, 10)
	if err != nil || len(pending) != 1 || string(pending[0].Content) != "accepted" {
		t.Fatal(pending, err)
	}
	if w[2].Len() != 0 {
		t.Fatal("sender failure overtook receipt")
	}
}

func TestTextMailRespectsSenderWorldGates(t *testing.T) {
	for _, gate := range []string{"loading", "battle", "minigame"} {
		t.Run(gate, func(t *testing.T) {
			s, p, w := friendFixture(t)
			a, b := p[0], p[2]
			switch gate {
			case "loading":
				a.ready = false
			case "battle":
				a.battle = &battleRun{}
			case "minigame":
				a.event = &eventSession{onMinigame: func(byte) error { t.Fatal("mail consumed event"); return nil }}
			}
			err := s.dispatch(context.Background(), a, nativeMailRequest(b.character.ID, []byte("blocked")))
			if gate == "loading" && !errors.Is(err, protocol.ErrMalformed) {
				t.Fatal(err)
			}
			if gate != "loading" && err != nil {
				t.Fatal(err)
			}
			pending, err := s.Store.PendingTextMail(context.Background(), b.character.ID, 10)
			if err != nil || len(pending) != 0 || w[0].Len() != 0 || w[2].Len() != 0 {
				t.Fatal("mail bypassed interaction gate", pending, err)
			}
		})
	}
}

func TestSocialNativeStatusGolden(t *testing.T) {
	packet, err := socialStatusPacket(&Session{character: &game.Character{ID: 0x12345678, Name: "Alice"}})
	expected := []byte{10, 1, 0x78, 0x56, 0x34, 0x12, 1, 5, 'A', 'l', 'i', 'c', 'e'}
	if err != nil || !bytes.Equal(packet, expected) {
		t.Fatal(packet, err)
	}
}

func TestSocialAddReplyListAndRemoval(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[1]
	ctx := context.Background()
	tradeDo(t, s, a, protocol.Builder{10, 1}.U32(b.character.ID))
	for _, pair := range []struct {
		owner, friend *Session
		wire          *captureConn
	}{{a, b, w[0]}, {b, a, w[1]}} {
		packets := pair.wire.packets(t)
		online := protocol.Builder{14, 11}.Bytes(serverwire.FriendEntry(*pair.friend.character, true, true))
		status, _ := protocol.Builder{10, 1}.U32(pair.friend.character.ID).U8(1).String(pair.friend.character.Name)
		if len(packets) != 3 || !contains(packets, protocol.Builder{14, 9}.U32(pair.friend.character.ID).U8(0)) || !contains(packets, online) || !contains(packets, status) {
			t.Fatal(packets)
		}
		friends, err := s.Store.Friends(ctx, pair.owner.character.ID)
		if err != nil || len(friends) != 1 || friends[0].ID != pair.friend.character.ID {
			t.Fatal(friends, err)
		}
	}
	if w[2].Len() != 0 {
		t.Fatal("social packets leaked")
	}
	// A repeat emits status only and does not duplicate the durable relationship.
	tradeDo(t, s, a, protocol.Builder{10, 1}.U32(b.character.ID))
	for _, wire := range w[:2] {
		packets := wire.packets(t)
		if len(packets) != 1 || packets[0][0] != 10 || packets[0][1] != 1 {
			t.Fatal(packets)
		}
	}
	for _, request := range [][]byte{{10, 3}, {10, 6}, protocol.Builder{10, 6}.U32(b.character.ID)} {
		tradeDo(t, s, a, request)
		packets := w[0].packets(t)
		if len(packets) != 3 || !contains(packets, protocol.Builder{14, 11}.Bytes(serverwire.FriendEntry(*b.character, true, true))) || !contains(packets, protocol.Builder{14, 5}.Bytes(serverwire.FriendEntry(*b.character, false, true))) {
			t.Fatal(packets)
		}
	}
	tradeDo(t, s, a, protocol.Builder{10, 4}.U32(b.character.ID))
	if !contains(w[0].packets(t), protocol.Builder{10, 4}.U32(b.character.ID)) || !contains(w[1].packets(t), protocol.Builder{14, 4}.U32(a.character.ID)) {
		t.Fatal("removal receipt missing")
	}
	// Legacy AC10:2 acceptance independently adds a same-map contact.
	tradeDo(t, s, b, protocol.Builder{10, 2}.U32(a.character.ID).U8(1))
	for _, wire := range w[:2] {
		packets := wire.packets(t)
		if len(packets) != 2 || packets[0][0] != 14 || packets[0][1] != 9 || packets[1][0] != 14 || packets[1][1] != 11 {
			t.Fatal(packets)
		}
	}
}

func TestSocialRejectsRemoteLoadingSelfAndDeclinedContacts(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b, remote := p[0], p[1], p[2]
	for _, packet := range [][]byte{protocol.Builder{10, 1}.U32(remote.character.ID), protocol.Builder{10, 2}.U32(remote.character.ID).U8(1), protocol.Builder{10, 1}.U32(a.character.ID), protocol.Builder{10, 1}.U32(999999), protocol.Builder{10, 2}.U32(b.character.ID).U8(0)} {
		tradeDo(t, s, a, packet)
	}
	b.ready = false
	tradeDo(t, s, a, protocol.Builder{10, 1}.U32(b.character.ID))
	friends, err := s.Store.Friends(context.Background(), a.character.ID)
	if err != nil || len(friends) != 0 {
		t.Fatal(friends, err)
	}
	for _, wire := range w {
		if wire.Len() != 0 {
			t.Fatal("rejected contact published packets")
		}
	}
}

func TestSocialValidatesBeforeMutationAndUsesWorldGates(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[1]
	for _, packet := range [][]byte{{10}, {10, 1}, {10, 1, 1, 2, 3}, protocol.Builder{10, 1}.U32(b.character.ID).U8(0), protocol.Builder{10, 2}.U32(b.character.ID), protocol.Builder{10, 2}.U32(b.character.ID).U8(2), {10, 3, 0}, {10, 4}, {10, 6, 1}} {
		if err := s.dispatch(context.Background(), a, packet); !errors.Is(err, protocol.ErrMalformed) {
			t.Fatal(packet, err)
		}
	}
	if err := s.dispatch(context.Background(), a, []byte{10, 99}); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
	for _, wire := range w {
		if wire.Len() != 0 {
			t.Fatal("invalid request emitted packets")
		}
	}
	for _, gate := range []string{"loading", "battle", "minigame"} {
		a.ready = gate != "loading"
		a.battle = nil
		a.event = nil
		if gate == "battle" {
			a.battle = &battleRun{}
		}
		if gate == "minigame" {
			a.event = &eventSession{onMinigame: func(byte) error { t.Fatal("social consumed event"); return nil }}
		}
		err := s.dispatch(context.Background(), a, protocol.Builder{10, 1}.U32(b.character.ID))
		if gate == "loading" && !errors.Is(err, protocol.ErrMalformed) {
			t.Fatal(err)
		}
		if gate != "loading" && err != nil {
			t.Fatal(err)
		}
	}
	friends, err := s.Store.Friends(context.Background(), a.character.ID)
	if err != nil || len(friends) != 0 {
		t.Fatal("blocked request added friend", friends, err)
	}
}

func TestSocialSharesAC14StateAndClearsInvitations(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[1]
	tradeDo(t, s, a, protocol.Builder{14, 2}.U32(b.character.ID))
	w[1].Reset()
	a.friendRequests = map[uint32]time.Time{b.character.ID: time.Now()}
	tradeDo(t, s, b, protocol.Builder{10, 2}.U32(a.character.ID).U8(1))
	w[0].Reset()
	w[1].Reset()
	if len(a.friendRequests) != 0 || len(b.friendRequests) != 0 {
		t.Fatal("obsolete invitations retained")
	}
	tradeDo(t, s, b, protocol.Builder{14, 3}.U32(a.character.ID))
	if w[0].Len() != 0 || w[1].Len() != 0 {
		t.Fatal("stale invitation acceptance replayed")
	}
	tradeDo(t, s, a, []byte{14, 2})
	if !contains(w[0].packets(t), protocol.Builder{14, 5}.Bytes(serverwire.FriendEntry(*b.character, false, true))) {
		t.Fatal("AC10 and AC14 lists diverged")
	}
	tradeDo(t, s, b, protocol.Builder{14, 4}.U32(a.character.ID))
	w[1].Reset()
	tradeDo(t, s, b, []byte{10, 3})
	if !contains(w[1].packets(t), []byte{14, 5}) {
		t.Fatal("AC14 removal retained AC10 contact")
	}
}

func TestSocialPersistenceFailureDoesNotPublishSuccess(t *testing.T) {
	s, p, w := friendFixture(t)
	s.Store.Close()
	for _, packet := range [][]byte{protocol.Builder{10, 1}.U32(p[1].character.ID), protocol.Builder{10, 2}.U32(p[1].character.ID).U8(1), protocol.Builder{10, 4}.U32(p[1].character.ID)} {
		if err := s.dispatch(context.Background(), p[0], packet); err == nil {
			t.Fatal("failed persistence accepted")
		}
	}
	for _, wire := range w {
		if wire.Len() != 0 {
			t.Fatal("failed commit published success")
		}
	}
}

func TestSocialFailedRecipientDoesNotFailActor(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[1]
	failed := &failedWorldConn{}
	b.conn = failed
	tradeDo(t, s, a, protocol.Builder{10, 1}.U32(b.character.ID))
	if !failed.closed || w[0].Len() == 0 {
		t.Fatal("recipient failure isolation failed")
	}
	w[0].Reset()
	tradeDo(t, s, a, protocol.Builder{10, 4}.U32(b.character.ID))
	if !contains(w[0].packets(t), protocol.Builder{10, 4}.U32(b.character.ID)) {
		t.Fatal("failed recipient prevented removal receipt")
	}
	friends, err := s.Store.Friends(context.Background(), a.character.ID)
	if err != nil || len(friends) != 0 {
		t.Fatal(friends, err)
	}
}

func TestSocialFullRecipientDoesNotCreateOneSidedContact(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[1]
	ctx := context.Background()
	for i := 0; i < store.FriendLimit; i++ {
		name := fmt.Sprintf("Contact%d", i)
		account, err := s.Store.Register(ctx, name, "password", "")
		if err != nil {
			t.Fatal(err)
		}
		character := b.character.Clone()
		character.ID = account.CharacterID(1)
		character.Name = name
		if err := s.Store.CreateCharacter(ctx, account, character); err != nil {
			t.Fatal(err)
		}
		if err := s.Store.AddFriend(ctx, b.character.ID, character.ID); err != nil {
			t.Fatal(err)
		}
	}
	tradeDo(t, s, a, protocol.Builder{10, 1}.U32(b.character.ID))
	actor, err := s.Store.Friends(ctx, a.character.ID)
	if err != nil || len(actor) != 0 {
		t.Fatal("full recipient left one-sided relationship", actor, err)
	}
	recipient, err := s.Store.Friends(ctx, b.character.ID)
	if err != nil || len(recipient) != store.FriendLimit {
		t.Fatal(len(recipient), err)
	}
	packets := w[0].packets(t)
	if len(packets) != 1 || packets[0][0] != 2 || packets[0][1] != 16 || w[1].Len() != 0 {
		t.Fatal("full list published success", packets)
	}
}

func TestNativeSocialProfileSavesAndPreservesPendingWalking(t *testing.T) {
	s, players, wires := friendFixture(t)
	c := players[0]
	ctx := context.Background()
	baseline := c.character.Clone()
	c.autosaveBaseline = &baseline
	c.character.X++
	walkedX := c.character.X
	// Actual native nickname layout, including a three-character six-byte frame.
	for _, packet := range [][]byte{{10, 1, 3, 'A', 'B', 'C'}, {10, 1, 5, 'H', 'e', 'l', 'l', 'o'}, {10, 2, 9, 2, 100, 2, 29}} {
		if err := s.dispatch(ctx, c, packet); err != nil {
			t.Fatal(packet, err)
		}
	}
	if c.character.Nickname != "Hello" || c.character.BloodType != 2 || c.character.BirthYearOffset != 100 || c.character.BirthMonth != 2 || c.character.BirthDay != 29 || c.character.SocialProfileCode != 9 || c.character.X != walkedX {
		t.Fatal("native profile or walking lost", c.character)
	}
	if c.autosaveBaseline.X != baseline.X || c.autosaveBaseline.Nickname != "Hello" {
		t.Fatal("checkpoint baseline overwritten")
	}
	friends, err := s.Store.Friends(ctx, c.character.ID)
	if err != nil || len(friends) != 0 {
		t.Fatal("profile created friendship", friends, err)
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Nickname != "Hello" || chars[0].BloodType != 2 || chars[0].BirthDay != 29 || chars[0].X != baseline.X {
		t.Fatal("profile not durable or walking prematurely saved", chars, err)
	}
	appearance, err := chars[0].AppearancePacket(false)
	if err != nil || !bytes.Equal(appearance[len(appearance)-4:], []byte{2, 100, 2, 29}) {
		t.Fatal("profile reconnect bytes", appearance, err)
	}
	peerPackets := wires[1].packets(t)
	want := protocol.Builder{10, 1}.U32(c.character.ID).U8(5).Bytes([]byte("Hello"))
	if !contains(peerPackets, want) || !contains(peerPackets, protocol.Builder{10, 2}.U32(c.character.ID).U8(9)) {
		t.Fatal("native profile peer update", peerPackets)
	}
	if wires[2].Len() != 0 {
		t.Fatal("profile leaked across maps")
	}
	s.autosaveCharacters(ctx)
	chars, err = s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Nickname != "Hello" || chars[0].X != walkedX {
		t.Fatal("walking checkpoint clobbered profile", chars, err)
	}
}

func TestNativeSocialMissingFieldsStayConnectedAndDoNotSave(t *testing.T) {
	s, players, wires := friendFixture(t)
	c := players[0]
	ctx := context.Background()
	for _, packet := range [][]byte{{10, 2, 9, 0, 100, 2, 29}, {10, 2, 9, 2, 100, 0, 29}, {10, 2, 9, 2, 101, 2, 29}, {10, 2, 9, 2, 100, 2, 0}} {
		if err := s.dispatch(ctx, c, packet); err != nil {
			t.Fatal("incomplete form rejected connection", packet, err)
		}
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].BloodType != 0 || c.character.BloodType != 0 {
		t.Fatal("invalid profile saved", chars, err)
	}
	for _, packet := range wires[0].packets(t) {
		if packet[0] != 2 || packet[1] != 16 {
			t.Fatal("invalid profile success reply", packet)
		}
	}
	if wires[1].Len() != 0 || wires[2].Len() != 0 {
		t.Fatal("invalid profile broadcast")
	}
}

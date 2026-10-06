package server

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func tradeFixture(t *testing.T) (*Server, []*Session, []*captureConn) {
	t.Helper()
	s, players, wires := worldFixture(t)
	for i, player := range players[:2] {
		next := player.character.Clone()
		next.Gold = uint32((i + 1) * 100)
		next.Bag = game.Inventory{}
		next.Bag[0] = game.Item{ID: 32176, Count: byte((i + 1) * 10), Damage: byte(i + 2)}
		next.Bag[0].Metadata[6] = byte(i + 7)
		if err := s.commit(context.Background(), player, next); err != nil {
			t.Fatal(err)
		}
		if err := s.dispatch(context.Background(), player, []byte{12, 1}); err != nil {
			t.Fatal(err)
		}
	}
	for _, wire := range wires {
		wire.Reset()
	}
	return s, players, wires
}
func tradeDo(t *testing.T, s *Server, c *Session, p []byte) {
	t.Helper()
	if err := s.dispatch(context.Background(), c, p); err != nil {
		t.Fatal(err)
	}
}
func openTrade(t *testing.T, s *Server, players []*Session, wires []*captureConn) {
	t.Helper()
	a, b := players[0], players[1]
	tradeDo(t, s, a, protocol.Builder{25, 1}.U32(b.character.ID))
	if !contains(wires[1].packets(t), protocol.Builder{25, 1}.U32(a.character.ID)) {
		t.Fatal("trade request not forwarded")
	}
	tradeDo(t, s, b, []byte{25, 2, 1})
	if a.trade == nil || a.trade != b.trade {
		t.Fatal("trade did not open")
	}
	if !contains(wires[0].packets(t), protocol.Builder{25, 1}.U32(b.character.ID)) || !contains(wires[1].packets(t), protocol.Builder{25, 1}.U32(a.character.ID)) {
		t.Fatal("native open packets missing")
	}
	for _, wire := range wires {
		wire.Reset()
	}
}
func tradeStored(t *testing.T, s *Server, players []*Session) [2]game.Character {
	t.Helper()
	var result [2]game.Character
	for i, c := range players[:2] {
		chars, err := s.Store.Characters(context.Background(), c.account.ID)
		if err != nil || len(chars) != 1 {
			t.Fatal(err)
		}
		result[i] = chars[0]
	}
	return result
}

func TestTradeDispatcherOfferAndAtomicCompletion(t *testing.T) {
	s, players, wires := tradeFixture(t)
	openTrade(t, s, players, wires)
	a, b := players[0], players[1]
	tradeDo(t, s, a, []byte{25, 3, 1, 1, 3})
	offer := protocol.Builder{25, 3}.U32(0).U8(1).U16(32176).U8(3).U8(2)
	if p := wires[1].packets(t); len(p) != 1 || !bytes.Equal(p[0], offer) {
		t.Fatal("offer wire", p)
	}
	tradeDo(t, s, b, protocol.Builder{25, 4}.U32(50))
	wires[0].Reset()
	tradeDo(t, s, a, []byte{25, 5}) // Native lock action is a no-op.
	tradeDo(t, s, a, []byte{25, 6})
	if wires[0].Len() != 0 || a.character.Gold != 100 || a.character.Bag[0].Count != 10 {
		t.Fatal("first confirmation transferred assets")
	}
	tradeDo(t, s, b, []byte{25, 6})
	if a.trade != nil || b.trade != nil || a.character.Gold != 150 || b.character.Gold != 150 || a.character.Bag[0].Count != 7 || b.character.Bag[1].Count != 3 || b.character.Bag[1].Damage != 2 || b.character.Bag[1].Metadata[6] != 7 {
		t.Fatal("trade result incorrect")
	}
	for _, wire := range wires[:2] {
		if !contains(wire.packets(t), []byte{25, 2, 4}) {
			t.Fatal("completion missing")
		}
	}
	if wires[2].Len() != 0 {
		t.Fatal("trade leaked to another map")
	}
	stored := tradeStored(t, s, players)
	if stored[0].Gold != 150 || stored[1].Gold != 150 || stored[0].Bag[0].Count != 7 || stored[1].Bag[1].Metadata[6] != 7 {
		t.Fatal("pair result not durable")
	}
	tradeDo(t, s, a, []byte{25, 6})
	tradeDo(t, s, b, []byte{25, 6})
	if wires[0].Len() != 0 || wires[1].Len() != 0 || a.character.Gold+b.character.Gold != 300 {
		t.Fatal("completion replayed")
	}
}

func TestTradeOfferRevisionResetsBothConfirmations(t *testing.T) {
	s, players, wires := tradeFixture(t)
	openTrade(t, s, players, wires)
	a, b := players[0], players[1]
	tradeDo(t, s, a, []byte{25, 3, 1, 1, 3})
	tradeDo(t, s, a, []byte{25, 6})
	tradeDo(t, s, b, protocol.Builder{25, 4}.U32(500)) // Clamp to owned gold, including the displayed offer.
	if p := wires[0].packets(t); len(p) != 1 || !bytes.Equal(p[0], protocol.Builder{25, 3}.U32(200).U8(0)) {
		t.Fatal("displayed gold differed from reserved gold", p)
	}
	tradeDo(t, s, b, []byte{25, 6})
	if a.trade == nil || a.character.Gold != 100 {
		t.Fatal("old confirmation accepted revised offer")
	}
	tradeDo(t, s, a, protocol.Builder{25, 4}.U32(10)) // Gold replaces the item offer, matching C#.
	if len(a.trade.offers[0].Items) != 0 || a.trade.accepted != [2]bool{} {
		t.Fatal("replacement retained items or confirmations")
	}
	for _, wire := range wires {
		wire.Reset()
	}
	tradeDo(t, s, a, []byte{25, 6})
	tradeDo(t, s, b, []byte{25, 6})
	if a.character.Bag[0].Count != 10 || b.character.Bag[0].Count != 20 || a.character.Gold != 290 || b.character.Gold != 10 {
		t.Fatal("replacement did not trade only gold")
	}
}

func TestTradeRefusesMalformedAndUnsolicitedReplies(t *testing.T) {
	s, players, wires := tradeFixture(t)
	for _, p := range [][]byte{{25}, {25, 1, 0}, {25, 2}, {25, 3, 1, 1}, {25, 4, 0}, {25, 5, 1}, {25, 6, 1}, {25, 7, 1}} {
		if err := s.dispatch(context.Background(), players[0], p); err == nil {
			t.Fatal("malformed trade accepted", p)
		}
	}
	tradeDo(t, s, players[1], []byte{25, 2, 1})
	if players[1].trade != nil {
		t.Fatal("unsolicited accept opened trade")
	}
	tradeDo(t, s, players[0], protocol.Builder{25, 1}.U32(players[0].character.ID))
	tradeDo(t, s, players[0], protocol.Builder{25, 1}.U32(players[2].character.ID))
	if players[0].trade != nil || players[1].tradeRequest != nil || wires[2].Len() != 0 {
		t.Fatal("self/cross-map trade accepted")
	}
	tradeDo(t, s, players[0], protocol.Builder{25, 1}.U32(players[1].character.ID))
	players[1].tradeRequest.at = time.Now().Add(-2 * tradeRequestTTL)
	tradeDo(t, s, players[1], []byte{25, 2, 1})
	if players[0].trade != nil || players[1].trade != nil {
		t.Fatal("expired request accepted")
	}
}

func TestTradeCancellationReleasesInventories(t *testing.T) {
	for _, action := range []string{"cancel", "move", "disconnect", "mutation"} {
		t.Run(action, func(t *testing.T) {
			s, players, wires := tradeFixture(t)
			openTrade(t, s, players, wires)
			a, b := players[0], players[1]
			tradeDo(t, s, a, []byte{25, 3, 1, 1, 3})
			for _, wire := range wires {
				wire.Reset()
			}
			tradeDo(t, s, a, []byte{23, 124, 1, 3, 0}) // Reserved inventory cannot be destroyed.
			if a.character.Bag[0].Count != 10 || a.trade == nil {
				t.Fatal("offered item changed while window open")
			}
			wires[0].Reset()
			switch action {
			case "cancel":
				tradeDo(t, s, b, []byte{25, 7})
			case "move":
				tradeDo(t, s, a, protocol.Builder{6, 1, 0}.U16(a.character.X+1).U16(a.character.Y))
			case "disconnect":
				s.leaveWorld(a)
			case "mutation":
				next := a.character.Clone()
				next.Gold++
				if err := s.commit(context.Background(), a, next); err != nil {
					t.Fatal(err)
				}
			}
			if a.trade != nil || b.trade != nil || a.character.Bag[0].Count != 10 || b.character.Bag[0].Count != 20 {
				t.Fatal("cancel changed offered items or retained session")
			}
			if !contains(wires[1].packets(t), []byte{25, 2, 2}) {
				t.Fatal("partner not told of cancellation")
			}
		})
	}
}

func TestTradeSaveFailureAndStaleMetadataNeverPublishSuccess(t *testing.T) {
	for _, failure := range []string{"database", "metadata"} {
		t.Run(failure, func(t *testing.T) {
			s, players, wires := tradeFixture(t)
			openTrade(t, s, players, wires)
			a, b := players[0], players[1]
			tradeDo(t, s, a, []byte{25, 3, 1, 1, 3})
			tradeDo(t, s, b, protocol.Builder{25, 4}.U32(50))
			for _, wire := range wires {
				wire.Reset()
			}
			if failure == "database" {
				s.Store.Close()
			} else {
				if err := s.Store.UpdateCharacter(context.Background(), a.account.ID, a.character.ID, func(c *game.Character) error { c.Bag[0].Metadata[6]++; return nil }); err != nil {
					t.Fatal(err)
				}
			}
			tradeDo(t, s, a, []byte{25, 6})
			err := s.dispatch(context.Background(), b, []byte{25, 6})
			if failure == "database" && err == nil {
				t.Fatal("failed DB save was ignored")
			}
			if a.character.Gold != 100 || b.character.Gold != 200 || a.character.Bag[0].Count != 10 || b.character.Bag[0].Count != 20 || a.trade != nil || b.trade != nil {
				t.Fatal("failed trade adopted assets")
			}
			for _, wire := range wires[:2] {
				if contains(wire.packets(t), []byte{25, 2, 4}) {
					t.Fatal("failed trade sent success")
				}
			}
		})
	}
}

func TestTradeConcurrentConfirmIsCommittedOnce(t *testing.T) {
	s, players, wires := tradeFixture(t)
	openTrade(t, s, players, wires)
	tradeDo(t, s, players[0], []byte{25, 3, 1, 1, 3})
	tradeDo(t, s, players[1], protocol.Builder{25, 4}.U32(50))
	for _, wire := range wires {
		wire.Reset()
	}
	var wg sync.WaitGroup
	errors := make(chan error, 20)
	for i := 0; i < 20; i++ {
		c := players[i%2]
		wg.Add(1)
		go func() { defer wg.Done(); errors <- s.dispatch(context.Background(), c, []byte{25, 6}) }()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if players[0].character.Gold != 150 || players[1].character.Gold != 150 || players[0].character.Bag[0].Count != 7 || players[1].character.Bag[1].Count != 3 {
		t.Fatal("concurrent confirmation duplicated assets")
	}
	for _, wire := range wires[:2] {
		count := 0
		for _, p := range wire.packets(t) {
			if bytes.Equal(p, []byte{25, 2, 4}) {
				count++
			}
		}
		if count != 1 {
			t.Fatal("completion count", count)
		}
	}
}

func TestTradePendingRequesterDepartureClosesPrompt(t *testing.T) {
	s, players, wires := tradeFixture(t)
	tradeDo(t, s, players[0], protocol.Builder{25, 1}.U32(players[1].character.ID))
	wires[1].Reset()
	s.leaveWorld(players[0])
	if players[1].tradeRequest != nil || !contains(wires[1].packets(t), []byte{25, 2, 2}) {
		t.Fatal("departed requester retained prompt")
	}
	tradeDo(t, s, players[1], []byte{25, 2, 1})
	if players[1].trade != nil {
		t.Fatal("departed requester accepted")
	}
}

func TestTradeTransfersMountedVehicleAndDismountsOwner(t *testing.T) {
	s, players, wires := tradeFixture(t)
	a, b := players[0], players[1]
	s.Assets.Items[48016] = game.ItemDefinition{ID: 48016, Type: game.VehicleType}
	next := a.character.Clone()
	next.Bag[1] = game.Item{ID: 48016, Count: 1, Damage: 27}
	next.ActiveVehicle, next.VehicleSlot = 48016, 2
	if err := s.commit(context.Background(), a, next); err != nil {
		t.Fatal(err)
	}
	openTrade(t, s, players, wires)
	tradeDo(t, s, a, []byte{25, 3, 1, 2, 1})
	tradeDo(t, s, a, []byte{25, 6})
	tradeDo(t, s, b, []byte{25, 6})
	if a.character.ActiveVehicle != 0 || a.character.VehicleSlot != 0 || !a.character.Bag[1].Empty() {
		t.Fatal("transferred vehicle retained by owner")
	}
	if b.character.Bag[1].ID != 48016 || b.character.Bag[1].Damage != 27 || b.character.ActiveVehicle != 0 {
		t.Fatal("recipient lost vehicle state or boarded automatically")
	}
	dismount := protocol.Builder{15, 11, 2}.U32(a.character.ID)
	for _, wire := range wires[:2] {
		if !contains(wire.packets(t), dismount) {
			t.Fatal("dismount not replicated")
		}
	}
	saved := tradeStored(t, s, players)
	if saved[0].ActiveVehicle != 0 || saved[1].Bag[1].Damage != 27 {
		t.Fatal("vehicle transfer not saved")
	}
}

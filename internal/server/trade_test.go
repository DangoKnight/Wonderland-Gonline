package server

import (
	"bytes"
	"context"
	"sync"
	"testing"
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

// tradeOffer is the Confirm button's AC25:2: gold, then bag slots.
func tradeOffer(gold uint32, slots ...byte) []byte {
	return protocol.Builder{25, 2}.U32(gold).Bytes(slots)
}

// openTrade requests a trade, which aLogin opens on both sides at once.
func openTrade(t *testing.T, s *Server, players []*Session, wires []*captureConn) {
	t.Helper()
	a, b := players[0], players[1]
	tradeDo(t, s, a, protocol.Builder{25, 1}.U32(b.character.ID))
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

func TestTradeNativeOfferAndAtomicCompletion(t *testing.T) {
	s, players, wires := tradeFixture(t)
	openTrade(t, s, players, wires)
	a, b := players[0], players[1]
	tradeDo(t, s, a, tradeOffer(0, 1))
	// FUN_003c704c: gold, then ID, count, damage, metadata 2..7, 0..1, 8..25.
	golden := []byte{25, 3, 0, 0, 0, 0, 0xb0, 0x7d, 10, 2, 0, 0, 0, 0, 7, 0, 0, 0}
	golden = append(golden, make([]byte, 18)...)
	if p := wires[1].packets(t); len(p) != 1 || !bytes.Equal(p[0], golden) {
		t.Fatalf("offer wire %v", p)
	}
	tradeDo(t, s, b, tradeOffer(50))
	if p := wires[0].packets(t); len(p) != 1 || !bytes.Equal(p[0], []byte{25, 3, 50, 0, 0, 0}) {
		t.Fatalf("gold offer wire %v", p)
	}
	wires[0].Reset()
	tradeDo(t, s, a, []byte{25, 3, 1})
	if wires[0].Len() != 0 || a.character.Gold != 100 || a.character.Bag[0].Count != 10 {
		t.Fatal("first acceptance transferred assets")
	}
	tradeDo(t, s, b, []byte{25, 3, 1})
	if a.trade != nil || b.trade != nil || a.character.Gold != 150 || b.character.Gold != 150 || !a.character.Bag[0].Empty() || b.character.Bag[1].Count != 10 || b.character.Bag[1].Damage != 2 || b.character.Bag[1].Metadata[6] != 7 {
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
	if stored[0].Gold != 150 || stored[1].Gold != 150 || !stored[0].Bag[0].Empty() || stored[1].Bag[1].Metadata[6] != 7 {
		t.Fatal("pair result not durable")
	}
	for _, wire := range wires {
		wire.Reset()
	}
	// The client answers 25/2 by closing its form, which sends a cancel.
	tradeDo(t, s, a, []byte{25, 3, 2})
	tradeDo(t, s, a, []byte{25, 3, 1})
	tradeDo(t, s, b, []byte{25, 3, 1})
	if wires[0].Len() != 0 || wires[1].Len() != 0 || a.character.Gold+b.character.Gold != 300 {
		t.Fatal("completion replayed")
	}
}

func TestTradeAcceptNeedsBothLockedOffers(t *testing.T) {
	s, players, wires := tradeFixture(t)
	openTrade(t, s, players, wires)
	a, b := players[0], players[1]
	tradeDo(t, s, a, []byte{25, 3, 1})
	tradeDo(t, s, a, tradeOffer(20, 1))
	tradeDo(t, s, a, []byte{25, 3, 1})
	if a.trade == nil || a.trade.accepted != [2]bool{} {
		t.Fatal("acceptance before both offers counted")
	}
	tradeDo(t, s, a, tradeOffer(90)) // A locked offer does not change.
	if a.trade.offers[0].Gold != 20 || len(a.trade.offers[0].Items) != 1 {
		t.Fatal("locked offer replaced")
	}
	tradeDo(t, s, b, tradeOffer(0))
	tradeDo(t, s, a, []byte{25, 3, 1})
	tradeDo(t, s, b, []byte{25, 3, 1})
	if a.character.Gold != 80 || b.character.Gold != 220 || !a.character.Bag[0].Empty() || b.character.Bag[1].Count != 10 {
		t.Fatal("locked offers not exchanged")
	}
}

func TestTradeRefusesMalformedAndUnavailable(t *testing.T) {
	s, players, wires := tradeFixture(t)
	a, b := players[0], players[1]
	for _, p := range [][]byte{{25}, {25, 1, 0}, {25, 2, 0, 0, 0}, {25, 3}, {25, 3, 1, 1}, {25, 3, 9}, {25, 4, 0, 0, 0, 0}, append(tradeOffer(0), make([]byte, 21)...)} {
		if err := s.dispatch(context.Background(), a, p); err == nil {
			t.Fatal("malformed trade accepted", p)
		}
	}
	tradeDo(t, s, a, protocol.Builder{25, 1}.U32(a.character.ID))
	tradeDo(t, s, a, protocol.Builder{25, 1}.U32(players[2].character.ID))
	if a.trade != nil || !contains(wires[0].packets(t), []byte{25, 2, 1}) {
		t.Fatal("trade with an absent player accepted")
	}
	tradeDo(t, s, players[2], []byte{12, 1})
	wires[2].Reset()
	tradeDo(t, s, a, protocol.Builder{25, 1}.U32(players[2].character.ID))
	if a.trade != nil || wires[2].Len() != 0 || !contains(wires[0].packets(t), []byte{25, 2, 5}) {
		t.Fatal("self/cross-map trade accepted")
	}
	openTrade(t, s, players, wires)
	tradeDo(t, s, a, protocol.Builder{25, 1}.U32(b.character.ID))
	if !contains(wires[0].packets(t), []byte{25, 2, 11}) {
		t.Fatal("second trade not refused")
	}
	for _, p := range [][]byte{tradeOffer(0, 0), tradeOffer(0, 51), tradeOffer(0, 2, 2)} {
		if err := s.dispatch(context.Background(), a, p); err == nil {
			t.Fatal("bad slots accepted", p)
		}
	}
	tradeDo(t, s, a, tradeOffer(0, 2)) // An empty slot ends the trade.
	if a.trade != nil || b.trade != nil || !contains(wires[0].packets(t), []byte{25, 2, 8}) || !contains(wires[1].packets(t), []byte{25, 2, 9}) {
		t.Fatal("empty slot offer kept trade")
	}
	openTrade(t, s, players, wires)
	tradeDo(t, s, a, tradeOffer(101))
	if a.trade != nil || !contains(wires[0].packets(t), []byte{25, 2, 6}) || !contains(wires[1].packets(t), []byte{25, 2, 9}) {
		t.Fatal("unowned gold offer kept trade")
	}
}

func TestTradeCancellationReleasesInventories(t *testing.T) {
	for _, action := range []string{"cancel", "move", "disconnect", "mutation"} {
		t.Run(action, func(t *testing.T) {
			s, players, wires := tradeFixture(t)
			openTrade(t, s, players, wires)
			a, b := players[0], players[1]
			tradeDo(t, s, a, tradeOffer(0, 1))
			for _, wire := range wires {
				wire.Reset()
			}
			tradeDo(t, s, a, []byte{23, 124, 1, 3, 0}) // Reserved inventory cannot be destroyed.
			if a.character.Bag[0].Count != 10 || a.trade == nil {
				t.Fatal("offered item changed while window open")
			}
			wires[0].Reset()
			partner, own := byte(9), []byte{25, 2, 3}
			switch action {
			case "cancel":
				tradeDo(t, s, b, []byte{25, 3, 2})
				a, b = b, a
				wires[0], wires[1] = wires[1], wires[0]
				own = nil
			case "move":
				tradeDo(t, s, a, protocol.Builder{6, 1, 0}.U16(a.character.X+1).U16(a.character.Y))
			case "disconnect":
				s.leaveWorld(a)
				partner, own = 1, nil
			case "mutation":
				next := a.character.Clone()
				next.Gold++
				if err := s.commit(context.Background(), a, next); err != nil {
					t.Fatal(err)
				}
			}
			if a.trade != nil || b.trade != nil || players[0].character.Bag[0].Count != 10 || players[1].character.Bag[0].Count != 20 {
				t.Fatal("cancel changed offered items or retained session")
			}
			if !contains(wires[1].packets(t), []byte{25, 2, partner}) {
				t.Fatal("partner not told of cancellation")
			}
			if own != nil && !contains(wires[0].packets(t), own) {
				t.Fatal("own window not closed")
			}
			if own == nil && contains(wires[0].packets(t), []byte{25, 2, 3}) {
				t.Fatal("closed window told again")
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
			tradeDo(t, s, a, tradeOffer(0, 1))
			tradeDo(t, s, b, tradeOffer(50))
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
			tradeDo(t, s, a, []byte{25, 3, 1})
			err := s.dispatch(context.Background(), b, []byte{25, 3, 1})
			if failure == "database" && err == nil {
				t.Fatal("failed DB save was ignored")
			}
			if a.character.Gold != 100 || b.character.Gold != 200 || a.character.Bag[0].Count != 10 || b.character.Bag[0].Count != 20 || a.trade != nil || b.trade != nil {
				t.Fatal("failed trade adopted assets")
			}
			for _, wire := range wires[:2] {
				if p := wire.packets(t); contains(p, []byte{25, 2, 4}) || !contains(p, []byte{25, 2, 8}) {
					t.Fatal("failed trade result", p)
				}
			}
		})
	}
}

func TestTradeConcurrentAcceptIsCommittedOnce(t *testing.T) {
	s, players, wires := tradeFixture(t)
	openTrade(t, s, players, wires)
	tradeDo(t, s, players[0], tradeOffer(0, 1))
	tradeDo(t, s, players[1], tradeOffer(50))
	for _, wire := range wires {
		wire.Reset()
	}
	var wg sync.WaitGroup
	errors := make(chan error, 20)
	for i := 0; i < 20; i++ {
		c := players[i%2]
		wg.Add(1)
		go func() { defer wg.Done(); errors <- s.dispatch(context.Background(), c, []byte{25, 3, 1}) }()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if players[0].character.Gold != 150 || players[1].character.Gold != 150 || !players[0].character.Bag[0].Empty() || players[1].character.Bag[1].Count != 10 {
		t.Fatal("concurrent acceptance duplicated assets")
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
	tradeDo(t, s, a, tradeOffer(0, 2))
	tradeDo(t, s, b, tradeOffer(0))
	tradeDo(t, s, a, []byte{25, 3, 1})
	tradeDo(t, s, b, []byte{25, 3, 1})
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

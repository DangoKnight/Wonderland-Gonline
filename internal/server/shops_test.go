package server

import (
	"bytes"
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/world"
)

func bankFixture(t *testing.T) (*Server, *Session, []*captureConn) {
	t.Helper()
	s, p, w := friendFixture(t)
	c := p[0]
	next := c.character.Clone()
	next.Gold = 500
	next.BankGold = 100
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	for _, wire := range w {
		wire.Reset()
	}
	return s, c, w
}

func TestBankNativeBalanceGolden(t *testing.T) {
	character := game.Character{BankGold: 0x12345678, Gold: 0x87654321}
	for _, operation := range []byte{8, 9, 10} {
		expected := []byte{45, operation, 0x78, 0x56, 0x34, 0x12, 0x21, 0x43, 0x65, 0x87}
		if packet := serverwire.BankBalancePacket(&character, operation); !bytes.Equal(packet, expected) {
			t.Fatal(packet)
		}
	}
}

func TestBankDispatcherDepositWithdrawAndReload(t *testing.T) {
	s, c, w := bankFixture(t)
	tradeDo(t, s, c, []byte{45, 8})
	if packets := w[0].packets(t); len(packets) != 1 || !bytes.Equal(packets[0], []byte{45, 8, 100, 0, 0, 0, 244, 1, 0, 0}) {
		t.Fatal(packets)
	}
	tradeDo(t, s, c, []byte{45, 9, 125, 0, 0, 0})
	packets := w[0].packets(t)
	if len(packets) != 2 || !bytes.Equal(packets[0], []byte{26, 4, 119, 1, 0, 0}) || !bytes.Equal(packets[1], []byte{45, 9, 225, 0, 0, 0, 119, 1, 0, 0}) {
		t.Fatal(packets)
	}
	tradeDo(t, s, c, []byte{45, 10, 200, 0, 0, 0})
	packets = w[0].packets(t)
	if len(packets) != 2 || !bytes.Equal(packets[0], []byte{26, 4, 63, 2, 0, 0}) || !bytes.Equal(packets[1], []byte{45, 10, 25, 0, 0, 0, 63, 2, 0, 0}) {
		t.Fatal(packets)
	}
	if c.character.BankGold != 25 || c.character.Gold != 575 {
		t.Fatal(c.character)
	}
	for _, wire := range w[1:] {
		if wire.Len() != 0 {
			t.Fatal("private bank balances leaked")
		}
	}
	// Reload from the fixture DB, proving both fields were saved.
	// The existing store API lists state from SQL rather than session memory.
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || len(saved) != 1 || saved[0].BankGold != 25 || saved[0].Gold != 575 {
		t.Fatal(saved, err)
	}
	// Simulate a new login session loaded from persisted state.
	session := &Session{conn: &captureConn{}, character: &saved[0], account: c.account, ready: true}
	if err := s.dispatch(context.Background(), session, []byte{45, 8}); err != nil {
		t.Fatal(err)
	}
	if packets := session.conn.(*captureConn).packets(t); len(packets) != 1 || !bytes.Equal(packets[0], []byte{45, 8, 25, 0, 0, 0, 63, 2, 0, 0}) {
		t.Fatal(packets)
	}
}

func TestBankRefusesInsufficientOverflowAndCarryingLimit(t *testing.T) {
	for _, test := range []struct {
		name               string
		gold, bank, amount uint32
		operation          byte
	}{
		{"empty", 500, 100, 0, 9}, {"insufficient wallet", 500, 100, 501, 9},
		{"insufficient bank", 500, 100, 101, 10}, {"bank overflow", 1, math.MaxUint32, 1, 9},
		{"wallet cap", 999999, 100, 1, 10}, {"cross wallet cap", 999998, 100, 2, 10},
		{"large withdrawal", 0, math.MaxUint32, math.MaxUint32, 10},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, c, w := bankFixture(t)
			next := c.character.Clone()
			next.Gold = test.gold
			next.BankGold = test.bank
			if err := s.commit(context.Background(), c, next); err != nil {
				t.Fatal(err)
			}
			before := c.character.Clone()
			tradeDo(t, s, c, protocol.Builder{45, test.operation}.U32(test.amount))
			if !reflect.DeepEqual(before, *c.character) {
				t.Fatal("rejected transaction mutated state")
			}
			packets := w[0].packets(t)
			if len(packets) != 2 || packets[0][0] != 2 || packets[0][1] != 16 || packets[1][0] != 45 || packets[1][1] != 8 {
				t.Fatal("rejected transaction emitted success", packets)
			}
			saved, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil || saved[0].Gold != before.Gold || saved[0].BankGold != before.BankGold {
				t.Fatal(saved, err)
			}
		})
	}
}

func TestBankValidatesPacketsAndLeavesPlaceholdersUnavailable(t *testing.T) {
	s, c, w := bankFixture(t)
	before := c.character.Clone()
	for _, packet := range [][]byte{{45}, {45, 8, 0}, {45, 9}, {45, 9, 1, 2, 3}, {45, 10, 1, 2, 3}, {45, 9, 1, 2, 3, 4, 5}} {
		if err := s.dispatch(context.Background(), c, packet); !errors.Is(err, protocol.ErrMalformed) {
			t.Fatal(packet, err)
		}
	}
	if w[0].Len() != 0 {
		t.Fatal("invalid packet emitted success")
	}
	for _, operation := range []byte{11, 12} {
		tradeDo(t, s, c, []byte{45, operation})
		packets := w[0].packets(t)
		if len(packets) != 1 || packets[0][0] != 2 || packets[0][1] != 16 {
			t.Fatal("placeholder claimed success", packets)
		}
	}
	if !reflect.DeepEqual(before, *c.character) {
		t.Fatal("invalid/unavailable request mutated state")
	}
}

func TestBankRespectsInteractionGates(t *testing.T) {
	for _, gate := range []string{"loading", "battle", "minigame", "trade", "event", "storm", "beach"} {
		t.Run(gate, func(t *testing.T) {
			s, c, w := bankFixture(t)
			before := c.character.Clone()
			switch gate {
			case "loading":
				c.ready = false
			case "battle":
				c.battle = &battleRun{}
			case "minigame":
				c.event = &eventSession{onMinigame: func(byte) error { t.Fatal("bank consumed minigame"); return nil }}
			case "trade":
				c.trade = &tradeSession{}
			case "event":
				c.event = &eventSession{}
			case "storm":
				c.storm = true
			case "beach":
				c.beach = &beachRun{}
			}
			err := s.dispatch(context.Background(), c, []byte{45, 9, 1, 0, 0, 0})
			if gate == "loading" && !errors.Is(err, protocol.ErrMalformed) {
				t.Fatal(err)
			}
			if gate != "loading" && err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, *c.character) {
				t.Fatal("bank bypassed gate")
			}
			for _, packet := range w[0].packets(t) {
				if packet[0] == 45 && packet[1] == 9 {
					t.Fatal("blocked mutation acknowledged")
				}
			}
		})
	}
}

func TestBankFailureBeforeSaveDoesNotPublishOrChangeState(t *testing.T) {
	s, c, w := bankFixture(t)
	before := c.character.Clone()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.dispatch(ctx, c, []byte{45, 9, 50, 0, 0, 0}); err == nil {
		t.Fatal("failed save ignored")
	}
	if !reflect.DeepEqual(before, *c.character) || w[0].Len() != 0 {
		t.Fatal("failed save mutated or published state")
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].Gold != before.Gold || saved[0].BankGold != before.BankGold {
		t.Fatal(saved, err)
	}
}

func TestBankReceiptFailureKeepsDurableBalances(t *testing.T) {
	s, c, _ := bankFixture(t)
	c.conn = &failedWorldConn{}
	if err := s.dispatch(context.Background(), c, []byte{45, 9, 50, 0, 0, 0}); err == nil {
		t.Fatal("write failure ignored")
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].BankGold != 150 || saved[0].Gold != 450 || c.character.BankGold != 150 || c.character.Gold != 450 {
		t.Fatal(saved, err)
	}
}

func TestDecodeHotel(t *testing.T) {
	for _, p := range [][]byte{{31}, {31, 1}, {31, 2}, {31, 3, 1, 1}, {31, 2, 11}, {31, 3, 5}, {31, 4, 1}, {31, 4, 1, 1, 1}, {31, 4, 1, 1, 0, 9}, {31, 4, 0, 0}} {
		if _, _, ok := decodeHotel(p); ok {
			t.Fatalf("accepted %v", p)
		}
	}
	d, w, ok := decodeHotel([]byte{31, 4, 2, 4, 1, 2, 10, 3})
	if !ok || !bytes.Equal(d, []byte{4, 1}) || !bytes.Equal(w, []byte{10, 3}) {
		t.Fatal(d, w, ok)
	}
}

func TestHotelExchangeAndPersistence(t *testing.T) {
	s, players, wires := petFixture(t)
	c, wire := players[0], wires[0]
	ctx := context.Background()
	next := c.character.Clone()
	for i := 0; i < 4; i++ {
		next.Pets = append(next.Pets, game.Pet{ID: uint32(14000 + i), Slot: byte(i + 1), Name: "Team", Level: 2, Exp: 10, Amity: 80, Skills: []game.PetSkill{{ID: 11001, Grade: 2, Exp: 99}}})
		c.pets.register(uint32(14000 + i))
	}
	for i := 0; i < 10; i++ {
		next.HotelPets = append(next.HotelPets, game.Pet{ID: uint32(15000 + i), Slot: byte(i + 1), Name: "Hotel", Level: 3})
	}
	next.Pets[0].Battle = true
	next.ActivePet = 14000
	next.Pets[0].Equipment[2] = game.Item{ID: 10063, Count: 1, Damage: 4}
	outgoing := next.Pets[0]
	outgoing.Battle = false
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	before := c.character.Clone()
	for _, p := range [][]byte{{31, 3, 1}, {31, 2, 1}, {31, 4, 1, 1, 1, 11}} {
		if err := s.worldCommand(ctx, c, p); err != nil {
			t.Fatal(err)
		}
		packets := wire.packets(t)
		if len(packets) != 0 && (!contains(packets, []byte{31, 8}) || !contains(packets, []byte{31, 12})) {
			t.Fatal("failed exchange unlock", packets)
		}
		if !reflect.DeepEqual(before, *c.character) {
			t.Fatal("rejected request mutated state")
		}
	}
	if err := s.worldCommand(ctx, c, []byte{31, 4, 1, 1, 1, 1}); err != nil {
		t.Fatal(err)
	}
	got := wire.packets(t)
	if !contains(got, []byte{31, 5, 1, 1}) || !contains(got, []byte{31, 8}) || !contains(got, []byte{31, 12}) {
		t.Fatal("native copy/unlock", got)
	}
	if !contains(got, []byte{31, 3, 1, 1}) || !contains(got, protocol.Builder{15, 2}.U32(next.ID).U8(1)) || !contains(got, []byte{19, 2}) {
		t.Fatal("missing exchange packets", got)
	}
	if c.character.ActivePet != 0 || c.pets.slot(14000) != 0 || c.pets.slot(15000) != 1 {
		t.Fatal("active pet or client slots")
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	stored := chars[0]
	if len(stored.Pets) != 4 || len(stored.HotelPets) != 10 {
		t.Fatal("exchange capacity")
	}
	found := false
	for _, pet := range stored.HotelPets {
		if pet.ID == outgoing.ID {
			found = true
			if !reflect.DeepEqual(pet, outgoing) {
				t.Fatal("pet metadata lost", pet, outgoing)
			}
		}
	}
	if !found {
		t.Fatal("deposited pet missing")
	}
}

func TestNPCShopCatalogPurchaseAndAuthority(t *testing.T) {
	s, players, wires := tradeFixture(t)
	c := players[0]
	s.Assets.NPCBuyPrices = map[uint16]uint32{32176: 7}
	m := s.Assets.Maps[c.character.Map]
	m.Interactive = []assets.Group{{Header: []byte{3}, Members: []byte{1, 0xb0, 0x7d, 0, 0}}}
	s.Assets.Maps[c.character.Map] = m
	if err := s.openService(c, world.ServicePropsShop); err != nil {
		t.Fatal(err)
	}
	if got := wires[0].packets(t); len(got) != 1 || !bytes.Equal(got[0], []byte{35, 12, 133, 251, 1, 0}) {
		t.Fatal(got)
	}
	// The cart's byte slot resolves through the current map's authored catalog.
	tradeDo(t, s, c, []byte{27, 1, 1, 1, 3})
	if c.character.Gold != 79 || c.character.Bag[1].ID != 32176 || c.character.Bag[1].Count != 3 {
		t.Fatal("NPC purchase")
	}
	stored := tradeStored(t, s, players)
	if stored[0].Gold != 79 || stored[0].Bag != c.character.Bag {
		t.Fatal("purchase not durable")
	}
	before := c.character.Clone()
	wires[0].Reset()
	tradeDo(t, s, c, protocol.Builder{27, 1}.U16(21001).U8(1))
	if c.character.Gold != before.Gold || c.character.Bag != before.Bag || !contains(wires[0].packets(t), []byte{27, 1, 1}) {
		t.Fatal("unlisted item bought")
	}
	// A stale rich session cannot spend a balance already removed from SQL.
	if err := s.Store.UpdateCharacter(context.Background(), c.account.ID, c.character.ID, func(c *game.Character) error { c.Gold = 0; return nil }); err != nil {
		t.Fatal(err)
	}
	tradeDo(t, s, c, []byte{27, 1, 1, 1, 1})
	stored = tradeStored(t, s, players)
	if stored[0].Gold != 0 || stored[0].Bag != before.Bag || !contains(wires[0].packets(t), []byte{27, 1, 2}) {
		t.Fatal("purchase trusted cached balance")
	}
}

func TestFallbackServiceClassification(t *testing.T) {
	for _, tc := range []struct {
		n    world.NPC
		want byte
	}{
		{world.NPC{Template: 14134}, 4}, {world.NPC{Name: "Stock Keeper"}, world.ServiceBank},
		{world.NPC{Name: "Witch Doctor Hotel"}, world.ServiceClinic}, {world.NPC{Name: "PET KEEPER"}, 5},
		{world.NPC{Template: 13006}, 1}, {world.NPC{Template: 13005}, 2},
		{world.NPC{Name: "Villager"}, 0},
	} {
		if got := serviceKind(tc.n); got != tc.want {
			t.Fatal(tc, got)
		}
	}
}

func TestDoctorHotelChoiceWithoutDatabase(t *testing.T) {
	s := &Server{}
	wire := &captureConn{}
	c := &Session{conn: wire, character: &game.Character{Map: 10017}}
	handled, err := s.fallbackService(context.Background(), c, world.NPC{ClickID: 22, Template: 14151})
	if !handled || err != nil || !contains(wire.packets(t), serverwire.ServiceQuestion(22, 3, 1)) {
		t.Fatal("doctor menu", handled, err)
	}
	if err := s.eventCommand(context.Background(), c, []byte{20, 9, 32}); err != nil {
		t.Fatal(err)
	}
	got := wire.packets(t)
	if !contains(got, []byte{31, 7}) || !contains(got, []byte{6, 2, 0}) || c.event != nil {
		t.Fatal("hotel choice", got)
	}
	if err := s.eventCommand(context.Background(), c, []byte{20, 9, 32}); err != nil {
		t.Fatal(err)
	}
	if len(wire.packets(t)) != 0 {
		t.Fatal("late choice reopened hotel")
	}
}

func TestMerchantFallbackSeparatesBuyAndSell(t *testing.T) {
	for _, buy := range []bool{false, true} {
		name := "sell"
		if buy {
			name = "buy"
		}
		t.Run(name, func(t *testing.T) {
			s, players, wires := tradeFixture(t)
			c := players[0]
			handled, err := s.fallbackService(context.Background(), c, world.NPC{ClickID: 22, Template: 13007})
			if !handled || err != nil {
				t.Fatal(handled, err)
			}
			if err := c.event.onChoice(30); err != nil {
				t.Fatal(err)
			}
			wires[0].Reset()
			choice := byte(31)
			if buy {
				choice = 30
			}
			if err := c.event.onChoice(choice); err != nil {
				t.Fatal(err)
			}
			packets := wires[0].packets(t)
			hasBuy, hasSell := false, false
			for _, p := range packets {
				if len(p) >= 2 && p[0] == 35 && p[1] == 12 {
					hasBuy = true
				}
				if len(p) == 2 && p[0] == 27 && p[1] == 3 {
					hasSell = true
				}
			}
			if hasBuy != buy || hasSell == buy || c.event != nil || !buy && len(c.view.ShopItems) != 0 {
				t.Fatal("merchant choice opened the wrong service", packets)
			}
		})
	}
}

func TestShopSale(t *testing.T) {
	s, players, wires := worldFixture(t)
	ctx := context.Background()
	c := players[0]
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	s.Assets.SalePrices = map[uint16]assets.SalePrice{32176: {Price: 3}, 21001: {Price: 50, Flags: 16}}
	wires[0].Reset()
	sell := func(p ...byte) [][]byte {
		t.Helper()
		if err := s.worldCommand(ctx, c, append([]byte{27, 2}, p...)); err != nil {
			t.Fatal(err)
		}
		return wires[0].packets(t)
	}
	if got := sell(1, 1); len(got) != 1 || !bytes.Equal(got[0], []byte{27, 2, saleRejected}) {
		t.Fatal("sold without an open shop", got)
	}
	if err := s.openService(c, 2); err != nil {
		t.Fatal(err)
	}
	if got := wires[0].packets(t); !bytes.Equal(got[0], []byte{35, 12, 0x85, 0xfb, 1, 0}) {
		t.Fatal("props shop", got)
	}
	if got := sell(1, 0); !bytes.Equal(got[0], []byte{27, 2, saleRejected}) {
		t.Fatal("sold props in the equipment mode", got)
	}
	got := sell(1, 1)
	if len(got) != 3 || !bytes.Equal(got[0], []byte{23, 9, 1, 50}) || !bytes.Equal(got[1], protocol.Builder{26, 4}.U32(150)) || !bytes.Equal(got[2], []byte{27, 2, saleSuccess}) {
		t.Fatal("sale", got)
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Gold != 150 || !chars[0].Bag[0].Empty() {
		t.Fatal("sale not persisted", err)
	}
	if got = sell(1, 1); !bytes.Equal(got[0], []byte{27, 2, saleUnavailable}) {
		t.Fatal("sold an empty slot", got)
	}
}

func TestStorageTransfers(t *testing.T) {
	s, players, wires := worldFixture(t)
	ctx := context.Background()
	c := players[0]
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	do := func(p ...byte) [][]byte {
		t.Helper()
		if err := s.worldCommand(ctx, c, p); err != nil {
			t.Fatal(err)
		}
		return wires[0].packets(t)
	}
	// Deposit bag slot 1, then withdraw storage slot 1 into bag slot 7.
	got := do(30, 2, 1)
	if !bytes.Equal(got[0], []byte{23, 9, 1, 50}) || !bytes.Equal(got[1], []byte{30, 8}) || c.character.Storage[0] != (game.Item{ID: 32176, Count: 50}) || !c.character.Bag[0].Empty() {
		t.Fatal("deposit", got)
	}
	got = do(30, 5, 7, 1)
	if got[0][1] != 5 || got[0][2] != 7 || c.character.Bag[6].Count != 50 || !c.character.Storage[0].Empty() {
		t.Fatal("withdraw", got)
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Bag[6].Count != 50 {
		t.Fatal("storage not persisted", err)
	}
	// A failed move still answers with the storage snapshot.
	if got = do(30, 1, 9); len(got) != 4 || !bytes.Equal(got[0], []byte{30, 8}) {
		t.Fatal("empty withdraw", got)
	}
}

func TestBankServiceOpensNativeAtmWindow(t *testing.T) {
	s, players, wires := worldFixture(t)
	if err := s.openService(players[0], world.ServiceBank); err != nil {
		t.Fatal(err)
	}
	// Without an event the counter opens immediately.
	got := wires[0].packets(t)
	if len(got) != 2 || !bytes.Equal(got[0], []byte{29, 5}) || !bytes.Equal(got[1], []byte{29, 4, 0, 0, 0, 0}) {
		t.Fatal(got)
	}
}

func TestBankOpensAfterDialogueClose(t *testing.T) {
	s, players, wires := worldFixture(t)
	c := players[0]
	c.event = &eventSession{mapID: c.character.Map}
	if err := s.openService(c, world.ServiceBank); err != nil {
		t.Fatal(err)
	}
	if wires[0].Len() != 0 {
		t.Fatal("bank opened before the dialogue closed")
	}
	if err := s.finishEvent(c, c.event); err != nil {
		t.Fatal(err)
	}
	// Finish releases movement, resumes the dialogue, then opens the bank.
	// It must also work for clients that do not ACK a resume packet.
	got := wires[0].packets(t)
	if len(got) != 4 || got[0][0] != 6 || got[0][2] != 0 || got[1][0] != 20 ||
		!bytes.Equal(got[2], []byte{29, 5}) || !bytes.Equal(got[3], []byte{29, 4, 0, 0, 0, 0}) {
		t.Fatal(got)
	}
	if c.event != nil {
		t.Fatal("dialogue session stayed open and would block portals")
	}
	// A trailing acknowledgement is ignored without a wall-clock deadline.
	if err := s.eventCommand(context.Background(), c, []byte{protocol.CommandEvent, protocol.EventAcknowledge}); err != nil {
		t.Fatal(err)
	}
	if got := wires[0].packets(t); len(got) != 0 {
		t.Fatalf("acknowledgment loop not coalesced: %v", got)
	}
}

func TestMoneyBankDepositAndWithdraw(t *testing.T) {
	s, c, w := bankFixture(t)
	before := c.character.Clone()
	tradeDo(t, s, c, protocol.Builder{protocol.CommandLegacyStorage, protocol.MoneyBankDeposit}.U32(125))
	got := w[0].packets(t)
	if len(got) != 3 || got[0][0] != 26 || got[0][1] != 4 ||
		!bytes.Equal(got[1], []byte{29, protocol.MoneyBankBankBalance, 225, 0, 0, 0}) ||
		!bytes.Equal(got[2], []byte{29, protocol.MoneyBankDepositComplete}) {
		t.Fatal(got)
	}
	if c.character.BankGold != before.BankGold+125 || c.character.Gold != before.Gold-125 {
		t.Fatal(c.character.BankGold, c.character.Gold)
	}
	tradeDo(t, s, c, protocol.Builder{protocol.CommandLegacyStorage, protocol.MoneyBankWithdraw}.U32(50))
	got = w[0].packets(t)
	if len(got) != 3 || !bytes.Equal(got[1], []byte{29, protocol.MoneyBankBankBalance, 175, 0, 0, 0}) ||
		!bytes.Equal(got[2], []byte{29, protocol.MoneyBankWithdrawComplete}) {
		t.Fatal(got)
	}
	if c.character.BankGold != before.BankGold+75 || c.character.Gold != before.Gold-75 {
		t.Fatal(c.character.BankGold, c.character.Gold)
	}
	// An impossible amount is rejected natively and still unlocks the form.
	tradeDo(t, s, c, protocol.Builder{protocol.CommandLegacyStorage, protocol.MoneyBankWithdraw}.U32(1_000_000))
	got = w[0].packets(t)
	if len(got) != 3 || !bytes.Equal(got[0], []byte{29, protocol.MoneyBankRejected}) ||
		!bytes.Equal(got[2], []byte{29, protocol.MoneyBankWithdrawComplete}) {
		t.Fatal(got)
	}
	if c.character.BankGold != before.BankGold+75 {
		t.Fatal(c.character.BankGold)
	}
}

func TestClinicRest(t *testing.T) {
	s, players, wires := worldFixture(t)
	ctx := context.Background()
	c := players[0]
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	if err := s.openService(c, world.ServiceClinic); err != nil {
		t.Fatal(err)
	}
	if got := wires[0].packets(t); !bytes.Equal(got[0], protocol.Builder{31, 2}.U32(0xffffffff)) {
		t.Fatal("full HP still offered a rest", got)
	}
	c.character.HP = 10
	if err := s.openService(c, world.ServiceClinic); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	if err := s.worldCommand(ctx, c, []byte{31, 1}); err != nil {
		t.Fatal(err)
	}
	got := wires[0].packets(t)
	if len(got) != 18 || !bytes.Equal(got[17], []byte{31, 1, 0}) || c.character.HP != c.character.MaxHP {
		t.Fatal("rest", got)
	}
	if err := s.worldCommand(ctx, c, []byte{31, 1}); err != nil || wires[0].Len() != 0 {
		t.Fatal("rest confirmed twice", err)
	}
}

func TestMenuWarps(t *testing.T) {
	s, players, wires := worldFixture(t)
	ctx := context.Background()
	for _, id := range []uint16{11016, 11094} {
		s.Assets.Maps[id] = assets.Map{ID: id}
	}
	s.World = world.New(s.Assets)
	c := players[0]
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.worldCommand(ctx, c, []byte{5, 17, 3}); err != nil || c.character.Map != 11094 || c.carnieReturn == nil || c.carnieReturn.Map != 10017 {
		t.Fatal("carnie", c.character.Map, err)
	}
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	// Carnie's exit returns to where the visit began.
	c.arrived = false
	c.character.X, c.character.Y = 600, 600
	if err := s.worldCommand(ctx, c, []byte{20, 8, 1, 0}); err != nil || c.character.Map != 10017 || c.character.X != 1042 {
		t.Fatal("carnie exit", c.character.Map, err)
	}
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	// Without a record point, the record warp goes to the starter beach.
	if err := s.worldCommand(ctx, c, []byte{5, 17, 2}); err != nil || c.character.Map != 11016 {
		t.Fatal("record point fallback", c.character.Map, err)
	}
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	if err := s.worldCommand(ctx, c, []byte{5, 7}); err != nil {
		t.Fatal(err)
	}
	if got := wires[0].packets(t); len(got) != 1 || !bytes.Equal(got[0], protocol.Builder{5, 8}.U32(c.character.ID).U8(0)) {
		t.Fatal("sprite refresh", got)
	}
}

func TestParseShopCartNativeForms(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want []npcShopCartEntry
	}{
		{"counted pairs", []byte{2, 5, 2, 6, 3}, []npcShopCartEntry{{5, 2}, {6, 3}}},
		{"counted triples", []byte{1, 0x2c, 0x7d, 2}, []npcShopCartEntry{{32044, 2}}},
		{"plain triples", []byte{0x2c, 0x7d, 2}, []npcShopCartEntry{{32044, 2}}},
		{"plain pairs", []byte{5, 2, 6, 3}, []npcShopCartEntry{{5, 2}, {6, 3}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeNPCShopCart(tc.data)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("len=%d want %d: %#v", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("entry %d=%#v want %#v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestNPCShopCatalogUsesEveInteractiveSlots(t *testing.T) {
	s := &Server{Assets: &assets.Catalog{Maps: map[uint16]assets.Map{
		12000: {Interactive: []assets.Group{{Header: []byte{3, 0, 0}, Members: []byte{1, 0x2c, 0x7d, 0, 0, 2, 0x11, 0x7d, 0, 0}}}},
	}}}
	c := &Session{conn: &captureConn{}, character: &game.Character{Map: 12000}, view: world.NewView()}
	if err := s.openNPCShop(c, world.ServicePropsShop); err != nil {
		t.Fatal(err)
	}
	if c.view.ShopItems[1] != 32044 || c.view.ShopItems[2] != 32017 {
		t.Fatal(c.view.ShopItems)
	}
}

func TestNPCShopFallbackCatalogDoesNotAuthorizeUnlistedIDs(t *testing.T) {
	s, players, _ := tradeFixture(t)
	c := players[0]
	s.Assets.NPCShopDefaults = map[uint16][]uint16{world.ServiceWeaponShop: {17064}, world.ServicePropsShop: {32011, 32002}}
	if err := s.openService(c, world.ServiceWeaponShop); err != nil {
		t.Fatal(err)
	}
	if c.view.ShopItems[1] != 17064 {
		t.Fatal(c.view.ShopItems)
	}
	if err := s.openService(c, world.ServicePropsShop); err != nil {
		t.Fatal(err)
	}
	if c.view.ShopItems[2] != 32002 {
		t.Fatal(c.view.ShopItems)
	}
	s.Assets.Items[20001] = game.ItemDefinition{ID: 20001}
	before := c.character.Clone()
	if err := s.worldCommand(context.Background(), c, []byte{27, 1, 0x21, 0x4e, 1}); err != nil {
		t.Fatal(err)
	}
	if c.character.Gold != before.Gold || c.character.Bag != before.Bag {
		t.Fatal("unlisted explicit ID bypassed the active catalog")
	}
}

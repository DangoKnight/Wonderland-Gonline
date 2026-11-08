package server

import (
	"bytes"
	"context"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestItemCardLoginCatalogUsesNativeReplyCode(t *testing.T) {
	s, players, wires := gachaFixture(t)
	s.Assets.ItemCards = map[uint16]assets.ItemCard{34171: {Rewards: []assets.CardReward{{ItemID: 32176, Quantity: 1}}}}
	wires[0].Reset()
	if err := s.sendCardIndex(players[0]); err != nil {
		t.Fatal(err)
	}
	packets := wires[0].packets(t)
	// AC23:108 dispatches to FUN_001d6e54 (gift cards). AC23:133
	// dispatches to FUN_003325bc, opening a vehicle-stall buyer window.
	if len(packets) != 1 || !bytes.Equal(packets[0], []byte{23, 108, 1, 1, 0, 123, 133, 1}) {
		t.Fatal("login card index must not open a native stall", packets)
	}
}

func TestItemCardNativeCatalogRequest(t *testing.T) {
	s, players, wires := gachaFixture(t)
	s.Assets.ItemCards = map[uint16]assets.ItemCard{50001: {Rewards: []assets.CardReward{{ItemID: 32176, Quantity: 1}}}}
	// Relay capture: Legacy sends 90,1,1,236,0 followed by 236 UInt16
	// IDs (477 bytes). The third byte is a mode, not part of the count.
	request := []byte{90, 1, 1, 1, 0, 81, 195}
	wires[0].Reset()
	if err := s.dispatch(context.Background(), players[0], request); err != nil {
		t.Fatal("native login request disconnected the client", err)
	}
	packets := wires[0].packets(t)
	if len(packets) != 1 || !bytes.Equal(packets[0], []byte{23, 108, 2, 1, 0, 81, 195, 1, 1, 176, 125, 1}) {
		t.Fatal("native catalog response", packets)
	}
	for _, malformed := range [][]byte{
		{90, 1, 1, 0, 81, 195}, // Old Gonline layout without mode.
		{90, 1, 2, 1, 0, 81, 195},
		{90, 1, 1, 2, 0, 81, 195},
		{90, 1, 1, 1, 0, 81, 195, 0},
	} {
		if err := s.dispatch(context.Background(), players[0], malformed); err == nil {
			t.Fatal("accepted malformed catalog request", malformed)
		}
	}
}

func TestItemCardChoiceIsTransactional(t *testing.T) {
	for _, mode := range []string{"success", "full", "save-failure", "stale", "forged", "locked", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			s, players, wires := gachaFixture(t)
			c := players[0]
			s.Assets.ItemCards = map[uint16]assets.ItemCard{34171: {Rewards: []assets.CardReward{{ItemID: 32176, Quantity: 3}, {ItemID: 32177, Quantity: 2}}}}
			s.Assets.Items[32177] = game.ItemDefinition{ID: 32177, Name: "Second reward", Type: 23}
			next := c.character.Clone()
			next.Bag[8] = game.Item{ID: 34171, Count: 2}
			if mode == "full" {
				for i := range next.Bag {
					next.Bag[i] = game.Item{ID: 24001, Count: 1}
				}
				next.Bag[8] = game.Item{ID: 34171, Count: 2}
			}
			if mode == "locked" {
				next.Bag[8].Locked = true
			}
			if err := s.commit(context.Background(), c, next); err != nil {
				t.Fatal(err)
			}
			before := c.character.Bag
			if err := s.dispatch(context.Background(), c, []byte{23, 96, 9}); err != nil {
				t.Fatal(err)
			}
			if c.character.Bag != before {
				t.Fatal("opening choices consumed a card")
			}
			if mode == "locked" {
				return
			}
			if mode == "stale" {
				next = c.character.Clone()
				next.Bag[8].Damage++
				if err := s.commit(context.Background(), c, next); err != nil {
					t.Fatal(err)
				}
				before = c.character.Bag
			}
			request := []byte{23, 133, 2, 9, 1, 2, 177, 125}
			if mode == "forged" {
				request[6] = 176
			}
			if mode == "cancel" {
				c.itemCard = nil
				return
			}
			ctx := context.Background()
			if mode == "save-failure" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			wires[0].Reset()
			err := s.dispatch(ctx, c, request)
			if mode == "success" {
				if err != nil || c.character.Bag[8].Count != 1 || c.character.Bag[0] != (game.Item{ID: 32177, Count: 2}) {
					t.Fatal("exchange", err, c.character.Bag)
				}
				packets := wires[0].packets(t)
				if !bytes.Equal(packets[2], []byte{23, 108, 3, 123, 133, 1, 177, 125, 2}) {
					t.Fatal("redemption reply", packets)
				}
			} else if c.character.Bag != before {
				t.Fatal("failed exchange consumed resources", mode)
			}
			saved, loadErr := s.Store.Characters(context.Background(), c.account.ID)
			if loadErr != nil || saved[0].Bag != c.character.Bag {
				t.Fatal("SQL mismatch", loadErr)
			}
		})
	}
}
func TestItemCardAutomaticGenderAndSingleReward(t *testing.T) {
	for _, body := range []uint16{game.BodySmallMale, game.BodySmallFemale, game.BodyBigMale, game.BodyBigFemale} {
		s, players, _ := gachaFixture(t)
		c := players[0]
		next := c.character.Clone()
		next.Body = body
		if err := s.commit(context.Background(), c, next); err != nil {
			t.Fatal(err)
		}
		s.Assets.Items[32177] = game.ItemDefinition{ID: 32177, Type: 23}
		s.Assets.ItemCards = map[uint16]assets.ItemCard{34171: {MaleItem: 32176, FemaleItem: 32177, Rewards: []assets.CardReward{{ItemID: 32176, Quantity: 1}, {ItemID: 32177, Quantity: 1}}}}
		if err := s.dispatch(context.Background(), c, []byte{23, 133, 1, 9, 123, 133, 0}); err != nil {
			t.Fatal(err)
		}
		want := uint16(32176)
		if body == game.BodySmallFemale || body == game.BodyBigFemale {
			want = 32177
		}
		if c.character.Bag[0].ID != want || !c.character.Bag[8].Empty() {
			t.Fatal(body, c.character.Bag)
		}
	}
	s, players, _ := gachaFixture(t)
	c := players[0]
	s.Assets.ItemCards = map[uint16]assets.ItemCard{34171: {Rewards: []assets.CardReward{{ItemID: 32176, Quantity: 3}}}}
	if err := s.dispatch(context.Background(), c, []byte{23, 15, 9, 1, 0, 0}); err != nil || c.character.Bag[0].Count != 3 {
		t.Fatal("single reward", err, c.character.Bag)
	}
}

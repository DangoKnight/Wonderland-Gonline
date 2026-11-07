package server

import (
	"bytes"
	"context"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestItemCardChoiceIsTransactional(t *testing.T) {
	for _, mode := range []string{"success", "full", "save-failure", "stale", "forged", "locked", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			s, players, wires := gachaFixture(t)
			c := players[0]
			s.Assets.ItemCards = map[uint16]assets.ItemCard{34171: {Rewards: []assets.CardReward{{32176, 3}, {32177, 2}}}}
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
				if !bytes.Equal(packets[2], []byte{23, 133, 3, 123, 133, 1, 177, 125, 2}) {
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
		s.Assets.ItemCards = map[uint16]assets.ItemCard{34171: {MaleItem: 32176, FemaleItem: 32177, Rewards: []assets.CardReward{{32176, 1}, {32177, 1}}}}
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
	s.Assets.ItemCards = map[uint16]assets.ItemCard{34171: {Rewards: []assets.CardReward{{32176, 3}}}}
	if err := s.dispatch(context.Background(), c, []byte{23, 15, 9, 1, 0, 0}); err != nil || c.character.Bag[0].Count != 3 {
		t.Fatal("single reward", err, c.character.Bag)
	}
}

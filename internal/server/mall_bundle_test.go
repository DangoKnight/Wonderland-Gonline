package server

import (
	"context"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestGachaBundleAtomicDelivery(t *testing.T) {
	for _, mode := range []string{"success", "full", "save-failure"} {
		t.Run(mode, func(t *testing.T) {
			s, players, wires := gachaFixture(t)
			c := players[0]
			s.Assets.Items[32177] = game.ItemDefinition{ID: 32177, Name: "Second reward", Type: 23}
			s.Assets.GachaPacks[34171] = assets.GachaPack{ID: 34171, OpenAll: true, Rewards: []assets.GachaReward{{ID: 32176, Weight: 5000, Quantity: 3}, {ID: 32177, Weight: 5000, Quantity: 2}}}
			next := c.character.Clone()
			if mode == "full" {
				for i := range next.Bag {
					next.Bag[i] = game.Item{ID: 24001, Count: 1}
				}
				next.Bag[0] = game.Item{}
				next.Bag[8] = game.Item{ID: 34171, Count: 2}
			}
			if err := s.commit(context.Background(), c, next); err != nil {
				t.Fatal(err)
			}
			before := c.character.Bag
			ctx := context.Background()
			if mode == "save-failure" {
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}
			err := s.dispatch(ctx, c, []byte{23, 75, 9, 0})
			if mode == "success" {
				if err != nil || c.character.Bag[0] != (game.Item{ID: 32176, Count: 3}) || c.character.Bag[1] != (game.Item{ID: 32177, Count: 2}) || !c.character.Bag[8].Empty() {
					t.Fatal("bundle delivery", err, c.character.Bag)
				}
			} else {
				if c.character.Bag != before {
					t.Fatal("partial bundle consumption")
				}
				if mode == "save-failure" && (err == nil || wires[0].Len() != 0) {
					t.Fatal("failed save published success", err)
				}
			}
			chars, loadErr := s.Store.Characters(context.Background(), c.account.ID)
			if loadErr != nil || chars[0].Bag != c.character.Bag {
				t.Fatal("SQL bundle state", loadErr)
			}
		})
	}
}

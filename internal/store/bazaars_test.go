package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"wonderland-gonline/internal/game"
)

func TestBazaarAtomicStockRaceAndRestart(t *testing.T) {
	db, ref, _, items, path := manufacturingStoreFixture(t)
	ctx := context.Background()
	if err := db.UpdateCharacter(ctx, ref.Account, ref.ID, func(c *game.Character) error {
		c.Map = 10019
		c.X, c.Y = 100, 100
		c.Gold = 500
		c.Bag = game.Inventory{}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	b, err := db.SaveBazaar(ctx, Bazaar{Name: "Merchant", Title: "Stock", Map: 10019, X: 100, Y: 100, Kind: BazaarItems, Listings: []BazaarListing{{ItemID: 100, Quantity: 1, Price: 30}, {ItemID: 101, Quantity: 4, Price: 40}}})
	if err != nil {
		t.Fatal(err)
	}
	chars, _ := db.Characters(ctx, ref.Account)
	position := chars[0]
	bad := []BazaarOrder{{Index: 0, ItemID: 100, Count: 1, Price: 30}, {Index: 1, ItemID: 101, Count: 5, Price: 40}}
	if _, _, _, err = db.BuyBazaar(ctx, ref, b.VendorID, bad, items, nil, position); !errors.Is(err, ErrBazaarUnavailable) {
		t.Fatal(err)
	}
	check, _ := db.Bazaar(ctx, b.VendorID)
	if check.Listings[0].Quantity != 1 {
		t.Fatal("partial cart")
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _, err := db.BuyBazaar(ctx, ref, b.VendorID, []BazaarOrder{{Index: 0, ItemID: 100, Count: 1, Price: 30}}, items, nil, position)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatal("sold scarce unit twice", wins)
	}
	check, _ = db.Bazaar(ctx, b.VendorID)
	if check.Listings[0].Quantity != 0 || check.Listings[1].Quantity != 4 {
		t.Fatal("wrong stock rows", check)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	check, err = db.Bazaar(ctx, b.VendorID)
	chars, _ = db.Characters(ctx, ref.Account)
	if err != nil || check.Listings[0].Quantity != 0 || chars[0].Gold != 470 || chars[0].Bag[0].ID != 100 {
		t.Fatal("restart lost sale", check, err)
	}
}
func TestBazaarPetCapacityQuoteAndFullBagRollback(t *testing.T) {
	db, ref, _, items, _ := manufacturingStoreFixture(t)
	ctx := context.Background()
	if err := db.UpdateCharacter(ctx, ref.Account, ref.ID, func(c *game.Character) error {
		c.Map = 10019
		c.Gold = 200
		c.X, c.Y = 10, 10
		for i := range c.Bag {
			c.Bag[i] = game.Item{ID: 100, Count: 50}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	b, err := db.SaveBazaar(ctx, Bazaar{Name: "Merchant", Title: "Animals", Map: 10019, X: 10, Y: 10, Kind: BazaarPets, Listings: []BazaarListing{{ItemID: 17162, Quantity: 2, Price: 50}}})
	if err != nil {
		t.Fatal(err)
	}
	chars, _ := db.Characters(ctx, ref.Account)
	pos := chars[0]
	order := []BazaarOrder{{ItemID: 17162, Count: 1, Price: 50}}
	next, _, _, err := db.BuyBazaar(ctx, ref, b.VendorID, order, items, map[uint16]game.PetTemplate{17162: {}}, pos)
	if err != nil || len(next.Pets) != 1 || next.Gold != 150 {
		t.Fatal(next.Pets, err)
	}
	if _, _, _, err = db.BuyBazaar(ctx, ref, b.VendorID, order, items, map[uint16]game.PetTemplate{17162: {}}, pos); err == nil {
		t.Fatal("duplicate pet")
	}
	b.Kind = BazaarItems
	b.Listings = []BazaarListing{{ItemID: 101, Quantity: 1, Price: 50}}
	b, err = db.SaveBazaar(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = db.BuyBazaar(ctx, ref, b.VendorID, []BazaarOrder{{ItemID: 101, Count: 1, Price: 50}}, items, nil, pos); !errors.Is(err, game.ErrInventoryFull) {
		t.Fatal(err)
	}
	check, _ := db.Bazaar(ctx, b.VendorID)
	chars, _ = db.Characters(ctx, ref.Account)
	if check.Listings[0].Quantity != 1 || chars[0].Gold != 150 {
		t.Fatal("full bag lost stock/funds")
	}
}

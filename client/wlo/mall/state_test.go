package mall

import (
	"bytes"
	"reflect"
	"testing"
)

// Authored native catalog/checkout bytes, independent of production encoders.
var catalogGolden = []byte{75, 1, 2, 0, 154, 152, 1, 32, 0, 100, 1, 1, 1, 0, 155, 152, 2, 48, 0, 100, 2, 4, 2, 0}

func TestMallCatalogAndCheckoutGoldens(t *testing.T) {
	var s State
	if err := s.Apply(catalogGolden); err != nil {
		t.Fatal(err)
	}
	if len(s.Catalog[0]) != 2 || s.Catalog[0][1].ID != 39067 || s.Catalog[0][1].Count != 2 || s.Catalog[0][1].Price() != 48 {
		t.Fatal("catalog fields", s.Catalog)
	}
	if err := s.Apply([]byte{75, 7, 1}); err != nil {
		t.Fatal(err)
	}
	s.Add(s.Catalog[0][0])
	s.Quantity(0, 2)
	if got := s.RequestCheckout(); !bytes.Equal(got, []byte{34, 1, 1}) {
		t.Fatal("checkout query", got)
	}
	if !s.CheckoutBalance([]byte{35, 4, 100, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}) {
		t.Fatal("balance rejected")
	}
	if got := s.Submit(); !bytes.Equal(got, []byte{75, 1, 1, 154, 152, 1, 2, 1, 0}) {
		t.Fatal("purchase writer", got)
	}
	if s.Add(s.Catalog[0][1]) || s.RequestCheckout() != nil {
		t.Fatal("pending purchase editable")
	}
	if err := s.Apply([]byte{75, 4, 154, 152, 1, 2, 1, 0, 1}); err != nil || len(s.Cart) != 0 || s.Busy() {
		t.Fatal("receipt", s, err)
	}
	if err := s.Apply([]byte{75, 4, 154, 152, 1, 2, 1, 0, 1}); err != nil || len(s.Cart) != 0 {
		t.Fatal("duplicate receipt", err)
	}
}

func TestMallMalformedFramesPreserveStateAndFailedReceiptKeepsCart(t *testing.T) {
	var s State
	s.Apply(catalogGolden)
	s.Apply([]byte{75, 7, 1})
	s.Balances[0] = 100
	s.Add(s.Catalog[0][0])
	before := s
	for _, p := range [][]byte{catalogGolden[:len(catalogGolden)-1], {75, 1, 255, 255}, {75, 3, 1}, {75, 4, 1}, {75, 7}, {75, 8, 0}} {
		if err := s.Apply(p); err == nil || !reflect.DeepEqual(s, before) {
			t.Fatal("malformed frame changed state", p, err)
		}
	}
	s.Submit()
	s.Apply([]byte{75, 4, 154, 152, 1, 1, 1, 0, 0})
	if len(s.Cart) != 1 || s.Busy() {
		t.Fatal("failure lost cart or stayed locked", s)
	}
	s.Balances[0] = 0
	s.RequestCheckout()
	if s.Submit() != nil || len(s.Cart) != 1 || s.Busy() {
		t.Fatal("insufficient balance submitted")
	}
}

func TestMallBonusCatalogIsolationAndQuantityLimits(t *testing.T) {
	var s State
	s.Apply(catalogGolden)
	bonus := append([]byte(nil), catalogGolden...)
	bonus[1] = 10
	s.Apply(bonus)
	s.Enabled = true
	s.Bonus = true
	s.Balances[1] = 10000
	s.Add(s.Catalog[1][0])
	s.Quantity(0, 999)
	if s.Cart[0].Quantity != 50 || s.Add(s.Catalog[1][0]) {
		t.Fatal("quantity/duplicate limit")
	}
	if got := s.RequestCheckout(); !bytes.Equal(got, []byte{75, 5, 1, 154, 152, 1, 50, 1, 0}) {
		t.Fatal("bonus request", got)
	}
	// Catalog reload never unlocks an outstanding purchase.
	s.Apply(bonus)
	if !s.WaitingReceipt || len(s.Cart) != 1 {
		t.Fatal("catalog invalidated sent transaction")
	}
}

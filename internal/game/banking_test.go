package game

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

func TestBankingConservesCurrencyAndPersists(t *testing.T) {
	c := Character{Gold: 500, BankGold: 250}
	if !c.DepositGoldToBank(125) || c.Gold != 375 || c.BankGold != 375 {
		t.Fatal(c)
	}
	if !c.WithdrawGoldFromBank(300) || c.Gold != 675 || c.BankGold != 75 {
		t.Fatal(c)
	}
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var loaded Character
	if err := json.Unmarshal(raw, &loaded); err != nil || loaded.Gold != 675 || loaded.BankGold != 75 {
		t.Fatal(loaded, err)
	}
	var legacy Character
	if err := json.Unmarshal([]byte(`{"gold":123}`), &legacy); err != nil || legacy.Gold != 123 || legacy.BankGold != 0 {
		t.Fatal(legacy, err)
	}
}

func TestBankingRejectsInvalidAmountsWithoutChangingState(t *testing.T) {
	for _, test := range []struct {
		gold, bank, amount uint32
		withdraw           bool
	}{
		{500, 200, 0, false}, {500, 200, 501, false}, {1, math.MaxUint32, 1, false},
		{500, 200, 0, true}, {500, 200, 201, true}, {999999, 200, 1, true},
		{999998, 200, 2, true}, {0, math.MaxUint32, math.MaxUint32, true},
		{500, 200, math.MaxUint32, false},
	} {
		c := Character{Gold: test.gold, BankGold: test.bank}
		before := c
		changed := false
		if test.withdraw {
			changed = c.WithdrawGoldFromBank(test.amount)
		} else {
			changed = c.DepositGoldToBank(test.amount)
		}
		if changed || !reflect.DeepEqual(before, c) {
			t.Fatal(test, c)
		}
	}
	c := Character{Gold: 1, BankGold: math.MaxUint32 - 1}
	if !c.DepositGoldToBank(1) || c.BankGold != math.MaxUint32 || c.Gold != 0 {
		t.Fatal(c)
	}
	c = Character{Gold: 999998, BankGold: 1}
	if !c.WithdrawGoldFromBank(1) || c.BankGold != 0 || c.Gold != 999999 {
		t.Fatal(c)
	}
}

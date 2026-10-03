package battle

import (
	"math"
	"testing"
	"wonderland-go/internal/assets"
)

func TestDropRateMultiplier(t *testing.T) {
	for _, test := range []struct {
		name                   string
		multiplier, rate, roll float64
		want                   bool
	}{
		{"default", 0, 100, .36, false},
		{"default hits", 0, 100, .34, true},
		{"doubled", 2, 100, .69, true},
		{"doubled misses", 2, 100, .71, false},
		{"floor boundary", .1, 10, .05, true},
		{"floor misses", .1, 10, .051, false},
		{"certain", 100, 10, .999, true},
		{"zero rate stays disabled", 100, 0, 0, false},
		{"negative clamps", -1, 100, .06, false},
		{"nan defaults", math.NaN(), 100, .36, false},
		{"infinity defaults", math.Inf(1), 100, .36, false},
		{"invalid table rate", 100, math.NaN(), 0, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := Rules{DropRateMultiplier: test.multiplier, Float: func() float64 { return test.roll }}
			drops := r.RollDrops([]assets.Drop{{Item: 1, Min: 1, Max: 1, Rate: test.rate}}, [5]uint16{1}, func(uint16) bool { return true })
			if (len(drops) == 1) != test.want {
				t.Fatal(drops)
			}
		})
	}
}

func TestMultiplierPreservesDropRestrictions(t *testing.T) {
	r := Rules{DropRateMultiplier: 100, Float: func() float64 { return .99 }}
	table := []assets.Drop{{Item: 9, Min: 1, Max: 1, Rate: 100}, {Item: 1, Min: 1, Max: 1, Rate: 100}, {Item: 2, Min: 1, Max: 1, Rate: 100}}
	got := r.RollDrops(table, [5]uint16{1, 2}, func(id uint16) bool { return id != 1 })
	if len(got) != 1 || got[0].Item != 2 {
		t.Fatal("native/known/one-entry restrictions changed", got)
	}
}

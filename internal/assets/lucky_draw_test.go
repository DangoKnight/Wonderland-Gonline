package assets

import (
	"encoding/json"
	"math"
	"os"
	"testing"
	"wonderland-go/internal/game"
)

func TestLuckyDrawWeightedIntervals(t *testing.T) {
	rows := []byte(`[{"item_id":100,"quantity":1,"weight":2,"slot":1},{"item_id":100,"quantity":3,"weight":5,"slot":2},{"item_id":200,"quantity":1,"weight":3,"slot":3}]`)
	items := map[uint16]game.ItemDefinition{100: {ID: 100}, 200: {ID: 200}}
	pool, err := ParseLuckyDrawPool(rows, items)
	if err != nil || pool.TotalWeight != 10 {
		t.Fatal(pool, err)
	}
	counts := map[byte]int{}
	for roll := int64(0); roll < 10; roll++ {
		r, ok := pool.RewardForRoll(roll)
		if !ok {
			t.Fatal(roll)
		}
		counts[r.Slot]++
	}
	if counts[1] != 2 || counts[2] != 5 || counts[3] != 3 {
		t.Fatal(counts)
	}
	for _, roll := range []int64{-1, 10, math.MaxInt64} {
		if _, ok := pool.RewardForRoll(roll); ok {
			t.Fatal(roll)
		}
	}
}

func TestLuckyDrawRejectsInvalidWholePool(t *testing.T) {
	items := map[uint16]game.ItemDefinition{100: {ID: 100}}
	good := LuckyDrawReward{ID: 100, Quantity: 1, Weight: 1, Slot: 1}
	for _, bad := range []LuckyDrawReward{
		{ID: 999, Quantity: 1, Weight: 1, Slot: 2}, {ID: 100, Quantity: 0, Weight: 1, Slot: 2},
		{ID: 100, Quantity: 256, Weight: 1, Slot: 2}, {ID: 100, Quantity: 1, Weight: 0, Slot: 2},
		{ID: 100, Quantity: 1, Weight: -1, Slot: 2}, {ID: 100, Quantity: 1, Weight: 1, Slot: 0},
		{ID: 100, Quantity: 1, Weight: 1, Slot: 1}, {ID: 100, Quantity: 1, Weight: math.MaxInt64, Slot: 2},
	} {
		data, _ := json.Marshal([]LuckyDrawReward{good, bad})
		if pool, err := ParseLuckyDrawPool(data, items); err == nil || pool.TotalWeight != 0 || pool.Rewards != nil {
			t.Fatal(pool, err)
		}
	}
	for _, data := range []string{`null`, `[null]`, `[{"item_id":100,"quantity":1,"weight":1.5,"slot":1}]`, `[{"item_id":100,"quantity":1,"weight":1,"slot":256}]`} {
		if _, err := ParseLuckyDrawPool([]byte(data), items); err == nil {
			t.Fatal(data)
		}
	}
	if pool, err := ParseLuckyDrawPool([]byte(`[]`), items); err != nil || pool.TotalWeight != 0 {
		t.Fatal(err)
	}
}

func TestLuckyDrawAuthoredDefaultsMatchReference(t *testing.T) {
	data, err := os.ReadFile("lucky_draw_rules.json")
	if err != nil {
		t.Fatal(err)
	}
	var policy struct {
		Rewards []LuckyDrawReward `json:"rewards"`
	}
	if err := json.Unmarshal(data, &policy); err != nil {
		t.Fatal(err)
	}
	ids := []uint16{35114, 35115, 30436, 30436, 30436, 30437, 30437, 30437, 34087, 34147, 34085, 34269, 61062, 34136}
	quantities := []int{1, 1, 1, 3, 5, 1, 3, 5, 1, 1, 1, 1, 1, 1}
	if len(policy.Rewards) != 14 {
		t.Fatal(policy)
	}
	for i, r := range policy.Rewards {
		if r.ID != ids[i] || r.Quantity != quantities[i] || r.Weight != 1 || int(r.Slot) != i+1 {
			t.Fatal(r)
		}
	}
}

func TestLuckyDrawNativeSlotCapacityAndOrder(t *testing.T) {
	items := map[uint16]game.ItemDefinition{100: {ID: 100}}
	rows := make([]LuckyDrawReward, 20)
	for i := range rows {
		rows[i] = LuckyDrawReward{ID: 100, Quantity: 1, Weight: 1, Slot: byte(i + 1)}
	}
	data, _ := json.Marshal(rows)
	if _, err := ParseLuckyDrawPool(data, items); err != nil {
		t.Fatal("twenty native slots refused", err)
	}
	rows = append(rows, LuckyDrawReward{ID: 100, Quantity: 1, Weight: 1, Slot: 21})
	data, _ = json.Marshal(rows)
	if _, err := ParseLuckyDrawPool(data, items); err == nil {
		t.Fatal("overflow native catalog accepted")
	}
	for _, slots := range [][]byte{{1, 3}, {2, 1}, {1, 21}} {
		rows = rows[:2]
		rows[0].Slot, rows[1].Slot = slots[0], slots[1]
		data, _ = json.Marshal(rows)
		if _, err := ParseLuckyDrawPool(data, items); err == nil {
			t.Fatal("unrepresentable slots accepted", slots)
		}
	}
}

package assets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"wonderland-gonline/internal/game"
)

func gachaItemFixture() map[uint16]game.ItemDefinition {
	return map[uint16]game.ItemDefinition{34171: {ID: 34171}, 34172: {ID: 34172}, 34333: {ID: 34333}, 100: {ID: 100}, 200: {ID: 200}, 300: {ID: 300}}
}
func TestGachaConfigurationAndWeightIntervals(t *testing.T) {
	table := []byte(`[{"item_id":34171,"name":"metadata","rewards":[{"item_id":100,"weight":6000,"quantity":1},{"item_id":200,"weight":2500,"quantity":2},{"item_id":300,"weight":1500,"quantity":255}]}]`)
	packs, err := ParseGachaPacks(table, gachaItemFixture())
	if err != nil {
		t.Fatal(err)
	}
	counts := map[uint16]int{}
	for roll := 0; roll < 10000; roll++ {
		r, ok := packs[34171].RewardForRoll(roll)
		if !ok {
			t.Fatal(roll)
		}
		counts[r.ID]++
	}
	if counts[100] != 6000 || counts[200] != 2500 || counts[300] != 1500 {
		t.Fatal(counts)
	}
	for _, roll := range []int{-1, 10000} {
		if _, ok := packs[34171].RewardForRoll(roll); ok {
			t.Fatal("invalid roll", roll)
		}
	}
	c := &Catalog{GachaPacks: packs}
	if !c.GachaAvailable(34171) || c.GachaAvailable(34172) || c.GachaAvailable(34333) || !c.GachaAvailable(100) {
		t.Fatal("availability")
	}
}
func TestGachaRejectsWholeInvalidTable(t *testing.T) {
	valid := GachaPack{ID: 34171, Rewards: []GachaReward{{ID: 100, Weight: 10000, Quantity: 1}}}
	for _, kind := range []string{"unknown pack", "withdrawn pack", "unknown reward", "zero quantity", "large quantity", "zero weight", "large weight", "bad total", "empty rewards", "too many rewards", "duplicate pack"} {
		t.Run(kind, func(t *testing.T) {
			bad := GachaPack{ID: 34172, Rewards: []GachaReward{{ID: 200, Weight: 10000, Quantity: 1}}}
			switch kind {
			case "unknown pack":
				bad.ID = 1234
			case "withdrawn pack":
				bad.ID = 34333
			case "unknown reward":
				bad.Rewards[0].ID = 9999
			case "zero quantity":
				bad.Rewards[0].Quantity = 0
			case "large quantity":
				bad.Rewards[0].Quantity = 256
			case "zero weight":
				bad.Rewards[0].Weight = 0
			case "large weight":
				bad.Rewards[0].Weight = 10001
			case "bad total":
				bad.Rewards[0].Weight = 9999
			case "empty rewards":
				bad.Rewards = nil
			case "too many rewards":
				bad.Rewards = make([]GachaReward, 42)
			case "duplicate pack":
				bad.ID = 34171
			}
			data, _ := json.Marshal([]GachaPack{valid, bad})
			if packs, err := ParseGachaPacks(data, gachaItemFixture()); err == nil || packs != nil {
				t.Fatal("partially enabled invalid configuration", packs, err)
			}
		})
	}
	for _, text := range []string{`null`, `[null]`, `[{"item_id":34171,"rewards":[null]}]`, `[] {}`, `[{"item_id":65536}]`} {
		if _, err := ParseGachaPacks([]byte(text), gachaItemFixture()); err == nil {
			t.Fatal(text)
		}
	}
	if packs, err := ParseGachaPacks([]byte(`[]`), gachaItemFixture()); err != nil || len(packs) != 0 {
		t.Fatal("explicit empty table", err)
	}
}
func TestNativeGachaTables(t *testing.T) {
	dir := os.Getenv("WONDERLAND_TEST_DATA")
	if dir == "" {
		t.Skip("set WONDERLAND_TEST_DATA for native gacha tables")
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join("../..", dir)
	}
	c, err := Load(dir, filepath.Join("../..", "data/item_data.json"))
	if err != nil {
		t.Fatal(err)
	}
	// The maintained client-only item asset excludes server-only gacha IDs.
	if len(c.GachaPacks) != 0 || len(c.Warnings) != 1 || c.Warnings[0] != "gacha_packs.json: unknown gacha pack 34381" {
		t.Fatal(c.Summary())
	}
	data, err := os.ReadFile(filepath.Join(dir, "gacha_packs.json"))
	if err != nil {
		t.Fatal(err)
	}
	if packs, err := ParseGachaPacks(data, c.Items); err == nil || packs != nil {
		t.Fatal("unknown items enabled gacha")
	}
	// Preserve independent authored-table coverage without importing binary items.
	var records []GachaPack
	if err = json.Unmarshal(data, &records); err != nil {
		t.Fatal(err)
	}
	fixture := map[uint16]game.ItemDefinition{}
	for _, p := range records {
		fixture[p.ID] = game.ItemDefinition{ID: p.ID}
		for _, reward := range p.Rewards {
			fixture[reward.ID] = game.ItemDefinition{ID: reward.ID}
		}
	}
	packs, err := ParseGachaPacks(data, fixture)
	if err != nil || len(packs) != 24 {
		t.Fatal("authored pools", err)
	}
	total := 0
	for _, p := range packs {
		total += len(p.Rewards)
	}
	if total != 394 || packs[34171].Rewards[0] != (GachaReward{ID: 34105, Weight: 1278, Quantity: 1}) {
		t.Fatal("authored pool changed", total)
	}
}

func TestCompatibleGachaPoolsPreserveCompleteOdds(t *testing.T) {
	table := []byte(`[{"item_id":1234,"rewards":[{"item_id":100,"weight":6000,"quantity":1},{"item_id":200,"weight":4000,"quantity":2}]},{"item_id":34172,"rewards":[{"item_id":999,"weight":5000,"quantity":1},{"item_id":999,"weight":5000,"quantity":2}]}]`)
	items := gachaItemFixture()
	items[1234] = game.ItemDefinition{ID: 1234}
	packs, issues, err := ParseCompatibleGachaPacks(table, items)
	if err != nil || len(packs) != 1 || len(issues) != 1 || issues[0].ID != 34172 || len(issues[0].MissingItems) != 1 || issues[0].MissingItems[0] != 999 {
		t.Fatal(packs, issues, err)
	}
	counts := map[uint16]int{}
	for roll := 0; roll < 10000; roll++ {
		reward, ok := packs[1234].RewardForRoll(roll)
		if !ok {
			t.Fatal(roll)
		}
		counts[reward.ID]++
	}
	if counts[100] != 6000 || counts[200] != 4000 {
		t.Fatal(counts)
	}
	c := Catalog{GachaPacks: packs, UnavailableGachaPacks: map[uint16]GachaPackIssue{34172: issues[0], 4321: {ID: 4321}}}
	if !c.IsGachaPack(1234) || !c.GachaAvailable(1234) || !c.IsGachaPack(4321) || c.GachaAvailable(4321) || c.GachaAvailable(34172) {
		t.Fatal("classification")
	}
	if strict, err := ParseGachaPacks(table, items); err == nil || strict != nil {
		t.Fatal("strict validator accepted missing reward")
	}
}

func TestCompatibleGachaRejectsMalformedUnavailablePools(t *testing.T) {
	for _, table := range []string{
		`[{"item_id":34171,"rewards":[{"item_id":100,"weight":10000,"quantity":1}]},{"item_id":1234,"rewards":[{"item_id":999,"weight":9999,"quantity":1}]}]`,
		`[{"item_id":1234,"rewards":[{"item_id":999,"weight":10000,"quantity":1}]},{"item_id":1234,"rewards":[{"item_id":999,"weight":10000,"quantity":1}]}]`,
		`[{"item_id":0,"rewards":[{"item_id":100,"weight":10000,"quantity":1}]}]`,
	} {
		if packs, issues, err := ParseCompatibleGachaPacks([]byte(table), gachaItemFixture()); err == nil || packs != nil || issues != nil {
			t.Fatal(packs, issues, err)
		}
	}
}

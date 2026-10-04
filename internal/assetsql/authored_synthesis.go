package assetsql

import (
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"math"
	"strconv"
	"strings"
	"wonderland-go/internal/assetdb"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
)

func parseAuthoredSynthesis(text string) ([]assets.SynthesisRate, error) {
	var rates []assets.SynthesisRate
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		halves := strings.SplitN(line, "|", 2)
		if len(halves) != 2 {
			return nil, fmt.Errorf("invalid authored synthesis row")
		}
		inputs := strings.Split(halves[0], ",")
		outputs := strings.Split(halves[1], ",")
		if len(inputs) != 2 || len(outputs) < 3 {
			return nil, fmt.Errorf("missing authored recipe rate")
		}
		var ids [3]uint16
		for i, raw := range []string{inputs[0], inputs[1], outputs[0]} {
			id, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 16)
			if err != nil || id == 0 {
				return nil, fmt.Errorf("invalid authored recipe identity")
			}
			ids[i] = uint16(id)
		}
		rate, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(outputs[2]), "%"), 64)
		if err != nil || math.IsNaN(rate) || math.IsInf(rate, 0) || rate < 0 || rate > economyPercentScale {
			return nil, fmt.Errorf("invalid authored recipe chance")
		}
		var fee uint64
		if len(outputs) > 3 {
			fee, err = strconv.ParseUint(strings.TrimSpace(outputs[3]), 10, 32)
			if err != nil || fee > game.MaxGold {
				return nil, fmt.Errorf("invalid authored recipe fee")
			}
		}
		rates = append(rates, assets.SynthesisRate{Input1: ids[0], Input2: ids[1], Output: ids[2], SuccessPercent: rate, Fee: uint32(fee)})
	}
	return rates, nil
}

// Seed source fee/chance values only where the old seed is unchanged. Operators'
// edits retain precedence. This runs exclusively during offline conversion.
func applyAuthoredSynthesis(tx *gorm.DB, e *assets.Economy) error {
	if !tx.Migrator().HasTable(&assetdb.Document{}) {
		return nil
	}
	raw, err := assetdb.ReadDocument(tx, "alchemy_recipes.txt")
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	var doc databaseExport
	if err = json.Unmarshal(raw, &doc); err != nil {
		return err
	}
	authored, err := parseAuthoredSynthesis(doc.Text)
	if err != nil {
		return err
	}
	defaults, err := defaultEconomy()
	if err != nil {
		return err
	}
	key := func(r assets.SynthesisRate) [3]uint16 {
		return [3]uint16{min(r.Input1, r.Input2), max(r.Input1, r.Input2), r.Output}
	}
	seed := map[[3]uint16]assets.SynthesisRate{}
	for _, r := range defaults.Synthesis.Rates {
		seed[key(r)] = r
	}
	for _, r := range authored {
		found := false
		for i, current := range e.Synthesis.Rates {
			if key(current) == key(r) {
				found = true
				if baseline, ok := seed[key(r)]; ok && current.SuccessPercent == baseline.SuccessPercent && current.Fee == baseline.Fee {
					e.Synthesis.Rates[i] = r
				}
				break
			}
		}
		if !found {
			e.Synthesis.Rates = append(e.Synthesis.Rates, r)
		}
	}
	return validateEconomy(*e)
}

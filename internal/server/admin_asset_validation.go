package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

const maxMallWireIndex = math.MaxUint8
const dropRewardColumnCount = 5
const maxDropProbabilityPercent = 100

// Imported compatibility tables may contain unavailable content. Administrator
// edits must be usable immediately rather than silently dropping invalid rows.
func validateAdminAsset(asset string, raw []byte, candidate *assets.Catalog) error {
	switch asset {
	case "item_mall.json":
		for _, item := range candidate.Mall {
			if _, ok := candidate.Items[item.ID]; !ok || item.Cost > game.MaxMallPoints || item.CategoryID < 0 || item.CategoryID > maxMallWireIndex || item.Order < 0 || item.Order > maxMallWireIndex || (item.Bonus != 0 && item.Bonus != 1) {
				return fmt.Errorf("invalid mall item %d or native category/order range", item.ID)
			}
		}
	case "starter_items.json":
		for _, grant := range candidate.StarterItems {
			if _, ok := candidate.Items[grant.ID]; !ok {
				return fmt.Errorf("unknown starter item %d", grant.ID)
			}
		}
	case "monster_drops.txt":
		var doc struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			return err
		}
		for _, line := range strings.Split(doc.Text, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if !strings.HasPrefix(line, "TID:") {
				return errors.New("drop lines must start with TID:")
			}
			columns := strings.Split(line, "|")
			id, err := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(columns[0], "TID:")), 10, 16)
			if err != nil || id == 0 {
				return errors.New("invalid monster ID")
			}
			if _, ok := candidate.NPCs[uint16(id)]; !ok {
				return errors.New("unknown drop monster")
			}
			for _, reward := range columns[1:] {
				fields := strings.Split(strings.TrimSpace(reward), ",")
				if len(fields) != dropRewardColumnCount {
					return errors.New("drop rewards need item,name,min,max,percent")
				}
				item, e1 := strconv.ParseUint(strings.TrimSpace(fields[0]), 10, 16)
				low, e2 := strconv.ParseUint(strings.TrimSpace(fields[2]), 10, 8)
				high, e3 := strconv.ParseUint(strings.TrimSpace(fields[3]), 10, 8)
				rate, e4 := strconv.ParseFloat(strings.TrimSpace(fields[4]), 64)
				_, known := candidate.Items[uint16(item)]
				if e1 != nil || e2 != nil || e3 != nil || e4 != nil || !known || low == 0 || low > high || math.IsNaN(rate) || math.IsInf(rate, 0) || rate < 0 || rate > maxDropProbabilityPercent {
					return errors.New("invalid drop item, quantity or probability")
				}
			}
		}
	}
	return nil
}

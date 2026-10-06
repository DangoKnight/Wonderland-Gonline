package assets

import (
	"fmt"
	"wonderland-gonline/internal/game"
)

const ManufacturingMaterialLimit = 5
const ManufacturingMinutesToSeconds = 60
const ManufacturingMaxDurationSeconds = 7 * 24 * 60 * 60

type ManufacturingFormula struct {
	ID              uint16
	Output          ManufacturingInput
	Inputs          [ManufacturingMaterialLimit]ManufacturingInput
	PlanID, ToolID  uint16
	DurationSeconds uint32
	TentOutput      bool
}
type RebornClass struct {
	Job     byte
	Name    string
	CapeID  uint16
	Enabled bool
}

func ValidateManufacturing(values map[uint16]ManufacturingFormula, items map[uint16]game.ItemDefinition) error {
	for id, r := range values {
		if id != r.ID || r.Output.ItemID == 0 || r.Output.Count == 0 || r.DurationSeconds > ManufacturingMaxDurationSeconds {
			return fmt.Errorf("invalid manufacturing formula %d", id)
		}
		if _, ok := items[r.Output.ItemID]; !ok {
			return fmt.Errorf("unknown manufacturing output %d", r.Output.ItemID)
		}
		count := 0
		for _, in := range r.Inputs {
			if (in.ItemID == 0) != (in.Count == 0) {
				return fmt.Errorf("invalid manufacturing ingredient")
			}
			if in.ItemID != 0 {
				count++
				if _, ok := items[in.ItemID]; !ok {
					return fmt.Errorf("unknown manufacturing ingredient %d", in.ItemID)
				}
			}
		}
		if count == 0 {
			return fmt.Errorf("manufacturing needs materials")
		}
		for _, required := range []uint16{r.PlanID, r.ToolID} {
			if required != 0 {
				if _, ok := items[required]; !ok {
					return fmt.Errorf("unknown manufacturing plan/tool %d", required)
				}
			}
		}
	}
	return nil
}

package assetsql

import (
	_ "embed"
	"encoding/json"
	"gorm.io/gorm"
	"math"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

// Original Compound2 ordinals are the native formula IDs; never compress them.
func importedManufacturing(tx *gorm.DB, items map[uint16]game.ItemDefinition) (map[uint16]assets.ManufacturingFormula, error) {
	out := map[uint16]assets.ManufacturingFormula{}
	if !tx.Migrator().HasTable(&assetdb.Record{}) {
		return out, nil
	}
	var rows []assetdb.Record
	if err := tx.Where("asset = ? AND collection = ?", "compound2.dat", "records").Order("ordinal").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		var record exportRow
		if err := json.Unmarshal([]byte(row.JSON), &record); err != nil {
			return nil, err
		}
		var fields map[string]uint32
		if err := json.Unmarshal(record.Fields, &fields); err != nil {
			return nil, err
		}
		if record.RecordIndex < 0 || record.RecordIndex > math.MaxUint16 {
			continue
		}
		if !manufacturingFieldsFit(fields) {
			continue
		}
		f := assets.ManufacturingFormula{ID: uint16(record.RecordIndex), Output: assets.ManufacturingInput{ItemID: uint16(fields["result_id"]), Count: byte(fields["result_count"])}, PlanID: uint16(fields["plan_id"]), ToolID: uint16(fields["tool_id"]), DurationSeconds: fields["build_time"] * assets.ManufacturingMinutesToSeconds}
		for i := range f.Inputs {
			idKey, countKey := manufacturingMaterialKeys[i][0], manufacturingMaterialKeys[i][1]
			f.Inputs[i] = assets.ManufacturingInput{ItemID: uint16(fields[idKey]), Count: byte(fields[countKey])}
		}
		if f.Output.Count == 0 {
			f.Output.Count = 1
		}
		def, known := items[f.Output.ItemID]
		if !known {
			continue
		}
		f.TentOutput = def.Type >= manufacturingFurnitureType && def.Type <= manufacturingDecorationType
		for _, id := range handToolOutputs {
			if f.Output.ItemID == id {
				f.TentOutput = false
			}
		}
		if assets.ValidateManufacturing(map[uint16]assets.ManufacturingFormula{f.ID: f}, items) != nil {
			continue
		} // Native headers/obsolete IDs are not executable recipes.
		out[f.ID] = f
	}
	return out, assets.ValidateManufacturing(out, items)
}

const manufacturingFurnitureType = 28
const manufacturingDecorationType = 30

var manufacturingMaterialKeys = [assets.ManufacturingMaterialLimit][2]string{{"material_1_id", "material_1_count"}, {"material_2_id", "material_2_count"}, {"material_3_id", "material_3_count"}, {"material_4_id", "material_4_count"}, {"material_5_id", "material_5_count"}}

// Source-authored portable tool exceptions; offline content projection only.
var handToolOutputs = []uint16{38004, 38031, 38035, 38036, 38037, 38040, 38054, 38058}

//go:embed reborn_defaults.json
var rebornDefaults []byte

func defaultRebornClasses(items map[uint16]game.ItemDefinition) ([]assets.RebornClass, error) {
	var classes []assets.RebornClass
	if err := json.Unmarshal(rebornDefaults, &classes); err != nil {
		return nil, err
	}
	for i, c := range classes {
		if _, ok := items[c.CapeID]; !ok {
			classes[i].Enabled = false
		}
	}
	return classes, nil
}

// Validate source widths before narrowing: malformed raw metadata must not wrap
// into a different, valid recipe or a much shorter timer.
func manufacturingFieldsFit(fields map[string]uint32) bool {
	for _, key := range []string{"result_id", "plan_id", "tool_id"} {
		if fields[key] > math.MaxUint16 {
			return false
		}
	}
	if fields["result_count"] > math.MaxUint8 || fields["build_time"] > assets.ManufacturingMaxDurationSeconds/assets.ManufacturingMinutesToSeconds {
		return false
	}
	for _, pair := range manufacturingMaterialKeys {
		if fields[pair[0]] > math.MaxUint16 || fields[pair[1]] > math.MaxUint8 {
			return false
		}
	}
	return true
}

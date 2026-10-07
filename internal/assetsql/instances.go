package assetsql

import (
	"encoding/json"
	"gorm.io/gorm"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
)

// Offline projection only. Runtime reads use catalog_instances.
func importedInstances(tx *gorm.DB) ([]assets.InstanceDefinition, error) {
	if !tx.Migrator().HasTable(&assetdb.Record{}) {
		return []assets.InstanceDefinition{}, nil
	}
	var rows []assetdb.Record
	if err := tx.Where("asset = ? AND collection = ?", "scenedata.dat", "records").Order("ordinal").Find(&rows).Error; err != nil {
		return nil, err
	}
	records := make([]json.RawMessage, 0, len(rows))
	for _, r := range rows {
		records = append(records, json.RawMessage(r.JSON))
	}
	raw, err := json.Marshal(struct {
		Records []json.RawMessage `json:"records"`
	}{records})
	if err != nil {
		return nil, err
	}
	defs, err := assets.ParseInstanceDefinitions(raw)
	if err != nil {
		return nil, err
	}
	var marks []assetdb.Record
	if err = tx.Where("asset = ? AND collection = ?", "mark.dat", "records").Find(&marks).Error; err != nil {
		return nil, err
	}
	records = records[:0]
	for _, r := range marks {
		records = append(records, json.RawMessage(r.JSON))
	}
	raw, err = json.Marshal(struct {
		Records []json.RawMessage `json:"records"`
	}{records})
	if err != nil {
		return nil, err
	}
	if err = assets.ApplyInstanceText(defs, raw); err != nil {
		return nil, err
	}
	return defs, nil
}

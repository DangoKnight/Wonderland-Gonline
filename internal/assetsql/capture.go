package assetsql

import (
	"encoding/json"
	"gorm.io/gorm"
	"wonderland-gonline/internal/assetdb"
)

// Offline migration only: project retained imported NPC metadata into its typed
// column without replacing edited names, stats, skills or other definitions.
func migrateCaptureRestrictions(tx *gorm.DB) error {
	if !tx.Migrator().HasTable(&NPCsRow{}) {
		return nil
	}
	if tx.Migrator().HasColumn(&NPCsRow{}, "ValueCaptureRestriction") {
		return nil
	}
	if err := tx.Migrator().AddColumn(&NPCsRow{}, "ValueCaptureRestriction"); err != nil {
		return err
	}
	if !tx.Migrator().HasTable(&assetdb.Record{}) {
		return nil
	}
	var records []assetdb.Record
	if err := tx.Where("asset = ? AND collection = ?", "npc.dat", "records").Order("ordinal").Find(&records).Error; err != nil {
		return err
	}
	for _, record := range records {
		var row struct {
			Fields struct {
				ID          uint16 `json:"id"`
				Restriction byte   `json:"unknown_u8_offset_34"`
			} `json:"fields"`
		}
		if err := json.Unmarshal([]byte(record.JSON), &row); err != nil {
			return err
		}
		result := tx.Model(&NPCsRow{}).Where("n_p_cs_key = ?", row.Fields.ID).Update("value_capture_restriction", row.Fields.Restriction)
		if result.Error != nil {
			return result.Error
		}
	}
	return nil
}

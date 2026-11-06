package assetsql

import (
	"encoding/binary"
	"fmt"

	"gorm.io/gorm"
	"wonderland-gonline/internal/assets"
)

// v14 projects previously unrepresented routes, signs and speeds from SQL map
// sections. Match click IDs rather than ordinals so edited/reordered NPCs keep
// their definitions, walking steps and positions. Runtime reads only these tables.
func migrateNPCRoutes(tx *gorm.DB) error {
	missing := map[string]bool{}
	for _, column := range []string{"ValueRoutesPresent", "ValueSign", "ValueSpeed"} {
		if !tx.Migrator().HasColumn(&MapsNPCsRow{}, column) {
			missing[column] = true
			if err := tx.Migrator().AddColumn(&MapsNPCsRow{}, column); err != nil {
				return err
			}
		}
	}
	for _, model := range []any{&MapsNPCsRoutesRow{}, &MapsNPCsRoutesValuesRow{}} {
		if !tx.Migrator().HasTable(model) {
			if err := tx.Migrator().CreateTable(model); err != nil {
				return err
			}
		}
	}
	if len(missing) == 0 {
		return nil
	}
	var maps []MapsRow
	if err := tx.Find(&maps).Error; err != nil {
		return err
	}
	for _, row := range maps {
		if len(row.ValueSections0) == 0 {
			continue
		}
		// An EVE map ends in eleven uint32 section offsets. Offset zero means
		// absent, so place its retained NPC section after a single padding byte.
		const offsetBytes = 11 * 4
		data := make([]byte, 1+len(row.ValueSections0)+offsetBytes)
		copy(data[1:], row.ValueSections0)
		binary.LittleEndian.PutUint32(data[len(data)-offsetBytes:], 1)
		native, err := assets.ParseEVEMap(row.MapsKey, row.ValueScene, data)
		if err != nil {
			return fmt.Errorf("project map %d NPC routes: %w", row.MapsKey, err)
		}
		byClick := map[uint16]assets.MapNPC{}
		for _, npc := range native.NPCs {
			byClick[npc.ClickID] = npc
		}
		var npcs []MapsNPCsRow
		if err := tx.Where("maps_key = ?", row.MapsKey).Find(&npcs).Error; err != nil {
			return err
		}
		for _, npc := range npcs {
			original, ok := byClick[npc.ValueClickID]
			if !ok {
				continue
			}
			updates := map[string]any{}
			if missing["ValueSign"] {
				updates["value_sign"] = original.Sign
			}
			if missing["ValueSpeed"] {
				updates["value_speed"] = original.Speed
			}
			if missing["ValueRoutesPresent"] && original.Routes != nil {
				if err := writeMapsNPCsRoutes(tx, original.Routes, row.MapsKey, npc.MapsNPCsOrdinal); err != nil {
					return err
				}
				updates["value_routes_present"] = true
			}
			if len(updates) == 0 {
				continue
			}
			if err := tx.Model(&MapsNPCsRow{}).Where("maps_key = ? AND maps_n_p_cs_ordinal = ?", row.MapsKey, npc.MapsNPCsOrdinal).Updates(updates).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

package assetsql

import (
	"gorm.io/gorm"
	"wonderland-gonline/internal/assets"
)

// Explicit offline migration only; runtime never reads the compiled seed table.
func migrateNPCNames(tx *gorm.DB) error {
	patches, err := assets.NPCNamePatches()
	if err != nil {
		return err
	}
	for id, patch := range patches {
		names := []string{patch.OriginalName}
		if patch.PreviousName != "" {
			names = append(names, patch.PreviousName)
		}
		if err := tx.Model(&NPCsRow{}).Where("n_p_cs_key = ? AND value_name IN ?", id, names).Update("value_name", patch.Name).Error; err != nil {
			return err
		}
	}
	return nil
}

func initializeNPCNames(c *assets.Catalog) error {
	patches, err := assets.NPCNamePatches()
	if err != nil {
		return err
	}
	for id, patch := range patches {
		if npc, ok := c.NPCs[id]; ok {
			npc.Name = patch.Apply(npc.Name)
			c.NPCs[id] = npc
		}
	}
	return nil
}

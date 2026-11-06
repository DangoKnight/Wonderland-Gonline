package assetsql

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"wonderland-gonline/internal/assets"
)

//go:embed skill_evolutions_defaults.json
var skillEvolutionsDefaults []byte

func defaultSkillEvolutions() ([]assets.SkillEvolution, error) {
	var values []assets.SkillEvolution
	if err := json.Unmarshal(skillEvolutionsDefaults, &values); err != nil {
		return values, err
	}
	return values, validateSkillEvolutions(values)
}

func validateSkillEvolutions(values []assets.SkillEvolution) error {
	seen := map[[2]uint16]bool{}
	for _, e := range values {
		key := [2]uint16{e.FromSkill, e.ToSkill}
		if e.FromSkill == 0 || e.ToSkill == 0 || e.FromSkill == e.ToSkill || e.RequiredGrade == 0 || seen[key] {
			return fmt.Errorf("invalid skill evolution %v", key)
		}
		seen[key] = true
	}
	return nil
}

func migrateSkillEvolutions(tx *gorm.DB) error {
	return migrateCatalogDataset(tx, "catalog_skill_evolutions", "skill_evolutions", func() error {
		values, err := defaultSkillEvolutions()
		if err != nil {
			return err
		}
		return writeSkillEvolutions(tx, values)
	})
}

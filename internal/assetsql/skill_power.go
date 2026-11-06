package assetsql

import (
	_ "embed"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"wonderland-gonline/internal/assets"
)

//go:embed skill_power_defaults.csv
var skillPowerDefaults string

type skillPowerDefault struct {
	power          float64
	element, layer byte
}

func defaultSkillPowers() (map[uint16]skillPowerDefault, error) {
	rows, err := csv.NewReader(strings.NewReader(skillPowerDefaults)).ReadAll()
	if err != nil {
		return nil, err
	}
	values := map[uint16]skillPowerDefault{}
	for _, row := range rows[1:] {
		if len(row) != 8 {
			return nil, fmt.Errorf("invalid skill power default row")
		}
		id, errID := strconv.ParseUint(row[0], 10, 16)
		power, errPower := strconv.ParseUint(row[1], 10, 16)
		element, errElement := strconv.ParseUint(row[2], 10, 8)
		layer, errLayer := strconv.ParseUint(row[3], 10, 8)
		if errID != nil || errPower != nil || errElement != nil || errLayer != nil || id == 0 {
			return nil, fmt.Errorf("invalid skill power default %q", row[0])
		}
		if _, exists := values[uint16(id)]; exists {
			return nil, fmt.Errorf("duplicate skill power default %d", id)
		}
		values[uint16(id)] = skillPowerDefault{float64(power), byte(element), byte(layer)}
	}
	return values, nil
}

// Seed only unresolved powers on matching native definitions; preserve explicit
// zero and every existing administrator power. No source files are read at runtime.
func seedSkillPowers(c *assets.Catalog) error {
	defaults, err := defaultSkillPowers()
	if err != nil {
		return err
	}
	for id, d := range defaults {
		s, exists := c.Skills[id]
		if exists && s.BasePower == nil && s.Element == d.element && s.EffectLayer == d.layer {
			power := d.power
			s.BasePower = &power
			c.Skills[id] = s
		}
	}
	return nil
}

func addSkillPowerColumns(tx *gorm.DB) error {
	for _, column := range []string{"ValueBasePowerPresent", "ValueBasePower"} {
		if !tx.Migrator().HasColumn(&SkillsRow{}, column) {
			if err := tx.Migrator().AddColumn(&SkillsRow{}, column); err != nil {
				return err
			}
		}
	}
	return nil
}

func migrateSkillPowers(tx *gorm.DB) error {
	defaults, err := defaultSkillPowers()
	if err != nil {
		return err
	}
	for id, d := range defaults {
		if err := tx.Model(&SkillsRow{}).
			Where("skills_key = ? AND value_element = ? AND value_effect_layer = ? AND (value_base_power_present IS NULL OR value_base_power_present = ?)", id, d.element, d.layer, false).
			Updates(map[string]any{"value_base_power_present": true, "value_base_power": d.power}).Error; err != nil {
			return err
		}
	}
	return nil
}

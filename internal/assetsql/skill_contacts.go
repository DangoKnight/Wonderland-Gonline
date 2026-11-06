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

//go:embed skill_contact_defaults.csv
var skillContactDefaults string

func defaultSkillContacts() (map[uint16]byte, error) {
	rows, err := csv.NewReader(strings.NewReader(skillContactDefaults)).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 || len(rows[0]) != 2 || rows[0][0] != "animation_id" || rows[0][1] != "contact_events" {
		return nil, fmt.Errorf("invalid skill contact defaults header")
	}
	counts := map[uint16]byte{}
	for _, row := range rows[1:] {
		if len(row) != 2 {
			return nil, fmt.Errorf("invalid skill contact default")
		}
		id, idErr := strconv.ParseUint(row[0], 10, 16)
		count, countErr := strconv.ParseUint(row[1], 10, 8)
		if idErr != nil || countErr != nil || id == 0 || count == 0 || count > assets.MaxSkillContactEvents || counts[uint16(id)] != 0 {
			return nil, fmt.Errorf("invalid or duplicate skill contact default %q", row[0])
		}
		counts[uint16(id)] = byte(count)
	}
	return counts, nil
}

func seedSkillContacts(c *assets.Catalog) error {
	counts, err := defaultSkillContacts()
	if err != nil {
		return err
	}
	assets.ApplySkillContacts(c.Skills, counts)
	return nil
}

// Explicit initialization/migration uses repository-owned definitions. No
// current client installation or retained MBTM export is required.
func migrateSkillContacts(tx *gorm.DB) error {
	counts, err := defaultSkillContacts()
	if err != nil {
		return err
	}
	for id, count := range counts {
		if err := tx.Model(&SkillsRow{}).Where("skills_key = ? AND COALESCE(value_contact_events, 0) = 0", id).Update("value_contact_events", count).Error; err != nil {
			return err
		}
	}
	return nil
}

func addSkillContactColumn(tx *gorm.DB) error {
	if !tx.Migrator().HasColumn(&SkillsRow{}, "ValueContactEvents") {
		return tx.Migrator().AddColumn(&SkillsRow{}, "ValueContactEvents")
	}
	return nil
}

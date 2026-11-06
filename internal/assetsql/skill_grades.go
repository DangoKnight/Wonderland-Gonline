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

//go:embed skill_grade_defaults.csv
var skillGradeDefaults string

func defaultSkillGrades() (map[uint16]byte, error) {
	rows, err := csv.NewReader(strings.NewReader(skillGradeDefaults)).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 || len(rows[0]) != 2 || rows[0][0] != "skill_id" || rows[0][1] != "maximum_grade" {
		return nil, fmt.Errorf("invalid skill grade defaults header")
	}
	values := map[uint16]byte{}
	for _, row := range rows[1:] {
		if len(row) != 2 {
			return nil, fmt.Errorf("invalid skill grade default")
		}
		id, idErr := strconv.ParseUint(row[0], 10, 16)
		grade, gradeErr := strconv.ParseUint(row[1], 10, 8)
		_, duplicate := values[uint16(id)]
		if idErr != nil || gradeErr != nil || id == 0 || duplicate {
			return nil, fmt.Errorf("invalid or duplicate skill grade default %q", row[0])
		}
		values[uint16(id)] = byte(grade)
	}
	return values, nil
}

func seedSkillGrades(c *assets.Catalog) error {
	values, err := defaultSkillGrades()
	if err != nil {
		return err
	}
	for id, skill := range c.Skills {
		if grade, exists := values[id]; skill.MaximumGrade == nil && exists {
			skill.MaximumGrade = assets.GradeLimit(grade)
			c.Skills[id] = skill
		}
	}
	return nil
}

func migrateSkillGrades(tx *gorm.DB) error {
	values, err := defaultSkillGrades()
	if err != nil {
		return err
	}
	for id, grade := range values {
		if err := tx.Model(&SkillsRow{}).Where("skills_key = ? AND COALESCE(value_maximum_grade_present, 0) = 0", id).Updates(map[string]any{"value_maximum_grade": grade, "value_maximum_grade_present": true}).Error; err != nil {
			return err
		}
	}
	return nil
}

func addSkillGradeColumn(tx *gorm.DB) error {
	for _, column := range []string{"ValueMaximumGradePresent", "ValueMaximumGrade"} {
		if !tx.Migrator().HasColumn(&SkillsRow{}, column) {
			if err := tx.Migrator().AddColumn(&SkillsRow{}, column); err != nil {
				return err
			}
		}
	}
	return nil
}

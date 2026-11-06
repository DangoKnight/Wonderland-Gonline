package assetsql

import (
	"path/filepath"
	"testing"
	"wonderland-gonline/internal/assetdb"
)

func TestSkillGradeMigrationWithoutClientPreservesEditsAndRollsBack(t *testing.T) {
	db, err := assetdb.Open(filepath.Join(t.TempDir(), "grades.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	if err := db.AutoMigrate(&SkillsRow{}, &catalogSchema{}); err != nil {
		t.Fatal(err)
	}
	// Hagendis Beat has a native cap of 15; Flame Attack has a native cap of 5.
	if err := db.Create(&[]SkillsRow{
		{SkillsKey: 15045, ValueID: 15045, ValueName: "Keep"},
		{SkillsKey: 11016, ValueID: 11016, ValueMaximumGradePresent: true, ValueMaximumGrade: 7},
		{SkillsKey: 11166, ValueID: 11166, ValueMaximumGradePresent: true, ValueMaximumGrade: 0},
		{SkillsKey: 65535, ValueID: 65535},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&catalogSchema{ID: catalogMetadataID, Version: 56}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TRIGGER fail_grades BEFORE UPDATE OF value_maximum_grade ON catalog_skills BEGIN SELECT RAISE(ABORT, 'forced grade failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(migrateCatalog); err == nil {
		t.Fatal("failed grade migration accepted")
	}
	var schema catalogSchema
	db.First(&schema, catalogMetadataID)
	var row SkillsRow
	db.First(&row, "skills_key = ?", 15045)
	if schema.Version != 56 || row.ValueMaximumGrade != 0 {
		t.Fatal("migration did not roll back", schema, row)
	}
	if err := db.Exec("DROP TRIGGER fail_grades").Error; err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{"ValueMaximumGradePresent", "ValueMaximumGrade"} {
		if err := db.Migrator().DropColumn(&SkillsRow{}, column); err != nil {
			t.Fatal(err)
		}
	}
	if err := addSkillGradeColumn(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&SkillsRow{}).Where("skills_key = ?", 11016).Updates(map[string]any{"value_maximum_grade": 7, "value_maximum_grade_present": true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&SkillsRow{}).Where("skills_key = ?", 11166).Updates(map[string]any{"value_maximum_grade": 0, "value_maximum_grade_present": true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
	loaded, err := readSkills(db)
	if err != nil || loaded[15045].GradeLimit() != 15 || loaded[15045].Name != "Keep" || loaded[11016].GradeLimit() != 7 || loaded[11166].GradeLimit() != 0 || loaded[65535].MaximumGrade != nil || loaded[65535].GradeLimit() != 10 {
		t.Fatal("grade defaults or preserved values differ", loaded, err)
	}
	if err := db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
}

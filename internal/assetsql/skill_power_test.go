package assetsql

import (
	"path/filepath"
	"testing"

	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
)

func TestSkillPowerDefaultsAndPreservingMigration(t *testing.T) {
	const blastAttack, flameAttack, windAttack, rocaTulwar = 11166, 11016, 11007, 15072
	defaults, err := defaultSkillPowers()
	if err != nil || len(defaults) != 224 || defaults[blastAttack].power != 20 || defaults[flameAttack].power != 20 || defaults[rocaTulwar].power != 145 {
		t.Fatal("reference powers", len(defaults), err)
	}
	c := &assets.Catalog{Skills: map[uint16]assets.Skill{blastAttack: {ID: blastAttack, Element: 3, EffectLayer: 2}}}
	if err := seedSkillPowers(c); err != nil || c.Skills[blastAttack].BasePower == nil || *c.Skills[blastAttack].BasePower != 20 {
		t.Fatal("fresh initialization", c.Skills, err)
	}
	for _, missing := range []bool{true, false} {
		db, err := assetdb.Open(filepath.Join(t.TempDir(), "powers.db"))
		if err != nil {
			t.Fatal(err)
		}
		if err := db.AutoMigrate(&SkillsRow{}, &catalogSchema{}); err != nil {
			t.Fatal(err)
		}
		rows := []SkillsRow{
			{SkillsKey: blastAttack, ValueID: blastAttack, ValueElement: 3, ValueEffectLayer: 2, ValuePowerPerLevel: 1.38, ValueAdditionalDamage: 4, ValueName: "Local name"},
			{SkillsKey: flameAttack, ValueID: flameAttack, ValueElement: 3, ValueEffectLayer: 1, ValueBasePowerPresent: true, ValueBasePower: 99},
			{SkillsKey: windAttack, ValueID: windAttack, ValueElement: 4, ValueEffectLayer: 1, ValueBasePowerPresent: true, ValueBasePower: 0},
			{SkillsKey: rocaTulwar, ValueID: rocaTulwar, ValueElement: 0, ValueEffectLayer: 1},
		}
		if err := db.Create(&rows).Error; err != nil {
			t.Fatal(err)
		}
		if missing {
			for _, column := range []string{"ValueBasePowerPresent", "ValueBasePower"} {
				if err := db.Migrator().DropColumn(&SkillsRow{}, column); err != nil {
					t.Fatal(err)
				}
			}
		}
		startingVersion := nativeNoaConversationSchemaVersion
		if !missing {
			startingVersion = skillPowerSchemaVersion
		}
		if err := db.Create(&catalogSchema{ID: catalogMetadataID, Version: startingVersion}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Transaction(migrateCatalog); err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			var got SkillsRow
			if err := db.First(&got, "skills_key = ?", rows[i].SkillsKey).Error; err != nil {
				t.Fatal(err)
			}
			want := defaults[got.SkillsKey].power
			if !missing && rows[i].ValueBasePowerPresent {
				want = rows[i].ValueBasePower
			}
			if !got.ValueBasePowerPresent || got.ValueBasePower != want || got.ValueName != rows[i].ValueName || got.ValuePowerPerLevel != rows[i].ValuePowerPerLevel || got.ValueAdditionalDamage != rows[i].ValueAdditionalDamage {
				t.Fatal("migration lost power/provenance/custom edits", missing, got)
			}
		}
		if err := db.Model(&SkillsRow{}).Where("skills_key = ?", blastAttack).Update("value_base_power", 77).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Transaction(migrateCatalog); err != nil {
			t.Fatal(err)
		}
		var got SkillsRow
		if err := db.First(&got, "skills_key = ?", blastAttack).Error; err != nil || got.ValueBasePower != 77 {
			t.Fatal("reinitialization overwrote edit", got, err)
		}
		loaded, err := readSkills(db)
		if err != nil || loaded[blastAttack].BasePower == nil || *loaded[blastAttack].BasePower != 77 {
			t.Fatal("structured skill power round trip", loaded[blastAttack], err)
		}
		assetdb.Close(db)
	}
}

func TestUniqueSkillPowersDoNotConfuseReusedIconsOrConditionalValues(t *testing.T) {
	defaults, err := defaultSkillPowers()
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[uint16]float64{
		15041: 34,            // Love Wish; reference icon60012 is not the native ID.
		11170: 500,           // Frederico's SP66 Blowout, not his SP115 Rage Thunder.
		30010: 53, 30011: 69, // Reused squirrel icon; separate Tail Attack/Hit.
		30035: 60,            // Electricity; reference icon11053 is Revival.
		30040: 20, 30041: 35, // Two distinct Horn Rising variants.
	} {
		if got, ok := defaults[id]; !ok || got.power != want {
			t.Fatal("unique power binding", id, got, want)
		}
	}
	for _, id := range []uint16{15135, 25013, 17003, 30051, 11064, 15164, 11053, 11171} {
		if _, exists := defaults[id]; exists {
			t.Fatal("unverified or unrelated skill received a power", id)
		}
	}
}

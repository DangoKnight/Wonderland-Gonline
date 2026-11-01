package assetsql

import (
	"testing"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
)

func TestPoisonDefaultUpgradePreservesEditedEffects(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	c, err := LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	c.SkillEffects = map[string]assets.EffectDefinition{
		"native_15_171_rounds_2": {ID: "native_15_171_rounds_2", Name: "Locally named poison", Effects: []assets.SkillEffect{{Target: assets.EffectEnemy, Rounds: 2, StackGroup: "periodic_poison", PeriodicDamage: &assets.PeriodicDamage{MaxHPPercent: 8, Minimum: 5}}}},
		"native_15_171_rounds_3": {ID: "native_15_171_rounds_3", Name: "Edited poison", Effects: []assets.SkillEffect{{Target: assets.EffectEnemy, Rounds: 3, StackGroup: "periodic_poison", PeriodicDamage: &assets.PeriodicDamage{MaxHPPercent: 8, Minimum: 6}}}},
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	if err = SaveEditedCatalog(db, c); err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(migratePoisonDefaults); err != nil {
		t.Fatal(err)
	}
	got, err := LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	a, b := got.SkillEffects["native_15_171_rounds_2"], got.SkillEffects["native_15_171_rounds_3"]
	if a.Effects[0].PeriodicDamage.MaxHPPercent != 10 || a.Effects[0].PeriodicDamage.Minimum != 15 || a.Name != "Locally named poison" || b.Effects[0].PeriodicDamage.Minimum != 6 {
		t.Fatal(a, b)
	}
}

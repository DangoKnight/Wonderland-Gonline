package assetsql

import (
	"testing"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/game"
)

func TestPetInitialStatsWikiAuthoredValues(t *testing.T) {
	values, err := defaultPetInitialStats()
	if err != nil {
		t.Fatal(err)
	}
	// Independent classic-WLO-first transcription; the Rhodes reference fills
	// absent classic companions. Parenthesized rebirth stats are excluded.
	want := map[uint32]game.Attributes{14162: {Strength: 4, Constitution: 5, Intelligence: 2, Wisdom: 2, Agility: 4}, 14156: {Strength: 1, Constitution: 1, Intelligence: 2, Wisdom: 2, Agility: 4}, 14081: {Strength: 1, Constitution: 1, Intelligence: 5, Wisdom: 5, Agility: 2}, 14548: {Strength: 12, Constitution: 16, Intelligence: 11, Wisdom: 14, Agility: 12}, 14175: {Strength: 7, Constitution: 6, Intelligence: 2, Wisdom: 1, Agility: 4}, 14609: {Strength: 7, Constitution: 6, Intelligence: 15, Wisdom: 15, Agility: 8}, 12068: {Strength: 7, Constitution: 6, Intelligence: 4, Wisdom: 5, Agility: 3}, 12095: {Strength: 15, Constitution: 19, Intelligence: 34, Wisdom: 33, Agility: 17}, 17914: {Strength: 2, Constitution: 2, Intelligence: 7, Wisdom: 10, Agility: 15}, 14230: {Strength: 3, Constitution: 10, Intelligence: 5, Wisdom: 4, Agility: 7}, 14615: {Strength: 3, Constitution: 5, Intelligence: 14, Wisdom: 10, Agility: 6}, 14243: {Strength: 7, Constitution: 15, Intelligence: 19, Wisdom: 19, Agility: 21}, 14242: {Strength: 4, Constitution: 5, Intelligence: 13, Wisdom: 12, Agility: 12}, 12147: {Strength: 11, Constitution: 10, Intelligence: 7, Wisdom: 8, Agility: 8}, 14274: {Strength: 21, Constitution: 16, Intelligence: 7, Wisdom: 4, Agility: 14}, 15657: {Strength: 18, Constitution: 18, Intelligence: 6, Wisdom: 11, Agility: 12}, 15689: {Strength: 14, Constitution: 10, Intelligence: 11, Wisdom: 13, Agility: 12}, 14637: {Strength: 12, Constitution: 10, Intelligence: 12, Wisdom: 11, Agility: 13}, 12148: {Strength: 19, Constitution: 27, Intelligence: 35, Wisdom: 31, Agility: 25}, 12149: {Strength: 16, Constitution: 24, Intelligence: 34, Wisdom: 33, Agility: 26}, 35253: {Strength: 21, Constitution: 20, Intelligence: 11, Wisdom: 5, Agility: 14}, 17162: {Strength: 1, Constitution: 2, Intelligence: 1, Wisdom: 1, Agility: 2}, 17454: {Strength: 16, Constitution: 10, Intelligence: 2, Wisdom: 1, Agility: 3}, 12179: {Strength: 9, Constitution: 9, Intelligence: 5, Wisdom: 5, Agility: 8}, 12217: {Strength: 19, Constitution: 23, Intelligence: 38, Wisdom: 37, Agility: 21}, 12219: {Strength: 11, Constitution: 19, Intelligence: 24, Wisdom: 25, Agility: 24}, 12222: {Strength: 12, Constitution: 20, Intelligence: 27, Wisdom: 26, Agility: 26}, 12178: {Strength: 7, Constitution: 10, Intelligence: 2, Wisdom: 4, Agility: 6}, 12180: {Strength: 21, Constitution: 30, Intelligence: 43, Wisdom: 36, Agility: 5}, 12181: {Strength: 44, Constitution: 40, Intelligence: 20, Wisdom: 25, Agility: 6}}
	variable := map[uint32]game.Attributes{14162: {Strength: 5, Constitution: 6, Intelligence: 3, Wisdom: 3, Agility: 5}, 14156: {Strength: 2, Constitution: 2, Intelligence: 4, Wisdom: 4, Agility: 6}, 14081: {Strength: 1, Constitution: 1, Intelligence: 6, Wisdom: 6, Agility: 2}, 14548: {Strength: 13, Constitution: 17, Intelligence: 12, Wisdom: 15, Agility: 12}, 14175: {Strength: 8, Constitution: 7, Intelligence: 2, Wisdom: 1, Agility: 5}, 14609: {Strength: 8, Constitution: 6, Intelligence: 16, Wisdom: 16, Agility: 9}, 12068: {Strength: 9, Constitution: 7, Intelligence: 6, Wisdom: 6, Agility: 3}, 14230: {Strength: 4, Constitution: 11, Intelligence: 6, Wisdom: 6, Agility: 8}, 14615: {Strength: 3, Constitution: 5, Intelligence: 15, Wisdom: 11, Agility: 7}, 14243: {Strength: 7, Constitution: 16, Intelligence: 21, Wisdom: 22, Agility: 23}, 14242: {Strength: 4, Constitution: 6, Intelligence: 15, Wisdom: 14, Agility: 14}, 12147: {Strength: 12, Constitution: 11, Intelligence: 7, Wisdom: 9, Agility: 9}, 14274: {Strength: 22, Constitution: 17, Intelligence: 8, Wisdom: 4, Agility: 15}, 15657: {Strength: 20, Constitution: 20, Intelligence: 6, Wisdom: 12, Agility: 13}, 15689: {Strength: 15, Constitution: 10, Intelligence: 11, Wisdom: 14, Agility: 12}, 35253: {Strength: 22, Constitution: 21, Intelligence: 11, Wisdom: 5, Agility: 14}, 17454: {Strength: 17, Constitution: 11, Intelligence: 3, Wisdom: 1, Agility: 4}, 12178: {Strength: 7, Constitution: 11, Intelligence: 2, Wisdom: 5, Agility: 7}}
	totals := map[uint32]uint32{14162: 18, 14156: 13, 14081: 15, 14548: 66, 14175: 21, 14609: 52, 12068: 27, 12095: 118, 17914: 36, 14230: 30, 14615: 39, 14243: 84, 14242: 48, 12147: 45, 14274: 63, 15657: 67, 15689: 61, 14637: 58, 12148: 137, 12149: 133, 35253: 72, 17162: 7, 17454: 33, 12179: 36, 12217: 138, 12219: 103, 12222: 111, 12178: 30, 12180: 135, 12181: 135}
	if len(values) != len(want) {
		t.Fatal(len(values))
	}
	for id, stats := range want {
		v := values[id]
		if v.Minimum != stats || v.Source == "" {
			t.Fatal(id, v)
		}
		maximum := stats
		if m, ok := variable[id]; ok {
			maximum = m
		}
		if v.Maximum != maximum || v.TotalPoints != totals[id] {
			t.Fatal("wrong range or total", id, v.Maximum, v.TotalPoints)
		}
	}
	if values[12178].Maximum != (game.Attributes{Strength: 7, Constitution: 11, Intelligence: 2, Wisdom: 5, Agility: 7}) {
		t.Fatal("Robinson ranges")
	}
}

func TestPetInitialStatsMigrationPreservesExistingDefinitions(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	if err := db.Model(&PetInitialStatsRow{}).Where("pet_initial_stats_key = ?", 12178).Update("value_minimum_strength", 6).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&catalogSchema{}).Where("id = ?", 1).Update("version", 17).Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	c, err := LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.PetInitialStats[12178].Minimum.Strength != 6 {
		t.Fatal("migration overwrote edited recruitment stats")
	}
}

func TestPetInitialStatsRejectsImpossibleTotals(t *testing.T) {
	for _, total := range []uint32{28, 33} {
		v := game.PetInitialStats{Minimum: game.Attributes{Strength: 7, Constitution: 10, Intelligence: 2, Wisdom: 4, Agility: 6}, Maximum: game.Attributes{Strength: 7, Constitution: 11, Intelligence: 2, Wisdom: 5, Agility: 7}, TotalPoints: total, Source: "authored fixture"}
		if err := validatePetInitialStats(map[uint32]game.PetInitialStats{12178: v}); err == nil {
			t.Fatal("accepted impossible initial stat budget", total)
		}
	}
}

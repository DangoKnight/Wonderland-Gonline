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
	want := map[uint32]game.Attributes{14162: {4, 5, 2, 2, 4}, 14156: {1, 1, 2, 2, 4}, 14081: {1, 1, 5, 5, 2}, 14548: {12, 16, 11, 14, 12}, 14175: {7, 6, 2, 1, 4}, 14609: {7, 6, 15, 15, 8}, 12068: {7, 6, 4, 5, 3}, 12095: {15, 19, 34, 33, 17}, 17914: {2, 2, 7, 10, 15}, 14230: {3, 10, 5, 4, 7}, 14615: {3, 5, 14, 10, 6}, 14243: {7, 15, 19, 19, 21}, 14242: {4, 5, 13, 12, 12}, 12147: {11, 10, 7, 8, 8}, 14274: {21, 16, 7, 4, 14}, 15657: {18, 18, 6, 11, 12}, 15689: {14, 10, 11, 13, 12}, 14637: {12, 10, 12, 11, 13}, 12148: {19, 27, 35, 31, 25}, 12149: {16, 24, 34, 33, 26}, 35253: {21, 20, 11, 5, 14}, 17162: {1, 2, 1, 1, 2}, 17454: {16, 10, 2, 1, 3}, 12179: {9, 9, 5, 5, 8}, 12217: {19, 23, 38, 37, 21}, 12219: {11, 19, 24, 25, 24}, 12222: {12, 20, 27, 26, 26}, 12178: {7, 10, 2, 4, 6}, 12180: {21, 30, 43, 36, 5}, 12181: {44, 40, 20, 25, 6}}
	variable := map[uint32]game.Attributes{14162: {5, 6, 3, 3, 5}, 14156: {2, 2, 4, 4, 6}, 14081: {1, 1, 6, 6, 2}, 14548: {13, 17, 12, 15, 12}, 14175: {8, 7, 2, 1, 5}, 14609: {8, 6, 16, 16, 9}, 12068: {9, 7, 6, 6, 3}, 14230: {4, 11, 6, 6, 8}, 14615: {3, 5, 15, 11, 7}, 14243: {7, 16, 21, 22, 23}, 14242: {4, 6, 15, 14, 14}, 12147: {12, 11, 7, 9, 9}, 14274: {22, 17, 8, 4, 15}, 15657: {20, 20, 6, 12, 13}, 15689: {15, 10, 11, 14, 12}, 35253: {22, 21, 11, 5, 14}, 17454: {17, 11, 3, 1, 4}, 12178: {7, 11, 2, 5, 7}}
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
	if values[12178].Maximum != (game.Attributes{7, 11, 2, 5, 7}) {
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
		v := game.PetInitialStats{Minimum: game.Attributes{7, 10, 2, 4, 6}, Maximum: game.Attributes{7, 11, 2, 5, 7}, TotalPoints: total, Source: "authored fixture"}
		if err := validatePetInitialStats(map[uint32]game.PetInitialStats{12178: v}); err == nil {
			t.Fatal("accepted impossible initial stat budget", total)
		}
	}
}

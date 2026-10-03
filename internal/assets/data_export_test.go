package assets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Compare readable bulk exports against the existing independently ported Go
// projections. Python's inverse tests also verify fields outside these projections.
func TestBulkNativeExportProjections(t *testing.T) {
	source := os.Getenv("WONDERLAND_TEST_CLIENT_DATA")
	if source == "" {
		t.Skip("set WONDERLAND_TEST_CLIENT_DATA to verify exported client data")
	}
	if !filepath.IsAbs(source) {
		source = filepath.Join("..", "..", source)
	}
	type record struct {
		Fields map[string]any `json:"fields"`
	}
	read := func(t *testing.T, name string) []record {
		t.Helper()
		data, err := os.ReadFile(filepath.Join("..", "..", "data", name))
		if err != nil {
			t.Fatal(err)
		}
		var document struct {
			Records []record `json:"records"`
		}
		if err := json.Unmarshal(data, &document); err != nil {
			t.Fatal(err)
		}
		return document.Records
	}
	check := func(t *testing.T, fields map[string]any, name string, expected float64) {
		t.Helper()
		if actual, exists := fields[name]; !exists || actual != expected {
			t.Fatalf("ID %.0f: %s = %v (present %v), expected %v", fields["id"], name, actual, exists, expected)
		}
	}
	t.Run("NPCs", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(source, "Npc.dat"))
		if err != nil {
			t.Fatal(err)
		}
		npcs, err := ParseNPCs(data)
		if err != nil {
			t.Fatal(err)
		}
		records := read(t, "npc_data.json")
		if len(records) != len(data)/138-1 {
			t.Fatal("export omitted native NPC records")
		}
		for _, row := range records {
			id := uint16(row.Fields["id"].(float64))
			if id == 0 {
				continue
			}
			npc, exists := npcs[id]
			if !exists {
				t.Fatalf("NPC %d missing", id)
			}
			for name, value := range map[string]float64{
				"type": float64(npc.Type), "level": float64(npc.Level),
				"hp": float64(npc.HP), "sp": float64(npc.SP), "element": float64(npc.Element),
				"strength": float64(npc.Stats[0]), "constitution": float64(npc.Stats[1]),
				"intelligence": float64(npc.Stats[2]), "wisdom": float64(npc.Stats[3]),
				"agility": float64(npc.Stats[4]), "book_index": float64(npc.BookIndex),
				"skill_1": float64(npc.Skills[0]), "skill_2": float64(npc.Skills[1]), "skill_3": float64(npc.Skills[2]),
				"drop_1": float64(npc.Drops[0]), "drop_2": float64(npc.Drops[1]), "drop_3": float64(npc.Drops[2]),
				"drop_4": float64(npc.Drops[3]), "drop_5": float64(npc.Drops[4]),
			} {
				check(t, row.Fields, name, value)
			}
		}
	})
	t.Run("Skills", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(source, "Skill.dat"))
		if err != nil {
			t.Fatal(err)
		}
		skills, err := ParseSkills(data)
		if err != nil {
			t.Fatal(err)
		}
		records := read(t, "skill_data.json")
		if len(records) != len(data)/148-1 {
			t.Fatal("export omitted native skill records")
		}
		for _, row := range records {
			skill, exists := skills[uint16(row.Fields["id"].(float64))]
			if !exists {
				continue // Empty native records are retained by the exporter.
			}
			for name, value := range map[string]float64{
				"type": float64(skill.Type), "sp": float64(skill.SP), "element": float64(skill.Element),
				"attack_category": float64(skill.Attack), "effect_layer": float64(skill.EffectLayer),
				"power_per_level": skill.PowerPerLevel, "stat_multiplier": skill.StatMultiplier,
				"additional_damage": float64(skill.AdditionalDamage), "table_order": float64(skill.TableOrder),
			} {
				check(t, row.Fields, name, value)
			}
		}
	})
}

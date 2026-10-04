package assets

import "testing"

func TestQuestVisibilityValidation(t *testing.T) {
	c := &Catalog{Maps: map[uint16]Map{10017: {ID: 10017, NPCs: []MapNPC{{ClickID: 1}, {ClickID: 2}}}}}
	valid := `[{"quest_id":99,"map_id":10017,"spawn_npc_click_ids":[1],"despawn_npc_click_ids":[2],"steps":[{"step":1,"spawn_npc_click_ids":[2]}]}]`
	got, err := ParseQuestVisibility([]byte(valid), c)
	if err != nil || len(got) != 1 || got[0].ID != 99 || got[0].Steps[0].Spawn[0] != 2 {
		t.Fatal(got, err)
	}
	for _, raw := range []string{
		`[{"quest_id":0,"map_id":10017}]`,
		`[{"quest_id":99,"map_id":20000}]`,
		`[{"quest_id":99,"map_id":10017},{"quest_id":99,"map_id":10017}]`,
		`[{"quest_id":99,"map_id":10017,"spawn_npc_click_ids":[0]}]`,
		`[{"quest_id":99,"map_id":10017,"spawn_npc_click_ids":[3]}]`,
		`[{"quest_id":99,"map_id":10017,"spawn_npc_click_ids":[1,1]}]`,
		`[{"quest_id":99,"map_id":10017,"steps":[{"step":0}]}]`,
		`[{"quest_id":99,"map_id":10017,"steps":[{"step":256}]}]`,
		`[{"quest_id":99,"map_id":10017,"steps":[{"step":1},{"step":1}]}]`,
	} {
		if _, err := ParseQuestVisibility([]byte(raw), c); err == nil {
			t.Fatal("accepted invalid visibility", raw)
		}
	}
}

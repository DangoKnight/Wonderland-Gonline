package assets

import "testing"

func TestStoryBattleBackgroundValidation(t *testing.T) {
	for _, value := range []int64{-1, 0, 140, 255, 256, 65535, 65536} {
		r := StoryRule{ID: "room_backdrop", Kind: StoryBattleBackground, Scope: StoryScope{Maps: []uint16{12002}}, Value: value}
		err := ValidateStoryDefinitions(StoryDefinitions{Rules: []StoryRule{r}})
		if (err == nil) != (value >= 0 && value <= 255) {
			t.Fatal("invalid background selector validation", value, err)
		}
	}
}

func TestStoryDefinitionsValidation(t *testing.T) {
	if err := ValidateStoryDefinitions(DefaultStoryDefinitions()); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"kind", "comparison", "cue_size", "cue_kind", "cue_range", "mark", "alias", "identity", "mode", "scope", "equipment"} {
		t.Run(scenario, func(t *testing.T) {
			d := DefaultStoryDefinitions()
			switch scenario {
			case "kind":
				d.Rules[0].Kind = "invented"
			case "comparison":
				d.Rules[7].Gates[0].Op = "invented"
			case "cue_size":
				d.Cinematics[0].Steps[0].Cues[0].Data = nil
			case "cue_kind":
				d.Cinematics[0].Steps[0].Cues[0].Kind = "invented"
			case "cue_range":
				d.Cinematics[0].Steps[0].Cues[0].Data[0] = 300
			case "mark":
				d.Recruitments[0].Marks[0].Mode = "invented"
			case "alias":
				d.CompanionAliases[12032] = 12178
			case "identity":
				d.Rules[1].ID = d.Rules[0].ID
			case "mode":
				d.Recruitments[0].MovieMode = 99
			case "scope":
				d.Rules[0].Scope.Maps = []uint16{0}
			case "equipment":
				d.CompanionEquipment[0].Equipment[0].Slot = 255
			}
			if err := ValidateStoryDefinitions(d); err == nil {
				t.Fatal("invalid story accepted")
			}
		})
	}
}

func TestStoryRebornCheckpointValidationAndReferences(t *testing.T) {
	rule := StoryRule{ID: "custom_checkpoint", Kind: StoryRebornCheckpoint, Gates: []StoryGate{{Source: "mark_exists", ID: 100001, Op: "eq", Value: 0}}, Marks: []StoryMark{{ID: 100000, Mode: "complete", PreserveStep: true, Value: 1}, {ID: 100001, Mode: "ensure_active", Value: 1}}}
	c := &Catalog{Story: StoryDefinitions{Rules: []StoryRule{rule}}, QuestDefinitions: map[uint32]QuestDefinition{100000: {ID: 100000}, 100001: {ID: 100001}}}
	if err := ValidateStoryCatalog(c); err != nil {
		t.Fatal(err)
	}
	delete(c.QuestDefinitions, 100000)
	if err := ValidateStoryCatalog(c); err == nil {
		t.Fatal("unknown checkpoint mark accepted")
	}
	for _, change := range []func(*StoryRule){
		func(r *StoryRule) { r.Marks = nil },
		func(r *StoryRule) { r.Pattern.Code = new(byte) },
		func(r *StoryRule) { r.Marks = []StoryMark{{ID: 1, Mode: "invented"}} },
		func(r *StoryRule) { r.Kind = StoryVisibility },
	} {
		r := rule
		change(&r)
		if err := ValidateStoryDefinitions(StoryDefinitions{Rules: []StoryRule{r}}); err == nil {
			t.Fatal("invalid checkpoint accepted")
		}
	}
}

package assets

import "testing"

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

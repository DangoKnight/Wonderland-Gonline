package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCharacterDesignValidationRejectsInvalidChoices(t *testing.T) {
	roles := []any{map[string]any{"index": float64(6)}}
	for _, mutate := range []func(*characterDesign){
		func(d *characterDesign) { d.Name = "TooLongForWizard" },
		func(d *characterDesign) { d.Name = "猫咪" },
		func(d *characterDesign) { d.Role = 99 },
		func(d *characterDesign) { d.Element = "light" },
		func(d *characterDesign) { d.Stats = []int{5} },
		func(d *characterDesign) { d.Stats = []int{0, 0, 0, 0, 0} },
		func(d *characterDesign) { d.Stats = []int{-1, 3, 2, 1, 0} },
		func(d *characterDesign) { d.Colors = nil },
		func(d *characterDesign) { d.Colors[0] = []int{4, 4} },
		func(d *characterDesign) { d.Colors[0][0] = 10 },
	} {
		d := fixtureCharacterDesign()
		mutate(&d)
		if d.validate(roles) == nil {
			t.Fatal("invalid design accepted", d)
		}
	}
	if err := fixtureCharacterDesign().validate(roles); err != nil {
		t.Fatal(err)
	}
}

func TestCharacterDesignerUsesPersonaAndNativeChoicesWithoutCredentials(t *testing.T) {
	design := fixtureCharacterDesign()
	design.Name = "MistSong"
	design.Element = "water"
	design.Colors[0] = []int{2, 6, 9}
	p := newPlayer(&rpcClient{session: 7}, credentials{account: "hidden-user", password: "private-password", key: "private-key"}, "test-model", t.TempDir(), nil)
	p.profile = &controllerProfile{Name: "Aria", Preferences: "Explore the sea and dress in blue.", Accounts: []int{1}}
	p.memory = "Find an interesting team role."
	p.group = &sessionGroup{accounts: []credentials{p.credentials}, states: map[int]map[string]any{1: {"player": map[string]any{"name": "ExistingHero"}}}}
	state := startupState(creationForm, map[string]any{"kind": "characterCreation", "value": map[string]any{"step": float64(1), "roles": []any{map[string]any{"index": float64(6), "body": float64(3), "head": float64(0)}}}}, map[string]any{"value": "private-password"})
	state["private"] = "never-expose-deletion-code"
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		input, instructions := textValue(body["input"]), textValue(body["instructions"])
		if !strings.Contains(instructions, p.profile.Preferences) || !strings.Contains(input, "ExistingHero") || !strings.Contains(input, "Unavailable") || !strings.Contains(input, `"index":6`) {
			t.Error("missing persona, names or native choices")
		}
		for _, secret := range []string{"hidden-user", "private-password", "private-key", "never-expose-deletion-code"} {
			if strings.Contains(jsonText(body), secret) {
				t.Error("private data reached model input")
			}
		}
		if strings.Contains(instructions, "Do not request login/password/character creation commands directly") {
			t.Error("creation mode inherited conflicting gameplay instructions")
		}
		d := decision{Kind: "command", Name: characterDesignCommand, ArgumentsJSON: jsonText(design), Memory: "New unrelated memory", Reason: "A water traveler in blue.", WaitSeconds: 1}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed", "output": []any{map[string]any{"content": []any{map[string]any{"type": "output_text", "text": jsonText(d)}}}}})
	}))
	defer model.Close()
	p.endpoint = model.URL
	chosen, err := p.designCharacter(context.Background(), state, []string{"Unavailable"})
	if err != nil || jsonText(chosen) != jsonText(design) {
		t.Fatal("model customization was not retained", err, chosen)
	}
	if p.memory != "Find an interesting team role." {
		t.Fatal("design request overwrote gameplay memory")
	}
}

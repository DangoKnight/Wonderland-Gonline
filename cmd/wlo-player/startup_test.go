package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixtureCharacterDesign() characterDesign {
	return characterDesign{Name: "AriaSage", Role: 6, Element: "wind", Stats: []int{0, 2, 2, 1, 0}, Colors: [][]int{{4, 4, 4}, {4, 4, 4}, {4, 4, 4}, {4, 4, 4}}}
}

func startupState(form string, controls ...map[string]any) map[string]any {
	items := []any{}
	for _, control := range controls {
		items = append(items, control)
	}
	return map[string]any{"openForms": []any{form}, "forms": []any{map[string]any{"name": form, "controls": items}}}
}

func startupControl(name string) map[string]any {
	return map[string]any{"name": name, "id": "handle-" + name, "revision": strings.Repeat("a", 64), "enabled": true}
}

func TestStartupCreatesEmptySlotAndSelectsOccupiedSlot(t *testing.T) {
	c := credentials{password: "private-login"}
	profile := characterProfile{Name: "Roverabcde", DeletionCode: "12345678"}
	empty := startupState("SelectCharacter", startupControl("Manage[0]"))
	action, err := chooseStartupAction(empty, c, 1, profile, map[string]bool{})
	if err != nil || action.key != "create-start" || action.command != "ui.invoke" || action.args["id"] != "handle-Manage[0]" {
		t.Fatal(action, err)
	}
	occupied := startupState("SelectCharacter", startupControl("Manage[0]"), startupControl("Enter[0]"))
	action, err = chooseStartupAction(occupied, c, 1, profile, map[string]bool{})
	if err != nil || action.command != "character.select" {
		t.Fatal(action, err)
	}
}

func TestStartupCreationUsesCurrentHandlesAndPrivatePasswordFields(t *testing.T) {
	c := credentials{password: "abcdefghijkl"}
	profile := characterProfile{Name: "Roverabcde", DeletionCode: "12345678"}
	design := fixtureCharacterDesign()
	profile.Design = &design
	root := map[string]any{"kind": "characterCreation", "value": map[string]any{"step": float64(2), "points": float64(5), "stats": []any{float64(0), float64(0), float64(0), float64(0), float64(0), float64(0)}}}
	state := startupState(creationForm, root, startupControl("StatArrows[4]"))
	action, err := chooseStartupAction(state, c, 1, profile, map[string]bool{})
	if err != nil || action.args["id"] != "handle-StatArrows[4]" || action.args["revision"] != strings.Repeat("a", 64) {
		t.Fatal(action, err)
	}
	sent := map[string]bool{}
	for field, want := range []string{c.password, c.password, profile.DeletionCode, profile.DeletionCode} {
		name := []string{"Fields[1]", "Fields[2]", "Fields[3]", "Fields[4]"}[field]
		state = startupState(creationPasswordForm, startupControl(name))
		action, err = chooseStartupAction(state, c, 1, profile, sent)
		if err != nil || action.args["value"] != want || action.args["action"] != "set" {
			t.Fatal("incorrect private field routing", err)
		}
		sent[action.key] = true
	}
}

func TestCreationProfilePersistsBeforeSubmission(t *testing.T) {
	directory := t.TempDir()
	first, err := loadCharacterProfile(directory)
	if err != nil || first.Name != "" || first.Design != nil || len(first.DeletionCode) != 8 {
		t.Fatal("invalid generated profile", err)
	}
	design := fixtureCharacterDesign()
	first.Name, first.Design = design.Name, &design
	if err := saveCharacterProfile(directory, first); err != nil {
		t.Fatal(err)
	}
	second, err := loadCharacterProfile(directory)
	if err != nil || jsonText(first) != jsonText(second) {
		t.Fatal("creation identity changed on retry")
	}
	data, err := os.ReadFile(filepath.Join(directory, "character.private.json"))
	if err != nil || !json.Valid(data) {
		t.Fatal("profile was not persisted")
	}
}

func TestStartupAppliesChosenRoleStatsElementAndAllColors(t *testing.T) {
	design := characterDesign{Name: "MeteoGale", Role: 6, Element: "wind", Stats: []int{3, 0, 0, 0, 2}, Colors: [][]int{{9, 0, 4}, {1, 7, 9}, {2, 3, 8}, {7, 1, 0}}}
	profile := characterProfile{Name: design.Name, Design: &design}
	value := map[string]any{"step": float64(1), "selected": float64(1), "points": float64(5), "part": float64(1), "appearance": map[string]any{"element": float64(1)}, "stats": []any{float64(0), float64(0), float64(0), float64(0), float64(0), float64(0)}, "colorParts": []any{[]any{float64(4), float64(4), float64(4)}, []any{float64(4), float64(4), float64(4)}, []any{float64(4), float64(4), float64(4)}, []any{float64(4), float64(4), float64(4)}}}
	controls := []map[string]any{{"kind": "characterCreation", "value": value}, startupControl("NameField"), startupControl("Buttons[2]"), startupControl("Buttons[1]")}
	for _, group := range []struct {
		name  string
		count int
	}{{"RolePics", 14}, {"StatArrows", 10}, {"Elements", 4}, {"Parts", 4}, {"ColorArrows", 6}} {
		for i := 1; i <= group.count; i++ {
			controls = append(controls, startupControl(fmt.Sprintf("%s[%d]", group.name, i)))
		}
	}
	state := startupState(creationForm, controls...)
	sent := map[string]bool{}
	finished := false
	for tick := 0; tick < 200; tick++ {
		a, err := chooseStartupAction(state, credentials{}, 1, profile, sent)
		if err != nil || a.key == "" || sent[a.key] {
			t.Fatal("wizard stalled", tick, a, err)
		}
		sent[a.key] = true
		name := strings.TrimPrefix(textValue(a.args["id"]), "handle-")
		var index int
		switch {
		case strings.HasPrefix(name, "RolePics"):
			fmt.Sscanf(name, "RolePics[%d]", &index)
			value["selected"] = float64(index)
		case name == "NameField":
			if a.args["value"] != design.Name {
				t.Fatal("wrong name")
			}
		case strings.HasPrefix(name, "StatArrows"):
			fmt.Sscanf(name, "StatArrows[%d]", &index)
			stats := value["stats"].([]any)
			stat := (index + 1) / 2
			if index%2 == 0 {
				stats[stat] = float64(number(stats[stat]) + 1)
				value["points"] = float64(number(value["points"]) - 1)
			} else {
				stats[stat] = float64(number(stats[stat]) - 1)
				value["points"] = float64(number(value["points"]) + 1)
			}
		case strings.HasPrefix(name, "Elements"):
			fmt.Sscanf(name, "Elements[%d]", &index)
			object(value["appearance"])["element"] = float64(index)
		case strings.HasPrefix(name, "Parts"):
			fmt.Sscanf(name, "Parts[%d]", &index)
			value["part"] = float64(index)
		case strings.HasPrefix(name, "ColorArrows"):
			fmt.Sscanf(name, "ColorArrows[%d]", &index)
			channels := value["colorParts"].([]any)[number(value["part"])-1].([]any)
			channel := (index - 1) / 2
			delta := 1
			if index%2 == 0 {
				delta = -1
			}
			channels[channel] = float64(number(channels[channel]) + delta)
		case name == "Buttons[2]":
			value["step"] = float64(number(value["step"]) + 1)
		case name == "Buttons[1]":
			finished = true
		default:
			t.Fatal("unexpected action", name)
		}
		if finished {
			break
		}
	}
	if !finished || number(value["selected"]) != 6 || number(object(value["appearance"])["element"]) != 4 {
		t.Fatal("chosen portrait and wind element were not applied")
	}
	if jsonText(value["stats"]) != "[0,3,0,0,0,2]" || jsonText(value["colorParts"]) != "[[9,0,4],[1,7,9],[2,3,8],[7,1,0]]" {
		t.Fatal("custom allocation or colors were lost", value)
	}
}

func TestStartupScrollsHiddenPortraitAndWaitsForCarousel(t *testing.T) {
	design := fixtureCharacterDesign()
	design.Role = 2 // Aria's selected portrait was initially outside the viewport.
	value := map[string]any{"step": float64(1), "selected": float64(1), "carouselPosition": float64(1)}
	root := map[string]any{"kind": "characterCreation", "value": value}
	state := startupState(creationForm, root, startupControl("NextChar"))
	profile := characterProfile{Name: design.Name, Design: &design}
	action, err := chooseStartupAction(state, credentials{}, 1, profile, nil)
	if err != nil || action.args["id"] != "handle-NextChar" {
		t.Fatal("hidden portrait did not scroll into view", action, err)
	}
	value["carouselMoving"] = true
	action, err = chooseStartupAction(state, credentials{}, 1, profile, nil)
	if err != nil || action.key != "" {
		t.Fatal("another action interrupted the carousel animation", action, err)
	}
	value["carouselMoving"] = false
	state = startupState(creationForm, root, startupControl("NextChar"), startupControl("RolePics[2]"))
	action, err = chooseStartupAction(state, credentials{}, 1, profile, nil)
	if err != nil || action.args["id"] != "handle-RolePics[2]" {
		t.Fatal("visible target portrait was not selected", action, err)
	}
}

func TestLoginRedesignsRejectedNameAndSavesBeforeSubmission(t *testing.T) {
	directory := t.TempDir()
	initial, err := loadCharacterProfile(directory)
	if err != nil {
		t.Fatal(err)
	}
	value := map[string]any{"step": float64(1), "selected": float64(6), "roles": []any{map[string]any{"index": float64(6)}}}
	state := startupState(creationForm, map[string]any{"kind": "characterCreation", "value": value}, startupControl("NameField"), startupControl("Buttons[2]"))
	state["details"] = groupState(1, false)["details"]
	state["commands"] = []any{}
	currentName := ""
	submissions := 0
	rpc := fakeRPC(t, func(method string, params map[string]any) any {
		if method == "agent.state" {
			return state
		}
		req := object(params["request"])
		args := object(req["arguments"])
		if args["id"] == "handle-NameField" {
			currentName = textValue(args["value"])
			saved, err := loadCharacterProfile(directory)
			if err != nil || saved.Name != currentName || saved.Design == nil || saved.DeletionCode != initial.DeletionCode {
				t.Error("design was not safely persisted before use", err)
			}
		} else if currentName != "TakenName" {
			state["inWorld"] = true
		}
		submissions++
		id := fmt.Sprint(submissions)
		state["commands"] = append(state["commands"].([]any), map[string]any{"id": id, "status": "executed"})
		return map[string]any{"id": id, "status": "accepted"}
	})
	designCalls := 0
	designer := func(_ context.Context, _ map[string]any, rejected []string) (characterDesign, error) {
		d := fixtureCharacterDesign()
		designCalls++
		if designCalls == 1 {
			d.Name = "TakenName"
		} else {
			if len(rejected) != 1 || rejected[0] != "TakenName" {
				t.Error("missing rejected name")
			}
			d.Name = "AriaBreeze"
		}
		return d, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := login(ctx, rpc, credentials{}, 1, directory, designer); err != nil {
		t.Fatal(err)
	}
	if designCalls != 2 {
		t.Fatal("name rejection was not handled", designCalls)
	}
}

func TestLoginLeavesExistingCharacterAndLegacyProfileUnchanged(t *testing.T) {
	directory := t.TempDir()
	legacy := []byte(`{"name":"Roverabcde","deletionCode":"12345678"}`)
	path := filepath.Join(directory, "character.private.json")
	if err := os.WriteFile(path, legacy, 0600); err != nil {
		t.Fatal(err)
	}
	state := startupState("SelectCharacter", startupControl("Enter[1]"))
	state["details"] = groupState(1, false)["details"]
	rpc := fakeRPC(t, func(method string, params map[string]any) any {
		if method == "agent.state" {
			return state
		}
		req := object(params["request"])
		if req["command"] != "character.select" || number(object(req["arguments"])["slot"]) != 2 {
			t.Error("occupied character was not selected")
		}
		state["inWorld"] = true
		return map[string]any{"id": "enter", "status": "accepted"}
	})
	designer := func(context.Context, map[string]any, []string) (characterDesign, error) {
		t.Error("existing character requested a new design")
		return fixtureCharacterDesign(), nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := login(ctx, rpc, credentials{}, 2, directory, designer); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != string(legacy) {
		t.Fatal("legacy character profile or code was overwritten", err)
	}
}

func TestLoginStopsAfterThreeRejectedNames(t *testing.T) {
	directory := t.TempDir()
	value := map[string]any{"step": float64(1), "selected": float64(6), "roles": []any{map[string]any{"index": float64(6)}}}
	state := startupState(creationForm, map[string]any{"kind": "characterCreation", "value": value}, startupControl("NameField"), startupControl("Buttons[2]"))
	state["details"] = groupState(1, false)["details"]
	state["commands"] = []any{}
	submissions := 0
	rpc := fakeRPC(t, func(method string, _ map[string]any) any {
		if method == "agent.state" {
			return state
		}
		submissions++
		id := fmt.Sprint(submissions)
		state["commands"] = append(state["commands"].([]any), map[string]any{"id": id, "status": "executed"})
		return map[string]any{"id": id, "status": "accepted"}
	})
	calls := 0
	designer := func(context.Context, map[string]any, []string) (characterDesign, error) {
		calls++
		d := fixtureCharacterDesign()
		d.Name = fmt.Sprintf("Chosen%d", calls)
		return d, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err := login(ctx, rpc, credentials{}, 1, directory, designer)
	if err == nil || !strings.Contains(err.Error(), "attempts exhausted") || calls != 3 {
		t.Fatal("name retries were not bounded", calls, err)
	}
	saved, err := loadCharacterProfile(directory)
	if err != nil || saved.Design != nil || jsonText(saved.RejectedNames) != `["Chosen1","Chosen2","Chosen3"]` {
		t.Fatal("rejected names were not retained safely", err)
	}
}

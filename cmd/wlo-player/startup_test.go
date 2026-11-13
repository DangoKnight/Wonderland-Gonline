package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	root := map[string]any{"kind": "characterCreation", "value": map[string]any{"step": float64(2), "points": float64(5)}}
	state := startupState(creationForm, root, startupControl("StatArrows[4]"))
	action, err := chooseStartupAction(state, c, 1, profile, map[string]bool{})
	if err != nil || action.key != "create-stat-5" || action.args["revision"] != strings.Repeat("a", 64) {
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
	if err != nil || len(first.Name) != 10 || len(first.DeletionCode) != 8 {
		t.Fatal("invalid generated profile", err)
	}
	second, err := loadCharacterProfile(directory)
	if err != nil || first != second {
		t.Fatal("creation identity changed on retry")
	}
	data, err := os.ReadFile(filepath.Join(directory, "character.private.json"))
	if err != nil || !json.Valid(data) {
		t.Fatal("profile was not persisted")
	}
}

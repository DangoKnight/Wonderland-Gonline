package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	creationNameStep            = 1
	creationStatsStep           = 2
	creationElementStep         = 3
	creationColorsStep          = 4
	creationBonusPoints         = 5
	creationPasswordFields      = 2
	creationCredentialFields    = 4
	creationNextControl         = "Buttons[2]"
	creationConfirmControl      = "Buttons[1]"
	creationFireControl         = "Elements[3]"
	creationConstitutionControl = "StatArrows[4]"
	creationIntelligenceControl = "StatArrows[6]"
	creationWisdomControl       = "StatArrows[8]"
	creationForm                = "TRE_CreateCharacter"
	creationPasswordForm        = "TRe_InputPwAndDarkPw"
)

type characterProfile struct {
	Name         string `json:"name"`
	DeletionCode string `json:"deletionCode"`
}

func loadCharacterProfile(directory string) (characterProfile, error) {
	path := filepath.Join(directory, "character.private.json")
	data, err := os.ReadFile(path)
	if err == nil {
		var profile characterProfile
		if json.Unmarshal(data, &profile) != nil || profile.Name == "" || profile.DeletionCode == "" {
			return profile, errors.New("invalid local character profile")
		}
		return profile, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return characterProfile{}, err
	}
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return characterProfile{}, err
	}
	profile := characterProfile{Name: "Rover" + hex.EncodeToString(random[:3])[:5], DeletionCode: hex.EncodeToString(random[4:])}
	// Persist the private deletion code before any character-creation request.
	if err := os.WriteFile(path, []byte(jsonText(profile)), 0600); err != nil {
		return profile, err
	}
	return profile, nil
}

func formControls(state map[string]any, formName string) []any {
	forms, _ := state["forms"].([]any)
	for _, value := range forms {
		form := object(value)
		if textValue(form["name"]) == formName {
			controls, _ := form["controls"].([]any)
			return controls
		}
	}
	return nil
}

func formControl(state map[string]any, formName, name string) map[string]any {
	for _, value := range formControls(state, formName) {
		control := object(value)
		if textValue(control["name"]) == name {
			return control
		}
	}
	return nil
}

type startupAction struct {
	key, command string
	args         map[string]any
}

func invokeStartup(state map[string]any, form, name, key, action string, value any) (startupAction, error) {
	control := formControl(state, form, name)
	if control == nil || !boolValue(control["enabled"]) {
		return startupAction{}, fmt.Errorf("startup control %s/%s is unavailable", form, name)
	}
	return startupAction{key, "ui.invoke", map[string]any{"id": control["id"], "revision": control["revision"], "action": action, "value": value}}, nil
}

func creationValue(state map[string]any) map[string]any {
	for _, control := range formControls(state, creationForm) {
		if textValue(object(control)["kind"]) == "characterCreation" {
			return object(object(control)["value"])
		}
	}
	return nil
}

func number(value any) int { n, _ := value.(float64); return int(n) }

func chooseStartupAction(state map[string]any, c credentials, slot int, profile characterProfile, sent map[string]bool) (startupAction, error) {
	switch {
	case hasForm(state, "SelectServer"):
		return startupAction{"connect", "server.connect", map[string]any{"region": 0, "channel": 0}}, nil
	case hasForm(state, "IDPassword"):
		return startupAction{"login", "login.submit", map[string]any{"account": c.account, "password": c.password}}, nil
	case hasForm(state, "SelectCharacter"):
		// Enter is only visible for an occupied slot. Manage creates when empty.
		if formControl(state, "SelectCharacter", fmt.Sprintf("Enter[%d]", slot-1)) != nil {
			return startupAction{"enter", "character.select", map[string]any{"slot": slot}}, nil
		}
		return invokeStartup(state, "SelectCharacter", fmt.Sprintf("Manage[%d]", slot-1), "create-start", "activate", nil)
	case hasForm(state, creationPasswordForm):
		for field := 1; field <= creationCredentialFields; field++ {
			key := fmt.Sprintf("password-field-%d", field)
			if sent[key] {
				continue
			}
			value := c.password
			if field > creationPasswordFields {
				value = profile.DeletionCode
			}
			return invokeStartup(state, creationPasswordForm, fmt.Sprintf("Fields[%d]", field), key, "set", value)
		}
		return invokeStartup(state, creationPasswordForm, "OK", "create-submit-password", "activate", nil)
	case hasForm(state, creationForm):
		creation := creationValue(state)
		if boolValue(creation["waiting"]) {
			return startupAction{}, nil
		}
		switch number(creation["step"]) {
		case creationNameStep:
			if !sent["create-name"] {
				return invokeStartup(state, creationForm, "NameField", "create-name", "set", profile.Name)
			}
			return invokeStartup(state, creationForm, creationNextControl, "create-next-name", "activate", nil)
		case creationStatsStep:
			remaining := number(creation["points"])
			if remaining < 0 || remaining > creationBonusPoints {
				return startupAction{}, errors.New("unexpected character creation point count")
			}
			if remaining == 0 {
				return invokeStartup(state, creationForm, creationNextControl, "create-next-stats", "activate", nil)
			}
			controls := []string{creationConstitutionControl, creationIntelligenceControl, creationWisdomControl, creationConstitutionControl, creationIntelligenceControl}
			return invokeStartup(state, creationForm, controls[creationBonusPoints-remaining], fmt.Sprintf("create-stat-%d", remaining), "activate", nil)
		case creationElementStep:
			if !sent["create-element"] {
				return invokeStartup(state, creationForm, creationFireControl, "create-element", "activate", nil)
			}
			return invokeStartup(state, creationForm, creationNextControl, "create-next-element", "activate", nil)
		case creationColorsStep:
			if formControl(state, creationForm, creationConfirmControl) != nil {
				return invokeStartup(state, creationForm, creationConfirmControl, "create-submit", "activate", nil)
			}
			return invokeStartup(state, creationForm, creationNextControl, "create-next-colors", "activate", nil)
		}
	}
	return startupAction{}, nil
}

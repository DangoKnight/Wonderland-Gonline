package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"wonderland-gonline/internal/game"
)

const (
	creationNameStep          = 1
	creationStatsStep         = 2
	creationElementStep       = 3
	creationColorsStep        = 4
	creationBonusPoints       = game.CreationStatPoints
	creationPasswordFields    = 2
	creationCredentialFields  = 4
	creationNextControl       = "Buttons[2]"
	creationConfirmControl    = "Buttons[1]"
	creationForm              = "TRE_CreateCharacter"
	creationPasswordForm      = "TRe_InputPwAndDarkPw"
	creationDeletionCodeBytes = 4 // Eight hexadecimal characters for the native secret-code editor.
)

type characterProfile struct {
	Name          string           `json:"name"`
	DeletionCode  string           `json:"deletionCode"`
	Design        *characterDesign `json:"design,omitempty"`
	RejectedNames []string         `json:"rejectedNames,omitempty"`
}

func loadCharacterProfile(directory string) (characterProfile, error) {
	path := filepath.Join(directory, "character.private.json")
	data, err := os.ReadFile(path)
	if err == nil {
		var profile characterProfile
		if json.Unmarshal(data, &profile) != nil || profile.DeletionCode == "" {
			return profile, errors.New("invalid local character profile")
		}
		if profile.Design != nil {
			if err := profile.Design.validate(nil); err != nil {
				return profile, fmt.Errorf("invalid saved character design: %w", err)
			}
			profile.Name = profile.Design.Name
		}
		return profile, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return characterProfile{}, err
	}
	var random [creationDeletionCodeBytes]byte
	if _, err := rand.Read(random[:]); err != nil {
		return characterProfile{}, err
	}
	profile := characterProfile{DeletionCode: hex.EncodeToString(random[:])}
	// Persist the private deletion code before any character-creation request.
	if err := saveCharacterProfile(directory, profile); err != nil {
		return profile, err
	}
	return profile, nil
}

func saveCharacterProfile(directory string, profile characterProfile) error {
	f, err := os.CreateTemp(directory, "character-profile-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	err = json.NewEncoder(f).Encode(profile)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), filepath.Join(directory, "character.private.json"))
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
		if profile.Design == nil {
			return startupAction{}, errors.New("new characters require an agent-selected design")
		}
		design := profile.Design
		switch number(creation["step"]) {
		case creationNameStep:
			if number(creation["selected"]) != design.Role {
				if boolValue(creation["carouselMoving"]) {
					return startupAction{}, nil
				}
				portrait := fmt.Sprintf("RolePics[%d]", design.Role)
				if control := formControl(state, creationForm, portrait); control != nil && boolValue(control["enabled"]) {
					return invokeStartup(state, creationForm, portrait, "create-role", "activate", nil)
				}
				// Native portraits outside the carousel viewport have no live UI
				// handle. Bring the selected design into view through its arrows.
				return invokeStartup(state, creationForm, "NextChar", fmt.Sprintf("create-carousel-%d", number(creation["carouselPosition"])), "activate", nil)
			}
			if !sent["create-name"] {
				return invokeStartup(state, creationForm, "NameField", "create-name", "set", profile.Name)
			}
			return invokeStartup(state, creationForm, creationNextControl, "create-next-name", "activate", nil)
		case creationStatsStep:
			remaining := number(creation["points"])
			if remaining < 0 || remaining > creationBonusPoints {
				return startupAction{}, errors.New("unexpected character creation point count")
			}
			stats, _ := creation["stats"].([]any)
			if len(stats) != creationStatCount+1 {
				return startupAction{}, errors.New("native creation stat state is unavailable")
			}
			// Refund excess allocations first, including after human intervention.
			for i, target := range design.Stats {
				current := number(stats[i+1])
				if current > target {
					return invokeStartup(state, creationForm, fmt.Sprintf("StatArrows[%d]", 2*(i+1)-1), fmt.Sprintf("create-stat-refund-%d-%d", i, current), "activate", nil)
				}
			}
			for i, target := range design.Stats {
				current := number(stats[i+1])
				if current < target {
					return invokeStartup(state, creationForm, fmt.Sprintf("StatArrows[%d]", 2*(i+1)), fmt.Sprintf("create-stat-add-%d-%d", i, current), "activate", nil)
				}
			}
			if remaining == 0 {
				return invokeStartup(state, creationForm, creationNextControl, "create-next-stats", "activate", nil)
			}
			return startupAction{}, errors.New("native creation point count does not match the design")
		case creationElementStep:
			if number(object(creation["appearance"])["element"]) != creationElements[design.Element] {
				return invokeStartup(state, creationForm, fmt.Sprintf("Elements[%d]", creationElements[design.Element]), "create-element", "activate", nil)
			}
			return invokeStartup(state, creationForm, creationNextControl, "create-next-element", "activate", nil)
		case creationColorsStep:
			parts, _ := creation["colorParts"].([]any)
			if len(parts) != creationColorPartCount {
				return startupAction{}, errors.New("native color-part state is unavailable; update the native client")
			}
			for part, target := range design.Colors {
				channels, _ := parts[part].([]any)
				if len(channels) != creationColorChannelCount {
					return startupAction{}, errors.New("invalid native color channels")
				}
				for channel, value := range target {
					current := number(channels[channel])
					if current == value {
						continue
					}
					if number(creation["part"]) != part+1 {
						return invokeStartup(state, creationForm, fmt.Sprintf("Parts[%d]", part+1), fmt.Sprintf("create-color-part-%d", part), "activate", nil)
					}
					arrow := 2*channel + 1
					if current > value {
						arrow++
					}
					return invokeStartup(state, creationForm, fmt.Sprintf("ColorArrows[%d]", arrow), fmt.Sprintf("create-color-%d-%d-%d-%d", part, channel, current, value), "activate", nil)
				}
			}
			if formControl(state, creationForm, creationConfirmControl) != nil {
				return invokeStartup(state, creationForm, creationConfirmControl, "create-submit", "activate", nil)
			}
			return invokeStartup(state, creationForm, creationNextControl, "create-next-colors", "activate", nil)
		}
	}
	return startupAction{}, nil
}

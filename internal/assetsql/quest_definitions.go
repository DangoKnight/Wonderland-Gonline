package assetsql

import (
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"regexp"
	"strings"
	"wonderland-go/internal/assetdb"
	"wonderland-go/internal/assets"
)

// importedQuestDefinitions is an offline, non-destructive metadata projection.
// Native IDs and text come from the imported Mark.dat JSON records. The reference
// guesses rewards/NPCs/types from English titles; those guesses are not native rules.
func importedQuestDefinitions(tx *gorm.DB, marks map[uint16]uint16) (map[uint32]assets.QuestDefinition, error) {
	out := map[uint32]assets.QuestDefinition{}
	if !tx.Migrator().HasTable(&assetdb.Record{}) {
		return out, nil
	}
	var rows []assetdb.Record
	if err := tx.Where("asset = ? AND collection = ?", "mark.dat", "records").Order("ordinal").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		var record struct {
			Name struct {
				Text string `json:"text"`
			} `json:"name"`
			Description struct {
				Text string `json:"text"`
			} `json:"description"`
			Fields struct {
				ID uint16 `json:"id"`
			} `json:"fields"`
		}
		if err := json.Unmarshal([]byte(row.JSON), &record); err != nil {
			return nil, err
		}
		id := record.Fields.ID
		if _, known := marks[id]; !known || id == 0 || record.Name.Text == "" {
			continue
		}
		definition := assets.QuestDefinition{ID: uint32(id), Title: record.Name.Text, Description: record.Description.Text, Type: assets.QuestDialogue, InProgressMarkID: uint32(id), CompletedMarkID: uint32(id), AllLinkedMarkIDs: []uint32{uint32(id)}, IntroDialogue: record.Description.Text, InProgressDialogue: record.Description.Text, CompleteDialogue: record.Description.Text, AlreadyCompletedDialogue: record.Description.Text}
		completed := ""
		for _, part := range markStepText.FindAllStringSubmatch(record.Description.Text, -1) {
			text := strings.TrimSpace(part[2])
			if text == "" {
				continue
			}
			if part[1] == "99" {
				completed = text
				continue
			}
			if len(definition.Steps) >= assets.MaxQuestSteps {
				return nil, fmt.Errorf("too many native quest text steps")
			}
			index := len(definition.Steps) + 1
			definition.Steps = append(definition.Steps, assets.QuestStep{Index: index, Type: assets.QuestDialogue, PromptDialogue: text, InProgressDialogue: text, CompleteDialogue: text})
		}
		if len(definition.Steps) > 0 {
			definition.IntroDialogue = definition.Steps[0].PromptDialogue
			definition.InProgressDialogue = definition.IntroDialogue
		}
		if completed != "" {
			definition.CompleteDialogue = completed
			definition.AlreadyCompletedDialogue = completed
			if len(definition.Steps) > 0 {
				definition.Steps[len(definition.Steps)-1].CompleteDialogue = completed
			}
		}
		out[uint32(id)] = definition
	}
	return out, nil
}

// Native #01/#02/.../#99 delimit text, not executable objective types.
var markStepText = regexp.MustCompile(`#(\d{2})([^#]*)`)

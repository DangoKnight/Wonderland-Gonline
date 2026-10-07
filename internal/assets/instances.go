package assets

import (
	"encoding/json"
	"fmt"
)

// InstanceDefinition projects TFSceneData's instance fields (FUN_0047be24).
// Dungeon scripts and rewards are not present in these records.
type InstanceDefinition struct {
	ID           uint16
	Name         string
	Description  string
	MarkID       uint16
	Capacity     byte
	GuildOnly    bool
	MinimumLevel uint16
	Minutes      uint16
	EntryMap     uint16
	EntryX       uint16
	EntryY       uint16
}

func ParseInstanceDefinitions(raw []byte) ([]InstanceDefinition, error) {
	var doc struct {
		Records []struct {
			Name struct {
				Text string `json:"text"`
			} `json:"name"`
			Fields struct {
				ID            uint16 `json:"id"`
				Capacity      byte   `json:"unknown_u8_offset_96"`
				Guild         byte   `json:"unknown_u8_offset_97"`
				EntryMap      uint16 `json:"unknown_u16_offset_38"`
				MarkReference uint16 `json:"unknown_u16_offset_101"`
				Level         uint16 `json:"unknown_u16_offset_103"`
				Minutes       uint16 `json:"unknown_u16_offset_105"`
				X             uint16 `json:"unknown_u16_offset_107"`
				Y             uint16 `json:"unknown_u16_offset_109"`
			} `json:"fields"`
		} `json:"records"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	out := []InstanceDefinition{}
	seen := map[uint16]bool{}
	for _, r := range doc.Records {
		f := r.Fields
		if f.Capacity == 0 {
			continue
		}
		if f.ID == 0 || seen[f.ID] || f.Minutes == 0 || f.EntryMap == 0 || f.Guild > 1 {
			return nil, fmt.Errorf("invalid instance definition %d", f.ID)
		}
		seen[f.ID] = true
		markID := uint16(0)
		if f.MarkReference != 0 {
			markID = f.MarkReference - 1
		}
		out = append(out, InstanceDefinition{ID: f.ID, Name: r.Name.Text, Capacity: f.Capacity, GuildOnly: f.Guild == 1, MinimumLevel: f.Level, Minutes: f.Minutes, EntryMap: f.EntryMap, EntryX: f.X, EntryY: f.Y, MarkID: markID})
	}
	return out, ValidateInstances(out)
}

// ApplyInstanceText follows FUN_001821bc: SceneData +0x6b is Mark ID +1.
func ApplyInstanceText(defs []InstanceDefinition, raw []byte) error {
	var doc struct {
		Records []struct {
			Fields struct {
				ID uint16 `json:"id"`
			} `json:"fields"`
			Name struct {
				Text string `json:"text"`
			} `json:"name"`
			Description struct {
				Text string `json:"text"`
			} `json:"description"`
		} `json:"records"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return err
	}
	for i := range defs {
		for _, r := range doc.Records {
			if defs[i].MarkID != 0 && defs[i].MarkID == r.Fields.ID {
				defs[i].Name = r.Name.Text
				defs[i].Description = r.Description.Text
				break
			}
		}
	}
	return nil
}

const MaxInstanceDefinitions = 35 // TDupMisManage's native alias groups.
func ValidateInstances(defs []InstanceDefinition) error {
	if len(defs) > MaxInstanceDefinitions {
		return fmt.Errorf("too many native instance definitions")
	}
	seen := map[uint16]bool{}
	for _, d := range defs {
		if d.ID == 0 || seen[d.ID] || d.Capacity == 0 || d.Minutes == 0 || d.EntryMap == 0 || d.Name == "" {
			return fmt.Errorf("invalid instance %d", d.ID)
		}
		seen[d.ID] = true
	}
	return nil
}

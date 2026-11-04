package assets

import (
	_ "embed"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
)

// NPCNamePatch is an ID-specific display-name restoration from the saved
// WLOHub reference. OriginalName guards against replacing administrator edits.
type NPCNamePatch struct {
	OriginalName string
	Name         string
	PreviousName string // Earlier WLOHub display name, before removing Quest annotations.
}

//go:embed npc_names.csv
var npcNamePatchCSV string

// NPCNamePatches supplies offline asset initialization and the compiled client
// display patch. Server gameplay must use the resulting SQL NPC definitions.
func NPCNamePatches() (map[uint16]NPCNamePatch, error) {
	rows, err := csv.NewReader(strings.NewReader(npcNamePatchCSV)).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 || strings.Join(rows[0], ",") != "id,original_name,name,source_ids,previous_name" {
		return nil, fmt.Errorf("invalid NPC name patch header")
	}
	patches := make(map[uint16]NPCNamePatch, len(rows)-1)
	for _, row := range rows[1:] {
		id, err := strconv.ParseUint(row[0], 10, 16)
		// Reviewed corrections can restore missing leading/middle words, such as
		// Snake -> Poisonous Snake. The native ID and original name guard each edit.
		if err != nil || id == 0 || len(row[1]) == 0 || len(row[1]) > nativeNPCNameBytes || len(row[2]) < len(row[1]) || row[3] == "" {
			return nil, fmt.Errorf("invalid NPC name patch %q", row)
		}
		if _, exists := patches[uint16(id)]; exists {
			return nil, fmt.Errorf("duplicate NPC name patch %d", id)
		}
		patches[uint16(id)] = NPCNamePatch{OriginalName: row[1], Name: row[2], PreviousName: row[4]}
	}
	return patches, nil
}

// Apply preserves full/custom names and only expands the verified source name.
func (p NPCNamePatch) Apply(current string) string {
	if current == p.OriginalName || p.PreviousName != "" && current == p.PreviousName {
		return p.Name
	}
	return current
}

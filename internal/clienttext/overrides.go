// Package clienttext compiles text overrides into client asset documents offline.
// It must not be imported by the game. Native bytes and server SQL stay intact.
package clienttext

import (
	_ "embed"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

type Table string

const (
	NPC         Table = "npc"
	Item        Table = "item"
	Skill       Table = "skill"
	Scene       Table = "scene"
	Talk        Table = "talk"
	Mark        Table = "mark"
	Name              = "name"
	Description       = "description"
	Text              = "text"
)

//go:embed igg_overrides.csv
var iggCSV string

//go:embed dialogue_overrides.csv
var dialogueCSV string

//go:embed chinese_text.csv
var chineseCSV string

type key struct {
	table Table
	id    uint32
	field string
}
type patch struct{ original, english string }

var englishOverrides = sync.OnceValue(func() map[key]patch {
	rows, err := parseOverrides(iggCSV)
	if err != nil {
		panic(fmt.Sprintf("IGG English overrides: %v", err))
	}
	return rows
})

var dialogueOverrides = sync.OnceValue(func() map[key]patch {
	r := csv.NewReader(strings.NewReader(dialogueCSV))
	rows, err := r.ReadAll()
	if err != nil || len(rows) == 0 || strings.Join(rows[0], ",") != "id,original,text" {
		panic("invalid dialogue override CSV")
	}
	out := map[key]patch{}
	for _, row := range rows[1:] {
		id, err := strconv.ParseUint(row[0], 10, 32)
		k := key{Talk, uint32(id), Text}
		if _, exists := out[k]; err != nil || id == 0 || exists || row[1] == "" || row[2] == "" {
			panic("invalid dialogue override entry")
		}
		out[k] = patch{row[1], row[2]}
	}
	return out
})

// Override changes only the source text verified by the offline matcher.
// Every lookup uses native identity, never file order or a global text replace.
func Override(table Table, id uint32, field, current string) string {
	value, _ := Resolve(table, id, field, current)
	return value
}

// Resolve reports an IGG match or an explicit authored display correction.
// Callers apply other name patches only when no such match exists. Dialogue
// corrections apply after translation, with their own exact source guards.
func Resolve(table Table, id uint32, field, current string) (string, bool) {
	matched := false
	if p, ok := englishOverrides()[key{table, id, field}]; ok && current == p.original {
		current, matched = p.english, true
	}
	if p, ok := dialogueOverrides()[key{table, id, field}]; ok && current == p.original {
		return p.english, true
	}
	return current, matched
}

func parseOverrides(data string) (map[key]patch, error) {
	r := csv.NewReader(strings.NewReader(data))
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	const header = "table,id,field,original,english,match,source_record_sha256,target_record_sha256,source"
	if len(rows) == 0 || strings.Join(rows[0], ",") != header {
		return nil, fmt.Errorf("invalid header")
	}
	result := make(map[key]patch, len(rows)-1)
	for _, row := range rows[1:] {
		id, err := strconv.ParseUint(row[1], 10, 32)
		table, field := Table(row[0]), row[2]
		valid := false
		switch table {
		case NPC, Scene:
			valid = field == Name
		case Item, Skill, Mark:
			valid = field == Name || field == Description
		case Talk:
			valid = field == Text
		}
		if err != nil || id == 0 || !valid || row[4] == "" || row[5] == "" || row[8] == "" || strings.ContainsRune(row[4], 0) {
			return nil, fmt.Errorf("invalid entry %s/%s/%s", table, row[1], field)
		}
		for _, digest := range row[6:8] {
			b, err := hex.DecodeString(digest)
			if err != nil || len(b) != 32 {
				return nil, fmt.Errorf("invalid record fingerprint")
			}
		}
		k := key{table, uint32(id), field}
		if _, ok := result[k]; ok {
			return nil, fmt.Errorf("duplicate entry %v", k)
		}
		result[k] = patch{row[3], row[4]}
	}
	return result, nil
}

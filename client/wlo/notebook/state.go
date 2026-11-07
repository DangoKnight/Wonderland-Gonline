package notebook

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"wonderland-gonline/internal/localization"
	"wonderland-gonline/internal/protocol"
)

const (
	RowsPerPage      = 10
	CompletionFlags  = 2000
	MaximumBookIndex = 5500
)

type Mark struct {
	ID, Flag, Journal uint16
	Name, Description string
}
type Monster struct {
	ID, Index      uint16
	Name           string
	Level, Element byte
	HP, SP         uint32
	Stats          [5]uint16
}
type Entry struct {
	ID                uint16
	Name, Description string
	Completed         bool
}
type State struct {
	Marks      map[uint16]Mark
	Monsters   map[uint16]Monster
	Active     map[uint16]byte
	Flags      [CompletionFlags / 8]byte
	Discovered map[uint16]bool
}

func (s *State) Reset() {
	s.Active = map[uint16]byte{}
	s.Flags = [CompletionFlags / 8]byte{}
	s.Discovered = map[uint16]bool{}
}
func (s *State) Apply(p []byte) bool {
	if len(p) < 2 {
		return false
	}
	if s.Active == nil {
		s.Reset()
	}
	if p[0] == protocol.CommandMonsterBook {
		if len(p) != 6 || p[1] != protocol.MonsterBookDiscover {
			return false
		}
		id := binary.LittleEndian.Uint32(p[2:])
		if id == 0 || id > 65535 {
			return false
		}
		s.Discovered[uint16(id)] = true
		return true
	}
	if p[0] != protocol.CommandQuest {
		return false
	}
	switch p[1] {
	case protocol.QuestMarkState:
		if len(p) != 5 {
			return false
		}
		s.Active[binary.LittleEndian.Uint16(p[2:])] = p[4]
	case protocol.QuestMark:
		if len(p) != 4 {
			return false
		}
		delete(s.Active, binary.LittleEndian.Uint16(p[2:]))
	case protocol.QuestFlag:
		if len(p) != 5 {
			return false
		}
		id := int(binary.LittleEndian.Uint16(p[2:]))
		if id < 1 || id > CompletionFlags || p[4] > 1 {
			return false
		}
		mask := byte(1 << ((id - 1) % 8))
		if p[4] == 1 {
			s.Flags[(id-1)/8] |= mask
		} else {
			s.Flags[(id-1)/8] &= ^mask
		}
	case protocol.QuestActiveMarks:
		if (len(p)-2)%4 != 0 {
			return false
		}
		active := map[uint16]byte{}
		for i := 2; i < len(p); i += 4 {
			if p[i] == 0 || p[i] > 200 {
				return false
			}
			id := binary.LittleEndian.Uint16(p[i+1:])
			if id != 0 && p[i+3] != 0 {
				active[id] = p[i+3]
			}
		}
		s.Active = active
	case protocol.QuestWireCode7:
		if (len(p)-2)%2 != 0 {
			return false
		}
		var flags [CompletionFlags / 8]byte
		for i := 2; i < len(p); i += 2 {
			if p[i] == 0 || int(p[i]) > len(flags) {
				return false
			}
			flags[int(p[i])-1] = p[i+1]
		}
		s.Flags = flags
	default:
		return false
	}
	return true
}

var stepPattern = regexp.MustCompile(`#([0-9]{2})([^#]*)`)

func StepText(raw string, step byte) string {
	parts := stepPattern.FindAllStringSubmatch(raw, -1)
	if len(parts) == 0 {
		return strings.TrimSpace(raw)
	}
	wanted := fmt.Sprintf("%02d", step)
	for _, part := range parts {
		if part[1] == wanted {
			return strings.TrimSpace(part[2])
		}
	}
	return ""
}
func (s *State) Entries() []Entry {
	entries := map[uint16]Entry{}
	for id, step := range s.Active {
		m, ok := s.Marks[id]
		if !ok || m.Flag != 0 || step == 0 {
			continue
		}
		entries[id] = Entry{id, m.Name, StepText(m.Description, step), false}
	}
	for _, m := range s.Marks {
		if m.Flag == 0 || m.Flag > CompletionFlags || m.Journal == 0 || s.Flags[(m.Flag-1)/8]&(1<<((m.Flag-1)%8)) == 0 {
			continue
		}
		base := s.Marks[m.Journal]
		entries[base.ID] = Entry{base.ID, base.Name, StepText(base.Description, 99), true}
	}
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func (s *State) MonsterList() []Monster {
	out := []Monster{}
	for id := range s.Discovered {
		if m, ok := s.Monsters[id]; ok && m.Index > 0 && m.Index <= MaximumBookIndex {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Index != out[j].Index {
			return out[i].Index < out[j].Index
		}
		return out[i].ID < out[j].ID
	})
	unique := out[:0]
	for _, m := range out {
		if len(unique) == 0 || unique[len(unique)-1].Index != m.Index {
			unique = append(unique, m)
		}
	}
	return unique
}

func (s *State) Load(markPath, npcPath string) error {
	raw, err := localization.ReadAsset(markPath)
	if err != nil {
		return err
	}
	var marks struct {
		Records []struct {
			Name, Description struct {
				Text string `json:"text"`
			}
			Fields struct {
				ID   uint16 `json:"id"`
				Flag uint16 `json:"completion_flag"`
			}
		}
	}
	if err = json.Unmarshal(raw, &marks); err != nil {
		return err
	}
	definitions := map[uint16]Mark{}
	journals := map[string]uint16{}
	for _, r := range marks.Records {
		m := Mark{ID: r.Fields.ID, Flag: r.Fields.Flag, Name: r.Name.Text, Description: r.Description.Text}
		if m.Flag == 0 {
			journals[m.Name] = m.ID
		} else {
			m.Journal = journals[m.Name]
		}
		definitions[m.ID] = m
	}
	raw, err = localization.ReadAsset(npcPath)
	if err != nil {
		return err
	}
	var npcs struct {
		Records []struct {
			Name struct {
				Text string `json:"text"`
			}
			Fields struct {
				ID      uint16 `json:"id"`
				Index   uint16 `json:"book_index"`
				Level   byte   `json:"level"`
				Element byte   `json:"element"`
				HP, SP  uint32
				STR     uint16 `json:"strength"`
				CON     uint16 `json:"constitution"`
				INT     uint16 `json:"intelligence"`
				WIS     uint16 `json:"wisdom"`
				AGI     uint16 `json:"agility"`
			}
		}
	}
	if err = json.Unmarshal(raw, &npcs); err != nil {
		return err
	}
	monsters := map[uint16]Monster{}
	for _, r := range npcs.Records {
		f := r.Fields
		monsters[f.ID] = Monster{ID: f.ID, Index: f.Index, Name: r.Name.Text, Level: f.Level, Element: f.Element, HP: f.HP, SP: f.SP, Stats: [5]uint16{f.STR, f.CON, f.INT, f.WIS, f.AGI}}
	}
	s.Marks, s.Monsters = definitions, monsters
	return nil
}

package assets

import (
	"bytes"
	"fmt"
	"golang.org/x/text/encoding/traditionalchinese"
	"strings"
	"wonderland-go/internal/protocol"
)

type Map struct {
	Sections       [11][]byte    `json:"-"`
	Entries        []AreaEntry   `json:"entries"`
	Mining         []AreaEntry   `json:"mining"`
	Groups         []Group       `json:"groups"`
	Interactive    []Group       `json:"interactive"`
	Battles        []BattleEntry `json:"battles"`
	ExtendedGroups []Group       `json:"extended_groups"`
	ID             uint16        `json:"id"`
	Scene          uint16        `json:"scene"`
	NPCs           []MapNPC      `json:"npcs"`
	Items          []GroundItem  `json:"items"`
	Warps          []Warp        `json:"warps"`
	Events         []Event       `json:"events"`
	PreEvents      []Event       `json:"pre_events"`
}
type MapNPC struct {
	ClickID      uint16
	Name         string
	Flags        byte // Bit 0 is initial visibility.
	X, Y         uint32
	Events       []byte
	Links        []byte // Door NPCs name their linked portal first.
	Template     uint32
	Rotation     byte
	WalkBehavior byte
	WalkSteps    []WalkStep
	UnknownWords [4]uint16
}
type WalkStep struct{ X, Y, Delay uint32 }
type GroundItem struct {
	ClickID uint16
	Name    string
	X, Y    uint32
	ItemID  uint32
	Unknown [2]uint16
}
type Warp struct {
	ClickID uint16
	Name    string
	MapID   uint16
	X, Y    uint32
	Flags   [3]byte
}

// Preserve all 21 bytes of native conditions and operations for the interpreter.
type Event struct {
	ClickID  uint16
	Kind     byte // 10 marks a field-encounter event.
	Name     string
	Branches []Branch
}
type Branch struct {
	Index      byte
	Condition  [21]byte
	Operations []Operation
}
type Operation struct {
	Index byte
	Data  [21]byte
}

func eveName(b []byte) string {
	if end := bytes.IndexByte(b, 0); end >= 0 {
		b = b[:end]
	}
	text, err := traditionalchinese.Big5.NewDecoder().Bytes(b)
	if err != nil {
		return strings.TrimSpace(strings.ToValidUTF8(string(b), "�"))
	}
	return strings.TrimSpace(string(text))
}

func ParseEVE(b []byte) (map[uint16]Map, error) {
	if len(b) < 12 || len(b) > 64<<20 {
		return nil, fmt.Errorf("EVE: invalid size")
	}
	count := int(le.Uint32(b[8:]))
	if count > (len(b)-12)/10 {
		return nil, fmt.Errorf("EVE: invalid map index")
	}
	out := map[uint16]Map{}
	for i := 0; i < count; i++ {
		h := b[12+i*10 : 22+i*10]
		id, scene := le.Uint16(h), le.Uint16(h[2:])
		start, n := int(le.Uint32(h[4:])), int(le.Uint16(h[8:]))
		if start < 12+count*10 || n < 44 || start > len(b)-n {
			return nil, fmt.Errorf("EVE: map %d invalid bounds", id)
		}
		if _, exists := out[id]; exists {
			return nil, fmt.Errorf("EVE: duplicate map %d", id)
		}
		m, err := ParseEVEMap(id, scene, b[start:start+n])
		if err != nil {
			return nil, err
		}
		out[id] = m
	}
	return out, nil
}

// ParseEVEMap decodes one map's record (its index row's byte range): the
// eleven category sections located by the 44-byte offset table at its end.
func ParseEVEMap(id, scene uint16, data []byte) (Map, error) {
	n := len(data)
	if n < 44 {
		return Map{}, fmt.Errorf("EVE: map %d invalid bounds", id)
	}
	m := Map{ID: id, Scene: scene}
	offsets := data[n-44:]
	for _, category := range []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10} {
		offset := int(le.Uint32(offsets[category*4:]))
		if offset == 0 {
			continue
		}
		end := n - 44
		for j := 0; j < 11; j++ {
			next := int(le.Uint32(offsets[j*4:]))
			if next > offset && next < end {
				end = next
			}
		}
		if offset < 0 || offset >= end {
			return Map{}, fmt.Errorf("EVE: map %d category %d bounds", id, category)
		}
		m.Sections[category] = append([]byte(nil), data[offset:end]...)
		r := protocol.NewReader(data[offset:end])
		entries := int(r.U16())
		for range entries {
			if r.Err() != nil {
				break
			}
			switch category {
			case 0:
				v := MapNPC{ClickID: r.U16(), Name: eveName(r.Bytes(20))}
				v.Flags = r.U8()
				v.X = r.U32()
				v.Y = r.U32()
				v.Events = append([]byte(nil), r.Bytes(int(r.U8()))...)
				v.Links = append([]byte(nil), r.Bytes(int(r.U8()))...)
				r.U8()
				v.Template = r.U32()
				v.Rotation = r.U8()
				v.WalkBehavior = r.U8()
				r.U8()
				steps := int(r.U8())
				for range steps {
					v.WalkSteps = append(v.WalkSteps, WalkStep{r.U32(), r.U32(), r.U32()})
				}
				r.Bytes(13)
				patterns := int(r.U8())
				r.Bytes(patterns * 92)
				for j := range v.UnknownWords {
					v.UnknownWords[j] = r.U16()
				}
				m.NPCs = append(m.NPCs, v)
			case 1, 2:
				v := AreaEntry{ClickID: r.U16(), Name: eveName(r.Bytes(20))}
				v.Flags = r.U8()
				v.X = r.U32()
				v.Y = r.U32()
				v.Events = append([]byte(nil), r.Bytes(int(r.U8()))...)
				v.Conditions = append([]byte(nil), r.Bytes(int(r.U8()))...)
				size := 18
				if category == 2 {
					// Native mining tails are 11 bytes; the C# loader crosses record boundaries here.
					size = 11
				}
				v.Tail = append([]byte(nil), r.Bytes(size)...)
				if category == 1 {
					m.Entries = append(m.Entries, v)
				} else {
					m.Mining = append(m.Mining, v)
				}
			case 5, 7, 10:
				v := Group{ClickID: r.U16(), Name: eveName(r.Bytes(20))}
				headerSize := 5
				if category == 7 {
					headerSize = 3
				}
				v.Header = append([]byte(nil), r.Bytes(headerSize)...)
				count := 0
				if category == 10 {
					count = int(r.U16())
				} else {
					count = int(r.U8())
				}
				width := 3
				if category == 7 {
					width = 5
				}
				v.Members = append([]byte(nil), r.Bytes(count*width)...)
				if category == 5 {
					v.Trailer = r.U8()
					m.Groups = append(m.Groups, v)
				} else if category == 7 {
					m.Interactive = append(m.Interactive, v)
				} else {
					m.ExtendedGroups = append(m.ExtendedGroups, v)
				}
			case 8:
				v := BattleEntry{ClickID: r.U16(), Name: eveName(r.Bytes(20))}
				copy(v.Header[:], r.Bytes(16))
				v.Team1 = append([]byte(nil), r.Bytes(int(r.U8())*3)...)
				v.Team2 = append([]byte(nil), r.Bytes(int(r.U8())*3)...)
				v.Trailer = r.U8()
				m.Battles = append(m.Battles, v)
			case 3:
				v := GroundItem{ClickID: r.U16()}
				r.U8()
				v.Name = eveName(r.Bytes(19))
				r.U8()
				v.X = r.U32()
				v.Y = r.U32()
				r.Bytes(int(r.U8()))
				r.Bytes(int(r.U8()))
				r.U8()
				v.ItemID = r.U32()
				r.Bytes(3)
				v.Unknown = [2]uint16{r.U16(), r.U16()}
				m.Items = append(m.Items, v)
			case 6:
				v := Warp{ClickID: r.U16(), Name: eveName(r.Bytes(20)), MapID: r.U16(), X: r.U32(), Y: r.U32()}
				for j := range v.Flags {
					v.Flags[j] = r.U8()
				}
				m.Warps = append(m.Warps, v)
			case 4, 9:
				v := Event{ClickID: r.U16()}
				v.Kind = r.U8()
				v.Name = eveName(r.Bytes(20))
				branches := int(r.U8())
				for range branches {
					br := Branch{Index: r.U8()}
					copy(br.Condition[:], r.Bytes(21))
					ops := int(r.U8())
					for range ops {
						op := Operation{Index: r.U8()}
						copy(op.Data[:], r.Bytes(21))
						br.Operations = append(br.Operations, op)
					}
					v.Branches = append(v.Branches, br)
				}
				if category == 4 {
					m.Events = append(m.Events, v)
				} else {
					m.PreEvents = append(m.PreEvents, v)
				}
			}
		}
		if r.Err() != nil {
			return Map{}, fmt.Errorf("EVE: map %d category %d: %w", id, category, r.Err())
		}
	}
	return m, nil
}

// Unknown native fields remain available without assigning guessed semantics.
type AreaEntry struct {
	ClickID                  uint16
	Name                     string
	Flags                    byte
	X, Y                     uint32
	Events, Conditions, Tail []byte
}
type Group struct {
	ClickID         uint16
	Name            string
	Header, Members []byte
	Trailer         byte
}
type BattleEntry struct {
	ClickID      uint16
	Name         string
	Header       [16]byte
	Team1, Team2 []byte
	Trailer      byte
}

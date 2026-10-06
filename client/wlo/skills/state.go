package skills

import (
	"encoding/binary"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

const (
	snapshotCountOffset      = 62 // AC5:3 full packet, before seven-byte order/grade/EXP records.
	snapshotRecordBytes      = 7
	snapshotFooterBytes      = 8
	compatibilityFooterBytes = 6
)

type Progress struct {
	Grade       byte
	EXP         uint32
	Proficiency uint16
}

func FromEXP(grade byte, exp uint32) Progress {
	p := Progress{Grade: grade, EXP: exp}
	if grade != 0 {
		p.Proficiency = uint16(min(uint64(game.SkillProficiencyScale), uint64(exp)*game.SkillProficiencyScale/(uint64(grade)*game.SkillGradeExpScale)))
	}
	return p
}

type State struct {
	Catalog  *Catalog
	Learned  map[uint16]Progress
	Revision uint64
}

func NewState(c *Catalog) *State { return &State{Catalog: c, Learned: map[uint16]Progress{}} }
func (s *State) Reset()          { s.Learned = map[uint16]Progress{}; s.Revision++ }

// Snapshot validates the entire variable-length list before replacing it.
// Table order, rather than skill ID, is carried in the native AC5:3 snapshot.
func (s *State) Snapshot(p []byte) bool {
	if len(p) < snapshotCountOffset+2 || p[0] != protocol.CommandCharacterState || p[1] != protocol.CharacterStateWireCode3 {
		return false
	}
	count := int(binary.LittleEndian.Uint16(p[snapshotCountOffset:]))
	tail := snapshotCountOffset + 2 + count*snapshotRecordBytes
	if len(p) != tail+snapshotFooterBytes && len(p) != tail+compatibilityFooterBytes {
		return false
	}
	next := map[uint16]Progress{}
	orders := map[uint16]bool{}
	for o := snapshotCountOffset + 2; o < tail; o += snapshotRecordBytes {
		order := binary.LittleEndian.Uint16(p[o:])
		grade := p[o+2]
		if order == 0 || orders[order] || grade > game.MaxSkillGrade {
			return false
		}
		orders[order] = true
		id, known := s.Catalog.Orders[order]
		if !known {
			continue
		}
		next[id] = FromEXP(grade, binary.LittleEndian.Uint32(p[o+3:]))
	}
	s.Learned = next
	s.Revision++
	return true
}
func (s *State) Apply(p []byte) bool {
	if len(p) < 2 {
		return false
	}
	var id uint32
	var value uint32
	var grade bool
	switch {
	case p[0] == protocol.CommandCharacterState && p[1] == protocol.CharacterStateWireCode12 && len(p) == 5:
		id = uint32(binary.LittleEndian.Uint16(p[2:]))
		value = uint32(p[4])
		grade = true
	case p[0] == protocol.CommandCharacterState && p[1] == protocol.CharacterStateSkillProficiency && len(p) == 8:
		id = binary.LittleEndian.Uint32(p[2:])
		value = uint32(binary.LittleEndian.Uint16(p[6:]))
	case p[0] == protocol.CommandStats && p[1] == protocol.StatsStatUpdate && len(p) == 12 && p[2] == game.StatSkillGrade && p[3] == protocol.StatsValueAbsolute:
		id = binary.LittleEndian.Uint32(p[8:])
		value = binary.LittleEndian.Uint32(p[4:])
		grade = true
	default:
		return false
	}
	if id == 0 || id > 65535 || (grade && value > game.MaxSkillGrade) || (!grade && value > game.SkillProficiencyScale) {
		return false
	}
	key := uint16(id)
	if _, ok := s.Catalog.Definitions[key]; !ok {
		return false
	}
	v := s.Learned[key]
	if grade {
		v.Grade = byte(value)
	} else {
		v.Proficiency = uint16(value)
	}
	if grade && value == 0 {
		delete(s.Learned, key)
	} else {
		s.Learned[key] = v
	}
	s.Revision++
	return true
}

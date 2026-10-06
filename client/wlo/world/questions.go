package world

import (
	"encoding/binary"

	native "wonderland-gonline/internal/assets"
)

// Event questions: the map record's category 7 (eve.Emg), copied by the map
// loader into the table at +0x5d9c (0x392fc0): per question its click ID, a
// mode (+0x42, the header's last byte: 0 the talk window, 1 and 3 other
// forms) and its entries, each a value word, a kind (1 a talk text, 2 an
// item) and a type: 1 the prompt, 2 a Yes/No button, 3 an option, 4 not
// shown (FUN_00304fd0 case 6).
type Question struct {
	Mode    byte
	Entries []QuestionEntry
}

// QuestionEntry is one 5-byte member: index, value, kind, type.
type QuestionEntry struct {
	Value      uint16
	Kind, Type byte
}

// Question entry kinds and types.
const (
	QuestionText     = 1
	QuestionItem     = 2
	QuestionPrompt   = 1
	QuestionYesNo    = 2
	QuestionOption   = 3
	questionMember   = 5
	questionModeByte = 2
)

// MapQuestions reads a map record's questions by click ID.
func MapQuestions(rec native.Map) map[uint16]Question {
	out := make(map[uint16]Question, len(rec.Interactive))
	for _, g := range rec.Interactive {
		q := Question{}
		if len(g.Header) > questionModeByte {
			q.Mode = g.Header[questionModeByte]
		}
		for m := g.Members; len(m) >= questionMember; m = m[questionMember:] {
			q.Entries = append(q.Entries, QuestionEntry{Value: binary.LittleEndian.Uint16(m[1:]), Kind: m[3], Type: m[4]})
		}
		out[g.ClickID] = q
	}
	return out
}

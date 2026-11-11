package protocol

import "encoding/binary"

// AFK uses a negotiated connection feature, never native client detection.
const (
	CommandAFK      = 253
	AFKSet          = 1 // C>S mode byte; S>C accepted mode byte, ordered before snapshot.
	AFKSnapshot     = 2
	AFKSceneBegin   = 3
	AFKSceneEnd     = 4
	AFKRejected     = 5
	AFKNormal       = 0
	AFKActive       = 1
	AFKPlayer       = 0
	AFKPet          = 1
	AFKMaxUnits     = 20 // Four owners, each with up to four pets.
	AFKMaxNameBytes = 80
)

type AFKUnit struct {
	Owner, ID                 uint32
	Kind                      byte
	Name                      string
	Level                     byte
	HP, MaxHP, SP, MaxSP, EXP uint32
	EXPNext                   uint64
	Amity                     byte
}

type AFKState struct {
	Map, X, Y uint16
	Units     []AFKUnit
}

func EncodeAFKState(s AFKState) ([]byte, error) {
	if len(s.Units) > AFKMaxUnits {
		return nil, ErrMalformed
	}
	p := Builder{CommandAFK, AFKSnapshot}.U16(s.Map).U16(s.X).U16(s.Y).U8(byte(len(s.Units)))
	for _, u := range s.Units {
		if len(u.Name) > AFKMaxNameBytes || u.Kind > AFKPet {
			return nil, ErrMalformed
		}
		p = p.U32(u.Owner).U32(u.ID).U8(u.Kind).U8(u.Level)
		var err error
		p, err = p.String(u.Name)
		if err != nil {
			return nil, err
		}
		p = p.U32(u.HP).U32(u.MaxHP).U32(u.SP).U32(u.MaxSP).U32(u.EXP).U64(u.EXPNext).U8(u.Amity)
	}
	return p, nil
}

func DecodeAFKState(p []byte) (AFKState, error) {
	if len(p) < 9 || p[0] != CommandAFK || p[1] != AFKSnapshot {
		return AFKState{}, ErrMalformed
	}
	r := NewReader(p[2:])
	s := AFKState{Map: r.U16(), X: r.U16(), Y: r.U16()}
	n := int(r.U8())
	if n > AFKMaxUnits {
		return AFKState{}, ErrMalformed
	}
	for range n {
		u := AFKUnit{Owner: r.U32(), ID: r.U32(), Kind: r.U8(), Level: r.U8(), Name: r.String()}
		u.HP, u.MaxHP, u.SP, u.MaxSP, u.EXP = r.U32(), r.U32(), r.U32(), r.U32(), r.U32()
		u.EXPNext, u.Amity = r.U64(), r.U8()
		if u.Kind > AFKPet || len(u.Name) > AFKMaxNameBytes {
			return AFKState{}, ErrMalformed
		}
		s.Units = append(s.Units, u)
	}
	if r.Err() != nil || r.Remaining() != 0 {
		return AFKState{}, ErrMalformed
	}
	return s, nil
}

// AFKNonessential is deliberately a narrow allowlist of droppable presentation.
// Unknown packets, NPCs, team formation/vitals, system/GM notices, acknowledgments,
// inventory and all battle/event traffic are vital and always retained.
func AFKNonessential(p []byte, self uint32, party []uint32) bool {
	if len(p) < 2 {
		return false
	}
	if p[0] == CommandChat {
		switch p[1] {
		case ChatWorldMessage, ChatMapMessage, ChatWhisperMessage, ChatTeamMessage, ChatGuildMessage, ChatAllyMessage:
			return true
		}
	}
	var id uint32
	switch p[0] {
	case CommandAppearance:
		if len(p) < 5 {
			return false
		}
		id = binary.LittleEndian.Uint32(p[1:])
	case CommandPosition:
		if len(p) < 5 {
			return false
		}
		id = binary.LittleEndian.Uint32(p[1:])
	case CommandMovement:
		if p[1] != MovementMove || len(p) < 6 {
			return false
		}
		id = binary.LittleEndian.Uint32(p[2:])
	case CommandPose:
		if p[1] != PoseBroadcast || len(p) < 6 {
			return false
		}
		id = binary.LittleEndian.Uint32(p[2:])
	case CommandCharacterState:
		if p[1] != CharacterStateEquipmentSnapshot && p[1] != CharacterStateSpriteRefresh {
			return false
		}
		if len(p) < 6 {
			return false
		}
		id = binary.LittleEndian.Uint32(p[2:])
	default:
		return false
	}
	if id == 0 || id == self {
		return false
	}
	for _, member := range party {
		if id == member {
			return false
		}
	}
	return true
}

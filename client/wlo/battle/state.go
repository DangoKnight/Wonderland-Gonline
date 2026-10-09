// Package battle ports TFightManage's native battle roster and action records.
package battle

import (
	"encoding/binary"
	"fmt"
	"image"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

const (
	GridColumns        = 4
	GridRows           = 4
	MaximumFighters    = GridColumns * GridRows
	FighterRecordBytes = 32
	Player             = 2
	Pet                = 4
	Monster            = 7
	FriendlyPet        = 5
	Attacker           = 1
	Defender           = 2
	actorHeaderBytes   = 6
	targetHeaderBytes  = 5
	statResultBytes    = 6
	AbsoluteStatBytes  = 7
	maximumStatResults = 9 // Native variable stat-result compatibility limit.
	// FUN_0038c64c, verified against the x87 constants and instructions in aLogin.
	formationColumnStep  = 25 // round(75 / sqrt(1))/3
	formationRowStep     = 63 // round(75 / sqrt(2)) + 10
	formationTop         = 266
	nativeCaptureSuccess = 10008
)

type Cell struct{ X, Y byte }

func (c Cell) Valid() bool { return c.X >= 1 && c.X <= GridColumns && c.Y >= 1 && c.Y <= GridRows }

// Position is the native formation's feet position, FUN_0038c64c.
func (c Cell) Position() image.Point {
	if !c.Valid() {
		return image.Point{}
	}
	bases := [GridColumns]int{210, 280, 540, 610}
	sign := 1
	if c.X <= 2 {
		sign = -1
	}
	return image.Pt(bases[c.X-1]+sign*formationColumnStep*int(c.Y), formationTop+formationRowStep*int(c.Y))
}

type Fighter struct {
	Cell
	Side, Kind         byte
	ID, Owner          uint32
	Click              uint16
	MaxHP, HP          uint32
	MaxSP, SP          uint16
	Level, Element     byte
	Submitted, Removed bool
}
type StatResult struct {
	Stat   byte
	Amount uint32
	Mode   byte
}
type Target struct {
	Cell
	Result, Guard byte
	Stats         []StatResult
}
type Action struct {
	Actor   Cell
	Skill   uint16
	Flags   byte
	Targets []Target
}
type State struct {
	Active, Ready, Finished bool
	Background              uint16
	Self                    uint32
	Side                    byte
	Fighters                []Fighter
	Actions                 []Action
	Turn                    uint64
	Prompt                  bool
	// Kind is AC11:10's battle kind (FUN_00395efc, +4), which sets the
	// turn limit.
	Kind byte
}

// Native turn limits, FUN_00395efc: 30 seconds for kinds 2, 4 and 7-9,
// 20 otherwise.
const (
	TurnLimitShort = 20
	TurnLimitLong  = 30
)

// TurnLimit is the native countdown's start (+0xce2).
func (s *State) TurnLimit() int {
	switch s.Kind {
	case 2, 4, 7, 8, 9:
		return TurnLimitLong
	}
	return TurnLimitShort
}

func (s *State) At(cell Cell) *Fighter {
	for i := range s.Fighters {
		if s.Fighters[i].Cell == cell {
			return &s.Fighters[i]
		}
	}
	return nil
}
func (s *State) Owned(f *Fighter) bool {
	return f != nil && (f.Kind == Player && f.ID == s.Self || f.Kind == Pet && f.Owner == s.Self)
}
func (s *State) CanChoose(cell Cell) bool {
	f := s.At(cell)
	return s.Active && s.Ready && !s.Finished && s.Owned(f) && f.HP > 0 && !f.Submitted && !f.Removed
}
func parseFighters(p []byte) ([]Fighter, error) {
	if len(p) == 0 || len(p)%FighterRecordBytes != 0 || len(p)/FighterRecordBytes > MaximumFighters {
		return nil, fmt.Errorf("invalid fighter records")
	}
	var out []Fighter
	seen := map[Cell]bool{}
	for len(p) > 0 {
		v := p[:FighterRecordBytes]
		p = p[FighterRecordBytes:]
		f := Fighter{Cell: Cell{v[12], v[13]}, Side: v[0], Kind: v[1], ID: binary.LittleEndian.Uint32(v[2:]), Click: binary.LittleEndian.Uint16(v[6:]), Owner: binary.LittleEndian.Uint32(v[8:]), MaxHP: binary.LittleEndian.Uint32(v[14:]), MaxSP: binary.LittleEndian.Uint16(v[18:]), HP: binary.LittleEndian.Uint32(v[20:]), SP: binary.LittleEndian.Uint16(v[24:]), Level: v[26], Element: v[27]}
		if f.Side == FriendlyPet && f.Kind == Pet {
			if f.X <= 2 {
				f.Side = Defender
			} else {
				f.Side = Attacker
			}
		}
		if !f.Cell.Valid() || seen[f.Cell] || f.ID == 0 || f.HP > f.MaxHP || f.SP > f.MaxSP || f.Side != Attacker && f.Side != Defender || f.Kind != Player && f.Kind != Pet && f.Kind != Monster {
			return nil, fmt.Errorf("invalid fighter")
		}
		seen[f.Cell] = true
		out = append(out, f)
	}
	return out, nil
}

// ParseActions follows FUN_003920a0 / FUN_00398dac. Each actor has a U16
// length, a six-byte header, and variable targets with six-byte stat results.
// Decode the entire packet before modifying any fighter or animation queue.
func ParseActions(p []byte) ([]Action, error) {
	var out []Action
	if len(p) == 0 {
		return nil, fmt.Errorf("empty actions")
	}
	for len(p) > 0 {
		if len(p) < 2 {
			return nil, fmt.Errorf("short actor length")
		}
		n := int(binary.LittleEndian.Uint16(p))
		p = p[2:]
		if n < actorHeaderBytes || n > len(p) || len(out) >= MaximumFighters {
			return nil, fmt.Errorf("invalid actor length")
		}
		v := p[:n]
		p = p[n:]
		a := Action{Actor: Cell{v[0], v[1]}, Skill: binary.LittleEndian.Uint16(v[2:]), Flags: v[4]}
		count := int(v[5])
		v = v[actorHeaderBytes:]
		if !a.Actor.Valid() || count < 1 || count > MaximumFighters {
			return nil, fmt.Errorf("invalid actor")
		}
		for i := 0; i < count; i++ {
			if len(v) < targetHeaderBytes {
				return nil, fmt.Errorf("short target")
			}
			t := Target{Cell: Cell{v[0], v[1]}, Result: v[2], Guard: v[3]}
			stats := int(v[4])
			v = v[targetHeaderBytes:]
			if !t.Cell.Valid() || stats > maximumStatResults || len(v) < stats*statResultBytes {
				return nil, fmt.Errorf("invalid target stats")
			}
			for j := 0; j < stats; j++ {
				t.Stats = append(t.Stats, StatResult{v[0], binary.LittleEndian.Uint32(v[1:]), v[5]})
				v = v[statResultBytes:]
			}
			a.Targets = append(a.Targets, t)
		}
		if len(v) != 0 {
			return nil, fmt.Errorf("actor trailing bytes")
		}
		out = append(out, a)
	}
	return out, nil
}

// Apply accepts only complete native layouts. Known malformed packets report
// handled=true, valid=false and leave state untouched.
func (s *State) Apply(p []byte) (handled, valid bool) {
	if len(p) < 2 {
		return false, false
	}
	switch p[0] {
	case protocol.CommandBattleState:
		switch p[1] {
		case protocol.BattleStateFormation, protocol.BattleStateParticipant:
			offset := 2
			if p[1] == protocol.BattleStateFormation {
				offset = 4
			}
			if len(p) < offset {
				return true, false
			}
			roster, err := parseFighters(p[offset:])
			if err != nil {
				return true, false
			}
			next := *s
			next.Fighters = append([]Fighter(nil), s.Fighters...)
			if p[1] == protocol.BattleStateFormation {
				own := false
				for _, f := range roster {
					if f.Kind == Player && f.ID == s.Self {
						own = true
						next.Side = f.Side
					}
				}
				if !own {
					return true, false
				}
				next = State{Self: s.Self, Side: next.Side, Active: true, Background: binary.LittleEndian.Uint16(p[2:]), Turn: s.Turn + 1}
			} else if !s.Active {
				return true, false
			}
			for _, f := range roster {
				if old := next.At(f.Cell); old != nil {
					*old = f
				} else {
					next.Fighters = append(next.Fighters, f)
				}
			}
			if len(next.Fighters) > MaximumFighters {
				return true, false
			}
			*s = next
			return true, true
		case protocol.BattleStateInitialize:
			if len(p) != 3 || !s.Active {
				return true, false
			}
			s.Kind = p[2]
			return true, true
		case protocol.BattleStateFinish:
			if len(p) < 2 || !s.Active {
				return true, false
			}
			s.Ready = false
			s.Finished = true
			return true, true
		case protocol.BattleStateWireCode0:
			if len(p) != 8 {
				return true, false
			}
			if binary.LittleEndian.Uint32(p[2:]) == s.Self {
				*s = State{Self: s.Self, Turn: s.Turn + 1}
			}
			return true, true
		case protocol.BattleStateExit:
			if (len(p) != 4 && len(p) != 5) || !(Cell{p[2], p[3]}).Valid() {
				return true, false
			}
			if f := s.At(Cell{p[2], p[3]}); f != nil {
				f.Removed = true
			}
			return true, true
		}
	case protocol.CommandBattleStat:
		if p[1] != protocol.BattleStatValues {
			return false, false
		}
		if !s.Active || len(p) == 2 || (len(p)-2)%AbsoluteStatBytes != 0 {
			return true, false
		}
		for i := 2; i < len(p); i += AbsoluteStatBytes {
			if !(Cell{p[i], p[i+1]}).Valid() {
				return true, false
			}
		}
		for i := 2; i < len(p); i += AbsoluteStatBytes {
			if f := s.At(Cell{p[i], p[i+1]}); f != nil {
				v := binary.LittleEndian.Uint32(p[i+3:])
				switch p[i+2] {
				case game.StatCurrentHP:
					f.HP = min(v, f.MaxHP)
				case game.StatCurrentSP:
					f.SP = uint16(min(v, uint32(f.MaxSP)))
				}
			}
		}
		return true, true
	case protocol.CommandBattleAction:
		if !s.Active {
			return true, false
		}
		switch p[1] {
		case protocol.BattleActionAnimation:
			actions, err := ParseActions(p[2:])
			if err != nil {
				return true, false
			}
			for _, a := range actions {
				if s.At(a.Actor) == nil {
					return true, false
				}
				for _, t := range a.Targets {
					if s.At(t.Cell) == nil {
						return true, false
					}
				}
			}
			for _, a := range actions {
				for _, t := range a.Targets {
					f := s.At(t.Cell)
					if a.Skill == nativeCaptureSuccess && t.Result == protocol.BattleHitLanded {
						f.Removed = true
						f.HP = 0
					}
					if t.Result != protocol.BattleHitLanded {
						continue
					}
					for _, result := range t.Stats {
						switch result.Stat {
						case game.StatCurrentHP:
							f.HP = applyResult(f.HP, f.MaxHP, result)
						case game.StatCurrentSP:
							f.SP = uint16(applyResult(uint32(f.SP), uint32(f.MaxSP), result))
						}
					}
				}
			}
			s.Actions = append(s.Actions, actions...)
			s.Ready = false
			return true, true
		case protocol.BattleActionTurn:
			if len(p) != 5 || !(Cell{p[2], p[3]}).Valid() {
				return true, false
			}
			if f := s.At(Cell{p[2], p[3]}); s.Owned(f) && f.Kind == Player {
				s.Prompt = true
			}
			return true, true
		}
	case protocol.CommandBattleReady:
		if p[1] != protocol.BattleReadyReady {
			return false, false
		}
		if len(p) != 2 || !s.Active {
			return true, false
		}
		if s.Prompt && !s.Finished {
			s.Ready = true
			s.Prompt = false
			s.Turn++
			for i := range s.Fighters {
				s.Fighters[i].Submitted = false
			}
		}
		return true, true
	case protocol.CommandBattleEffect:
		if !s.Active {
			return true, false
		}
		switch p[1] {
		case protocol.BattleEffectDefeated, protocol.BattleEffectSubmitted:
			if len(p) != 4 || !(Cell{p[2], p[3]}).Valid() {
				return true, false
			}
			if f := s.At(Cell{p[2], p[3]}); f != nil {
				if p[1] == protocol.BattleEffectDefeated {
					f.HP = 0
				} else {
					f.Submitted = true
				}
			}
			return true, true
		}
	}
	return false, false
}

// Native digit mode carries the direction independently of skill metadata.
func applyResult(current, maximum uint32, r StatResult) uint32 {
	switch r.Mode {
	case protocol.BattleStatDamage, protocol.BattleStatCriticalDamage:
		return current - min(current, r.Amount)
	case protocol.BattleStatRecovery, protocol.BattleStatCriticalRecovery:
		return uint32(min(uint64(maximum), uint64(current)+uint64(r.Amount)))
	}
	return current
}

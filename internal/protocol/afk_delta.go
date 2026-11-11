package protocol

import "bytes"

const (
	AFKDelta            = 8
	AFKDeltaLocation    = 1
	AFKDeltaMaxChanges  = AFKMaxUnits * 2
	AFKDeltaHeaderBytes = 4
)
const (
	AFKDeltaName uint16 = 1 << iota
	AFKDeltaLevel
	AFKDeltaHP
	AFKDeltaMaxHP
	AFKDeltaSP
	AFKDeltaMaxSP
	AFKDeltaEXP
	AFKDeltaEXPNext
	AFKDeltaAmity
	AFKDeltaRemove
	AFKDeltaAll = AFKDeltaName | AFKDeltaLevel | AFKDeltaHP | AFKDeltaMaxHP | AFKDeltaSP | AFKDeltaMaxSP | AFKDeltaEXP | AFKDeltaEXPNext | AFKDeltaAmity
)

type afkUnitKey struct {
	owner, id uint32
	kind      byte
}

func afkKey(u AFKUnit) afkUnitKey { return afkUnitKey{u.Owner, u.ID, u.Kind} }

// CompactAFKUpdate chooses a delta only if it is smaller and reconstructs the
// exact canonical next state, including roster order. The reliable TCP stream
// owns the base; callers reset their base on AFK entry and binding changes.
func CompactAFKUpdate(previous, next []byte) []byte {
	if len(previous) == 0 {
		return next
	}
	old, e := DecodeAFKState(previous)
	if e != nil {
		return next
	}
	state, e := DecodeAFKState(next)
	if e != nil {
		return next
	}
	p, e := EncodeAFKDelta(old, state)
	if e != nil || len(p) >= len(next) {
		return next
	}
	applied, e := ApplyAFKDelta(old, p)
	if e != nil {
		return next
	}
	full, e := EncodeAFKState(applied)
	if e != nil || !bytes.Equal(full, next) {
		return next
	}
	return p
}

func EncodeAFKDelta(old, next AFKState) ([]byte, error) {
	if len(old.Units) > AFKMaxUnits || len(next.Units) > AFKMaxUnits {
		return nil, ErrMalformed
	}
	before := map[afkUnitKey]AFKUnit{}
	after := map[afkUnitKey]bool{}
	for _, u := range old.Units {
		key := afkKey(u)
		if _, ok := before[key]; ok {
			return nil, ErrMalformed
		}
		before[key] = u
	}
	flags := byte(0)
	if old.Map != next.Map || old.X != next.X || old.Y != next.Y {
		flags = AFKDeltaLocation
	}
	p := Builder{CommandAFK, AFKDelta, flags}
	if flags != 0 {
		p = p.U16(next.Map).U16(next.X).U16(next.Y)
	}
	countAt := len(p)
	p = p.U8(0)
	count := byte(0)
	for _, u := range next.Units {
		key := afkKey(u)
		if after[key] || u.Kind > AFKPet || len(u.Name) > AFKMaxNameBytes {
			return nil, ErrMalformed
		}
		after[key] = true
		prev, exists := before[key]
		mask := uint16(0)
		if !exists {
			mask = AFKDeltaAll
		} else {
			if prev.Name != u.Name {
				mask |= AFKDeltaName
			}
			if prev.Level != u.Level {
				mask |= AFKDeltaLevel
			}
			if prev.HP != u.HP {
				mask |= AFKDeltaHP
			}
			if prev.MaxHP != u.MaxHP {
				mask |= AFKDeltaMaxHP
			}
			if prev.SP != u.SP {
				mask |= AFKDeltaSP
			}
			if prev.MaxSP != u.MaxSP {
				mask |= AFKDeltaMaxSP
			}
			if prev.EXP != u.EXP {
				mask |= AFKDeltaEXP
			}
			if prev.EXPNext != u.EXPNext {
				mask |= AFKDeltaEXPNext
			}
			if prev.Amity != u.Amity {
				mask |= AFKDeltaAmity
			}
		}
		if mask == 0 {
			continue
		}
		count++
		p = p.U32(u.Owner).U32(u.ID).U8(u.Kind).U16(mask)
		if mask&AFKDeltaName != 0 {
			p, _ = p.String(u.Name)
		}
		if mask&AFKDeltaLevel != 0 {
			p = p.U8(u.Level)
		}
		if mask&AFKDeltaHP != 0 {
			p = p.U32(u.HP)
		}
		if mask&AFKDeltaMaxHP != 0 {
			p = p.U32(u.MaxHP)
		}
		if mask&AFKDeltaSP != 0 {
			p = p.U32(u.SP)
		}
		if mask&AFKDeltaMaxSP != 0 {
			p = p.U32(u.MaxSP)
		}
		if mask&AFKDeltaEXP != 0 {
			p = p.U32(u.EXP)
		}
		if mask&AFKDeltaEXPNext != 0 {
			p = p.U64(u.EXPNext)
		}
		if mask&AFKDeltaAmity != 0 {
			p = p.U8(u.Amity)
		}
	}
	for _, u := range old.Units {
		if !after[afkKey(u)] {
			count++
			p = p.U32(u.Owner).U32(u.ID).U8(u.Kind).U16(AFKDeltaRemove)
		}
	}
	p[countAt] = count
	return p, nil
}

// ApplyAFKDelta is atomic: malformed updates never mutate the owned base slice.
func ApplyAFKDelta(base AFKState, p []byte) (AFKState, error) {
	if len(p) < AFKDeltaHeaderBytes || p[0] != CommandAFK || p[1] != AFKDelta || len(base.Units) > AFKMaxUnits {
		return AFKState{}, ErrMalformed
	}
	r := NewReader(p[2:])
	flags := r.U8()
	if flags & ^byte(AFKDeltaLocation) != 0 {
		return AFKState{}, ErrMalformed
	}
	next := base
	next.Units = append([]AFKUnit(nil), base.Units...)
	if flags&AFKDeltaLocation != 0 {
		next.Map, next.X, next.Y = r.U16(), r.U16(), r.U16()
	}
	n := int(r.U8())
	if n > AFKDeltaMaxChanges {
		return AFKState{}, ErrMalformed
	}
	seen := map[afkUnitKey]bool{}
	for range n {
		u := AFKUnit{Owner: r.U32(), ID: r.U32(), Kind: r.U8()}
		key := afkKey(u)
		mask := r.U16()
		if seen[key] || u.Kind > AFKPet || mask == 0 || mask & ^uint16(AFKDeltaAll|AFKDeltaRemove) != 0 {
			return AFKState{}, ErrMalformed
		}
		seen[key] = true
		index := -1
		for i, old := range next.Units {
			if afkKey(old) == key {
				index = i
				u = old
				break
			}
		}
		if mask&AFKDeltaRemove != 0 {
			if mask != AFKDeltaRemove || index < 0 {
				return AFKState{}, ErrMalformed
			}
			next.Units = append(next.Units[:index], next.Units[index+1:]...)
			continue
		}
		if index < 0 && mask != AFKDeltaAll {
			return AFKState{}, ErrMalformed
		}
		if mask&AFKDeltaName != 0 {
			u.Name = r.String()
			if len(u.Name) > AFKMaxNameBytes {
				return AFKState{}, ErrMalformed
			}
		}
		if mask&AFKDeltaLevel != 0 {
			u.Level = r.U8()
		}
		if mask&AFKDeltaHP != 0 {
			u.HP = r.U32()
		}
		if mask&AFKDeltaMaxHP != 0 {
			u.MaxHP = r.U32()
		}
		if mask&AFKDeltaSP != 0 {
			u.SP = r.U32()
		}
		if mask&AFKDeltaMaxSP != 0 {
			u.MaxSP = r.U32()
		}
		if mask&AFKDeltaEXP != 0 {
			u.EXP = r.U32()
		}
		if mask&AFKDeltaEXPNext != 0 {
			u.EXPNext = r.U64()
		}
		if mask&AFKDeltaAmity != 0 {
			u.Amity = r.U8()
		}
		if index < 0 {
			next.Units = append(next.Units, u)
		} else {
			next.Units[index] = u
		}
	}
	if r.Err() != nil || r.Remaining() != 0 || len(next.Units) > AFKMaxUnits {
		return AFKState{}, ErrMalformed
	}
	return next, nil
}

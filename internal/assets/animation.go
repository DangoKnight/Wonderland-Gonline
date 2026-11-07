package assets

import (
	"fmt"
	"math"
	"wonderland-gonline/internal/protocol"
)

const (
	nativeAnimationDamageModeOffset   = 24
	nativeAnimationContactCountOffset = 25
	nativeAnimationContactHeaderBytes = 26
	nativeAnimationContactBytes       = 5
	nativeAnimationSingleDamage       = 0
	MaxSkillContactEvents             = 50
)

// AnimationContactCount matches the client's Motion.HitCount. Damage mode zero
// produces one contact; other modes use the authored event count.
func AnimationContactCount(record []byte) (byte, error) {
	if len(record) <= nativeAnimationContactCountOffset {
		return 0, fmt.Errorf("MBTM: truncated contact header")
	}
	count := record[nativeAnimationContactCountOffset]
	if count > MaxSkillContactEvents || len(record) < nativeAnimationContactHeaderBytes+int(count)*nativeAnimationContactBytes+1 {
		return 0, fmt.Errorf("MBTM: invalid contact events")
	}
	if record[nativeAnimationDamageModeOffset] == nativeAnimationSingleDamage || count == 0 {
		return 1, nil
	}
	return count, nil
}

func ParseAnimationContacts(data []byte) (map[uint16]byte, error) {
	result := map[uint16]byte{}
	err := walkAnimations(data, func(id uint16, record []byte) error {
		count, err := AnimationContactCount(record)
		if err == nil {
			result[id] = count
		}
		return err
	})
	return result, err
}

func ApplySkillContacts(skills map[uint16]Skill, contacts map[uint16]byte) {
	for id, count := range contacts {
		if s, ok := skills[id]; ok && s.ContactEvents == 0 {
			s.ContactEvents = count
			skills[id] = s
		}
	}
}

// ParseAnimationTiming ports SkillAnimationTiming.Read. Unknown movement modes
// are excluded, so callers keep their configured fallback duration.
func ParseAnimationTiming(data []byte) (map[uint16]int, error) {
	out := map[uint16]int{}
	err := walkAnimations(data, func(id uint16, record []byte) error {
		if estimate, ok := estimateAnimation(record); ok {
			out[id] = estimate
		}
		return nil
	})
	return out, err
}

func walkAnimations(data []byte, visit func(uint16, []byte) error) error {
	if len(data) < 2 || len(data) > 64<<20 {
		return fmt.Errorf("MBTM: invalid length")
	}
	count := int(le.Uint16(data[len(data)-2:]))
	start := len(data) - 2 - count*10
	if count == 0 || start < 0 {
		return fmt.Errorf("MBTM: invalid index")
	}
	ids := map[uint16]bool{}
	previous := 0
	for i := 0; i < count; i++ {
		h := data[start+i*10 : start+(i+1)*10]
		id := le.Uint16(h)
		offset, size := int(le.Uint32(h[2:])), int(le.Uint32(h[6:]))
		if ids[id] || offset != previous || size < 27 || offset > start-size {
			return fmt.Errorf("MBTM: invalid record %d", id)
		}
		ids[id] = true
		previous = offset + size
		if err := visit(id, data[offset:offset+size]); err != nil {
			return fmt.Errorf("MBTM %d: %w", id, err)
		}
	}
	if previous != start {
		return fmt.Errorf("MBTM: incomplete index")
	}
	return nil
}
func estimateAnimation(b []byte) (int, bool) {
	r := protocol.NewReader(b)
	r.Bytes(3)
	approach, ret := r.U8(), r.U8()
	if approach > 1 || ret > 1 {
		return 0, false
	}
	r.Bytes(20)
	events := int(r.U8())
	r.Bytes(events * 5)
	count := int(r.U8())
	if count == 0 || count > 50 {
		return 0, false
	}
	speeds := make([]float64, count)
	duration := 0.0
	for i := range speeds {
		r.Bytes(2)
		speeds[i] = r.F64()
		hold := r.U32()
		if math.IsNaN(speeds[i]) || math.IsInf(speeds[i], 0) || speeds[i] < 0 || hold > 30000 {
			return 0, false
		}
		duration += float64(hold)
	}
	if int(r.U8()) != count {
		return 0, false
	}
	x, y := 0.0, 0.0
	for i := range speeds {
		nx, ny := float64(int32(r.U32())), float64(int32(r.U32()))
		distance := math.Max(math.Abs(nx-x), math.Abs(ny-y))
		speed := speeds[i]
		if i == 0 {
			speed = .6
			if approach == 1 {
				speed = .8
			}
		}
		if distance > 0 {
			if speed <= 0 {
				return 0, false
			}
			duration += distance / speed
		}
		x, y = nx, ny
	}
	r.U8()
	speed := .6
	if ret == 1 {
		speed = .8
	}
	duration += math.Max(math.Abs(x), math.Abs(y))/speed + float64((count+2)*35)
	if r.Err() != nil || duration <= 0 || duration > 30000 {
		return 0, false
	}
	return int(math.Ceil(duration)), true
}
func AnimationDelay(durations map[uint16]int, id uint16, fallback, margin int) int {
	delay := fallback
	if estimate, ok := durations[id]; ok {
		delay = max(delay, estimate+margin)
	}
	if id == 30074 {
		delay = max(delay, 6200)
	}
	return delay
}

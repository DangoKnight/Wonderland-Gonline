package game

import (
	"math"
	"wonderland-go/internal/protocol"
)

// Pets and companions. Reference: Player.PlayerPetData and the companion helpers of
// QuestManager and Player.

// MaxPets is the party size limit.
const MaxPets = 4

// MaxHotelPets is the native pet hotel capacity (AC31).
const MaxHotelPets = 10

// Allocate follows AC08 pet allocation, including UInt16 attribute wrapping.
func (p *Pet) Allocate(requests []StatAllocation) bool {
	c := Character{Base: p.Base, StatPoints: p.StatPoints}
	if !c.Allocate(requests) {
		return false
	}
	p.Base, p.StatPoints = c.Base, c.StatPoints
	return true
}

// PetSkill is a pet's learned skill progress.
type PetSkill struct {
	ID    uint16 `json:"id"`
	Grade byte   `json:"grade"`
	Exp   uint32 `json:"exp"`
}

// Pet is one owned companion. Slot is its party (or reserve) slot; the client's own
// slot numbering is assigned per login and is not stored.
type Pet struct {
	Slot       byte       `json:"slot"`
	ID         uint32     `json:"id"`
	Name       string     `json:"name"`
	Level      byte       `json:"level"`
	Exp        uint32     `json:"exp"` // EXP within the current level.
	HP         int32      `json:"hp"`
	MaxHP      int32      `json:"max_hp"`
	SP         int32      `json:"sp"`
	MaxSP      int32      `json:"max_sp"`
	Base       Attributes `json:"base"`
	StatPoints uint16     `json:"stat_points,omitempty"`
	Amity      byte       `json:"amity"`
	Battle     bool       `json:"battle,omitempty"`
	Reborn     bool       `json:"reborn,omitempty"`
	Job        byte       `json:"job,omitempty"`
	Equipment  Equipment  `json:"equipment"`
	Skills     []PetSkill `json:"skills,omitempty"`
}

// PetTemplate is the Npc.dat data a pet derives from.
type PetTemplate struct {
	Type   byte
	Stats  Attributes
	Skills [3]uint16
}

// BroadcastID is GetCompanionBroadcastId: Robinson is 12032 as an NPC and 12178 as a pet.
func BroadcastID(id uint32) uint32 {
	if id == 12032 {
		return 12178
	}
	return id
}

// SamePet is IsSamePetOrCompanion: Robinson's and Roca's two IDs are one companion.
func SamePet(a, b uint32) bool {
	switch {
	case a == b:
		return true
	case a == 0 || b == 0:
		return false
	case (a == 12032 || a == 12178) && (b == 12032 || b == 12178):
		return true
	case (a == 14161 || a == 14162) && (b == 14161 || b == 14162):
		return true
	}
	return false
}

// StoryCompanion is IsStoryCompanion: Npc.dat type 4, plus listed type 2/7 companions.
func StoryCompanion(id uint32, t PetTemplate, known bool) bool {
	if !known {
		return false
	}
	if t.Type == 4 {
		return true
	}
	switch id {
	case 12068, 12081, 12095, 12132, 12147, 12148, 12149, 17914, 31036, 25020, 17162, 17454:
		return true
	}
	return false
}

// Element is the pet's element: only Robinson is earth.
func (p Pet) Element() byte {
	if p.ID == 12032 || p.ID == 12178 {
		return Earth
	}
	return 0
}

func roundStat(v float64) int32 { return int32(math.RoundToEven(v)) }

// Calculated stats (PlayerPetData.Calculated*).
func (p Pet) CalculatedMaxHP() int32 {
	l, con := float64(p.Level), float64(p.Base.Constitution)
	return max(1, roundStat(math.Pow(l, .35)*con*2+l+con*2+180))
}
func (p Pet) CalculatedMaxSP() int32 {
	l, wis := float64(p.Level), float64(p.Base.Wisdom)
	return max(0, roundStat(math.Pow(l, .3)*wis*3.2+l+wis*2+94))
}

// Combat is the pet's derived battle stats with its equipment.
func (p Pet) Combat(items map[uint16]ItemDefinition) Combat {
	l := float64(p.Level)
	e := p.Element()
	pick := func(water, other float64) float64 {
		if e == 2 {
			return water
		}
		return other
	}
	defLevel := 2.0
	if e == 0 {
		defLevel = 8
	}
	spdLevel := 1.6
	if e == 3 {
		spdLevel = 2.1
	}
	b := p.Equipment.Bonuses(items)
	return Combat{
		MaxHP: max(1, p.CalculatedMaxHP()+b.HP), MaxSP: max(0, p.CalculatedMaxSP()+b.SP),
		ATK: max(0, roundStat(l*pick(2, 1.4)+float64(p.Base.Strength)*2)+int32(int16(b.ATK))),
		DEF: max(0, roundStat(l*defLevel+float64(p.Base.Constitution)*1.75)+int32(int16(b.DEF))),
		MAT: max(0, roundStat(l*pick(1.6, 1.4)+float64(p.Base.Intelligence)*2)+int32(int16(b.MAT))),
		MDF: max(0, roundStat(l*pick(2.2, 2)+float64(p.Base.Wisdom)*2.2)+int32(int16(b.MDF))),
		SPD: max(0, roundStat(l*spdLevel+float64(p.Base.Agility)*2.2)+int32(int16(b.SPD))),
	}
}

// Normalize is NormalizeClientStats: maxima follow stats and equipment; current
// vitals are clamped or refilled.
func (p *Pet) Normalize(items map[uint16]ItemDefinition, refill bool) {
	c := p.Combat(items)
	p.MaxHP, p.MaxSP = c.MaxHP, c.MaxSP
	if refill {
		p.HP, p.SP = p.MaxHP, p.MaxSP
		return
	}
	p.HP, p.SP = min(max(p.HP, 0), p.MaxHP), min(max(p.SP, 0), p.MaxSP)
}

// EnsureSkills adds the template's default skills (known to the skill catalog).
func (p *Pet) EnsureSkills(t PetTemplate, known func(uint16) bool) {
	for _, id := range t.Skills {
		if id == 0 || !known(id) {
			continue
		}
		found := false
		for _, s := range p.Skills {
			if s.ID == id {
				found = true
			}
		}
		if !found {
			p.Skills = append(p.Skills, PetSkill{ID: id, Grade: 1})
		}
	}
}

// ClientTotalExp is GetClientTotalExp: six base EXP, every completed level, then progress.
func (p Pet) ClientTotalExp() uint32 {
	total := uint64(6) + uint64(p.Exp)
	for l := 1; l < max(1, min(int(p.Level), MaxLevel)); l++ {
		total += LevelExp(l)
	}
	return uint32(min(total, math.MaxUint32))
}

// grow adds one point per level using the selected five-attribute weight formula.
// Zero weights retain a minimum chance of one; capped attributes are excluded.
func (p *Pet) grow(t PetTemplate, known bool, roll func(int) int, options ...PetGrowthOptions) {
	weights := p.growthWeights(t, known, options)
	values := []*uint16{&p.Base.Strength, &p.Base.Constitution, &p.Base.Intelligence, &p.Base.Wisdom, &p.Base.Agility}
	var candidates []int
	for i := range values {
		if *values[i] < math.MaxUint16 {
			candidates = append(candidates, i)
		}
	}
	if len(candidates) == 0 {
		return
	}
	total := 0
	for _, i := range candidates {
		total += max(1, weights[i])
	}
	r := roll(total)
	for _, i := range candidates {
		if r -= max(1, weights[i]); r < 0 {
			*values[i]++
			return
		}
	}
}

// GainExp adds EXP and levels the pet up to MaxLevel, growing one stat per level.
func (p *Pet) GainExp(amount uint32, t PetTemplate, known bool, roll func(int) int, options ...PetGrowthOptions) int {
	p.Exp = uint32(min(uint64(p.Exp)+uint64(amount), math.MaxUint32))
	before := p.Level
	p.Level = max(p.Level, 1)
	for p.Level < MaxLevel {
		need := LevelExp(int(p.Level))
		if uint64(p.Exp) < need {
			break
		}
		p.Exp -= uint32(need)
		p.Level++
		p.grow(t, known, roll, options...)
	}
	return int(p.Level) - int(before)
}

// NewPet is a freshly recruited companion at level 1 with full vitals.
func NewPet(id uint32, name string, slot byte, t PetTemplate, items map[uint16]ItemDefinition) Pet {
	p := Pet{Slot: slot, ID: id, Name: name, Level: 1, Amity: 60, Base: t.Stats}
	p.Normalize(items, true)
	return p
}

// Pet finds an owned party pet by ID, treating companion aliases as one.
func (c *Character) Pet(id uint32) (int, bool) {
	for i := range c.Pets {
		if SamePet(c.Pets[i].ID, id) {
			return i, true
		}
	}
	return -1, false
}

// FreePetSlot is the lowest unused party slot, or 0 when the party is full.
func (c Character) FreePetSlot() byte {
	if len(c.Pets) >= MaxPets {
		return 0
	}
	for slot := byte(1); ; slot++ {
		used := false
		for _, p := range c.Pets {
			if p.Slot == slot {
				used = true
			}
		}
		if !used {
			return slot
		}
	}
}

// BattlePet is the pet marked for battle, if any.
func (c *Character) BattlePet() *Pet {
	for i := range c.Pets {
		if c.Pets[i].Battle || (c.ActivePet != 0 && SamePet(c.Pets[i].ID, c.ActivePet)) {
			return &c.Pets[i]
		}
	}
	return nil
}

// packSkills writes three (grade, EXP) records in the template's native skill order.
func (p Pet) packSkills(b protocol.Builder, t PetTemplate) protocol.Builder {
	for _, id := range t.Skills {
		var s PetSkill
		for _, have := range p.Skills {
			if id != 0 && have.ID == id {
				s = have
			}
		}
		b = b.U8(s.Grade).U32(s.Exp)
	}
	return b
}

// RecruitPacket is CreatePetPacket (AC15:1), the one-time "joined" notification.
func (p Pet) RecruitPacket(owner uint32, t PetTemplate) []byte {
	b := protocol.Builder{protocol.CommandPetControl, protocol.PetControlWireCode1}.U32(owner).U32(BroadcastID(p.ID)).U8(1).
		U16(p.Base.Strength).U16(p.Base.Constitution).U16(p.Base.Intelligence).U16(p.Base.Wisdom).U16(p.Base.Agility).
		U8(max(p.Level, 1)).U32(p.ClientTotalExp())
	b = p.packSkills(b, t)
	reborn := byte(0)
	if p.Reborn {
		reborn = 1
	}
	return b.U8(p.Amity).U16(0).U8(0).U8(reborn).U8(p.Job).U8(0).U8(0).U8(0).U16(0).U16(0)
}

// ListRecord is one CreatePetListPacket (AC15:8) record.
func (p Pet) ListRecord(b protocol.Builder, clientSlot byte, name string, t PetTemplate) protocol.Builder {
	b = b.U8(clientSlot).U16(uint16(BroadcastID(p.ID))).U32(p.ClientTotalExp()).U8(p.Level).U32(uint32(max(0, p.HP))).
		U16(uint16(min(max(0, p.SP), math.MaxUint16))).
		U16(p.Base.Intelligence).U16(p.Base.Strength).U16(p.Base.Constitution).U16(p.Base.Agility).U16(p.Base.Wisdom).
		U8(0).U8(p.Amity).U8(1).U16(p.StatPoints)
	n := []byte(name)[:min(16, len(name))]
	b = b.U8(byte(len(n))).Bytes(n)
	b = p.packSkills(b, t)
	for _, item := range p.Equipment {
		var meta [19]byte
		meta[0], meta[13] = item.Damage, item.Forge()
		b = b.U16(item.ID).Bytes(meta[:])
	}
	reborn := byte(0)
	if p.Reborn {
		reborn = 1
	}
	return b.U8(0).U8(0).U8(reborn).U8(p.Job).U8(0).U8(0).U8(0).U16(0).U16(0)
}

// PetStat is AC8:2 for a pet: the pet collection (4), client slot, stat, sign, value.
// HotelRecord is Player.SendPetHotelList's 64-byte record plus its name bytes.
func (p Pet) HotelRecord(b protocol.Builder, t PetTemplate) protocol.Builder {
	reborn := byte(0)
	if p.Reborn {
		reborn = 1
	}
	b = b.U8(p.Slot).U16(uint16(BroadcastID(p.ID))).U32(p.ClientTotalExp()).U8(p.Level).
		U32(uint32(max(0, p.HP))).U16(uint16(min(max(0, p.SP), math.MaxUint16))).
		U16(p.Base.Intelligence).U16(p.Base.Strength).U16(p.Base.Constitution).U16(p.Base.Agility).U16(p.Base.Wisdom).
		U8(reborn).U8(p.Job).U8(0).U16(p.StatPoints)
	var name []byte
	for _, v := range p.Name {
		if v > 127 {
			name = append(name, '?')
		} else {
			name = append(name, byte(v))
		}
	}
	name = name[:min(10, len(name))]
	b = b.U8(byte(len(name))).Bytes(name)
	b = p.packSkills(b, t)
	for _, item := range p.Equipment {
		b = b.U16(item.ID)
	}
	return b.U8(0).U8(0).U8(0).U16(0).U16(0)
}

func PetStat(clientSlot, stat byte, value int64) []byte {
	sign := byte(1)
	if value < 0 {
		sign, value = 2, -value
	}
	return protocol.Builder{protocol.CommandStats, protocol.StatsWireCode2, 4, clientSlot, 0, stat, sign}.U32(uint32(value)).U32(0)
}

// ProgressionPackets are SendPetProgression plus SendPetEquipmentStats.
func (p Pet) ProgressionPackets(clientSlot byte, items map[uint16]ItemDefinition) [][]byte {
	b := p.Equipment.Bonuses(items)
	c := p.Combat(items)
	out := [][]byte{
		PetStat(clientSlot, 35, int64(p.Level)), PetStat(clientSlot, 37, int64(max(0, int(p.Level)-1))),
		PetStat(clientSlot, 38, int64(p.StatPoints)), PetStat(clientSlot, 36, int64(p.ClientTotalExp())),
		PetStat(clientSlot, 28, int64(p.Base.Strength)), PetStat(clientSlot, 29, int64(p.Base.Constitution)),
		PetStat(clientSlot, 30, int64(p.Base.Agility)), PetStat(clientSlot, 27, int64(p.Base.Intelligence)),
		PetStat(clientSlot, 33, int64(p.Base.Wisdom)),
		PetStat(clientSlot, 210, int64(int16(b.ATK))), PetStat(clientSlot, 211, int64(int16(b.DEF))),
		PetStat(clientSlot, 215, int64(int16(b.MAT))), PetStat(clientSlot, 216, int64(int16(b.MDF))),
		PetStat(clientSlot, 214, int64(int16(b.SPD))),
	}
	for i, total := range []int32{c.ATK, c.DEF, c.MAT, c.MDF, c.SPD} {
		out = append(out, PetStat(clientSlot, byte(41+i), int64(min(total, math.MaxUint16))))
	}
	return append(out, PetStat(clientSlot, 207, int64(b.HP)), PetStat(clientSlot, 208, int64(b.SP)), PetStat(clientSlot, 25, int64(p.HP)), PetStat(clientSlot, 26, int64(p.SP)))
}

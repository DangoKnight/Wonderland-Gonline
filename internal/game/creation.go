package game

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"wonderland-go/internal/protocol"
)

type Attributes struct {
	Strength     uint16 `json:"strength"`
	Constitution uint16 `json:"constitution"`
	Intelligence uint16 `json:"intelligence"`
	Wisdom       uint16 `json:"wisdom"`
	Agility      uint16 `json:"agility"`
}

type Appearance struct {
	Body, Head                 uint16
	Hair, Skin, Clothing, Eyes uint16
	Element                    byte
	Base                       Attributes
}

type ItemDefinition struct {
	ID        uint16    `json:"id"`
	Name      string    `json:"name"`
	Type      byte      `json:"type"`
	EquipSlot uint16    `json:"equip_slot"`
	Level     uint16    `json:"level"`
	Status    [2]uint16 `json:"status"`
	Values    [2]int32  `json:"values"`
}

func (d ItemDefinition) StackLimit() byte {
	switch d.Type {
	case 10, 17, 20, 21, 23, 24, 25, 26, 28, 30, 31, 32, 33, 34, 35, 36, 37, 38, 40, 41, 51, 52, 54:
		return MaxItemStack
	default:
		return 1
	}
}

// Droppable follows Item.Dropable; other types must be destroyed instead.
func (d ItemDefinition) Droppable() bool {
	switch d.Type {
	case 1, 2, 3, 4, 5, 6, 8, 9, 10, 12, 13, 14, 15, 23, 31, 32, 33, 34, 35, 36, 37:
		return true
	}
	return false
}

type StarterGrant struct {
	ID    uint16
	Count int
}
type LearnedSkill struct {
	ID    uint16 `json:"id"`
	Grade byte   `json:"grade"`
	EXP   uint32 `json:"exp"`
}

func ValidateCharacterName(name string) error {
	if len(name) < 4 || len(name) > 14 || strings.TrimSpace(name) != name {
		return errors.New("character name must contain 4–14 printable ASCII bytes")
	}
	for _, b := range []byte(name) {
		if b < 32 || b > 126 {
			return errors.New("character name must be printable ASCII")
		}
	}
	return nil
}

func (a Appearance) Validate() error {
	heads := map[uint16]uint16{1: 1, 2: 2, 3: 4, 4: 8}
	if a.Body < 1 || a.Body > 4 || a.Head >= heads[a.Body] || a.Element < 1 || a.Element > 4 {
		return errors.New("invalid character appearance or element")
	}
	return nil
}

// CreationStatPoints matches native TRE_CreateCharacter.reset/statArrow:
// five distributable points, with model bonuses calculated separately.
const CreationStatPoints = 5

func (a Appearance) ValidateCreationAllocation() error {
	total := uint32(a.Base.Strength) + uint32(a.Base.Constitution) + uint32(a.Base.Intelligence) + uint32(a.Base.Wisdom) + uint32(a.Base.Agility)
	if total != CreationStatPoints {
		return fmt.Errorf("creation attributes must total %d points", CreationStatPoints)
	}
	return nil
}

const (
	creationDeletionCodeMinBytes = 6
	creationDeletionCodeMaxBytes = 14
)

// CharacterCreationRequest preserves the two credentials sent by aLogin.
// Credentials are transient and must never appear in logs or character state.
type CharacterCreationRequest struct {
	Appearance              Appearance
	ConfirmationPassword    string
	DeletionCode            string
	HasConfirmationPassword bool
}

// DecodeCharacterCreation reads AC9:1 without its action/subaction. The native
// sender at 0x2c396c emits appearance and stats, then (on first creation) two
// length-prefixed strings: account password, deletion password (0x2c3bd0).
// Legacy clients may omit both fields or send only a deletion password.
func DecodeCharacterCreation(data []byte) (CharacterCreationRequest, error) {
	r := protocol.NewReader(data)
	a := Appearance{Body: r.U16(), Head: r.U16(), Hair: r.U16(), Skin: r.U16(), Clothing: r.U16(), Eyes: r.U16(), Element: r.U8()}
	a.Base.Strength = uint16(r.U8())
	a.Base.Agility = uint16(r.U8())
	a.Base.Wisdom = uint16(r.U8())
	a.Base.Intelligence = uint16(r.U8())
	a.Base.Constitution = uint16(r.U8())
	request := CharacterCreationRequest{Appearance: a}
	if r.Remaining() > 0 {
		first := r.String()
		if r.Remaining() > 0 {
			request.HasConfirmationPassword = true
			request.ConfirmationPassword = first
			request.DeletionCode = r.String()
		} else {
			request.DeletionCode = first
		}
	}
	if r.Err() != nil {
		return request, r.Err()
	}
	if r.Remaining() != 0 {
		return request, protocol.ErrMalformed
	}
	if request.DeletionCode != "" && (len(request.DeletionCode) < creationDeletionCodeMinBytes || len(request.DeletionCode) > creationDeletionCodeMaxBytes) {
		return request, errors.New("invalid character deletion code")
	}
	return request, a.Validate()
}

// DecodeAppearance retains the legacy API while recognizing native requests.
func DecodeAppearance(data []byte) (Appearance, string, error) {
	request, err := DecodeCharacterCreation(data)
	return request.Appearance, request.DeletionCode, err
}

// CharacterBonuses uses Equip.BonusStr/Con/Int/Wis/Agi in the C# source.
func CharacterBonuses(body, head uint16) Attributes {
	tables := map[uint16][]Attributes{
		1: {{Intelligence: 2, Agility: 1}},
		2: {{Intelligence: 1, Wisdom: 1, Agility: 1}, {Intelligence: 1, Agility: 2}},
		3: {{Strength: 2, Constitution: 1}, {Strength: 2, Constitution: 1}, {Wisdom: 2, Agility: 1}, {Strength: 1, Constitution: 1, Intelligence: 1}},
		4: {{Strength: 1, Constitution: 2}, {Strength: 2, Agility: 1}, {Intelligence: 3}, {Strength: 1, Constitution: 1, Intelligence: 1}, {Intelligence: 1, Wisdom: 2}, {Intelligence: 2, Wisdom: 1}, {Intelligence: 2, Agility: 1}, {Intelligence: 1, Wisdom: 1, Agility: 1}},
	}
	if head >= uint16(len(tables[body])) {
		return Attributes{}
	}
	return tables[body][head]
}
func (c Character) Attributes() Attributes {
	b := CharacterBonuses(c.Body, c.Head)
	b.Strength += c.Base.Strength
	b.Constitution += c.Base.Constitution
	b.Intelligence += c.Base.Intelligence
	b.Wisdom += c.Base.Wisdom
	b.Agility += c.Base.Agility
	return b
}

func StarterOutfit(body, head uint16) []uint16 {
	sets := map[uint16][][]uint16{
		1: {{21001, 24001}}, 2: {{22003, 21002, 24002}, {22001, 21003, 24003}},
		3: {{21004, 24004}, {21005, 24005}, {21012, 24012}, {22009, 21014, 18002, 24014}},
		4: {{22005, 21006, 23001, 24006}, {21007, 23002, 24007}, {21008, 24008}, {22007, 21009, 10002, 24009}, {22002, 21010, 10003, 24010}, {24013, 21013}, {22006, 21011, 10004, 24011}, {21015, 22008, 24015}},
	}
	if head >= uint16(len(sets[body])) {
		return nil
	}
	return append([]uint16(nil), sets[body][head]...)
}
func StarterStunt(body, head uint16) uint16 {
	skills := map[uint16][]uint16{1: {11075}, 2: {15041, 12036}, 3: {15038, 11076, 15039, 11182}, 4: {11078, 12053, 15040, 15060, 12051, 12049, 11077, 11183}}
	if head >= uint16(len(skills[body])) {
		return 15003
	}
	return skills[body][head]
}
func StarterSkills(body, head uint16, element byte) []LearnedSkill {
	elements := map[byte][]uint16{1: {15085, 11017, 11057}, 2: {15091, 11001, 15100}, 3: {11016, 11166, 11056}, 4: {11007, 15079, 11052}}
	ids := append([]uint16{StarterStunt(body, head)}, elements[element]...)
	out := make([]LearnedSkill, 0, len(ids))
	for _, id := range ids {
		out = append(out, LearnedSkill{ID: id, Grade: 1})
	}
	return out
}

func NewCharacter(id uint32, slot byte, name string, a Appearance, grants []StarterGrant, items map[uint16]ItemDefinition, now time.Time, growth ...ElementalGrowth) (Character, error) {
	if err := ValidateCharacterName(name); err != nil {
		return Character{}, err
	}
	if err := a.Validate(); err != nil {
		return Character{}, err
	}
	c := Character{ID: id, Slot: slot, Name: name, Body: a.Body, Head: a.Head, Element: a.Element, Color1: uint32(a.Hair) | uint32(a.Skin)<<16, Color2: uint32(a.Clothing) | uint32(a.Eyes)<<16, Base: a.Base, Level: 1, EXP: 6, Map: 10017, X: 1042, Y: 1075, Quests: map[uint32]Quest{1: {ID: 1, State: InProgress, Step: 1, StartedAt: now.UTC()}}, Skills: StarterSkills(a.Body, a.Head, a.Element)}
	for _, id := range StarterOutfit(a.Body, a.Head) {
		item, ok := items[id]
		if !ok || item.EquipSlot < 1 || item.EquipSlot > 6 {
			return Character{}, fmt.Errorf("starter equipment %d missing or invalid", id)
		}
		c.Equipment[item.EquipSlot-1] = Item{ID: id, Count: 1}
	}
	stats := c.Attributes()
	level := float64(c.Level)
	g := characterGrowth(c.Element, growth)
	hp := math.RoundToEven(g.HP.value(level, stats))
	sp := math.RoundToEven(g.SP.value(level, stats))
	bonus := c.Equipment.Bonuses(items)
	hp += float64(bonus.HP)
	sp += float64(bonus.SP)
	if hp < 1 || sp < 0 || hp > math.MaxUint16 || sp > math.MaxUint16 {
		return Character{}, errors.New("starter vital stats out of range")
	}
	c.MaxHP, c.HP = uint32(hp), uint32(hp)
	c.MaxSP, c.SP = uint32(sp), uint32(sp)
	for _, grant := range grants {
		def, ok := items[grant.ID]
		if !ok {
			return Character{}, fmt.Errorf("starter item %d missing", grant.ID)
		}
		if err := c.Bag.Add(Item{ID: grant.ID}, grant.Count, def.StackLimit()); err != nil {
			return Character{}, err
		}
	}
	return c, c.Validate()
}

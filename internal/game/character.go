package game

import (
	"errors"
	"time"
	"wonderland-go/internal/protocol"
)

type Character struct {
	EventTimers map[uint16]time.Time `json:"event_timers,omitempty"`
	LuckyDraw   LuckyDrawState       `json:"lucky_draw,omitempty"`
	// Reborn supplies combo effective-level metadata. Rebirth progression and
	// character roster/job presentation remain unported.
	Reborn   bool            `json:"reborn,omitempty"`
	Settings *ClientSettings `json:"settings,omitempty"`
	Base     Attributes      `json:"base"`
	Skills   []LearnedSkill  `json:"skills,omitempty"`
	ID       uint32          `json:"id"`
	Slot     byte            `json:"slot"`
	Name     string          `json:"name"`
	Level    byte            `json:"level"`
	Element  byte            `json:"element"`
	HP       uint32          `json:"hp"`
	MaxHP    uint32          `json:"max_hp"`
	SP       uint32          `json:"sp"`
	MaxSP    uint32          `json:"max_sp"`
	EXP      uint32          `json:"exp"` // Total EXP; the level is derived from it.
	// StatPoints are unallocated attribute points (stat 38), three per level gained.
	StatPoints uint16           `json:"stat_points,omitempty"`
	Gold       uint32           `json:"gold"`
	BankGold   uint32           `json:"bank_gold,omitempty"`
	Body       uint16           `json:"body"`
	Head       uint16           `json:"head"`
	Color1     uint32           `json:"color1"`
	Color2     uint32           `json:"color2"`
	Equipment  Equipment        `json:"equipment"`
	Map        uint16           `json:"map"`
	X          uint16           `json:"x"`
	Y          uint16           `json:"y"`
	Bag        Inventory        `json:"bag"`
	Storage    Inventory        `json:"storage"`
	Quests     map[uint32]Quest `json:"quests"`
	// RecordPoint is the saved return location (AC5:21), absent until a Record site saves one.
	RecordPoint *Location `json:"record_point,omitempty"`
	// Pets is the party (at most four). ReservePets keeps story companions that
	// temporarily left (C# QuestPets). ActivePet is the battle pet's broadcast ID.
	Pets          []Pet  `json:"pets,omitempty"`
	ReservePets   []Pet  `json:"reserve_pets,omitempty"`
	HotelPets     []Pet  `json:"hotel_pets,omitempty"`
	ActivePet     uint32 `json:"active_pet,omitempty"`
	ActiveVehicle uint16 `json:"active_vehicle,omitempty"` // Exact owned bag item ID.
	VehicleSlot   byte   `json:"vehicle_slot,omitempty"`
	ActiveMount   uint32 `json:"active_mount,omitempty"` // Owned party pet's broadcast ID.
}

type Location struct {
	Map  uint16 `json:"map"`
	X, Y uint16
}

// Clone copies the character so mutations cannot reach the original's maps or slices.
func (c Character) Clone() Character {
	if c.EventTimers != nil {
		timers := make(map[uint16]time.Time, len(c.EventTimers))
		for id, expiry := range c.EventTimers {
			timers[id] = expiry
		}
		c.EventTimers = timers
	}
	if c.Settings != nil {
		settings := *c.Settings
		c.Settings = &settings
	}
	c.Skills = append([]LearnedSkill(nil), c.Skills...)
	quests := make(map[uint32]Quest, len(c.Quests))
	for id, q := range c.Quests {
		quests[id] = q
	}
	c.Quests = quests
	if c.RecordPoint != nil {
		point := *c.RecordPoint
		c.RecordPoint = &point
	}
	c.Pets = clonePets(c.Pets)
	c.ReservePets = clonePets(c.ReservePets)
	c.HotelPets = clonePets(c.HotelPets)
	return c
}

func (c Character) Validate() error {
	for id, expiry := range c.EventTimers {
		if id == 0 || expiry.IsZero() {
			return errors.New("invalid event timer")
		}
	}
	if !c.LuckyDraw.Valid() {
		return errors.New("invalid Lucky Draw state")
	}
	if c.ID == 0 || c.Slot < 1 || c.Slot > 2 || len(c.Name) == 0 || len(c.Name) > 14 || c.Level < 1 || c.Level > 200 || c.HP > c.MaxHP || c.SP > c.MaxSP {
		return errors.New("invalid character")
	}
	for _, b := range []byte(c.Name) {
		if b < 32 || b > 126 {
			return errors.New("character name must be printable ASCII")
		}
	}
	return nil
}

// SelectionRecord follows aLogin's roster decoder (FUN_00402468): maximum
// HP/SP precede current values, and rebirth/job bytes precede six equipment IDs.
// The legacy C# serializer has these fields misordered; retain native alignment.
func (c Character) SelectionRecord() ([]byte, error) {
	if e := c.Validate(); e != nil {
		return nil, e
	}
	p, e := protocol.Builder{c.Slot}.String(c.Name)
	if e != nil {
		return nil, e
	}
	p = p.U8(c.Level).U8(c.Element).U32(c.MaxHP).U32(c.HP).U32(c.MaxSP).U32(c.SP).U32(c.EXP).U32(c.Gold).U16(c.Body).U16(c.Head).U32(c.Color1).U32(c.Color2)
	p = p.U8(RosterNotReborn).U8(RosterNoJob)
	for _, item := range c.Equipment {
		p = p.U16(item.ID)
	}
	return p, nil
}

func clonePets(pets []Pet) []Pet {
	if pets == nil {
		return nil
	}
	out := make([]Pet, len(pets))
	for i, p := range pets {
		p.Skills = append([]PetSkill(nil), p.Skills...)
		out[i] = p
	}
	return out
}

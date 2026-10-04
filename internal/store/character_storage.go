package store

import (
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
	"wonderland-go/internal/game"
)

const characterWriteBatch = 20
const (
	inventoryBag       = "bag"
	inventoryStorage   = "storage"
	inventoryEquipment = "equipment"
	petParty           = "party"
	petReserve         = "reserve"
	petHotel           = "hotel"
	timerEvent         = "event"
	timerChest         = "chest"
)

// Every child is owned by the identity row, including items and inactive pets.
type CharacterOwner struct {
	CharacterID uint32        `gorm:"primaryKey;autoIncrement:false"`
	Character   *characterRow `gorm:"foreignKey:CharacterID;references:ID;constraint:OnDelete:CASCADE"`
}
type characterStateRow struct {
	CharacterOwner
	Base              game.Attributes `gorm:"embedded;embeddedPrefix:base_"`
	Level             byte
	Element           byte
	HP                uint32
	MaxHP             uint32
	SP                uint32
	MaxSP             uint32
	EXP               uint32
	StatPoints        uint16
	Gold              uint32
	BankGold          uint32
	Body              uint16
	Head              uint16
	Color1            uint32
	Color2            uint32
	Map               uint16
	X                 uint16
	Y                 uint16
	ActivePet         uint32
	ActiveVehicle     uint16
	VehicleSlot       byte
	ActiveMount       uint32
	Reborn            bool
	MutedUntil        time.Time
	SettingsPresent   bool
	PKAllowed         bool
	JoinAllowed       bool
	TradeAllowed      bool
	Channels          byte
	TentReturnPresent bool
	TentReturnMap     uint16
	TentReturnX       uint16
	TentReturnY       uint16
	RecordPresent     bool
	RecordMap         uint16
	RecordX           uint16
	RecordY           uint16
	LuckyDay          string
	LuckyUsed         byte
}

func (characterStateRow) TableName() string { return "character_state" }

type characterItemRow struct {
	CharacterOwner
	Container    string `gorm:"primaryKey"`
	OwnerOrdinal int    `gorm:"primaryKey;autoIncrement:false"`
	Slot         int    `gorm:"primaryKey;autoIncrement:false"`
	ItemID       uint16
	Count        byte
	Damage       byte
	Metadata     []byte
}

func (characterItemRow) TableName() string { return "character_items" }

type characterSkillRow struct {
	CharacterOwner
	Ordinal int `gorm:"primaryKey;autoIncrement:false"`
	SkillID uint16
	Grade   byte
	EXP     uint32
}

func (characterSkillRow) TableName() string { return "character_skills" }

type characterQuestRow struct {
	CharacterOwner
	QuestID     uint32 `gorm:"primaryKey;autoIncrement:false"`
	StoredID    uint32
	State       game.QuestState
	Step        int
	Kills       int
	StartedAt   time.Time `gorm:"autoCreateTime:false"`
	CompletedAt *time.Time
}

func (characterQuestRow) TableName() string { return "character_quests" }

type characterTimerRow struct {
	CharacterOwner
	Kind    string `gorm:"primaryKey"`
	TimerID uint32 `gorm:"primaryKey;autoIncrement:false"`
	Expiry  time.Time
}

func (characterTimerRow) TableName() string { return "character_timers" }

type characterDiscoveryRow struct {
	CharacterOwner
	Ordinal  int    `gorm:"primaryKey;autoIncrement:false"`
	Template uint16 `gorm:"index"`
}

func (characterDiscoveryRow) TableName() string { return "character_discoveries" }

type characterPetRow struct {
	CharacterOwner
	Location   string          `gorm:"primaryKey"`
	Ordinal    int             `gorm:"primaryKey;autoIncrement:false"`
	Base       game.Attributes `gorm:"embedded;embeddedPrefix:base_"`
	Slot       byte
	PetID      uint32
	Name       string
	Level      byte
	Exp        uint32
	HP         int32
	MaxHP      int32
	SP         int32
	MaxSP      int32
	StatPoints uint16
	Amity      byte
	Battle     bool
	Reborn     bool
	Job        byte
}

func (characterPetRow) TableName() string { return "character_pets" }

type characterPetSkillRow struct {
	CharacterOwner
	Location   string `gorm:"primaryKey"`
	PetOrdinal int    `gorm:"primaryKey;autoIncrement:false"`
	Ordinal    int    `gorm:"primaryKey;autoIncrement:false"`
	SkillID    uint16
	Grade      byte
	EXP        uint32
}

func (characterPetSkillRow) TableName() string { return "character_pet_skills" }

func characterTables() []any {
	return []any{&characterStateRow{}, &characterItemRow{}, &characterSkillRow{}, &characterQuestRow{}, &characterTimerRow{}, &characterDiscoveryRow{}, &characterPetRow{}, &characterPetSkillRow{}}
}

// The caller owns the transaction: scalar state and all collections commit together.
func writeCharacterState(tx *gorm.DB, c game.Character) error {
	owner := CharacterOwner{CharacterID: c.ID}
	state := characterStateRow{CharacterOwner: owner, Base: c.Base, Level: c.Level, Element: c.Element, HP: c.HP, MaxHP: c.MaxHP, SP: c.SP, MaxSP: c.MaxSP, EXP: c.EXP, StatPoints: c.StatPoints, Gold: c.Gold, BankGold: c.BankGold, Body: c.Body, Head: c.Head, Color1: c.Color1, Color2: c.Color2, Map: c.Map, X: c.X, Y: c.Y, ActivePet: c.ActivePet, ActiveVehicle: c.ActiveVehicle, VehicleSlot: c.VehicleSlot, ActiveMount: c.ActiveMount, Reborn: c.Reborn, MutedUntil: c.MutedUntil, LuckyDay: c.LuckyDraw.Day, LuckyUsed: c.LuckyDraw.Used}
	if c.Settings != nil {
		state.SettingsPresent = true
		state.PKAllowed = c.Settings.PKAllowed
		state.JoinAllowed = c.Settings.JoinAllowed
		state.TradeAllowed = c.Settings.TradeAllowed
		state.Channels = c.Settings.Channels
	}
	if c.TentReturn != nil {
		state.TentReturnPresent = true
		state.TentReturnMap = c.TentReturn.Map
		state.TentReturnX = c.TentReturn.X
		state.TentReturnY = c.TentReturn.Y
	}
	if c.RecordPoint != nil {
		state.RecordPresent = true
		state.RecordMap = c.RecordPoint.Map
		state.RecordX = c.RecordPoint.X
		state.RecordY = c.RecordPoint.Y
	}
	if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&state).Error; err != nil {
		return err
	}
	for _, model := range characterTables()[1:] {
		if err := tx.Where("character_id = ?", c.ID).Delete(model).Error; err != nil {
			return err
		}
	}
	var items []characterItemRow
	addItems := func(container string, ordinal int, values []game.Item) {
		for slot, item := range values {
			if item == (game.Item{}) {
				continue
			}
			items = append(items, characterItemRow{CharacterOwner: owner, Container: container, OwnerOrdinal: ordinal, Slot: slot, ItemID: item.ID, Count: item.Count, Damage: item.Damage, Metadata: append([]byte(nil), item.Metadata[:]...)})
		}
	}
	addItems(inventoryBag, 0, c.Bag[:])
	addItems(inventoryStorage, 0, c.Storage[:])
	addItems(inventoryEquipment, 0, c.Equipment[:])
	var skills []characterSkillRow
	for i, s := range c.Skills {
		skills = append(skills, characterSkillRow{CharacterOwner: owner, Ordinal: i, SkillID: s.ID, Grade: s.Grade, EXP: s.EXP})
	}
	var quests []characterQuestRow
	for id, q := range c.Quests {
		quests = append(quests, characterQuestRow{CharacterOwner: owner, QuestID: id, StoredID: q.ID, State: q.State, Step: q.Step, Kills: q.Kills, StartedAt: q.StartedAt, CompletedAt: q.CompletedAt})
	}
	var timers []characterTimerRow
	for id, t := range c.EventTimers {
		timers = append(timers, characterTimerRow{CharacterOwner: owner, Kind: timerEvent, TimerID: uint32(id), Expiry: t})
	}
	for id, t := range c.ChestRespawns {
		timers = append(timers, characterTimerRow{CharacterOwner: owner, Kind: timerChest, TimerID: id, Expiry: t})
	}
	var discoveries []characterDiscoveryRow
	for i, id := range c.DiscoveredMonsters {
		discoveries = append(discoveries, characterDiscoveryRow{CharacterOwner: owner, Ordinal: i, Template: id})
	}
	var pets []characterPetRow
	var petSkills []characterPetSkillRow
	for _, group := range []struct {
		location string
		pets     []game.Pet
	}{{petParty, c.Pets}, {petReserve, c.ReservePets}, {petHotel, c.HotelPets}} {
		for i, p := range group.pets {
			pets = append(pets, characterPetRow{CharacterOwner: owner, Location: group.location, Ordinal: i, Base: p.Base, Slot: p.Slot, PetID: p.ID, Name: p.Name, Level: p.Level, Exp: p.Exp, HP: p.HP, MaxHP: p.MaxHP, SP: p.SP, MaxSP: p.MaxSP, StatPoints: p.StatPoints, Amity: p.Amity, Battle: p.Battle, Reborn: p.Reborn, Job: p.Job})
			addItems(group.location, i, p.Equipment[:])
			for j, s := range p.Skills {
				petSkills = append(petSkills, characterPetSkillRow{CharacterOwner: owner, Location: group.location, PetOrdinal: i, Ordinal: j, SkillID: s.ID, Grade: s.Grade, EXP: s.Exp})
			}
		}
	}
	for _, batch := range []struct {
		n    int
		rows any
	}{{len(items), &items}, {len(skills), &skills}, {len(quests), &quests}, {len(timers), &timers}, {len(discoveries), &discoveries}, {len(pets), &pets}, {len(petSkills), &petSkills}} {
		if batch.n > 0 {
			if err := tx.CreateInBatches(batch.rows, characterWriteBatch).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func readCharacterState(tx *gorm.DB, row characterRow) (game.Character, error) {
	var state characterStateRow
	if err := tx.Where("character_id = ?", row.ID).Take(&state).Error; err != nil {
		return game.Character{}, err
	}
	c := game.Character{ID: row.ID, Slot: row.Slot, Name: row.Name, Base: state.Base, Level: state.Level, Element: state.Element, HP: state.HP, MaxHP: state.MaxHP, SP: state.SP, MaxSP: state.MaxSP, EXP: state.EXP, StatPoints: state.StatPoints, Gold: state.Gold, BankGold: state.BankGold, Body: state.Body, Head: state.Head, Color1: state.Color1, Color2: state.Color2, Map: state.Map, X: state.X, Y: state.Y, ActivePet: state.ActivePet, ActiveVehicle: state.ActiveVehicle, VehicleSlot: state.VehicleSlot, ActiveMount: state.ActiveMount, Reborn: state.Reborn, MutedUntil: state.MutedUntil, LuckyDraw: game.LuckyDrawState{Day: state.LuckyDay, Used: state.LuckyUsed}, Quests: make(map[uint32]game.Quest)}
	if state.SettingsPresent {
		c.Settings = &game.ClientSettings{PKAllowed: state.PKAllowed, JoinAllowed: state.JoinAllowed, TradeAllowed: state.TradeAllowed, Channels: state.Channels}
	}
	if state.TentReturnPresent {
		c.TentReturn = &game.Location{Map: state.TentReturnMap, X: state.TentReturnX, Y: state.TentReturnY}
	}
	if state.RecordPresent {
		c.RecordPoint = &game.Location{Map: state.RecordMap, X: state.RecordX, Y: state.RecordY}
	}
	var items []characterItemRow
	var skills []characterSkillRow
	var quests []characterQuestRow
	var timers []characterTimerRow
	var discoveries []characterDiscoveryRow
	var pets []characterPetRow
	var petSkills []characterPetSkillRow
	for _, q := range []struct {
		rows  any
		order string
	}{{&items, "container, owner_ordinal, slot"}, {&skills, "ordinal"}, {&quests, "quest_id"}, {&timers, "kind, timer_id"}, {&discoveries, "ordinal"}, {&pets, "location, ordinal"}, {&petSkills, "location, pet_ordinal, ordinal"}} {
		if err := tx.Where("character_id = ?", row.ID).Order(q.order).Find(q.rows).Error; err != nil {
			return game.Character{}, err
		}
	}
	for _, s := range skills {
		c.Skills = append(c.Skills, game.LearnedSkill{ID: s.SkillID, Grade: s.Grade, EXP: s.EXP})
	}
	for _, q := range quests {
		c.Quests[q.QuestID] = game.Quest{ID: q.StoredID, State: q.State, Step: q.Step, Kills: q.Kills, StartedAt: q.StartedAt, CompletedAt: q.CompletedAt}
	}
	for _, t := range timers {
		switch t.Kind {
		case timerEvent:
			if t.TimerID > uint32(^uint16(0)) {
				return game.Character{}, fmt.Errorf("invalid event timer ID %d", t.TimerID)
			}
			if c.EventTimers == nil {
				c.EventTimers = make(map[uint16]time.Time)
			}
			c.EventTimers[uint16(t.TimerID)] = t.Expiry
		case timerChest:
			if c.ChestRespawns == nil {
				c.ChestRespawns = make(map[uint32]time.Time)
			}
			c.ChestRespawns[t.TimerID] = t.Expiry
		default:
			return game.Character{}, fmt.Errorf("unknown timer kind %q", t.Kind)
		}
	}
	for _, d := range discoveries {
		c.DiscoveredMonsters = append(c.DiscoveredMonsters, d.Template)
	}
	for _, p := range pets {
		var group *[]game.Pet
		switch p.Location {
		case petParty:
			group = &c.Pets
		case petReserve:
			group = &c.ReservePets
		case petHotel:
			group = &c.HotelPets
		default:
			return game.Character{}, fmt.Errorf("unknown pet location %q", p.Location)
		}
		if p.Ordinal != len(*group) {
			return game.Character{}, fmt.Errorf("invalid pet ordinal %d", p.Ordinal)
		}
		*group = append(*group, game.Pet{Base: p.Base, Slot: p.Slot, ID: p.PetID, Name: p.Name, Level: p.Level, Exp: p.Exp, HP: p.HP, MaxHP: p.MaxHP, SP: p.SP, MaxSP: p.MaxSP, StatPoints: p.StatPoints, Amity: p.Amity, Battle: p.Battle, Reborn: p.Reborn, Job: p.Job})
	}
	findPet := func(location string, ordinal int) *game.Pet {
		var group []game.Pet
		switch location {
		case petParty:
			group = c.Pets
		case petReserve:
			group = c.ReservePets
		case petHotel:
			group = c.HotelPets
		}
		if ordinal < 0 || ordinal >= len(group) {
			return nil
		}
		return &group[ordinal]
	}
	for _, s := range petSkills {
		p := findPet(s.Location, s.PetOrdinal)
		if p == nil || s.Ordinal != len(p.Skills) {
			return game.Character{}, fmt.Errorf("orphaned or unordered pet skill")
		}
		p.Skills = append(p.Skills, game.PetSkill{ID: s.SkillID, Grade: s.Grade, Exp: s.EXP})
	}
	for _, r := range items {
		var values []game.Item
		switch r.Container {
		case inventoryBag:
			values = c.Bag[:]
		case inventoryStorage:
			values = c.Storage[:]
		case inventoryEquipment:
			values = c.Equipment[:]
		default:
			p := findPet(r.Container, r.OwnerOrdinal)
			if p != nil {
				values = p.Equipment[:]
			}
		}
		if r.Slot < 0 || r.Slot >= len(values) || len(r.Metadata) != game.ItemMetadataBytes {
			return game.Character{}, fmt.Errorf("invalid persisted item location or metadata")
		}
		if (r.Container == inventoryBag || r.Container == inventoryStorage || r.Container == inventoryEquipment) && r.OwnerOrdinal != 0 {
			return game.Character{}, fmt.Errorf("invalid item owner ordinal")
		}
		item := game.Item{ID: r.ItemID, Count: r.Count, Damage: r.Damage}
		copy(item.Metadata[:], r.Metadata)
		values[r.Slot] = item
	}
	return c, nil
}

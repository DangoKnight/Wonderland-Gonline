package assetsql

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"math"
	"reflect"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
)

// Dataset names describe server definitions, rather than source filenames.
var DefinitionNames = []string{"Manufacturing", "RebornClasses", "Fishing", "Terrains", "Arcades", "Tents", "Economy", "NativeItems", "NPCs", "Skills", "SkillEffects", "Maps", "Talks", "Marks", "StarterItems", "Mall", "Drops", "SalePrices", "PetVouchers", "DisabledEvents", "Critical", "GachaPacks", "LuckyDraw", "Forging", "AlchemyRecipes", "AnimationTiming", "ChestPools", "CombatTrials", "QuestVisibility"}

func definition(c *assets.Catalog, name string) (reflect.Value, error) {
	for _, allowed := range DefinitionNames {
		if name == allowed {
			return reflect.ValueOf(c).Elem().FieldByName(name), nil
		}
	}
	return reflect.Value{}, fmt.Errorf("unknown definition dataset %q", name)
}
func ReadDefinition(tx *gorm.DB, name string) ([]byte, error) {
	c, err := LoadTransaction(tx)
	if err != nil {
		return nil, err
	}
	field, err := definition(c, name)
	if err != nil {
		return nil, err
	}
	return json.Marshal(field.Interface())
}
func ReadDefinitionRecord(tx *gorm.DB, name string, id int) ([]byte, error) {
	c, err := LoadTransaction(tx)
	if err != nil {
		return nil, err
	}
	field, err := definition(c, name)
	if err != nil {
		return nil, err
	}
	if name == "Talks" {
		field = field.FieldByName("ByID")
	}
	if field.Kind() != reflect.Map || field.Type().Key().Kind() == reflect.String {
		return nil, fmt.Errorf("dataset has no numeric record IDs")
	}
	key := reflect.New(field.Type().Key()).Elem()
	if id < 0 || key.OverflowUint(uint64(id)) {
		return nil, fmt.Errorf("invalid record ID")
	}
	key.SetUint(uint64(id))
	value := field.MapIndex(key)
	if !value.IsValid() {
		return nil, gorm.ErrRecordNotFound
	}
	return json.Marshal(value.Interface())
}
func DefinitionCandidate(tx *gorm.DB, name string, raw []byte, recordID *int) (*assets.Catalog, error) {
	c, err := LoadTransaction(tx)
	if err != nil {
		return nil, err
	}
	field, err := definition(c, name)
	if err != nil {
		return nil, err
	}
	if recordID != nil {
		if name == "Talks" {
			field = field.FieldByName("ByID")
		}
		if field.Kind() != reflect.Map || field.Type().Key().Kind() == reflect.String {
			return nil, fmt.Errorf("dataset has no numeric record IDs")
		}
		key := reflect.New(field.Type().Key()).Elem()
		if *recordID < 0 || key.OverflowUint(uint64(*recordID)) {
			return nil, fmt.Errorf("invalid record ID")
		}
		key.SetUint(uint64(*recordID))
		if !field.MapIndex(key).IsValid() {
			return nil, gorm.ErrRecordNotFound
		}
		value := reflect.New(field.Type().Elem())
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(value.Interface()); err != nil {
			return nil, err
		}
		field.SetMapIndex(key, value.Elem())
	} else {
		value := reflect.New(field.Type())
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(value.Interface()); err != nil {
			return nil, err
		}
		field.Set(value.Elem())
	}
	// EVE source sections are opaque interpreter provenance, omitted from API JSON.
	// Preserve them when editing the map's typed geometry and event definitions.
	if name == "Maps" {
		previous, err := readMaps(tx)
		if err != nil {
			return nil, err
		}
		for id, m := range c.Maps {
			if old, ok := previous[id]; ok {
				m.Sections = old.Sections
				c.Maps[id] = m
			}
		}
	}
	if name == "NativeItems" {
		c.Items = make(map[uint16]game.ItemDefinition, len(c.NativeItems))
		for id, item := range c.NativeItems {
			c.Items[id] = item.Definition
		}
	}
	if err = assets.ValidateManufacturing(c.Manufacturing, c.Items); err != nil {
		return nil, err
	}
	if err = validateFishingCatalog(c); err != nil {
		return nil, err
	}
	if err = validateDefinition(name, c); err != nil {
		return nil, err
	}
	return c, nil
}
func validateDefinition(name string, c *assets.Catalog) error {
	marshal := func(v any) []byte { raw, _ := json.Marshal(v); return raw }
	switch name {
	case "Manufacturing":
		return assets.ValidateManufacturing(c.Manufacturing, c.Items)
	case "RebornClasses":
		seen := map[byte]bool{}
		for _, r := range c.RebornClasses {
			if r.Job < game.JobKiller || r.Job > game.JobSeer || r.Name == "" || seen[r.Job] {
				return fmt.Errorf("invalid reborn class")
			}
			seen[r.Job] = true
			if r.Enabled {
				if _, ok := c.Items[r.CapeID]; !ok {
					return fmt.Errorf("missing reborn cape")
				}
			}
		}
		return nil
	case "Fishing":
		return validateFishingCatalog(c)
	case "Terrains":
		return assets.ValidateTerrains(c.Terrains)
	case "Tents":
		return validateTents(c.Tents)
	case "Economy":
		return validateEconomy(c.Economy)
	case "NativeItems":
		if len(c.NativeItems) == 0 {
			return fmt.Errorf("item definitions cannot be empty")
		}
		for id, item := range c.NativeItems {
			if id == 0 || item.Definition.ID != id || item.Definition.Name == "" {
				return fmt.Errorf("invalid item definition %d", id)
			}
		}
	case "NPCs":
		for id, n := range c.NPCs {
			if id == 0 || n.ID != id {
				return fmt.Errorf("NPC identity mismatch %d", id)
			}
		}
	case "Skills", "SkillEffects":
		for id, e := range c.SkillEffects {
			if id != e.ID || id == "" {
				return fmt.Errorf("effect identity mismatch")
			}
			if err := assets.ValidateSkillEffects(e.Effects); err != nil {
				return err
			}
		}
		for id, s := range c.Skills {
			if id == 0 || id != s.ID || math.IsNaN(s.PowerPerLevel) || math.IsInf(s.PowerPerLevel, 0) || math.IsNaN(s.StatMultiplier) || math.IsInf(s.StatMultiplier, 0) {
				return fmt.Errorf("invalid skill %d", id)
			}
			if s.EffectRefs != nil {
				s.Effects = nil
			}
			if err := assets.ResolveSkillEffects(&s, c.SkillEffects); err != nil {
				return err
			}
			c.Skills[id] = s
		}
	case "Maps":
		for id, m := range c.Maps {
			if id == 0 || id != m.ID {
				return fmt.Errorf("map identity mismatch")
			}
			for _, w := range m.Warps {
				if w.MapID != 0 {
					if _, ok := c.Maps[w.MapID]; !ok {
						return fmt.Errorf("unknown warp map %d", w.MapID)
					}
				}
			}
		}
	case "StarterItems":
		for _, g := range c.StarterItems {
			if _, ok := c.Items[g.ID]; !ok || g.Count == 0 {
				return fmt.Errorf("invalid starter item %d", g.ID)
			}
		}
	case "Mall":
		for _, m := range c.Mall {
			if _, ok := c.Items[m.ID]; !ok || m.Count == 0 || m.Cost < 0 || m.Cost > game.MaxMallPoints || m.CategoryID < 0 || m.CategoryID > math.MaxUint8 || m.Order < 0 || m.Order > math.MaxUint8 || (m.Bonus != 0 && m.Bonus != 1) {
				return fmt.Errorf("invalid mall item %d", m.ID)
			}
		}
	case "Drops":
		for id, rewards := range c.Drops {
			if id > math.MaxUint16 {
				return fmt.Errorf("invalid drop monster %d", id)
			}
			if _, ok := c.NPCs[uint16(id)]; !ok {
				return fmt.Errorf("unknown drop monster %d", id)
			}
			for _, r := range rewards {
				if _, ok := c.Items[r.Item]; !ok || r.Min == 0 || r.Min > r.Max || r.Rate < 0 || r.Rate > 100 || math.IsNaN(r.Rate) || math.IsInf(r.Rate, 0) {
					return fmt.Errorf("invalid drop reward %d", r.Item)
				}
			}
		}
	case "Arcades":
		return assets.ValidateArcades(c.Arcades, c.Items)
	case "LuckyDraw":
		pool, err := assets.ParseLuckyDrawPool(marshal(c.LuckyDraw.Rewards), c.Items)
		if err != nil {
			return err
		}
		c.LuckyDraw = pool
	case "ChestPools":
		rows, err := assets.ParseChestPools(marshal(c.ChestPools), c)
		if err != nil {
			return err
		}
		c.ChestPools = rows
	case "CombatTrials":
		rows, err := assets.ParseCombatTrials(marshal(c.CombatTrials), c)
		if err != nil {
			return err
		}
		c.CombatTrials = rows
	case "QuestVisibility":
		rows, err := assets.ParseQuestVisibility(marshal(c.QuestVisibility), c)
		if err != nil {
			return err
		}
		c.QuestVisibility = rows
	case "GachaPacks":
		rows := []assets.GachaPack{}
		for id, p := range c.GachaPacks {
			if id != p.ID {
				return fmt.Errorf("gacha identity mismatch")
			}
			rows = append(rows, p)
		}
		packs, err := assets.ParseGachaPacks(marshal(rows), c.Items)
		if err != nil {
			return err
		}
		c.GachaPacks = packs
		c.UnavailableGachaPacks = nil
	case "Critical":
		if c.Critical.Multiplier < 1 || c.Critical.Multiplier > 10 || math.IsNaN(c.Critical.Multiplier) || math.IsInf(c.Critical.Multiplier, 0) {
			return fmt.Errorf("invalid critical multiplier")
		}
		for id, chance := range c.Critical.Chances {
			if id == 0 || chance < 1 || chance > 100 {
				return fmt.Errorf("invalid critical chance")
			}
		}
	case "PetVouchers":
		for item, pet := range c.PetVouchers {
			if _, ok := c.Items[item]; !ok {
				return fmt.Errorf("unknown voucher item")
			}
			if _, ok := c.NPCs[pet]; !ok {
				return fmt.Errorf("unknown voucher pet")
			}
		}
	case "SalePrices":
		for id := range c.SalePrices {
			if _, ok := c.Items[id]; !ok {
				return fmt.Errorf("unknown sale item")
			}
		}
	case "AlchemyRecipes":
		for _, r := range c.AlchemyRecipes {
			for _, id := range []uint16{r.Input1, r.Input2, r.Output} {
				if _, ok := c.Items[id]; !ok {
					return fmt.Errorf("unknown alchemy item %d", id)
				}
			}
		}
	case "AnimationTiming":
		for id, t := range c.AnimationTiming {
			if id == 0 || t < 0 {
				return fmt.Errorf("invalid animation timing")
			}
		}
	case "Forging":
		for id, u := range c.Forging.Upgrades {
			if id == 0 || u.Scrolls == 0 {
				return fmt.Errorf("invalid forge upgrade")
			}
			if u.Next != 0 {
				if _, ok := c.Forging.Upgrades[u.Next]; !ok {
					return fmt.Errorf("unknown forge successor")
				}
			}
		}
	}
	return nil
}

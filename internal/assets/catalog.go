// Package assets loads runtime catalogs from SQL and provides offline native decoders.
package assets

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"wonderland-gonline/internal/game"
)

var le = binary.LittleEndian

type NPC struct {
	ID        uint16    `json:"id"`
	Name      string    `json:"name"`
	Type      byte      `json:"type"`
	Level     byte      `json:"level"`
	HP        uint32    `json:"hp"`
	SP        uint32    `json:"sp"`
	Element   byte      `json:"element"`
	Stats     [5]uint16 `json:"stats"`
	Skills    [3]uint16 `json:"skills"`
	BookIndex uint16    `json:"book_index"`
	Drops     [5]uint16 `json:"drops"` // Native loot slots, most to least likely.
}

func ParseNPCs(data []byte) (map[uint16]NPC, error) {
	if len(data) < 138 || len(data)%138 != 0 {
		return nil, fmt.Errorf("Npc.dat: invalid record length")
	}
	out := map[uint16]NPC{}
	// Record zero is the native header, not an NPC.
	for off := 138; off < len(data); off += 138 {
		b := data[off : off+138]
		word := func(i int) uint16 { return (le.Uint16(b[i:]) ^ 0x5209) - 1 }
		n := NPC{ID: word(12), Type: (b[11] ^ 0xc8) - 1, Level: (b[37] ^ 0xc8) - 1, HP: (le.Uint32(b[38:]) ^ 0x0baeb716) - 1, SP: (le.Uint32(b[42:]) ^ 0x0baeb716) - 1, Element: (b[57] ^ 0xc8) - 1, BookIndex: word(90)}
		if n.ID == 0 {
			continue
		}
		var name []byte
		for i := 10; i >= 1; i-- {
			if b[i] >= 32 && b[i] <= 126 {
				name = append(name, b[i])
			}
		}
		n.Name = strings.TrimSpace(string(name))
		if n.Name == "" {
			n.Name = fmt.Sprintf("NPC_%d", n.ID)
		}
		for i := range n.Stats {
			n.Stats[i] = word(46 + 2*i)
		}
		for i := range n.Skills {
			n.Skills[i] = word(58 + 2*i)
		}
		for i := range n.Drops {
			n.Drops[i] = word(64 + 2*i)
		}
		out[n.ID] = n
	}
	return out, nil
}

type Talks struct {
	ByID     map[uint32]string
	ByIndex  map[uint32]string
	ByOffset map[uint32]string
}

func ParseTalks(data []byte) (Talks, error) {
	t := Talks{map[uint32]string{}, map[uint32]string{}, map[uint32]string{}}
	if len(data)%292 != 0 {
		return t, fmt.Errorf("Talk.dat: invalid record length")
	}
	for off := 0; off < len(data); off += 292 {
		b := data[off : off+292]
		id := uint32((le.Uint16(b) ^ 0xecea) - 5)
		n := int(b[2])
		if n < 1 || n > 254 {
			continue
		}
		start := 257 - n
		text := make([]byte, n)
		for i := range text {
			text[i] = b[start+n-1-i]
		}
		s := strings.TrimSpace(string(text))
		s = strings.TrimSpace(strings.TrimPrefix(s, "fffff"))
		if s == "" {
			continue
		}
		if id > 0 {
			t.ByID[id] = s
		}
		t.ByIndex[uint32(off/292)] = s
		t.ByOffset[uint32(off)] = s
		t.ByOffset[uint32(off+start)] = s
	}
	return t, nil
}
func (t Talks) Resolve(id uint32) (string, bool) {
	if id == 0 {
		return "", false
	}
	if s, ok := t.ByID[id]; ok {
		return s, true
	}
	s, ok := t.ByOffset[id]
	return s, ok
}

type MallItem struct {
	ID            uint16 `json:"item_id"`
	Name          string `json:"item_name"`
	Category      string `json:"category"`
	CategoryID    int    `json:"category_id"`
	Cost          int    `json:"point_cost"`
	OriginalPrice int    `json:"original_price"`
	GoldCost      int    `json:"gold_cost"`
	Count         byte   `json:"count"`
	Hot           int    `json:"is_hot"`
	New           int    `json:"is_new"`
	Limited       int    `json:"is_limited"`
	Sale          int    `json:"on_sale"`
	Discount      int    `json:"discount"`
	Badge         int    `json:"badge"`
	Order         int    `json:"order_idx"`
	Bonus         int    `json:"is_bonus"`
}
type Catalog struct {
	Instances        []InstanceDefinition
	QuestDefinitions map[uint32]QuestDefinition `json:"quest_definitions,omitempty"`
	Manufacturing    map[uint16]ManufacturingFormula
	RebornClasses    []RebornClass
	Fishing          FishingRules
	Terrains         map[uint16]Terrain

	Arcades               []ArcadeGame      `json:"arcades"`
	Tents                 TentRules         `json:"tents"`
	Economy               Economy           `json:"economy"`
	CombatTrials          []CombatTrial     `json:"combat_trials,omitempty"`
	QuestVisibility       []QuestVisibility `json:"quest_visibility,omitempty"`
	ChestPools            []ChestPool
	LuckyDraw             LuckyDrawPool
	Forging               Forging
	AssetsDatabase        string
	NativeItems           map[uint16]NativeItem
	ItemCatalog           string
	Items                 map[uint16]game.ItemDefinition
	StarterItems          []game.StarterGrant
	Skills                map[uint16]Skill
	SkillEffects          map[string]EffectDefinition
	AnimationTiming       map[uint16]int
	NPCs                  map[uint16]NPC
	Talks                 Talks
	Maps                  map[uint16]Map
	Mall                  []MallItem
	Marks                 map[uint16]uint16 // Mark ID to completion flag; zero flags are journal entries.
	DisabledEvents        map[uint32]string // EventKey to the reason the event is unavailable.
	Drops                 map[uint32][]Drop // monster_drops.txt by monster template.
	Critical              game.CriticalHits
	SalePrices            map[uint16]SalePrice
	GachaPacks            map[uint16]GachaPack
	UnavailableGachaPacks map[uint16]GachaPackIssue
	AlchemyRecipes        []AlchemyRecipe
	PetVouchers           map[uint16]uint16
	Warnings              []string
}
type Summary struct {
	CombatTrials      int      `json:"combat_trials,omitempty"`
	ForgeUpgradeItems int      `json:"forge_upgrade_items"`
	PointForgeItems   int      `json:"point_forge_items"`
	AssetsDatabase    string   `json:"assets_database,omitempty"`
	NativeItems       int      `json:"native_items"`
	ItemCatalog       string   `json:"item_catalog,omitempty"`
	Items             int      `json:"items"`
	StarterItems      int      `json:"starter_items"`
	Skills            int      `json:"skills"`
	AnimationTimings  int      `json:"animation_timings"`
	NPCs              int      `json:"npcs"`
	Dialogues         int      `json:"dialogues"`
	Maps              int      `json:"maps"`
	MapNPCs           int      `json:"map_npcs"`
	Events            int      `json:"events"`
	PreEvents         int      `json:"pre_events"`
	Warps             int      `json:"warps"`
	GroundItems       int      `json:"ground_items"`
	MallItems         int      `json:"mall_items"`
	GachaPacks        int      `json:"gacha_packs"`
	AlchemyRecipes    int      `json:"alchemy_recipes"`
	Marks             int      `json:"marks"`
	DisabledEvents    int      `json:"disabled_events"`
	Warnings          []string `json:"warnings"`
}

func (c *Catalog) Summary() Summary {
	s := Summary{CombatTrials: len(c.CombatTrials), ForgeUpgradeItems: len(c.Forging.Upgrades), PointForgeItems: len(c.Forging.PointItems), AssetsDatabase: c.AssetsDatabase, NativeItems: len(c.NativeItems), ItemCatalog: c.ItemCatalog, GachaPacks: len(c.GachaPacks), AlchemyRecipes: len(c.AlchemyRecipes), Items: len(c.Items), StarterItems: len(c.StarterItems), Skills: len(c.Skills), AnimationTimings: len(c.AnimationTiming), NPCs: len(c.NPCs), Dialogues: len(c.Talks.ByID), Maps: len(c.Maps), MallItems: len(c.Mall), Marks: len(c.Marks), DisabledEvents: len(c.DisabledEvents), Warnings: c.Warnings}
	for _, m := range c.Maps {
		s.MapNPCs += len(m.NPCs)
		s.Events += len(m.Events)
		s.PreEvents += len(m.PreEvents)
		s.Warps += len(m.Warps)
		s.GroundItems += len(m.Items)
	}
	return s
}

// Load is the offline reference loader used by format and compatibility tests.
// Server startup must use assetsql.LoadDatabase.
func Load(dir, itemCatalog string) (*Catalog, error) {
	c := &Catalog{Items: map[uint16]game.ItemDefinition{}, NPCs: map[uint16]NPC{}, Maps: map[uint16]Map{}, Warnings: []string{}, Mall: []MallItem{}, Marks: map[uint16]uint16{}, DisabledEvents: map[uint32]string{}}
	load := func(name string, fn func([]byte) error) {
		b, e := os.ReadFile(filepath.Join(dir, name))
		if e == nil {
			e = fn(b)
		}
		if e != nil {
			c.Warnings = append(c.Warnings, name+": "+e.Error())
		}
	}
	if err := c.loadItems(itemCatalog); err != nil {
		return nil, err
	}
	load("gacha_packs.json", func(b []byte) error {
		v, e := ParseGachaPacks(b, c.Items)
		if e == nil {
			c.GachaPacks = v
		}
		return e
	})
	load("starter_items.json", func(b []byte) error {
		v, e := ParseStarterItems(b)
		if e == nil {
			c.StarterItems = v
		}
		return e
	})
	load("pet_vouchers.json", func(b []byte) error {
		v, e := ParsePetVouchers(b)
		if e == nil {
			c.PetVouchers = v
		}
		return e
	})
	c.AlchemyRecipes = defaultAlchemyRecipes()
	// Native AlchemyManager appends Compound2 before Compound; the first match wins.
	for _, name := range []string{"Compound2.dat", "Compound.dat"} {
		load(name, func(b []byte) error {
			v, e := ParseAlchemyRecipes(b)
			if e == nil {
				c.AlchemyRecipes = append(c.AlchemyRecipes, v...)
			}
			return e
		})
	}
	load("Skill.dat", func(b []byte) error {
		v, e := ParseSkills(b)
		if e == nil {
			c.Skills = v
		}
		return e
	})
	load("SkillData.MBTM", func(b []byte) error {
		v, e := ParseAnimationTiming(b)
		if e == nil {
			c.AnimationTiming = v
		}
		return e
	})
	load("Npc.dat", func(b []byte) error {
		v, e := ParseNPCs(b)
		if e == nil {
			c.NPCs = v
		}
		return e
	})
	load("Talk.dat", func(b []byte) error {
		v, e := ParseTalks(b)
		if e == nil {
			c.Talks = v
		}
		return e
	})
	load("eve.Emg", func(b []byte) error {
		v, e := ParseEVE(b)
		if e == nil {
			c.Maps = v
		}
		return e
	})
	load("Mark.dat", func(b []byte) error {
		v, e := ParseMarks(b)
		if e == nil {
			c.Marks = v
		}
		return e
	})
	load("disabled_quest_events.csv", func(b []byte) error {
		v, e := ParseDisabledEvents(b)
		if e == nil {
			c.DisabledEvents = v
		}
		return e
	})
	load("monster_drops.txt", func(b []byte) error {
		v, e := ParseDrops(b)
		if e == nil {
			c.Drops = v
		}
		return e
	})
	load("npc_sale_prices.csv", func(b []byte) error {
		v, e := ParseSalePrices(b)
		if e == nil {
			c.SalePrices = v
		}
		return e
	})
	load("critical_hits.json", func(b []byte) error {
		v, e := game.ParseCriticalHits(b)
		if e == nil {
			c.Critical = v
		}
		return e
	})
	load("item_mall.json", func(b []byte) error {
		var v []MallItem
		if e := json.Unmarshal(b, &v); e != nil {
			return e
		}
		for _, i := range v {
			if i.ID == 0 || i.Count == 0 || i.Cost < 0 {
				return fmt.Errorf("invalid mall item %d", i.ID)
			}
		}
		c.Mall = v
		return nil
	})
	return c, nil
}

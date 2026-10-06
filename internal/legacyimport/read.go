// Package legacyimport reads a C# SQLite snapshot offline. Runtime never opens it.
package legacyimport

import (
	"context"
	"fmt"
	"gorm.io/gorm"
	"math"
	"strconv"
	"strings"
	"time"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/store"
)

type Report struct {
	Accounts, Characters, Items, Pets, Skills, Quests, Friends int
	Unmapped                                                   map[string]int64
}
type Plan struct {
	Accounts    []store.LegacyAccount
	Friendships []store.LegacyFriendship
	Report      Report
}
type row map[string]any
type decoder struct {
	row row
	err error
}

func (d *decoder) n(key string, limit uint64) uint64 {
	value := d.row[strings.ToLower(key)]
	if value == nil {
		return 0
	}
	n, err := strconv.ParseUint(fmt.Sprint(value), 10, 64)
	if err != nil || n > limit {
		if d.err == nil {
			d.err = fmt.Errorf("invalid %s", key)
		}
		return 0
	}
	return n
}
func (d *decoder) s(key string) string {
	value := d.row[strings.ToLower(key)]
	if value == nil {
		return ""
	}
	if b, ok := value.([]byte); ok {
		return string(b)
	}
	return fmt.Sprint(value)
}
func rows(tx *gorm.DB, table string) ([]row, error) {
	if !tx.Migrator().HasTable(table) {
		return nil, nil
	}
	var values []map[string]any
	if err := tx.Table(table).Find(&values).Error; err != nil {
		return nil, err
	}
	out := make([]row, len(values))
	for i, v := range values {
		out[i] = row{}
		for k, value := range v {
			out[i][strings.ToLower(k)] = value
		}
	}
	return out, nil
}

// Read uses one read-only transaction so children cannot come from a different
// checkpoint. It reports unmapped tables; callers must resolve these before import.
func Read(ctx context.Context, path string, catalog *assets.Catalog) (plan Plan, err error) {
	if catalog == nil {
		return plan, fmt.Errorf("an SQL asset catalog is required")
	}
	db, err := assetdb.OpenReadOnly(path)
	if err != nil {
		return plan, err
	}
	defer assetdb.Close(db)
	plan.Report.Unmapped = map[string]int64{}
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if !tx.Migrator().HasTable("users") || !tx.Migrator().HasTable("characters") {
			return fmt.Errorf("not a supported Private Server snapshot: users/characters missing")
		}
		users, e := rows(tx, "users")
		if e != nil {
			return e
		}
		owners := map[uint32]int{}
		for _, raw := range users {
			d := decoder{row: raw}
			id := uint32(d.n("userID", math.MaxUint32))
			if id == 0 || id >= store.SecondCharacterIDOffset-store.UserIDOffset {
				return fmt.Errorf("missing legacy user ID")
			}
			if _, dup := owners[id]; dup {
				return fmt.Errorf("duplicate legacy user ID")
			}
			points := d.n("im_points", uint64(game.MaxMallPoints))
			if _, ok := raw["im_points"]; !ok {
				points = d.n("im", uint64(game.MaxMallPoints))
			}
			a := store.LegacyAccount{Account: store.Account{ID: id, Username: d.s("username"), Email: d.s("email"), IM: int64(points), IMBonus: int64(d.n("im_bonus", uint64(game.MaxMallPoints))), GMLevel: byte(d.n("gm_level", math.MaxUint8)), Banned: d.n("banned", 1) != 0}, Password: d.s("password")}
			if d.err != nil {
				return fmt.Errorf("legacy user %d: %w", id, d.err)
			}
			if e := store.ValidateCredentials(strings.ToLower(a.Account.Username), a.Password); e != nil {
				return fmt.Errorf("legacy user %d: credentials need reset before import", id)
			}
			// Salted forum credentials are deliberately not interpreted as plaintext.
			if d.s("members_pass_salt") != "" {
				return fmt.Errorf("legacy user %d: IPBoard credentials require a password reset", id)
			}
			owners[id] = len(plan.Accounts)
			plan.Accounts = append(plan.Accounts, a)
		}
		chars, e := rows(tx, "characters")
		if e != nil {
			return e
		}
		indices := map[uint32][2]int{}
		for _, raw := range chars {
			d := decoder{row: raw}
			id := uint32(d.n("charID", math.MaxUint32))
			slot := byte(d.n("slot", 2))
			if slot == 0 || id == 0 {
				return fmt.Errorf("invalid legacy character identity")
			}
			offset := uint32(store.UserIDOffset)
			if slot == 2 {
				offset += store.SecondCharacterIDOffset
			}
			if id <= offset {
				return fmt.Errorf("legacy character %d: invalid owner", id)
			}
			owner, ok := owners[id-offset]
			if !ok {
				return fmt.Errorf("legacy character %d: orphaned account", id)
			}
			if _, dup := indices[id]; dup {
				return fmt.Errorf("duplicate legacy character %d", id)
			}
			if d.n("job", math.MaxUint8) != 0 || d.s("nickname") != "" {
				return fmt.Errorf("legacy character %d: job/nickname mapping remains unresolved", id)
			}
			c := game.Character{ID: id, Slot: slot, Name: d.s("name"), Body: uint16(d.n("body", math.MaxUint16)), Head: uint16(d.n("head", math.MaxUint16)), Element: byte(d.n("element", 4)), Map: uint16(d.n("location_map", math.MaxUint16)), X: uint16(d.n("location_x", math.MaxUint16)), Y: uint16(d.n("location_y", math.MaxUint16)), Gold: uint32(d.n("gold", uint64(game.MaxGold))), Reborn: d.n("rebirth", 1) != 0, Level: 1, Quests: map[uint32]game.Quest{}}
			if c.Reborn {
				return fmt.Errorf("legacy character %d: rebirth EXP mapping remains unresolved", id)
			}
			c.Color1 = uint32(d.n("haircolor", math.MaxUint16)) | uint32(d.n("skincolor", math.MaxUint16))<<16
			c.Color2 = uint32(d.n("clothingcolor", math.MaxUint16)) | uint32(d.n("eyecolor", math.MaxUint16))<<16
			if _, ok := catalog.Maps[c.Map]; !ok {
				return fmt.Errorf("legacy character %d: map %d unavailable", id, c.Map)
			}
			code := d.s("cipher")
			a := &plan.Accounts[owner]
			if code != "" {
				if a.DeletionCode != "" && a.DeletionCode != code {
					return fmt.Errorf("legacy account %d: inconsistent deletion codes", a.Account.ID)
				}
				a.DeletionCode = code
			}
			if d.err != nil {
				return fmt.Errorf("legacy character %d: %w", id, d.err)
			}
			indices[id] = [2]int{owner, len(a.Characters)}
			a.Characters = append(a.Characters, c)
		}
		get := func(d *decoder) (*game.Character, error) {
			id := uint32(d.n("charID", math.MaxUint32))
			at, ok := indices[id]
			if !ok {
				return nil, fmt.Errorf("orphaned legacy child for character %d", id)
			}
			return &plan.Accounts[at[0]].Characters[at[1]], nil
		}
		stats, e := rows(tx, "stats")
		if e != nil {
			return e
		}
		seenStats := map[[2]uint32]bool{}
		for _, raw := range stats {
			d := decoder{row: raw}
			c, e := get(&d)
			if e != nil {
				return e
			}
			id := uint32(d.n("statID", math.MaxUint8))
			key := [2]uint32{c.ID, id}
			if seenStats[key] {
				return fmt.Errorf("duplicate legacy stat %d/%d", c.ID, id)
			}
			seenStats[key] = true
			if d.n("potential", math.MaxUint16) != 0 {
				return fmt.Errorf("legacy character %d: potential mapping remains unresolved", c.ID)
			}
			switch id {
			case uint32(game.StatCurrentHP):
				c.HP = uint32(d.n("StatusUp", math.MaxUint32))
			case uint32(game.StatCurrentSP):
				c.SP = uint32(d.n("StatusUp", math.MaxUint32))
			case uint32(game.StatTotalEXP):
				c.EXP = uint32(d.n("StatusUp", math.MaxUint32))
			case uint32(game.StatUnallocatedPoints):
				c.StatPoints = uint16(d.n("StatusUp", math.MaxUint16))
			case uint32(game.StatSTR):
				c.Base.Strength = uint16(d.n("StatusUp", math.MaxUint16))
			case uint32(game.StatCON):
				c.Base.Constitution = uint16(d.n("StatusUp", math.MaxUint16))
			case uint32(game.StatINT):
				c.Base.Intelligence = uint16(d.n("StatusUp", math.MaxUint16))
			case uint32(game.StatWIS):
				c.Base.Wisdom = uint16(d.n("StatusUp", math.MaxUint16))
			case uint32(game.StatAGI):
				c.Base.Agility = uint16(d.n("StatusUp", math.MaxUint16))
			default:
				return fmt.Errorf("legacy character %d: unsupported stat %d", c.ID, id)
			}
			if d.err != nil {
				return fmt.Errorf("legacy stat %d/%d: %w", c.ID, id, d.err)
			}
		}
		inv, e := rows(tx, "inventory")
		if e != nil {
			return e
		}
		for _, raw := range inv {
			d := decoder{row: raw}
			c, e := get(&d)
			if e != nil {
				return e
			}
			id := uint16(d.n("itemID", math.MaxUint16))
			if id == 0 {
				continue
			}
			def, ok := catalog.Items[id]
			if !ok {
				return fmt.Errorf("legacy item %d: unavailable definition", id)
			}
			if d.n("socketID", math.MaxUint32) != 0 || d.n("bombID", math.MaxUint32) != 0 || d.n("sewID", math.MaxUint32) != 0 {
				return fmt.Errorf("legacy item %d: socket/bomb/sew metadata mapping unresolved", id)
			}
			item := game.Item{ID: id, Count: byte(d.n("qty", uint64(def.StackLimit()))), Damage: byte(d.n("dmg", math.MaxUint8))}
			item.SetForge(byte(d.n("forge", math.MaxUint8)))
			container := d.n("storID", 2)
			position := int(d.n("pos", game.BagSize))
			if position < 1 || item.Count == 0 {
				return fmt.Errorf("invalid legacy inventory position/count")
			}
			var target *game.Item
			switch container {
			case 0:
				target = &c.Bag[position-1]
			case 1:
				if position > len(c.Equipment) || item.Count != 1 {
					return fmt.Errorf("invalid legacy equipment")
				}
				target = &c.Equipment[position-1]
			case 2:
				target = &c.Storage[position-1]
			}
			if d.err != nil {
				return fmt.Errorf("legacy item %d: %w", id, d.err)
			}
			if !target.Empty() {
				return fmt.Errorf("duplicate legacy inventory position")
			}
			*target = item
			plan.Report.Items++
		}
		skillRows, e := rows(tx, "character_skills")
		if e != nil {
			return e
		}
		for _, raw := range skillRows {
			d := decoder{row: raw}
			c, e := get(&d)
			if e != nil {
				return e
			}
			id := uint16(d.n("skillID", math.MaxUint16))
			if _, ok := catalog.Skills[id]; !ok {
				return fmt.Errorf("legacy skill %d: unavailable definition", id)
			}
			skill := game.LearnedSkill{ID: id, Grade: byte(d.n("grade", 10)), EXP: uint32(d.n("exp", math.MaxUint32))}
			if skill.Grade == 0 || d.err != nil {
				return fmt.Errorf("invalid legacy skill %d", id)
			}
			for _, old := range c.Skills {
				if old.ID == id {
					return fmt.Errorf("duplicate legacy skill %d", id)
				}
			}
			c.Skills = append(c.Skills, skill)
			plan.Report.Skills++
		}
		questRows, e := rows(tx, "charquest")
		if e != nil {
			return e
		}
		for _, raw := range questRows {
			d := decoder{row: raw}
			c, e := get(&d)
			if e != nil {
				return e
			}
			id := uint32(d.n("quest_started", math.MaxUint32))
			state := d.n("quest_pos", 3)
			if id == 0 || state == 0 {
				return fmt.Errorf("invalid legacy quest identity/state")
			}
			q := game.Quest{ID: id, Step: int(d.n("step", math.MaxUint16)), Kills: int(d.n("kill_count", math.MaxInt32)), StartedAt: time.Time{}}
			switch state {
			case 1:
				q.State = game.InProgress
			case 2:
				q.State = game.NotStarted
			case 3:
				q.State = game.Completed
			}
			if value := d.s("completed_at"); value != "" {
				completed, err := time.Parse(time.RFC3339Nano, value)
				if err != nil || q.State != game.Completed {
					return fmt.Errorf("legacy quest %d: invalid completion timestamp", id)
				}
				completed = completed.UTC()
				q.CompletedAt = &completed
			}
			if _, dup := c.Quests[id]; dup {
				return fmt.Errorf("duplicate legacy quest")
			}
			if d.err != nil {
				return d.err
			}
			c.Quests[id] = q
			plan.Report.Quests++
		}
		if e = readPets(tx, catalog, &plan, get); e != nil {
			return e
		}
		extRows, e := rows(tx, "charactersextdata")
		if e != nil {
			return e
		}
		seenSettings := map[uint32]bool{}
		unmappedExtended := false
		for _, raw := range extRows {
			d := decoder{row: raw}
			c, e := get(&d)
			if e != nil {
				return e
			}
			if seenSettings[c.ID] {
				return fmt.Errorf("duplicate legacy settings")
			}
			seenSettings[c.ID] = true
			if d.s("Friends") != "" || d.s("Mail") != "" || (d.s("Guild") != "" && d.s("Guild") != "0") {
				unmappedExtended = true
			}
			if text := d.s("Settings"); text != "" {
				fields := strings.Fields(text)
				if len(fields) != 4 {
					return fmt.Errorf("invalid legacy settings")
				}
				pk, e1 := strconv.ParseBool(fields[0])
				join, e2 := strconv.ParseBool(fields[1])
				channels, e3 := strconv.ParseUint(fields[2], 10, 8)
				trade, e4 := strconv.ParseBool(fields[3])
				if e1 != nil || e2 != nil || e3 != nil || e4 != nil || channels > game.AllChatChannels {
					return fmt.Errorf("invalid legacy settings")
				}
				c.Settings = &game.ClientSettings{PKAllowed: pk, JoinAllowed: join, TradeAllowed: trade, Channels: byte(channels)}
			}
		}
		records, e := rows(tx, "character_record_points")
		if e != nil {
			return e
		}
		for _, raw := range records {
			d := decoder{row: raw}
			c, e := get(&d)
			if e != nil {
				return e
			}
			location := game.Location{Map: uint16(d.n("mapID", math.MaxUint16)), X: uint16(d.n("posX", math.MaxUint16)), Y: uint16(d.n("posY", math.MaxUint16))}
			if _, ok := catalog.Maps[location.Map]; !ok || c.RecordPoint != nil || d.err != nil {
				return fmt.Errorf("invalid legacy record point")
			}
			c.RecordPoint = &location
		}
		discovered, e := rows(tx, "character_monster_book")
		if e != nil {
			return e
		}
		for _, raw := range discovered {
			d := decoder{row: raw}
			c, e := get(&d)
			if e != nil {
				return e
			}
			id := uint16(d.n("npcID", math.MaxUint16))
			if id == 0 || d.err != nil {
				return fmt.Errorf("invalid legacy monster book entry")
			}
			if _, ok := catalog.NPCs[id]; !ok {
				return fmt.Errorf("unknown legacy monster book NPC")
			}
			for _, existing := range c.DiscoveredMonsters {
				if existing == id {
					return fmt.Errorf("duplicate legacy monster book entry")
				}
			}
			c.DiscoveredMonsters = append(c.DiscoveredMonsters, id)
		}
		friendRows, e := rows(tx, "Friends")
		if e != nil {
			return e
		}
		seenFriends := map[[2]uint32]bool{}
		friendCounts := map[uint32]int{}
		for _, raw := range friendRows {
			d := decoder{row: raw}
			a, b := uint32(d.n("CharID1", math.MaxUint32)), uint32(d.n("CharID2", math.MaxUint32))
			if a > b {
				a, b = b, a
			}
			_, aOK := indices[a]
			_, bOK := indices[b]
			key := [2]uint32{a, b}
			if d.err != nil || !aOK || !bOK || a == b || seenFriends[key] {
				return fmt.Errorf("invalid legacy friendship")
			}
			friendCounts[a]++
			friendCounts[b]++
			if friendCounts[a] > store.FriendLimit || friendCounts[b] > store.FriendLimit {
				return store.ErrFriendLimit
			}
			seenFriends[key] = true
			plan.Friendships = append(plan.Friendships, store.LegacyFriendship{Character1: a, Character2: b})
			plan.Report.Friends++
		}
		for i := range plan.Accounts {
			for j := range plan.Accounts[i].Characters {
				c := &plan.Accounts[i].Characters[j]
				c.Level = game.LevelForExp(uint64(c.EXP))
				v := c.Combat(catalog.Items)
				c.MaxHP, c.MaxSP = uint32(max(0, v.MaxHP)), uint32(max(0, v.MaxSP))
				if c.HP > c.MaxHP || c.SP > c.MaxSP {
					return fmt.Errorf("legacy character %d: vitals exceed compiled growth limits", c.ID)
				}
				if e = c.Validate(); e != nil {
					return e
				}
			}
		}
		mapped := map[string]bool{"users": true, "characters": true, "stats": true, "inventory": true, "character_skills": true, "character_pets": true, "charquest": true, "friends": true, "character_record_points": true, "character_monster_book": true, "charactersextdata": !unmappedExtended}
		tables, e := tx.Migrator().GetTables()
		if e != nil {
			return e
		}
		for _, table := range tables {
			if mapped[strings.ToLower(table)] || strings.HasPrefix(strings.ToLower(table), "sqlite_") {
				continue
			}
			var count int64
			if e = tx.Table(table).Count(&count).Error; e != nil {
				return e
			}
			if count > 0 {
				plan.Report.Unmapped[table] = count
			}
		}
		plan.Report.Accounts = len(plan.Accounts)
		plan.Report.Characters = len(indices)
		return nil
	})
	if err != nil {
		return Plan{}, err
	}
	return plan, nil
}

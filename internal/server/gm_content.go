package server

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/assetsql"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/store"
)

func (s *Server) gmBazaar(ctx context.Context, c *Session, _ string, args []string) error {
	usage := "/bazaar list | remove <ID> | create <item|pet|vehicle> <finite|infinite> <ID:price:stock,...> <title>"
	if len(args) == 0 {
		return s.chatFeedback(c, usage)
	}
	switch strings.ToLower(args[0]) {
	case "list":
		if len(args) != 1 {
			return s.chatFeedback(c, usage)
		}
		bs, err := s.Store.Bazaars(ctx, 0)
		if err != nil {
			return err
		}
		for _, b := range bs {
			s.sendOrClose(c, serverwire.SystemLine(fmt.Sprintf("Bazaar %d: %s, map %d (%d,%d), %d listings", b.VendorID, b.Title, b.Map, b.X, b.Y, len(b.Listings))))
		}
		return nil
	case "remove":
		if len(args) != 2 {
			return s.chatFeedback(c, usage)
		}
		id, err := strconv.ParseUint(args[1], 10, 32)
		if err != nil {
			return s.chatFeedback(c, usage)
		}
		b, err := s.Store.Bazaar(ctx, uint32(id))
		if err != nil {
			return s.chatFeedback(c, "Bazaar not found.")
		}
		if err = s.Store.DeleteBazaar(ctx, b.VendorID); err != nil {
			return err
		}
		packet := protocol.Builder{protocol.CommandInventory, protocol.InventoryStallDespawn}.U32(b.VendorID)
		for _, peer := range s.world {
			delete(peer.bazaarQuotes, b.VendorID)
			if peer.tentOwner == 0 && peer.character.Map == b.Map {
				s.sendOrClose(peer, packet, protocol.Builder{protocol.CommandMapAcknowledgment}.U32(b.VendorID).U16(0).U16(0).U16(0).U16(0).U8(0))
			}
		}
		return s.chatFeedback(c, "Bazaar removed.")
	case "create":
		if len(args) < 5 || c.tentOwner != 0 {
			return s.chatFeedback(c, usage)
		}
		kinds := map[string]byte{"item": store.BazaarItems, "pet": store.BazaarPets, "vehicle": store.BazaarVehicles}
		kind := kinds[strings.ToLower(args[1])]
		mode := strings.ToLower(args[2])
		if kind == 0 || mode != "finite" && mode != "infinite" {
			return s.chatFeedback(c, usage)
		}
		b := store.Bazaar{Name: "Merchant", Title: strings.Join(args[4:], " "), Map: c.character.Map, X: c.character.X, Y: c.character.Y, Kind: kind, Infinite: mode == "infinite"}
		for _, entry := range strings.Split(args[3], ",") {
			parts := strings.Split(entry, ":")
			if len(parts) != 3 {
				return s.chatFeedback(c, usage)
			}
			id, e1 := strconv.ParseUint(parts[0], 10, 16)
			price, e2 := strconv.ParseUint(parts[1], 10, 32)
			stock, e3 := strconv.ParseUint(parts[2], 10, 32)
			if e1 != nil || e2 != nil || e3 != nil {
				return s.chatFeedback(c, usage)
			}
			if kind == store.BazaarPets {
				if _, known := s.Assets.NPCs[uint16(id)]; !known {
					return s.chatFeedback(c, "Unknown pet template.")
				}
			} else {
				d, known := s.Assets.Items[uint16(id)]
				if !known || kind == store.BazaarVehicles && d.Type != game.VehicleType && d.Type != game.ItemTypeAppliance {
					return s.chatFeedback(c, "Unknown item or vehicle/appliance definition.")
				}
			}
			b.Listings = append(b.Listings, store.BazaarListing{ItemID: uint16(id), Price: uint32(price), Quantity: uint32(stock)})
		}
		saved, err := s.Store.SaveBazaar(ctx, b)
		if err != nil {
			return s.chatFeedback(c, err.Error())
		}
		for _, peer := range s.world {
			if peer.tentOwner == 0 && peer.character.Map == b.Map {
				s.sendOrClose(peer, bazaarPackets(saved)...)
			}
		}
		return s.chatFeedback(c, fmt.Sprintf("Bazaar %d created.", saved.VendorID))
	}
	return s.chatFeedback(c, usage)
}

const (
	gmPetNameMaxBytes         = 16
	gmRobinsonTemplateID      = 12032
	gmRobinsonBroadcastID     = 12178
	gmRocaTemplateID          = 14161
	gmRocaAlternateTemplateID = 14162
	gmRocaMacheteItemID       = 10063
	gmXaolanTemplateID        = 14156
	gmXaolanJadeItemID        = 25028
)

func gmSelectedPet(c *game.Character) int {
	for i, p := range c.Pets {
		if p.Battle {
			return i
		}
	}
	for i, p := range c.Pets {
		if c.ActivePet != 0 && game.SamePet(p.ID, c.ActivePet) {
			return i
		}
	}
	if len(c.Pets) > 0 {
		return 0
	}
	return -1
}

func (s *Server) gmPetEdit(ctx context.Context, c *Session, command string, args []string) error {
	if command == "pet" {
		return s.gmRecruitPet(ctx, c, args)
	}
	next := c.character.Clone()
	index := gmSelectedPet(&next)
	if index < 0 {
		return s.chatFeedback(c, "No companion or pet is available.")
	}
	if len(args) > 1 {
		return s.chatFeedback(c, "Too many arguments for this pet command.")
	}
	pet := &next.Pets[index]
	var packets [][]byte
	switch command {
	case "amity", "petamity":
		value := uint64(game.PetRebirthRequiredAmity)
		if len(args) == 1 {
			n, err := strconv.ParseUint(args[0], 10, 8)
			if err != nil {
				return s.chatFeedback(c, "Amity must be a number from 0 to 100.")
			}
			value = min(n, game.PetRebirthRequiredAmity)
		}
		pet.Amity = byte(value)
		packets = append(packets, game.PetStat(c.pets.slot(pet.ID), petFeedingAmityStatus, int64(pet.Amity)))
	case "rebirth", "petrebirth":
		if len(args) != 0 {
			return s.chatFeedback(c, "Usage: /rebirth")
		}
		if pet.Reborn {
			return s.chatFeedback(c, "This pet is already reborn.")
		}
		if pet.StatPoints > math.MaxUint16-game.PetRebirthBonusStatPoints {
			return s.chatFeedback(c, "Spend some pet points before rebirth.")
		}
		// GM override bypasses level/amity requirements, but repeated rebirth must
		// not mint the bonus again. Normalize before saving as the native path does.
		pet.Reborn = true
		pet.Level = game.PetRebirthStartingLevel
		pet.Exp = 0
		pet.StatPoints += game.PetRebirthBonusStatPoints
		pet.Amity = game.PetRebirthRequiredAmity
		pet.Normalize(s.Assets.Items, true)
		packets = append(packets, []byte{protocol.CommandPetRebirth, protocol.PetRebirthAscend, c.pets.slot(pet.ID), protocol.PetRebirthSucceeded})
	case "petlvl", "petlevel":
		if len(args) != 1 {
			return s.chatFeedback(c, "Usage: /petlvl <level>")
		}
		level, err := strconv.ParseUint(args[0], 10, 8)
		if err != nil {
			return s.chatFeedback(c, "Invalid pet level.")
		}
		level = min(max(level, 1), game.MaxLevel)
		gained := max(int(level)-int(pet.Level), 0)
		pet.StatPoints = uint16(min(int(pet.StatPoints)+gained*game.StatPointsPerLevel, math.MaxUint16))
		pet.Level = byte(level)
		pet.Exp = 0
		pet.Normalize(s.Assets.Items, true)
	case "petexp":
		if len(args) != 1 {
			return s.chatFeedback(c, "Usage: /petexp <EXP gain>")
		}
		gain, err := strconv.ParseUint(args[0], 10, 32)
		if err != nil {
			return s.chatFeedback(c, "Invalid pet EXP gain.")
		}
		grown := s.gainPetExp(pet, uint32(gain), petGrowth)
		pet.Normalize(s.Assets.Items, grown > 0)
	}
	packets = append(packets, s.petListPacket(&next, c.pets))
	packets = append(packets, pet.ProgressionPackets(c.pets.slot(pet.ID), s.Assets.Items)...)
	if err := s.commit(ctx, c, next); err != nil {
		return err
	}
	if err := s.sendAll(c, packets); err != nil {
		return err
	}
	return s.chatFeedback(c, fmt.Sprintf("Updated pet %s (#%d).", pet.Name, pet.ID))
}

func (s *Server) gmRecruitPet(ctx context.Context, c *Session, args []string) error {
	if len(args) == 0 {
		return s.chatFeedback(c, "Usage: /pet <template ID> [name]")
	}
	id, err := strconv.ParseUint(args[0], 10, 16)
	if err != nil || id == 0 {
		return s.chatFeedback(c, "Invalid pet template ID.")
	}
	templateID := uint32(id)
	if templateID == gmRobinsonBroadcastID {
		templateID = gmRobinsonTemplateID
	}
	if _, known := s.World.Template(game.BroadcastID(templateID)); !known {
		return s.chatFeedback(c, "Unknown pet template.")
	}
	if slices.ContainsFunc(c.character.HotelPets, func(p game.Pet) bool { return game.SamePet(p.ID, templateID) }) {
		return s.chatFeedback(c, "Retrieve this companion from Pet Hotel first.")
	}
	next := c.character.Clone()
	index, owned := next.Pet(templateID)
	fresh := false
	if !owned {
		slot := next.FreePetSlot()
		if slot == 0 {
			return s.chatFeedback(c, "No free companion slot available.")
		}
		pet := game.NewPet(templateID, s.companionName(templateID, ""), slot, s.petTemplate(templateID), s.Assets.Items)
		if i := slices.IndexFunc(next.ReservePets, func(p game.Pet) bool { return game.SamePet(p.ID, templateID) }); i >= 0 {
			pet = next.ReservePets[i]
			pet.Slot = slot
			next.ReservePets = slices.Delete(next.ReservePets, i, i+1)
		} else {
			fresh = true
			switch {
			case (templateID == gmRocaTemplateID || templateID == gmRocaAlternateTemplateID) && s.hasItem(gmRocaMacheteItemID):
				pet.Equipment[2] = game.Item{ID: gmRocaMacheteItemID, Count: 1}
			case templateID == gmXaolanTemplateID && s.hasItem(gmXaolanJadeItemID):
				pet.Equipment[5] = game.Item{ID: gmXaolanJadeItemID, Count: 1}
			}
			pet.EnsureSkills(s.petTemplate(templateID), s.hasSkill)
			pet.Normalize(s.Assets.Items, true)
		}
		next.Pets = append(next.Pets, pet)
		index = len(next.Pets) - 1
	}
	if len(args) > 1 {
		name := strings.Join(args[1:], " ")
		if !validChatText(name, gmPetNameMaxBytes) {
			return s.chatFeedback(c, "Pet names must contain 1-16 printable bytes.")
		}
		next.Pets[index].Name = name
	}
	for i := range next.Pets {
		next.Pets[i].Battle = i == index
	}
	next.ActivePet = game.BroadcastID(next.Pets[index].ID)
	if err := s.commit(ctx, c, next); err != nil {
		return err
	}
	pet := c.character.Pets[index]
	c.pets.register(pet.ID)
	var packets [][]byte
	if fresh {
		packets = append(packets, pet.RecruitPacket(next.ID, s.petTemplate(pet.ID)))
	}
	packets = append(packets, s.petListPacket(c.character, c.pets), serverwire.PetNamePacket(next.ID, c.pets.slot(pet.ID), pet.Name), protocol.Builder{protocol.CommandBattlePet, protocol.BattlePetSelect}.U32(next.ActivePet))
	packets = append(packets, pet.ProgressionPackets(c.pets.slot(pet.ID), s.Assets.Items)...)
	packets = append(packets, s.World.Sync(c.character, c.view, false)...)
	s.broadcastWorld(c, serverwire.PetMapPacket(next.ID, next.ActivePet, pet.Name))
	if err := s.sendAll(c, packets); err != nil {
		return err
	}
	return s.chatFeedback(c, "Companion recruited, selected for battle, and saved.")
}

// AssetSnapshot returns a shallow, immutable snapshot for administration readers.
// Reload replaces collections rather than modifying published collection entries.
func (s *Server) AssetSnapshot() *assets.Catalog {
	s.catalogMu.RLock()
	defer s.catalogMu.RUnlock()
	snapshot := *s.Assets
	return &snapshot
}

func (s *Server) ChangeGMLevel(ctx context.Context, account uint32, level byte) error {
	s.privilegeMu.Lock()
	defer s.privilegeMu.Unlock()
	if err := s.Store.SetGMLevel(ctx, account, level); err != nil {
		return err
	}
	s.SetGMLevel(account, level)
	return nil
}

func (s *Server) gmReload(ctx context.Context, c *Session, _ string, args []string) error {
	if len(args) > 1 {
		return s.chatFeedback(c, "Usage: /reload [all|quests|mall|drops|gms]")
	}
	scope := "all"
	if len(args) == 1 {
		scope = strings.ToLower(args[0])
	}
	switch scope {
	case "quest":
		scope = "quests"
	case "itemmall", "items":
		scope = "mall"
	case "drop":
		scope = "drops"
	case "gm":
		scope = "gms"
	}
	switch scope {
	case "all", "quests", "mall", "drops", "gms":
	default:
		return s.chatFeedback(c, "Unknown reload category.")
	}
	var next *assets.Catalog
	var err error
	if scope != "gms" {
		next, err = assetsql.LoadDatabase(s.Config.AssetsDatabase)
		if err != nil {
			s.Log.Error("GM asset reload failed", "category", scope, "error", err)
			return s.chatFeedback(c, "Asset reload failed; the current catalog is unchanged.")
		}
	}
	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()
	quests := scope == "all" || scope == "quests"
	if quests {
		s.mu.Lock()
		loading := false
		for _, player := range s.sessions {
			if player.info.CharacterID != 0 && s.friendSessions[player.info.CharacterID] != player {
				loading = true
				break
			}
		}
		s.mu.Unlock()
		if loading {
			return s.chatFeedback(c, "Quest reload requires all online characters to finish loading.")
		}
		// Authored events are held by active dialogues/battles. Keep those definitions
		// stable until all interactions have ended, including clients still loading.
		for _, player := range s.friendSessions {
			if !gmIdle(player) {
				return s.chatFeedback(c, "Quest reload requires all online characters to finish loading and active interactions.")
			}
		}
		for id := range s.Assets.Maps {
			if _, ok := next.Maps[id]; !ok {
				return s.chatFeedback(c, fmt.Sprintf("Reload is missing map %d; the current catalog is unchanged.", id))
			}
		}
	}
	var levels map[uint32]byte
	if scope == "all" || scope == "gms" {
		s.privilegeMu.Lock()
		defer s.privilegeMu.Unlock()
		levels, err = s.Store.GMLevels(ctx)
		if err != nil {
			return err
		}
	}
	// Everything above validates before publishing any selected category.
	if quests {
		maps := make(map[uint16]assets.Map, len(s.Assets.Maps))
		for id, old := range s.Assets.Maps {
			updated := next.Maps[id]
			old.Events, old.PreEvents = updated.Events, updated.PreEvents
			maps[id] = old
		}
		s.Assets.Maps = maps
		s.Assets.Marks, s.Assets.DisabledEvents, s.Assets.Talks = next.Marks, next.DisabledEvents, next.Talks
		s.Assets.QuestVisibility = next.QuestVisibility
		s.Assets.QuestDefinitions = next.QuestDefinitions
		s.Assets.Story = next.Story
		s.World.ReloadStory(next.Story)
		s.Assets.CombatTrials = next.CombatTrials
		s.World.ReloadEvents(maps)
	}
	if scope == "all" || scope == "mall" {
		s.Assets.Mall = next.Mall
	}
	if scope == "all" || scope == "drops" {
		s.Assets.Drops = next.Drops
	}
	if levels != nil {
		s.mu.Lock()
		for _, player := range s.sessions {
			if player.account.ID != 0 {
				player.gmLevel.Store(uint32(levels[player.account.ID]))
			}
		}
		s.mu.Unlock()
	}
	return s.chatFeedback(c, "Reloaded "+scope+" from the configured SQL databases.")
}

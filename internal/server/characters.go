package server

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

func (s *Server) reserveName(ctx context.Context, c *Session, name string) (bool, error) {
	if game.ValidateCharacterName(name) != nil {
		return false, nil
	}
	available, e := s.Store.CharacterNameAvailable(ctx, name)
	if e != nil || !available {
		return false, e
	}
	key := strings.ToLower(name)
	s.mu.Lock()
	defer s.mu.Unlock()
	if owner, ok := s.names[key]; ok && owner != c.info.ID {
		return false, nil
	}
	if c.pendingName != "" {
		delete(s.names, strings.ToLower(c.pendingName))
	}
	s.names[key] = c.info.ID
	c.pendingName = name
	return true, nil
}
func (s *Server) releaseName(c *Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := strings.ToLower(c.pendingName)
	if owner, ok := s.names[key]; ok && owner == c.info.ID {
		delete(s.names, key)
	}
	c.pendingName = ""
}

func (s *Server) createCharacter(ctx context.Context, c *Session, p []byte) error {
	s.catalogMu.RLock()
	defer s.catalogMu.RUnlock()
	if len(p) < 2 || c.account.ID == 0 || c.slot < 1 || c.slot > 2 || c.character != nil {
		return protocol.ErrMalformed
	}
	switch p[1] {
	case protocol.CharacterCreationCreate:
		available, e := s.reserveName(ctx, c, string(p[2:]))
		if e != nil {
			return e
		}
		status := byte(1)
		if available {
			status = 0
		}
		return c.send([]byte{protocol.CommandCharacterCreation, protocol.CharacterCreationCheckName, status})
	case protocol.CharacterCreationWireCode1:
		request, e := game.DecodeCharacterCreation(p[2:])
		appearance, code := request.Appearance, request.DeletionCode
		if e != nil {
			return s.rejectCharacterCreation(c, "decode_request", e)
		}
		if e := appearance.ValidateCreationAllocation(); e != nil {
			return s.rejectCharacterCreation(c, "point_budget", e)
		}
		if request.HasConfirmationPassword {
			account, err := s.Store.Authenticate(ctx, c.account.Username, request.ConfirmationPassword)
			if err != nil || account.ID != c.account.ID {
				if err == nil {
					err = store.ErrCredentials
				}
				return s.rejectCharacterCreation(c, "confirm_password", err)
			}
		}
		if c.pendingName == "" {
			available, e := s.reserveName(ctx, c, c.account.Username)
			if e != nil {
				return e
			}
			if !available {
				return s.rejectCharacterCreation(c, "reserve_name", errors.New("character name unavailable"))
			}
		}
		if len(s.Assets.StarterItems) == 0 || len(s.Assets.Items) == 0 {
			return s.rejectCharacterCreation(c, "starter_assets", errors.New("starter items or item catalog unavailable"))
		}
		char, e := game.NewCharacter(c.account.CharacterID(c.slot), c.slot, c.pendingName, appearance, s.Assets.StarterItems, s.Assets.Items, time.Now())
		if e != nil {
			return s.rejectCharacterCreation(c, "build_character", e)
		}
		s.unlockQualifiedSkills(&char, false)
		view, pets := world.NewView(), newPetRoster()
		packets, e := s.worldEntryPackets(char, view, pets)
		if e != nil {
			return s.rejectCharacterCreation(c, "world_entry_assets", e)
		}
		if e = s.Store.CreateCharacterWithCode(ctx, c.account, char, code); e != nil {
			return s.rejectCharacterCreation(c, "database_commit", e)
		}
		s.Log.Info("character created", "account", c.account.ID, "character", char.ID, "slot", char.Slot)
		s.releaseName(c)
		if err := s.enterWorld(c, char, view, pets, packets); err != nil {
			s.Log.Warn("character created but world entry failed", "account", c.account.ID, "character", char.ID, "error", err)
			return err
		}
		return nil
	default:
		return ErrUnsupported
	}
}

// Rejections happen before persistence; never log credential payloads.
func (s *Server) rejectCharacterCreation(c *Session, stage string, err error) error {
	s.Log.Warn("character creation rejected", "session", c.info.ID, "account", c.account.ID, "slot", c.slot, "stage", stage, "error", err)
	return c.send([]byte{protocol.CommandDiscovery, protocol.DiscoveryCharacterCreationRejected})
}

func (s *Server) worldEntryPackets(char game.Character, view *world.View, pets *petRoster) ([][]byte, error) {
	if _, ok := s.Assets.Maps[char.Map]; !ok {
		return nil, errors.New("character map is missing")
	}
	appearance, e := char.AppearancePacket(false)
	if e != nil {
		return nil, e
	}
	base, e := s.nativeStatsSnapshot(char).BaseStatsPacket(func(id uint16) (uint16, bool) { skill, ok := s.Assets.Skills[id]; return skill.TableOrder, ok })
	if e != nil {
		return nil, e
	}
	packets := [][]byte{{protocol.CommandEvent, protocol.EventResume}, {protocol.CommandQuest, protocol.QuestFlag, 183, 0, 0}, {protocol.CommandQuest, protocol.QuestFlag, 53, 0, 0}, {protocol.CommandQuest, protocol.QuestFlag, 52, 0, 0}, {protocol.CommandQuest, protocol.QuestFlag, 54, 0, 0}, {protocol.CommandEvent, protocol.EventWireCode33, 0}, {protocol.CommandFriends, protocol.FriendsWireCode13, 3}, appearance, base}
	packets = append(packets, s.monsterBookPackets(&char)...)
	packets = append(packets, char.StatPackets(s.Assets.Items)...)
	packets = append(packets, char.Bag.Packet(protocol.CommandInventory, protocol.InventoryItems), char.EquipmentPacket(), protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(char.Gold), char.Preferences().Packet(), []byte{protocol.CommandTrade, protocol.TradeOnlineTime, protocol.TradeOnlineTimeEnough}, []byte{protocol.CommandCharacterState, protocol.CharacterStateRefresh})
	arrival, err := s.arrivalPackets(char, view, pets, 0)
	if err != nil {
		return nil, err
	}
	packets = append(packets, arrival...)
	packets = append(packets, []byte{protocol.CommandCharacterState, protocol.CharacterStateWireCode15, 0}, []byte{protocol.CommandWire62, protocol.Wire62WireCode53, 2, 0}, serverwire.RecordPointStatus(char), []byte{protocol.CommandCharacterState, protocol.CharacterStateWireCode14, 2})
	for slot := byte(1); slot <= 10; slot++ {
		packets = append(packets, []byte{protocol.CommandCharacterState, protocol.CharacterStateWireCode24, slot, 0, 0})
	}
	if char.Title != 0 {
		packets = append(packets, protocol.Builder{protocol.CommandTitle, protocol.TitleBroadcast}.U16(char.Title))
	}
	if char.RebornJob != 0 {
		packets = append(packets, []byte{protocol.CommandRebornJob, protocol.RebornJobMetadata, char.RebornJob, protocol.NativeActive})
	}
	packets = append(packets, s.storyConstellations(&char))
	packets = append(packets, []byte{protocol.CommandHandshake, protocol.HandshakeWorldReady}, []byte{protocol.CommandCharacterSelection, protocol.CharacterSelectionReady}, protocol.Builder{protocol.CommandCharacterSelection, protocol.CharacterSelectionRecordPoint}.U32(char.ID).U8(0))
	return packets, nil
}

func (s *Server) enterWorld(c *Session, char game.Character, view *world.View, pets *petRoster, packets [][]byte) error {
	points, err := s.Store.TeleportPoints(context.Background(), store.CharacterRef{Account: c.account.ID, ID: char.ID})
	if err != nil {
		return err
	}
	c.shapeshift = 0
	baseline := char.Clone()
	c.autosaveBaseline = &baseline
	c.view, c.pets = view, pets
	s.endEvent(c)
	c.walkMode = 0
	c.fishing = nil
	c.manufacturing = nil
	c.manufacturingLoaded = false
	c.ready = false
	c.restoreUIAfterMapLoad = false
	c.motdSent = false
	c.warped = false
	c.storm, c.stormAck, c.beachPending, c.beach, c.battle = false, false, false, nil, nil
	c.restMap, c.saleMode = 0, -1
	c.storageMap, c.hotelMap = 0, 0
	c.encounter = encounterState{next: firstEncounter}
	c.walk = playerWalk{}
	c.encounter.enterMap()
	c.slot = char.Slot
	c.arrived = true
	// Publish the character under s.mu: tentPeers inspects sessions that are
	// still loading and therefore yet to enter the world registry.
	s.mu.Lock()
	c.character = &char
	c.timedTickAt = time.Now()
	c.openTent, c.tentOwner = nil, 0
	c.tentFloor = 0
	c.info.CharacterID = char.ID
	c.info.CharacterName = char.Name
	c.info.Map = char.Map
	s.mu.Unlock()
	for _, p := range packets {
		// Mall synchronization precedes the snapshot's final world-ready markers.
		if len(p) == 2 && p[0] == protocol.CommandHandshake && p[1] == protocol.HandshakeWorldReady {
			if s.Assets.LuckyDraw.TotalWeight > 0 {
				if err := c.send(s.luckyDrawCatalogPacket(c.character, time.Now())); err != nil {
					return err
				}
			}
			if err := s.sendInitialMallSync(context.Background(), c); err != nil {
				return err
			}
		}
		if e := c.send(p); e != nil {
			return e
		}
	}
	s.Log.Info("world entry sent", s.sessionLogAttrs(c)...)
	if err := s.sendAll(c, timedPackets(c.character.TimedEffects, false)); err != nil {
		return err
	}
	for point := byte(1); point <= store.MaxTeleportPoints; point++ {
		if location, ok := points[point]; ok {
			if err := c.send(teleportPointPacket(point, location)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Server) worldCommand(ctx context.Context, c *Session, p []byte) error {
	if c.character == nil || len(p) == 0 {
		return protocol.ErrMalformed
	}
	command := commandRegistry[p[0]]
	if p[0] == protocol.CommandMapAcknowledgment {
		return command.handle(s, ctx, c, p)
	}
	if p[0] == protocol.CommandEvent {
		if len(p) < 2 {
			return protocol.ErrMalformed
		}
		if p[1] == protocol.EventPortal {
			return s.usePortal(ctx, c, p)
		}
	}
	// Published state can change from other sessions (GM summons), so check it under worldMu.
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	if c.agentRequested && (!s.agentAPIEnabled || !c.connectionNegotiated || c.connectionFeatures&protocol.ConnectionAgentAPI == 0) {
		return s.denyAgentAccess(c)
	}
	if err := c.outputError(); err != nil {
		return err
	}
	if p[0] == protocol.CommandLuckyDraw {
		s.Log.Debug("Lucky Draw dispatch state", "session", c.info.ID, "character", c.character.ID, "map", c.character.Map, "map_ready", c.ready, "warped", c.warped, "battle", c.battle != nil, "trade", c.trade != nil, "minigame", c.event != nil && c.event.onMinigame != nil)
	}
	// An open sign cannot reserve an item across arbitrary inventory changes.
	// Close it before other actions; chat and read-only login sync may continue.
	if c.stall != nil && !stallPacket(p) && p[0] != protocol.CommandChat && !command.policy.BeforeWorldGates && !(command.beforeWorldGates != nil && command.beforeWorldGates(p)) {
		s.closeStall(c)
	}
	// Settings and native login synchronization precede the interaction gates.
	if command.policy.BeforeWorldGates || (command.beforeWorldGates != nil && command.beforeWorldGates(p)) {
		return command.handle(s, ctx, c, p)
	}
	if err := s.advancePlayerWalk(c, time.Now()); err != nil {
		return err
	}
	if c.battle != nil {
		// In battle only battle commands and chat are handled (C# AC20 and others
		// return early while a battle is registered).
		if command.policy.AllowedDuringBattle {
			return command.handle(s, ctx, c, p)
		}
		if p[0] == protocol.CommandCharacterState && len(p) >= 2 && p[1] == protocol.CharacterStateTeleport {
			return c.send(serverwire.HeadBanner("Leave the battle before teleporting."))
		}
		return nil
	}
	if !c.ready {
		// Actions sent before a warp's AC12 arrived are stale; before the first map
		// acknowledgment they are invalid, except event messages, which are ignored.
		if c.warped || p[0] == protocol.CommandEvent {
			return nil
		}
		return protocol.ErrMalformed
	}
	if c.event != nil && c.event.onMinigame != nil && !command.policy.AllowedDuringMinigame {
		return nil // The minigame owns this interaction until result or cancel.
	}
	// Shared template interiors have no overworld events, shops or encounters.
	if c.tentOwner != 0 {
		switch p[0] {
		case protocol.CommandEvent, protocol.CommandNPCService, protocol.CommandShop, protocol.CommandPalaceTrial, protocol.CommandMinigame:
			return nil
		}
	}
	// Offers reserve inventories; movement and lifecycle transitions cancel them.
	if c.trade != nil && command.policy.BlockedDuringTrade {
		return c.send(serverwire.TradeMessage("Cancel the trade before changing items or interacting."))
	}
	return command.handle(s, ctx, c, p)
}

func (s *Server) deleteCharacter(ctx context.Context, c *Session, p []byte) error {
	if c.account.ID == 0 || c.character != nil || len(p) < 3 {
		return protocol.ErrMalformed
	}
	if p[1] != protocol.CharacterSelectionDelete {
		return ErrUnsupported
	}
	r := protocol.NewReader(p[2:])
	slot := r.U8()
	_ = r.String() // Reserved field in the native deletion request.
	code := r.String()
	if r.Err() != nil || r.Remaining() != 0 || slot < 1 || slot > 2 {
		return protocol.ErrMalformed
	}
	e := s.Store.DeleteCharacter(ctx, c.account, slot, code)
	if errors.Is(e, store.ErrCredentials) {
		return c.send([]byte{protocol.CommandCharacterSelection, protocol.CharacterSelectionDelete, 3, slot})
	}
	if e != nil {
		return e
	}
	s.releaseName(c)
	c.slot = 0
	for _, flag := range []byte{53, 52, 54, 183} {
		if e = c.send([]byte{protocol.CommandQuest, protocol.QuestFlag, flag, 0}); e != nil {
			return e
		}
	}
	if e = c.send([]byte{protocol.CommandEvent, protocol.EventResume}); e != nil {
		return e
	}
	return c.send([]byte{protocol.CommandCharacterSelection, protocol.CharacterSelectionDelete, 1, slot})
}

// The reference server's alternative AC63:3 creation layout shares its subcommand with the
// native two-byte cancellation. Keep cancellation intact and validate the longer
// authenticated request against the same allocation/name/ownership rules as AC9.
func (s *Server) createCompactCharacter(ctx context.Context, c *Session, p []byte) error {
	if c.account.ID == 0 || c.character != nil {
		return protocol.ErrMalformed
	}
	r := protocol.NewReader(p[2:])
	slot, name := r.U8(), r.String()
	appearance := game.Appearance{Body: uint16(r.U8()), Element: r.U8()}
	appearance.Base = game.Attributes{Strength: uint16(r.U8()), Constitution: uint16(r.U8()), Intelligence: uint16(r.U8()), Wisdom: uint16(r.U8()), Agility: uint16(r.U8())}
	if r.Err() != nil || r.Remaining() != 0 || slot < 1 || slot > 2 {
		return protocol.ErrMalformed
	}
	fail := func() error {
		return c.send([]byte{protocol.CommandHandshake, protocol.HandshakeCompactCreationResult, protocol.CompactCreationFailed})
	}
	if game.ValidateCharacterName(name) != nil || appearance.Validate() != nil || appearance.ValidateCreationAllocation() != nil {
		return fail()
	}
	s.catalogMu.RLock()
	defer s.catalogMu.RUnlock()
	if len(s.Assets.StarterItems) == 0 || len(s.Assets.Items) == 0 {
		return fail()
	}
	c.slot = slot
	available, err := s.reserveName(ctx, c, name)
	if err != nil {
		return err
	}
	if !available {
		return fail()
	}
	defer s.releaseName(c)
	char, err := game.NewCharacter(c.account.CharacterID(slot), slot, name, appearance, s.Assets.StarterItems, s.Assets.Items, time.Now())
	if err != nil {
		return fail()
	}
	s.unlockQualifiedSkills(&char, false)
	view, pets := world.NewView(), newPetRoster()
	packets, err := s.worldEntryPackets(char, view, pets)
	if err != nil {
		return fail()
	}
	if err = s.Store.CreateCharacterWithCode(ctx, c.account, char, ""); err != nil {
		return fail()
	}
	if err = c.send([]byte{protocol.CommandHandshake, protocol.HandshakeCompactCreationResult, protocol.CompactCreationSucceeded}); err != nil {
		return err
	}
	return s.enterWorld(c, char, view, pets, packets)
}

func (s *Server) grantStarterPack(ctx context.Context, ref store.CharacterRef, reservations ...game.Character) (game.Character, []game.Addition, error) {
	return s.Store.GrantStarterPack(ctx, ref, s.Assets.StarterItems, s.Assets.Items, reservations...)
}

type StarterPackDelivery struct {
	CharacterID uint32 `json:"character_id"`
	Name        string `json:"name"`
	Delivered   bool   `json:"delivered"`
	Error       string `json:"error,omitempty"`
}

const adminStarterPackBatchTimeout = 5 * time.Second

// AdminGiveStarterPacks targets loaded idle online characters. Each whole pack
// commits before delivery packets; failed or busy recipients are reported.
func (s *Server) AdminGiveStarterPacks(ctx context.Context) ([]StarterPackDelivery, error) {
	s.catalogMu.RLock()
	defer s.catalogMu.RUnlock()
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	if len(s.Assets.StarterItems) == 0 {
		return nil, fmt.Errorf("starter pack is empty")
	}
	ctx, cancel := context.WithTimeout(ctx, adminStarterPackBatchTimeout)
	defer cancel()
	players := s.gmOnline()
	sort.Slice(players, func(i, j int) bool { return players[i].character.ID < players[j].character.ID })
	result := make([]StarterPackDelivery, 0, len(players))
	for _, c := range players {
		row := StarterPackDelivery{CharacterID: c.character.ID, Name: c.character.Name}
		if !c.ready || !gmIdle(c) || c.stall != nil || c.trade != nil {
			row.Error = ErrAdminPlayerUnavailable.Error()
		} else {
			next, adds, err := s.grantStarterPack(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, *c.character)
			if err != nil {
				row.Error = err.Error()
			} else {
				s.adoptSavedCharacter(c, next)
				row.Delivered = true
				s.sendOrClose(c, next.Bag.AdditionPacket(adds))
			}
		}
		result = append(result, row)
	}
	s.Log.Info("administrator starter pack delivery", "recipients", result)
	return result, nil
}

// Native AC5:3 overlays skill records (FUN_004381c4). Zero removed grades and
// EXP in the wire snapshot only; removed records must not survive persistence.
func skillSnapshotWithRemovals(previous, next game.Character) game.Character {
	snapshot := next.Clone()
	retained := map[uint16]bool{}
	clientID := func(id uint16) uint16 {
		if id == game.StarterStunt(next.Body, next.Head) {
			return game.StarterStuntClientID
		}
		return id
	}
	for _, skill := range next.Skills {
		retained[clientID(skill.ID)] = true
	}
	for _, skill := range previous.Skills {
		id := clientID(skill.ID)
		if !retained[id] {
			snapshot.Skills = append(snapshot.Skills, game.LearnedSkill{ID: skill.ID})
			retained[id] = true
		}
	}
	return snapshot
}

// unlockQualifiedSkills applies SQL evolution definitions. The retained compiled
// source table supports synthetic/older catalogs; a populated SQL dataset owns
// evolution decisions and can disable individual links through administration.
func (s *Server) unlockQualifiedSkills(c *game.Character, evolve bool) [][]byte {
	packets := c.UnlockQualifiedSkills(evolve && s.Assets.SkillEvolutions == nil, func(id uint16) bool {
		return s.hasSkill(id) && s.Assets.Skills[id].GradeLimit() > 0
	})
	if evolve {
		for _, e := range s.Assets.SkillEvolutions {
			if !s.hasSkill(e.ToSkill) || e.RequiredGrade == 0 {
				continue
			}
			for _, sk := range c.Skills {
				if sk.ID == e.FromSkill && sk.Grade >= e.RequiredGrade {
					packets = append(packets, c.LearnSkill(e.ToSkill, s.Assets.Skills[e.ToSkill].GradeLimit())...)
					break
				}
			}
		}
	}
	if c.Reborn {
		id := game.RebornJobSkill(c.Job)
		if id != 0 && s.hasSkill(id) {
			packets = append(packets, c.LearnSkill(id, s.Assets.Skills[id].GradeLimit())...)
		}
	}
	return packets
}

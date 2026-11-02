package server

import (
	"context"
	"time"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
)

type commandHandler func(*Server, context.Context, *Session, []byte) error

type commandRegistration struct {
	handler          commandHandler
	policy           protocol.CommandPolicy
	beforeWorldGates func([]byte) bool
}

func (command commandRegistration) handle(s *Server, ctx context.Context, c *Session, p []byte) error {
	if command.handler == nil || (command.policy.RequiresBattle && c.battle == nil) {
		return ErrUnsupported
	}
	return command.handler(s, ctx, c, p)
}

func withoutContext(handler func(*Server, *Session, []byte) error) commandHandler {
	return func(s *Server, _ context.Context, c *Session, p []byte) error { return handler(s, c, p) }
}

// commandRegistry is immutable after initialization. Register each inbound command
// once, together with its handler and exceptions to the default interaction gates.
var commandRegistry = map[byte]commandRegistration{
	protocol.CommandBazaar:              {handler: (*Server).bazaarCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandTeleportScroll:      {handler: (*Server).teleportScrollCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandRebirthJobSelection: {handler: (*Server).rebirthJobCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandAppearance:          {handler: (*Server).nativeCommand, policy: protocol.CommandPolicy{World: true, BeforeWorldGates: true}},
	protocol.CommandPosition:            {handler: (*Server).waypointCommand, policy: protocol.CommandPolicy{World: true}},
	protocol.CommandGesture:             {handler: (*Server).nativeCommand, policy: protocol.CommandPolicy{World: true}},
	protocol.CommandScene:               {handler: (*Server).nativeCommand, policy: protocol.CommandPolicy{World: true}},
	protocol.CommandHeartbeatSync:       {handler: (*Server).nativeCommand, policy: protocol.CommandPolicy{World: true, BeforeWorldGates: true}},
	protocol.CommandMovie:               {handler: (*Server).nativeCommand, policy: protocol.CommandPolicy{World: true, BeforeWorldGates: true}},
	protocol.CommandMallMatrixRequest:   {handler: (*Server).nativeCommand, policy: protocol.CommandPolicy{World: true, BeforeWorldGates: true}},
	protocol.CommandNativeMall:          {handler: (*Server).nativeMallCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}, beforeWorldGates: matchingSubcommands(protocol.NativeMallWindow, protocol.NativeMallBalance)},
	protocol.CommandTitle:               {handler: (*Server).nativeCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandRebornJob:           {handler: (*Server).nativeCommand, policy: protocol.CommandPolicy{World: true}},
	protocol.CommandBath:                {handler: (*Server).nativeCommand, policy: protocol.CommandPolicy{World: true}},
	protocol.CommandFishing:             {handler: (*Server).nativeCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandTent:                {handler: (*Server).tentCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandTentFurniture:       {handler: (*Server).tentFurnitureCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandTentManufacture:     {handler: (*Server).tentManufactureCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandGemSocket:           {handler: (*Server).gemSocketCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandManufacture:         {handler: (*Server).manufactureCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandGuild:               {handler: (*Server).guildCommand, policy: protocol.CommandPolicy{World: true}},
	protocol.CommandStall:               {handler: (*Server).stallCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandSceneReady:          {handler: withoutContext((*Server).sceneReadyCommand), policy: protocol.CommandPolicy{World: true, BeforeWorldGates: true}},
	protocol.CommandSceneReadyAck:       {handler: withoutContext((*Server).sceneReadyCommand), policy: protocol.CommandPolicy{World: true, BeforeWorldGates: true}},
	protocol.CommandLuckyDraw:           {handler: (*Server).luckyDrawCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandDiscovery:           {handler: (*Server).discoveryCommand, policy: protocol.CommandPolicy{}},
	protocol.CommandHandshake:           {handler: (*Server).handshakeCommand, policy: protocol.CommandPolicy{}},
	protocol.CommandLogin:               {handler: (*Server).login, policy: protocol.CommandPolicy{}},
	protocol.CommandTeam:                {handler: (*Server).partyCommand, policy: protocol.CommandPolicy{}},
	protocol.CommandInstance:            {handler: (*Server).instanceCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandCharacterCreation:   {handler: (*Server).createCharacter, policy: protocol.CommandPolicy{}},
	protocol.CommandCharacterSelection:  {handler: (*Server).deleteCharacter, policy: protocol.CommandPolicy{CharacterDeletion: true}},
	protocol.CommandChat:                {handler: (*Server).chat, policy: protocol.CommandPolicy{World: true, AllowedDuringBattle: true, AllowedDuringMinigame: true}},
	protocol.CommandCharacterState:      {handler: (*Server).menuCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}, beforeWorldGates: matchingSubcommands(protocol.CharacterStateRefreshRequest, protocol.CharacterStateRefresh)},
	protocol.CommandMovement:            {handler: (*Server).movementCommand, policy: protocol.CommandPolicy{World: true}},
	protocol.CommandStats:               {handler: (*Server).allocateStats, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandPalaceTrial:         {handler: withoutContext((*Server).palaceTrialCommand), policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandBattleState:         {handler: withoutContext((*Server).battleStateCommand), policy: protocol.CommandPolicy{World: true, AllowedDuringBattle: true}},
	protocol.CommandMapAcknowledgment:   {handler: (*Server).mapAcknowledgmentCommand, policy: protocol.CommandPolicy{World: true}},
	protocol.CommandSocialRelations:     {handler: (*Server).socialCommand, policy: protocol.CommandPolicy{World: true}},
	protocol.CommandFriends:             {handler: (*Server).friendCommand, policy: protocol.CommandPolicy{World: true}},
	protocol.CommandPetControl:          {handler: (*Server).petControlCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandDirectSettings:      {handler: (*Server).settingsCommand, policy: protocol.CommandPolicy{World: true, BeforeWorldGates: true}},
	protocol.CommandBattlePet:           {handler: (*Server).petCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandEvent:               {handler: (*Server).eventCommand, policy: protocol.CommandPolicy{World: true, AllowedDuringMinigame: true, BlockedDuringTrade: true}},
	protocol.CommandInventory:           {handler: (*Server).itemCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}, beforeWorldGates: matchingSubcommands(protocol.InventoryStallListRequest, protocol.InventoryMallCatalog, protocol.InventoryMallBalance)},
	protocol.CommandTrade:               {handler: (*Server).tradeCommand, policy: protocol.CommandPolicy{World: true}},
	protocol.CommandShop:                {handler: (*Server).sellItems, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandStorage:             {handler: (*Server).storageCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandNPCService:          {handler: (*Server).npcServiceCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandPose:                {handler: withoutContext((*Server).emote), policy: protocol.CommandPolicy{World: true}, beforeWorldGates: matchingSubcommands(protocol.PoseStop)},
	protocol.CommandSettings:            {handler: (*Server).settingsCommand, policy: protocol.CommandPolicy{World: true, BeforeWorldGates: true}},
	protocol.CommandBattleAction:        {handler: withoutContext((*Server).battleCommand), policy: protocol.CommandPolicy{World: true, AllowedDuringBattle: true, RequiresBattle: true}},
	protocol.CommandMinigame:            {handler: withoutContext((*Server).minigameCommand), policy: protocol.CommandPolicy{World: true, AllowedDuringMinigame: true}},
	protocol.CommandArcadeGame:          {handler: (*Server).arcadeCommand, policy: protocol.CommandPolicy{World: true, AllowedDuringMinigame: true, BlockedDuringTrade: true}},
	// The fruit machines query their balance with AC34:1 while the minigame runs.
	protocol.CommandMallCheckout:    {handler: (*Server).mallCheckoutCommand, policy: protocol.CommandPolicy{World: true, AllowedDuringMinigame: true}},
	protocol.CommandMall:            {handler: (*Server).mallCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}, beforeWorldGates: matchingSubcommands(protocol.MallRefresh)},
	protocol.CommandPackContents:    {handler: withoutContext((*Server).packContentsCommand), policy: protocol.CommandPolicy{World: true}},
	protocol.CommandEquipmentRepair: {handler: (*Server).repairCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandBank:            {handler: (*Server).bankCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandAlchemy:         {handler: (*Server).hotbarOrAlchemyCommand, beforeWorldGates: isNativeHotbarAssignment, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandPetFeeding:      {handler: (*Server).petFeedingCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandPetTraining:     {handler: (*Server).petTrainingCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandPetRebirth:      {handler: (*Server).petRebirthCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
}

// These read-only native synchronization requests also run while loading or in
// an interaction. Matching a subcommand never bypasses its handler's validation.
func matchingSubcommands(subcommands ...byte) func([]byte) bool {
	return func(p []byte) bool {
		if len(p) < 2 {
			return false
		}
		for _, sub := range subcommands {
			if p[1] == sub {
				return true
			}
		}
		return false
	}
}

func (s *Server) discoveryCommand(_ context.Context, c *Session, _ []byte) error {
	if e := c.send(append([]byte{protocol.CommandHandshake, protocol.HandshakeServerDescription, 101, 0, 1}, []byte(s.Name())...)); e != nil {
		return e
	}
	return c.send([]byte{protocol.CommandDiscoveryChannels, protocol.DiscoveryChannelsWireCode201, 0, 1, 101, 0, 3, 103, 0, 2, 104, 0, 3, 102, 0, 3})
}

func (s *Server) handshakeCommand(_ context.Context, c *Session, p []byte) error {
	sub := byte(1)
	if len(p) > 1 {
		sub = p[1]
	}
	out := protocol.Builder{protocol.CommandHandshake, sub, 1}
	if sub == 1 {
		out = out.U32(uint32(time.Since(s.Started).Milliseconds()))
	}
	return c.send(out)
}

func (s *Server) mapAcknowledgmentCommand(_ context.Context, c *Session, p []byte) error {
	if len(p) != 2 || p[1] != protocol.MapAcknowledgmentMapLoaded {
		return protocol.ErrMalformed
	}
	return s.acknowledgeWorld(c)
}

func (s *Server) npcServiceCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) == 2 && p[1] == protocol.ClinicRestConfirm {
		return s.confirmRest(ctx, c)
	}
	return s.hotelCommand(ctx, c, p)
}

// nativeWord accepts the reference's optional UInt16 operand, defaulting only
// when it is entirely absent. A partial operand or trailing data is malformed.
func nativeWord(p []byte, fallback uint16) (uint16, error) {
	switch len(p) {
	case 2:
		return fallback, nil
	case 4:
		return uint16(p[2]) | uint16(p[3])<<8, nil
	default:
		return 0, protocol.ErrMalformed
	}
}

// nativeCommand ports small AC handlers. World dispatch owns worldMu and the
// normal gameplay gates; only read-only synchronization bypasses those gates.
func (s *Server) nativeCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	switch p[0] {
	case protocol.CommandAppearance:
		if len(p) != 2 {
			return protocol.ErrMalformed
		}
		packet, err := c.character.AppearancePacket(true)
		if err != nil {
			return err
		}
		return c.send(packet)
	case protocol.CommandHeartbeatSync:
		if p[1] != protocol.HeartbeatSyncRequest {
			return ErrUnsupported
		}
		if len(p) != 2 {
			return protocol.ErrMalformed
		}
		return s.sendAll(c, [][]byte{
			{protocol.CommandHeartbeatSync, protocol.HeartbeatSyncRequest, 0},
			{protocol.CommandHeartbeatSync, protocol.HeartbeatSyncStatus, protocol.HeartbeatSyncStatusWireValue9, protocol.HeartbeatSyncStatusWireValue2},
		})
	case protocol.CommandMovie:
		if p[1] != protocol.MoviePlaybackAcknowledgment {
			return ErrUnsupported
		}
		id, err := nativeWord(p, protocol.NativeDefaultMovieID)
		if err != nil {
			return err
		}
		if id == protocol.NativeDefaultMovieID {
			s.acknowledgeStorm(c)
		}
		return c.send(protocol.Builder{protocol.CommandMovie, protocol.MoviePlaybackAcknowledgment}.U16(id).U8(protocol.NativeActive).U32(0))
	case protocol.CommandMallMatrixRequest:
		if p[1] != protocol.MallMatrixQuery {
			return ErrUnsupported
		}
		if len(p) != 2 {
			return protocol.ErrMalformed
		}
		return s.sendAll(c, [][]byte{
			protocol.Builder{protocol.CommandMallMatrix, protocol.MallMatrixRows, 0, protocol.MallMatrixQuery}.U16(protocol.MallMatrixRowWireID27).U8(1).U16(protocol.MallMatrixRowWireID29).U8(2).U16(protocol.MallMatrixRowWireID24).U8(0),
			protocol.Builder{protocol.CommandMallClaimState, protocol.MallClaimSnapshot}.Bytes(make([]byte, protocol.MallClaimReservedBytes)),
		})
	case protocol.CommandScene:
		id, err := nativeWord(p, protocol.NativeDefaultEntityID)
		if err != nil {
			return err
		}
		return c.send(protocol.Builder{protocol.CommandScene, p[1]}.U16(id).U8(protocol.NativeActive))
	case protocol.CommandGesture:
		id, err := nativeWord(p, protocol.NativeDefaultGestureID)
		if err != nil {
			return err
		}
		packet := protocol.Builder{protocol.CommandGesture, p[1]}.U32(c.character.ID).U16(id)
		s.broadcastWorld(c, packet)
		return c.send(packet)
	case protocol.CommandTitle:
		if len(p) == 3 && p[1] == protocol.CompanionRebirthRequest {
			return s.companionRebirthCommand(ctx, c, p[2])
		}
		id, err := nativeWord(p, 0)
		if err != nil {
			return err
		}
		// Update only the authoritative SQL field; leave pending walking dirty.
		if err = s.Store.UpdateCharacter(ctx, c.account.ID, c.character.ID, func(stored *game.Character) error { stored.Title = id; return nil }); err != nil {
			return err
		}
		c.character.Title = id
		if c.autosaveBaseline != nil {
			c.autosaveBaseline.Title = id
		}
		if err = c.send(protocol.Builder{protocol.CommandTitle, p[1]}.U16(id)); err != nil {
			return err
		}
		packet := protocol.Builder{protocol.CommandTitle, protocol.TitleBroadcast}.U32(c.character.ID).U16(id)
		s.broadcastWorld(c, packet)
		return c.send(packet)
	case protocol.CommandRebornJob:
		if len(p) != 2 && len(p) != 3 {
			return protocol.ErrMalformed
		}
		job := byte(protocol.NativeDefaultRebornJob) // AC66's absent-operand compatibility default.
		if len(p) == 3 {
			job = p[2]
		}
		if err := s.Store.UpdateCharacter(ctx, c.account.ID, c.character.ID, func(stored *game.Character) error { stored.RebornJob = job; return nil }); err != nil {
			return err
		}
		c.character.RebornJob = job
		if c.autosaveBaseline != nil {
			c.autosaveBaseline.RebornJob = job
		}
		return c.send([]byte{protocol.CommandRebornJob, p[1], job, protocol.NativeActive})
	case protocol.CommandBath:
		if len(p) != 2 {
			return protocol.ErrMalformed
		}
		if c.event != nil {
			return nil
		}
		if err := s.heal(ctx, c, nil); err != nil {
			return err
		}
		return c.send([]byte{protocol.CommandBath, p[1], protocol.NativeActive})
	case protocol.CommandFishing:
		return s.fishingCommand(ctx, c, p)
	}
	return ErrUnsupported
}

// waypointCommand accepts the reference X/Y layout and an inferred map/X/Y
// adapter based on AC07's documented intent. Native captures are still needed. Coordinates go through AC6's terrain, interaction and vehicle checks,
// checkpointing and encounter processing instead of bypassing movement rules.
func (s *Server) waypointCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) == 2 {
		return c.send(protocol.Builder{protocol.CommandPosition, p[1]}.U16(c.character.X).U16(c.character.Y))
	}
	if len(p) != 6 && len(p) != 8 {
		return protocol.ErrMalformed
	}
	offset := 2
	if len(p) == 8 {
		mapID := uint16(p[2]) | uint16(p[3])<<8
		if mapID != c.character.Map {
			return c.send(c.character.PositionPacket())
		}
		offset += 2
	}
	r := protocol.NewReader(p[offset:])
	x, y := r.U16(), r.U16()
	if x != 0 || y != 0 {
		if err := s.movementCommand(ctx, c, protocol.Builder{protocol.CommandMovement, protocol.MovementMove, 0}.U16(x).U16(y)); err != nil {
			return err
		}
	}
	return c.send(protocol.Builder{protocol.CommandPosition, p[1]}.U16(c.character.X).U16(c.character.Y))
}

func (s *Server) nativeMallCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	switch p[1] {
	case protocol.NativeMallWindow:
		if len(p) != 2 {
			return protocol.ErrMalformed
		}
		if err := s.sendMallBalances(ctx, c); err != nil {
			return err
		}
		packet := protocol.Builder{protocol.CommandNativeMall, protocol.NativeMallWindow}
		for slot := 1; slot <= protocol.NativeMallWindowSlots; slot++ {
			packet = packet.U8(byte(slot))
		}
		return c.send(packet)
	case protocol.NativeMallBalance:
		if len(p) != 2 {
			return protocol.ErrMalformed
		}
		return s.sendMallBalances(ctx, c)
	case protocol.NativeMallBuy:
		if len(p) != 3 {
			return protocol.ErrMalformed
		}
		slot := int(p[2])
		entries := s.mallEntries(false)
		if slot == 0 || slot > len(entries) {
			return c.send(serverwire.HeadBanner("Item Mall: this slot is unavailable. No points deducted."))
		}
		entry := entries[slot-1]
		// One purchase means one catalog bundle, not Count copies of that bundle.
		return s.purchaseMall(ctx, c, []mallCartRow{{item: entry.ID, category: mallCategory(entry), quantity: 1, order: mallOrder(entry)}}, false, false)
	default:
		return ErrUnsupported
	}
}

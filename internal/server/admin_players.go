package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

const adminDefaultWarpCoordinate = 600

var ErrAdminPlayerUnavailable = errors.New("character must be online, loaded and idle")

func (s *Server) AdminPlayerAction(ctx context.Context, id uint32, command string, args []string) error {
	_, err := s.ExecuteAdminPlayerAction(ctx, id, command, args)
	return err
}
func (s *Server) ExecuteAdminPlayerAction(ctx context.Context, id uint32, command string, args []string) ([]string, error) {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	c := s.onlineByID(id)
	if c == nil {
		return nil, ErrAdminPlayerUnavailable
	}
	messages := []string{}
	c.adminFeedback = &messages
	defer func() { c.adminFeedback = nil }()
	err := s.adminPlayerActionLocked(ctx, id, command, args)
	return messages, err
}
func (s *Server) adminPlayerActionLocked(ctx context.Context, id uint32, command string, args []string) error {
	c := s.onlineByID(id)
	if c == nil || !c.ready {
		return ErrAdminPlayerUnavailable
	}
	command = strings.ToLower(command)
	if spec, ok := gmCommandRegistry[command]; ok {
		if spec.idle && !gmIdle(c) {
			return ErrAdminPlayerUnavailable
		}
		return spec.handle(s, ctx, c, command, args)
	}
	words := append([]string{"/" + command}, args...)
	switch command {
	case "actor":
		if !gmIdle(c) || len(args) != 2 {
			return ErrAdminPlayerUnavailable
		}
		click, err := parseAdminUint16(args[0])
		if err != nil {
			return err
		}
		m, ok := s.World.Map(c.character.Map)
		if !ok {
			return errors.New("map unavailable")
		}
		if _, ok := m.NPC(click); !ok {
			return errors.New("unknown map actor")
		}
		switch args[1] {
		case "hide", "show":
			if c.view.AdminActors == nil {
				c.view.AdminActors = map[uint16]bool{}
			}
			shown := args[1] == "show"
			c.view.AdminActors[click] = shown
			if shown {
				return c.send(s.World.ShowActor(c.view, c.character.Map, click))
			}
			return c.send(s.World.HideActor(c.view, c.character.Map, click))
		case "open", "close":
			state := int32(0)
			if args[1] == "open" {
				state = 1
			}
			c.view.Props[click] = state
			return c.send(protocol.Builder{protocol.CommandScene, protocol.SceneActorState}.U16(click).U8(byte(state)))
		default:
			return errors.New("unknown actor action")
		}
	case "fullheal":
		if !gmIdle(c) || len(args) != 0 {
			return ErrAdminPlayerUnavailable
		}
		next := c.character.Clone()
		next.Refill(s.Assets.Items)
		packets := next.StatPackets(s.Assets.Items)
		if i := gmSelectedPet(&next); i >= 0 {
			next.Pets[i].Normalize(s.Assets.Items, true)
			packets = append(packets, next.Pets[i].ProgressionPackets(c.pets.slot(next.Pets[i].ID), s.Assets.Items)...)
		}
		if err := s.commit(ctx, c, next); err != nil {
			return err
		}
		return s.sendAll(c, packets)
	case "heal", "hp", "full":
		return s.heal(ctx, c, words)
	case "level", "lvl", "points", "sp", "statpoint", "statpoints", "stats", "stat", "exp", "skill":
		if !gmIdle(c) {
			return ErrAdminPlayerUnavailable
		}
		return s.gmProgress(ctx, c, command, words)
	case "restat", "resetstats":
		return s.gmRestat(ctx, c, args)
	case "clearskills", "resetskills":
		return s.gmClearSkills(ctx, c, args)
	case "repair", "fixall":
		return s.gmRepair(ctx, c, args)
	case "item":
		if !gmIdle(c) {
			return ErrAdminPlayerUnavailable
		}
		if len(args) < 1 || len(args) > 2 {
			return errors.New("item requires an ID and optional quantity")
		}
		id, err := parseAdminUint16(args[0])
		if err != nil || !s.hasItem(id) {
			return errors.New("unknown item ID")
		}
		count := uint16(1)
		if len(args) == 2 {
			count, err = parseAdminUint16(args[1])
			if err != nil || count == 0 {
				return errors.New("invalid item quantity")
			}
		}
		limit, _ := s.stackLimit(id)
		next := c.character.Clone()
		adds, err := next.Bag.Grant(game.Item{ID: id}, int(count), limit, s.Assets.Items)
		if err != nil {
			return err
		}
		if err = s.commit(ctx, c, next); err != nil {
			return err
		}
		return c.send(next.Bag.AdditionPacket(adds))
	case "warp", "goto", "tp":
		if !gmIdle(c) {
			return ErrAdminPlayerUnavailable
		}
		if len(args) == 1 {
			for _, target := range s.gmOnline() {
				if strings.EqualFold(target.character.Name, args[0]) || fmt.Sprint(target.character.ID) == args[0] {
					return s.commandTeleport(ctx, c, world.Destination{Map: target.character.Map, X: target.character.X, Y: target.character.Y})
				}
			}
			mapID, err := parseAdminUint16(args[0])
			if err != nil {
				return errors.New("unknown online character or map")
			}
			if _, ok := s.World.Map(mapID); !ok {
				return errors.New("unknown map")
			}
			return s.commandTeleport(ctx, c, world.Destination{Map: mapID, X: adminDefaultWarpCoordinate, Y: adminDefaultWarpCoordinate})
		}
		if len(args) != 3 {
			return errors.New("warp requires a target, or map/X/Y")
		}
		values := [3]uint16{}
		for i, arg := range args {
			value, err := strconv.ParseUint(arg, 10, 16)
			if err != nil {
				return err
			}
			values[i] = uint16(value)
		}
		if _, ok := s.World.Map(values[0]); !ok {
			return errors.New("unknown map")
		}
		return s.commandTeleport(ctx, c, world.Destination{Map: values[0], X: values[1], Y: values[2]})
	case "town":
		if !gmIdle(c) {
			return ErrAdminPlayerUnavailable
		}
		if len(args) != 1 {
			return errors.New("town requires an alias")
		}
		if _, ok := townDestinations[strings.ToLower(args[0])]; !ok {
			return errors.New("unknown town alias")
		}
		return s.gmTown(ctx, c, words)
	case "summon", "bring":
		if !gmIdle(c) || len(args) != 1 {
			return ErrAdminPlayerUnavailable
		}
		target := s.findOnline(args[0])
		if target == nil || !gmIdle(target) {
			return ErrAdminPlayerUnavailable
		}
		return s.commandTeleport(ctx, target, world.Destination{Map: c.character.Map, X: c.character.X, Y: c.character.Y})
	case "mallpoints", "im", "points_im":
		return s.gmMallPoints(ctx, c, words)
	case "unride", "dismount":
		if !gmIdle(c) {
			return ErrAdminPlayerUnavailable
		}
		return s.publicDismount(ctx, c)
	case "summonall":
		if !gmIdle(c) {
			return ErrAdminPlayerUnavailable
		}
		return s.summonAll(ctx, c)
	case "kick":
		s.gmKick(c, strings.Join(args, " "))
		return nil
	case "gold":
		if !gmIdle(c) || len(args) != 1 {
			return ErrAdminPlayerUnavailable
		}
		gain, err := parseAdminGold(args[0])
		if err != nil {
			return err
		}
		next := c.character.Clone()
		if uint64(next.Gold)+gain > game.MaxGold {
			return errors.New("gold limit exceeded")
		}
		next.Gold += uint32(gain)
		if err = s.commit(ctx, c, next); err != nil {
			return err
		}
		return c.send(protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(next.Gold))
	case "deleteitem":
		if !gmIdle(c) || len(args) != 1 {
			return ErrAdminPlayerUnavailable
		}
		slot, err := parseAdminSlot(args[0])
		if err != nil {
			return err
		}
		next := c.character.Clone()
		item := next.Bag[slot-1]
		if item.Empty() {
			return errors.New("bag slot is empty")
		}
		next.Bag[slot-1] = game.Item{}
		if err = s.commit(ctx, c, next); err != nil {
			return err
		}
		return c.send([]byte{protocol.CommandInventory, protocol.InventoryRemove, slot, item.Count})
	case "dialogue":
		if !gmIdle(c) {
			return ErrAdminPlayerUnavailable
		}
		text := strings.ReplaceAll(strings.Join(args, " "), "#n", c.character.Name)
		if err := store.ValidateMailContent([]byte(text)); err != nil {
			return err
		}
		packet, err := protocol.Builder{protocol.CommandInventory, protocol.InventoryMessage, 0}.String(text)
		if err != nil {
			return err
		}
		return c.send(packet)
	case "event":
		if !gmIdle(c) || len(args) != 1 {
			return ErrAdminPlayerUnavailable
		}
		click, err := parseAdminUint16(args[0])
		if err != nil {
			return err
		}
		return s.npcClick(ctx, c, protocol.Builder{}.U16(click))
	case "clearground":
		if !gmIdle(c) {
			return ErrAdminPlayerUnavailable
		}
		for _, packet := range s.World.ClearDropped(c.character.Map) {
			s.broadcastMap(c.character.Map, packet)
		}
		return nil
	}
	return errors.New("unknown player action")
}

func (s *Server) EditAdminCharacter(ctx context.Context, id uint32, version string, next game.Character) error {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()
	// Match reload's world-before-catalog lock order. A write lock also
	// excludes character selection/creation while checking loading ownership.
	if _, ok := s.Assets.Maps[next.Map]; !ok {
		return errors.New("unknown destination map")
	}
	if err := s.validateAdminState(next); err != nil {
		return err
	}
	return s.editAdminCharacterValidated(ctx, id, version, next)
}
func (s *Server) editAdminCharacterValidated(ctx context.Context, id uint32, version string, next game.Character) error {
	if s.adminCharacterLoading(id) {
		return ErrAdminPlayerUnavailable
	}
	c := s.friendSessions[id]
	if c != nil && (!gmIdle(c) || c.openTent != nil || c.tentOwner != 0) {
		return ErrAdminPlayerUnavailable
	}
	if err := s.Store.ReplaceAdminCharacter(ctx, id, version, next); err != nil {
		return err
	}
	if c != nil {
		*c.character = next.Clone()
		c.autosaveBaseline = nil
		c.conn.Close()
	}
	return nil
}
func (s *Server) DeleteAdminCharacter(ctx context.Context, id uint32, version string) error {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()
	if s.adminCharacterLoading(id) {
		return ErrAdminPlayerUnavailable
	}
	c := s.friendSessions[id]
	if c != nil && (!gmIdle(c) || c.openTent != nil || c.tentOwner != 0) {
		return ErrAdminPlayerUnavailable
	}
	friends, err := s.Store.Friends(ctx, id)
	if err != nil {
		return err
	}
	if err = s.Store.DeleteAdminCharacter(ctx, id, version); err != nil {
		return err
	}
	if c != nil {
		c.autosaveBaseline = nil
		c.conn.Close()
	}
	for _, f := range friends {
		if peer := s.friendPlayer(f.ID); peer != nil {
			s.sendOrClose(peer, protocol.Builder{protocol.CommandFriends, protocol.FriendsRemove}.U32(id))
		}
	}
	return nil
}
func (s *Server) AdminRemoveFriend(ctx context.Context, a, b uint32) error {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	removed, err := s.Store.RemoveFriend(ctx, a, b)
	if err != nil {
		return err
	}
	if !removed {
		return errors.New("friendship not found")
	}
	for owner, other := range map[uint32]uint32{a: b, b: a} {
		if c := s.friendPlayer(owner); c != nil {
			s.sendOrClose(c, protocol.Builder{protocol.CommandFriends, protocol.FriendsRemove}.U32(other))
		}
	}
	return nil
}
func (s *Server) AdminSaveAll(ctx context.Context) error {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	for _, c := range s.friendSessions {
		if err := s.autosaveSession(ctx, c); err != nil {
			return err
		}
	}
	return nil
}
func (s *Server) AdminBroadcast(text string) error {
	return s.AdminAnnouncement(text, "notice")
}
func (s *Server) AdminScheduleShutdown(seconds int) error {
	if seconds < 1 || seconds > gmShutdownMaximumSeconds {
		return errors.New("shutdown delay must be 1–300 seconds")
	}
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	if !s.shutdownAt.IsZero() {
		return errors.New("shutdown already scheduled")
	}
	s.shutdownAt = time.Now().Add(time.Duration(seconds) * time.Second)
	s.notice(fmt.Sprintf("Server shutdown in %d seconds.", seconds))
	return nil
}
func (s *Server) AdminWarp(ctx context.Context, id uint32, destination world.Destination) error {
	return s.AdminPlayerAction(ctx, id, "warp", []string{fmt.Sprint(destination.Map), fmt.Sprint(destination.X), fmt.Sprint(destination.Y)})
}

// EditAdminCharacterFields only changes the selected tab's owned fields. It
// preserves unrelated SQL state and pending session walking and publishes after commit.
func (s *Server) EditAdminCharacterFields(ctx context.Context, id uint32, version string, requested game.Character, scope string) error {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()
	if s.adminCharacterLoading(id) {
		return ErrAdminPlayerUnavailable
	}
	c := s.friendSessions[id]
	if c != nil && (!gmIdle(c) || c.stall != nil || c.openTent != nil || c.tentOwner != 0) {
		return ErrAdminPlayerUnavailable
	}
	row, err := s.Store.AdminCharacter(ctx, id)
	if err != nil {
		return err
	}
	if row.Version != version || version == "" {
		return store.ErrAdminConflict
	}
	next := row.State.Clone()
	switch scope {
	case "inventory":
		next.Bag, next.Equipment, next.Storage = requested.Bag, requested.Equipment, requested.Storage
		next.RecalculateVitals(s.Assets.Items)
	case "stats":
		next.Base, next.Skills, next.Level, next.EXP, next.StatPoints = requested.Base, requested.Skills, requested.Level, requested.EXP, requested.StatPoints
		next.RecalculateVitals(s.Assets.Items)
	case "player_settings":
		next.Settings = requested.Clone().Settings
	case "quests":
		next.Quests = requested.Clone().Quests
		for id, q := range next.Quests {
			if id == 0 || q.ID != id || q.State > game.Failed || q.Step < 0 || q.Step > assets.MaxQuestSteps || q.Kills < 0 || q.Kills > math.MaxInt32 {
				return errors.New("invalid quest progress")
			}
		}
	default:
		return errors.New("unsupported live character tab")
	}
	if err = s.validateAdminState(next); err != nil {
		return err
	}
	var packets [][]byte
	switch scope {
	case "inventory":
		// Native AC23:5 overlays slots; remove old contents before publishing the replacement.
		if c != nil {
			for i, item := range c.character.Bag {
				if !item.Empty() {
					packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryRemove, byte(i + 1), item.Count})
				}
			}
		}
		packets = append(packets, next.Bag.Packet(protocol.CommandInventory, protocol.InventoryItems), next.EquipmentPacket())
		packets = append(packets, next.StatPackets(s.Assets.Items)...)
	case "stats":
		snapshot := skillSnapshotWithRemovals(row.State, next)
		base, err := s.nativeStatsSnapshot(snapshot).BaseStatsPacket(func(id uint16) (uint16, bool) { skill, ok := s.Assets.Skills[id]; return skill.TableOrder, ok })
		if err != nil {
			return err
		}
		packets = append(packets, base)
		packets = append(packets, next.StatPackets(s.Assets.Items)...)
	case "player_settings":
		packets = append(packets, next.Preferences().Packet())
	}
	if err = s.Store.ReplaceAdminCharacter(ctx, id, version, next); err != nil {
		return err
	}
	if c == nil {
		return nil
	}
	s.adoptSavedCharacter(c, next)
	if scope == "quests" {
		packets = append(packets, s.World.Journal(c.character, c.view)...)
		packets = append(packets, storyConstellations(c.character))
		packets = append(packets, s.World.Sync(c.character, c.view, false)...)
	}
	packets = append(packets, []byte{protocol.CommandCharacterState, protocol.CharacterStateRefresh})
	if err := s.sendAll(c, packets); err != nil {
		c.conn.Close()
	}
	if scope == "inventory" {
		s.broadcastWorld(c, protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateEquipmentSnapshot}.U32(id).Bytes(next.WornEquipment()))
	}
	if c.party != nil {
		s.partyUpdate(c.party)
	}
	return nil
}

type AdminBattle struct {
	PvP        bool              `json:"pvp"`
	LeaderID   uint32            `json:"leader_id"`
	Turn       int               `json:"turn"`
	Processing bool              `json:"processing"`
	Players    []string          `json:"players"`
	Attackers  []*battle.Fighter `json:"attackers"`
	Defenders  []*battle.Fighter `json:"defenders"`
}

func (s *Server) AdminBattles() []AdminBattle {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	out := []AdminBattle{}
	seen := map[*battleRun]bool{}
	for _, c := range s.friendSessions {
		run := c.battle
		if run == nil || run.b == nil || seen[run] {
			continue
		}
		seen[run] = true
		row := AdminBattle{PvP: run.b.PvP, LeaderID: run.members[0].c.character.ID, Turn: run.b.Turn, Processing: run.b.Processing, Players: []string{}}
		for _, m := range run.members {
			row.Players = append(row.Players, m.c.character.Name)
		}
		clone := func(fighters []*battle.Fighter) []*battle.Fighter {
			out := []*battle.Fighter{}
			for _, f := range fighters {
				copy := *f
				copy.Char = nil
				copy.Pet = nil
				copy.Effects = append([]battle.ActiveEffect{}, f.Effects...)
				out = append(out, &copy)
			}
			return out
		}
		row.Attackers, row.Defenders = clone(run.b.Attackers), clone(run.b.Defenders)
		out = append(out, row)
	}
	return out
}
func (s *Server) AdminAbortBattle(id uint32) error {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	c := s.friendSessions[id]
	if c == nil || c.battle == nil {
		return errors.New("battle not found")
	}
	run := c.battle
	if run.b.Processing {
		return errors.New("wait for battle animation")
	}
	s.endBattle(run, battle.Fled)
	return nil
}
func (s *Server) AdminReload(ctx context.Context, scope string) error {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	conn := &adminMessageConn{}
	c := &Session{conn: conn}
	if err := s.gmReload(ctx, c, "reload", []string{scope}); err != nil {
		return err
	}
	if !conn.ok {
		return errors.New(conn.message)
	}
	return nil
}

// Server operations have no player socket; capture native notice replies without
// fabricating an online GM or changing an account's privileges.
type adminMessageConn struct {
	net.Conn
	ok      bool
	message string
}

func (c *adminMessageConn) SetWriteDeadline(_ time.Time) error { return nil }
func (c *adminMessageConn) Write(raw []byte) (int, error) {
	reader := bytes.NewReader(raw)
	packet, err := protocol.Read(reader)
	if err != nil {
		return 0, err
	}
	if len(packet) > 2 {
		c.message = string(packet[2:])
		c.ok = strings.HasPrefix(c.message, "Reloaded ")
	}
	return len(raw), nil
}

func (s *Server) LoadIPBans(ctx context.Context) error {
	rows, err := s.Store.IPBans(ctx)
	if err != nil {
		return err
	}
	bans := map[string]bool{}
	for _, row := range rows {
		bans[row.IP] = true
	}
	s.bannedIPs.Store(bans)
	return nil
}
func (s *Server) AdminSetIPBan(ctx context.Context, ip, reason string, banned bool) error {
	s.ipBanMu.Lock()
	defer s.ipBanMu.Unlock()
	if err := s.Store.SetIPBan(ctx, ip, reason, banned); err != nil {
		return err
	}
	address, _ := netip.ParseAddr(ip)
	ip = address.Unmap().String()
	existing, _ := s.bannedIPs.Load().(map[string]bool)
	next := make(map[string]bool, len(existing)+1)
	for address, value := range existing {
		next[address] = value
	}
	if banned {
		next[ip] = true
	} else {
		delete(next, ip)
	}
	s.bannedIPs.Store(next)
	if banned {
		s.mu.Lock()
		for _, c := range s.sessions {
			host, _, _ := net.SplitHostPort(c.info.Remote)
			address, err := netip.ParseAddr(host)
			if err == nil && address.Unmap().String() == ip {
				c.conn.Close()
			}
		}
		s.mu.Unlock()
	}
	return nil
}
func (s *Server) ipBanned(address net.Addr) bool {
	host, _, err := net.SplitHostPort(address.String())
	if err != nil {
		return false
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	bans, _ := s.bannedIPs.Load().(map[string]bool)
	return bans[ip.Unmap().String()]
}
func (s *Server) AdminDispatchMail(ctx context.Context, receivers []uint32, message store.AdminMail) error {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	if message.ItemID != 0 && !s.hasItem(message.ItemID) {
		return errors.New("unknown gift item")
	}
	if err := s.Store.SendAdminMail(ctx, receivers, message); err != nil {
		return err
	}
	for _, id := range receivers {
		if c := s.friendPlayer(id); c != nil && gmIdle(c) {
			if err := s.deliverAdminMail(ctx, c); err != nil {
				s.Log.Warn("GM mail delivery deferred", "character", id, "error", err)
			}
		}
	}
	return nil
}
func (s *Server) deliverAdminMail(ctx context.Context, c *Session) error {
	rows, err := s.Store.PendingAdminMail(ctx, c.character.ID)
	if err != nil {
		return err
	}
	if len(rows) > 0 {
		if err := s.autosaveSession(ctx, c); err != nil {
			return err
		}
	}
	for _, message := range rows {
		limit := byte(1)
		if message.ItemID != 0 {
			var ok bool
			limit, ok = s.stackLimit(message.ItemID)
			if !ok {
				continue
			}
		}
		next, adds, err := s.Store.ClaimAdminMail(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, message.ID, limit, s.Assets.Items, c.character.Clone())
		if err != nil {
			s.Log.Warn("GM gift retained for later claim", "character", c.character.ID, "mail", message.ID, "error", err)
			continue
		}
		s.adoptSavedCharacter(c, next)
		var packets [][]byte
		if len(adds) > 0 {
			packets = append(packets, next.Bag.AdditionPacket(adds))
		}
		if message.Gold > 0 {
			packets = append(packets, protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(next.Gold))
		}
		text := message.Subject + "\n" + message.Body
		packet, err := serverwire.TextMailPacket(store.TextMail{Content: []byte(text), SentAtMillis: message.At})
		if err != nil {
			return err
		}
		packets = append(packets, packet)
		if err = s.sendAll(c, packets); err != nil {
			c.conn.Close()
			return err
		}
		if err = s.Store.MarkAdminMailDelivered(ctx, c.character.ID, message.ID); err != nil {
			return err
		}
	}
	return nil
}
func (s *Server) AdminSummonMarriage(ctx context.Context, id uint32) error {
	rows, err := s.Store.AdminMarriages(ctx)
	if err != nil {
		return err
	}
	for _, m := range rows {
		if m.ID == id {
			return s.AdminPlayerAction(ctx, m.Character2, "warp", []string{strconv.FormatUint(uint64(m.Character1), 10)})
		}
	}
	return fmt.Errorf("marriage not found")
}

func parseAdminGold(value string) (uint64, error) { return strconv.ParseUint(value, 10, 32) }
func parseAdminUint16(value string) (uint16, error) {
	n, err := strconv.ParseUint(value, 10, 16)
	return uint16(n), err
}
func parseAdminSlot(value string) (byte, error) {
	n, err := strconv.ParseUint(value, 10, 8)
	if err != nil || n < 1 || n > game.BagSize {
		return 0, errors.New("invalid bag slot")
	}
	return byte(n), nil
}

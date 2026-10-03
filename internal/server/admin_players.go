package server

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/store"
	"wonderland-go/internal/world"
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
		adds, err := next.Bag.Grant(game.Item{ID: id}, int(count), limit)
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
	if c != nil && !gmIdle(c) {
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
	if c != nil && !gmIdle(c) {
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
	if !validChatText(text, chatCommandMaxBytes) {
		return errors.New("invalid announcement")
	}
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	s.notice(text)
	return nil
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

package server

import (
	"context"
	"encoding/binary"
	"sort"
	"strconv"
	"strings"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/world"
)

// chat handles AC2. Reference: AC02.Recv1/Recv2. The caller holds worldMu.
func (s *Server) chat(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	if c.character == nil || !c.ready {
		return nil
	}
	text := string(p[2:])
	switch p[1] {
	case protocol.ChatWorldMessage:
		return s.worldChat(c, text, false)
	case protocol.ChatMapMessage:
		if strings.HasPrefix(text, ":") || strings.HasPrefix(text, "/") {
			if !validChatText(text, chatCommandMaxBytes) {
				return protocol.ErrMalformed
			}
			if handled, err := s.chatChannelCommand(c, text); handled {
				return err
			}
			return s.command(ctx, c, text)
		}
		if !validChatText(text, chatMessageMaxBytes) {
			return s.chatFeedback(c, "Chat messages must contain 1–60 bytes without control characters.")
		}
		s.deliverChat(c, s.peers(c), game.ChatChannelLocal, chatPacket(protocol.ChatMapMessage, c, p[2:]), false)
		return nil
	case protocol.ChatWhisperMessage:
		// The client names the target by ID before the text.
		if len(p) < 2+4 {
			return protocol.ErrMalformed
		}
		target := s.onlineByID(binary.LittleEndian.Uint32(p[2:]))
		return s.whisper(c, target, string(p[6:]))
	case protocol.ChatTeamMessage:
		return s.teamChat(c, text)
	case protocol.ChatGuildMessage:
		// Guilds are not ported; the client refuses the channel without one.
		return nil
	}
	return ErrUnsupported
}

// command runs a chat command. GM rights come only from an administrator-granted account level;
// the C# default names and gm_list.txt are deliberately not trusted.
func (s *Server) command(ctx context.Context, c *Session, text string) error {
	words := strings.Split(text, " ")
	name := strings.ToLower(words[0])
	switch name[1:] {
	case "unride", "dismount", "carnie":
		if !commandTravelAvailable(c) || c.trade != nil {
			return nil
		}
		if name[1:] == "carnie" {
			return s.visitCarnie(ctx, c)
		}
		return s.publicDismount(ctx, c)
	}
	if c.gmLevel.Load() == 0 {
		return nil
	}
	s.Log.Info("GM command", "account", c.account.Username, "character", c.character.Name, "command", text)
	switch name[1:] {
	case "town", "summonall", "warp", "goto", "tp", "summon", "bring":
		if !commandTravelAvailable(c) {
			return nil
		}
	}
	switch name[1:] {
	case "level", "lvl", "points", "sp", "statpoint", "statpoints", "stats", "stat", "exp", "skill":
		return s.gmProgress(ctx, c, name[1:], words)
	case "restat", "resetstats":
		return s.gmRestat(ctx, c, words[1:])
	case "repair", "fixall":
		return s.gmRepair(ctx, c, words[1:])
	case "droprate":
		return s.gmDropRate(c, words[1:])
	case "heal", "hp", "full":
		return s.heal(ctx, c, words)
	case "gold", "money":
		if len(words) < 2 {
			return nil
		}
		amount, e := strconv.ParseInt(words[1], 10, 32)
		if e != nil {
			return nil
		}
		gold := uint32(max(0, amount))
		if e = s.Store.UpdateCharacter(ctx, c.account.ID, c.character.ID, func(char *game.Character) error { char.Gold = gold; return nil }); e != nil {
			return e
		}
		s.cancelTrade(c)
		c.character.Gold = gold
		return c.send(protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(gold))
	case "im", "points_im", "mallpoints":
		return s.gmMallPoints(ctx, c, words)
	case "item":
		return s.giveItem(ctx, c, words)
	case "town":
		return s.gmTown(ctx, c, words)
	case "summonall":
		return s.summonAll(ctx, c)
	case "warp", "goto", "tp":
		return s.gmWarp(ctx, c, words)
	case "summon", "bring":
		if len(words) < 2 {
			return nil
		}
		if target := s.findOnline(words[1]); target != nil && commandTravelAvailable(target) {
			return s.commandTeleport(ctx, target, world.Destination{Map: c.character.Map, X: c.character.X, Y: c.character.Y})
		}
	case "kick":
		if len(words) < 2 {
			return nil
		}
		if target := s.findOnline(words[1]); target != nil {
			s.Log.Info("GM kick", "target", target.character.Name)
			target.conn.Close()
		}
	case "b", "broadcast", "notice":
		if len(words) >= 2 {
			s.notice(strings.TrimSpace(text[len(words[0]):]))
		}
	default:
		s.Log.Debug("GM command not ported", "command", name)
	}
	return nil
}

// findOnline is GmManager.FindOnlinePlayer over published characters: character ID,
// then exact name, then the first name containing the query. Caller holds worldMu.
func (s *Server) findOnline(query string) *Session {
	query = strings.TrimSpace(query)
	online := make([]*Session, 0, len(s.world))
	for _, c := range s.world {
		online = append(online, c)
	}
	sort.Slice(online, func(i, j int) bool { return online[i].info.ID < online[j].info.ID })
	if id, e := strconv.ParseUint(query, 10, 32); e == nil {
		for _, c := range online {
			if c.character.ID == uint32(id) {
				return c
			}
		}
	}
	for _, c := range online {
		if strings.EqualFold(c.character.Name, query) {
			return c
		}
	}
	lower := strings.ToLower(query)
	for _, c := range online {
		if strings.Contains(strings.ToLower(c.character.Name), lower) {
			return c
		}
	}
	return nil
}

// notice is GmManager.BroadcastNotice: AC2:4 from sender 0 to every character in game.
func (s *Server) notice(text string) {
	if text == "" {
		return
	}
	packet := protocol.Builder{protocol.CommandChat, protocol.ChatGMMessage}.U32(0).Bytes([]byte(text))
	s.mu.Lock()
	recipients := make([]*Session, 0, len(s.sessions))
	for _, c := range s.sessions {
		if c.info.CharacterID != 0 {
			recipients = append(recipients, c)
		}
	}
	s.mu.Unlock()
	for _, c := range recipients {
		if c.send(packet) != nil {
			c.conn.Close()
		}
	}
}

// commandTeleport is GameMap.Teleport(CmD). Caller holds worldMu.
func (s *Server) commandTeleport(ctx context.Context, c *Session, dst world.Destination) error {
	if _, ok := s.World.Map(dst.Map); !ok {
		return nil
	}
	if e := s.teleportPrelude(c); e != nil {
		return e
	}
	return s.teleport(ctx, c, dst, 0)
}

func (s *Server) gmWarp(ctx context.Context, c *Session, words []string) error {
	if len(words) == 2 {
		if m, e := strconv.ParseUint(words[1], 10, 16); e == nil {
			return s.commandTeleport(ctx, c, world.Destination{Map: uint16(m), X: 600, Y: 600})
		}
		if target := s.findOnline(words[1]); target != nil {
			t := target.character
			return s.commandTeleport(ctx, c, world.Destination{Map: t.Map, X: t.X, Y: t.Y})
		}
		return nil
	}
	if len(words) >= 4 {
		m, e1 := strconv.ParseUint(words[1], 10, 16)
		x, e2 := strconv.ParseUint(words[2], 10, 16)
		y, e3 := strconv.ParseUint(words[3], 10, 16)
		if e1 == nil && e2 == nil && e3 == nil {
			return s.commandTeleport(ctx, c, world.Destination{Map: uint16(m), X: uint16(x), Y: uint16(y)})
		}
	}
	return nil
}

// heal restores HP/SP, optionally to given values, capped at the equipped maxima.
func (s *Server) heal(ctx context.Context, c *Session, words []string) error {
	next := *c.character
	full := next.Combat(s.Assets.Items)
	maxHP, maxSP := uint32(max(full.MaxHP, 1)), uint32(max(full.MaxSP, 0))
	hp, sp := maxHP, maxSP
	if len(words) >= 2 {
		if v, e := strconv.Atoi(words[1]); e == nil {
			hp = uint32(min(max(v, 0), int(maxHP)))
			if len(words) >= 3 {
				if v, e := strconv.Atoi(words[2]); e == nil {
					sp = uint32(min(max(v, 0), int(maxSP)))
				}
			}
		}
	}
	next.MaxHP, next.MaxSP, next.HP, next.SP = maxHP, maxSP, hp, sp
	base, e := next.BaseStatsPacket(func(id uint16) (uint16, bool) { skill, ok := s.Assets.Skills[id]; return skill.TableOrder, ok })
	if e != nil {
		return nil
	}
	if e = s.Store.UpdateCharacter(ctx, c.account.ID, next.ID, func(char *game.Character) error {
		char.MaxHP, char.MaxSP, char.HP, char.SP = maxHP, maxSP, hp, sp
		return nil
	}); e != nil {
		return e
	}
	*c.character = next
	for _, packet := range append(next.StatPackets(s.Assets.Items), base) {
		if e = c.send(packet); e != nil {
			return e
		}
	}
	return nil
}

// radioSet may be held only once.
const radioSet = 34076

// giveItem is ":item <id> [count]" or ":item add <id> [count]".
func (s *Server) giveItem(ctx context.Context, c *Session, words []string) error {
	args := words[1:]
	if len(args) > 0 && strings.EqualFold(args[0], "add") {
		args = args[1:]
	}
	if len(args) == 0 {
		return nil
	}
	id, e := strconv.ParseUint(args[0], 10, 16)
	if e != nil || id == 0 {
		return nil
	}
	count := uint64(1)
	if len(args) > 1 {
		if n, e := strconv.ParseUint(args[1], 10, 8); e == nil {
			count = max(n, 1)
		}
	}
	def, ok := s.Assets.Items[uint16(id)]
	if !ok {
		return nil
	}
	if id == radioSet {
		for _, item := range c.character.Bag {
			if item.ID == radioSet {
				return nil
			}
		}
		for _, item := range c.character.Equipment {
			if item.ID == radioSet {
				return nil
			}
		}
	}
	bag := c.character.Bag
	adds, e := bag.Grant(game.Item{ID: uint16(id)}, int(count), def.StackLimit())
	if e != nil {
		return nil
	}
	if e = s.saveBag(ctx, c, bag); e != nil {
		return e
	}
	return c.send(bag.AdditionPacket(adds))
}

// emote handles AC32. Reference: AC32.Recv1-3. Only a changed pose is broadcast, and the
// sender does not receive its own. The caller holds worldMu.
func (s *Server) emote(c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	sub, pose := p[1], byte(0)
	switch sub {
	case protocol.PoseSet:
		if len(p) < 3 {
			return nil
		}
		pose = p[2]
	case protocol.PoseBroadcast:
		if len(p) > 2 {
			pose = p[2]
		}
	case protocol.PoseStop:
		// Stopping a pose is reported as AC32:2 with pose zero.
		sub = protocol.PoseBroadcast
	default:
		return ErrUnsupported
	}
	if c.emote == pose {
		return nil
	}
	c.emote = pose
	s.broadcastWorld(c, protocol.Builder{protocol.CommandPose, sub}.U32(c.character.ID).U8(pose))
	return nil
}

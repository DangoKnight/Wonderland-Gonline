package server

import (
	"context"
	"fmt"
	"time"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

const guildInvitationTTL = time.Minute

type guildInvitation struct {
	From    *Session
	GuildID uint32
	At      time.Time
}

func guildBadge(character uint32, guild *store.GuildInfo) []byte {
	icon, name := uint32(0), ""
	if guild != nil {
		icon, name = guild.Icon, guild.Name
	}
	p, _ := protocol.Builder{protocol.CommandGuild, protocol.GuildBadge}.U32(character).U32(icon).String(name)
	return p
}
func (s *Server) guildRosterPacket(guild *store.GuildInfo) ([]byte, error) {
	leader := ""
	for _, member := range guild.Roster {
		if member.Character.ID == guild.LeaderID {
			leader = member.Character.Name
		}
	}
	p, err := protocol.Builder{protocol.CommandGuild, protocol.GuildRoster}.String(guild.Name)
	if err != nil {
		return nil, err
	}
	p = p.U32(guild.Icon)
	p, err = p.String(leader)
	if err != nil {
		return nil, err
	}
	if len(guild.Roster) > store.GuildMemberLimit {
		return nil, fmt.Errorf("guild exceeds native member limit")
	}
	p = p.U8(byte(len(guild.Roster)))
	for _, member := range guild.Roster {
		char := member.Character
		p = p.U32(char.ID)
		p, err = p.String(char.Name)
		if err != nil {
			return nil, err
		}
		online := byte(0)
		if s.onlineByID(char.ID) != nil {
			online = 1
		}
		p = p.U8(byte(char.Level)).U8(char.Job).U8(byte(char.Element)).U8(member.Rank).U8(online)
	}
	return p.String(guild.Notice)
}
func (s *Server) syncGuild(ctx context.Context, c *Session) error {
	guild, err := s.Store.GuildForCharacter(ctx, c.character.ID)
	if err != nil {
		return err
	}
	if guild == nil {
		return nil
	}
	roster, err := s.guildRosterPacket(guild)
	if err != nil {
		return err
	}
	if err = s.sendAll(c, [][]byte{{protocol.CommandGuild, protocol.GuildJournal, 0}, roster}); err != nil {
		return err
	}
	badge := guildBadge(c.character.ID, guild)
	s.broadcastWorld(c, badge)
	return c.send(badge)
}
func (s *Server) refreshGuilds(ctx context.Context) error {
	// Membership remains SQL-authoritative, including edits through Admin.
	for _, peer := range s.world {
		if !peer.ready {
			continue
		}
		if err := s.syncGuild(ctx, peer); err != nil {
			return err
		}
	}
	return nil
}
func (s *Server) guildChat(ctx context.Context, c *Session, text string) error {
	if !validChatText(text, chatMessageMaxBytes) {
		return s.chatFeedback(c, "Guild messages must contain 1–60 bytes.")
	}
	if time.Now().Before(c.character.MutedUntil) {
		return s.chatFeedback(c, "Your chat privileges are temporarily muted.")
	}
	guild, err := s.Store.GuildForCharacter(ctx, c.character.ID)
	if err != nil {
		return err
	}
	if guild == nil {
		return s.chatFeedback(c, "You are not in a guild.")
	}
	var recipients []*Session
	for _, id := range guild.Members {
		if peer := s.onlineByID(id); peer != nil {
			recipients = append(recipients, peer)
		}
	}
	s.deliverChat(c, recipients, game.ChatChannelGuild, chatPacket(protocol.ChatGuildMessage, c, []byte(text)), true)
	return nil
}
func (s *Server) guildCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < protocol.GuildHeaderRequestBytes {
		return protocol.ErrMalformed
	}
	r := protocol.NewReader(p[2:])
	switch p[1] {
	case protocol.GuildJournal:
		if len(p) != protocol.GuildHeaderRequestBytes {
			return protocol.ErrMalformed
		}
		return s.sendAll(c, s.World.Journal(c.character, c.view))
	case protocol.GuildInvite:
		if len(p) != protocol.GuildTargetRequestBytes {
			return protocol.ErrMalformed
		}
		target := s.mapPlayer(c, r.U32())
		if target == nil || target == c || !tradeNear(c, target) {
			return nil
		}
		guild, err := s.Store.GuildForCharacter(ctx, c.character.ID)
		if err != nil {
			return err
		}
		if guild == nil {
			return s.chatFeedback(c, "You are not in a guild.")
		}
		permitted := guild.LeaderID == c.character.ID
		for _, member := range guild.Roster {
			if member.Character.ID == c.character.ID && member.Rank == store.GuildRankViceLeader {
				permitted = true
			}
		}
		if !permitted {
			return s.chatFeedback(c, store.ErrGuildPermission.Error())
		}
		own, err := s.Store.GuildForCharacter(ctx, target.character.ID)
		if err != nil {
			return err
		}
		if own != nil {
			return s.chatFeedback(c, "That character already belongs to a guild.")
		}
		if target.guildInvitation != nil && time.Since(target.guildInvitation.At) < guildInvitationTTL {
			return s.chatFeedback(c, "That character already has a pending guild invitation.")
		}
		target.guildInvitation = &guildInvitation{From: c, GuildID: guild.ID, At: time.Now()}
		return target.send(protocol.Builder{protocol.CommandGuild, protocol.GuildAccept}.U32(c.character.ID))
	case protocol.GuildAccept:
		if len(p) != protocol.GuildTargetRequestBytes {
			return protocol.ErrMalformed
		}
		id := r.U32()
		request := c.guildInvitation
		if request == nil || request.From.character.ID != id {
			return nil
		}
		c.guildInvitation = nil
		from := request.From
		if time.Since(request.At) > guildInvitationTTL || s.world[from.info.ID] != from || !from.ready || !tradeNear(c, from) {
			return s.chatFeedback(c, "Guild invitation expired.")
		}
		guild, err := s.Store.GuildForCharacter(ctx, from.character.ID)
		if err != nil {
			return err
		}
		if guild == nil || guild.ID != request.GuildID {
			return s.chatFeedback(c, "Guild invitation is no longer valid.")
		}
		if err = s.Store.JoinGuild(ctx, id, c.character.ID); err != nil {
			return s.chatFeedback(c, err.Error())
		}
		return s.refreshGuilds(ctx)
	case protocol.GuildLeave, protocol.GuildDismiss:
		target := c.character.ID
		if p[1] == protocol.GuildDismiss {
			if len(p) != protocol.GuildTargetRequestBytes {
				return protocol.ErrMalformed
			}
			target = r.U32()
		} else if len(p) != protocol.GuildHeaderRequestBytes {
			return protocol.ErrMalformed
		}
		if err := s.Store.LeaveGuild(ctx, c.character.ID, target); err != nil {
			return s.chatFeedback(c, err.Error())
		}
		if peer := s.onlineByID(target); peer != nil {
			badge := guildBadge(target, nil)
			s.broadcastWorld(peer, badge)
			s.sendOrClose(peer, []byte{protocol.CommandGuild, protocol.GuildJournal, 0}, badge)
		}
		return s.refreshGuilds(ctx)
	case protocol.GuildMessage:
		text := r.String()
		if r.Err() != nil || r.Remaining() != 0 {
			return protocol.ErrMalformed
		}
		return s.guildChat(ctx, c, text)
	case protocol.GuildNotice:
		// Original UnpackStringN consumes the remaining bytes without a length prefix.
		text := string(r.Rest())
		if !validChatText(text, store.GuildNoticeMaxBytes) && text != "" {
			return protocol.ErrMalformed
		}
		if err := s.Store.EditGuild(ctx, c.character.ID, &text, nil, 0, nil); err != nil {
			return s.chatFeedback(c, err.Error())
		}
		return s.refreshGuilds(ctx)
	case protocol.GuildDemote, protocol.GuildPromote, protocol.GuildPermission:
		expected := protocol.GuildTargetRequestBytes
		if p[1] == protocol.GuildPermission {
			expected = protocol.GuildPermissionRequestBytes
		}
		if len(p) != expected {
			return protocol.ErrMalformed
		}
		target := r.U32()
		rank := store.GuildRankMember
		if p[1] == protocol.GuildPromote {
			rank = store.GuildRankViceLeader
		}
		if p[1] == protocol.GuildPermission {
			rank = r.U8()
		}
		if err := s.Store.EditGuild(ctx, c.character.ID, nil, nil, target, &rank); err != nil {
			return s.chatFeedback(c, err.Error())
		}
		return s.refreshGuilds(ctx)
	case protocol.GuildMemberList:
		if len(p) != protocol.GuildHeaderRequestBytes {
			return protocol.ErrMalformed
		}
		guild, err := s.Store.GuildForCharacter(ctx, c.character.ID)
		if err != nil {
			return err
		}
		if guild == nil {
			return c.send([]byte{protocol.CommandGuild, protocol.GuildMemberList, 0})
		}
		return s.syncGuild(ctx, c)
	case protocol.GuildInsignia:
		if len(p) != protocol.GuildTargetRequestBytes {
			return protocol.ErrMalformed
		}
		icon := r.U32()
		if err := s.Store.EditGuild(ctx, c.character.ID, nil, &icon, 0, nil); err != nil {
			return s.chatFeedback(c, err.Error())
		}
		return s.refreshGuilds(ctx)
	default:
		return ErrUnsupported
	}
}

// Administration follows the same world ownership as native guild commands.
// Successful database edits refresh badges/rosters of all affected online players.
func (s *Server) AdminSaveGuild(ctx context.Context, guild store.AdminGuild) error {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	if err := s.Store.SaveAdminGuild(ctx, guild); err != nil {
		return err
	}
	return s.refreshGuildAdministration(ctx)
}
func (s *Server) AdminDeleteGuild(ctx context.Context, id uint32) error {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	if err := s.Store.DeleteAdminGuild(ctx, id); err != nil {
		return err
	}
	return s.refreshGuildAdministration(ctx)
}
func (s *Server) refreshGuildAdministration(ctx context.Context) error {
	for _, peer := range s.world {
		if !peer.ready {
			continue
		}
		guild, err := s.Store.GuildForCharacter(ctx, peer.character.ID)
		if err != nil {
			return err
		}
		if guild == nil {
			badge := guildBadge(peer.character.ID, nil)
			s.sendOrClose(peer, []byte{protocol.CommandGuild, protocol.GuildJournal, 0}, badge)
			s.broadcastWorld(peer, badge)
		} else if err = s.syncGuild(ctx, peer); err != nil {
			return err
		}
	}
	return nil
}
func (s *Server) guildPresence(ctx context.Context, id uint32) error {
	guild, err := s.Store.GuildForCharacter(ctx, id)
	if err != nil || guild == nil {
		return err
	}
	p, err := s.guildRosterPacket(guild)
	if err != nil {
		return err
	}
	for _, member := range guild.Members {
		if peer := s.onlineByID(member); peer != nil {
			s.sendOrClose(peer, p)
		}
	}
	return nil
}

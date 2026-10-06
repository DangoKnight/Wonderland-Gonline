package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

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
		packet, err := textMailPacket(store.TextMail{Content: []byte(text), SentAtMillis: message.At})
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

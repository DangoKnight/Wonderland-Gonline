package server

import (
	"context"
	"time"

	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
)

// party is a team of up to four characters; members[0] leads. Membership is
// guarded by worldMu. Reference: Player.cs (Team region) and AC13.
type party struct{ members []*Session }

const (
	partyMax        = 4
	partyRequestTTL = time.Minute
)

func (p *party) leader() *Session { return p.members[0] }

func (p *party) index(c *Session) int {
	for i, m := range p.members {
		if m == c {
			return i
		}
	}
	return -1
}

// party handles AC13. Reference: AC13.ProcessPkt.
func (s *Server) partyCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	r := protocol.NewReader(p[2:])
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	if p[1] == protocol.TeamMallRefresh {
		if len(p) != 2 || c.character == nil {
			return protocol.ErrMalformed
		}
		if err := c.send(protocol.Builder{protocol.CommandTeam, protocol.TeamMallConfirmed}.U32(c.character.ID)); err != nil {
			return err
		}
		if err := s.sendMallBalances(ctx, c); err != nil {
			return err
		}
		return s.sendAll(c, [][]byte{s.mallCatalogPacket(false), s.mallCatalogPacket(true)})
	}
	if !c.ready {
		return nil
	}
	switch p[1] {
	case protocol.TeamRequest:
		target := r.U32()
		if r.Err() != nil {
			return r.Err()
		}
		return s.partyRequest(c, target)
	case protocol.TeamReply:
		reply, requester := r.U8(), r.U32()
		if r.Err() != nil {
			return r.Err()
		}
		if reply == 1 || reply == 3 {
			return s.partyAccept(c, requester)
		}
		delete(c.partyRequests, requester)
		return nil
	case protocol.TeamLeave:
		s.partyLeave(c, true)
		return nil
	case protocol.TeamKick:
		target := r.U32()
		if r.Err() != nil {
			return r.Err()
		}
		if c.party != nil && c.party.leader() == c {
			if m := c.party.member(target); m != nil && m != c {
				s.partyLeave(m, true)
			}
		}
		return nil
	case protocol.TeamTransferLeadership:
		target := r.U32()
		if r.Err() != nil {
			return r.Err()
		}
		return s.partyTransfer(c, target)
	}
	return ErrUnsupported
}

// mapPlayer finds a published character on c's map. The C# handlers also accept
// the ID shifted right by a byte.
func (s *Server) mapPlayer(c *Session, id uint32) *Session {
	var shifted *Session
	for _, peer := range s.world {
		if !s.samePlayerScene(peer, c) {
			continue
		}
		if peer.character.ID == id {
			return peer
		}
		if peer.character.ID == id>>8 {
			shifted = peer
		}
	}
	return shifted
}

func (p *party) member(id uint32) *Session {
	var shifted *Session
	for _, m := range p.members {
		if m.character.ID == id {
			return m
		}
		if m.character.ID == id>>8 {
			shifted = m
		}
	}
	return shifted
}

// partyRequest is AC13 Recv1: ask the target to take the requester into its team.
// A requester already in a team must leave it first (C# would overwrite it).
func (s *Server) partyRequest(c *Session, id uint32) error {
	target := s.mapPlayer(c, id)
	if target == nil || target == c || target.character.Preferences().PartyInvitesBlocked || (c.party != nil && len(c.party.members) > 1) {
		return nil
	}
	packet, err := protocol.Builder{protocol.CommandTeam, protocol.TeamRequest}.U32(c.character.ID).String(c.character.Name)
	if err != nil {
		return err
	}
	if target.partyRequests == nil {
		target.partyRequests = map[uint32]time.Time{}
	}
	target.partyRequests[c.character.ID] = time.Now()
	if target.send(packet) != nil {
		target.conn.Close()
	}
	return nil
}

// partyAccept is AC13 Recv3: the requester joins c's team. Unlike C#, a reply is
// only honoured for a pending request, and only a leader or a solo player accepts.
func (s *Server) partyAccept(c *Session, id uint32) error {
	requester := s.mapPlayer(c, id)
	if requester == nil {
		return nil
	}
	at, ok := c.partyRequests[requester.character.ID]
	delete(c.partyRequests, requester.character.ID)
	if !ok || time.Since(at) > partyRequestTTL || requester == c || c.character.Preferences().PartyInvitesBlocked {
		return nil
	}
	if requester.party != nil && len(requester.party.members) > 1 {
		return nil
	}
	if c.party == nil {
		c.party = &party{members: []*Session{c}}
	}
	if c.party.leader() != c || len(c.party.members) >= partyMax {
		return nil
	}
	c.party.members = append(c.party.members, requester)
	requester.party = c.party
	// Player.JoinParty: the follow formation goes to the leader's whole map.
	follow := protocol.Builder{protocol.CommandTeam, protocol.TeamFormation}.U32(c.character.ID).U32(requester.character.ID)
	s.sendMap(c.character.Map, follow, nil)
	for _, m := range []*Session{c, requester} {
		s.sendOrClose(m, m.character.StatPackets(s.Assets.Items)...)
	}
	s.partyUpdate(c.party)
	return nil
}

// partyLeave is Player.LeaveParty. notify is false when the character is gone.
func (s *Server) partyLeave(c *Session, notify bool) {
	p := c.party
	if p == nil {
		return
	}
	c.party = nil
	if c.view != nil {
		c.view.Team.Store(0)
	}
	if i := p.index(c); i >= 0 {
		p.members = append(p.members[:i:i], p.members[i+1:]...)
	}
	if notify {
		// The leaver's HUD resets to a team of one.
		s.sendOrClose(c, protocol.Builder{protocol.CommandTeam, protocol.TeamRoster}.U32(c.character.ID).U8(0))
		s.sendMap(c.character.Map, protocol.Builder{protocol.CommandTeam, protocol.TeamLeave}.U32(c.character.ID), nil)
	} else {
		s.sendMap(c.character.Map, protocol.Builder{protocol.CommandTeam, protocol.TeamLeave}.U32(c.character.ID), c)
	}
	s.partyUpdate(p)
	if len(p.members) == 1 {
		p.members[0].party = nil
		if v := p.members[0].view; v != nil {
			v.Team.Store(0)
		}
	}
}

// partyTransfer is Player.TransferLeadership.
func (s *Server) partyTransfer(c *Session, id uint32) error {
	p := c.party
	if p == nil || p.leader() != c {
		return nil
	}
	next := p.member(id)
	if next == nil || next == c {
		return nil
	}
	i := p.index(next)
	p.members = append([]*Session{next}, append(p.members[:i:i], p.members[i+1:]...)...)
	s.partyUpdate(p)
	return nil
}

// partyUpdate is BroadcastPartyUpdate: every member receives the roster (AC13:6)
// and each other member's vitals (AC8:3).
func (s *Server) partyUpdate(p *party) {
	if len(p.members) == 0 {
		return
	}
	roster := protocol.Builder{protocol.CommandTeam, protocol.TeamRoster}.U32(p.leader().character.ID).U8(byte(len(p.members) - 1))
	for _, m := range p.members[1:] {
		roster = roster.U32(m.character.ID)
	}
	for _, m := range p.members {
		if m.view != nil {
			m.view.Team.Store(int32(len(p.members) - 1))
		}
		packets := [][]byte{roster}
		for _, other := range p.members {
			if other != m {
				packets = append(packets, s.teammateStats(other)...)
			}
		}
		s.sendOrClose(m, packets...)
		m.partyVitals = s.vitals(m)
	}
}

// vitals are the values SendTeammateStats reports, in its order.
func (s *Server) vitals(c *Session) [7]int64 {
	char := c.character
	b := char.Equipment.Bonuses(s.Assets.Items)
	return [7]int64{int64(char.Level), int64(char.MaxHP), int64(char.MaxSP), int64(char.HP), int64(char.SP), int64(b.HP), int64(b.SP)}
}

var vitalStats = [7]uint16{0x011d, 0x0119, 0x011a, 0x0123, 0x0124, 0x01cf, 0x01d0}

func (s *Server) teammateStats(c *Session) [][]byte {
	v := s.vitals(c)
	out := make([][]byte, len(v))
	for i := range v {
		out[i] = protocol.Builder{protocol.CommandStats, protocol.StatsWireCode3}.U32(c.character.ID).U16(vitalStats[i]).U64(uint64(v[i]))
	}
	return out
}

// partySync reports c's changed vitals to its team. C# rebroadcasts the whole
// team whenever Send8_1 runs; reporting changes after each command sends the
// same values without depending on every call site.
func (s *Server) partySync(c *Session) {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	if c.party == nil || c.character == nil {
		return
	}
	if v := s.vitals(c); v != c.partyVitals {
		c.partyVitals = v
		packets := s.teammateStats(c)
		for _, m := range c.party.members {
			if m != c {
				s.sendOrClose(m, packets...)
			}
		}
	}
}

// partyArrival is Map.Warp_In's party sync: the formation is replayed to the map,
// except the leader, and the team is refreshed. Caller holds worldMu.
func (s *Server) partyArrival(c *Session) {
	p := c.party
	if p == nil {
		return
	}
	leader := p.leader()
	for _, m := range p.members {
		if m == leader || !s.samePlayerScene(leader, m) || !m.ready || !leader.ready || (c != leader && m != c) {
			continue
		}
		s.sendMap(leader.character.Map, protocol.Builder{protocol.CommandTeam, protocol.TeamFormation}.U32(leader.character.ID).U32(m.character.ID), leader)
	}
	s.partyUpdate(p)
}

// partyFollow is Teleport(Regular)'s party follow: members on the leader's former
// map are brought to its destination (C# brings members from any map). Caller
// holds worldMu.
func (s *Server) partyFollow(ctx context.Context, c *Session, from uint16) error {
	p := c.party
	if p == nil || p.leader() != c {
		return nil
	}
	dst := world.Destination{Map: c.character.Map, X: c.character.X, Y: c.character.Y}
	for _, m := range append([]*Session(nil), p.members[1:]...) {
		if m.character.Map != from || m.tentOwner != 0 || m.battle != nil {
			continue
		}
		if err := s.commandTeleport(ctx, m, dst); err != nil {
			m.conn.Close()
		}
	}
	return nil
}

// sendMap sends to every published character on a map except skip. Caller holds worldMu.
func (s *Server) sendMap(mapID uint16, packet []byte, skip *Session) {
	if skip == nil && s.World.HideOtherPlayers(mapID) {
		return
	}
	for _, peer := range s.world {
		if peer != skip && peer.character.Map == mapID && (skip == nil && peer.tentOwner == 0 || skip != nil && s.samePlayerScene(peer, skip)) {
			s.sendOrClose(peer, packet)
		}
	}
}

func (s *Server) sendOrClose(c *Session, packets ...[]byte) {
	for _, packet := range packets {
		if c.send(packet) != nil {
			c.conn.Close()
			return
		}
	}
}

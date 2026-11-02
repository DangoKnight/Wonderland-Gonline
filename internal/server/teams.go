package server

import (
	"context"
	"sort"
	"strings"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
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
	// Detach using the old formation before replacing the leaver's roster.
	// Native FUN_00406848 needs the previous leader/member links to clear
	// the other visible actors' follow state.
	leave := protocol.Builder{protocol.CommandTeam, protocol.TeamLeave}.U32(c.character.ID)
	if notify {
		s.sendOrClose(c, leave)
	}
	s.sendMap(c.character.Map, leave, c)
	if notify {
		s.partyReset(c)
	}
	if len(p.members) == 1 {
		remaining := p.members[0]
		remaining.party = nil
		if v := remaining.view; v != nil {
			v.Team.Store(0)
		}
		s.partyReset(remaining)
		return
	}
	s.partyUpdate(p)
}

// partyReset clears the native party role as well as the HUD. FUN_00407ccc
// marks even a zero-member roster as a leader; FUN_00406848 must run after it.
// The self roster also makes the reset work when the old leader is off-map.
func (s *Server) partyReset(c *Session) {
	s.sendOrClose(c,
		protocol.Builder{protocol.CommandTeam, protocol.TeamRoster}.U32(c.character.ID).U8(0),
		protocol.Builder{protocol.CommandTeam, protocol.TeamLeave}.U32(c.character.ID),
	)
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
func (s *Server) vitals(c *Session) [8]int64 {
	char := c.character
	hpBonus, spBonus := char.NativeVitalBonuses(s.Assets.Items)
	a := char.Attributes()
	return [8]int64{int64(char.Level), int64(a.Constitution), int64(a.Wisdom), int64(char.HP), int64(char.SP), int64(hpBonus), int64(spBonus), int64(char.RebornByte())}
}

// FUN_00451718 reads a stat byte, sign byte, value U32 and target U32.
// FUN_0043b7bc stores CON/WIS so native Formula.dat can derive the maxima.
var vitalStats = [8]byte{game.StatLevel, game.StatCON, game.StatWIS, game.StatCurrentHP, game.StatCurrentSP, game.StatHPBonus, game.StatSPBonus, game.StatRebirth}

func (s *Server) teammateStats(c *Session) [][]byte {
	v := s.vitals(c)
	out := make([][]byte, len(v))
	for i := range v {
		sign, value := byte(protocol.StatValuePositive), v[i]
		if value < 0 {
			sign = protocol.StatValueNegative
			value = -value
		}
		out[i] = protocol.Builder{protocol.CommandStats, protocol.StatsWireCode3}.U32(c.character.ID).U8(vitalStats[i]).U8(sign).U32(uint32(value)).U32(0)
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

// Lobby membership is transient, guarded by worldMu. Until dungeon objectives
// are verified, starting never warps characters or grants rewards.
type instanceRoom struct {
	ID         uint16
	Definition assets.InstanceDefinition
	Name       string
	Members    []*Session
}

func (s *Server) instanceOf(c *Session) *instanceRoom {
	for _, r := range s.instanceRooms {
		for _, m := range r.Members {
			if m == c {
				return r
			}
		}
	}
	return nil
}
func (s *Server) instanceLeave(c *Session) {
	room := s.instanceOf(c)
	if room == nil {
		return
	}
	for i, m := range room.Members {
		if m == c {
			room.Members = append(room.Members[:i], room.Members[i+1:]...)
			break
		}
	}
	if len(room.Members) == 0 {
		delete(s.instanceRooms, room.ID)
	} else {
		s.instancePublish(room)
	}
}
func (s *Server) instancePublish(room *instanceRoom) {
	p := protocol.Builder{protocol.CommandInstance, protocol.InstanceRoomSnapshot}.U16(room.ID).U16(room.Definition.ID).U8(byte(len(room.Members)))
	for _, m := range room.Members {
		p = p.U32(m.character.ID).U8(m.character.Level)
		p, _ = p.String(m.character.Name)
	}
	for _, m := range room.Members {
		native := protocol.Builder{protocol.CommandInstance, protocol.InstanceMembership, 1, 1}.U16(room.ID).U8(byte(len(room.Members)))
		native, _ = native.String(room.Members[0].character.Name)
		native = native.U32(room.Members[0].character.ID)
		s.sendOrClose(m, native, p)
	}
}
func (s *Server) instanceCommand(_ context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	status := func(code byte) error { return c.send([]byte{protocol.CommandInstance, protocol.InstanceStatus, code}) }
	r := protocol.NewReader(p[2:])
	switch p[1] {
	case protocol.InstanceBrowse:
		page := byte(1)
		if r.Remaining() > 0 {
			page = r.U8()
			if page == 0 {
				page = 1
			}
		}
		if r.Err() != nil || r.Remaining() != 0 {
			return protocol.ErrMalformed
		}
		defs := protocol.Builder{protocol.CommandInstance, protocol.InstanceDefinitions}.U8(byte(len(s.Assets.Instances)))
		for _, d := range s.Assets.Instances {
			defs = defs.U16(d.ID).U8(protocol.InstanceDefinitionAvailable)
		}
		if err := c.send(defs); err != nil {
			return err
		}
		ids := make([]int, 0, len(s.instanceRooms))
		for id := range s.instanceRooms {
			ids = append(ids, int(id))
		}
		sort.Ints(ids)
		pages := (len(ids) + protocol.InstancePageSize - 1) / protocol.InstancePageSize
		if pages == 0 {
			page = 0
		} else if int(page) > pages {
			page = byte(pages)
		}
		start := 0
		if page > 0 {
			start = (int(page) - 1) * protocol.InstancePageSize
		}
		end := min(start+protocol.InstancePageSize, len(ids))
		out := protocol.Builder{protocol.CommandInstance, protocol.InstanceBrowse, byte(pages), page, byte(end - start)}
		for _, id := range ids[start:end] {
			room := s.instanceRooms[uint16(id)]
			out = out.U16(room.ID)
			out, _ = out.String(room.Members[0].character.Name)
			out, _ = out.String(room.Name)
			out = out.U8(byte(len(room.Members))).U8(0)
		}
		if err := c.send(out); err != nil {
			return err
		}
		if room := s.instanceOf(c); room != nil {
			s.instancePublish(room)
		}
		return nil
	case protocol.InstanceCreate:
		definition := r.U16()
		name := r.String()
		if r.Err() != nil || r.Remaining() != 0 || len(name) > protocol.InstanceNameLimit || strings.ContainsAny(name, "\x00\r\n") {
			return protocol.ErrMalformed
		}
		if s.instanceOf(c) != nil {
			return status(protocol.InstanceRejected)
		}
		index := -1
		var def assets.InstanceDefinition
		for i, d := range s.Assets.Instances {
			if d.ID == definition {
				index = i
				def = d
				break
			}
		}
		if index < 0 {
			return status(protocol.InstanceUnavailable)
		}
		if def.GuildOnly {
			return status(protocol.InstanceUnavailable)
		}
		if uint16(c.character.Level) < def.MinimumLevel {
			return status(protocol.InstanceLevelTooLow)
		}
		if s.instanceRooms == nil {
			s.instanceRooms = map[uint16]*instanceRoom{}
		}
		for i := 0; i < protocol.InstanceRoomsPerDefinition; i++ {
			id := uint16(protocol.InstanceAliasFirst + index*protocol.InstanceRoomsPerDefinition + i)
			if s.instanceRooms[id] != nil {
				continue
			}
			if name == "" {
				name = def.Name
			}
			room := &instanceRoom{id, def, name, []*Session{c}}
			s.instanceRooms[id] = room
			s.instancePublish(room)
			return s.instanceCommand(context.Background(), c, []byte{protocol.CommandInstance, protocol.InstanceBrowse})
		}
		return status(protocol.InstanceRoomsFull)
	case protocol.InstanceDetails, protocol.InstanceJoinRoom:
		id := r.U16()
		if r.Err() != nil || r.Remaining() != 0 {
			return protocol.ErrMalformed
		}
		room := s.instanceRooms[id]
		if room == nil {
			return status(protocol.InstanceUnavailable)
		}
		if p[1] == protocol.InstanceJoinRoom {
			if s.instanceOf(c) != nil {
				return status(protocol.InstanceRejected)
			}
			if room.Definition.GuildOnly {
				return status(protocol.InstanceUnavailable)
			}
			if uint16(c.character.Level) < room.Definition.MinimumLevel {
				return status(protocol.InstanceLevelTooLow)
			}
			if len(room.Members) >= int(room.Definition.Capacity) {
				return status(protocol.InstanceRoomsFull)
			}
			room.Members = append(room.Members, c)
			s.instancePublish(room)
			return nil
		}
		out := protocol.Builder{protocol.CommandInstance, protocol.InstanceDetails}.U16(id).U8(byte(len(room.Members)))
		for _, m := range room.Members {
			out, _ = out.String(m.character.Name)
			out = out.U32(m.character.ID)
		}
		return c.send(out)
	case protocol.InstanceLeaveRoom:
		if r.Remaining() != 0 {
			return protocol.ErrMalformed
		}
		s.instanceLeave(c)
		if err := c.send([]byte{protocol.CommandInstance, protocol.InstanceMembership, 2}); err != nil {
			return err
		}
		return c.send(protocol.Builder{protocol.CommandInstance, protocol.InstanceRoomSnapshot}.U16(0).U16(0).U8(0))
	case protocol.InstanceStartRoom:
		if r.Remaining() != 0 {
			return protocol.ErrMalformed
		}
		room := s.instanceOf(c)
		if room == nil || room.Members[0] != c {
			return status(protocol.InstanceRejected)
		}
		return status(protocol.InstanceUnavailable)
	default:
		return status(protocol.InstanceUnavailable)
	}
}

package server

import (
	"bytes"
	"context"
	"time"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
)

const afkStateInterval = time.Second

type afkOutputFilter struct {
	self  uint32
	party [protocol.TeamMaximumMembers]uint32
	count int
}

// Own the world lock here instead of bypassing gameplay gates for other commands.
func (s *Server) afkCommand(_ context.Context, c *Session, p []byte) error {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	if err := c.outputError(); err != nil {
		return err
	}
	if (len(p) != protocol.AFKModePacketBytes && len(p) != protocol.AFKGroupedModePacketBytes) || p[1] != protocol.AFKSet || p[2] > protocol.AFKActive ||
		!c.connectionNegotiated || c.connectionFeatures&protocol.ConnectionAFK == 0 || c.character == nil {
		return protocol.ErrMalformed
	}
	var token [protocol.AFKGroupTokenBytes]byte
	if len(p) == protocol.AFKGroupedModePacketBytes {
		if c.connectionFeatures&protocol.ConnectionAFKAggregation == 0 || p[2] != protocol.AFKActive {
			return protocol.ErrMalformed
		}
		copy(token[:], p[protocol.AFKModePacketBytes:])
		if token == [protocol.AFKGroupTokenBytes]byte{} {
			return protocol.ErrMalformed
		}
	}
	want := p[2] == protocol.AFKActive
	if want && (!c.ready || c.event != nil || c.arcade != nil || c.trade != nil || c.stall != nil || c.storageMap != 0 || c.hotelMap != 0 || c.restMap != 0 || c.saleMap != 0) {
		return c.send([]byte{protocol.CommandAFK, protocol.AFKRejected})
	}
	if c.afk == want {
		if want && token != c.afkGroupToken {
			return protocol.ErrMalformed
		}
		return c.send([]byte{protocol.CommandAFK, protocol.AFKSet, p[2]})
	}
	c.afk = want
	c.afkGroupToken, c.afkStream = token, nil
	c.afkLastState, c.afkNextState = nil, time.Time{}
	if want {
		s.refreshAFKFilter(c)
		if err := c.send([]byte{protocol.CommandAFK, protocol.AFKSet, protocol.AFKActive}); err != nil {
			return err
		}
		s.syncAFK(c, time.Now())
		return nil
	}
	c.afkFilter.Store(nil)
	// Refresh only the suppressed peer presentation. Do not repeat map entry,
	// arrival scripts, grants, event resume, team joins or interaction release.
	if err := c.send([]byte{protocol.CommandAFK, protocol.AFKSceneBegin}); err != nil {
		return err
	}
	for _, peer := range s.peers(c) {
		if peer.invisible {
			continue
		}
		packets, err := serverwire.PeerPackets(*peer.character, false, false)
		if err != nil {
			return err
		}
		packets = append(packets, s.peerPetPackets(peer.character)...)
		if move := playerWalkPacket(peer); move != nil {
			packets = append(packets, move)
		}
		if peer.emote != 0 {
			packets = append(packets, protocol.Builder{protocol.CommandPose, protocol.PoseBroadcast}.U32(peer.character.ID).U8(peer.emote))
		}
		if peer.shapeshift != 0 {
			packets = append(packets, shapeshiftPacket(peer))
		}
		if peer.battle != nil {
			packets = append(packets, battlePresenceState(peer))
		}
		if peer.fishing != nil {
			packets = append(packets, fishingStartedPacket(peer))
		}
		if err := s.sendAll(c, packets); err != nil {
			return err
		}
	}
	s.partySceneSnapshot(c, s.peers(c))
	return c.send([]byte{protocol.CommandAFK, protocol.AFKSceneEnd})
}

func (s *Server) refreshAFKFilter(c *Session) {
	if !c.afk || c.character == nil {
		return
	}
	f := afkOutputFilter{self: c.character.ID}
	if c.party != nil {
		for _, m := range c.party.members {
			if m.character != nil {
				if f.count < len(f.party) {
					f.party[f.count] = m.character.ID
					f.count++
				}
			}
		}
	}
	old := c.afkFilter.Load()
	if old != nil && *old == f {
		return
	}
	c.afkFilter.Store(&f)
}

// Caller owns worldMu. Private and shared snapshots use identical gameplay data.
func (s *Server) afkSnapshot(c *Session, private bool) protocol.AFKState {
	state := protocol.AFKState{Map: c.character.Map, X: c.character.X, Y: c.character.Y}
	members := []*Session{c}
	if !private && c.party != nil {
		members = c.party.members
	}
	for _, m := range members {
		ch := m.character
		if ch == nil {
			continue
		}
		u := protocol.AFKUnit{Owner: ch.ID, ID: ch.ID, Name: ch.Name, Level: ch.Level, HP: ch.HP, MaxHP: ch.MaxHP, SP: ch.SP, MaxSP: ch.MaxSP, EXP: ch.EXP}
		remaining := uint64(ch.EXP)
		for level := 1; level < int(ch.Level); level++ {
			need := ch.LevelRequirement(level)
			if remaining < need {
				remaining = 0
				break
			}
			remaining -= need
		}
		u.EXP, u.EXPNext = uint32(remaining), ch.LevelRequirement(int(ch.Level))
		if m.battle != nil {
			if bm := m.battle.member(m); bm != nil && bm.self != nil {
				f := bm.self
				u.HP, u.SP, u.MaxHP, u.MaxSP = uint32(max(0, f.HP)), uint32(max(0, f.SP)), uint32(max(0, f.MaxHP)), uint32(max(0, f.MaxSP))
			}
		}
		if !private {
			state.Units = append(state.Units, u)
		}
		for _, pet := range ch.Pets {
			if private && pet.ID == ch.ActivePet || m != c && pet.ID != ch.ActivePet {
				continue
			}
			u := protocol.AFKUnit{Owner: ch.ID, ID: pet.ID, Kind: protocol.AFKPet, Name: s.companionName(pet.ID, pet.Name), Level: pet.Level, HP: uint32(max(0, pet.HP)), MaxHP: uint32(max(0, pet.MaxHP)), SP: uint32(max(0, pet.SP)), MaxSP: uint32(max(0, pet.MaxSP)), EXP: pet.Exp, EXPNext: game.LevelExp(int(pet.Level)), Amity: pet.Amity}
			if m.battle != nil {
				if bm := m.battle.member(m); bm != nil && bm.pet != nil && pet.ID == ch.ActivePet {
					f := bm.pet
					u.HP, u.SP, u.MaxHP, u.MaxSP = uint32(max(0, f.HP)), uint32(max(0, f.SP)), uint32(max(0, f.MaxHP)), uint32(max(0, f.MaxSP))
				}
			}
			state.Units = append(state.Units, u)
		}
	}
	return state
}

// Server-pushed, change-only snapshots. No AFK client polling or SQL reads.
func (s *Server) syncAFK(c *Session, now time.Time) {
	if !c.afk || c.character == nil || !c.ready {
		return
	}
	s.refreshAFKFilter(c)
	stream := s.ensureAFKStream(c)
	if stream != nil {
		s.syncAFKStream(stream, now)
	}
	if now.Before(c.afkNextState) {
		return
	}
	c.afkNextState = now.Add(afkStateInterval)
	state := s.afkSnapshot(c, stream != nil)
	p, err := protocol.EncodeAFKState(state)
	if err != nil {
		c.conn.Close()
		return
	}
	if bytes.Equal(p, c.afkLastState) {
		return
	}
	wire := p
	if c.connectionFeatures&protocol.ConnectionAFKDelta != 0 {
		wire = protocol.CompactAFKUpdate(c.afkLastState, p)
	}
	if err := c.send(wire); err != nil {
		c.conn.Close()
		return
	}
	c.afkLastState = p
}

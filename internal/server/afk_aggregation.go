package server

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"sort"
	"time"
	"wonderland-gonline/internal/protocol"
)

// Owned only by its current authenticated subscribers. No global token registry:
// churn cannot accumulate abandoned streams, and obsolete groups are collectible.
type afkPartyStream struct {
	id, sequence uint64
	token        [protocol.AFKGroupTokenBytes]byte
	members      []*Session
	next         time.Time
	last         []byte
}

func (s *Server) ensureAFKStream(c *Session) *afkPartyStream {
	members := []*Session{}
	if c.party != nil && c.afkGroupToken != [protocol.AFKGroupTokenBytes]byte{} && c.connectionFeatures&protocol.ConnectionAFKAggregation != 0 {
		for _, m := range c.party.members {
			if m.afk && m.ready && m.character != nil && m.outputError() == nil && m.connectionNegotiated && m.connectionFeatures&protocol.ConnectionAFKAggregation != 0 && m.afkGroupToken == c.afkGroupToken {
				members = append(members, m)
			}
		}
	}
	if len(members) < 2 {
		if c.afkStream != nil {
			c.afkStream = nil
			c.afkLastState = nil
			c.afkNextState = time.Time{}
			p, _ := protocol.EncodeAFKBinding(protocol.AFKStreamBinding{})
			s.sendOrClose(c, p)
		}
		return nil
	}
	sort.Slice(members, func(i, j int) bool { return members[i].character.ID < members[j].character.ID })
	var stream *afkPartyStream
	for _, m := range members {
		old := m.afkStream
		if old == nil || old.token != c.afkGroupToken || len(old.members) != len(members) {
			continue
		}
		match := true
		for i := range members {
			match = match && old.members[i] == members[i]
		}
		if match {
			stream = old
			break
		}
	}
	if stream == nil {
		var id uint64
		for id == 0 {
			var raw [8]byte
			if _, err := rand.Read(raw[:]); err != nil {
				return nil
			}
			id = binary.LittleEndian.Uint64(raw[:])
		}
		stream = &afkPartyStream{id: id, token: c.afkGroupToken, members: members}
	}
	unchanged := true
	for _, m := range members {
		unchanged = unchanged && m.afkStream == stream
	}
	if unchanged {
		return stream
	}
	binding := protocol.AFKStreamBinding{Stream: stream.id, Carrier: members[0].character.ID}
	for _, m := range members {
		binding.Members = append(binding.Members, m.character.ID)
	}
	p, _ := protocol.EncodeAFKBinding(binding)
	for _, m := range members {
		if m.afkStream != stream {
			m.afkStream = stream
			m.afkLastState = nil
			m.afkNextState = time.Time{}
			s.sendOrClose(m, p)
		}
	}
	return stream
}

func (s *Server) syncAFKStream(stream *afkPartyStream, now time.Time) {
	if now.Before(stream.next) {
		return
	}
	stream.next = now.Add(afkStateInterval)
	carrier := stream.members[0]
	state := s.afkSnapshot(carrier, false)
	state.Map, state.X, state.Y = 0, 0, 0
	common := state.Units[:0]
	for _, u := range state.Units {
		if u.Kind == protocol.AFKPet && u.Owner == carrier.character.ID && u.ID != carrier.character.ActivePet {
			continue
		}
		common = append(common, u)
	}
	state.Units = common
	p, err := protocol.EncodeAFKState(state)
	if err != nil {
		carrier.conn.Close()
		return
	}
	if bytes.Equal(p, stream.last) {
		return
	}
	stream.sequence++
	full := p
	delta := true
	for _, m := range stream.members {
		delta = delta && m.connectionFeatures&protocol.ConnectionAFKDelta != 0
	}
	if delta {
		p = protocol.CompactAFKUpdate(stream.last, p)
	}
	p, err = protocol.EncodeAFKSharedUpdate(stream.id, stream.sequence, p)
	if err != nil {
		carrier.conn.Close()
		return
	}
	if err = carrier.send(p); err != nil {
		carrier.conn.Close()
		return
	}
	stream.last = full
}

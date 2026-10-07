package server

import (
	"context"
	"sort"
	"strings"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/protocol"
)

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

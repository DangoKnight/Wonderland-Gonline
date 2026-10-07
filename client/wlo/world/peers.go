package world

import (
	"bytes"
	"encoding/binary"
	"errors"
	"time"

	"wonderland-gonline/client/wlo/login"
)

// Other players. AC4 (receive case 0x2e0878, record FUN_00429a38) adds a
// player to the map when its map is ours; 6/1 for another ID walks it to a
// point (FUN_00418968, path FUN_0041b218); AC7 places it; AC12 with map 0
// takes it away. Names are drawn cyan (0x7ff, the overlay's ink for other
// players, 0x4154fe) above them.
type Peer struct {
	Player
	Element, Level byte
	Rebirth        byte
	Role           login.RoleView
	Walker         Walker
}

var errShortOther = errors.New("AC4: packet too short")

// ParseOther reads AC4 after its command byte: ID, body, element, level,
// map, X, Y, a byte, head (a word), the colour values, the worn items, a
// dword, three bytes, then the name.
func ParseOther(p []byte) (*Peer, error) {
	r := reader{b: p}
	pr := &Peer{}
	pr.ID = r.u32()
	pr.Body = r.u8()
	pr.Element, pr.Level = r.u8(), r.u8()
	pr.Map = r.u16()
	pr.X, pr.Y = int(r.u16()), int(r.u16())
	r.u8()
	pr.Head, pr.Look = r.u8(), r.u8()
	pr.Color1, pr.Color2 = r.u32(), r.u32()
	for n := int(r.u8()); n > 0; n-- {
		pr.Items = append(pr.Items, r.u16())
	}
	r.u32()
	r.u8()
	pr.Rebirth = r.u8()
	r.u8()
	pr.Name = r.str()
	if len(r.b) > 0 {
		pr.Nickname = r.str()
	}
	if len(r.b) > 0 {
		pr.Presence = r.u8()
	}
	if r.bad {
		return nil, errShortOther
	}
	pr.Direction = standingFront
	return pr, nil
}

// AddPeer is AC4 on this map: the player appears (again) where it stands,
// dressed by newBody.
func (w *World) AddPeer(p *Peer, newBody func() login.RoleView) {
	if p.Map != w.Player.Map || p.ID == w.Player.ID {
		return
	}
	if w.Peers == nil {
		w.Peers = map[uint32]*Peer{}
	}
	if newBody != nil {
		p.Role = newBody()
		Dress(p.Role, p.Player)
	}
	w.Peers[p.ID] = p
}

// Dress fills a role from an appearance (FUN_004013d0's fields).
func Dress(body login.RoleView, p Player) {
	if body == nil {
		return
	}
	slot := &login.CharacterSlot{Name: p.Name, Level: 1, Body: p.Body, Head: p.Head,
		Color1: p.Color1, Color2: p.Color2, Direction: p.Direction}
	for i, id := range p.Items {
		if i+1 < len(slot.Equipment) {
			slot.Equipment[i+1] = id
		}
	}
	body.SetCharacter(slot)
}

// MovePeer is 6/1 for another player: it walks to (x, y) along a planned
// path, or steps there directly when no path is found.
func (w *World) MovePeer(id uint32, x, y int, now time.Time) {
	p := w.Peers[id]
	if p == nil {
		return
	}
	path := w.Scene.Plan(p.X, p.Y, x, y)
	if path == nil {
		p.Walker.Stop(&p.Player)
		p.X, p.Y = x, y
		return
	}
	p.Walker.Start(&p.Player, path, now)
}

// PlacePeer is AC7 for another player.
func (w *World) PlacePeer(id uint32, x, y int) {
	if p := w.Peers[id]; p != nil {
		p.Walker.Stop(&p.Player)
		p.X, p.Y = x, y
		if companion := w.Companions[id]; companion != nil {
			companion.reset(&p.Player)
		}
	}
}

// RemovePeer is AC12 to map 0: the player leaves.
func (w *World) RemovePeer(id uint32) {
	delete(w.Peers, id)
	delete(w.Expressions, id)
	delete(w.Companions, id)
}

// PeerName is the name of a player on the map, nil when unknown.
func (w *World) PeerName(id uint32) []byte {
	if id == w.Player.ID {
		return w.Player.Name
	}
	if p := w.Peers[id]; p != nil {
		return p.Name
	}
	return nil
}

// Peers' names: cyan, as far above the feet as the player's own (the
// player's feet are at the screen centre, its name at 0xc8).
const (
	peerNameInk  = 0x7ff
	peerNameLift = screenH/2 - nameY
)

// Packet layouts after the command byte.
const (
	// 6/1: subcommand, ID, facing, X, Y.
	moveBytes = 10
	// AC7: ID, map, X, Y.
	placeBytes = 10
)

// ParseMove reads 6/1's ID and point.
func ParseMove(p []byte) (id uint32, x, y int, ok bool) {
	if len(p) < moveBytes {
		return 0, 0, 0, false
	}
	return binary.LittleEndian.Uint32(p[1:]), int(binary.LittleEndian.Uint16(p[6:])), int(binary.LittleEndian.Uint16(p[8:])), true
}

// ParsePlace reads AC7.
func ParsePlace(p []byte) (id uint32, mapID uint16, x, y int, ok bool) {
	if len(p) < placeBytes {
		return 0, 0, 0, 0, false
	}
	return binary.LittleEndian.Uint32(p), binary.LittleEndian.Uint16(p[4:]),
		int(binary.LittleEndian.Uint16(p[6:])), int(binary.LittleEndian.Uint16(p[8:])), true
}

// PeerByName is FUN_0042a478 over the map's players: the ID of the player
// with that name (letter case ignored), the player's own included.
func (w *World) PeerByName(name []byte) (uint32, bool) {
	if bytes.EqualFold(name, w.Player.Name) {
		return w.Player.ID, true
	}
	for id, p := range w.Peers {
		if bytes.EqualFold(name, p.Name) {
			return id, true
		}
	}
	return 0, false
}

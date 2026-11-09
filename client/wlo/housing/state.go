// Package housing decodes the native AC65 tent signs and AC62 furniture tree.
// Definitions are shared; this state belongs to one client connection/scene.
package housing

import (
	"encoding/binary"
	"wonderland-gonline/internal/protocol"
)

const (
	SignBytes            = 16 // owner, item, X, Y, appearance and colour
	SignHeaderBytes      = 2
	FurnitureFloorOffset = 49
)

type Sign struct {
	Owner             uint32
	Item              uint16
	X, Y              uint32
	Appearance, Color byte
}

type Furniture struct {
	Index, Item uint16
	X, Y, Z     uint32
	Rotation    byte
	Parent      uint16
	Floor       byte
}

type State struct {
	Signs                   map[uint32]Sign
	Owner                   uint32
	Floor                   byte
	FloorSurface, Wallpaper uint16
	Furniture               map[uint16]Furniture
	Locked                  bool
}

func sign(p []byte) Sign {
	return Sign{binary.LittleEndian.Uint32(p), binary.LittleEndian.Uint16(p[4:]), binary.LittleEndian.Uint32(p[6:]), binary.LittleEndian.Uint32(p[10:]), p[14], p[15]}
}

func furniture(p []byte) Furniture {
	return Furniture{binary.LittleEndian.Uint16(p), binary.LittleEndian.Uint16(p[2:]), binary.LittleEndian.Uint32(p[4:]), binary.LittleEndian.Uint32(p[8:]), binary.LittleEndian.Uint32(p[12:]), p[16], binary.LittleEndian.Uint16(p[17:]), p[FurnitureFloorOffset]}
}

// Receive validates a whole packet before publishing a replacement snapshot.
// TentExit is a sign list in the server-to-client direction, not an exit order.
func (s *State) Receive(p []byte) bool {
	if len(p) < 2 {
		return false
	}
	switch p[0] {
	case protocol.CommandTent:
		switch p[1] {
		case protocol.TentEnter:
			if len(p) != SignHeaderBytes+SignBytes {
				return false
			}
			v := sign(p[2:])
			if v.Owner == 0 || v.Item == 0 {
				return false
			}
			if s.Signs == nil {
				s.Signs = map[uint32]Sign{}
			}
			s.Signs[v.Owner] = v
		case protocol.TentExit:
			if (len(p)-SignHeaderBytes)%SignBytes != 0 {
				return false
			}
			next := map[uint32]Sign{}
			for at := SignHeaderBytes; at < len(p); at += SignBytes {
				v := sign(p[at:])
				if v.Owner == 0 || v.Item == 0 {
					return false
				}
				next[v.Owner] = v
			}
			s.Signs = next
		case protocol.TentClosed:
			if len(p) != 6 {
				return false
			}
			delete(s.Signs, binary.LittleEndian.Uint32(p[2:]))
		case protocol.TentInteriorStatus:
			if len(p) != 4 {
				return false
			}
			s.Locked = p[2] != 0
		default:
			return false
		}
	case protocol.CommandTentFurniture:
		switch p[1] {
		case protocol.TentFurnitureTree:
			if len(p) < protocol.TentTreeHeaderBytes || (len(p)-protocol.TentTreeHeaderBytes)%protocol.TentTreeRecordBytes != 0 {
				return false
			}
			next := map[uint16]Furniture{}
			for at := protocol.TentTreeHeaderBytes; at < len(p); at += protocol.TentTreeRecordBytes {
				v := furniture(p[at:])
				if v.Index == 0 || v.Item == 0 {
					return false
				}
				next[v.Index] = v
			}
			s.Owner = binary.LittleEndian.Uint32(p[2:])
			s.Furniture = next
		case protocol.TentPlaceFurniture:
			if len(p) != 2+protocol.TentTreeRecordBytes {
				return false
			}
			v := furniture(p[2:])
			if v.Index == 0 || v.Item == 0 {
				return false
			}
			if s.Furniture == nil {
				s.Furniture = map[uint16]Furniture{}
			}
			s.Furniture[v.Index] = v
		case protocol.TentMoveFurniture:
			if len(p) != protocol.TentMoveRotatedRequestBytes {
				return false
			}
			index := binary.LittleEndian.Uint16(p[2:])
			v, ok := s.Furniture[index]
			if !ok {
				return false
			}
			v.X = binary.LittleEndian.Uint32(p[4:])
			v.Y = binary.LittleEndian.Uint32(p[8:])
			v.Z = binary.LittleEndian.Uint32(p[12:])
			v.Rotation = p[16]
			s.Furniture[index] = v
		case protocol.TentPickupFurniture:
			if len(p) != protocol.TentNativePickupBytes {
				return false
			}
			delete(s.Furniture, binary.LittleEndian.Uint16(p[2:]))
		case protocol.TentFloorSurface, protocol.TentWallpaperSurface:
			if len(p) != 4 {
				return false
			}
			id := binary.LittleEndian.Uint16(p[2:])
			if p[1] == protocol.TentFloorSurface {
				s.FloorSurface = id
			} else {
				s.Wallpaper = id
			}
		default:
			return false
		}
	default:
		return false
	}
	return true
}

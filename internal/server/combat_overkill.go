package server

import (
	"context"
	"fmt"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
)

// returnOverkilledPlayer commits vitals, pet results and the return location
// together before publishing a warp. Caller holds worldMu. Other participants
// keep playing the remaining steps of this round.
func (s *Server) returnOverkilledPlayer(run *battleRun, m *battleMember, notifyOwner bool) error {
	c := m.c
	if c.battle != run {
		return nil
	}
	dst := starterBeach
	if point := c.character.RecordPoint; point != nil && point.Map != 0 {
		if _, ok := s.World.Map(point.Map); ok {
			dst = world.Destination{Map: point.Map, X: point.X, Y: point.Y}
		}
	}
	if _, ok := s.World.Map(dst.Map); !ok {
		return fmt.Errorf("overkill return map %d unavailable", dst.Map)
	}
	next := c.character.Clone()
	// Use the ordinary one-HP revival rule. This changes the destination
	// only for overkills; ordinary knockouts remain on the battle field.
	next.HP = game.PostBattleRevivalHP
	next.SP = uint32(max(0, m.self.SP))
	next.HP, next.SP = min(next.HP, next.MaxHP), min(next.SP, next.MaxSP)
	next.Map, next.X, next.Y = dst.Map, dst.X, dst.Y
	next.TentReturn = nil
	roster := newPetRoster()
	roster.synced = c.pets.synced
	for id, slot := range c.pets.slots {
		roster.slots[id] = slot
	}
	var petPackets, peerPackets [][]byte
	if !m.resultSaved {
		petPackets, peerPackets = s.petResults(run, m, &next, battle.Defeat, roster)
	}
	oldPeers := s.peers(c)
	wasPublished := s.world[c.info.ID] == c
	if err := s.commit(context.Background(), c, next); err != nil {
		return err
	}
	c.pets = roster
	m.resultSaved = true
	c.battle = nil
	run.b.Leave(next.ID)
	c.encounter.battleOver()
	closePacket := protocol.Builder{protocol.CommandBattleState, protocol.BattleStateWireCode0}.U32(next.ID).U16(0)
	if m.pet != nil && !m.pet.Overkilled {
		s.sendBattleSpectators(run, []byte{protocol.CommandBattleState, protocol.BattleStateExit, m.pet.X, m.pet.Y, 0})
	}
	s.sendBattleSpectators(run, closePacket)
	// The owner has finished its overkill flight. Remaining clients also
	// withdraw the owner's pet and clear its participant/world battle flag.
	for _, other := range run.active() {
		if other.roundBundled {
			continue
		} // Already scheduled in its full round.
		var packets [][]byte
		if m.pet != nil && !m.pet.Overkilled {
			packets = append(packets, []byte{protocol.CommandBattleState, protocol.BattleStateExit, m.pet.X, m.pet.Y, 0})
		}
		packets = append(packets, closePacket)
		if err := s.sendAll(other.c, packets); err != nil {
			other.c.conn.Close()
		}
	}
	state := battleState(next.ID, false)
	packets := [][]byte{closePacket, {protocol.CommandMovement, protocol.MovementMovementLock, 0}, {protocol.CommandEvent, protocol.EventResume}, state}
	packets = append(packets, petPackets...)
	packets = append(packets, c.character.StatPackets(s.Assets.Items)...)
	// The full commit adopted the destination. Publish the old-map departure
	// to the captured recipients rather than looking them up by the new map.
	if wasPublished {
		delete(s.world, c.info.ID)
		c.ready = false
		if !c.invisible {
			departure := protocol.Builder{protocol.CommandMapAcknowledgment}.U32(next.ID).U16(dst.Map).U16(dst.X).U16(dst.Y).U16(0).U8(0)
			for _, peer := range oldPeers {
				packets := append([][]byte{state}, peerPackets...)
				packets = append(packets, departure)
				if err := s.sendAll(peer, packets); err != nil {
					peer.conn.Close()
				}
			}
		}
	}
	if !notifyOwner {
		s.endEvent(c)
		return nil
	}
	if err := s.sendAll(c, packets); err != nil {
		return err
	}
	c.restoreUIAfterMapLoad = true
	if err := s.teleportPrelude(c); err != nil {
		return err
	}
	return s.teleportAfterSave(c, dst, 0)
}

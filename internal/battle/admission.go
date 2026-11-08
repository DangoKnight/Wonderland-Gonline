package battle

import "wonderland-gonline/internal/protocol"

// AdmissionSlot reserves an owner/pet pair without moving any existing actor.
func (b *Battle) AdmissionSlot(side Side) int {
	slots := b.AdmissionSlots(side)
	if len(slots) > 0 {
		return slots[0]
	}
	return -1
}

func (b *Battle) AdmissionSlots(side Side) []int {
	allies, _ := b.Teams(side)
	var slots []int
	for slot := range playerSlots {
		probe := &Fighter{Kind: Player}
		Place(probe, side, slot)
		occupied := false
		for _, f := range allies {
			if !f.Withdrawn && !f.Overkilled && f.Y == probe.Y && (f.Kind == Player || f.Kind == Pet) {
				occupied = true
			}
		}
		if !occupied {
			slots = append(slots, slot)
		}
	}
	return slots
}

// Admit adds an already-positioned pair during command selection. Existing
// pending actions remain intact; the caller includes the new owner in readiness.
func (b *Battle) Admit(self, pet *Fighter) {
	team := &b.Attackers
	if self.Side == Defender {
		team = &b.Defenders
	}
	*team = append(*team, self)
	if pet != nil {
		*team = append(*team, pet)
	}
}

func ParticipantPacket(f *Fighter, mode byte) []byte {
	return record(protocol.Builder{protocol.CommandBattleState, protocol.BattleStateParticipant}, mode, f, byte(f.Kind), f.ClickID, f.Owner)
}

// EntrySnapshot uses repeated (mode, 31-byte actor) blocks, including the
// off-grid spectator identity used by the native client to end its own scene.
func (b *Battle) EntrySnapshot(self *Fighter, background byte, spectator bool) [][]byte {
	mode := byte(protocol.BattlePresenceJoined)
	if spectator {
		mode = protocol.BattlePresenceSpectator
	}
	p := record(protocol.Builder{protocol.CommandBattleState, protocol.BattleStateFormation}.U8(background).U8(compatibilityFormationMetadata), mode, self, byte(Player), 0, 0)
	for _, f := range b.all() {
		if f == self || f.Withdrawn || f.Overkilled || f.Captured {
			continue
		}
		if !spectator && f.Kind == Pet && f.Owner == self.ID {
			continue
		}
		mode := byte(Attacker) // Native existing ally mode, independent of grid side.
		if f.Side != self.Side {
			mode = protocol.BattleParticipantEnemySnapshot
		}
		p = record(p, mode, f, byte(f.Kind), f.ClickID, f.Owner)
	}
	out := [][]byte{p, {protocol.CommandBattleState, protocol.BattleStateInitialize, KindStandard}}
	if !spectator {
		for _, f := range b.all() {
			if f.Kind == Pet && f.Owner == self.ID && !f.Withdrawn && !f.Overkilled {
				out = append(out, ParticipantPacket(f, protocol.BattleParticipantWaitNextTurn))
			}
		}
	}
	return out
}

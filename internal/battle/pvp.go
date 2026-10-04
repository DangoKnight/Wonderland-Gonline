package battle

const (
	friendlyPetMarker     = 5
	mirroredGridColumnSum = 5
)

// Teams resolves allegiance for actions, independently of fighter kind.
func (b *Battle) Teams(side Side) (allies, enemies []*Fighter) {
	if side == Defender {
		return b.Defenders, b.Attackers
	}
	return b.Attackers, b.Defenders
}

// Place assigns a participant to the native player/pet formation on either side.
func Place(f *Fighter, side Side, slot int) {
	f.Side = side
	positions := playerSlots
	if f.Kind == Pet {
		positions = petSlots
	}
	f.X, f.Y = positions[slot][0], positions[slot][1]
	if side == Defender {
		f.X = mirroredGridColumnSum - f.X
	}
}

func NewPvP(attackers, defenders []*Fighter) *Battle {
	return &Battle{Attackers: attackers, Defenders: defenders, PvP: true, Pending: map[int]Action{}, Roster: map[uint32][]uint32{}}
}

// Forfeit returns an ending only when an entire PvP side has disconnected.
func (b *Battle) Forfeit() Outcome {
	if !b.PvP {
		return Continue
	}
	for _, side := range []Side{Attacker, Defender} {
		members, _ := b.Teams(side)
		present := false
		for _, f := range members {
			present = present || (f.Kind == Player && !f.Absent)
		}
		if !present {
			if side == Attacker {
				return Defeat
			}
			return Victory
		}
	}
	return Continue
}

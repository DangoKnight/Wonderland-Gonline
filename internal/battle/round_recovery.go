package battle

import "wonderland-gonline/internal/game"

func (b *Battle) roundRecovery(r Rules) [][]byte {
	if r.RoundRecoveryHP <= 0 && r.RoundRecoverySP <= 0 {
		return nil
	}
	var packets [][]byte
	for _, f := range b.Attackers {
		if !validExperienceParticipant(f) || f.Dead() {
			continue
		}
		if hp := min(f.MaxHP, f.HP+max(0, r.RoundRecoveryHP)); hp > f.HP {
			f.HP = hp
			packets = append(packets, StatSync(f.X, f.Y, game.StatCurrentHP, uint32(f.HP)))
		}
		if sp := min(f.MaxSP, f.SP+max(0, r.RoundRecoverySP)); sp > f.SP {
			f.SP = sp
			packets = append(packets, StatSync(f.X, f.Y, game.StatCurrentSP, uint32(f.SP)))
		}
	}
	return packets
}

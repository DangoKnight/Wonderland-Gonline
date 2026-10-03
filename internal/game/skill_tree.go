package game

import "wonderland-go/internal/protocol"

// UnlockQualifiedSkills ports the stat tree and, when requested, grade-ten
// evolutions. Attributes include avatar bonuses, not equipment combat bonuses.
// Missing catalog entries are left unlearned rather than breaking login snapshots.
// The caller saves the character before sending the returned incremental packets.
func (c *Character) UnlockQualifiedSkills(evolve bool, available func(uint16) bool) [][]byte {
	a := c.Attributes()
	var qualified []uint16
	for _, r := range progressionSkills {
		b := r.minimum
		if c.Element == r.element && a.Strength >= b.Strength && a.Constitution >= b.Constitution && a.Intelligence >= b.Intelligence && a.Wisdom >= b.Wisdom && a.Agility >= b.Agility {
			qualified = append(qualified, r.id)
		}
	}
	if evolve {
		for _, sk := range c.Skills {
			if id, ok := skillEvolutions[sk.ID]; ok && sk.Grade >= 10 {
				qualified = append(qualified, id)
			}
		}
	}
	var packets [][]byte
	for _, id := range qualified {
		if available != nil && !available(id) {
			continue
		}
		found := false
		for _, sk := range c.Skills {
			if sk.ID == id {
				found = true
				break
			}
		}
		if found {
			continue
		}
		c.Skills = append(c.Skills, LearnedSkill{ID: id, Grade: 1})
		clientID := id
		if id == StarterStunt(c.Body, c.Head) {
			clientID = 15003
		}
		packets = append(packets,
			protocol.Builder{protocol.CommandStats, protocol.StatsStatUpdate, 110, 1}.U32(1).U32(uint32(clientID)),
			protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateWireCode12}.U16(id).U8(1),
			protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateSkillProficiency}.U32(uint32(id)).U16(0))
	}
	if len(packets) > 0 {
		packets = append(packets, []byte{protocol.CommandCharacterState, protocol.CharacterStateRefresh})
	}
	return packets
}

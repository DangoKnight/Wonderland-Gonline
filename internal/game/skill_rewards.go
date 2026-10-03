package game

import "wonderland-go/internal/protocol"

// LearnSkill is EVE opcode 11 and SkillManager.UnlockSkill. Replayed learning
// leaves trained grade/proficiency intact; a new reward starts at grade one.
func (c *Character) LearnSkill(id uint16) [][]byte {
	if id == 0 {
		return nil
	}
	for _, skill := range c.Skills {
		if skill.ID == id && skill.Grade >= MinSkillGrade {
			return nil
		}
	}
	return c.SetSkillGrade(id, MinSkillGrade)
}

// SetSkillGrade ports SkillManager.UnlockSkill for an explicit grade override.
// Unlike quest learning, it resets proficiency even when the grade is unchanged.
// The caller validates catalog membership and commits before emitting packets.
func (c *Character) SetSkillGrade(id uint16, grade byte) [][]byte {
	if id == 0 || grade < MinSkillGrade || grade > MaxSkillGrade {
		return nil
	}
	old := byte(0)
	found := false
	for i := range c.Skills {
		if c.Skills[i].ID != id {
			continue
		}
		old = c.Skills[i].Grade
		c.Skills[i].Grade, c.Skills[i].EXP = grade, 0
		found = true
		break
	}
	if !found {
		c.Skills = append(c.Skills, LearnedSkill{ID: id, Grade: grade})
	}
	clientID := id
	if id == StarterStunt(c.Body, c.Head) {
		clientID = StarterStuntClientID
	}
	var packets [][]byte
	if grade > old {
		packets = append(packets, protocol.Builder{protocol.CommandStats, protocol.StatsStatUpdate, StatSkillGrade, protocol.StatsValueAbsolute}.U32(uint32(grade)).U32(uint32(clientID)))
	}
	return append(packets,
		protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateWireCode16, 0}.U16(clientID).U8(grade),
		protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateWireCode12}.U16(clientID).U8(grade),
		protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateSkillProficiency}.U32(uint32(clientID)).U16(0))
}

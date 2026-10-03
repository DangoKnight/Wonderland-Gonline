package game

import "wonderland-go/internal/protocol"

// AddSkillEXP follows SkillManager.AddSkillExp, including the native stunt alias.
// The caller persists the changed character before sending the returned packets.
func (c *Character) AddSkillEXP(id uint16, gain uint32) [][]byte {
	if id == 0 || gain == 0 {
		return nil
	}
	for i := range c.Skills {
		sk := &c.Skills[i]
		clientID := sk.ID
		if sk.ID == StarterStunt(c.Body, c.Head) {
			clientID = 15003
		}
		if (sk.ID != id && clientID != id) || sk.Grade == 0 || sk.Grade >= 10 {
			continue
		}
		old := sk.Grade
		sk.EXP += gain // C# UInt32 addition wraps.
		for sk.Grade < 10 && sk.EXP >= uint32(sk.Grade)*SkillGradeExpScale {
			sk.EXP -= uint32(sk.Grade) * 100
			sk.Grade++
		}
		prof := min(uint32(SkillProficiencyScale), sk.EXP*SkillProficiencyScale/(uint32(sk.Grade)*SkillGradeExpScale))
		packets := [][]byte{protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateSkillProficiency}.U32(uint32(clientID)).U16(uint16(prof))}
		if sk.Grade > old {
			packets = append(packets, protocol.Builder{protocol.CommandStats, protocol.StatsStatUpdate, 110, 1}.U32(uint32(sk.Grade)).U32(uint32(clientID)))
		}
		return append(packets, protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateWireCode12}.U16(clientID).U8(sk.Grade))
	}
	return nil
}

// AddSkillEXP follows AddPetSkillExp: pet EXP uses a wide accumulator and clears
// at grade ten. A zero client slot suppresses packets without suppressing progress.
func (p *Pet) AddSkillEXP(id uint16, gain uint32, slot byte) [][]byte {
	if gain == 0 {
		return nil
	}
	for i := range p.Skills {
		sk := &p.Skills[i]
		if sk.ID != id || sk.Grade == 0 || sk.Grade >= 10 {
			continue
		}
		old := sk.Grade
		exp := uint64(sk.Exp) + uint64(gain)
		for sk.Grade < 10 && exp >= uint64(sk.Grade)*100 {
			exp -= uint64(sk.Grade) * 100
			sk.Grade++
		}
		sk.Exp = uint32(exp)
		if sk.Grade >= 10 {
			sk.Exp = 0
		}
		if slot == 0 {
			return nil
		}
		var packets [][]byte
		if sk.Grade > old {
			packets = append(packets, protocol.Builder{protocol.CommandStats, protocol.StatsWireCode2, 4}.U16(uint16(slot)).U8(110).U8(1).U32(uint32(sk.Grade)).U32(uint32(id)))
		}
		return append(packets, protocol.Builder{protocol.CommandStats, protocol.StatsWireCode2, 4}.U16(uint16(slot)).U8(111).U8(1).U32(sk.Exp).U32(uint32(id)))
	}
	return nil
}

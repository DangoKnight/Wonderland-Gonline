package server

import "wonderland-gonline/internal/game"

// unlockQualifiedSkills applies SQL evolution definitions. The retained compiled
// source table supports synthetic/older catalogs; a populated SQL dataset owns
// evolution decisions and can disable individual links through administration.
func (s *Server) unlockQualifiedSkills(c *game.Character, evolve bool) [][]byte {
	packets := c.UnlockQualifiedSkills(evolve && s.Assets.SkillEvolutions == nil, s.hasSkill)
	if evolve {
		for _, e := range s.Assets.SkillEvolutions {
			if !s.hasSkill(e.ToSkill) || e.RequiredGrade == 0 {
				continue
			}
			for _, sk := range c.Skills {
				if sk.ID == e.FromSkill && sk.Grade >= e.RequiredGrade {
					packets = append(packets, c.LearnSkill(e.ToSkill)...)
					break
				}
			}
		}
	}
	if c.Reborn {
		id := game.RebornJobSkill(c.Job)
		if id != 0 && s.hasSkill(id) {
			packets = append(packets, c.LearnSkill(id)...)
		}
	}
	return packets
}

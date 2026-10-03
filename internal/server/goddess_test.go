package server

import (
	"context"
	"testing"

	"wonderland-go/internal/game"
)

func TestSQLGoddessEffectsAndDurableProficiency(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint16{15188, 15189} {
		name := "Shrink"
		if id == 15189 {
			name = "Hot Fire Attack"
		}
		t.Run(name, func(t *testing.T) {
			s, c, wire := battleFixture(t, 1000)
			defer s.abandonBattle(c)
			skill, ok := catalog.Skills[id]
			if !ok || skill.Name != name {
				t.Fatal("native Goddess skill missing", skill)
			}
			s.Assets.Skills = catalog.Skills
			c.character.Skills = append(c.character.Skills, game.LearnedSkill{ID: id, Grade: 1})
			run := c.battle
			actor := run.members[0].self
			actor.Char = c.character
			actor.SP = 120
			tx := byte(2)
			if id == 15189 {
				tx = 4
			}
			wire.Reset()
			if err := s.worldCommand(context.Background(), c, []byte{50, 1, 4, 2, tx, 2, byte(id), byte(id >> 8)}); err != nil {
				t.Fatal(err)
			}
			settle(t, s, c)
			s.worldMu.Lock()
			shrunk, hot := run.b.Defenders[0].HasEffect(15188), actor.HasEffect(15189)
			hp, sp := run.b.Defenders[0].HP, actor.SP
			s.worldMu.Unlock()
			if hp != 1000 || sp != 120-int(skill.SP) || id == 15188 && !shrunk || id == 15189 && !hot {
				t.Fatal("Goddess became damage or missed its target", hp, sp, shrunk, hot)
			}
			stored, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, sk := range stored[0].Skills {
				if sk.ID == id {
					found = sk.EXP == 1
				}
			}
			if !found {
				t.Fatal("successful Goddess cast did not save proficiency")
			}
			want := []byte{50, 1, 17, 0, 4, 2, byte(id), byte(id >> 8), 0, 1, tx, 2, 1, 0, 1, 0, 0, 0, 0, 0, 1}
			if !contains(wire.packets(t), want) {
				t.Fatal("non-HP Goddess animation missing")
			}
		})
	}
}

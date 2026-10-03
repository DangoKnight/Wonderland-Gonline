package server

import (
	"context"
	"testing"
	"wonderland-go/internal/game"
)

func TestSQLControlAbilityBlocksWithoutDamageAndSavesProgress(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	skill := catalog.Skills[11051] // Native Freezing Spell: code 1, control layer.
	if len(skill.Effects) != 1 || !skill.Effects[0].BlockActions {
		t.Fatal("SQL control definition missing", skill)
	}
	s, c, _ := battleFixture(t, 1000)
	defer s.abandonBattle(c)
	s.Assets.Skills = catalog.Skills
	c.character.Skills = append(c.character.Skills, game.LearnedSkill{ID: 11051, Grade: 1})
	run := c.battle
	actor := run.members[0].self
	actor.Char = c.character
	actor.SP = 120
	// Cast before the monster so the restriction suppresses this round's turn.
	actor.Spd = 100
	hp := actor.HP
	if err := s.worldCommand(context.Background(), c, []byte{50, 1, 4, 2, 2, 2, 43, 43}); err != nil {
		t.Fatal(err)
	}
	settle(t, s, c)
	s.worldMu.Lock()
	blocked, targetHP, remainingSP, ownerHP := !run.b.Defenders[0].CanAct(), run.b.Defenders[0].HP, actor.SP, actor.HP
	s.worldMu.Unlock()
	if !blocked || targetHP != 1000 || remainingSP != 120-int(skill.SP) || ownerHP != hp {
		t.Fatal("control became damage or failed to suppress enemy turn", blocked, targetHP, remainingSP, ownerHP)
	}
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, sk := range stored[0].Skills {
		if sk.ID == 11051 {
			found = sk.EXP == 1
		}
	}
	if !found {
		t.Fatal("successful control cast did not persist proficiency")
	}
}

func TestSQLVanishProtectsAndSavesProgress(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	skill := catalog.Skills[15190]
	if len(skill.Effects) != 1 || !skill.Effects[0].MissPhysicalAttacks || !skill.Effects[0].MissAreaAttacks {
		t.Fatal("SQL Vanish protection missing", skill)
	}
	s, c, _ := battleFixture(t, 1000)
	defer s.abandonBattle(c)
	s.Assets.Skills = catalog.Skills
	c.character.Skills = append(c.character.Skills, game.LearnedSkill{ID: 15190, Grade: 1})
	run := c.battle
	actor := run.members[0].self
	actor.Char = c.character
	actor.SP = 300
	actor.Spd = 100
	hp := actor.HP
	if err := s.worldCommand(context.Background(), c, []byte{50, 1, 4, 2, 4, 2, 86, 59}); err != nil {
		t.Fatal(err)
	}
	settle(t, s, c)
	s.worldMu.Lock()
	protected, remainingHP, remainingSP := actor.HasEffect(15190), actor.HP, actor.SP
	s.worldMu.Unlock()
	if !protected || remainingHP != hp || remainingSP != 300-int(skill.SP) {
		t.Fatal("Vanish cast failed to protect", protected, remainingHP, remainingSP)
	}
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, sk := range stored[0].Skills {
		if sk.ID == 15190 {
			found = sk.EXP == 1
		}
	}
	if !found {
		t.Fatal("Vanish proficiency not saved")
	}
}

func TestSQLStoneWallProtectsWithoutDamageAndSavesProgress(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	skill := catalog.Skills[11055]
	if len(skill.Effects) != 1 || !skill.Effects[0].MissPhysicalAttacks || skill.Effects[0].MissAreaAttacks {
		t.Fatal("SQL Stone Wall protection missing", skill)
	}
	s, c, _ := battleFixture(t, 1000)
	defer s.abandonBattle(c)
	s.Assets.Skills = catalog.Skills
	c.character.Skills = append(c.character.Skills, game.LearnedSkill{ID: 11055, Grade: 1})
	run := c.battle
	actor := run.members[0].self
	actor.Char = c.character
	actor.SP = 300
	actor.Spd = 100
	hp := actor.HP
	if err := s.worldCommand(context.Background(), c, []byte{50, 1, 4, 2, 4, 2, 47, 43}); err != nil {
		t.Fatal(err)
	}
	settle(t, s, c)
	s.worldMu.Lock()
	protected, remainingHP, remainingSP, enemyHP := actor.HasEffect(11055), actor.HP, actor.SP, run.b.Defenders[0].HP
	s.worldMu.Unlock()
	if !protected || remainingHP != hp || remainingSP != 300-int(skill.SP) || enemyHP != 1000 {
		t.Fatal("Stone Wall failed to protect or damaged an enemy", protected, remainingHP, remainingSP, enemyHP)
	}
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, sk := range stored[0].Skills {
		if sk.ID == 11055 {
			found = sk.EXP == 1
		}
	}
	if !found {
		t.Fatal("Stone Wall proficiency not saved")
	}
}

func TestSQLWaterShieldAllowsPhysicalDamageAndSavesProgress(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	skill := catalog.Skills[11080]
	if len(skill.Effects) != 1 || len(skill.Effects[0].Modifiers) != 1 || skill.Effects[0].Modifiers[0].Stat != "magical_damage_taken" {
		t.Fatal("SQL Water Shield scope incorrect", skill)
	}
	s, c, _ := battleFixture(t, 1000)
	defer s.abandonBattle(c)
	s.Assets.Skills = catalog.Skills
	c.character.Skills = append(c.character.Skills, game.LearnedSkill{ID: 11080, Grade: 1})
	run := c.battle
	actor := run.members[0].self
	actor.Char = c.character
	actor.SP = 300
	actor.Spd = 100
	actor.HP, actor.MaxHP = 1000, 1000
	actor.Def = 20
	run.b.Defenders[0].Atk = 100 // Native monster basic attack deals 100 physical damage.
	if err := s.worldCommand(context.Background(), c, []byte{50, 1, 4, 2, 4, 2, 72, 43}); err != nil {
		t.Fatal(err)
	}
	settle(t, s, c)
	s.worldMu.Lock()
	protected, remainingHP, remainingSP, enemyHP := actor.HasEffect(11080), actor.HP, actor.SP, run.b.Defenders[0].HP
	s.worldMu.Unlock()
	if !protected || remainingHP != 900 || remainingSP != 300-int(skill.SP) || enemyHP != 1000 {
		t.Fatal("Water Shield blocked physical damage or cast incorrectly", protected, remainingHP, remainingSP, enemyHP)
	}
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, sk := range stored[0].Skills {
		if sk.ID == 11080 {
			found = sk.EXP == 1
		}
	}
	if !found {
		t.Fatal("Water Shield proficiency not saved")
	}
}

func TestSQLShieldDefenseBuffsStatsAndSavesProgress(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	skill := catalog.Skills[11057]
	if len(skill.Effects) != 1 || len(skill.Effects[0].Modifiers) != 2 || skill.Effects[0].Modifiers[0].Stat != "def" || skill.Effects[0].Modifiers[1].Stat != "mdef" {
		t.Fatal("SQL Shield Defense scope incorrect", skill)
	}
	s, c, _ := battleFixture(t, 1000)
	defer s.abandonBattle(c)
	s.Assets.Skills = catalog.Skills
	c.character.Skills = append(c.character.Skills, game.LearnedSkill{ID: 11057, Grade: 1})
	run := c.battle
	actor := run.members[0].self
	actor.Char = c.character
	actor.SP = 300
	actor.Spd = 100
	actor.HP, actor.MaxHP = 1000, 1000
	actor.Def = 20
	run.b.Defenders[0].Atk = 100 // Native monster basic attack deals 100 physical damage.
	if err := s.worldCommand(context.Background(), c, []byte{50, 1, 4, 2, 4, 2, 49, 43}); err != nil {
		t.Fatal(err)
	}
	settle(t, s, c)
	s.worldMu.Lock()
	protected, remainingHP, remainingSP, enemyHP := actor.HasEffect(11057), actor.HP, actor.SP, run.b.Defenders[0].HP
	s.worldMu.Unlock()
	if !protected || remainingHP != 902 || remainingSP != 300-int(skill.SP) || enemyHP != 1000 {
		t.Fatal("Shield Defense failed to increase defense or cast incorrectly", protected, remainingHP, remainingSP, enemyHP)
	}
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, sk := range stored[0].Skills {
		if sk.ID == 11057 {
			found = sk.EXP == 1
		}
	}
	if !found {
		t.Fatal("Shield Defense proficiency not saved")
	}
}

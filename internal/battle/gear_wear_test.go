package battle

import "testing"

func TestCombatGearHitsExcludeMissesAlliesAndPvP(t *testing.T) {
	enemy := &Fighter{Kind: Monster, Side: Defender, Level: 30, HP: 100}
	player := &Fighter{Kind: Player, Side: Attacker, HP: 100}
	b := &Battle{}
	b.recordGearHit(enemy, player, 12, 6)
	if len(b.GearHits[player]) != 1 || b.GearHits[player][0].Contacts != 6 {
		t.Fatal(b.GearHits)
	}
	b.recordGearHit(enemy, player, 0, 6)
	b.PvP = true
	b.recordGearHit(enemy, player, 12, 6)
	b.PvP = false
	enemy.Side = Attacker
	b.recordGearHit(enemy, player, 12, 6)
	if len(b.GearHits[player]) != 1 {
		t.Fatal("invalid hit wore equipment")
	}
}

func TestEnemyAttackRecordsWearAfterDamage(t *testing.T) {
	b, self := setup(10000)
	b.Idle(self)
	_, _ = b.Round(lowest)
	if self.HP >= 100 || len(b.GearHits[self]) != 1 {
		t.Fatal("enemy damage did not record wear", self.HP, b.GearHits)
	}
	if b.GearHits[self][0].EnemyLevel != 5 || b.GearHits[self][0].Contacts != 1 {
		t.Fatal(b.GearHits)
	}
}

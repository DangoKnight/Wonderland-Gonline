package battle

type GearHit struct {
	EnemyLevel byte
	Contacts   int
}

func (b *Battle) recordGearHit(enemy, target *Fighter, damage, contacts int) {
	if b.PvP || enemy.Kind != Monster || target.Kind != Player && target.Kind != Pet || enemy.Side == target.Side || damage <= 0 || target.Dead() {
		return
	}
	if b.GearHits == nil {
		b.GearHits = map[*Fighter][]GearHit{}
	}
	b.GearHits[target] = append(b.GearHits[target], GearHit{enemy.Level, min(damage, max(1, contacts))})
}

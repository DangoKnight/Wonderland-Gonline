package battle

import (
	"math"
	"sort"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

const (
	minimumMonsterExperience        = 10
	monsterExperiencePerLevel       = 15
	participationExperiencePerLevel = 8
	experiencePercentBase           = 100
	experienceTwoUnitComboBonus     = 40
	experienceExtraComboUnitBonus   = 10
	experienceComboMinimum          = 2
	experienceFullCredit            = 2 // Integer half-units avoid fractional participation.
	experienceDamageCredit          = 1
)

func MonsterExperience(level byte) uint64 {
	return uint64(max(minimumMonsterExperience, int(level)*monsterExperiencePerLevel))
}

func participationMonsterExperience(level byte) uint64 {
	return uint64(max(1, level)) * participationExperiencePerLevel
}

type experienceIdentity struct {
	kind      Kind
	owner, id uint32
}

func (f *Fighter) experienceID() experienceIdentity {
	id := f.ID
	owner := uint32(0)
	if f.Kind == Pet {
		id = game.BroadcastID(id)
		owner = f.Owner
	}
	return experienceIdentity{f.Kind, owner, id}
}
func experienceLess(a, b experienceIdentity) bool {
	if a.owner != b.owner {
		return a.owner < b.owner
	}
	if a.id != b.id {
		return a.id < b.id
	}
	return a.kind < b.kind
}
func validExperienceParticipant(f *Fighter) bool {
	return f != nil && f.ID != 0 && (f.Kind == Player || f.Kind == Pet && f.Owner != 0) && !f.Absent && !f.Withdrawn && !f.Joining
}

func (f *Fighter) recordImmediateSupport(caster *Fighter) {
	if caster == nil || caster == f || caster.Side != f.Side {
		return
	}
	if f.immediateSupport == nil {
		f.immediateSupport = map[*Fighter]bool{}
	}
	f.immediateSupport[caster] = true
}

// Non-persistent support belongs to the next executed offensive action, even
// when it misses or fails to kill. Buffs follow effect lifetime; revives last
// until the battle ends and are not consumed by attacks.
func (f *Fighter) consumeSupport() []*Fighter {
	seen := map[experienceIdentity]bool{}
	var result []*Fighter
	add := func(caster *Fighter) {
		if !validExperienceParticipant(caster) || caster.Side != f.Side || caster == f {
			return
		}
		id := caster.experienceID()
		if seen[id] {
			return
		}
		seen[id] = true
		result = append(result, caster)
	}
	for caster := range f.immediateSupport {
		add(caster)
	}
	f.immediateSupport = nil
	for caster := range f.reviveSupport {
		add(caster)
	}
	for _, effect := range f.Effects {
		if effect.Turns > 0 {
			add(effect.caster)
		}
	}
	sort.Slice(result, func(i, j int) bool { return experienceLess(result[i].experienceID(), result[j].experienceID()) })
	return result
}

func (b *Battle) recordReviveSupport(target, caster *Fighter) {
	if caster == nil || caster == target || caster.Side != target.Side {
		return
	}
	if b.reviveSupport == nil {
		b.reviveSupport = map[experienceIdentity]map[*Fighter]bool{}
	}
	id := target.experienceID()
	if b.reviveSupport[id] == nil {
		b.reviveSupport[id] = map[*Fighter]bool{}
	}
	b.reviveSupport[id][caster] = true
	target.reviveSupport = b.reviveSupport[id]
}

func (f *Fighter) applyEffectFrom(caster *Fighter, source uint16, index int, e assets.SkillEffect) {
	f.ApplyEffect(source, index, e)
	for i := range f.Effects {
		if f.Effects[i].Source == source && f.Effects[i].Index == index {
			f.Effects[i].caster = caster
			return
		}
	}
}

func killExperiencePool(base uint64, participants, comboUnits int) uint64 {
	pool := base * uint64(participants)
	if comboUnits < experienceComboMinimum {
		return pool
	}
	percent := experiencePercentBase + experienceTwoUnitComboBonus + (comboUnits-experienceComboMinimum)*experienceExtraComboUnitBonus
	return uint64(math.RoundToEven(float64(pool) * float64(percent) / experiencePercentBase))
}

// The base amount is bounded by the native byte-sized monster level. Largest
// remainders preserve the bonused pool; ties use stable entity identity.
func divideKillExperience(base uint64, participants []*Fighter, comboUnits int) map[experienceIdentity]uint64 {
	return divideWeightedKillExperience(base, participants, nil, comboUnits)
}

func divideWeightedKillExperience(base uint64, participants, earlier []*Fighter, comboUnits int) map[experienceIdentity]uint64 {
	unique := map[experienceIdentity]*Fighter{}
	credit := map[experienceIdentity]uint64{}
	for _, f := range earlier {
		if validExperienceParticipant(f) {
			id := f.experienceID()
			unique[id] = f
			credit[id] = experienceDamageCredit
		}
	}
	for _, f := range participants {
		if validExperienceParticipant(f) {
			id := f.experienceID()
			credit[id] = experienceFullCredit
			if _, exists := unique[id]; !exists {
				unique[id] = f
			}
		}
	}
	ids := make([]experienceIdentity, 0, len(unique))
	sum := uint64(0)
	for id, f := range unique {
		ids = append(ids, id)
		sum += uint64(max(1, f.Level))
	}
	sort.Slice(ids, func(i, j int) bool { return experienceLess(ids[i], ids[j]) })
	result := map[experienceIdentity]uint64{}
	if len(ids) == 0 {
		return result
	}
	var credits uint64
	for _, id := range ids {
		credits += credit[id]
	}
	percent := experiencePercentBase
	if comboUnits >= experienceComboMinimum {
		percent += experienceTwoUnitComboBonus + (comboUnits-experienceComboMinimum)*experienceExtraComboUnitBonus
	}
	pool := uint64(math.RoundToEven(float64(base*credits) * float64(percent) / (experienceFullCredit * experiencePercentBase)))
	if len(ids) == 1 {
		result[ids[0]] = pool
		return result
	}
	var denominator uint64
	for _, id := range ids {
		denominator += credit[id] * (sum - uint64(max(1, unique[id].Level)))
	}
	used := uint64(0)
	remainders := map[experienceIdentity]uint64{}
	for _, id := range ids {
		numerator := pool * credit[id] * (sum - uint64(max(1, unique[id].Level)))
		result[id] = numerator / denominator
		remainders[id] = numerator % denominator
		used += result[id]
	}
	sort.SliceStable(ids, func(i, j int) bool { return remainders[ids[i]] > remainders[ids[j]] })
	for i := uint64(0); i < pool-used; i++ {
		result[ids[i]]++
	}
	return result
}

func (b *Battle) recordExperienceDamage(target, actor *Fighter, damage int) {
	if b.PvP || target == nil || target.Kind != Monster || target.Dead() || target.Captured || damage <= 0 || !validExperienceParticipant(actor) || actor.Side == target.Side {
		return
	}
	if b.experienceDamage == nil {
		b.experienceDamage = map[*Fighter]map[experienceIdentity]*Fighter{}
	}
	if b.experienceDamage[target] == nil {
		b.experienceDamage[target] = map[experienceIdentity]*Fighter{}
	}
	b.experienceDamage[target][actor.experienceID()] = actor
}

func (b *Battle) recordKillExperience(target *Fighter, actors []Action) {
	if b.PvP || target.Kind != Monster || target.Captured || !target.Dead() {
		return
	}
	if b.experienceKills == nil {
		b.experienceKills = map[*Fighter]bool{}
	}
	if b.experienceKills[target] {
		return
	}
	b.experienceKills[target] = true
	var participants []*Fighter
	var supporters []*Fighter
	comboUnits := map[experienceIdentity]bool{}
	for _, a := range actors {
		if !validExperienceParticipant(a.Actor) || a.Actor.Side == target.Side {
			continue
		}
		participants = append(participants, a.Actor)
		comboUnits[a.Actor.experienceID()] = true
		supporters = append(supporters, a.supporters...)
	}
	participants = append(participants, supporters...)
	var earlier []*Fighter
	for _, f := range b.experienceDamage[target] {
		earlier = append(earlier, f)
	}
	delete(b.experienceDamage, target)
	if b.experience == nil {
		b.experience = map[experienceIdentity]uint64{}
	}
	for id, share := range divideWeightedKillExperience(participationMonsterExperience(target.Level), participants, earlier, len(comboUnits)) {
		b.experience[id] += share
	}
}

func (b *Battle) ParticipantEXP(f *Fighter) uint64 {
	if f == nil {
		return 0
	}
	return b.experience[f.experienceID()]
}
func (b *Battle) PetParticipantEXP(owner, id uint32) uint64 {
	return b.experience[experienceIdentity{Pet, owner, game.BroadcastID(id)}]
}

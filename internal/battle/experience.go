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
	participationExperiencePerLevel = 1.5
	participationMinimumMultiplier  = 0.5
	participationMaximumMultiplier  = 3.0
	participationLevelRatioWeight   = 4.0 // Makes equal-level rewards exactly 1x.
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

func participationMonsterExperience(level byte) float64 {
	return float64(max(1, level)) * participationExperiencePerLevel
}

// Apply separately to each participant's share, including pets and assistants.
// The smooth ratio curve is 1 at equal levels and tends to 3/0.5 as the
// participant becomes much lower/higher level than the defeated monster.
func participationLevelMultiplier(participantLevel, monsterLevel byte) float64 {
	ratio := float64(max(1, participantLevel)) / float64(max(1, monsterLevel))
	return participationMinimumMultiplier + (participationMaximumMultiplier-participationMinimumMultiplier)/(1+participationLevelRatioWeight*ratio*ratio)
}

type experienceIdentity struct {
	kind      Kind
	owner, id uint32
}

type experienceContribution struct {
	level  byte
	credit uint64
}

// ExperienceBonus is an unscaled, once-per-battle pool. It uses the same
// inverse-level participation weights, without combo or monster-level bonuses.
type ExperienceBonus map[experienceIdentity]uint64

func (e ExperienceBonus) Participant(f *Fighter) uint64 {
	if f == nil {
		return 0
	}
	return e[f.experienceID()]
}
func (e ExperienceBonus) Pet(owner, id uint32) uint64 {
	return e[experienceIdentity{Pet, owner, game.BroadcastID(id)}]
}

func (b *Battle) ParticipationBonus(perParticipant uint64) ExperienceBonus {
	if perParticipant == 0 || b.PvP {
		return nil
	}
	ids := make([]experienceIdentity, 0, len(b.experienceContributors))
	unique := map[experienceIdentity]*Fighter{}
	credits := map[experienceIdentity]uint64{}
	var sum uint64
	for id, contribution := range b.experienceContributors {
		owner := id.id
		if id.kind == Pet {
			owner = id.owner
		}
		if b.EXPAllowed != nil && !b.EXPAllowed(owner) {
			continue
		}
		ids = append(ids, id)
		unique[id] = &Fighter{Level: contribution.level}
		credits[id] = contribution.credit
		sum += uint64(max(1, contribution.level))
	}
	sort.Slice(ids, func(i, j int) bool { return experienceLess(ids[i], ids[j]) })
	return divideExperiencePool(perParticipant*uint64(len(ids)), ids, unique, credits, sum)
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
	return divideWeightedKillExperience(float64(base), participants, nil, comboUnits)
}

func divideWeightedKillExperience(base float64, participants, earlier []*Fighter, comboUnits int) map[experienceIdentity]uint64 {
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
	pool := uint64(math.RoundToEven(base * float64(credits) * float64(percent) / (experienceFullCredit * experiencePercentBase)))
	return divideExperiencePool(pool, ids, unique, credit, sum)
}

func divideExperiencePool(pool uint64, ids []experienceIdentity, unique map[experienceIdentity]*Fighter, credit map[experienceIdentity]uint64, sum uint64) map[experienceIdentity]uint64 {
	result := ExperienceBonus{}
	if len(ids) == 0 {
		return result
	}
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
	if !b.experienceAllowed(actor) {
		return
	}
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
		if !validExperienceParticipant(a.Actor) || !b.experienceAllowed(a.Actor) || a.Actor.Side == target.Side {
			continue
		}
		participants = append(participants, a.Actor)
		comboUnits[a.Actor.experienceID()] = true
		supporters = append(supporters, a.supporters...)
	}
	for _, f := range supporters {
		if b.experienceAllowed(f) {
			participants = append(participants, f)
		}
	}
	var earlier []*Fighter
	for _, f := range b.experienceDamage[target] {
		if b.experienceAllowed(f) {
			earlier = append(earlier, f)
		}
	}
	delete(b.experienceDamage, target)
	if b.experience == nil {
		b.experience = map[experienceIdentity]uint64{}
	}
	levels := map[experienceIdentity]byte{}
	for _, f := range append(earlier, participants...) {
		if validExperienceParticipant(f) {
			levels[f.experienceID()] = f.Level
		}
	}
	if b.experienceContributors == nil {
		b.experienceContributors = map[experienceIdentity]experienceContribution{}
	}
	record := func(f *Fighter, credit uint64) {
		if !validExperienceParticipant(f) {
			return
		}
		id := f.experienceID()
		previous := b.experienceContributors[id]
		b.experienceContributors[id] = experienceContribution{f.Level, max(credit, previous.credit)}
	}
	for _, f := range earlier {
		record(f, experienceDamageCredit)
	}
	for _, f := range participants {
		record(f, experienceFullCredit)
	}
	for id, share := range divideWeightedKillExperience(participationMonsterExperience(target.Level), participants, earlier, len(comboUnits)) {
		b.experience[id] += uint64(math.RoundToEven(float64(share) * participationLevelMultiplier(levels[id], target.Level)))
	}
}

func (b *Battle) experienceAllowed(f *Fighter) bool {
	if f == nil {
		return false
	}
	if b.EXPAllowed == nil {
		return true
	}
	owner := f.ID
	if f.Kind == Pet {
		owner = f.Owner
	}
	return b.EXPAllowed(owner)
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

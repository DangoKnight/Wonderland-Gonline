// Package battle is the turn-based PvE engine. It computes a whole round at once and
// returns its animation steps; the server plays them with their delays.
// Reference: wlo.pserver.core/Game/Battle/PvEBattleManager.cs.
package battle

import (
	"math"
	"sort"
	"strings"
	"time"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

type Side byte

const (
	Attacker Side = 1 // Right side: players at x 4, pets at x 3.
	Defender Side = 2 // Left side: monsters at x 1..2.
)

type Kind byte

const (
	Player  Kind = 2
	Pet     Kind = 4
	Monster Kind = 7
)

// Fighter is BattleFighter. Char is set for player fighters.
type Fighter struct {
	Side                 Side
	Kind                 Kind
	ID                   uint32
	ClickID              uint16
	Owner                uint32
	Name                 string
	Level, Element       byte
	MaxHP, HP, MaxSP, SP int
	Atk, Def, Matk, Mdef int
	Spd                  int
	Weapon               byte // Item type of the equipped weapon (5 is a staff).
	X, Y                 byte
	Effects              []ActiveEffect
	Captured             bool
	Char                 *game.Character
	Pet                  *game.Pet // A copy of the battle pet; results are applied after the battle.
	Template             uint32    // Monster template.
	Deaths               int       // Times knocked out, for pet amity.
	// Absent marks a disconnected player's fighters. They stay on the field
	// and defend; the turn no longer waits for their commands.
	Absent bool
}

// hurt lowers HP and counts a knock-out.
func (f *Fighter) hurt(damage int) {
	alive := !f.Dead()
	f.HP = max(0, f.HP-damage)
	if damage > 0 {
		f.breakDamageEffects()
	}
	if alive && f.Dead() {
		f.Deaths++
	}
}

func (f *Fighter) Dead() bool { return f.HP <= 0 }

// CanAct reads capability restrictions supplied by active effects.
func (f *Fighter) CanAct() bool {
	for _, e := range f.Effects {
		if e.Turns > 0 && e.Definition.BlockActions {
			return false
		}
	}
	return true
}

func (f *Fighter) key() int { return int(f.X)<<8 | int(f.Y) }

// Action is a submitted command (PendingAction).
type Action struct {
	Actor         *Fighter
	Player        uint32
	Kind          string // attack, defend, catch, flee, heal, effect, monster
	Redirected    bool   // Target selected by an active redirection effect.
	Skill         uint16
	TX, TY        byte
	comboSpeed    int
	speedRecorded bool
}

// Step is a batch of packets for every participant, followed by an animation delay.
type Step struct {
	Packets   [][]byte
	Delay     time.Duration
	SkillUses []Action // Successfully executed actions, for durable proficiency awards.
}

type Outcome byte

const (
	Continue Outcome = iota
	Victory
	Defeat
	Fled
)

// Rules supplies data and randomness. Next(lo, hi) returns lo..hi-1 like System.Random.
type Rules struct {
	// Zero retains the default multiplier for existing rule constructors.
	DropRateMultiplier float64
	Skills             map[uint16]assets.Skill
	Timing             map[uint16]int
	Critical           game.CriticalHits
	Next               func(lo, hi int) int
	Float              func() float64
}

// Battle is ActiveBattle for one party against monsters. PvP is not ported.
type Battle struct {
	Attackers, Defenders []*Fighter
	Pending              map[int]Action
	Turn                 int
	Processing, Finished bool
	// Roster lists each player's pet templates, for capture limits; Captures records
	// the monsters caught, to be added after the battle.
	Roster   map[uint32][]uint32
	Captures []Capture
}

// Capture is a caught monster for its owner's party.
type Capture struct {
	Owner    uint32
	Template uint32
	Name     string
	Level    int
}

// Grid positions (PvEBattleManager.*GridSlots).
var (
	playerSlots = [][2]byte{{4, 2}, {4, 3}, {4, 1}, {4, 4}}
	petSlots    = [][2]byte{{3, 2}, {3, 3}, {3, 1}, {3, 4}}
	enemySlots  = [][2]byte{{2, 2}, {2, 3}, {2, 1}, {2, 4}, {1, 2}, {1, 3}, {1, 1}, {1, 4}}
)

// MaxEnemies is the number of enemy grid slots.
const MaxEnemies = 8

// Enemy describes one monster to spawn.
type Enemy struct {
	Template uint32
	Name     string
	Level    int
	HP       int
	Element  byte
	ClickID  uint16
}

// MonsterStats are the derived monster attributes of StartPvEBattle/ApplyQuestTemplate.
func MonsterStats(level int) (sp, atk, def, spd int) {
	r := func(v float64) int { return int(math.RoundToEven(v)) }
	return level*20 + 50, r(float64(level)*1.5 + 5), r(float64(level)*1.2 + 3), r(float64(level)*1.3 + 4)
}

// PlayerFighter is BuildFighters for a character (no pets).
func PlayerFighter(c *game.Character, items map[uint16]game.ItemDefinition, slot int, growth ...game.ElementalGrowth) *Fighter {
	full := c.Combat(items, growth...)
	f := &Fighter{Side: Attacker, Kind: Player, ID: c.ID, Name: c.Name, Level: c.Level, Element: c.Element,
		MaxHP: max(1, int(full.MaxHP)), HP: max(1, int(c.HP)), MaxSP: max(0, int(full.MaxSP)), SP: max(0, int(c.SP)),
		Atk: int(full.ATK), Def: int(full.DEF), Matk: int(full.MAT), Mdef: int(full.MDF), Spd: int(full.SPD),
		X: playerSlots[slot][0], Y: playerSlots[slot][1], Char: c}
	if weapon := c.Equipment[2]; weapon.ID != 0 {
		f.Weapon = items[weapon.ID].Type
	}
	return f
}

// PetFighter is BuildFighters for an owner's battle pet.
func PetFighter(p game.Pet, owner uint32, items map[uint16]game.ItemDefinition, slot int) *Fighter {
	full := p.Combat(items)
	f := &Fighter{Side: Attacker, Kind: Pet, ID: game.BroadcastID(p.ID), Owner: owner, Name: p.Name, Level: p.Level, Element: p.Element(),
		MaxHP: max(1, int(p.MaxHP)), HP: max(1, int(p.HP)), MaxSP: max(0, int(p.MaxSP)), SP: max(0, int(p.SP)),
		Atk: int(full.ATK), Def: int(full.DEF), Matk: int(full.MAT), Mdef: int(full.MDF), Spd: int(full.SPD),
		X: petSlots[slot][0], Y: petSlots[slot][1], Pet: &p}
	if weapon := p.Equipment[2]; weapon.ID != 0 {
		f.Weapon = items[weapon.ID].Type
	}
	return f
}

// New builds a battle of one or more characters against enemies.
func New(players []*Fighter, enemies []Enemy) *Battle {
	b := &Battle{Attackers: players, Pending: map[int]Action{}, Roster: map[uint32][]uint32{}}
	for i, e := range enemies {
		if i == MaxEnemies {
			break
		}
		sp, atk, def, spd := MonsterStats(e.Level)
		click := e.ClickID
		if click == 0 {
			click = uint16(2000 + i)
		}
		b.Defenders = append(b.Defenders, &Fighter{Side: Defender, Kind: Monster, ID: e.Template, Template: e.Template, ClickID: click, Name: e.Name,
			Level: byte(min(255, e.Level)), Element: e.Element, MaxHP: max(1, e.HP), HP: max(1, e.HP), MaxSP: sp, SP: sp,
			Atk: atk, Def: def, Matk: atk, Mdef: def, Spd: spd, X: enemySlots[i][0], Y: enemySlots[i][1]})
	}
	return b
}

func record(p protocol.Builder, side byte, f *Fighter, kind byte, click uint16, owner uint32) protocol.Builder {
	return p.U8(side).U8(kind).U32(f.ID).U16(click).U32(owner).U8(f.X).U8(f.Y).U32(uint32(f.MaxHP)).U16(uint16(min(0xffff, f.MaxSP))).
		U32(uint32(f.HP)).U16(uint16(min(0xffff, f.SP))).U8(f.Level).U8(f.Element).U8(0).U8(0).U16(0)
}

// StatSync is AC51:1 for one fighter's stat.
func StatSync(x, y, stat byte, value uint32) []byte {
	return protocol.Builder{51, 1, x, y, stat}.U32(value)
}

// Intro is InitializeAndStartBattle's sequence for one player, after the map-side
// AC11:4 indicator: enter battle, own fighter, enemies, HP/SP sync, then the action menu.
func (b *Battle) Intro(self *Fighter, background uint16) [][]byte {
	out := [][]byte{{protocol.CommandEvent, protocol.EventBattleBegin}, {protocol.CommandMovement, protocol.MovementMovementLock, 1}}
	out = append(out, record(protocol.Builder{protocol.CommandBattleState, protocol.BattleStateFormation}.U16(background), byte(self.Side), self, 2, 0, 0), []byte{protocol.CommandBattleState, protocol.BattleStateInitialize, 1})
	for _, f := range b.Attackers {
		if f != self && f.Kind == Player {
			out = append(out, record(protocol.Builder{protocol.CommandBattleState, protocol.BattleStateParticipant}, byte(f.Side), f, 2, 0, 0))
		}
	}
	for _, f := range b.Attackers {
		if f.Kind == Pet {
			// Side byte 5 marks a pet fighter.
			out = append(out, record(protocol.Builder{protocol.CommandBattleState, protocol.BattleStateParticipant}, 5, f, byte(Pet), 0, f.Owner))
		}
	}
	for _, f := range b.Defenders {
		out = append(out, record(protocol.Builder{protocol.CommandBattleState, protocol.BattleStateParticipant}, byte(f.Side), f, byte(f.Kind), f.ClickID, f.Owner))
	}
	for _, f := range b.all() {
		out = append(out, StatSync(f.X, f.Y, 0x19, uint32(f.HP)), StatSync(f.X, f.Y, 0x1a, uint32(f.SP)))
	}
	return append(out, []byte{protocol.CommandBattleAction, protocol.BattleActionTurn, self.X, self.Y, 0}, []byte{protocol.CommandBattleReady, protocol.BattleReadyReady})
}

func (b *Battle) all() []*Fighter {
	return append(append([]*Fighter(nil), b.Attackers...), b.Defenders...)
}

// Expected is ExpectedActionCount: one command per living player and living pet.
func (b *Battle) Expected() int {
	n := 0
	for _, f := range b.Attackers {
		if (f.Kind == Player || f.Kind == Pet) && !f.Dead() && !f.Absent {
			n++
		}
	}
	return max(1, n)
}

// Leave marks a player's fighters absent, dropping their unplayed commands.
func (b *Battle) Leave(player uint32) {
	for _, f := range b.Attackers {
		if (f.Kind == Player && f.ID == player) || (f.Kind == Pet && f.Owner == player) {
			f.Absent = true
			delete(b.Pending, f.key())
		}
	}
}

// Present reports whether any player fighter still has a connected owner.
func (b *Battle) Present() bool {
	for _, f := range b.Attackers {
		if f.Kind == Player && !f.Absent {
			return true
		}
	}
	return false
}

// Basic reports the native weapon-attack skill IDs.
func Basic(skill uint16) bool {
	return (skill >= basicAttackSkill && skill <= 10007) || skill == 10010 || skill == 10018 || skill == 10019 || skill == 10020 || skill == 10023 || skill == 10026 || skill == 10027
}

const (
	basicAttackSkill = 10001
	defendSkill      = 60021
	fleeSkill        = 60041
	catchSkill       = 10008
	catchFailure     = 10009
	stuntAlias       = 15003
)

// Skill effect classes come from names: the C# revision never loads the numeric
// target and effect fields, so they are zero.
func has(name string, words ...string) bool {
	for _, w := range words {
		if strings.Contains(name, w) {
			return true
		}
	}
	return false
}

func isHeal(s assets.Skill) bool   { return has(s.Name, "Heal", "Recover", "Blessing", "Cure", "Rest") }
func isRevive(s assets.Skill) bool { return has(s.Name, "Reviv", "Restoration") }

// learned returns a character's grade of a skill, accepting the stunt alias.
func learned(c *game.Character, skill uint16) (game.LearnedSkill, bool) {
	stunt := game.StarterStunt(c.Body, c.Head)
	for _, s := range c.Skills {
		if s.Grade > 0 && (s.ID == skill || (skill == stuntAlias && s.ID == stunt)) {
			return s, true
		}
	}
	return game.LearnedSkill{}, false
}

func petLearned(p *game.Pet, skill uint16) (game.PetSkill, bool) {
	for _, s := range p.Skills {
		if s.ID == skill && s.Grade > 0 {
			return s, true
		}
	}
	return game.PetSkill{}, false
}

// CanUse is CanUseSkill for player, pet and monster fighters.
func (r Rules) CanUse(f *Fighter, skill uint16) bool {
	if f == nil || f.Dead() || !f.CanAct() {
		return false
	}
	if Basic(skill) || skill == defendSkill || skill == fleeSkill {
		return true
	}
	if skill == catchSkill {
		return f.Char != nil || f.Pet != nil
	}
	s, ok := r.Skills[skill]
	if !ok {
		return false
	}
	if f.Pet != nil {
		_, ok := petLearned(f.Pet, skill)
		return ok && f.SP >= int(s.SP)
	}
	if f.Char != nil {
		_, ok := learned(f.Char, skill)
		return ok && f.SP >= int(s.SP)
	}
	return true
}

// Submit is HandleBattleAction: the client names the acting grid cell, a target cell
// and a skill (default basic attack). It returns the AC53:5 acknowledgment, or nil when
// the command is ignored.
func (b *Battle) Submit(r Rules, player uint32, sub byte, data []byte) []byte {
	if b.Finished || b.Processing {
		return nil
	}
	var sx, sy, tx, ty byte
	skill := uint16(basicAttackSkill)
	if len(data) >= 4 {
		sx, sy, tx, ty = data[0], data[1], data[2], data[3]
		data = data[4:]
	}
	if len(data) >= 2 {
		if v := uint16(data[0]) | uint16(data[1])<<8; v > 0 {
			skill = v
		}
	}
	var actor *Fighter
	for _, f := range b.all() {
		if f.X == sx && f.Y == sy && ((f.Char != nil && f.Char.ID == player) || f.Owner == player) {
			actor = f
			break
		}
	}
	if actor == nil || actor.Dead() {
		return nil
	}
	if _, dup := b.Pending[actor.key()]; dup {
		return nil
	}
	if skill == stuntAlias && actor.Char != nil {
		if s, ok := learned(actor.Char, stuntAlias); ok {
			skill = s.ID
		}
	}
	kind := "attack"
	s, known := r.Skills[skill]
	switch {
	case sub == 5 || skill == fleeSkill:
		kind, skill = "flee", fleeSkill
	case sub == 4 || skill == defendSkill:
		kind, skill = "defend", defendSkill
	case skill == catchSkill:
		kind = "catch"
	case known && hasCastEffects(s.Effects):
		kind = "effect"
	case known && (isHeal(s) || isRevive(s)):
		kind = "heal"
	}
	// An invalid or stale command still consumes the turn, as Defend.
	if !r.CanUse(actor, skill) {
		kind, skill = "defend", defendSkill
	}
	b.Pending[actor.key()] = Action{Actor: actor, Player: player, Kind: kind, Skill: skill, TX: tx, TY: ty}
	return []byte{protocol.CommandBattleEffect, protocol.BattleEffectEscape, sx, sy}
}

// Ready reports whether every expected command has arrived.
func (b *Battle) Ready() bool {
	return !b.Finished && !b.Processing && len(b.Pending) >= b.Expected()
}

// Timeout is OnTurnTimeout: missing commands become Defend.
func (b *Battle) Timeout() {
	for _, f := range b.Attackers {
		if !f.Dead() && (f.Kind == Player || f.Kind == Pet) {
			if _, ok := b.Pending[f.key()]; !ok {
				player := f.ID
				if f.Kind == Pet {
					player = f.Owner
				}
				b.Pending[f.key()] = Action{Actor: f, Player: player, Kind: "defend", Skill: defendSkill}
			}
		}
	}
}

// Delay is GetAnimationDelayMs.
func (r Rules) Delay(skill uint16, kind string) time.Duration {
	switch kind {
	case "defend":
		return 700 * time.Millisecond
	case "exit", "flee":
		return 2500 * time.Millisecond
	case "catch":
		return 6000 * time.Millisecond
	}
	if skill == 0 {
		skill = basicAttackSkill
	}
	basic := Basic(skill) && kind != "heal" && kind != "buff" && kind != "status"
	fallback, margin := 3400, 400
	if basic {
		fallback, margin = 1600, 300
	}
	return time.Duration(assets.AnimationDelay(r.Timing, skill, fallback, margin)) * time.Millisecond
}

// Elemental is GetElementalMultiplier (Earth 1, Water 2, Fire 3, Wind 4).
func Elemental(hitter, target byte) float64 {
	switch {
	case hitter == 1 && target == 2, hitter == 2 && target == 3, hitter == 4 && target == 1:
		return 1.7
	case hitter == 3 && target == 4:
		return 1.5
	case hitter == 1 && target == 4, hitter == 2 && target == 1, hitter == 3 && target == 2, hitter == 4 && target == 3:
		return 0.6
	}
	return 1
}

// baseDamage is CalculateBaseDamage.
func (r Rules) baseDamage(actor, target *Fighter, skill uint16, roll int) int {
	s, ok := r.Skills[skill]
	if Basic(skill) || !ok {
		if actor.Weapon == 5 { // A staff strikes with magic.
			return max(10, actor.magicAttack()*2-target.magicDefense()+roll)
		}
		return max(10, actor.attack()*2-target.defense()+roll)
	}
	if s.EffectLayer != assets.SkillLayerPhysical && s.EffectLayer != assets.SkillLayerMagical {
		return max(15, int(float64(actor.attack())*2.8)-target.defense()/2)
	}
	grade := 1
	if actor.Pet != nil {
		if l, ok := petLearned(actor.Pet, skill); ok {
			grade = max(1, min(int(l.Grade), 10))
		}
	} else if actor.Char != nil {
		if l, ok := learned(actor.Char, skill); ok {
			grade = max(1, min(int(l.Grade), 10))
		}
	}
	offense, defense := actor.attack(), target.defense()
	if s.EffectLayer == assets.SkillLayerMagical {
		offense, defense = actor.magicAttack(), target.magicDefense()
	}
	damage := float64(offense)*s.StatMultiplier + float64(grade)*s.PowerPerLevel + float64(s.AdditionalDamage) - float64(defense)/2
	return int(math.Max(1, math.Min(math.MaxInt32, math.Floor(damage))))
}

func hitRecord(p protocol.Builder, actor, target *Fighter, skill uint16, defended bool, stat byte, amount int, mode byte) protocol.Builder {
	return resultRecord(p, actor, target, skill, protocol.BattleHitLanded, defended, stat, amount, mode)
}

func missRecord(p protocol.Builder, actor, target *Fighter, skill uint16) protocol.Builder {
	return resultRecord(p, actor, target, skill, protocol.BattleHitMiss, false, game.StatCurrentHP, 0, effectNormalHitMode)
}

func resultRecord(p protocol.Builder, actor, target *Fighter, skill uint16, result byte, defended bool, stat byte, amount int, mode byte) protocol.Builder {
	d := byte(0)
	if defended {
		d = 1
	}
	return p.U8(0x11).U8(0).U8(actor.X).U8(actor.Y).U16(skill).U8(0).U8(1).U8(target.X).U8(target.Y).U8(result).U8(d).U8(1).U8(stat).U32(uint32(amount)).U8(mode)
}

func turnPacket(f *Fighter) []byte {
	return []byte{protocol.CommandBattleAction, protocol.BattleActionTurn, f.X, f.Y, 0}
}

// order snapshots effective speed for this round. Support actions and enemy
// turns remain barriers: a combo cannot move an attacker ahead of either.
func (b *Battle) order(r Rules, actions []Action) [][]Action {
	var ordered []Action
	for _, a := range actions {
		switch a.Kind {
		case "attack", "status", "catch", "heal", "buff", "effect":
			ordered = append(ordered, a)
		}
	}
	for _, m := range b.Defenders {
		ordered = append(ordered, Action{Actor: m, Kind: "monster"})
	}
	for i := range ordered {
		ordered[i].comboSpeed = ordered[i].Actor.speed()
		ordered[i].speedRecorded = true
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		a, c := ordered[i], ordered[j]
		if a.comboSpeed != c.comboSpeed {
			return a.comboSpeed > c.comboSpeed
		}
		if a.Actor.X != c.Actor.X {
			return a.Actor.X < c.Actor.X
		}
		return a.Actor.Y < c.Actor.Y
	})
	return r.comboGroups(ordered)
}

func living(fs []*Fighter) []*Fighter {
	var out []*Fighter
	for _, f := range fs {
		if !f.Dead() {
			out = append(out, f)
		}
	}
	return out
}

func allDead(fs []*Fighter) bool { return len(living(fs)) == 0 }

func at(fs []*Fighter, x, y byte, alive bool) *Fighter {
	for _, f := range fs {
		if f.X == x && f.Y == y && (!alive || !f.Dead()) {
			return f
		}
	}
	return nil
}

func (r Rules) spend(f *Fighter, skill uint16) []byte {
	cost := 15
	if s, ok := r.Skills[skill]; ok {
		cost = int(s.SP)
	}
	f.SP = max(0, f.SP-cost)
	return StatSync(f.X, f.Y, 0x1a, uint32(f.SP))
}

// Round is ExecuteTurn: it applies one round and returns its steps. A non-Continue
// outcome ends the battle; the caller then runs the matching ending.
func (b *Battle) Round(r Rules) ([]Step, Outcome) {
	b.Turn++
	for _, f := range b.Attackers {
		if f.Absent && !f.Dead() {
			if _, ok := b.Pending[f.key()]; !ok {
				player := f.ID
				if f.Kind == Pet {
					player = f.Owner
				}
				b.Pending[f.key()] = Action{Actor: f, Player: player, Kind: "defend", Skill: defendSkill}
			}
		}
	}
	actions := make([]Action, 0, len(b.Pending))
	keys := make([]int, 0, len(b.Pending))
	for k := range b.Pending {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	for _, k := range keys {
		actions = append(actions, b.Pending[k])
	}
	b.Pending = map[int]Action{}
	var steps []Step
	emit := func(delay time.Duration, packets ...[]byte) {
		steps = append(steps, Step{Packets: packets, Delay: delay})
	}
	// Phase 0: periodic damage from active effects.
	for _, f := range b.all() {
		if damage := f.periodicDamage(); !f.Dead() && damage > 0 {
			f.hurt(damage)
			packets := [][]byte{StatSync(f.X, f.Y, 0x19, uint32(f.HP))}
			if f.Dead() {
				packets = append(packets, []byte{protocol.CommandBattleEffect, protocol.BattleEffectDefeated, f.X, f.Y})
			}
			emit(0, packets...)
		}
	}
	for _, a := range actions {
		if a.Kind == "flee" && a.Actor != nil && !a.Actor.Dead() {
			return steps, Fled
		}
	}
	defending := map[int]bool{}
	for _, a := range actions {
		if a.Kind == "defend" && !a.Actor.Dead() && a.Actor.CanAct() {
			defending[a.Actor.key()] = true
		}
	}
	for _, step := range b.order(r, actions) {
		for _, a := range step {
			if a.Kind == "effect" {
				steps = append(steps, r.abilityEffect(b, a))
				continue
			}
			if a.Kind == "heal" || a.Kind == "buff" {
				steps = append(steps, r.support(b, a))
			}
		}
		for _, a := range step {
			if a.Kind == "catch" {
				if s, ok := r.capture(b, a); ok {
					steps = append(steps, s...)
				}
			}
		}
		var offensive []Action
		for _, a := range step {
			if a.Kind == "attack" || a.Kind == "status" {
				offensive = append(offensive, a)
			}
		}
		if len(offensive) > 0 {
			s, won := r.attack(b, offensive, defending)
			steps = append(steps, s...)
			if won {
				return steps, Victory
			}
		}
		if allDead(b.Defenders) {
			return steps, Victory
		}
		for _, a := range step {
			if a.Kind == "monster" && !a.Actor.Dead() {
				steps = append(steps, r.monster(b, a.Actor, defending)...)
			}
		}
		if allDead(b.Attackers) {
			return steps, Defeat
		}
	}
	for _, f := range b.all() {
		f.tickEffects()
	}
	return steps, Continue
}

func (r Rules) support(b *Battle, a Action) Step {
	actor := a.Actor
	if !r.CanUse(actor, a.Skill) {
		return Step{}
	}
	s := r.Skills[a.Skill]
	packets := [][]byte{r.spend(actor, a.Skill)}
	target := at(b.Attackers, a.TX, a.TY, false)
	if target == nil {
		if alive := living(b.Attackers); len(alive) > 0 {
			target = alive[0]
		} else {
			target = actor
		}
	}
	packets = append(packets, turnPacket(actor))
	stat, amount := byte(0), 0
	switch {
	case isRevive(s) && target.Dead():
		amount = max(50, target.MaxHP/3)
		target.HP, stat = amount, 0x19
	case isHeal(s):
		amount = max(40, int(float64(actor.attack())*2.2)+int(actor.Level)*15)
		target.HP, stat = min(target.MaxHP, target.HP+amount), 0x19
	}
	packets = append(packets, hitRecord(protocol.Builder{protocol.CommandBattleAction, protocol.BattleActionAnimation}, actor, target, a.Skill, false, stat, amount, 1))
	if stat != 0 {
		packets = append(packets, StatSync(target.X, target.Y, 0x19, uint32(target.HP)))
	}
	return Step{Packets: packets, Delay: r.Delay(a.Skill, a.Kind), SkillUses: []Action{a}}
}

// capture is the catch phase. The chance rises with the owner's level advantage and
// the monster's lost HP; a full or duplicate roster always fails.
func (r Rules) capture(b *Battle, a Action) ([]Step, bool) {
	if a.Actor.Dead() || !a.Actor.CanAct() {
		return nil, false
	}
	target := at(b.Defenders, a.TX, a.TY, true)
	if target == nil {
		return nil, false
	}
	owner := a.Player
	level := 1
	for _, f := range b.Attackers {
		if f.Kind == Player && f.ID == owner {
			level = int(f.Level)
		}
	}
	monster := int(target.Level)
	chance := 75 + float64(level-monster)*5 + (1-float64(target.HP)/float64(max(1, target.MaxHP)))*20
	if level >= monster {
		chance = math.Max(85, chance)
	}
	chance = math.Min(98, math.Max(15, chance))
	roster := b.Roster[owner]
	duplicate := false
	for _, id := range roster {
		duplicate = duplicate || game.SamePet(id, target.Template)
	}
	full := len(roster) >= game.MaxPets
	reason := ""
	switch {
	case full && duplicate:
		reason = "Your pet team is full (4/4), and you already have this monster."
	case full:
		reason = "Your pet team is full (4/4). Make room before capturing."
	case duplicate:
		reason = "You already have this monster in your pet team."
	}
	roll := r.Float() * 100
	success := reason == "" && roll < chance
	if !success && reason == "" {
		reason = "The monster resisted capture. Try weakening it further."
	}
	skill := uint16(catchFailure)
	if success {
		skill = catchSkill // Its animation removes the target in the client.
	}
	steps := []Step{{Packets: [][]byte{turnPacket(a.Actor), hitRecord(protocol.Builder{protocol.CommandBattleAction, protocol.BattleActionAnimation}, a.Actor, target, skill, false, 0, 0, 1)}, Delay: r.Delay(catchSkill, "catch")}}
	if !success {
		steps = append(steps, Step{Packets: [][]byte{systemLine("Cannot capture " + target.Name + ": " + reason)}})
		return steps, true
	}
	target.HP, target.Captured = 0, true
	b.Roster[owner] = append(b.Roster[owner], target.Template)
	b.Captures = append(b.Captures, Capture{Owner: owner, Template: target.Template, Name: target.Name, Level: int(target.Level)})
	return steps, true
}

// systemLine is AC23:57 with a length-prefixed message.
func systemLine(text string) []byte {
	p, _ := protocol.Builder{protocol.CommandInventory, protocol.InventoryMessage, 0}.String(text)
	return p
}

func (r Rules) attack(b *Battle, group []Action, defending map[int]bool) ([]Step, bool) {
	var steps []Step
	type targetKey struct {
		x, y       byte
		side       Side
		redirected bool
	}
	byTarget := map[targetKey][]Action{}
	var order []targetKey
	for _, a := range group {
		if r.CanUse(a.Actor, a.Skill) {
			if target := r.redirectedTarget(b, a.Actor); target != nil {
				a.TX, a.TY = target.X, target.Y
				a.Redirected = true
			}
		}
		k := targetKey{a.TX, a.TY, a.Actor.Side, a.Redirected}
		if _, seen := byTarget[k]; !seen {
			order = append(order, k)
		}
		byTarget[k] = append(byTarget[k], a)
	}
	for _, k := range order {
		targets := b.Defenders
		if byTarget[k][0].Actor.Side == Defender {
			targets = b.Attackers
		}
		if byTarget[k][0].Redirected {
			targets = b.Attackers
			if byTarget[k][0].Actor.Side == Defender {
				targets = b.Defenders
			}
		}
		target := at(targets, k.x, k.y, true)
		if target == nil {
			if byTarget[k][0].Redirected {
				continue
			}
			alive := living(targets)
			if len(alive) == 0 {
				return steps, true
			}
			target = alive[0]
		}
		var valid []Action
		for _, a := range byTarget[k] {
			if !a.Actor.Absent && r.CanUse(a.Actor, a.Skill) {
				valid = append(valid, a)
			}
		}
		groups := r.comboGroups(valid)
		for len(groups) > 0 {
			combo := groups[0]
			groups = groups[1:]
			if target.Dead() {
				if byTarget[k][0].Redirected {
					break
				}
				alive := living(targets)
				if len(alive) == 0 {
					break
				}
				target = alive[0]
			}
			if len(combo) > 1 && !r.comboSucceeds(b, combo[0].Actor.Side, target) {
				// A failed chain executes as singles in its existing speed order.
				// Do not reroll smaller subsets or retain a lost bridge bonus.
				singles := make([][]Action, 0, len(combo)-1+len(groups))
				for _, a := range combo[1:] {
					singles = append(singles, []Action{a})
				}
				groups = append(singles, groups...)
				combo = combo[:1]
			}
			var packets [][]byte
			for _, a := range combo {
				packets = append(packets, turnPacket(a.Actor))
			}
			anim := protocol.Builder{protocol.CommandBattleAction, protocol.BattleActionAnimation}
			total := 0
			var delay time.Duration
			var landed []Action
			for _, a := range combo {
				actor := a.Actor
				if a.Skill > 0 && !Basic(a.Skill) {
					packets = append(packets, r.spend(actor, a.Skill))
				}
				delay = max(delay, r.Delay(a.Skill, a.Kind))
				if r.attackMisses(actor, target, a.Skill) {
					skill := a.Skill
					if skill == 0 {
						skill = basicAttackSkill
					}
					anim = missRecord(anim, actor, target, skill)
					continue
				}
				landed = append(landed, a)
				dmg := r.baseDamage(actor, target, a.Skill, r.Next(1, 6))
				dmg = max(1, int(float64(dmg)*Elemental(actor.Element, target.Element)))
				dmg = max(1, actor.modified(assets.EffectDamageDealt, dmg))
				if len(combo) > 1 {
					dmg = int(min(float64(dmg)*comboDamageMultiplier, math.MaxInt32))
				}
				// Players and pets use their own equipment's critical chance.
				crit := false
				var equipment *game.Equipment
				if actor.Pet != nil {
					equipment = &actor.Pet.Equipment
				} else if actor.Char != nil {
					equipment = &actor.Char.Equipment
				}
				if equipment != nil {
					var d int32
					d, crit = r.Critical.Damage(int32(dmg), equipment.IDs(), a.Kind, r.Next(0, 100))
					dmg = int(d)
				}
				guarded := defending[target.key()]
				dmg = max(1, r.incomingDamage(target, a.Skill, dmg))
				if guarded {
					dmg = max(1, dmg/2)
				}
				total = int(min(int64(total)+int64(dmg), int64(math.MaxInt32)))
				skill := a.Skill
				if skill == 0 {
					skill = basicAttackSkill
				}
				mode := byte(1)
				if crit {
					mode = 2 // Enlarged critical digits.
				}
				anim = hitRecord(anim, actor, target, skill, guarded, 0x19, dmg, mode)
			}
			packets = append(packets, anim)
			target.hurt(total)
			for _, a := range landed {
				r.applyHitEffects(a.Actor, target, a.Skill)
			}
			steps = append(steps, Step{Packets: packets, Delay: delay, SkillUses: combo})
			if target.Dead() {
				steps = append(steps, Step{Packets: [][]byte{{protocol.CommandBattleEffect, protocol.BattleEffectDefeated, target.X, target.Y}}})
			}
		}
	}
	return steps, false
}

func (r Rules) monster(b *Battle, m *Fighter, defending map[int]bool) []Step {
	if !m.CanAct() {
		return nil
	}
	targets := living(b.Attackers)
	if len(targets) == 0 {
		return nil
	}
	if chance := m.allyAttackChance(); chance > 0 && r.Next(0, 100) < chance {
		var allies []*Fighter
		for _, o := range living(b.Defenders) {
			if o != m {
				allies = append(allies, o)
			}
		}
		if len(allies) > 0 {
			t := allies[r.Next(0, len(allies))]
			if r.attackMisses(m, t, basicAttackSkill) {
				return []Step{{Packets: [][]byte{turnPacket(m), missRecord(protocol.Builder{protocol.CommandBattleAction, protocol.BattleActionAnimation}, m, t, basicAttackSkill)}, Delay: r.Delay(basicAttackSkill, "attack")}}
			}
			dmg := max(5, int(float64(m.attack())*1.2)-t.defense())
			dmg = max(1, m.modified(assets.EffectDamageDealt, dmg))
			dmg = max(1, r.incomingDamage(t, basicAttackSkill, dmg))
			anim := hitRecord(protocol.Builder{protocol.CommandBattleAction, protocol.BattleActionAnimation}, m, t, basicAttackSkill, defending[t.key()], 0x19, dmg, 1)
			t.hurt(dmg)
			steps := []Step{{Packets: [][]byte{turnPacket(m), anim}, Delay: r.Delay(basicAttackSkill, "attack")}}
			if t.Dead() {
				steps = append(steps, Step{Packets: [][]byte{{protocol.CommandBattleEffect, protocol.BattleEffectDefeated, t.X, t.Y}}})
			}
			return steps
		}
	}
	t := targets[r.Next(0, len(targets))]
	if r.attackMisses(m, t, basicAttackSkill) {
		return []Step{{Packets: [][]byte{turnPacket(m), missRecord(protocol.Builder{protocol.CommandBattleAction, protocol.BattleActionAnimation}, m, t, basicAttackSkill)}, Delay: r.Delay(basicAttackSkill, "attack")}}
	}
	dmg := max(5, int(float64(m.attack())*1.2)-t.defense())
	dmg = max(1, m.modified(assets.EffectDamageDealt, dmg))
	dmg = max(1, r.incomingDamage(t, basicAttackSkill, dmg))
	guarded := defending[t.key()]
	if guarded {
		dmg = max(1, int(float64(dmg)*0.4))
	}
	anim := hitRecord(protocol.Builder{protocol.CommandBattleAction, protocol.BattleActionAnimation}, m, t, basicAttackSkill, guarded, 0x19, dmg, 1)
	t.hurt(dmg)

	steps := []Step{{Packets: [][]byte{turnPacket(m), anim}, Delay: r.Delay(basicAttackSkill, "attack")}}
	if t.Dead() {
		steps = append(steps, Step{Packets: [][]byte{{protocol.CommandBattleEffect, protocol.BattleEffectDefeated, t.X, t.Y}}})
	}
	return steps
}

// NextRound is Phase 7: reopen the action menu for each living player.
func (b *Battle) NextRound(player uint32) [][]byte {
	for _, f := range b.Attackers {
		if f.Kind == Player && f.ID == player && !f.Dead() {
			return [][]byte{turnPacket(f), {protocol.CommandBattleReady, protocol.BattleReadyReady}}
		}
	}
	// A knocked-out player commands through its pet.
	for _, f := range b.Attackers {
		if f.Kind == Pet && f.Owner == player && !f.Dead() {
			return [][]byte{turnPacket(f), {protocol.CommandBattleReady, protocol.BattleReadyReady}}
		}
	}
	return nil
}

// Departure is the leave animation shared by victory and defeat (AC11:12, AC11:1).
func (b *Battle) Departure(victory bool) [][]byte {
	out := [][]byte{{protocol.CommandBattleState, protocol.BattleStateFinish, 1}}
	if victory {
		for _, f := range b.Attackers {
			if f.Kind == Pet {
				out = append(out, []byte{protocol.CommandBattleState, protocol.BattleStateExit, f.X, f.Y})
			}
		}
		for _, f := range b.Attackers {
			if f.Kind == Player {
				out = append(out, []byte{protocol.CommandBattleState, protocol.BattleStateExit, f.X, f.Y, 0})
			}
		}
		return out
	}
	for _, f := range b.all() {
		out = append(out, []byte{protocol.CommandBattleState, protocol.BattleStateExit, f.X, f.Y, 0})
	}
	return out
}

// Escape is EndBattleFlee's leave animation for living allies.
func (b *Battle) Escape() [][]byte {
	var out [][]byte
	for _, kind := range []Kind{Pet, Player} {
		for _, f := range living(b.Attackers) {
			if f.Kind == kind {
				out = append(out, []byte{protocol.CommandBattleState, protocol.BattleStateExit, f.X, f.Y, 0})
			}
		}
	}
	return out
}

// Rewards are the victory totals: EXP and gold per defeated monster.
func (b *Battle) Rewards() (exp, gold uint64) {
	for _, m := range b.Defenders {
		if m.Captured {
			continue
		}
		exp += uint64(max(10, int(m.Level)*15))
		gold += uint64(max(5, int(m.Level)*8))
	}
	return exp, gold
}

// Drop is a rolled loot item.
type Drop struct {
	Item  uint16
	Count byte
}

const (
	DefaultDropRateMultiplier = 1.0
	MinDropRateMultiplier     = 0.1
	MaxDropRateMultiplier     = 100.0
	dropRateCalibration       = 0.35
	dropRateFloorPercent      = 5.0
	dropPercentScale          = 100.0
	dropNativeItemIDLimit     = 65000
)

// RollDrops is MonsterDropManager.RollDrops for one monster: configured entries that
// also appear in the monster's native Npc.dat slots, calibrated to 35% of the configured
// rate times the live multiplier with a 5% floor, and at most one entry.
func (r Rules) RollDrops(table []assets.Drop, native [5]uint16, known func(uint16) bool) []Drop {
	multiplier := r.DropRateMultiplier
	if multiplier == 0 || math.IsNaN(multiplier) || math.IsInf(multiplier, 0) {
		multiplier = DefaultDropRateMultiplier
	}
	multiplier = min(MaxDropRateMultiplier, max(MinDropRateMultiplier, multiplier))
	for _, e := range table {
		inNative := false
		for _, id := range native {
			if id > 0 && id < dropNativeItemIDLimit && id == e.Item {
				inNative = true
			}
		}
		if !inNative || e.Rate <= 0 || e.Min < 1 || e.Max < e.Min || e.Max > game.MaxItemStack || math.IsNaN(e.Rate) || math.IsInf(e.Rate, 0) {
			continue
		}
		roll := r.Float() * dropPercentScale
		if roll > math.Max(dropRateFloorPercent, e.Rate*dropRateCalibration*multiplier) || !known(e.Item) {
			continue
		}
		count := e.Min
		if e.Max > e.Min {
			count = byte(r.Next(int(e.Min), int(e.Max)+1))
		}
		return []Drop{{e.Item, count}}
	}
	return nil
}

# Investigation: original-game burst EXP

## Finding

The described setup matches the original game's **bursting** mechanic. The
user confirms that Water's Goddess skill debuffs the enemy, Fire's Goddess
skill boosts the attackers, and higher final damage produces higher EXP.

Neither the sibling C# emulator nor the Go port currently implements this
reward behavior. Both use monster-level-based victory rewards. The original
commercial server implementation is not present in either project, so the
historical reward formula and whether it began as a bug remain unverified.
No gameplay formula was changed during this investigation.

## Evidence in the projects

- `internal/battle/battle.go`, `Battle.Rewards`: each uncaptured defender
  contributes `max(10, monsterLevel * 15)` EXP. Damage, player/pet level,
  buffs, support actions, speed and combo participation do not enter the formula.
- `internal/server/combat.go`, `endBattle`: each active winning member receives
  the same total, and their participating pet receives the same base EXP.
- `Wonderland-Private-Server/wlo.pserver.core/Game/Battle/PvEBattleManager.cs`,
  `EndBattleVictory`: the same monster-level formula; each winning player and
  pet receives the total, with the configured server EXP rate applied. This
  is emulator behavior, not evidence of the commercial server's formula.
- `internal/battle/battle.go`, `Battle.order`: adjacent offensive actions with
  the same target are grouped into at most two attackers. There is no
  speed-distance eligibility test. Damage receives a 1.25 combo multiplier.
- `internal/battle/battle.go`, `Rules.attack`: individual calculated damage
  values are summed before the target's HP is clamped to zero. Go therefore
  already distinguishes calculated damage from actual HP lost, but does not
  retain that damage or assist ownership for EXP calculation.
- The SQL asset catalog identifies **Shrink** as skill **15188**, effect layer
  15, and **Hot Fire Attack** as skill **15189**, effect layer 19. At investigation time,
  the `isHot` name classifier recognized Hot-blooded/War Cry/Fiery, not Hot Fire
  Attack. Shrink had no effect handler. Both fell through to offensive action
  handling instead of their intended Goddess debuff/buff behavior. The generic
  recognized HotBlooded status doubles damage; this is not an implementation
  of the historically reported threefold Hot Fire effect.
- `internal/game/equipment.go`, `Equipment.Bonuses`: signed equipment bonuses
  are accumulated in int16 and converted to uint16. `Character.Combat` then
  adds the unsigned result to base SPD. For example, a -10 equipment SPD
  bonus becomes 65526; it cannot subtract ten from base speed. Player battle
  fighters use this derived SPD directly. Zero-speed weight setups therefore
  require correcting signed stat arithmetic and verifying the native clamp.

## Historical observations

These are player observations, not a verified server formula:

1. A [2008 firsthand burst setup](https://exiliainwonderland.blogspot.com/2008/11/bursting-with-gs.html)
   describes Fire Slowdown, Poison and self-Goddess buffs; Water Shrink,
   support and control; and pets defending until the final attacking round.
   It explicitly calls for pets faster than the Fire characters.
2. A [2019 player's account](https://www.reddit.com/r/MMORPG/comments/akxlo8/the_batshit_insane_leveling_process_in_wonderland/)
   reports damage-dependent EXP, threefold Hot Fire damage, single-target combo
   attacks within 99 speed, and replacing low-level cows as they grow.
   The author acknowledges uncertainty about Shrink's exact reward role;
   other participants disagree with some details. The speed boundary and
   reward cutoffs should be measured before implementation.
3. [IGG's own guide index](https://service.igg.com/faq/view.php?id=110)
   links Comprehensive Guide On Bursting, Cuss Bursting, and Pet Bursting
   (Debuff and Buff). This establishes publisher recognition of the practice,
   but does not establish its origin or arithmetic. The linked historical
   forum pages were unavailable during this investigation.

## Most plausible explanation

The strongest candidate is an interaction between **damage-based contribution
credit, combo participation and support credit**:

- Shrink improves the target's vulnerability and Hot Fire amplifies attacker
  damage, producing a large synchronized hit.
- The reward accounting may use calculated damage before limiting it to the
  enemy's remaining HP. This would let enormous overkill on weak enemies
  produce rewards beyond the enemy's ordinary EXP budget.
- Combo membership may share damage credit, and successful earlier buffs or
  debuffs may assign their casters support credit from the eventual hit.
  This explains how a Water character can gain substantial EXP without
  delivering the finishing damage.
- Zero/equal attacker speeds help coordinate attack order and combo eligibility.
  Faster pets participate before the slow Fire finishers, but must still meet
  the actual combo rules. Arbitrarily fast pets are not necessarily beneficial.
- Low-level pets may affect level-dependent reward eligibility or the level
  attached to shared combo credit. Their reported influence is evidence that
  level/combo attribution matters, but it does not prove which level is used
  or that a pet's level substitutes for its owner's.

These are separate hypotheses. In particular, uncapped overkill credit and
low-level pet attribution cannot be confirmed from the available source.
There is no evidence here for division by zero speed or an EXP integer
underflow/overflow in the original server. Speed zero is explained by action
ordering without requiring a singular arithmetic formula.

## Measurements needed before reproducing the mechanic

Use observed original-server rewards or reliable recordings to isolate:

| Variable | Controlled comparison | What it distinguishes |
| --- | --- | --- |
| Overkill | Same enemy HP/level; raise final damage far past lethal | Calculated damage credit versus HP-loss credit or reward cap |
| Shrink | Match final damage with and without Shrink | Pure damage amplification versus an additional support/eligibility effect |
| Hot Fire | Match final damage with and without Hot Fire | Damage contribution versus a separate Goddess multiplier |
| Support credit | Same final hit; vary successful prior casts and caster | Assist ownership, repeated-cast credit and duration |
| Combo | Same total damage, grouped versus separate hits | Shared credit, combo multiplier and final-hit attribution |
| Speed | Keep a combo intact while varying 0, 1 and higher SPD; then break it | Scheduling versus a direct speed term; eligibility boundary |
| Pet level | Same pet damage/SPD and combo; vary only pet level | Level penalty, owner/pet attribution and participant weighting |
| Eligibility | Compare character level and rebirth with the same battle | Burst level/rebirth cutoffs versus ordinary EXP behavior |

Capture each character's and pet's EXP before/after, all skill IDs and grades,
pre-battle stats, status applications, combo grouping, raw damage, remaining HP,
and any EXP potions/server event rates. Change one variable at a time. Do not
choose an arbitrary burst multiplier and present it as the original formula.

To support a later implementation, the battle engine will need to retain
per-actor damage and per-target successful assist records, accurate Goddess
skill effects, signed speed equipment, original combo eligibility/group size,
and separate player/pet reward attribution. Reward policy can then be calibrated
against observations without changing durable progression handling.


## Development follow-up

The Goddess continuation now handles IDs 15188/15189 as non-damaging temporary
effects, with explicit stat-reduction/damage-buff compatibility rules. See the
Goddess checkpoint in [PORTING.md](PORTING.md) for durations, tests and remaining
native-parity limits. The preceding missing-handler observations describe the
state when this investigation was made. Fixed victory EXP, combo rules and
signed speed equipment remain separate issues; burst EXP was not implemented.


The later generic-effect continuation replaces the initial Goddess-specific
executor with catalog-defined numerical modifiers. See the generic ability
effects checkpoint and development conventions for aggregation and stacking.
The original-game burst reward investigation remains independent of this work.

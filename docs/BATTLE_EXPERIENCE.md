# Battle experience policies

`battle_exp_formula` is a startup-only configuration selector. Its default is
`simple`; set it to `participation` and restart to enable contribution-based EXP.
Both policies use the same native stat/progression messages for legacy and
Gonline clients, including optional round-chain delivery. No asset rebuild or
database migration is needed.

## Simple

The current behavior is retained: sum `max(10, monster level * 15)` across the
battle's uncaptured monsters, apply the global EXP rate once, and award the full
result to each participating player and their current battle pet on victory.
There is no level or support weighting. PvP does not award monster EXP.

## Participation

Each direct killing attack records the distinct player/pet entities that landed
hits in that killing combo, plus eligible direct assistants. Earlier damage
alone is insufficient. A solo attack, area attack or damaging item can have one
killing contributor. Assisters count once even if they helped multiple comboers
or also attacked. Pets are independent entities for both the pool and shares;
their EXP is awarded to the pet, not added to the owner's personal EXP.

For each monster, let `BASE = monster level * 8`, `N` be the number
of unique eligible participants, `COMBO_UNITS` the number of distinct attacking
units that landed hits in the killing combo, and `LEVEL_SUM` the sum of all
eligible participants' levels. Support-only assistants count in `N` and the
division but do not increase `COMBO_UNITS`.
The normalized inverse-level formula requested by the owner is:

```text
POOL = BASE * N
COMBO_UNITS >= 2: POOL *= 1.40 + 0.10 * (COMBO_UNITS - 2)
N = 1: participant gets POOL
N > 1: share = POOL * (LEVEL_SUM - participant level)
                    / ((N - 1) * LEVEL_SUM)
```

For a base reward of 200, two killing comboers plus one assistant produce a
600 base pool, increased by 40% to 840 EXP. Levels 10/20/30 receive 350/280/210
respectively. Three attacking units grant +50%, four +60%, and each additional
attacking unit adds ten percentage points. A solo attacker gets no combo bonus,
even with assistants. This EXP bonus is independent of the combo damage switch.
Fractional bonused pools use midpoint-even rounding. Shares are rounded
down first; leftover EXP goes to the largest fractional remainders, with stable
entity-identity ordering for ties. This preserves the complete pool per kill.
The calculation uses each fighter's battle level at the time of that kill.
The global EXP rate applies once to each entity's accumulated base share at
victory, using the existing midpoint-even rounding and native caps.

## Assistance lifetime

- Persistent friendly buffs retain their caster as transient battle metadata.
  Their caster qualifies while the effect runs on a killing comboer. Expiry,
  removal and replacement stop that effect's old attribution.
- A successful revive qualifies its caster for every subsequent kill involving
  the revived entity for the remainder of that battle. Attacks, later knockouts
  and buff removal do not consume it. Repeated revives by the same caster do not
  multiply participation. Attribution follows pet identity through switching;
  a replacement pet does not inherit it, and it is not carried into a new battle.
- Successful HP healing, SP restoration and status cleansing qualify
  their caster for the supported entity's next executed offensive action.
  Full-resource/no-op support earns no participation. An attack consumes this
  pending assistance even if it misses or does not kill. Area contacts share
  the one cast's assistance snapshot. Unused pending support can cross rounds.
- Assistance is direct, not recursive. Supporting another assistant does not
  automatically qualify unless that assistant also contributes a killing hit.
  Self-support cannot create an extra participant. Enemy casters, disconnected,
  withdrawn and joining entities are excluded.

Entity identities and assistance links remain in memory; no process references
or combat ledger are persisted. The ledger is only a pending victory reward.
Defeat, escape and disconnect do not grant it. Switched-out pets retain their
own pending shares and receive them if their owner later wins; a replacement
pet does not inherit the outgoing pet's EXP. Captured monsters grant no kill EXP.
Normal pet knockouts still incur amity loss only; this policy adds no EXP penalty.

EXP amounts commit through the existing victory transaction before progression
packets are published. Item loot, immediate capture recruitment and battle-gold
policy remain independent. Participation is recorded for direct killing
strikes/items; periodic damage without a killing attack has no killing combo
and does not create a participation award.

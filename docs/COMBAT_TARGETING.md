# Combat targeting

Skills can define explicit grade ranges and formation targets in `assets.db`.
The executor supports area damage, healing, revival, buffs and debuffs for
players, pets and monsters, including both PvP sides.

## Author definitions

Edit an existing entry in the validated **Skills** administration dataset,
keeping its other fields. This example defines a single target at grades 1–5
and an entire side at grades 6–10:

```json
"targeting": [
  {"min_grade": 1, "max_grade": 5, "offsets": [{"x": 0, "y": 0}]},
  {"min_grade": 6, "max_grade": 10, "all": true}
]
```

For a bounded shape, use offsets relative to the selected fighter's actual
native grid coordinates. `(0,0)` must occur exactly once. Offsets cannot repeat,
range from -3 through 3 and contain at most eight cells. `all` and offsets are
mutually exclusive. Grade ranges are inclusive, ordered and nonoverlapping,
within 1–10. A missing grade range retains existing single-target execution.

Typed storage uses `catalog_skills_targeting` and
`catalog_skills_targeting_offsets`; generated parent-presence flags preserve
absent versus explicitly empty definitions. Use the validated editor rather
than inserting incomplete child rows. Saving/reloading Skills publishes the
SQL definitions through the existing catalog path. No startup configuration
parameter or runtime JSON fallback is involved.

## Resolution and parity

- The selected fighter comes first, followed by authored offsets or side order.
  Empty cells, defeated damage targets and captured fighters are excluded.
- Allies and enemies remain separate. Self effects apply once, even when the
  ability also affects an area. Monsters use grade one; players and pets use
  their learned grade.
- One cast spends SP once and records one proficiency use. Area abilities do
  not enter a combo. Each target gets its own damage, element, critical,
  protection, guard, on-hit effects and knockout processing.
- Vanish blocks harmful area casts; friendly buffs still apply. Existing
  physical/magical protection and damage modifiers use the same strike helper
  as single-target attacks.
- Ordinary healing never revives defeated fighters. Revival may include them.
  Each changed recipient receives its own stat synchronization.
- AC50:1 serializes one length-prefixed actor record with a target count and
  eleven bytes per result. Its one-target case matches the existing native
  golden packet. The multi-target framing generalizes that grammar; actual
  aLogin animation acceptance still needs a capture comparison. Skills without
  explicit targeting retain their existing separate single-recipient animations.

Native Skill.dat bytes 109–112 remain provenance and supply the existing area
classification when no explicit range is available. Their automatic grade
bands (1–3, 4–6, 7–9, 10) are still compatibility assumptions. Private Server
resolves only one target and its old AttackPattern enum has three incomplete
values. The available client decompilation has not established a reliable code
mapping for all eight native patterns. Consequently the v6 migration adds empty
targeting metadata and preserves current behavior; it does not seed guessed
native shapes. The example above is illustrative, not a claim about a native
skill. Verified pattern/grade definitions, native hit/resistance formulas and
visual status acceptance remain in the porting queue.

Implementation: `internal/assets/skill_targeting.go` validates definitions;
`internal/battle/targeting.go` selects recipients and serializes results.
Tests cover grade boundaries, SP/proficiency, mixed protection, guard, status
application, death rules, allegiance and typed SQL round trips.

## Other remaining combat parity

Registered AC50 action submission validates ownership and handles attack,
defend/flee/capture and skills. The server sends AC53:5 acknowledgements only to
the submitting session; the source broadcasts them to battle participants. Its
explicit battle-entry teammate HUD and owner pet-progress refresh are not fully
reproduced by having other stat packet helpers elsewhere. Go awards no PvP EXP or
gold; the source attacker-victory path grants 150 EXP and 100 gold. Retaining that
rule requires an explicit decision. Character reborn jobs/potential remain absent,
independently of implemented rebirth combo-level adjustment.

Combat rewards, paid resources and item/vehicle wear remain durable operations.
Buffering ordinary movement does not authorize periodic saving of scarce resources
or a whole-character checkpoint that overwrites a later reward transaction.

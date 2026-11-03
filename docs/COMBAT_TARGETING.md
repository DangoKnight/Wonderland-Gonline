# Combat targeting

Skills can define explicit grade ranges and formation targets in `assets.db`.
The executor supports area damage, healing, revival, buffs and debuffs for
players, pets and monsters, including both PvP sides.

## Monster capture

Capture follows the saved `var/reference-web/モンスター - Wonderland ONLINE Wiki_.htm`
capture section: ordinary attempts permit player level +5, or +10 with an
equipped usable Monster Net (25218) or Pet Net (25225). Targets must have lost
HP and must not be under an active action-blocking seal/control effect. Native
Npc.dat capture restriction byte 34 decides species eligibility, including boss
exceptions; unknown definitions are rejected. Full parties and duplicate species
remain prohibited, while distinct elemental templates remain distinct species.
The wiki's tentative support-skill ban and music-based boss guess are not used
as classifiers. It supplies no precise weakening threshold or success formula:
the minimum weakening check is HP below maximum, and existing level/HP-weighted
success odds are retained.

Each net attempt consumes one of ten uses, even on failure. Durability is checked
again and updated transactionally in SQL before publishing the attempt. Exhausted
nets grant no level allowance. Failed/stale SQL writes do not grant the allowance
or remove a monster. Existing pending recoverable session state is preserved.

Captured monsters recruit at level one and amity 60. Each base attribute is
halved with truncation; one extra attribute point per pair of odd original
attributes is allocated using the configured pet growth policy. Fresh vitals
are recalculated after those bonuses. Story recruitment and existing owned pets
are unchanged. These server rules serve both legacy and Gonline clients. Asset
schema v17 requires the preserving offline upgrade in `ASSET_DATABASE.md`.

## Monster target selection weights

Hostile monster casts and basic attacks choose among eligible opponents with
elemental weights: advantage 4, neutral 2, disadvantage 1. Thus fire prefers wind
over neutral elements and avoids water without making any eligible target
impossible to select. The relationships use the existing elemental damage table;
these weights do not change damage. Players and pets have identical weights for
the same element, regardless of roster order. Area skills choose their primary
target this way, then expand using their normal formation rules.

Usable skills remain uniformly selected before sampling the chosen skill's
target. Healing, SP restoration and cleansing retain their existing support
priorities; confusion redirects retain random ally selection. Previously,
offensive skills picked the first eligible opponent, which favored players
because they normally precede pets in the roster. Basic attacks were uniform.
This server-side policy applies to both legacy and round-chain transport.

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
within 1–10. A missing grade range falls back to native targeting metadata.

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
  aLogin animation acceptance still needs a capture comparison. Automatic area
  targeting uses the same multi-recipient record in native and chain modes.

Native Skill.dat bytes 109–112 supply automatic targeting when no explicit
grade range is available. The implementation follows Private Server revision
`0cb6915d38982391fcf62fb0509da0b0966269ce`, including grade bands 1–3, 4–6,
7–9 and 10, falling back to the preceding nonzero band. Code 1 selects one
fighter; 2 selects the paired column; 3 expands along the selected column;
4 selects a cross; 5 and 6 expand to five and six recipients; 7 and 8 select
the eligible side. Sparse formations follow the reference's neighboring-cell
and stable distance ordering. Unknown codes conservatively select one fighter.
An explicit `area_attack: false` disables automatic expansion; authored grade
shapes take precedence. These shapes reproduce the C# reference and still need
an official-client capture comparison. No asset rebuild or database migration
is needed: the native columns already exist in the SQL catalog.

## SP restoration and cleansing

Native layer 6 restores SP by `max(25, floor(max(ATK, MATK) * 1.5) + level * 10)`,
capped at each living recipient's maximum SP. Native layer 5 cleanses statuses
using the existing effect code, so classification also works with translated
names. Specific cures remove freeze, sleep, poison, confusion, coma/paralysis or
seal/tree bind as appropriate. General recovery clears all active effects,
including buffs, matching Private Server. English names retain compatibility
with older catalogs that lack native metadata.

Both support types use the same automatic or authored area targeting, spend SP
once and award proficiency once per cast. Monsters can choose them when an ally
needs SP or has a matching status. SP changes use native stat 26 results; cures
send their skill animation without inventing an HP/SP change. Existing native
and Gonline round-chain playback consume these results.

Implementation: `internal/assets/skill_targeting.go` validates definitions;
`internal/battle/native_targeting.go` resolves automatic patterns;
`internal/battle/targeting.go` serializes results; `internal/battle/support.go`
handles selective cleansing.
Tests cover grade boundaries, SP/proficiency, mixed protection, guard, status
application, death rules, allegiance and typed SQL round trips.

## Other remaining combat parity

Registered AC50 action submission validates ownership and handles attack,
defend/flee/capture and skills. AC53:5 submission acknowledgements broadcast to
active battle participants, and entry refreshes teammate HUD and owner pet
progress. PvP deliberately grants no EXP, gold, drops or captures; the source's
attacker-victory grant is not used. Typed character/pet potential, actual class,
separate reborn-job metadata and the GM rebirth workflow are implemented.
See [world/combat parity](WORLD_COMBAT_PARITY.md) and
[character state](MANUFACTURING_CHARACTER_STATE.md). Official-client area-pattern
verification, hit/resistance formulas and visual acceptance remain separate work.

Combat rewards, paid resources and item/vehicle wear remain durable operations.
Buffering ordinary movement does not authorize periodic saving of scarce resources
or a whole-character checkpoint that overwrites a later reward transaction.

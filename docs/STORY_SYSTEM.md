# Story and quest execution

Native EVE programs and the `Story` definition dataset in `assets.db` own story
content. Go implements reusable condition evaluation, action execution, protocol
encoding and transaction boundaries. Changing a storyline's map, event, branch,
actor, dialogue, continuation or presentation policy does not require a rebuild.

Asset schema **v16** extends the v15 typed `catalog_story*` tables with owned
checkpoint marks on rules. Gameplay remains
schema **v19**; existing character quest marks and progression retain their IDs
and values. Use the preserving [offline upgrade](ASSET_DATABASE.md) before running
this server against an older assets database. Startup never loads story JSON or
silently supplies missing policies from compiled defaults.

Arrival presentation completion is stored independently in the gameplay table
`character_cinematics`, keyed by character and authored cinematic ID. Finishing
the Robinson rescue records this receipt before publishing its final cues, so
logging back in does not replay it or create Robinson's native quest mark.
An interrupted sequence can resume by replaying; an explicit shipwreck
transition still requests the introduction. The v19 startup migration preserves
existing characters and quest state and creates only the missing receipt table.

## Definitions and ownership

| Definition | Content |
| --- | --- |
| `rules` | Selection priorities, forward-only execution, continuations, click/arrival recovery, visibility, condition compatibility, presentation modes, operation repairs, scene policies and rebirth checkpoints. |
| `recruitments` | Companion identity, completion marks, authored-movie selection, fallback dialogue, full-party presentation and success text. |
| `transformations` | Expected program shape, allowed operation, prerequisites, target appearance and checkpoint mutations. |
| `rewards` | Extra items, exact adjacent operation tail and completion checkpoints for atomic reward bundles. |
| `battle_rewards` | Compatibility quest IDs, encounter templates/name aliases, companions, equipment and gold when no native callback owns settlement. |
| `cinematics` | Map-scoped movie-handshake and arrival sequences, semantic presentation cues and destinations. |
| `companion_aliases`, `companion_equipment` | Recruitment aliases and default equipment for newly created companions. |
| `private_scenes`, `full_bag_warnings` | Introductory scene isolation and dialogue warnings suppressed while inventory has space. |

The current initialization seed contains 113 policy rules, one recruitment, one
transformation, one reward checkpoint, three battle fallbacks and two cinematic
sequences. These are seed counts; edited SQL definitions are authoritative.
[story_defaults.json](../internal/assets/story_defaults.json) is the versioned
initialization seed and native-file test compatibility profile. Editing that file
changes future imports, not existing installations. Migration writes defaults
only when the story root table is new, except for the explicit v16 migration:
it appends the missing rebirth checkpoint to nonempty rule collections, retaining
existing rules and same-ID overrides. Empty collections stay empty. Repeating an
upgrade preserves edits and intentional deletions. A deliberate asset rebuild
replaces content.

The generated projection stores scopes, predicates, edits, checkpoint mutations,
program guards and cinematic steps in typed parent/child tables. Reward messages
may insert the literal `{quantity}` token; they are not format programs. JSON is editor
transport and seed input, not a stored executable document. Imported EVE bytes,
branch order and provenance remain unchanged; repairs amend execution copies.

## Authoring policies

Use the existing **Story** dataset in the authenticated definition editor/API.
An update supplies the current optimistic version and the complete definition
value, preserving collections not being edited. Validation runs before the SQL
transaction publishes a new snapshot. Active interactions and map loading block
edits; successful content edits refresh the catalog and disconnect sessions using
the normal asset editor flow. `/reload quests` also reloads story policies after
all characters finish their interactions. See [administration](ADMINISTRATION.md).

A rule has a stable, readable `id`, a supported `kind`, a `scope`, optional
`gates` and the result fields appropriate for its kind. Rule order is significant.
These are examples of entries inside `rules`, not complete replacement datasets:

```json
[
  {
    "id": "grotto_mirror_continuation",
    "kind": "continue",
    "scope": {"maps": [12464], "events": [1, 2, 3, 5]}
  },
  {
    "id": "grotto_forward_selection",
    "kind": "forward_only",
    "scope": {"maps": [12464]}
  },
  {
    "id": "altar_nonholding_movies",
    "kind": "movie_mode",
    "scope": {"maps": [12465], "events": [2, 3]},
    "value": 1
  }
]
```

Scope lists select maps, events, authored branch IDs and actor click IDs; an
omitted/empty list matches any identity in that dimension. Branch IDs are EVE
`Index` values, not Go slice indexes. New scopes validate their map/event/branch
and actor references. Imported compatibility rows explicitly set `allow_missing`
so a partial installation may retain dormant definitions. That flag does not
make an unknown opcode or missing resource executable.

Patterns match EVE `code`, `d1` through `d6`, `d6_low`, or the packed action
`value`. An omitted field is unconstrained; an explicit zero must match zero.
The same representation describes expected condition bytes and operation shapes.
Do not infer opcode meanings from a story's name. Keep unresolved operands
explicit and preserve independent native packet fixtures.

Gates use `source`, `id`, `op`, `value` and optional `group`/`slot`. Operators are
`eq`, `ne`, `lt`, `le`, `gt` and `ge`. Gates within a group are ANDed; groups are
ORed. The default group is zero; an empty gate list is unconditional.

| Gate source | Meaning |
| --- | --- |
| `mark_state` | EVE lifecycle: 1 in progress, 2 not started/other, 3 completed. |
| `mark_step` | Stored mark step, or zero when absent. Supports custom 32-bit quest IDs. |
| `active_mark_step` | Mark step only while in progress; otherwise zero. |
| `mark_exists` | Whether a character has a row for the mark, regardless of state. |
| `party`, `party_exact`, `party_broadcast` | Companion presence using alias, exact or broadcast identity. |
| `active_pet_exact`, `active_vehicle` | Current active identity. |
| `npc_template` | Template of the clicked actor. |
| `equipment` | Item ID in an owned party companion's zero-based equipment slot. |
| `inventory` | Quantity of an item in the bag. |

Common result fields are explicit: `continue` uses `target_event`,
`target_actor` and optional `target_branch`; `arrival` selects `target_event` and
`use_pre_event`; `transition` uses `destination` with `map`, `x` and `y`.
`movie_mode` uses `value` (1 nonholding, 2 holding) and optional `later_movie`.
The latter restricts an override to a branch containing that subsequent movie.
Without an override, movies use mode 2 only when a later operation in the current
branch loads a map (player scene transition or service/teleport to a map ID of at
least 1000). Otherwise they use mode 1. Both modes retain the movie acknowledgment
barrier before advancing.

`reborn_checkpoint` applies configured `marks` after rebirth and before character
selection publishes an already-reborn character. It accepts map scopes and mark
gates, without event/branch/actor scopes or operation patterns. The default
`job_palace_rebirth_checkpoint` runs only while mark 15091 is absent: it completes
15090 and ensures 15091 is active at step 1. These identities belong to SQL
content. Each mark uses `id`, `mode`, `value` and optional `preserve_step`.
Completion with `preserve_step` retains the previous step (using `value` as a
floor) and an existing completion timestamp. Removing the policy or clearing
rules disables this repair without a compiled fallback.

Selection priorities precede ordinary EVE ordering. Continuations try eligible
targets in rule order; forward-only policies prevent earlier branches from being
replayed. Visibility and mode lookups use the first matching rule. Repairs apply
all matching edits in order: remove, prepend, replace the first match, move before
another operation, or copy speech from an authored branch. Recovery routes use
existing durable marks to resume or finish interrupted story checkpoints.

### Native honey quest example

Kelan's honey quest uses this same executor. The EVE programs own acceptance,
dialogue, honey delivery, the bee formation, battle outcomes, hand-in and the
patient's pose. Native pre-events hide the honey actors while honey is carried
and after hand-in. The only additional runtime policy is a scoped `continue`
rule linking map 12000, event 25, branch 1 to branch 4: after the honey grant,
the warning and battle continue within the same interaction. Its ID is an
editor label; runtime dispatch depends on its kind, scope and operands.

The native/custom distinction matters: `QuestDefinitions` offers simpler custom
dialogue and step-based quests. Native quests use Story/EVE to retain the
original action sequence and Legacy protocol. No honey-specific runtime module
or replacement dialogue engine is required. See the
[captured reference](HONEY_QUEST_NATIVE_RESEARCH.md).

### Native Lost Dog visibility

Kelan's hogpen dog (map 12000, actor 20) is visible before acceptance. Native
event 26 then runs its bark and hogpen dialogue without changing progress. Once
request mark 13046 reaches active step 1, the same event selects the recovery
branch, moves/hides that actor, reveals the returned dog (actor 28) and advances
the request to step 2. The native pre-events reconstruct the two actors' states
on re-entry; mark 13047 gates the completed hand-in. The server does not equate
being visible with being eligible for recovery.

Asset revision v44 removes the four stock `dog_*` visibility overrides that
previously hid the hogpen dog before acceptance. This uses the same revision
helper as the honey correction, with its before-images authored in
[`native_quest_revision_v44.json`](../internal/assetsql/native_quest_revision_v44.json).
Installed-data tests cover fresh, accepted, recovered and finished visibility,
and execute both the pre-acceptance dialogue and accepted recovery branch.

### Battle backdrop policies

`battle_background` selects the native byte formation background with `value`
(0–255). AC11:250's next header byte is independent metadata, not the high
byte of a UInt16 backdrop. `FUN_0038e4f4` reads these separately before passing
the first byte to `FUN_0036f448`.
It supports ordinary map, event, branch, actor and player-state gates as an
explicit administrator override for server-resolved recipients (Legacy or
unnegotiated clients). Negotiated Gonline clients resolve their native regions
locally; see [backdrop negotiation](BATTLE_BACKDROPS.md). Operation patterns are not used for this kind.
Otherwise, schema v50's `BattleBackdrops` dataset selects from native
`Ground.MMG` rectangles at the initiating player's position, then uses the map's
native default. Selection happens at battle entry and all participants receive
the same selector. Maps without definitions retain the documented compatibility
selection.

Earlier revisions inferred encounter-specific overrides from screenshots. Schema
v50 retires their exact original policies after decoding the native map records;
operator-edited policies remain intact. See [native backdrop evidence and
coverage](BATTLE_BACKDROPS.md) for the format, migration and audit command.

### Declarative policy revisions

Offline asset upgrades can use the reusable `storyRevision` migration helper.
The v43 definition is authored in
[`native_quest_revision_v43.json`](../internal/assetsql/native_quest_revision_v43.json).
It contains exact `remove` before-images, `replace` pairs with the same rule ID,
and `append` entries optionally guarded by `when_any_present` policy IDs.
Guards inspect the original collection, so removing an obsolete parent and
adding its successor works in one revision. Existing same-ID additions are
preserved. Intentionally empty collections remain empty; unrelated definitions,
operator edits and surviving rule order are retained.

This file is versioned migration input, not a runtime definition store. The
catalog migration runner invokes it inside the upgrade transaction; subsequent
gameplay uses the structured Story SQL tables. Future native quest corrections
can reuse the helper with their own revision data rather than adding a
quest-specific Go migration function. Revisions are applied once at their
schema boundary, preventing later upgrades from restoring deleted policies.

Cinematic cues name protocol operations such as movie, pose, actor action,
movement lock and quest display; they contain no arbitrary socket bytes or SQL.
Lengths and native numeric ranges are validated before publication. Character IDs
are injected from the owning session. A running cinematic retains its selected
definition and an ownership token until completion, cancellation or timeout.

## Execution and persistence

The world builds a map/kind index from the SQL snapshot, merges global/local
candidates in authored order and compiles gate sources, comparisons and groups
once. Event and visibility queries use that immutable index and current player
state; they do not query SQL or reopen seed files per packet. First-match queries
stop at the first eligible rule. Multi-match iteration captures eligibility
before yielding any action, so a quest change by the first action does not change
which later actions run. Returned rule references are read-only. Ordinary
candidate lists need no heap allocation; lists above 256 candidates use a
temporary eligibility bitmap. Player predicate results are never cached,
including during resource-transaction rechecks.

Unconditional repairs of known event branches are prepared once as immutable
execution copies. A branch with any applicable conditional repair is evaluated
in full at execution time to preserve edit order. Session counters, acknowledgments
and callbacks remain separate from these shared programs. Story/catalog reloads
rebuild the compiled indexes and repair cache; event reloads rebuild repairs for
the replacement scripts. Removing policies takes effect when a new snapshot is
published, including deliberately clearing the dataset. Reloads retain the
existing requirement to exclude readers and active event sessions.

SQL projection loading lazily batches each required story table into one ordered
read, then reconstructs its parent-owned children with the generated presence and
ordinal checks. The batch belongs to one projection read inside the caller's
transaction; another read after an edit starts a fresh batch. Empty definitions
do not fetch unused child tables. This changes neither the typed schema nor its
migration version.

Automatic continuation chains stop after 128 transitions without a client step,
release the interaction and log the limit. Genuine choices/movie acknowledgments
start a fresh chain budget. Existing session tokens reject stale completions.
Debug logs identify selected events/branches, policy IDs and continuation links.
Content must still place resource grants behind appropriate completion guards:
the chain limit does not undo already committed actions.

Item exchanges, grouped rewards and permanent appearance changes use owned SQL
transactions. Recruitment rechecks current SQL ownership, party/hotel capacity
and completion, then saves the companion and configured marks together. Receipts
follow commit; packet failure cannot repeat a grant. Adopting transaction results
preserves buffered walking under the existing checkpoint policy. Battle fallbacks
remain part of the normal victory settlement transaction, with native callback
ownership, capture exclusions and replay guards retained.

Rebirth, its class skill and configured checkpoint marks commit in one owned SQL
transaction. Login repair also rechecks fresh owned SQL state before publishing
the character; an existing completion mark skips repair and its write. A failed
checkpoint save rolls back the transaction. Neither path replaces unrelated
progress, inventory or balances with a cached snapshot.

Actor contact requests (AC20:2) share AC20:1 click validation and execution.
Area entry scripts of kinds 1–3 take precedence over colliding warp-table IDs.
When a committed event warp interrupts an interaction, a transient flag releases
movement/UI once after the next map acknowledgment's AC5:4, sending AC6:2:0 then
AC20:8. A new interaction on that map keeps ownership of its UI lock.

Compiled growth formulas, numeric wire layouts, inventory placement, resource
validation and transaction primitives remain in their owning Go packages. The
existing custom `QuestDefinitions` registry and victory kill counters retain their
current behavior; this migration does not invent missing NPC bindings/rewards or
activate its dormant acceptance/completion workflow. New verified mechanics can
add reusable capabilities without adding map-specific story switches.

Current coverage and recipes are in
[TESTING.md](TESTING.md#automated-coverage), with optimization
[benchmarks](TESTING.md#benchmarks) and recent validation records on that page. Native script and wire checks
remain distinct from live native-client acceptance.

### Visit-scoped actor visibility

`visit_visibility` snapshots the selected actors' native visibility during the
first scene synchronization. Subsequent quest-mark changes do not remove them
mid-scene; explicit scripted show/hide commands still take precedence. Map entry
resets the snapshot and evaluates the durable quest progress again. Schema v46
replaces the stock Xaolan rescue visibility overrides with this policy for
actors 2 and 3 in map 12002. Native pre-events hide them after the rescue on the
next visit, while later quest stages can show alternate actors 6 and 7.

### Native dialogue facing

NPC speech retains the EVE operation's dword2 facing mode in AC20's dialogue
frame: mode 1 permits turning toward the player; mode 2 preserves the actor's
authored facing. This applies equally to Legacy and Gonline clients. For example,
North Island map 60001 actor 62 (bound Xaolan, template 14154), event 46 and talk
30062 use mode 2. Limited sprite artwork must not be mistaken for permission to
turn the actor. Existing hand-authored operations that omit this operand (zero)
retain the server's historical turn-to-player default. The native EVE speech
operations use modes 1 and 2; no map-specific override or asset migration is needed.

### North Island rescue battle reference

The earlier screenshot-based selector-90 rescue override is superseded by
schema v50's native position regions. For example, map 60001 at (2622,755)
selects 81, while other forest regions select 90. Encounter IDs do not determine
the biome. Explicit operator overrides remain supported.

Native formations 15 and 33 contain templates 12049 at slot 5, 12050 at slot 8,
and 14155 at slot 2, in different record orders. The reference positions and
client's native grid coordinates establish slots 1–4 as wire column 1 and slots
5–8 as wire column 2, with rows 1–4 in each column. Thus the two Hijackers occupy
(2,1)/(2,4), and the Pirate Leader occupies (1,2). This reverses the columns in
Private Server's `GetEnemySlotCoordinates` helper; its output is not used as a
Legacy golden. Gonline now honors authored slots for initial positions while
retaining first-seen roster order, repeated-slot reinforcement queues, and the
existing sequential fallback for encounters without authored slots.

# Development conventions

## Server data ownership and initialization

All persistent server data must be managed through the databases. Shared game
definitions, read-only during gameplay, belong in `assets.db`. Accounts,
characters, mutable gameplay state and persistent server settings belong in
`wonderland.db`. These are the default database names; the startup configuration
may select other paths. Read-only during gameplay does not prevent validated
administrator edits to shared definitions.

JSON may seed initial defaults in either database. Initialization must preserve
existing data, including administrator edits, and must be safe to repeat. Seed
only missing defaults; use explicit, versioned migrations for existing records.
Do not reload seed files on restart to overwrite database values. Deliberate
reset/rebuild operations are separate operator actions, never normal startup
initialization.

Durable reads and administrative edits must use database interfaces. Online
recoverable state may be served from the owning session cache. Prefer
structured tables, typed columns, relationships and indexes for fields queried
or edited independently. Avoid JSON files and filename-based virtual JSON
documents wherever practical, except for server initialization parameters such
as listener addresses, database paths and startup-only policy selectors. JSON
columns may hold genuinely nested data when relational storage adds no useful
querying or integrity guarantees; they must not become a substitute for an
entire queryable schema. SQL-derived, validated in-memory catalogs are allowed.

Sockets, timers, locks and pending actions remain in memory. Persist only the
state needed for reconnects or recovery, such as cooldown expiry timestamps,
durable player progress and recoverable operation records. Do not serialize
process handles or synchronization objects into either database.

Binary assets remain outside the server databases. Store only metadata required
by the server; do not import images, audio or video payloads for client rendering.
Client asset formats, offline extraction/export artifacts and API serialization
are outside this server persistence policy.

Executable rules belong in code. Compiled growth formulas and their coefficients
remain in `internal/game/growth_parameters.go`; changing them requires a rebuild.
Players, pets and enemies share `game.InnateCombat` for derived combat stats;
enemy HP/SP use stored NPC values, with explicit encounter HP overrides.
Resolve NPC element and base attributes from the asset catalog rather than
copying definitions into pet state.
See [combat stats](COMBAT_STATS.md) for encounter overrides and fallbacks.
Game content definitions belong in `assets.db`. Preserve the startup-only
`pet_growth_formula` and `battle_exp_formula` selectors, `battle_gold_enabled`
switch and `combo_damage_per_participant` switch as server
initialization parameters. Capture combo damage selection at startup; only actual
successful chain participants count, including pets, and singles receive no bonus.

Gameplay schema v21 stores character state in typed tables and owned child rows.
v21 adds shared collection and ground-item expiry timestamps in `resource_cooldowns`.
Claims and rewards commit together; keys identify the resource kind, map and source,
independently of the character and native event timer ID. Per-source cooldown
durations belong in the assets database's `ResourceRespawns` definitions.
v20 adds each pet's lifetime stat-food use counter; v19 adds owned cinematic-completion receipts independently of native quest marks.
v18 adds owned Teleport Scroll points, attached tent tools, manufacturing input
escrow/batch metadata and typed persistent bazaars; v17 separated Party Invites from Joining
Battle while preserving earlier invitation restrictions. Structured asset schema v40 stores runtime definitions
in generated `catalog_*` tables. Migration retains legacy representations solely
as snapshots/provenance;
runtime and administration must not consult them. Use the offline upgrade procedure
in [ASSET_DATABASE.md](ASSET_DATABASE.md) to upgrade existing installations.
Regenerate asset models after changing catalog types with
`go run ./tools/asset-schema-generator`, bump the structured schema version and
provide an explicit preserving migration for installations using earlier versions.
Keep content definitions, default initialization, validation and migrations with
the owning feature. Migration runners coordinate schema versions and transaction
boundaries; feature names identify the individual steps. Record
reference repository revisions and import evidence in documentation.

## Transaction boundaries and session checkpoints

Use a hybrid model. The databases own durable state; an online session owns its
current recoverable state in memory. Persistence ownership does not require a
SQL write for every packet. SQL-derived asset catalogs already follow this model
for immutable gameplay reads.

| State/operation                                                            | Required persistence boundary                                                                                                           |
| -------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------- |
| Purchases, sales, trades, bank transfers, fees and resource consumption    | Validate authoritative balances/ownership and commit debit plus delivery in one database transaction before success replies             |
| Loot, gacha/Lucky Draw, quest rewards, mail claims and shared-node claims  | Commit reward and consumption/claim/cooldown together; concurrent or repeated claims must not duplicate rewards                         |
| Account/security, administrator edits, durable relationships and ownership | Validate permissions/version and commit immediately; publish the result after commit                                                    |
| Walking and other explicitly classified recoverable session fields         | Update memory, mark dirty, periodically checkpoint only those fields; also flush at character handoff, disconnect and graceful shutdown |
| Map transitions and recovery destinations                                  | Persist the destination/return state before publishing a transition until a recoverable transition protocol is implemented              |
| Sockets, timers, locks, pending actions and presentation state             | Keep in memory; persist only recovery timestamps/results where needed                                                                   |

Do not move inventory, currency, item durability/consumption, paid draws or scarce
resource claims into periodic snapshots. A precomputed session copy alone cannot
validate a purchase: recheck SQL balances, ownership and replay state inside the
transaction, including both parties when appropriate. Reply failures after commit
must not cause an automatic refund or repeat grant.

Checkpoint only dirty fields and coalesce repeated updates. Clean sessions must
need no database transaction. Prefer narrow typed updates over rewriting owned
inventory/pet/quest rows for a position change. Choose and document the interval
and its maximum normal crash-loss window. A crash may lose recent walking; it
must not undo a completed purchase. Failed checkpoints remain dirty and retry;
log failures, and never report a successful flush or advance its baseline on error.

Merge committed transaction results with pending session fields explicitly.
Purchasing must not teleport a character back to their saved position. A late
checkpoint must not overwrite committed inventory/currency, administrator edits,
a newer warp or a deleted character. Use ownership checks, version/compare-and-save
conditions and serialized session mutations. Direct external edits to a live
player's mutable rows are unsupported; route edits through the server's conflict
checks and cache refresh/disconnect flow. Multiple server processes sharing the
same player state require an additional ownership/version protocol.

Test walking followed by purchase/trade/warp, rollback, stale snapshots, deletion,
reconnect, failed checkpoint retries and concurrent players under the race detector
before enabling buffered persistence. A global world lock must not be held through
an unbounded batch of SQL writes. Any asynchronous saver needs immutable snapshots,
per-session ordering and protection against logout/reconnect races.

**Current implementation:** ordinary walking and resting HP/SP recovery change
the session in memory. See [resting recovery](ITEMS_PLAYER_STATE.md#resting-recovery).
`character_save_seconds` selects the startup checkpoint interval (default 30,
accepted 1–3600 seconds). Clean sessions skip SQL entirely. Walking and resting
use narrow X/Y/HP/SP updates guarded by the saved map, position and vitals,
without rewriting inventory/pet/quest child rows; other pending state retains
full-checkpoint conflict validation. Purchases and other
SQL-returned results preserve pending walking when the transaction leaves the
saved position unchanged. Buffered recovery is retained per vital when the
transaction leaves that vital unchanged, capped to the adopted maximum.
The active following pet recovers on the same resting tick. Its HP/SP are also
buffered: checkpoints update only those columns in the owned party-pet row,
guarded by pet identity and checkpointed vitals, in the same transaction as
the character state. Pet recovery survives unrelated SQL results per vital;
explicit vital changes and pet removal/replacement take precedence.
Warps and recoverable tent returns remain immediate.
Disconnect and graceful shutdown attempt a final save; failures retain the
checkpoint baseline and are logged. Gathering remains on the independent
one-second world tick. Vehicle wear/consumption can commit during movement and
is deliberately not deferred.

Combat disconnects commit resolved player/pet vitals and active-pet knockout/amity
consequences before the final session checkpoint. Both delivery modes use the
whole calculated round, including animations not yet played. Withdrawn fighters
cannot act, defend or be targeted in later rounds; disconnect is not escape.
Already committed final results are not settled twice. Failed writes retain an
immutable pending snapshot for the final-save retry and do not publish roster
changes or advance the checkpoint. See [combat transport](COMBAT_TRANSPORT.md).

The saver currently serializes checkpoints under `worldMu`, with a bounded
five-second batch context. This reduces per-packet SQL work but does not remove
the global world lock or establish load/soak acceptance. Under healthy saves,
a crash can lose up to one interval of movement and resting recovery; failed
saves can extend that window. Runtime interval changes require restart. Tests cover no per-packet
writes, transaction-result merging, narrow updates, failure/retry, stale warps,
ownership/deletion, disconnects and concurrent sessions.

For the current development installation, the owner has authorized backed-up
rebuilds of `wonderland.db` and recreating `assets.db`; there is no requirement to
retain its existing player data or reverse-migrate model changes. Take consistent
backups before any deliberate rebuild, stop writers, and document regenerated
asset inputs. This is permission for explicit development maintenance, not a
normal-startup reset rule or permission to discard another installation's data.
No database rebuild was needed for movement checkpointing.

## Give numeric values names

Use named constants for values that encode a protocol command, event operation,
status, flag, game limit, content reference or binary layout. Apply this to new
code and to existing code when touching its behavior. Ordinary indexes, zero
checks, arithmetic identities and standard binary shifts can remain literals.

### Where definitions belong

- `internal/protocol/commands.go`: wire commands, subcommands and reply values.
- `internal/protocol/packet.go`: framing and string length limits.
- `internal/world/event_codes.go`: EVE actions, branch conditions, comparison
  operators and context-specific operands.
- `internal/game/limits.go`: shared game limits and record sizes.
- `internal/game/settings.go`: preference flags and chat channel bits.
- `internal/game/map_ids.go`: shared map references without verified location names.
- `internal/store/limits.go`: account ID offsets, credential parameters and Store limits.
- Package-local constants: distances, durations and rules used only by that package.

Put a constant beside its owner or in a focused file. Avoid a global constants
package. Reuse an existing definition when the meaning is the same.

### Naming and values

1. Name the meaning and namespace: `CommandTrade`, `TradeDecision`, `MaxGold`,
   `SecondCharacterIDOffset`, `npcInteractionRangePixels`.
2. Keep separate definitions for equal values with different meanings. A bag
   capacity and an item stack limit happen to both be 50. Incoming and outgoing
   packets can also reuse a code: `PetControlDismountVehicle` and
   `PetControlVehicleMount` both use AC15:10, in different directions.
3. Assign explicit numeric values to wire codes, persisted enums and content IDs.
   Do not use `iota` where adding or moving a declaration could change compatibility.
   Bit flags may use explicit shifts such as `1 << 3`.
4. Use types when they prevent mixing domains without complicating the existing
   byte-based encoders. Existing protocol constants are untyped so they work in
   byte literals, switch cases and builders without casts. Preserve public API
   types during naming refactors.
5. Include units in scalar names, such as `RangePixels` or `MetadataBytes`. Use
   `time.Duration` for time values.
6. Check comments, legacy handlers and assets before choosing a name. A best guess
   is acceptable if its uncertainty is documented. If the meaning is still unclear,
   use a stable namespace and numeric suffix: `FriendsWireCode13`,
   `ActionWireCode16` or `MapID12000`. Document that it is unresolved. Replace the
   fallback with a verified name when evidence becomes available.
7. Preserve numeric values. A naming change must not alter packet bytes, persisted
   IDs, database schemas, credentials or gameplay rules.

### Data and verification

Keep raw expected bytes in golden protocol tests: they independently verify the
named production constants. Test fixture IDs may remain numeric when arbitrary.
Authored data tables, generated skill mappings and versioned schema SQL may retain
numeric data; do not create thousands of constants that merely duplicate a table.
Explain semantic exceptions in nearby comments when their role is unclear.

Prefer named packet builders when an entire layout is repeated or difficult to
read. Avoid inventing wrappers for every one-line packet just to hide its bytes.

Follow [test selection and correctness](TESTING.md#test-selection-and-correctness):
use focused affected tests during development, the race detector for concurrency,
and full server suites only before a push; commits use focused affected checks.
Commands, required environments and recorded results belong in TESTING.md. Review namespaces as well as values:
byte-for-byte tests cannot detect a misleading name.

## Editable sprite assets

Use transparent RGBA PNG sheets and `editable.json` for editable client sprites.
The runtime loads these directly; native JMA/JXA and JXAN files are conversion
and compatibility inputs. Keep each sprite's archive `index` stable, retain
frame placement metadata and use animation arrays of frame references. PNG
colors and alpha are unrestricted by the original palette. Keep validation
independent of source hashes so deliberate artwork edits remain valid.

Do not regenerate existing editable assets automatically. The export command
requires `-overwrite-edits` before replacing them. Document new format versions
and keep the loader and exporter compatible. See
[data/sprites/EDITING.md](../data/sprites/EDITING.md) for the format and commands.

## Server asset queries

Use `assetsql.LoadDatabase` at startup and the resulting catalog in gameplay
handlers. Follow [the database ownership policy](#server-data-ownership-and-initialization).
Route new static content through structured tables in `internal/assetdb` with
GORM and add its runtime projection to `internal/assetsql/database.go`. Keep
`internal/assets` free of GORM and SQL imports: the client links it for the
native file decoders and must not pull in the database. Never add a runtime
JSON/native-file fallback. Use the gameplay store for player-owned state. Exclude original account
database snapshots and SQLite sidecars from asset export/import.
Retain unknown exported fields in SQL and preserve authored row order. Document
which SQL fields are authoritative when adding a projection, and validate before
publishing the startup snapshot. Keep file decoders for offline tools and
independent compatibility tests.

Use [test data and opt-ins](TESTING.md#test-data-and-opt-ins) for SQL/native inputs
and absolute paths, and [pre-push server validation](TESTING.md#pre-push-server-validation)
for the separate server/client suites and installed-data timeout.

## Migration source inventory

Current migration priority is behavior implemented in the recorded Private Server
revision. Keep its empty classes, placeholder handlers and unfinished components
on hold in PORTING.md. Trace callers before treating a pending source entry as a
missing feature. Port reachable implementations and their regression evidence;
separate new Gonline gameplay and native-client parity research from that queue.

Update `docs/source-inventory.json` when porting a legacy subsystem. Map the C#
source to the Go implementation and its tests, and describe implemented behavior
and remaining work in the notes. Preserve the recorded source revision and
hashes; verify the files used against those hashes after CRLF normalization.
Keep the inventory and the current scope in `docs/PORTING.md` consistent.
The inventory retains C# findings in `files` and other reference artifacts in
`supporting_files`; preserve their hashes, evidence and review limitations.
Use `audit_verdict`/notes to distinguish missing live behavior from dormant helpers,
source placeholders, intended replacements and retired scaffolding. Do not infer
feature completion from file counts or a historical verification report.

## Client port catalog

Rerun `python3 tools/decompile/catalog.py` after client porting work. It reads the
local decompile in `var/decompiled` and the Go sources, and regenerates
`docs/ALOGIN_CATALOG.md` and `docs/client-inventory.json`. Cite ported functions
by address (`FUN_xxxxxxxx`) so the catalog counts them, and update a subsystem's
status in the script's `SUBSYSTEMS` table when it moves. The plan is in
`docs/CLIENT_ROADMAP.md`. The receive-command table uses
`var/ghidra/aLogin.exe`; use `--exe /path/to/aLogin.exe` for another location of
the same WLRI build. Command coverage indicates a receive branch exists, not
that every subcommand or its interface is complete.

## Editable picture assets

Use `client/cmd/pic-export` for offline BMg/JMg and loose picture conversion into
`data/pictures/`. Keep original archive/resource names in the manifest and keep
patch archives separate. Preserve defined palette colors and record any fallback
for undefined source colors. Protect edited PNGs by exporting to a new directory.
Keep generated image and audio payloads under `data/` ignored by Git.
Track JSON data, manifests, editable metadata and documentation. Client map pictures and login UI load PNG exports from a direct `data/` root.
Keep original-format loaders for conversion and compatibility tests.
See [data/pictures/README.md](../data/pictures/README.md).

## Original asset inputs and decompiled outputs

Original client assets come from the WLRI / Wonderland Rhode Island installation.
The client table/media regeneration procedure accepts
`regenerate.py --client <installation-root>` and needs no private-server checkout.
Tracked server-only JSON exports supply the additional definitions for a complete
SQL import tree. To re-export those from their original source too, use
`export.py --client <client-data> --server <private-server>`.
Keep original source assets, indexes and decryption inputs (including `aLogin.exe`)
in their original projects. Exclude database-path overrides from asset export/import.
Treat client assets and manifests in `data/` as derived outputs. Never require
previously generated client data or manifests to regenerate those outputs.
Retained `data/server/` definitions are versioned inputs and never supply native
decryption keys. Temporary
intermediates may be created during an export and must be cleaned up afterward.
Do not retain decoded JMA files in `data/`. Preserve complete decoded bytes as
zlib-compressed Base64 in `sprites.json` for palette indices and verification.
PNG regeneration must start from fresh sibling-source decryption.

Follow [ASSET_IMPORT.md](ASSET_IMPORT.md) to restore missing ignored payloads.
Use `python3 tools/data_export/regenerate.py --client <installation-root> --output var/asset-import` for a fresh
source-only export of audio, sprites, pictures and remaining client media.
Extraction instructions must use portable example paths and an explicit source;
do not infer a developer's installation name or sibling-directory layout.
Install the Python dependencies in `tools/data_export/requirements.txt` and make
FFmpeg available. Media metadata documents unresolved source content;
check it before claiming complete decryption. Run `verify_media.py` after a fresh
export. Client login UI can load exported media directly from a `data/` asset root.
Export into a new directory to protect authored edits. Client packaging and SQL
import may consume `data/`; they do not regenerate the decompilations.

Retain complete source content and provenance in exports. Preserve unresolved
fields losslessly and label their unknown meaning; never claim full semantic
decompilation when a decoder only understands part of the structure. Record
missing source companions explicitly. See [coverage limits](../data/README.md).

## Optimize sprite outputs

Use `internal/spritepack.OptimizeEditable` to apply the client's atlas packing
and frame deduplication to editable sprite data. Keep `editable.json` compatible
with direct loading from `data/sprites/`; the long-term client asset root is
`data/`. Preserve all frame placement, animations, painted colors and original
source provenance. Compare visible pixels before replacing each archive.
Remove empty frame directories after packing. Small pictures (up to 512 pixels
on each side) use shared PNG pages and manifest rectangles; retain larger
pictures standalone. Keep patch archives in separate namespaces. Resolve
logical picture paths through the manifest in client code. Compare every packed
pixel and publish existing picture conversions transactionally to protect edits.
Source-only regeneration runs optimization on fresh temporary exports; it never
uses saved data files as original decompilation inputs.

## Preserve usable standard media containers

Focus asset decompilation on proprietary, encrypted or obsolete formats that
need conversion for editing or runtime access. Keep usable standard containers
whole when conversion provides no needed benefit. Preserve AVI videos byte for
byte, including their embedded audio and timing. Record source/output hashes in
the media manifest and keep video payloads ignored by Git. Regeneration must copy
them directly from the authoritative sibling sources.

## Consolidate style descriptions

Export original STY files into `data/media/sty/style_data.json`, using the
`style-collection` format and a `styles` object keyed by original filename.
Preserve each record's decoded fields and source provenance. Media manifest rows
point to the collection through `metadata` and select a style through `record`.
Verify every style by reconstructing its original bytes. Regenerate this collection
from sibling source files and protect existing collections from replacement.

## One client asset tree

Use repository `data/` for normal client rendering and gameplay data. Development
runs reference repository `data/` directly without links or asset copies.
`cmd/asset-build` only verifies it; standalone packaging copies the complete
tree with `cmd/asset-build -copy`. Do not maintain duplicated
`client/assets/spritepacks` or `client/assets/legacy` directories. Keep saved
user state separate and preserve it during layout migrations. Missing exports
must fail clearly rather than use original native files. Native inspection
modes require an explicit sibling source directory.

## Battle buffs and debuffs

Define temporary ability effects through `assets.Skill.Effects`. Each definition
specifies its target (`ally`, `enemy` or `self`), duration in rounds, modifiers
and optional stacking group. Native effect classes are converted by offline exporters into JSON definitions
and references before SQL import; combat must not branch on
particular skill IDs or names to calculate buffed stats or damage. Unknown native
classes must remain documented until decoded. Use a dedicated ability handler
only for mechanics that cannot be expressed by the shared effect model, and
document the reason.

Use `Fighter.ApplyEffect` for transient modifiers. Never overwrite base combat
stats when applying a buff or debuff. The same source/index refreshes instead of
adding another copy. Independent effects add their percentage and flat changes.
Within a named stacking group, the strongest positive and negative contribution
for each stat and operation is used. Apply summed percentages to the base,
then add flat changes; truncate once and clamp to the nonnegative int32 range.
The battle damage floor still applies afterward. Effects expire at round end,
including the casting round. Recalculate speed from active effects before
ordering the next round; do not reorder an already collected round.

Definitions are validated before publishing the SQL catalog. Runtime skill data
continues to come exclusively from that catalog. Keep compatibility projections
separate from the generic battle executor. Add tests using arbitrary skill IDs,
combined buffs/debuffs, refresh/expiry and targeting; retain independent native
packet fixtures and SQL/native projection checks.

Control, periodic and visual effects use the same definitions and active-effect
lifecycle. Use `block_actions`, `break_on_damage`,
`ally_attack_chance_percent`, `periodic_damage` and `presentation` to describe
behavior. Do not add a separate named-condition enum, status list or skill-name
classifier. Capabilities are read from all active effects; damage removes only
effects marked to break on damage. Periodic damage uses max-HP percentage plus
flat damage with a minimum; independent ticks add and named groups use their
strongest tick. Redirection chances likewise add with a 100% cap or use the
strongest chance within a group.

Set `on_hit` for an effect applied after a damaging attack. Hit-triggered effects
are applied after existing damage-sensitive effects are removed, and only to
living recipients. An ability must use one trigger mode throughout its effect
list; mixed cast/hit modes are rejected until combined action semantics are
implemented. Pure cast effects cost SP and award proficiency once. Presentation
labels describe visual intent and do not imply undiscovered immunity or targeting
rules. Add those behaviors explicitly when verified.

Use `miss_physical_attacks` and `miss_area_attacks` for effect-driven protection.
Any active matching protection forces a zero-damage miss before damage floors,
critical rolls or on-hit application. Missed casts still spend SP and retain
proficiency. Do not implement protection by querying a specific buff name or ID.
Basic strikes (including staff strikes) and physical abilities use physical
classification; magical abilities use the skill layer. Read AOE classification
from native targeting patterns, never from `attack_category`, which describes
range. An authored SQL `area_attack` boolean overrides pattern classification.
Current pattern grade bands (1–3, 4–6, 7–9 and 10) are a compatibility policy until
native thresholds are verified. Explicit `targeting` grade ranges take precedence
over classification and drive recipient expansion. Keep shapes in SQL metadata,
not skill-ID conditionals. See [COMBAT_TARGETING.md](COMBAT_TARGETING.md).

Use `physical_damage_taken` and `magical_damage_taken` for modifiers restricted
to an attack category. Aggregate the applicable category with `damage_taken` in
one pass, using the existing named-group policy and rounding once. Non-basic
magical-layer skills use magical protection; basic strikes (including staff
strikes), physical abilities and unresolved offensive layers use physical
protection. Monster basic attacks and redirected monster strikes use the same
helper. Periodic damage is separate from attack-category modifiers. Native
Shield Defense (buff layer 19/code 52) increases DEF and MDEF by 10%; Water
Shield (protection layer 4/code 105) reduces magical damage by 50%. Authored
physical protection uses `physical_damage_taken`. Authored generic `damage_taken`
effects continue to protect against both attack types.

## Party combo grouping

Use the shared `Rules.comboGroups` eligibility and adjacent speed-gap rules for
both round scheduling and execution. Preserve the collected speed snapshot and
turn barriers. Revalidate chains after removing unavailable or redirected bridge
attackers. Do not reintroduce pair-only damage loops or ability-specific combo
exceptions; area eligibility comes from the SQL skill metadata at learned grade.
Keep SP costs, hit protection and durable proficiency per participant. See
[COMBOS.md](COMBOS.md) for evidence and compatibility limits.

Combo probability is an execution decision, separate from speed-chain grouping.
Use the whole present player/pet roster's fractional average level, with a linear
curve from 0% at −25 through 50% at equal level to 100% at +25 against the actual
target. Roll once per eligible chain; a failed chain executes as singles without
rerolling subsets. Skip random draws for singles and the 0%/100% endpoints.

For combo calculations, add `comboRebirthLevelBonus` (99) to each reborn player
or pet before averaging. Use `Fighter.comboLevel` for both roster participants
and the target. Keep the result as an integer; never write it into native
byte-sized levels or persisted EXP. Characters store optional `reborn` metadata;
pets retain their existing flag. Rebirth progression and character roster/job
presentation require separate migration work.

## Skill JSON and reusable effects

Native skill exports preserve all decoded fields and bytes and add
`fields.effect_refs`. The source-derived `data/skill_effects.json` contains named
`records`, each with a stable string `id`, a readable name and an `effects` list.
Definitions reflect decoded native classes and the documented compatibility
policy; they are not a claim that unknown native semantics have been recovered.
JSON schemas live in `schemas/skill_data.schema.json` and
`schemas/skill_effects.schema.json`.

Use either inline `fields.effects` or `fields.effect_refs`; mixing them is an
error. Empty lists intentionally disable effects. Reference lists concatenate
effects in order and validate the final list, including trigger consistency.
Identifiers must be unique; missing or duplicate references fail startup. The
SQL loader clones resolved effects so skills do not share mutable modifier lists.

Typed `catalog_skill_effects*` rows own definitions and `catalog_skills*` rows
own skill references. Use the `SkillEffects` and `Skills` administration datasets;
legacy `asset_skill_effects` views are provenance and may be stale after edits.
Every SQL skill must explicitly provide `effects` or `effect_refs`, including an
empty list when it has no effects. The loader validates and resolves the typed
metadata; it does not infer behavior from native codes or embedded policies.

Ability-specific conversion mappings and tuning values live in
`internal/assets/native_effect_rules.json`, outside the derived `data/` tree.
This authored compatibility policy is conversion logic expressed as data; it
contains native layer/code pairs, readable names and generic effect templates.
Template `rounds` are added to native field 51 (normally one casting round plus
following rounds). Its schema is `schemas/native_effect_rules.schema.json`.
Add or change mappings, magnitudes, targets and capabilities there without adding
ability-specific Go constants or switch cases. Keep unresolved native semantics
explicitly documented. The native decoder embeds this policy for offline tools
and reference tests; SQL gameplay loading never consults it.

For a one-off alternate conversion policy, use `go run ./cmd/skill-effects-export
-skills <fresh-native-skill-json> -effects <output-json> -rules <policy-json>`.
For an existing installation, edit the authoritative SQL definitions through
the validated database interface and reload/restart as appropriate. Offline
conversion policies supply reproducible seeds and source projections. A deliberate
source rebuild replaces the asset database, so preserve administrator edits
separately before rebuilding; normal initialization must not replace them.
Do not use an old exported asset as an original decompilation input.

Fresh source-only exports annotate skills automatically. To refresh only skills
and effects from the preferred sibling source, use
`python3 tools/data_export/skill_effects.py --output data --overwrite`. This
replaces edited definitions; preserve authored changes separately before running
it. Rebuild the asset database with `go run ./cmd/data-rebuild -data data
-assets-db var/assets.db`, then restart the server to load the snapshot. The
command backs up the old asset database; accounts and characters stay in the
gameplay database. Native descriptions and decrypted bytes stay unchanged.

## Server chat

AC2:2 keeps map-scoped local chat, excluding sender echo and respecting recipients'
local-channel settings. AC2:1 and `/world <message>` deliver world chat; `/team`
(or `/party`, `/p`) targets only the sender's party. `/whisper <name-or-ID>
<message>` (or `/w`, `/tell`) resolves exact names or numeric IDs, never fuzzy
matches. Recipient channel settings apply; absent/loading characters receive no
messages. Failed recipient writes close that recipient without disconnecting
the sender.

Messages allow up to 60 bytes of native encoded text; channel commands allow
512 bytes including command/recipient overhead. Reject empty, overlong or
ASCII control-bearing messages. Public channel commands run before the GM
authorization gate; existing administrative commands retain that gate.

World, local, whisper and team chat use native AC2:1/2/3/5 layouts; guild
membership is SQL-authoritative and AC2:6 or `/guild` reaches its online members.
World/local exclude sender echo for native sends; team/guild and whispers include
the appropriate sender copy. GM announcements use AC2:4. Feedback/errors use
AC2:16 head banners. Go-client channel controls and colors are implemented;
the separate guild-management UI remains pending.

## Pet rebirth

Use `game.Pet.Rebirth` for the reference AC69 ascension transition. Eligibility,
reset level and bonus points have descriptive package constants. Select owned
party pets by their per-login client slot, never the persisted party slot or a
reserve/hotel pet. Preserve base stats, skill progress, equipment, job and active
selection, and normalize/refill vitals before saving. Reject stat-point overflow
and repeat ascension. Publish the success acknowledgment, roster and progression
only after the save succeeds. Keep rebirth inside world interaction gates so it
cannot change a collected battle roster or bypass trade/event ownership. Quest
and model evolution are separate mechanics until their source semantics are
ported; do not invent new NPC IDs or grant additional skills during ascension.

## Companion training

AC68 selectors 1–5 map to named AC8 attribute identifiers; these are separate
protocol namespaces. Use `Pet.AllocatePoint` to spend one shared unallocated
point and reject uint16 overflow. Existing bulk AC8 wrapping behavior retains
its own compatibility tests. Normalize current vitals without refilling, persist
first, then emit progression and the success receipt. Invalid selectors and
exhausted budgets receive failure receipts without spending points.

Funded potential training uses native AC23:126 for normal, golden and Super
Potential Pills; see [Potential Pill consumption](#potential-pill-consumption).
Keep it separate from manual stat allocation. The reference's free AC68 increment
remains rejected, and stat37 must not encode potential. Player/pet potential is
typed durable state, with compiled success rates and native attribute bonuses.

## Battle-pet feeding

AC67 requests identify food by item ID, while AC23 item use identifies its bag
slot and client pet slot. Route both through `feedPet` so status-64 SQL effects,
amity caps, item consumption and persistence use one policy. AC67 selects only
registered party pets marked for battle and consumes one unit from the first
usable matching stack in bag order. Preserve the stack's metadata and publish
its receipt after durable consumption. Never infer food effects from names or
maintain a separate list of food IDs. Shared feeding must reject invalid slots,
zero counts and empty stacks before indexing or mutating state.

## Inbound command routing

Register each inbound opcode once in `internal/server/commands.go`, with
its handler and `protocol.CommandPolicy`. The zero policy grants no world or
interaction exceptions. Use `IsWorldCommand` and `IsDeleteCharacter` for semantic
classification; classification does not validate payloads, authorize accounts,
or create a generic packet reply. Handlers retain their native response layouts.

Declare battle, minigame and trade policies alongside the handler. Exact native
login synchronization subcommands may run before world interaction gates; keep
those exceptions narrow and let the handler validate the complete packet.
Map acknowledgments and portals retain their separate lock ownership. Preserve
gate ordering when adding policies. Avoid new parallel opcode lists in dispatch
or interaction gates; keep switches that decode distinct operations inside a
handler. Tests retain independent raw opcode tables to verify routing parity.

## Native text mail

Use the gameplay store's `SendTextMail` transaction for AC14:1 text mail. Verify
sender account ownership and recipient existence before acknowledging acceptance.
Persist native text bytes losslessly and reject content above 255 bytes.
Deliveries carry the text as the rest of the packet without a length byte:
aLogin reads everything after the date as the letter (case 0xe/1 at 0x2e29cc). Line breaks and tabs are allowed. Preserve the
ignored native type byte without assigning parcel or attachment semantics.
Use UTC OLE Automation timestamps in delivery packets.

Deliver pending messages only after the recipient's map snapshot is complete.
Hold `worldMu` to serialize queue admission with logout and map transitions.
Mark each message delivered only after observing its successful socket-write
receipt; queue acceptance is not delivery. Failed writes leave it queued. The
mail receipt worker retries failed SQL markers without sending the same live
notification twice, and continues subsequent batches only while the recipient
is ready. Disconnect cleanup observes completed receipts before dropping the
session. See [outgoing networking](#outgoing-networking).
There is no native receipt acknowledgment, so delivery is at least once across a
crash between writing and marking. Keep recipient failures isolated from senders.
Foreign keys cascade mail when either character is deleted, preventing correspondence
from reaching a newly created character that reuses an ID. Parcel attachments,
mailbox management and the ambiguous AC14:1 short name fallback need separate
verified operations; do not implement them through the text-mail type byte.

## Native contact compatibility

Keep inbound AC10 social operations and outbound AC10 presence names separate.
Register `CommandSocialRelations` once with its world policy. AC10:1 and accepted
AC10:2 add same-map published contacts immediately, as the reference does; AC14
continues to require a pending invitation. Share the friendship store, bilateral
limits, removal and deletion cleanup. Clear obsolete invitations after a durable
addition, and publish success only after persistence.

Use the existing AC14 friend snapshots for AC10 list/status queries. Do not
maintain a second in-memory contact roster. Isolate recipient write failures from
the actor, including removal refreshes. Retain independent raw native status and
removal packet expectations and cross-protocol tests. Keep actual-client capture
acceptance separate from source and synthetic-packet verification.

## Bank currency

Keep bank and carried gold separate in `game.Character`. Follow
`DepositGoldToBank` and `WithdrawGoldFromBank` for currency movement: reject zero,
insufficient funds, native uint32 bank overflow and transfers exceeding `MaxGold`.
Reject before changing either balance so their combined total stays constant.
Both counter and ATM transfers use `Store.TransferBankGold`: one conditional SQL
update rechecks character ownership, durable funds and both caps, then reads back
both balances in the same transaction. Update only those two state columns; do
not checkpoint pending walking, recovery, pets or inventory. Refresh only those
session/baseline currency fields after commit. Do not introduce a second bank
table or a separate save. Older JSON states default to zero bank gold.

Publish AC26:4 wallet refresh and native AC45 balances only after commit. Receipt
write failures cannot undo a durable transfer. Keep bank operations inside the
shared world/trade gates and scripted interaction ownership. PIN and character
transfer reference handlers contain no working behavior; retain explicit
unavailable responses until verified semantics exist. Keep actual-client
activation and packet acceptance separate from synthetic protocol verification.

The bank counter (event service 6) opens with AC29:5 followed by AC29:4 carrying
the account balance. The native form then sends AC29:2 to deposit and AC29:1 to
withdraw, both with the amount as a U32, and keeps an in-flight flag that only a
completion clears: AC29:8 after a deposit and AC29:7 after a withdrawal. Answer
an accepted operation with AC26:4, AC29:4 and that completion; answer a rejected
amount with AC29:3, the unchanged AC29:4 and the same completion so the form
unlocks. Clinics use event service 7. The AC45 subcommands remain the balance
and ATM query/receipt layout, but the counter's own currency path is AC29.

Queue a scripted bank opening on its owning event, with the character identity.
`finishEvent` sends the movement release/resume before opening the bank; it must
work without an extra acknowledgement of that resume. Cancellation, replacement,
map changes and character handoff discard the old intent. With no active event,
ignore trailing AC20:6 acknowledgements at any latency. Never answer an idle ACK
with another resume or use a wall-clock debounce to hide that protocol loop.

## Native mall checkout

AC34:1 reads current account points and returns AC75:3/9 followed by AC35:4 to
resume the client's pending cart. Build all replies from one SQL snapshot, and
use the shared mall wire-range conversion. Preserve both accepted mode bytes and
the source's ordinary-point resume until their separate semantics are verified.
Use numeric fallback names for unresolved modes.

This handshake must not refresh catalogs, mall settings or status, which can clear
the selected cart. It spends no points and grants no inventory. Keep checkout in
the shared world gates; trade and running minigames may allow this read-only
query while the eventual purchase retains its mutation gate. The fruit machines
(sport kinds 11, 12 and 14) send it from their constructor, so a minigame gate
would leave them without a balance. Balance read failures cannot publish success
or overwrite the session cache. Preserve independent raw resume bytes and tests
that submit the existing cart after the query.

## SQL gacha pools

Author pack membership and ordered reward rows in the assets database rather
than extending Go ID lists. Each pool has one to 41 rewards, positive integer
weights totaling 10,000 and quantities one to 255. Native pack IDs provide only
fallback recognition of unavailable legacy packs. Lucky Pack 34333 remains
withdrawn. The strict offline validator requires every item definition.

The offline compatibility importer validates the whole authored table before
selecting available pools; malformed definitions fail import. Missing item
definitions disable the entire affected pool with an explicit diagnostic while
other complete pools remain available. The typed runtime catalog loads that
projection, including unavailable pack IDs and diagnostics. The `GachaPacks`
editor validates complete pools against the current item catalog. Never drop
unavailable rewards or redistribute their weights; keep disabled configured IDs
recognizable for item use and mall filtering.

Runtime uses typed `catalog_gacha_packs` and ordered child reward rows.
Use the `GachaPacks` administration dataset; legacy import views are provenance.
Preserve atomic pack removal and reward grant before publishing native receipts;
use fresh reward metadata and cryptographic uniform rolls. Changes to source
pools belong in sibling inputs; exported data remains derived.

## Daily Lucky Draw

Use `game.LuckyDrawState` for three draws per character per UTC calendar day.
Keep usage in `character_state.lucky_day` and `lucky_used`. Reset eligibility follows its saved date;
do not refill a session counter on login, accrue missed days or replenish draws
after clock rollback. Grant inventory and consume allowance in `Store.DrawLucky`
using one transaction against current durable state. Publish receipts after
commit; failed receipts cannot restore credits. Preserve allowance on full
inventory or failed persistence.

The SQL startup catalog owns the ordered reward pool. Each outcome has
`item_id`, `quantity`, positive integer `weight` and a one-based `slot`.
Slots must follow SQL row order consecutively from 1; the native UI supports at
most 20 rewards.
Weights are relative and their total must fit int64; use cryptographic uniform
rolls across the total. Different quantities of one item are separate outcomes.
Defaults give all nine outcomes weight 1 (approximately 11.11% each), including
three Fun Tokens (34008). Obsolete seasonal
exchange items Qiqiao Poems (30436) and Ghost Fest Voucher (30437), including
their quantity variants, are excluded from the default pool. Replacement rewards
require an explicit content choice; keep slots consecutive after edits. Missing definitions, invalid quantities,
missing/out-of-order slots, more than 20 rewards and overflow fail startup; an explicit empty pool disables draws.

Runtime uses `catalog_lucky_draw_rewards` and the `LuckyDraw` administration
dataset. Edit typed SQL rows and restart or reload the snapshot.
Initialization defaults belong in
`internal/assets/lucky_draw_rules.json`, the offline compatibility policy.
Updating defaults does not replace existing database edits during normal
initialization. For an explicit source rebuild, regenerate with `python3 tools/data_export/lucky_draw.py
--output data --overwrite`, rebuild the assets database, then restart. Full
source-only export includes the projection. Generation reads fresh sibling
Item.dat, never saved item exports. Item mappings remain compatibility choices; catalog order determines native
presentation positions.

Send the native catalog `[104,1,1,used,count,(item:uint16,quantity:byte)...]`
at login, warp arrival, character refresh and midnight UTC. Send successful
results as `[104,1,2,slot,used]`. Both usage fields are cumulative draws used;
the client displays three minus that value. Keep request `[104,1]` separate
from outgoing catalog/result layouts. Never send a constant one in the result
usage field or use AC35:12 as the allowance. Derive effective usage from the
saved date without prematurely saving a reset. Honor shared world,
trade, battle and scripted/cutscene gates. Public announcements and paid draws
require separately verified semantics.

## Periodic character checkpoints

Gameplay mutations must still commit before success packets. The server autosave
worker checkpoints dirty recoverable session fields at `character_save_seconds`
(default 30 seconds), with final attempts on disconnect and graceful shutdown.
The checkpoint uses a cloned baseline and the Store's transactional
`AutosaveCharacter` comparison; the one-second world/gathering ticker is separate. Never replace this with an
unconditional session snapshot write: stale caches could undo rewards or recreate
state after deletion. A conflict leaves durable state and the baseline unchanged;
resolve conflicting mutations through the subsystem that owns them. Already saved
or unchanged snapshots avoid writes. Account balances remain authoritative in the
Store and must not be restored from cached session values. Access session snapshots
under `worldMu`; retain warp presence and bound persistence work with cancellation.

## Timed gathering rewards

Keep event timer expiries in persisted character state and clone timer maps with
character snapshots. Evaluate a branch's timer conditions against one instant.
Resource rewards that start cooldowns must validate the complete supported branch
and recheck current durable conditions inside the same transaction that grants
items and records marks/expiry. Require the authored capacity prerequisite even
when an existing stack has room. Emit reward packets only after commit. Do not
enable a general timer opcode or unknown reward pool merely because one verified
resource workflow is supported; add its data/semantics and atomic validation first.

## Permanent event transformations

Enable verified transformation operands only within their recognized authored
scripts. Keep the static SQL catalog immutable when correcting branch order or
execution. Save model changes and their unlock/completion marks together using
fresh character state inside the gameplay transaction, before publishing success.
Breillat's conversion removes all copies of the original character's standard
starter outfit from the bag and worn slots, returns other worn items to the bag
with their damage and metadata intact, and equips Breillat's starter outfit.
Use the shared starter outfit mapping and SQL item definitions. Commit the outfit,
model and marks together; reject insufficient bag space or reserved affected items
without changing anything. Publish bag deltas and the complete worn list after commit.
Unsupported model conversions remain rejected before
ordinary branch mutations. See the Breillat regression suite for the ten-talk
unlock, decline, stale state and failed delivery cases.

Breillat's verified voucher branches take precedence over greetings and the
transformation offer. Read item IDs, quantities and conditions from the SQL
event; do not add a parallel hardcoded reward catalog. After the dialogue,
recheck the selected branch against fresh owned character state and commit
voucher consumption plus the complete reward in one transaction. Preserve
pending walking and item reservations. The native Cabin event accepts voucher
30002: one gives Vanilla Ice Cream 32071; two or more consume two for Chocolate
Ice Cream 32072. Map 10002 gives two chocolate items, while variants 10021–10030
give one. Same-named vouchers 30029 and 30030 are different content references.

## Character growth settings

Keep player growth tuning in `internal/game/growth_parameters.go`, in the
compiled `DefaultElementalGrowth()` table. Each element has its own complete
level, attribute and vital coefficients. Standard combat coefficients follow the
saved WLRI Japanese wiki in docs/References; HP/SP follow Formula.Dat. Keep
independent reference-value and formula-export tests to detect accidental drift.
Edit the table and rebuild to change
gameplay; do not expose these values in startup JSON or web administration.
Use the shared character creation, combat, refill, recalculation and stat-packet
helpers in gameplay handlers instead of duplicating formulas. HP/SP bases and
equipment bonuses stay outside the growth multiplier. `baselineGrowth()` and
`nativeElementGrowth()` are fixed native-client compatibility references;
keep them independent of gameplay tuning so packet adjustments still work after
rebuilding with different coefficients. Growth arguments on low-level helpers
allow isolated formula tests; production server handlers use the compiled table.
Run focused growth tests after edits, including validation of finite coefficients
and bounds. Pet and monster growth are separate policies. See
[CONFIGURATION.md](CONFIGURATION.md#elemental-stat-growth) for field meanings and
build instructions.

## Pet level-up stat distribution

Pet automatic growth allocates one attribute point per gained level. Keep all
five attributes eligible and weight them through `Pet.growthWeights`; do not
restrict candidates to the strongest attributes. `PetGrowthBaseStats` uses known
NPC template stats or current attributes when the template is missing.
`PetGrowthCombatStats` maps STR/CON/INT/WIS/AGI to current ATK/DEF/MAT/MDF/SPD from
`Pet.Combat`, including equipment and existing elemental modifiers. Recalculate
weights for each point after increasing the level. Vitals and temporary battle
effects are separate and do not add weights.

The formula selector is startup JSON `pet_growth_formula`, captured by `Server.New`
and immutable for that process. Do not expose it through live runtime operations
or persisted database settings. Route automatic EXP growth through `gainPetExp`
so battle rewards and GM pet EXP use the same selection and catalog. Pass an
explicit roll function for independent interval tests. Retain minimum weight one,
exclude capped attributes before summing weights, and skip the draw if all are
capped. Keep manual AC8/AC68 spending separate. Test exact intervals, equipment
bonuses/penalties, elemental behavior, per-level recalculation and startup-save
immutability; avoid probabilistic sampling tests.

## World Entry policy

Validate `Appearance.ValidateCreationAllocation` on incoming creation requests:
exactly `game.CreationStatPoints` (five) base points, excluding model bonuses.
Do not impose creation budgets on stored characters, GM edits or level-up
allocation. Low-level appearance decoding preserves fields independently.

Derive elemental starter skills from the zero-requirement entries in
`internal/game/skill_tree_data.go` through `game.StarterSkills`, plus the avatar
stunt. Creation and `/clearskills` then use `UnlockQualifiedSkills` with actual
attributes for gated skills. Do not maintain a separate elemental starter ID
list: the legacy starter helper incorrectly bypasses its own progression gates
for Water Icicle Attack, Earth Attack and Wind Air Attack. Preserve existing
learned skills during startup/login; explicit skill reset is a separate action.

Keep entry synchronization read-only: restore durable monster discoveries and
story completion marks before final ready markers. Discoveries are character
state saved together with victory rewards. AC53:9 carries the NPC template ID,
not its book index. Unknown/out-of-range book entries are omitted from snapshots
without rewriting the saved discoveries. AC15:19 lists earned star IDs in native
UI order. Updating a story mark publishes its collection after commit.

AC89:0 and AC92:1 are scene-ready synchronization, independent of AC12:1 map
publication. Share their once-per-login MOTD flag, validate their complete
request layouts (AC89:0 supports two or six bytes; AC92:1 supports two or three)
and keep world interaction gates unchanged. The four-byte native AC89 metadata
must never select or authenticate a character. Do not introduce a
socket transfer/token protocol without native evidence: the reference queues
its existing authenticated connection into WorldServer.

The optional `QuestVisibility` administration dataset stores ordered typed
`catalog_quest_visibility*` rows. Definition API payloads contain the current
version and a `value` array, not a filename-based JSON document. Preserve reference
registration order: completed quest despawns precede quest spawns; active-step
despawns precede step spawns. Spawn lists own visibility even when inactive.
Native story/party and explicit administrator visibility retain their earlier
precedence. Catalog publication or `/reload quests` loads a fresh immutable
collection; no runtime file fallback is allowed. The reference loader has no
populated lists to extract. See [quest actor visibility](ADMINISTRATION.md#optional-quest-actor-visibility).

## Combat sides, AI and trial content

- Resolve allies and opponents with `battle.Teams(side)`. Player/pet commands,
  menus, timeouts and disconnect accounting cover both formations. Avoid using
  `Defenders` as a synonym for monsters. Gate captures and PvE economy/notebook/
  quest rewards by battle kind and fighter kind. PvP grants no EXP, gold, drops
  or captures; proficiency still uses the ordinary executed-action path.
- Monster skill lists come from the SQL `npc.dat` skill slots captured when an
  encounter starts. `monster_ai.go` selects distinct affordable skills, using
  generic effect targets and the shared damage/support implementation. Keep
  ability IDs/names out of buff/debuff decisions. Healing retains the existing
  name-based compatibility classification. No new ability-specific exception
  belongs in the AI.
- Palace definitions use operator-authored typed SQL rows through the
  `CombatTrials` administration dataset.
  They are not decrypted external assets and must not be generated into `data/`.
  Validate all identities against the SQL catalog and freeze the definition for
  the battle. The C# stage placeholders are invalid for WLRI; do not restore
  its immediate free-reward behavior. See [ADMINISTRATION.md](ADMINISTRATION.md#palace-trials).
- Legacy monster-ID quest fallbacks run only for a PvE victory without an EVE
  callback or trial owner. Stage completion, companion recruitment and currency
  in the same character clone as battle settlement. Captures are not defeats;
  completed quests cannot pay twice. Keep failed saves from changing client
  rosters, quest visibility or success packets.

## World Simulation

Use the SQL-derived `Catalog.Terrains` for scene bounds and native X-major,
20-pixel collision grids. Never open Ground.MMG or its JSON export at runtime.
Terrain cells are server geometry stored as BLOBs, not image payloads. Schema v5
projects imported Ground.MMG rows during explicit offline migration. Preserve
existing typed edits and reject malformed geometry transactionally.

Native 15-byte AC6 packets announce a destination before animation completes;
validate that endpoint against terrain/bounds without treating the previous
announced destination as the current rendered position. Rejected native endpoints
are ignored, avoiding an AC7 teleport during a walk. Short compatibility packets
retain straight-segment validation and AC7 correction. Rejection precedes dirty
state, trade cancellation, encounter progress and vehicle wear. Impose no elapsed
speed/distance cap. A blocked saved spawn may recover to a valid destination;
AC6:2 stop reports update the stopped position without another encounter/wear step.
The eight native movement-check bytes remain opaque.
Validated active item vehicles retain native water/air travel within scene bounds;
land collision bits are not a vehicle capability table. Missing grids preserve
legacy movement, without falling back to files or fabricated geometry.

Player visibility is personal in introductory Ship Deck, Cabin and Robinson's
Island (WLRI "Newbie Island"). `World.HideOtherPlayers` uses SQL EVE scene IDs
10001, 10002 and 10003, covering every map variant without hiding players on
later ships or cabins with similar names. `Server.samePlayerScene` gates player
arrival snapshots, movement, pets, vehicles, poses, local chat, battle indicators,
team formations and local player lookup. Other players' tent signs are also
hidden. NPCs, authored events and terrain keep their existing behavior; shared
tent interiors keep owner-based visibility. Definitions are read from the SQL
startup catalog; the visibility policy lives in `internal/world/player_visibility.go`.

Spawn definitions remain immutable. `World.NPC` and `World.NPCs` report current
ambient positions for map entry, interaction distance and encounter checks.
Serialize occupied-map simulation with session events under `worldMu`; lock
actor snapshots independently and never hold monster locks while resetting
actors. Preserve private tent isolation, actor concealment, companion ownership,
battle/event reservations and recipient failure isolation. Ambient patrol cursors,
positions and deadlines remain transient; monster respawn returns to its authored
spawn and resets its patrol with the reference three-second grace.

Bounded wandering, random roaming, patrols and pursuit wait a random 1–3 seconds
after estimated arrival before choosing the next route. Their first movement
uses the same 1–3-second startup delay; acquiring a player can interrupt that
wait. Bounded and random wandering pick the longest walkable leg among four
random candidates. Bounded wandering retains its authored spawn-relative
rectangle, 300-pixel offset cap and coordinate limits. Random roaming uses a fixed
rectangle extending up to 150 pixels per axis from its authored spawn; its area
never follows the moving actor. Patrols retain their authored waypoint order,
but use the common stationary wait instead of authored waypoint delays.
Tune these compiled values in `internal/world/simulation.go`; rebuild the server
after changes.

Hostile bounded, random roaming and patrol actors search for the nearest eligible
player every three seconds, within 200 pixels of their current position. Bounded
and random roaming actors acquire and chase players only inside their fixed
areas. Pathless patrols use the random roaming area; authored patrols may pursue
nearby players throughout their scene. Existing town restrictions still prevent
random roaming there. Cached targets are revalidated each tick and discarded as
soon as they become ineligible or leave the area or detection radius. Pursuit uses
legs of at most 100 pixels. Patrol cursors pause while chasing and resume afterward.
Keep the chosen player while eligible, even when a nearer player appears. A
monster without a player target searches every three seconds, including while
walking toward a wandering or patrol destination. Acquiring a player interrupts
that nonplayer leg or its stationary wait immediately. Losing a target starts a
fresh three-second search interval. Failed pursuit routes retry after the common
1–3-second wait.

Hold an active pursuit destination until estimated arrival. Native WLRI AC22:2
dispatch calls `FUN_0041869c`; its speed helper `FUN_00482980` computes
`speed / 8 * 0.16` pixels per millisecond, and `FUN_004126b8` advances along
the direction using elapsed milliseconds. The server's speed byte 2 therefore
gives a nominal 40 pixels per second. Estimate arrival from straight-line distance
at this rate, rounded up to milliseconds. NPC snapshots interpolate positions,
and interrupted paths start from the estimated current position. The protocol
supplies no NPC arrival acknowledgement, so client frame rounding, latency and
pauses can still affect visual timing. Ambient contact checks wait for estimated
arrival and run before issuing another walking leg. Respawn clears the previous
leg's arrival deadline.

Collision checks allow an axis-aligned approach when a diagonal is blocked; this
does not implement navigation around arbitrary obstacles. A battle starts within
the existing 72-pixel encounter radius only when the intervening terrain is clear.
Movement-triggered proximity encounters also respect the actor's target area;
explicit clicks retain their existing interaction rules. Native roaming actors
run their authored EVE battle formations. Map-entry grace, post-battle cooldowns,
concealment, owned companions, private tents, GM invisibility and active
interactions prevent ambient targeting. Target scans and detection radius are
compiled constants in `internal/server/simulation.go`. Actor positions, targets
and timers remain in memory; no movement or target scan writes to SQL.

Clicking a wild enemy or starting a proximity/native roaming encounter hides
and reserves its overworld actor for all public-scene viewers when battle starts.
The player's existing fighting animation remains on the map. Victory, defeat,
fleeing or the last participant disconnecting releases the reservation and starts
a 30-second respawn delay (`world.EncounterRespawn` in
`internal/world/monsters.go`). Leaving one member of a party battle does not release
the actor while others remain. Respawn restores the authored spawn position and
the normal three-second movement grace. The reservation and delay are transient;
a server restart restores the map actors. These rules extend the legacy behavior,
which hid ordinary wild actors only after victory for 60 seconds.

## Native compatibility handlers

Keep native aliases in the command registry and reuse the owning subsystem's
validation and transactions. AC7 waypoints must pass the ordinary terrain,
vehicle and event checks and remain buffered like AC6 walking. Read-only
synchronization may run during loading/battle; paid AC21 purchases retain all
world/trade/battle gates. AC32:3 clears callbacks even when no pose changed.

Title and AC66 reborn-job metadata belong in typed gameplay columns (schema v12).
Commit them before packets and update only their corresponding checkpoint fields;
never advance the saved position merely because metadata was committed. AC66
metadata is distinct from actual rebirth/class advancement and stat modifiers.
Fishing cast state stays in memory. Fishing deadlines/catch counts live in
`fishing_progress` (gameplay schema v13); SQL rod ownership, reward delivery and
skill proficiency commit together. Read rules from the structured `Fishing`
dataset (asset schema v8), with no runtime seed fallback. Full-bag fishing
retains proficiency and discards the catch. See [FISHING.md](FISHING.md).

### Manufacturing and character state

See [manufacturing/character state](MANUFACTURING_CHARACTER_STATE.md). Gameplay
v14 owns nickname/class/potential, pet potential, second-floor tent metadata and
manufacturing escrow. Asset v9 owns full Compound2 formulas, rebirth cape mappings
and authored recipe fee/chance projections. Debit ingredients/create escrow and
deliver output/delete escrow transactionally before acknowledging. Recover pending
jobs once per ready session; never reload or save walking on each timer tick.

### World and combat parity

See [world/combat parity](WORLD_COMBAT_PARITY.md). Gameplay v15 extends owned
shared prop cooldown rows with explicit frames; preserve old broken-frame values
when upgrading. Scripted frame/reset writes precede broadcasts; quest frames stay
private. Starter packs are saved with character creation and never refilled on login.
Explicit Admin grants validate SQL ownership and deliver whole packs atomically.
Manual mass grants atomically audit each successful recipient and keep pending
walking; enforce a bounded batch context and report failures individually.
Clinic rest includes the accompanying roster only. Broadcast accepted action
ACKs inside the battle, never to map spectators. PvP reward suppression is an
explicit owner decision; do not restore the source grant while porting handlers.

### Administration and custom quest data

Use optimistic SQL versions for tab-scoped character and linked-prop edits.
Validate and commit before publishing native updates; preserve unrelated state
and pending walking through `adoptSavedCharacter`. Block loading, battles,
scripted interactions, trades, stalls and private tent edits. Full character
edits retain reconnect; live inventory/stat/settings/quest tabs clear native
records that overlay rather than replace. Admin actor map scope is visit-local
and cannot change another viewer's durable quests.

Custom quest definitions use asset schema v10 typed child tables. Native Mark.dat
text/IDs may seed metadata during explicit offline migration, without guessed
rewards or destructive regeneration. Keep source reachability clear: registry
editing/loading and PvE defeat counters are implemented; custom NPC acceptance,
advance, completion, repeat/daily/cooldown and reward execution remain dormant.
Count only defeated noncaptured monsters in PvE victory settlement; persist kills
and ordinary rewards together. Never publish objective notices after a failed save.

### Verified migration compatibility

AC8 uses the legacy greedy amount decoder (4/2/1 available bytes); do not replace
it with uniform batch widths without native evidence. Skip invalid, unaffordable
or overflowing attribute allocations independently, preserving remaining points
for later entries. This matches the pet source guard and deliberately protects
character allocations from the source's UInt16 wrap.

Keep default replies local to their owning handler. Unknown AC45 subcommands are
read-only balance queries; PIN/transfer stay unavailable. There is no default
ACK for unregistered commands. Do not copy AC03's short replies: native AC3 is a
full self-character/map snapshot. Use the verified AC12/89/92 synchronization.
Source Tradeable/use-type metadata has no restriction caller in the inspected
trade/wear paths. Preserve historical recipe order in typed SQL arrays for
provenance, and retain first matching workbench recipe selection for manufacturing.
Gameplay compounding intentionally replaces the legacy recipe/fallback paths
with the rank/base model documented in [COMPOUNDING.md](COMPOUNDING.md).
Its approved probability curves are compiled in `internal/game/compounding_policy.go`;
all inventory consumption/output/proficiency must commit together from fresh SQL
state. Invalidate the derived alchemy candidate index after catalog replacement.
See [focused verification](TESTING.md#migration-verification-recorded-on-2026-10-04).

## Inventory footprints

Pass `s.Assets.Items` (or the equivalent SQL-derived catalog) to inventory
operations. Item `CellWidth`/`CellHeight` come from decrypted native disk offsets
406/407. Native `TRE_ItemGrid.FUN_0010f8ec` / aLogin `FUN_0011010c` reads object
offsets 0x19a/0x19b, including the four-byte prefix. `FUN_0010fa2c` derives
row-major anchor rectangles; the grid has five columns and ten rows. A zero
in either size field falls back to 1 × 1, matching the native grid helper.

Persist and transmit anchor items only. Derive occupancy with
`Inventory.Occupancy`; use `Grant` for automatic placement and `CanPlace` when a
protocol operation requires a particular anchor. Covered cells are unavailable,
even though their anchor records are empty. Plan debits and delivery together;
insufficient rectangular space must not consume payments or mark rewards claimed.
Do not add a global catalog or persist duplicate child item records. SQL asset
schema v11 explicitly seeds typed sizes from retained SQL bytes; gameplay reads
the typed sizes exclusively. See [Items and Player State](ITEMS_PLAYER_STATE.md).

## Live legacy-client acceptance fixtures

Use [native-client acceptance](TESTING.md#native-client-acceptance) and `cmd/live-test` to prepare a fresh
isolated gameplay database with scenario-specific characters. Load assets
read-only from SQL; never alter the regular gameplay database or active sessions
for fixture setup. Generated credentials and startup config belong under ignored
`var/` with private permissions. Preparation and automated fixture tests must not
be reported as passing native-client acceptance.

Map transitions must refresh the session checkpoint from the state actually
committed to SQL before sending arrival packets. A narrow location update must
not declare unrelated pending session fields durable. Carnie's authored EVE exit
must apply the same saved return point as its regular portal; its raw WLRI warp
record leads to Underground Maze and cannot be used directly for this exit.

### Potential Pill consumption

Route native AC23:126 through `potentialPillCommand` and
`Store.MutateOwnedCharacter`; never debit a session-only inventory snapshot.
Validate current SQL ownership, target, pill ID, limits and live reservations,
then commit consumption and potential-derived state together before AC23:213 or
stat receipts. Keep pending walking buffered. Effective attributes include the
compiled native bonus table in `internal/game/potential.go`; never rewrite base
allocation to bake in potential. Compiled enhancement coefficients in that
module follow the official WLRI potential table for normal/golden attempts;
Super Pills are guaranteed. A normal failure loses one level and forgets newly
unqualified stat-tree skills; a golden failure retains potential. Consume one
pill on either outcome, but none on a rejected request or failed persistence. See [native Potential Pills](NATIVE_COMMANDS.md#potential-pills).

## Client build and session ownership

Client-derived exports under `data/` and generated client resource manifests/
dimension baselines remain local and ignored, including JSON and lossless
sprite payloads. Retain `data/server/`, documentation, exporter code, authored
compatibility policies and independent fixtures. Regeneration reads the supplied
original client installation and combines it with tracked server-only exports;
it must not depend on a previous checkout's generated data or overwrite local
asset edits. See [ASSET_IMPORT.md](ASSET_IMPORT.md).

The Gonline client must implement the same protocol contract as the legacy
client. Fix missing client requests, replies and sequencing in the client;
do not relax server validation or add client-specific exceptions to accommodate
an incomplete client implementation. Preserve independent raw-byte fixtures
and verify the client against the existing server handshake and interaction gates.

Explicitly negotiated transport optimizations may run when both peers opt in,
while preserving native fallback and the same gameplay rules and gates. Combat
round chains and batched choices use the versioned capability exchange in
[COMBAT_TRANSPORT.md](COMBAT_TRANSPORT.md); client brand or server display names
must never select the alternate path.
Offer this optimization only when the startup configuration's
`combat_round_chain` switch is true (default false). Capture the switch in `New`;
keep native delivery for unnegotiated clients even when it is enabled.

Explicit escape results must remain separate from overkill and disconnect.
Either owner or pet escaping removes that owner/pet pair; only a successful
team-leader escape ends the remaining team's battle. Capture current leadership
under `worldMu` before round calculation. Use the strongest living opponent for
the reference level-based escape check. Keep withdrawal/timing state in memory,
but commit departing vitals and pet results before native success packets or a
round envelope. Truncate only that owner's chain; remaining participants keep
the rest of the round. Failure uses native motion 60042 without damage, SP cost,
defense or proficiency; retain 60041 as the repeatable request.

Use `make build-client` to validate/recompile editable assets before building Go.
`client/asset-contract.json` holds PNG dimension compatibility constraints;
`cmd/client-bundle` produces incremental core/world/sprite/audio packs with binary map and sprite
records without regenerating or
changing edited sources. Preserve source JSON fields and transparent PNG pixels.
See [CLIENT_ARCHITECTURE.md](CLIENT_ARCHITECTURE.md) for build/edit guidance.

The single-window workspace owns shared read-only definitions and decoded asset
caches. Each session owns its connection, UI/input, character/world actor state,
timers and automation. Never put mutable player state into `app.Resources` or
mutate shared pixel/terrain buffers. Main-thread session mutations are sequential;
socket callbacks post to the owning session. New disposal paths must reject late
dial results and release that session without closing shared resources. Keep
background networking/simulation active when thumbnails are not being redrawn.

## Client GPU drawing

Normal client windows use `client/wlo/render.Device` through `surface.Backend`.
Use surface draw/fill/blend operations for all frame rendering. GPU targets have
no CPU `Pix` buffer; direct pixel access is reserved for CPU compatibility and
immutable asset/glyph preparation. Increment `Surface.Revision` after directly
editing an uploaded CPU source. Allocate temporary GPU-compatible canvases with
`NewCompatible`, freeze with `Clone`, and release targets with `Close`.
Keep textures, glyph caches and drawing on the Ebitengine game thread. Sessions
share the device/asset cache but own their targets; disposing one session must
not dispose another session or the window's shared assets. GPU destination-reading
effects use GPU scratch surfaces, never per-frame CPU readbacks. Preserve native
integer color-key, alpha, light and scaling semantics with the actual graphics
parity tests in [CLIENT_ARCHITECTURE.md](CLIENT_ARCHITECTURE.md#gpu-rendering).

## Teams and instance rooms

Party vitals use the native AC8:3 layout decoded by `FUN_00451718`:
character U32, stat U8, sign U8, magnitude U32, target U32. Level is stat 35;
current HP is stat 25. Do not reuse Private Server's misleading U16 stat IDs.
Send CON/WIS, growth-adjusted HP/SP bonuses and rebirth so native clients can
calculate maxima. Keep independent wire fixtures for these fields.

Instance metadata belongs to `assets.Catalog.Instances` and SQL
`catalog_instances` (asset schema v12). Offline initialization projects
SceneData.dat; its map list supplies the entry map. Field +0x6b references
Mark.dat ID +1 and supplies the display title and description. It is not an
entry map. Runtime server code reads only the typed SQL catalog. Room membership
is transient presentation state, protected by `worldMu`, and ends on disconnect.
No fee or reward is granted by joining a lobby. Starting dungeons remains pending
until objectives and rewards are verified. Do not substitute guessed content.

Native AC85 browser/create/detail requests and replies are separate from
Gonline's explicit room-control extensions (200–203); see [Teams](TEAMS.md).

## Private Server updates

The separately pinned update ledger is [source-update-inventory.json](source-update-inventory.json).
See [SOURCE_UPDATES.md](SOURCE_UPDATES.md) for imported behavior and explicit
prototype/platform exclusions. Baseline source hashes remain unchanged. Asset v14
adds the update's typed content and performs preserving migrations of exact old
tent/poison defaults. Missing NPC route/sign/speed projections come from retained
SQL map sections and preserve edited definitions. Poison compatibility is now 10% of maximum HP, minimum 15;
customized effects remain authoritative. Native protocol goldens and compiled
growth formulas retain their independent ownership.

The story follow-up through `5c36839` adds forward-only Waterfall Grotto mirror
continuations and nonholding Fairy Altar movies. Existing visit-local scripted
actor visibility and map-entry reset remain authoritative. These fixes require
no database migration; the ledger preserves the earlier snapshot hashes.

## Story definitions and execution

Story-specific identities, prerequisites, links, presentation modes, actor rules,
script repairs, recruitment/conversion parameters and cinematic steps belong to
the typed `Story` dataset in assets.db (asset schema v16). Use reusable engine
capabilities rather than adding map/event/mark switches. See [STORY_SYSTEM.md](STORY_SYSTEM.md)
for scopes, gates, operation patterns, ordered rules and administration.
Keep imported EVE programs and independent raw packet fixtures intact. Apply
repairs to execution copies. Runtime constructors must not load seed defaults;
seeds are for explicit offline initialization and native compatibility fixtures.
Versioned migrations must retain edited and intentionally empty story definitions.
Recheck sensitive story actions against owned SQL state and commit delivery plus
checkpoints together before receipts. Preserve buffered walking when adopting SQL
results. Bound automatic chains and retain per-session completion tokens; story
content must not create unbounded execution or bypass battle/interaction gates.

## Server component layout

Use the [server component guide](../internal/server/README.md) to locate handlers
and their integration tests. Keep session ownership and checkpoint adoption in
`internal/server/session.go`; command registration and interaction policies belong
in `internal/server/commands.go`. Group small related adapters with their owning
feature. Independent packet construction belongs in `internal/server/wire`, and
administrative log capture/tracing in `internal/server/logging`. These components
must not import `server` or acquire gameplay ownership. Extract further packages
around narrow responsibilities without exposing mutable session fields merely
to satisfy a directory boundary. Preserve raw goldens, authored tables and source
inventory evidence when moving code. Include affected subpackages in focused
tests and pre-push validation.

## Outgoing networking

All accepted login/gameplay sockets use the bounded queue and single writer in
`internal/server/transport`. Construct them through `Server.networkSession`
before publishing a session. Handlers may enqueue while holding `worldMu`, but
must never wait for a socket or a write receipt under that lock. The transport
writer performs no SQL or gameplay callbacks and never takes server locks.
Socket-free administrator reply capture and deterministic fixtures retain direct
output; launcher status is a separate unframed response outside gameplay locking.

Each connection admits at most 4,096 frames and 2 MiB of encoded frame data,
including its in-flight write. Queue storage grows lazily. Payloads are copied
into owned encoded frames, so later caller mutations cannot alter queued data.
Frames remain ordered across concurrent producers and partial socket writes.
Overflow disconnects the congested recipient; never silently discard arbitrary
gameplay packets. Each frame expires ten seconds after enqueue, including queue
residence, so a backlog cannot renew its timeout on every write.

Successful `Session.send` means accepted into the queue. Keep purchases, trades,
rewards and gift claims committed before enqueueing success packets. A later
write failure must not undo or repeat a resource transaction. Mail delivery uses
a receipt on the final packet of each notification; only a complete successful
write permits its SQL delivery marker. Claimed GM gifts retain their claim if
notification fails. Receipt signals are coalesced; the worker visits only
sessions with pending mail work, retains dirty markers for retry, and uses a
five-second batch context under the existing world ownership. A one-second tick
retries deferred work without polling healthy players' mailboxes.

Connection close cancels queued output and interrupts the current write without
waiting for the writer. Session teardown joins it outside gameplay locks before
processing completed mail receipts and releasing presence. Explicit final
notices, including GM kicks and duplicate-login reasons, drain existing frames
under their original deadlines before closing. Server shutdown may abort that
drain. Closing sessions must reject further gameplay after acquiring world
ownership, including map acknowledgments and portals.

The global lock still serializes gameplay mutations and SQL work. This transport
change isolates slow sockets; it does not remove database contention or establish
concurrent-player capacity. Queue limits are compiled transport policy, with no
new configuration keys or database schema changes. Verification is recorded in
[TESTING.md](TESTING.md#outgoing-network-isolation--2026-11-01).

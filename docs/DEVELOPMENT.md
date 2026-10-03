# Development conventions

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

1. Name the meaning and namespace: `CommandTrade`, `TradeConfirm`, `MaxGold`,
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

During development, run only tests relevant to the modified modules and affected
integrations. Use focused test selections within large packages; protocol or event
changes should include their relevant native-data compatibility tests. Use the
race detector when changing concurrent behavior. Reserve full test-suite runs for
pre-commit validation. Run formatting, lint and build checks appropriate to the
change. Review namespaces as well as values: byte-for-byte tests cannot detect a
misleading name.

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
handlers. Route new static content through `internal/assetdb` with GORM and add
its runtime projection to `internal/assetsql/database.go`. Keep
`internal/assets` free of GORM and SQL imports: the client links it for the
native file decoders and must not pull in the database. Never add a runtime
JSON/native-file fallback. Use the gameplay store for player-owned state. Exclude original account
database snapshots and SQLite sidecars from asset export/import.
Retain unknown exported fields in SQL and preserve authored row order. Document
which SQL fields are authoritative when adding a projection, and validate before
publishing the startup snapshot. Keep file decoders for offline tools and
independent compatibility tests.

Run relevant gameplay tests against imported assets with
`WONDERLAND_TEST_ASSETS_DB=var/assets.db go test ./internal/<package> -run <tests>`.
Add `-race` for concurrent behavior. At pre-commit validation, run the full suite
with `WONDERLAND_TEST_ASSETS_DB=var/assets.db go test -race ./...`. Reference native
format tests still accept `WONDERLAND_TEST_DATA` and
`WONDERLAND_TEST_CLIENT_DATA`.

## Migration source inventory

Update `docs/source-inventory.json` when porting a legacy subsystem. Map the C#
source to the Go implementation and its tests, and describe implemented behavior
and remaining work in the notes. Preserve the recorded source revision and
hashes; verify the files used against those hashes after CRLF normalization.
Keep the inventory and the current scope in `docs/PORTING.md` consistent.

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
The external-media regeneration/import procedure accepts
`regenerate.py --client <installation-root>` and needs no private-server checkout. Static
server-only tables already have tracked JSON exports; complete offline game-data
regeneration remains `export.py --client <client-data> --server <private-server>`.
Keep original source assets, indexes and decryption inputs (including `aLogin.exe`)
in their original projects. Exclude database-path overrides from asset export/import.
Treat every asset and manifest in `data/` as a derived output. Never require
existing `data/` assets or manifests to regenerate those outputs. Temporary
intermediates may be created during an export and must be cleaned up afterward.
Do not retain decoded JMA files in `data/`. Preserve complete decoded bytes as
zlib-compressed Base64 in `sprites.json` for palette indices and verification.
PNG regeneration must start from fresh sibling-source decryption.

Follow [ASSET_IMPORT.md](ASSET_IMPORT.md) to restore missing ignored payloads.
Use `python3 tools/data_export/regenerate.py --output var/asset-import` for a fresh
source-only export of audio, sprites, pictures and remaining client media.
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
native thresholds are verified. Protection does not expand an attack's targets.


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

The asset importer indexes definitions under `skill_effects.json/records`; the
`asset_skill_effects` SQL view exposes IDs and names. Indexed SQL rows are
authoritative for definitions and skill references. The server reads only SQL.
Every SQL skill must explicitly provide `effects` or `effect_refs`, including an
empty list when it has no effects. Missing metadata fails startup with a rebuild
instruction; there is no native-code inference or embedded policy fallback in
the runtime loader.

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
For an existing installation, edit the derived `data/skill_effects.json` definitions
(or indexed SQL rows) and rebuild/reload as appropriate; those definitions are
runtime-authoritative. Source regeneration uses the authored conversion policy
and replaces derived edits, so transfer permanent tuning to the policy first.
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

Additional channels and feedback use the verified AC2:16 native head-banner
format with explicit channel/name text. Dedicated native channel receive layouts,
guild membership and client channel controls require separate migration work.
Keep their framing documented rather than inventing unverified wire codes.


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

Potential training remains unavailable until item consumption, combat behavior
and native display are verified. The reference's free increment and stat-37
level-minus-one display do not establish these semantics. Do not add a second
point budget or invent pill IDs/costs to fill this gap.


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

Register each inbound opcode once in `internal/server/command_registry.go`, with
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
Persist native text bytes losslessly; deliveries have a one-byte string length,
so reject content above 255 bytes. Line breaks and tabs are allowed. Preserve the
ignored native type byte without assigning parcel or attachment semantics.
Use UTC OLE Automation timestamps in delivery packets.

Deliver pending messages only after the recipient's map snapshot is complete.
Hold `worldMu` to serialize delivery with logout and map transitions. Mark each
message delivered after its successful socket write; failed writes leave it queued.
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

Keep bank and carried gold separate in `game.Character`. Use
`DepositGoldToBank` and `WithdrawGoldFromBank` for currency movement: reject zero,
insufficient funds, native uint32 bank overflow and transfers exceeding `MaxGold`.
Reject before changing either balance so their combined total stays constant.
The existing character transaction persists both balances; do not introduce a
second bank table or a separate save. Older JSON states default to zero bank gold.

Publish AC26:4 wallet refresh and native AC45 balances only after commit. Receipt
write failures cannot undo a durable transfer. Keep bank operations inside the
shared world/trade gates and scripted interaction ownership. PIN and character
transfer reference handlers contain no working behavior; retain explicit
unavailable responses until verified semantics exist. Do not infer an ATM NPC
service operand from unused numeric slots or route deposits through AC29's unsafe
legacy item-storage code. Keep actual-client activation and packet acceptance
separate from synthetic protocol verification.


## Native mall checkout

AC34:1 reads current account points and returns AC75:3/9 followed by AC35:4 to
resume the client's pending cart. Build all replies from one SQL snapshot, and
use the shared mall wire-range conversion. Preserve both accepted mode bytes and
the source's ordinary-point resume until their separate semantics are verified.
Use numeric fallback names for unresolved modes.

This handshake must not refresh catalogs, mall settings or status, which can clear
the selected cart. It spends no points and grants no inventory. Keep checkout in
the shared world gates; trade may allow this read-only query while the eventual
purchase retains its mutation gate. Balance read failures cannot publish success
or overwrite the session cache. Preserve independent raw resume bytes and tests
that submit the existing cart after the query.


## SQL gacha pools

Author pack membership and ordered reward rows in the assets database rather
than extending Go ID lists. Each pool has one to 41 rewards, positive integer
weights totaling 10,000 and quantities one to 255. Native pack IDs provide only
fallback recognition of unavailable legacy packs. Lucky Pack 34333 remains
withdrawn. The strict offline validator requires every item definition.

Runtime loading validates structural correctness across the whole table first;
malformed configuration fails startup. Missing item definitions disable the
entire affected pool and produce an explicit diagnostic. Keep other complete
pools available. Never drop unavailable rewards or redistribute their weights.
Remember disabled configured pack IDs for item use and mall filtering.

Indexed `asset_records` rows for `gacha_packs.json/value` are authoritative.
`asset_gacha_packs` and `asset_gacha_rewards` expose ordered query projections;
edit indexed record JSON and restart to load a new snapshot. The views include
unavailable pools for inspection. Rebuild the database to create new views.
Preserve atomic pack removal and reward grant before publishing native receipts;
use fresh reward metadata and cryptographic uniform rolls. Changes to source
pools belong in sibling inputs; exported data remains derived.


## Daily Lucky Draw

Use `game.LuckyDrawState` for three draws per character per UTC calendar day.
Keep usage in durable character JSON. Reset eligibility follows its saved date;
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
Defaults give all 14 outcomes weight 1. Missing definitions, invalid quantities,
missing/out-of-order slots, more than 20 rewards and overflow fail startup; an explicit empty pool disables draws.

Indexed `asset_records` rows under `lucky_draw.json/value` are authoritative.
Use `asset_lucky_draw_rewards` for inspection; edit indexed JSON and restart to
reload. Durable tuning belongs in `internal/assets/lucky_draw_rules.json`, the
offline compatibility policy. Regenerate with `python3 tools/data_export/lucky_draw.py
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

Gameplay mutations must still commit before success packets. The one-second
server autosave worker is an additional checkpoint for pending session changes,
with a final checkpoint on disconnect. It uses a cloned baseline and the Store's
transactional `AutosaveCharacter` comparison. Never replace this with an
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
Preserve inventory and equipment unless a verified exchange explicitly requires
changes. Never clear equipment to imitate a reference handler with missing or
misclassified item IDs. Unsupported model conversions remain rejected before
ordinary branch mutations. See the Breillat regression suite for the ten-talk
unlock, decline, stale state and failed delivery cases.

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

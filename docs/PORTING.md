# Port status and migration queue

## Scope and completion

The goal is a Go replacement for Wonderland-Private-Server with web administration.
**The port is incomplete.** Working login, gameplay paths and administration do
not establish complete native-client parity. The separate Go/Ebitengine client
has its own [status and commands](CLIENT.md) and [roadmap](CLIENT_ROADMAP.md).

The reference is sibling `Develop` at
`bc4a140d5d145e5bad4ec08581aa47f9ce6c066e`. Independent source review on
2026-10-04 covered all **566 tracked files**, including **331 C# files**. Its
checkout had no untracked or ignored files. Review coverage is not a percentage
of the port completed.

[Source inventory](source-inventory.json) retains every file's finding, normalized
source hash and Go evidence paths. `files` holds C# records; `supporting_files`
holds project metadata, scripts, data, binaries and documentation. Evidence links
identify implementations reviewed, not tests executed. For C# files the review
found 43 migrated, 110 replaced, 74 partial, 13 missing, 53 legacy stubs, seven
legacy incomplete, 23 retired and eight metadata records. These categories are
a dated baseline; update per-file findings as implementation advances.

C# bodies and dispatch/callers were checked independently of old documentation.
Supporting artifacts received format/structure/consumer review. The large native
client decompilation was inspected by imports and function inventory, not every
native function; binaries were not fully disassembled and every authored data
value was not reverified. This was static review, not runtime or aLogin acceptance.
Historical verification reports are not evidence that current tests passed.

## Current implemented scope and limits

| Area | Current implementation | Remaining reference differences |
| --- | --- | --- |
| Transport and login | Framing/XOR, bounded readers, native login/roster/creation/deletion, duplicate-login protection, configurable phase idle timers, status service | Native appearance lacks character job/nickname/potential/title; some generic synchronization replies differ |
| Launcher status | C9 replies, configured server IDs, load colors, persisted name/MOTD/EXP/drop rates | Cluster is fixed; offline mode absent. Source GoldRate/MaxPlayers are stored but not enforced |
| Persistence | GORM/SQLite stores, normalized owned state, atomic reward/exchange paths, optimistic edits, buffered walking and dirty periodic/disconnect checkpoints | SQLite-only provider; optional import rejects unsupported state; checkpoints still serialize under the global world lock and load/soak acceptance is pending |
| Asset pipeline | WLRI native decoding offline, typed SQL runtime catalogs and validated administration; no runtime native/JSON fallback | Full editor coverage and sound/portrait/token inspection differ. Compound2 projection omits manufacturing fields |
| World entry and maps | Native snapshots, scene-ready gate, peer visibility, movement, portals, map-local packets, terrain validation, actor simulation and respawns | Inbound self-spawn/waypoint/emote and several sync requests absent; exact entry HUD sequence and tent pose replay differ |
| Events and quests | Native EVE branches, callbacks, choices, marks, journal, atomic rewards, story continuations, minigames, companion identity and visibility | Custom game_quests definition loading/editor/kill counters absent; dormant progression is additional integration. Scripted shared prop timers and generic NPC greeting fallback differ |
| Combat | PvE/PvP formations, action ownership, turns/timeouts, capture, damage/elements/criticals, effects, shields, combos, loot/progression and authored continuations | Action ACK broadcast and owner/party entry refresh differ; PvP rewards differ; native animation/area acceptance remains pending |
| Companions and vehicles | Party/hotel/reserve rosters, vouchers, skills, gear, feeding/amity, pet rebirth, mounts, vehicle ownership/wear and travel | Pet potential absent; clinic omits accompanying-pet healing. Character reborn jobs, bonuses and GM workflow absent |
| Items and tents | Bag/equipment/storage, metadata-preserving moves/grants, recovery/repair, transient reservations, isolated homes, furniture placement/recovery and saved return points | Type restrictions and remaining special items need verification; one floor/wallpaper pair, floor-zero placement; native AC64 manufacturing incomplete |
| Economy | Atomic mall/forging/gacha/Lucky Draw, bank balances, trades, stalls, synthesis, two-input manufacture, gathering and parcel escrow | AC21/226 mall compatibility, AC37/59 requests, native formula/tool/timer manufacturing, recipe fee/chance preservation and some legacy aliases absent |
| Social | Chat/preferences, friendships, parties, guild membership/ranks/insignia/rosters, marriage, mail and SQL administration | Job/nickname presentation absent; Cupid model is absent but reference excludes it from transmitted lists; several request aliases/default replies differ |
| Administration | Accounts/security, sessions/bans, mall balances, character editors, GM studio, content editors, logs/audit, guilds/marriages/mail, operations/config editing | Offline status, broadcast channel/color, live tab refresh, mass starter delivery, custom quest editor and MySQL workflows absent; NPC controls affect selected session |
| Compatibility | Read-only offline SQLite import with preserving output, explicit unsupported-state failures and validation | Jobs/nickname/potential/socket/bomb/sewing data, guild/mail mappings, forum hashes and MySQL conversion remain separate work |

See [world simulation](WORLD_SIMULATION.md), [items/player state](ITEMS_PLAYER_STATE.md),
[economy/social](ECONOMY_SOCIAL.md), [combat targeting](COMBAT_TARGETING.md),
[administration](ADMINISTRATION.md), [legacy import](LEGACY_IMPORT.md) and
[configuration](CONFIGURATION.md) for behavior, commands and remaining limits.

## Remaining work in dependency order

Port reachable reference implementations first. Keep empty handlers and unfinished
source systems on hold. A request being registered or an enum being copied does
not prove its dependent behavior works.

1. **Missing native commands:** AC4 self-spawn; AC7 waypoint update/ACK; AC18
   emote; AC183:17 and AC186:9 synchronization; AC13:238 mall refresh; AC21
   mall window/buy/balance; AC22 generic acknowledgement; AC226:255 matrix/claim
   state; AC44 title; AC66 reborn job; AC87 bath recovery; AC90 fishing.
   Complete AC32:3 interaction cleanup even when the pose is already zero.
2. **Manufacturing and character state:** native AC64 start/continue/stop,
   formula IDs, up to five materials, plans/tools, duration and bag/tent output;
   direct AC37 and AC59 paths; authored recipe chance/fee handling. Add character
   job/nickname/potential/title and pet potential metadata without inventing
   finished training rules. Port the source GM reborn class/reset/cape/aura flow
   and class stat modifiers. Preserve two-floor tent metadata and pose replay.
3. **World and combat:** scripted shared prop changes and 60-second reset,
   generic NPC greeting fallback, clinic offer/healing for pets, level-one
   starter redelivery and administration mass starter grant. Compare explicit
   battle entry owner/party refreshes and broadcast action ACKs. Decide whether
   source PvP attacker-victory rewards (150 EXP/100 gold) should be retained;
   Go currently awards neither. Check the Laura exit gift/mark against native
   authored content before adding a duplicate fallback.
4. **Administration and reachable quest data:** offline launcher state,
   announcement channel/color, map-wide versus selected-session actor controls,
   linked quest changes, live character refresh, combined spawn/opcode reports
   and custom game_quests definition editing/loading/kill counters. Preserve
   optimistic SQL edits and permission checks while extending coverage.
5. **Focused migration verification:** compare allocation packet widths/batches,
   generic/default acknowledgements, bank defaults, guild journal requests,
   inventory restrictions, pet outcomes, source recipe order and applicable
   story/protocol scenarios. Test restart, rollback, duplicate requests, ownership
   and full containers. Inspect actual callers before treating a helper as a
   missing live feature. Native captures and multiplayer acceptance are separate
   from synthetic unit coverage.

## On hold: unfinished source and further development

These do not block porting reachable implemented reference behavior:

- **Custom quest progression:** accept/advance/complete/reset and NPC matching
  helpers have no external runtime callers in the reference. Definitions/editor
  and PvE kill-counter updates are reachable; native EVE execution is separate.
  Wiring the dormant engine into gameplay is new integration work.
- **Combat research:** verified native area/grade patterns, hit/resistance rules
  beyond compatibility formulas and multi-target/status animation acceptance.
  The source resolves one target; explicit SQL area execution is a Go addition.
- **Incomplete companions/items:** funded potential-item training, unfinished
  evolution and unresolved special items. Source AC68 increments potential for
  free without consuming its advertised pill; do not expose that as complete
  training. AC70 reset is success-only. Preserve useful state separately.
- **Economy placeholders:** arcade ticket/prize exchange, bank PIN/transfer/ATM,
  guild vault/alliance operations, nonempty market browser/history and unresolved
  native parcel requests. Source AC71 fixed score is a stub; Go paid arcade
  payments/rewards are an additional implementation with inferred rules.
- **Housing:** source AC60 decoration purchase, AC61 upstairs and AC62:4 special
  placement are empty/success-only. Garage/team/pet/RiceBall blocks are commented
  or incomplete. Verified upgrade/purchase rules require further work.
- **Compatibility:** optional MySQL provider and real source cross-backend
  migration, forum password conversion and unsupported import metadata. These
  remain explicit limitations of the SQLite replacement.
- **Acceptance/operations:** aLogin captures for world entry, movement, NPCs,
  vehicles, items/tents and social/economy; load/soak, crash recovery and deployment.

## Deliberate replacements and retired behavior

Preserve requested generic skill effects, typed protections, full-party speed
chains, rebirth combo-level adjustment, all-stat weighted pet allocation and
compiled elemental/vital growth. These are intentional changes to reference
rules, not gaps to undo. SQL-derived catalogs replace runtime JSON/native reads.
Passwords, ownership, transactions and metadata checks replace unsafe source
paths; broken partial grants and unchecked stale listings are not requirements.

Do not restore unauthenticated username-only HTTP mall purchases, local client
file patching, hardcoded staff ownership, destructive startup schema repairs,
Windows GUI launch/layout code or obsolete deployment scripts. Source synthetic
GM-bot global broadcasts have commented callers; Cupid is not transmitted in
friend lists. Stored GoldRate/MaxPlayers do not implement source gold scaling or
player caps. Original database/spawn overlays remain excluded from asset inputs.

Historical tests sometimes expect superseded bugs, old reward pools or dormant
MySQL/custom-quest code. Reproduce only scenarios that match reachable source
behavior or an explicitly retained compatibility requirement. Keep per-file
findings current in the source inventory when those decisions change.

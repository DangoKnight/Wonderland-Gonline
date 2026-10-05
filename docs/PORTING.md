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
holds project metadata, scripts, data, binaries and documentation. Evidence links identify implementations reviewed, not tests
executed. Current C# findings
are 65 migrated, 116 replaced, 57 partial, 2 missing,
53 legacy stubs, 7 legacy incomplete, 23 retired and
8 metadata records. Update findings as implementations advance; these
categories are not a completion percentage.

C# bodies and dispatch/callers were checked independently of old documentation.
Supporting artifacts received format/structure/consumer review. The large native
client decompilation was inspected by imports and function inventory, not every
native function; binaries were not fully disassembled and every authored data
value was not reverified. This was static review, not runtime or aLogin acceptance.
Historical verification reports are not evidence that current tests passed.

## Upstream follow-up — 2026-10-04

Reviewed both new `Develop` commits, `63d8c421` and `e63e9fea`, through
`e63e9feabc3b3bf596fe8a4f61d5d3e488c34ba1`: all 19 added or changed files.
The source inventory's `upstream_updates` ledger records their separate hashes
and dispositions; the original 566-file review and baseline hashes remain intact.

- Robinson's Event 19 now continues from branch 3 into recruitment on both
  beach maps (10035 and 10039), completing the raft → dialogue → companion chain.
- Clicking Robinson can recover interrupted recruitment while mark 12047 is
  active and the companion is absent. Existing candidate ordering, disabled-event
  checks, companion capacity and transactional reward checks remain in effect.
- The launcher item-file fix is already covered by offline native decoding and
  SQL catalog loading. Upstream starter equipment matches Go's existing creation
  behavior; this project retains creation-only grants without login refill.
- The changed legacy database is a development snapshot. Starter rows and chest
  reward pools are unchanged; generated pool IDs and player/settings state differ.
  It is excluded from asset import. Windows tooling remains replaced by Go tests
  and web administration.

Focused server race tests passed using unmodified native EVE scripts from a
schema-v11 SQL copy. They cover both maps, fresh and interrupted progression,
companion aliases, exactly-once rewards, SQL durability, full parties and failed
saves with successful retries. Adjacent story and native character-creation
regressions also passed. Interactive aLogin acceptance remains pending.
Remote refs were fetched; the legacy working checkout and installed databases
were unchanged.

## Current implemented scope and limits

| Area                    | Current implementation                                                                                                                                                 | Remaining reference differences                                                                                                                                                                                            |
| ----------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Transport and login     | Framing/XOR, bounded readers, native login/roster/creation/deletion, duplicate-login protection, configurable phase idle timers, status service                        | Job/nickname/potential now replay; native HUD acceptance remains pending; title uses separate native replay                                                                                                                |
| Launcher status         | C9 replies, configured server IDs, load colors, persisted name/MOTD/EXP/drop rates                                                                                     | Cluster is fixed; explicit offline advertising is implemented. Source GoldRate/MaxPlayers are stored but not enforced                                                                                                      |
| Persistence             | GORM/SQLite stores, normalized owned state, atomic reward/exchange paths, optimistic edits, buffered walking and dirty periodic/disconnect checkpoints                 | SQLite-only provider; optional import rejects unsupported state; checkpoints still serialize under the global world lock and load/soak acceptance is pending                                                               |
| Asset pipeline          | WLRI native decoding offline, typed SQL runtime catalogs and validated administration; no runtime native/JSON fallback                                                 | Full editor coverage and sound/portrait/token inspection differ. Compound2 manufacturing and authored chances/fees now have typed projections                                                                              |
| World entry and maps    | Native snapshots, scene-ready gate, peer visibility, movement, portals, map-local packets, terrain validation, actor simulation and respawns                           | Native request handlers implemented; exact entry HUD sequence, inferred waypoint map hint and tent pose replay need acceptance                                                                                             |
| Events and quests       | Native EVE branches, callbacks, choices, marks, journal, atomic rewards, story continuations, minigames, companion identity and visibility                             | Typed custom quest definitions/editor and transactional kill counters implemented; dormant progression is additional integration. Shared prop reset/replay and generic NPC greeting implemented; native acceptance pending |
| Combat                  | PvE/PvP formations, action ownership, turns/timeouts, capture, damage/elements/criticals, effects, shields, combos, loot/progression and authored continuations        | ACK broadcast and owner/party entry refresh implemented; PvP rewards deliberately omitted; native animation/area acceptance remains pending                                                                                |
| Companions and vehicles | Party/hotel/reserve rosters, vouchers, skills, gear, feeding/amity, pet rebirth, mounts, vehicle ownership/wear and travel                                             | Character/pet potential metadata and GM rebirth/classes implemented; clinic heals accompanying pets; funded normal/golden/Super pill training implemented; mercenary eligibility and native acceptance pending             |
| Items and tents         | Bag/equipment/storage, metadata-preserving moves/grants, recovery/repair, transient reservations, isolated homes, furniture placement/recovery and saved return points | Reachable type restrictions are verified; unverified special effects remain on hold; two-floor decoration metadata persisted; placement remains floor zero; upstairs access pending                                        |
| Economy                 | Atomic mall/forging/gacha/Lucky Draw, bank balances, trades, stalls, synthesis, two-input manufacture, gathering and parcel escrow                                     | AC59 and timed AC64 implemented; AC37 socket effects remain unfinished in the reference; some legacy aliases absent                                                                                                        |
| Social                  | Chat/preferences, friendships, parties, guild membership/ranks/insignia/rosters, marriage, mail and SQL administration                                                 | Job/nickname presentation implemented; Cupid model is absent but reference excludes it from transmitted lists; success-only guild/mail/alliance aliases remain on hold                                                     |
| Administration          | Accounts/security, sessions/bans, mall balances, character editors, GM studio, content editors, logs/audit, guilds/marriages/mail, operations/config editing           | Offline status, native announcement channels, live tab refresh, scoped actors, linked quest edits, combined reports and SQL quest registry implemented; MySQL workflows replaced by SQLite                                 |
| Compatibility           | Read-only offline SQLite import with preserving output, explicit unsupported-state failures and validation                                                             | Jobs/nickname/potential/socket/bomb/sewing data, guild/mail mappings, forum hashes and MySQL conversion remain separate work                                                                                               |

See [world simulation](WORLD_SIMULATION.md), [items/player state](ITEMS_PLAYER_STATE.md),
[economy/social](ECONOMY_SOCIAL.md), [world/combat parity](WORLD_COMBAT_PARITY.md), [manufacturing/character state](MANUFACTURING_CHARACTER_STATE.md), [combat targeting](COMBAT_TARGETING.md),
[administration](ADMINISTRATION.md), [legacy import](LEGACY_IMPORT.md) and
[configuration](CONFIGURATION.md) for behavior, commands and remaining limits.

## Remaining work in dependency order

Port reachable reference implementations first. Keep empty handlers and unfinished
source systems on hold. A request being registered or an enum being copied does
not prove its dependent behavior works.

No further items are scheduled in this dependency queue. The focused verification
below is complete; unfinished source systems and native acceptance remain on hold.

## Focused migration verification — 2026-10-04

Compared current handlers and their callers with the recorded reference revision;
25 reviewed source hashes match after CRLF normalization. Ran **175 unique
top-level tests** with the race detector across game, assets, assetsql, protocol,
world, server and store. A stale AC37/AC64 dispatch test matrix was corrected and
its affected checks rerun successfully. This was a focused battery, not the full suite.

| Area                    | Verified outcome                                                                                                                                                                                                                                                                                                                                                                    |
| ----------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Allocation              | Direct/header/target/batch formats preserve greedy 4/2/1-byte amounts, missing/zero defaults and partial-batch behavior. Invalid/overflowing entries retain points; pet overflow now matches the reference guard. Character overflow is deliberately rejected as well. HP/SP remain unchanged apart from clamping to maxima; skill unlocks and allocations commit before snapshots. |
| Generic replies         | All handshake/entity subcommands keep their owning layouts. Unknown bank subcommands now return the private AC45:8 balance snapshot without mutation. No global ACK fallback is invented.                                                                                                                                                                                           |
| Native scene readiness  | Source AC3 short ACK/map broadcast conflicts with native AC3 character/map snapshots (receive address 0x2dfdb3); it remains unsupported. AC12/89/92 synchronization, replay and loading gates are tested independently.                                                                                                                                                             |
| Guild journal           | AC39:1 works without guild membership; repeated requests clear old AC24:4 entries, replace active marks and send completion flags without leaking to peers.                                                                                                                                                                                                                         |
| Inventory rules         | All 256 stack/drop type values match source. Wear guards, metadata, locks, whole grants and full-container rollback pass. Source Tradeable and use-type metadata have no reachable restriction callers; no new trade/body/level restriction is imposed.                                                                                                                             |
| Pet outcomes            | Feeding, alias/capacity/initialization checks for vouchers, sparse client slots, rebirth eligibility/reset/bonus and desertion below 20 pass. Failed saves do not consume vouchers or change published rosters. An unrelated mount survives desertion.                                                                                                                              |
| Recipe order            | First symmetric alchemy match preserves authored → Compound2 → Compound order through typed SQL reload, without provenance documents. Two-input manufacturing retains first workbench match and atomic costs/output.                                                                                                                                                                |
| Persistence and stories | Restart, rollback, duplicate requests, ownership, receipt failures and full containers are covered; native ship/storm/Monkey/Xaolan/Niss/Fred/Elin scenarios run against imported SQL assets. Buffered walking survives committed resource operations.                                                                                                                              |

The SQL census covered 1,119 maps, 8,181 NPCs, 209 ground items, 2,790 resolvable
warps and 786 manufacturing formulas. Of 7,313 NPCs with events, fresh-character
branch selection found 4,872 runnable branches, 30 with unsupported operations,
and 2,411 with no matching branch for that state. Unsupported here means the
ordinary opcode executor refuses the shape; complete water-gathering branches
have a separate validated controller. The reference also rejects unresolved
reward kinds. This census is selection/decoding coverage, not execution of every
story or proof that every state has been ported. Unresolved shapes remain pending.

Regression entry points are `migration_verification_test.go` in game, server and
assetsql, alongside the affected existing suites. To repeat the new checks:

```sh
WONDERLAND_TEST_ASSETS_DB=var/assets.db go test -race \
  ./internal/game ./internal/assetsql ./internal/server \
  -run 'Test(Migration.*|CommandRegistry.*|AllocationUnlock.*|Bank.*|PetAllocation.*)' -count=1
```

Use migrated asset schema v10 with gameplay schema v15. Verification used
disposable fixtures and asset database copies; installed databases were unchanged.
Interactive aLogin captures, multiplayer acceptance and load/soak/crash testing
remain separate work below.

## On hold: unfinished source and further development

These do not block porting reachable implemented reference behavior:

- **Native command acceptance and fishing parity:** synchronization, appearance,
  waypoint, gesture, mall aliases, title, reborn-job metadata, bath and pose-stop
  cleanup are implemented. Verify with aLogin captures, especially the inferred
  AC7 map hint. Fishing now has native AC23/AC90 paths and transactional weighted catches.
  Fishing Skilled 02, Unskilled, Full Bag, Advancement and Carnie Return 02
  have tester-reported live passes. Native movement and poses pass; AC7 remains
  unobserved. The tester confirms friend requests, Social Setup/profile settings
  and emotes now work correctly, correcting the earlier failure report.
  The AC6:2 stopped-position handler still needs live verification.
  Exact original probabilities and remaining live-client checks remain research; see
  [fishing](FISHING.md) and
  [native commands](NATIVE_COMMANDS.md). The [live-test framework](LIVE_TESTING.md)
  prepares isolated characters, credentials and manual acceptance checklists;
  preparation does not establish live acceptance.

- **Custom quest progression:** accept/advance/complete/reset and NPC matching
  helpers have no external runtime callers in the reference. Definitions/editor
  and transactional PvE kill-counter updates are implemented; native EVE execution
  remains authoritative. Mark metadata seeds use actual native IDs and text, without
  guessed rewards, map/NPC mappings or objectives from English titles.
  Wiring the dormant engine into gameplay is new integration work.
- **Laura exit fallback:** source gift IDs identify missing/food WLRI items and
  no matching reward was found in the inspected map 10000/10001 operations.
  Keep the fallback pending verified tent/remote/cutscene/mark data; see
  [world/combat parity](WORLD_COMBAT_PARITY.md).
- **Combat research:** verified native area/grade patterns, hit/resistance rules
  beyond compatibility formulas and multi-target/status animation acceptance.
  The source resolves one target; explicit SQL area execution is a Go addition.
- **Incomplete companions/items:** unfinished evolution, unresolved special
  items and native potential-item acceptance. AC23:126 now consumes normal,
  golden and Super Potential Pills transactionally using the official WLRI
  level-dependent rates and native attribute bonus table. Native mercenary
  eligibility remains pending. Source AC68 increments potential for free without
  consuming its advertised pill; its free increment remains disabled. AC70 reset
  is success-only. Preserve useful state separately.
- **Economy placeholders:** arcade ticket/prize exchange, bank PIN/transfer/ATM,
  guild vault/alliance operations, nonempty market browser/history and unresolved
  native parcel requests. Source AC71 fixed score is a stub; Go paid arcade
  payments/rewards are an additional implementation with inferred rules.
- **Manufacturing and rebirth acceptance:** timer units and native HUD class
  contributions require aLogin verification. AC37 has no implemented socket
  effect in the reference and its gem IDs identify WLRI oil; reject without
  consuming items. Reborn class skills are absent from the source; default cape
  variants are authored WLRI mappings. See [details](MANUFACTURING_CHARACTER_STATE.md).
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

## Multi-slot inventory follow-up

Inventory acquisitions now use native item footprint dimensions from asset SQL
schema v11. Rafts require a contiguous 4 × 3 rectangle; stack space alone is
insufficient. Covered cells, row/bottom boundaries, movement, equipment swaps,
storage transfers, crafting, forging and admin edits are checked without
persisting duplicate child items. Focused regressions verify source/claim rollback,
SQL size initialization and anchor persistence. Live aLogin multi-cell acceptance
remains pending. See [Items and Player State](ITEMS_PLAYER_STATE.md).

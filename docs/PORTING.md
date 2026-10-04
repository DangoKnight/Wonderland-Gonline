# Port status

The client runs from the decompiled data in repository `data/` (JSON exports, PNG pictures and sprite atlases, media and audio); `cmd/asset-build` verifies it and `-copy` packages it for distribution. The original client's files are needed only for native inspection modes, which take an explicit `-client` directory; `cmd/client-assets` can stage them into `var/client-original-assets` with tracked SHA-256 manifests and upload ZIPs. See [CLIENT.md](CLIENT.md) for the layout and verification. This does not imply animation decoding or playable-client completion.

## Completion criterion

The requested outcome is a full Go replacement for Wonderland-Private-Server, including a web replacement for the Windows administration interface. **That outcome has not been reached.** A successful Go build or working dashboard does not establish gameplay parity.

The separately requested Go/Ebitengine client includes server login, character selection/creation and world rendering alongside the native artwork workbench. Full gameplay parity and native-client acceptance remain pending. See [client status and commands](CLIENT.md).

Reference: sibling `Develop` at `bc4a140`, 331 C# files and 71,144 lines. `source-inventory.json` records the full commit and hashes, including UI and support libraries. Counts are an inventory, not a percentage-complete metric.

## Implemented and checked

| Area                         | Go implementation                                                                        | Scope / evidence                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| ---------------------------- | ---------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Wire format                  | `internal/protocol`                                                                      | `0x44F4`, little-endian length, full-frame XOR `0xAD`, fragmented/coalesced streams, malformed input, golden response                                                                                                                                                                                                                                                                                                                                                                   |
| Launcher status              | `internal/server`                                                                        | Raw `C9 00` response with server IDs 1 and 101; threshold tests                                                                                                                                                                                                                                                                                                                                                                                                                         |
| Runtime                      | `internal/server`, `cmd/wonderland`                                                      | TCP listeners, bounded connections, per-connection command serialization, write deadlines, idle timeout, packet rate cap, signal shutdown                                                                                                                                                                                                                                                                                                                                               |
| Login                        | `internal/server`                                                                        | AC0 discovery, AC1 handshake, AC63:4 version prefix and login; AC63:1 native roster fields, bare AC63:0 return and same-connection reauthentication, duplicate login rejection, empty character selection                                                                                                                                                                                                                                                                               |
| Accounts                     | `internal/store`                                                                         | Separate SQLite DB, password hashing, banned account rejection, concurrent registration, restart persistence                                                                                                                                                                                                                                                                                                                                                                            |
| Character creation           | `internal/game`, `internal/server`                                                       | AC9 name reservation and appearance, all 15 character presets and four elements, initial skills/outfits/items, five-point creation budget excluding model bonuses, atomic creation and hashed secondary deletion code                                                                                                                                                                                                                                                                                                                       |
| Initial world state          | `internal/game`, `internal/server`                                                       | AC3/5:3/8:1/23:5/23:11/12/7 packets; AC12:1 releases the map gate; movement persists and broadcasts within the map.                                                                                                                                                                                                                                                                                                                                                                     |
| Map scene                    | `internal/world`                                                                         | SendMapInfo order: AC23:138, AC22:4 actors (14-byte records, idle/opened prop frame, concealment), AC23:4 native ground items, entrant AC23:122/10:3/23:76, AC23:102, AC20:8. Golden bytes and a 1,119-map native census.                                                                                                                                                                                                                                                               |
| NPC visibility               | `internal/world/preevent.go`                                                             | EVE flag bit 0, cumulative PreEvent rules with ANDed branches, the Sealed Bead rule filter, and the quest-mark story overrides of `ShouldNpcBeVisible`. Conditions 0/1/2/5/6/10/14/15 evaluate against Go state; owned-companion and active-vehicle operands use persisted state; the four water timer conditions use durable UTC expiries.                                                                                                                                                                          |
| Portal warps                 | `internal/world/portal.go`, `internal/server/world.go`                                   | AC20:8, ignored until the character has moved since it landed (the client reports the area it lands in); LookupPortal priorities (reverse geometry, click ID, index, Gray code, single exit, test-map recovery) and the Carnie fallback; persisted destination; old-map AC12 departure; entrant Warp_In packets; peers exchanged after AC12:1 without the login sprite refresh.                                                                                                                                                       |
| Map peers                    | `internal/server/world.go`                                                               | Acknowledged players exchange AC4/5:0/10:3/5:8 appearance and AC7 position; AC6 movement stays within the map; AC12 logout despawns once. Concurrent entry and failed-recipient tests pass.                                                                                                                                                                                                                                                                                             |
| Character deletion           | `internal/store`, `internal/server`                                                      | AC35:2 ownership, secondary-code verification, last-character code removal; schema v1→v2 preserves accounts                                                                                                                                                                                                                                                                                                                                                                             |
| Character storage            | `internal/store`, `internal/game`                                                        | Account-owned character records, atomic mutation callback, rollback, native selection serialization                                                                                                                                                                                                                                                                                                                                                                                     |
| Items and player state | `internal/server/tents.go`, `items.go`; `internal/game/item_locks.go`; `internal/store/tents.go` | Source-supported special-item dispatch, targeted gacha use, owner-isolated native tents, furniture placement/movement/recovery, durable access locks and per-visitor return points. Transient item reservations protect inventory, equipment, trades and SQL reward transactions. See [ITEMS_PLAYER_STATE.md](ITEMS_PLAYER_STATE.md). Native-client acceptance remains pending. |
| Inventory library            | `internal/game`                                                                          | 50 slots, all-or-nothing grants with additive AC23:5 deltas, transfers, metadata-compatible stacks, partial moves (MoveItem), droppable item types, 31-byte serialization                                                                                                                                                                                                                                                                                                               |
| Bag and ground actions       | `internal/server/items.go`, `internal/world/ground.go`                                   | AC23:2 pickup (dropped items first, then native slot, then click ID; 180 px reach; full-bag banner), AC23:3 drop (one ground slot per unit avoiding native slots and click IDs; destroy prompt for non-droppable types), AC23:10 move, native respawn on a one-second tick. The bag commits before success packets. Dropped items live in memory, as in C#.                                                                                                                             |
| Quest library                | `internal/game`                                                                          | Monotonic progress, completion timestamps, atomic item reward and replay prevention; interpreter pending                                                                                                                                                                                                                                                                                                                                                                                |
| Critical hits                | `internal/game`                                                                          | Legacy config bounds, equipment chance cap, attack-only criticals and int32 saturation; combat integration pending                                                                                                                                                                                                                                                                                                                                                                      |
| Equipment                    | `internal/game/equipment.go`, `internal/server/items.go`                                 | AC23:11 wear (swap into the same bag slot) and AC23:12 remove (empty destination only), then Send8_1 stats and the equipment change banner. Worn items keep damage and forge; schema v2 ID arrays still load. Forge-aware bonuses, UInt16 wrap of combat totals and banker's rounding follow EquipManager.                                                                                                                                                                              |
| Chat and poses               | `internal/server/chat.go`                                                                | AC2:2 map chat to peers (not echoed), AC2:1 world chat to every other player, AC2:3 whispers by target ID (echoed to the sender), AC2:5 team chat to the party, AC32:1/2/3 changed-pose broadcast and replay to later arrivals.                                                                                                                                                                                                                                                                                                                                                                |
| GM commands                  | `internal/server/chat.go`, `internal/store`, `internal/admin`                            | Administrator-granted account GM level (schema v3, audited, applied to online sessions immediately). `:heal`, `:gold`, `:item`, `:warp/:goto/:tp`, `:summon/:bring`, `:kick`, `:b/:broadcast/:notice`, `:town` (all 16 legacy aliases), `:summonall` with a stable recipient snapshot and failure isolation, and durable `:level/:lvl`, `:points/:sp/:statpoint/:statpoints`, `:stat/:stats`, `:exp` and `:skill` progression overrides; `:clearskills/:resetskills` rebuilds starter and current stat-qualified skills with native removal synchronization. All remaining AC02 GM commands and advertised pet progression/bag clearing are implemented; see the configuration command reference. SQL reload, persistent moderation, invisibility, forced PvE victory and graceful countdown shutdown have focused coverage. Native-client acceptance remains pending. |
| Public travel commands       | `internal/server/travel_commands.go`, `internal/server/travel_commands_test.go`             | `:carnie`/`/carnie` preserve the visit return point; `:unride`/`:dismount` and slash aliases dismount item vehicles without consuming the item, with legacy map-10036 shore relocation. Chat cannot bypass loading, battle, trade, event or cutscene ownership. Vehicle companions remain mounted; Carnie memo changes only after durable travel. |
| NPC events                   | `internal/world/event.go`, `internal/server/events.go`                                   | AC20:1 click (reach 200 px, concealment, 1.5 s repeat guard, door portals), linked/direct event selection, FindBranch with trailing AND conditions and choice callbacks, AC20:6/9 session runner: speech, questions, path groups, movies, music, effects, quest marks, gold/EXP/recovery/record point, atomic quest items, chest marks, prop frames, actor show/hide, poses, minimap markers, teleports. Branches with unported actions or disabled data are refused before any change. |
| Quest journal and scene sync | `internal/world/view.go`, `internal/assets/quests.go`                                    | Mark.dat (2,154 marks), AC24:4/6/7 journal on entry, AC24:1/4/5 updates, completed-event replay, PreEvent visibility and prop-frame re-evaluation, authored and question-mark minimap icons.                                                                                                                                                                                                                                                                                            |
| Starter story                | `internal/server/story.go`                                                               | Ship storm movie, warp to the shipwreck beach, the timed rescue sequence (mark 12040) and Robinson's dialogue.                                                                                                                                                                                                                                                                                                                                                                          |
| Combat                       | `internal/battle`, `internal/server/combat.go`                                           | Quest and field battles from EVE actions 4/6 (formations from EVE category 8, Npc.dat stats), AC11:250/11:5 fighter records, AC50 commands with acknowledgment, 30 s turn timeout, speed order with combos, basic and skill damage, elements, criticals, guard and status effects, heals/buffs, explicit Shrink/Hot Fire compatibility effects, monster turns, victory/defeat/flee endings, EXP/levels/stat points, gold, native loot, and the event's battle callback branch.                                                          |
| Levels                       | `internal/game/progress.go`                                                              | Total-EXP level curve (CalcMaxExp), three stat points per level, AC8:1 stat 36, refill on level-up.                                                                                                                                                                                                                                                                                                                                                                                     |
| Stats and items              | `internal/game/progress.go`, `internal/server/items.go`                                  | AC8 stat allocation (all accepted packet layouts, character target), AC23:96/15 recovery items and equipment use, AC23:124 confirmed destruction.                                                                                                                                                                                                                                                                                                                                       |
| Skill progression            | `internal/game/skill_tree.go`, `internal/game/skill_progress.go`, `internal/server`      | Player/pet combat proficiency, 92 element/stat requirements, 78 grade-ten evolution links, creation/login qualification and incremental skill-book updates. Skill and attribute changes persist before success; native-client validation remains pending.                                                                                                                                                                                                                               |
| Teams                        | `internal/server/party.go`                                                               | AC13:1 join request, AC13:3 reply, AC13:4 leave, AC13:9 kick and AC13:10 leadership; AC13:5 formation to the map, AC13:6 roster and AC8:3 teammate vitals; formation replay on map entry; members on the leader's map follow its portals; disconnected members leave; EVE team-size condition.                                                                                                                                                                                          |
| Team battles                 | `internal/server/combat.go`, `internal/battle`                                           | GetTeamMembers: teammates on the battle's map join (four at most, not those fighting or in an event); shared turns; any escape ends the battle for all; every winner earns full EXP and gold, the starter receives loot and continues its event; captures go to their owner; a disconnected member's fighters stay and defend.                                                                                                                                                          |
| NPC services                 | `internal/server/services.go`                                                            | EVE action 7: shops (AC27:2 selling with npc_sale_prices.csv, equipment/props modes, 999,999 gold limit), props/stock keeper storage (AC30:1–5 transfers and snapshots), clinic rest (AC31:2 offer, AC31:1 confirm).                                                                                                                                                                                                                                                                    |
| Regions and doors            | `internal/world/event.go`, `internal/server/events.go`                                   | EVE entry areas (20 px cell rectangles, cells counted from 1 as the client's scene loader builds them): walk-in regions on movement and before portal steps, door scripts on AC20:8 (one-cell margin), AC20:4 region requests, story arrivals on maps 12055/12000/11167/11039 and arrival regions on seven story maps. Movement is ignored while an event holds the player.                                                                                                                                                                            |
| Field encounters             | `internal/server/encounters.go`, `internal/world/monsters.go`                            | IsWildMonster and IsSafeTownMap rules, wild-monster clicks, proximity encounters (72 px, re-armed after leaving every monster), step encounters every 18–34 moves (25 first), 4 s map-entry grace, 2–4 s post-battle cooldown, defeated overworld monsters hidden for everyone for 60 s and respawned on the ground-item tick.                                                                                                                                                          |
| Menu warps                   | `internal/server/services.go`                                                            | AC5:17 starter beach, record point and Carnie (with its remembered exit), refused in battle; AC5:7 sprite refresh.                                                                                                                                                                                                                                                                                                                                                                      |
| Pets and companions          | `internal/game/pets.go`, `internal/server/pets.go`, `internal/battle`                    | Pet model and persistence (party of four, story-companion reserve), per-login client slots, AC15:8 roster/AC15:1 recruit/AC15:9 name/AC8:2 progression, battle pet on the map for peers, AC19 select/rest, EVE companion actions (recruit, dismiss, amity) and pet conditions, recruited companions leaving the map, battle pets with their own commands, skills and criticals, pet EXP with stat growth, amity loss and desertion, captures, pet food and pet recovery items, durable AC69 rebirth ascension with refreshed pet stats, AC68:2 one-point pet allocation, and AC67 battle-pet feeding by food item ID.          |
| Player stalls and social economy | `internal/server/stalls.go`, `guilds.go`, `social_commands.go`; `internal/store` | Atomic native stalls; persisted guild membership/roles/notices/insignia and native guild chat; proposals, ceremony and divorce; parcel escrow/read/claim/delete with native mailbox replies. See [ECONOMY_SOCIAL.md](ECONOMY_SOCIAL.md). |
| Manufacturing and gathering | `internal/server/manufacturing.go`, `synthesis.go`; `internal/assetsql` | SQL-owned recipes, rates, pools and marriage rules; native Forge manufacture, named-workbench public commands, chance-based synthesis and timed gathering with atomic inventory updates. |
| Bank currency                | `internal/game/banking.go`, `internal/server/banking.go`                                  | Persistent AC45:8 balance queries, :9 deposits and :10 withdrawals; atomic bank/wallet saves, uint32 bank overflow and 999,999 carrying-limit checks, authoritative wallet refresh. PIN changes, character transfers and verified native ATM activation remain pending. |
| Friendships and presence     | `internal/store/friends.go`, `internal/server/friends.go`                                | AC14:2/3/4 requests, acceptance, removal and lists; durable symmetric relationships, native AC14:5/11 records and online/offline notifications across maps. Native AC14:1 text mail persists before receipt, delivers across maps and queues offline/loading recipients. AC10 same-map contact add/reply, list/status and removal share the durable friend store. Parcel escrow and mailbox commands are implemented separately (see ECONOMY_SOCIAL.md); native parcel request dispatch remains pending.                                                                                                                                                                                                                                                                               |
| Quest minigames              | `internal/server/minigames.go`, `internal/world/event.go`                                | EVE opcode 9, native AC57 start/result/finish and outcome branch selection; rabbit win/loss rewards, event ownership and cancellation. Arcade ticket/prize exchange remains pending.                                                                                                                                                                                                                                                                                                    |
| Player preferences           | `internal/game/settings.go`, `internal/server/settings.go`                               | Persisted AC16/AC33 PK, team, trade and channel preferences; native settings snapshots and acknowledgments, team request rejection and trade cancellation. Walk mode and team-follow retain native acknowledgment behavior.                                                                                                                                                                                                                                                             |
| Item mall                    | `internal/server/mall.go`, `internal/server/forging.go`, `internal/store/mall.go`          | AC75 points/bonus catalogs, initialization, balances and native carts; AC34:1/AC35:4 checkout balance/resume without clearing the pending cart; AC23:25/26/54 compatibility; account debit and all inventory additions commit atomically. Schema v5 persists bonus points. Audited GM point adjustments and web point/bonus controls update active clients and synchronize after map loading. Configured gacha packs can be purchased and opened; AC75:3 scroll upgrades and point forging use atomic costs and native results. AC75:4 forwards arcade categories and refreshes balances; ticket/prize exchange remains pending.                                                                    |
| Gacha packs                  | `internal/assets/gacha.go`, `internal/server/gacha.go`, `internal/game/gacha.go`         | SQL-authored pack IDs and ordered reward views, native AC91 contents, AC23:75/128/96 opening, cryptographic weighted draws, exact-slot consumption and atomic reward delivery. Twenty complete source pools match translated-client items; four incompatible pools are individually unavailable without changing odds. Malformed tables fail startup; withdrawn Lucky Pack remains unavailable. Transient item reservations are enforced; original-client validation remains pending.                                                                                                                                                                              |
| Breillat conversion | `internal/world/breillat.go`, `internal/server/breillat.go` | Ten completed greetings unlock the native choice; acceptance permanently saves body 4/head 3 and quest marks atomically. Equipment is preserved. All eleven SQL-native scripts, decline/retry and failed claims have regression coverage; original-client validation remains pending. |
| Water gathering | `internal/world/gathering.go`, `internal/server/gathering.go`, `internal/store/gathering.go` | Four verified EVE water nodes, one sea/fresh water per gather, three-minute durable character/node cooldown, complete initial/repeat branch validation and atomic item/mark/timer saves. Other gathering pools remain pending. |
| Periodic autosave | `internal/server/autosave.go`, `internal/store/autosave.go` | One-second character checkpoints plus final disconnect checkpoint; transactional previous-snapshot comparison, no-op write avoidance, warp presence inclusion, bounded cancellation and per-character failure isolation. Current gameplay and account currencies continue to commit immediately. |
| Daily Lucky Draw             | `internal/server/lucky_draw.go`, `internal/store/lucky_draw.go`, `internal/assets/lucky_draw.go` | AC104:1 spin, native catalog/results with cumulative usage, three draws per character per UTC day, atomic durable rewards/usage, weighted SQL pool with 14 equal-weight reference outcomes, login/warp synchronization and online midnight refresh. Native catalog/result layouts recovered from the translated client; user verified rewards, usage and refresh in the original client. |
| Bag-item repairs             | `internal/game/repair.go`, `internal/server/repair.go`                                   | AC36 bag-slot repair, wrench-first or 500-gold payment, damage restoration with metadata preserved and atomic persistence. Healthy items are free; transient reservations are enforced; native-client validation remains pending.                                                                                                                                                                                                                                                                              |
| Compound synthesis           | `internal/assets/alchemy.go`, `internal/game/compound.go`, `internal/server/compound.go`, `internal/server/alchemy.go` | AC23:14 two-slot synthesis; seed and native binary recipes with first-match precedence, deterministic fallback, atomic ingredient/result persistence and native receipts/animation. AC40 recipe-only synthesis with compatible stack grants, atomic saves and status replies; full manufacturing and recipe editing remain pending.                                                                                                                                                                                                                               |
| Player trading               | `internal/game/trade.go`, `internal/server/trade.go`, `internal/store/character_pair.go` | AC25 requests, acceptance, item/gold offers, confirmations and cancellation; both character rows commit atomically, with metadata-preserving transfers and inventory reservations.                                                                                                                                                                                                                                                                                                      |
| Item vehicles and rafts      | `internal/game/vehicles.go`, `internal/server/vehicles.go`                               | AC15:14 owner-only placement, AC15:7/9 boarding, AC15:10 landing and AC15:13 acknowledgment; exact owned slot, capsule-family condition checks, persisted boarding, AC15:10/11 peer synchronization after map load, raft wear and exact-slot wrecking, mounted-slot move guard and transaction cleanup. Transient item reservations are enforced; native-client acceptance remains pending.                                                                                                                                                      |
| Native assets                | `internal/assets`                                                                        | NPC, Talk, Skill, exclusive maintained JSON item catalog with complete decoded native records, starter JSON, MBTM animation timing; all eleven EVE categories with raw sections preserved and Big5 names                                                                                                                                                                                                                                                                                |
| Web administration           | `internal/admin`                                                                         | Token authentication; account/session controls; versioned character, inventory, stats and preferences editors; GM studio; EXP/drop/status/log settings; SQL content and map editors; guild/marriage records; GM gifts; IP bans; battles; diagnostics; atomic startup configuration (see ADMINISTRATION.md)                                                                                                                                                                                                                                                                                                                        |

## Native data census

Read directly from the sibling Data directory, without modifying it:

| Data                                                  |    Count |
| ----------------------------------------------------- | -------: |
| Converted item definitions                            |    4,817 |
| Starter item grants                                   |        8 |
| NPC templates                                         |    4,928 |
| Nonempty dialogue IDs                                 |   17,493 |
| Skills                                                |      952 |
| Animation timing estimates                            |    1,795 |
| Maps                                                  |    1,119 |
| Map NPCs                                              |    8,181 |
| Events                                                |   10,644 |
| PreEvents                                             |    1,412 |
| Warps                                                 |    2,791 |
| Ground items                                          |      209 |
| Mall entries                                          |      199 |
| Two-input recipe rows (16 seeds + native projections) |      877 |
| Configured gacha pools / reward rows                  | 24 / 394 |

Parsing event bytecode does not execute events. Unknown native fields are retained without inventing semantics. Names use the source's ASCII rules for NPC/skills and Big5 for EVE. Dialogue byte strings retain the source bytes; localization beyond the supplied dialogue file remains unverified.

## Remaining work in dependency order

Only pending features belong in this list. Implemented behavior and verification evidence are recorded in the scope table and checkpoints.

1. **World simulation:** scene collision/constraints, NPC movement and other instance types.
2. **Quests:** other transformations, non-water gathering, unsupported event operands; remaining C# map-specific story patches in TryExecute/StartSession/FindBranch; shared prop break/respawn; remaining legacy database quest-definition execution. Port the outstanding Fred/Roca/Elin/Clive/Maka regression scenarios.
3. **Combat parity:** native area-target expansion and grade thresholds, hit/resistance rules and visual status acceptance.
4. **Companions and vehicles:** quest/model-driven pet evolution, potential training, remaining legacy pet operations and actual-client validation.
5. **Economy and social:** verified arcade scoring/ticket/prize rules; bank PIN authentication, character transfers and native ATM activation; guild vault/alliance channels; native nonempty market catalog and purchase history; native parcel request dispatch and gathering animations; verified physical workbench ownership, tent upgrades and decoration purchase rules.
6. **Storage compatibility:** read-only legacy import tooling, legacy password-format handling and character/inventory/pet/quest mappings; MySQL adapter and cross-backend checks.
7. **Acceptance:** World Entry native-client capture comparison (creation/login/reconnect, scene-ready, welcome, monster book, constellation and quest visibility); Items/Player State native-client acceptance (special-item targets, tents, furniture synchronization and return locations); Economy/Social native-client acceptance (stalls, guilds, marriage, parcels, manufacturing, synthesis and gathering); outstanding `.codex-verify` scenarios, actual-client packet comparisons, multiplayer load/soak and crash-recovery checks, native game-client validation, production server deployment/service packaging and operator procedures.

Each category still contains pending work. Full native-client acceptance and execution of the legacy PowerShell suite against Go remain pending. Historical checkpoint limitations below may be superseded by later implemented features and verification.

## Architecture rules for further ports

- Trace behavior to the recorded C# revision and the existing verification fixtures.
- Keep data parsing separate from gameplay execution and packet serialization.
- Decode a whole command before mutating state; reject truncated fields.
- Commit related economy and quest state together; emit success packets only after durable commit.
- Keep each session's commands serialized. Shared map/trade/battle mutations need explicit ownership or transactions.
- Preserve native packet layouts with golden bytes, including metadata and reserved fields.
- Update the status table and source inventory when a subsystem is verified. A source file is not considered ported merely because a similarly named Go type exists.
- Remove finished features from the remaining-work list as development proceeds. Keep only outstanding subfeatures; mark a category complete when none remain.
- Keep operator and developer instructions usable on Linux and Windows. Use shared commands where possible, label shell-specific syntax, and provide alternatives for paths, environment variables and platform dependencies.

### Platform scope of checkpoint commands

Checkpoint entries below are historical verification records on the named workstation, not platform prerequisites. Bash inline assignments (`WONDERLAND_TEST_DATA=... go test ...`) require the PowerShell equivalent: set `$env:WONDERLAND_TEST_DATA = '../Wonderland-Private-Server/Data'`, then run `go test -race ./...`. `make fmt-check lint build` uses a POSIX shell; direct Windows equivalents and dependency setup are in [README.md](../README.md). Client prerequisites, local asset packaging, extraction and test instructions for both OSes are in [CLIENT.md](CLIENT.md). Historical Windows-only CGO limitations do not establish a Linux limitation; use a CGO-enabled compiler toolchain on either OS for SQLite tests. Later successful verification supersedes earlier blocked-checkpoint reports.

## Resume checkpoint — migrated Linux workstation

- Actual checkout: `Wonderland-Go-Server`; session metadata initially still referenced `Wonderland-Go`.
- Fixed the inherited EVE mining-tail read (11 bytes, not 13). Added a two-record fixture and truncated-tail rejection; all 1,119 maps now load with every category enabled.
- Native item support currently reads `itemDat.wpdat`, the 47-byte converted format used by legacy inventory regression tests. Encrypted `Item.dat` is now decoded separately; see the native-item continuation below.
- Character creation uses the C# body/head bonuses and outfits, the starter JSON order, native skill IDs with the stunt's wire alias, and the original ship position (10017, 1042, 1075).
- All character and inventory changes in creation commit together. In-session name reservations are released on disconnect; database uniqueness handles committed names. Deletion codes use the existing password-hash format and are not returned in API responses.
- Characters can receive initial world packets and acknowledge the map, then persist movement and exchange peer appearance/movement/logout packets. This is a protocol development milestone, not a complete playable server. No real game client has been tested.
- At this checkpoint, the source accepted arbitrary creation attribute bytes. The World Entry completion below now enforces the verified native five-point budget; movement/scene validation has its own remaining work.

### Verification after resuming

- All 331 reference C# hashes still match the recorded source revision.
- `make fmt-check lint build` and `WONDERLAND_TEST_DATA=../Wonderland-Private-Server/Data go test -race ./...` pass.
- Native census: 4,817 converted items, eight starter grants, and 1,119 maps; no asset-loader warnings.
- A built-server smoke test used OS-assigned loopback ports and a temporary database. HTTP registration and administrator authorization, launcher status, two native-data TCP creations, peer appearance/movement/logout, and SIGTERM shutdown passed. This used a protocol test client; it does not establish native game-client compatibility.
- Map visibility operations currently serialize packet writes under one world mutex. Writes have deadlines, but slow-client throughput and large-world load still need work before deployment.

## Resume checkpoint — map scene and warps

- New `internal/world` package builds immutable per-map state from EVE: NPCs (click ID 0 and origin entries skipped, AC22:4 order by click ID) and ground items (slot from click ID 1–255, else a running index; respawn word, default 120 s).
- The EVE NPC parser now keeps the flag byte (bit 0 = initial visibility) and the linked-portal byte array previously discarded.
- `World.Visible` ports the data-driven core of `PreEventInterpreter.ShouldNpcBeVisible` plus its quest-mark overrides (maps 11016, 11040, 12000, 12002, 60002). Companion, vehicle, prop-runtime and timer state do not exist in Go yet, so those operands evaluate as absent; this matches a new character but not a migrated one. Quest spawn/despawn lists from the legacy quest database are not loaded.
- Quest-linked props show the opened frame when the linked event's quest is completed. The C# name-based `IsStaticNpc` heuristics are not ported; the check is limited to template 0, 12000–12999 and ≥19000.
- Map info reaches the entrant on login and after each warp, before AC12:1. Only the entrant is listed in the per-player records: Go still publishes peers at acknowledgment, where C# lists them earlier.
- Warps persist the destination before any success packet. The first destination-side snapshot follows AC12:1 as before.
- Not ported: tents, the portal override table (empty by default), Carnie return memory, party follow, mall catalog and quest replay after warps, territory ownership (map 60002 always sends an empty emblem).

### Verification

- `make fmt-check lint build` and `WONDERLAND_TEST_DATA=../Wonderland-Private-Server/Data go test -race ./...` pass.
- Native census: 1,119 maps, 8,181 NPCs (2,584 initially hidden for a new character), 209 ground items; 2,790 of 2,791 EVE warps resolve to a loaded map.
- Live smoke test with native data: a created character on map 10017 received 11 actor records, took portal 1 to map 10037 at (1038, 2235), and acknowledged the new map. This used a protocol test client, not the game client.

## Resume checkpoint — bag and ground items

- AC23:2/3/10 are handled under the world lock, so pickups, drops and respawns on a map are serialized. Inventory changes persist before AC23:5/9/10 and the AC23:2/4 broadcasts are sent.
- Pickups grant with the Go metadata-compatible stacking rule. C# stacks by item ID alone, so damaged or forged copies can occupy separate slots in Go.
- An empty bag slot in a drop request is ignored. C# replies with a destroy prompt for item 0.
- Not ported: item locks (no trade yet), mounted-vehicle slot protection, event/battle guards (neither system exists), and dropped-item expiry, which C# declares but never applies.

### Verification

- `make fmt-check lint build` and `WONDERLAND_TEST_DATA=../Wonderland-Private-Server/Data go test -race ./...` pass.
- A live smoke test warped to map 10037, walked onto native ground item 46113, picked it up into bag slot 9, rejected a repeated pickup, dropped it into ground slot 3 (slots 1 and 2 belong to native nodes), and picked it up again. A non-droppable starter item produced the destroy prompt.

## Resume checkpoint — equipment, chat and GM commands

- **GM trust deviation:** C# seeds GM names (`admin`, `Admin`, `developer`, `GM`, `test`, `gmone`) and reads `Data/gm_list.txt`; anyone registering such a name gains GM commands. Go grants GM only through the authenticated web administration (`POST /api/accounts/{id}/gm`), audited in the database.
- C# `SendSystemMessage` is disabled in the reference revision, so command feedback and public `:help` produce no packets in either server.
- All published-character actions (movement, bag, chat, poses) now run under the world lock. A summon or GM warp can change another session's state; actions that arrive after a warp but before its AC12:1 are ignored rather than treated as malformed.
- `:item` grants all-or-nothing (C# adds what fits). `:heal` stores the equipped maxima as the character's MaxHP/MaxSP. Not ported: the remaining GM commands (pets, bulk skill administration, rebirth, mute/jail, invisibility, battles, reloads, shutdown) and other administrative operations. Carnie travel is implemented in the travel-command continuation.
- Equipment changes are not broadcast to map peers, matching C#; peers see the new outfit after the next map entry.

### Verification

- `make fmt-check lint build` and `WONDERLAND_TEST_DATA=../Wonderland-Private-Server/Data go test -race ./...` pass, including a concurrent summon against the target's own movement and acknowledgments.

## Resume checkpoint — NPC events, journal and the starter story

- Census for a new character over all maps: 7,313 actors have events; 2,819 run end to end, 2,083 are refused before any change (1,906 start battles), and 2,411 have no eligible branch yet.
- Event state follows C# token semantics: a replaced or cancelled interaction ignores late acknowledgments; teleports and map entry clear it. Every reward commits the character before its packets are sent.
- **Deliberate fix:** a fallback dialogue line (opcodes 1/2 without a typed speech) ends the event on acknowledgment. C# only released the client and left the event marked active, so the next click was ignored until a map change.
- Not ported from the C# runner: the per-map story patches (Clive, Roca, Niss, Xaolan, Holy Village, mirrors, wells, S. Monkey's scripted recruitment), companion/party checks (evaluated as an empty party), shared prop break/respawn for other players, NPC services, and the second ReplayActorVisibility pass on entry (duplicate packets only).
- Live smoke test with native data: on map 10017 all eleven ship NPCs answered; the captain's storm warped to 10035, the rescue sequence completed, and Robinson's question and answer branch played through to release.

## Resume checkpoint — combat and levels

- The engine computes each round as soon as every command is in, then the server plays its animation steps without holding the world lock. A disconnect or replacement stops playback. All results (HP/SP, EXP, level, stat points, gold, loot) commit once before the ending animation, then the event resumes through its trigger-4 callback branch (result 1 victory, 2 defeat; flee only on the authored maps).
- Formation enemies use Npc.dat level, HP and element where C# reads its game database's `npc_data`; the same values feed both in the reference data set.
- **Deliberate fixes:** victory sends the new gold balance (AC26:4); C# adds gold silently. The EVE "EXP" reward now adds experience as C#'s CurExp setter does; the earlier Go port replaced the total.
- Capture always fails with the failure animation because pets are not ported. Skill proficiency, party members, companions, random encounters and the overworld monster's removal are not ported. Server EXP rate is fixed at 1.0.
- Census for a new character over all maps: 4,736 actor entry branches run, 166 are refused (companions, minigames, service windows, unsupported rewards) and 2,411 have no eligible branch.
- Live smoke test with native data: a GM-warped character on map 10070 clicked a field monster; the three-enemy formation spawned with Npc.dat stats, the round played and the defeat ending ran.

## Resume checkpoint — stats, items and services

- Stat points from level-ups are spent with AC8; HP/SP are not refilled and the maxima follow the new attributes. Pet targets and progression-skill unlocks are not ported.
- Recovery items restore their HP/SP status values above 100 on the character; pets, pet food, vouchers, gacha and tents are not ported. Shops only buy from players, as in C#.
- Census for a new character: 4,784 entry branches run; 118 are refused (companions, minigames, the Pet Hotel and unsupported rewards).

## Resume checkpoint — field encounters

- Wild-monster rules read the Npc.dat template name, level, HP and element, which the C# NPC catalog also supplies. Random encounters draw from every wild actor on the map, including those concealed for the character, as C# does; proximity encounters only consider visible ones.
- Live smoke test with native data: on map 11038 a step encounter started after 26 moves with three monsters, and fleeing returned the player to the map. The staged monsters there stay hidden for a new character, so no proximity battle started, matching the C# visibility rule.

- **AC29 is deliberately not ported.** Despite its name in the Go status list, the C# AC29 handler is a legacy item-storage path: depositing into an occupied slot overwrites that slot's count and withdrawing grants the whole stack while removing part of it. The C# AC30 code records that the native client uses AC30 for item storage.

## Resume checkpoint — pets

- Census for a new character: 4,826 entry branches run; 76 are refused (minigames, the Pet Hotel and unsupported rewards).
- Captured monsters join the party when the battle ends rather than mid-round. Mounts, pet equipment changes (AC23:17/18), the Pet Hotel and pet stat allocation are not ported.

### Pets continuation — Pet Hotel

- Added persisted `hotel_pets` separate from the party and story reserve, with deep cloning of skill progress. Existing character JSON loads without a schema change.
- EVE service 5 opens the native hotel. AC31:2 withdraws hotel slots, AC31:3 deposits client team slots, and AC31:4 exchanges selections. The whole request is validated before mutation; full-list exchanges work, companion aliases cannot duplicate in the final party, and state commits before roster changes or success packets.
- Hotel records use the native AC31:6 layout, explicit AC31:4 stale-slot removals, and AC31:3 copying before AC15:2 removal. Transfers preserve names, skills, EXP, attributes and equipment; deposited battle pets rest and despawn for peers. Recruitment and ownership conditions include the hotel.
- Build, vet, game/world/protocol/assets tests, the hotel golden-record test and AC31 decoder tests pass. The SQLite-backed exchange/persistence regression compiles but cannot run with this workstation's CGO-disabled SQLite stub. Native-client testing remains pending.
- Remaining pet work after this checkpoint is updated in the continuation below.

### Pets and event services continuation — 2026-10-01

- AC8 allocates points on the pet identified by its per-login client slot. Equipment wear/remove (AC23:17/18) swaps or removes complete item records, commits bag and pet together, then sends native AC23:23/22 and progression/stat updates. Pet HP/SP are clamped after recalculation without refilling.
- `pet_vouchers.json` loads all-or-nothing, rejecting empty, duplicate, zero and out-of-range mappings. AC23:96/15 redeem one voucher on the character; invalid count/target, missing data, duplicate party pets and full parties retain the voucher. New pet and item consumption commit together. As in C#, hotel copies do not block voucher redemption; retrieval still prevents duplicate party companions. Item locks remain unported.
- Actors without native events use QuestNpc's prioritized name/template service fallbacks: storage, Pet Hotel, doctor rest/record-point/hotel choices, and the two-stage shop selling menu. Existing EVE, visibility, reach and repeat-click handling take precedence. Shop menus release the movement lock after selection, a deliberate fix for C#'s lingering callback/lock.
- Supported EVE opcode 11 skill rewards commit the learned skill before native AC8:1 and AC5:16/12/11 packets. Repeated learning preserves trained grade and proficiency; unknown skills and unsupported reward operands are rejected before event actions run.
- Native-data `go test ./... -count=1` and `go test -race ./... -count=1`, build and vet pass. SQLite-backed hotel/equipment/stat/voucher/story recovery regressions execute successfully with the user's toolchain outside the restricted environment (CGO enabled, GCC available). Earlier CGO limitations in these checkpoints describe the restricted environment only and are superseded by this verification. The later skill-reward persistence/unknown-skill preflight regression also passes. No native-client validation is claimed.
- Updated `source-inventory.json` mappings and partial-status notes without replacing the recorded source hashes. The sibling Git checkout is clean; 330 of 331 hashes match after CRLF-to-LF normalization preserving BOM. `Src/Network/ActionCodes/AC39.cs` still differs from its inventory hash and was not used in this continuation.
- Remaining pet work: item vehicle replication, rebirth, item locks and native-client validation. Companion mounts, native pet rename/release, character skill progression/evolution and battle proficiency are covered below.

## Combat skill proficiency — 2026-10-01

- Executed offensive and support actions award one skill EXP to the player or pet. Frozen/skipped turns, unaffordable skills and unlearned skills do not advance proficiency. Defend, capture, flee and monster actions do not issue skill-use receipts.
- Player progression accepts the starter-stunt alias 15003, advances through grade thresholds of grade × 100 and emits native AC5:11/12 and AC8:1 updates. Pet progression emits AC8:2 stats 110/111 using the per-login slot, uses a wide EXP accumulator and clears EXP at grade ten; players preserve remaining EXP as in C#.
- Each animation step saves the changed character before proficiency or action packets. Failed saves retain session skill state, stop playback and disconnect; detached battle pet skills refresh after a successful save. Progress also persists when a pet has no client slot.
- Stat-qualified skill unlocks and grade-ten evolution unlocks are covered below. Native-client validation remains pending.
- Validation: native server-data full tests and race suite, `go vet ./...` and `go build ./...` pass. Added support-action, frozen-turn, pet persistence without a client slot and save-failure regressions pass with race detection.

## Stat-based skill progression and evolution — 2026-10-01

- Ported all 92 element/stat requirements (including Dark and Undefined) and 78 grade-ten evolution links from SkillManager. Requirements use base attributes plus avatar bonuses. Compound requirements check both attributes; evolution unlocks one successor at grade one and preserves existing grades/EXP.
- Creation and login add catalog-supported qualified skills before building the native snapshot. Login saves newly learned skills before sending the snapshot, without replaying incremental allocation updates. Stat allocation and combat grade increases unlock qualified skills in the same durable character transaction, before native AC8:1 and AC5:12/11/4 packets. Allocation does not refill HP/SP or clear shortcuts.
- Missing catalog entries remain unlearned so incomplete deployments do not create invalid login snapshots. This is a deliberate guard beyond the C# reference. Login performs stat qualification only, matching CheckAndUnlockProgressionSkillsNoSend; allocation and grade increases include evolutions.
- Added requirement, secondary-attribute, element, avatar-bonus, replay-preservation, missing-catalog, evolution-chain, allocation persistence, login persistence and failed-save regressions. Native-client skill-book validation remains pending.

## Battle reward commit ordering — 2026-10-01

- Battle captures and pet desertion now stage client roster changes and peer broadcasts. Roster adoption, rewards, departure packets and pet broadcasts happen only after the character save succeeds.
- Failed result saves stop playback, abandon the battle and disconnect without sending success. Existing persisted pets, party state and client-slot numbering remain unchanged. This fixes the earlier behavior that logged failed saves but continued departure playback and could publish uncommitted pet changes.
- Added failed-capture/failed-desertion and committed-desertion regressions. No new deployment steps are required on Linux or Windows; use the shared test instructions in README.md.
- Validation: full native server-data tests, the full race suite for skill progression, and an additional server/game race run after battle commit changes pass. `go vet ./...` and `go build ./...` pass. These checks ran on Windows with CGO/GCC; Linux execution and native-client skill-book behavior were not tested in this session.

## Companion mounts and native pet controls — 2026-10-01

- AC15:11/12 mount/rest a party companion using a validated per-login client slot and pet ID/alias. The owned mount's broadcast ID is persisted in character JSON. Owner and peer packets match native AC15:16 (37 bytes, 26 reserved zero bytes), AC15:17 and AC5:8; owner snapshots retain the party slot, while peer-entry snapshots use slot one as in Map.SendPeerCompanionAndVehicle. Mounts survive world entry/reconnect and are resent on map transitions.
- AC15:6 renames a client-selected party pet using raw ASCII payload bytes, .NET ASCII replacement, trimming and a 16-byte cap. AC15:9 confirms the new name and active battle-pet visuals update for peers. Blank and internal control-character names are refused.
- AC15:2 permanently releases the selected party pet and frees its client slot. This differs from EVE story dismissal, which still preserves eligible companions in reserve. Release, EVE dismissal, hotel deposit and battle desertion clear a matching mount only after the character save; removing another battle pet preserves the current mount appearance.
- All mutations save before owner or peer success packets. Failed saves retain pet/roster/mount state; malformed commands and stale slot/ID pairs are refused. The slot/alias checks, durable mount retention and control-character name guard are deliberate robustness improvements over the C# handlers.
- Added mount wire, alias/slot, persistence, owner/peer snapshot, login restoration, replay, stale-rest, hotel/dismissal cleanup, rename replication, permanent release, preservation of another mount and failed-save regressions. Item vehicles, raft subcodes and native-client validation remain pending. No platform-specific setup is added; shared Linux/Windows server instructions apply.
- Validation: full native server-data tests, `go test -race ./... -count=1`, vet and build pass on Windows with CGO/GCC. Linux execution and actual-client mount/rename/release validation were not performed in this session.

## Resume checkpoint — map 11077 interrupted arrival recovery

The following is a historical checkpoint; later verification above supersedes its toolchain limitations.

- Ported `EveEventRuntime.TryExecuteStoryArrival`'s map 11077 recovery: quest 13086 in progress at step 2 resumes event 12 with actor click 13 and no transition. If quest 13087's death flag is already in progress above step 0, only branch 1's authored opcode completing quest 13086 runs, through the existing durable quest mutation path.
- Missing event data or an ineligible branch does nothing; completed progress does not replay. Added regression coverage for farewell resumption, cleanup without dialogue, durable completion, and repeated arrival.
- `go build ./...`, `go vet ./...`, and tests for `internal/game`, `internal/world`, `internal/protocol`, and `internal/assets` pass on Windows. The new server regression tests compile but cannot execute in this environment: CGO is disabled, and the SQLite driver reports its CGO-required stub. No GCC/Clang compiler or WSL distribution was found. This slice remains pending runtime verification with a CGO-enabled toolchain; no native-client validation is claimed.

## Teams and team battles — 2026-10-01

- Ported from AC13 and the Team region of `Player.cs`. Deviations from C#: a join reply is honoured only for a pending request (one minute); only a leader or a solo player accepts; a player already in a team cannot ask to join another; disconnected members leave their team (C# keeps them); only members on the leader's map follow its portals (C# moves members from any map). Teammate vitals are sent when they change, after each command, rather than on every Send8_1.
- Team battles follow PvEBattleManager's GetTeamMembers and endings. Teammates already fighting or in an event are left out, which C# does not check. A disconnected member's fighters stay on the field and defend without delaying the turn.
- Added regressions for join, decline, unsolicited accept, roster and vitals, leadership transfer, kicks, the four-player limit, portal follow, arrival replay, logout, shared turns, team rewards, team escape and a member's disconnect. The full suite passes with the race detector. Native-client validation of the team packets remains pending.

## Item vehicles and raft continuation — 2026-10-01

- Ported VehicleManager and Inventory.ApplyVehicleWear through the existing serialized world handlers. Placement sends AC15:18 coordinates to the owner and does not board; confirmations broadcast AC15:10. Landing another vehicle dismounts without removal, while the authored quest raft (48016) breaks on landing, on the beach at (280+, 950+), or on a successful regular exit from map 11016. Raft and capsule-family 48010/48028 wear by one per moved packet and break at damage 100; stationary packets and other vehicles do not wear.
- Wrecking consumes one item from the exact mounted slot and emits AC23:9, AC15:15, AC15:11 and AC5:4 after saving. A stale slot only dismounts; another matching raft is never substituted. Repeated board/landing callbacks are harmless. Bag moves involving the mounted slot are refused. Removal through storage, destruction or quest rewards clears invalid riding state in the same character save.
- Vehicle boarding survives JSON persistence and login. Invalid riding state is cleared before the login snapshot is saved; owner mount synchronization waits for AC12:1. Peer entry includes the vehicle before companions. Boarding and companion mounting exclude one another, with their native removal packets sent after the save. Capsule family comparisons now qualify EVE's active-vehicle subject (2/1/4). No SQLite schema bump is needed for the added optional character fields.
- Deliberate changes: riding state is persisted rather than limited to the C# runtime, malformed vehicle commands are refused, and character/bag changes commit before receipts. Item locks remain unported. The reference declares fuel fields but has no active fuel consumption or refueling handler, so no fuel behavior is invented.
- Added golden placement/boarding/wreck sequences, exact-slot and alias checks, map isolation, mounted bag-move protection, storage cleanup, ordinary-vehicle landing, stationary/raft movement, mount exclusion, battle gating, portal landing, failed-save and database reopen regressions. Native-data tests and race suite, vet and build pass on Linux with CGO/GCC. Native game-client validation remains pending.

- The live TCP smoke test exposed an inherited dispatch omission: AC15 was handled inside `worldCommand` but absent from `dispatch`. Added the route and dispatcher regressions for vehicle commands and companion mount/rename/release.
- Built-server native-data smoke test passes using OS-assigned loopback ports and a temporary DB: two clients exchange vehicle boarding and movement; raft damage survives SIGTERM/restart; map acknowledgment restores the owner mount; landing emits the exact-slot wreck sequence. Graceful shutdown passes. This verifies the protocol test client, not the original game client.

## Player trade continuation — 2026-10-01

- Ported Trade.cs and AC25 through the world dispatcher. Requests require nearby available players and expire after one minute. AC25:3 and :4 replace the entire offer, as in the reference; AC25:5 is a no-op in the C# handler. No extra lock handshake is required.
- Each offer revision clears both confirmations. Inventory-changing client commands are blocked while the window is open. Movement, successful warps, battle entry, disconnect and external inventory/gold changes cancel trades. Pending-request cancellation closes the partner prompt.
- Completion validates both saved characters and applies outgoing removals, incoming grants and gold in one SQLite transaction. Item damage and metadata survive transfers. Removing both outgoing offers first permits swaps between full bags. Stale items, insufficient gold, capacity failures and balances above 999999 reject the entire exchange. No success packets precede the commit; repeated confirmations cannot transfer assets twice.
- These checks deliberately fix the reference's separate saves, stale confirmations and metadata loss. General item locks, player shops and other economy handlers remain pending. No schema change is required. Original game-client validation remains pending.
- Validation passes: full native-data race suite, formatting, vet, HTML lint and build. The built-server TCP smoke uses temporary accounts/database and OS-assigned loopback ports, verifies native request/accept/offer/completion packets, a first-confirmation barrier and replay safety, then restarts the process and checks both saved balances and the transferred item. SIGTERM shutdown passes. This verifies a protocol test client; original game-client validation remains pending.

## Player preferences continuation — 2026-10-01

- Ported AC16 direct setters and AC33 toggles/current-settings snapshots through the world dispatcher. PK permission, team request permission, trade permission and channel masks share a persisted JSON settings record. Older characters without that record use the native ClientSettings constructor defaults (all three permissions enabled, channel mask 31). This replaces Go's fixed login packet, which previously disabled PK permission. PvP execution remains pending.
- AC16 defaults a missing value to one; trade-lock/team-reject use inverted permission semantics. AC33 toggles take a setting ID; the channel setting takes an additional byte and has no acknowledgment in C#. Boolean toggle acknowledgments use one for on and two for off. AC33:2 and login return the current settings. Settings remain available before map acknowledgment and during battle.
- Walk mode is session state, matching Player.WalkMode; AC33:5 returns the supplied team-follow value (default one). The reference does not apply either setting to movement or party warps. Channel masks are stored and returned; channel filtering is not added because the reference chat handler does not use them.
- Deliberate improvements: both team request and acceptance recheck permission, disabling team requests removes pending invitations, and trading requires permission on both sides. Disabling trading cancels active trades and pending prompts without transferring assets. Existing teams remain intact. The native request handlers expose settings but omit these guards. Persisted preferences commit before acknowledgment, and malformed or unknown settings cannot mutate state.
- Added dispatcher golden-packet, malformed-input, save-failure, clone-isolation, login snapshot, team request/accept/re-enable, active/pending trade cancellation, and pre-ack/battle regressions. Original game-client validation remains pending. No schema bump is required for the optional JSON field.
- Validation passes: full native-data race suite, formatting, vet, HTML lint and build. A built-server loopback smoke with temporary accounts/database checks native defaults, setters/toggles/channel queries, walk/follow acknowledgments and rejected team/trade requests, then verifies the saved settings snapshot and toggles after process restart. SIGTERM shutdown passes. These checks use a protocol test client, not the original game client.

## Admin account management — 2026-10-01

- Added authenticated account deletion and password reset endpoints and actions in the Accounts table. Reset uses the existing salted PBKDF2 hash and game password validation; deletion removes both character slots and the account-security record. Account changes and audit entries share a transaction, and no credential is written to the audit record. Existing account attributes and character-deletion codes survive password reset.
- Successful changes disconnect the active account session. Account mutations exclude concurrent login authentication/publication so an old credential cannot escape the disconnect. Missing accounts and validation failures return explicit errors. The UI confirms permanent deletion and provides masked password/confirmation fields.
- Store/API race tests, formatting, vet, HTML lint, JavaScript syntax and build checks pass. A built-server smoke with a temporary database verifies active-session disconnection, rejection of the old password, acceptance of the replacement, account/character removal across restart and registration under a fresh account ID. Browser interaction has not been exercised.

## Admin session termination — 2026-10-01

- Exposed the existing kick operation as Terminate session in the renamed Sessions view and online account rows, with account online/offline status, pending-button protection and result feedback. Requests use the existing authenticated kick endpoint; zero/invalid IDs return 400 and missing sessions return 404. Successful admin requests are logged.
- Targeted race tests, formatting, vet, HTML lint, JavaScript syntax and build checks pass. A temporary-database built-server smoke verifies TCP closure, session-list removal, trade cancellation and peer departure notification, continued connectivity of the other player and reconnection under the same account/character. Browser interaction remains untested.

## Quest minigames continuation — 2026-10-01

- Ported EVE opcode 9 and AC57:1. The start packet contains game type and a 24-bit seed (authored dialog2 with high byte one, or 0x012AF8), followed by AC20:9 mode lock. Valid win/loss reports consume the current outcome callback before sending AC57:2 and selecting condition-kind 8 by game type and result. EVE owns dialogue, rewards and movement release; no generic voucher or extra reward is added.
- AC20:6 cannot advance a running game. Malformed, unknown-result, unsolicited and duplicate reports outside a current game cannot consume or reward. Movement and ordinary item actions are held while the game owns the interaction. Choice cancellation, successful warp and disconnect clear event ownership; retained callbacks of a replaced event are inert. Outcome branches can launch another game without losing its callback. Unsupported game-type operands are refused before a branch mutates state.
- The native rabbit event (map 12000, event 41, actor 25) asks question 12, routes success to dialogue 20363 and grants ten carrots (32102), and routes loss to dialogue 20364 with no reward. Capacity checks and database commit precede reward receipts. Full bags and failed saves cannot grant a partial reward.
- The protocol reports client-declared outcomes and carries no per-game identifier. Results are not server-verified scores, and a delayed valid report arriving during a later game cannot be distinguished from that game's report. Arcade tickets/prizes (AC71/72), gathering timers and original-client validation remain pending.
- Full native-data race suite, formatting, vet, HTML lint and build pass. Native-data tests cover rabbit outcomes and persistence; synthetic tests cover golden seeds/packets, acknowledgment holds, malformed results, stale callbacks, cancellation/warp/disconnect, chained games, capacity and failed saves. Built-server loopback smoke with temporary accounts/database passes the real actor/question/game/result path, win/loss rewards, replay and reward persistence after restart. SIGTERM shutdown passes. The original game client remains untested.

## Friendships and presence continuation — 2026-10-01

- Ported the friendship portion of AC14: requests by exact character ID across maps, acceptance with group/default one, removal, and list queries with no ID or ID zero. Acceptance emits native AC14:3/9/7, AC10:3 presence and AC14:11/5 lists. Records use a length-prefixed name, level/element/body/head and four color words; rebirth/job/nickname/guild use empty defaults because those character features remain unported. Group is an acknowledgment field, as in native AC14; it is not stored as a list column.
- Schema version 4 adds one symmetric relationship per ordered pair, foreign keys to characters and deletion cascades. Existing accounts, credentials and character JSON remain intact. The 49-player capacity follows Friendlist's fifty slots with slot zero reserved for Cupid. Acceptance checks both lists inside the insert transaction; repeated inserts are idempotent. Character deletion and administrator account deletion remove relationships. Online friends receive removal and refreshed lists when an administrator deletes an account.
- Requests expire after a minute and require a pending invitation before acceptance. Self/unknown targets, duplicate prompts, expired/disconnected requests and replayed acceptances cannot create relationships. These checks deliberately tighten C# AC14, which permits unsolicited acceptance and bypasses Friendlist capacity. Success follows durable storage; failed saves cannot publish acceptance.
- Online/offline notifications refresh friends across maps. Presence stays online while a map is loading, and disconnect during that phase still sends offline status. Empty lists are returned on query; users without friends receive no extra unsolicited login packets. The in-memory native Cupid entry is excluded from native friend-list serialization and is not added to the Go lists. AC14:1 mail/name fallback and legacy AC10 command compatibility remain pending.
- Full native-data race suite, formatting, vet, HTML lint and build pass. Tests cover golden color/appearance records, symmetric persistence, concurrent inserts, both-side capacity, invalid targets, request expiry/departure, failures, cross-map presence, map loading and deletion cascades. Built-server temporary-database TCP smoke passes request/accept/list/removal, duplicate acceptance, offline/online notifications, persistence across restart, and administrator deletion cleanup. SIGTERM shutdown passes. Original-client validation remains pending.

## GORM persistence migration

- Replaced application SQL in Store with GORM models, query builders, upserts and explicit transactions. Accounts, character creation/deletion, atomic character updates and trades, deletion credentials, friendships, settings and audit entries use the abstraction. The public Store API and missing-record errors remain compatible.
- Schema version stays at 4. Existing table definitions, foreign keys, case-insensitive names, account ID sequence, password hashes and character JSON are preserved. SQLite-specific schema SQL is isolated in `internal/store/migrations.go`; fixture SQL remains in tests. Automatic schema migration is disabled. SQLite remains the supported backend; a MySQL adapter still needs implementation and verification.
- GORM query logging is disabled so bound credentials are not logged. Zero-value updates explicitly persist false, zero and empty strings. Multi-record operations retain atomic rollback.
- Verification passes: full native-data race suite, formatting, vet, HTML lint and build. Regression tests open an independently created version-4 database and check schema preservation, authentication, name uniqueness, ID allocation and character updates, plus zero-value updates, setting upserts and context cancellation. Built-server temporary-database smoke tests pass for trading, friendship presence/deletion cascades and administrator password reset/account deletion, including restart persistence and graceful shutdown. Original game-client validation remains pending.

## Numeric naming convention

- Named protocol commands/subcommands and reply values, EVE action/condition/comparison codes and selected operands, shared limits, account ID offsets, credential parameters, channel bits, packet framing sizes, distances and map references. Names belong to the package that owns their meaning. Direction-specific codes have separate names when incoming and outgoing meanings differ.
- Unverified content locations and wire meanings use documented namespace-based fallback names (`MapID12000`, `FriendsWireCode13`). Numeric compatibility values remain explicit. Raw expected bytes in golden tests and authored data tables remain independent.
- The convention is in [DEVELOPMENT.md](DEVELOPMENT.md), linked from the README and root agent instructions. Full native-data race tests, formatting, vet, HTML lint and build pass.

## Resume checkpoint — item-mall purchases

- Traced `ItemMallManager`, AC75 and AC23:25/26/54. World entry sends the points and bonus catalogs, settings/status and both balance records before the final ready markers. Catalog rows retain the native ten-byte item/bundle/price/discount/badge/category/order layout. Balance records use separate AC75:3 and AC75:9 packets with zero spent/item/count fields.
- Native carts validate every item's category, order and nonzero quantity against the available catalog. Quantities buy catalog bundles. Every row gets an AC75:4 success/failure receipt. AC23:26 retains the legacy omitted/zero quantity default of one bundle. Malformed commands are decoded completely before mutation.
- Currency deduction and all inventory additions use one explicit GORM transaction on authoritative saved account/character state. Insufficient funds, stale rows, unknown items, full bags, ownership failures, bans and failed writes cannot deliver a partial cart or publish success. Gameplay ownership gates prevent purchases before map acknowledgment or during battles, trades and quest minigames.
- Schema v5 adds `accounts.im_bonus`, default zero with a nonnegative constraint. Existing accounts, point balances, characters and deletion credentials survive migration. Bonus balances appear in the administration account list. Recognized gacha packs remain unavailable until their reward tables and opening support are ported; forging, arcade launch and mall point-grant controls remain pending.
- Verification passes: full native-data race suite, final targeted mall race tests, formatting, vet, HTML lint and build. Independent golden tests cover catalog/balance/cart packets and legacy requests; Store tests cover v4 migration, currency selection, concurrency, ownership, rollback and restart. A built-server native-data TCP smoke with a temporary funded account passes initialization, both currency purchases, bundle delivery, stale-cart rejection, restart persistence and graceful shutdown. Original game-client and browser interaction remain untested.

## Resume checkpoint — client login and character selection

- Corrected selection records against the decompiled aLogin decoder now ported in the Go client. Rebirth/job placeholders precede the six equipment IDs, and HP/SP fields carry maximum then current values. This deliberately corrects the legacy C# serializer's shifted equipment and reversed displayed vitals. Rebirth and jobs remain unported and use zero placeholders.
- Bare AC63:0 returns from character selection to the account form without an acknowledgment or socket closure. It releases the account and pending creation-name reservation, resets session identity and GM status, and permits reauthentication or account switching on the same connection. Malformed requests and returns from an active world character are rejected.
- Authentication validates the complete optional native XOR token tail before checking credentials or reserving an account. Requests without a tail remain supported. The token remains a diagnostic hint rather than an authentication factor. No schema change is required.
- Validation passes: full native-data race suite, formatting, vet, HTML lint and build. Independent golden records and server regressions cover roster alignment, asymmetric vitals, reservation cleanup, account switching, malformed requests and optional token tails. A built-server TCP smoke uses the actual Go client login encoder and roster decoder to check both character slots, equipment, vitals, Previous, reauthentication and account switching with a temporary database. Graceful shutdown passes. The original game client remains untested.

## Resume checkpoint — native compound synthesis

- Ported native AC23:14 from Recv14: exactly two distinct inventory slots, one consumed unit per slot, one fresh output, AC23:9 removals, AC23:8 result with 28 reserved zero bytes, AC23:13 popup and AC23:122 animation to the owner and acknowledged peers on the same map. The command is reached through the existing inventory dispatcher.
- AlchemyManager's sixteen seed recipes precede Compound2.dat and Compound.dat projections. Native word decoding and file/row order are preserved; the first symmetric input match wins. The census exposes 877 rows, including repeated input pairs. The reference handler ignores stored success rates and uses the greater input ID when there is no recipe; Go preserves that deterministic behavior. The two-input projection deliberately follows the reference alchemy lookup even when manufacturing records have additional ingredients, quantities or tools. Full manufacturing is still pending.
- Ingredients and the fresh result commit in one character transaction before receipts. The lower ingredient slot is preferred when emptied; otherwise another empty slot is used. Leftover quantities, damage and metadata survive. Results use a separate fresh slot rather than merging into an existing stack. This deliberately fixes the reference's consumed ingredients when its preferred slot cannot accept the output, misleading success receipts and save-after-success order. Full bags, failed saves, unknown IDs and malformed or extra operands consume nothing. Empty-slot replay is harmless; repeating a valid command with remaining ingredients is a new synthesis.
- Synthesis is held before map acknowledgment and during battles, trades, events, storm/beach cutscenes and use of an ingredient as the mounted vehicle. No SQLite schema change is required. Item locks, advanced alchemy, alternate AC40, custom legacy database recipe import/editing and native-client validation remain pending.
- Validation passes: full native-data race suite, formatting, vet, HTML lint and build. Regressions cover independent encoded recipe records, first-match precedence, deterministic fallback, exact response bytes, map isolation, persistence, leftovers, invalid slots/counts, full bags, failed saves, replay and ownership gates. A built-server native-data TCP smoke with temporary accounts/database checks the real binary recipe 37206 + 37207 → 20031, native receipts, animation, leftover metadata, full-bag rollback and replay, then restarts and verifies the inventory snapshot. Graceful shutdown passes. This uses a protocol test client; original game-client validation remains pending.

## Resume checkpoint — audited mall currency controls

- Ported AC02's `:im`, `:points_im`, `:mallpoints` and slash aliases through GmManager.AddMallPoints semantics. A signed delta adjusts ordinary points for the GM or an explicitly named online character. Existing GM authorization applies. Unknown explicit targets make no change rather than falling back to the GM's own account.
- Added Accounts → Adjust mall points and authenticated `POST /api/accounts/{id}/mall` with currency `points` or `bonus` and a nonzero signed 32-bit `delta`. Both online and offline accounts are supported. Deductions floor at zero as in C#; positive overflow and invalid saved balances are refused instead of wrapping. No database schema change is required.
- A GORM transaction reads the authoritative balance, applies the delta and creates one `mall_adjust` audit record. Its JSON subject records the account, actor, currency, requested/applied delta and before/after values. A failed audit insertion rolls back the balance. Concurrent grants and purchases cannot lose increments or spend stale funds.
- Administrative changes follow account/world lock order and serialize receipts against gameplay purchases. Ready clients receive native AC75:3 and AC75:9 balances after commit. Changes during initial login or a warp synchronize at the next AC12:1; initial snapshots stay contiguous. A failed receipt closes that client while the committed administration request still succeeds, avoiding a misleading failure after funds have changed.
- The web form selects currency, validates integer bounds, disables repeated submission while saving, refreshes the account list and confirms the saved balances. A refresh failure cannot reopen a successfully submitted form. Dashboard status now includes native compound synthesis and currency controls rather than listing them as pending.
- Validation passes: full native-data race suite, formatting, vet, HTML lint, JavaScript syntax and build. Store regressions cover both currencies, signed limits, corrupt balances, audit rollback, restart and concurrent purchases; server tests cover GM authorization/aliases/targeting, map-loading synchronization, ordered concurrent updates, failed writes and spending the saved grant. HTTP tests cover token authorization, required fields, malformed/out-of-range JSON, missing accounts and audit failures. A built-server native-data loopback smoke with temporary accounts/database passes HTTP point/bonus updates, live GM grants/deductions, non-GM rejection, login synchronization, a funded native purchase, audit records, offline updates, restart persistence and graceful shutdown. Browser interaction and original game-client validation remain pending.

## Resume checkpoint — configured gacha packs

- Ported GachaManager and native AC91:1/2 contents previews. The request carries a UInt16 pack ID and cache byte; the response always supplies the authoritative ordered list with version one. Unknown, missing and withdrawn pack IDs return empty lists. Preview requires no owned pack and remains read-only during trades.
- `gacha_packs.json` loads only after item definitions. Every pool must use a recognized nonwithdrawn pack, known reward IDs, one to 41 rows, quantities one to 255 and positive weights summing to 10,000; duplicate pack IDs are refused. A missing or invalid table enables no pools and reports an asset warning. The supplied data contains 24 pools and 394 reward rows. These supplied weights are custom. The recognized withdrawn Lucky Pack (34333) remains unavailable.
- AC23:75 and :128 decode a full UInt16 inventory slot; AC23:96 shares the opening path after pet-voucher handling. One cryptographic draw uses rejection sampling, then consumes one pack from that exact slot and grants the configured fresh item quantity. Grants split at each item's native stack limit and preserve other stacks' damage/metadata. The freed pack slot can hold a reward. A full bag or failed save retains the pack and publishes no success.
- The whole character change commits before native AC23:9 removal, additive AC23:5 reward records, AC23:15 use sound and the private gacha banner. Empty-slot replay grants nothing; a repeated request with another pack is a new opening. Opening is held during loading, battles, trades, events, storm/beach cutscenes and use of that inventory slot as a mounted vehicle. This deliberately improves C#'s save-after-receipts ordering and metadata-insensitive stacking. General item locks and remaining special-item use handlers are still pending.
- Mall filtering now checks configured pack availability rather than suppressing every recognized pack. Both purchases and previews use the startup asset snapshot. No SQLite schema change is required. Dashboard status includes gacha opening and reports the available pool count.
- Validation passes: full native-data race suite, targeted gacha race regressions, formatting, vet, HTML lint, JavaScript syntax and build. Independent packet tests cover all three opening paths, cache hints, previews without ownership, large UInt16 slots, malformed input, exact-slot metadata, quantity splitting, unavailable pools, capacity, failed saves, gameplay gates and purchase/opening. Asset tests enumerate all 10,000 roll slots against fixed expected weights and reject whole invalid tables. A built-server native-data TCP smoke with temporary accounts/database checks authored contents and reward membership, all three openings, private receipts, full-bag retention, withdrawn Lucky Pack behavior, a funded mall purchase/opening, restart inventory and graceful shutdown. Browser interaction and original game-client validation remain pending.

### Continuation: atomic bag-item repairs

- Ported AC36 and EquipmentRepairManager: a three-byte request selects a bag slot and echoes the subcommand/slot with a success flag. One wrench (48050) takes priority over a 500-gold fee; the hammer effect (60015) reaches the owner and peers on the same map. This addresses inventory slots, despite AC36's worn-equipment description.
- Deliberate corrections: the reference charges and announces success without restoring damage or saving. Go clears damage to zero, preserves quantity/forge metadata, and commits payment and repair together before receipts. Healthy, empty and unknown items cannot charge payment. If the selected item is itself the sole wrench, payment must use another wrench or gold so the repaired item remains present.
- Native inventory receipts remove the old repaired stack before adding its replacement with AC23:5, whose quantities are additive. Wrench payment from the same stack accounts for its remaining quantity. This works with a full bag and preserves other copies of the same item.
- Loading, battles, trades, events, storm/beach scenes and a mounted target slot block repairs. Equipped items must first be moved into the bag. General item locks and original-client validation remain pending; no schema change is needed.
- Verification: full native-data race suite, formatting, vet, HTML lint and build passed. Tests cover golden packets, payment priority/boundaries, persistence, metadata, healthy replay, same-slot wrench payment, full bags, malformed/unknown slots, gameplay guards and failed saves. A built-server loopback smoke with a temporary database passed exact receipts, same-map effects, gold/wrench payment, insufficient funds, healthy replay, metadata/alias isolation, restart inventory/balances and graceful shutdown. The smoke uses a protocol test client.

### Continuation: native Item.dat decryption

- Traced the local native client's loader (`FUN_003cd92c`) and transformation routine (`FUN_003cdfa4`). Version-nine files have a plaintext 451-byte header and encrypted 451-byte item records. Go reverses name/description buffers and applies every native byte/word/dword XOR-minus-nine transform with field-width wrapping. All decoded bytes are retained, including unresolved fields; client-only content patches are not applied.
- Native records supply the base gameplay catalog; optional converted records override matching definitions and retain server-only IDs. The native records remain separate for inspection. Either format can load alone, absent optional files do not warn, and corrupt present files warn without publishing partial definitions. Status reports native, converted and combined counts.
- The supplied server catalog has 7,243 native definitions, 4,817 converted overrides and 7,520 combined IDs. The translated client catalog has 7,312 native definitions. Converted rows differ in names, stats and IDs, so their precedence deliberately preserves existing definitions. Some authored gacha rewards are converted-only and require the override file.
- Added complete-record artificial goldens, Big5/description/negative-value/wrapping checks, header/text/bounds validation, duplicate/zero-ID handling, source-buffer isolation, format fallback and corruption tests, and independent full-decoded-table hashes for both supplied native catalogs. Native equipment-level rules and other property consumers remain unresolved; item locks and original-client validation remain pending. See [the format notes](ITEM_FORMAT.md).

- Verification passed: full native-data race suite with both item catalogs, about 90,000 bounded fuzz inputs, formatting, vet, HTML lint and build. Built-server temporary-database loopback checks passed native-only catalog status, login, equip/unequip metadata and persistence across restart, the expected warning for converted-only gacha dependencies, and graceful shutdown. Combined-catalog gacha purchase/opening and repair/restart checks also passed. These use protocol test clients; original game-client validation remains pending.

### Continuation: exclusive JSON item catalog

- Runtime item loading now reads only the maintained translated-client JSON asset. `item_catalog` defaults to `data/item_data.json` and is separate from the directory for other native assets. Missing, corrupt or unsupported JSON stops startup before listeners or database initialization. Runtime code never opens either binary catalog or the export's source metadata path.
- Readable JSON definitions supply gameplay values; complete decoded records remain available for further migration work. The parser validates schema/native versions, header, source-size/hash structure, record indexes/offsets, decoded record sizes and unique nonzero matching IDs before publishing the catalog. Binary decoders remain available to offline conversion and format tests.
- The active catalog contains exactly 7,312 translated-client items. Converted overrides and server-only IDs are absent. All configured starters remain available. The sibling gacha table references absent IDs (including pack 34381), so its existing all-or-nothing validation disables the table and reports a warning. Authored pool tests still verify 24 pools/394 rewards independently; enabling runtime pools requires compatible JSON IDs.

- Verification passed: full native-data race suite, formatting, vet, HTML lint and build. JSON tests cover authoritative readable fields, unusable binary paths, missing/corrupt JSON without fallback, schema/header/hash/position/ID validation and the 7,312-item census. Built-server loopback checks passed exclusive JSON loading with both legacy item paths unusable, login, equipment metadata, repair payments, restart persistence and graceful shutdown. Missing/corrupt JSON also failed inspection/startup despite valid legacy binary catalogs. The maintained asset was unchanged.

### Continuation: complete resource exports

- Added a reproducible offline exporter for all 19 translated-client data files, 18 server-only `Data` files, 4 `listdata` files. The client wins all 17 filename overlaps; the manifest retains selected and overridden source hashes.
- Native NPC, skill, dialogue, quest-mark, scene and both compound tables now have complete decrypted JSON exports. Native routines verify SceneData's six-byte object prefix, field-width wrapping and every byte/word/dword transformation. Unknown field meanings remain explicitly named by offset. The existing Go item exporter preserves the authoritative 7,312-item JSON catalog.
- Plaintext EVE, terrain, WEM and animation containers retain complete records, indexes, gaps and headers. Formula and LBD structures, text encodings, authored JSON/CSV/XML, server-only converted items and all SQLite schema/typed rows are exported without merging server item overrides.
- Native archive footer tracing established the shared 29-byte named index used by terrain, WEM and audio. Audio exports contain all 19,717 Ogg streams and full filename/index metadata. Source page framing and sequences are checked; one missing end-of-stream flag is recorded without modifying the source. Audio payloads are saved locally and ignored by Git. Original account databases are excluded from exports.
- Verification reconstructs all 42 selected source files byte for byte. Tests cover raw cipher fixtures, wrapping, SceneData's prefix, filename padding, archive bounds, terrain order, client precedence and duplicate CSV rows. Native-data Go tests independently compare readable NPC and skill exports with existing parsers. Other runtime asset loaders still await migration to these JSON exports.

### Continuation: SQL asset catalog and database rebuild

- Added `cmd/data-rebuild` and a Go/GORM asset importer. The catalog preserves every exported JSON document, indexed collection records, source/manifest checksums and views for common gameplay resources. Audio metadata remains queryable; audio payloads are excluded.
- Rebuilding creates a separate `var/assets.db`. The explicit `-reset-gameplay` flag also prepares a fresh current gameplay schema. Existing destinations are backed up before publication, failed imports leave current files intact, and ordinary publication failures restore replacements. Role/path/sidecar/open-handle checks guard offline replacement; two database files are not atomically published across an OS crash.
- Tests cover exact document preservation, duplicate IDs, SQL projections, unknown-field queries, audio exclusion without audio files, checksum failure preservation, gameplay reset/backups, unsafe destinations, independent asset rebuilds and rollback after partial publication. Runtime asset-loader migration to SQL remains separate work.

- Applied the authorized reset: `var/assets.db` contains all 42 exported assets and 95,751 indexed records (406,327,296 bytes), with 7,312 item, 4,930 NPC and 953 skill rows. Every stored document matches its export checksum; both databases pass SQLite integrity checks. The previous gameplay database is retained at `var/wonderland.db.backup-20261001T214319Z-1791945608`; the fresh gameplay schema is version 5 with no accounts or characters.
- Corrected the stale local `item_catalog` path from `data/items_data.json` to `data/item_data.json`. Native inspection, affected race tests, formatting, lint and build passed. A built-server smoke started web administration and all three TCP listeners on ephemeral loopback ports, verified the authenticated empty account list and shut down gracefully.


## SQL runtime assets

- Server startup and inspection now load all gameplay catalogs through GORM from
  the read-only assets database. `assets_database` replaces `item_catalog` and
  `data_directory`; gameplay storage remains separate.
- Indexed record JSON is authoritative for native projections, item definitions,
  array settings and CSV rows. EVE and animation plaintext records are assembled
  from SQL in memory and checked by the existing format parsers. Authored recipe
  defaults are imported rather than selected from the reference loader's table.
- Loading creates a consistent startup snapshot; missing required assets or
  invalid SQL content stop startup without a file fallback. SQL edits require a
  restart; offline export changes require rebuilding the assets database.
- Tests verify read-only/WAL behavior, edits and deletions, source-independent
  startup, invalid catalogs and native projection parity. Installed gameplay
  tests accept `WONDERLAND_TEST_ASSETS_DB` to exercise the runtime SQL source.


## Alternate AC40 alchemy

- Ported the legacy AC40 handler's two-slot, first symmetric recipe lookup from
  the SQL-loaded catalog. The exact request is `40, subcommand, slot1, slot2`;
  the status response is `40, subcommand, success, outputID:u16`. Subcommands are
  echoed, as in the reference, whose handler defines no distinct operations.
  Recipe rates are not rolled. A missing recipe returns failure; AC23's existing
  greater-ID fallback is preserved separately.
- Both removals and the normal fresh-item grant commit together before AC23:9,
  additive AC23:5 and the AC40 success status. Compatible stacks can accept the
  result in a full bag; damaged/forged metadata is preserved. Missing catalogs,
  invalid slots, same-slot requests and unavailable space consume nothing.
  Complete requests are required instead of ignoring extra operands.
- Existing loading, battle and trade gates apply. Events, minigames, storm/beach
  cutscenes and use of a mounted vehicle slot cannot synthesize. A failed save
  publishes no receipts; a failed receipt retains the already committed result.
  Empty-slot replay fails without extra output. No schema change is required.
- Independent golden packets and regressions cover SQL recipes, symmetric
  precedence, full bags and stack compatibility, persistence, failed saves and
  receipts, ownership gates and concurrent replay. Original-client AC40
  validation and advanced alchemy remain pending.
- Validation passes: the full SQL-backed race suite, formatting, vet, HTML lint
  and build. A built-server TCP smoke with a temporary gameplay database verifies
  the real SQL recipe 37206 + 37207 → 20031, native inventory/status receipts,
  leftover damage/metadata, replay failure and persistence after restarting.
  Both temporary server runs shut down gracefully.


## Mall forging

- Ported AC75:3 and MallForgingManager's SQL-configured Strong Scroll families
  and point eligibility. Scroll upgrades consume tier-dependent costs across
  bag stacks, replace equipment in its original slot, preserve damage and reset
  other metadata. Terminal tiers and unavailable/wrong-slot successors refuse
  changes. The imported catalog contains 291 family items and 1,940 point IDs.
- Point forging costs three ordinary IM points per attempt with a cryptographic
  50% roll. Success increases native metadata byte 18, capped at 200; failed
  rolls still charge the attempt. Equipment slots 1–5 need an eligible stat.
  Bonus funds are retained. Account payment and character state use the existing
  GORM mall transaction and the authoritative balance, with world lock ordering.
- Native AC75:6 progress, roll failure, insufficient funds/scrolls, upgrade
  success and rejection replies clear the pending selection. Scroll receipts
  remove the old client ID before its additive replacement. Point receipts use
  metadata-only success packets and refresh balances. All receipts follow commit.
- Requests are exactly one slot byte after AC75:3. Loading, battle, trade,
  interactions/cutscenes and mounted source items block forging. Tests cover
  raw packets, split costs, durability/metadata, max progress, insufficient
  funds, invalid catalogs, rollback, receipt failure and concurrent attempts.
  General item locks and original-client validation remain pending.
- Updated source-inventory mappings for AC75 and MallForgingManager, and corrected
  the earlier AC40 and AlchemyManager entries. Source hashes and revision are
  preserved; all four source hashes match after CRLF normalization.
- Validation passes: full SQL-backed race suite, formatting, vet, HTML lint and
  build. A built-server TCP smoke with a temporary gameplay database verifies
  Sky Sword → Sky Sword+1, native scroll receipts, insufficient-scroll reset,
  a point attempt and authoritative balance, metadata, persistence across restart
  and graceful shutdown. Live player storage is untouched.


## Mall arcade category launcher

- Ported the remaining AC75:4 request: exactly one category byte, native
  `57,1,category,0,0,0` start and ordinary/bonus balance refresh. The category is
  forwarded unchanged, matching the legacy handler, without an invented game
  whitelist or seeded difficulty. Opening a category spends no currency and
  saves no character state. Balance-read failures publish no partial start.
- Existing map loading, battle and trade gates apply; quest interactions,
  minigames and storm/beach cutscenes cannot be replaced by this launcher.
  It installs no outcome callback, matching AC75/AC57: standalone results create
  no quest progress, items or tickets. Native EVE outcome routing is preserved.
- Tests cover independent golden start/balance packets, category boundaries,
  repeated launch, unsolicited win/loss reports, persistence, fresh balances,
  failed reads, malformed operands, ownership and peer privacy. Updated the
  AC75 source inventory mapping with its verified source hash preserved.
  All defined AC75 request handlers are now mapped; ticket/prize exchange and
  original-client validation remain pending.
- Full SQL-backed race tests, formatting, vet, HTML lint and build pass. A
  built-server TCP smoke with SQL
  assets and a temporary gameplay database verifies native launches, point
  refreshes, standalone win/loss reports, unchanged character/currency state,
  repeated categories, restart and graceful shutdown.

## Resume checkpoint — public travel and GM destinations

- Ported public AC02 `:carnie`/`/carnie` and `:unride`/`:dismount` slash/colon aliases. Vehicle dismounts persist before native owner/peer replies; raft counts, wear and companion pet mounts remain intact. Map 10036 retains the legacy shore destination (1038, 2235).
- Carnie chat and AC5 menu travel share the same return point. Missing destination maps and failed saves preserve the previous memo. Repeated visits retain the original departure location, and the existing exit portal returns there.
- Ported GM `:town`/`/town` using all 16 exact legacy aliases/coordinates, and `:summonall`/`/summonall`. GM rights continue to require an administrator grant. Summons use a stable ordered snapshot because teleporting removes sessions from world publication. Each recipient commits independently; one failed recipient is logged and disconnected for a fresh snapshot, while later recipients continue.
- Travel commands reject loading, battle, event/minigame and cutscene ownership. Public commands also reject active trades; GM teleports retain the existing trade cancellation behavior. This deliberately closes the legacy chat-command bypass of interaction guards. No actual-client validation is claimed.
- Independent native packet bytes, all town aliases, durable failures, peer replication, public/GM permissions, Carnie return/exit behavior, failed recipients and concurrent public travel against GM summons are covered in `travel_commands_test.go`.

Source audit: AC02, AC05 and GmManager source hashes match the pinned inventory. An existing AC39 recorded-hash mismatch was confirmed against both the pinned revision and current sibling file; its inventory note records the actual normalized hash without changing the original hash or pending status.


## Resume checkpoint — GM progression commands

- Ported AC02's level/lvl, points/sp/statpoint/statpoints, stat/stats, exp and individual skill commands, with colon and slash aliases. Existing administrator grants control access. Loading, combat, event/minigame, cutscene and trade ownership prevent changes to active snapshots.
- Level changes use Equip.SetLevel's threshold EXP and grant three points per positive requested level difference. Lowering a level retains points. The legacy requested-level-200 behavior is preserved: displayed non-reborn level stays 199, while points follow the requested target. Point grants saturate rather than wrapping.
- Attribute overrides accept STR, CON, INT, WIS, AGI base values, preserve avatar bonuses and refill equipped HP/SP. Unlike the legacy attribute command, Go saves immediately. Direct EXP assignment grants no points, clamps negative values to zero and rejects values beyond the current uint32 character schema. Level and EXP commands explicitly synchronize total EXP alongside native stat packets; downward EXP changes clamp current vitals to the new maxima. Rebirth remains pending.
- Individual skill overrides require an ID from the SQL-loaded asset catalog and a grade from 1 through 10 (default 1). They reset proficiency, including repeated or lower grades, preserve the native avatar stunt alias and send unlock/grade/proficiency replies. A grade increase additionally sends AC8:1 stat 110. Quest learning still preserves existing training. Bulk skill commands remain pending.
- Commands persist a cloned character before publishing success packets. Tests cover independent packet bytes, aliases, threshold EXP, avatar bonuses, requested-level limits, point saturation, skill overrides, default grades, persistence, failed saves/replies, permissions and interaction ownership. Original-client validation remains pending.
- Source mappings for AC02, Equip and SkillManager include the new implementation and regression tests. Their existing pinned source hashes are preserved.


## Investigation — original-game burst EXP

The current C# and Go victory rewards are fixed by monster level and do not
reproduce damage-dependent bursting. Shrink/Hot Fire now have initial non-damaging compatibility handlers. Full native
effect parity, original combo eligibility, zero-speed equipment arithmetic and
damage/support reward attribution still need separate work. See [EXP_BURSTING.md](EXP_BURSTING.md) for source evidence,
historical observations, hypotheses and the measurements needed to establish a
compatible formula. This investigation does not change gameplay or claim the
commercial server formula has been recovered.


## Resume checkpoint — Goddess skill effects

- Shrink (SQL skill ID 15188) is now a separate enemy debuff action, rather than a damaging attack. It halves effective ATK, MATK, DEF and MDEF without changing HP or the underlying battle stats. Physical attacks, staff attacks, magic skills, monster attacks and confused-monster attacks read the effective stats. Recasts refresh duration without compounding reductions; expiry restores the exact original values.
- Hot Fire Attack (SQL skill ID 15189) now buffs a living ally instead of dealing fallback damage. It triples outgoing player/pet attack damage. When the weaker generic Fiery/HotBlooded status is also present, Hot Fire takes precedence rather than multiplying the buffs together. Existing Fiery behavior remains otherwise unchanged.
- Dispatch uses explicit IDs from the SQL-loaded catalog, so translated names do not change the effect. Debuffs retain normal speed ordering, execute separately from offensive combos, and affect subsequent actions in the same round. Wrong-side, missing or defeated targets are rejected without redirection or SP/proficiency consumption. Unable or unlearned casters cannot apply effects.
- Compatibility policy: Shrink uses a 50% combat-stat reduction and four rounds including the cast; Hot Fire uses threefold damage and three rounds including the cast. Native skill records contain byte 51 values 3 and 2 respectively, interpreted here as subsequent rounds. That interpretation and the exact magnitude are not a complete semantic decompilation. Native hit-chance/resistance rules, precise interaction precedence and persistent visual status rendering remain pending. Successful casts follow existing deterministic status handling and the existing non-HP support animation packet layout; original-client validation is still required.
- Successful casts participate in the existing save-before-success skill proficiency path. Battle effects are transient and never overwrite durable base attributes. Independent packet tests cover non-damaging effects, SP costs, refresh/expiry, damage paths, buff precedence, wrong/dead targets, insufficient SP, unlearned/sealed casters and combo separation. Server tests use the actual SQL Goddess skill records and verify dispatch, output and saved proficiency.
- The C# source has the same missing-name-classification gap; these are explicit Go corrections, not a claim of complete commercial-server parity. Source mappings and pinned hashes for PvEBattleManager and SkillManager are updated/verified. Damage-dependent burst EXP remains outside this change.


## Resume checkpoint — generic ability effects

- Replaced the preceding Goddess ID checks and numerical status checks with `Skill.Effects` definitions and a shared battle modifier engine. Definitions specify target side/self, rounds, modifiers and stacking group. Abilities may apply multiple effects atomically after validating every target; they spend SP and earn proficiency once, with one non-HP animation per affected target.
- ATK, MATK, DEF, MDEF, SPD, outgoing damage and incoming damage aggregate all active modifiers. Independent percentages/flats add. Named groups keep their strongest positive and negative contributions per stat/operation; weaker members resume after stronger members expire. Base stats stay unchanged. Recasts refresh the source/index without compounding reductions. Aggregated values clamp before native conversion, including combo damage before critical calculation.
- SQL/native skill projections expose decoded fields 51/52 and translate recognized effect-layer/code pairs into definitions. These definitions cover reduction, outgoing-damage amplification, speed and protection classes without matching skill IDs or translated names. Authored SQL effect definitions override compatibility projections and are validated during startup. Unresolved effect classes and non-numerical control statuses retain their existing partial coverage.
- Compatibility strength/duration limits from the Goddess checkpoint remain. Speed effects now expire instead of permanently changing base SPD. Protection uses the same incoming-damage modifier for player and monster attacks, replacing the former inconsistent 50%/60% reductions. Defend remains a separate one-turn action. Native effect hit-chance/resistance and persistent visual status remain pending; no burst reward policy is added.
- Generic regressions cover arbitrary IDs, combined buffs/debuffs, stacking groups, refresh/expiry, modifier order, clamping, speed ordering, multiple effect targets and monster-side ability targeting. Existing Goddess and pet tests now exercise this generic engine. SQL tests cover native projection, authored definitions, startup rejection of invalid effects and independent parity against translated client Skill.dat. Source inventory mappings and development conventions are updated.


## Resume checkpoint — unified control and periodic effects

- Removed the separate battle status enum/list and condition-name classifiers. Frozen, Sleep, Sealed, Confused, Poisoned, Paralyzed and Vanish now project into the same catalog-defined effect records as numerical buffs/debuffs. Runtime capability checks use effect properties rather than condition names or ability IDs.
- Blocking effects aggregate across all active records. Damage removes only `break_on_damage` effects, so waking does not clear unrelated restrictions. Periodic effects combine max-HP percentage, flat damage and a minimum, follow stacking groups, tick at round start and use the existing HP sync/knockout path. Periodic damage can wake a damage-sensitive restriction. This makes damage-triggered removal consistent across attacks and ticks.
- Attack-redirection effects specify a percentage. The battle executor can select another living ally for player/pet attacks as well as monster attacks. Targets remain on the selected side; redirected attacks do not retarget to their caster after an ally dies. Blocking, redirection and periodic damage all expire through the shared effect lifecycle.
- Abilities may define `on_hit` effects, applied to living recipients after attack damage. Pure condition casts no longer deal fallback damage or join offensive combos. Native mappings use effect-layer/code pairs and duration field 51; healing/cure layers sharing a code are excluded. Mixed cast/hit effect lists are rejected at catalog load until combined action semantics are supported.
- Vanish was initially a timed presentation effect using the existing non-HP skill animation. Its verified physical/AOE protection is now implemented in the continuation below; persistent visual behavior remains pending. Native secondary attack effects are not inferred from elemental skill names. Exact duration, hit-chance/resistance, dispels and complete commercial-server parity remain pending.
- Regression tests cover arbitrary control skills, overlapping restrictions, waking, independent/grouped DOT, copied effect definitions, periodic knockout, on-hit order, redirection, visual expiry and native class projection. An actual SQL Freezing Spell cast is non-damaging, suppresses the enemy turn and persists proficiency. Development conventions and source mappings are updated; no burst EXP behavior is added.


### Resume checkpoint — Vanish attack protection

- User investigation established that Vanish forces physical and AOE attacks to miss the buffed character; the translated native description corroborates physical and AOE magic immunity. Fire-to-Wind remains ×1.5 and all other elemental multipliers remain unchanged.
- Native buff layer 19/code 53 now projects `miss_physical_attacks` and `miss_area_attacks` along with presentation and duration. Combat reads these properties from every active effect without ability IDs/names. Protection covers player/pet attacks, combos, monster basic attacks and redirected attacks; single-target magic, support casts and periodic damage are unaffected.
- Misses consume SP and retain proficiency, emit a zero-damage HP record with target result 0 (native `FUN_00398dac`/`FUN_003990ac` parser and effect application), and skip damage-sensitive removal and on-hit effects. The target result is separate from the final critical digit mode. The original server's unused `UnknownType.miss = 2` enum does not describe this native field. Original-client MISS rendering still needs a live check.
- The SQL/native skill projections retain pattern bytes 109–112. Codes 2–8 classify multi-target attacks; 0/1 remain single. Current grade bands 1–3, 4–6, 7–9 and 10 are an explicit compatibility policy, with an authored SQL `area_attack` override. Exact native thresholds and grid shapes remain unresolved. The current attack executor still resolves one target; complete AOE target expansion is outside this correction.
- Regression tests cover arbitrary protection abilities, physical/basic/staff attacks, single-target versus area magic, player/pet grades, mixed combos, wake-on-damage preservation, monster and redirected monster strikes, expiry, native projection, independent miss bytes and an installed-SQL Vanish cast with durable proficiency.


### Resume checkpoint — Stone Wall physical protection

- Stone Wall's native protection layer 4/code 101 now projects an ally-targeted `miss_physical_attacks` effect. Casting uses the generic effect executor; it no longer falls through to offensive damage. Runtime behavior has no Stone Wall skill-ID or name checks.
- Basic and physical attacks miss the protected fighter; single-target and area magical attacks remain effective. Physical misses use the native zero-result packet and do not apply damage or on-hit effects. SP cost (110 in the installed SQL catalog) and durable proficiency use the existing cast path.
- The current duration policy reads field 51 plus the casting round: native value 3 gives four rounds including the cast. Refresh and expiry reuse the shared active-effect lifecycle. Exact commercial duration and persistent client visuals remain under verification.
- Tests use an arbitrary skill ID/name with the native layer/code, cast on a selected ally, reject enemy targets, verify unchanged HP on cast, physical misses, allowed magic, expiry and independent support packet bytes. An installed-SQL Stone Wall cast protects against the monster's immediate physical turn and saves proficiency.


### Resume checkpoint — Water Shield magical damage reduction

- Water Shield's native protection layer 4/code 105 now projects `magical_damage_taken: -50%` instead of the generic incoming damage modifier. It halves single-target and area magical attack damage; physical/basic/staff/monster attacks and periodic ticks remain unaffected. The native description and user direction establish its magical-only scope.
- Combat aggregates generic `damage_taken` and applicable `magical_damage_taken` contributions before rounding, preserving independent additions and named-group strongest-contribution behavior. Native layer 19/code 52 still supplies generic protection. There are no Water Shield ID/name checks in combat.
- SP cost (35 in the installed SQL catalog), ally targeting, proficiency, refresh and current five-round duration policy remain unchanged. Exact commercial reduction magnitude/duration and persistent client visuals still require verification.
- Regression tests cover arbitrary-ID casting, single/area physical and magical attacks, native projection, generic shield scope, mixed generic/type-specific stacking, single rounding, periodic damage and expiry. An installed-SQL cast receives full physical retaliation and saves proficiency; Stone Wall coverage remains in place.


### Full-party speed chains

PvE offensive combos now support all players and pets instead of pairs. Adjacent
single-target attackers against the same enemy join when their effective SPD gap
is at most 99. The collected round retains its speed snapshot and support/enemy
turn barriers. Execution rechecks bridge availability and redirected targets.
One animation carries every participant's hit record, with individual SP costs
and durable skill progression. Damage uses the shared combo multiplier; rewards retain the existing
policy. See [COMBOS.md](COMBOS.md) for saved reference evidence, examples and
the user-requested linear level probability and remaining native-client parity.


Party combos now roll once per eligible chain using the whole present player/pet
roster's fractional average level against the actual target. The user-requested
linear curve yields 0% at −25 levels, 50% at equal levels and 100% at +25, clamped
at its endpoints. Failed chains execute as singles without a combo bonus or
subset rerolls. Support casters and knocked-out fighters contribute to the mean;
absent fighters do not. Reborn players and pets add 99 effective levels before
averaging; target levels use the same rule. Endpoint,
fractional-average, roster and failed-damage regressions cover this policy.


Rebirth combo continuation adds a saved optional character `reborn` flag and
uses the existing pet flag. Combo effective levels add 99 without altering
stored/displayed levels or EXP, and use integer arithmetic above the native
byte range. Mixed-party averaging, target rebirth, high-level overflow and
durable party battle saves are tested. Full character rebirth progression, jobs
and roster presentation remain pending.


### Shield Defense stat protection

Shield Defense (native buff layer 19/code 52) increases effective DEF and MDEF
by 10% through two generic modifiers. The SQL skill's 15 SP cost and four-round
duration remain in effect. Base attributes stay unchanged; refresh and expiry
follow the shared effect lifecycle. Water Shield still reduces magical damage;
authored physical/generic damage protection remains available through effects.
Native projection, attack-category/stat/expiry and installed-SQL cast/proficiency
tests cover the current policy. The source description remains "Increases target's
defence"; exported assets retain the original text.

### Skill effect JSON, SQL and server chat

Skill JSON adds reusable `effect_refs`, and `skill_effects.json` exports named
effect definitions from fresh native skill records with the compatibility policy.
Source-only export, manifest provenance, reconstruction verification and JSON
schemas cover the new format. SQL imports/indexes these definitions and resolves
references using authoritative rows; invalid references/definitions fail startup.
Inline effects and explicit empty lists retain their override semantics. The local
asset database has been rebuilt with backups and gameplay data preserved.

Shield Defense now increases DEF and MDEF by 10% through the generic modifier
engine, with its existing 15 SP cost, duration, refresh/expiry and proficiency.
This supersedes the earlier physical damage reduction policy. Original native
description and bytes remain unchanged.

Server chat now delivers local/world messages, supports public party and exact
whisper commands, honors channel preferences, isolates failed recipients and
validates message limits/control bytes. Local map framing remains native AC2:2;
additional channels use verified AC2:16 notices while dedicated channel receive
layouts and guild membership remain unported. Client changes are outside this
TODO's confirmed server-only scope. See DEVELOPMENT.md for commands and limits.


### Data-driven native effect conversion

The former ability-specific constants and switch in `skill_effects.go` have been
removed. `internal/assets/native_effect_rules.json` owns conversion mappings and
tuning; generic code validates and clones templates and applies native duration
metadata. The offline exporter accepts an alternate JSON policy through `-rules`.
Runtime SQL resolution requires explicit references or inline effects and never
uses native-code compatibility inference. Older imports missing this metadata
must regenerate skills/effects and rebuild the assets database. Tests cover new
JSON-only mappings, tuning, duration bounds, independent copies, malformed and
duplicate rules, and rejection of missing SQL effect metadata.


### Pet rebirth ascension (AC69)

AC69:1 accepts an owned party pet's session client slot. Eligible pets require
level 100+ and amity 100+, and must not already be reborn. Ascension sets reborn,
level 1, zero level-progress EXP and amity 100, and adds 50 unallocated stat points.
Base attributes, learned skill progress, equipment, job and battle-pet selection
remain intact. Rebirth metadata participates in the existing +99 combo effective
level rule; the visible level remains 1.

Vitals are recalculated with SQL equipment definitions and refilled before saving,
correcting the reference handler's save-before-normalize order. Success AC69:1,
AC15:8 roster and AC8:2 progression packets are emitted only after durable commit.
Unknown slots, repeated ascension and ineligible pets receive a failure result;
malformed requests and unsupported subcommands cannot mutate state. Stat-point
overflow is rejected without wrapping. Loading, battle, trade, event, storm and
beach interactions cannot trigger ascension.

Tests verify independently authored reply/stat bytes, client slots differing from
persistent slots, companion aliases, saved/reconnected roster parity, preserved
progress, eligibility boundaries, repeated requests, blocked interactions and
failed-save isolation. Source hashes were checked after CRLF normalization and
inventory mappings updated. Model changes, rebirth quests, potential training and
actual-client AC69 capture validation remain separate work.


### Companion stat allocation (AC68)

AC68:2 accepts a client pet slot and one of five selectors (STR, CON, INT, WIS,
AGI) and spends one unallocated stat point. It uses the shared pet allocation
budget, so AC68 and existing AC8 commands cannot overspend each other's points.
Derived HP/SP maxima are normalized while retaining current vitals. State saves
before any AC8:2 progression or AC68 success receipt, correcting the reference's
publish-before-save order.

Unknown slots/selectors, insufficient points and full uint16 attributes receive
a failure result without mutation; full attributes do not wrap to zero. Malformed
or unknown subcommands are rejected. World loading, battle, trade, event, storm
and beach ownership gates apply. Reconnect retains base attributes, current and
maximum vitals, equipment and remaining points. Independently authored bytes,
all selectors, client/persistent slot differences, companion aliases, shared
budgets, failed saves and blocked interactions are covered by tests.

AC68:1 returns unavailable without changing state. The source potential branch
increments freely and never checks or consumes a pill; its shared progression
packet still emits level-minus-one for stat 37. Potential-item consumption,
potential's combat effect and native client representation need verification
before implementing that branch. Do not infer those semantics from its label.
Source hashes were checked after CRLF normalization and source inventory updated.


### Battle-pet feeding by food ID (AC67)

AC67:1 accepts a WORD food item ID and selects the first registered party pet
marked for battle. It scans bag slots in order and consumes one item from the
first usable matching stack through the existing amity-feeding executor. SQL
status-64 item values above 100 supply the additive gain; amity caps at 100.
Pet aliases and per-login client slots retain the existing roster semantics.

Food consumption and amity save together before inventory removal, pet amity,
item-use completion, banner and the native AC67 receipt. Missing battle pets,
food or valid effects and capped amity receive failure without consuming an item.
Malformed requests and unknown subcommands cannot mutate state. Loading, battle,
trade, event, storm and beach gates apply. Shared feeding now rejects out-of-range
bag slots, zero counts and empty stacks before indexing or saving.

Tests verify authored item-ID/reply/stat bytes, exact-stack consumption and
metadata, untouched other pets/stacks, persistent state, cap/repeat behavior,
invalid target/food/effect cases, interaction gates and failed-save isolation.
Source hashes were checked after CRLF normalization and inventory mappings updated.
Item-lock enforcement and actual-client AC67 capture validation remain pending.


### Native text mail (AC14:1)

The native request carries a type byte, uint32 recipient and raw text. Delivery
uses the sender ID, little-endian OLE Automation double timestamp and a
byte-length-prefixed string; AC14:9 acknowledges accepted mail. Text mail uses
schema v6's `text_mail` table through GORM. Both characters and sender ownership
are verified within the insertion transaction. The sender receives success only
after persistence. Missing recipients receive feedback without a success receipt.

Messages preserve native encoded bytes, including Big5 text, without the
reference serializer's lossy ASCII conversion. Content allows 1–255 bytes,
including line breaks and tabs; trailing NUL terminators are stripped, embedded
NULs/control bytes and oversized content are rejected before writes. The source
ignores the type byte; Go retains it without inventing attachment behavior.
Timestamps use UTC consistently across replay and server timezone changes.

Online recipients receive mail across maps after acknowledgment of their map.
Offline and loading recipients receive queued messages after their next map
snapshot, including a warp snapshot. Delivery runs in ordered batches and marks
each message only after its socket write succeeds. Recipient write failures close
the recipient and leave the message queued without failing the sender. The native
packet has no delivery receipt: a crash after the socket write and before the
marker can repeat a message. Deleting either character cascades related messages
so reused character IDs cannot inherit old correspondence.

Tests cover raw native packet bytes and timestamp, non-ASCII text, persistence
across reopen, ordered batches, cross-map delivery, offline/warp/duplicate map
acknowledgments, failed recipient and storage writes, identity/recipient validation,
malformed requests, account deletion, and v5 upgrade preserving player state and
friendships. Reference hashes were verified and source inventory updated. Native
client mail-screen acceptance, short name-fallback requests, parcels/attachments,
mailbox history/read/delete controls and administration remain pending.


Verification: the imported-asset suite passed with `go test -race ./...`, as did
focused mail/schema race tests and `make fmt-check lint build`. These checks use
temporary gameplay databases and do not establish actual-client mail acceptance.


### Native social contact commands (AC10)

AC10:1 immediately adds two same-map published characters as contacts. AC10:2
accepts or declines a contact addition; the reference does not require an
invitation for this operation. The existing AC14 invitation flow keeps its
pending-request check. Both paths share the durable symmetric friendship store
and the 49-contact limit on either side. Offline/loading, remote-map, self and
missing targets do not create contacts. Successful creation clears obsolete
AC14 invitations only after persistence.

New contacts receive native AC14:9/7 records. AC10:1 also emits its verified
AC10:1 ID/online/name status to both participants; repeated additions emit only
this status. AC10:2 acceptance emits the new-contact records without an additional
status packet, matching the source. Declines do not change the relationship.
AC10:3 and AC10:6 publish the existing AC14:11/5 authoritative friend snapshots.
Go omits the reference's redundant in-memory list before its database snapshot.
Status queries accept a bare command or an ignored uint32-shaped operand; the
reference does not read that operand. Extra malformed fields are rejected.

AC10:4 removes the shared relationship and returns the native AC10:4 receipt
alongside the existing bilateral AC14 removal/list updates. Failed persistence
never publishes success. A recipient's failed write closes that recipient without
failing the actor, including the shared AC14 removal refresh. AC10 registers its
world policy beside its handler and retains loading, battle and minigame gates.
Inbound social operations and outbound presence both use opcode 10, with separate
names for their different meanings.

Tests cover authored status bytes, symmetric persistence, add/reply/decline,
repeat behavior, list/status and removal, AC14 interoperability and stale request
cleanup, both-side capacity enforcement, malformed payloads, world gates, rejected targets and persistence/socket
failures. Source hashes were verified after CRLF normalization and the inventory
updated. Actual-client AC10 capture comparison remains pending.


Verification: the full imported-asset suite passed with `go test -race ./...`.
Focused social/friend/registry race tests, including full-recipient isolation,
and `make fmt-check lint build` also passed. Tests use temporary gameplay stores;
no running server or production gameplay database was changed.


### Persistent bank currency (AC45)

AC45:8 reports bank and carried gold as two uint32 values. AC45:9 deposits and
AC45:10 withdraws a uint32 amount. Character `bank_gold` persists in the existing
SQL-backed JSON state; old characters default to zero without a schema upgrade.
`DepositGoldToBank` and `WithdrawGoldFromBank` move currency without changing its
total. Deposits reject insufficient carried gold and uint32 bank overflow.
Withdrawals reject insufficient bank funds and amounts that cross the existing
999,999 carrying limit. The bank's upper limit is the native unsigned wire range;
no lower economic cap is established by the reference.

Both balances commit together before publishing success. Clients receive the
existing authoritative AC26:4 wallet balance followed by native AC45:9/10 bank
and wallet values. This replaces the reference's pre-save debit delta and avoids
its lost-gold/overflow paths. Rejected transactions leave state unchanged and
return feedback plus AC45:8 current balances. Receipt write failures retain the
durable transaction; failed saves emit no receipts and leave session state intact.

The handler registers its world/trade policy with the shared command registry.
Loading, battle, minigame, trade and scripted event/storm/beach ownership gates
apply. PIN and character-transfer source handlers only emit success without
performing either operation; Go reports them unavailable. The recorded EVE
interpreter has no money-bank activation mapping, so Go does not invent a service
operand or reuse the unsafe legacy AC29 item-storage handler. Verified native
ATM activation, PIN authentication, character transfers and actual-client capture
acceptance remain pending.

Tests cover independent native balance/receipt bytes, successful deposit and
withdrawal, currency conservation, exact limits/overflow, zero and insufficient
amounts, truncated/unknown requests, unavailable placeholders, all interaction
gates, failed saves and receipt writes, SQL reload and database reopen, legacy
JSON defaults and character isolation. Source hashes were verified after CRLF
normalization and source inventory updated.


Verification: the full imported-asset suite passed with `go test -race ./...`.
Focused banking and registry race tests, formatting/lint checks, server build and
client build also passed. Verification used temporary gameplay stores; native
ATM activation and actual-client AC45 acceptance remain pending.


### Native mall checkout balance handshake (AC34)

AC34:1 carries a mode byte (0 or 1) before the client submits its cart. Go now
queries one authoritative SQL account balance snapshot and sends AC75:3 ordinary
points, AC75:9 bonus points and AC35:4 checkout resume, in that order. The resume
contains ordinary points, a zero second DWORD and eight reserved zero bytes.
Both accepted modes use ordinary points, preserving the source behavior; separate
mode meanings remain unresolved and have numeric fallback names. All three
packets use the existing native signed point range.

Checkout sends no catalog, settings or mall status refresh: those can clear the
client's pending cart. The read-only handshake refreshes the session cache without
spending points, granting inventory or changing character state. It registers
beside the mall handlers with normal world gates, and does not bypass loading,
battle or minigame ownership. A query during trade is allowed because it does not
change offers or inventories; the later purchase retains its existing trade gate.
Failed balance reads publish no checkout success or cache changes. Socket failures
cannot charge the account or grant items.

Independent raw fixtures cover all three packets, both modes, stale-cache/current
SQL behavior, unchanged balances and inventory, the subsequent cart purchase,
malformed/unknown requests, loading/battle/minigame/trade behavior, read and socket
failures, and consistent native wire bounds. Source hashes were verified and
source inventory updated. Actual-client pending-cart/resume capture acceptance
remains pending.


Verification: the full imported-asset suite passed with `go test -race ./...`.
Focused checkout/registry race tests and `make fmt-check lint build` also passed.
Tests used temporary gameplay databases; actual-client checkout acceptance
remains pending.


## Resume checkpoint — usable SQL gacha pools

- Runtime gacha now validates the complete indexed SQL table before selecting pools compatible with the startup item catalog. The 20 complete translated-client pools remain available; pools 34381–34384 reference missing definitions and are disabled individually with ordered, deduplicated missing-item diagnostics. No reward is dropped or reweighted. Structural errors, duplicate pack IDs, withdrawn Lucky Pack configuration and invalid weights/quantities fail startup.
- Pack membership is authored in SQL. New valid item IDs work through AC91 preview, AC23:75/128/96 opening and mall filtering without Go ID additions. Native IDs remain fallback recognition for unavailable legacy packs. Disabled configured IDs retain their recognition so they cannot be advertised or consumed.
- The asset importer adds `asset_gacha_packs` and `asset_gacha_rewards` views with pack/reward ordinals, reward quantities and integer weights. Runtime authority remains `asset_records` under `gacha_packs.json/value`; retained documents and source files are provenance only. Rebuild the asset database to add the convenience views; existing indexed catalogs gain compatible pool loading on server restart without a rebuild.
- Existing atomic consumption/reward persistence, cryptographic draws, fresh reward metadata and interaction gates remain in use. Source probabilities are authored custom weights rather than recovered official drop rates. Item locks and actual native-client validation remain pending.
- Regression coverage includes all 10,000 weighted intervals, malformed incompatible pools, SQL edits/reload snapshots, ordered SQL views, disabled custom-pack previews/mall filtering and all three custom-pack opening paths with durable inventory checks. Source hashes for GachaManager, AC23 and AC91 match the inventory after CRLF normalization.
- Validation passed: complete native-data race suite, additional custom-pack mall purchase/opening regression, formatting, lint, server build and client build. Existing asset databases load the compatible pools after restarting with the new binary; adding the convenience SQL views requires an asset database rebuild. No running server was restarted or gameplay database reset.


## Resume checkpoint — daily Lucky Draw

- AC104:1 now grants three draws per character per UTC calendar day. This is the user's requested rule; the C# handler had no enforcement or reset. `game.LuckyDrawState` persists date/usage in existing character JSON; older states default to three available draws. A saved future date prevents clock rollback from replenishing draws; missed days do not accumulate credits.
- The store transaction reads current durable state, checks allowance and grants fresh-metadata inventory with usage in the same save. Concurrent requests, reconnects and restarts cannot restore spent draws. Full inventory, missing configuration, invalid requests and failed saves consume nothing. Receipt failure retains the committed reward and usage.
- Fourteen outcomes follow the saved 2013-08-09 線上抽獎 announcement in `docs/References`. Poems and Ghost Festival vouchers have separate ×1/×3/×5 outcomes. All rows initially have weight 1 (1/14 probability), an authored policy rather than recovered official odds. Compatibility mappings are HP/SP light packs 35114/35115, poems 30436, vouchers 30437, statue 34087, holy EXP 34147, chocolate 34085, potential pill 34269, chaos crystal 61062 and EXP capsule 34136. The translated catalog names statue 34087 “Mud Statue”; the reference calls it 金菩薩像. Keep these mappings reviewable in the policy.
- `internal/assets/lucky_draw_rules.json` is offline compatibility policy. The exporter validates it against fresh sibling Item.dat, emits derived `data/lucky_draw.json` and updates manifest provenance. Fresh full exports include it automatically. Verification checks source/policy hashes, native names and every reward field. Saved exports are never original inputs. Runtime reads only indexed `lucky_draw.json/value` SQL rows; `asset_lucky_draw_rewards` exposes their ordered projection.
- The original result/allowance assumptions in this checkpoint were superseded by the native-client correction below. Catalog and result packets now carry cumulative usage. Login synchronizes before world-ready, warps refresh after map acknowledgment, and a cancellable worker refreshes ready characters at midnight UTC. Battle, trade, loading, event, minigame and cutscene gates remain enforced. Results stay private; no global announcement was inferred from the source's actor-only send.
- Display slots 1–14 follow saved list order. The native catalog explicitly supplies these positions and reward quantities; the announcement supplies the curated reward list. Live-client visual verification is recorded in the correction below. The Go client's Lucky Draw interface is a separate client migration task.
- Validation passed: complete native-data race suite, focused Lucky Draw/login/registry race regressions, weighted SQL view tests, source-only regeneration into an empty temporary directory, full source reconstruction verification, formatting, lint, server build and client build. The assets database was rebuilt with a backup; gameplay state was preserved. No running server was restarted.

## Resume checkpoint — periodic character autosave

- Ported `WorldServer.AutoSaveLoop`'s one-second cadence. Active characters and characters loading a warp participate; initial map loading remains covered by immediate gameplay commits. A final bounded checkpoint runs on disconnect, including shutdown.
- Retain an independent cloned baseline per character session. Store loads the owned durable character inside a GORM transaction: already durable or unchanged snapshots require no write; pending changes save only when durable state matches the previous baseline. Divergent durable state returns an explicit conflict, preserving newer rewards, balances and daily draw usage. Conflicts retain the previous baseline for investigation/retry; the worker never silently merges character state or resurrects deleted rows.
- Existing mutations still persist before success packets. Account IM/bonus changes already commit transactionally, so the worker never writes cached account balances. Autosave sends no packets, uses a five-second batch deadline, isolates errors per character and stops with server cancellation.
- The reference `Src/Server/WorldServer.cs` matches its recorded normalized SHA256. Store/server regressions cover pending-state persistence, no-op writes, clone normalization, conflicts, ownership/deletion, validation, rollback/retry, cancellation, warp presence, disconnect and concurrent movement.
- Validation passes: full native-data `go test -race ./...` (server package 311.680 s), finalized autosave race regressions, formatting, vet, HTML/JavaScript lint and server build. No gameplay database reset or new schema/configuration is required.

## Resume checkpoint — native water gathering

- Ported `EveGatheringRuntime` for map/event pairs 60001/88, 60003/48, 12268/2 and 60005/12. The first two grant item 60001 (sea water), the latter two item 60002 (fresh water), using SQL item definitions and EVE actions from the startup catalog. Each grant is one item with a 180-second per-character/per-node cooldown.
- Intercept the complete authored branch before ordinary event execution: branch 5 grants the initial reward with its mark, branch 2 repeats it. Validate reward variant, matching timer and optional mark, and reject extra mutations or malformed shapes. Standalone opcode 14 and unrelated resource pools remain unsupported. Native condition 14 now checks durable timer activity; branch selection evaluates all timers against one instant.
- Store rechecks the selected branch, character ownership/map, cooldown and the native one-free-slot prerequisite inside a GORM transaction. Bag, quest mark and UTC expiry persist in the character JSON together, surviving reconnect/restart without a schema migration. Cooldown claims cannot race into duplicate rewards; failed commits retain availability, and failed receipts retain committed rewards/cooldowns.
- Named native timer/content references follow the numeric convention. Character clones copy the timer map independently, and optional timer state preserves old character JSON compatibility. Source inventory hashes for gathering/runtime/interpreter match the recorded revision. Native game-client validation, other resource pools and legacy timer-table/MySQL compatibility remain pending.
- Validation passes: full native-data `go test -race ./...` (server package 328.487 s), final targeted golden-packet/canceled-commit/branch-condition regressions, formatting, vet, HTML/JavaScript lint, server build and nested client build. No database reset, new schema, asset regeneration or configuration change is required.

## Breillat character conversion

- Corrected native event 4's first-match ordering: the ten-talk offer takes priority over its broad greeting condition. The first completed greeting increments mark 50030 once; the tenth opens the offer immediately. Declining or closing preserves the count and allows another offer.
- Acceptance runs the native dialogue, then commits the body/head change and marks 50030/50031 in one character transaction. The transaction checks fresh map, talk count and conversion state. Failed commits publish no conversion; failed packet delivery leaves the committed transformation durable. Login and map-peer appearance use the saved model.
- Existing inventory and equipment remain intact. The reference outfit handler deletes worn equipment, names missing item 21991, and treats item 21009 as shoes although the installed catalog defines a body uniform. Go does not port those destructive outfit operations.
- Native scripts on maps 10002 and 10021–10030 are covered through the SQL assets catalog. AC5:12 model 13 has an independent byte fixture. Other transformation operands remain unsupported, and end-to-end validation with the original game client remains pending.

## Lucky Draw live diagnostics

Run `./bin/wonderland -config config.local.json -debug -log-file var/lucky-draw-diagnostics.jsonl`
and reproduce opening and spinning the original client's Lucky Draw. Debug logs
include decrypted AC104 payload hex (bounded to 64 bytes), request intent,
expected request length, world dispatch gates and durable draw outcomes.
Outgoing AC104 catalogs/results include used counts, catalog sizes and selected
slots. AC35 and credential/chat commands retain metadata only. Tracing does not
change protocol behavior. The live capture led to the native correction below.

## Lucky Draw native-client correction

- The original client sends `68 01` for a spin. Captured requests reached the handler and persisted rewards/usage normally, while the client displayed blank rewards and repeatedly showed two draws remaining. The final result byte had been fixed at one, and no native reward catalog was sent.
- Fresh metadata/disassembly/decompilation of sibling `Wonderland-Client/aLogin.exe` (SHA-256 `a5ecb1deb3f1f363e4efecc3b580a654248bdf31b6e4e483afa1efad7eef016c`) verifies Lottery handlers `FUN_00175cf0` (catalog), `FUN_00175ddc` (result) and `FUN_00175eac` (nested dispatch). The form renderer subtracts its byte at offset `0xfc` from three. Saved older decompilations have different addresses and require version checks.
- The command is removed before nested dispatch. Full outgoing catalog bytes are `68 01 01 USED COUNT [ITEM_ID:u16le QUANTITY:u8]...`; result bytes are `68 01 02 SLOT USED`. Usage is cumulative, capped at three. AC35:12 does not synchronize this Lottery form; existing unrelated AC35 messages are retained.
- Login, acknowledged warp arrival, AC5:7/4 character refresh and online UTC-midnight refresh publish the SQL-authored catalog with effective persisted usage. Successful spins use the result packet's usage byte; no redundant catalog resets the selected icon. Denied exhausted spins resend the catalog. Empty pools disable spins. Reward slots must be sequential in authored SQL order and fit the native twenty-slot display.
- Regression coverage includes independent catalog/result bytes, cumulative usage across three draws, reload/refresh with saved usage, midnight/future-date handling, SQL slot bounds/order and credential-safe bounded diagnostics. No gameplay allowance was reset for testing. The user confirmed the corrected rewards and allowance display, including refresh/reopen, in the original client. A fresh character also completed three live spins; captured results carry cumulative usage 1, 2 and 3 with distinct winning slots.

## Exclude original account database snapshots

Original account databases and SQLite sidecars are excluded from the source asset
inventory. The obsolete database snapshot export and legacy-table indexing are
removed. Asset import rejects old snapshot entries and snapshot-format documents;
static catalogs and Go gameplay storage remain separate. The maintained asset
manifest no longer requires a database snapshot for rebuilding or verification.

Validation: all 41 remaining source assets reconstruct byte for byte; 19 Python
asset-tool tests, affected Go race tests against the cleaned catalog, formatting,
lint and server build pass. The local snapshot export and retained copies in the
active asset catalog and three asset backups were removed; SQLite integrity
checks passed after cleanup. Go gameplay storage was preserved.

## WLRI-only external media import

External regeneration and restoration now use only `Wonderland-Client` (WLRI).
The import scope is audio, sprites, pictures and media; root gameplay definitions
and server-only JSON remain tracked. The obsolete database-path override export
and retained SQL document are removed and excluded from future exports/imports.
Complete static table regeneration remains a separate offline workflow.

Validation: 20 Python asset-tool tests and affected Go race tests pass. All 40
remaining static sources reconstruct byte for byte; formatting, lint and server
build pass. The override document and its manifest entries were purged from the
active assets database and three asset backups; integrity checks passed.

## Live monster drop multiplier continuation

- Ported AC02 `/droprate` and `:droprate`: GM-only query/update, finite numeric validation and the original 0.1–100 clamp. The process-local setting defaults to 1.0 after restart.
- Victory loot reads the live setting under world ownership, so existing battles use the value at reward time. MonsterDropManager calibration retains the 5% floor, disabled rows, native/known item validation, authored row order and one-entry limit. Captured monsters remain excluded.
- Regression tests cover deterministic probability boundaries, malformed values, authorization/revocation, private AC2:16 feedback and actual victory-loot integration. SQL drop definitions remain authoritative; no asset fallback or original account database was added.
- Remaining GM commands, loot editing and native-client acceptance stay in the pending categories above.
- Verification: original AC02 and MonsterDropManager SHA-256 hashes matched after CRLF normalization; focused regression tests, full `go test -race ./...` with `WONDERLAND_TEST_ASSETS_DB=var/assets.db` and both native reference data paths, plus `make fmt-check lint build`, passed. Native-client interaction with the new command remains untested.


## GM repair continuation

- Ported AC02 `/repair`, `:repair`, `/fixall` and `:fixall`, backed by the original GmManager.RepairAllItems behavior: free durability restoration for worn gear and bag stacks, with optional online target selection.
- Repairs save before native AC23 bag removals/additions and equipment synchronization. Metadata, quantities and currency remain intact. Healthy repeats make no changes. Missing targets fail explicitly; active interactions on either actor or target block repair. Failed delivery to a repaired target closes that target's connection for a fresh snapshot while retaining the GM session.
- Regression tests cover independent packet layouts, native additive inventory replay, forge preservation, persistence, aliases, target lookup, permission/revocation, interaction ownership and save failure. Original source hashes match after CRLF normalization. Remaining GM operations and native-client acceptance remain pending.
- Verification: focused GM repair regressions, the full `go test -race ./...` suite with imported SQL assets and both original native data paths, and `make fmt-check lint build` passed. The server binary was rebuilt; the running instance was not restarted.


## Native Local chat settings correction

The native `aLogin` Local sender reports `Whisper is closed` when its Local
channel gate is off. Its AC33:2 receive case at `0x2ea812..0x2ea872` reads four
option bytes, then the channel mask and a final byte passed to a no-op.
The C# and previous Go snapshot had only three option bytes and the mask;
that six-byte packet left the native mask field absent.

Go now sends eight bytes: `33, 2, PK, join, trade, option10, channels, tail`.
`option10` retains the native enabled constructor default (its gameplay meaning
is unresolved); `tail` is zero. Channel bits and stored preferences stay intact.
The same builder serves login/reconnect and AC33:2 refresh requests. Independent
byte fixtures and native-offset tests cover all five channel flags, disabled
channels, and saved-setting synchronization. After rebuilding, restarting and
logging in again, the user confirmed Local chat works in the real alogin client.

Verification: focused settings/chat regressions, full native-data and SQL-assets
race suite, formatting, lint and server build passed.


## GM attribute reset continuation

- Ported AC02 `/restat`, `:restat`, `/resetstats` and `:resetstats`, with optional online character target selection. Each base attribute resets to the legacy baseline of 10, positive excess base points return with uint16 saturation, and equipped HP/SP refill.
- Corrected the reference refund calculation: Equip getters add permanent avatar bonuses, while setters write base values. Refunding the getters would grant avatar-bonus points on every repeated reset. Go refunds only stored base allocations above the 50-point reset budget; low-total characters receive no refund, preserving the legacy baseline behavior.
- SQL skill table orders supply AC5:3 serialization. Stat updates and the full snapshot publish only after durable save; snapshot or storage failures leave state untouched. Missing targets fail explicitly, busy actors/targets cannot bypass interactions, and failed target delivery is isolated for reconnect. Skills, level/EXP, inventory and currency remain intact.
- Tests cover independent stat packet bytes, persistence, usable refunds, repeat safety, saturation, permanent bonuses, target authorization, aliases, gameplay gates and failure recovery. Native-client reset acceptance and remaining GM operations are pending.
- Verification: AC02, GmManager and Equip reference hashes matched after CRLF normalization; focused reset tests and their race run, the full native-data/SQL-assets race suite, formatting, lint and server build passed. The server binary was rebuilt without restarting the running instance.


## Configurable login and gameplay idle limits

`idle_seconds` now applies to login, character selection and character creation,
with a ten-minute default. `world_idle_seconds` applies after a character is
selected or created, including map loading and warps; its default of zero disables
idle logout. Both accept zero (disabled) or 1–86400 seconds. Each incoming packet
renews the current phase's deadline; outgoing packets do not. Existing login
deadlines are cleared when disabled gameplay begins. Restart to apply changes.

The reference SocketClient inherits Client3, which retries socket receive timeouts.
Map.Process separately checks a 30-minute IdleTimer; its LastPacketTime is never
refreshed in this checkout. AC63 also imposes a 60-second character selection wait.
Go uses the requested configurable idle policy rather than copying those timers.
Reference hashes were verified after CRLF normalization. Configuration and socket
regressions cover expiry, disabled waits, and login/gameplay phase transitions.

Verification: focused idle-policy race regressions, the full SQL-assets/native-data
race suite, formatting, lint and server build passed. The local configuration was
updated and the binary rebuilt; restart the running server to apply the policy.


## GM skill reset continuation

- Ported AC02 `/clearskills`, `:clearskills`, `/resetskills` and `:resetskills`, for self or an online character selected by ID/name. GmManager.ClearSkills rebuilds SkillManager starter and current stat-qualified skills at grade one with zero proficiency; quest/reward/evolved skills outside that baseline are removed, while quest progress and other character state remain intact.
- SQL skill table orders supply AC5:3 synchronization. Native FUN_004381c4 (`0x438526..0x43858a`) overlays indexed records without clearing omitted entries. The outgoing snapshot includes removed skills at grade/EXP zero, followed by AC5:4; these transient zero-grade records are never persisted. The reference incremental AC5:12/11 replies are insufficient to clear removed entries.
- Saves and validates the complete snapshot before sending. Both actor and target must be ready and free of battles, trades, events and movie sequences. Missing targets fail explicitly, privileges are checked per command, and a failed target delivery closes only that target for reconnect.
- Focused race regressions cover independent native skill records and overwrite replay, progression rebuilding, persistence and repeat safety, target lookup, GM revocation, interaction gates, missing catalog records, canceled saves and failed target delivery. Source hashes matched after CRLF normalization. Actual-client acceptance and remaining GM commands are pending.
- Verification: affected GM/chat/skill race tests passed, including all four elements against the installed SQL catalog and adjacent allocation/login/evolution tests. Formatting, lint and Server build passed. The binary was rebuilt without restarting the running instance.

## GM command completion

- Completed all AC02 chat command aliases and the advertised `petlvl`, `petexp`
  and `clearinv` commands. Shared definitions drive new command dispatch and help.
  Added bulk element skills, god attributes, pet recruitment/amity/rebirth/level/EXP,
  bag clearing, Tent grants, mall purchases, online information, mute/jail,
  map invisibility, bulk kicking, forced PvE victory, SQL reload and shutdown.
- Durable mute expiry covers every chat channel and survives reconnects. Jail
  persists mute and location together; explicit unjail clears both. Visibility
  changes synchronize existing peers and exclude hidden GMs from late arrivals.
- Forced victories use the existing party reward and quest continuation path;
  processing/finished battle guards prevent repeated awards. Countdown shutdown
  exits through listener/session cleanup and normal disconnect autosaves.
- SQL reload replaces selected immutable collections. Quest reload rejects active
  interactions and retains map geometry and ground/monster timers. Catalog locks
  cover pre-world readers and administration snapshots. GM privilege reload reads
  all accounts and orders persisted/live changes against administration grants.
- Intentional reference corrections: `hide`/`unhide` set explicit states, pet
  rebirth cannot repeatedly mint points, and god pet vitals use calculated caps
  rather than fixed values that disagree with native roster/reconnect normalization.
  Tent grants do not implement tent interiors. Quest/mall/drop reloads use the
  configured assets database exclusively; file import/export is offline tooling.
- Verification: focused GM/chat, SQL reload, world-state preservation and complete
  privilege reload race tests passed against the installed SQL assets. Formatting,
  lint and Server build passed. The running server was not restarted. Native
  aLogin acceptance of these newly completed commands remains pending.

## Water magic defense contribution (superseded)

The compiled standard-growth table below supersedes this earlier custom change.

- Water player MDF now uses `round(level * 3 + WIS * 2)` before equipment
  bonuses, matching Earth's level contribution to physical DEF. Other elements
  retain `round(level * 2 + WIS * 2)` for MDF. This is an intentional gameplay
  change requested by the user; Private Server's Equip.cs has no Water MDF bonus.
- Focused stat regressions cover all four elemental bonuses, equipment MDF and
  Water's isolated contribution at levels 1, 40 and 199.


## EXP rate and Windows administration migration

- Added persisted `/exprate` (`/experience`) and Server operations tuning with
  source midpoint-even rounding, positive minimum and native EXP cap. Player and
  pet victory rewards scale once; direct GM/native EVE progression remains unscaled.
- Replaced the original MainForm/ExtendedTabs controls with authenticated web
  operations, character editors, GM studio, account/session/security controls,
  friends, guild/marriage records, GM gifts, battles, static content editors,
  map/NPC/event inspection, dialogue resolution, logs and startup configuration.
  See [ADMINISTRATION.md](ADMINISTRATION.md) for controls and constraints.
- Character and asset editors reject stale versions. SQL edits validate a complete
  catalog before commit, retain source provenance and reconnect loaded players.
  Optional map/chest documents live only in SQL; no external override file is
  restored. Weighted chest rewards/cooldowns are per character and atomic.
- Schema v7 adds administration records. Guild/marriage management does not imply
  native client guild/marriage gameplay is complete. SQLite configuration replaces
  legacy provider selection; original account snapshots remain excluded. NPC
  show/hide and prop controls are live view changes, matching the editor's scope.
- Focused regressions cover permission checks for every new route, EXP persistence
  and player/pet scaling, state conflicts/loading gates, SQL rollback/projection,
  gift idempotency/full bags, guild cleanup, IP normalization, log concurrency,
  configuration replacement and chest respawns. All 22 new administration views render in a headless Chromium smoke, including
  read-only inspector controls. Focused race tests, formatting, lint and build
  pass. Interactive native aLogin acceptance remains pending. The running server
  is not restarted by this work.

## Compiled player elemental growth

Player ATK/DEF/MAT/MDF/SPD and nonlinear HP/SP growth share a compiled table in
`internal/game/growth_parameters.go`. Every element has explicit level and
attribute coefficients; HP/SP also expose base, power, nonlinear attribute
coefficients and multiplier. HP/SP use Formula.Dat coefficients with multiplier
1 for all elements. Standard combat growth follows the saved WLRI Japanese wiki
in docs/References: DEF uses CON × 1.75, MDF WIS × 2.2 and SPD AGI × 1.8;
Earth has DEF level × 2.6 and MDF level × 2.2. This replaces the custom Water MDF
bonus and assumed Earth HP/Water SP bonuses. Changing parameters requires rebuilding;
startup JSON and administration do not expose them. Creation, battle, rest,
allocation, equipment, progression and GM/admin stat operations use the same
policy. Login recalculates and persists maxima without healing. Native AC8
contributions use fixed original formulas as their reference, independent of
compiled gameplay tuning. Custom aLogin display acceptance remains pending.
Pet/monster growth and reborn-job multipliers remain separate. See
[CONFIGURATION.md](CONFIGURATION.md#elemental-stat-growth).

## Pet growth across all attributes

Automatic pet level-up points now use weighted draws across STR, CON, INT, WIS
and AGI. This intentionally replaces the private server's strongest-three rule.
Weights use the NPC template when known and current attributes otherwise, with a
minimum of one per uncapped attribute. Capped attributes are excluded and all-capped
pets skip the draw. One point is allocated per gained level; manual training and
allocation budgets retain their existing rules. Exhaustive interval tests cover
exact weight proportions, low-stat eligibility, missing/zero template weights,
capacity exclusions and multi-level growth. Pet battle EXP/progression tests
verify the shared allocation path.

## Selectable pet growth formula

Startup `pet_growth_formula` selects `base_stats` (default, all-five species/base
attribute weights) or `combat_stats` (ATK/DEF/MAT/MDF/SPD weights for
STR/CON/INT/WIS/AGI). The new calculation includes current level, attributes,
equipment/forging and existing pet combat elemental rules. Weights are rebuilt
for every gained-level point, and capped attributes are excluded. HP/SP and
transient battle effects do not add weights. Battle reward and GM pet EXP grants
share the same startup selection; manual training and `/petlvl` point budgets
remain separate. The selector is captured at startup and requires a restart to
change; live operations and gameplay database settings cannot alter it.
Exact weighted intervals, equipment penalties/bonuses, elemental differences,
multi-level recalculation, missing-template fallback, selector validation and
save-without-live-application are covered by focused regressions. See
[CONFIGURATION.md](CONFIGURATION.md#pet-automatic-point-allocation).


## World Entry completion

- Admission validates exactly five creation points before name fallback,
  credentials verification or persistence. The native reset at `0x2190f8`
  zeros the six-byte attribute array and sets the remaining budget to five;
  allocation decrements it. Model bonuses remain separate. The old 25-point
  server test requests have been corrected. Existing characters and GM/stat
  allocation are unaffected; invalid requests receive AC0:30 and can retry.
- Character entry restores the monster book through sorted, unique AC53:9
  template IDs filtered by SQL NPC book indexes 1–5500. PvE victories stage
  discoveries with rewards in one character save, including captured monsters;
  failed saves publish nothing. Disconnect/restart retains discoveries.
- AC15:19 restores the four reference story constellations from active positive
  completion marks, before final world-ready markers. Quest mark changes send an
  updated collection after commit. Removed marks do not count as earned stars.
- Native AC89:0 receives captured AC90:1 scene status; AC92:1 acknowledges it.
  The first request sends the optional saved MOTD as AC23:57 once per character
  login. Sync may occur during loading and does not replace AC12:1 or publish
  world presence. MOTD is limited to 255 encoded bytes.
- Optional ordered SQL `quest_visibility.json/value` rows implement the reference
  quest/step spawn and despawn lists for entry, warp and quest resynchronization.
  Map/actor/step validation rejects invalid edits before commit; indexed SQL rows
  are authoritative. The legacy quest database loader populates no such lists,
  so the default is empty. There is no account database import or JSON fallback.
  Full legacy quest definitions remain in the Quests queue.
- LoginServer hands the same socket to the world queue; its separate assist-tool
  path is not an authenticated client transfer protocol. Go uses the authenticated
  stream on either configured listener. Socket regressions cover creation,
  acknowledgement and gameplay on both service paths; no ticket/reconnect is
  invented. Native-client capture comparison remains in Acceptance.

Verification: reference hashes matched after CRLF normalization; focused race
regressions cover allocation rejection/retry, socket lifecycle, native packet
bytes, durable discovery/failed saves, story state, SQL ordering and rollback.
Interactive aLogin checks for these additions remain pending.

## Combat completion checkpoint

The remaining queued Combat ports—native PK, Palace trial execution, monster
skill AI and legacy monster-ID quest rewards—are implemented.

- AC11:2 type 3 resolves the raw character ID on the same map, checks PK
  preferences and interaction availability, and creates two party formations.
  Defender players occupy column 1 and pets column 2. Both sides submit owned
  actions, receive menus/timeouts, and use side-aware healing/effects/combos.
  Disconnected fighters stop blocking turns; an absent entire PvP side forfeits.
  Ending during animation prevents subsequent stale attack packets. PvP results
  save vitals and proficiency without EXP, gold, drops, notebook discoveries,
  capture or pet amity/desertion penalties. The losing player's HP recovers to
  the usual defeat amount. GM forced victory respects the invoking player's
  side, and the administration battle API identifies PvP encounters.
- The separate C# PvPManager duel helper has no dispatch callers and only emits
  a battle background; active native PK uses PvEBattleManager. Go routes native
  PK to the real engine rather than creating an unregistered duel animation.
  Join/watch/NPC challenge cases remain inert as in the reference handlers.
- NPC skill slots are copied from the SQL catalog for quest, wild, trial and GM
  encounters. AI selects uniformly among distinct usable slots, spends native
  SP costs, and casts generic damage, effects or compatible healing/revival.
  Healing chooses the lowest HP ratio; cast effects avoid refreshing already
  active sources, and offensive skills use a stable live enemy target. Dead or
  action-blocked monsters do not cast. Missing/unknown/unaffordable/useless
  slots fall back to the original basic-attack damage/guard behavior. Native
  basic attacks retain their random target selection. The C# reference always
  basic-attacks; skill AI extends that implementation through SQL data.
- AC77:1 and GM `/palace` start real battles using optional validated SQL stages.
  Victory rewards precede ordinary loot and share the durable character save;
  defeat, flee and duplicate settlement award none. No default stage table is
  installed: reference guardian IDs 1001–1012 do not exist in WLRI, and its
  alleged chests 48030–48033 are vehicle capsules. Configure real content using
  [the administration procedure](ADMINISTRATION.md#palace-trials). Native EVE
  Zodiac encounters continue using their existing event formations/rewards.
- Legacy rescue fallbacks retain reference monster IDs/name aliases for Niss,
  Xaolan and Little Red Riding Hood. Niss uses WLRI companion 14081, correcting
  the reference's enemy-template reward. Completed quests cannot pay repeatedly;
  full/hotel/fate-unavailable companion rosters leave the quest incomplete.
  Reserve companions keep their progression. Native EVE callbacks and trials
  own their rewards and suppress these fallbacks. Ordinary bounty definitions
  and kill objectives belong to the remaining legacy Quests port.

Focused tests cover native formation/ownership bytes, both PvP outcomes and
party participation, timeouts, forfeits, stopped playback, effects, protection,
monster skills/SP/fallbacks, SQL projection/rollback, trial admission/rewards,
quest idempotency, companion identity/capacity and failed-save isolation.
Source hashes remain pinned and match after CRLF normalization. Native aLogin
acceptance and commercial-server parity remain unverified; existing area-target
expansion, exact native grade thresholds and hit/resistance/visual semantics
remain separate parity work. Burst EXP remains outside the user-approved scope.

## Structured server persistence

- Gameplay schema v8 moves character scalars and owned collections into typed
  GORM tables. All Store consumers, character administration and checkpoints use
  these tables transactionally. Legacy character JSON remains an inert migration
  snapshot. Account constraints, credentials and IDs are preserved.
- Structured asset schema v1 projects the complete runtime catalog into typed
  tables and ordered child rows, preserving protocol bytes and unknown imported
  provenance. Gameplay never assembles JSON documents or native archives.
- Content administration edits named SQL datasets directly. Optimistic versions,
  validation, session gates and reconnect publication remain. JSON is API transport.
- `database-migrate` prepares upgraded copies, verifies conversion parity and SQLite
  integrity, and leaves originals untouched. New rebuilds prepare the structured
  catalog before publication. See ASSET_DATABASE.md for activation and rollback.

## Economy and Social checkpoint

Implemented native AC56 stalls and AC39 guild gameplay, guild chat, public marriage
and parcel mailbox workflows, native AC59 manufacturing, source gathering and
chance-based public synthesis. Wallet/inventory transfers, parcel escrow/claims
and marriage fees/rings/relationships commit atomically. Guild roles/icons and
parcel rows use gameplay schema v9; shared economy rules use asset schema v2.
The v1-to-v2 upgrade preserves all existing catalog tables and content edits.

Focused tests cover raw native packets, stale offers, overselling, full-bag
rollback, metadata, unauthorized guild actions, missing invitations, leader
repair, concurrent claims, ceremony rollback, gathering timing and synthesis
rate extremes. See [ECONOMY_SOCIAL.md](ECONOMY_SOCIAL.md) for commands, migration
and the exact remaining native limitations. Private Server's arcade, bank
PIN/transfer and decoration success acknowledgements do not implement their
claimed economies. Per the user's instruction, unresolved handlers stay pending.

Local verification prepared `var/migrated-v2-economy/{assets,wonderland}.db`
from the configured v1/v8 copies, retaining the sources. `config.local.json`
selects the verified v2/v9 copies. The server binary was rebuilt; no server
process was started.

## Items and Player State completion

- Native AC23:96/15 dispatches recovery, equipment, vouchers, gacha and tent items.
  Targeted packs reject quantities other than one and non-character targets before
  consumption; unavailable/withdrawn pools retain their items. Unsupported special
  effects remain unavailable rather than consuming items without verified rules.
- AC65 opens/closes owner-specific homes and enters/exits native interior 63507.
  Scene identity includes the owner, isolating visibility, movement, local chat,
  social requests and trades. Homes do not run overworld events or encounters;
  PK is unavailable inside. Closing/travel/logout evicts visitors, including those
  still loading. Each visitor's own return coordinates persist for reconnects.
- AC62 placement and zero-based ordinal movement/rotation preserve full metadata.
  Bag debit, furniture insertion and pickup grants are transactional. Only the
  owner may edit; public commands expose access locks and furniture recovery.
- Gameplay schema v10 owns tent rows, furniture and return fields. Asset schema v3
  adds source-derived initial tent rules without changing existing definitions.
  Item reservations remain process-owned and are omitted from SQL/JSON and autosave
  comparisons. Grants skip reserved slots; removal/movement/repair/equipment and
  pair transactions honor reservations.
- Focused regression tests cover native packets, home separation, locked entry,
  owner-only edits, stale/full-bag rollback, metadata, ordinal indices, loading
  visitors, logout/crash recovery, transient locks and preserving v9/v2 upgrades.
  Original-client tent acceptance remains pending. AC60/61 and AC62:4 have no
  implemented source rules; their decoration/upstairs/special operations remain
  pending in Economy rather than becoming false-success handlers.

Local upgrade verification created `var/migrated-v3-items/{assets,wonderland}.db`
from the selected v2/v9 copies, preserving both sources. `config.local.json` now
selects the verified v3/v10 copies. Imported WLRI definitions include Tent 36002,
Coconut Basin 38027 and Work Platform 38049 with their expected native types.
Focused race tests, vet and the server build passed. The server remains stopped.

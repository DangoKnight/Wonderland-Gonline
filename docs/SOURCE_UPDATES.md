# Private Server update import

This import reviews the 40 commits and 431 distinct changed files from
`e63e9feabc3b3bf596fe8a4f61d5d3e488c34ba1` through
`5c368395146c0c54bb01bcee3ae88d50513beae8` (2026-11-01).
The sibling's working checkout remains unchanged; its remote refs were fetched
and the pinned revision was exported for review.

[The update ledger](source-update-inventory.json) preserves the original 431-file
snapshot at `1ff5bba` and records the two subsequent commits and their two changed
files separately under `follow_up_imports`, with exact source-byte hashes,
dispositions and applicable Go owners. It supplements
the original [baseline inventory](source-inventory.json), whose revision and
normalized hashes remain intact. File counts describe review coverage, not a
percentage of feature completion. Native-client acceptance remains separate.

## Imported behavior

- Waterfall Grotto mirror events 1/2/3/5 continue forward through their authored
  beam and door rules without replaying earlier branches. Existing visit-local
  actor show/hide overrides already keep turned mirrors visible through entry
  layout synchronization; leaving the map resets them. Fairy Altar events 2/3
  use movie mode 1, allowing the HUD and dialogue to resume without a map load.
- Asset v14 stores boxes, skill evolutions, EVE gathering pools/timers, NPC buy
  prices/catalogs, tent surfaces/tool hosts, chest classifications, travel and
  map-exit rewards in typed SQL tables. Embedded inputs are initialization seeds;
  runtime and content administration use SQL. Existing edits are preserved.
  Missing NPC route tables and sign/speed columns are projected from retained SQL
  map sections without replacing edited NPCs.
- Deterministic boxes consume one box and grant its configured item atomically.
  Unavailable rewards and full bags retain the box. Boxes take precedence over
  a conflicting weighted-pack definition, matching the new source dispatcher.
- Companion altar rebirth uses native AC44 and EVE condition 21: level 100,
  amity 60, bare equipment and no previous rebirth. It preserves allocated points
  and grants no bonus points. Existing AC69 ascension retains its separate policy.
- Character rebirth supports AC79, deduction of 99 levels, class skills and
  Transfer Mirror changes. GM rebirth accepts numeric classes and optional force.
  Level requests cap at 199; displayed EXP caps without reducing stored EXP.
  See [character state](MANUFACTURING_CHARACTER_STATE.md).
- Mission opcode 17/condition 19 use scene-local counters. Marker opcode 10 no
  longer aborts valid branches. Quest lifecycle, gender, element, class and team
  size follow the new interpreter; PreEvents honor authored AND-condition counts.
- Actor pose replay includes facing/stance and preserves animated idle frames.
  Dialogue references carry all 24 bits, including Go-client text/voice lookup.
  Scene-local actor/prop state applies to every map; quest visibility resynchronizes
  after battle. Portal click IDs take priority over reverse geometry.
- The storm waits for the movie handshake before completion. Beach arrival starts
  wake-up immediately, stages Robinson as actor one, releases all scene/posture
  gates and awaits a manual click. First contact continues into the guide question.
  S.Monkey movies use playback mode one, with the full-party path stopping after
  the rescue movie. Existing atomic companion/mark delivery is retained.
- Generalized EVE gathering supports complete multi-pool reward/timer branches.
  Rewards, cooldown and quest mark commit together. Source pool choices are equal;
  historical official weights remain unknown.
- Skill proficiency uses effective primary-target damage/healing, ignores
  overkill/overheal, awards once per area cast and applies the server EXP rate.
  Buffs and revives use source flat awards. Poison defaults become 10% maximum
  HP with minimum 15; only exact old defaults migrate, preserving custom effects.
- Formations beyond eight enemies retain per-slot reinforcement queues. New
  occupants enter at round end; defeated waves remain in EXP/loot history.
- Gold chest rewards commit with cooldowns. Beach crate/chest categories seed when
  their referenced definitions exist. Static chest templates 16000–16011 are
  recognized. Laura's corrected exit gift grants Tent 36002 and Remote Control
  34058 with mark 10035 and the destination in one transaction.
- NPC purchases resolve the active catalog and commit whole carts against SQL
  funds/capacity. Unlisted IDs are rejected; movement invalidates shop authority.
- Native furniture placement/move/pickup, root/child tree snapshots and Work
  Platform tool attachment are implemented. Host/tool refunds preserve metadata
  and roll back together on insufficient space. Owned tents and edited defaults
  are preserved. [Housing details](ITEMS_PLAYER_STATE.md#using-a-tent).
- Manufacturing accepts native word benches and batches, persists exact consumed
  instances, refunds atomically on native cancellation and recovers after restart.
  Completion places furniture without overlap or applies a surface and returns
  spare copies. Legacy starts retain pause/resume. [Manufacturing details](MANUFACTURING_CHARACTER_STATE.md).
- Personal AC23/AC56 stalls share the existing pair transaction. Alternative
  AC53/AC20 adapters list real owned items. Persistent administrator bazaars add
  finite/infinite stock and separate native item/pet/vehicle layouts and purchases.
  Quotes, funds, stock and delivery are validated atomically. [Bazaar administration](ECONOMY_SOCIAL.md#persistent-administrator-bazaars).
- Teleport Scroll points persist in gameplay v18. Scroll travel, Kelan Scroll and
  Bamboo Dragonfly destinations debit items with the destination commit; missing
  destinations retain items. Recording/travel require ordinary world scenes.
  Notepad and Remote Control use their actual WLRI definitions.
- Authenticated compact AC63:3 creation shares normal name/allocation/ownership
  checks and is distinguished from native cancellation by length. Native AC9
  creation remains available. Packet debugging reuses durable log-level control
  with redaction; Bangkok GM aliases use the Temple approach coordinates.

## Preserved Go behavior and explicit exclusions

Existing native inventory footprints, owned-slot/account validation, durable
settings, friend-status rosters, native trade offers/confirmation, guild bulletins,
instance lobbies, actual PvP, fishing, rank-based compounding, funded forging,
resting and paid arcades already implement their active workflows. The ledger
maps these to Go rather than replacing them with less complete source versions.

Source claims do not override independently verified native layouts. In
particular, AC23:122 clears fishing presentation; it is not a crafting/rebirth
sparkle effect. Those new effect broadcasts are excluded, while AC5:5 shape
resets use zero. Native trade metadata and expression/held-pose behavior retain
their verified layouts. Chocolate/White Rice are not reinterpreted as utility
items. Valid naked level-one characters are not refilled or re-equipped on login.

The experimental directory contains one imported real bazaar implementation and
23 other engine/manager files. Their remaining prototype behavior stays on hold:

| Source prototype | Reason / retained implementation |
| --- | --- |
| Aquaculture | Unconditional catch grants lack cast/bait/timing validation; retain configured fishing. |
| Arcade | Fallback submits session zero and never grants its calculated reward; retain real arcade sessions. |
| ATM | Memory-only balances/transfers; retain SQL deposit/withdrawal. |
| Church charity | Karma/blessing packets have no combat HP/EXP integration. |
| Combat status lab | Separate test dictionary does not drive battle effects. |
| Community bulletin | Handler discards user text; retain real guild bulletin content. |
| Companion talents | Global template counters lack ownership and combat integration. |
| Weather/lighting | Unverified layout, unrestricted client setters and no integrated clock. |
| Equipment forge lab | Fake durability/grade packets do not mutate owned equipment or consume tools. |
| Festival races | Checkpoints ignore position/lap constraints; medals are packet-only. |
| Gobang wagers | Self-versus-self wire path and no stake escrow. |
| Guild siege | Crystal combat/timers are not integrated; tax collection does not pay gold. |
| Hot spring lab | GUI-only unrestricted healing collides with the real AC44 altar namespace. |
| Instance wave trials | Counters advance without real dungeon battles or token delivery; retain lobbies/mission counters. |
| Lottery lab | GUI-only hardcoded grants lack durable jackpot/payment boundaries. |
| Mechanism lab | Guesses door IDs as switch+100; preserve authored actor/prop state and verified transformations. |
| Achievement titles | Memory unlocks and stated bonuses have no achievement/growth integration. |
| Ranked arena | Memory matches never start a battle or receive real battle outcomes. |
| Secondary PIN | Plaintext memory state guards no resources and collides with native quest marks. |
| Tent assistant | Free food without ingredients; auto-craft is an unused flag. |
| Excavation lab | GUI-only loot lacks tool/position/cooldown validation. |
| Vehicle lab | Fictional fleet IDs and no ownership/fuel consumption; retain actual native vehicles. |
| Experimental manager | WinForms/INI toggles do not enable incomplete systems in Go. |

Windows UI/build metadata, 141 PowerShell/.NET investigation artifacts, 105
native decompilation artifacts and 68 source documentation files are classified
as supporting evidence. They are not copied into the runtime or counted as
executed validation. Existing Go administration replaces WinForms; persistent
vendor controls are available through `/bazaar`.

The source database's quest definitions, starter values and existing chest
rewards are unchanged. Its shared new portal route/schema changes are represented
in Go. Source accounts, characters, GM grants, settings and administrative stock
are operator state and are excluded from initialization. No live database or
source working checkout was overwritten.

## Upgrade and verification

Use the [offline database upgrade](ASSET_DATABASE.md) before running this version:
asset v15 and gameplay v18 are required by the current server. The v15 story
policy migration is described in [STORY_SYSTEM.md](STORY_SYSTEM.md). The installed databases remain unchanged;
disposable upgraded copies are used for validation. Commands and results are
centralized in [TESTING.md](TESTING.md#source-update-verification--2026-11-01).
The two-commit story follow-up requires no schema/data changes; its focused
checks are recorded [separately](TESTING.md#story-follow-up-verification--2026-11-01).

Native-client captures, multiplayer/load acceptance and Go-client interfaces for
new native server forms remain separate acceptance/roadmap work. Source prototype
exclusions are explicit, not claims that these gameplay systems are complete.

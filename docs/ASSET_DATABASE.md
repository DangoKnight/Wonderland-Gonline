# Server databases and migration

Shared game definitions and administrator content edits live in the configured
`assets_database`. Accounts, characters, mutable gameplay state, relationships,
audit records and persistent server settings live in `database`. Defaults are
`var/assets.db` and `var/wonderland.db`. See the
[ownership policy](DEVELOPMENT.md#server-data-ownership-and-initialization).

## Upgrade an existing installation

Assets v57 stores each skill's native `maximum_grade`. The repository-owned
`skill_grade_defaults.csv` supplies all 953 definitions without client extraction.
The migration fills absent limits and preserves administrator
values. Progression, damage, targeting and grade validation use this limit;
missing definitions fall back to ten, while explicit zero limits remain zero.
An independent presence flag distinguishes these states in SQL. The embedded server seed is updated
to v57. Stop the server, migrate, rebuild and restart using the procedure below.

Assets v56 supplies repository-owned contact defaults and a complete server-only
seed. No client installation, exported manifest or retained MBTM archive is
required to build, initialize, run or migrate the server. Contact migration fills
only missing/zero counts and preserves administrator edits. Normal startup
initializes only a missing catalog, never replacing or silently upgrading one.
`go run ./cmd/data-rebuild -config config.local.json` explicitly rebuilds from
the repository seed with backups; `-data data` opts into extracted inputs.

Assets v55 adds `contact_events` to the structured skill catalog. The offline
migration originally derived counts from retained SkillData.MBTM records; v56
uses the tracked extracted contact table instead. It preserves existing
nonzero administrator counts and leaves missing records at the one-contact
compatibility fallback. Damage now uses
`P = (AdditionalDamage + PowerPerLevel * Grade) * ContactEvents`, with
`StatMultiplier` scaling only the relevant ATK/MATK term. Wiki BasePower values
remain stored as references. Stop the server, migrate, rebuild and restart as
below; no gameplay database or client protocol change is required.

Assets v54 adds 71 verified character, pet and quest skill powers from the saved
unique-skill reference. Native NPC ownership, SP costs, descriptions and Chinese
actor names identify the skills independently of unreliable icon filenames.
Six matched but approximate/grade-conditional powers remain review-only; see
[skill powers](DAMAGE.md). The additive migration fills only unresolved matching
definitions and preserves all existing powers, including administrator edits
and explicit zero. Stop the server, migrate, rebuild and restart as below.

Assets v53 adds an editable nullable base skill power to `catalog_skills` and
seeds 153 verified elemental skill powers from the saved Japanese wiki tables.
Blast Attack and Flame Attack both have reference power 20. Existing explicit
powers, including zero, and all native coefficients remain intact. The former
wiki-power fallback and provisional five-percent grade curve were superseded
by the v55 [native-coefficient formula](DAMAGE.md).

Assets v52 keeps Noa's puzzle, result evaluation, retry invitation and recruitment
in one conversation. Scoped continuation policies connect Kelan event 56's native
branches without unlocking movement between stages. Decline and cancel still end
the interaction. The migration preserves existing same-ID administrator rules,
deliberately empty policy collections and character progress. Stop the server,
run the offline migration below, then rebuild and restart the server. No client
asset rebuild is required.

Assets v51 keeps Xaolan's house recruitment dialogue and its following choice
in one interaction. It adds a scoped continuation from event 9, branch 1 to
branch 4, preserving existing same-ID administrator policies and deliberately
empty policy collections. Stop the server and run the offline migration below
before starting the rebuilt server. No client asset rebuild is required.

Assets v50 imports every map's native default backdrop and ordered spatial
regions from retained `Ground.MMG` bytes into `catalog_battle_backdrops` and
`catalog_battle_backdrops_regions`. All 1,119 EVE maps have matching definitions;
the archive contains 1,147 terrain records and 1,398 regional rectangles.
Battle entry selects by the initiating player's coordinates. The migration
retires only exact original v45/v47/v48/v49 backdrop policies; administrator
edits, explicit overrides and intentionally empty backdrop tables remain intact.
See [native backdrop evidence](BATTLE_BACKDROPS.md). Stop the server, run the
offline migration below, rebuild and restart. Existing client assets suffice.
The following v45–v49 notes describe historical migrations superseded by v50's
native map regions.

Assets v49 assigns the Beach backdrop to North Island map 60000's seven
seaflower encounter events (36, 37, 38, 40–43). Map 60001 is also named
North Island but has different encounter definitions. This revision preserves
same-ID administrator edits and empty policy collections.

Assets v48 adds the verified Lakeside Jungle backdrop for North Island's
lakeside Fresh Mushroom encounter (event 65), and Kelan Village scenery from
the captured honey battle. Existing same-ID backdrop edits and intentionally
empty policy collections are preserved. The formation backdrop is one byte;
the next byte is independent metadata. Rebuild the client as well to correct
backdrop decoding when using a Legacy server.

Assets v47 adds the verified Jungle backdrop for Xaolan's North Island rescue
(map 60001, events 44 and 83). Native selector 90 resolves to scene 59090. The
content revision preserves same-ID edits, intentionally removed quest families
and empty policy collections. Enemy placement now also honors authored EVE
formation slots instead of packing initial enemies into sequential cells. Run
the offline upgrade and rebuild the server to apply these corrections.

Assets v46 replaces the stock Xaolan rescue visibility overrides with a
visit-scoped visibility policy. Xaolan and her grandmother remain for the rescue
scene, then native map-entry events hide them on the next visit. Later quest
stages retain their native alternate actors. The migration preserves edited
rules and player quest progress. Stop the server and run the offline upgrade
below before restarting the rebuilt server.

Assets v45 adds a modular `battle_background` policy for Xaolan's Kelan house
(map 12002). Its native wire selector 140 resolves to room scene 59140. Client
scene loading now supports native single-layer backgrounds as well as layered
outdoor scenes. The content migration retains same-ID edits and intentionally
removed quest families. Rebuild the server, run the offline upgrade below and
restart both server and client to apply the correction.

Assets v44 removes the stock Lost Dog visibility overrides in favor of the
native Kelan pre-events. The Shiba Inu in the hogpen is visible before accepting
the quest, but its native interaction only recovers it after acceptance. After
recovery it disappears from the hogpen and appears beside its owner. The shared
declarative policy revision preserves edited rules and intentionally empty
collections; it does not change existing player quest progress.

Assets v43 follows the 2026-11-05 Legacy capture of Kelan's honey quest.
Collecting honey continues into the bee warning and battle. Battle outcomes hide
the bees without completing the hand-in mark. Native pre-events keep the patient
lying down and the honey actors hidden while carrying honey; the hand-in exchanges
item 30034 for 23108 and advances progress, allowing the patient to stand.
The migration removes only matching stock visibility/completion/pose overrides
from earlier versions; edited rules and deliberately empty collections survive.
Use the offline upgrade command below. Characters already affected by the old
premature completion can be repaired, with the server stopped and a backup, using
`tools/sql/repair-honey-hand-in.sql` against the gameplay database. That repair
only clears the invalid completed/step-zero mark while the original request is
active and the character still carries honey; it never grants another reward.

Assets v40 (Client v31) adds the typed equipment minimum level (`EquipLevel`), projected once
from retained decoded Item.dat byte 113. This differs from the legacy `Level`
field. Player and pet equipment checks use this requirement; Radio Set requires
level 10. Repeated upgrades preserve administrator edits. Stop the server and
run the offline migration below before deploying this version.

Client v30 through v32 catalogs are recognized independently of quest schema versions.
They upgrade to v50 without losing existing definitions.

Assets v29–v38 add atomic native quest completion, temporary escorts/disguises, native quest NPC reactions and removal of two obsolete quest disable reasons. See [quest research and implementation](QUEST_RESEARCH.md) and the [complete 345-entry notepad list](QUEST_LIST.md). Gameplay now uses v21.

Assets v30 adds typed `ResourceRespawns` overrides for individual collection and
ground-item cooldown durations. Gameplay v21 adds `resource_cooldowns`, keyed by
resource kind, map and source ID. Each expiry is shared across players and commits
with its reward. Existing definitions and character timer history are preserved.
See [collection cooldown configuration](COLLECTION_SPOTS.md#shared-cooldowns-and-individual-durations).

Assets v29 on `Client-rewrite` enables complete native manual-collection events
whose old generated disable reason is now resolved. It preserves custom disable
reasons, unsupported branches, edited reward pools and intentionally empty data.
Stop the server and use the offline migration procedure below before deploying
this branch. Quest-development catalogs use a separate later schema; do not
downgrade or replace those databases with a Client catalog.

Assets v39 combines the quest and collection migrations. Client v30 and quest v38 catalogs can both be upgraded offline while preserving existing definitions. Upgrade older Client catalogs to main v30 before moving to this branch.

Assets v28 applies 93 owner-confirmed entity renames and preserves 85 confirmed existing names. Unchecked review proposals do not affect gameplay. See [enemy name review](ENEMY_NAME_REVIEW.md).

Assets v27 adds 129 researched enemy name corrections, bringing the compiled patch to 898 names. See [enemy name review](ENEMY_NAME_REVIEW.md).

Assets v26 adds normal special equipment for 20 freshly acquired companions. See [companion starting equipment](PET_SPECIAL_EQUIPMENT.md).

Assets v25 adds 162 collection-spot definitions, including 30 vacuum spots, with unresolved metadata retained explicitly. See [collection spots](COLLECTION_SPOTS.md).

Assets v24 additionally restores Pineapple Monster (NPC 17001).

Assets v23 restores 696 item display names with a preserving data migration. See [item name restoration](ITEM_NAMES.md).

Assets v22 includes 768 NPC name mappings: verified names, owner-approved single-option suggestions, six explicit owner choices and Quest-free display names. It preserves edited names and absent records. See [monster name restoration](MONSTER_NAMES.md).

Assets v19 adds typed stat-food compatibility definitions. Gameplay v20 adds a
persistent 1,200-use limit counter to each pet, initialized at zero for existing
pets. See [stat-boosting pet food](PET_FOOD.md).

Asset v18 adds SQL-owned pet initial-attribute ranges and desired totals, using
the owner's classic-WLO-first references. The migration preserves existing
definitions and owned pets. See [pet initial attributes](PET_INITIAL_STATS.md).

Asset v17 adds the typed NPC capture-restriction column from retained imported
Npc.dat byte 34. It preserves all existing NPC edits and does not replay seeds
on subsequent migrations. Runtime uses this column, never raw imported records.
Upgrade before starting the new server; existing owned pets are unchanged.

Stop the server and database editors first, and keep them stopped until the
command finishes. The migration preserves accounts, credentials, character state
and existing asset edits.

```sh
go run ./cmd/database-migrate -config config.local.json
```

The upgraded databases land at the paths the configuration already names
(`var/assets.db` and `var/wonderland.db` by default), so the configuration does
not change. The command opens each database read-only, takes an SQLite snapshot
with committed WAL data into a staging directory beside it (`.migrate-<stamp>`),
upgrades the copy, compares converted content, and checks SQLite integrity and
foreign keys. Only when both copies verify does it move each original, with its
`-wal`, `-shm` and `-journal` files, to `backups/pre-migrate-<stamp>/` beside it
and the copy into its place. It refuses to overwrite an existing destination.
A reported failure attempts to undo completed moves, removes staging files and
returns a nonzero exit status; restoration errors are included in the error.
The two replacements are separate filesystem operations: a process crash or
power loss during cutover can leave a mixed pair. Keep writers stopped and
inspect both configured paths and the `backups/pre-migrate-<stamp>/` directories
before retrying.

Build and start the server:

```sh
go build -o bin/wonderland ./cmd/wonderland
./bin/wonderland -config config.local.json
```

For rollback, stop the server and database editors, move the upgraded databases
and any SQLite sidecars to a separate location, then restore both originals and
their backed-up sidecars to the configured paths. The configuration paths stay
the same. Progress written after cutover will not exist in the original
snapshots. Keep the backups until acceptance testing finishes.

Gameplay schema v21 automatically upgrades older character storage transactionally
when `store.Open` opens a database. The offline upgrade command is the recommended
upgrade procedure because it leaves a complete original database available.
Asset conversion is explicit: runtime requires `catalog_schema` version 42 and
never falls back to source documents. Repeating conversion preserves typed edits.
The v1-to-v2 asset upgrade adds only `catalog_economy*` tables and source-derived
initial rules; it preserves every existing typed content table. Gameplay v9 adds
guild roles/icons and parcel escrow without replacing existing relationships.
The v2-to-v3 asset upgrade adds only `catalog_tents*` rules and default furniture.
The v3-to-v4 asset upgrade adds only `catalog_arcades*` tables and their presence
flag, with equal-weight defaults for supported paid machines (the original five
egg/slot kinds, plus fruit-machine seeds in the current migrator). Existing
content edits remain intact. See [MINIGAMES.md](MINIGAMES.md) for balance defaults.
The v4-to-v5 upgrade adds `catalog_terrains` and its presence flag. It projects
scene dimensions and collision-cell BLOBs from the imported Ground.MMG rows;
existing content and arcade edits remain intact. Runtime reads typed SQL only.
An older database without imported terrain retains maps without collision
grids; rebuild from the complete WLRI exports to supply those grids.
See [WORLD_SIMULATION.md](WORLD_SIMULATION.md) for movement policy.
The v5-to-v6 upgrade adds owned skill-targeting and offset tables plus their
presence flag. Existing skill rows retain their behavior until verified shapes
are authored; see [COMBAT_TARGETING.md](COMBAT_TARGETING.md).
Gameplay v11 adds shared map-prop cooldowns. Rewards and cooldowns commit together
and survive reconnects/restarts.
Gameplay v10 adds owned tent/furniture tables and per-character tent return
coordinates. Both upgrades preserve existing records and authored edits.
See [ECONOMY_SOCIAL.md](ECONOMY_SOCIAL.md) and
[ITEMS_PLAYER_STATE.md](ITEMS_PLAYER_STATE.md) for initialization and commands.

## Authoritative tables

Gameplay identity constraints remain in `characters`; mutable fields are in:

- `character_state`: stats, location, appearance, balances, preferences, active
  companions, saved return location, mute expiry and Lucky Draw day/usage.
- `tents`, `tent_items`: owner-specific homes, access locks and ordered furniture
  with full item metadata.
- `character_items`: bag, storage, equipment and pet equipment, with slot keys and
  binary item metadata.
- `character_skills`, `character_quests`, `character_timers`,
  `character_discoveries`, `character_pets`, `character_pet_skills`: ordered
  progress and owned collections. Foreign keys cascade when an identity is deleted.

`characters.state` remains an inert pre-migration snapshot for existing rows.
Runtime never reads or updates it. New characters leave that column empty.
Do not use it to inspect current progress or restore only that column.

Asset definitions use generated `catalog_*` tables with typed scalar columns,
ordered child tables and ownership foreign keys. There are no JSON columns in
these tables. Arrays of fixed protocol bytes use BLOBs. Nested events, branches,
operations, effects, modifiers, reward lists and movement steps have child rows.
`catalog_presence` preserves absent versus empty collections; `catalog_schema`
versions the projection. Items have one authoritative definition: gameplay item
lookups derive from the native item definition rows.

Models and query projections are in
`internal/assetsql/catalog_storage_generated.go`. Dataset names match
`assets.Catalog` fields; SQL column names follow the generated models. Inspect
`sqlite_master` or a database browser for exact names. For example:

```sql
SELECT native_items_key, value_definition_name, value_definition_level
FROM catalog_native_items
ORDER BY native_items_key;

SELECT lucky_draw_rewards_ordinal, value_id, value_quantity, value_weight, value_slot
FROM catalog_lucky_draw_rewards
ORDER BY lucky_draw_rewards_ordinal;

SELECT arcades_ordinal, value_kind, value_enabled, value_point_cost
FROM catalog_arcades
ORDER BY arcades_ordinal;

SELECT arcades_ordinal, value_index, value_item_id, value_quantity, value_weight
FROM catalog_arcades_rewards
ORDER BY arcades_ordinal, arcades_rewards_ordinal;

SELECT character_id, gold, lucky_day, lucky_used
FROM character_state;
```

Use validated administration editors for content changes. Direct SQL edits need
valid typed values and matching presence/order metadata; restart or issue the
relevant GM reload command to publish a new snapshot.

## Seeds and provenance

JSON exports under `data/` are offline initialization inputs. Startup reads SQL
only, in a consistent read transaction, and builds an immutable catalog snapshot.
It does not open original files, reconstruct native archives or reseed edits.

`asset_import`, `asset_documents`, `asset_records` and their legacy `asset_*`
views retain imported metadata, unknown fields and original source representations.
They are migration/provenance records and are not authoritative for gameplay or
administration after conversion. Their views can be stale after typed edits.
Editing them does not change the running definitions. Audio, images and video
payloads remain on disk; the server does not load them into its runtime tables.

## Explicit rebuild/reset

`data-rebuild` deliberately replaces asset definitions from maintained exports;
it does not preserve installation-specific content edits. Existing destinations
are backed up. Stop the server and database clients first; never delete active
SQLite sidecars to bypass its checks.

```sh
go run ./cmd/data-rebuild -config config.local.json
# Also erase accounts, characters and gameplay settings:
go run ./cmd/data-rebuild -config config.local.json -reset-gameplay
```

`-data` selects the export directory and `-assets-db` overrides the asset path.
The command verifies export hashes, creates and validates the structured catalog
before publication, and prints backups. Restore backups with the server stopped
if needed. A crash between two file replacements can leave a mixed pair.
Ordinary migrations and restarts preserve edits; deliberate rebuilds replace them.

## Importing Private Server accounts and characters

Use the separate read-only procedure in [LEGACY_IMPORT.md](LEGACY_IMPORT.md).
`database-migrate` upgrades Go database snapshots; `legacy-import` converts
supported original SQLite player data into a new Go database.

## Persistence scheduling

These databases own durable state; they do not require every online movement
packet to produce SQL writes. Assets are served from validated SQL-derived memory
catalogs. Sensitive mutable operations retain immediate atomic transactions.
Walking and resting recovery use dirty session checkpoints (default 30 seconds)
under the policy in [DEVELOPMENT.md](DEVELOPMENT.md#transaction-boundaries-and-session-checkpoints).
Checkpoints for these fields update guarded X/Y/HP/SP columns without rewriting
owned child rows; clean sessions need no SQL transaction. Gathering/world ticks
remain independent. Stop the server before snapshots/upgrades so pending session
state can flush and both databases represent the same installation.

Fishing rules are available through the `Fishing` dataset; see [FISHING.md](FISHING.md).


## Manufacturing and character metadata upgrade

Asset v9 adds typed `Manufacturing` and `RebornClasses` datasets. Compound2
records retain their original formula ordinals and all five ingredients, plan,
tool and duration fields. Conversion imports authored synthesis chances/fees and
preserves customized rates. Runtime reads only the typed catalog. Existing
v8 content edits remain intact; repeat conversion does not reseed edited tables.
Gameplay v14 adds nickname, actual class and potential, pet potential,
second-floor tent decoration metadata, and durable `manufacture_jobs` escrow.
See [manufacturing and character state](MANUFACTURING_CHARACTER_STATE.md).


Gameplay v15 adds `map_props.frame` with default one to preserve existing broken
nodes. Timed scripted shared frames now replay on entry and reset after 60 seconds.
Asset schema is now v50. The upgrade procedure upgrades existing older installations
without resetting accounts, characters, manufacturing jobs or typed content.

### Administration quest registry (asset v10)

The additive v9-to-v10 upgrade creates typed `catalog_quest_definitions*` tables
for the custom registry, with owned child rows for ordered steps, requirements,
rewards, prerequisites and actor metadata. Existing content edits remain intact.
It seeds native Mark.dat metadata from retained imported SQL records, using real
native IDs and names; it grants no guessed rewards and creates no gameplay state.
Repeated migration does not overwrite an edited or cleared registry. Runtime
uses only the typed catalog. See [quest administration](ADMINISTRATION.md#custom-quest-definitions-and-reachable-kill-counters).

### Inventory dimensions (asset v11)

The preserving upgrade adds `value_definition_cell_width` and
`value_definition_cell_height` to `catalog_native_items`. It seeds them from
retained SQL `value_record` bytes at disk offsets 406/407. Existing names,
metadata and other authored definitions remain unchanged. Normal startup reads
typed dimension columns; it does not regenerate or overwrite them.

Before deploying this version, stop the server and run the upgrade above
(`go run ./cmd/database-migrate -config config.local.json`). Gameplay remains
schema v18. Inventory anchors are preserved; old overlapping placements require
an explicit correction and are never silently discarded or repacked.

### Instance definitions (asset v12)

The preserving upgrade adds `catalog_instances` and its catalog presence flag.
Definitions are seeded from retained SceneData.dat and Mark.dat SQL import rows,
including native titles/descriptions, capacity, level, guild requirement, time
limit, entry map and coordinates. Existing catalog edits remain intact. Runtime
reads use these typed rows. Lobby membership remains in memory; verified dungeon
objectives and rewards are pending. Follow the offline upgrade procedure above to
upgrade an older installation before running the new server.

### Fruit machines (asset v13)

The upgrade appends missing seed definitions to `catalog_arcades*`, including the
three fruit machines (sport kinds 11, 12 and 14), after the existing rows.
Existing machines, including edited costs, weights and disabled states, remain
unchanged. A deleted egg/slot kind is also reseeded by this upgrade; keep an
explicit disabled row to preserve its unavailability. Machines whose prize
items are missing are added disabled. Follow the offline upgrade procedure above
before running the new server; runtime requires v50. See
[fruit machines](MINIGAMES.md#fruit-machines).

### Private Server update content (asset v14, gameplay v18)

Boxes, travel, map transitions, NPC shops/prices, skill evolutions, gathering,
tent tools/surfaces and chests each own their initialization, validation and
preserving migration in `internal/assetsql`. The catalog runner coordinates these
steps within the versioned SQL transaction.
Gameplay migrations likewise live with teleport points, tent tools, manufacturing
and bazaars in `internal/store`; its version runner retains the common transaction.

Asset v14 adds typed box rewards, skill evolution links, EVE gathering pools and
timers, NPC buy prices/catalogs, tent surfaces/tool hosts, chest classifications,
travel destinations and map-exit rewards. Runtime and administrator edits use
these tables. NPC buy prices are projected from retained decoded Item.dat rows
at offset `0x72` during initialization, supplementing the authored CSV fallback.
Existing typed price edits remain authoritative; runtime never rederives prices. The initializer never reloads their JSON/CSV inputs during gameplay.
It appends available new chest categories and changes only exact prior tent and
poison defaults; administrator content and owned tents remain intact.
It also supplies the previously missing NPC event-route child tables and sign/speed
columns from map sections retained in SQL, matching click IDs so edited NPC names,
positions and walking steps remain intact. Chest rewards gain a typed gold column.

Gameplay v18 adds `character_teleport_points`, `tent_tools`, manufacturing input
escrow and native batch/cancellation fields, plus `bazaar_stalls` and
`bazaar_listings`. All are typed, owned or related SQL rows. Upgrades retain
accounts, balances, jobs, inventory, quests and server settings. Existing legacy
manufacturing jobs retain pause/resume semantics. The source development database's
accounts, settings, stock and administrative vendor row are not seeded here.

### Story policies (asset v15)

The preserving upgrade creates `catalog_story` and its typed child tables for
rules, scopes, predicates, repairs, program guards, recruitment, transformations,
reward checkpoints and cinematic cues. It seeds current story behavior only when
the root table is new; repeat upgrades retain edits and deliberately empty
collections. Gameplay remains v18, with existing quest progress unchanged.
Runtime requires v50 and reads only SQL definitions. The `Story` administration
dataset and `/reload quests` publish validated policies through the existing
interaction ownership gates. See [story execution and authoring](STORY_SYSTEM.md).

### Rebirth checkpoints (asset v16)

The upgrade adds the rule mark-list presence column and
`catalog_story_rules_marks` owned table. It appends the new
`job_palace_rebirth_checkpoint` policy only to nonempty rule collections that
lack that ID. Existing rules, ordering, overrides and other story families are
preserved; intentionally empty rules stay empty. Initialization of the new
column records that this step ran, so later upgrades retain removed policies.
Schema and data changes roll back together on failure. Fresh databases receive
the current complete seed. Gameplay remains v18; character repairs run through
owned transactions at rebirth or character selection, rather than during the
assets migration. The source development database and its player rows are not
imported.

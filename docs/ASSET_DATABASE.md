# Server databases and migration

Shared definitions and administrator content edits live in `assets_database`.
Accounts, characters, relationships, progress, audit and persistent server
settings live in `database`. Defaults are `var/assets.db` and
`var/wonderland.db`. See the [ownership policy](DEVELOPMENT.md#server-data-ownership-and-initialization).

The server builds and initializes from the repository alone. Client extraction
is needed only for the Go client's rendering assets or an optional content import.
First startup, including `-inspect-data`, creates a missing asset catalog from
the embedded [server seed](../internal/assetsql/server_defaults/README.md).
Existing asset catalogs are never replaced or silently upgraded at startup.

## Upgrade an existing installation

Current versions are **asset schema 71** and **gameplay schema 25**, defined in
[`catalog_migration.go`](../internal/assetsql/catalog_migration.go) and
[`migrations.go`](../internal/store/migrations.go). Asset v62 adds typed item-card
reward/gender mappings. Gameplay v22 adds durable public message-board posts;
v23 preserves those posts and adds a monotonic ID allocator to prevent stale
requests from addressing a different notice after deletion.
Asset v64 adds the typed item `value_definition_indestructible` flag. Its
preserving migration seeds starter outfits and the six rebirth cape series from
`internal/assetsql/indestructible_equipment_defaults.json`. Later administrator
edits are retained. Existing installations must run the normal database migration
before starting the updated server.

Asset v71 installs the missing Hill Pepper quiz completion-branch priority in SQL.
The migration adds only this rule, preserving existing administrator priorities
and edited or removed completion policies. Fresh installations include the rule
in the repository seed; runtime startup does not reload the JSON defaults.

Asset v66 removes Notepad from starter grants so it is obtained through the
Astrology lady's native event. Other starter grants and existing characters'
inventories are preserved.

Asset v69 corrects Training Voucher 34181 from the former one-hour default to
six hours, following its English/stored description. Only the previous default
is changed; different administrator durations/kinds and existing character timers
are preserved. Training Fare remains one hour. Asset v70 similarly updates Training Ticket
34253 to six hours by administrator policy, preserving non-default overrides.

Asset v68 adds typed timed-item kind, duration and EXP multiplier columns, plus
Training Island entitlement/entry metadata. Gameplay v25 stores remaining online
EXP and Training Island seconds and the EXP pause state in `character_state`.
Activation/consumption is transactional; countdown checkpoints do not rewrite
inventory. Existing characters begin without timed effects.

Asset v67 adds map-specific battle recovery and flat victory EXP columns in
`catalog_maps`: `value_battle_round_recovery_hp`, `value_battle_round_recovery_sp`
and `value_battle_victory_exp`. Training Island maps receive 100 HP/50 SP recovery
and 200 unscaled EXP per contributor. Other map data is preserved; later
administrator edits survive migration and initialization.

Asset v65 stores manufacturing reset baselines in
`catalog_manufacturing.value_default_duration_seconds` and furniture floor-switch
flags in `catalog_native_items.value_definition_tent_floor_switch`. Migration
preserves existing recipe durations as their baseline; subsequent duration edits
do not change that baseline. Repository-only installations support reset without
`compound2.dat` archive records. Floor-switch flags identify stairs, ladders and
elevators independently of translated item names; later administrator edits survive.

Asset v63 adds the SQL-owned `account_ground_once` policy for Newbie Island.
Gameplay v24 stores permanent account/node claims transactionally with pickups.
All Newbie Island map variants share the same claim namespace; each account can
collect each authored material once across its characters, without respawn.
Player-dropped items retain their normal behavior. Old shared cooldown rows are
ignored for these personal nodes; past collectors cannot be inferred from those
rows, so existing accounts begin with no permanent claims after this upgrade.
Earlier supported revisions are handled by the same
migrator; the old Client/Quest branch-specific upgrade sequence is obsolete.
Do not downgrade a newer catalog to match an older executable.

1. Stop the server and all database editors so buffered session state flushes.
2. Update the checkout to the revision you intend to run.
3. Run the migration from that checkout at the repository root:

   ```sh
   go run ./cmd/database-migrate -config config.local.json
   ```

4. Rebuild the server and restart it with the same configuration. On Windows,
   use `bin/wonderland.exe` instead of `bin/wonderland`:

   ```sh
   go build -o bin/wonderland ./cmd/wonderland
   ./bin/wonderland -config config.local.json
   ```

Keep the normal admin-token environment when restarting. This command works
from Bash or PowerShell; it does not require client extraction. Migrate with
the current source rather than an old `bin/` utility. `unsupported structured
asset schema` means the catalog and executable disagree; rebuilding alone
cannot upgrade a catalog.

The migrator preserves accounts, credentials, character progress and authored
content edits. Default initialization fills missing definitions according to
each versioned migration; intentionally empty tables and existing edits remain
authoritative. New game defaults require preserving migrations, not replacing
an administrator's catalog with the repository seed.

### Backups and recovery

The command snapshots both configured databases read-only, including committed
WAL data, into staging directories beside them. It migrates the copies, compares
converted content and checks SQLite integrity and foreign keys before cutover.
The original files and their SQLite sidecars move into
`backups/pre-migrate-<stamp>/` beside the configured databases. Paths in the
configuration remain unchanged.

A reported failure attempts to undo completed moves and reports restoration
errors. Replacing two files is not one filesystem transaction: a crash during
cutover can leave a mixed pair. Keep writers stopped and inspect both configured
paths and backup directories before retrying. Never delete active WAL/SHM files
to bypass a migration error.

For rollback, stop all writers, move the upgraded databases and sidecars aside,
then restore both originals and their backed-up sidecars to their configured
paths. Progress written after cutover is absent from the old snapshots. Keep a
complete backup until acceptance testing finishes.

Gameplay storage can upgrade transactionally through `store.Open`; the offline
two-database procedure above is recommended because it keeps the original pair.
Asset runtime requires the current structured schema and reads no legacy JSON
document fallback. Feature migration implementations retain the older schema
history; current damage rules are documented in [DAMAGE.md](DAMAGE.md), not the
superseded v55 contact-multiplier formula.

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

Use migration for upgrades. `data-rebuild` deliberately replaces asset content
and discards installation-specific definition edits, keeping a backup first.
Stop all writers before running it.

```sh
# Recreate assets from the tracked repository seed; retain gameplay storage.
go run ./cmd/data-rebuild -config config.local.json

# Recreate assets from a verified local extraction instead.
go run ./cmd/data-rebuild -config config.local.json -data data

# Also erase accounts, characters, settings and audit. Use only for an intended reset.
go run ./cmd/data-rebuild -config config.local.json -reset-gameplay
```

Without `-data`, no exported manifest or client installation is needed. With
`-data`, the importer verifies the supplied export before publication.
`-assets-db` explicitly overrides the catalog destination; gameplay reset still
uses the configured `database` path. Review the printed paths and backups.
The same mixed-pair crash/recovery considerations apply when resetting both
databases. Ordinary startup and preserving migration are not reset operations.

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


## Content-specific definitions

Use the owning guide for current behavior and content configuration:

| Definitions | Guide |
| --- | --- |
| Story policies, native continuation and recruitment | [Story system](STORY_SYSTEM.md) |
| Collection spots and per-resource shared cooldowns | [Collection](COLLECTION_SPOTS.md) |
| Skill power, contacts and grade limits | [Damage](DAMAGE.md), [Skill.dat](SKILL_DAT.md) |
| Native map backdrop regions | [Backdrops](BATTLE_BACKDROPS.md) |
| Pet initial stats, food and starting equipment | [Initial stats](PET_INITIAL_STATS.md), [food](PET_FOOD.md), [equipment](PET_SPECIAL_EQUIPMENT.md) |
| Mall defaults and container pools | [Mall](LEGACY_MALL_RESEARCH.md) |
| Card reward and gender mappings | [Item cards](ITEM_CARDS.md) |
| Manufacturing and character metadata | [Manufacturing](MANUFACTURING_CHARACTER_STATE.md) |
| World PK reward pools | [World PK](WORLD_PK_REFERENCE.md) |

Do not use old per-version walkthroughs as current schema or gameplay guidance.
The version runner and feature migration tests describe the preserving steps;
the repository seed manifest records the distributable catalog's exact version,
checksums and contents.

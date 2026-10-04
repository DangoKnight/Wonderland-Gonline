# Server databases and migration

Shared game definitions and administrator content edits live in the configured
`assets_database`. Accounts, characters, mutable gameplay state, relationships,
audit records and persistent server settings live in `database`. Defaults are
`var/assets.db` and `var/wonderland.db`. See the
[ownership policy](DEVELOPMENT.md#server-data-ownership-and-initialization).

## Upgrade an existing installation

Stop the server and database editors before taking snapshots. Keep them stopped
until configuration points to the upgraded copies. The migration preserves
accounts, credentials, character state and existing asset edits.

```sh
go run ./cmd/database-migrate -config config.local.json -output-dir var/migrated-v4
```

The output directory must be new. The command opens source databases read-only,
takes SQLite snapshots with committed WAL data, upgrades the copies, compares
converted content, and checks SQLite integrity and foreign keys. It never
replaces the originals. Failure removes the new output directory and returns a
nonzero exit status. Taking both snapshots while the server is stopped ensures
they represent the same installation state.

Set these startup parameters to activate the verified copies:

```json
{
  "database": "var/migrated-v4/wonderland.db",
  "assets_database": "var/migrated-v4/assets.db"
}
```

Keep other configuration fields. Build and start the server:

```sh
go build -o bin/wonderland ./cmd/wonderland
./bin/wonderland -config config.local.json
```

For rollback, stop the server and restore the original configuration paths.
Progress written to the upgraded database after cutover will not exist in the
original snapshot. Keep both copies until acceptance testing finishes.

Gameplay schema v10 automatically converts older character rows transactionally
when `store.Open` opens a database. The offline copy command is the recommended
upgrade procedure because it leaves a complete original database available.
Asset conversion is explicit: runtime requires `catalog_schema` version 4 and
never falls back to source documents. Repeating conversion preserves typed edits.
The v1-to-v2 asset upgrade adds only `catalog_economy*` tables and source-derived
initial rules; it preserves every existing typed content table. Gameplay v9 adds
guild roles/icons and parcel escrow without replacing existing relationships.
The v2-to-v3 asset upgrade adds only `catalog_tents*` rules and default furniture.
The v3-to-v4 asset upgrade adds only `catalog_arcades*` tables and their presence
flag, with equal-weight defaults for the five supported paid machines. Existing
content edits remain intact. See [MINIGAMES.md](MINIGAMES.md) for balance defaults.
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

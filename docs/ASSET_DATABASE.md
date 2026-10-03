# Asset SQL database and gameplay reset

The SQL asset catalog lives in `var/assets.db`. The gameplay database remains
`var/wonderland.db` (or the `database` path in the selected server config).

The asset database is a generated copy of the maintained `data/` exports. This
makes decoded data searchable and supports joins across items, NPCs, skills,
scenes and quest marks. Original JSON documents and unknown fields are retained.
GORM owns import writes and schema creation; explicit SQL is limited to schema
views, integrity checks and SQLite capability checks.

Audio payloads remain outside the database. Their indexes and file references
are available in SQL, and rebuilding does not require reading the audio files.
For initial setup, see [GETTING_STARTED.md](GETTING_STARTED.md). Runtime settings
and behavior tuning are documented in [CONFIGURATION.md](CONFIGURATION.md).

## Runtime data source

Server startup and `-inspect-data` read exclusively from `assets_database`
(default `var/assets.db`). Remove `item_catalog` and `data_directory` from older
configs. Accounts, characters and audit records remain in the separate
`database`; configuration rejects paths that identify the same database file.

GORM opens assets read-only without creating a missing database. One read
transaction builds a consistent in-memory catalog for all gameplay handlers:
items, NPCs, skills, dialogue, maps/events, animation timings, marks, alchemy,
starter grants, pet vouchers, drops, sale prices, critical hits, mall, gacha, daily Lucky Draw and
forging families/eligibility.
Packet handlers use this snapshot. Restart the server after SQL edits or imports.
Missing required documents, invalid schemas and invalid required catalogs stop
startup; there is no file fallback. The existing unavailable gacha-item warning
still disables that optional table.

`asset_records.json` is authoritative for indexed arrays, ordered by `ordinal`.
Deleted rows are omitted when loading; the original document array does not
restore them. Edits must preserve catalog validity: item positions, source count
and IDs must agree with export metadata, and archive indexes must match blocks.
Invalid edits stop startup. CSV settings use indexed `rows`, JSON settings use
indexed `value` arrays, and text settings use `asset_documents.json`'s `text`.
Object-valued JSON settings and export metadata also come from `asset_documents`.
EVE maps and animation records retain their plaintext bytes in SQL: edit
`decoded_hex` and the corresponding index/header metadata together. Their
readable section projections remain inspection aids. Numeric NPC/skill/mark
fields and readable item definitions are loaded directly from SQL JSON fields.

The original export files are used only by offline import/export and reference
tests. The `assets.Load` reference API is not used by server startup.

## Rebuild procedure

Stop the server and close database browsers before rebuilding. The command
rejects SQLite journals/WAL sidecars, concurrent rebuilds, source-data
replacement, incorrect database roles and, on Linux, open database file handles.
Never remove active WAL/SHM files to bypass this check. The server and external
clients must stay stopped throughout publication; sidecar checks alone cannot
prove that every SQLite client on every platform is offline.

Recreate only the asset catalog:

```sh
go run ./cmd/data-rebuild
```

Recreate the asset catalog and reset the configured gameplay database:

```sh
go run ./cmd/data-rebuild -reset-gameplay -config config.local.json
```

If using the example configuration, substitute `config.example.json`. Omitting
`-config` selects the built-in default gameplay database. Optional flags:

- `-data`: exported data directory; default `data`.
- `-assets-db`: override the configured asset database path.
- `-reset-gameplay`: recreate gameplay storage with the current server schema.
- `-config`: configuration selecting assets and gameplay storage.

A gameplay reset erases live accounts, characters, relationships, deletion
credentials, settings and audit records. Original account databases are excluded
from the asset catalog. New gameplay accounts can be created through the
existing administration interface after restarting the server.

The rebuild prepares and validates fresh databases before replacing the current
files. Every existing destination becomes a unique sibling backup, for example
`wonderland.db.backup-20261001T...-...`. The command prints the actual paths.
Checksum failures leave current databases untouched. Publication errors restore
files already replaced. Database files are private (`0600` on Unix).

The two file replacements are separate operations. An operating-system crash
between them can leave a mixed pair. Keep the printed backups until the new
files have been inspected. With the server stopped, restore a backup to the
configured destination if needed. Backups, databases and temporary files are
under the repository's ignored `var/` directory by default.

## SQL schema

| Table/view | Purpose |
| --- | --- |
| `asset_import` | Import schema version and exact manifest JSON/hash |
| `asset_documents` | All exported JSON documents, exact bytes and provenance |
| `asset_records` | Indexed collection rows, IDs, names and full record JSON |
| `asset_items` | Client item IDs, names, types, slots and levels |
| `asset_npcs` | NPC IDs, names, levels, HP, SP and elements |
| `asset_skills` | Skill IDs, names, SP and elements |
| `asset_dialogues` | Dialogue IDs and text |
| `asset_scenes` | Scene IDs and names |
| `asset_quest_marks` | Quest IDs, names and completion flags |
| `asset_audio` | Audio filenames, offsets and sizes; no audio payload bytes |

`asset_records` uses `(asset, collection, ordinal)` as its primary key. Duplicate
IDs and duplicate authored rows are preserved. `(asset, game_id)` and `name`
are indexed. Each JSON document remains intact, so all nested arrays, field
layouts, headers, raw bytes, comments and authored metadata remain accessible.
This is a lossless catalog with useful SQL projections; it is not yet a fully
normalized gameplay schema with relationships for every unresolved native field.

Original account databases are excluded from the asset export and import.
The catalog contains static resources; player-owned state belongs to gameplay storage.

### Query examples

Open `var/assets.db` in a SQLite browser or use the `sqlite3` command:

```sql
SELECT id, name, level, equip_slot
FROM asset_items
WHERE name LIKE '%Sword%'
ORDER BY level, id;

SELECT id, name, level, hp, sp
FROM asset_npcs
WHERE level >= 50
ORDER BY level, id;

SELECT n.id, n.name, s.name AS first_skill
FROM asset_npcs AS n
LEFT JOIN asset_skills AS s
  ON s.id = json_extract(n.json, '$.fields.skill_1');

SELECT json_extract(json, '$.value') AS authored_settings
FROM asset_documents
WHERE asset = 'critical_hits.json';

SELECT game_id, json_extract(json, '$.fields.unknown_u16_offset_14')
FROM asset_records
WHERE asset = 'npc.dat' AND collection = 'records';
```

SQLite's `json_extract`, `json_each` and `json_tree` give SQL access to remaining
fields and nested collections. See the [SQLite JSON documentation](https://www.sqlite.org/json1.html).
The importer verifies JSON support in the actual bundled SQLite library.

## Design tradeoffs

A generated asset database is useful for inspection, validation, joins and
runtime loading. Keep JSON exports as the reproducible source, and load gameplay
lookups into memory when the server starts. Repeated SQL queries inside packet
handling would add work compared with existing in-memory lookups.

A separate asset database allows rebuilding static content without erasing
players. Preserving complete JSON plus indexed records duplicates some content,
so this initial catalog is larger than a later normalized schema could be.
Updates to JSON require a rebuild. Direct SQL edits will be overwritten by the
next import; edit the maintained source or a future explicit override layer.
Audio has no server gameplay role and would inflate storage and backup time.


## Daily Lucky Draw rewards

`asset_lucky_draw_rewards` exposes ordered rows with `item_id`, `quantity`,
`weight` and `slot`. Runtime resolves `asset_records` for
`asset='lucky_draw.json' AND collection='value'`; document JSON is provenance.
Weights are relative positive integers. All 14 initial rows use weight 1,
including separate quantities of the same event item. Player allowance lives
in gameplay character JSON, outside the asset database.

Regenerate with `python3 tools/data_export/lucky_draw.py --output data --overwrite`,
then rebuild the asset database and restart. The exporter validates compatibility
policy against fresh sibling Item.dat. Older installations must rebuild to add
`lucky_draw.json`; startup has no embedded or native-file fallback.

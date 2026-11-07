# Repository server definitions

`catalog.db.gz` is a compressed SQLite seed containing the server's structured
shared catalog at the schema version recorded in `catalog.db.gz.json`. It is
embedded in the server and used only to create a missing assets database or by
an explicit default `data-rebuild`. Existing SQL definitions and administrator
edits remain authoritative. No client installation or extraction is required.

The seed contains items and native item metadata, NPCs, skills and effects,
animation timings/contact counts, dialogue, maps, events, warps, collision grids,
backdrop regions, recipes, shops, quests and other shared gameplay definitions.
It contains no accounts, characters, player progress, server secrets, images,
audio, video or original archive/import payloads. Empty import tables retain
the database's existing role checks. The manifest records the exact database
checksum, byte sizes, table/row counts and definition census.

Refresh from an offline, current-schema assets database after intentionally
changing the distributable definitions:

```sh
go run ./tools/server-seed-export -database PATH_TO_ASSETS_DB
```

The exporter refuses gameplay databases, strips all non-catalog tables and
source-specific import paths, and verifies that every shared catalog value is
unchanged after stripping. Commit the compressed seed and its manifest together.
Review any intentional content changes; do not export a live or private gameplay
database. Existing installations receive new defaults through preserving,
versioned migrations rather than replacing their catalogs with this seed.

Contact counts are independently tracked in `../skill_contact_defaults.csv` with
source hashes. To refresh that small table from a legally supplied client export:

```sh
python tools/data_export/extract_skill_contacts.py --animation-export data/animation_data.json --output internal/assetsql/skill_contact_defaults.csv
```

Native skill grade limits are independently tracked in `../skill_grade_defaults.csv`.
Refresh them before migrating and regenerating the SQL seed:

```sh
python tools/data_export/extract_skill_grades.py --skill-export data/skill_data.json --output internal/assetsql/skill_grade_defaults.csv
```

Both tables include portable provenance/checksum files. Grade zero is an explicit
zero limit. Only an absent definition uses the compatibility fallback of ten.

Extraction is an offline maintainer operation. End users need only the repository,
Go and a working CGO/C compiler to build and run the server. See
`docs/GETTING_STARTED.md` for server startup and optional client extraction.

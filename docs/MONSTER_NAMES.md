# Full monster names

The authored name patch covers **898** NPC names from the owner's
12 saved [WLOHub monster database](https://wlohub.com/monsters) pages. The pages
advertise 1,190 entries; extraction checks that all 1,190 are present, with
consecutive page numbers and no duplicate source IDs. Website IDs identify
WLOHub entries, not native NPC IDs.

The extracted factual table is
[`monsters.csv`](../tools/data_export/reference/wlohub/monsters.csv). It retains
names, elements, levels, vitals, SPD, attributes and capture labels. The page
filenames/checksums and native-export checksum are recorded in
[`sources.json`](../tools/data_export/reference/wlohub/sources.json). Reference
values are evidence for matching names; they do not replace combat stats,
drops, capture restrictions or other gameplay definitions.

## Matching and unresolved records

The original 558 automatic matches require an existing native name of exactly ten bytes, a full
name beginning with that prefix, and an exact match for level, element and all
five base attributes. Multiple matching website records are accepted only when
they agree on the full name. The owner additionally approved all 192 review
entries with one proposed name despite differing reference stats. Removing
`Quest` annotations makes another 12 formerly different options identical;
these are accepted as well. Whitespace is normalized and the word `Quest`,
including `(Quest)`, is removed from display names in both `monsters.csv` and
the compiled patch. Original source names remain in the saved HTML; the patch
retains previous names only where needed to upgrade already-patched databases.

The owner resolved the final ambiguous records explicitly in
[`name_choices.csv`](../tools/data_export/reference/wlohub/name_choices.csv):
14469 becomes Inspecting Soldier; 17738/17739 become Half Beast Man;
30310/30334 become Yellow Rock Monster; 40089 becomes Tyrannosaur Lord.
NPC 17001 additionally becomes Pineapple Monster through an explicit short-name override. The extractor applies this authored table and checks that each choice is a
reference candidate. It can be overridden with `--name-choices`.

The resulting ID-specific table is
[`npc_names.csv`](../internal/assets/npc_names.csv). Each row retains the original
name, supporting website IDs and any previous Quest-labelled display name.
The original ten-byte prefix review is resolved;
[`review.csv`](../tools/data_export/reference/wlohub/review.csv) contains only its
header. The broader [enemy name review](ENEMY_NAME_REVIEW.md) adds 129 researched
corrections, including shorter names and missing leading/middle words, through
`enemy_name_resolutions.csv`. It also records unresolved candidates separately
in `enemy_name_review.csv`. NPCs without sufficient evidence remain unchanged.
This is a supported subset, not a claim to restore every NPC name.

## Builds and databases

The CSV is embedded into Go builds. The Gonline client applies the ID-specific
display patch when loading NPC templates, including battle enemies. A name is
updated only if it still equals the recorded original or previous Quest-labelled name; other full/custom names
are preserved. Raw native records and the lossless exported JSON are untouched.
No asset re-extraction or online lookup is required for the new client build.

Server gameplay reads names exclusively from `catalog_n_p_cs.value_name` in
the assets database. Fresh offline asset conversion seeds the verified names.
Assets migration **v27** adds the 129 researched corrections. The earlier v24
adds the Pineapple Monster correction to v22 and its six explicit choices to v21's approved
suggestions and Quest cleanup. It conditionally updates original/previous names and leaves
administrator edits, absent records and unrelated fields unchanged. The compiled
table is used for offline initialization/migration, never a runtime server
fallback. Repeated migration does not replay the seed. Existing player-owned
pet names are not rewritten.

Upgrade existing assets with the backed-up offline procedure in
[ASSET_DATABASE.md](ASSET_DATABASE.md). The local assets-only upgrade was
verified against all 898 rows; its v27 original database is in
`var/backups/pre-migrate-20261104-064512`. The previous v24 original is in
`var/backups/pre-migrate-20261104-054909`. The v21 backup remains at
`var/backups/pre-migrate-20261104-052954`; the earlier v20 backup remains at
`var/backups/pre-migrate-20261104-052141`. Gameplay data was not migrated.

## Reproduce

Use saved, unfiltered WLOHub pages covering the complete database, plus an
unmodified `npc_data.json` generated from the native client assets. Inputs can
reside anywhere; the reference pages need not use the owner's directory layout.
No network access or third-party Python modules are required.

```powershell
python tools/data_export/extract_monster_names.py --pages /path/to/saved/pages --npc-export /path/to/npc_data.json --accept-single-candidates --strip-quest --output-dir /path/to/output
```

Alternatively, `--assets-db /path/to/unpatched/assets.db` reads a structured
database in read-only mode. Use the native export for stable regeneration after
upgrading SQL names. The extractor emits `monsters.csv`, `npc_names.csv`,
`review.csv` and `sources.json`. Review changes before replacing authored tables.
For the supplied inputs, all three CSV files reproduce byte-for-byte.

Tests cover HTML entity/whitespace decoding, overlapping pages, ambiguous and
mismatched candidates, authored name examples, preserving custom SQL names,
missing NPCs, repeat migrations and client template loading. See
[TESTING.md](TESTING.md).

# IGG English client overrides

The English IGG version-six installation is the primary translation reference.
The older IGG installation was also compared: all its native IDs occur in the
newer installation, so it contributes no additional overrides. The importer can
retain older installations as fallbacks for IDs absent from the primary source.

The current patch contains **10,894 verified text entries**. These include IGG
names identical to the modern text when needed to suppress a different fallback
name correction:

| Table | English records examined | Name overrides | Description/text overrides |
| --- | ---: | ---: | ---: |
| NPCs/monsters | 4,101 | 2,379 | — |
| Items | 4,219 | 1,560 | 2,491 |
| Skills | 799 | 44 | 428 |
| Scenes | 1,063 | 233 | — |
| Dialogue | 14,711 | — | 2,952 |
| Quest marks | 1,632 | 509 | 298 |

## Matching and precedence

Explicit owner-requested dialogue display corrections are compiled after the
translation lookup, guarded by native talk ID and exact text. Noa's quiz answer
records 57166, 57171, 57174, 57181 and 57184 omit the trailing answer-revealing
asterisk. The extracted records, question option IDs and server scoring remain
unchanged; other asterisks and custom translations are preserved. These entries
are data in [`dialogue_overrides.csv`](../internal/clienttext/dialogue_overrides.csv),
not hardcoded character/quest rules in the game.

[`igg_overrides.csv`](../internal/clienttext/igg_overrides.csv) stores native ID,
field, exact modern source text, English replacement, matching evidence, source
installation and SHA-256 fingerprints of both decoded records. The original
binary files and lossless JSON exports remain available as provenance. No raw
record bytes or gameplay values are changed by the display patch.

NPC identity checks compare native ID, type, body sprite, inventory icon, portrait
and element. Item checks compare ID, type, both icons, equipment overlays and
equipment slot. Skill checks compare ID, type, element, attack category, effect
layer, target policy and icon. Scene IDs and environment categories must agree.
Descriptions additionally require identical non-text record bytes. Quest marks
require identical non-text metadata, including their completion flags.

Dialogue requires identical ID/non-text metadata and at least one matching
event/branch/operation, question/option or cinematic line reference. Cinematic
matches include movie identity, speaker, line index and stage. All 640 STY files
in IGG version six and 456 in the older installation decoded successfully.
Format placeholders must agree. This leaves 441 differing version-six dialogue
records in the review report because their shared context could not be verified.

Verified IGG translations take first priority, including over authored full-name
patches and earlier owner-confirmed NPC names. Those existing corrections are
fallbacks wherever IGG has no verified match. IGG spelling, capitalization and
shorter names take priority as well. Repeated spaces in names are normalized;
`(quest)` annotations are removed. Each offline compilation
lookup also requires the exact verified source text, preserving later custom
translations. IDs absent from IGG retain their modern content.

The English installation has wider NPC and item names: NPC records are 148 bytes
with a 20-byte name, versus modern 138-byte records with 10-byte names; item
records are 457 bytes with a 20-byte name, versus 451/14. Integer cipher fields
after these names shift by 10 and 6 bytes respectively. The importer decrypts
and round-trip checks these layouts independently, then normalizes offsets only
for comparison. Some English names remain limited by IGG's own storage capacity.

## Client integration

The offline asset tools apply the CSV tables to the text fields of NPC, item,
skill, scene, dialogue and quest-mark records. Both optimized and ordinary bundle
compilation resolve English overrides offline and place the final strings in the
English language catalog. Text fields contain stable references such as
`@{talk.57166.text}`. Cache
reuse includes the translation-table fingerprint, so changing a CSV invalidates
the affected text entries even when the extracted source files have not changed.
Original source hashes retain their original meaning in the bundle manifest.

For loose development assets, run from the repository root:

```powershell
go run ./cmd/asset-build -data data -text-only
```

This generates six keyed documents in `data/runtime/text` and language catalogs
in `data/runtime/locales/en.json` and `zh-TW.json`. The normal
`asset-build -data data` also compiles them after verifying the extraction; use
`-text-only` when updating text in an intentionally edited development tree.
The two compiled language catalogs are tracked repository assets, formatted with
sorted keys and one entry per line. Include regenerated catalogs with changes to
their sources. The keyed record projections and other extracted assets remain
ignored. See [the compiled catalog workflow](../data/runtime/locales/README.md).
Source JSON files, source checksums and native decoded bytes are preserved.
The client prefers these generated documents and resolves their keys once while
loading typed definitions. It does not match source strings during gameplay.
Without compiled text, loose assets show their original source text.

For packaged assets, rebuild the bundle:

```powershell
go run ./cmd/client-bundle -source data -output bin/client-assets.zip -contract client/asset-contract.json
```

Bundle compilation always reads the original exports and ignores old loose text
projections. Restart the client after recompilation. It contains no IGG/Noa
override tables or matching logic and does not import `internal/clienttext`.
Changing a translation requires asset compilation, not a game executable rebuild.
No extraction or database migration is needed. Server definitions continue to
come from their SQL catalog.

## English and Traditional Chinese

The sidebar's **Settings** panel (F10) has a **Language** selector. Changes are
saved with the workspace's display preferences in `display.json` and apply
immediately when confirmed. All open sessions refresh their UI and source text
in place; characters remain connected and active conversations retain their
choice IDs without replaying audio or sending event replies. A command-line override
is also available from the client directory:

```powershell
go run . -language en
go run . -language zh-TW
```

The selected language is shared by all sessions in the same workspace, including
sessions opened after a live change. Client
labels and generated notices use explicit `ui.*` keys in
`internal/localization/ui_en.json` and `ui_zh-TW.json`. Native asset text uses
`<table>.<id>.<field>` keys. Unmatched Chinese entries fall back to English.
Server messages and player-entered names/chat are never translated or matched
against these tables. Protocol IDs, raw records and server quest logic are unchanged.

Import the original Chinese text and raster interface labels from an
untranslated installation (read-only):

```powershell
python tools/data_export/extract_chinese_text.py --client "PATH\TO\CHINESE_CLIENT" --target data --media-output data/media
go run ./cmd/asset-build -data data -text-only
```

`internal/clienttext/chinese_text.csv` preserves the imported text, native IDs
and source/target record hashes. Matching requires compatible non-text identity;
descriptions additionally require matching non-text record contents. The current
installation contributes 45,608 fields. The media option exports the original UI
images under `media/locales/zh-TW/menu/Skins`, retaining their logical names and
including backgrounds as well as buttons. The initial import contained 1,436
BMP images. New imports also include JPEG/PNG and other supported image sources,
normalized to PNG. Requested artwork takes priority, with per-file fallback to
the installed alternative; either English or Chinese UI can be installed alone.
See [UI import selections](ASSET_IMPORT.md#select-ui-artwork-languages).
Fonts use explicit UTF-8 to Big5 conversion at rendering
boundaries, including the area-title mask.

Rebuild the bundle after changing source CSVs, UI catalogs or locale artwork.
Both catalogs and locale bitmap variants are packaged; original extraction
documents remain untouched. Source matching and IGG/Noa correction tables belong
only to the asset compiler, not the game executable.

## Reproduce the import

Use lossless modern exports in `data/` and point the importer at an original
English installation. Paths are parameters; the original installation is read-only.

```powershell
python tools/data_export/extract_igg_translations.py --igg "PATH\TO\IGG_VERSION_6" --fallback "PATH\TO\OLDER_IGG" --target data
```

Omit `--fallback` when only the newer installation is available. The command
rewrites the tracked override CSV and writes `var/reports/igg_translation_review.csv`
and `var/reports/igg_translation_sources.json`. The review file lists rejected,
unchanged and unmatched entries. The manifest records per-file checksums and all
cinematic source checksums. Re-run matching after updating either installation
or the modern exports, then review the table diff and run focused tests.

Version-six source SHA-256 fingerprints used for this import:

| File | SHA-256 |
| --- | --- |
| Npc.dat | `54d576005664d3fefff0429b4eef9af42d645054a0cc65888ca71f90e9bb32c8` |
| Item.dat | `96b82e0b4afee02fba44463cf38b38a879caf1cf00a5579a664ca8b14541921a` |
| Skill.dat | `c3c8d51d022c226f2e47f09a3135c7b4a96eb7dd8f76268d22f39561219203dc` |
| SceneData.Dat | `d80db2b75bae4968251fffed0f684cda7515b29f83e4f5695079fe770aad2958` |
| Talk.dat | `0e3edf758e1818b7fc4c1d205036a023a6c63fc2fb61b8cfb6e403a6aeabab44` |
| Mark.dat | `cd3b47ab7f9087f3068c1beff954692bbd95a8b0386a422e756f5d2a34fbe436` |

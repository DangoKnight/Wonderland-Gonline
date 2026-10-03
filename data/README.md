# Exported Wonderland data

`item_data.json` contains every item record from
`../Wonderland-Client/data/Item.dat`.

The export includes:

- Readable item definitions, descriptions, icons and appearance references.
- All 451 decoded bytes of each record as `decoded_hex`, preserving unresolved
  fields, original text bytes and padding.
- The plaintext header as `header_hex`, original record indexes and file offsets.
- Format/schema versions, source size and source SHA-256 for provenance.

The maintained export contains 7,312 item records. Source SHA-256:
`f85cbee375e3918e337e6579b6731ba1396e16063b86a85cda9a86ffc5e826b0`.
Re-encrypting the decoded records reproduces the complete 3,298,163-byte input
file exactly. Client content patches are separate from decoding.

Regenerate from the repository root:

```sh
go run ./cmd/item-export \
  -input ../Wonderland-Client/data/Item.dat \
  -output data/item_data.json
```

On PowerShell, put the command on one line or use backticks for continuation.
The exporter preserves file order and rejects duplicate/zero IDs that the current
lookup parser would omit. It writes a complete temporary export before replacing
the previous asset. The offline importer reads this JSON; runtime reads its SQL
records from `assets_database`. Missing or invalid required SQL assets prevent
startup. Readable `definition` fields supply gameplay
values; decoded bytes preserve imported source data. Runtime loading is documented
in [the item format notes](../docs/ITEM_FORMAT.md).

## Complete resource export

`asset_manifest.json` inventories all 40 selected source assets. Sources are:

- Every file in `Wonderland-Client/data` (19 files).
- Server `Data` resources absent from the client (17 files).
- Server `listdata` tables (4 files).

The client supplies all 17 overlapping resources, including identical copies.
Server source code, build/project settings and license documents are outside the
resource inventory. The manifest records both source hashes for each overlap,
selected origin, output paths, byte lengths and SHA-256 checksums.

Regenerate and verify from the repository root using Python 3.10+ and Go:

```sh
python3 tools/data_export/export.py
python3 tools/data_export/verify.py
python3 -m unittest discover -s tools/data_export -v
WONDERLAND_TEST_CLIENT_DATA=../Wonderland-Client/data \
  go test ./internal/assets ./cmd/item-export -count=1
```

The exporter accepts `--client`, `--server` and `--output` to select other source
and destination directories. Source projects remain untouched. Unsupported files
fail the inventory check before export. Each output replaces its previous version
only after a complete temporary write; the manifest is written last. Verification
also checks inventory completeness and source priority. Re-encryption or
reassembly reproduces every selected source file byte for byte.

| Resources | Export | Retained content |
| --- | --- | --- |
| Item | `item_data.json` | 7,312 full decoded records and readable definitions |
| NPC | `npc_data.json` | 4,930 decoded records, names, stats, skills and drops |
| Skill | `skill_data.json` | 953 decoded records, names, descriptions and coefficients |
| Dialogue | `talk_data.json` | 17,493 decoded records and texts |
| Quest marks | `mark_data.json` | 2,158 decoded records, names, descriptions and completion flags |
| Scene | `scene_data.json` | 1,163 decoded records, names and fields |
| Compounds | `compound_data.json`, `compound2_data.json` | 168 and 789 full decoded manufacturing records |
| Experience formula | `formula_data.json` | Version, 45 coefficients, scaler and 21 gates |
| LBD | `lbd_data.json` | All 9 groups and child entries |
| Terrain | `ground_data.json` | 1,147 map records, filename index, terrain layers and grids |
| WEM | `wem_data.json` | 5,761 resources, including both 21-byte and 22-byte variants |
| Events | `eve_data.json` | 1,119 maps, all 11 section types, map bytes and unindexed gaps |
| Animations | `animation_data.json` | All 1,832 complete records and their index |
| Client text settings | `ride_pet_positions.json`, `player_titles.json`, `traffic_settings.json` | Readable text and original bytes |
| Server resources | `server/` | JSON, text, CSV, XML, converted items and list tables |
| Audio | `audio/` | 19,700 main streams and 17 additional streams, names and complete indexes |

Native tables retain every decrypted byte in `decoded_hex`, along with unchanged
headers, record order and file offsets. `field_layout` names verified properties;
fields with unresolved meanings use names such as `unknown_u16_offset_109`.
Text is decoded as UTF-8 or Big5, with encoding labels. Invalid text sequences are
marked with `big5-with-replacement`; their original bytes remain intact. Client
content substitutions and hardcoded patches are separate from decryption.

Formula, LBD, EVE, MBTM, MMG and audio resources are already plaintext containers.
Their exports retain every payload and index byte. Terrain prefixes and archive
indexes are decoded. Some terrain tails, animation payloads, event semantics and
WEM field meanings remain unresolved; the retained bytes support further work.
Server text exports preserve comments, duplicate entries, BOMs and line endings
in `source_hex`, alongside readable text and parsed JSON/CSV/XML when applicable.
These server JSON files are export envelopes; parsed authored JSON is under
`value`. Converted items remain in `server/converted_item_data.json` and are
separate from the authoritative client catalog.

### Audio and database files

Individual Ogg streams live in `audio/odd/` and `audio/odd_d01/`; the combined
`odd.ogg` and `odd_d01.ogg` payloads have been removed. `audio/manifest.json`
records per-track checksums and source warnings. The original `*_index.json`
files retain names, offsets, sizes, page counts and complete filename footers.
Concatenate the tracks in index order and append `footer_hex` to reconstruct
an original archive. The verifier accepts either individual tracks or temporary
combined payloads created by the full data exporter.
One main audio stream lacks its final end-of-stream flag in the source; that
anomaly is recorded as `source_warnings` and the source bytes are preserved.

The tracks total about 1.43 GB and remain local and ignored by Git. JSON indexes
and the track manifest remain available for version control. Regeneration reads the original `odd.dat` / `odd_d01.dat` files directly
from the sibling client; saved indexes and combined exports are never inputs.
Original account databases and their SQLite sidecars are excluded from the
asset export. Accounts and characters belong exclusively to Go gameplay storage.

The server's runtime loads all gameplay catalogs exclusively from the assets SQL
database. These exports are offline import inputs; server startup does not read
JSON or native files. Rebuild and restart to apply export changes.

## SQL access

`go run ./cmd/data-rebuild` rebuilds `var/assets.db` from these exports using
Go and GORM. All JSON content is preserved with indexed collection rows and SQL
views for major gameplay tables. Audio indexes are included; audio bytes stay
on disk. To reset gameplay storage at the same time, use the explicit
`-reset-gameplay` flag with the server configuration. Existing databases are
backed up before replacement. See [the reset procedure and SQL examples](../docs/ASSET_DATABASE.md).

## Sprite assets

All client JMA, JXA and JXAN resources are exported separately under
[`sprites/`](sprites/README.md), with lossless decoded pixels, readable frame
metadata and a PNG extraction command. See that document for regeneration and
verification instructions.

## Picture assets

[`pictures/`](pictures/README.md) contains PNG exports of the translated client's
`pic/` directory: loose images and the BMP/JPEG resources inside BMg/JMg archives.
The manifest retains archive/resource identity, dimensions and source hashes.

## Git storage policy

Generated PNG images, decoded native sprite payloads and audio files remain local
and are ignored by Git. JSON data, manifests, editable metadata and documentation
remain tracked. Regenerate or obtain the image/audio payloads separately for a
fresh checkout. Assets committed previously remain in Git/LFS history; stopping
tracking does not purge that history or remote LFS storage.

### Individual OGG tracks

Run `python3 tools/data_export/extract_audio.py` from the repository root to
extract the original sibling audio archives into `audio/odd/` and `audio/odd_d01/`. These are plaintext Ogg Vorbis streams, so no decryption
or transcoding is needed. Original resource names and bytes are preserved.
`audio/manifest.json` records individual checksums and source warnings.
Existing track folders and manifests are protected from replacement; use
`--output` for a fresh export directory.
Individual OGG files are ignored by Git, like their source containers.

## Regenerate from original sibling sources

All files and manifests here are derived outputs. To restore missing ignored
images, audio and video after cloning, only the `Wonderland-Client` (WLRI /
Wonderland Rhode Island) installation is required. Decryption keys come from
its matching executable. See [the asset import procedure](../docs/ASSET_IMPORT.md).

```sh
python3 tools/data_export/regenerate.py \
  --client ../Wonderland-Client \
  --output var/asset-import
```

This produces `audio/`, `sprites/`, `pictures/` and `media/`, including PNG
atlases, individual Ogg tracks, PCM sounds/voices and the preserved intro movie.
It does not regenerate the tracked root gameplay tables or `server/` definitions.
The restore tool checks matching media metadata before copying missing ignored
payloads into `data/`. No saved asset provides decryption inputs.

To regenerate the complete static game-data export, use the separate offline
`export.py --client <WLRI-root>/data --server <private-server-root> --output <new-directory>`
workflow. Private-Server supplies server-only tables for that operation; these
JSON definitions are already tracked and are not external media dependencies.
Original account databases and database-path overrides are excluded.

### Coverage limits

The exports preserve unresolved fields and original bytes where their semantics
are unknown. This is complete byte preservation for the supported data formats,
not a claim that all fields have fully understood meanings. All 1,147 Ground records and 768 style files now have complete structural
parsers. Unknown Ground, WEM and style field meanings remain explicitly named.
The 121 encoded `info/*.dat` resources are preserved as unresolved source hex;
their encryption is not decoded. Font2 layout and JPC character mapping remain
unverified even though all decrypted font bytes are retained.
Three sprite indexes have 18 records whose companion archives are missing from
the supplied sibling client; their pixels cannot be recovered. Ten picture
resources reference missing palette colors and document a black fallback.
Windows `Thumbs.db` is excluded as a thumbnail cache, not game artwork.

## Remaining client media

`media/media_manifest.json` accounts for 3,796 original source files and records
checksums for all 3,246 output files. UI BMP and JPEG variants retain their original
extensions (`name.bmp.png`, `name.jpg.png`) so neither variant overwrites the other.
GIFs and ANI cursors use PNG frames and JSON playback metadata. The intro
movie remains whole at `media/sty/WLOnline.avi`, copied byte for byte with its
embedded audio and timing. Video payloads are ignored by Git.
ANI cursor manifests preserve hotspots, timings and framebuffer inversion pixels.
Sound/voice WAV files are decoded to PCM16 without lossy audio encoding. Font
atlases retain physical glyph indices; their manifests also retain all decrypted
font bits. `media/sty/style_data.json` contains all 768 decoded styles, keyed by
original filename. It preserves every operand, unknown field and source hash;
there are no individual style JSON files. Agreement
and officer texts are decrypted to readable CP950 text with reversible byte data.

Verify source reconstruction and every exported file:

```sh
python3 tools/data_export/verify_media.py --data data/media
python3 tools/data_export/verify.py
```

Verification compares fresh exports to originals. Intentional artwork edits will
change output checksums; do not regenerate to undo those edits. The client accepts
`-assets ../data` when run from its module and discovers repository `data/` by
default. Its login UI reads the exported PNG skins/backgrounds, PNG font and PCM
sounds. Legacy packaged asset roots remain compatible. Executables, DLLs,
website/help pages, runtime logs and saved player state are outside media scope.

**Extraction is not fully complete:** the encoded info resources and absent sprite
companions remain unresolved. Their presence in a preservation export is not a
claim that their content has been recovered.


## Skill effects

`skill_data.json` retains original skill records and adds `fields.effect_refs`.
`skill_effects.json` contains reusable, source-derived compatibility definitions
with stable string identifiers. These express the server's documented behavior
for decoded native classes, including Shield Defense's +10% DEF/MDEF policy.
They preserve source provenance and do not alter original decoded bytes or text.
Model schemas live under repository `schemas/`. See `docs/DEVELOPMENT.md` for
editing, SQL resolution and source-only regeneration instructions.


`lucky_draw.json` projects daily Lucky Draw compatibility policy against freshly
decrypted sibling Item.dat. It preserves source item names/hashes alongside
announcement labels, quantities, relative weights and presentation slots. The
14 default outcomes have equal weight 1; these are authored probabilities rather
than recovered official drop rates. Full source-only export includes it. Refresh
with `python3 tools/data_export/lucky_draw.py --output data --overwrite` and
rebuild the asset database. Runtime reads only indexed SQL rows.

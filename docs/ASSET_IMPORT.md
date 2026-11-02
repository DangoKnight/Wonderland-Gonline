# Import local assets from WLRI

Use this procedure after cloning the repository to restore the asset payloads
that Git excludes. The client source is a **WLRI (Wonderland Rhode Island)**
installation, including its matching `aLogin.exe`. Supply its root through
`--client`. Wonderland-Private-Server is not required for this procedure.
Client-derived tables, JSON metadata, lossless sprite payloads and media are
local generated files. Only server-specific definitions and documentation remain
tracked under `data/`. This procedure restores the complete client-derived tree.
The installation can be anywhere on disk and its directory can have any name.
Supply its path explicitly; no sibling-directory layout is assumed.

The import regenerates assets from originals. It does not download assets,
require an old checkout's `data/`, or modify the original installation.

For the full path from checkout to a running server/client, see
[GETTING_STARTED.md](GETTING_STARTED.md).

## 1. Prepare the originals and tools

Run the following commands from this repository's root. The shell examples use
Bash on Linux/macOS. Set the client path to your actual installation; spaces
are fine.

```sh
wlo_client_source="/absolute/path/to/WLRI-client"
wlo_import_output="$PWD/var/asset-import"
```

Expected layout:

```text
WLRI-client/                    # Any directory name; supplied with --client
  aLogin.exe
  data/
  jma/
  pic/
  menu/, images/, upimg/, cursor/, font/, sound/, voice/, sty/, info/, txt/
```

Supply the full WLRI installation rather than just its `data/` folder. Sprite
keys come from that installation's executable. Available media directories are
exported; missing files or companion archives cannot be reconstructed.

Install Git, Go 1.25 or newer, a C compiler,
Python 3.11 or newer, FFmpeg and FFprobe. The Go tools may download their declared
dependencies on the first run. For the Python image tools:

```sh
python3 -m venv var/asset-import-venv
var/asset-import-venv/bin/python -m pip install -r tools/data_export/requirements.txt
```

## 2. Regenerate temporary external assets

```sh
var/asset-import-venv/bin/python tools/data_export/regenerate.py \
  --client "$wlo_client_source" \
  --output "$wlo_import_output"
```

The destination must not exist. For a repeat attempt, choose a new output path;
retain the previous tree until you have checked the new one. `--client` is
required and has no default. The default output is the repository-relative
`var/asset-import`.
On Windows, the source, output and system temporary directory may be on different
drives; they do not need to share a volume.

This creates the following media directories with matching JSON metadata and
local payloads. `external_asset_import.json` records the restoration scope:

| Output | Content |
| --- | --- |
| Root `*_data.json`, text tables, `skill_effects.json`, `lucky_draw.json` | Native client tables and projections from repository-owned compatibility policies |
| `asset_manifest.json` | Recreated client inventory plus preserved server-only export provenance |
| `sprites/` | Decrypted sprite data, editable PNG atlases and animation metadata |
| `pictures/` | PNG pictures and atlases from loose images and BMg/JMg archives |
| `audio/odd/`, `audio/odd_d01/` | Individual original Ogg Vorbis tracks |
| `media/` | UI PNGs, font atlases, cursor/GIF frames, PCM WAV sound and voice, styles and text |
| `media/sty/WLOnline.avi` | Original video, preserved whole when present |

No decoded JMA files or combined `odd.ogg` containers are retained. Atlas packing
and frame deduplication run during the sprite/picture export. Original account
databases and SQLite sidecars are excluded.
The exporter copies the repository's server-only JSON definitions into the
temporary tree and rebuilds their manifest entries. It does not require a
Private Server installation or infer server policies from the client.

## 3. Verify the regenerated tree

Keep the original paths available while running verification:

```sh
var/asset-import-venv/bin/python tools/data_export/verify.py --data "$wlo_import_output"
var/asset-import-venv/bin/python tools/data_export/extract_audio.py \
  --input "$wlo_client_source/data" --output "$wlo_import_output/audio" --verify-existing
var/asset-import-venv/bin/python tools/data_export/verify_sprites.py "$wlo_import_output/sprites"
var/asset-import-venv/bin/python tools/data_export/verify_media.py --data "$wlo_import_output/media"
go run ./cmd/sprite-export -verify-editable -compare-original -output "$wlo_import_output/sprites"
```

Picture export checks atlas pixels before publication. Verification failures must
be resolved before restoration. Reports of preserved unknown fields or missing
source companions describe extraction limits; they are not recovered content.
See [coverage limits](../data/README.md#coverage-limits).

## 4. Restore only missing ignored files into `data/`

Preview, then apply:

```sh
var/asset-import-venv/bin/python tools/data_export/import_untracked.py \
  --from "$wlo_import_output" --dry-run
var/asset-import-venv/bin/python tools/data_export/import_untracked.py \
  --from "$wlo_import_output"
git status --short
```

The restore command:

- Checks any retained tracked JSON within the declared import scope before copying.
  Relocated absolute source paths and resulting JSON manifest checksums may differ;
  source hashes, content, frame placement and relative output paths must agree.
- Copies only missing files that the repository's current Git ignore rules exclude.
- Preserves tracked files, existing local files and edited artwork.
- Refuses links that would direct writes outside `data/`.

This restores missing client tables, sprite payloads, manifests, PNG, OGG, WAV
and AVI files under the current rules.
It does not stage files or change ignore rules. Untracked files that are not ignored are deliberately left in the temporary
output for review. The importer does not restore local configuration, logs,
executables, Python caches or gameplay databases.

If it reports differing metadata, the WLRI installation or exporter output does
not match the tracked media definitions. Use an installation matching those
source hashes, or review an asset-version update before restoring pixels. The
generated tree is also usable as an alternate asset root: it includes client
tables and copies of the repository's server definitions. Existing local files
are never overwritten; use a new asset root when changing client versions.

## 5. Use the restored assets

Check the maintained game-data manifest and run the client against `data/`:

```sh
go run ./cmd/asset-build -data data
(cd client && go run . -assets ../data)
```

If you also need to recreate the server's SQL asset catalog, stop the server before
rebuilding its active catalog, then run:

```sh
go run ./cmd/data-rebuild -data data -assets-db var/assets.db
```

This rebuilds static assets and backs up an existing asset database. It does not
reset accounts or characters. See [database operations](ASSET_DATABASE.md).
Keep the temporary output until restoration is satisfactory, then remove it if
space is needed. Existing edited artwork is preserved, so fresh-export checksum
verifiers can report differences in the installed tree; verify the untouched
temporary export separately from deliberate edits.

Generated client resource catalogs (`client/assets-manifest.json` and
`client/asset-packages.json`) are local outputs of `cmd/client-assets`. If needed:

```powershell
$wlo_client_source = 'C:/path/to/WLRI-client'
go run ./cmd/client-assets -source $wlo_client_source -dest var/client-original-assets -pack var/client-original-packages
```

`client/asset-contract.json` is initialized by the first `cmd/client-bundle`
build. Existing local catalogs and dimension baselines remain intact.
Untracking generated files does not rewrite Git history or delete old LFS objects.

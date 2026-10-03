# Import local assets from WLRI

Use this procedure after cloning the repository to restore the asset payloads
that Git excludes. `Wonderland-Client` means the **WLRI (Wonderland Rhode Island)**
installation, including its matching `aLogin.exe`. Supply its root through
`--client`. Wonderland-Private-Server is not required for this procedure.
Server gameplay tables and client runtime JSON definitions are already tracked
in the repository; this procedure restores external media payloads.

The import regenerates assets from originals. It does not download assets,
require an old checkout's `data/`, or modify the original installation.

For the full path from checkout to a running server/client, see
[GETTING_STARTED.md](GETTING_STARTED.md).

## 1. Prepare the originals and tools

Run the following commands from this repository's root. The shell examples use
Bash on Linux/macOS. Set the client path to your actual installation; spaces
are fine.

```sh
wlo_client_source="/absolute/path/to/Wonderland-Client"
wlo_import_output="$PWD/var/asset-import"
```

Expected layout:

```text
Wonderland-Client/               # WLRI installation root
  aLogin.exe
  data/
  jma/
  pic/
  menu/, images/, upimg/, cursor/, font/, sound/, voice/, sty/, info/, txt/
```

Supply the full WLRI installation rather than just its `data/` folder. Sprite
keys come from that installation's executable. Available media directories are
exported; missing files or companion archives cannot be reconstructed.

Install Git (and Git LFS for the tracked LFS files), Go 1.25 or newer, a C compiler,
Python 3.11 or newer, FFmpeg and FFprobe. The Go tools may download their declared
dependencies on the first run. For the Python image tools:

```sh
python3 -m venv var/asset-import-venv
var/asset-import-venv/bin/python -m pip install -r tools/data_export/requirements.txt
git lfs pull
```

## 2. Regenerate temporary external assets

```sh
var/asset-import-venv/bin/python tools/data_export/regenerate.py \
  --client "$wlo_client_source" \
  --output "$wlo_import_output"
```

The destination must not exist. For a repeat attempt, choose a new output path;
retain the previous tree until you have checked the new one. The default client
root is
`../Wonderland-Client`, and the default output is `var/asset-import`.

This creates the following media directories with matching JSON metadata and
local payloads. `external_asset_import.json` records the restoration scope:

| Output | Content |
| --- | --- |
| `sprites/` | Decrypted sprite data, editable PNG atlases and animation metadata |
| `pictures/` | PNG pictures and atlases from loose images and BMg/JMg archives |
| `audio/odd/`, `audio/odd_d01/` | Individual original Ogg Vorbis tracks |
| `media/` | UI PNGs, font atlases, cursor/GIF frames, PCM WAV sound and voice, styles and text |
| `media/sty/WLOnline.avi` | Original video, preserved whole when present |

No decoded JMA files or combined `odd.ogg` containers are retained. Atlas packing
and frame deduplication run during the sprite/picture export. Original account
databases and SQLite sidecars are excluded.

## 3. Verify the regenerated tree

Keep the original paths available while running verification:

```sh
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

- Checks regenerated JSON against tracked metadata under `data/audio`,
  `data/sprites`, `data/pictures` and `data/media` before copying.
  Relocated absolute source paths and resulting JSON manifest checksums may differ;
  source hashes, content, frame placement and relative output paths must agree.
- Copies only missing files that the repository's current Git ignore rules exclude.
- Preserves tracked files, existing local files and edited artwork.
- Refuses links that would direct writes outside `data/`.

This restores missing PNG, OGG, WAV and AVI payloads under the current rules.
It does not stage files or change ignore rules. Untracked files that are not ignored are deliberately left in the temporary
output for review. The importer does not restore local configuration, logs,
executables, Python caches or gameplay databases.

If it reports differing metadata, the WLRI installation or exporter output does
not match the tracked media definitions. Use an installation matching those
source hashes, or review an asset-version update before restoring pixels. The
external output is not a standalone client/server data tree: it intentionally
contains no root gameplay exports or server tables.

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

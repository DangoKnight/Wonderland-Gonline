# Import local assets from WLRI

Use this procedure after cloning the repository to restore the asset payloads
that Git excludes. The client source is a **WLRI (Wonderland Rhode Island)**
installation, including its matching `aLogin.exe`. Supply its root through
`--client`. Either the updated original Chinese client or its updated manually
translated English edition is sufficient. You can also supply both.
Neither an IGG installation nor Wonderland-Private-Server is required.
Client-derived tables, JSON metadata, lossless sprite payloads and media are
local generated files. Server-specific definitions, documentation and compiled
language catalogs are tracked under `data/`. This procedure restores the complete client-derived tree.
The installation can be anywhere on disk and its directory can have any name.
Supply its path explicitly; no sibling-directory layout is assumed.

The import regenerates assets from originals. It does not download assets,
require an old checkout's `data/`, or modify the original installation.

For the full path from checkout to a running server/client, see
[GETTING_STARTED.md](GETTING_STARTED.md).

## Import with a compiled client

The native Gonline executable can extract and compile a Legacy WLRI installation
without a repository checkout or Go toolchain. Install Python 3.11 or newer with
Pillow (`python -m pip install "Pillow>=12,<13"`), and put FFmpeg and FFprobe on
`PATH`. These tools are needed only for extraction; ordinary gameplay does not
use them. The executable embeds the extraction scripts, authored inputs and
translation policies, and contains the native sprite/picture/table exporters.
No existing Gonline asset pack is required to enter extraction mode.
Developers can build the importer executable first with
`go build -o ../bin/wonderland-client.exe .` from `client/`; this does not need
an extracted asset tree. The browser/WebAssembly build cannot perform this import.

From the directory containing the compiled client, in PowerShell:

```powershell
.\wonderland-client.exe -extract-assets -client "C:\Games\WLRI" -extract-language zh-TW
```

For an updated manually translated English installation, use
`-extract-language en`. To import both editions, supply the English installation
with `-client` and the Chinese installation with `-extract-chinese-client PATH`.
`-extract-ui-language en`, `zh-TW` or `both` can restrict artwork to available
source languages. The language fallback and translation precedence are the same
as the checkout workflow below.

The default destination is a **new** `assets` directory beside the executable.
`-extract-output PATH` selects another new directory; `-extract-python PATH`
selects a Python environment containing Pillow. `-extract-workers N` bounds the
parallel compilation workers. For example:

```powershell
.\wonderland-client.exe -extract-assets -client "C:\Games\WLRI English" -extract-language en -extract-chinese-client "C:\Games\WLRI Chinese" -extract-output "D:\Gonline Assets"
.\wonderland-client.exe -assets "D:\Gonline Assets\client-assets.zip"
```

Extraction reports progress in the terminal and exits when finished. Its output
contains editable PNG/JSON/Ogg assets under `editable/`, an image dimension
contract and the optimized `client-assets.zip` core pack with companion packs.
Keep the packs together. Normal startup automatically discovers
`assets/client-assets.zip` beside the executable; other destinations use
`-assets` as shown above. The editable directory is retained for later changes
and rebuilding with `cmd/client-bundle` in a development checkout.

The originals are read-only inputs. Existing destinations are rejected, including
paths inside either source installation. Extraction and compilation take place
in a separate staging directory; failed imports do not publish a partial asset
installation. To replace an older import, build into a fresh directory and select
it after completion. This mode does not initialize or modify server databases.

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

### Select UI artwork languages

Choose the source setup explicitly. These commands run from the repository root;
use the Python environment prepared above (`var/asset-import-venv/Scripts/python.exe`
on Windows, `var/asset-import-venv/bin/python` on Linux):

| Installations available | Import arguments | Default UI artwork |
| --- | --- | --- |
| Updated original Chinese only | `--client PATH --client-language zh-TW` | Chinese |
| Updated manually translated English only | `--client PATH --client-language en` | English |
| Both | `--client ENGLISH_PATH --client-language en --chinese-client CHINESE_PATH` | Both |

For example:

```powershell
python tools/data_export/regenerate.py --client "PATH\TO\UPDATED_CHINESE_CLIENT" --client-language zh-TW --output var/import-chinese
python tools/data_export/regenerate.py --client "PATH\TO\UPDATED_TRANSLATED_CLIENT" --client-language en --output var/import-english
python tools/data_export/regenerate.py --client "PATH\TO\UPDATED_TRANSLATED_CLIENT" --client-language en --chinese-client "PATH\TO\UPDATED_CHINESE_CLIENT" --output var/import-bilingual
```

Run one command, then use its output as `$wlo_import_output` in the verification
and restoration steps below. `--client-language en` remains the compatibility
default; the importer does not guess a language from folder names.

With both installations, non-text definitions, sprites, sounds, movies and maps
come from `--client`. Chinese text is matched by native ID and record identity;
the Chinese copy supplies matching language fields and UI artwork. It never
replaces the primary installation's gameplay records. Native bytes and original
source hashes are preserved, with separate provenance for companion text.

The repository supplies previously imported English/Chinese translations and
authored corrections as build inputs. They do not require their original IGG
installation. Available matching English overrides also apply to known Chinese
source text. New untranslated content falls back to its source text; a
Chinese-only installation does not manufacture missing English artwork or
translations. Chinese-only builds retain all Chinese source text, including
entries newer than the repository's translation snapshot.

With both sources, `--ui-language en`, `zh-TW` or `both` can restrict the artwork
included independently of the two text catalogs. Build the bundle from the selected
complete asset tree. When an image is unavailable in the chosen language, the
client uses the installed alternative, including login and character-creation
backgrounds. The selected text language remains independent of available images.

UI images can also be imported separately:

```powershell
python tools/data_export/ui.py --english "PATH\TO\ENGLISH_UI_CLIENT" --output var/ui-english/media
python tools/data_export/ui.py --chinese "PATH\TO\CHINESE_CLIENT" --output var/ui-chinese/media
```

Provide both source options in one command to import both sets. These commands
produce `media/locales/en/menu/Skins` and/or `media/locales/zh-TW/menu/Skins`, with
PNG-normalized images and source manifests. Selected locale directories must not
already exist. Use a fresh asset tree for a strict single-language distribution;
the restoration tool preserves existing artwork and does not remove another set
that was imported earlier.

A complete regeneration creates the following directories with matching JSON metadata and
local payloads. `external_asset_import.json` records the restoration scope:

| Output | Content |
| --- | --- |
| Root `*_data.json`, text tables, `skill_effects.json`, `lucky_draw.json` | Native client tables and projections from repository-owned compatibility policies |
| `asset_manifest.json` | Recreated client inventory plus preserved server-only export provenance |
| `sprites/` | Decrypted sprite data, editable PNG atlases and animation metadata |
| `pictures/` | PNG pictures and atlases from loose images and BMg/JMg archives |
| `audio/odd/`, `audio/odd_d01/` | Individual original Ogg Vorbis tracks |
| `media/` | UI PNGs, font atlases, cursor/GIF frames, Ogg Vorbis sound/music/voice, styles and text |
| `media_formats.json` | Standard media profile and validated image/audio/video counts |
| `media/sty/WLOnline.avi` | Original video, preserved whole when present |

No decoded JMA files or combined `odd.ogg` containers are retained. Atlas packing
and frame deduplication run during the sprite/picture export. Original account
databases and SQLite sidecars are excluded.
The exporter copies the repository's server-only JSON definitions into the
temporary tree and rebuilds their manifest entries. It does not require a
Private Server installation or infer server policies from the client.

The single regeneration command uses the `png-vorbis-v1` media profile:
all extracted raster images are PNG, standalone audio is Ogg Vorbis, and AVI
videos retain their original container and timing. Static source images, font
atlases, animation frames, sprites and picture archives all follow the PNG
output convention. Existing Vorbis streams are copied byte-for-byte. Other
audio, including PCM/ADPCM WAV sound effects, music and loose voices, is encoded
once with libvorbis quality 6 at 44.1 kHz stereo. This encoding is lossy;
original installations and source fingerprints remain intact. Existing Vorbis
recordings retain their source sample rates/channels to avoid re-encoding.
`Mp3s/`, when populated, is included too. AVI's embedded streams remain part of
the preserved video, separate from standalone audio assets.

The client resolves native `.wav` resource names to `.ogg` at the loading
boundary, for music, effects, ambience and battle voice fallbacks. Older
WAV-only exports remain readable. Use a fresh extraction directory for a clean
standardized tree; restoring only missing files into an older `data/` directory
intentionally preserves its existing files and edits.

PowerShell, from the repository root:

```powershell
python tools/data_export/regenerate.py --client "PATH\TO\UPDATED_CHINESE_CLIENT" --output var/asset-import-standard
```

The output directory must not already exist. All asset types come from that one
installation, including its matching executable. Compiled bundles may later use
the client's tiled image representation; that is a separate build stage from
the editable PNG/Vorbis extraction.

## 3. Verify the regenerated tree

Keep the original paths available while running verification:

```sh
var/asset-import-venv/bin/python tools/data_export/verify.py --data "$wlo_import_output"
var/asset-import-venv/bin/python tools/data_export/extract_audio.py \
  --input "$wlo_client_source/data" --output "$wlo_import_output/audio" --verify-existing
var/asset-import-venv/bin/python tools/data_export/verify_sprites.py "$wlo_import_output/sprites"
var/asset-import-venv/bin/python tools/data_export/verify_media.py --data "$wlo_import_output/media"
var/asset-import-venv/bin/python tools/data_export/media_formats.py --data "$wlo_import_output"
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

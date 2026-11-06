# Set up and run Wonderland Gonline

The server builds and runs from a fresh checkout without WLRI extraction. This
guide also covers optional Go-client extraction and a local server/client session.
Both Go programs are still being
ported; supported gameplay and client screens do not cover the entire original
game. See [port status](PORTING.md) and [client details](CLIENT.md).

Run commands from the repository root unless a command explicitly enters
`client/`. The main walkthrough uses Bash on Linux/macOS; Windows equivalents
are listed below. Endpoint examples assume `config.example.json` defaults; use
your configured addresses when preserving an existing local configuration.

## 1. Install the tools

| Tool | Used for |
| --- | --- |
| Git | Checkout; client-derived JSON and media are generated locally |
| Go 1.25 or newer | Build both programs; the server alone requires Go 1.24 |
| C compiler and C development headers | Server SQLite driver with CGO |
| Python 3.11 or newer and `venv` | Go-client asset extraction and verification only |
| Pillow | Go-client image conversion only; installed from the repository requirements file |
| FFmpeg and FFprobe on `PATH` | Go-client sound/voice conversion only |
| Working desktop and graphics drivers | Run the Go/Ebitengine client |
| OpenSSL | Generate the admin token in the Bash examples |

Node/npm is used for development lint checks, not server or client startup.
The client uses the patched Ebitengine copy under `client/third_party/ebiten`;
keep it in the checkout. Linux graphics library requirements are listed in
[CLIENT.md](CLIENT.md#run). A headless machine can build and run the server;
running the graphical client needs a desktop/display environment.

After checking out the repository:

```sh
go version
python3 --version
ffmpeg -version
ffprobe -version
```

The server's shared definitions are tracked and embedded. Client rendering files,
including `data/ground_data.json` and sprite metadata, remain excluded from Git.
For a server-only installation, skip section 2 and proceed to configuration.

## 2. Extract WLRI assets for the Go client only

Obtain the full **WLRI (Wonderland Rhode Island)** installation. Its root must
contain `aLogin.exe`, `data/`, `jma/`, `pic/` and its other media directories.
Replace the example source path below with that installation's root. Its folder
name and location are unrestricted; no sibling checkout is assumed. No
Wonderland-Private-Server checkout is needed for this setup. Either the updated
original Chinese edition or the updated manually translated English edition
is sufficient; IGG is not required. For Chinese-only input, add
`--client-language zh-TW` to the regeneration command below. For both editions,
use the translated edition as `--client` and add `--chinese-client PATH_TO_CHINESE`.
See [source language choices](ASSET_IMPORT.md#select-ui-artwork-languages).

```sh
wlo_client_source="/absolute/path/to/WLRI-client"
wlo_import_output="$PWD/var/asset-import"
python3 -m venv var/asset-import-venv
var/asset-import-venv/bin/python -m pip install -r tools/data_export/requirements.txt
var/asset-import-venv/bin/python tools/data_export/regenerate.py \
  --client "$wlo_client_source" --output "$wlo_import_output"
```

Choose an output directory that does not exist. Allow space for both the
complete temporary export and the installed payloads; the audio alone is about
1.43 GB. The original installation is read-only input. Matching original data,
including its executable, is required for sprite keys and source provenance.

Verify the temporary output, then restore missing files into this checkout:

```sh
var/asset-import-venv/bin/python tools/data_export/verify.py --data "$wlo_import_output"
var/asset-import-venv/bin/python tools/data_export/extract_audio.py \
  --input "$wlo_client_source/data" --output "$wlo_import_output/audio" --verify-existing
var/asset-import-venv/bin/python tools/data_export/verify_sprites.py "$wlo_import_output/sprites"
var/asset-import-venv/bin/python tools/data_export/verify_media.py --data "$wlo_import_output/media"
go run ./cmd/sprite-export -verify-editable -compare-original -output "$wlo_import_output/sprites"
var/asset-import-venv/bin/python tools/data_export/import_untracked.py \
  --from "$wlo_import_output" --dry-run
var/asset-import-venv/bin/python tools/data_export/import_untracked.py \
  --from "$wlo_import_output"
go run ./cmd/asset-build -data data
```

The result includes root client tables, PNG sprite/image atlases, manifests,
Ogg Vorbis music/sounds/voices and preserved AVI video under `data/`. The importer fills
missing ignored files and preserves existing artwork and tracked definitions.
Server-specific JSON remains tracked and is copied into the generated tree;
client-derived tables and metadata are regenerated locally. The original database and
database-path override are excluded. See [ASSET_IMPORT.md](ASSET_IMPORT.md) for
scope, extraction limits and repeat-import behavior.

Keep the temporary tree until the restored client works. Normal runs use
repository `data/`, not `var/asset-import` or the original WLRI installation.

## 3. Create local configuration and the asset database

Create the local configuration once, preserving an existing file:

```sh
if [ ! -e config.local.json ]; then
  cp config.example.json config.local.json
fi
```

The example uses loopback addresses and two separate databases:

| Path | Contents | Created by |
| --- | --- | --- |
| `var/assets.db` | Static gameplay catalog | First server startup or explicit `cmd/data-rebuild` |
| `var/wonderland.db` | Go accounts, characters and settings | Server startup |
| `data/` | Local client exports and tracked server-only definitions | Original client extraction plus checkout |

Build the static catalog and check that it loads:

```sh
export CGO_ENABLED=1
go run ./cmd/data-rebuild -config config.local.json
go run ./cmd/wonderland -config config.local.json -inspect-data
```

`-inspect-data` prints catalog counts and warnings without opening listeners or
requiring an admin token. Missing-item warnings can disable individual gacha
pools. The server creates a missing catalog from its embedded repository seed;
the rebuild command above explicitly recreates that same seed with a backup.
No extracted client tables or media are needed. Existing catalogs are preserved
on normal startup; outdated schemas require the explicit migration procedure.

Stop the server before rebuilding an existing active catalog. Rebuilds preserve
old asset databases as backups. The commands above do not reset gameplay storage.
`-reset-gameplay` is a separate destructive operation; use the
[database procedure](ASSET_DATABASE.md) when a reset is intended.

## 4. Compile and start the server

```sh
mkdir -p bin var
go build -o bin/wonderland ./cmd/wonderland
export WONDERLAND_ADMIN_TOKEN="$(openssl rand -hex 32)"
./bin/wonderland -config config.local.json -log-file var/server.jsonl
```

The compiled server embeds its web administration files. Node and a separate
web development server are not needed. Once `assets.db` has been built, the
server loads that SQL file rather than client media or `data/` JSON.

Keep this terminal open. In a second terminal, check:

```sh
curl http://127.0.0.1:8080/healthz
```

The response includes `status: ok` and `parity: incomplete`. Open
`http://127.0.0.1:8080` for administration. Enter the token from the server
terminal; to display the generated value there:

```sh
printf '%s\n' "$WONDERLAND_ADMIN_TOKEN"
```

The token must contain at least 24 characters. It is an environment variable,
not a JSON config field or game-account password. Reuse it when restarting in the
same shell; generating a new token changes the credential the admin UI accepts.
The log file appends JSON lines and does not create a missing parent directory.
Add `-debug` when investigating packets: ordinary packets log metadata; Lucky
Draw, movement and pose packets also have bounded decoded traces.

For development, the equivalent source run is:

```sh
go run ./cmd/wonderland -config config.local.json -log-file var/server.jsonl
```

`make build` builds the server. `make run` uses `config.example.json`, so use the
explicit command above when you want local overrides. Stop a foreground server
with Ctrl+C; a background process accepts `kill -TERM <server-pid>`. Shutdown
closes sessions and checkpoints pending character state.

## 5. Register a game account

With the server running, use another terminal:

```sh
curl http://127.0.0.1:8080/register \
  -H 'Content-Type: application/json' \
  --data '{"username":"tester","password":"change-me","email":""}'
```

Choose your own credentials. Names accept 4–14 ASCII letters, digits or
underscores; passwords accept 4–14 printable ASCII bytes. Registration creates
an ordinary account. Use the admin Accounts view to grant GM access, ban/unban,
reset passwords, delete accounts or adjust mall balances. Sessions can be
terminated there without restarting the server. See [Web administration](ADMINISTRATION.md)
for character editing, GM tools, EXP tuning, gifts, content editors and maintenance.

## 6. Compile and run the Go client

The client is a separate Go module. The canonical build validates and compiles
editable PNG/JSON assets before compiling Go. From the repository root:

```sh
make build-client
./bin/wonderland-client
```

See [Client architecture](CLIENT_ARCHITECTURE.md) for asset editing, bundle
validation, Windows builds and the session drawer. Each session has its own
connection and state while sharing loaded assets.

Or run directly from loose sources:

```sh
(cd client && go run . -assets ../data)
```

Without a server-list override, the client falls back to one local server at
`127.0.0.1`. Select it and log in with the game account. The native server list
uses login port 6414 and launcher status port 6416; the HTTP admin health endpoint
is separate. Keep the default game ports for this local setup.

The Go client has login, character and world code, with ongoing rendering,
interaction and gameplay work. A successful connection does not imply every
original feature is available. The native WLRI client and the Go client are
separate executables.

To connect to another host, create an explicit server list in ignored `var/`:

```ini
01[Local]1
Local1*192.168.1.50
```

Save it as `var/SERVER.INI` and run:

```sh
./bin/wonderland-client -serverini var/SERVER.INI
```

Use the host reachable from the client, not a listener bind address such as
`0.0.0.0`. Bind the server's game listeners to a reachable interface as described
in [CONFIGURATION.md](CONFIGURATION.md). Defaults only accept local connections.
For a source run inside `client/`, the override path is `../var/SERVER.INI`.

## 7. Adjust behavior

Use [CONFIGURATION.md](CONFIGURATION.md) for every server config field, CLI
flags, client overrides and gameplay-tuning sources. Changes to listeners and
limits require a restart. Admin name/account/session changes apply live; static
SQL gameplay changes publish through the validated editor or supported reload
scopes; other catalog/startup changes require a restart.

## Windows PowerShell

The server requires Go and a GCC/MinGW-w64 toolchain on `PATH`. Python and FFmpeg
are required only for Go-client extraction. The optional client asset commands above work
with these substitutions; use PowerShell backticks instead of Bash backslashes
for multiline commands, or put each command on one line:

```powershell
$wlo_client_source = 'C:/path/to/WLRI-client'
$wlo_import_output = Join-Path (Get-Location) 'var/asset-import'
python -m venv var/asset-import-venv
$wlo_python = '.\var\asset-import-venv\Scripts\python.exe'
& $wlo_python -m pip install -r tools/data_export/requirements.txt
& $wlo_python tools/data_export/regenerate.py --client $wlo_client_source --output $wlo_import_output
```

That command assumes the manually translated English edition. With only the
original Chinese edition, append `--client-language zh-TW`. With both editions,
keep the translated edition in `$wlo_client_source` and append
`--chinese-client 'C:/path/to/updated-Chinese-client'`; both UI sets and matching
Chinese text are included automatically.

For the Go client, run the same verification and restoration scripts using
`& $wlo_python` in place of `var/asset-import-venv/bin/python`. For the server
alone, skip all extraction commands and start here:

```powershell
if (-not (Test-Path config.local.json)) { Copy-Item config.example.json config.local.json }
$env:CGO_ENABLED = '1'
New-Item -ItemType Directory -Force bin, var | Out-Null
go run ./cmd/data-rebuild -config config.local.json
go build -o bin/wonderland.exe ./cmd/wonderland
$env:WONDERLAND_ADMIN_TOKEN = [guid]::NewGuid().ToString('N') + [guid]::NewGuid().ToString('N')
.\bin\wonderland.exe -config config.local.json -log-file var/server.jsonl
```

In another terminal, register the account and build/run the client:

```powershell
$body = @{ username = 'tester'; password = 'change-me'; email = '' } | ConvertTo-Json
Invoke-RestMethod -Uri http://127.0.0.1:8080/register -Method Post -ContentType 'application/json' -Body $body
go run ./cmd/client-bundle -source data -output bin/client-assets.zip -contract client/asset-contract.json
Push-Location client
go build -o ../bin/wonderland-client.exe .
Pop-Location
.\bin\wonderland-client.exe -assets data
```

Display `$env:WONDERLAND_ADMIN_TOKEN` in the server terminal to obtain the admin
credential. Use Ctrl+C there to stop it.

## Troubleshooting

| Symptom | Check |
| --- | --- |
| Missing client JSON tables or sprite metadata | Regenerate them from the original installation; these outputs are intentionally ignored |
| Import reports differing metadata | Retain the temporary output for comparison; existing local files and any retained tracked metadata are preserved |
| Asset export destination exists | Select a new temporary output directory |
| Missing PNG, WAV, font or sprite file | Complete verified restoration into `data/`; normal runtime has no native fallback |
| Assets database cannot be created | Check permissions for the configured path; first startup creates a missing catalog from the embedded seed |
| Asset rebuild reports checksum mismatch | Check manifest/export consistency; use a supported exporter to regenerate intentional table changes |
| Admin token error | Set `WONDERLAND_ADMIN_TOKEN` in the same shell that starts the server |
| `address already in use` | Stop the previous instance or change the relevant listener; keep client game-port expectations in mind |
| Launcher shows offline while admin health works | Check TCP 6416, host/firewall and `status_server_ids`; `/healthz` does not serve launcher status |
| Client connection lost after inactivity | Review `idle_seconds` for login or `world_idle_seconds` for gameplay (0 disables the limit), and check logs |
| Client cannot open a window | Check display session and graphics drivers; see [CLIENT.md](CLIENT.md#run) |
| Config change appears ineffective | Restart for startup settings; saved admin `server_name` overrides config `name` |
| Files appear under an unexpected directory | Relative paths resolve from the working directory, including database and log paths |

Use [Testing](TESTING.md) for development selections, the separate server/client
suites, required environments and optional native-data checks.

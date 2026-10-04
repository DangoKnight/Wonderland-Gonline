# Wonderland Go

An ongoing rewrite of the Wonderland Online private server and port of its
client to Go, with web administration and editable game assets.

Both programs remain incomplete. Implemented server features include native
login and character management, map movement and portals, NPC events, PvE
battles, inventory, crafting, companions, trading, server chat, mall and Lucky
Draw. The Go client has login, character and world code, with further rendering
and gameplay work in progress. See [port status](docs/PORTING.md) and
[client details](docs/CLIENT.md) for coverage and limitations.

## Start here

Follow [the setup guide](docs/GETTING_STARTED.md) in this order:

1. Install Go, CGO tooling, Python/Pillow, FFmpeg and Git LFS.
2. Extract and verify the ignored assets from a matching WLRI installation.
3. Create `config.local.json` and build the separate SQL assets database.
4. Compile the server and start it with an admin token.
5. Register an account, then build and run the Go client.

Only the WLRI installation is needed for missing image/audio/video payloads;
server gameplay definitions are already tracked. The native WLRI executable
supplies extraction keys and is separate from the Go client.

| Guide | Contents |
| --- | --- |
| [Getting started](docs/GETTING_STARTED.md) | End-to-end setup, Linux/macOS and Windows commands, troubleshooting |
| [WLRI asset import](docs/ASSET_IMPORT.md) | Source layout, extraction, verification and safe restoration |
| [Configuration](docs/CONFIGURATION.md) | Server JSON, flags, admin settings, gameplay tuning and client overrides |
| [Database operations](docs/ASSET_DATABASE.md) | Asset rebuilds, SQL authority, backups and explicit gameplay reset |
| [Items and player state](docs/ITEMS_PLAYER_STATE.md) | Tent controls, furniture, item reservations and recovery |
| [Client](docs/CLIENT.md) | Graphics requirements, asset layout, inspection and packaging |

## Build and run

After completing asset/database setup, run from the repository root:

```sh
export CGO_ENABLED=1
mkdir -p bin var
go build -o bin/wonderland ./cmd/wonderland
export WONDERLAND_ADMIN_TOKEN="$(openssl rand -hex 32)"
./bin/wonderland -config config.local.json -log-file var/server.jsonl
```

The setup guide includes Windows PowerShell equivalents and token handling.
Keep the server running and use a second terminal for the client:

```sh
(cd client && go build -o ../bin/wonderland-client .)
./bin/wonderland-client -assets data
```

Default endpoints are TCP 6414 (login/game), 6415 (world), 6416 (native launcher
status), and HTTP `http://127.0.0.1:8080` (admin, registration and `/healthz`).
They bind to loopback. Stop the foreground server with Ctrl+C.

Gameplay storage is `var/wonderland.db`; static catalogs are loaded read-only
from `var/assets.db`; the client reads `data/`. Paths resolve from the working
directory. SQL persistence uses GORM/SQLite, and the server needs a C compiler.
Use Go 1.25 or newer for both programs (the server alone requires Go 1.24).

## Register an account

Linux Bash:

```bash
curl http://127.0.0.1:8080/register \
  -H 'Content-Type: application/json' \
  --data '{"username":"tester","password":"change-me","email":""}'
```

Windows PowerShell:

```powershell
$body = @{ username = 'tester'; password = 'change-me'; email = '' } | ConvertTo-Json
Invoke-RestMethod -Uri http://127.0.0.1:8080/register -Method Post -ContentType 'application/json' -Body $body
```

Account names accept 4–14 ASCII letters, digits or underscores. Passwords accept 4–14 printable ASCII bytes to match the game client. Passwords are stored as salted PBKDF2-SHA256 hashes. Registration does not grant administrator or GM access. Grant in-game GM commands from the web interface's account list; legacy default GM names are not trusted.

## Validate

The following commands work in Linux Bash and Windows PowerShell after enabling CGO and installing the compiler as above:

```text
go test ./...
go test -race ./...
go vet ./...
go run ./cmd/wonderland -inspect-data
```

Native server-data tests need the sibling server's `Data` directory and this repository's maintained JSON item catalog; the client package is a separate data set. Set its location explicitly:

Linux Bash:

```bash
export WONDERLAND_TEST_DATA='../Wonderland-Private-Server/Data'
go test -v ./internal/assets
```

Windows PowerShell:

```powershell
$env:WONDERLAND_TEST_DATA = '../Wonderland-Private-Server/Data'
go test -v ./internal/assets
```

The root test command does not include the nested `client/` module; run its checks separately as described in [CLIENT.md](docs/CLIENT.md). The Makefile requires a POSIX shell and GNU Make. Windows users can use the direct Go commands above, or run Make through Git Bash/WSL. For the Makefile's additional formatting/lint checks, install Node/npm and use the following equivalents from the repository root in either shell:

```text
gofmt -l .
go vet ./...
npx prettier --check "internal/admin/web/**/*.{html,css,js}"
npx htmlhint --config .htmlhintrc internal/admin/web/index.html
```

`gofmt -l .` should print no filenames; if `golangci-lint` is installed, also run `golangci-lint run ./...` to match the optional Makefile check.

Tests cover golden wire bytes, fragmented TCP frames, truncated payloads, native record offsets, EVE boundaries, Big5 names, animation timing, login over a duplex connection, administrator authorization, persistent accounts, duplicate registrations, bans, transaction rollback, inventory capacity and quest reward replay. Creation tests cover name reservations, malformed requests, starter grants, map acknowledgment ordering, deletion codes, and reconnect persistence; native asset checks exercise all 60 body/head/element combinations. Peer tests cover concurrent arrival, appearance before movement, map isolation, disconnect cleanup, and failed recipients. World tests cover golden map-info bytes, PreEvent rule order and conditions, every portal lookup priority, and a warp between maps with persistence, departure, acknowledgment and cooldown. Trade tests cover offer revisions, metadata preservation, full-bag swaps, concurrent confirmations, pending-request cleanup and rollback of both characters. Native asset tests are opt-in because game assets are not bundled. Relative `WONDERLAND_TEST_DATA` paths resolve from the Go module root. Friendship tests cover cross-map requests, native lists and appearance fields, request expiry, capacity, concurrent acceptance, presence across map loads and deletion cleanup. Minigame tests cover start/result packets, native rabbit win/loss rewards, cancellation, replay, chained games, full bags and failed saves. Preference tests cover native settings packets, persistence, malformed commands, failed saves and team/trade request controls. Vehicle tests cover placement/boarding packets, raft wear and wrecking, mounted-slot protection, save failures, portal landing, and login after database reopen.

Optional fuzzing (same commands in Linux Bash and Windows PowerShell):

```text
go test ./internal/protocol -run '^$' -fuzz FuzzRead -fuzztime 10s
go test ./internal/assets -run '^$' -fuzz FuzzEVE -fuzztime 10s
```

## Layout

- `cmd/wonderland`: executable, configuration, signals and shutdown.
- `internal/protocol`: binary framing, XOR and bounded field readers.
- `internal/server`: TCP lifecycle, sessions, launcher status and login handlers.
- `internal/store`: GORM models, queries and transactions for accounts, character state, friendships and settings; versioned SQLite schema migrations.
- `internal/game`: inventory, character creation and serialization, quest rewards, critical-hit rules, pets and stat/skill progression, connected to the corresponding server gameplay handlers.
- `internal/battle`: deterministic PvE round engine producing animation steps; the server plays and commits them.
- `internal/world`: immutable map state from EVE, per-character map info and PreEvent NPC visibility, portal lookup.
- `internal/assets`: SQL-backed gameplay catalogs and offline native format readers.
- `internal/assetdb`: GORM asset import, read-only materialization and database rebuild.
- `internal/admin`: authenticated HTTP API and embedded HTML/CSS/JavaScript interface.
- `docs/source-inventory.json`: source revision, hashes and independent per-file findings for all 566 reference files, including 331 C# files.
- `docs/PORTING.md`: current implemented scope, remaining executable migration work and unfinished-source hold list.

Unknown actions are counted and logged at debug level; they do not receive a fabricated success response. `GET /api/status` reports incomplete parity and remaining areas. Administrative API requests require `Authorization: Bearer <token>`.

## License

The source project's GPLv3 license is preserved in [LICENSE](LICENSE). See [NOTICE](NOTICE) for provenance. Tracked JSON exports contain game data;
the original WLRI installation and ignored media payloads are supplied separately.

### Admin account actions

The **Accounts** table provides **Reset password** and **Delete** actions. Password reset uses a masked form with confirmation and accepts 4–14 printable ASCII characters, matching registration and the game client. It preserves characters, bans, GM grants and the separate character-deletion code. Deletion asks for confirmation and permanently removes the account, both character slots and the character-deletion credential. Both actions disconnect the account's active session after the database transaction commits and create an audit entry without credentials.

Authenticated API endpoints:

- `POST /api/accounts/{id}/password` with `{"password":"new-password"}`.
- `DELETE /api/accounts/{id}`.

Missing accounts return 404, invalid IDs or password requests return 400, and failed database changes return 500. Login authentication is synchronized with these actions so an in-flight login cannot publish an old credential session after a reset or deletion.

### Admin session termination

Use **Sessions → Terminate session**, or **Accounts → Terminate session** for an online account. The Accounts table shows Online/Offline status. Termination closes only the selected connection and runs normal logout cleanup (including trade/team/battle cleanup); the account can reconnect. The interface refreshes and reports the result. The authenticated API is `POST /api/sessions/{id}/kick`; invalid IDs return 400 and sessions that have already disconnected return 404. Successful administrator termination is recorded in the server log.

## Development conventions

See [DEVELOPMENT.md](docs/DEVELOPMENT.md) for numeric naming, constant ownership, unresolved-code naming and verification requirements.

## Item mall

World entry synchronizes the points and bonus catalogs, mall settings/status, and
both account balances. Native AC75 carts and legacy AC23 catalog, balance and
purchase requests are supported. Currency deduction and inventory delivery commit
in one GORM transaction; stale catalog rows, insufficient points and full bags
cannot partially deliver a cart. Catalogs use typed SQL Mall definitions initialized from exported defaults.
Gacha purchase/opening support requires a valid pool table. Invalid or unavailable pools are excluded from the advertised catalogs.
The withdrawn Lucky Pack remains unavailable.
Bonus balances appear beside mall points in the administration account list.
Accounts → Adjust mall points adds or deducts either points or bonus points for
online and offline accounts. Enter a signed integer; deductions stop at zero,
and balances above 2,147,483,647 are refused. Balance changes and audit records
save together. Active clients receive both native balance records; changes during
login or a warp synchronize after map acknowledgment.

GM commands `:im <delta> [character]`, `:points_im` and `:mallpoints` (also `/`
aliases) adjust ordinary points. Omitting the character targets the GM; an unknown
explicit target makes no change. Each adjustment records its actor, currency,
requested/applied amounts, and before/after balances in the database audit table.

The authenticated API is `POST /api/accounts/{id}/mall` with, for example,
`{"currency":"bonus","delta":100}`. It returns both saved balances. Mall forging and the arcade launcher are
implemented below; additional native mall request compatibility remains pending.

## Compound synthesis

AC23:14 consumes one item from each of two distinct bag slots and creates one fresh
result. Recipes come from the SQL-loaded assets catalog: authored defaults, then
`Compound2.dat` and `Compound.dat` projections in file order. Asset inspection
includes the two-input recipe count; missing required SQL assets stop startup. The first symmetric
match wins. This native handler is deterministic and reproduces the greater input
ID when no recipe matches.

Consumption and delivery save together before success packets. Leftover stacks
keep damage and metadata; the result needs an empty slot after consumption.
Full bags and failed saves leave the bag unchanged.

AC40 takes a subcommand and two distinct slot bytes. It requires a matching
recipe and echoes the subcommand with success and the output ID. No match returns
failure without consuming anything. Its result uses normal inventory grants,
including compatible stack merging. Both handlers preserve ingredient metadata,
commit before receipts, and wait during map loading, battle, trade, events and
cutscenes. A mounted vehicle cannot be used as an ingredient. Recipe rates are
unused, matching the legacy handlers.

Native five-material formula/tool/timer manufacturing remains incomplete.
Two-input chat manufacturing, probabilistic synthesis and validated SQL recipe
editing exist; see ECONOMY_SOCIAL.md. Original-client validation remains pending.

## Mall forging

AC75:3 forges one equipment item in a bag slot. Upgrade families and point-forging
eligibility come from `mall_forging.json` in the assets database. Strong Scroll
families follow their configured successors: the first upgrade costs one scroll,
the next costs two, and terminal items refuse further upgrades. Scrolls can come
from multiple stacks. The replacement stays in the same slot, keeps durability,
and resets its other metadata, matching the legacy upgrade behavior.

Eligible equipment outside those families uses three ordinary IM points per
attempt with a 50% cryptographic success roll. Success increments forge metadata
up to 200; a failed roll still costs three points. Bonus points are retained.
Costs and item state save in one transaction before native AC75:6 results and
balance updates. Missing data, insufficient costs and failed saves spend nothing.
Forging waits during battles, trades, loading and interactions, and respects
transient item reservations. Original-client validation remains pending.

## Arcade category launcher

AC75:4 takes one category byte, opens the selected client minigame with the
legacy zero seed, and refreshes ordinary/bonus point balances. Opening a category
costs nothing and changes no character state. The launcher waits during map
loading, battles, trades, quests and cutscenes.

The legacy launcher installs no result callback: standalone AC57 reports grant
no items, tickets or quest progress. EVE minigames keep their existing authored
outcome routing. Arcade ticket/prize exchange remains pending.

## Gacha packs

Native AC91 previews return the current ordered reward list. AC23:75, AC23:128
and ordinary item use (AC23:96) open one pack from the requested bag slot.
`gacha_packs.json` supplies the reward IDs, quantities and integer weights; the
supplied data contains 24 pools and 394 reward rows with custom weights. Draws
use cryptographic rejection sampling over 10,000 weight slots.

Pack consumption and all reward additions save together before receipts. Full
bags and failed saves retain the pack. Missing or invalid configuration disables
all configured packs, reports asset warnings and removes those packs from mall
catalogs. The withdrawn Lucky Pack (34333) stays unavailable. Pack quantities,
reward definitions and duplicate pool IDs are validated at startup.

Rewards are delivered as items. Their use depends on the corresponding item
handler; remaining scroll and other special-item behavior and general
item restrictions need further verification. Transient item reservations are
implemented. Original game-client validation remains pending.

## Bag-item repairs

AC36 repairs the selected inventory slot. One Repair Wrench (48050) takes
priority over a 500 gold fee. Repair restores damage to zero and keeps the
stack's quantity and other metadata, apart from the one wrench used for payment.
Payment and repair save together before inventory, balance and success packets.
Healthy items cost nothing; insufficient funds and failed saves retain the item.
Mounted vehicles must be landed before repair. The request addresses the bag;
unequip worn gear first. Transient reservations are enforced; native-client
validation remains pending.

## Item catalog

The server loads all gameplay assets from the separate SQL database selected by
`assets_database` (default `var/assets.db`). Items, NPCs, skills, dialogue, maps,
events, animation timings, recipes and authored gameplay settings are read in a
single transaction at startup and cached for packet handlers. Missing or invalid
required assets stop startup. No runtime fallback reads JSON or native files.

The maintained `data/` exports are offline import inputs. Items originate from
`data/item_data.json`, containing 7,312 translated-client items; readable
properties and all decoded bytes remain in SQL. Regenerate exports with the
offline tools, then run `go run ./cmd/data-rebuild` to import them. Restart the
server to load changes. Remove the old `item_catalog` and `data_directory`
configuration fields and use `assets_database` instead. Accounts and characters
continue to use the separate `database` setting.

The sibling's current gacha configuration references server-only items absent
from this JSON. Its validation disables the entire gacha table and reports a
warning. All configured starter items are available.

See [item format and verification](docs/ITEM_FORMAT.md) for validation, the
export format and remaining field interpretation.

## Complete data exports

To restore ignored image, audio and video assets after cloning, follow the
[asset import procedure](docs/ASSET_IMPORT.md), using the path to your
Wonderland-Client (WLRI) installation. No private-server checkout is required.

`data/` contains lossless exports of all client data resources and server-only
resources, with translated-client copies preferred for overlapping files.
Regenerate with `python3 tools/data_export/export.py`, then verify with
`python3 tools/data_export/verify.py`. The manifest accounts for 40 selected
assets and every source file can be reconstructed byte for byte. Audio payloads remain local and ignored by Git. Original account databases
are excluded from export and asset import.
See [the data directory documentation](data/README.md) for formats, source
selection, regeneration and the scope of current runtime loading.

## Rebuild databases from extracted data

Run `go run ./cmd/data-rebuild` to recreate the separate SQL asset catalog at
`var/assets.db`. To also erase and recreate gameplay storage, stop the server
and run `go run ./cmd/data-rebuild -reset-gameplay -config config.local.json`.
Existing databases are preserved as timestamped backups. Audio bytes stay
outside SQL. See [the database procedure](docs/ASSET_DATABASE.md) for query
examples, recovery and current runtime loading behavior.

### Economy and social gameplay

Player shops, guilds, marriage, parcel mail, manufacturing, synthesis and gathering
are described in [ECONOMY_SOCIAL.md](docs/ECONOMY_SOCIAL.md), including database
upgrades, player commands, editable economy definitions and remaining native
client limitations.

For original Private Server player snapshots, see the separate
[legacy SQLite import procedure](docs/LEGACY_IMPORT.md). For authored combat
area shapes, see [combat targeting](docs/COMBAT_TARGETING.md).

## Migration findings and persistence direction

The maintained source inventory records an independent review of 566 reference
files. Native request, manufacturing, character metadata and administration gaps
remain; see [PORTING.md](docs/PORTING.md). Empty reference handlers and dormant
quest helpers are listed separately from missing working behavior. Static source
review is not a substitute for native-client or multiplayer acceptance.

The database owns durable gameplay state. Purchases, exchanges, rewards and claims
must commit atomically before success replies. Ordinary walking stays in session memory and uses dirty checkpoints, selected by
startup `character_save_seconds` (default 30 seconds), plus disconnect flushing.
Purchases and rewards remain immediately durable; checkpoints preserve those
committed results. See the
[persistence policy](docs/DEVELOPMENT.md#transaction-boundaries-and-session-checkpoints).

# Wonderland Gonline

**Wonderland Gonline Server** is the Go server rewrite; **Wonderland Gonline** is
the Go client. The client uses Xaolan’s small portrait as its window icon and
restores open sessions, account names and server choices from a local
`workspace.json` profile. See [client architecture](docs/CLIENT_ARCHITECTURE.md#workspace-profile-and-application-identity).

An ongoing rewrite of the Wonderland Online private server and port of its
client to Go, with web administration and editable game assets.

Both programs remain incomplete. Implemented server features include native
login and character management, map movement and portals, NPC events, PvE
battles, inventory, crafting, companions, trading, server chat, mall and Lucky
Draw. The Go client includes native battle scenes and targeting, inventory and
compounding, Skills and Teams windows, riding/following pets, Social and mail,
item trading, a scene minimap and several minigames. GPU rendering and decoded
assets are shared by independent sessions in one window; gameplay parity remains
in progress. See [port status](docs/PORTING.md) and
[client details](docs/CLIENT.md) for coverage and limitations.

## Start here

Follow [the setup guide](docs/GETTING_STARTED.md) in this order:

1. Install Git, Go and CGO tooling for the server.
2. Create `config.local.json` from `config.example.json` if you need overrides.
3. Build and start the server with an admin token; first startup initializes a missing SQL assets database from tracked server definitions.
4. Register an account.
5. For the Go client, separately install Python/Pillow and FFmpeg, extract its WLRI assets, then build and run it.

The server needs no client installation or extraction. Its complete shared
catalog and contact-count table ship in the repository. Existing SQL databases
and administrator edits remain authoritative; use the explicit migration tool
when upgrading them. The WLRI installation supplies the Go client's generated
tables, metadata and media. The native WLRI executable
supplies extraction keys and is separate from the Go client.
Use the updated original Chinese installation, its manually translated English
edition, or both. No IGG or Private Server installation is required. See the
[source-language import commands](docs/ASSET_IMPORT.md#select-ui-artwork-languages).

| Guide | Contents |
| --- | --- |
| [Getting started](docs/GETTING_STARTED.md) | End-to-end setup, Linux/macOS and Windows commands, troubleshooting |
| [WLRI asset import](docs/ASSET_IMPORT.md) | Source layout, extraction, verification and safe restoration |
| [Configuration](docs/CONFIGURATION.md) | Server JSON, flags, admin settings, gameplay tuning and client overrides |
| [Database operations](docs/ASSET_DATABASE.md) | Preserving upgrades, repository seeds, SQL authority, backups and deliberate resets |
| [Items and player state](docs/ITEMS_PLAYER_STATE.md) | Tent controls, furniture, item reservations and recovery |
| [Client](docs/CLIENT.md) | Graphics requirements, asset layout, inspection and packaging |
| [Story system](docs/STORY_SYSTEM.md) | SQL story policies, event execution, authoring and recovery |
| [Testing](docs/TESTING.md) | Automated checks, graphics/snapshots, live acceptance and recorded findings |

## Build and run

Run from the repository root; a missing assets database is initialized automatically:

### Linux/macOS (Bash)

```sh
export CGO_ENABLED=1
mkdir -p bin var
go build -o bin/wonderland ./cmd/wonderland
export WONDERLAND_ADMIN_TOKEN="$(openssl rand -hex 32)"
./bin/wonderland -config config.local.json -log-file var/server.jsonl
```

Keep the server running and use a second terminal for the client:

```sh
make build-client
./bin/wonderland-client
```

### Windows (PowerShell)

Install Go and a GCC/MinGW-w64 toolchain matching Go's target architecture,
with both `go` and `gcc` available on `PATH`. Build and start the server from
the repository root. Client asset extraction is not required:

```powershell
$env:CGO_ENABLED = '1'
New-Item -ItemType Directory -Force bin, var | Out-Null
go build -o bin/wonderland.exe ./cmd/wonderland
$env:WONDERLAND_ADMIN_TOKEN = [guid]::NewGuid().ToString('N') + [guid]::NewGuid().ToString('N')
.\bin\wonderland.exe -config config.local.json -log-file var/server.jsonl
```

Keep the server running. In a second PowerShell terminal at the repository root,
compile the editable client assets, then build and run the client:

```powershell
go run ./cmd/client-bundle -source data -output bin/client-assets.zip -contract client/asset-contract.json
Push-Location client
go build -o ../bin/wonderland-client.exe .
Pop-Location
.\bin\wonderland-client.exe
```

These client commands perform the same asset compilation and Go build as
`make build-client`, without requiring Make. Display `$env:WONDERLAND_ADMIN_TOKEN`
in the server terminal to obtain the admin credential.

Default endpoints are TCP 6414 (login/game), 6415 (world), 6416 (native launcher
status), and HTTP `http://127.0.0.1:8080` (admin, registration and `/healthz`).
They bind to loopback. Stop the foreground server with Ctrl+C.

Combat uses native delivery by default. To enable full-round action chains for
compatible Gonline clients, set `"combat_round_chain": true` in the server's
`config.local.json` and restart. Legacy clients continue using native packets.
See [combat transport](docs/COMBAT_TRANSPORT.md).

Gameplay storage is `var/wonderland.db`; static catalogs are loaded read-only
from `var/assets.db`; compiled clients discover split asset packs, while
`-assets data` selects loose editable exports. Paths resolve from the working
directory. SQL persistence uses GORM/SQLite, and the server needs a C compiler.
Use Go 1.25 or newer for both programs (the server alone requires Go 1.24).

An account may have one authenticated connection at a time, including character
selection. A second login is rejected while the first remains connected. The Go
client displays “Character is already logged in”; the original client displays
its native “Repeated Login” message (AC0:19).

## Register an account

Open `/register` on the server's HTTP address. With the example HTTP address, use
`http://127.0.0.1:8080/register`; use the configured port for local overrides.
The public page needs no administrator token.
It accepts 8–10 character usernames/passwords for the original client's limits,
confirms the password, and shows registration errors or successful creation.
Start the server first; the page is embedded in the server binary.

The existing JSON API is also available:

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

Account names accept 4–14 ASCII letters, digits or underscores. The JSON API accepts passwords of 4–14 printable ASCII bytes; use at most 10 characters with the original client. Passwords are stored as salted PBKDF2-SHA256 hashes. Registration does not grant administrator or GM access. Grant in-game GM commands from the web interface's account list; legacy default GM names are not trusted.

## Testing

See [TESTING.md](docs/TESTING.md) for focused checks, pre-push server validation,
server/client module commands, optional assets, graphics tests and live acceptance.

## Layout

- `cmd/wonderland`: executable, configuration, signals and shutdown.
- `internal/protocol`: binary framing, XOR and bounded field readers.
- `internal/server`: TCP lifecycle and gameplay handlers grouped by feature; independent logging and packet-building packages. See the [server component guide](internal/server/README.md).
- `internal/store`: GORM models, queries and transactions for accounts, character state, friendships and settings; versioned SQLite schema migrations.
- `internal/game`: inventory, character creation and serialization, quest rewards, critical-hit rules, pets and stat/skill progression, connected to the corresponding server gameplay handlers.
- `internal/battle`: deterministic PvE round engine producing animation steps; the server plays and commits them.
- `internal/world`: immutable map state from EVE, per-character map info and PreEvent NPC visibility, portal lookup.
- `internal/assets`: gameplay catalog types, validation and offline native format readers; no SQL dependency.
- `internal/assetsql`: typed SQL catalog loading, projections, migrations and validated content edits.
- `internal/assetdb`: GORM asset import, provenance storage and database rebuild.
- `internal/admin`: authenticated HTTP API and embedded Vue/Tailwind interface.
  Frontend development and rebuilding are documented in
  [the admin frontend guide](internal/admin/frontend/README.md); normal server
  builds use the checked-in bundle and do not require Node.js.
- `docs/source-inventory.json`: source revision, hashes and independent per-file findings for all 566 reference files, including 331 C# files.
- `docs/PORTING.md`: current implemented scope, remaining executable migration work and unfinished-source hold list.

Unknown actions are counted and logged at debug level; they do not receive a fabricated success response. `GET /api/status` reports incomplete parity and remaining areas. Administrative API requests require `Authorization: Bearer <token>`.

## License

The source project's GPLv3 license is preserved in [LICENSE](LICENSE). See [NOTICE](NOTICE) for provenance. Local generated JSON exports contain game data;
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

AC23:14/87/101, compatibility AC40 and `/compound` (`/synthesize`) share the
rank-and-material-base engine. Native requests accept two to five distinct bag
slots, including optional Alchemy Books. Each selected slot consumes one unit;
at least two non-book materials are required. Primary, Junior and Superior use
separate compiled probability curves, learned-skill requirements and a native
30-level proficiency progression. Books shift the rank roll; secondary-base
loss and catastrophic junk outcomes follow the selected tier.

Current SQL ownership, learned skills, ingredient removal, output placement and
proficiency commit in one transaction before receipts. Rejected attempts and
failed saves retain resources; committed catastrophes consume the materials.
Historical `AlchemyRecipes` and `Economy.Synthesis` rows remain provenance and
no longer choose outputs, charge fees or control synthesis probabilities.
See [compounding](docs/COMPOUNDING.md) for eligibility, formulas and client controls.

Manufacturing is separate: AC59 and `/manufacture` retain two-input recipes;
AC64 supports five-material formulas, owned benches/tools/plans, durable timers
and atomic bag/tent delivery. See [manufacturing and character state](docs/MANUFACTURING_CHARACTER_STATE.md).

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
outcome routing. AC71 separately charges and grants weighted prizes for supported
egg, slot and fruit machines, without using client result claims. See
[minigames](docs/MINIGAMES.md). Arcade ticket/prize exchange remains pending.

## Gacha packs

Native AC91 previews return the current ordered reward list. AC23:75, AC23:128
and ordinary item use (AC23:96) open one pack from the requested bag slot.
Typed SQL `GachaPacks` definitions supply reward IDs, quantities and integer
weights. `data/server/gacha_packs.json` is an offline seed, not a runtime file. Draws
use cryptographic rejection sampling over 10,000 weight slots.

Pack consumption and all reward additions save together before receipts. Full
bags and failed saves retain the pack. Malformed pool definitions fail validation;
compatibility import disables only pools with missing item definitions, records
warnings and excludes those packs from mall catalogs. Other valid pools stay
available; unavailable rewards are never dropped or reweighted. The withdrawn
Lucky Pack (34333) stays unavailable. Import and validated administration edits
check pack quantities, reward definitions and duplicate pool IDs.

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

The tracked server catalog includes item, NPC, skill, map, dialogue, terrain and
event definitions. `go run ./cmd/data-rebuild` rebuilds it from the repository
seed, keeping a backup. Optional extracted `data/` exports can refresh definitions
with `go run ./cmd/data-rebuild -data data`. Restart the
server to load changes. Remove the old `item_catalog` and `data_directory`
configuration fields and use `assets_database` instead. Accounts and characters
continue to use the separate `database` setting.

Offline gacha import reports missing pack/reward definitions per pool, preserving
other complete pools. Use the validated `GachaPacks` editor for current contents;
exported seed counts do not describe an administrator-edited installation.
All configured starter items are available in the maintained catalog.

See [item format and verification](docs/ITEM_FORMAT.md) for validation, the
export format and remaining field interpretation.

## Complete data exports

For the Go client, restore ignored tables, JSON metadata, images, audio and video after cloning using the
[asset import procedure](docs/ASSET_IMPORT.md), using the path to your
original WLRI installation. Supply its location explicitly; no private-server
checkout or particular directory layout is required.

`data/` holds local lossless client exports and tracked server-only definitions.
Client-derived tables, sprite payloads, editable metadata and manifests are
excluded from Git; their existing local copies remain usable. The setup exporter
regenerates them from the original client and retains repository-owned server data.
Regenerate with the complete `tools/data_export/regenerate.py --client` procedure
in the import guide, then verify the selected output tree. The manifest records
the files supplied by that installation; unresolved fields remain lossless rather
than being claimed as fully decoded. Media stays local and ignored by Git.
Original account databases are excluded from export and asset import.
See [the data directory documentation](data/README.md) for formats, source
selection, regeneration and the scope of current runtime loading.

## Database upgrades and deliberate rebuilds

For an existing installation, stop the server and run
`go run ./cmd/database-migrate -config config.local.json` before restarting the
rebuilt executable. This preserves gameplay and content edits. See the
[database procedure](docs/ASSET_DATABASE.md) for current versions and recovery.

Use `go run ./cmd/data-rebuild` only to deliberately recreate the separate SQL asset catalog from
tracked server definitions at `var/assets.db`, without client exports. To also erase and recreate gameplay storage, stop the server
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

## Live legacy-client acceptance

The [complete wiki notepad quest list](docs/QUEST_LIST.md) preserves all 345 source
entries. [Quest research and implementation](docs/QUEST_RESEARCH.md) records the
native-script bindings, preserving asset upgrades, implemented gaps and remaining
walkthrough verification.

Use the [native-client acceptance procedure](docs/TESTING.md#native-client-acceptance)
to prepare isolated scenario characters, login credentials and feature checklists.

Client builds compile editable PNG/JSON sources into `bin/client-assets.zip`.
Picture/media images become lossless tiled pages with shared small-image atlases,
deduplicated content and a 64 MiB LRU cache. Map drawing reads visible tiles while
source PNGs remain editable in `data/`.
The collapsible session panel beside the game adds, previews, switches and removes independent
sessions sharing loaded assets. See [Client architecture](docs/CLIENT_ARCHITECTURE.md)
for asset editing, build validation and session/resource ownership.

# Testing and validation

This is the maintained guide for automated checks, test data, graphics/snapshots,
benchmarks, native-client acceptance and recorded testing findings. Commands run
from the repository root unless they explicitly enter `client/`. Feature guides
retain implementation/source evidence and link here for execution and outcomes.
Asset import/build integrity checks stay with their operational workflows in
[asset import](ASSET_IMPORT.md) and [client architecture](CLIENT_ARCHITECTURE.md).
Machine-readable historical evidence remains in [source-inventory.json](source-inventory.json).

- [Test selection and correctness](#test-selection-and-correctness)
- [Environment requirements](#environment-requirements)
- [Development checks](#development-checks)
- [Pre-push server validation](#pre-push-server-validation)
- [Test data and opt-ins](#test-data-and-opt-ins)
- [Subsystem checks](#subsystem-checks)
- [Graphics, snapshots and benchmarks](#gpu-parity-and-snapshots)
- [Testing findings from the documentation audit](#findings-from-the-2026-11-01-audit)
- [Native-client acceptance and recorded outcomes](#native-client-acceptance)
- [Historical validation reports](#historical-validation-reports)
- [Benchmarks](#benchmarks)
- [Resource exhaustion review](#resource-exhaustion-review--2026-11-01)

## Test selection and correctness

During development, run only affected modules and integrations. Use `-run` within
large packages such as `internal/server` and `client/wlo/app`, including before
commits. Run full server validation only before a push. Use `-race` for changed
concurrency, shared caches, session ownership or resource lifetimes. Include relevant native-data
compatibility checks for protocol/event changes. Confirm that a selection
actually runs tests: `[no tests to run]` does not validate that package.

Keep independent raw-byte expected packets in golden tests. Arbitrary fixture
IDs may stay numeric; preserve authored data tables and source/reference hashes.
Review constant meanings and namespaces as well as wire values. Do not rewrite
expected bytes to mirror production constants. See [development conventions](DEVELOPMENT.md).

Exercise transaction rollback, stale ownership, full containers, replay and
post-commit receipt failures for resource changes. Buffered-state changes also
need walking followed by purchase/trade/warp, stale snapshots, deletion,
reconnect, failed checkpoint retry and concurrent-session coverage. Test dirty
field checkpoints without rewriting inventory/pet/quest children.

A fixture or synthetic protocol test does not establish native-client acceptance.
Record manual steps PASS, FAIL, BLOCKED or NOT TESTED only from observed outcomes;
preparation starts NOT RUN. A timeout is an incomplete run, not a pass. Keep
historical results labeled with their scope and source revision.

## Environment requirements

The root module is the server; `client/` is a separate Go module. Root test/vet
commands never include the client. Build both with Go 1.25 or newer; server
SQLite checks require CGO and a working C compiler. Use the graphics libraries
and display requirements in [client setup](CLIENT.md#run).

Client/UI tests need a usable desktop/display even when they render CPU samples;
real GPU parity also requires an OpenGL context. Local login/world integration
tests open loopback TCP listeners. Sandboxes must permit those listeners; do not
classify `socket: operation not permitted` as a protocol regression.

Keep credentials for opt-in live-server tests separate from regular accounts.
Creation/deletion tests mutate slot 1. Prefer isolated `cmd/live-test` databases.
No manual acceptance or asset tests should reset the regular gameplay store.

## Development checks

Select a relevant recipe under [subsystem checks](#subsystem-checks), for example:

```sh
go test -race ./internal/game ./internal/server -run '^Test(Potential|Compounding|Trade|Resting)' -count=1
```

For client interaction work, run from `client/`:

```sh
go test ./wlo/app ./wlo/world -run '^Test(Battle|Hotbar|Compound|Route|ActorWalk|Settings)' -count=1
```

Use `-count=1` when fresh execution is needed. Narrow packages/selections further
for small changes. Source-only runs use repository exports; compiled paths and
other opt-ins are listed below.

Server packet builders and their pure golden tests live in
`internal/server/wire`; bounded log capture and packet trace checks live in
`internal/server/logging`. Include the affected component when changing it:

```sh
go test -race ./internal/server/logging ./internal/server/wire -count=1
```

Session/persistence integration tests remain in `internal/server`, grouped by
feature. The [component guide](../internal/server/README.md) maps handlers to
their files. Existing test names and `-run` selections remain valid.

## Pre-push server validation

Run the full server checks only before a push, from the repository root with a
current imported assets database:

```sh
WONDERLAND_TEST_ASSETS_DB="$PWD/var/assets.db" go test -race -timeout 60m ./...
go vet ./...
```

Run the separate client module as well:

```sh
cd client
go test -timeout 60m ./...
go vet ./...
```

Use client `-race` when concurrency or shared lifetimes change. Full installed
story/catalog checks can exceed Go's default ten-minute package timeout; `60m`
is an outer limit, not an expected duration or evidence of load acceptance.

PowerShell, from the repository root:

```powershell
$env:WONDERLAND_TEST_ASSETS_DB = (Resolve-Path var/assets.db).Path
go test -race -timeout 60m ./...
go vet ./...
Push-Location client
go test -timeout 60m ./...
go vet ./...
Pop-Location
```

These commands omit explicitly opt-in GPU, original-asset and live-account tests
unless their environment variables are set. Report skips separately. Normal
build verification remains `make build` / `make build-client` (or the direct
commands in [setup](GETTING_STARTED.md)); client builds validate edited assets.

### Formatting and lint

```sh
gofmt -l .
npx prettier --check "internal/admin/web/**/*.{html,css,js}"
npx htmlhint --config .htmlhintrc internal/admin/web/index.html
```

`gofmt -l` should print no files. Run `golangci-lint run ./...` when installed to
match the optional Makefile check. Node/npm is needed for web lint, not startup.
Makefile test/race/vet targets cover only the root module; their defaults do not
set optional data paths or extend the installed-suite timeout.

### Fuzzing

From the root module:

```sh
go test ./internal/protocol -run '^$' -fuzz FuzzRead -fuzztime 10s
go test ./internal/assets -run '^$' -fuzz FuzzEVE -fuzztime 10s
```

## Test data and opt-ins

Use absolute paths, especially before changing directories. Some packages
normalize relative SQL paths from the module root; others use their own test
working directory. Do not rely on a shared relative-path convention.

| Variable | Input / scope |
| --- | --- |
| `WONDERLAND_TEST_ASSETS_DB` | Current read-only imported `assets.db`; SQL integration/story/fixture checks. Current schemas are in [database upgrades](ASSET_DATABASE.md). |
| `WONDERLAND_TEST_DATA` | Original native `Data` directory for server-format tests; some `internal/clientassets` tests instead expect client `data/`. Set it for the selected package, not every suite indiscriminately. |
| `WONDERLAND_TEST_CLIENT_DATA` | Original client `data/` for independent native export/decoder checks. |
| `WONDERLAND_CLIENT_DATA` | Original WLRI installation root, including `pic/`, `data/` and `jma/`. |
| `WONDERLAND_SPRITE_CLIENT` | Original installation used by authenticated sprite checks. |
| `WONDERLAND_TEST_DATA_ROOT` | Loose exported repository `data/` for explicit client asset checks. |
| `WONDERLAND_TEST_CLIENT_BUNDLE` | Absolute compiled core `client-assets.zip`; keep its named companion packs together. |
| `WONDERLAND_TEST_GPU` | Set to `1` to opt into real GPU/context comparisons. |
| `GPU_DIFF_DIR` | Optional output directory for failing CPU/GPU comparison images. |
| `SNAPSHOT_DIR`, feature snapshot variables | Output paths for opt-in rendered samples; see [snapshots](#snapshot-outputs). |
| `WLO_SERVER_HOST`, `WLO_ACCOUNT`, `WLO_PASSWORD` | Explicit live-server test host/account; ports 6414 and 6416. |
| `WLO_CREATE_NAME`, `WLO_DELETE_CODE` | Opt into slot-1 creation/deletion using the live account. |

Original native assets are not bundled; opt-in checks may skip without them.
Restore ignored editable media using [asset import](ASSET_IMPORT.md). Keep
native sources separate from editable exports and compiled packs: they test
different contracts. An import/build verifier checks integrity, not gameplay.

## Subsystem checks

### Automated coverage

Tests cover golden wire bytes, fragmented TCP frames, truncated payloads, native record offsets, EVE boundaries, Big5 names, animation timing, login over a duplex connection, administrator authorization, persistent accounts, duplicate registrations, bans, transaction rollback, inventory capacity and quest reward replay. Creation tests cover name reservations, malformed requests, starter grants, map acknowledgment ordering, deletion codes, and reconnect persistence; native asset checks exercise all 60 body/head/element combinations. Peer tests cover concurrent arrival, appearance before movement, map isolation, disconnect cleanup, and failed recipients. World tests cover golden map-info bytes, PreEvent rule order and conditions, every portal lookup priority, and a warp between maps with persistence, departure, acknowledgment and cooldown. Trade tests cover the native AC25 wire (golden offer records), locked offers, metadata preservation, cancellation notices, concurrent acceptance and rollback of both characters. Native asset tests are opt-in because game assets are not bundled. Native-data path handling differs by package; prefer the absolute paths in this guide. Friendship tests cover cross-map requests, native lists and appearance fields, request expiry, capacity, concurrent acceptance, presence across map loads and deletion cleanup. Minigame tests cover start/result packets, native rabbit win/loss rewards, cancellation, replay, chained games, full bags and failed saves. Preference tests cover native settings packets, persistence, malformed commands, failed saves and team/trade request controls. Vehicle tests cover placement/boarding packets, raft wear and wrecking, mounted-slot protection, save failures, portal landing, and login after database reopen.

### Growth and progression checks

```sh
go test ./internal/game ./internal/battle ./internal/server -run 'Test(ElementalGrowth|PlayerFighterConfiguredGrowth|CompiledElementalGrowth)'
```

Potential Pills and compounding also have focused game/server transaction,
probability-boundary, progression and rollback tests; select their `TestPotential`
and `TestCompounding` prefixes as appropriate.

### Compounding checks

Existing sensitive-resource tests cover metadata, rectangular
placement, full bags, locks, ownership gates, save/receipt failure and replay;
Additional tests cover rank sums, book caps, fallback, base preservation, curve ordering,
multi-input SQL consumption, proficiency, forced-catastrophe consumption and
concurrent requests.

```sh
go test -race ./internal/game ./internal/server -run '^Test(Compounding|Compound|Alchemy)' -count=1
```

For client controls, run from the root in a separate shell/block:

```sh
cd client
go test ./wlo/app ./wlo/inventory -run '^TestCompound' -count=1
```

Manual native-client fixtures are in the [compounding acceptance batch](#compounding-acceptance-batch).

### Native-format verification

A committed artificial golden record checks every decoded byte, text reversal,
Big5, negative stats and arithmetic wrap boundaries. Native-data tests compare
all decoded records in file order against independent reference SHA-256 hashes.
No original game data is committed with the golden fixture.

From the repository root in Bash:

```sh
WONDERLAND_TEST_DATA="$PWD/../Wonderland-Private-Server/Data" \
WONDERLAND_TEST_CLIENT_DATA="$PWD/../Wonderland-Client/data" \
go test -race ./internal/assets -count=1
```

In PowerShell, set the two environment variables before running the same Go
command. Server SQL integration checks use `WONDERLAND_TEST_ASSETS_DB` pointing to an
absolute path for the current imported catalog. Native-format checks above
verify the independent source decoders. Catalog decoding and protocol test clients do not establish
original game-client compatibility.

### World simulation checks

```sh
go test -race ./internal/world ./internal/assetsql -run 'Test(Terrain|WorldSimulation|WildMonster|Proximity|InstalledSQLWorldSimulationGeometry)'
go test -race ./internal/server -run 'TestWorldSimulation|TestWildMonster|TestProximity|TestWorldVisibility|TestTent|TestVehicle'
WONDERLAND_TEST_ASSETS_DB=/absolute/path/to/migrated/assets.db go test ./internal/assetsql -run TestInstalledSQLWorldSimulationGeometry
```

Coverage includes golden movement bytes, current-position snapshots, single
waypoint return, looping patrols, blocked patrol legs, signed bounds, roaming
leashes, town exclusions, static props, script pauses, monster respawn,
collision rejection before saving, visibility/instance isolation, recipient
failure isolation and SQL upgrade preservation. Installed-grid verification
checks the terrain census and Ship Deck geometry. Native-client and multiplayer
acceptance are still required; synthetic tests do not establish that acceptance.

### Teams and instance checks

Focused automated tests cover pet mode/release requests and authoritative replies,
recruitment validation, invitation acceptance/decline, teammate level versus HP,
SQL instance migration/authority, malformed packets, room capacity, minimum levels,
leadership, disconnect cleanup and unavailable starts. Server tests run under the
race detector. Rendered Teams and Instance windows are compared with the supplied
screenshots.

Manual acceptance: log in with two clients, form a party, confirm both level/HP
rows, transfer leadership, leave, then select/rest/ride/dismiss a disposable pet.
Create a room on an eligible character, join on a second character, refresh the
browser, leave and disconnect the leader. Start must display the pending message
without changing maps or granting rewards.

### Minigame checks

From `client/`:

```sh
go test ./wlo/minigame
go test ./wlo/app -run 'Test(MoleFlow|HunterFlow|AdditionalLocalMinigameFlows|NativeArcadeDispatchAndCleanup|UnportedMinigame)'
```

To render the newly implemented games using extracted WLRI assets:

```sh
ARCADE_SNAPSHOT_DIR=/tmp/wonderland-arcade \
  go test ./wlo/app -run '^TestAdditionalMinigameSnapshots$' -count=1
```

This writes separate explanation and play PNGs for each supported new kind.
It does not purchase anything. The existing `MOLE_SNAPSHOT` and `HUNTER_SNAPSHOT`
checks remain available. After adding native ports or changing their references,
regenerate the client inventory from the repository root:

```sh
python3 tools/decompile/catalog.py
```

Fruit-machine and dodgeball coverage includes `TestMarioFlow`,
`TestArcadeFruitMachines`, `TestThrowFlow` and `TestThrowStartSceneVariant`.
From `client/`, render their explanation/spin/play samples with:

```sh
MARIO_SNAPSHOT=/tmp/mario.png go test ./wlo/app -run '^TestMarioSnapshot$' -count=1
THROW_SNAPSHOT=/tmp/throw.png go test ./wlo/app -run '^TestThrowSnapshot$' -count=1
```

### Login checks

The capture/live-server results below were recorded during earlier login
bring-up. They describe historical evidence and available checks, not a rerun
as part of this documentation reorganization.

`client/wlo/app/reference_test.go` renders the empty server list, the server list with a region selected, and the account form. All three match the captures in `client/reference/screenshots/Login/` exactly.

Other tests:

- `flow_test.go` drives a connection to a fake server, checking action 0, the 1/9 reply, typing, Enter and the exact 63/4 bytes.
- `server_test.go` runs the same flow against a live server, including a rejected and an accepted login. It is opt-in: set `WLO_SERVER_HOST`, `WLO_ACCOUNT` and `WLO_PASSWORD`. It passed against this repository's server.
- `login_test.go` holds golden bytes for 63/4, plus tests for the validator, status records, reframing, SERVER.INI parsing and disconnect texts.
- `seui/editor_test.go` covers the editor's filters, limits and double-byte editing, and the combo box's ordering.

### Character presentation checks

Earlier sprite/selection bring-up recorded the following coverage and comparisons:

- `role_test.go` covers archive lookups against the installed archives, including the weapon order, and the palette rules.
- The opt-in snapshot test renders two starter characters. With body 4/head 0 and body 1/head 0, the hair, hat, clothes, shoes and portraits all line up. One character has neutral colours (444444444 twice); the other has shifted hair and skin. A second frame dresses them in Knight's Cape and Angel Wings+5 (`select_character_costumes.png`); `role_test.go` checks the patch fallback.
- `colors_test.go` covers the digit layout and the shift and clamp.

`selectchar_snapshot_test.go` (opt-in with `SNAPSHOT_DIR`) renders the selection screen for comparison, and the empty screen matches `select_character_empty.png` visually.

These recorded capture comparisons do not establish current native gameplay
acceptance. Use the source test's opt-in output path to reproduce them.

### Character creation checks

Earlier creation bring-up recorded these comparisons and live-account checks:

- `create_snapshot_test.go` (opt-in with `SNAPSHOT_DIR`) renders every step with the reference screenshots' character. Step 2 matches `create_character_stats.png` except 16 pixels in the window corners; steps 3 and 4 differ only in the body's idle frame; the password dialog sits one pixel left and one down of the capture.
- `TestCreatePacket` checks 9/1's layout.
- `TestCreateAgainstServer` (opt-in, with `WLO_SERVER_HOST`, `WLO_ACCOUNT`, `WLO_PASSWORD` and `WLO_CREATE_NAME`) creates a character on a live server and waits for the world-entry packets. Against this repository's server it passes, and the stored character has the chosen look, element, points, colours and starter outfit. The account needs no characters, and a password of at most 10 characters (the field's limit).

Use a disposable live account: creation requires an empty slot 1, and
`TestDeleteAgainstServer` deletes slot 1 with `WLO_DELETE_CODE`. Supply the
explicit host/account credentials from [test data and opt-ins](#test-data-and-opt-ins).

### Exporter checks

From the repository root:

```sh
python3 -m unittest discover -s tools/data_export -v
WONDERLAND_TEST_CLIENT_DATA="$PWD/../Wonderland-Client/data" \
  go test ./internal/assets ./cmd/item-export -count=1
```

### Original-client resource checks

Run only the native resource package from the repository root. Bash:

```sh
WONDERLAND_CLIENT_DATA="$PWD/../Wonderland-Client" \
WONDERLAND_TEST_DATA="$PWD/../Wonderland-Client/data" \
go test ./internal/clientassets -count=1
```

PowerShell:

```powershell
$env:WONDERLAND_CLIENT_DATA = (Resolve-Path ../Wonderland-Client).Path
$env:WONDERLAND_TEST_DATA = (Resolve-Path ../Wonderland-Client/data).Path
go test ./internal/clientassets -count=1
```

These inputs are original resources, not the runtime SQL database or loose
exported `data/`. Use the separate [full-validation procedure](#pre-push-server-validation)
for full client suites; avoid leaving native-data variables set for unrelated
package selections.

### Inventory and recovery checks

Tests cover native envelopes, independent scenes, admission/ownership, complete
metadata, stale/full-bag rollback, ordinal indices, loading/logout/crash recovery,
reservations, and preserving gameplay v9 and asset v2 upgrades.

Use focused `TestTent`, `TestTrade`, `TestAutosave`, `TestResting` and inventory
selections in the owning game/store/server packages. Add race checks when
session ownership or checkpoint behavior changes. For buffered fields, retain
the cases in [test selection and correctness](#test-selection-and-correctness).

### Inventory checks

From the repository root:

```sh
cd client
go test ./wlo/inventory ./wlo/world ./wlo/login ./wlo/seui ./wlo/app -run 'TestInventory|TestFormula|TestEditor|TestCombo|Test.*BaseStats'
INVENTORY_SNAPSHOT=/tmp/inventory.png go test ./wlo/app -run TestInventoryFlow -count=1
INVENTORY_REFERENCE_SNAPSHOT=/tmp/inventory-reference.png go test ./wlo/app -run TestInventoryOriginalSampleSnapshot -count=1
INVENTORY_INTERACTION_SNAPSHOT=/tmp/inventory-interactions go test ./wlo/app -run TestInventoryInteractionSamples -count=1
```

## Compiled assets and workspace checks

Focused tests cover image/JSON edits, transparency, failed-build atomicity,
compression/reuse, per-image invalidation, lossless page formats, native color keys,
trim/atlas offsets, viewport clipping, LRU byte budgets, ZIP64 offsets, safe mounted paths, independent session state,
shared buffers, background packet processing without redraw, sidebar input and
socket disposal. With a real bundle:

```sh
cd client
WONDERLAND_TEST_CLIENT_BUNDLE="$PWD/../bin/client-assets.zip" go test ./wlo/app -run TestCompiledBundleLoginAndSharedWorld
```

For live acceptance, log in as two different accounts, switch between their
previews, and check movement/chat/Remote continue in the background. Remove one
session, verify the observer sees it disconnect, and continue using the remaining
session. Modify one same-size PNG and a JSON value, rebuild, and inspect the change;
resize that PNG and verify the build fails while the previous bundle remains usable.

## GPU parity and snapshots

Graphics tests require an available display/context:

```sh
cd client
WONDERLAND_TEST_GPU=1 go test ./wlo/render -count=1
WONDERLAND_TEST_GPU=1 \
  WONDERLAND_TEST_CLIENT_BUNDLE=/absolute/path/to/bin/client-assets.zip \
  go test ./wlo/app -run '^TestGPUClientParity$' -count=1
```

The app graphics test runs in a subprocess because Ebitengine owns
process-global graphics state and shuts down its native backend when `RunGame`
returns. This keeps graphics teardown separate from later CPU/UI tests in a full
suite; the subprocess uses the same test binary, including race instrumentation.

The renderer test compares all 65,536 native colors, all alpha levels, clipped
blits, scaling, indexed palettes, lighting, all gauge percentages, sparse compiled
canvases, loose image tiles, cache reuse/eviction and GPU-to-GPU copies against the
CPU renderer through real GPU readbacks. The app test compares entire login,
Ship Deck, inventory, skills, settings and two-session frames, collapsed previews
session removal and clipped, partially scrolled session cards. It also checks that normal composition performs no readbacks
and that removal releases the session target. `GPU_DIFF_DIR` optionally saves
CPU/GPU images when the app comparison fails. Run focused CPU regressions and
race checks on these modules when changing shared caches or lifetime behavior.


### Snapshot outputs

Output images are local comparison artifacts, not automatic acceptance reports.
Use a writable output location; opt-in tests can skip when its variable is absent.
Common selectors in `client/wlo/app` include:

| Area | Variables / selection |
| --- | --- |
| Login, creation, map, Social, mailbox, minimap, trade | `SNAPSHOT_DIR`; use the relevant snapshot test, not the entire app package. |
| Inventory and item use | `INVENTORY_SNAPSHOT`, `INVENTORY_REFERENCE_SNAPSHOT`, `INVENTORY_INTERACTION_SNAPSHOT`, `ITEM_USE_SNAPSHOT`. |
| Battle and shortcuts | `BATTLE_SNAPSHOT`, `BATTLE_HOVER_SNAPSHOT`, `HOTBAR_SNAPSHOT`. |
| Compounding and Remote | `COMPOUND_SNAPSHOT`, `REMOTE_SNAPSHOT`. |
| Minigames | `MOLE_SNAPSHOT`, `HUNTER_SNAPSHOT`, `MARIO_SNAPSHOT`, `THROW_SNAPSHOT`, `ARCADE_SNAPSHOT_DIR`. |
| Teams/instances | `TEAM_SNAPSHOT`, `TEAM_DISMISS_SNAPSHOT`, `TEAM_INVITATION_SNAPSHOT`, `TEAM_CONTROLS_SNAPSHOT`, `TEAM_RENAME_SNAPSHOT`, `INSTANCE_SNAPSHOT`. |
| Workspace | `WORKSPACE_SNAPSHOT`, `WORKSPACE_FOUR_SNAPSHOT`, `WORKSPACE_COLLAPSED_SNAPSHOT`. |
| Chat | `EMOTICON_SNAPSHOT`, `EMOTICON_PREVIEW_SNAPSHOT`, `CHAT_RESIZE_SNAPSHOT`. |

The source tests define additional outputs and exact selections. A native capture
can confirm layout without proving server rules or native packet acceptance.

## Benchmarks

The following measurements were recorded in earlier optimization work; timings
and pack sizes are machine/input dependent. They are not current acceptance or
whole-client FPS claims.

The focused first-terrain benchmark for Ship Deck measured 108–127 ms and about
154 MB of allocations when parsing the editable ground export. The first compiled
lookup measured about 6 ms and 2.7 MB, including ZIP directory initialization;
subsequent fresh record lookups measured about 0.14 ms and 50 KB. These numbers
measure terrain retrieval, not total startup, world rendering or peak resident RAM.
Reproduce with:

```sh
cd client
WONDERLAND_TEST_CLIENT_BUNDLE="$PWD/../bin/client-assets.zip" go test ./wlo/world -run '^$' -bench BenchmarkFirstTerrainRecord -benchtime=1x -count=3
```

On an Intel Core i5-13600K, `BenchmarkWorldBackgroundDrawing` for Ship Deck at
camera `(700, 500)` measured **0.774 ms** for the editable-image path and
**0.030 ms** for compiled tile drawing. Both loops allocated zero bytes per draw.
This is a warm-cache, background-only comparison, not an overall FPS measurement.
Direct copies from prepared tile rows avoid a temporary viewport buffer. Run:

```sh
cd client
WONDERLAND_TEST_CLIENT_BUNDLE="$PWD/../bin/client-assets.zip" go test ./wlo/world -run '^$' -bench BenchmarkWorldBackgroundDrawing -benchtime=1s
```


## Findings from the 2026-11-01 audit

These observations apply to commit `1a1672e` and the documentation audit that
followed it. Documentation-only reorganization does not constitute a fresh
execution of every historical test below.

| Check | Observed result and scope |
| --- | --- |
| Server development checks | Focused race tests for game/assets and server compounding, Potential Pills, GM rebirth, gacha, resting, movement and clinics passed. |
| SQL catalog checks | Focused structured round-trip/authority, named effects, gacha compatibility, quest visibility and combat-trial checks passed. |
| Client development checks | Focused app/world/settings checks passed; complete small battle/skills package checks also passed. |
| Full client run before commit | Other packages passed; `TestLoginFlow` could not bind a loopback socket inside the sandbox, then passed with socket access. This does not include all optional GPU/live tests. |
| Full root race run before commit | Incomplete: the server package reached the ten-minute limit while loading installed story data. The run also exposed relative SQL-path failures and a blocked shutdown listener. Do not describe it as a full-suite pass. |
| Corrected reruns | Absolute-path assetsql/livetest checks passed. Focused server race checks, including the interrupted Fred story case, shutdown and world-entry socket cases, passed with needed loopback access. The whole server suite was not rerun with the extended timeout. |
| Documentation audit | Local links/anchors, JSON and generator syntax, reproducible client catalogs, whitespace and preserved source hashes/status/history passed. |

### Read-only Go cache

The sandbox's default `/home/dango/.cache/go-build` was read-only. Errors included
cache write failures and misleading `testing/internal/testdeps` / `runtime/race`
"cannot find package" messages. Selecting a writable `GOCACHE` resolved the build
failures; they were not evidence that those standard packages were missing.
For example, run from the root module:

```sh
mkdir -p /tmp/wonderland-go-cache
GOCACHE=/tmp/wonderland-go-cache go test ./internal/game -run '^TestPotential' -count=1
```

Use an equivalent writable location on other systems. Do not modify the global
cache configuration merely to run an isolated check.

### Loopback permissions and timeouts

`TestLoginFlow`, the graceful shutdown test and authenticated world-entry tests
need local TCP listeners. A denied listener requires an environment with loopback
access; rerun the failed test with that access rather than masking the assertion.
Check cache permissions and data paths before diagnosing a compiler or database
regression. `var/assets.db` existed, but a relative path failed in packages whose
working directory was not the repository root; absolute paths resolved it.

The installed server suite remained CPU-active before timing out. The timeout
interrupted a story test during SQL catalog loading; that specific test passed
on a focused rerun. Extend the package timeout for the next full installed run,
and retain a clear incomplete status until it finishes. Longer limits do not
establish multiplayer load, soak, crash-loss or native-client acceptance.

## Native-client acceptance

The `live-test` command prepares database fixtures and manual acceptance
checklists for the original launcher/aLogin. It does not emulate the client,
start a server, or declare a feature passed. Accounts and character prerequisites
are persisted through the normal Go store; assets come exclusively from the
read-only SQL catalog.

### Prepare and run

From the repository root:

```sh
go run ./cmd/live-test -list
mkdir -p var/live-tests
go run ./cmd/live-test -config config.local.json \
  -scenario fishing-skilled -output var/live-tests/fishing-skilled-01
```

Use a current assets database (schema v15, including the item dimensions
introduced in v11). If testing
with an upgraded copy, add `-assets-database /absolute/path/to/assets.db`.
See [asset database upgrades](ASSET_DATABASE.md) if preparation reports a schema
or content error. No asset upgrade or gameplay reset is performed by this tool.
Config paths follow the regular server's working-directory rules; invoke the
command from the project root.

The new directory contains:

- `wonderland.db`: fresh, isolated gameplay database with a tester and observer.
- `config.json`: original listener/startup policies with absolute test database
  paths. The regular gameplay database is never opened or copied.
- `instructions.md`: randomized 10-character usernames/passwords (within the original client's
  8–10-character range), deletion
  codes, selected characters, positions, server startup command, expected outcomes
  and evidence checklist. An independently generated admin token is included.

The directory is owner-only (0700); configuration and instructions are 0600.
Default `var/` output stays gitignored. Console output gives the instructions path
rather than printing credentials. Keep the generated instructions private when
sharing logs or screenshots.

Stop the regular server before using the generated startup command, because its
listener addresses are retained. Configure the legacy launcher with your usual
local-server setup; the instructions list login/world/status endpoints. The
launcher selects the endpoint—it is not automatically reconfigured by this tool.
For remote-client testing, change the source config's bind addresses appropriately
before preparing a run.

Open `instructions.md`, start the test server using its command, then log in with
the supplied credentials. Use a second legacy-client instance for the observer
when checking broadcasts. The observer has no test items or quest progress and
starts beside the tester. The framework grants no GM privileges.

### Scenarios

| ID | Prepared state and acceptance target |
| --- | --- |
| `native-commands` | Two ungrouped Starter Beach characters; appearance, gestures, movement, mall synchronization and reconnect. Unreached title/job/bath commands stay NOT TESTED. |
| `carnie-return` | Starter Beach (11016), where the native Carnie teleport is available; saved visit origin, native teleport/exit, movement and reconnect without autosave conflicts. |
| `fishing` | Configured rod, no learned fishing skill, SQL-derived walkable shoreline; native start/stop, timed catches and interruption. |
| `fishing-skilled` | Same, with the configured fishing skill at grade one; proficiency and reconnect. |
| `fishing-advancement` | Learned skill at grade one with EXP one below the SQL threshold; first-catch advancement, cumulative native display and reconnect. |
| `fishing-full-bag` | Learned skill and every bag cell occupied by a full rod stack; due catches retain proficiency without delivering items. |
| `inventory-raft` | One 4 × 3 raft at anchor slot 1; valid/invalid moves and reconnect without duplicate items. |
| `raft-no-space` | Alternating full rod stacks leave fragmented free cells; raft pickup fails until a contiguous rectangle is freed. |
| `robinson-recovery` | Completed raft dialogue checkpoint, pending recruitment, no Robinson/raft; actor interaction completes recruitment once. |
| `compound-unskilled` | Stackable materials, no learned skills; Primary level 0, consumption, result replies and reconnect without teaching alchemy. |
| `compound-junior` | Three materials with different first/base-sum winners; Junior level 1 outranks Primary level 30, secondary loss and rank fallback. |
| `compound-books` | Four material stacks and Books 1/4, all three tiers learned; one book per native recipe, 3/4/5-input requests, Superior level 1, highest-book additive shift and secondary retention. |
| `compound-fallback` | SQL-verified impossible material/rank pair; every accepted attempt consumes inputs and produces catastrophic junk. |
| `compound-full-bag` | Full inventory with stacked materials; rejected delivery changes nothing, then clearing slot 50 permits one result. |
| `compound-advancement` | Primary level 10 one EXP below the native Formula.dat threshold; first committed attempt advances to 11, subsequent progress and reconnect. |

Shoreline coordinates, rod/skill IDs and catch timing come from installed SQL
rules. Preparation fails when required definitions or geometry are unavailable.
The test does not shorten timers or alter catch weights. Skill tests require
waiting for the configured advancement threshold. Existing authored fishing
uncertainties remain documented in [FISHING.md](FISHING.md); a working client
interaction alone cannot prove exact original probabilities.

### Results and repeatability

Each run starts **NOT RUN**. Mark checklist steps PASS, FAIL or NOT TESTED and
record timestamps, observed outcomes, screenshots and relevant lines from
`server-diagnostics.jsonl`. Debug logging uses the existing server diagnostics;
it is not a full network capture. Use packet captures when an uncertain native
layout cannot be resolved from those logs. Record packet evidence for the
inferred AC7 adapter rather than assuming its acceptance.

Keep the same database/config for reconnect and persistence checks. Afterward,
log out both accounts and stop the server gracefully. Keep failed runs for
investigation. For a clean repeat choose a new output directory; existing
output directories are refused, so previous evidence and characters survive.
Deleting an old run directory is a separate operator action, after its server
has stopped. Preparation failures clean up only the newly created directory.

Fishing captures should show `[23,53,bagSlot,presentation]` when starting with
a rod and `[23,54]` when stopping. The chosen rod must match the selected bag
slot, including when a stronger rod is present elsewhere. The presentation byte
does not change SQL catch rules. Observer rod state uses AC23:123/122 and
must replay after map entry. Delivered catches should show an item flight toward
the character and an AC23:51 acquisition chat line. The current flight adapter
reuses native ground pickup presentation; crowded/reserved ground slots skip
only that visual. Fishing EXP uses AC8:1 stat 111, cumulative on the wire;
SQL progress remains per grade. Reconnect and verify both the grade and overlay.
A full bag should display its warning and retained proficiency without a false
acquisition notice or flight.

The checklist is the acceptance record. Automated fixture tests verify prepared
state, isolation, credential authentication, private file modes, failed setup and
refusal to overwrite; they do not turn manual checkboxes into a live pass.

### Adding scenarios

Add a named `Scenario` in `internal/livetest/scenarios.go` with explicit steps and
expected outcomes, then extend `Character` with its prerequisites. Use descriptive
content constants and SQL-derived definitions/terrain. Keep observer preparation
free of tester rewards. Never edit active sessions or the regular gameplay
store; sensitive test state is created through transactional store APIs before
the server starts.

Add relevant prerequisite tests in `internal/livetest/scenarios_test.go`. Run:

```sh
WONDERLAND_TEST_ASSETS_DB="$PWD/var/assets.db" go test -race ./internal/livetest ./cmd/live-test
```

Native SQL integration checks skip when that variable is absent. They remain
fixture checks, not live-client acceptance.

### Recorded live outcomes

On 2026-10-04, the tester reported these results using the legacy client:

| Run | Result | Evidence |
| --- | --- | --- |
| Carnie Return 02 (`carnie-return-02`) | PASS, tester-reported | “seem to be running properly” |
| Fishing Skilled 02 (`fishing-skilled-02`) | PASS, tester-reported | “seem to be running properly” |
| Carnie Return 01 | BLOCKED by external conditions | Tester attributes failure to outside forces. |
| Fishing Skilled First | BLOCKED by external conditions | Tester attributes failure to outside forces. |

The specific external causes were not supplied. Preserve the earlier runs for
investigation; they do not establish server regressions. The passing reports
confirm overall scenario operation. Individual checklist steps, including catch
flight, observer rod rendering, private notifications and reconnect progress,
remain unverified separately unless their observations are recorded.

### Native command acceptance and fishing parity batch

The following fresh runs were prepared on 2026-10-04. Each has its own isolated
`wonderland.db`, tester/observer credentials and startup command in
`var/live-tests/<run>/instructions.md`. Initial status was **NOT RUN**; the results below were subsequently reported:

| Run | What to verify |
| --- | --- |
| `native-commands-01` | Login/self synchronization, native walking/waypoints, peer gestures and pose cleanup, mall catalog/balance aliases, reconnect and peer replay. |
| `fishing-unskilled-01` (PASS) | Catching without learning the skill, configured timing, owner-only notifications, peer rods, cancellation and reconnect cooldown. |
| `fishing-full-bag-01` (PASS) | No item or success notice for discarded catches, retained proficiency, and delivery after freeing inventory space. |
| `fishing-advancement-01` (PASS) | First due catch advances grade one to two, cumulative display and reconnect, then ordinary progression. |

Run one server at a time using the exact generated startup command. The
prepared configurations referenced the then-current schema-v11 read-only catalog at
`/tmp/wlo-multislot-db-verified/assets.db`; retain that file while testing. To
regenerate against your current imported database, pass its absolute path with
`-assets-database` and select a new output directory. Rod IDs, thresholds and
prepared skill progress are printed in the private instructions. Timers and
reward weights remain those from the selected catalog.

Native title, reborn-job, bath, heartbeat, cutscene and matrix checks count only
when their requests are reached through the legacy client's actual UI flows.
The fixture does not manufacture unlocks or send substitute protocol requests.
Record each unreached request as **NOT TESTED**. In particular, ordinary AC6
walking does not establish acceptance of the inferred AC7 map/X/Y layout.

For fishing, record item IDs, catch times, inventory before/after, skill grade
and EXP, reconnect display, observer rod state and notification privacy. These
runs test the configured fishing behavior; observed samples do not establish
exact original reward probabilities. The already reported successful
`fishing-skilled-02` and `carnie-return-02` runs remain preserved.

#### Batch results and native checklist guidance

On 2026-10-04 the tester reported all checks passing for Fishing Unskilled,
Fishing Full Bag and Fishing Advancement. Advancement also played the native
level-up animation and voice line. These are live passes for the configured
behavior; exact historical reward probabilities remain unresolved.

Native Commands remains a partial pass: ordinary movement and pose controls
work; AC7 was not observed. Observer expressions failed. The log recorded ten
AC32:1 requests and only one outgoing expression packet; the server incorrectly
suppressed transient expressions using the held pose field. Native
`FUN_00432478`/`FUN_00430700` treats expressions as timed animations; AC32:2
`FUN_00438684` controls persistent poses. The corrected expression path and
bounded expression packet diagnostics require an observer retest after restart.

For the numbered native checklist:

- **5:** Open Item Mall, check items/balance, close and reopen, then walk. No
  purchase is required. Report whether the UI works; packet review is developer work.
- **6:** Reconnect the tester with the observer online, then reverse roles.
  Each client should see exactly one of the other character.
- **7:** **Skip.** Mark **NOT TESTED**. There is no client action to perform.
- **8:** Walk, take a screenshot, reconnect, and compare position, appearance
  and Inventory HP/SP.
- **9:** Open **Social → Setup**, save a custom title/nickname, blood type and
  valid birthday, then reopen and reconnect. Retry with a required field missing;
  a warning must leave the connection alive and retain previous blood type/birthday.
  Awarded titles, reborn jobs and bath flows still need separate fixtures.

#### Social setup and movement follow-up

The tester confirmed steps 5 (mall) and 6 (peer reconnect) pass. Social Setup
failed with AC10:1 lengths 8 and 10 rejected as malformed; those are native
length-prefixed nicknames, not contact IDs. Native sender disassembly at
0x2c3ddb establishes `[10,1,length,nickname]`; 0x2c3ea0 establishes
`[10,2,socialCode,bloodType,birthYearOffset,birthMonth,birthDay]`. The birth
UI adds 1900 to its stored year byte. Schema v16 preserves existing rows and
stores these fields. Invalid filled-form values return a warning without a
disconnect or overwriting the saved blood type/birthday. Nickname and profile
are separate native requests and commit separately. This correction needs live
acceptance; do not count the earlier failure as resolved by unit tests.

The native movement log also contains AC7 position resets between AC6 requests.
Go was checking a straight leg from the previous announced destination, which
may still be ahead of the avatar, then resetting the client to that destination
on rejection. Native 15-byte AC6 announcements now validate the endpoint while
the client resolves its route; rejected endpoints leave session state unchanged
and receive no position-reset packet. Short request adapters and server NPC
straight-leg checks retain their current collision validation. Vehicle bounds
checks remain. This is a compatibility correction, not a server-side pathfinder
or speed/anti-cheat implementation. Live retarget/unreachable-click testing is
pending; decoded rejection coordinates are logged at debug level.

#### Native movement disconnect follow-up

Tester disconnected at 21:53:20 on a 15-byte incoming AC6:2. Decompilation
identifies it as a stopped-current-position report. The server now updates the
buffered character position and sends the corrected endpoint to visible peers,
without echoing movement to the sender. Restart, log in Tester and Observer on
the same map, begin walking, and select a pose before reaching the destination.
Also click to change facing while a pose is active. Confirm:

- A received AC6:2 logs `stopped: true` and no malformed rejection.
- Tester stays connected without restarting movement or snapping to the old target.
- Observer sees Tester finish at the corrected endpoint and receive the pose update.
- After waiting for a checkpoint, or logging out normally, reconnect at that endpoint.

Keep this retest pending until observed in aLogin. Invalid endpoints remain
ignored, and the eight trailing check bytes are not yet validated.

#### Latest acceptance correction

The tester corrected the preceding report: friend requests, profile settings and
emotes now work properly. Record these as tester-reported functional passes and
remove the blockers introduced by that typo. This confirmation does not provide
individual results for every accept/reject, persistence or invalid-form subcase;
retain those detailed checks as unverified where no earlier result exists.
AC6:2 stopping still needs an explicit observer/reconnect retest. The previously
reported fishing passes remain valid; AC7 and unreached title, reborn-job and
bath flows remain NOT TESTED.

### Compounding acceptance batch

Prepare each scenario in a separate new directory. For example:

```sh
go run ./cmd/live-test -config config.local.json \
  -scenario compound-books -output var/live-tests/compound-books-01
```

Run the six `compound-*` scenarios above independently. Start with
`compound-unskilled`, then Junior, books, fallback, full bag and advancement.
Stop the previous test server before starting the next configuration. Each
report contains its own 10-character credentials, starting ingredient quantities,
item IDs/names, ranks, material codes, selected skill state and allowed junk pool.
Left-click items in the native Compound bag, choosing one unit when prompted.
Use its named bag slots, not items produced by a previous attempt. aLogin allows
only one Alchemy Book per recipe; use Book 1 and Book 4 in separate attempts. Repeat with a
new output directory to replenish ingredients; preparation refuses overwrites.

Definitions come from the installed assets database. Preparation requires
eligible 1×1 stackable ordinary bulk materials (native types 31–38) of rank at least 5, the relevant alchemy skills,
the selected books and at least one eligible catastrophic reward. Junior and
book fixtures require a reachable normal item with both material bases, so they
can verify successful material retention as well as failures. The forced
fallback fixture checks every candidate at or below the maximum Superior ceiling
for the chosen base pair, with the higher-ranked material first so Superior's
rank-sum primary agrees with the checklist. It does not alter probabilities,
inject a random seed or modify `assets.db`. Fixture tests also verify the forced
fallback for every possible delta using independent controlled rolls.

The book checklist compares each committed log row against
`rank_ceiling = max(1, base_rank + delta)`, where logged `delta` already includes
the highest book. Subtract `book_bonus` to check the original tier range.
Separate live attempts have different random rolls: Book 1 need not produce an
item exactly one rank above a previous no-book attempt. Initial catastrophes
skip delta selection and may log a zero delta/ceiling; candidate exhaustion can
also force catastrophe after the delta is rolled. Junk need not fit the normal
rank ceiling or secondary requirements. Exact weighted probabilities require
statistical analysis, not a handful of successful UI attempts.

Enable **Use Junior Alchemy** for Junior tests and select Superior for Superior
tests. Capture AC23:14/87/101 requests and AC23:9/8/13 replies. Use screenshots for the client
result presentation, inventory quantities and skill display. Keep reconnect
checks on the same prepared database. If aLogin cannot display per-grade EXP,
stop the test server and inspect the isolated database with a read-only query:

```sh
sqlite3 -readonly var/live-tests/compound-advancement-01/wonderland.db \
  'SELECT character_id, skill_id, grade, exp FROM character_skills WHERE skill_id IN (15997,15998,15999);'
```

These tests verify client acceptance and durable mechanics. They start **NOT RUN**;
passing fixture tests do not establish a live-client pass. See
[compounding policy](COMPOUNDING.md) for the compiled rates and full rules.

#### Prepared compounding runs

On 2026-10-06, the finalized fixtures were prepared in these private, ignored
directories. Each remains **NOT RUN**. Open the corresponding `instructions.md`
for credentials and the exact server command; no server was started.

| Scenario | Run directory |
| --- | --- |
| Unskilled | `var/live-tests/compound-unskilled-02/` |
| Junior | `var/live-tests/compound-junior-02/` |
| Books | `var/live-tests/compound-books-02/` |
| Forced fallback | `var/live-tests/compound-fallback-02/` |
| Full bag | `var/live-tests/compound-full-bag-02/` |
| Advancement | `var/live-tests/compound-advancement-02/` |

The `-02` fixtures are superseded by the corrected `-03` batch below. The initial `-01` preparations
remain untouched; the finalized Junior/book fixtures additionally guarantee an
attainable normal two-base result.

#### Corrected native compounding fixtures

The tester reported that the earlier materials (27012/27013) were blocked by
alogin as not compoundable. They are native lucky-bag items; the broad metadata
filter alone did not make them suitable ordinary ingredients. Runs `-01`/`-02`
therefore do not establish compounding acceptance. The unskilled logs contain
no AC23:14 attempts or committed calculations. AC23:115 was incorrectly inferred
to be compounding; `FUN_0021bd90` actually sends AC23:14.

Use `var/live-tests/compound-<scenario>-03/instructions.md` for the corrected
batch (unskilled, junior, books, fallback, full-bag, advancement). Only ordinary
bulk material types 31–38 are eligible fixture inputs. The checklists use native
left-click selection, with one unit if prompted. aLogin's `FUN_0021bd90` limits
recipes to one book; the five-input test uses four material stacks plus Book 4.
Multiple-book highest-bonus logic remains supported by Go and its automated tests.
These fresh runs start NOT RUN; no server was started during preparation.


## Historical validation reports

### Migration verification recorded on 2026-10-04

Compared current handlers and their callers with the recorded reference revision;
25 reviewed source hashes match after CRLF normalization. Ran **175 unique
top-level tests** with the race detector across game, assets, assetsql, protocol,
world, server and store. A stale AC37/AC64 dispatch test matrix was corrected and
its affected checks rerun successfully. This was a focused battery, not the full suite.

| Area                    | Verified outcome                                                                                                                                                                                                                                                                                                                                                                    |
| ----------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Allocation              | Direct/header/target/batch formats preserve greedy 4/2/1-byte amounts, missing/zero defaults and partial-batch behavior. Invalid/overflowing entries retain points; pet overflow now matches the reference guard. Character overflow is deliberately rejected as well. HP/SP remain unchanged apart from clamping to maxima; skill unlocks and allocations commit before snapshots. |
| Generic replies         | All handshake/entity subcommands keep their owning layouts. Unknown bank subcommands now return the private AC45:8 balance snapshot without mutation. No global ACK fallback is invented.                                                                                                                                                                                           |
| Native scene readiness  | Source AC3 short ACK/map broadcast conflicts with native AC3 character/map snapshots (receive address 0x2dfdb3); it remains unsupported. AC12/89/92 synchronization, replay and loading gates are tested independently.                                                                                                                                                             |
| Guild journal           | AC39:1 works without guild membership; repeated requests clear old AC24:4 entries, replace active marks and send completion flags without leaking to peers.                                                                                                                                                                                                                         |
| Inventory rules         | All 256 stack/drop type values match source. Wear guards, metadata, locks, whole grants and full-container rollback pass. Source Tradeable and use-type metadata have no reachable restriction callers; no new trade/body/level restriction is imposed.                                                                                                                             |
| Pet outcomes            | Feeding, alias/capacity/initialization checks for vouchers, sparse client slots, rebirth eligibility/reset/bonus and desertion below 20 pass. Failed saves do not consume vouchers or change published rosters. An unrelated mount survives desertion.                                                                                                                              |
| Recipe order            | Historical first-match alchemy tables preserve authored → Compound2 → Compound order through typed SQL reload. Gameplay now intentionally uses the owner-approved [rank/base model](COMPOUNDING.md) instead. Two-input manufacturing retains first workbench match and atomic costs/output.                                                                                                                                                                |
| Persistence and stories | Restart, rollback, duplicate requests, ownership, receipt failures and full containers are covered; native ship/storm/Monkey/Xaolan/Niss/Fred/Elin scenarios run against imported SQL assets. Buffered walking survives committed resource operations.                                                                                                                              |

The SQL census covered 1,119 maps, 8,181 NPCs, 209 ground items, 2,790 resolvable
warps and 786 manufacturing formulas. Of 7,313 NPCs with events, fresh-character
branch selection found 4,872 runnable branches, 30 with unsupported operations,
and 2,411 with no matching branch for that state. Unsupported here means the
ordinary opcode executor refuses the shape; complete water-gathering branches
have a separate validated controller. The reference also rejects unresolved
reward kinds. This census is selection/decoding coverage, not execution of every
story or proof that every state has been ported. Unresolved shapes remain pending.

Regression entry points are `migration_verification_test.go` in game, server and
assetsql, alongside the affected existing suites. To repeat the new checks:

```sh
WONDERLAND_TEST_ASSETS_DB="$PWD/var/assets.db" go test -race \
  ./internal/game ./internal/assetsql ./internal/server \
  -run 'Test(Migration.*|CommandRegistry.*|AllocationUnlock.*|Bank.*|PetAllocation.*)' -count=1
```

That recorded verification used asset v10 and gameplay v15. Repeating it with
the current server requires the current schemas documented in
[database upgrades](ASSET_DATABASE.md). Verification used
disposable fixtures and asset database copies; installed databases were unchanged.
Interactive aLogin captures, multiplayer acceptance and load/soak/crash testing
remain separate work below.

### Battle validation history

The following evidence was recorded during the earlier battle-port work. It is
not a full-suite result from the 2026-11-01 audit.

Independent raw-byte fixtures cover formation, turn ownership, repeated readiness,
variable actions, malformed/truncated packets, batched stats, recovery and capture.
All 1,832 exported motions are decoded. Additional tests cover scene commands,
world movement exclusion, direct skill/pet targeting, transient HUD state,
background workspace progression, damage distribution and once-only sounds.
CPU/GPU parity includes formation, radial controls, action/damage, the previous
action icon, exit and roaming NPC movement (20 states).
Server rules have an independent recovery-wire golden and low-maximum revival case.

The earlier battle-port pass recorded: full root and client Go suites,
root/client `go vet`, focused battle/protocol/server/app race tests, and 18-stage
CPU/GPU parity with both loose assets and the compiled bundle. Tests used a
60-minute outer timeout rather than the previous ten-minute limit. The final
focused app race run, including the stale-target corrections, completed in
239 seconds.

`make build-client CLIENT_BUNDLE=/tmp/wonderland-battle-assets.zip
CLIENT_OUTPUT=/tmp/wonderland-battle-client` succeeded, packaging 72,829 files
from approximately 6,017 MiB of source assets and compiling the client binary.
Existing authored sprite exports were used without regeneration. Final request
and stale-target corrections have additional focused regression/race coverage.
The server binary also built successfully with
`go build -o /tmp/wonderland-battle-server ./cmd/wonderland`.

The subsequent roaming/previous-action pass has focused world/battle/hotbar
regressions, session-ownership race coverage and 20-state CPU/GPU parity. See
[client roaming](CLIENT_ROAMING.md) for native packet evidence and the explicitly
named initial-zero-speed compatibility case.

### Administration validation history

The earlier administration pass recorded the following; it is historical
evidence, not a rerun during this documentation change:

A headless Chromium smoke renders all 24 administration views and checks read-only
inspectors, including the combined map/quest actor report. Interactive native aLogin acceptance still requires manual validation; the automated checks cover API access,
transaction behavior, native packet layouts and affected gameplay paths.

### Archive validation history

The following native-archive census and `client/ui` verification results were
recorded during earlier inspection work. They are historical evidence, not a
claim that current gameplay or native-client acceptance passed. The normal
client's screens and remaining work are described in [the client guide](CLIENT.md)
and [the roadmap](CLIENT_ROADMAP.md).

- Local migration verification passed for all 4,070 files against the SHA-256 manifest. Staging/ZIP/checksum tests, client tests and vet/build pass; the base image census and a 11016.jpg PNG export pass using only local assets. Upload packages have been prepared locally, not published.
- Directory and payload-signature checks pass for `map.JMG`, `images.BMg`, and `item.BMg` in all three client installations. These three base archives are byte-identical across the installations (SHA-256 comparison).
- Full image decoding passes for 326 map images, 2,190 item icons and 268 of 269 base UI images. `HiTurtle_2.bmp` declares 255 palette entries but uses missing palette index 255. It is retained unchanged, reported explicitly by the test, and displayed as a decode error by the workbench.
- A native 11016.jpg frame was rendered through Ebitengine and visually inspected. No player, NPC, UI composition or gameplay compatibility is implied by rendering a background.
- Image payloads are capped at 64 MiB and decoded dimensions at 128 million pixels. Unsupported/corrupt resources display an error and remain browsable. Additional expansion archives have not yet received the same complete image census.
- JMA/JXA and JXAN asset decoding is implemented in `internal/clientassets` and `cmd/sprite-export`. All selected client resources are exported under `data/sprites`; see [the sprite export documentation](../data/sprites/README.md). The native SHA padding and AES key expansion quirks are covered by independent golden vectors and authenticated native sprite tests. Three orphan JXAN files have no corresponding pixel archives in the source directory.

### Source update verification — 2026-11-01

The update ledger covers 38 commits and all 431 changed files through `1ff5bba`.
Source hashes were rechecked byte-for-byte against the pinned archive, and every
Go evidence path resolves. Prototype/platform exclusions are documented in
[SOURCE_UPDATES.md](SOURCE_UPDATES.md); they are not counted as working features.

Focused race checks cover native companion/job rebirth, deterministic boxes,
mission/PreEvent conditions, generalized gathering, proficiency and reinforcement
waves, NPC shopping, personal/persistent bazaars, native tent trees/tools,
manufacturing batch/refund/restart, travel and Laura's one-time exit gift.
Independent packet expectations cover 24-bit speech, reinforcement records,
furniture trees, item/pet/vehicle browse layouts and cancellation acknowledgments.
Client regressions retain 24-bit talk references and ID-over-offset lookup precedence.
Failure paths include full bags, stale quotes/funds, repeated claims, SQL rollback,
scarce-stock races and preservation of instance metadata. The final focused
battery uses:

```sh
GOCACHE=/tmp/wonderland-precommit-go-cache go test -race \
  ./internal/assetsql ./internal/world ./internal/game ./internal/battle \
  ./internal/store ./internal/server \
  -run 'Test(CatalogDefinition|NPCRoutesMigration|QuestLifecycle|MissionVariables|RebirthLevelDeduction|EventSpeech|PoisonDefault|Tent|NativeManufactur|Manufacturing|Bazaar|NativeStall|AlternativeStall|CompactCreation|SpeechTrace|Repair|GMReborn|RebirthJob|CompanionRebirth|Boxed|EventGathering|TeleportScroll|KelanReturn|BambooDragonfly|Reinforcement|StormAndBeach|Robinson|Hotel|CombatSkill|BattleSkill)' -count=1
```

Disposable SQLite backup copies of installed asset v13/gameplay v17 were upgraded
through `database-migrate`, including content and integrity verification. The
result is asset v14/gameplay v18, with 19 boxes, 124 evolution links, 7,130 NPC
buy prices, three fallback shop catalogs, the exit reward and travel tables.
Native poison definitions migrate to 10%/minimum 15. Round trips cover all ten
new catalog roots; separate checks preserve edited tent/poison defaults and
existing owned state. The upgrade also projects 1,512 event-route entries (2,029
points) from retained map sections. A migration regression checks reordered/added
NPCs, edited walking steps, signs/speeds and repeat-upgrade preservation. Native
ship/Fred scenarios and installed manufacturing/tent reference checks passed against the upgraded copy.

The installed databases remain at v13/v17; they were not overwritten. The sibling
working checkout and original source inventory hashes are unchanged. Tests use
`GOCACHE=/tmp/wonderland-precommit-go-cache` to avoid the read-only host cache.
The client port catalog was regenerated (12,130 functions, 900 citations,
228 regions, zero unclassified regions, 29/91 receive commands with branches).
Command coverage is not full interface or feature acceptance.

Both module `go vet` runs and the focused client race checks pass (app: 203.646s).
The complete client race run reached its
30-minute outer timeout during active asset loading, without preceding assertion
failures or race reports; it is incomplete, not a pass. Final validation uses the
full server race suite, the full client suite without `-race`, and focused client
race checks for changed dialogue/event/voice/lookup paths. Loopback socket access
is enabled. The full server race suite passed (server: 793.903s; assets SQL:
77.541s), as did the full client suite (app: 251.513s). These runs preceded the
owner’s instruction to reserve full server validation for pushes. Commands used:

```sh
WONDERLAND_TEST_ASSETS_DB=/absolute/path/to/upgraded-copy/assets.db \
  GOCACHE=/tmp/wonderland-precommit-go-cache go test -race -timeout=60m ./...
cd client
GOCACHE=/tmp/wonderland-precommit-go-cache go test -timeout=60m ./...
GOCACHE=/tmp/wonderland-precommit-go-cache go test -race -timeout=10m \
  ./wlo/app ./wlo/world \
  -run 'Test(Dialogue|NPCTalk|TalkWrap|CatTalk|Question|StepCompleteAcknowledged|RouteEvent|EventTurns|EventMusic|TalksPreserves)' -count=1
```

Interactive aLogin captures, the new Go-client native-form interfaces,
multiplayer/load/soak and crash acceptance remain pending. No default-suite result
implies those optional or live checks passed.


### Resource exhaustion review — 2026-11-01

The timed-out client race run was actively registering skin images in
`picdb.LoadDir`, not waiting on a deadlock. A focused CPU profile attributed about
75% of sampled time to race runtime/external instrumentation. The same installed
client-entry/dialogue test took 1.558s without `-race` and 25.689s with it after
hardening. These are individual local measurements, not production latency or
load acceptance. Each app test constructs an isolated client and repeats cold
resource preparation; production workspace sessions share their resources.

The review found and addressed these amplification paths:

| Finding | Exposure and change |
| --- | --- |
| Quadratic skin registration | The directory was reread/sorted for each image. `LoadDir` now resolves filenames in one enumeration and retains them for deferred loading. Three one-iteration samples at 1,000 tiny images improved from median 121.3ms / 130.0MB allocated to 8.60ms / 6.70MB. Case-insensitive names, PNG precedence, first-directory overrides and compiled/loose rendering are preserved. |
| Unbounded native verification expansion | Editable `sprites.json` trusted its declared expansion size and had no JSON allocation ceiling. Input is capped at 128 MiB and decoded payloads at 512 MiB before decompression. All 107 installed exports fit; the largest is about 67 MiB of JSON and 198 MiB decoded. At most two native decoders run per workspace manager, and close cancels queued work. |
| Oversized/repeated UI decoding | PNG/BMP configuration and compiled dimensions are checked before pixel allocation: maximum side 16,384 and maximum area 16,777,216 pixels. Failed deferred decoding is attempted once per resource load, and skin preparation propagates errors. |

The asset findings require malformed or unusually large local/supplied assets;
this pass did not demonstrate a remote player's ability to supply those bytes.
Bounds cover the reviewed UI/native-verification paths, not every client decoder.
Native verification files are still retained until manager close; an aggregate
disk/cache budget across distinct archives remains a follow-up.

A separate potential multiplayer denial-of-service path remains: synchronous
peer writes in [broadcastWorld](../internal/server/world.go) occur while holding
`worldMu`, and [Session.send](../internal/server/server.go) permits ten seconds
per write. A non-reading peer whose socket buffer fills can delay unrelated
world actions; multiple slow peers can amplify it. Existing packet/connection
limits do not remove outbound backpressure. This is a code-level finding, not
a completed live exploit/load reproduction. A bounded per-session writer queue
with disconnect-on-overflow should move network waits outside the global lock
while preserving packet ordering and transaction receipts. That networking change
has not been made in this pass.

Focused validation covers oversized/sparse JSON, excessive declared expansion,
checksum/length mismatch, oversized PNG headers, decode failure caching,
PNG/case precedence, compiled-image parity and native close/cancellation races.
The queued-close race check passes 20 repetitions. Resource package race checks
pass (picdb: 1.176s; sprites: 5.497s), as do the focused app/shared-workspace checks
(49.485s), JSON guard tests and affected-package `go vet`. Commands:

```sh
go test -race ./internal/clientassets -run '^TestSpriteJSON' -count=1
cd client
go test -race ./wlo/picdb ./wlo/sprites -count=1
go test -race ./wlo/sprites -run '^TestCloseCancelsQueuedNativeDecode$' -count=20
go test -race -timeout=5m ./wlo/app \
  -run '^Test(WorkspaceSharedAssetsAndIndependentSessions|NativePresentationAndAllocation)$' -count=1
go test ./wlo/picdb -run '^$' -bench '^BenchmarkLoadDir$' -benchtime=1x -count=3 -benchmem
```

The full server/client results above precede this resource-hardening follow-up.
Only affected tests are rerun here; full server validation is reserved for a push.


### Migration ownership verification — 2026-11-01

Definitions, embedded defaults, validation and additive asset migrations live
with their owning features. Gameplay v18 DDL lives with teleport points, tent
tools, manufacturing and bazaars; the schema runners coordinate their existing
version and transaction boundaries. Source-labelled modules/tests were replaced
by domain names. The reference ledger is [source-update-inventory.json](source-update-inventory.json).

Focused race checks pass for catalog migration/rollback/administration, NPC routes,
tent/poison defaults, event/mission conditions, speech packets, rebirth/proficiency,
travel, manufacturing, tent tools and bazaars. Preservation checks reenter the v14
upgrade with edited box quantities/NPC prices and deliberately empty skill
links. Existing edits and deletions survive. Generated SQL models are byte-identical
after regeneration; all 431 reference hashes and evidence paths still verify.
Affected-package `go vet` passes. No full server suite was run for this refactor.

```sh
go test -race ./internal/assetsql ./internal/world ./internal/game ./internal/server \
  -run '^Test(CatalogDefinition|StructuredCatalogMigration|StructuredAdministration|NPCRoutesMigration|TentDefaultsMigration|PoisonDefault|QuestLifecycleAndCharacterSubjects|MissionVariablesAndPreEventGroups|EventSpeech24BitGolden|CombatSkillEXP|RebirthLevelDeductionAndLevel199|Boxed|EventGathering|TeleportScroll|KelanReturn|BambooDragonfly|MapTransition|NativeNPCShop|NPCShop|NativeRebirthJob)' -count=1
go test -race ./internal/store ./internal/server \
  -run '^Test(TeleportPoints|Tent|NativeManufacturing|Manufacturing|Bazaar|CharacterMetadataV13|NativeStall|AlternativeStall)' -count=1
go vet ./internal/assets ./internal/assetsql ./internal/world ./internal/game ./internal/store ./internal/server
```


### Pull request integration — 2026-11-01

Five native-client contributions were reconciled with the newer server and Go
client implementation. The PR head revisions are preserved as merge parents.

| PR | Resulting behavior |
| --- | --- |
| [#1](https://github.com/DangoKnight/Wonderland-Gonline/pull/1) | Region kind 3 resolves authored portal events instead of colliding warp IDs. |
| [#2](https://github.com/DangoKnight/Wonderland-Gonline/pull/2) | Subject kind 3 accepts exact body IDs 1–4 as well as gender codes 7/8. |
| [#3](https://github.com/DangoKnight/Wonderland-Gonline/pull/3) | Existing native trade, exact metadata, locked offers and atomic pair commits retained; protocol documentation reconciled with independent fixtures. |
| [#4](https://github.com/DangoKnight/Wonderland-Gonline/pull/4) | Native five-byte slot/price offers, styles, scene-list completion and bag-slot browse keys supported alongside the six-byte compatibility form; guarded pair purchases retained. |
| [#5](https://github.com/DangoKnight/Wonderland-Gonline/pull/5) | Native item prices supplement SQL initialization, fallback buy/sell choices are separate, and existing pet-hotel acknowledgments are retained. |

Focused race checks cover portal areas, every body/gender combination, trading
replay/metadata/rollback/concurrent acceptance, native and legacy stalls, native
cart forms, SQL price initialization/edit preservation and repeated hotel use.
No GitHub status checks, workflow runs or reviews were present on the reviewed
heads. Final pre-push validation passes: the full server suite with the race
detector and the migrated installed asset database (server package: 735.793s),
the full client suite (app package: 179.440s), and `go vet ./...` in both modules.
The initial isolated-checkout run was invalidated by missing client fixtures
and temporary disk exhaustion; these results come from the complete workspace
after removing those temporary copies. Offline migration and database integrity
checks also pass on disposable copies; the regular databases were not upgraded.
Live native-client acceptance has not been rerun for these integrations.

### Story follow-up verification — 2026-11-01

Private Server commits `d1020d3` and `5c36839` add Waterfall Grotto mirror
continuation and Fairy Altar nonholding movies. Focused server race checks pass
(7.407s): all four mirror events continue through subsequent beam/door rules
without replaying earlier branches; runtime actor swaps survive event completion
and reset on a new visit. The unmodified native EVE scripts also pass repeated
turns in both directions for all four events. Fairy Altar events 2/3 emit movie
mode 1, preserve the completion barrier and resume dialogue without a warp;
unrelated events/maps retain their existing mode. Movie expectations use raw
packet bytes independent of production builders.

Affected world race checks pass (1.012s), covering forward selection, PreEvent
and quest visibility, and mission rule grouping. `go vet` passes for game, world
and server. No schema/data changes or installed database upgrades were needed.
Full server validation remains reserved for a push. Native-script execution and
wire checks do not establish live native-client HUD or puzzle acceptance.

```sh
WONDERLAND_TEST_DATA="$PWD/../Wonderland-Private-Server/Data" \
go test -race -timeout=5m ./internal/server \
  -run '^Test(NativeWaterfallMirrorChoices|WaterfallMirrorContinuationAndVisitReset|FairyAltarMovieResumesDialogue)$' -count=1
go test -race ./internal/world \
  -run '^Test(Story|PreEventVisibility|QuestVisibility|MissionVariablesAndPreEventGroups)' -count=1
go vet ./internal/game ./internal/world ./internal/server
```

### Story system migration — 2026-11-01

Existing story-specific selection, continuation, visibility, condition repairs,
recovery, movie/choice modes, recruitment/conversion parameters, atomic reward
checkpoints, battle fallbacks and starter cinematic steps now use typed asset
schema v15 definitions. [STORY_SYSTEM.md](STORY_SYSTEM.md) documents authoring,
SQL ownership, seed preservation and execution limits. Gameplay remains v18.

Focused race validation passes:

- Native EVE story parity (65.419s): eleven Breillat variants and exchanges,
  Fred, Elin, S.Monkey, Robinson recovery, Xaolan rescue, Waterfall mirrors and
  ship/storm transitions. Independent wire bytes and authored scripts retain
  their expectations. Subsequent focused exchange/ship checks pass (27.311s).
- Definition migration, structured storage, invalid-edit rollback, predicates,
  visibility, ordering and outfit checks pass. Generated SQL models reproduce
  byte-for-byte. Historical source hashes remain unchanged and capability-owner
  evidence paths resolve.
- Installed assets v13 upgrade to v15 passes on a disposable copy (18.315s),
  including loading the complete default projection and repeat migration.
  Existing edits and intentionally empty story definitions survive reentry into
  the v15 upgrade; an injected seed write failure rolls the schema back.
- Typed administrator publication/reload passes (64.317s): optimistic versions,
  active-interaction guards, immutable old snapshots and refreshed policy indexes.
  Existing general definition-edit rollback tests also pass.
- Event/pet/battle integration checks pass (20.230s), including legacy reward
  ownership, replay/capture exclusions and raw speech packets. Custom quest kill
  counters and live administration checks pass separately (6.125s).
- New content at unrelated map/event IDs runs through policies alone. Clearing
  definitions does not restore compiled seeds. Automatic continuation cycles
  release their interaction. Movie-handshake triggers remain map scoped.
  Recruitment preserves fresh SQL state and buffered walking, and replay does
  not grant another companion. Atomic exchanges can be authored independently
  of appearance-conversion content.

Affected-package `go vet` and the server build pass. The regular databases remain
unchanged. No full server suite was run for this development migration; it is
reserved for pre-push validation. Live native-client HUD, puzzle and multiplayer
acceptance remain separate from script/wire tests.

Representative commands (run from the repository root):

```sh
go test -race ./internal/assets ./internal/assetsql ./internal/world ./internal/game \
  -run '^Test(Story|Structured|CatalogDefinition|QuestDefinitions|PreEventVisibility|QuestVisibility|Elin|Replace|ReloadEvents|FindBranch|MissionVariablesAndPreEventGroups|StarterScene)' -count=1
WONDERLAND_TEST_DATA="$PWD/../Wonderland-Private-Server/Data" \
  go test -race -timeout=8m ./internal/server \
  -run '^Test(InstalledBreillat|Breillat|NativeXaolanRobbery|Xaolan|NativeFredFinale|NativeMonkeyRescue|NativeElin|NativeRobinson|NativeWaterfall|NativeStarterShip|Storm|Spider|StarterShip|Monkey|Story|NissRest)' -count=1
WONDERLAND_TEST_ASSETS_DB="$PWD/var/assets.db" \
  go test -race -timeout=5m ./internal/assetsql -run '^TestStoryInstalledMigration$' -count=1
WONDERLAND_TEST_ASSETS_DB="$PWD/var/assets.db" \
  go test -race -timeout=5m ./internal/server -run '^TestStorySQLPublicationAndReload$' -count=1
go test -race ./internal/server \
  -run '^Test(Story|NPCConversation|NPCEventRefusals|RegionsAndDoor|AreaRegion|EventSpeech|RecruitSelect|PetGrowthAndConditions|LegacyCombat|LegacyCompanion|GMFreshPetRecruit|ScriptedSharedProp|Waterfall|Fairy|Storm|Spider|Starter)' -count=1
go vet ./internal/assets ./internal/assetsql ./internal/world ./internal/server ./internal/game
go build -o bin/wonderland ./cmd/wonderland
```

### Story runtime optimizations — 2026-11-01

The SQL-backed story migration now preorders candidate indexes, compiles grouped
predicates, uses allocation-free first-match and ordinary multi-match iteration,
caches unconditional event repairs and batches SQL projection reads. The SQL
batch is local to one read and transaction; it does not cache player outcomes or
replace transactional resource checks. See [STORY_SYSTEM.md](STORY_SYSTEM.md#execution-and-persistence)
for reload and immutable-program ownership.

Permanent benchmarks use the default authored story definitions and disposable
story-only SQL tables. Medians of three 300ms runs on an Intel i5-13600K,
Go 1.27.3, linux/amd64, GOMAXPROCS=20:

| Operation | Before | After | Allocations before → after |
| --- | ---: | ---: | ---: |
| First movie policy | 175.6ns | 48.12ns | 2 → 0 |
| Gated visibility policy | 692.2ns | 82.19ns | 3 → 0 |
| Unmatched policy | 35.71ns | 37.37ns | 0 → 0 |
| Multi-match visibility iteration | 665.5ns | 164.2ns | 3 → 0 |
| Unconditional repair of a known event | 277.2ns | 14.37ns | 5 → 0 |
| Complete story SQL projection | 7.01ms | 3.47ms | 46,527 → 26,448 |

The projection now uses 45 queries instead of 391, with approximately 1.60MB
allocated instead of 2.92MB. SQL is loaded at startup or administrative publication,
not per story step. These are isolated operation measurements, not end-to-end
server latency or concurrent-player capacity results. The miss path stays close
to its prior cost; startup compilation moves work out of repeated gameplay reads.

Focused race checks pass for authored global/local precedence, selection before
actions mutate quest state, iterator early termination and lists above 256
candidates, compiled/ad hoc gate grouping, current SQL-character rechecks,
cross-player immutable repair reuse, conditional edit ordering and reload cache
invalidation. Batched SQL round trips preserve sibling ownership, row order and
nil/empty children; each required table is fetched once. Tests also retain
ordinal/query/cancellation failures, skip unused child tables, read edits afresh
within the same transaction and verify rollback. The combined affected-package
selection passes (assetsql: 5.491s).

Native-story race parity passes (67.882s), including the migration's existing
raw-script and wire expectations. Administrator publication/reload passes
(70.724s), and installed v13-to-v15 migration plus repeated loading passes on a
disposable database copy (17.929s). Generated SQL output reproduces byte-for-byte;
affected-package/generator vet and the server build pass. The optimization adds
no schema version or migration. Regular databases remain unchanged. Full server
validation remains reserved for a push, and live native-client acceptance has
not been rerun.

```sh
go test ./internal/world ./internal/assetsql \
  -run '^$' -bench '^BenchmarkStory' -benchtime=300ms -count=3 -benchmem
go test -race ./internal/assets ./internal/assetsql ./internal/world ./internal/game \
  -run '^Test(Story|Structured|CatalogDefinition|QuestDefinitions|PreEventVisibility|QuestVisibility|Elin|Replace|ReloadEvents|FindBranch|MissionVariablesAndPreEventGroups|StarterScene)' -count=1
# Native and installed-SQL checks use the migration commands immediately above.
go vet ./internal/assets ./internal/assetsql ./internal/world ./internal/server ./internal/game ./tools/asset-schema-generator
go build -o /tmp/wonderland-story-server-check ./cmd/wonderland
```

### Server component layout — 2026-11-01

The server root now has 34 implementation files and 39 integration-test files,
down from 238 Go files. Related command adapters and tests are grouped by
feature. Independent log capture/tracing and 32 packet builders live in
`logging` and `wire` subpackages. Pure speech, text-mail and tent-tree goldens
and trace tests moved beside their implementations with their raw expected bytes
intact. Root constructor/session/administration APIs remain compatible.

A one-off declaration comparison against the pre-layout workspace found all
1,206 original functions and tests. After accounting for moved package names,
their signatures and bodies are unchanged except that `New` now calls the
logging handler constructor, which wraps the same handler and buffer. Session
ownership, world locking, authored tables and SQL transaction boundaries remain
unchanged. Source-inventory owner/test paths now resolve to the new files;
historical source hashes and revisions are preserved.

Validation passes:

- Logging and wire package race checks: 1.007s and 1.009s. Existing root tests
  also retain concurrent bounded/redacted snapshots and capture isolation.
- Focused server race checks: 60.734s, covering dispatch/gates, log integration,
  world visibility/concurrency, trading/stalls, NPC shops, banking, tents,
  companion control, Potential Pills, mall checkout, Lucky Draw, story policies,
  reloads, mail, native stopping, compounding, manufacturing, fishing and recovery.
- Native-story race parity: 60.591s, including Breillat, Xaolan, Fred, Elin,
  S.Monkey, Robinson, Waterfall and ship/storm sequences against original EVE.
- Compilation of all server/component tests, affected server/admin/command vet,
  and the server executable build.

Full server validation remains reserved for a push. Regular databases were not
modified, and live native-client acceptance was not rerun. Commands:

```sh
go test -race ./internal/server/logging ./internal/server/wire -count=1
go test -race -timeout=6m ./internal/server \
  -run '^Test(CommandRegistry|RejectedCommand|PacketTrace|AdminLog|RuntimeSettings|WorldVisibility|ConcurrentWorld|Trade|NativeStall|NPCShop|Bank|Tent|PetControl|PotentialPill|MallCheckout|LuckyDraw|Story|GMReload|TextMail|NativeMovementStop|Compounding|Manufactur|Fishing|Resting)' -count=1
WONDERLAND_TEST_DATA="$PWD/../Wonderland-Private-Server/Data" \
  go test -race -timeout=8m ./internal/server \
  -run '^Test(InstalledBreillat|Breillat|NativeXaolanRobbery|Xaolan|NativeFredFinale|NativeMonkeyRescue|NativeElin|NativeRobinson|NativeWaterfall|NativeStarterShip|Storm|Spider|StarterShip|Monkey|NissRest)' -count=1
go vet ./internal/server/... ./internal/admin ./cmd/wonderland ./cmd/live-test
go build -o /tmp/wonderland-server-layout-check ./cmd/wonderland
```

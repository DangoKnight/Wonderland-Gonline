# Testing and validation

This page contains current test recipes, opt-in environments and observed live
acceptance results. Use the repository root for server commands and `client/`
for client commands. The modules are independent.

Start with [test selection](#test-selection-and-correctness), then choose
[subsystem checks](#subsystem-checks). Full suites belong to
[pre-push validation](#pre-push-server-validation). Older per-change validation
transcripts are retained in Git history; they do not describe current behavior.
Authored measurements, source hashes and independent golden tests remain in
their owning files.

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

For measured slow-test causes and optimization priorities, see the
[test performance investigation](TEST_PERFORMANCE.md). It separates repeated
asset/catalog fixture work from production loading costs and race instrumentation.

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
`internal/server/logging`. Socket queue, expiry and close tests live in
`internal/server/transport`. Include the affected component when changing it:

```sh
go test -race ./internal/server/logging ./internal/server/wire ./internal/server/transport -count=1
```

Session/persistence integration tests remain in `internal/server`, grouped by
feature. The [component guide](../internal/server/README.md) maps handlers to
their files. Existing test names and `-run` selections remain valid.

## Pre-push server validation

Run the full server checks only before a push, from the repository root with a
current-schema SQL assets database. The repository seed supports the current
server/catalog checks without client extraction. For the additional installed
legacy-archive compatibility check (`TestInstalledSQLGachaCompatibility`), use an
imported catalog that also retains its original archive rows; the server-only
seed deliberately does not contain those legacy documents:

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
git ls-files -z '*.go' | xargs -0 gofmt -l
npx prettier --check "internal/admin/web/**/*.{html,css,js}"
npx htmlhint --config .htmlhintrc internal/admin/web/index.html
```

The formatting check should print no files; using tracked source excludes local
scratch programs and generated Go files under ignored validation directories.
Run `golangci-lint run ./...` when installed to
match the optional Makefile check. Node/npm is needed for web lint, not startup.
Makefile test/race/vet targets cover only the root module; their defaults do not
set optional data paths or extend the installed-suite timeout.

### Fuzzing

From the root module:

```sh
go test ./internal/protocol -run '^$' -fuzz FuzzRead -fuzztime 10s
go test ./internal/assets -run '^$' -fuzz FuzzEVE -fuzztime 10s
go test ./internal/assets -run '^$' -fuzz FuzzNativeItems -fuzztime 10s
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
| `WONDERLAND_TEST_CLIENT_MEDIA` | Extracted `media/` directory for the independent font-atlas comparison. |
| `WONDERLAND_TEST_CLIENT_FONT` | Matching original `font/TATPC1.TWN` file for that font-atlas comparison. |
| `WONDERLAND_CLIENT_DATA` | Original WLRI installation root, including `pic/`, `data/` and `jma/`. |
| `WONDERLAND_SPRITE_CLIENT` | Original installation used by authenticated sprite checks. |
| `WONDERLAND_CHINESE_SPRITE_CLIENT` | Optional untranslated Chinese installation for a second authenticated sprite/decryption check. |
| `WONDERLAND_TEST_DATA_ROOT` | Loose exported repository `data/` for explicit client asset checks. |
| `WONDERLAND_TEST_CLIENT_BUNDLE` | Absolute compiled core `client-assets.zip`; used by compiled/GPU checks and explicit shared-resource integration groups. Keep its named companion packs together. |
| `WONDERLAND_TEST_GPU` | Set to `1` to opt into real GPU/context comparisons. |
| `GPU_DIFF_DIR` | Optional output directory for failing CPU/GPU comparison images. |
| `SNAPSHOT_DIR`, feature snapshot variables | Output paths for opt-in rendered samples; see [snapshots](#snapshot-outputs). |
| `WLO_SERVER_HOST`, `WLO_ACCOUNT`, `WLO_PASSWORD` | Explicit live-server test host/account; ports 6414 and 6416. |
| `WLO_CREATE_NAME`, `WLO_DELETE_CODE` | Opt into slot-1 creation/deletion using the live account. |

Original native assets are not bundled; opt-in checks may skip without them.
Restore ignored editable media using [asset import](ASSET_IMPORT.md). Keep
native sources separate from editable exports and compiled packs: they test
different contracts. An import/build verifier checks integrity, not gameplay.

### Fixture ownership and test parallelism

Compatible client integration groups use `newClientFixture(t)` to reuse decoded
resources within one sequential parent test. Children still construct independent
sessions and writable profiles. Tests that override a shared definition must copy
it first; language/artwork checks retain independent fixtures. The parent closes
resources after all children finish. Ungrouped `testClient` calls continue to
exercise loose exports even when a compiled bundle is configured.

The quest migration matrix covers all 17 historical versions with at most two
cases concurrently, each with its own database. `-parallel=1` serializes it when
needed. Do not apply `t.Parallel` indiscriminately to client tests: graphics,
locale and mutable asset state have additional ownership constraints. Catalog
batching uses one caller-owned transaction and no cross-read cache.

Focused examples (from the appropriate module root):

```sh
go test -race ./internal/assetsql -run '^Test(CatalogBatched|QuestBranchMigrationFromTranslationAndDevelopmentSchemas)' -count=1
WONDERLAND_TEST_CLIENT_BUNDLE=/absolute/path/to/bin/client-assets.zip go test ./wlo/app -run '^Test(ClientFixtureResourcesRetainSessionIsolation|RightClickPoseBlocked)$' -count=1
```

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

Potential-window checks cover its native unselected/player/pet states, button
hover artwork and Escape, sparse pet-slot mapping, canonical seated Robinson
appearance, base attributes versus potential bonuses, aggregate unlocked pill
counts, reusable attempts, duplicate-request gates and replacement pets:

```sh
cd client
go test ./wlo/inventory ./wlo/app -run 'TestPotential|TestItemUsePotential' -count=1
POTENTIAL_SNAPSHOT_DIR=/tmp/potential go test ./wlo/app -run '^TestPotentialNativeWindowAndReplies$' -count=1
```

Create the snapshot directory first. The snapshot test exports unselected,
player, pet, Confirm/Cancel hover and successful-result views. CPU/GPU parity now
covers 79 stages, including these six Potential states, with the compiled bundle.
Sitting previews use native seated frames only; roster portraits remain available
for pets whose artwork has no seated body frames.

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


## Test environment troubleshooting

Use a writable Go build cache and temporary directory. If the environment's
default cache is read-only, choose an ignored directory with `GOCACHE`; do not
reuse a deleted validation worktree's cache path. Restore the usual environment
after the run. Listener permission failures and missing graphics contexts are
environment failures, not passes or protocol regressions. Preserve the failing
output, correct the environment, and rerun the affected check.

Native fixtures must match the tested decoder. In particular,
`WONDERLAND_SPRITE_CLIENT` must identify the updated client installation with its
matching executable and sprite archives. The older Chinese installation can
fail encrypted-sprite key discovery. Keep fixture failures distinct from skips.

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

Use a current assets database (schema v16, including the item dimensions
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


## Recent validation records

### Clean post-merge full validation (2026-11-07)

Commit `2564304b978263cb35959ba4f14724935c5b9e71` completed a clean full run
from `main`, using installed native/editable fixtures, an isolated copy of the
migrated assets database and the fresh PR #10 bundle. Both full commands exited
zero without corrective reruns:

| Suite | Packages | Passing test/subtest events | Optional skips | Failures |
| --- | ---: | ---: | ---: | ---: |
| Server (`go test -race -count=1 -timeout=60m -p 2 ./...`) | 43 | 2,547 | 2 | 0 |
| Client (`go test -count=1 -timeout=60m -p 2 ./...`, GPU enabled) | 29 | 996 | 42 | 0 |

Server/client vet, server/client/migration/bundle-compiler builds, all three
10-second protocol/EVE/native-item fuzz checks, Go formatting, whitespace,
Prettier and HTML lint passed. Both Python UTF-8 modes completed 78 exporter
tests with one optional skip and no failures. Client GPU parity passed in the
full run. Skips remain the opt-in live-account/reference-capture checks and
unavailable source-font/platform-specific fixtures.

Logs are in `var/validation/postmerge-2564304-20261107/`; disposable test
databases and build outputs were removed during the repository cleanup below.
No code changes were required; this follow-up records the clean merged-code
validation. Live databases and installed game binaries were not modified.

### Repository cleanup validation (2026-11-07)

Removed obsolete local builds, generated caches, test databases, merged review
worktrees and superseded migration backups. Kept the configured and default
databases, newest complete migration backup, current bundle and its companion
packs, current runtime build cache, authored content, research references and
unique legacy exports. Old extraction copies are removed only when their bytes
match the canonical export. Validation logs remain available.

Focused checks passed:

```powershell
go test -count=1 -timeout=10m ./internal/clientbundle ./internal/clientassets ./internal/clientfs
go test -count=1 -timeout=10m ./internal/assetsql -run '^TestRepositorySeed'
git diff --check
```

Native fixture checks used the updated translated client through
`WONDERLAND_SPRITE_CLIENT`; the default older installation lacks the expected
encrypted-sprite key and failed that fixture on the first attempt. The current
bundle mounted successfully and all 74,841 asset entries resolved across its
three companion packs. SQLite integrity checks passed for both retained asset
databases, the gameplay database and the newest backup. No full suite was
repeated for this cleanup.

### Documentation verification (2026-11-07)

The documentation cleanup checked 77 maintained Markdown files and all 489 local
file/heading links with no unresolved targets. All 12 PowerShell examples in the
edited guides parsed successfully. Schema versions (assets 62, gameplay 21),
setup initialization behavior and documented CLI flags were checked against
their owning source files. `git diff HEAD --check` passed. No runtime code changed,
so no additional Go suite was run. Native acceptance records and the preceding
recent validation results were preserved.

### Test performance investigation (2026-11-07)

Reviewed the retained full-run timing logs and profiled selected client,
administration and migration tests. The client right-click guard test, the
administration edit/rollback test (with and without `-race`), and migration
subcases 28 and 49 passed. Read-only SQL query counting and isolated client
construction probes identified repeated fixture loading and catalog query
amplification. No full suite or production/test implementation change was made.
See [measurements, commands and priorities](TEST_PERFORMANCE.md); local profiles
and probe results are in `var/test-performance-20261107/`.

### Test performance optimizations (2026-11-07)

Implemented transaction-scoped catalog batching, shared decoded resources in 15
sequential client integration groups, and two-worker migration coverage. The
matched timings and limitations are in [the implementation report](TEST_PERFORMANCE.md#implemented-optimizations).

The entire affected `internal/assetsql` package passed with `-race` (659.061 s).
Selected server integration tests for administrator version/rollback gates,
concurrent reload snapshots, installed skills, voucher exchange and Story SQL
publication/reload also passed with `-race` (337.638 s).
Focused client race checks passed for resource/session ownership and right-click
and AUTO behavior; a further race run of item selection, cancellation and sibling
isolation passed after the final definition-copy adjustment. The 15 groups plus
fixture-isolation checks passed using compiled packs (48.230 s); loose-source,
workspace-profile, Chinese artwork, live-language and connection-hello checks
passed together (57.676 s). The serial and two-worker migration comparisons both
passed all 17 versions. These are selected package/integration runs, not a new
full-server or full-client validation.

Server build, affected server/client vet, generator reproducibility and whitespace
checks passed. The Markdown audit checked 78 files and 502 local links without
errors. Evidence is in local `var/test-optimizations-20261107/`; the earlier
whole-suite baseline remains above. No schema migration, asset re-extraction,
live database edits, commit or push was required.

### Full-suite timing after optimization (2026-11-07)

At the user's request, both full Go suites ran concurrently with the baseline
CPU/GC limits, `-count=1 -timeout=60m -p 2`, native/SQL fixtures and compiled packs.
The server used `-race`; the client enabled GPU checks. Both exited zero without
corrective reruns:

| Suite | Command wall time | Packages | Passing test/subtest events | Optional skips | Failures |
| --- | ---: | ---: | ---: | ---: | ---: |
| Server | 19 min 9.4 s | 43 | 2,555 | 2 | 0 |
| Client | 13 min 36.3 s | 29 | 999 | 42 | 0 |

Combined wall time was about 19 min 9 s because the runs overlapped. Comparable
test-event spans improved by 26.4% server and 23.8% client; skip counts match the
baseline. See [timing methodology and package comparisons](TEST_PERFORMANCE.md#full-suite-timing-after-optimization).
Logs, environment and stopwatch results are in local
`var/test-optimizations-20261107/full/`. This run did not repeat auxiliary vet,
exporter, web-lint or standalone timed-fuzz commands. No code corrections, commit
or push were needed.

### Client runtime investigation (2026-11-07)

Three offline GPU profiling samples covered one/two sessions walking between
Chicken Farm and North Island, plus a stationary procedural gateway, targeting
120 FPS presentation and 60 Hz simulation. Separate cold-audio and compiled
sprite probes passed. They used temporary profiles and no live connections.
The samples found procedural-texture cache churn, substantial CPU gateway work
and synchronous cold-effect decoding; they did not reproduce a half-second
freeze. See [scope, measurements and priorities](CLIENT_PERFORMANCE.md).
Probe source, profiles and JSON logs remain in local
`var/client-runtime-20261107/`; the temporary test was removed. No runtime code
changed and no full suite was repeated for this investigation.

### Client rendering optimizations (2026-11-07)

Implemented stable mutable-texture identities for procedural effects, reusable
GPU upload storage, and invariant gateway geometry/plume calculations. Captured
pre-optimization RGB565 frames match at all 32 sampled phases. Matched offline
profiling found the stationary gateway cache reduced from 128.0 to 9.6 MiB and
mean update cost from 7.47 to 1.74 ms. See
[implementation and measurements](CLIENT_PERFORMANCE.md#implemented-rendering-optimizations).

Focused checks passed (commands from `client/`, with CGO/compiler available for
race checks and the absolute compiled-bundle path configured for integrations):

```sh
WONDERLAND_TEST_GPU=1 go test -race ./wlo/render ./wlo/surface -count=1 -timeout=5m
go test ./wlo/world -run '^Test(Gateway|Marker|HeldMarker|ExitIndicator)' -count=1 -timeout=5m
WONDERLAND_TEST_GPU=1 go test ./wlo/app -run '^Test(GPUClientParity|AreaTitle.*|NativeAreaTitle.*|HeldMarkerUsesDrawingCamera|PlacementDoesNotShowWalkMarker|CompiledBundleLoginAndSharedWorld)$' -count=1 -timeout=10m
go vet ./wlo/app ./wlo/render ./wlo/world ./wlo/surface
go build -o ../var/client-runtime-20261107/wonderland-client-optimized.exe .
```

The render/surface race checks passed in 5.780/4.269 seconds, selected world tests
in 1.358 seconds and app integrations in 16.395 seconds, including full CPU/GPU
client pixel parity. Regression coverage includes ordered mutable updates,
transparency changes, unchanged revisions, retry after failure, resizing and
eviction without exceeding the texture budget. No corrective test rerun was
needed; the first non-escalated gateway command could not read the build cache
and was rerun with access to it. No full suite, schema migration, asset rebuild,
commit or push was required. Logs/profiles remain in local
`var/client-runtime-20261107/`.

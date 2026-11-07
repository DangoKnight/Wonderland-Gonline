# Test performance investigation

Measured on Windows on 2026-11-07, against commit
`2564304b978263cb35959ba4f14724935c5b9e71` with the subsequent documentation
cleanup. The baseline investigation made no implementation changes; the
subsequent optimizations and their measurements are recorded below.
The evidence supports both avoidable fixture overhead and production loading
inefficiencies. It does not indicate that ordinary combat calculations take
minutes or that the full suite has an unavoidable minimum near its current time.

## Implemented optimizations

Nested catalog projections now load each needed child table once within the
caller's SQL transaction. Typed parent keys preserve ownership and ordering.
The lazy batch belongs to one root read, so later reads see intervening writes;
there is no process-wide catalog cache. Presence flags, ordinal/length checks,
cancellation and rollback remain covered by tests. The schema generator owns
this behavior; generated output is reproducible.

Fifteen compatible client test groups now share decoded resources within one
sequential parent test. Every child retains separate UI, network, input,
inventory, settings and user directories. Definition overrides copy the shared
map first. Explicit groups can use `WONDERLAND_TEST_CLIENT_BUNDLE`; ungrouped
tests retain independent loose-export coverage. Parent cleanup closes shared
resources only after all children finish.

The 17-version quest migration matrix runs at most two independent databases
concurrently. A controlled serial/parallel comparison justified this limit.
UI tests remain sequential, and SQL queries remain on their original transaction
rather than being dispatched concurrently.

| Matched measurement | Before | After |
| --- | ---: | ---: |
| Catalog read, SQL query count | 56,999 | 162 |
| Catalog read, elapsed | 1.546 s | 0.816 s |
| Catalog read, cumulative allocations | 639.3 MiB | 434.1 MiB |
| `TestRightClickPoseBlocked`, loose assets, elapsed | 29.17 s | 3.70 s |
| Same client test, cumulative allocations | 19.13 GiB | 2.32 GiB |
| Migration matrix, non-race serial vs two workers | 22.05 s | 11.92 s |

These runs used the same CPU/GC limits as the baseline. Catalog probes include
query-counting overhead; allocation figures measure turnover, not peak memory.
The migration comparison uses the new batched reader on both sides, isolating
scheduling. This is not a measured whole-suite speedup. Live databases, installed
asset packs and authored golden expectations are unchanged; no database upgrade
or asset extraction is required.

Implementation evidence is in local `var/test-optimizations-20261107/`.
The historical investigation below describes the pre-optimization code.

### Full-suite timing after optimization

Fresh full Go suites ran concurrently on 2026-11-07, matching the earlier run's
`-count=1 -timeout=60m -p 2`, `GOMAXPROCS=4`, `GOMEMLIMIT=1GiB` and `GOGC=75`.
The server used `-race`; the client enabled GPU parity. Native fixtures, the
current SQL catalog and compiled client packs were enabled. Live-account checks
remained disabled. Existing build caches were retained; tests were not cached.

| Suite | Earlier test-event span | New test-event span | New command wall time | Event-span reduction |
| --- | ---: | ---: | ---: | ---: |
| Server | 25 min 56.6 s | 19 min 5.2 s | 19 min 9.4 s | 26.4% |
| Client | 17 min 48.5 s | 13 min 34.3 s | 13 min 36.3 s | 23.8% |

Earlier logs do not retain command stopwatch timings, so percentage comparisons
use first-to-last test events on both runs. New command timings include build
and orchestration work. Both suites overlapped: combined wall time was about
19 min 9 s, rather than the sum of their durations. These are local observations,
not guarantees for other machines or cold build caches.

All 43 server packages and 29 client packages completed successfully. The server
reported 2,555 passing test/subtest events and two optional skips; the client
reported 999 passing events and 42 optional skips. Both commands exited zero,
without corrective reruns. Skip counts match the baseline; passing counts grew
by eight server and three client events from the added regression tests.

The dominant packages now took 1,013.9 s (`internal/server`), 612.0 s
(`internal/assetsql`) and 806.4 s (`client/wlo/app`), versus 1,423.7 s, 871.1 s and
1,060.9 s respectively. Package durations overlap and must not be summed into
wall time. Full-catalog administration writes and remaining independent client
fixtures still account for substantial work.

Commands, environment, stopwatch records, JSON logs and a compact summary are in
local `var/test-optimizations-20261107/full/`; the runner is
`var/test-optimizations-20261107/time-full.ps1`. This timing covered the full Go
suites, including GPU checks and normal fuzz seed execution; it did not repeat
standalone timed fuzzing, Python exporter checks, vet or web lint.

## Recorded full-suite baseline

The retained clean-run JSON logs are under local
`var/validation/postmerge-2564304-20261107/`. That run used `-count=1 -p 2`,
`GOMAXPROCS=4`, `GOMEMLIMIT=1GiB`, `GOGC=75`, native/SQL fixtures and client GPU
opt-ins. The server used `-race`; the client did not.

| Package | Recorded elapsed time |
| --- | ---: |
| Server `internal/server` | 1,423.7 s (23 min 44 s) |
| Server `internal/assetsql` | 871.1 s (14 min 31 s) |
| Server `internal/world` | 48.0 s |
| Server `internal/store` | 35.5 s |
| Client `wlo/app` | 1,060.9 s (17 min 41 s) |
| Next-slowest client package, module root | 14.8 s |

Package timings overlap: do not sum this table into wall time. The span between
recorded events was 1,556.6 seconds for the server and 1,068.5 seconds for the
client. These spans omit work before the first test event, including some build
time. The two dominant server packages account for about 92% of summed package
elapsed time; client `wlo/app` accounts for about 95% of its module's total.

This is broad repeated work, not one runaway test. The ten longest top-level
tests account for only 24% of summed server test durations and 14% of client
durations. Fifty-four server tests directly call `installedCatalog`; their
recorded top-level durations total 612 seconds. Those durations include their
assertions and other setup, so they are not a measurement of loading alone.

## Client: repeated complete initialization

The [client fixture](../client/wlo/app/reference_test.go) constructs a new client
with loose `../../../data` for each call. `enteredClient` adds world entry;
many table-driven subtests call it independently. This helper does not use
`WONDERLAND_TEST_CLIENT_BUNDLE`. The dedicated GPU parity fixture does support
that variable and shared resources.

Profiling `TestRightClickPoseBlocked` alone, with its 14 subcases, produced:

- 29.17 seconds test elapsed time and approximately 19.13 GiB cumulative Go
  allocations. This is allocation turnover, **not peak resident memory**.
- `app.New`: 57.6% of sampled CPU, including `Resources.prepare` at 50.9%.
- Skin loading: 36.7% of sampled CPU. Native sprite verification decoding also
  consumed substantial CPU on its background path.
- PNG/DEFLATE decoding, JSON, image conversion, checksums and temporary decoded
  sprite storage dominated the profile. These percentages overlap in the call
  tree and must not be added together.

The blocked-click checks do not require all that artwork to be decoded again.
The cost belongs largely to their fixture, rather than waiting for a game timer.
Several existing tests already use small synthetic clients, such as
`shoreClient` in [water travel tests](../client/wlo/app/water_travel_test.go).

### Controlled construction comparison

A separate probe constructed four clients per configuration using existing
`app.New`, `Options.Shared` and `Resources` APIs. It did not enter the world or
render a complete test scenario. Each configuration ran in its own process,
with separate probe-only user directories, the same CPU/GC settings, and no
network connections. It did not flush the OS filesystem cache.

| Asset source / ownership | Four constructions, total | Median construction after first | Cumulative allocations |
| --- | ---: | ---: | ---: |
| Loose exports, independent resources | 5.368 s | 1.340 s | 3,703.6 MiB |
| Loose exports, shared resources | 1.461 s | 0.028 s | 1,033.8 MiB |
| Compiled packs, independent resources | 2.271 s | 0.537 s | 2,529.1 MiB |
| Compiled packs, shared resources | 0.695 s | 0.024 s | 711.6 MiB |

Reusing resources made subsequent loose-source constructions approximately
48 times faster in this small probe. This is **not** a measured 48-fold suite
speedup. World setup, state, rendering and assertions remain additional work.

Normal [workspace sessions](../client/wlo/app/workspace.go) already share
immutable definitions and decoded caches through
[Resources](../client/wlo/app/resources.go). Consequently much of the repeated
work observed in tests is absent when adding ordinary sessions. Compiled assets
also benefit real client startup; loose export decoding is a development path.

Safe improvements are smaller fixtures for input/state-only tests and scoped
resource reuse for compatible integration cases. Each client must retain fresh
UI, clocks, network state, inventory and settings. A child test must not close
shared sprite resources still used by its siblings. Language changes, artwork
edits and tests mutating definitions require isolation. Retain independent
loose-export, compiled-pack and GPU integration coverage.

## Server: catalog query amplification

A read-only, instrumented `assetsql.LoadTransaction` against the configured
487 MB catalog loaded 1,119 maps and measured:

| Measurement | Result |
| --- | ---: |
| Elapsed time, without race instrumentation | 1.546 s |
| Cumulative Go allocations | 639.3 MiB |
| SQL queries | 56,999 |
| Distinct tables read | 162 |
| Queries against map child tables and their parent map table | 50,575 |
| Event-operation child-table queries alone | 23,723 |

These are local warm-system observations, not cold-disk guarantees. Query
counting adds some overhead; allocation sampling and race profiles are separate
measurements.

The [generated catalog readers](../internal/assetsql/catalog_storage_generated.go)
query children separately for each map, event, branch, NPC route and other
parent. An inspected operation lookup uses its composite primary-key index;
missing indexes are not the main explanation for tens of thousands of queries.
The [generator](../tools/asset-schema-generator/main.go) already selects a batched
reader for Story, but not for Maps and many other datasets.

Extend the existing [per-read Story batching](../internal/assetsql/story_read.go)
to other nested catalog families. Reading each required child table once and
grouping by parent should remove many queries. Preserve a consistent SQL
transaction, deterministic order, absent/empty flags, length/ordinal validation,
context cancellation and a fresh read after writes. Change the generator and
regenerate its output; do not hand-edit generated readers. This is a measured
optimization target, not an implemented or benchmarked fix.

The [installed server fixture](../internal/server/characters_test.go) reloads
the catalog each time. Pure gameplay tests can instead use appropriately small
catalogs; compatible read-only fixture reuse could reduce integration overhead.
Mutable catalogs need independent copies, and migration/edit tests must continue
to exercise actual SQL ownership, rollback and administrator edits.

### Administration reads and writes

[ReadDefinitionRecord](../internal/assetsql/administration.go) loads the complete
catalog even for one NPC. [EditAsset](../internal/server/admin_assets.go) reads
the current definition, builds another catalog candidate, validates it, and
persists the complete catalog through `SaveEditedCatalog`/`writeCatalog`.
The edit path also holds the world lock.

Profiling `TestAdminDefinitionsVersionRollbackAndInteractionGates` without
`-race` took 20.72 seconds and approximately 6.20 GiB cumulative allocations.
Catalog loading accounted for about 54% of sampled CPU; Maps alone accounted
for about 40%. SQL/GORM query and write work dominated rather than combat logic.

Targeted definition reads and, later, targeted transactional writes could improve
administration responsiveness and integration-test duration. Keep stale-version
checks, global content validation where required, rollback, presence metadata
and coherent snapshot publication. This is more involved than batching reads.

Catalog loading is paid at startup, reload and administration boundaries;
ordinary gameplay uses SQL-derived in-memory definitions. Administration edits
holding the world lock can still pause gameplay. These measurements do not
profile normal combat throughput or establish a concurrent-player capacity.

## Costs that should remain represented

The same selected administration test took **80.77 seconds with `-race`** versus
20.72 seconds without it, about 3.9 times longer under the controlled settings.
Its race-enabled command took 93.98 seconds overall; the test package reported
82.01 seconds, leaving roughly 12 seconds outside package execution. Build,
linking and command orchestration are distinct from test-body time.

Keep race checks for concurrent behavior and pre-push validation. Removing them
would hide defects rather than address the underlying repeated work.

The historical migration matrix covers 17 starting versions and repeats migration
to verify preservation and idempotence. It took 123.11 seconds in the recorded
race-enabled full run. Focused non-race profiles of versions 28 and 49 took
1.75 and 0.57 seconds respectively, largely in SQL migration, reading/writing
Story rules and fixture preparation. They are not matched before/after results
for the complete matrix. Preserve the version, rollback and deletion coverage;
consider a reusable immutable source fixture copied into independent databases.

Other justified costs include durable SQLite operations, production-strength
credential hashing, native decoder integrity, GPU pixel comparisons and timed
fuzzing. Real sleeps/timeouts occur in transport tests, but the profiled slow
paths above were doing CPU/I/O work. Many animation and gameplay tests already
advance injected clocks instead of waiting in real time.

`-count=1` disables cached test results, not Go's build cache. The race build has
its own cache entries. The recorded validation also limited package execution to
two processes and Go work to four CPUs. Raising those limits alone does not
parallelize serial subtests inside `wlo/app` or `internal/server`; indiscriminate
`t.Parallel` or global fixture caches would conflict with mutable state, locale
and resource ownership. A 1 GiB memory limit and allocation-heavy concurrent
tests also make blindly increasing parallelism risky.

## Original recommendations

1. Batch nested catalog reads, starting with Maps, using the existing
   transaction-scoped batching pattern. This benefits server startup,
   administration and many installed-data tests.
2. Replace unnecessary full client fixtures in logic tests; share immutable
   resources within compatible fixture groups. Keep UI/state isolation and
   independent real-asset integration checks.
3. Add a compiled-asset option to the general client integration fixture while
   retaining explicit loose-export checks. It currently applies only in specific
   tests such as GPU parity.
4. Reduce repeated database fixture construction with independent snapshots and
   smaller feature catalogs. Do not cache mutable SQL results across tests.
5. Profile again before optimizing targeted administration persistence, GC settings
   or suite scheduling. Do not predict a whole-suite speedup from constructor
   microbenchmarks or query-count reductions alone.

## Evidence and reproduction

Local JSON logs, CPU/allocation profiles, query counts and construction-probe
sources are in `var/test-performance-20261107/`. They are ignored investigation
outputs, not required server or client assets. All five focused test invocations
passed; no full suite was rerun for this investigation. No runtime/test logic,
live databases, installed asset packs or real user profiles were modified.

To reproduce a focused profile, create an output directory and use the existing
test with the same name. Example from the repository root in PowerShell:

```powershell
New-Item -ItemType Directory -Force var/performance | Out-Null
$env:CGO_ENABLED = '1' # A working C compiler must be on PATH.
$env:WONDERLAND_TEST_ASSETS_DB = (Resolve-Path var/assets.db).Path
$env:GOMAXPROCS = '4'
$env:GOMEMLIMIT = '1GiB'
$env:GOGC = '75'
go test ./internal/server -run '^TestAdminDefinitionsVersionRollbackAndInteractionGates$' -count=1 -timeout=10m -o var/performance/server.test.exe -cpuprofile var/performance/server.cpu -memprofile var/performance/server.mem
go tool pprof -top -cum var/performance/server.test.exe var/performance/server.cpu
go tool pprof -top -alloc_space var/performance/server.test.exe var/performance/server.mem
```

Select the current configured asset database if it differs from the example.
Use `-race` in a separate run to measure its overhead. From `client/`, the
profiled client test is `go test ./wlo/app -run '^TestRightClickPoseBlocked$'`;
give its profile/binary outputs explicit `../var/performance/` paths and use
`-count=1`. Client extraction must be installed. Keep profile binaries with
their matching profiles for inspection and remove them when no longer needed.

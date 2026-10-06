# Live legacy-client tests

The `live-test` command prepares database fixtures and manual acceptance
checklists for the original launcher/aLogin. It does not emulate the client,
start a server, or declare a feature passed. Accounts and character prerequisites
are persisted through the normal Go store; assets come exclusively from the
read-only SQL catalog.

## Prepare and run

From the repository root:

```sh
go run ./cmd/live-test -list
mkdir -p var/live-tests
go run ./cmd/live-test -config config.local.json \
  -scenario fishing-skilled -output var/live-tests/fishing-skilled-01
```

Use a current assets database (including schema-v11 item dimensions). If testing
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

## Scenarios

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

## Results and repeatability

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

## Adding scenarios

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

## Recorded live outcomes

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

## Native command acceptance and fishing parity batch

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
prepared configurations reference the current schema-v11 read-only catalog at
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

### Batch results and native checklist guidance

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

### Social setup and movement follow-up

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

### Native movement disconnect follow-up

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

### Latest acceptance correction

The tester corrected the preceding report: friend requests, profile settings and
emotes now work properly. Record these as tester-reported functional passes and
remove the blockers introduced by that typo. This confirmation does not provide
individual results for every accept/reject, persistence or invalid-form subcase;
retain those detailed checks as unverified where no earlier result exists.
AC6:2 stopping still needs an explicit observer/reconnect retest. The previously
reported fishing passes remain valid; AC7 and unreached title, reborn-job and
bath flows remain NOT TESTED.

## Compounding acceptance batch

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

### Prepared compounding runs

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

### Corrected native compounding fixtures

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

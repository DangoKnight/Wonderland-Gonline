# Native command compatibility

These handlers port reachable Private Server action-code implementations. They
use the registry in `internal/server/command_registry.go` and share existing
movement, mall and character persistence helpers. Golden tests use independent
raw expected packet bytes in `internal/server/native_commands_test.go`.
Native-client acceptance is still required; unit tests establish handler behavior.

| Request                                       | Behavior                                                                                                                                                      |
| --------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| AC4 with subcommand                           | Return the requesting character's AC4 appearance, without republishing presence                                                                               |
| AC7 with subcommand and optional X/Y          | Validate movement through AC6, buffer position, then reply `[7,sub,x:u16,y:u16]`; absent coordinates and zero/zero only synchronize                           |
| AC18 with subcommand and optional gesture:u16 | Broadcast `[18,sub,character:u32,gesture:u16]` to the same scene including sender; absent gesture defaults to 1                                               |
| AC183:17                                      | Reply `[183,17,0]`, then `[183,11,9,2]`                                                                                                                       |
| AC186:9 with optional cutscene:u16            | Reply `[186,9,cutscene:u16,1,0:u32]`; absent cutscene defaults to 1                                                                                           |
| AC13:238                                      | Reply `[13,42,character:u32]`, current SQL point/bonus balances, then both catalogs                                                                           |
| AC21:1                                        | Current SQL point/bonus balances, then `[21,1,1,2,...,21]` window slots                                                                                       |
| AC21:2 with slot:byte                         | Purchase one bundle from the ordered nonbonus catalog; use atomic point debit and inventory delivery                                                          |
| AC21:3                                        | Current SQL point/bonus balances                                                                                                                              |
| AC22 with subcommand and optional entity:u16  | Reply `[22,sub,entity:u16,1]`; absent entity defaults to 0                                                                                                    |
| AC226:255                                     | Fixed reference `[238,183,0,255,27:u16,1,29:u16,2,24:u16,0]` matrix and `[225,252]` with ten zero bytes                                                       |
| AC44 with subcommand and optional title:u16   | Persist title, reply `[44,sub,title:u16]`, then map broadcast `[44,1,character:u32,title:u16]`; absent title clears it                                        |
| AC66 with subcommand and optional job:byte    | Persist separate reborn-job metadata, reply `[66,sub,job,1]`; absent job defaults to 1                                                                        |
| AC87 with subcommand                          | Restore HP/SP through transactional vital recalculation, send stat synchronization, then `[87,sub,1]`                                                         |
| AC23:53 / AC23:54                             | Native start `[23,53,bagSlot,presentation]` validates the selected rod; two-byte auto-select remains supported. Stop `[23,54]` retains mall refresh when idle |
| AC90:1 / AC90:3                               | Validated start/stop; reply with actual active state                                                                                                          |
| AC90:2                                        | Attempt a due, transactional catch; early/replayed requests grant nothing                                                                                     |
| AC32:3                                        | Clear event callbacks and arcade interaction even at pose zero; broadcast a changed pose as before                                                            |

## Persistence and gates

Gameplay schema **v12** adds `character_state.title` and `reborn_job` with
zero defaults. Initialization upgrades existing databases transactionally and
preserves other state. Tests exercise v11 upgrade and reopen. No installed
database reset is required. Titles replay on login and peer publication;
reborn-job metadata replays to its owner on login. Actual character job, class
bonuses and rebirth progression remain separate unfinished work.

Self-spawn, heartbeat, movie acknowledgement, mall matrix and mall catalog/balance
queries can run during loading. Other actions use ordinary world, battle and
minigame gates. Paid mall requests are blocked during trades. Pose stop is a
cleanup request and bypasses interaction gates. Map broadcasts retain private
scene isolation and recipient-failure handling.

## Compatibility limits

- AC7's six-byte X/Y layout follows the source reader. The eight-byte
  map/X/Y layout follows its documented intent and remains an inferred adapter;
  reject a foreign map with ordinary position correction. Both layouts retain
  terrain checks, region/encounter processing and vehicle wear. Invalid movement
  is corrected; no SQL write is made for ordinary walking.
- The reference does not validate title unlock ownership or AC66 job values.
  These remain metadata assignments, not paid unlocks. Its AC66 `RebornJob`
  property is separate from `Equip.Job`; assigning it does not grant rebirth,
  a cape, levels or class stat bonuses.
- The source bath handler has no location/equipment eligibility checks. This
  compatibility path does not establish a complete hot-spring rules system.
- The matrix is a fixed compatibility reply, not an editable reward catalog or
  a claim operation. No currency/items are changed by synchronization.
- Source fishing hardcodes item **26001**, which WLRI names **Sky Sword Box**.
  The new documented fishing pool replaces that reward; see [FISHING.md](FISHING.md).

- Optional operands may be wholly absent. Truncated operands and trailing bytes
  are rejected rather than partially mutating state. Further native layouts
  should be added from captures or decompilation evidence.

## Saved fishing references

The replacement files in `docs/References/` provide three kinds of evidence:

| Saved reference                                                                                                   | What it establishes                                                                                                |
| ----------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| `釣りスキル - Wonderland ONLINE Wiki_.html`                                                                       | Older Japanese fishing observations, two-minute interval, catch-count table and full-bag experience behavior       |
| `釣魚エリア_初心者ビーチ - Wonderland ONLINE Wiki_.html`                                                          | Beginner-beach catch observations grouped by simple/advanced/deluxe rod and skill level, including unlearned skill |
| `スキル - Wonderland ONLINE Wiki_.html` and `ＦＡＱ - Wonderland ONLINE Wiki_.html`                               | Skill improves high-grade catch chances; fishing can start without the skill by using a rod at the water's edge    |
| `クエスト_地域別_サウスアイランド - Wonderland ONLINE Wiki_.html`                                                 | Fishing-bait quest consumes a captured pink worm; releasing it also grants a star, and both choices teach fishing  |
| `初心者ガイド - 飄流幻境（Wonderland ONLINE）羅德島傳　日本語wiki Wiki_.html`                                     | WLRI beginner guidance and fishing as a source of magnet material                                                  |
| `【情報】《飄流幻境Online羅德島傳說》釣魚活動開釣！（11_05維護後~11_18） @飄流幻境 Online 精華區 - 巴哈姆特.html` | November 2020 seasonal fishing drops, chest outcomes and event manufacturing recipes                               |
| `【心得】半天釣魚結果統整-羅德島釣魚活動 @飄流幻境 Online 哈啦板 - 巴哈姆特.html`                                 | WLRI 2020 event observations: one catch per minute, samples with/without the skill, subsequent box observations    |
| `《飄流幻境羅德島傳說》 官方網站.html`                                                                            | September 2021 seasonal pearl/chest drops and event furniture recipes                                              |

These saved pages describe gameplay, not AC90 packet definitions. The removed
English-named copy is no longer a reference path.

### Ordinary fishing and progression

Fishing does **not** require the learned fishing skill. The FAQ describes using
a rod beside water; the beginner-beach table includes catches without the skill
for all three rods. Skill improves high-grade catch probability. The skills page's
suggestion that even grade ten might be possible without it is tentative, so
observed table entries must not be treated as exact minimum-level unlocks.

The beach table supplies candidate materials, consumables and fish by rod/skill,
but not exhaustive pools or probabilities. Empty cells are missing observations,
not proof of impossibility. Item grades are sometimes inconsistent between rows.
Older reported rod maxima are six/eight/ten, not verified hard limits. The general
wiki lists catch-count values 14, 35, 79, 152, 263, 422, 635, 913, 1264, 1697,
2220, 2844, 3578, 4430 and 5410 for levels one through fifteen; cumulative versus
per-level semantics and later requirements still need verification.

When a catch cannot fit, the old wiki reports fishing experience with no retained
item. Existing stacks can filter retained catches in a full bag. Implement this
as an explicit fishing transaction outcome, preserving ordinary full-bag grant
behavior elsewhere.

**Interval is version-dependent evidence:** the old Japanese wiki reports
120 seconds; the WLRI November 2020 event report observes 614 catches over
614 minutes. Its one-minute observation is relevant to WLRI, but does not prove
an official permanent interval for every rod/event. Replies disagree on whether
rod type affects speed. Preserve interval as an explicit SQL rule when enabled;
do not present the old two-minute value as universal.

Offline inspection of the existing WLRI exports identifies these candidates:

| ID    | Exported name   | Scope                                                                      |
| ----- | --------------- | -------------------------------------------------------------------------- |
| 37105 | Simple Fishing  | Rod candidate, item type 28                                                |
| 37011 | Great Fishing   | Rod candidate, item type 28                                                |
| 37082 | Luxury Fishing  | Rod candidate, item type 28                                                |
| 15993 | Primary Fishing | Skill candidate                                                            |
| 15994 | Junior Fishing  | Skill candidate                                                            |
| 30585 | Delicious Fish  | Seasonal reward candidate; description says available at a particular time |

These names/IDs are confirmed export fields, not proof of quest mapping or exact
rod tiers/skill progression. Runtime must load their definitions through assets.db,
not reopen exports. Source AC90 item **26001** remains Sky Sword Box.

### Seasonal WLRI events

The November 2020 announcement lists delicious fish, delicious shellfish, ocean
essence and wet boxes as nontradeable event catches. The sample report records
614 catches per character: learned skill produced 70 fish/62 shellfish/16 essence;
unlearned skill produced 84/64/18. These are finite event observations, not exact
ordinary-fishing weights. Zero boxes in the original sample does not imply zero
probability: its later edit reports boxes after maintenance. The chest table also
lists 345 openings but sums to 343 outcomes, so its percentages require independent
validation before being used as exact weights.

The September 2021 official announcement instead lists sea-blue pearls and
plunderer's boxes for September 2–9. Both events contain separate, time-limited
crafting recipes; neither is a permanent catch pool. Chest contents are a second
reward roll after catching the chest. Keep seasonal availability, tradeability,
box pools and crafting windows separate from ordinary fishing definitions.

### Implementation

Fishing now uses SQL rods, weighted catches, shoreline checks, timed delivery and
learned-skill progression. Full bags retain proficiency and discard catches.
Native AC23:53/54 start/stop and legacy AC90 are supported. Peer rods use
AC23:123/122; catches send native acquisition chat and reuse the pickup flight
renderer. Fishing progression uses cumulative AC8:1 stat 111 on the wire,
including reconnect snapshots. These visual changes still need live acceptance.
Authored defaults and
remaining exact-parity questions are documented in [FISHING.md](FISHING.md).

### Live acceptance runs

The expanded `native-commands` checklist separates naturally captured requests
from unreached handlers and checks observer replay, pose cleanup and mall aliases.
Fishing fixtures cover unlearned, full-bag and threshold-minus-one advancement
states. See the [prepared acceptance batch](LIVE_TESTING.md#native-command-acceptance-and-fishing-parity-batch)
for run directories and evidence requirements. Preparation is not live acceptance.

### Expressions and held poses

The live native-command run emits expressions as AC32:1 (not AC18). WLRI
`FUN_00432478` calls `FUN_00430700` to restart a temporary expression animation;
AC32:2 `FUN_00438684` sets the held pose. Expressions are now forwarded on every
request to same-scene observers without overwriting or replaying the held pose.
Only changed held poses are deduplicated. Debug logs include bounded AC32 payloads
for verifying expression IDs and recipients. The tester's original observer
failure is recorded; the correction still needs live retesting.

### Native Social Setup and movement destination recovery

WLRI Social Setup uses AC10:1 length-prefixed custom title/nickname and AC10:2
five profile bytes. This differs from the Private Server AC10 contact adapter.
Go accepts the native forms and persists the profile in typed schema-v16 columns;
existing recognizable contact-ID adapters remain supported. Birth year is stored
as the native offset from 1900; zero/missing blood type or invalid birthday does
not disconnect the client or overwrite the previous profile. Native owner updates
its form locally; observers receive nickname/code updates after commit. Reconnect
appearance carries blood type and birthday. The profile code is kept by its wire
name because its complete semantics are unresolved.

Native 15-byte AC6 announces a destination before walking. Checking a straight
leg from a previous destination can incorrectly reject a client-routed path;
AC7 correction then snaps the avatar ahead to that destination. Native requests
now validate destination walkability/bounds without imposing that straight leg,
and invalid endpoints receive no snap packet. Short requests and NPC movement
retain straight-leg collision validation. This fixes the diagnosed correction
path; live retargeting acceptance remains pending.

### Incoming AC6:2 during native walking

On 2026-10-04 at 21:53:20 (America/Santiago), Native Commands tester session 2,
character 10001 on Starter Beach, disconnected after a 15-byte AC6:2 was
rejected as malformed. The preceding AC6:1 destinations were accepted.
Private Server's AC06 switch leaves incoming Recv2 disabled, so it ignores this
request. Go now handles exactly the native 15-byte AC6:2 as a stopped-position
report. Valid coordinates replace the session's announced destination and are
checkpointed with ordinary buffered walking. Nearby visible players receive
AC6:1 with the corrected endpoint; the sender receives no movement echo or
movement-lock reply. Stop corrections do not run regions, encounters or vehicle
wear again. Active event locks remain authoritative, and invalid coordinates
are ignored without an AC7 snap. Malformed lengths or unknown pose/facing bytes
remain rejected. Bounded AC6 traces capture up to 15 bytes.

Unlike walking's direction range 0–7, stops copy the current packed pose/facing
byte. `FUN_00427150`/`FUN_0042646c` recognize values 0–73 and 99; the stop handler
accepts and preserves those values when publishing the corrected endpoint.
The eight trailing check bytes remain opaque, as for AC6:1. Focused tests cover
checkpointing, peer isolation, no sender echo, packed pose values, invalid
coordinates/lengths and event locks.

#### Decompilation: what sends AC6:2

The native movement-stop routine `FUN_004263e4` (address `0x4263e4`) sends it.
It clears the path counters at role offsets `0x140`/`0x144`, replaces the movement
target (`0x84`/`0x88`) with the current rendered position (`0x20`/`0x24`), and
copies current facing (`0x121`) into the outgoing direction (`0x122`). Assembly
at `0x42642a`–`0x42642e` explicitly selects subcode 2 and action 6 before calling
`FUN_002c2394`; the decompiler obscures this register-based call.

Confirmed callers are:

- Receiving server AC6:2 calls this routine with the supplied movement-lock byte.
  The server command and client response have different payload layouts.
- `FUN_00430f90` changes the local player's pose/action, stops movement through
  this routine, then normally sends AC32:2. The pose-selection UI calls it.
- `FUN_0043b6ac` changes facing while preserving the current pose and calls
  `FUN_00430f90`. Map-click handling routes some clicks through this path when
  a pose/action is active, so a click can produce AC6:2 without selecting a new
  pose. Several gameplay action paths also use the same pose routine.

The common AC6 sender builds a 15-byte frame:
`[6, 2, direction, x:u16LE, y:u16LE, eight movement-check bytes]`.
For AC6:2, X/Y are the stopped current position rather than a future destination.
The last eight bytes combine random selectors, derived check values and reset
state; their full validation rules remain pending. Sending is conditional:
role flag `0x123` must be zero, the sender's `0x212e` flag must be zero, and the
coordinates must differ from the last sent coordinates (`0x94`/`0x98`). A pose
change at an unchanged position therefore need not emit AC6:2.

Evidence: `var/decompiled/aLogin_ca19ee087b60_full.c`, functions
`FUN_004263e4`, `FUN_00430f90`, `FUN_0043b6ac`, `FUN_00438684`, and the AC6
branches of the sender/dispatcher; verified against `var/ghidra/aLogin.exe`.
The exact click/action behind the 21:53:20 disconnect cannot be recovered from
that trace: it contains neither operands nor UI input, and rejection prevented
any following AC32:2 from being recorded. Go now applies the stopped position as described above; native-client live
verification remains pending.

## Character creation starter-skill correction

The legacy `GetStarterElementSkills` helper and its stat progression checks
disagree. Go previously copied the helper, granting Water Icicle Attack (11001)
before 13 STR, Earth Attack (11017) before 16 STR, and Wind Air Attack (15079)
before 5 STR plus 10 AGI. Grants are now derived from zero-requirement entries
in the shared progression table, alongside the avatar stunt. Fresh characters
receive these elemental defaults from the WLRI SQL skill catalog:

| Element | Starter skills                                                        |
| ------- | --------------------------------------------------------------------- |
| Earth   | Rock Attack (15085), Rock Elf Attack (12006), Shield Defence (11057)  |
| Water   | Ice Attack (15091), Water Pole Attack (15097), Detoxification (15100) |
| Fire    | Flame Attack (11016), Blast Attack (11166), Slowdown (11056)          |
| Wind    | Wind Attack (11007), Wind Blade Attack (30002), Speed Up (11052)      |

Stat allocation still unlocks the gated skills at their effective attribute
thresholds, including avatar bonuses. `/clearskills` uses the corrected defaults
and adds skills qualified by current attributes. Existing character records are
not automatically stripped: quest, GM and evolved skills have separate learning
paths, and resetting them requires an explicit operation. Character creation commits
the initial skills before sending the native snapshot. No asset
or database rebuild is required. Native starter names above reflect WLRI rather
than the misleading names in the legacy comments.

## Potential Pills

Native aLogin `FUN_001cce98` sends `[23,126,bagSlot,target]` (exactly four
bytes). Target 0 is the player; targets 1–4 are the session's pet roster slots,
which can differ from saved pet slots. `FUN_001cd1c4`, dispatched by AC23:213,
receives `[23,213,processed,target,newPotential]`. Processed is 1 for a consumed
attempt and 0 for a rejected request; it is not a probability-success flag.

The cap is 12. Super Potential Pill (34350) succeeds with certainty. Normal
Potential Pill (34269) loses potential on failure; Golden Potential Pill (34289)
requires current potential >=10 and preserves potential on failure. The normal
and golden probabilities use the saved official WLRI table in
`docs/References/Potential.html`. Chances for attempted levels +1 through +12 are
100%, 100%, 100%, 70%, 60%, 55%, 55%, 50%, 35%, 20%, 18% and 15%.
`Golden_Potential.html` and `Potential_Fail.html` confirm the failure rules;
golden pills use the same published potential table, since no separate golden
rate is provided. Normal failure subtracts one potential level, as anticipated
by the native warning's previous-level bonus comparison. Both outcomes consume
one pill. Keep legacy AC68:1 free increments disabled.

`internal/game/potential.go` contains the recovered cumulative bonus table from
native address `0x4b7850`: 0, 1, 2, 3, 4, 6, 9, 13, 18, 24, 31, 39, 48. The bonus
applies to each of STR, CON, INT, WIS and AGI for players and pets. Keep allocated
base points unchanged. Derived combat/vital stats, player skill qualification,
pet roster records and progression packets use effective attributes. Existing
out-of-range potential metadata remains preserved; its bonus is bounded to 48.
Increasing maxima does not heal current HP/SP; losses clamp current vitals.
Normal failure forgets newly unqualified elemental stat-tree skills and their
evolutions, preserving unrelated avatar/quest skills. Publish native snapshots
with zero-grade removed entries so aLogin clears its cached skills. Regaining
qualification relearns the forgotten skill at grade one. Do not strip skills on
login. The saved Bahamut potential bonus reference independently confirms the
cumulative bonus table.

The handler validates SQL item ownership, identity, reservation, cap and target;
consumes one pill and saves potential/vitals/newly qualified skills in the same
transaction. Failed persistence sends no success. Adopt the committed snapshot
while preserving pending walking. Failed receipt delivery does not refund a pill.
Only the acting character receives the inventory/result/stat packets. Debug mode
logs at most four request bytes or five result bytes, never arbitrary payloads.

Each committed attempt emits an INFO `potential pill attempt` entry, even
without `-debug`. It records SQL current and attempted potential levels, pill and
target IDs, success percentage, zero-based random roll, resulting potential and
per-attribute bonus delta. Rolls range from 0 to 99; success means
`roll < success_chance_percent`. Guaranteed attempts record `roll: null` and
`random_roll_used: false`. `committed: true` means consumption/result were saved,
even if the later network reply fails. Failed saves after calculation emit WARN
`potential pill attempt not committed` with `committed: false` and the error;
the logged resulting level is then only a rolled-back candidate. Keep probability
rules unchanged while inspecting diagnostic samples.

Pending native detail: verification of pet mercenary eligibility. The native UI
rejects mercenaries through NPC lookup
`FUN_0048259c`; the current typed NPC catalog does not expose that field. Current
pet handling verifies owned party membership, rather than claiming complete
mercenary-rule parity. Native-client pill animation/reconnect acceptance
remains pending. No gameplay or asset schema change/reset is required.

### Initial pet potential synchronization

The pill window reads cached party-pet potential from aLogin role offset 0x1f9a;
it does not issue a separate query in the reported login/open-window trace.
AC15:8 `FUN_0040b1e0` and AC15:1 `FUN_00409820` populate this field from the
byte immediately after rebirth and before job. The old Go records incorrectly
put job there and zero in the following job field. Send the saved potential
byte in that position on initial entry, roster replacement and recruitment.
Keep native client slot mapping separate from sparse persistent pet slots.

AC8 stat37 is not potential: native getter/setter `FUN_004166e4` /
`FUN_00416ebc` addresses role field 0x1fa4, while potential is 0x1f9a.
Remove that incorrect mapping for both players and pets. AC5:3 initializes player
potential; AC15:1/8 initializes pet potential; AC23:213 updates the selected
target after a pill attempt. Do not fabricate a pill result during login.

Reported diagnostic sample: Robinson had saved potential 4; the 60% attempt
rolled 90 and failed to potential 3. The zero shown before that attempt was a
roster-display error. This correction changes synchronization only, preserving
probabilities, saved potential and pill outcomes. Native initial-window and
reconnect acceptance remains pending user retest.


## Native hotbar assignment (AC40:1)

Dragging an item or action onto aLogin's hotbar sends
`[40,1,kind,id:u16,page,slot]`: kind 1 is an item, kind 2 a skill/action,
page is 1–3 and slot is 1–8. Basic Attack is ID 10001; Defense is 60021.
The decompiled `FUN_002aa92c` / `FUN_0029082c` sends this seven-byte packet
**after updating the local hotbar**. No acknowledgement is required.
Inbound AC40:1 instead contains a list of five-byte binding records, as decoded
by `FUN_002aa7f8` / `FUN_002906f8`; a legacy alchemy result is incompatible.

The server validates and accepts native assignments without changing inventory,
currency or learned skills, including during login synchronization and battles.
Debug logging records kind, ID, page and slot. Hotbar bindings are currently
client-local; server database storage and replay on reconnect remain pending.
The Go client saves per-character hotbar preferences beside settings.json and
restores them on character entry, independently of server resource state.

Private Server's AC40 handler implements four-byte synthesis instead. Go retains
that legacy compatibility layout separately. Previously the seven-byte native
assignment was rejected as malformed, closing the requesting connection. Tests
in `internal/server/hotbar_test.go` cover Basic Attack, Defense, item/empty
bindings, invalid layouts and unchanged resources; the existing alchemy tests
verify synthesis still behaves as before.

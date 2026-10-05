# Native command compatibility

These handlers port reachable Private Server action-code implementations. They
use the registry in `internal/server/command_registry.go` and share existing
movement, mall and character persistence helpers. Golden tests use independent
raw expected packet bytes in `internal/server/native_commands_test.go`.
Native-client acceptance is still required; unit tests establish handler behavior.

| Request | Behavior |
| --- | --- |
| AC4 with subcommand | Return the requesting character's AC4 appearance, without republishing presence |
| AC7 with subcommand and optional X/Y | Validate movement through AC6, buffer position, then reply `[7,sub,x:u16,y:u16]`; absent coordinates and zero/zero only synchronize |
| AC18 with subcommand and optional gesture:u16 | Broadcast `[18,sub,character:u32,gesture:u16]` to the same scene including sender; absent gesture defaults to 1 |
| AC183:17 | Reply `[183,17,0]`, then `[183,11,9,2]` |
| AC186:9 with optional cutscene:u16 | Reply `[186,9,cutscene:u16,1,0:u32]`; absent cutscene defaults to 1 |
| AC13:238 | Reply `[13,42,character:u32]`, current SQL point/bonus balances, then both catalogs |
| AC21:1 | Current SQL point/bonus balances, then `[21,1,1,2,...,21]` window slots |
| AC21:2 with slot:byte | Purchase one bundle from the ordered nonbonus catalog; use atomic point debit and inventory delivery |
| AC21:3 | Current SQL point/bonus balances |
| AC22 with subcommand and optional entity:u16 | Reply `[22,sub,entity:u16,1]`; absent entity defaults to 0 |
| AC226:255 | Fixed reference `[238,183,0,255,27:u16,1,29:u16,2,24:u16,0]` matrix and `[225,252]` with ten zero bytes |
| AC44 with subcommand and optional title:u16 | Persist title, reply `[44,sub,title:u16]`, then map broadcast `[44,1,character:u32,title:u16]`; absent title clears it |
| AC66 with subcommand and optional job:byte | Persist separate reborn-job metadata, reply `[66,sub,job,1]`; absent job defaults to 1 |
| AC87 with subcommand | Restore HP/SP through transactional vital recalculation, send stat synchronization, then `[87,sub,1]` |
| AC23:53 / AC23:54 | Native start `[23,53,bagSlot,presentation]` validates the selected rod; two-byte auto-select remains supported. Stop `[23,54]` retains mall refresh when idle |
| AC90:1 / AC90:3 | Validated start/stop; reply with actual active state |
| AC90:2 | Attempt a due, transactional catch; early/replayed requests grant nothing |
| AC32:3 | Clear event callbacks and arcade interaction even at pose zero; broadcast a changed pose as before |

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

| Saved reference | What it establishes |
| --- | --- |
| `釣りスキル - Wonderland ONLINE Wiki_.html` | Older Japanese fishing observations, two-minute interval, catch-count table and full-bag experience behavior |
| `釣魚エリア_初心者ビーチ - Wonderland ONLINE Wiki_.html` | Beginner-beach catch observations grouped by simple/advanced/deluxe rod and skill level, including unlearned skill |
| `スキル - Wonderland ONLINE Wiki_.html` and `ＦＡＱ - Wonderland ONLINE Wiki_.html` | Skill improves high-grade catch chances; fishing can start without the skill by using a rod at the water's edge |
| `クエスト_地域別_サウスアイランド - Wonderland ONLINE Wiki_.html` | Fishing-bait quest consumes a captured pink worm; releasing it also grants a star, and both choices teach fishing |
| `初心者ガイド - 飄流幻境（Wonderland ONLINE）羅德島傳　日本語wiki Wiki_.html` | WLRI beginner guidance and fishing as a source of magnet material |
| `【情報】《飄流幻境Online羅德島傳說》釣魚活動開釣！（11_05維護後~11_18） @飄流幻境 Online 精華區 - 巴哈姆特.html` | November 2020 seasonal fishing drops, chest outcomes and event manufacturing recipes |
| `【心得】半天釣魚結果統整-羅德島釣魚活動 @飄流幻境 Online 哈啦板 - 巴哈姆特.html` | WLRI 2020 event observations: one catch per minute, samples with/without the skill, subsequent box observations |
| `《飄流幻境羅德島傳說》 官方網站.html` | September 2021 seasonal pearl/chest drops and event furniture recipes |

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

| ID | Exported name | Scope |
| --- | --- | --- |
| 37105 | Simple Fishing | Rod candidate, item type 28 |
| 37011 | Great Fishing | Rod candidate, item type 28 |
| 37082 | Luxury Fishing | Rod candidate, item type 28 |
| 15993 | Primary Fishing | Skill candidate |
| 15994 | Junior Fishing | Skill candidate |
| 30585 | Delicious Fish | Seasonal reward candidate; description says available at a particular time |

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

# Traditional PK reference

## Capture on 2026-11-06

The user exercised traditional player PK with two Alogin instances on Legacy
Rhodes3 (`20.187.123.156:6414`). They believe Test0002 challenged Test0001 and
confirm Test0001 lost, with a pet amity penalty. The trace does not contain
character names or the initial challenge request, so the challenger/name mapping
is user-reported rather than independently decoded.

Local capture artifacts (ignored, not repository dependencies):

- `var/pet-switch-capture/traditional-pk-reference-20261106-01.pcapng`
- Matching `-status.json`, `-helper.log` and `-decode.log`
- `traditional-pk-reference-20261106-01.traditional-pk.json`
- Decoder: `var/pet-switch-capture/decode_traditional_pk.go`

Capture started at `2026-11-07T01:21:57.7447552Z`. Packet Monitor produced 742
observations, reported ten packet drops and no ETL events lost, and restored the
initial empty filter set. TCP deduplication/reassembly recovered 420 framed
messages across local ports 50450 and 61942. Only one outgoing AC50 action per
client was recovered; missing challenge, setting or playback requests must not
be inferred to have been absent on the live connection. This trace cannot prove
whether a separate acceptance step occurred.

## Observed sequence

Times below are relative to the first decoded frame, not capture start.

| Time | Observed server messages | Meaning / limit |
| --- | --- | --- |
| 53.036 s | `[11,10,1]` to both clients | Native standard battle kind 1, distinct from World PK kind 13 |
| 53.036 s | AC11:250 formation plus AC11:5 participants | Two player/pet pairs; backdrop selector 14 |
| 55.058 s | `[52,1]`, AC51:1 vitals | Action selection begins |
| 56.579–60.221 s | AC53:5 readiness | Both pairs submit actions |
| 60.221 s | AC50:6 and AC50:1 action results | Ordinary combat transport, including a multi-target pet action |
| 69.308–69.311 s | `[53,3,2,2]`, `[11,12,1]`, AC11:1 departures | Battle completion and participant removal |
| 69.308 s | Owner/pet HP updates to 1 outside battle | Defeated owner and pet survive at one HP in this Legacy sample |

The winning connection is local port 50450, player ID 4617590, initially placed
at (4,2), with pet template 25020 at (3,2). The losing connection is local port
61942, player ID 4617671, at (1,2), with pet template 14156 at (2,2).
The defeat is consistent with the user's Test0001 observation. Formation records
are recipient-dependent: one initial AC11:250 contains several existing actors,
while the other receives the additional records through AC11:5. Do not require
identical grouping/order for both clients.

Background AC183 status/chest traffic also appears in this recording. It is
not evidence that traditional PK awards World PK score, activity or chests.
No equipment durability change was established by this sample.

## Native client cross-check and implementation scope

`var/decompiled/aLogin_ca19ee087b60_full.c` contains the native PK toolbar
callback `FUN_003a9020`: followers receive “Only team leader can PK.” Field
targeting checks local PK permission, teammate membership, stall state and map
restrictions before dispatching the AC11:2 path. The receive dispatcher maps
AC33:1 errors to notices such as “Target's PK is off.” These are decompile
observations, not missing packets recovered from this trace.

Gonline already handles the nine-byte player challenge layout
`[11,2,3,target_U32,click_U16]` in `battleStateCommand`, starts both parties
through `startPvP`, and uses the shared combat engine. This capture establishes
the standard entry/round/departure sequence; it does not independently establish
that request layout. Further request capture/native writer analysis is needed
before changing its compatibility contract or adding an invitation handshake.

## Gonline PvP protection policy

The user explicitly requires **all PvP modes** to preserve pet amity and player
and pet equipment durability. This deliberately differs from the reported Legacy
pet amity penalty. It applies to victory, defeat, withdrawal, pet switching and
disconnect checkpoints. Do not introduce a mode that bypasses the shared
`Battle.PvP` policy flag. World PK must use the same protection when implemented.

`activePetResults` already suppresses knockout amity/desertion in PvP. Capture-net
preparation now also explicitly rejects PvP before any durability transaction,
in addition to the engine's normal capture rejection. Ordinary combat currently
does not apply general equipment wear. Regression tests cover preserved amity,
equipment and resolved pet vitals across checkpoints, and the capture-net guard.
PvE pet knockout penalties remain unchanged. No additional HP restoration or EXP
policy is inferred from this protection requirement.

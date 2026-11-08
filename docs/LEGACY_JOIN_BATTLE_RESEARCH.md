# Legacy Join Battle capture

Recorded 2026-11-08 against the Legacy server at 20.187.123.156:6414.
The user identified Test0002 as the original combatant and Test0001 as the
joining character. Packet Monitor was stopped and its original empty filter
set restored afterward. Both clients may now disconnect safely.

Local evidence (ignored capture files):

- `var/pet-switch-capture/join-battle-20261108.pcapng`
- `var/pet-switch-capture/join-battle-20261108.teams.json`
- `var/pet-switch-capture/join-battle-20261108-summary.txt`

The capture contains 1,544 packet observations, 313 unique TCP segments and 892
decoded application frames. Times below are relative to the first decoded frame.

## Confirmed server sequence

| Time | Recipient | Observed behavior |
| --- | --- | --- |
| 47.449 s | Test0002, local TCP port 51280 | AC11:250 initial formation, AC11:10 initialization, AC11:5 pet/enemy records |
| 47.449 s | Test0001, local TCP port 51406 | AC11:4 field battle presence for Test0002 |
| 69.317 s | Test0002 | AC11:5 adds Test0001; AC11:4 publishes their field presence; another AC11:5 adds their pet |
| 69.318 s | Test0001 | AC11:250 complete existing formation, AC11:10 initialization and AC11:5 for their own pet |
| 71–83 s | Both | Selection notifications and subsequent AC50:6 / AC50:1 combat playback reach both clients |
| 88.892 s | Both | Enemy defeat and battle-finish/removal packets |

Test0002's character ID is 4617590, occupying cell (4,2), with pet template
25020 at (3,2). Test0001's ID is 4617671, added at (4,3), with pet template
14156 at (3,3). The enemy template is 17045 at (2,2). The joining character
and pet participate in the same battle; this is not spectator-only entry.

The joining client's full formation is 132 bytes: a four-byte prefix
`[11,250,90,44]`, followed by four blocks of **one mode byte plus a 31-byte
participant record**. The blocks describe Test0001, Test0002, Test0002's pet
and the enemy, with observed modes 5, 1, 1 and 100. Test0001's pet arrives in
the separate AC11:5 packet. Do not decode this as one global mode followed
by contiguous 31-byte records; that misaligns every record after the first.
The meanings of all mode values are not established by this capture alone.

## Request evidence and decompilation

The outgoing Join Battle request is absent from the captured TCP segments.
Its absence does not imply that joining is performed locally. The initial
battle-trigger request is also missing, although their server replies and later
combat requests were recorded. The request must therefore not be represented
as an independently captured golden fixture.

Native `FUN_0038e1d4` corroborates the Join Battle sender: selection mode 4
requires an already-battling target, checks proximity on both axes (less than
171 coordinate units), resolves the target's team leader when applicable,
clears the NPC click operand, assigns selector 5 and sends AC11:2.
The expected nine-byte request, using the existing AC11:2 serializer layout,
is selector 5, target character/leader U32 little-endian, and zero NPC click
U16. This layout is a decompilation inference, not captured request bytes.

## Gonline implementation implications

Gonline implements selector 5 entry separately from selector 3 (traditional PK):

- Validate the target's existing battle, scene, proximity, admission settings,
  available formation cells and the joining player's current interaction state.
- Add the player and active pet to the existing battle's membership and side;
  preserve the original combatants, enemies and calculated playback.
- Send the joining client the complete formation and initialization, while
  publishing incremental player/pet additions to existing clients.
- Ensure later selection, round playback, cleanup and resource persistence
  include the new member exactly once.

The capture proves successful entry and subsequent shared combat. It does not
establish rejection behavior, a full side, joining during every round phase,
leader redirection on the server, or admission-setting enforcement. These cases are covered by server regression tests; their precise rejection UI
on the Legacy server remains unverified.

## Spectator follow-up

A second Legacy-server capture on 2026-11-08 is retained locally as
`var/pet-switch-capture/spectate-battle-20261108.pcapng`, with decoded flow data
in the matching `.teams.json`. It contains 924 observations, 186 unique TCP
segments and 551 application frames. The recording was stopped and the original
empty packet-filter set restored.

The same TCP flows and character IDs identify Test0002 (51280, 4617590) as the
fighter and Test0001 (51406, 4617671) as the spectator. This trace contains one
spectator entry through battle completion; it does not establish an independent
leave/re-enter cycle despite that being included in the suggested test steps.

| Relative time | Recipient | Confirmed packet behavior |
| --- | --- | --- |
| 27.736 s | Test0002 | Initial formation and player/pet/enemy initialization |
| 38.645 s | Test0002 | AC11:4 announces Test0001 with field-presence status 4 |
| 38.645 s | Test0001 | AC11:250 complete formation, then AC11:10 initialization |
| 43.952 s | Both | Same AC50:6 and AC50:1 enemy action |
| 45.047 s | Both | AC52:1 selection-phase notification |
| 49.219 s | Both | Same pet attack and damage result |
| 57.673 s | Both | AC11:12 battle finish and AC11:0 field-presence cleanup |

The spectator formation has four mode-plus-record blocks: the spectator at
(255,255), mode 4; Test0002 at (4,2), mode 1; Test0002's pet at (3,2), mode 1;
and the enemy at (2,2), mode 100. Test0001's own pet is not added as a fighting
unit. No AC50 action request was captured from the spectator connection.
Receiving selection-phase notifications therefore must not, by itself, enable
combat controls or make the spectator a round participant.

The outgoing spectator request was also missed by Packet Monitor.
`FUN_0038dfe0` supplies the independent decompilation evidence: cursor selection
mode 3 requires a battling target, resolves its leader when applicable, sets
the NPC click operand to zero, assigns selector **4** and sends AC11:2. It
also marks native spectator state at +0x56eb. The expected request is the
existing AC11:2 nine-byte shape with selector 4, target/leader U32 and zero
NPC click U16. This remains inferred request layout, not captured bytes.

Gonline should keep spectator subscriptions separate from fighting members:
send the existing battle snapshot and subsequent native playback without adding
the spectator or their pet to targeting, action readiness, EXP participation or
resource settlement. Field presence status 4 must preserve the normal map body,
unlike a fighting participant's dust-cloud presentation. Battle completion must
release the subscription without removing any combatant. Explicit spectator
exit requests and rejection cases remain unverified by this trace.

## Implemented lifecycle and remaining live checks

The client toolbar now supports Join Battle and Watch Battle target selection,
with right-click or invalid-target cancellation. The server validates proximity,
scene, free owner/pet pairs, busy state and the target's Join setting. It adds
helpers to the target side without moving existing actors. A helper does not
replace the original battle's escape leadership. Party followers cannot initiate
entry; admission currently covers the requesting player and their active pet.
Trials are excluded from entry.

The joining capture places entry at 69.317 seconds, while the next AC52:1 is at
77.723 seconds; the joiner first submits a command at 79.533 seconds. Accordingly,
an entry during selection is visible but waits until the next command phase.
Entries requested during calculated playback queue until its boundary, avoiding
partial rounds and snapshots whose health already includes unplayed actions.
Existing pending commands are preserved. Each queued request reserves capacity
and admissions are processed in request order.

Spectators use separate transient subscriptions and an off-grid identity. They
receive public native playback or negotiated round chains, without private item
receipts, skill progress, EXP, rewards or settlement. Fighter disconnection,
escape, overkill and battle completion clean up observers without keeping a
battle alive. Native spectator Exit is AC11:1:3, established by the spectator
branch in the decompiled battle-button dispatch; it is not a fighting flee action.

Captured incoming formations remain independent raw-byte fixtures. Request
fixtures are explicitly decompilation-derived. Live mixed-client checks of Join,
Watch and independent spectator exit/re-entry are still recommended; the captures
do not prove every admission phase or PvP-side combination.

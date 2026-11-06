# Legacy team protocol and implementation

## Implementation status

Implemented on `Client-rewrite` after synchronizing with `main` at `e9ab71e`.
The baseline differences recorded below explain the changes; they are historical,
not a list of outstanding work.

- The existing Join Team toolbar selects a player or pet owner, redirecting an
  existing follower to their leader. ID-only invitations resolve the name from
  actor data; the older optional-name form remains accepted.
- Join-result, formation, snapshot, leadership and departure messages update
  local membership and a separate public formation model. Relationships survive
  actor replacement and can precede appearances. Pet NPC formation messages from
  compatible servers do not create player teams.
- Joining uses the native immediate snap to the leader's current feet. Appearance
  delays defer that snap rather than substituting a catch-up walk. Adding a
  follower preserves existing trail history and does not teleport other members.
  `FUN_004274cc` hides both leader and follower pets in the world, including their
  names and picking, while retaining selection and battle records. Departure
  resets pet presentation so they reappear beside their current owner.
- `FUN_004245bc` supplies the walking rule: five leader positions, shifted when
  either axis changes by at least 20 pixels. Followers one through three target
  entries one through three. They use the normal actor walker without sending
  individual walking requests. Relocations reset trails, combat pauses following,
  and an absent/off-map leader does not impose a local movement lock.
- Server followers retain the accepted group endpoint in buffered character state
  and checkpoint through the existing autosave path. Client trail positions are
  visual; they are not additional database writes or per-frame network commands.
- The serializer jump table at `002c49b8` and branch `002c4f3c` resolve transfer
  requests: offer `[13,10,1,target U32]`, accept `[13,10,2,3]`, refusal
  `[13,10,2,2]`, timeout `[13,10,3]`. Callback entries `0040760c`, `00407624`
  and `0040763c` set refusal/acceptance/timeout respectively. They were checked
  against executable SHA-256 `ca19ee087b601182e872235e58e6bf367f4bb21879226dfe3cc96f1c99624f61`.
- The native five-second leadership prompt is implemented on both ends, with
  refusal, expiry, busy, map-change, battle-entry and departure cancellation.
  Accepted transfers swap the new and old leader's formation slots. Pending
  transfers are indexed separately so expiry does not scan all online players.
- Battle cells use the current party's formation order even when an enemy
  contacts a follower first. The battle initiator still owns its quest callback
  and existing loot flow. Party cleanup preserves selected battle/riding pets.
- Serializer branch `002c4b69` emits Leave Team as `[13,4,current leader U32]`.
  The client now uses that layout. The server validates that leader against the
  authenticated party and also retains the older two-byte request as an explicit
  compatibility fallback. A departure naming the leader clears the client's
  whole corresponding formation.

Independent captured-byte client tests and server integration/race tests cover
these transitions, public observer snapshots, map replacement, persistence,
malformed packets and concurrent offer/reply/expiry. The full two-client live
sequence has not yet been repeated against the updated programs.

## Reference and scope

Captured two Alogin instances connected to the Legacy server on 2026-11-06.
The user joined a team, walked, completed a battle, changed maps, changed
leader, completed a second battle and disbanded. Test0001 led first.
Closing the clients afterward does not invalidate the saved trace.

- Capture: `var/pet-switch-capture/team-reference-20261106-01.pcapng`
- SHA-256: `38f85eb4464d8289549aeae7dc7bf47e375211046dcfd395b3a107ac1d411f79`
- Two client TCP ports: 59124 and 54895; 2,674 packet observations,
  490 unique TCP payload segments and 2,060 reconstructed application frames.
- Test0001: character 4617671, port 59124. The second character is 4617590,
  port 54895; its name was not supplied and is not inferred.
- Decode artifacts: `.teams.json`, `decode_team.py`, `summarize_team.py`
  alongside the capture. These retain per-client stream identity.

Times below are seconds since the first decoded application frame, not since
recording started. The server was very laggy during the test. Outbound payloads
are sparse: the trace establishes the server-side exchanges but does not contain
every initiation/acceptance request. Do not invent missing request bytes or
attribute an absent request solely to server latency. Packet Monitor reported
no lost logging events; its separate packet-drop counter was 31. Neither fact
establishes that every application request was captured.

This document records observations, the baseline implementation gaps and the
resulting implementation. Existing team coverage is described in
[TEAMS.md](TEAMS.md).

## Join and roster establishment

| Time | Recipient/direction | Decoded payload |
| --- | --- | --- |
| 29.595 | Second character → server | `AC13:1, target=4617671` |
| 30.538 | Server → Test0001 | `AC13:1, requester=4617590` |
| 34.759 | Server → second character | `AC13:3, result=1, leader=4617671` |
| 34.759–34.764 | Server → both | `AC13:5, leader=4617671, follower=4617590` |
| Immediately afterward | Server → each | Other member's `AC8:3` vitals |
| 85.684–85.685 | Server → both, during map reinitialization | `AC13:6, leader=4617671, follower_count=1, follower=4617590` |

The invitation received by Test0001 is exactly six bytes: action, subcode,
requester U32. It contains **no name-length byte or name**. Resolve the displayed
name from the known player appearance, with a documented fallback if unavailable.
The join-result message is seven bytes, including its result byte and leader U32.
The public formation addition is ten bytes, containing two U32 character IDs.

There is no immediate `AC13:6` roster in this captured join exchange. Native
`FUN_00406e0c` constructs the leader/follower relationship from `AC13:5`.
A client cannot wait for a later full roster before recognizing its own team.
`AC13:6` remains a full roster snapshot and must also support unrelated teams
visible on the same map.

Baseline differences:

- `client/wlo/app/team.go` requires a name-bearing invitation of at least seven
  bytes. It rejects the captured native six-byte form.
- That handler ignores `AC13:5` and has no `AC13:3` join-result handler.
  `team.State.InParty` depends on a roster with multiple members.
- `internal/server/teams.go` sends a name-bearing invitation and immediately
  sends a roster. This happens to serve the custom client's assumptions but
  does not make the custom client compatible with the captured Legacy exchange.
- The server already uses pending invitations and expiry checks. Retain that
  validation while correcting the native wire behavior.

## Following and map changes

From 38.367 through 92.555, captured team-player movement received by the second
character identifies **Test0001**, not the follower. Example:

`AC6:1, character=4617671, direction=7, destination=(1122,3975)`

After the confirmed leadership change at 105.437, team-player movement received
by Test0001 identifies **4617590** instead. This is evidence for following a
leader locally from its movement and formation links. It is not evidence for
issuing an independent network movement command for every following step.
Unrelated NPC movement is excluded from this comparison.

Native `FUN_00406e0c` maintains each actor's leader link and the leader's follower
list, including the local player and remote actors. `FUN_00407650` reassigns
those links after leadership transfer. The precise formation spacing and trail
sampling still need tracing through the native movement code; do not reuse pet
spacing merely because both behaviors involve following.

The trace contains map initialization and readiness traffic for both clients,
with full `AC13:6` snapshots restoring the existing party. A map transition is
not a leave/rejoin operation. Preserve membership while replacing map actors;
reconcile formation links when the corresponding appearances become available.

Baseline differences and implementation requirements:

- `client/wlo/world` has player movement and pet-following support but no public
  team formation model. `teamPacket` explicitly ignores formation presentation.
- Add public leader/follower links independently of the local Team window state.
  Use them to drive local and remote followers from the leader's movement.
- Suppress independent walking requests for a following local player while
  preserving battle and UI actions. Validate the same restriction on the server.
- The server has party map-follow support (`partyFollow`/`partyArrival`), but
  ordinary movement does not update followers' positions. Keep authoritative
  follower positions coherent for interactions, encounters and saved locations,
  without broadcasting redundant individual walk commands.
- Clear movement trails on relocation, map change, detachment and leader change.
  Reconcile out-of-order snapshots/appearances instead of dropping relationships.

## Leadership transfer is an exchange

| Time | Recipient | Decoded payload |
| --- | --- | --- |
| 103.868 | Proposed leader, character 4617590 | `AC13:15, mode=1, old_leader=4617671` |
| 105.437–105.443 | Both characters | `AC13:15, mode=3, new_leader=4617590, old_leader=4617671` |
| 125.620–125.625 | Both, later roster refresh | `AC13:6, leader=4617590, follower_count=1, follower=4617671` |

The offer is seven bytes; successful confirmation is eleven bytes. The immediate
leadership update must come from the success notification rather than waiting
twenty seconds for this later roster refresh.

Decompilation evidence from `var/decompiled/aLogin_ca19ee087b60_full.c`:

- Receive dispatcher `AC13:15`, mode 1 → `FUN_00407304`: validates the existing
  relationship and opens **"Want to be a team leader?"** with a 5,000 ms timeout.
- Mode 2 → `FUN_00407468`: refusal/status handling. Its status 2 displays a
  refusal, status 3 a timeout, status 4 clears pending state, and status 5 reports
  that the player is busy. These branches are decompiled, not exercised here.
- Mode 3 → `FUN_00407650`: reads new-leader and old-leader U32 fields and
  reassigns actor formation links.
- The invalid-offer path sends `AC13:10` with a response byte 2. The complete
  successful outgoing offer/acceptance layout was not recorded in this trace;
  the missing sending callbacks are resolved in the implementation status above.

The baseline `partyTransfer` immediately reordered members and sent a roster. The
client sends `AC13:10` plus target U32 and has no `AC13:15` handling. Implement a
pending transfer with membership/leadership checks, acceptance, refusal, timeout
and disconnect cancellation. Only accepted confirmation changes leadership.
Keep this transient request state in memory under the existing party lock.
Do not guess client-to-server response layouts from server-to-client packets.

## Battles and formation

Both clients received native `AC11:250` initial formations and `AC11:10` battle
kind messages. The first battle began around 57.013 and ended around 66.811;
the second began around 164.109 and ended around 176.995.

The formation header is four bytes, followed by **32-byte fighter records**.
The byte at offset four is the first fighter's side/entry mode, not a separate
header flag. Preserve independent wire fixtures when translating these records.

| Battle | Leader player cell | Leader pet cell | Follower player cell |
| --- | --- | --- | --- |
| First, Test0001 leads | `(4,2)` | `(3,2)`, pet 14156 | `(4,3)` |
| Second, 4617590 leads | `(4,2)` | `(3,2)`, pet 25020 | `(4,3)` |

The leader's initial packet contains its own fighter. The follower's initial
packet contains its own fighter with entry mode 5, plus the leader and leader's
pet. Further `AC11:5` messages complete the participants. Keep the existing
waiting-participant handling and command-unlock semantics; do not enable the
follower solely because it has received an initial formation.

Gonline already gathers same-scene eligible party members in `battleTeam`, and
its client parses waiting fighters. Retain the combat engine and compare the
captured entry ordering, cell assignments and unlock timing with integration
tests. Team membership, battlefield participation and temporary waiting state
are distinct concepts.

## Disband and cleanup

At 191.449–191.453 both clients receive:

`AC13:4, character=4617590`

That character was the **current leader**. A client must detach the whole public
formation when the departing character is its leader; checking only whether
the supplied ID equals self leaves the follower in stale party state.
Afterward, both characters have independent player movement in the trace.
A later `AC13:4, character=4617671` appears on the second character's connection;
do not require that later message to perform the initial disband cleanup.

The baseline `teamPacket` reset local state only when the departure ID was self.
The server has additional self-roster/leave cleanup for native compatibility;
retain the independent tests for those packets while making the client tolerate
the captured leader-only disband notification.

## Validation sequence

1. Pin captured invitation, join confirmation, formation addition, roster,
   leadership notification and leader-disband payloads as authored raw-byte
   fixtures. Include malformed/truncated input and unrelated public teams.
2. Implement public formation state and local membership transitions. Verify
   join without an immediate full roster, delayed appearance creation, duplicate
   messages, leader disband and reconnection/map replacement.
3. Trace native walking/trail behavior and leadership sending callbacks, then
   implement following and the complete transfer handshake on both ends.
4. Exercise two simultaneous sessions: leader movement, follower input rejection,
   authoritative follower coordinates, map readiness, two battle formations,
   transfer acceptance/refusal/timeout, disband and disconnect cleanup.
5. Run focused client tests and server race tests, then verify Legacy/Gonline
   combinations live. The native sending callbacks resolved the outgoing layouts
   missing from this capture; live testing remains useful for presentation and
   timing acceptance.

All implementation belongs on `Client-rewrite`, synchronized with local `main`
at `e9ab71e` before this investigation. No new Gonline-only team protocol is
needed for the behavior observed here.

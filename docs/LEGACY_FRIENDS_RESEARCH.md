# Legacy friendship and text-mail capture

Recorded on 2026-11-08 against the Legacy server, following the user's test
between Test0001 and Test0002. Local evidence is retained under
`var/pet-switch-capture/friends-20261108-01`: `.pcapng`, `.friends.json` and
the helper status/log. These ignored files are not distributable dependencies.
The capture stopped successfully and restored the original empty filter set.

The trace contains 340 packet observations, 68 deduplicated TCP segments and
82 application frames. The established identities from the preceding captures
are Test0001 (character 4617671, client port 51406) and Test0002 (4617590,
client port 51280). Times below are relative to the first captured frame.

| Time (seconds) | Direction | Observed behavior |
| --- | --- | --- |
| 25.188 | Server → Test0001 | AC14:2 carries Test0002's U32 ID: incoming friend request. |
| 29.767 | Test0001 → Server | AC14:3 carries Test0002's U32 ID and result 1: accept. |
| 31.211 | Server → Test0002 | AC14:3 carries Test0001's ID and result 1, followed by AC14:9 with the same ID and result 0. |
| 31.215 | Server → Test0001 | Matching AC14:3 and AC14:9 for Test0002. |
| 49.290 | Server → Test0002 | AC14:1 delivers text `Test!` from Test0001. |
| 60.699 | Server → Test0001 | AC14:1 delivers text `Test 2!` from Test0002. |
| 92.388 | Server → both | AC14:4 carries the other character's ID: reciprocal friend removal. |

Incoming text mail has the shape AC14:1, sender U32 little-endian, date F64
little-endian, then the remaining bytes as message text. Both captured messages
have this shape; the first has 19 total bytes and the second 21.

The raw acceptance request is `[14,3,118,117,70,0,1]`. Incoming request bytes
are `[14,2,118,117,70,0]`. The acceptance notifications are
`[14,3,199,117,70,0,1]` and `[14,3,118,117,70,0,1]`; their subsequent AC14:9
notifications end in 0 instead of 1. Decompilation of `FUN_00280034` establishes AC14:9 as repeated
U32-ID/status-byte presence records. The final 0 means online, not mail-send
success. Status 3 is offline.

The outgoing initial add request, text-mail sends and removal request are absent
from the recorded segments. Their incoming effects are established, but their
request layouts and who initiated removal cannot be claimed from this trace.
No refusal, ignored-request timeout, offline delivery or reconnect persistence
was demonstrated. The implementation now handles the captured exchange, reciprocal removal,
batched AC14:9 presence, AC14:8 profile refresh and AC14:11 incremental profile
addition. Profiles and presence survive map departures. The server publishes
proper AC14:9 presence and AC14:11 profiles instead of repurposing AC14:7
(native guild-name update) or sending truncated AC14:8 records as logout.
Mail remains durable and may be delivered after login; this pre-existing server
extension is covered by tests, not proven by the capture. Captured incoming
bytes and the acceptance request are independent client regression fixtures.

# Connection-wide Gonline mode

Gonline determines its application protocol immediately after TCP connects,
during native discovery. TCP framing and the discovery request remain unchanged.
The server replies with the ordinary description and channel list, then one
versioned application hello. No gameplay feature starts its own negotiation on
a current Gonline Server + Client connection.

## Wire format

All fields use normal Wonderland framing and little-endian integers:

| Field | Bytes |
| --- | --- |
| Command | 1 (AC1) |
| Subcommand | 248 offer, 249 acceptance, 250 confirmation |
| Signature | Four ASCII bytes `GLON` |
| Version | UInt16, currently 1 |
| Feature mask | UInt32 |

The payload is exactly 12 bytes. Bits are:

| Bit | Feature |
| --- | --- |
| 0 | Collection retries at server respawn expiry plus 10 seconds |
| 1 | Full combat round chains and batched choices |
| 2 | Client-side native backdrop resolution |
| 3 | AC5:4 completes the entire initial map snapshot |
| 4 | Shared ground pickups include the collector for observer animations |
| 5 | Per-session AFK mode, vital snapshots and ordered peer refresh |
| 6 | Explicitly authorized, workspace-local shared AFK party snapshots (requires bit 5) |
| 7 | Changed-field AFK updates (requires bit 5) |

The server includes bit 1 only when its captured startup `combat_round_chain`
setting is enabled. The client intersects the offer with its supported features;
bit 2 requires loaded local backdrop metadata. Unknown offered bits are ignored.
The server rejects unoffered bits and unsupported versions. Features activate only
after the server's confirmation exactly matches the client's acceptance.

The server captures its offer once per connection. Repeated discovery sends no
new hello. Identical repeated acceptance is harmless; a different acceptance or
late feature negotiation cannot change the established mode. Negotiation state,
playback and pending backdrop context clear on disconnect/reconnect. No account
or gameplay database fields store this transient transport state.

## Compatibility and interaction traffic

Legacy clients ignore the unknown AC1 subcommands and remain on the native path.
The client does not probe unidentified servers or infer their software from the
server name. Legacy servers continue controlling their own native behavior.
Clients also understand the older separate GLBR/GLCT/GLBD feature advertisements
for older Gonline servers. Current servers advertise only GLON; once GLON starts,
legacy feature messages cannot activate extra features.

Interaction messages still carry their necessary data. For example, AC1:247
GLBD map/X/Y specifies the shared battle-entry position used for local backdrop
selection. It requires no response and does not renegotiate the connection.
Round prompts, choices and result chains likewise use the mode already selected.

With bit 4 selected, shared pickups send observers an extended AC23:2:
`command:u8, subcommand:u8, ground_slot:u16, collector_id:u32, x:u16, y:u16`
(12 bytes). X/Y are the collector's world position at pickup. Observers remove
the ground node and fly its icon along a parabola toward that position, without
granting an item or displaying an acquisition notice. The collector retains the
native pickup/grant sequence. Permanent account-owned tutorial nodes never send
this animation to another account. Hidden collectors receive no visual disclosure.

Unnegotiated observers receive the four-byte native AC23:2 removal. Decompilation
`FUN_003dc72c` checks packet length, not the extra byte's value: adding even a zero
byte requests `FUN_003d74d4` to animate toward the receiving client's own player
through `FUN_003e0548`. No remote collector field is available in that native
message. Full observer flight therefore currently requires Gonline Client.

With bit 3 selected, the server emits the existing AC5:4 map-ready reply after
the complete destination snapshot, including peer positions, vehicle state,
props, bazaars and tent furniture, before starting an arrival script. Without
bit 3, its native packet ordering remains unchanged. No new per-map handshake
or polling request is added. The client's loading screen also waits for local
render preparation and pending sprite decoding; it does not block networking.

New capabilities should be added as named bits in `internal/protocol`, selected
through this common hello. Do not add a new per-feature offer/accept exchange.
Independent wire goldens, configuration-gate tests, feature-subset tests,
reconnection tests and repeated/forged-message tests cover this protocol.

## AFK mode

With bit 5 confirmed, AC253:1 carries a single mode byte (0 normal, 1 AFK).
The server rejects malformed or unnegotiated requests. Entering AFK requires a
loaded character without an active dialogue/service/trade/minigame; a rejected
entry receives AC253:5. Exiting remains available during interactions and combat.
Entry acknowledgment is AC253:1 with mode 1, followed by an initial snapshot.
No startup setting, headless service or web endpoint is required.

AC253:2 is a bounded, server-pushed snapshot. Its fields are map/X/Y U16,
unit count U8, then up to 20 units. Each unit contains owner U32, identity U32,
kind U8 (0 player, 1 pet), level U8, byte-length-prefixed UTF-8 name (at most
80 bytes), HP/max HP/SP/max SP/level EXP progress U32, next-level EXP U64 and
amity U8. The recipient receives its own player/pets and its authenticated party's
players/active pets, including authoritative calculated combat vitals. Snapshots
are checked at most once per second and sent only when changed; clients never
poll. State is transient and adds no database schema or persistence fields.

Only ordinary chat and unrelated players' appearance, position, movement, pose
and equipment/sprite refreshes are suppressed. Self/party presentation, NPCs,
unknown packets, system/GM notices, acknowledgments, combat, events, inventory,
rewards and map transitions remain vital. The same classifier filters queued
client packets. Classification precedes output-queue admission; arbitrary queued
frames are never discarded, and durable delivery receipts retain their rules.

On exit, the server disables suppression, sends AC253:3, refreshes current visible
peers and their pets/vehicles/poses/team presentation, then sends AC253:4. This
does not repeat map-entry scripts, rewards, team arrivals or interaction release.
The client clears stale peers at the begin fence and applies this contiguous
refresh even during combat playback. It keeps the AFK screen until the end fence,
map readiness and a covered render preparation frame with settled sprite jobs.
Normal gameplay streams continue throughout. A missing transition reply times
out after ten seconds and disconnects instead of revealing a partial snapshot.

With bit 6 also confirmed, entering AFK may append a nonzero 16-byte workspace
nonce to AC253:1 (19 bytes total). The nonce is random, process-local and never
persisted; each connection still authenticates independently. An ordinary
three-byte request retains independent snapshots. No grouping crosses parties.

For two or more consenting AFK connections in the same party/workspace, the
server sends AC253:6 on each subscriber socket: stream U64, carrier character
U32, count U8 and sorted subscriber character IDs U32 (2–4). A zero stream,
carrier and count clears the binding. The lowest subscriber ID carries common
updates. AC253:7 contains stream U64, monotonic sequence U64, followed by a full
AC253:2 packet with zero map/coordinates and only party players/active pets.
It is sent once per group, change-only and at most once per second. Ordinary
AC253:2 packets retain each owner's map/coordinates and resting pets on that
owner's socket. Gameplay commands, inventory and durable receipts stay separate.

The workspace shares common updates only between individually bound sessions
connected to the same actual endpoint with identical stream/carrier/membership.
It rejects old sequences/streams and retains one latest payload to handle binding
arrival across sockets. Membership or carrier changes establish a fresh random
stream; fewer than two subscribers restore full independent snapshots. Normal
sessions, Legacy clients/servers and older Gonline peers retain their existing
behavior. Stream state is transient, bounded by live sessions, with no global
nonce registry, polling requests, web endpoint or database changes.

With bit 7 confirmed, subsequent snapshots may use AC253:8 when smaller than a
full snapshot. The initial update, any new binding and roster reordering retain
full snapshots. The delta contains flags U8 (bit 0 means map/X/Y U16 follow),
change count U8 (at most 40), then owner U32, unit ID U32, kind U8 and field mask
U16 per change. Mask bits 0–8 encode name (U8 byte length plus UTF-8), level U8,
HP U32, maximum HP U32, SP U32, maximum SP U32, EXP progress U32, next-level EXP
U64 and amity U8, in that order. Bit 9 alone removes a unit. New units require
all nine fields; malformed, duplicate or unknown-unit partial updates fail
atomically. The resulting roster remains limited to 20 units.

AC253:7 may wrap a delta instead of a full AC253:2 only when every subscriber
negotiated bit 7. The carrier reconstructs complete state before sharing locally,
so a subscriber whose binding arrives later never depends on a missed delta.
Each private stream has a separate base. TCP ordering and existing stream IDs
and sequence checks are preserved. Updates are never dropped or coalesced across
acknowledgments, inventory receipts or battle progression packets.

Native AC8:3 teammate-stat updates also omit unchanged fields for all clients.
These are already independent absolute-value packets: full eight-field snapshots
remain on joins/refreshes, while an HP-only change sends one packet rather than
eight. This optimization does not require a Gonline-specific capability.
Battle entry refreshes only the entering recipient's party snapshot. Refreshing
the entire party once per entrant previously multiplied identical roster/stat
snapshots by party size; each client still receives its complete native snapshot.

# Negotiated combat round transport

Gonline retains the native combat protocol in both directions. A Gonline client
connected to a legacy server sends native AC50 requests and interprets the timed
AC50/51/53 stream. A legacy client connected to Gonline receives that same stream.
When both peers negotiate round transport, the client submits its character and
active pet choices together and the server sends the entire computed action
chain before playback begins. The server must first enable `"combat_round_chain":
true` in its startup configuration. The default is `false`, including when the
field is omitted; restart to apply changes. No database migration is needed.

This changes transport and presentation scheduling, not combat rules. The server
still owns damage, hit/miss, targets, combos, effects, overkills, rewards and
turn readiness. A mixed battle can use both transports simultaneously.

## Negotiation and fallback

Immediately after TCP connection establishment, the normal discovery response
contains the native description and channel list followed by one connection-wide
application hello. All current Gonline-specific behavior uses this same feature
selection for the lifetime of the connection. No feature negotiates again at
interaction or battle entry. See [the connection protocol](CONNECTION_PROTOCOL.md).

| Field | Encoding |
| --- | --- |
| Command / subcommand | U8 `1`, U8 `248` offer / `249` acceptance / `250` confirmation |
| Signature | Four ASCII bytes `GLON` |
| Version | U16 `1` |
| Features | U32: bit 0 collection timing, bit 1 round chains, bit 2 local backdrops, bit 3 complete map snapshot readiness |

The server advertises round chains only when its captured startup
`combat_round_chain` setting is enabled. The client accepts the supported subset
and waits for exact confirmation. A client without backdrop metadata excludes
that feature. Repeated discovery does not issue another offer; established modes
cannot be changed by late feature handshakes. Legacy clients ignore the extension,
and Legacy servers continue using native packets. Connection state resets on
reconnect/disconnect. Older Gonline servers' GLBR/GLCT/GLBD messages remain
understood by the client for compatibility, but current servers do not emit these
separate feature offers.

## Disconnect settlement

A disconnect withdraws the player and their pet immediately from server command
and target eligibility. Pending choices are discarded; neither fighter remains
to defend or take damage in later rounds. Other connected teammates continue,
including after the leader logs out. This is not a successful escape.

Both transports calculate an entire round before sending its animations. A
disconnect therefore saves the full calculated player HP/SP and active-pet
HP/SP/knockout/amity consequences, including effects whose animations have not
played yet. It does not roll back committed item consumption or skill progress,
and does not award uncommitted victory rewards or pending captures. An already
committed ending/escape/overkill return is not applied a second time.

The server commits these consequences before the final logout checkpoint. Failed
writes retain their snapshot and checkpoint baseline for retry; pet roster changes
are published only after SQL succeeds. If the database remains unavailable,
the failed final save is logged rather than reported as successful.

Native player/pet removal records follow the current committed round's playback
and precede the next prompt in both transports. This avoids removing an actor
that already appears in that round's immutable animations. Between rounds removal
is announced immediately. When the last participant leaves, the timer stops,
playback terminates and the encounter reservation is released. If only knocked-out
teammates remain, the battle settles as a defeat rather than waiting for commands
they cannot submit. This disconnect policy is a Gonline server rule; packet
layouts remain compatible with legacy clients.

## Choices and replay protection

For a negotiated recipient, AC254:3 replaces the native turn prompt/readiness
pair. Its payload is command U8, subcommand U8, choice token U32, acting X U8,
acting Y U8. The token increases across the connection, including new battles.

The client collects choices locally until all its living, present fighters have
choices. Manual targeting, skills/hotbar, radial Auto and Remote use the same
submission path. AC254:2 contains command U8, subcommand U8, token U32, count U8,
then that many native AC50 requests. Attack/defend/flee use eight bytes
(command, subcommand, actor X/Y, target X/Y, skill U16). Item requests use twelve:
command/subcommand `50,2`, actor X/Y, target X/Y, item U16, two opaque random
bytes, bag slot U8, compatibility flag U8. The subcommand determines each
request's length, allowing mixed item/skill choices without changing existing
eight-byte request goldens. There are at most two: character and active battle pet.
If the countdown expires after one choice, the client sends that selected action
as an ordinary native AC50 request. The server timeout leaves the missing fighter
idle, without defense. With no choices, the client sends nothing and the server
idles both fighters. Complete batches and their negotiation remain unchanged.

Item selection reserves a command and available stack count in memory; it
does not consume inventory. At execution, the server rechecks the actor, target
and selected item identity. Consumption and a player/pet target's HP/SP commit
in one SQL transaction, including when the target belongs to a teammate. A failed
save or stale bag publishes no item effect or removal receipt. The successful
owner-only AC23:9 receipt travels in the corresponding native step or round chain.

Ordinary native AC50:2 also accepts the observed 52-byte legacy request: the same
12-byte fields followed by a 40-byte opaque compatibility trailer. This fixes
legacy-client choices being silently ignored by the shorter length check. The
versioned batch above retains its existing 12-byte entries. Captured legacy item
recovery results use digit mode 0, which the client now applies at contact.
See [live item capture findings](BATTLE_ITEM_CAPTURE.md).

A Team pet change uses native AC19 during the player's command phase. It consumes
the player's command and replaces/rests the pet without an attack or defense by
either actor. Any queued outgoing-pet choice is discarded. The server saves the
outgoing pet's vitals and selected mode before acknowledging the request; its
native exit and replacement participant records precede the round's actions in
both transports. Native order is owner AC53:5, four-byte old-pet AC11:1,
owner-only AC19 selection receipt, and incoming AC11:5 with entry mode 5 plus its
owned roster slot. Mode 5 excludes the pet from acting and targeting, including
area coverage, until AC52:1, rather than marking its allegiance. Gonline's client shows it walking in while a detached visual
copy of the old pet walks out; command eligibility remains authoritative and
readiness does not replay entry. The server omits the previous
immediate replacement stat/submission packets; HP/SP already travel in AC11:5.
The first action follows a 350 ms compatibility floor (342 ms observed in the
reference capture). Owner receipts retain their private scope inside each
recipient's chain and in native fallback. A fresh turn prompt is required before
another command.

The server validates the entire batch through the native submission rules before
publishing acknowledgments or trying to execute the round. A duplicate or foreign
actor invalidates the batch without partially reserving turns. Stale or consumed
tokens have no effect. Native AC53:5 acknowledgments remain unchanged. A single
actor still needs one request; character plus pet now need one instead of two.

## Complete result chain

The engine computes its normal ordered `battle.Step` chain once. Negotiated
recipients receive a per-owner envelope containing these original native result
packets. Actors in a combo stay together in one step; multi-target effects retain
their native action records and all HP/SP results. The body is:

```text
step count: U16
repeat for each step:
    minimum interval after this step, milliseconds: U32
    packet count: U16
    repeat for each packet:
        native payload length: U16
        native payload bytes
```

AC254:1 transports the body: command U8, subcommand U8, sequence U32, total body
bytes U32, body offset U32, then chunk bytes. Usually there is one frame. Larger
rounds are split only to respect the native 65,535-byte payload limit; these are
not per-action exchanges. The sequence shares the connection's increasing
counter with choice tokens. No playback acknowledgments or per-action requests
are introduced.

Limits are 1 MiB per body, 512 presentation steps, 4,096 embedded packets, one
minute per step interval and 30 minutes of intervals per round. Chunks must be
contiguous and agree on sequence/size. Nested extension packets are forbidden.
The client assembles and validates the entire body, including known native battle
record layouts, before applying any step. Malformed chains close the connection;
old completed sequences cannot replay damage. If a computed round exceeds these
extension limits, the server can send it through the existing native stream.

## Playback and server ownership

The client passes each scheduled step to its normal native packet interpreter.
The next step waits for both the supplied minimum interval and local animations,
damage cues, reactions and removals to finish. Damage numbers and displayed HP/SP
still change at contact. Combos animate simultaneously. Reinforcements enter in
sequence, after the previous fighter's removal. Background sessions advance the
same playback without drawing. Incoming next-round prompts, settlement and map
packets are bounded and deferred until local playback finishes; chat and explicit
disconnect reasons remain immediate.

The server retains its authoritative timeline for legacy peers, turn locking,
overkill returns and settlement. It does not wait for client animation receipts
or allow a client to accelerate rewards/turns. For Gonline recipients it skips
duplicate per-step network sends. Executed skill proficiency is committed once
per owner before publishing its chain; its receipts appear at the first step.
A native fallback reuses those receipts without applying the gains twice.
Rewards and map transitions retain their existing commit-before-publication
boundaries and arrive separately when due. No durable resource grant is inferred
by the client from the envelope.

An overkilled owner's envelope ends after its flight, excluding later actions.
Other owners' envelopes schedule that member and its pet leaving, then continue.
The actual forced spawn return still commits on the server, including on
disconnect, before the owner receives map packets. Ordinary knockouts retain
their native fallen/revival behavior.

An explicit escape attempt is resolved for its actor against the strongest living
opponent: equal/higher level always succeeds; below that level, the chance is
`75 - floor(enemy level / 6)` percent, matching the reference `CanFlee` rule.
Either the owner or its pet succeeding withdraws both and cancels their other
commands. A nonleader's envelope ends after their joint departure; remaining
recipients receive the participant close and continue the round. The server
commits that member's vitals and pet results before sending departure packets or
an envelope. An actual team leader's successful escape ends the battle for the
remaining participants. Leader overkill or logout alone never triggers this
escape outcome. Ordinary knockouts remain combat state.

Failure emits native AC50:1 motion **60042**, a self animation with no stat
results: run, trip, recover. It grants no defense or proficiency and consumes no
SP. Both native delivery and round chains use the same packet. Gonline retains
Flee (60041) as the repeat command rather than submitting the failure motion ID.

Implementation: `internal/protocol/combat_round.go`,
`internal/server/combat_round.go`, and `client/wlo/app/combat_round.go`.
Use the focused wire, compatibility, persistence, race and rendering recipes
in [TESTING.md](TESTING.md#subsystem-checks); recent suite results are recorded
separately on that page.

## Client-side backdrop resolution

Backdrop resolution is bit 2 of the connection-wide GLON hello and remains
available with round-chain mode disabled. Negotiated clients receive the battle entry
map/coordinates and choose scenery locally; Legacy clients retain the native
server-selected backdrop. See [backdrop protocol](BATTLE_BACKDROPS.md#gonline-client-resolution-protocol).

## Knockout recovery and equipment wear

At ordinary defeat settlement, players return with 1 HP and retain their resolved
SP. A knocked-out active pet returns with 1 HP when its owner's battle ends;
previously switched-out knockouts also revive at the end, not during the switch.
The same one-HP rule applies to an overkilled player's spawn-point return.
Knockout amity penalties and the existing PvP amity/durability exemptions remain.

Successful HP-damaging monster contacts can wear equipped player and pet gear.
There is no roll against an enemy below the wearer's level. Each eligible item
has a 1% chance to lose one point at equal level, increasing linearly to 20% at
an enemy level advantage of 20, capped thereafter. Multi-contact attacks roll
per contact, capped by their positive damage amount. Misses, zero damage,
periodic status damage and allied damage do not cause combat wear. Damage is
capped at 100; indestructible and unknown item definitions are excluded. This
curve is a Gonline policy approximation, not a proven Legacy rate.

Wear is calculated after the authoritative round and committed once per owner
(player plus active pet) before playback, with SQL ownership/equipment checks.
Private equipment receipts go only to the owner. Disconnects or repeated playback
do not apply wear a second time. SQL item definitions own indestructibility;
missing databases bootstrap from the updated repository catalog without client
extraction. The migration/default list covers creation outfits and rebirth capes;
additional exemptions can be edited through the typed item column.

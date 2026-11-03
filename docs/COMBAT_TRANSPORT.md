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

With `combat_round_chain` enabled, after the normal discovery description and channel list, Gonline sends
AC1:240. Native WLRI's AC1 dispatcher (`FUN_002dde1c`, case 1) has no branch
for subcommands 240–242 and ignores the offer. The client never probes a legacy
server or identifies capabilities by the server's display name.

All three capability messages use this exact payload, including AC/subcommand:

| Field | Encoding |
| --- | --- |
| Command / subcommand | U8 `1`, U8 `240` offer / `241` acceptance / `242` confirmation |
| Signature | Four ASCII bytes `GLBR` |
| Version | U16 `1` |
| Capabilities | U32 bit mask; bit 0 enables round chains and batched choices |

All integers are little endian; ordinary Wonderland TCP framing still applies.
The client accepts a supported offer and waits for confirmation before enabling
the extension. The server requires its own prior offer and an explicit valid
acceptance, and rejects renegotiation during battle. A short generic AC1 echo
cannot negotiate anything. Capability and playback state belong to one
connection and are cleared on reconnect/disconnect.

## Choices and replay protection

For a negotiated recipient, AC254:3 replaces the native turn prompt/readiness
pair. Its payload is command U8, subcommand U8, choice token U32, acting X U8,
acting Y U8. The token increases across the connection, including new battles.

The client collects choices locally until all its living, present fighters have
choices. Manual targeting, skills/hotbar, radial Auto and Remote use the same
submission path. AC254:2 contains command U8, subcommand U8, token U32, count U8,
then that many original eight-byte AC50 requests (command, subcommand, actor X/Y,
target X/Y, skill U16). There are at most two: character and active battle pet.
If the countdown expires after one choice, the client sends that selected action
as an ordinary native AC50 request. The server timeout leaves the missing fighter
idle, without defense. With no choices, the client sends nothing and the server
idles both fighters. Complete batches and their negotiation remain unchanged.

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
Focused wire, compatibility, persistence, race and rendering checks are recorded
in [TESTING.md](TESTING.md#negotiated-combat-round-transport---2026-11-02).

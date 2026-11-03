# Legacy battle item capture — 2026-11-03

Two user-operated tests established the native item behavior. Original captures
remain ignored under `var/pet-switch-capture/`; regression tests retain only the
relevant combat packets, independently of production constants.

## Legacy client and legacy server

The player and pet each used Grape Syrup (32280) on the player. The server
acknowledged both actors with AC53:5 before executing their item actions. Each
action consumed one item with AC23:9, selected the actor with AC50:6, and sent
the recovery using motion 10024 in AC50:1. The player gained 400 SP per use:
779 → 1179 → 1579. The action's SP result used digit mode **0**, rather than
the previously supported recovery modes 3/4. `FUN_0039b558` confirms that only
modes 1/2 negate the amount; the captured mode 0 adds it. Gonline now interprets
mode 0 as recovery, including at the motion's contact cue.

The user subsequently supplied legacy Grape Syrup and Nian Noodle screenshots.
They show outlined cyan SP digits and outlined orange HP digits on the recipient,
respectively. `FUN_0039e884` confirms `Num_Blue10_1` for SP and
`Num_YellowRed10_2` for HP, including recovery. The client now uses these sheets
for recovery instead of plain green HP text. The screenshots are visual evidence;
they do not establish Nian Noodle's packet contents or recovery formula.

The PktMon reference trace did not retain the outgoing item requests; its
request layout must not be inferred from the server responses alone.

## Legacy client and Gonline server

A local TCP relay captured both directions while the player and pet each used
Spicy Hot Pot (32176) on the player. Both requests were **52 bytes**:

| Field | Bytes |
| --- | ---: |
| AC50, item subcommand 2 | 2 |
| Actor and target X/Y | 4 |
| Item ID | 2 |
| Opaque random bytes | 2 |
| Bag slot and compatibility flag | 2 |
| Native send-path compatibility trailer | 40 |

The two requests named bag slot 3 and different actors, with unrelated trailer
contents. Gonline's exact 12-byte length check discarded both; neither received
an item submission acknowledgment, and the battle waited for timeout. The server
now accepts the observed 52-byte native form and the existing 12-byte form.
Other lengths remain rejected. The trailer grants no authority: actor ownership,
target eligibility, bag identity/count and transactional consumption are still
validated normally. The negotiated round-choice format retains its existing
12-byte item entries and version; legacy clients do not negotiate that extension.

Engine and race-enabled server tests replay the actual captured requests and
verify immediate round progression and durable consumption of exactly two items.
Client tests replay the captured mode-zero recovery and verify contact timing.
These are automated replays; the corrected server was not restarted for a second
live acceptance test during this capture. The relay was stopped, PktMon filters
were restored and the temporary legacy server-list entry was removed.

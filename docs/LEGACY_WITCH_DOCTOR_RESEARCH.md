# Kelan Witch Doctor treatment: Legacy reference

## Evidence

Legacy Client on the Legacy server, 2026-11-04. The packet capture is
`var/pet-switch-capture/kelan-treatment-20261104-01.pcapng`; screenshots are
under `var/screenshots/witch doctor/`. Packet filters were restored afterward.
The capture contains other players' traffic, which is not attributed to this
interaction merely by timing.

The Witch Doctor asks “What can I do for you?” and offers **Treatment**,
**Save the record**, **Pet hotel** and **Close**. Treatment displays a
confirmation saying “This stay free, Stay?” with Confirm and Cancel. Before
treatment, the screenshot shows the player at 185/196 HP and 100/106 SP, with
25 gold. On confirmation the scene darkens and shows the player lying on a bed
with a floating Z. The captured treatment subsequently reports 196 HP and
106 SP. There is no gold deduction in the observed treatment response; the
confirmation describes this treatment as free.

## Captured packets

One offer was followed later by a second interaction and offer. The final
successful sequence was:

1. Client `AC20:9 [30]`, then `AC20:6`; server `AC31:2` with a zero 32-bit
   value and `AC20:9`. The free-treatment confirmation was displayed.
2. The captured server response starts with `AC31:10`, followed by player
   `AC8:1` stat 25 = 196; `AC8:2` pet slot 2, stat 25 = 185; player `AC8:1`
   stat 26 = 106; and `AC31:1` result 0.
3. `AC20:8` followed approximately 6.3 seconds later. This is an observed
   interval in one interaction, not proof of a fixed animation duration or of
   the cause of the final event packet.

The screenshots and timing associate `AC31:10` with the bed presentation,
but its exact semantics should be confirmed against a further capture or the
Legacy decompilation. The pet HP update establishes that a pet stat was sent;
it does not by itself prove which pets are eligible or that the pet's HP was
previously below its maximum. No outgoing `AC31` confirmation appears in the
capture, even though treatment succeeded; outgoing traffic was incomplete in
previous captures too. Do not infer the request from its absence.

## Gonline comparison and implementation work

`internal/server/shops.go` already offers free clinic rest, restores player
HP/SP and accompanying pet vitals, persists the changes, and returns an
`AC31:1` completion. Its existing tests cover clinic rest and pet persistence.
The current server sends its stat packets and a character-state rest effect
before completion; the reference instead shows `AC31:10` before the observed
stat updates. The Gonline client and server need their treatment presentation,
message order, and native packet semantics checked against this trace. Preserve
transactional persistence and do not introduce a separate protocol exception
for the Gonline client.

Further tests should record the complete outgoing confirmation and compare
cancel, full-health, injured active and resting pets, unavailable funds at
paid clinics if any, and the end of the bed animation. The **Save the record**
and **Pet hotel** options are separate mechanisms and are not established by
this treatment capture.

## Save the record: 2026-11-04

The separate capture `var/pet-switch-capture/kelan-spawn-20261104-01.pcapng`
covers two consecutive saves at the Kelan Witch Doctor. The screenshot
`var/screenshots/witch doctor/spawn point.png` shows the dialogue: “Your record
point has been restored here.” No separate confirmation window or cost is
visible in that screenshot. The capture alone does not establish the exact
coordinates stored or demonstrate a later respawn.

Both attempts produced the same observed response sequence: the Witch Doctor
question frame, client `AC20:6`, a server dialogue frame with talk ID `0x0379b6`,
`AC5:21 [1]`, `AC20:10`, then `AC20:8`. The repeated save did not show a
different result in these packets. The outgoing capture is known to be
incomplete, so it cannot establish every choice or acknowledgement sent.

Gonline's Witch Doctor fallback already commits the character's current map
and coordinates, sends that dialogue and `AC5:21 [1]`, then event completion
and resume; the EVE record-point action also persists a location and sends the
status. This is consistent with the observed response, though the destination
coordinates, persistence across relog, actual return after defeat, and behavior
when saving a different location require separate verification.

## Pet Hotel deposit and withdrawal: 2026-11-04

The capture `var/pet-switch-capture/kelan-pet-hotel-20261104-01.pcapng`
and the hotel screenshots under `var/screenshots/witch doctor/` show the
player storing and then retrieving Fragrance, a level 5 pet. Gold remained 25
in the screenshots. The hotel window has a stored-pet list on the left, an
accompanying-pet list on the right, transfer arrows between them, a details
panel and a Close button. Selecting Fragrance on the right showed its sprite,
level, HP 185/185 and intimacy 64. Depositing it displayed “Fragrance left
party”; withdrawing it displayed “Fragrance joined party”.

Observed server packets, excluding unrelated surrounding traffic:

1. Opening: `AC31:7`, then `AC20:9`. The hotel was initially empty.
2. Deposit: `AC31:3 [1,2]`, `AC15:2` with the owner ID and party slot 2,
   `AC31:9`, then `AC31:12`. The screenshot shows Fragrance in hotel slot 1.
3. Withdrawal: fourteen `AC8:2` pet stat packets for party slot 2, followed
   by an `AC15:1` pet record, `AC31:5 [1,2]`, `AC31:4 [1]`, `AC31:8`,
   and `AC31:12`. The screenshots show it back in party slot 2 and the hotel
   slot empty. An `AC20:8` arrived later; its exact relationship to closing
   the window remains to be checked.

The outgoing deposit and withdrawal requests are absent from this capture,
which is known to miss some client-to-server traffic. The screenshot does not
establish the UI controls or server semantics for exchanging multiple pets,
locked hotel slots, full capacity, or a pet that is currently in battle.

Gonline Server already has durable hotel storage, capacity checks, pet slot
management and transactional exchanges in `internal/server/shops.go`. It
sends additional roster and hotel-list packets, uses `AC31:8` and `AC31:12`
as final unlocks for transfers, and sends incoming pet progression after its
copy-to-team packet. Compare that behavior against the captured `AC31:9` on
deposit and the observed withdrawal ordering before changing it. Gonline's
client needs the hotel window, selection and transfer presentation verified
end to end, including the two notifications. Capture reliable outgoing
requests before claiming that the existing request decoder matches Legacy.

## Gonline implementation follow-up

Implemented on `Client-rewrite`: session-owned native service forms and packet
handling, with captured response ordering on the server. The client sends the
reference counted slot/quantity cart and selected-slot transfer requests; the
shop and storage selectors are corroborated by `FUN_00369670` /
`FUN_0036c8ec` and `FUN_001df658` / `FUN_001df77c`, and Pet Hotel selectors
by the reference sender at `0x2d0f96`. The capture's missing outgoing messages
remain an evidence limitation; they were not silently treated as absent requests.

Purchases, sales, storage and hotel transfers publish results after durable SQL
commit. Sales and transfers recheck changed resources against owned SQL state;
service scope is invalidated on movement, map change, character handoff or a new
NPC interaction. Native item-picker questions authorize only their listed stock.
The client applies server receipts rather than predicting inventory or gold.
Treatment uses the native five-second timer (`FUN_002b6d08`) and defers its
completion acknowledgement through the presentation. Spawn-point saving retains
its existing durable event path.

Focused validation covers native request bytes, captured receipt sequences,
malformed packets, covered inventory cells, pending submissions, hotel stat/copy
ordering, scope, stale SQL resources and character cleanup. Rendered service
screens are under ignored `var/validation/npc-service-screens/`. Relevant client
integrations and server race tests were run; live acceptance against both server
implementations is still required. No database schema migration is needed.

### Presentation traced in the original client

`FUN_002242d0` packs occupied hotel and team slots into consecutive visual
rows; transfer requests retain the original slot numbers. Selection uses the
blue row and orange name shown in `select pet.png`; hover alone controls the
lower preview, and leaving the rows clears it without cancelling selection.
The screenshot's selected background/name RGB565 values are `0xc6fe`/`0xfc00`.

Treatment rendering is `FUN_003bb51c`, called by `FUN_004a1d60` while the
clinic animation flag is active. It uses item 39011's inventory-footprint
source rectangle (`FUN_00459048`), centers that rectangle on the screen,
and draws a copied human at `(screenWidth/2, screenHeight/2 - 10)` with
action 26, frame 1. Every 300 ms its phase advances modulo four: small Z at
`(centerX, bedTop - 10)`, medium Z at `(+10, bedTop - 45)`, large Z at
`(+25, bedTop - 100)`, then a blank phase. The scales are 0.2, 0.4 and 0.7.
`FUN_003ba370` uses font 1's uppercase `Word_Red_64U` bitmap strip for Z;
the ordinary dialogue font is not used. The five-second duration remains
owned by `FUN_002b6d08`.

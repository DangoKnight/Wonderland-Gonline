# Kelan Props Keeper: Legacy reference

## Evidence

Legacy Client on the Legacy server, 2026-11-04. Capture:
`var/pet-switch-capture/kelan-props-keeper-20261104-01.pcapng`. Screenshots:
`var/screenshots/props keeper/`. Packet filters were restored after capture.

The Props Keeper opens a window titled **bank**, with **store items** on the
left, **items held** on the right, transfer arrows and Close. The screenshots
show multiple inventory stacks highlighted and moved together, including five
Training Fare items; one deposited bottle was then selected and withdrawn.
Hovering Training Fare displayed its name and item details. No change to the
25-gold balance is visible in the screenshots.

## Observed packets

The earliest captured shop interaction includes server `AC29:6` followed by
`AC20:9`. The trace begins near the opening, so it does not establish that
these were the only packets used to open or populate the window.

The deposit response contained the following alternating messages, followed
by `AC30:7`:

- `AC23:9 [8,1]`, then `AC30:2` carrying item ID 34330 and quantity 1.
- `AC23:9 [9,1]`, then `AC30:2` carrying item ID 34331 and quantity 1.
- `AC23:9 [11,5]`, then `AC30:2` carrying item ID 34258 and quantity 5.

The withdrawal response contained `AC30:5 [2,1]`, `AC23:6` carrying item
ID 34331 and quantity 1, then `AC30:6`. The screenshot shows the second
stored stack disappearing and the blue bottle returning to the bag. The
meaning of each byte in `AC30:5` needs more examples before assigning names.
A later `AC20:8` was observed after the withdrawal; its exact relationship to
window closure requires confirmation.

The outgoing deposit and withdrawal requests are absent from the trace. This
capture setup has missed outgoing requests in previous interactions, so their
absence does not imply that Legacy Client performed transfers locally. The
remaining bytes of the item records are not fully interpreted by these examples.

## Gonline comparison and remaining work

`internal/server/shops.go` has storage opening and item transfer handling with
durable commit and inventory updates. It sends a cleared, full storage snapshot
and both completion messages after transfers. The captured Legacy responses
instead show incremental storage additions/removal, paired inventory updates,
and a single direction-specific completion. Verify how these packets interact
with the Legacy client before changing the server, and keep the transaction
atomic and authoritative in the gameplay database.

Verify the Gonline client's two-pane storage window, multi-selection, whole
stack transfers, item tooltips, highlights and update handling against the
screenshots. A reliable bidirectional capture is needed for request formats;
additional tests should cover partial stacks, full inventory or storage,
reordering, restricted items and reopening an already populated storage.

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

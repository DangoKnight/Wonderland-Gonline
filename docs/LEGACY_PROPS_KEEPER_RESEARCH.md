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

### Native storage close handshake (2026-11-08)

The mixed-client reproduction is recorded in
`var/pet-switch-capture/mixed-client-lock-6414.events.jsonl`. Legacy character
10004 clicked Kelan actor 38 itself; a second client's action was not required.
The trace shows AC20:1, movement lock, AC20:9, two premature AC20:8 resumes,
then the client's AC20:6 with no response. Subsequent movement still received
replies, while no further NPC/warp requests were sent. This capture filtered
out AC29/30, so their opening payloads are established by source/decompilation,
not claimed as captured here.

Native storage hide handler `FUN_001ded68` calls `FUN_0030a508`, which sets the
event-completion flag at +0x7108. The event loop sends its completion ACK and
waits for resume. Storage must therefore retain an owned event until the close
ACK, rather than ending the event as soon as the window opens. Both authored
Props/Stock Keeper services and fallback storage now use that lifetime.
Opening sends the storage snapshot, AC29:6 and AC20:9 without an early resume
or unrelated record-point update. Closing acknowledges once, releases movement
and storage authority, and leaves duplicate ACKs inert. Gonline's cancellation
uses the same cleanup; teleport/disconnect cannot revive the old service.

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

### Service lifecycle audit (2026-11-08)

The same completion ownership applies to weapon/props/armor shop buy and sell
windows, Props/Stock Keeper, Pet Hotel and Treatment. They now retain their event
until native AC20:6 closes the window or completes the treatment presentation.
Pet Hotel hide handler `FUN_002238cc` also calls `FUN_0030a508`; Treatment's
`FUN_002b6d08` sets the completion flag after its five-second presentation.
Opening these services must not send an early AC20:8. Completion releases the
event and its resource authority once; cancellation and map changes invalidate
the same ownership. Duplicate acknowledgements remain inert.

Fallback service questions use mode zero. Their answer acknowledgement is
consumed separately from the eventual service completion, including nested
buy/sell questions and treatment, hotel and spawn-save choices. Native item-picker
questions relinquish their question callback when opening a cart, retain close
ownership and authorize only one purchase from their listed stock.

Money Bank and Message Board remain independent forms opened after the dialogue
resume. Rebirth job selection and companion rebirth now follow that ordering
as well, because their handlers reject commands during an active conversation.
The audit covers every currently named EVE service family; unsupported service
codes retain the existing fallback and are not claimed as implemented.

Regression coverage includes authored and fallback entry, trailing question
acknowledgements, close/completion, cancellation, duplicate acknowledgements,
private packet isolation and native-cart replay rejection. These are source and
decompilation checks; the generalized changes still need live Legacy acceptance.

Sell additionally needs AC20:9 immediately after AC27:3/4. Native sale hide
handler `FUN_0036c8c8` sets completion pending (+0x7108), but leaves the
waiting-for-server flag (+0x7109) untouched. The event loop cannot send AC20:6
while that wait flag remains set. AC20:9 clears it without prematurely ending
the interaction. Earlier lifecycle tests injected AC20:6 directly and missed
this prerequisite; a separate raw-byte opening golden now covers both sale modes.

The wait-release requirement is shared by all conversation-owned service
handoffs, not specific to Sell. `publishNPCService` now sends the window/offer
payload followed by AC20:9 for shop catalogs, sale windows, storage, hotel and
treatment. It deliberately leaves event ownership intact until completion.
Native item-picker event frames already clear the native wait flag through
AC20:6's receive handler; independent forms instead follow the final AC20:8.
The common lifecycle tests model a waiting native client and refuse to emit a
close acknowledgement until an actual wait-release packet has arrived. Both
authored service actions and fallback merchant/doctor choices are covered.

### Stored-pet field layout correction (2026-11-08)

AC31:6's 64-byte-plus-name hotel record uses header byte 25 for amity and
header byte 26 for the native compatibility value 1. After the name, three
skill records and six equipment IDs, bytes 57/58/59 plus the name length are
rebirth count, potential and job. `FUN_003b377c` decodes them into
+0x2a72/73/74; `FUN_003b3cfc` copies them to +0x1f99/9a/9b.
The hotel preview calls `FUN_00485490(level, rebirth)` to obtain
`level + 100*rebirth`. Sending amity in that rebirth field therefore inflated
the displayed maximum HP. Sending job in the header amity field also produced
zero intimacy for ordinary pets. The server record and Gonline decoder now use
the native layout. This is a wire correction; stored pet data is unchanged.

Focused validation covers native request bytes, captured receipt sequences,
malformed packets, covered inventory cells, pending submissions, hotel stat/copy
ordering, scope, stale SQL resources and character cleanup. Rendered service
screens are under ignored `var/validation/npc-service-screens/`. Relevant client
integrations and server race tests were run; live acceptance against both server
implementations is still required. No database schema migration is needed.

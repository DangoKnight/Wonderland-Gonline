# Kelan Props Shop: Legacy reference

## Evidence

Legacy Client on the Legacy server, 2026-11-04. The player had 0 gold.
Screenshots are locally available under `var/screenshots/props shop/`; the
scoped packet capture is `var/pet-switch-capture/kelan-shop-20261104-01.pcapng`.
The capture started before the screenshots and its packet filters were restored
afterwards. It does not contain a successful purchase.

The shopkeeper is labelled **Props Shop** in Kelan Village. Talking opens a
question, “What can I do for you?”, with **Buy items**, **Sell goods** and
**Close**. Buying opens a **Miscellaneous** window with columns for name,
effects and single price. The observed rows were Bread (10), Rice Wine (50),
Ordinary Wood (30), Lauan (30) and Copper Ore (10). A ten-slot purchase list,
purchase amount, money held, Buy and Close appear below the rows.

The screenshot shows one Ordinary Wood and two Lauan in the cart, with total
90 and money held 0. Pressing Buy displayed “Not enough Gold” while retaining
the cart. There was no purchase request in the decoded capture during this
attempt. A later sale capture was also missing its outgoing sale request despite
a successful server response, so this capture cannot establish whether the
insufficient-funds check was local or whether a request was sent. The screenshot shows the
notification, not whether it automatically disappears or how it is dismissed.

During the interaction the trace contains event traffic including client
`AC20:6` and server `AC20:1`; it does not establish a complete shop-opening
packet sequence. Other server packets in the capture cannot be attributed to
the shop solely by their timing. In particular, do not treat `AC35:12` traffic
as a confirmed shop catalog packet without further evidence.

## Gonline implementation assessment

The server already has NPC shop handling in `internal/server/shops.go`: it
constructs an authorized catalog from map definitions or configured defaults,
checks selected items, prices and inventory capacity, and commits the gold and
items together. Its supported request formats, replies, fallback catalog and
prices still need comparison with a successful Legacy capture; existing code
and tests are not independent evidence of the Legacy wire format.

The Gonline client needs the reference interaction verified end to end. The
Miscellaneous view needs the item rows, cart quantities, calculated total and
current gold, plus the observed insufficient-gold notice and cart retention.
Local feedback should not replace server-side authorization or transactional
purchase validation. Shop data and prices belong in the shared assets database;
player currency and inventory belong in the gameplay database.

## Remaining capture

With enough gold, record opening the shop and purchasing a known item and
quantity. Pause between opening, adding to the cart, confirming and closing;
record before/after gold and inventory as well as screenshots. Capture a
multi-item purchase and inventory-full case later if practical. These traces
are needed to identify the exact request payload, item and gold updates,
confirmation or error packets, ordering, and whether opening or closing the
window requires additional messages. Selling is a separate interaction.

## Successful sale: 2026-11-04

A second capture, `var/pet-switch-capture/kelan-sale-20261104-02.pcapng`,
contains the Legacy server response to selling four selected inventory stacks.
The screenshots (`sell.png`, `select sell.png` and `confirm sell.png`) show a
props-only inventory grid with selected stacks highlighted, a selling amount of
83 and money held of 0. After confirmation the four selected stacks disappear,
the selling amount resets to 0, money held becomes 83, the sale window remains
open and a notification lists the separate 68, 2, 2 and 11 gold gains.

The observed server packets, in order, were:

1. `AC27:3` opens the props sale window, followed by `AC20:9`.
2. On sale completion, four `AC23:9` inventory removals for slot/count pairs
   (14,34), (15,2), (17,1), (18,11) alternate with four `AC26:1` messages
   carrying 68, 2, 2 and 11 respectively.
3. `AC27:2` with result 0 follows those updates. A later `AC20:8` is observed
   after the sale; its relationship to window closing needs confirmation.

The server's current `sell` implementation sends all inventory removals, then
an `AC26:4` balance, then the `AC27:2` result. This differs in wire ordering
and gold subcode from the Legacy response above. The client's current gold
dispatch handles `AC26:4`; verify and implement `AC26:1` gain handling and
the corresponding per-item notices when this feature is implemented. Preserve
server-side price, ownership and capacity checks and atomic persistence.

**Capture limitation:** the raw trace contains just one outgoing game segment
(`AC20:6`), even though the server processed the sale. It therefore does not
contain the sale request. The existing Gonline request decoder is not verified
by this capture. Diagnose outbound capture completeness or use a suitable
relay before inferring the request layout. The user can now afford a purchase,
which should be captured separately after verifying both directions.

## Successful purchase: 2026-11-04

The capture `var/pet-switch-capture/kelan-buy-20261104-02.pcapng` and
`confirm buy.png` show a purchase of one Copper Ore for 10 gold and two
Ordinary Wood for 60 gold. The starting balance was 83 and the resulting
balance was 13. The shop window closed, separate notices reported the two
gold deductions, and an item notice listed both acquisitions.

The server response, in order, was:

1. `AC26:2` carrying 10; `AC23:6` carrying item ID 42008 and quantity 1.
2. `AC26:2` carrying 60; `AC23:6` carrying item ID 45001 and quantity 2.
3. `AC27:1` with result 0, followed by `AC20:8`.

Both item messages have a 31-byte payload, including the command and subcode;
the bytes after the item ID and quantity were zero in this capture. These
examples do not establish the meanings of all remaining fields. Each price
message reflects a deduction for one selected entry; do not interpret it as
the new total balance.

The existing Gonline purchase handler sends an inventory addition packet, an
`AC26:4` final balance, and `AC27:1` success. That differs from the observed
per-entry deduction and addition order. Client support must account for
`AC26:2` deductions and the purchase/item notices, while the server must
continue to validate and commit the entire purchase transaction atomically.

The outgoing `AC27` buy request is absent from this capture, as the sale
request was from the preceding trace. The observed outgoing `AC20:9` and
`AC20:6` do not establish the cart request format. A capture method that
reliably observes both directions remains necessary before finalizing the
request encoder and decoder.

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

### Window rendering reference

The native map loader translates the first question-header byte (source record
`+0x1d`) into runtime presentation field `+0x42`: 1 becomes dialogue mode 0,
2 becomes equipment-shop mode 1, and 3 becomes props-shop mode 3. The trailing
header byte is not the mode. Kelan question 6 has header `[3,1,0]`; reading the
last byte incorrectly presented its merchandise as dialogue options and also
prevented server purchase authorization. Both paths now use the native category.
Only item entries with option type 3 enter the catalog; the hidden type-4 entry
is not stock. A client regression loads the extracted Kelan map and verifies its
five items and the 70-gold Copper Ore/Ordinary Wood cart. Server tests cover the
matching authorization and hidden-entry exclusion.

The fixed bitmap constructors are `FUN_00368f10` (shop, 409×525),
`FUN_0036c60c` (sale, 241×505), `FUN_001de750` (storage, 421×481), and
`FUN_00223208` (hotel, 472×476). These are fixed forms; drawing their
backgrounds as nine-slice panels crops the artwork and misaligns the controls.
The client uses their native grid, cart, button and arrow coordinates. Hotel
rows use the 37-pixel pitch, `Icon_Inn_1`, element icons and the compact
`FUN_0029d3a4` level/HP/SP artwork. Shop effect cells show the item effects
before falling back to its description, and bound wrapped text to the row.
Visual fixtures reproduce the five-item Props Shop catalog and save rendered
screens to `var/validation/npc-service-screens/` for comparison with the
screenshots under `var/screenshots/`.

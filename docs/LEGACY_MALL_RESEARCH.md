# Legacy Item Mall reference

## Evidence and test

Legacy Alogin on Rhodes3 (`20.187.123.156:6414`), 2026-11-06. The user supplied
the Item Mall screenshot in chat and tested with Test0002: select one **Robot
Maid**, automatically enter Shopping Cart, return to the mall, reopen Shopping
Cart, attempt checkout and receive a rejection. The screenshot shows both IM
Points and Bonus Points at zero. No successful purchase was tested; Robot Maid's
price was not provided and must not be inferred from other screenshot rows.

Capture artifacts are local, ignored references:

- `var/pet-switch-capture/mall-failed-purchase-20261106-01.pcapng`
- Matching `-status.json`, `-helper.log`, `-decode.log` and `.mall.json`
- Decoder: `var/pet-switch-capture/decode_mall.go`

Capture started `2026-11-07T01:31:15.5687759Z`. Packet Monitor recorded 1,074
observations, reported 13 packet drops and no ETL events lost, and restored the
initial empty filters. Deduplicated TCP reassembly recovered 506 protocol frames.
The active mall connection was local port 50450. Some outgoing TCP sequence ranges
are missing, so packet absence alone is not proof a request was never sent.

## Observed checkout exchange

Times are relative to the first decoded frame.

| Time | Direction | Decimal payload | Interpretation |
| --- | --- | --- | --- |
| 2.414 s | Server → client | `[35,4,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0]` | Initial balance response, all zero |
| 53.931 s | Client → server | `[34,1,1]` | Checkout balance/resume query |
| 55.864 s | Server → client | `[35,4,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0]` | Refreshed balance, all zero |

The response batch also includes `[54,201,0,1,102,0,3,104,0,3,101,0,3,103,0,2]`.
Its purpose is not resolved here; do not reinterpret it as a cart receipt or
inventory acquisition. No AC75 purchase request or AC75 purchase receipt was
recovered. No mall catalog refresh packet was recovered during this test.

## Decompiled client sequencing

Cross-check: `var/decompiled/aLogin_ca19ee087b60_full.c`.

- `FUN_0024a734` appends the chosen item to client cart memory, initializes its
  quantity to one and updates the cart presentation. Cart entries use native
  35-byte records; leaving the cart does not itself purchase the item.
- The incoming balance dispatcher copies the refreshed balance into the mall
  model and invokes `FUN_00242ae4` on the checkout continuation path.
- `FUN_00242ae4` compares the available balance at object offset `0x565c` with
  the calculated cart cost at `0x5658`. Insufficient points produce the local
  notice **“Not enough Points”** for 1,500 ms and return before purchase sending.
- If funds suffice, inventory/furniture capacity is checked by `FUN_002423c4`;
  failures produce “Inventory is full” or “No space for furniture.”
- Only the successful preflight reaches `FUN_00242230`, which serializes cart
  rows and dispatches AC75:1. The bonus path `FUN_00242bd8` dispatches AC75:5.
  These senders are decompile evidence, not captured successful purchases.

The captured zero-balance exchange and native guard explain this failed attempt:
the client receives fresh balances, then rejects locally before the actual buy
request. There is no observed server-side failed purchase transaction in this
sample. Do not add a fabricated purchase-failure receipt to the golden sequence.

## Visual and behavior requirements for Gonline

The supplied 800×600 Legacy viewport shows a nearly full-screen pale-blue mall
window, with native decorative borders and a separate Shopping Cart view.

- Left tabs: **Points Mall** and **Bonus Mall**; current balances below.
- Left character panel: selected character name, element, level, animated actor,
  character navigation arrows, HP/SP/EXP bars, gold, point allocation and stats;
  a Reset control sits at the bottom.
- Right category tabs: **Hot**, **Armory**, **Weaponry**, **Grocery**,
  **Furniture**, **Slot Machine**, **Forging Room**.
- Twelve catalog rows per shown page, containing icon/badge, item name,
  description, points price and **Add to Cart** button. Long names/descriptions
  wrap within their respective columns. The screenshot's price examples belong
  to those displayed items, not necessarily the Robot Maid.
- Bottom controls: **Previous**, page indicator, **Next**, **Security Lock**,
  **Shop Cart**, **Check Out**, **Exit**.
- Adding the tested item opened the cart immediately. Returning to the catalog
  and reopening the cart retained the user's pending selection.

Use the native form/assets and decompilation for layout and cart details before
implementing, rather than replacing this with NPC dialogue choices. Keep the cart
as session-local client state; it is not an inventory grant. Balance refresh must
not refresh/rebuild the catalog and accidentally empty the pending cart.

## Existing server support and remaining checks

`internal/server/mall.go` already supports read-only AC34 checkout balance
queries, AC75 points/bonus cart purchasing and transactional SQL debit/delivery.
This capture corroborates the `[34,1,1]` query. It does not verify every existing
cart writer, receipt or successful delivery layout.

Remaining evidence: successful purchase receipts and inventory delivery, exact
Robot Maid catalog ID/price on the tested server, cart quantity/removal behavior,
bonus checkout, security lock and catalog/category refresh. The catalog has a
**Maid Robot** candidate at item 39066; do not identify the user's selected item
solely by the reversed English name without a catalog/request record.

Client-side preflight improves feedback, but authoritative server balance,
catalog price, ownership and capacity checks remain mandatory. Debit and grant
must commit together before success receipts.

## Gonline implementation

`client/wlo/mall` now owns the native catalog/cart presentation and validated
per-session catalog, balance and receipt state. The mall toolbar button opens
the 778×554 catalog; Add to Cart opens the separate 548×534 seven-row cart.
Both use the original skins and controls, twelve catalog rows per page, wrapped
names/descriptions, badges, proportional 32-pixel furniture thumbnails, hover
details and the player's status/animated preview. Escape closes the module;
map changes/battle entry hide it and disconnect resets its account state.

Points checkout follows the captured `[34,1,1]` balance query, waits for AC35:4,
then performs funds/capacity preflight before AC75:1. Bonus checkout uses AC75:5.
Cart rows retain exact catalog item/category/order identities. Quantities are
bounded by the native 50 limit. At the user's request, Gonline permits editable
amounts for every cart item, including equipment and fixed prop bundles; fields
are read-only only while checkout is outstanding.
Points and bonus carts do not mix. Catalog replacement invalidates unsent carts;
balance refresh does not. Only matching success receipts remove cart rows;
inventory/points are never granted or debited speculatively. Pending checkout
is locked against duplicate clicks. A timed-out submitted purchase remains
locked until receipts arrive or the connection resets, because it may already
have committed. The server retains transactional authoritative validation.

Native `FUN_00240584` also establishes that the wire category is **not** the tab
index: 1 Weaponry, 2 Armory, 3/4 Grocery/Props, 5 Furniture. Gonline's descriptive
constants/name-derived defaults were corrected without rewriting explicit
administrator category IDs. Hot is assembled in Grocery, Armory, Weaponry,
Furniture order from badges (1–4 for Grocery, 1–3 otherwise), capped at 60 entries.
The old compatibility values 6/7 remain available for authored category data;
they do not implement the separate native slot-machine/forging interfaces.

Security Lock is visibly unavailable and reports that limitation. Its
unverified password/account protocol is not simulated or claimed to protect
purchases. Character rotation/reset and outfit fitting affect the preview only.
Successful live Legacy purchase acceptance remains untested. The dedicated
machine launcher and forging panel are described below.

## Screenshot catalog assets (2026-11-06)

`internal/assetsql/mall_defaults.json` is the authored, plain JSON initialization
asset for the supplied complete mall screenshots. The same definitions are built
into the tracked `server_defaults/catalog.db.gz`; servers need no client extraction.
Gameplay and administration use structured `catalog_mall` rows, not the JSON.

| Currency | Grocery, including bundles | Armory | Weaponry | Furniture | Total | Hot view |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Points | 130 | 9 | 10 | 10 | 159 | 55 |
| Bonus | 35 | 15 | 12 | 9 | 71 | 16 |

Hot pages reuse the category entries; they are not additional purchasable rows.
Rows retain item IDs, category, order, bundle count, effective cost, original
price, discount percentage and NEW/HOT badges. Distinct single/five-pill and
crystal offers retain distinct identities. Material-pack offers grant 50 packs;
the other explicit `(N pcs)` suffixes are quantities. Charges printed on a
Repairing Spanner, VIP Card, Monster Net, Fast Jet or Sandglass are properties of
one purchased item and do not multiply the delivered quantity.

Matches use the imported item definitions and English descriptions, not names
alone: for example screenshot Luxury Hat Bag is item 57819 (Gent Hat Bag in the
current name catalog); Ancient Wand Bag is 57465; Maid Robot is 39066.
The 36-point Gold Chest Key matches tradable item 30099 by description, rather
than the similarly named nontransferable item 30334.
Display names continue to use the existing language catalogs. New bag reward contents
are not inferred from the shop screenshots.

The previous 199-row import is retained as `mall_legacy_defaults.json` solely
for migration comparison. Explicit schema-v59 migration replaces it only when
the complete existing catalog exactly matches that import. Administrator edits,
extra/deleted rows, reordering and an intentionally empty catalog are preserved.
Fresh repository databases already contain all 230 listings. To upgrade an
existing default database, stop the server and run:

```powershell
go run ./cmd/database-migrate -config config.local.json
```

An administrator-customized catalog needs an intentional administrative import
of the new JSON if these defaults are desired. Restarting never reapplies them.

`point_cost` is the effective per-listing charge; `original_price` and `discount`
describe the crossed-out/list price. The server advertises the discount only
when native round-to-even arithmetic reproduces the charged amount. Otherwise
it advertises the effective price without a discount, keeping existing custom
catalogs consistent. Registration Voucher retains the previous import's 30%
payable percentage (4 points rounds to 1); screenshots alone cannot establish
the precise percentage from those rounded numbers.

Slot-machine thumbnails and the forging screen are separate services, not item
offers. They do not supply enough information to author game costs or reward
pools, and are not inserted as invented purchasable items.

## Bag contents preview

Clicking a catalog item's icon/name opens the native `Article` contents panel
when its reward list is known. It uses ten rows per column, 36-pixel row spacing,
32-pixel icons, quantities, localized item names and hover details. Cancel, the
close button and Escape dismiss the preview; closing/resetting the mall also
clears its modal ownership. Viewing contents never adds a cart row or purchases.

`FUN_002a3404`, `FUN_002a37fc`, `FUN_002a467c` and `FUN_002a494c` establish the
layout. The native forty-icon limit is retained. `FUN_001ada04` reads `Lbd.dat`:
a little-endian U16 group count, one U8 reward count per group, then each group's
U16 item ID, U8 metadata/version and repeated U16 reward ID/U8 quantity records.
The group metadata is not a probability. Native `FUN_001adfb8` can also request
updated definitions through AC91; the current preview uses exported definitions.

`internal/clientassets/mall_contents.json` contains 55 client-only display lists:
30 observed native Lbd groups, 24 old executable reward tables from the existing
authored gacha import, and the Luxury Hat Bag list transcribed from screenshot
`var/mall/bags/Captura de pantalla 2026-11-07 001329.png`. Native Lbd takes priority
over old imports; authored screenshot corrections apply last. Previous exported
observations are retained because Lbd is a bounded, server-populated cache. Pet Upgrade
Bag 55134 has 26 entries matching the supplied screenshot, with three Pet Return
Scrolls. These lists do not enable server awards, assign weights or change odds.
The 2026-11-07 Lbd observation is 1,250 bytes, SHA-256
`38db8f695600c6ab8c89d8facdb660e5097c458d74b94aa38dd005131966b4a7`.
Some newer bags are absent from both sources; the UI reports unavailable contents
instead of inventing rewards. The source client may learn more lists from its
server and persist them to Lbd.dat. Rebuild the tracked preview file with:

```powershell
go run ./cmd/mall-previews -source "C:\Games\UpdatedWonderlandClient"
```

The command accepts either language installation, reads IDs/quantities only and
retains existing language catalogs for display. It does not require live-server
access, modify the installation or depend on a machine-specific path at runtime.

The additional references under `var/mall/bags` confirm Wood/Metal Material Packs,
Trilobite, Tarsius, Dolphin, War Horse, One Yuan, the first two Hot pages of bags,
Transform/Strengthen/Gear Packs and equipment fitting. Seven Colors Egg uses
companion names and portraits instead of voucher icons; its presentation flag
lives in the authored asset and its voucher-to-companion mapping is generated
from the existing pet-voucher definitions.

### Equipment fitting

Clicking a wearable item or equipment box now changes a separate mall preview.
Equipment-box mappings in `internal/clientassets/mall_fitting.json` are generated
from the existing authored boxed-item definitions. They are not guessed from
names. The normal layered Human renderer draws the trial robe, wings or weapon.
Trials replace the previous trial in the same equipment slot, retain other slots,
and can be cleared with Reset. No equip, consume or purchase packet is sent.
Session reset clears trial state; the real character, inventory and world sprite
are untouched.

The comparison subtracts replaced worn equipment (including its forging), then
adds trial equipment. Changed combat values are red `base+delta`/`base-delta`,
while changed maximum HP/SP are red and update only the preview bars. As in
`002132.png`, Purple Wing Robe plus Angel Wings gives +100 maximum HP, +10 MAT,
+12 DEF and +12 MDEF. Native catalog hover handling distinguishes fitting from
contents inspection (`FUN_00263038`); box resolution uses the authored mappings
also used by server box opening. Fitting does not enforce equip-level limits,
since it is a local comparison rather than equipping the item.

### Tab poses and badges

Native tab callbacks `FUN_00264fa8` through `FUN_00265160` pass pose modes to
`FUN_002669fc`: Hot, Armory and Slot Machine use standing action 13; Weaponry
uses battle-ready action 17; Grocery, Furniture and Forging use seated action
51. The mall applies these poses to both ordinary and fitted previews, resets
the facing when switching tabs, and retains the selected tab's pose on Reset.

Catalog icons are centered in a 40-pixel box, capped at their native size and
32 pixels to avoid blurry enlargement. Cart and contents retain their existing
32-pixel layout. Forge icons likewise never upscale their source artwork.
Promotional NEW/HOT/LIMITED overlays are no longer drawn. Their wire metadata
is retained for native compatibility and Hot-tab membership; this does not
change the normal category tabs or currency/price labels.

Contents windows drag from the title bar using exclusive pointer capture.
Clicking outside dismisses the modal and consumes that gesture, preventing the
release from selecting a catalog entry or interacting with the world beneath.
Clicks in the reward list do not initiate a drag.

### Machine launcher and forging room

Native `FUN_0024c9f4` maps the ten displayed machines to TSport kinds
21, 7, 8, 16, 19, 22, 10, 14, 9 and 6. The menu uses two pages, with eight
`Icon_Game777_*` thumbnails on the first page. These are single-image assets,
not three-state button strips. Opening a supported machine sends AC75:4:kind;
AC57:1 opens its existing instructions and game. Opening the menu or instructions
does not charge points. Diamond Cast, Diamond Hit, Fortune Egg 2, Potion Box,
Jigsaw and Fortune Egg use the existing server-owned arcade engines. Lucky Wheel,
Baby Dolls, Lucky Cards and Golden Glove still lack gameplay engines and show an
unavailable notice; they never launch an unrelated minigame or charge points.

Requesting entry leaves the mall open until the server starts the game. The
minigame manager temporarily hides and remembers it with the HUD, then restores
the same tab, page and cart on native AC57:2, including when Leave or Close cancels
the explanation screen. Games entered without an open mall do not open one on exit.

The forge screen uses `Form_ BeatScaffold_1`, the five-column inventory grid,
the selected furnace icon, point cost, grade, Confirm and Cancel. Covered cells
select their canonical item anchor. AC75:3:slot goes to the existing transactional
server handler after confirmation. Family upgrades consume Strong Scrolls;
other eligible equipment costs three IM Points per roll. Client-only family
metadata in `internal/clientassets/mall_forging.json` is generated by
`cmd/mall-previews` from the authored forging definitions. Server decisions still
come exclusively from the SQL catalog and authoritative transactions.

No item, scroll or currency changes optimistically. AC75:6 success applies point
progress; family upgrades rely on the inventory delivery preceding their receipt.
Pending attempts remain locked through timeouts until a reply or reconnect.
Malformed, wrong-slot and stale-item point receipts cannot modify replacement
equipment. Currency and tab changes are blocked during the pending attempt.

Thunder Spear (12101) uses weapon sprites 2708, 3708, 4708 and 5708. The native
lookup table selects patch names absent from the updated installation. Rendering
now falls back through earlier installed family archives when the base archive
also lacks the sprite. Raw compatibility tables and their independent lookup
goldens remain unchanged; this fallback applies generally to layered equipment.

### Expanded container catalog (2026-11-07)

The default catalog now has 251 rows (180 Points, 71 Bonus). Twenty-one new
single-container Points listings retain NEW metadata without displaying badges.
Prices are inferred from the closest existing container, as authorized by the
user; `internal/assetsql/mall_additions_evidence.json` records each comparison.
These are configurable prices, not claimed official prices. Both authored
`mall_defaults.json` and the repository SQL seed contain the new listings.

Added: EXP Pack and Water Pack (35 points); Fortune Pack (54); Potential Pack
and Chaos Pack (58); four Nostalgia bags, Speedy, Guard Spirit, Dark Halberd,
White Cheongsam, Black Fox Ears, Green Teddy, East Lance, Coldmist and Soul
Umbrella bags (32); Snow Love Box and Viking Box (132); Winter Cloth Bag (32).
The last has a prepared partial preview but is filtered out of the live mall
until its full pool is known; there are twenty newly purchasable containers.
Pack IDs 34381–34384 and 57937 remain excluded because the pack and/or reward
definitions are missing from the current item catalog. Do not invent definitions
or silently delete their missing rewards to make a pool purchasable.

The 58 tracked client preview lists now include the Chobits Ears containers.
Snow Love Box contains Snow Coat 21366, Snow Boots 24259 and Chobits Ears 22593.
Viking Box contains Viking Clothes 21364, Viking Boots 24257 and the same ears.
Their descriptions specify bundles: all three rewards are granted together,
with container removal and inventory delivery committed as one transaction.
Full inventory, missing definitions or failed persistence retain the container.

Winter Cloth Bag 56276 has three featured rewards: ears 22593, boot card 50707
and coat card 50706. Their reference probabilities are 0.7%, 0.2% and 0.3%.
The remaining practical props are unspecified; the preview explicitly says
"Partial contents". The SQL unavailable-pool entry prevents advertising, selling or consuming
this unresolved bag. WLOHub record 3926 identifies the ears, not their game ID.

`mall_pack_defaults.json` owns the twenty complete new/updated reward definitions;
`mall_pack_previous.json` preserves the nine exact old pools for migration
comparison. Random pools use the existing 10,000-slot engine. EXP/Water Pack
probabilities use the published nineteen-entry distribution from `var/slots/01.htm`,
matching multiplier/capsule effects and quantities. The modern Holy/ordinary
variants retain their current item identities. Other newly listed pools use
near-equal weights (differing by at most one 10,000th), as authorized. These
provisional distributions are not represented as recovered official odds.

Schema v60 replaces only exact old authored pool values and inserts missing
complete definitions, preserving edited/disabled pools and empty catalogs.
The separate exact v59 screenshot catalog remains tracked and independently
tested. Upgrading does not reset existing administrator prices or add missing
products to a customized/empty mall. Use the normal assets database migration;
normal startup never overwrites SQL definitions from JSON.

Schema v61 clears promotional flags on the original 230 authored listings.
The first tab remains **Hot**, containing the twenty newly enabled containers;
they also remain in Grocery. The prepared but unavailable Winter Cloth Bag is
filtered out as before. The flag-only migration preserves customized prices,
quantities, category assignments and ordering. Badge artwork remains hidden.

# Economy and social gameplay

## Database upgrade

Use the current versions in the database guide. Economy/social tables were
introduced in earlier versions; their introduction versions are not runtime requirements.
Use the preserving offline upgrade procedure in [ASSET_DATABASE.md](ASSET_DATABASE.md)
before starting an existing installation. Gameplay adds guild icons/ranks and
parcel escrow; assets adds typed `catalog_economy*` tables. Existing characters,
relationships and administrator content edits are retained. Repeat migration
preserves existing economy edits.

Economy definitions are available under **Economy** in Admin's definition editor.
Manufacturing recipes, gathering pools, marriage requirements/fees/rings and public
synthesis rates are stored in SQL. `internal/assetsql/economy_defaults.json` seeds
only the explicit offline migration or a new asset database. Editing that seed does
not change an existing installation. The seed projects the authored definitions
in Private Server's `TentManufactureManager`, `GatheringManager`,
`MarriageManager` and `AlchemyManager`; their item names are not assumed to match
WLRI. Missing item definitions reject operations before consuming resources.

## Player item trades

AC25 follows aLogin's direction-specific protocol. AC25:1 with a target ID opens
both trade forms immediately when both nearby players are available and allow
trades. The Private Server reference at `1ff5bba` also opens both forms immediately;
its older pending-request and incremental-offer aliases remain separate paths.
There is no pending invitation. World entry sends AC25:44 value 2 to enable the
native client's online-time flag; the server imposes no online-time delay.

AC25:2 locks one offer: gold as U32 followed by up to 20 distinct bag-slot bytes,
each offering a whole stack. It cannot be revised after locking. The partner
receives AC25:3 with gold and 30-byte item records, including damage and all
26 metadata bytes. Once both offers are locked, AC25:3 value 1 from each player
commits both inventories and balances in one SQL transaction against current
ownership and exact offered records. Failed validation or persistence transfers
nothing. Success publishes inventory/gold updates and AC25:2 result 4 after commit.

The reference still saves players separately and transfers received items by
ID/quantity, which recreates their instance state. Go preserves damage and metadata
and rejects offers whose exact items changed. It removes outgoing items from both
planned inventories before granting incoming items, permitting valid full-bag
swaps. The reference checks incoming capacity first and does not enforce its
offer-lock fields. These differences are why the existing Go exchange is retained.

AC25:3 value 2 cancels, notifying the partner with AC25:2 result 9. Disconnect
uses result 1; other server interruptions close both forms with results 3 and 9.
See [client controls and wire layouts](CLIENT.md#trade-ported-in-part-clientwloinventorytradego-clientwlohudbusinessgo-clientwloapptradego)
for the remaining client restrictions and presentation gaps.

## Player stalls

AC56:1 opens a named stall with bag slot, item ID, unit price and quantity per
listing. AC56:2 closes it; :3 views a nearby seller; :4 buys from a listed slot.
AC56:30 broadcasts signs and replays them to players arriving on the map.

Purchases commit both inventories and both gold balances in one transaction.
Offered records include damage and all item metadata. Stale items, insufficient
funds, carrying-limit overflow and full bags leave both parties unchanged.
Repeated purchases cannot oversell. An empty stall closes automatically.

Stalls last for the owner's online session. They close on travel, disconnect,
other inventory-changing actions and slash commands that change player state.
Ordinary chat and read-only synchronization can continue. Items are not removed
from the owner's inventory until a purchase commits. Sellers in battle or active
interactions cannot sell. A stall's distance limit matches player trading.

Native item stalls also accept AC23:30/31/32/33 for open, close, browse and
purchase. Browse packets contain up to five native rows; quoted item IDs and
prices must still match. Native AC23:30 opens with a title, style and five-byte
bag-slot/price records, offering the whole stack; six-byte quantity records remain
an explicit compatibility form. Native listings retain bag-slot keys and replay
map state/styles, with AC56:30 zero terminating the scene stall list.
AC23:77 retains its separate empty market list.
Persistent administrator bazaars and pet/vehicle stalls use typed SQL stock and
transactional purchases; see [the source update](SOURCE_UPDATES.md). Purchase
history remains pending.

## Guilds

`/guildcreate <name>` creates a guild with the caller as leader. Names contain
1–20 bytes. Guild invitations use AC39:2; an acceptance :3 requires the stored,
unexpired invitation from the same online, nearby inviter. Leaders and vice
leaders may invite. Membership is exclusive and persists in `wonderland.db`.

AC39:6 leaves; :7 dismisses a member; :9 edits the notice; :11 and :14 demote and
promote vice leaders; :16 sets the nonleader rank; :18 changes the insignia.
Leader-only operations verify database membership and permissions. Leaving or
deleting a leader elects the lowest-ID remaining member; empty guilds disband.
Admin edits preserve roles and refresh online rosters and badges.

AC39:1 requests the quest journal, including for characters without a guild.
Repeated requests replace prior native entries; this is separate from the guild UI.
AC39:12 requests the roster. Login/map entry restores the roster and badges;
online/offline roster status updates on connection changes. Native AC2:6 and
`/guild <message>` deliver guild chat across maps, respecting channel preferences
and mute status. AC39:8 accepts the reference guild message string. Notices have
at most 255 bytes and rosters at most 255 members. Persisted character jobs are used in roster presentation.

## Marriage

- `/marry <online character ID or name>` proposes to a nearby available player.
- `/acceptmarry` accepts; `/declinemarry` declines.
- `/divorce` ends the relationship; `/warptospouse` visits an available spouse.

Proposals expire after one minute and disappear on disconnect. Acceptance checks
both partners again, including level, existing relationships, fee and bag space.
The fee, both configured wedding rings and the relationship commit together.
A failed save or grant charges neither partner. Source defaults are level 30,
60,000 gold and rings 49001/49002. Spouse relationships remain SQL-authoritative,
including administrator annulments. Travel respects loading/battle/event gates.

## Parcel mailbox

- `/mail`: show the inbox with native AC23:76 records.
- `/readmail <ID>`: mark an owned letter read and send its AC23:77 body.
- `/claimmail <ID>`: claim an owned letter's attachments once.
- `/deletemail <ID>`: delete an owned letter after claiming its attachments.
- `/sendmail <recipient ID> <gold> <bag slot or 0> <count or 0> <subject> | <body>`.

Example: `/sendmail 10002 100 3 2 Supplies | Here are two items.`

Sending atomically debits gold and the exact owned stack into persistent escrow.
Receiving preserves item damage and metadata. Full bags or carrying-limit overflow
leave attachments available for a later claim. Repeated/concurrent claims do not
duplicate rewards. Subjects allow 100 bytes, bodies 255 bytes and inboxes 255
letters. Deleting the sender retains an addressed parcel; deleting the recipient
cascades their inbox. Unclaimed attachments must be claimed before normal deletion.

Native AC14 text mail and GM gift delivery retain their existing behavior. The
reference documents mailbox response layouts, but supplies no verified native
parcel request dispatcher. The public commands expose mailbox management until
native-client requests are captured.

## Manufacturing, synthesis and gathering

`/manufacture <workbench> <item1> <count1> <item2> <count2>` uses the configured
recipe order. Workbench names may contain spaces. Use `0 0` for an absent second
ingredient. Native AC59 preserves the reference's Forge workbench and reply.
Fees, ingredients and output commit in one authoritative SQL transaction; a full
bag on a successful attempt or missing material consumes nothing. An authored
failed attempt consumes ingredients and its fee without granting output. Repeated input IDs consume the combined recipe cost. These
handlers follow the reference's workbench-name selection. Tent furniture now
lives in isolated homes. These two-input paths retain workbench-name matching;
native AC64 separately checks placed owned benches, tools and plans, with durable
timed output. See [manufacturing](MANUFACTURING_CHARACTER_STATE.md).

`/compound <slot1> <slot2> [slot3] [slot4] [slot5]` (alias `/synthesize`)
shares the rank/base engine with native AC23 and compatibility AC40. See
[COMPOUNDING.md](COMPOUNDING.md) for books, skill levels, rank ceilings, secondary
bases and catastrophic failures. Resources and proficiency commit together in
SQL. Historical `AlchemyRecipes` and `Economy.Synthesis` chances/fees remain
preserved but no longer control gameplay synthesis.

`/mine` and `/chop` retain SQL gathering pools and their configured interval.
Movement, travel, battle, active interactions, disconnect and full bags stop
these gathering runs. `/fish` uses the rod/shoreline/proficiency system described
in [FISHING.md](FISHING.md); `/stop` stops either activity. Fishing retains
proficiency when a full bag discards a catch. No incompatible short AC5 animation
packets are emitted.

## NPC vendor purchases

EVE shop services open the native AC35:12 catalog. AC27:1 accepts native carts,
resolves slots through the current map catalog (or configured SQL defaults), and
commits the complete delivery with the gold debit. An arbitrary item ID outside
the active catalog cannot be bought. Prices are in `NPCBuyPrices`; existing sale
prices remain independent. Native sell choices retain their AC27 sale window.

## EVE gathering nodes

`EventGathering` supplies all imported EVE pool/variant and timer definitions.
Complete branches may grant multiple pooled rewards followed by a three-minute
node timer and optional mark. SQL commits rewards, timer and mark together after
rechecking the branch. The pools use upstream equal weights. Ordinary continuous
`/mine`, `/chop` and fishing pools remain separate definitions.

## Shared resource nodes

Unlinked static resource props now use SQL reward pools with atomic inventory
and shared cooldown updates. Other players see their broken/respawned frames;
full inventories do not consume the node. Cooldowns persist across reconnects
and restarts. See [WORLD_SIMULATION.md](WORLD_SIMULATION.md#shared-resource-props).
Legacy SQLite imports also preserve supported friendships and chat preferences;
see [LEGACY_IMPORT.md](LEGACY_IMPORT.md).

## Remaining native work

Arcade scores, ticket prices/rewards, bank PIN authentication/transfers, ATM
activation, guild vault operations and alliance chat require rules absent from
the reference or verified native-client requests. Their success-only C# stubs do
not prove a transaction exists. They remain pending and do not return invented
successes. Tent placement, movement and recovery are implemented; see
[ITEMS_PLAYER_STATE.md](ITEMS_PLAYER_STATE.md). Tent upgrades, decoration
purchases remain pending. AC64 checks owned bench/tool identities; the older
two-input recipe path retains its source workbench-name matching.
Live aLogin acceptance remains pending
for the newly ported packets.

## Implemented backends versus native requests

Mall checkout/forging, native AC21 slot purchases and AC71 paid arcade purchases
have SQL transaction paths. AC13:238 and AC226:255 compatibility replies are
implemented; see [native commands](NATIVE_COMMANDS.md). Native AC59 retains
its two-input Forge layout. AC64 implements the original formula IDs, five
materials, owned plans/tools, timed start/continue/stop and bag/tent output.
Authored synthesis chances and fees are projected into SQL during conversion.
See [manufacturing and character state](MANUFACTURING_CHARACTER_STATE.md) for
recovery, defaults and the deliberately pending AC37 socket effects.

`/stopgather` and `/inbox` source aliases are absent; use `/stop` and `/mail`.
AC87 bath recovery, native AC23 fishing and AC90 compatibility are implemented.
Fishing uses authored SQL defaults for unresolved weights and progression;
see [FISHING.md](FISHING.md). Friend lists include saved class/nickname and guild lists include class. Source Cupid is excluded from transmitted friend lists;
absence of a visible contact alone does not establish a wire gap.

The source player-facing HTTP mall page and form-urlencoded registration routes
are not compatible with Go's current API. Do not restore unauthenticated purchases
by supplied username. Any future player HTTP checkout must authenticate ownership
and use the same SQL transaction guarantees as native purchases.

Purchases, fees, transfers, rewards and parcel claims stay immediately durable.
Buffered walking and resting recovery preserve pending position and HP/SP
when adopting a committed SQL result that leaves those saved fields unchanged.
Guarded checkpoints must not undo balances, attachments or draw usage.
See DEVELOPMENT.md for the persistence boundary and remaining concurrency limits.

## Retained compatibility rules

AC45 retains the reference default: unknown bank subcommands return the private
AC45:8 bank/wallet snapshot, ignoring any payload and changing no balances. PIN
and transfer operations continue to report unavailable; no success-only source
stub is treated as a completed transaction. Existing mutation/loading/trade gates
still apply. Tests cover all default subcommands, private delivery, rollback,
overflow and reopen.

Recipe lookup retains authored entries before Compound2 then Compound, including
first-match priority when inputs have multiple outputs. Typed SQL reload preserves
this order independently of import snapshots. Manufacturing keeps first matching
workbench order. See [automated coverage](TESTING.md#automated-coverage).

## Persistent administrator bazaars

Administrators can create vendors at their current ordinary-world position:

```
/bazaar create item finite 32176:100:50 Traveling supplies
/bazaar create pet infinite 17162:10000:1 Companions
/bazaar create vehicle finite 48010:5000:3 Transport
/bazaar list
/bazaar remove <vendor ID>
```

Each entry is `definition ID:unit price:stock`. Item, pet and vehicle/appliance
stalls have native capacities of five, fifteen and three rows. Definitions must
exist; no fictional fallback catalog is created. Vendor IDs start above the
player-character range. Vendor identity, stock and settings persist in gameplay
SQL and replay on map entry. Source operator-owned example stalls are excluded
from initialization.

Native AC23:32 browses a vendor. Item carts use AC23:33; pet and vehicle purchases
use AC23:63 and :68, with their separate :113/:133 browse layouts. Browsing saves
a quote; purchases require proximity and revalidate the entire cart, SQL funds,
finite stock, bag capacity or pet eligibility before committing. Stock, payment
and delivery commit together. Pets retain the normal ownership/capacity and
companion-alias checks. Limited-stock sales survive restarts.

Personal item stalls retain AC56 and native AC23. Alternative AC53 and AC20
adapters share those transactions; automatic opening lists actual unlocked owned
items and never invents goods for an empty bag. Native item quotes include the
advertised ID and price. Walking closes personal stalls and invalidates NPC shop
sessions. Administrative vendor creation/removal requires GM authority.

# Economy and social gameplay

## Database upgrade

Current server startup requires gameplay schema **v11** and structured assets
schema **v6**. Economy/social tables were introduced in earlier versions.
Use the preserving offline copy procedure in [ASSET_DATABASE.md](ASSET_DATABASE.md)
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

The separate native market browser (AC23:77) keeps its verified empty-list/end
sequence. Private Server supplies no nonempty catalog layout; use map signs and
AC56 instead. Purchase-history requests remain unimplemented.

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

AC39:12 requests the roster. Login/map entry restores the roster and badges;
online/offline roster status updates on connection changes. Native AC2:6 and
`/guild <message>` deliver guild chat across maps, respecting channel preferences
and mute status. AC39:8 accepts the reference guild message string. Notices have
at most 255 bytes and rosters at most 255 members. Job presentation uses the
existing model's no-job marker until rebirth/job presentation is ported.

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
Costs and output are planned before saving; a full bag or missing material
consumes nothing. Repeated input IDs consume the combined recipe cost. These
handlers follow the reference's workbench-name selection. Tent furniture now
lives in isolated homes; linking manufacturing recipes to verified physical
workbench item identities remains pending.

`/compound <bag slot1> <bag slot2>` (alias `/synthesize`) uses source chance-based
synthesis. Recipe results come from SQL `AlchemyRecipes`; chances and the failure
item come from `Economy.Synthesis`. Authored source rates override the default
85% native-recipe rate by matching both inputs and output. A failed attempt grants
the configured charcoal item. Both ingredient removals and the output commit
together. Native AC23/AC40 retain their verified deterministic behavior.
`AlchemyRecipes` and `Economy` can both be edited through Admin.

`/fish`, `/mine` and `/chop` start source gathering; `/stop` stops it. SQL pools
set the reward IDs and interval (source default: five seconds). Each maintenance
tick grants at most one reward per session, with no catch-up burst. Movement,
travel, battle, active interactions, disconnect, unavailable definitions or a
full bag stop gathering. Rewards persist before packets are sent. Source fishing
animations remain pending: its short AC5:12 packet conflicts with the verified
native model-transform payload, so the server sends status/reward feedback only.

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
purchases and verified manufacturing workbench ownership remain pending.
Live aLogin acceptance remains pending
for the newly ported packets.

## Implemented backends versus native requests

Mall checkout/forging and AC71 paid arcade purchases have SQL transaction paths,
but native AC21 and AC226:255 compatibility replies are absent. Direct AC37
requests are not covered by mall forging. Direct AC59 handling differs from the
chat recipe path; its protocol registration/layout needs completion. Native AC64
requires bench/formula IDs, up to five materials, plans/tools, build duration,
continue/stop and bag/tent output. The current two-input operation is not a full
native manufacturing port. Compound2 extraction's two-input projection does not
represent all those source fields; source recipe fee/chance preservation also
needs verification. Validated SQL AlchemyRecipes editing is already available.

`/stopgather` and `/inbox` source aliases are absent; use `/stop` and `/mail`.
AC90 fishing toggle/reel-in and AC87 bath recovery remain missing even though
other gathering/healing backends exist. Job/nickname fields in friend/guild
presentation are empty. Source Cupid is excluded from transmitted friend lists;
absence of a visible contact alone does not establish a wire gap.

The source player-facing HTTP mall page and form-urlencoded registration routes
are not compatible with Go's current API. Do not restore unauthenticated purchases
by supplied username. Any future player HTTP checkout must authenticate ownership
and use the same SQL transaction guarantees as native purchases.

Purchases, fees, transfers, rewards and parcel claims stay immediately durable.
Buffered walking preserves pending position when adopting a committed SQL
result. Guarded checkpoints must not undo balances, attachments or draw usage.
See DEVELOPMENT.md for the persistence boundary and remaining concurrency limits.

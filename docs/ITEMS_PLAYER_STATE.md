# Items and Player State

## Initialization

Current server startup requires gameplay schema **v18** and structured assets
schema **v15**. Tents were introduced in earlier schema versions.
Follow the preserving upgrade procedure in [ASSET_DATABASE.md](ASSET_DATABASE.md)
before deploying against older databases. The upgrade replaces the configured
databases in place; the originals remain in `backups/` for rollback.

`assets.db` owns the `Tents` dataset: interior spawn coordinates, initial floor
and wallpaper references, and ordered default furniture. Offline initialization
seeds `internal/assetsql/tent_defaults.json`, traced to the recorded C# Tent.cs.
Administrator content edits use the existing typed dataset editor. Existing
homes retain their saved contents and decoration references when defaults change.

`wonderland.db` owns `tents` and `tent_items`, keyed by character identity.
Furniture retains item identity, count, damage and all 26 metadata bytes. Typed
character state stores each visitor's overworld return map and coordinates.
Deletion cascades through owned home/furniture records.

## Using a tent

Use Tent item 36002 with native AC23:96, or AC23:15 with count one and target zero.
Opening displays the native sign and reserves that bag slot. Native AC65:1 enters
an open home by owner character ID, :2 packs up the caller's home, and :3 exits.
The client sees native map 63507; server scene identity also includes its owner.
Different owners' homes cannot share local player visibility or interactions.

The owner can place furniture with native AC62:1 (`slot:u8`, X/Y/Z UInt32,
optional floor byte), pick it up with AC62:2 and move/rotate it with AC62:3.
Native indices are one-based in the ordered root list; zero addresses the first
root for compatibility. X/Y are tent-grid coordinates; Z is not a floor selector.
The older zero-prefixed placement adapter remains available. Bounds and rotated
footprints prevent overlapping native placements. An existing upgraded tent
(type 2+) can place on floor one; new house upgrades remain pending.

AC62:4 attaches an owned appliance to a configured host. The Work Platform
(38049) accepts up to ten distinct tools, retaining each full item instance.
Native AC62:4 tree snapshots include root/child indices and are sent after map
acknowledgment, before releasing the scene gate. Pickup refunds the host and all
its tools together; a full bag leaves the whole tree intact. SQL owns furniture,
tool attachments, surfaces and access policy.

New tents use Resource Recycling (38055) at (34,42) and the Work Platform at
(37,42). Migration updates only the exact previous shared defaults; owned tents
and edited defaults retain their content.

Public chat commands provide workflows whose native requests are unimplemented
in the reference:

- `/tentlock` and `/tentunlock`: persist access policy for the caller's home.
  Locked homes admit their owner. Changing the lock does not evict current guests.
- `/tentpickup <index>`: recover the zero-based furniture record into the bag
  while inside the caller's home.
- `/tentexit`: return to the visitor's saved overworld position.
- `/tentclose`: pack up the caller's open home and return its occupants.

Closing, owner logout and travel away from the opening map pack up the home,
release the reserved tent item and return visitors, including loading sessions.
Visitors return to their own positions. Disconnect/reconnect and crash recovery
restore durable return coordinates instead of recreating a process-owned opening.
Homes run no overworld events, encounters or PK. Public-map collision and NPC
movement are implemented separately; see [WORLD_SIMULATION.md](WORLD_SIMULATION.md).
The reference defines no additional functioning instance type.

## Item acquisition capacity

Every item acquisition must fit its complete quantity before success. Use
`Inventory.Grant`/`Add` for delivery and `ApplyQuestItems` for ordered hand-ins
and rewards. These plan on a copy and return `ErrInventoryFull` without partial
items or additions when capacity is insufficient. Capacity includes compatible,
unlocked stack space and contiguous unlocked rectangles in a 5 × 10 grid; differing damage or metadata
cannot share a stack. Dimensions come from typed SQL item definitions
(`cell_width`/`cell_height`); both raft variants occupy 4 × 3 cells. Only the
anchor stores an item record. Covered cells cannot receive another item, and
placement cannot wrap across rows or extend below the bag. A bag with no empty slots may still accept a reward that
fits entirely in an existing compatible stack.

Failure must preserve payments, consumed packs/materials, claim availability,
cooldowns and quest completion. Commit delivery and those associated changes
in the same gameplay transaction before success packets. Quest hand-ins and
pack consumption may free slots within the planned operation. Ground pickups
leave the item available when delivery fails. Individual combat drops that
cannot fit are not acquired; battle EXP and gold remain independent rewards.

Every runtime call to `Grant`, `Add`, `Move`, `Transfer`, `Compound`,
`ApplyQuestItems`, `GrantQuestReward` and `Unwear` must pass the SQL-derived item
catalog. Omitting definitions preserves one-cell synthetic test fixtures only.
Footprints also apply to storage, equipment swaps, forging replacements and
administrative inventory edits. Moving a whole item may overlap its previous
footprint; splitting a stack cannot overlap the retained source.

Asset schema v11 seeds dimension columns from retained decrypted SQL record
bytes during the explicit offline upgrade. Runtime does not decode records or
read JSON to obtain dimensions. Later SQL edits survive restart. No gameplay
schema change or automatic inventory repacking is performed. Existing overlapping
or out-of-bounds layouts are rejected by placement checks; fix their anchors
through an explicit inventory edit rather than silently removing items.

Regression coverage includes fragmented space, grid boundaries, partial capacity,
reservations, metadata, multi-cell moves/transfers, whole quest rewards, draw
rollback, native ground pickup retry and preserving SQL migration.

## Resting recovery

Sitting or lying down heals slowly. This is a server rule of this port; neither
aLogin nor the reference server defines one. A ready character holding a
sitting pose (AC32:2 actions 46–53, the Emote grid's action 13) or lying pose
(actions 26–27, grid action 4) outside a battle or event recovers 2% of their
equipped maximum HP and SP, at least one point each, every three seconds
(`restRecoveryPercent`, `restRecoveryInterval` in `internal/server/simulation.go`),
on the one-second world ticker. Each recovery sends AC8 absolute updates for
current HP (25) and SP (26) only. Native clients end a held pose by walking
without an AC32 report, so a position change clears the server's held pose;
this also lets a player sit again after walking.

Resting HP/SP are buffered session fields like walking: they update memory and
mark the session dirty. The checkpoint writes them with X/Y in the guarded
state-row update (matching the checkpointed X, Y, HP and SP), and adopting a
committed SQL result keeps buffered recovery unless that transaction changed
the vital, capped to the adopted maximum. Under healthy saves, a crash can lose
up to one checkpoint interval of recovery; failed saves can extend that window.
Disconnect and graceful shutdown attempt a final checkpoint.

## Item use and reservations

Native item use supports the reference's equipment, recovery, pet voucher, gacha,
quest-token information and tent workflows. Targeted gacha consumes one pack only
for the character target; invalid quantities/targets and unavailable pools retain
items. Unsupported special effects remain unavailable pending verified rules.

`game.Item.Locked` is a transient reservation. It is omitted from JSON, wire item
metadata, typed SQL records and autosave comparisons. Inventory removal/movement,
equipment swaps, repairs, crafting and vehicle selection respect it. Grants skip
reserved stacks and slots. SQL-backed trades, stalls and reward transactions
receive live reservation snapshots before planning their mutation. Restart does
not restore process locks.

The source defines `Item.isLocked` but exposes no working native lock-toggle
protocol. Opening a tent is currently the concrete reservation owner; future
operations should acquire/release reservations explicitly and keep them in memory.
Persist the gameplay result required for recovery, never the process lock.

## Verification and unresolved reference behavior

Coverage is centralized in [inventory and recovery checks](TESTING.md#inventory-and-recovery-checks). Live aLogin tent
and furniture synchronization acceptance remains pending.

AC60 decoration purchases and AC61 upstairs operations remain empty or
success-only source handlers. Their costs and unlock rules remain pending. Wallpaper and floor application now consume the selected item with the
metadata update in one transaction. Arriving occupants receive the saved surfaces;
house upgrades remain unresolved. AC59 and `/manufacture` retain workbench-name recipe matching;
native AC64 validates placed owned benches, bag tools and plans. See
[manufacturing](MANUFACTURING_CHARACTER_STATE.md#native-manufacturing).

## Remaining state and native compatibility

Character title, nickname, actual class, potential and separate AC66 metadata
persist in typed SQL, along with pet potential. GM rebirth, all six class stat
modifiers and normalized rebirth progression are implemented. AC23:126 consumes
normal, golden and Super Potential Pills transactionally for players and owned
party pets. Failure/bonus rules are in [Potential Pills](NATIVE_COMMANDS.md#potential-pills).
AC66 changes separate metadata without granting advancement.

Home Locked/Enlarged/Type and both floor/wallpaper pairs persist. Native furniture
placement accepts floor one for existing tents of type two or greater; the legacy
placement adapter uses floor zero. Upgrade purchases and upstairs access remain
unresolved. Occupant poses replay through AC32:2. Configured
starters are granted and persisted at creation, without automatic login
redelivery. Admin offers explicit mass online delivery.
See [world/combat parity](WORLD_COMBAT_PARITY.md).
Focused verification covers all stack/drop type values and reachable wear guards.
The source Tradeable property and use-type metadata have no restriction callers
in trade/wear, so they do not impose additional rules. Unverified special item
effects remain on hold. AC8 allocation rejects invalid/overflowing entries without
spending points; pet overflow matches the reference guard, and character overflow
is deliberately rejected too. See [verification](TESTING.md#migration-verification-recorded-on-2026-10-04).

Routine walking follows the buffered session policy in DEVELOPMENT.md.
Furniture transfers, purchases, claims, item wear/consumption and inventory
mutations retain immediate SQL transactions.


## Boxes, saved travel points and companion altar

`BoxedItems` contains deterministic box contents. Opening exchanges one owned box
for its item in SQL before sending receipts. Missing definitions, locked boxes and
failed delivery retain the box. Weighted gacha packs remain separate.

AC74 records up to three Teleport Scroll points according to the scroll definition.
Owned saved points persist in gameplay SQL and replay on login. AC23:89 uses the
40 imported Bamboo Dragonfly destinations. Travel commits consumption and the
recovery destination together. These requests currently require an ordinary world
scene; unavailable destinations do not consume items.

Native altar service 27 opens AC44:2; AC44:1 selects a client pet slot. It requires
level 100, amity 60, no previous rebirth and no equipment. The transaction preserves
allocated points, skills and amity and resets level/EXP. It grants no bonus points;
AC69 ascension retains its separate amity-100/50-point policy.

## Manufacturing and extended character state

Native AC64 now persists timed output escrow and checks owned plans/tools and all
five materials. Character nickname/class/potential, pet potential and both tent
decoration pairs live in typed gameplay v14 rows. GM rebirth, class bonuses and
tent peer pose replay are implemented. See [manufacturing and character
state](MANUFACTURING_CHARACTER_STATE.md) for packet layouts, SQL upgrade steps,
recovery, funded potential training and the pending socket/upstairs rules.

## Utility item use

Notepad 34038 opens the SQL-backed quest journal. Remote Control 34058 opens its
native control form. Kelan Scroll 30001 consumes one item with its SQL-defined
return destination in the same transaction. These use real WLRI definitions;
legacy Chocolate/White Rice IDs are not reinterpreted as utility items. Short
AC23:15 self-use requests and normal AC23:96 share the same dispatcher. Owned
vehicles use the existing validated mount/dismount workflow.

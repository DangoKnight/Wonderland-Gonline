# Items and Player State

## Initialization

Current server startup requires gameplay schema **v15** and structured assets
schema **v11**. Tents were introduced in earlier schema versions.
Follow the preserving copy procedure in [ASSET_DATABASE.md](ASSET_DATABASE.md)
before deploying against older databases. Upgrade copies, then select their
paths in the startup configuration. Source databases remain available for rollback.

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

The owner may place furniture, workbenches and decorations with AC62:1 and move
or rotate them with AC62:3. Only floor zero is implemented. Movement uses the
zero-based index in the current ordered furniture list, including after pickup.
Only the owner can edit. Bag debit and insertion commit together; recovery leaves
furniture intact if the bag is full. Metadata is preserved in both directions.

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

Tests cover native envelopes, independent scenes, admission/ownership, complete
metadata, stale/full-bag rollback, ordinal indices, loading/logout/crash recovery,
reservations, and preserving gameplay v9 and asset v2 upgrades. Live aLogin tent
and furniture synchronization acceptance remains pending.

AC60 decoration purchases, AC61 upstairs operations and AC62:4 special placement
are empty or success-only source handlers. Their costs, unlock and transaction
rules remain pending; they are not implemented as false successes. Initial floor
and wallpaper references persist, but native decoration editing and house upgrades
remain unresolved. Manufacturing currently retains reference workbench-name
selection; binding recipes to verified physical furniture identities remains pending.

## Remaining state and native compatibility

Character title, nickname, actual class, potential and separate AC66 metadata
persist in typed SQL, along with pet potential. GM rebirth, all six class stat
modifiers and normalized rebirth progression are implemented. Funded potential
training remains pending; AC66 changes its metadata without granting advancement.

Home Locked/Enlarged/Type and both floor/wallpaper pairs persist. Furniture
placement stays floor zero; occupant poses replay through AC32:2. Configured
starters are granted and persisted at creation, without automatic login
redelivery. Admin offers explicit mass online delivery.
See [world/combat parity](WORLD_COMBAT_PARITY.md).
Focused verification covers all stack/drop type values and reachable wear guards.
The source Tradeable property and use-type metadata have no restriction callers
in trade/wear, so they do not impose additional rules. Unverified special item
effects remain on hold. AC8 allocation rejects invalid/overflowing entries without
spending points; pet overflow matches the reference guard, and character overflow
is deliberately rejected too. See [verification](PORTING.md#focused-migration-verification--2026-10-04).

Routine walking follows the buffered session policy in DEVELOPMENT.md.
Furniture transfers, purchases, claims, item wear/consumption and inventory
mutations retain immediate SQL transactions.


## Manufacturing and extended character state

Native AC64 now persists timed output escrow and checks owned plans/tools and all
five materials. Character nickname/class/potential, pet potential and both tent
decoration pairs live in typed gameplay v14 rows. GM rebirth, class bonuses and
tent peer pose replay are implemented. See [manufacturing and character
state](MANUFACTURING_CHARACTER_STATE.md) for packet layouts, SQL upgrade steps,
recovery and explicitly pending socket/training/upstairs rules.

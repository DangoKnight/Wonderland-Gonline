# Items and Player State

## Initialization

This port uses gameplay schema **v10** and structured assets schema **v3**.
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
Homes run no overworld events, encounters or PK. Other instance types, geometry
constraints and NPC movement remain separate World work.

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

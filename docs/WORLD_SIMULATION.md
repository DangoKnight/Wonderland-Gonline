# World Simulation

## Implemented scope

`internal/world/simulation.go` ports `QuestNpc.Update` from Private Server:

- Behavior 1 and static props stay at their authored spawn.
- Behaviors 2/5 follow EVE waypoints. One waypoint alternates with the spawn;
  multiple waypoints loop in order. Authored delays have a two-second minimum.
- Behavior 3 wanders within signed EVE offsets, bounded to ±300 pixels.
- Behavior 4 and eligible wild monsters roam field maps with a 60-pixel leash.
  The native town classification is separate from random encounter safety.
- Initial movement waits 1–8 seconds. Ambient movement uses native AC22:2
  `[22,2,click:uint16,x:uint16,y:uint16,speed:byte]`, with walking speed 2.
- Defeated monsters stay still and respawn at their authored position, restarting
  their patrol after three seconds. Existing ground-item/chest ticks remain active.

The server advances occupied, acknowledged public maps on the existing
one-second process ticker. Events and battles reserve their actors. Packets go
only to viewers who can see the actor and do not own it as a story companion.
Private tent occupants and players on other maps receive no public-map movement.
Failed recipients do not stop other viewers' updates.

Map-entry snapshots, actor show packets, NPC click reach and proximity encounters
all use current simulated positions. Original EVE spawns remain immutable.
Positions, patrol cursors and deadlines stay in memory and reset after restart;
ordinary character walking stays in session memory, then checkpoints to
`wonderland.db` at the configured interval and disconnect. See the
[checkpoint policy](DEVELOPMENT.md#transaction-boundaries-and-session-checkpoints).
Purchases, rewards, vehicle wear and map transitions remain immediate. NPC
simulation and gathering keep their independent one-second world cadence.

Private Server's `MapType` defines only RegularMap and Tent. Go already supports
owner-isolated tents and their saved return points. There is no additional
implemented instance type in that reference to invent for this port. Shared
static resource props now use durable cooldowns as described below.


## Shared resource props

Unlinked static props use the existing SQL chest-pool categories and map overrides
from Private Server's `QuestNpc.Interact` fallback. Authored EVE scripts and NPC
services take precedence. A claim grants the complete weighted reward and saves
the shared cooldown in `wonderland.db.map_props` in one transaction. A full bag
or failed write leaves the node available.

Visible public-map viewers receive broken AC22:1 frames after commit; hidden
actors, foreign maps and tents are excluded. Entry and event completion replay
active broken frames. The existing one-second ticker expires cooldowns and
broadcasts intact frames after the database update. Reconnects and process
restarts retain cooldowns. Per-character scripted chests retain their existing
separate quest/cooldown behavior. Adjust reward pools and respawn seconds through
the existing ChestPools administration dataset.

## Scene constraints

`Catalog.Terrains` contains scene dimensions and raw collision grids from WLRI
Ground.MMG, read exclusively from typed `catalog_terrains` rows in `assets.db`.
The cells are X-major, spaced 20 pixels apart; the land blocked mask is `0x17`,
as in the native client pathfinder. No pictures are imported into the server.

Player walking validates the destination and the straight cell segment from the
last accepted position. Rejected requests receive the saved AC7 position and do
not change the database, trade, encounter counters or vehicle wear. An invalid
saved spawn may recover to valid terrain. Native clients announce whole walking
legs in advance, so movement is not limited by packet-time speed estimates.

Validated active item vehicles retain water/air movement within scene bounds.
The land mask cannot describe their traversal capabilities. Actors use ordinary
land constraints; blocked patrol legs are skipped. Maps absent from the terrain
catalog retain legacy unrestricted movement. There is no runtime file fallback.

## Upgrade and tuning

The rebuilt server requires asset schema **v6**. Stop the server, then follow
[ASSET_DATABASE.md](ASSET_DATABASE.md) to create verified database copies:

```sh
go run ./cmd/database-migrate -config config.local.json -output-dir var/migrated-v6
```

Point `assets_database` and `database` at the verified copies before restarting.
The migration preserves accounts, character state and typed content edits. It
projects existing imported Ground.MMG rows once; repeated migration preserves
terrain edits. A complete installed export yields **1,147 terrain grids**.

Adjust authored patrols in the `Maps` administration dataset (`WalkBehavior`,
`WalkSteps`) and collision geometry in `Terrains`, then restart or use the existing
catalog reload. The compatibility limits and scheduling ranges are compiled
constants in `internal/world/simulation.go`; native collision constants are in
`internal/assets/terrain.go`. No new startup JSON knobs are required.

## Verification

```sh
go test -race ./internal/world ./internal/assetsql
go test -race ./internal/server -run 'TestWorldSimulation|TestWildMonster|TestProximity|TestWorldVisibility|TestTent|TestVehicle'
WONDERLAND_TEST_ASSETS_DB=/absolute/path/to/migrated/assets.db go test ./internal/assetsql -run TestInstalledSQLWorldSimulationGeometry
```

Coverage includes golden movement bytes, current-position snapshots, single
waypoint return, looping patrols, blocked patrol legs, signed bounds, roaming
leashes, town exclusions, static props, script pauses, monster respawn,
collision rejection before saving, visibility/instance isolation, recipient
failure isolation and SQL upgrade preservation. Installed-grid verification
checks the terrain census and Ship Deck geometry. Native-client and multiplayer
acceptance are still required; synthetic tests do not establish that acceptance.

## Remaining reference differences

Scripted EVE actor prop-state/hide/show actions change the requesting view; they
do not perform the reference map-wide broken-state transition and 60-second reset.
The SQL fallback resource-node path above is separate and does not close this gap.
Unlinked nonservice NPCs release interaction without the reference resolved/default
Kelan greeting. Clinic offer/confirmation heals the character, not accompanying
pets. Native waypoint/emote and some synchronization requests remain absent; see
PORTING.md. Scene geometry/simulation tests do not establish those command paths.

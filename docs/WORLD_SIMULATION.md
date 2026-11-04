# World Simulation

## Implemented scope

`internal/world/simulation.go` ports `QuestNpc.Update` from Private Server:

- Behavior 1 and static props stay at their authored spawn.
- Behaviors 2/5 follow EVE waypoints. One waypoint alternates with the spawn;
  multiple waypoints loop in order. All stationary waits use the common 1–3 seconds.
- Behavior 3 wanders within signed EVE offsets, bounded to ±300 pixels.
- Behavior 4 and eligible wild monsters roam field maps with a 150-pixel leash.
  The native town classification is separate from random encounter safety.
- Behaviors 2–5 begin movement after 1–3 seconds and wait 1–3 seconds after
  estimated arrival before choosing another route. This replaces authored
  waypoint delays and the previous 5–10-second random roaming schedule.
- Hostile actors search every three seconds when no eligible player is retained,
  including while wandering or patrolling. Acquiring a player can interrupt
  those routes or their stationary waits. A valid player target and active
  pursuit leg remain selected; losing a target restarts the search interval.
  Bounded/random roamers restrict acquisition and pursuit to fixed spawn areas.
- NPC snapshots interpolate the estimated current position at the native
  speed of 40 pixels per second. Interruptions start from that position.
- Ambient movement uses native AC22:2
  `[22,2,click:uint16,x:uint16,y:uint16,speed:byte]`, with walking speed 2.
- Defeated monsters stay still and respawn at their authored position, restarting
  their patrol after three seconds. Existing ground-item/chest ticks remain active.

The server advances occupied, acknowledged public maps on the existing
one-second process ticker. Events and battles reserve their actors. Packets go
only to viewers who can see the actor and do not own it as a story companion.
Private tent occupants and players on other maps receive no public-map movement.
Failed recipients do not stop other viewers' updates.

Map-entry snapshots, actor show packets, NPC click reach and proximity encounters
use interpolated current actor positions. Arrival gating uses native speed byte
2 (40 pixels/second); interrupted paths start from the estimated current position.
Original EVE spawns remain immutable. The Go client animates these destinations
with the same speed mapping; see [client roaming](CLIENT_ROAMING.md).
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

Native ground items and scripted collection nodes use shared durable expiries in
`wonderland.db.resource_cooldowns`. Each map/source has an independent timer and
can have a different cooldown duration through the typed `ResourceRespawns`
asset definition. Ground-item availability is restored before publishing map
entry; the world ticker broadcasts respawns to current viewers. Claims commit
inventory rewards and expiry together. Player-dropped items do not respawn.
See [collection cooldowns](COLLECTION_SPOTS.md#shared-cooldowns-and-individual-durations).

## Scene constraints

`Catalog.Terrains` contains scene dimensions and raw collision grids from WLRI
Ground.MMG, read exclusively from typed `catalog_terrains` rows in `assets.db`.
The cells are X-major, spaced 20 pixels apart; the land blocked mask is `0x17`,
as in the native client pathfinder. No pictures are imported into the server.

Native 15-byte AC6 requests validate the destination cell and scene bounds. The
previous announced destination is not the avatar's current rendered position,
so the server does not reject a native route merely because the straight line
between announced endpoints crosses an obstacle. Invalid native endpoints are
ignored without AC7 snapping. Short compatibility requests retain segment checks
and saved-position corrections. Rejection precedes session changes, trade
cancellation, encounters and vehicle wear. A blocked saved spawn may recover to
valid terrain; movement has no packet-time speed cap. Native AC6:2 stop reports
update the stopped endpoint without another wear/encounter step; the eight
trailing check bytes remain opaque. See [native movement](NATIVE_COMMANDS.md).

Validated active item vehicles retain water/air movement within scene bounds.
The land mask cannot describe their traversal capabilities. Actors use ordinary
land constraints; blocked patrol legs are skipped. Maps absent from the terrain
catalog retain legacy unrestricted movement. There is no runtime file fallback.

## Upgrade and tuning

The current server requires asset schema **v16**; terrain tables were introduced
in v5. Stop the server, then follow
[ASSET_DATABASE.md](ASSET_DATABASE.md) to upgrade the databases in place:

```sh
go run ./cmd/database-migrate -config config.local.json
```

The migration preserves accounts, character state and typed content edits. It
projects existing imported Ground.MMG rows once; repeated migration preserves
terrain edits. A complete installed export yields **1,147 terrain grids**.

Adjust authored patrols in the `Maps` administration dataset (`WalkBehavior`,
`WalkSteps`) and collision geometry in `Terrains`, then restart or publish through
the validated administration flow. `/reload quests` preserves existing spawn
geometry and does not serve as a terrain/patrol reload. The compatibility limits and scheduling ranges are compiled
constants in `internal/world/simulation.go`; native collision constants are in
`internal/assets/terrain.go`. No new startup JSON knobs are required.

## Verification

Commands and coverage are centralized in [world simulation checks](TESTING.md#world-simulation-checks).

## Implemented parity and remaining limits

Nonquest scripted shared prop frames persist and replay through the durable
cooldown store, with a 60-second reset; quest-conditioned frames remain private.
Unlinked nonservice NPCs use the resolved/default Kelan greeting. Clinic
confirmation heals the character and accompanying pets. Native waypoints, poses
and scene-ready synchronization have handlers. See
[world/combat parity](WORLD_COMBAT_PARITY.md) and [native commands](NATIVE_COMMANDS.md).

Starting a wild/proximity battle reserves and hides its overworld actor from
public viewers. The last participant leaving the battle starts the 30-second
respawn delay; respawn restores the immutable spawn and three-second movement
grace. Reservations, patrols and pursuit state reset after server restart.

Arbitrary obstacle navigation, complete native movement-check validation and
multiplayer/load acceptance remain unfinished. Instance lobbies have their own
implemented browser and transient room membership, but verified dungeon
objectives and rewards remain pending; see [Teams](TEAMS.md#instance-tab).

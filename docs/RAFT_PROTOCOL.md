# Native raft placement and landing

Verified against Legacy Client on the real Legacy server on 2026-11-03.
The two private captures remain ignored under `var/pet-switch-capture/`:

| Capture | SHA-256 |
| --- | --- |
| `raft-reference-20261103-01.pcapng` | `e025c3a19c15ea7e8f593d0370873f9968a10c9195dd08d64c7746b9831d1b19` |
| `raft-reference-20261103-02.pcapng` | `d790a71dadf4d99676cbf0546ca14b4f956bd0c6567a8595ec7a00efd1cb6fce` |

Both show item 48016 in slot 12. Normal landing keeps the item in inventory.
Neither trace sends item removal (`23:9`), vehicle destruction (`15:15`), or a
scene reload (`5:4`) for this landing. Wear-triggered destruction and authored
Starter Beach travel rules are separate from normal dismounting.

## Exchanges

All integers are little-endian. Codes have different meanings by direction.

| Direction | Code | Payload after command/subcommand | Meaning |
| --- | --- | --- | --- |
| Client to server | 15:9 | slot byte, item ID uint16 | Place vehicle from inventory |
| Server to client | 15:12 | slot byte, owner uint32, item ID uint16, X uint32, Y uint32 | Show unoccupied vehicle |
| Client to server | 15:7 | slot byte, item ID uint16 | Board placed vehicle |
| Server to client | 15:10 | slot byte, owner uint32, item ID uint16 | Confirm mounted state |
| Client to server | 15:10 | discriminator byte, item ID uint16 | Dismount |
| Server to client | 15:11 | slot byte, owner uint32 | Release mounted state; retain item |

The captures missed the outgoing initial placement and boarding requests.
Those requests are established independently by the native executable's send
dispatcher: `0x002c5cde` builds 15:9 from the selected inventory slot/item;
`0x002c5b11` builds 15:7 from the selected placed vehicle. The executable SHA-256
is `ca19ee087b601182e872235e58e6bf367f4bb21879226dfe3cc96f1c99624f61`.
`FUN_00449318`'s inventory-use path selects send case 9; `FUN_0044aac0`'s
boarding path selects case 7 and checks strict proximity bounds: X within
100 pixels either side, player Y between vehicle Y minus 150 and plus 100.

Automatic water-click boarding remains a distinct exchange: client 15:14,
server 15:18, client 15:7, server 15:10. `FUN_0044a514` handles manual 15:12
without automatically boarding; `FUN_0044a6dc` handles automatic 15:18 and
sends the boarding confirmation. Do not substitute one receipt for the other.

Water clicks first use ordinary land pathfinding when land lies between the
player and the selected water cell. Reaching that shore does not retain an
automatic boarding action: another click must select water with no intervening
land or obstruction. The direct-water check scans the same walk grid and chooses
the first water cell on that click ray as the boarding entry point.

The native landing request is followed by a normal 6:1 movement request for
the shore coordinate. Gonline now sends the same order. Walking is checkpointed
under the regular recoverable-position policy; mounted state is committed
before the dismount receipt. Repeated dismount requests are harmless.
Landing clears the selected destination and stops at the dismount position;
walking farther inland requires a new click.

Server 15:14 carries slot byte, owner uint32, wear uint32 and fuel uint16.
`FUN_0044d5d0` updates instance fields +0x20 and +0x24, respectively. The item
constructor `FUN_003d7cdc` maps these to bag metadata offsets 12 and 16; unrelated
metadata, including forging at offset 18, is preserved. The HUD painter
`FUN_00262974` uses `round(rank^1.3) * 1800` as capacity (catalog record offset
45), subtracts accumulated wear for remaining health, and divides fuel by the
same capacity. Exact wear accumulation and fuel-consumption rules are not
established by these traces; no new fuel-consumption rule is introduced.

The client displays the native item icon and `icon_HP2`/`icon_SP2` bars on the
left below player stats. Native progress replies and inventory updates refresh
these values. The existing server wear percentage also limits displayed health.
Inventory items show the same condition while mounted or stored in the bag:
native health/fuel strips overlay the item at Y+5/Y+10, with compact strips for
capsules. Hover details include fuel percentages; health is shown by its bar.
HUD/inventory share one
condition calculation so progress replies cannot produce different values.
Robinson's raft (48016) keeps full displayed health and no longer takes movement
wear; the authored Starter Beach wreck remains a distinct story rule.

Robinson's raft stays mounted through Newbie Island's authored water exit and
edge areas, including the captured area-three route to Beach. Map entry does
not consume it. While it remains mounted on Starter Beach, portal and walk-in
region exits are rejected with the ordinary event-resume reply. Landing on a
clear terrain cell (raw value zero) commits the landing position and exact
mounted-slot consumption together, then emits `23:9`, `15:15`, `15:11` with no
scene refresh. Water, alternate water, blocked cells and missing terrain do not
trigger the break. The old X/Y threshold and portal-consumption hook are retired;
the imported `portal_consume_vehicle` hint is retained for data compatibility.

For the special Beach landing, Gonline Client reports the shore movement and
waits for the server's break/dismount receipts. The server also accepts Legacy's
dismount-before-movement order: a dismount announced while still over water does
not release or consume the raft; the following clear-land movement breaks it.
Ordinary landings elsewhere retain their existing dismount-before-movement flow.

Boarding preserves the active battle pet and its durable battle flag. Vehicle
normalization excludes a companion mount but no longer rests battle pets, including
on login. Battle/rest selection remains available while riding; riding a pet
remains mutually exclusive with a vehicle. No automatic pet-rest packets are sent
on boarding. Gonline Client hides companion sprites, names and world targeting
while their owner rides, for both local and peer characters. Hidden companions do
not follow the sailing trail. After landing the currently selected pet resumes
beside its owner; a pet set to rest stays absent. Battle participation and pet
selection state remain independent of this world visibility.

## Starter Beach scripted wreck capture

`var/pet-switch-capture/raft-beach-reference-20261103-02.pcapng` records
Legacy Client on the legacy server after the second character was set idle.
SHA-256: `7f08b176809751de572e2ff437bb01f9b3b557e9dcb9832301b59ed772c5e227`.
The capture was stopped and its packet filters restored. The initial `-01`
attempt is superseded because the other character was running Remote.

The travelling connection uses local port 51915, character ID 4617590;
local port 52863 is the other connection and must be excluded from analysis.
All values below are decoded decimal bytes. The raft is Robinson's raft 48016,
bag slot 12. Times are relative to the first decoded frame, not capture startup.

| Time | Direction | Payload / meaning |
| --- | --- | --- |
| 13.988 s | Server → client | `15,18,12,118,117,70,0,144,187,222,15,0,0,7,13,0,0`: automatic placement at (4062,3335) |
| 16.184 s | Server → client | `15,10,12,118,117,70,0,144,187`: board raft |
| 22.452 s | Server → client | `12,118,117,70,0,8,43,134,0,102,4,1,0,0`: transition to Beach 11016, position (134,1126) |
| 24.327 s | Server → client | `15,10,12,118,117,70,0,144,187,0,0`: mounted vehicle restored during map initialization |
| 38.694 s | Client → server | `6,1,1,104,1,212,3,0,1,79,16,28,16,1,0`: movement reporting (360,980) |
| 40.491 s | Server → client | `23,9,12,1`: remove one item from bag slot 12 |
| same batch | Server → client | `15,15,118,117,70,0,144,187`: break/remove raft 48016 for character 4617590 |
| same batch | Server → client | `15,11,12,118,117,70,0`: dismount bag slot 12 |

The break is server-driven after movement on Beach, not part of the initial
map transfer. No AC5:4 refresh follows the break in this trace; the earlier
AC5:4 at 25.475 s belongs to map entry. Gonline now sends the same three core
packets without the former extra AC5:4. The former coordinate trigger
(X >= 280, Y >= 950) could break the raft over water and has been replaced by
clear-land detection, following the owner's clarification that consumption
occurs on beaching. This single trip alone does not establish every legacy
trigger boundary.

The trace also contains AC15:14 updates whose raw progress field increases
from 720 through 1154 while this raft is mounted. This establishes wire values,
not a change to Robinson's special displayed-health rule. Some outgoing
placement/boarding and intermediate movement requests were not recovered;
the server transition and complete ordered wreck receipts were recovered.

## Regression checks

Server `TestVehiclePlacementBoardingAndLanding`,
`TestVehicleLegacyLandingThenMovementPreservesRaft`, and the vehicle save-failure
tests cover placement without boarding, same-map publication, retained inventory,
idempotency, transaction failure and the land-position checkpoint.
`TestStarterBeachRaftWaitsForLand`, `TestStarterBeachRaftPortalAndLandingFailure`
and `TestNewbieRaftPortalArrivalPreservesMountedRaft` cover water survival,
deferred dismount, land consumption, exact break order, blocked Beach exits,
save failure and mounted arrival from Newbie Island. Terrain tests distinguish
clear land from water and obstructions. Client
`TestNewbieIslandRaftEdgeUsesAuthoredExit` and
`TestStarterBeachQuestRaftLandingReportsLandBeforeBreak` cover the authored
edge-area request and special landing handshake.
Wire `TestVehicleLegacyCapture` uses independent raw captured reply bytes.

Client `TestVehicleUseRequestsManualPlacement`, `TestPlacedVehicleLegacyCapture`,
`TestPlacedVehicleSailsFromShore`, `TestPlacedVehicleRejectsStaleAndPeerBoarding`,
`TestPlacedVehicleDrawAndOpaqueHit`, and the water-travel tests cover inventory
requests, scene placement, source-pixel picking, ownership, acknowledgment gates,
water pathing and landing order. Placement sprites use the regular scene depth
queue and are removed when the matching vehicle is mounted.

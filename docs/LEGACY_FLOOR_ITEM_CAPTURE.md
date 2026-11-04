# Legacy floor items and destruction — 2026-11-03

The user dropped a Bee Spine, picked it up, then dragged Grape Syrup out of the
inventory and confirmed destruction on the legacy client connected to the legacy
server. All three actions succeeded. Source trace and decoded packets remain
ignored under `var/pet-switch-capture/floor-items-reference-20261103-01.*`.
Packet Monitor stopped successfully and restored the initially empty filter set.
This investigation changes documentation only.

## Observed server sequence

Times are relative to the first decoded packet. Payloads below use decimal bytes.
All seven relevant packets were server-to-client; outgoing requests were not
retained, so this trace does not establish their wire layouts.

| Time (ms) | Packet | Meaning |
| --- | --- | --- |
| 13213.352 | `[23,3,238,179,190,10,209,9,0,0,0,0,1]` | Bee Spine (46062) appears on the floor at X=2750, Y=2513; final byte identifies ground slot 1. |
| 13213.352 | `[23,9,27,1]` | One Bee Spine removed from bag slot 27. |
| 34899.941 | `[23,2,1,0,1]` | Successful pickup/removal of ground slot 1. |
| 34899.941 | AC23:6, 33 bytes | One Bee Spine added to inventory. Prefix `[23,6,238,179,1]`, followed by 28 zero bytes. No explicit bag slot. |
| 48702.672 | `[23,212,7,26,24,126,1]` | Confirmation prompt for destroying one Grape Syrup (32280) from bag slot 26. Native prompt marker is 7. |
| 61934.197 | `[23,26,24,126,1]` | Destruction acknowledgement for one Grape Syrup. |
| 61934.197 | `[23,9,26,1]` | One Grape Syrup removed from bag slot 26 after confirmation. |

The Bee Spine inventory addition is byte-for-byte identical to each Bee Spine
acquisition in the earlier combat-drop capture. Floor pickup and battle drops
therefore share the same observed AC23:6 inventory-addition format. Acquisition
is separate from ground visibility/removal and from battle item-flight effects.
This single-unit test does not establish multi-unit ground stack behavior.

The Grape Syrup confirmation sends no bag-removal packet before the user confirms.
Unlike the Bee Spine drop, it does not spawn a ground item. The user's drag-out
gesture alone therefore does not mean the server drops an item: it can request
confirmation to destroy an undroppable item instead.

## Gonline comparison at capture time

- Current Gonline pickup sends the bag update before ground removal, whereas this
  legacy trace sends AC23:2 before AC23:6. Its bag update uses AC23:5 rather than
  the captured slotless AC23:6 acquisition.
- Current Gonline drop sends AC23:9 before AC23:3. This trace sends ground spawn
  first, then inventory removal. The captured item and removal quantities agree.
- Gonline's server sends AC23:212 marker 255 and its client accepts only marker
  255. The captured legacy prompt uses marker 7. This is a compatibility gap.
- Gonline's existing confirmed-destruction acknowledgement/removal order matches
  the captured AC23:26 then AC23:9 sequence for this one-item case.

See [combat item acquisition and recruitment](LEGACY_REWARDS_CAPTURE.md) for the
other live captures. No inference about client destruction-request bytes or
general item eligibility rules is made from server replies alone.

## Implemented behavior

The server now emits the captured floor-spawn/removal and pickup/acquisition
order, and uses destruction prompt marker 7. The client reads the native 33-byte
AC23:6 gain, stacks/places it using catalog item dimensions, draws dropped and
authored ground items, walks into pickup range and requests pickup without
optimistically removing the item. Confirmed pickup removes its ground sprite.
Marker 255 remains accepted for compatibility with older servers; it is no
longer emitted by Gonline. Confirmed destruction still waits for server removal.

# Collection-spot definitions

Assets schema **v25** adds structured collection-spot definitions sourced from
the owner's two saved WLOHub Collections pages and two additional coordinates.
The dataset contains **162 spots**, **251 listed rewards** and **82 distinct
native reward items**. Thirty spots are classified as vacuum collection spots:
28 reference entries with multiple rewards, following the owner's rule, plus
the two supplemental locations. Single-reward reference entries retain an
unresolved collection mechanism; the source also includes loose-item pickups.

This dataset does not implement vacuum interaction, change `/mine` or `/chop`,
or assign unknown probabilities, intervals, quantities or tool restrictions.
The reference does not specify those rules. The database can retain partially
resolved definitions without treating them as active nodes. Native EVE-backed
manual collection is implemented as described below.

## Coordinates and matching

The source provides coordinates for 150 reference spots. Ten have only small
screenshots; those images were inspected and do not display coordinate text.
Their positions remain explicitly unknown instead of being guessed.

Native map IDs are matched by scene name and terrain bounds. Two authored name
aliases normalize Universal Island to Universal and Hawaii - Large Island to
Large Island. Bounds shared by a location group can distinguish map variants;
inconsistent coordinates do not invalidate other entries in that group. A map
is assigned only when one candidate remains. Candidate IDs are retained when
multiple variants fit. In total, **107 spots** have resolved maps, coordinates
and reward IDs. This indicates data completeness, not gameplay implementation.

The owner's additions have stable definition keys:

| Definition key | Location | Coordinates | Native map | Rewards |
| --- | --- | --- | --- | --- |
| 65001 | North Island | X:5300, Y:1900 | 60001 | Unspecified |
| 65002 | South Island | X:1362, Y:3575 | Candidates 60002/60003 | Unspecified |

The additional reward pools remain empty until identified. Existing reference
IDs 1–160 identify WLOHub records; keys 65001/65002 are owner-authored definitions.
They are not native map click IDs and must not be sent as click IDs to clients.

All 82 distinct reference rewards map to native item IDs using name, icon and
rank. Sea Water uses an authored choice (WLOHub item 2010 to native item 60001):
the website uses a placeholder icon, while the native name/rank and the existing
water-gathering implementation identify the item. No reward probabilities are
inferred from item order or repeated source references.

## SQL ownership and administration

`Catalog.CollectionSpots` loads from these generated, typed tables:

- `catalog_collection_spots`: source, location, region, collection kind, resolved
  map, coordinate values and whether a position is known.
- `catalog_collection_spots_map_candidates`: ordered native map candidates.
- `catalog_collection_spots_rewards`: native item IDs and source item references.

The `CollectionSpots` administrator dataset supports validated edits. Zero map
or reward IDs indicate unresolved native links; missing coordinates have a
separate boolean. Owner-defined rewards can use native item IDs without a
website reference. `CollectionSpot.Resolved()` checks location/reward data only.

The embedded JSON is an offline initialization seed, not a runtime fallback.
Migration creates missing feature tables and seeds only a new root. Existing
administrator edits and intentionally empty definitions survive repeat upgrades.
Importing a reduced catalog preserves source records with unresolved native IDs
instead of referring to missing maps/items. Raw EVE/native records are unchanged.

The local assets database was upgraded and checked for all 162 spot rows, 251
rewards and 30 vacuum entries. Originals are backed up at
`var/backups/pre-migrate-20261104-061037`. Gameplay data was not migrated.
Existing installations follow [ASSET_DATABASE.md](ASSET_DATABASE.md).

## Evidence and reproduction

The factual spot/reward reports, source hashes, two name aliases, Sea Water
choice and owner coordinates are in
[`tools/data_export/reference/wlohub-collection`](../tools/data_export/reference/wlohub-collection).
The spot report records unresolved fields and source screenshot filenames.
The seed is
[`collection_spots_defaults.json`](../internal/assetsql/collection_spots_defaults.json).

```powershell
python tools/data_export/extract_collection_spots.py --pages /path/to/saved/collection/pages --item-export /path/to/item_data.json --scene-export /path/to/scene_data.json --assets-db /path/to/assets.db --output-dir /path/to/output
```

The assets database is opened read-only for terrain bounds. Input locations are
portable and no network is needed. The reference/item choices, aliases and
supplemental spots default to authored repository CSVs, with CLI overrides
available. Seeds and both CSV reports reproduce byte-for-byte; provenance
includes checksums of source pages, native exports, authored choices and terrain
bounds. See [TESTING.md](TESTING.md) for focused validation.

## Legacy packet reference: North Island grass, 2026-11-04

A real Legacy client/server capture confirms manual collection without a tool.
The owner collected Common Grass, Vine Grass and Vine, with battles, Remote
activation and repeated cooldown attempts between collections. This does not
establish vacuum behavior or continuous tool-based collection.

The captured click `AC20:1 [20,1,33,0]` selects actor 33. Its location and authored
grass pool identify map **60000**, actor position **(1303,1326)**, linked event
**13** and timer/mark **11020**. The captured player movement immediately before
that click ends at (1282,1315). The owner independently identified North Island.

The successful Vine Grass exchange is complete in both directions:

| Relative capture time | Direction | Packet / meaning |
| --- | --- | --- |
| 273296.165 ms | Client to server | `AC20:1`, actor 33 |
| 273930.820 ms | Server to client | `AC23:6`, item 40012, quantity 1 |
| 273930.820 ms | Server to client | `AC66:2`, one timer, ID 11020, 180 seconds |
| 273930.820 ms | Server to client | `AC6:2 [6,2,1]`, event movement lock |
| 273930.820 ms | Server to client | `AC20:10 [20,10]`, step complete |
| 274016.149 ms | Client to server | `AC20:6 [20,6]`, acknowledgement |
| 274921.171 ms | Server to client | `AC20:8 [20,8]`, event resume |

The item receipt is the existing 33-byte slotless acquisition packet. Its final
actor-flight flag is zero in all three observed collection rewards. The timer
packet is `[66,2,1,12,43,0,0,0,0,0,128,102,64]`: ID 11020 is a little-endian
UInt16, followed by the eight-byte little-endian floating-point value 180.0.
The first observed collection also sends `AC24:1 [24,1,12,43,1]`, recording mark
11020, step 1; the next two collections do not repeat that mark packet.

The three observed collection awards occur at 83904.176 ms (Common Grass 40014),
273930.820 ms (Vine Grass 40012) and 475152.071 ms (Vine 40011). The intervals
are 190.027 and 201.221 seconds, consistent with the explicit 180-second timer.
The trace does not measure the earliest possible successful retry.

Cooldown attempts send movement lock followed by `AC20:1` dialogue 50061,
**"No more things to collect now."** The client acknowledges with `AC20:6` where
captured, and the server releases the event with `AC20:8`. These attempts contain
no collection reward or timer-reset packet. The imported event also contains
dialogue 50062 for insufficient inventory space; that branch was not tested.

The authored reward operation selects pool type 1, variant 1. Its six imported
items are Common Grass (40014), Cotton (40007), Harl Grass (40008), Vine (40011),
Vine Grass (40012) and Pine (40020). Only the three reported rewards were observed;
this sample does not establish probabilities for any item. The successful EVE
branch contains the reward, timer start and optional first-use mark, without a
tool/skill prerequisite.

This observed multi-item pool works without a tool. Pool size alone therefore
does not prove a vacuum requirement; the registry's earlier source-based vacuum
classification must not override a native event's actual prerequisites.

Gonline `PlanEventGathering` / `tryWaterGathering` recognizes the complete
reward/cooldown branch and commits its resource claim atomically. It now uses
slotless acquisition receipts, the native timer notification and a step-complete
exchange. The client presents the item notification without an actor flight,
updates its transient timer cache and acknowledges completion. The server then
releases movement; acknowledging never grants resources. Normal cooldown and
inventory-capacity dialogue branches remain authored EVE behavior. Timer state
and rewards survive reconnects through the gameplay database.

Assets v29 removes only the exact old generated collection-disable diagnostic
when every reward branch has a known pool/timer, every reward item exists and
the remaining event branches contain supported conditions and resolvable speech.
The installed Client catalog enables 25 events, including North Island 60000/13.
Custom reasons, mixed failure reasons, unknown pools/items/actions/conditions,
absent entries and intentionally empty data are preserved. Repeat initialization
does not clear later operator disables. New imports apply the same validation.
The imported `CollectionSpots` registry remains separate: it does not invent
actor IDs or turn unresolved website coordinates into new gameplay nodes.

Use a Client-compatible assets database and run its offline upgrade before
starting the new binaries; the feature requires assets schema v30 and gameplay
schema v21. Development validation upgrades isolated older catalog copies. The local
quest-development v38 database and gameplay database are left untouched.

### Shared cooldowns and individual durations

Each source has its own server-wide cooldown. Collecting from a spot makes that
spot unavailable to every player until its expiry; collecting elsewhere neither
starts nor resets that timer. Different spots can use different durations.
The same rule applies to native ground-item respawns. Player-dropped items remain
transient and do not respawn.

Gameplay `resource_cooldowns` keys combine resource kind, map ID and source ID.
Kind 1 identifies a collection actor's click ID, kind 2 a native ground item's
canonical pickup slot, and kind 3 an area entry's ID. Actors sharing an EVE event
or native timer ID still have independent cooldowns. The SQL claim, inventory
grant and expiry commit atomically, preventing duplicate claims across accounts
and database connections. Expiries survive server restarts.

Edit the typed `ResourceRespawns` asset definition to override an individual
source's duration. For example, this row gives North Island actor 33 a 45-second
cooldown:

```json
{"kind":1,"map_id":60000,"node_id":33,"cooldown_seconds":45}
```

Omitted sources retain their authored EVE duration (180 seconds in the captured
manual collection) or the native ground-item default of 120 seconds. Overrides
must be positive and unique per resource key. Editing a duration affects the next
successful claim; an already-running expiry remains unchanged.

Native timer IDs remain condition and wire identifiers. Collection branch
selection projects the source's shared expiry into those conditions; historical
character-owned timer rows no longer determine collection availability. Migration
preserves those rows without guessing which source they originally represented.
The shared policy is an administrator requirement, rather than a cross-player
behavior established by the single-character reference capture above.

Vacuum interaction and continuous tool collection remain unverified. No vacuum
request format, tool cost, skill restriction or reward probabilities were added
from this manual-collection trace. Existing imported pool selection is retained.

Capture: ignored `var/pet-switch-capture/collection-reference-20261104-01.pcapng`,
613,760 bytes; SHA-256
`D8FE7FB902D51A403B2583395ACC85B7597A4C6397DDECA8D62919D7C3F1CE7A`.
The server-to-client TCP payload stream has no internal sequence gaps. The
client-to-server stream has missing segments (1,332 sequence bytes between
observed segments), so most repeated click requests cannot be reconstructed.
The Vine Grass request/response/acknowledgement above is present. Decoded event
and collection-only extracts remain beside the capture; authentication payloads
are excluded from those extracts. Capture stopped and restored the original
empty packet-filter set. No server-list changes were made.

## Alogin vacuum capture on Gonline - 2026-11-04

The owner tested a standard Vacuum Cleaner (34077) using Alogin connected to
Gonline through a local forwarding relay. This establishes client requests and
current Gonline replies, rather than the real server's vacuum validation rules.
The character first collected North Island actor 33, then moved to South Island
(60001), near the reported coordinates X:4942, Y:575, to avoid encounters.
The South Island collection actor is 70.

Observed client packets, shown as decimal bytes:

| Operation | Client request |
| --- | --- |
| Equip the vacuum from bag slot 9 | `[23,11,9]` |
| Collect South Island actor 70 | `[20,1,70,0]` |
| Start vacuum collection at actor 70 | `[23,73,70]` |
| Stop vacuum collection | `[23,73,0]` |

At 309.927 seconds into the trace, the client requests actor 70. Gonline grants
one Ordinary Wood (45001) using the 33-byte slotless `AC23:6` receipt, starts
native timer 11008 for 180 seconds using `AC66:2`, and sends `AC20:10`.
The client sends `[23,73,70]` at 310.168 seconds, then acknowledges completion
with `[20,6]`; Gonline releases the event with `[20,8]`.

At 499.977 seconds the client sends another `[20,1,70,0]`, 190.050 seconds after
the first request. Gonline grants one Lauan (45007), sends the same timer and
completion exchange, and receives another acknowledgement. No second vacuum-start
request occurs between these two collections. The trace cannot distinguish an
automatic client retry from a manual click; the owner's reply confirmed the two
items but did not explicitly answer whether the second collection was automatic.
At 576.958 seconds the client sends `[23,73,0]` to stop. The owner then logs out.

The earlier North Island test also contains `[23,73,33]` and `[23,73,0]`.
The observed start/stop payload carries a single-byte actor reference, unlike
the ordinary click's UInt16 reference. Behavior for actor IDs above 255, vacuum
durability/consumption, special vacuum variants and real-server validation remain
unverified. No new reward probabilities or tool costs follow from this capture.

The relay forwards original frames and records only selected gameplay commands;
login and character-creation credentials are excluded. Capture stopped after
logout, and the original Alogin `SERVER.INI` was restored byte-for-byte after
checking for unrelated edits. The retained ignored trace is
`var/pet-switch-capture/vacuum-gonline-20261104-03.events.jsonl` (646,446 bytes,
3,789 frames), SHA-256
`459bf2cfc294c3bb906e3f940af1cd180bc3bebcd01da12fefc2e168a7ea06dd`.
A collection-only extract is stored beside it as `.events.collection.json`.

## Gonline client vacuum scheduling

Equip a vacuum and click a native vacuum collection actor within reach to
start automatic collection. The client sends the native `AC23:73` start/stop
requests and repeats ordinary `AC20:1` collection requests. Rewards and shared
availability remain server-authoritative. Moving, changing maps, entering battle,
removing the tool or disconnecting ends the local run.

On Gonline, a successful collection's `AC66:2` respawn expiry schedules the next
attempt **10 seconds after expiry**, respecting each source's configured duration.
Bit 0 of the initial connection-wide `GLON` hello identifies this behavior,
independently of combat-chain configuration. No collection-specific handshake
is required. Older Gonline servers' `AC1:243 GLCT` announcement remains supported
for compatibility. Server names and generic echoes cannot enable it; connection
state resets on reconnect. Rebuild both server and client
to use this timing policy; older servers retain the Legacy fallback.

Other servers use the Legacy client's fixed **190-second interval**, rather than
adding ten seconds to their received timer. `FUN_00456824` compares elapsed Delphi
date/time multiplied by 86,400,000 against 190,000 milliseconds. These constants
at `0x456898` and `0x45689c` were verified in the matching `var/ghidra/aLogin.exe`.
Until a newly selected spot supplies its first successful timer, the client uses
this native cadence, including when collection initially fails due to cooldown.
Each subsequent Gonline collection updates the run from its own received timer;
unrelated actors/timers and malformed timer packets cannot change its schedule.
`FUN_00455154` limits native vacuum targets to templates 19052, 19058–19061 and
19069–19071; the client preserves this list instead of treating all props as
collection nodes.

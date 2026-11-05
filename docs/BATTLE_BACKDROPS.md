# Native battle backdrop selection

`Ground.MMG` contains the map-to-backdrop data. It includes a default selector
for every map and rectangles for maps with multiple environments. EVE formation
headers and the inspected battle operations do not provide this spatial mapping.
`SceneData.dat` defines the scenes and their artwork, rather than these regions.
There is no need to invent terrain boundaries from screenshots.

## Evidence and binary layout

In `var/decompiled/aLogin_ca19ee087b60.c`, `FUN_003f830c` loads the terrain tail
after the collision grid, sound zones and map objects. The next fields are:

| Bytes | Meaning |
| --- | --- |
| 1 | Unresolved parameter, usually 100; preserved in the original export |
| 1 | Default battle backdrop selector |
| 1 | Number of regional overrides |
| 9 per override | Selector byte, then left/top/right/bottom UInt16 coordinates |

`FUN_003fa0b8` draws the nine-byte records as screen rectangles and looks up
their labels with `SceneData[59000 + selector]`. Its offsets are selector
`TMap+0xc703b+9*i`, followed by coordinates at `+0xc703c/e` and `+0xc7040/2`;
records are indexed from one. The loader's default is at `TMap+0xc7039`.
The former export treated each rectangle as `<IHHB>`, obscuring these meanings.
The exporter now adds descriptive fields while retaining its original raw fields.

AC11:250 carries the selector as one byte followed by separate formation metadata.
`FUN_0038e4f4` passes the first byte to `FUN_0036f448`, which loads the scene and
falls back to scene 59081 when it is unavailable. Both client implementations
receive the same selector from Gonline Server.

## Coverage and selection

The supplied native archive has 1,147 terrain maps. All 1,119 EVE map IDs are
covered. There are 1,398 rectangles across 101 maps; the remaining maps use
their own native default. This includes indoor maps and maps where combat is
normally unavailable. Original map IDs, selectors, bounds and record order are
retained without biome classification or encounter/species exceptions.

For Legacy clients, the server selects at battle entry: explicit administrator
`battle_background` policies first, then the initiating player's native map
regions and default. For a negotiated Gonline Server + Client connection, the
server sends the shared entry map/coordinates and a compatibility selector;
the client resolves its own native backdrop metadata. A Gonline-only battle
performs no server spatial lookup. Mixed groups resolve on the server only for
their unnegotiated members. Server-side backdrop policies affect only those
server-resolved recipients.

All recipients use the same initiating position. Native defaults therefore match
when client/server assets match. Both implementations use inclusive boundaries
and ordered first-match precedence; the closed Legacy server's exact overlap
tie-breaking remains unverified. Gonline Client on a Legacy server always honors
the supplied selector. Missing local metadata prevents opt-in; a missing map in
an opted-in catalog retains the compatibility selector. Native selector 150 on
map 58000 is preserved, with the existing missing-scene fallback for scene 59150.

| Map | Player position | Selector | Backdrop |
| --- | --- | --- | --- |
| 60001, North Island | 2202,1395 | 91 | Lakeside Jungle |
| 60001, North Island | 2000,400 | 90 | Jungle |
| 60001, North Island | 5000,3000 | 50 | Beach |
| 60001, North Island | 2622,755 | 81 | Desert Grassland |
| 60000, North Island | 1694,963 | 50 | Beach |
| 12000, Kelan | Default | 130 | Welling Street |
| 12002, Civilian House | Default | 140 | Room Scape |

The map-12000 selector also matches the captured honey battle's raw header
`[11,250,130,1]`. The lakeside region contains the supplied Fresh Mushroom
screenshot's player coordinates. Similar-looking forest scenes should not be
assigned by encounter ID: the native regions distinguish them spatially.

## SQL ownership and audit

Asset schema v50 stores these definitions in `catalog_battle_backdrops` and
ordered child rows in `catalog_battle_backdrops_regions`. The `BattleBackdrops`
administration dataset permits validated server edits. Server-side resolution
reads only these tables. Negotiated clients use their own metadata instead.
The offline migration decodes retained native bytes already in the assets DB;
it does not depend on the developer's filesystem. It preserves existing typed
definitions, including intentionally empty tables, and retires only the exact
original five encounter-specific defaults from v45/v47/v48/v49. Edited Story
overrides remain intact. Migration and schema changes commit transactionally.

Stop the server and follow [the offline upgrade procedure](ASSET_DATABASE.md).
Rebuild the server after migrating; no new client artwork extraction is needed.

Generate a complete audit, including every map, rectangle, coverage counts and
source-file hashes:

```powershell
go run ./tools/battle-backdrop-report -source <native-data-directory> -output var/reports/battle-backdrops.json
```

The report is an offline inspection artifact, not a runtime configuration source.
Decoder goldens use independently authored bytes; integration tests verify native
map counts and positions, SQL migration/rollback/edit preservation and formation
packet selectors. Live boundary tie-breaking remains unverified.

## Gonline client resolution protocol

Local backdrop resolution is bit 2 of the initial connection-wide `GLON`
feature exchange, independent of `combat_round_chain`. The client includes that
bit only when local metadata is available. No backdrop-specific handshake occurs
after the shared hello. Connection changes clear the mode and pending location.
See [the connection protocol](CONNECTION_PROTOCOL.md).

Before each opted-in recipient's AC11:250, AC1:247 carries the `GLBD` signature and
version 1 followed by UInt16 map, X and Y (14 bytes total). The location is captured
once for the initiating player and shared by the battle. The client consumes it
once when a valid formation arrives, before loading scene artwork/music. During
round-chain playback, location and formation wait in their original packet order.
No additional requests are required per battle.

Asset packaging writes `runtime/battle_backdrops.bin`, a compact map of native
defaults and rectangles decoded from the original `ground_data.json` retained
bytes. Raw archives and JSON are not shipped in this runtime record. Source-data
runs can load the same definitions from that existing export. Rebuild runtime
bundles to add the compact file; old bundles simply decline negotiation and keep
server-side resolution. No new image extraction or SQL schema change is needed.

# Collection-spot definitions

Assets schema **v25** adds structured collection-spot definitions sourced from
the owner's two saved WLOHub Collections pages and two additional coordinates.
The dataset contains **162 spots**, **251 listed rewards** and **82 distinct
native reward items**. Thirty spots are classified as vacuum collection spots:
28 reference entries with multiple rewards, following the owner's rule, plus
the two supplemental locations. Single-reward reference entries retain an
unresolved collection mechanism; the source also includes loose-item pickups.

This change imports definitions. It does not implement vacuum interaction,
change `/mine` or `/chop`, or assign unknown probabilities, intervals, quantities
or tool restrictions. The reference does not specify those rules. The database
can retain partially resolved definitions without treating them as active nodes.

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

# Full item names

The 69 saved [WLOHub item database](https://wlohub.com/items) pages contain
**6,882 entries**. The compiled patch restores **696** item names, with **50**
unresolved candidate records retained for review. This is a conservative subset;
already complete names and items without a supported match remain unchanged.

## Extracted evidence and matching

[`items.csv`](../tools/data_export/reference/wlohub-items/items.csv) retains the
website ID, full name, icon, level, rank, type/subtype, alchemy bases, displayed
effects, description and page. `(Quest)` annotations are removed from names.
Website IDs are not native item IDs. Individual saved-page checksums and the
native export checksum are in
[`sources.json`](../tools/data_export/reference/wlohub-items/sources.json).
Extraction rejects overlapping, filtered or missing pages and verifies the
advertised total. Unknown website ranks remain unknown, not zero.

The native name field holds 14 bytes; English translations commonly use 13
bytes plus a terminator. Candidate names must extend an existing name of at
least 13 bytes. Automatic matches require the same icon and native rank
(decoded byte 45). If multiple names survive, an exact description match can
disambiguate them; otherwise they remain unresolved. No level mapping is
assumed for the legacy item level byte, whose meaning remains unresolved.

Names already confirmed complete by icon/rank are preserved. Enhancement
numbers cannot be extended by prefix matching: `Golden Rings+1` must not become
`Golden Rings+10`. Different boxes, equipment, recipe variants and other
ambiguous names are retained in
[`review.csv`](../tools/data_export/reference/wlohub-items/review.csv).
The resulting native-ID-specific corrections, original names and supporting
website IDs are authored in [`item_names.csv`](../internal/assets/item_names.csv).

The reference table does not replace gameplay item values, recipes, prices,
effects or descriptions. Only supported display names change.

## Compilation and SQL ownership

The name CSV is embedded into Go builds. The client applies it after loading
its lossless item export, so inventory, tooltips and other consumers use the
full names without rebuilding the extracted assets. Full/custom names are
preserved: a patch applies only to its recorded original name and native ID.
Raw decoded bytes and native/JSON golden fixtures remain unchanged.

Server gameplay reads names from SQL only. Fresh offline conversion initializes
the name corrections into the native item definitions. Assets migration **v23**
updates `catalog_native_items.value_definition_name` only where ID and original
name still match, preserving administrator edits, absent rows and every other
column. The compiled name seed is not consulted during server gameplay. Both
`Catalog.NativeItems` and its `Catalog.Items` projection receive the same names.
Gameplay inventory and item IDs are unchanged.

The backed-up local assets-only upgrade verified all 696 names. Its original
database is in `var/backups/pre-migrate-20261104-054400`. Existing installations
use the offline procedure in [ASSET_DATABASE.md](ASSET_DATABASE.md).

## Reproduce

Use complete saved, unfiltered item-list pages and an unmodified native
`item_data.json` export; inputs can reside anywhere. No network or third-party
Python libraries are needed.

```powershell
python tools/data_export/extract_item_names.py --pages /path/to/saved/item/pages --item-export /path/to/item_data.json --output-dir /path/to/output
```

The command writes `items.csv`, `item_names.csv`, `review.csv` and `sources.json`.
Review changes before replacing authored tables. Supplied inputs reproduce all
four artifacts byte-for-byte. Tests cover saved-page parsing, unknown ranks,
HTML entities, duplicate pages, icon/rank/description matching, complete names,
enhancement collisions, custom names, native data preservation, SQL migration
and client inventory definitions. See [TESTING.md](TESTING.md).

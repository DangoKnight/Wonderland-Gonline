# Item catalog formats

The maintained [data/item_data.json](../data/item_data.json) is the offline item
import source. The server reads its imported SQL records from the assets database.
Binary-format decoders support the offline exporter and format tests.

## Runtime loading

Configuration field `assets_database` defaults to `var/assets.db`. Paths are
relative to the process working directory. `item_catalog` and `data_directory`
are removed from server configuration.

Runtime loading reads typed `catalog_native_items` rows through
`assetsql.LoadDatabase`. `NativeItems` is the validated administration dataset;
its readable `value_definition_*` columns supply the gameplay item definitions.
The catalog also retains decoded record bytes, descriptions, icons and sprite
references. `asset_records`, `asset_documents` and legacy views are migration
provenance, not live item sources. Missing required structured tables or catalogs
stop startup, including inspection mode. Restart or use the administration
publication flow after validated edits; see [asset database](ASSET_DATABASE.md).

The JSON export's readable `definition` fields seed the typed definitions during
explicit offline import. `decoded_hex` preserves source bytes for field
interpretation and independent verification. Normal startup neither opens the
export/native files nor overwrites edited definitions from retained bytes.
Offline JSON validation checks IDs, schema/native versions, headers, source
size/hash structure, record lengths and positions before import. Its source hash
records provenance rather than requiring the original binary at runtime.

The maintained export contains 7,312 translated-client items. Status reports
`assets_database`, `native_items` and `items`; an installed catalog may contain
administrator edits. Converted server items are not merged into the WLRI item
definitions. Gacha import validates each complete pool: missing pack/reward IDs
disable only that pool with a diagnostic, while structurally invalid definitions
fail validation. Other complete pools remain available. Edit current rewards
through `GachaPacks`; source `gacha_packs.json` is an offline seed.

## Native version nine

The reference is the local WLRI `aLogin.exe` decompile, loader entry
`FUN_003cd92c` (previous notes identified an instruction at `003cd960`) and
`FUN_003cdfa4`. The latter transforms the disk data, then applies separate
client content patches and builds lookup tables. This Go decoder implements the
record transformations; client content patches and lookup side effects are
outside the format decoder.

- Record size: 451 bytes (`0x1c3`). Record zero is a plaintext header.
- Header version: little-endian UInt32 at disk offset 114 (`0x72`); supported
  value is 9. The client stores the file at object offset four, so its version
  read at object offset `0x76` refers to disk offset `0x72`.
- Name: length byte at offset zero; reverse the 14 bytes at offsets 1–14.
- Description: length byte at offset 146; reverse the 254 bytes at 147–400.
- Selected byte fields decode as `(encoded XOR 0x9a) - 9`.
- Selected UInt16 fields decode as `(encoded XOR 0xefc3) - 9`.
- Selected UInt32 fields decode as `(encoded XOR 0x0b80f4b4) - 9`.
- Arithmetic wraps at the field width. Status values are then interpreted as
  signed int32, preserving negative values.
- Text uses the existing ASCII/UTF-8 and Big5 decoding policy.

Only the fields transformed by the client are transformed by Go. Other bytes
remain intact. `NativeItem.Record` retains the complete decoded record;
unresolved fields remain positional data. The three offset tables deliberately
contain raw authored compatibility offsets, with the client's four-byte object
prefix already removed. They are a format map, not gameplay magic numbers.

### Server projection

| Disk offset | Width              | Field                           |
| ----------- | ------------------ | ------------------------------- |
| 15          | 1                  | Item type                       |
| 16          | 2                  | Item ID                         |
| 18, 20      | 2 each             | Small and large icon IDs        |
| 22–28       | Four UInt16 values | Appearance references           |
| 30, 32      | 2 each             | Status types                    |
| 34          | 1                  | Legacy importer's level mapping |
| 36, 40      | 4 each             | Signed status values            |
| 45          | 1                  | Compounding rank                |
| 46          | 1                  | Equipment slot                  |
| 112         | 1                  | Compounding restriction byte    |
| 123         | 2                  | Item flags, including compounding exclusion bit 2 |
| 136–145     | 2 each             | Five ordered material bases     |
| 406–407     | 1 each             | Inventory cell width and height |

The level mapping follows the existing converted import script. All supplied
native records have zero in that field; complete native equipment-level rules
remain unresolved. Existing equipment handling does not enforce level rules.
Other native properties can be added to the gameplay model when their meaning
and consumers are ported; decryption alone does not enable those mechanics.

The header is skipped rather than treated as an item. Decoded zero IDs are
skipped; the first duplicate wins, following the server's existing lookup policy.
Invalid lengths, unsupported versions and overlong text lengths reject the whole
native table. Source buffers are never modified.

## Verification

Independent raw/native format checks and their environment variables are in
[Testing](TESTING.md#native-format-verification). Runtime format rules above
remain separate from native-client acceptance.

## Maintained JSON export

[data/item_data.json](../data/item_data.json) preserves the translated
client's complete native catalog. Each item includes readable properties and
`decoded_hex` containing every byte of its decoded record. The header, file order,
source size and source hash are also retained. Re-encryption has been verified to
reproduce the source file byte-for-byte.

Regenerate with `go run ./cmd/item-export`; its default input is the sibling
translated client's `data/Item.dat` and its default output is
`data/item_data.json`. See [the asset notes](../data/README.md) for the
explicit command and provenance. The exporter rejects catalogs whose duplicate
or zero IDs would be collapsed by the lookup parser.

# Item catalog formats

The maintained [data/item_data.json](../data/item_data.json) is the offline item
import source. The server reads its imported SQL records from the assets database.
Binary-format decoders support the offline exporter and format tests.

## Runtime loading

Configuration field `assets_database` defaults to `var/assets.db`. Paths are
relative to the process working directory. `item_catalog` and `data_directory`
are removed from server configuration.

Runtime loading reads the `item.dat` / `items` collection in `asset_records`,
using `asset_documents` for export metadata. The existing JSON validator checks
the SQL materialization. It never opens the JSON export, `Item.dat`,
`itemDat.wpdat`, or the export's `source` metadata path. Missing or invalid SQL
assets stop startup, including inspection mode. Restart after SQL edits or a
rebuild. See [asset database](ASSET_DATABASE.md) for import instructions.

The JSON's readable `definition` fields are authoritative for gameplay. Its
`decoded_hex` preserves imported source bytes for later field interpretation;
editing a readable stat or name does not require changing those provenance bytes.
Item IDs must agree with the decoded record, and IDs must be unique and nonzero.
Schema/native versions, header, source-size/hash structure, record lengths and
positions are validated before any definitions are published. The source hash
records export provenance; runtime loading does not re-open or hash the binary.

The maintained asset contains 7,312 translated-client items. Status reports
`assets_database`, `native_items` and `items`. No server-only definitions or converted
stats are merged. All configured starters exist in this asset. The sibling
`gacha_packs.json` references absent server-only pack/reward IDs, so its existing
validation disables the entire table and reports a warning. Gacha code remains
implemented; enabling it requires a configuration whose IDs exist in the JSON.

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
| 46          | 1                  | Equipment slot                  |

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

A committed artificial golden record checks every decoded byte, text reversal,
Big5, negative stats and arithmetic wrap boundaries. Native-data tests compare
all decoded records in file order against independent reference SHA-256 hashes.
No original game data is committed with the golden fixture.

From the repository root in Bash:

```sh
WONDERLAND_TEST_DATA=../Wonderland-Private-Server/Data \
WONDERLAND_TEST_CLIENT_DATA=../Wonderland-Client/data \
go test -race ./internal/assets -count=1
```

In PowerShell, set the two environment variables before running the same Go
command. The full server native-data race suite checks gameplay with the
maintained JSON catalog. Catalog decoding and protocol test clients do not establish
original game-client compatibility.

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

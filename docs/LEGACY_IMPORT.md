# Import Private Server player data

`cmd/legacy-import` reads a Private Server SQLite snapshot offline and writes a
new Go gameplay database. This is separate from WLRI asset extraction. The
running server uses only its configured Go databases.

## Inspect first

Stop the original server and use a SQLite snapshot that includes committed WAL
writes. Upgrade the Go asset database using [ASSET_DATABASE.md](ASSET_DATABASE.md).
Then inspect the original gameplay snapshot:

```sh
go run ./cmd/legacy-import \
  -source /absolute/path/to/ServerDataBase.db \
  -assets-db /absolute/path/to/upgraded/assets.db
```

Without `-output-dir`, this command creates no destination. It opens the source
read-only, validates identities and definitions, and reports counts and unmapped
nonempty tables. Reports contain no passwords or deletion codes.

## Create and verify a new database

```sh
go run ./cmd/legacy-import \
  -source /absolute/path/to/ServerDataBase.db \
  -assets-db /absolute/path/to/upgraded/assets.db \
  -output-dir var/imported-private-server
```

The output directory must not exist. Accounts, characters, child records,
friendships and deletion security commit in one transaction. The command closes
and reopens the database, compares typed character state and verifies imported
friendships. Failure removes only the directory it created. Existing databases
and the original snapshot remain unchanged.

Every nonempty unmapped table blocks an import until reviewed. For tables that
you intentionally exclude, pass their exact names:

```sh
# Example only: replace these names with reviewed tables from your own report.
go run ./cmd/legacy-import \
  -source /absolute/path/to/ServerDataBase.db \
  -assets-db /absolute/path/to/upgraded/assets.db \
  -output-dir var/imported-private-server \
  -skip-tables Archive,UnusedDefinitions
```

Do not exclude guilds, mail or other player data merely to make the command pass.
Those rows require a supported mapping or a separate migration plan.

To activate a verified import, stop Go Server and set `database` in
`config.local.json` to `var/imported-private-server/wonderland.db`. Keep
`assets_database` pointing to the upgraded Go assets database. Build with
`go build -o bin/wonderland ./cmd/wonderland`, then run
`./bin/wonderland -config config.local.json`. Keep the original snapshot for
rollback; new progress after cutover will exist only in the new database.

## Supported mappings

| Source | Go state |
| --- | --- |
| `users` | Original account IDs, normalized usernames, email, IM balances and optional ban/GM fields |
| `characters` | Two native character ID offsets, appearance/colors, position, gold and deletion codes |
| `stats` | Five base attributes, current HP/SP, total EXP and unallocated points |
| `inventory` | Bag, six equipment slots, storage, quantity, damage and forge metadata |
| `character_skills` | Learned grades and proficiency |
| `charquest` | Progress state, step, optional kills and RFC3339 completion timestamps |
| `character_pets` | Party, hotel and reserved companions, active/mount flags, all attributes, EXP, amity, gear and optional saved skills |
| `Friends` | Validated, canonical undirected friendships within the native list limit |
| `character_record_points` | Saved return location |
| `character_monster_book` | Discovered NPC IDs |
| `charactersextdata.Settings` | PK, battle joining, channel mask and trading preferences |

The extended-data table still appears as unmapped when its Friends/Guild/Mail
fields contain unsupported state. Its supported settings are copied regardless.
Plain native passwords and deletion codes become PBKDF2 hashes. Password hashes
from salted IPBoard/forum schemas require a separate reset/conversion procedure;
they are never mistaken for plaintext passwords.

Levels and maximum vitals derive from current compiled growth formulas and SQL
item definitions. Current HP/SP must fit those maxima. The supplied reference
snapshot currently fails this check for pet 12032. The importer does not clamp
vitals or discard metadata silently; repair a separate source copy deliberately
before importing if this is the intended conversion.

Unsupported jobs/rebirth presentation, nicknames, potential bonuses, socket/bomb/
sewing metadata, unknown definitions, invalid credentials, duplicate slots,
orphans and oversized containers fail validation. Other nonempty tables are
reported for review. A MySQL runtime adapter and cross-backend compatibility
remain pending. This importer does not change asset-import sources or turn the
legacy database into a runtime dependency.

## Scope relative to the reference database tools

This is a preserving offline SQLite conversion, not a port of every database
provider/tool in Private Server. Its MySQL connection testing, live provider
reconfiguration and SQLite-to-MySQL migration are concrete source workflows with
no Go equivalent. Do not describe them as source placeholders. Unsupported field
failures above remain intentional until there is an explicit, verified mapping;
never silently discard job/potential/tent decoration or other durable data to
claim a complete import. Runtime checkpoints must not be used as an import tool.

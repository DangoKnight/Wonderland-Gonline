# Configuration and behavior controls

For initial setup, follow [GETTING_STARTED.md](GETTING_STARTED.md). Run from the
repository root so relative paths resolve consistently.

## Server JSON configuration

Copy `config.example.json` to ignored `config.local.json`, then pass
`-config config.local.json`. Omitted fields keep the built-in defaults. Unknown
fields and trailing JSON are rejected; use the supported keys below.

| Key | Default | Meaning / constraints |
| --- | --- | --- |
| `name` | `Wonderland Go` | Initial server name, 1–200 bytes; a saved admin `server_name` takes precedence |
| `login_address` | `127.0.0.1:6414` | TCP login/game listener |
| `world_address` | `127.0.0.1:6415` | TCP world listener |
| `status_address` | `127.0.0.1:6416` | Native launcher status listener, separate from HTTP health |
| `status_server_ids` | `[1, 101]` | Status records advertised to launchers; 1–100 unique IDs, each 1–9999 |
| `http_address` | `127.0.0.1:8080` | Web administration, `/healthz` and `/register` |
| `database` | `var/wonderland.db` | Persistent Go accounts, characters, relationships, settings and audit |
| `assets_database` | `var/assets.db` | Read-only static catalog loaded at startup; must already exist |
| `max_connections` | `512` | Maximum concurrent TCP sessions across the game/status services; accepted range 1–100000 |
| `idle_seconds` | `120` | Incoming-packet inactivity timeout; accepted range 1–86400 seconds |

Restart to apply JSON changes. `database` and `assets_database` must be separate
files, including when symlinks/hard links are involved. Relative paths are based
on the process working directory, not the config file's directory. There is no
runtime JSON/native asset fallback. Removed keys such as `item_catalog` and
`data_directory` are rejected.

Listener addresses use `host:port`; repeated identical addresses are rejected.
Port `0` asks the OS for a free port and is useful in tests, but the client has
fixed default game-port expectations.

### Local and LAN access

The default loopback addresses accept connections only from the same machine.
For LAN clients, bind the three TCP game/status listeners to your LAN interface
or to `0.0.0.0`, retaining ports 6414, 6415 and 6416. For example, these are JSON
field values to apply to your existing configuration:

```json
{
  "login_address": "0.0.0.0:6414",
  "world_address": "0.0.0.0:6415",
  "status_address": "0.0.0.0:6416",
  "http_address": "127.0.0.1:8080"
}
```

The client server list contains the server's reachable IP/hostname. It must not
contain `0.0.0.0`, which is a bind address. Permit the game/status ports through
the relevant firewall. HTTP administration can remain local; use an authenticated
TLS reverse proxy when providing remote web access. The server itself serves
plain HTTP and has no certificate config keys.

The Go client currently uses login 6414 and status 6416 as constants. Changing
server listener ports alone does not change those client constants. Keep the
default game ports unless also adapting the connecting client.

### Launcher status IDs

The server status screen reads TCP 6416, not `/healthz`. The Go client's
`SERVER.INI` region flag contributes `flag * 100 + server_index + 1` to the
status ID, where the first server index is zero. Thus `01[Local]1` with one
server expects ID 101; the defaults also include legacy ID 1. If you author a
different region flag or additional server entries, advertise the corresponding
IDs through `status_server_ids`.

Status load colors currently use fixed thresholds: green below 10 authenticated
accounts, yellow below 30, red otherwise. They are independent of
`max_connections` and are not JSON tuning fields.

## Server flags and environment

| Setting | Effect |
| --- | --- |
| `-config <file>` | Load JSON overrides; omitting it uses defaults |
| `-inspect-data` | Print catalog summary and exit before starting listeners or requiring an admin token |
| `-log-file <file>` | Append JSON logs to a file as well as stderr; parent directory must exist |
| `-debug` | Log packet metadata and bounded decoded Lucky Draw traces |
| `WONDERLAND_ADMIN_TOKEN` | Admin bearer credential, at least 24 characters; required for normal server startup |
| `CGO_ENABLED=1` | Build-time SQLite driver support; requires a working C compiler |

Use `./bin/wonderland -help` to inspect flags. There is no admin-token JSON key
or flag to disable admin authentication. The token is separate from game
accounts and GM permission. Restarting with a different environment token changes
admin access; the browser retains its current token only in memory.

## Live administration

The admin UI is served at the configured HTTP address. Its Settings view stores
`server_name` and `motd` in the gameplay database. `server_name` updates the running
server immediately and overrides JSON `name` on later startups. `motd` is stored;
in-game MOTD delivery is still pending.

Accounts and Sessions provide live ban/GM controls, password reset, account
removal, session termination and audited mall-point adjustments. These operate
on gameplay state. Registration does not grant GM/admin access. Admin changes do
not rebuild static data or change the startup listener/timeout configuration.

## Static gameplay tuning

The running server uses a startup snapshot of `assets_database`. SQL indexed rows
are authoritative for indexed arrays; editing only a retained document's array
will not override them. Close your SQL editor and restart after changes. Use
`-inspect-data` to catch catalog errors before starting listeners. See
[ASSET_DATABASE.md](ASSET_DATABASE.md) for schema, validation and backups.

| Behavior | Runtime source / editing location |
| --- | --- |
| Daily Lucky Draw rewards | `asset_records`: `asset='lucky_draw.json'`, `collection='value'`; view `asset_lucky_draw_rewards` |
| Gacha pack contents and probabilities | Indexed `gacha_packs.json/value`; views `asset_gacha_packs` and `asset_gacha_rewards` |
| Skill buffs, debuffs and protection | Indexed `skill_effects.json/records` plus skill effect references; [effect conventions](DEVELOPMENT.md#skill-json-and-reusable-effects) |
| Starter grants, pet vouchers, shops/mall, drops, alchemy and forging | Corresponding exported setting/table documents and indexed collections in SQL |
| NPCs, skills, dialogue and events | SQL-loaded exported records; native IDs, ordering and binary-derived fields must stay valid |
| Client artwork | Editable PNGs/atlases and frame metadata under `data/`; [sprite editing](../data/sprites/EDITING.md) |

For example, to increase the relative weight of the first Lucky Draw reward,
stop the server, back up the assets database, then execute in a SQLite editor:

```sql
UPDATE asset_records
SET json = json_set(json, '$.weight', 5)
WHERE asset = 'lucky_draw.json' AND collection = 'value' AND ordinal = 0;
```

This changes that reward's weight to 5 while leaving its item, quantity and slot
intact. Weights are relative; this does not mean a 5% probability. Positive
integer weights must fit the supported total. Daily Draw allows at most 20
consecutive one-based reward slots. Gacha packs have their own policy: positive
weights total 10,000, with up to 41 outcomes. Missing item definitions can disable
a whole affected gacha pack; the loader does not silently drop its rewards.

SQL edits last until the next rebuild. A rebuild regenerates the catalog from
maintained exports and overwrites those edits. To make conversion tuning
reproducible, maintain the relevant authored policy/source, run its offline
exporter, rebuild and restart. Skill conversion policy is
`internal/assets/native_effect_rules.json`; daily reward policy is
`internal/assets/lucky_draw_rules.json`. Policy/schema documentation lives in
[DEVELOPMENT.md](DEVELOPMENT.md). The WLRI media-only import does not regenerate
these gameplay definitions. Some complete gameplay exporters still need the
private-server reference tables as separate offline inputs.

Do not casually edit exported JSON and then rebuild: import validates file
checksums against `data/asset_manifest.json`. Use the appropriate exporter to
update the definitions and manifest together. Original-data reconstruction
verification also distinguishes original bytes from deliberate tuning.

### Values that need code changes

There is currently no server-config EXP multiplier, global drop multiplier or
combo-probability slider. Shared fixed rules live in their owning Go packages;
changing them requires rebuilding the server and testing the resulting behavior.
Examples:

- Daily Draw allowance: `internal/game/lucky_draw.go`, three draws per character
  per UTC calendar day; the native UI also assumes three.
- Inventory and progression limits: `internal/game/limits.go` and related game
  progression code.
- Combo speed/level probability rules: `internal/battle/combos.go`; see
  [COMBOS.md](COMBOS.md) for the current chaining and rebirth policy.

Use the existing descriptive constants and data-driven skill effects rather than
adding skill-specific or unnamed compatibility values.

## Client overrides

Run compiled clients from the repository root, for example:

```sh
./bin/wonderland-client -assets data -serverini var/SERVER.INI
```

| Flag | Purpose |
| --- | --- |
| `-assets <directory>` | Decompiled client asset root; explicit `data` avoids discovery ambiguity |
| `-serverini <file>` | Native-format server list; otherwise `SERVER.INI` under the asset root or a local fallback |
| `-sprites <directory>` | Optional sprite pack/editable sprite-directory override |
| `-snapshot <file>` | Save a rendered frame and exit; needs a graphics/display environment |
| `-legacy` | Older reference front end; requires `-client <original-WLRI-root>` |
| `-workbench` | Original artwork inspection; requires `-client <original-WLRI-root>` |

Normal client runs read `data/`; they do not need `-client`. Artwork/terrain
flags such as `-archive`, `-name`, `-list` and `-terrain` automatically select
inspection mode. Saved client state is separate under `var/client/user`.
See [CLIENT.md](CLIENT.md) for detailed inspection and packaging commands.

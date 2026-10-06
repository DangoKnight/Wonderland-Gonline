# Configuration and behavior controls

For initial setup, follow [GETTING_STARTED.md](GETTING_STARTED.md). Run from the
repository root so relative paths resolve consistently.

## Server JSON configuration

Copy `config.example.json` to ignored `config.local.json`, then pass
`-config config.local.json`. Omitted fields keep the built-in defaults. Unknown
fields and trailing JSON are rejected; use the supported keys below.

Startup parameters are the JSON-file exception in the
[server data policy](DEVELOPMENT.md#server-data-ownership-and-initialization).
Shared definitions belong in the assets database; mutable gameplay state and
persistent server settings belong in the gameplay database. Database defaults
may be seeded from JSON, but initialization must preserve existing records and
administrator edits. Compiled growth formulas remain in code.

| Key | Default | Meaning / constraints |
| --- | --- | --- |
| `name` | `Wonderland Gonline Server` | Initial server name, 1–200 bytes; a saved admin `server_name` takes precedence |
| `login_address` | `127.0.0.1:6414` | TCP login/game listener |
| `world_address` | `127.0.0.1:6415` | TCP world listener |
| `status_address` | `127.0.0.1:6416` | Native launcher status listener, separate from HTTP health |
| `status_server_ids` | `[1, 101]` | Status records advertised to launchers; 1–100 unique IDs, each 1–9999 |
| `http_address` | `127.0.0.1:8080` | Web administration, `/healthz` and `/register` |
| `database` | `var/wonderland.db` | Persistent Go accounts, characters, relationships, settings and audit |
| `assets_database` | `var/assets.db` | Read-only static catalog loaded at startup; must already exist |
| `combo_damage_per_participant` | `false` | Replace fixed ×1.3 combo damage with `1 + 0.1 × actual attackers` (players and pets); restart required; see [combos](COMBOS.md) |
| `pet_growth_formula` | `base_stats` | Automatic pet level-up weighting: `base_stats` or `combat_stats`; fixed until restart |
| `max_connections` | `512` | Maximum concurrent TCP sessions across the game/status services; accepted range 1–100000 |
| `idle_seconds` | `600` | Incoming-packet inactivity timeout during login, character selection and creation; 0 disables it, otherwise 1–86400 seconds |
| `world_idle_seconds` | `0` | Incoming-packet inactivity timeout after selecting or creating a character, including map loading and warps; 0 disables idle logout, otherwise 1–86400 seconds |
| `character_save_seconds` | `30` | Dirty recoverable session checkpoint interval, 1–3600 seconds; fixed until restart. Purchases/rewards and map transitions remain immediate |

Each incoming packet renews the timeout for the current phase. Server replies do
not renew it. By default, login allows ten minutes of inactivity and gameplay
has no idle logout. Existing `idle_seconds` values now apply only before gameplay.

Restart to apply JSON changes. `database` and `assets_database` must be separate
files, including when symlinks/hard links are involved. Relative paths are based
on the process working directory, not the config file's directory. There is no
runtime JSON/native asset fallback. Removed keys such as `item_catalog` and
`data_directory` are rejected.

Listener addresses use `host:port`; repeated identical addresses are rejected.
Port `0` asks the OS for a free port and is useful in tests, but the client has
fixed default game-port expectations.

### Elemental stat growth

Elemental growth is compiled gameplay tuning. Edit
[`internal/game/growth_parameters.go`](../internal/game/growth_parameters.go),
inside `DefaultElementalGrowth()`. The returned table has an explicit definition
for Earth, Water, Fire and Wind, each with ATK, DEF, MAT, MDF, SPD, HP and SP.
Every stat can use STR, CON, INT, WIS and AGI through `PointGrowth`.
Zero coefficients disable contributions; attributes include permanent avatar
bonuses. For example, Fire attack is defined as:

```go
ATK: StatGrowth{Level: 2, PointGrowth: PointGrowth{STR: 2}},
```

After editing, run the focused growth tests and rebuild, then restart the server:

```sh
go test ./internal/game ./internal/battle ./internal/server -run 'Test(ElementalGrowth|PlayerFighterConfiguredGrowth|CompiledElementalGrowth)'
make build
./bin/wonderland -config config.local.json
```

`elemental_growth` is not a supported JSON setting or administration control.
Remove it from old local configuration files; unknown fields are rejected.
Do not change `baselineGrowth()` or `nativeElementGrowth()` in
`internal/game/elemental_growth.go` when tuning gameplay. Those are fixed
references for aLogin's built-in formulas, used to calculate packet adjustments.

Combat stats use `round(level * level_coefficient + sum(attribute * coefficient))`
and add equipment afterward. Rounding uses ties to even, as the original server
does. Defaults:

| Element | ATK | DEF | MAT | MDF | SPD |
| --- | --- | --- | --- | --- | --- |
| Earth | 1.4 × level + 2 × STR | 2.6 × level + 1.75 × CON | 1.4 × level + 2 × INT | 2.2 × level + 2.2 × WIS | 1.6 × level + 1.8 × AGI |
| Water | 1.4 × level + 2 × STR | 2 × level + 1.75 × CON | 1.4 × level + 2 × INT | 2 × level + 2.2 × WIS | 1.6 × level + 1.8 × AGI |
| Fire | 2 × level + 2 × STR | 2 × level + 1.75 × CON | 1.6 × level + 2 × INT | 2 × level + 2.2 × WIS | 1.6 × level + 1.8 × AGI |
| Wind | 1.4 × level + 2 × STR | 2 × level + 1.75 × CON | 1.4 × level + 2 × INT | 2 × level + 2.2 × WIS | 2.1 × level + 1.8 × AGI |

These combat coefficients come from the saved WLRI Japanese wiki reference in
`docs/References/`, section “属性別パラメーターアップ”. Earth has the extra
DEF and MDF level contributions. Water uses the standard MDF coefficient; this
replaces the earlier custom Water MDF bonus. The reference also discusses
rebirth (+100 effective levels) and Water seal resistance; those mechanics are
outside this stat-coefficient change.

HP and SP use:

```text
round(base + multiplier * (
    level * level_coefficient + sum(attribute * coefficient)
    + level^level_power * sum(attribute * growth.coefficient)
)) + equipment_bonus
```

HP defaults: base 180, level coefficient 1, CON coefficient 2, level power 0.35,
`Growth.CON` 2. SP defaults: base 94, level coefficient 1, WIS coefficient 2,
level power 0.3, `Growth.WIS` 3.2. Every element uses multiplier 1. These values
come from `data/formula_data.json`, decoded from the WLRI `Formula.Dat`:
HP doubles at offsets 0xf9–0x111 and base at 0x16d; SP doubles at
0x119–0x131 and base at 0x16f. The original files and export have matching hashes.

```text
HP = round(2 × CON × level^0.35 + level + 2 × CON) + 180 + equipment_HP
SP = round(3.2 × WIS × level^0.3 + level + 2 × WIS) + 94 + equipment_SP
```

The earlier assumed Earth HP/Water SP 20% bonuses have been removed. HP/SP bases
and equipment remain outside the configurable compiled growth multiplier.
Runtime calculation uses compiled values; the JSON export is provenance and
independent test input, and is not loaded by server gameplay.

Coefficients must be finite, nonnegative and at most 1000; `LevelPower` must
be between 0 and 2 and vital multipliers must be positive. Growth validation tests reject formulas
that overflow the signed combat range at byte-sized levels and uint16 attributes.
Existing native uint16 stat/SP conversions and packet field limits still apply;
choose values within the original client's practical limits. Creation rejects
out-of-range starter vitals before saving a character.

Rebuilt coefficients apply to creation, player battle stats, allocation, equipment
changes, rest, progression, avatar changes and administration healing/reset operations.
On the next login, maxima are recalculated and saved; current HP/SP are clamped,
without healing. Pet and monster formulas remain independent. Native AC8 stat
packets carry adjustments relative to aLogin's built-in formulas, including
signed adjustments for reductions. Packet tests cover these adjustments;
interactive aLogin acceptance of custom formulas remains to be checked.

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
`max_connections`. Server operations can override the traffic color with
`green`, `yellow` or `red`; `auto` restores these thresholds. This choice is
saved in the gameplay database.

## Pet automatic point allocation

Set the formula in `config.local.json` before starting the server:

```json
{
  "pet_growth_formula": "combat_stats"
}
```

| Formula | Weights for STR / CON / INT / WIS / AGI |
| --- | --- |
| `base_stats` (default) | All five NPC template attributes; current pet base attributes if no template is available |
| `combat_stats` | Calculated ATK / DEF / MAT / MDF / SPD from the current pet, including level, permanent attributes, equipment/forging and existing pet elemental rules |

Each gained level allocates one point. For each point, the probability of an
attribute is `max(1, weight) / sum(max(1, eligible_weight))`. Attributes already
at uint16 capacity are excluded. If all attributes are capped, no draw occurs.
Weights are recalculated after the level increases and before each point, so
previous allocations affect subsequent draws in `combat_stats` mode. Current
HP/SP, maximum HP/SP, amity and temporary battle buffs/debuffs do not add weights;
this formula maps the five derived combat stats directly to their attributes.

The selection is captured when the server starts. Editing the file or saving a
new startup configuration does not change the running formula; restart to apply
it. Admin Operations and database-backed live settings cannot change this
selection. Battle rewards and `/petexp` share it. Manual AC8/AC68 spending and
`/petlvl` unallocated-point grants retain their existing behavior. Player stat
growth coefficients remain compiled parameters.

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
use Broadcast to send a notice to connected characters.

Accounts and Sessions provide live ban/GM controls, password reset, account
removal, session termination and audited mall-point adjustments. These operate
on gameplay state. Registration does not grant GM/admin access. Admin changes do
not rebuild static data. See [ADMINISTRATION.md](ADMINISTRATION.md) for the full
web panel, SQL content editors, and restart-only startup configuration controls.

### GM command reference

Grant a nonzero account GM level through Administration. Commands accept either
`/` or `:`; aliases share permissions and behavior. `/help`, `/cmd` and `/cmds`
list available commands. Public chat, Carnie and dismount commands remain public.
GM rights are checked on every invocation and online revocations apply immediately.

| Command (aliases) | Behavior |
| --- | --- |
| `heal` (`hp`, `full`) `[HP] [SP]` | Refill vitals or set requested vitals. |
| `gold` (`money`) `<amount>` | Set gold. |
| `item [add] <ID> [count]` | Grant an item from the SQL catalog. |
| `level` (`lvl`) `<level>`; `exp <total>` | Override player progression; direct EXP assignment grants no points. |
| `points` (`sp`, `statpoint`, `statpoints`) `<gain>` | Add available stat points. |
| `stats` (`stat`) `<STR CON INT WIS AGI>` | Set base attributes. |
| `skill <ID> [grade]` | Set a known skill's grade and reset its proficiency. |
| `allskills` (`maxskills`) `[grade]` | Grant the original element's authored skill list plus the starter stunt. Grade defaults to 1 and clamps to 1–10. Other learned skills remain. A missing SQL skill rejects the whole grant. |
| `god` (`godmode`) | Set all five player and selected-pet base attributes to 999 and refill vitals. This does not grant invulnerability. Pet caps follow normal stat calculation so saved, displayed and reconnected state agree. |
| `restat` (`resetstats`) `[character]` | Reset base attributes and refund points; see below. |
| `clearskills` (`resetskills`) `[character]` | Rebuild starter/stat-qualified skills; see below. |
| `repair` (`fixall`) `[character]` | Repair player equipment and inventory; see below. |
| `clearinv` | Empty the player's bag with native stack removals. Equipment and storage remain. |
| `tent` | Grant Tent item 34001 unless already carried. Tent interiors are still pending. |
| `buy <ID or name> [count]` | Purchase a normal mall entry with normal prices, quantities and atomic checkout. Names use case-insensitive substring matching. A known item ID outside the mall grants a free GM item. |
| `im` (`points_im`, `mallpoints`) `<gain> [character]` | Adjust account mall points with an audit record. |
| `pet <template ID> [name]` | Recruit a known NPC template and select it for battle. An owned companion keeps its progress; a reserved companion is restored; a hotel companion must first be retrieved. Optional names allow 1–16 printable bytes. |
| `amity` (`petamity`) `[0-100]` | Set selected pet amity; default 100. |
| `rebirth` (`petrebirth`) | Force selected pet rebirth, bypassing normal eligibility, resetting level/EXP and adding 50 points. Already reborn pets are refused. |
| `petlvl` (`petlevel`) `<level>` | Set pet level to 1–199 and reset EXP. Increases grant normal level points; lowering does not reclaim points. |
| `petexp <gain>` | Add pet EXP using normal level growth. |
| `warp` (`goto`, `tp`) `<map> [X Y]` or `<character>` | Warp to a known map or an online character. |
| `town <name>` | Warp to a legacy named destination. |
| `summon` (`bring`) `<character>`; `summonall` | Bring an online character or eligible online characters to the GM. |
| `invis` (`invisible`, `ghost`) | Toggle map visibility for this connection. Hidden GMs continue seeing other players; their own appearance, movement and map pet updates are suppressed. |
| `hide`; `unhide` | Explicitly hide or restore map visibility. |
| `mute <character> [minutes]`; `unmute <character>` | Persist or clear a chat mute. Duration defaults to 10 minutes; accepts 1–525600. All chat channels and chat commands are blocked while muted. Expiry is automatic and survives reconnects. |
| `jail <character> [minutes]`; `unjail <character>` | Save the mute and warp together. Jail is map 10000 at (600,600); release is map 10001 at (800,750). As in Private Server, mute expiry does not automatically release or confine the character. |
| `kick <character> [reason]`; `kickall [reason]` | Send a reason and disconnect. `kickall` preserves GM sessions. |
| `online` (`who`); `info` (`whois`) `[character]` | Report online characters or player/pet state. Explicit targets accept exact online names or character IDs. |
| `broadcast` (`b`, `notice`) `<message>` | Send a native GM notice to in-game characters. |
| `battle` (`fight`) `<monster ID>` | Start a PvE battle using a known SQL NPC template and the normal party battle path. |
| `winbattle` (`killall`, `killmonsters`) | Defeat the current battle's monsters and use normal victory rewards and quest continuation. Wait for any current animation to finish. Finished battles cannot award twice. |
| `exprate` (`experience`) `[multiplier]` | Query/set the persisted global battle EXP multiplier, 0.01–1000 (default 1). Saved to two decimals. Player and pet rewards are scaled once. |
| `droprate [multiplier]` | Query/set the live loot multiplier; see loot tuning below. |
| `reload [all\|quests\|mall\|drops\|gms]` | Reload selected SQL-backed runtime definitions or gameplay account GM levels. Also accepts `quest`, `itemmall`, `items`, `drop`, `gm`. `all` covers these four categories. Quest reload requires all in-game characters to finish loading/interactions and preserves existing spawn geometry, ground items and respawn timers. Invalid catalogs publish no changes; runtime reload never falls back to JSON/native files. Item/NPC definitions and startup config still require restart. |
| `shutdown [seconds]` | Schedule a graceful shutdown, default 10 seconds, clamped to 1–300. Notices precede connection cleanup and disconnect autosaves. A pending countdown cannot be rescheduled. |

Pet edits select the battle pet, then the active pet, then the first party pet.
Character/pet/inventory edits require the GM to finish active battles, trades,
events, storms and beach sequences. Targeted resets/repairs/jail also require the
target to be idle. State is saved before success replies. A failed reply to a
separate target closes that target for a fresh snapshot while preserving the GM.
The older healing command remains usable during battle as in the reference.

### GM item repair

GM accounts can use `/repair` or `/fixall` (also with `:`) to repair their own
items, or append an online character ID/name to repair that character. Repairs
clear damage on equipped items and bag stacks without spending gold or wrenches.
Counts, slots and forge metadata are preserved. Companions' equipment and stored
items are outside this command's scope. Missing targets fail explicitly.

Both characters must have finished loading and have no active battle, trade,
event, storm or beach sequence. Repairs persist before client updates; repeating
the command on healthy items makes no changes.

### GM attribute reset

GM accounts can use `/restat` or `/resetstats` (also with `:`), optionally followed
by an online character ID/name. The command sets each stored base attribute to
10, refills equipped HP/SP and returns `max(sum(base attributes) - 50, 0)` points.
Available points saturate at 65,535; feedback reports the points actually added.
Permanent avatar bonuses still apply to displayed attributes and do not count
as refundable allocations. Repeating a reset therefore grants no extra points.

The command preserves skills, level, EXP, items and currency. Both characters
must have finished loading and have no active battle, trade, event, storm or
beach sequence. State saves before client stat and character snapshots; missing
targets fail explicitly. This is a free GM operation and is separate from
player stat-reset items.

### GM skill reset

GM accounts can use `/clearskills` or `/resetskills` (also with `:`), optionally
followed by an online character ID/name. The command resets starter and skills
qualified by current attributes to grade 1 with zero proficiency. Other learned
skills, including quest rewards and grade evolutions, are removed. Quest progress
is preserved; completed skill rewards are not automatically granted again.

Stats, level, EXP, points, pets, items and currency remain intact. Both characters
must have finished loading and have no active battle, trade, event, storm or beach
sequence. Missing targets fail explicitly. State saves before the native skill
snapshot and refresh; removed skills are cleared from the client's indexed table.

## Static gameplay tuning

The running server uses a SQL-derived snapshot of `assets_database`. Typed
`catalog_*` tables are authoritative. Source records and old JSON views are
provenance; editing them does not change gameplay. Use the administration datasets
for validated edits, then reconnect/reload as directed by the panel. Direct SQL
edits require a restart or the relevant GM reload command. Use `-inspect-data`
to validate a snapshot before starting listeners. See [ASSET_DATABASE.md](ASSET_DATABASE.md).

| Behavior | Administration dataset |
| --- | --- |
| Daily Lucky Draw rewards | `LuckyDraw` |
| Gacha pack contents and probabilities | `GachaPacks` |
| Skill buffs, debuffs and protection | `Skills`, `SkillEffects` |
| Monster loot | `Drops` |
| Mall offers | `Mall` |
| Character starter grants | `StarterItems` |

Example reward-weight edit:

```sql
UPDATE catalog_lucky_draw_rewards
SET value_weight = 5
WHERE lucky_draw_rewards_ordinal = 0;
```

This changes that reward's weight to 5 while leaving its item, quantity and slot
intact. Weights are relative; this does not mean a 5% probability. Use the validated
`LuckyDraw` editor so its saved total is recalculated. Positive integer weights must fit the supported total. Daily Draw allows at most 20
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

### Live monster drop multiplier

A GM account can use `/droprate` (or `:droprate`) to query the current multiplier,
and `/droprate <number>` to change it. Finite values are clamped to 0.1–100.
The default is 1.0; restarting the server restores that default. This is a live
server-wide setting and affects subsequent victory loot rolls, including battles
already underway. Feedback goes only to the requesting GM.

Each eligible row uses `max(5%, configured_rate × 0.35 × multiplier)`.
Rates reaching 100% guarantee that eligible row; zero-rate rows stay disabled.
Only known items listed in the monster's native drop slots can drop. At most one
row succeeds per uncaptured monster, in authored order. Quantities, EXP, gold,
gacha rewards and Lucky Draw weights are unchanged.

### Values that need code changes

There is currently no server-config EXP multiplier or combo-probability slider. Shared fixed rules live in their owning Go packages;
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


### EXP reward scaling

`/exprate 2` doubles subsequent PvE victory rewards, including battles already in
progress. The Server operations view controls the same value. Midpoint-even
rounding matches Private Server: positive rewards give at least one EXP, zero
stays zero, and scaled rewards are capped at the native signed 32-bit maximum.
Pets receive the same once-scaled reward as their owners. Direct GM progression
edits and native EVE EXP assignments retain their authored values. This command
requires a nonzero account GM level; HTTP operations require the administrator
token. A failed database save leaves the active rate unchanged.

Server operations also persists the drop multiplier. `/droprate` retains the
reference's process-local behavior: it changes the live value until another
saved operations update or restart. To make a drop rate durable, save it through
Server operations.

### Combat content controls

NPC skill choices use the three skill slots in the SQL `npc.dat` records and
the effects in `skill.dat`; there is no separate AI skill-ID list or launch
switch. Palace stages use the optional SQL `combat_trials.json` table, managed
through the authenticated asset API. GM `/palace <stage>` and native AC77:1
share those definitions and admission rules. See
[ADMINISTRATION.md](ADMINISTRATION.md#palace-trials) for configuration and the
reference placeholder limitations. Empty definitions disable Palace challenges.

## Structured database upgrade

Server startup requires structured asset tables. Upgrade existing databases with
`go run ./cmd/database-migrate -config config.local.json -output-dir var/migrated-v1`,
then set `database` and `assets_database` to the verified copies. See
[ASSET_DATABASE.md](ASSET_DATABASE.md). These paths are startup parameters; content
definitions and durable settings remain in their respective SQL databases.

## Persistence policy and current scheduling

Purchases, trades, bank/item consumption, rewards and claims must commit in SQL
before success replies. Ordinary walking updates session memory and saves dirty recoverable fields
periodically, at disconnect and graceful shutdown. See
[transaction boundaries](DEVELOPMENT.md#transaction-boundaries-and-session-checkpoints).

`character_save_seconds` sets the startup interval (default 30 seconds). Omitted
fields keep that default; zero is rejected. Clean sessions skip SQL. Position-only
saves update X/Y with stale-position checks and do not rewrite owned inventory,
pets or quests. Committed purchases preserve pending walking in the online copy.
Warps remain immediately durable, and gathering keeps its one-second cadence.

With healthy checkpoints, a crash can lose up to one interval of ordinary movement;
failed saves extend that window and are logged/retried. Disconnect/graceful
shutdown attempts a final flush. Vehicle wear and resource consumption remain
immediate even when triggered by movement. Checkpoints still hold the world lock
through a bounded batch; multiplayer load/soak validation remains pending.

GM command coverage is also incomplete: source `/reborn` character class/reset
workflow is absent. Pet rebirth is a separate implemented operation. `/reload
quests` refreshes native EVE/visibility, not the missing custom game_quests registry.

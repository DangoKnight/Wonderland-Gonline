# Web administration

Start the server with `WONDERLAND_ADMIN_TOKEN` set, then open its configured HTTP
address (default `http://127.0.0.1:8080`). Enter that token. It remains in browser
memory until reload; every administration API route checks it. The public
registration endpoint creates ordinary accounts. Node is needed only for web
formatting and lint; the Go binary embeds the interface.

## Database ownership

Shared game definitions live in `assets.db` and are read-only to gameplay;
validated administrator edits are allowed. Accounts, characters, mutable player
state and persistent server settings live in `wonderland.db`. Startup connection
parameters remain in the server configuration file. Initialization must preserve
existing data and administrator edits. See the
[development policy](DEVELOPMENT.md#server-data-ownership-and-initialization).

JSON is API transport for the editors. Character and definition edits persist
in typed SQL tables. Content datasets use names such as `NPCs`, `Skills`, `Maps`,
`Mall` and `LuckyDraw`; they do not use source filenames or virtual documents.
The panel uses `/api/assets/definitions`. `/api/assets/documents` remains as a
URL alias, with the same typed dataset payloads and record IDs. See
[ASSET_DATABASE.md](ASSET_DATABASE.md) for upgrading an existing installation.

## Operations and diagnostics

**Server operations** edits the name, saved MOTD, EXP/drop multipliers, launcher
traffic color and log level. These settings live in the gameplay database and
survive restart. MOTD is shown once after native scene-ready synchronization and is limited to
255 encoded bytes. EXP defaults to 1 and accepts 0.01–1000. See
[CONFIGURATION.md](CONFIGURATION.md#exp-reward-scaling) for reward semantics.
Broadcast sends a native notice to connected characters. Maintenance controls
reload quests/mall/drops/GM privileges, checkpoint players, disconnect every
session, or schedule an announced graceful shutdown in 1–300 seconds.

**Live logs** displays the latest 500 entries at the selected log level. Refresh
to fetch new entries. Password/token attributes are redacted from the panel.
Existing `-log-file` diagnostics remain available for durable logs. **Audit
history** displays the latest 500 durable administration entries.

## Accounts and characters

**Accounts** lists account bans, GM access and normal/bonus mall balances. It
supports ban/unban, password reset, account deletion and audited point changes.
**Sessions** terminates a connection. **IP bans** adds/removes canonical IPv4 or
IPv6 addresses, immediately closes matching connections and rejects new ones.
IP bans and account bans remain separate controls.

**Characters** searches names or exact character IDs and pages through 500
records at a time. The full JSON editor exposes durable quests, timers, pets,
progression, avatar, location and inventory. **Inventory & equipment**, **Stats &
skills**, and **Player settings** provide editors limited to their fields.
Character ID, name and roster slot cannot change. Edits validate known content,
stack limits, skill grades and party state. Refresh after a conflict; an older
version cannot overwrite a later edit. Online edits/deletions require a loaded,
idle character and disconnect it so the next login receives a complete snapshot.

**GM studio** acts on a selected online character without granting that account
GM access. It provides healing (including the companion), gold/points/levels,
items, skills, repair/reset/god tools, pets/amity/rebirth, warp/summon, mall points,
invisibility, mute/jail, test battles, forced victory, NPC event triggers and
inventory cleanup. Character mutations keep gameplay interaction gates. Responses
show command feedback, including rejected requests. NPC show/hide and prop
open/close controls affect the current live map view; edit quest state in the
character editor when changing durable story progression.

**Friendships** removes durable pairs and informs online peers. **Live battles**
inspects fighter snapshots, finishes battles with normal victory rewards or
aborts them. An active animation must finish first.

## Social records and GM gifts

**Guilds** edits the name, announcement, leader and complete member list, or
disbands a guild. The leader must belong to the list and a character can belong
to only one guild. Character/account deletion cleans memberships and elects a
remaining leader, or removes an empty guild. **Marriages** inspects records,
annuls a marriage or brings online spouses together. These controls administer
stored records; client guild/marriage gameplay and their native synchronization
remain separate unported subsystems. No original account database is imported.

**GM mail & gifts** sends a message with optional gold and one item reward to a
single character, online characters or all listed characters. A dispatch is
atomic across its recipients. Subject plus newline plus body must fit the native
255-byte message. Gifts are claimed once in a transaction; insufficient bag
space or a gold limit leaves them pending for a later login. A lost connection
can replay the notice, but cannot award the gift twice. Deleting claimed mail
preserves the gift. Lists/dispatches are bounded to 500 characters; use explicit
recipient batches through the API for larger populations.

## Content editors

**Item mall catalog**, **Monster drops**, **Starter items**, **Chest loot** and
**Portals & destinations** edit the configured assets database. **SQL asset
editor** finds typed item/NPC/skill/talk/mark records by game ID. **NPC library**
inspects templates; **Dialogue resolver** searches text and resolves talk IDs,
byte offsets or record indexes, previews `#n` substitution and sends dialogue to
an online character. Map inspection exposes NPCs, authored events and warps.

Table editors send typed definitions as JSON API payloads. Monster drops are a
map from monster ID to ordered rewards with `Item`, `Name`, `Min`, `Max`, `Rate`.
Record lookup addresses a game ID rather than an import ordinal. Content saves
check a version, validate the edited dataset, preserve unrelated SQL edits and
commit typed rows in one transaction. Idle connected characters reconnect to
receive refreshed snapshots; active interactions and map loading block edits.
Existing maps cannot be removed. Opaque source map sections survive geometry
and event edits. Optional chest, trial and visibility datasets exist from migration
and start empty when not previously configured. Initialization endpoints only
verify availability. For skills with `effect_refs`, the referenced effects are
derived; set `effect_refs` to null when switching to skill-local `effects`.

Back up the assets database before content editing. Rebuilding it from extracted
assets replaces administrator changes, so preserve the edited database or export
those changes deliberately. The extraction inputs remain in the WLRI sibling.

## Startup configuration

**Startup configuration** reads the file supplied through `-config` and edits
listener addresses, SQLite paths, connection limits and idle timers. Saving
validates the complete configuration, rejects stale versions and replaces the
file atomically. Restart to apply changes; the current configuration is shown
separately. If startup used defaults, saving creates `config.admin.json`; start
with `-config config.admin.json` to load it. This port uses SQLite with GORM;
the original MySQL provider configuration is replaced by SQLite file paths.

Schema v7 automatically adds IP bans, guilds, marriages and GM mail tables without
resetting accounts or characters. A headless Chromium smoke renders all 22 new views and checks read-only
inspectors. Interactive native aLogin acceptance still requires manual validation; the automated checks cover API access,
transaction behavior, native packet layouts and affected gameplay paths.


## Optional quest actor visibility

The authenticated definition API exposes the SQL `QuestVisibility` dataset.
It exists after migration; its initial contents may be empty.
Read it with `GET /api/assets/definitions/QuestVisibility`; save with `PUT`
to that address, supplying the returned `version` and the definition array in
`value`. This is an operator-authored extension:
the reference quest loader does not populate actor lists. It creates no asset
file and imports no original account database.

Example PUT body (include the current version from GET):

```json
{
  "version": "<version returned by GET>",
  "value": [
    {
      "quest_id": 99,
      "map_id": 10017,
      "spawn_npc_click_ids": [1],
      "despawn_npc_click_ids": [2],
      "steps": [
        {"step": 2, "spawn_npc_click_ids": [3]}
      ]
    }
  ]
}
```

Quest-level lists activate on `Completed`; step lists activate on `InProgress`
at that exact step. Actor IDs are scene click IDs. Ordered rows preserve the
reference's first matching decision; an inactive spawn list hides its actor.
Invalid maps, actors, duplicate quest/step IDs and steps outside 1–255 are
rejected. Successful edits use the existing validation, catalog publication and
player reconnect procedure. `/reload quests` also reloads this collection.
Back up the assets database to preserve these authored rows across rebuilding.

## Palace trials

Palace trials use the `CombatTrials` SQL dataset, available after migration. GET and PUT it through the same versioned definition API
described above. No file under
`data/` or launch configuration is required. Stages are unavailable until an
operator supplies valid content.

The reference's guardian IDs 1001–1012 are absent from WLRI, and reward IDs
48030–48033 are vehicle capsules. Go does not install those placeholders or
copy the reference's immediate reward without a fight. Choose actual NPC,
location and reward IDs from the administration asset inspectors.

Example stage definition, using a WLRI Aries fighter and a recovery pack:

```json
{
  "version": "<version returned by GET>",
  "value": [
    {
      "stage": 1,
      "name": "Aries Palace",
      "map_id": 10017,
      "guardian_id": 20161,
      "hp": 15000,
      "attack": 450,
      "reward_item_id": 35114,
      "reward_count": 1
    }
  ]
}
```

This is an example, not an official stage/reward table. Choose the intended
trial map before saving. Stage numbers must be unique and between 1 and 12;
NPCs, maps and items must exist in the current SQL catalog. Guardians must
have positive native HP and level. Boss HP/attack must be positive signed
32-bit values, and reward counts must be between 1 and 50. Admission uses the
item's actual stack limit and available bag slots. Invalid edits
roll back. Successful administration edits publish the catalog and disconnect
online characters for coherent re-entry, as with other asset edits.

Native requests use `77,1,stage`; replies use `77,1,stage,accepted`, where
accepted is 1 or 0. GM `/palace <stage>` uses the same admission checks. The
initiator must be ready and idle on the configured map. Nearby available
teammates join through ordinary party battle rules; every participant needs
room for the reward before admission. Each winner receives the reward after
victory, before ordinary loot can occupy that space. Defeat, flee and repeated
settlement never grant a chest. Inventory/reward state saves before success
packets. The selected definition is copied for the encounter; `/reload quests`
or `/reload all` loads edited SQL stages for future encounters.

### Economy and guild gameplay

The Economy definition dataset edits manufacturing recipes, synthesis chances,
gathering pools and marriage requirements/fees/rings. These values live in typed
asset SQL tables. See [ECONOMY_SOCIAL.md](ECONOMY_SOCIAL.md). Guild edits preserve
member ranks and publish refreshed native badges/rosters to online players.
Gameplay uses the same guild and marriage records displayed by Admin.

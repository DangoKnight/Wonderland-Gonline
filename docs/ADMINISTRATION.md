# Web administration

Start the server with `WONDERLAND_ADMIN_TOKEN` set, then open its configured HTTP
address (default `http://127.0.0.1:8080`). Enter that token. It remains in browser
memory until reload; every administration API route checks it. The public
registration endpoint creates ordinary accounts. Node is needed only for web
formatting and lint; the Go binary embeds the interface.

## Operations and diagnostics

**Server operations** edits the name, saved MOTD, EXP/drop multipliers, launcher
traffic color and log level. These settings live in the gameplay database and
survive restart. EXP defaults to 1 and accepts 0.01–1000. See
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
editor** finds indexed item/NPC/skill/talk/mark records by game ID. **NPC library**
inspects templates; **Dialogue resolver** searches text and resolves talk IDs,
byte offsets or record indexes, previews `#n` substitution and sends dialogue to
an online character. Map inspection exposes NPCs, authored events and warps.

Table editors use JSON; monster drops retain the `TID:monster | item,name,min,max,
percent` text schema. Indexed record JSON is authoritative. Content saves verify
the record version, reindex edited documents and load/validate the entire
candidate SQL catalog inside the transaction. Failed validation rolls back.
Success publishes a new catalog and reconnects loaded players for fresh native
snapshots. Finish ongoing battles, trades, event scripts and map loading first.
Existing maps cannot be removed. There is no runtime native/JSON fallback.

Map overrides and chest pools are optional **SQL documents** initialized by the
panel, named `map_overrides.json` and `chest_drops.json`. They do not create files
under `data/`. A map override contains its complete parsed map, including warps,
NPCs, events and resources; original binary sections/provenance remain in imported
SQL rows. Chest pools select a map or match an NPC name category, list positive
weighted rewards and set a per-character cooldown of 1–86400 seconds. Matching categories take priority over map pools, followed by
`default_chest`, matching the reference manager. Categories `medicine`,
`headband` and `ore` retain the reference NPC-name aliases. Unconfigured chests retain authored rewards.
Configured chests choose their reward before capacity checks, persist reward and
cooldown together, and close their prop at expiry or reconnect.

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

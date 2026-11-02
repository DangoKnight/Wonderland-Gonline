# Server components

`server` owns the TCP services, sessions and handlers that coordinate live
gameplay. Related helpers and integration tests share a feature file instead of
creating a file for every command alias or small adapter. The root has 34
implementation files and 39 integration-test files, down from 238 Go files.

Independent components have their own Go packages:

| Package | Responsibility |
| --- | --- |
| [logging](logging/buffer.go) | Bounded administrative log capture, runtime levels, snapshot isolation, redaction and bounded packet traces. Owns its synchronization. |
| [wire](wire/events.go) | Server packet builders grouped by events, messages, commerce, companions, world, housing and mail. Reads supplied snapshots; performs no database operations or socket writes. |

Both packages are dependencies of `server`; neither imports it. Existing
`server.Server`, `server.Session`, `server.SessionInfo`, `server.LogEntry` and
constructor APIs remain available to the executable, administration API and
fixture tools. `LogEntry` aliases the logging component's record type.

## Finding a handler

| Area | Implementation |
| --- | --- |
| Listeners, connections and command registration | [server.go](server.go), [session.go](session.go), [commands.go](commands.go) |
| Characters, skills, potential and rebirth | [characters.go](characters.go), [progression.go](progression.go) |
| Map entry, movement, encounters and recovery | [world.go](world.go), [simulation.go](simulation.go), [travel.go](travel.go) |
| EVE execution and SQL story policies | [events.go](events.go), [story.go](story.go), [props.go](props.go) |
| Battles, rewards and companion controls | [combat.go](combat.go), [combat_rewards.go](combat_rewards.go), [pets.go](pets.go) |
| Inventory, crafting, fishing and manufacturing | [inventory.go](inventory.go), [crafting.go](crafting.go), [fishing.go](fishing.go), [manufacturing.go](manufacturing.go) |
| Trading, stalls, bazaars, NPC shops and paid games | [trade.go](trade.go), [shops.go](shops.go), [mall.go](mall.go), [minigames.go](minigames.go) |
| Chat, friends, guilds, mail, teams and housing | [chat.go](chat.go), [social.go](social.go), [teams.go](teams.go), [tents.go](tents.go) |
| Administration, definitions and live character edits | [admin.go](admin.go), [admin_assets.go](admin_assets.go), [admin_players.go](admin_players.go), [observability.go](observability.go) |
| Runtime settings and GM commands | [settings.go](settings.go), [gm.go](gm.go), [gm_progression.go](gm_progression.go), [gm_content.go](gm_content.go) |

## Ownership and extension

The root remains one package because handlers coordinate private session state
under `worldMu`, alongside the existing account/catalog/administration locks.
Session fields, transport writes and checkpoint adoption live in `session.go`.
Moving a handler between files does not change its locking or transaction
contract. Do not expose session fields or add callbacks between features solely
to put an existing method in a different directory.

Add command bindings and interaction policies in `commands.go`, behavior in the
owning feature and reusable serialization in the corresponding `wire` file.
Extract another package when it has a narrow boundary and can own its state
without depending on `Server` or mutable `Session` internals. Extend the existing
feature file for small related helpers; split a feature when its responsibilities
actually diverge.

Pure packet golden tests and trace checks live with their component. Tests that
need shared session fixtures, persistence or multiple players stay in the root,
grouped by feature. Native-story and SQL publication tests remain separately
named. Independent raw-byte expectations and authored tables are preserved.

See [development conventions](../../docs/DEVELOPMENT.md),
[story authoring](../../docs/STORY_SYSTEM.md) and
[testing commands and results](../../docs/TESTING.md#server-component-layout--2026-11-01).

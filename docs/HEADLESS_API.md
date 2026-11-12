# Structured headless API (version 2)

The native Go client runs the API and spectator transport directly, without a
browser, WASM, HTTP server or WebSocket gateway. `cmd/wlo-agent` is a separate,
optional browser backend. Both use the same application commands, snapshots and
exclusive-control dispatcher. Gameplay commands invoke
application actions directly. No mouse emulation, synthetic keyboard input,
screen coordinates, screenshots or JavaScript evaluation are required.
Build/start instructions are in [the host guide](HEADLESS_AGENT_GUIDE.md).

## Server permission

Game servers deny agent/API-controlled gameplay by default. The server operator
must explicitly enable **Administration → Operations → Agent / gameplay API
access**. This persists as `agent_api_enabled` in the gameplay database's settings
table; missing values are seeded as `false`, preserving existing operator edits.
Disabling it takes effect immediately and disconnects declared agent sessions
with an explanation that this server specifically disallows agentic AI/API use.
Manual clients, including Legacy clients, are unaffected. Administration itself
remains a separately authenticated operator interface.

Hosted clients declare `ConnectionAgentAPI` during the connection-wide hello and
wait for the server's explicit confirmation. API commands, raw hosted-client game
writes, gameplay queries and spectator data are blocked without that permission.
Reconnects require a fresh grant. Older Gonline/Legacy servers without this feature
cannot authorize API gameplay. Offline setup, server selection, contract/status
queries and local session teardown remain available to establish a connection or
explain a refusal; an API listener starting successfully is not a gameplay grant.

This is enforced negotiation and policy, not proof that a human is operating a
client. A modified client or external automation that imitates ordinary Legacy
traffic cannot reliably be distinguished at this protocol layer. No AI-detection
or tamper-proof-client guarantee is made.

## Native host (no web functionality)

Build the regular Go client, then explicitly enable its native listener:

```powershell
cd client
go build -o ../bin/wonderland-client.exe .
cd ..
$env:WLO_AGENT_TOKEN = [guid]::NewGuid().ToString("N") + [guid]::NewGuid().ToString("N")
.\bin\wonderland-client.exe -headless -api-listen 127.0.0.1:9000 -assets bin/client-assets.zip
```

The native client simulates the game with the CPU renderer, does not open a game
window or audio device. Native spectators render locally from its ordered packet/action stream. Supply
`-serverini` to select another server list. Missing assets produce a startup error;
headless startup never opens the interactive asset-setup prompt.
Ordinary client startup does not enable an API listener. Both `-headless` and
`-api-listen` are required, together with the private token.

Connect directly to the TCP listener and exchange newline-delimited JSON objects.
The first request authenticates the connection (replace the token placeholder):

```json
{"jsonrpc":"2.0","id":1,"method":"auth","params":{"token":"<WLO_AGENT_TOKEN>"}}
{"jsonrpc":"2.0","id":2,"method":"agent.state","params":{"session":1}}
```

After authentication, the gameplay RPC methods below have the same command and
query payloads as the web backend. The native agent ID is `native`; `id` may be
omitted. One native process owns a workspace of sessions. `agent.create` returns
that workspace and recreates its first session if all were closed; use
`workspace.action/add` for additional sessions. `agent.destroy` closes all game
sessions and revokes viewer tickets while leaving the authenticated API listener
alive. Session IDs are not reused. API/viewer disconnections never imply destroy.
Window/display previews and browser diagnostics are unavailable in this mode.

`agent.viewer.open` returns a scoped **`url`** such as
`tcp://127.0.0.1:9000/#<ticket>`. Start another regular native Go client:

```powershell
.\bin\wonderland-client.exe -attach "tcp://127.0.0.1:9000/#<ticket>"
```

The spectator opens as a regular Gonline client workspace session and starts readonly.
It uses normal asset setup and saved client settings: fullscreen/window mode,
frame limit, scaling, language controls and session management. The session card
provides **Take control** and **Return to API**. No HTML, browser
or web endpoint is involved. The optional browser backend instead returns an
HTTP viewer `path` as described below; a browser cannot attach to the native TCP
listener. The native viewer supports either transport by its URL scheme.

## Persistent session host and detachable viewer

The host keeps the canonical game client and its connection alive independently
of RPC transports and viewers. An optional native Gonline client or Vue browser
viewer watches the same session and can temporarily take exclusive input control.
There is no second game login. The native spectator uses the regular client assets;
the browser viewer receives streamed images without extracting client assets.

For the optional web backend, call `agent.viewer.open` with `{id,session}` (an explicit positive session ID).
The result contains `path`, a scoped viewer URL path valid for one hour. Prefix
it with the RPC listener's HTTP origin, for example `http://127.0.0.1:9000`.
Open that complete URL in a browser, or launch a built native client:

```powershell
.\bin\wonderland-client.exe -attach "http://127.0.0.1:9000/viewer/#<ticket>"
```

The browser shows **Take control** and **Return to API**. The native client exposes
those actions on its spectator session card. Attaching and reconnecting start readonly.
The ticket permits viewing and requesting intervention for this one session;
keep it private. It is exchanged in the first WebSocket message, not in an HTTP
request URL or referrer. Mint another ticket after expiry.

Native Go spectators receive ordered server packets and accepted agent/UI actions,
not screenshots or browser output. Each spectator runs the normal map, sprite,
battle, dialogue and sound code using its local assets and display settings. Its
game sockets are disabled: replay cannot log in again or send duplicate purchases,
combat choices or acknowledgements. Agent input remains authoritative at the host.
Rendering runs at the spectator's configured frame rate, independently of transport
updates. Use matching client builds and compatible extracted assets on both ends.

A newly attached native spectator catches up from a compressed, in-memory history;
input stays blocked while it catches up, and historical audio is muted. A reconnect
resumes from its last applied frame. History has a 128 MiB safety limit per hosted
session; exceeding it reports an error and requires a fresh hosted client session.
Long-running sessions can therefore take longer to attach. No game history or
viewer capability is written to disk. Login credentials are redacted from actions.

The optional Vue browser viewer still uses complete 800x600 JPEG snapshots at up
to eight frames per second. That browser-only path does not stream audio. A native
Go spectator attached to the web backend requests the native event stream instead.
API gameplay uses direct semantic commands without mouse emulation.

The native spectator shares the regular client's window, renderer, preferences
and session sidebar rather than running a separate viewer shell. Saved normal
sessions remain available; closing a spectator only detaches it from the host.
Spectator tickets and temporary spectator sessions are excluded from workspace
profiles. Local Settings and sidebar interactions never become hosted gameplay
input. The replicated scene retains hosted UI state; the viewer's
local settings govern its own window and presentation.

The session host must own the game TCP connection, login/character state,
protocol processing, timers and authoritative client-side session state. Neither
an API transport nor a viewer connection owns that lifetime. Attaching a viewer
must not create another game login, transfer the game socket to the viewer, or
restart the session. A viewer disconnect/reconnect must leave the host's game
connection and existing API connections operational.

The attached client is a presentation/input frontend for the hosted session.
Attaching starts with a consistent presentation snapshot followed by ordered
updates, including any current battle, dialogue, menu or pending confirmation.
Reconnecting replaces the old subscription and resynchronizes from current state;
it must not replay gameplay packets into the game server or duplicate rewards.
This is viewing one's hosted session, separate from in-game battle spectating.

Each hosted game session must have exactly one gameplay-input owner:

| Owner | API | Attached client |
|---|---|---|
| API | Gameplay commands and queries allowed | Readonly viewing |
| Attached client | Queries/observations allowed; gameplay commands rejected | Human intervention allowed |

Ownership is enforced in the session's command dispatcher, not merely by disabling
buttons in the viewer. Switching owners must be atomic and expose an ownership
generation. Queued actions from an earlier generation are rejected/cancelled;
held movement, target selections and other unsubmitted input are cleared.
Client AUTO/Remote action producers must obey the same ownership boundary and
cannot continue submitting competing actions during human intervention.
Actions already sent to the game server remain sent; handoff must never resend
or attempt to reverse them. Readonly queries remain available in both modes.

The viewer policy is readonly attachment by default, explicit takeover
and release, and a bounded controller lease. A lost controlling viewer revokes
its lease, clears unsent input and returns control to the API with a new
generation. Old API queues are not resumed or replayed. Late viewer messages and
messages from a replaced connection are rejected. Ownership changes and viewer
attach/detach events must be observable through the API.

`agent.state` exposes `details.control = {owner,viewer?,generation,viewers}` and command
history includes accepted/executed/rejected control tickets. Gameplay requests may
include `generation` to reject stale API work across an intervention. A controlling
viewer sends one-second heartbeats; its five-second lease is revoked if it stops.
Disconnect normally releases it immediately. AUTO, Remote, vacuum collection,
held keys/dragging, pending walking interactions and target selections are cleared
on either handoff; automatic producers remain suspended during intervention.
Release never restarts them or replays cancelled work. Already-submitted server
actions continue normally. Workspace changes are blocked while a viewer controls
any session so another API session cannot reset that viewer's held input.
Taking control of an AFK session requests its full scene; human input waits for
the normal server resynchronization. Releasing control does not re-enable AFK.

Required acceptance checks include API-only operation; attaching/detaching during
walking, dialogue, transactions and combat; simultaneous takeover attempts;
queued commands crossing a handoff; held-input cleanup; stale viewer generations;
controller loss/reconnect; and proving that viewer loss neither closes the game
socket nor interrupts the existing API connection. Reconnection must restore the
same hosted character and current state, rather than perform another login.

## RPC contract

For the optional web backend, connect to the loopback `/rpc` WebSocket using
`Authorization: Bearer <token>`. The native backend uses the TCP authentication above.
Each request has the standard JSON-RPC 2.0 envelope. One browser agent can own
multiple workspace sessions. Positive session IDs select exact instances;
omitted/zero session selects the currently active instance. Scripts should use
explicit session IDs, especially while adding or switching instances.

| Method | Params and result |
|---|---|
| `agent.create` | `{}` creates a browser and returns `{id}`. Credential parameters are rejected. |
| `agent.list` | `{}` lists browser IDs and creation times. |
| `agent.destroy` | `{id}` closes that browser and all its sessions. |
| `agent.viewer.open` | `{id,session}` returns a scoped TCP `url` on native hosts or HTTP `path` on web hosts, session ID and ticket lifetime. Does not change input ownership. |
| `agent.contract` | `{id}` returns API version, commands, argument types/ranges, prerequisites, operation variants, query schemas and control payload schemas. |
| `agent.execute` | `{id,request:{session,command,arguments,context?,generation?,timeoutMs?}}` returns a command ticket. |
| `agent.commands` | `{id,session?}` returns queued, executing and retained results. |
| `agent.cancel` | `{id,session?,ticket}` cancels a command that has not begun executing. |
| `agent.state` | `{id,session?}` reads a fresh client snapshot. |
| `agent.observe` | `{id,session?,pointer,equals,timeoutMs}` waits for a JSON Pointer to equal a JSON value without sending gameplay requests. |
| `agent.lookup` | `{id,session?,kind,entry}` reads an item, skill, NPC or known quest definition from client assets. |
| `agent.navigation` | `{id,request:{session?,query,...}}` reads collision grids, short-range paths or land/water eligibility. |
| `agent.screenshot` | `{id}` captures the browser for optional visual diagnostics. |

Normal server startup enables none of these endpoints. The host requires
`-enable`, numeric loopback binding and a bearer secret of at least 32 bytes.
Host/origin checks protect the local asset server and game gateway.
`-diagnostic-input` enables trusted `agent.eval` diagnostics. The historical
`agent.command` synthetic-input bridge remains disabled for hosted sessions;
human input must pass through viewer ownership. `-browser-log` optionally logs browser console
output, which may include gameplay text. Chrome's sandbox remains enabled.

`-game` scopes the gateway to the configured server's consecutive login/world/
status ports. Native logical ports 6414/6415/6416 map to that family even when an
asset pack names another server. Other service ports are rejected. For example,
`-game 127.0.0.1:6714` maps those services to 6714/6715/6716.

## Execution and completion

Admission validates exact argument sets, types, ranges and structured payloads.
Each session owns its bounded game-loop queue: 64 pending commands, eight per
frame, 128 retained results. Default pending timeout is ten seconds, with a
30-second maximum. Reconnect/disconnect cancels queued work and retires handles.
States are `accepted`, `executing`, `executed`, `rejected`, `cancelled` and
`timed_out`. Cancellation cannot undo an action already executing.

**Executed means local dispatch, not authoritative server success.**
`serverConfirmed` remains false. Mall checkout, for example, dispatches a balance
preflight that can subsequently fail. Observe inventory, balances, party state,
forms and notices for the outcome. Results include `afterSequence`;
`details.events` contains bounded, monotonic received-packet metadata without raw
payloads. This distinguishes fresh traffic; it is not a transaction receipt.
Legacy protocols do not provide universal request IDs. Never blindly retry a
purchase, trade, paid game, reward or item consumption after a timeout.

`walk.to` also accepts an asynchronous raft boarding or landing request before
the character starts walking. Observe position, vehicle and `waterTravel` state
for completion. After a warp, `details.mapLoading` identifies the loading
transition and `details.sceneFrozen` covers transition/fade input locks;
`details.mapReady` alone does not mean those presentation locks have ended.

Supply the current `details.context` as request `context` to fence an action to
the observed character, map and battle turn. Item commands also require the
expected item ID beside the slot. Credentials are omitted from state/results;
do not log login request arguments in your orchestrator.

## Forms and editing

`state.forms[].controls[]` describes visible controls with an opaque `id`,
content `revision`, semantic `name`, `kind`, `enabled`, available `actions`,
`payloads`, text and structured value. Built-in controls expose `activate`,
`set`, `select`, `open`, `remove`, `submit`, `commit` or `alternate` where native
callbacks exist. Password/account values are redacted. Readonly fields,
disabled ancestors and another modal owner block actions.

```json
{"jsonrpc":"2.0","id":10,"method":"agent.execute","params":{
  "id":"agent-...","request":{"session":1,"command":"ui.invoke","arguments":{
    "id":"<handle>","revision":"<current revision>",
    "action":"set","value":"Message text"
  }}}
}
```

Fetch a fresh node after changing its value/selection. Hidden, removed and
disconnected controls lose their handles. Another session cannot use them.
Root revisions include relevant resource snapshots to prevent an old selection
silently acting on a different item. Payload descriptions use `type`,
`properties`, `required`, `items`, exact fixed-array `length` and `nullable`.
This is a compact contract, not the complete JSON Schema specification.

## Coverage

The running contract defines exact argument spellings and limits. Form/list
indexes are zero-based; inventory slots retain their native one-based numbering.

| Workflow | Direct entry and state |
|---|---|
| Login/lifecycle | `server.connect`, `login.submit`, `character.select`, creation wizard controls, `session.logout/reconnect`; explicit confirmation/password forms remain required. |
| Movement/interaction | `walk.to/stop`, `world.interactNPC`, `world.pickup`, `world.target`, `pose.set`, `selection.cancel`; world coordinates and visible entity IDs. Vacuum collection starts through its equipped tool/NPC and stops with `collection.stop`. |
| Navigation | Query `grid` with radius 1–100 collision cells; `overview` with step 1–64; `plan` or `terrain` with world `x,y`. Cells span 20 world pixels; grids encode bytes as base64. Overviews cap at 262,144 cells. |
| Dialogue/quests | `dialogue.advance/answer`, `talk.lines/options`, event state, known quests and notebook controls. |
| Inventory | `inventory.use/unequip`; inventory form actor/use/equip/move/drop/allocation actions and destruction/quantity confirmations. |
| NPC services | `npcService` select/page/confirm/deposit/withdraw/close; shop, sale, storage, treatment and hotel normal callbacks/transactions. |
| Pets/teams | Team form pet mode, dismissal, rename, leadership, kick and leave; `world.target` requests and confirmation forms. |
| Combat | Attack/skill/item/defend/capture/flee/AUTO, pet changes through Team, Join Battle, spectating, traditional PK and `battle.leaveSpectating`. |
| Remote | `remote.configure/start/stop`; full independent player/pet advanced arrays, thresholds, supply ranges and exclusions; nested assignment forms. |
| Vehicles/housing | Inventory use/placement, `vehicle.board`, water-aware movement; `housing.action` entry/exit/close/place/move/floorSwitch with room cells and expected furniture IDs. |
| Potential/cards | Target, confirmation, explicit skill-loss warning and reward choices through their owning forms. |
| Compound/trade | Ingredients/tier/synthesis; trade offer/withdraw/confirm/accept/close and editable gold. |
| Social/chat/mail | Friends/profile/emotes/phrases, `chat.send`, mailbox actions, editable compose/read/reply forms. |
| Boards | Board page/read/compose/close and subject/body/confirmation/reply/delete controls. |
| Mall | Currency/category/page/cart/quantity/checkout, structured bag contents, fitting, forge selection/confirmation and game entry. |
| Minigames | `minigame.action` uses the current game session and published actions: memory choices, dream/mole strikes, hunter/pumpkin shots, catch position, dodgeball actions, fighter controls and paid-slot confirmations. Hidden digits/future outcomes remain hidden. |
| Notebook/draw | Book type/page/selection/scroll; Lucky Draw state/rewards and native Spin/preview/close controls. |
| Settings/workspace | Local settings/permissions; display preview/confirm/revert; workspace add/switch/close/title/collapse; hotbar assign/use and AFK mode. |
| Notices | `notice.dismiss`, `battle.resultDismiss`, scrollable result controls. |

`menu.open/close` module names: `inventory`, `skills`, `compound`, `team`,
`social`, `settings`, `mall`, `worldPK`, `notebook`, `mailbox`, `business`,
`luckyDraw`. Remote and reward forms open from their normal owning items.

The API covers existing native functionality. World PK currently provides its
presentation/settings with matchmaking/reward buttons disabled. Unimplemented
pet/furniture stalls and guild-specific forms remain unavailable. Browser
window/fullscreen changes remain subject to browser capabilities. Such controls
have no supported action or reject it; the API fabricates no success/resources.

## State and persistence

Snapshots include player/map/peers/NPCs/areas, bag/equipment, pets/hotel, stats,
battle, talk, quests, shop, forms and sessions. `details` adds map readiness,
interaction hold, party/vitals, Remote/options, AUTO/spectating, learned skills,
hotbar, housing, collection/timers, creation, settings/display, minigame, notices/chat, context and
packet observations. Displayed names/text are UTF-8. Binary grids and retained
native byte arrays use JSON base64.

Small client preference/history documents use atomic private desktop files and
origin-scoped localStorage in WASM. This supports Remote configuration, settings,
workspace preferences and mail without relying on unavailable browser OS files.
Storage failures are reported. Chrome agents use temporary profiles: destroying
an agent does not preserve its local documents for a new agent. Persist required
configuration in the orchestrator and restore it explicitly. Existing workspace
preferences may remember account names; passwords, deletion codes and
server-owned gameplay resources are not saved by this adapter.

Focused tests cover strict/malformed input, ownership/stale/context fences,
queue bounds/cancellation/expiry/in-flight visibility, redaction, native battle/
shop packet goldens, transaction pending gates, Remote arrays, minigame rules
and GPU mailbox rendering. Race checks cover the queue and host boundaries.
Live isolated tests use two real WASM sessions with input diagnostics disabled
to check login/world entry, character creation/redaction, inventory, chat, teams,
friendship, mail, storage, shop purchases, a complete PK combat round/escape,
Remote persistence, navigation, AFK resynchronization and logout/relogin.
Additional live checks attach competing readonly viewers, transfer ownership,
disconnect a controller holding a movement key, and reconnect during an active
two-character fight while retaining the original API WebSocket and game login.
Paid rewards, every quest path and every minigame are not
exhaustively live-tested; their native/protocol tests and server rules remain.

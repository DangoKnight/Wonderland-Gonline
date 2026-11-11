# Headless agent notes

Working notes for `cmd/wlo-agent` and the `window.wlo` automation API.
This file and `docs/headless-agent-screenshots/` are temporary research
artifacts and can be removed once the feature is stable.

## Architecture

- `cmd/wlo-agent` starts headless Chrome through chromedp and serves the host
  page, the WASM client, the asset packs and a `/ws` gateway on one loopback
  port.
- `client/web/agent_api.go` exposes `window.wlo`:
  - `state()` returns a JSON snapshot of `app.Client` / `world.World`.
  - `command(name, ...args)` dispatches synthetic input.
  - `screenshot()` returns the Ebiten canvas as a PNG data URL.
- JSON-RPC over WebSocket lives at `/rpc` on the agent. The game gateway
  uses `/ws` on the asset server.
- Screenshots are taken with `chromedp.FullScreenshot`, not
  `canvas.toDataURL` (the canvas capture came back blank in headless mode).

## Key findings

- The WASM client dials the game server through a WebSocket at
  `/ws?target=host:port`. The agent must run a gateway for `/ws`, otherwise
  the client cannot connect and the server selection screen never advances.
- Native client resolution is 800x600. With a 1020x600 viewport the game is
  rendered 1:1 in the left 800x600 area; the remaining 220 px are the Ebiten
  workspace side panel. Synthetic mouse coordinates are the logical 800x600
  coordinates.
- `seui` updates `Input.Hovered` only in the Draw pass. A click must therefore
  dispatch `mousemove`, then `mousedown`, then `mouseup` in **different
  frames**. `window.wlo.command("click")` uses 300 ms timers between the
  steps for this reason. Sending all three synchronously makes the click land
  on a stale control.
- CDP `Input.dispatchMouseEvent` and JS `MouseEvent` both work once the
  events are separated by frames.
- `chromedp.FullScreenshot` gives a 1020x600 image; `canvas.toDataURL` is
  blank.
- Asset packs: the core `_packs.json` manifest selects the exact companions,
  including audio. Stale bundles are ignored.
- Login and character-creation text fields are limited to **10 bytes**
  (`fieldMaxBytes`). A longer password is silently truncated and the login
  fails with `LoginSelectAlternate`.
- The character-creation "Set password" page asks for the **account
  password** (the server validates it with `Store.Authenticate`), plus a
  secret code of at least 6 characters. The password and code must differ.
- The character-creation wizard requires all 5 attribute points to be spent
  before `Next` advances from the stats page.
- The HTTP admin/registration port is **8081** on this host; host port 8080
  belongs to qBittorrent.

## Agent commands

Besides synthetic clicks, the agent exposes direct client commands:

- `walkWorld(x, y)` calls `Client.AgentWalkWorld`, the same walk planner used
  by a ground click, but with world coordinates.
- `interactNPC(id)` calls `clickNPC` for the NPC with that click id.
- `pickupFloor(slot)` calls `pickupFloorItem` for a ground-item slot.

The state also exposes `camera`, `mapSize`, `areas`, `floorItems`, `talk`
(lines/options/choosing), `eventActive`, `eventDone`, `movieActive`,
`walking`, `waterTravel`, `vehicle`, `pets`, `companions`, `bag`, `equipment`
and `quests`. `pets` is the authoritative party/hotel roster
(`{"slot","id","name","level","hp","maxHP","sp","maxSP","amity","potential"}`).
`bag` and `equipment` carry item
names (`{"slot","id","count","name"}`), and `quests` carries
`{"id","name","description","completed"}`. When a dialogue offers options,
`talk.options` lists the text and `talk.optionPoints` lists the screen centre
of each row, so an agent can click a choice directly. `areas` identifies the
island exit: on Newbie Island the warp is area id 11 at x 3320..3420,
y 2040..2140.

`tools/headless-agent/examples/wlo.example.py` (copied to `session/wlo.py` for a session) also has `choose(index)`, `bag()`,
`equipment()`, `quests()`, `ascii_grid()` and `ascii_map()` helpers.

## Collision grid and autopilot

`window.wlo.grid(radius)` returns a JSON walkability grid centred on the
player: 20-pixel cells, `radius` cells in each direction, `cells` as a
base64 byte array (1 = walkable). `window.wlo.mapGrid(step)` returns the same
kind of grid for the whole map at a coarse step (default 4). `window.wlo.plan(x, y)`
returns the client's own short-range path waypoints.

`tools/headless-agent/examples/wlo.example.py` uses the grid for an A* autopilot:

- `Agent.goto(x, y)` walks to world coordinates, re-fetching the grid every
  few cells and issuing `walkWorld` waypoints.
- The client's own `Scene.Plan` only reaches about 14 cells (280 px), so long
  walks need these intermediate waypoints.
- The grid made the Newbie Island east beach navigable without hitting the
  jungle wall; the earlier manual trial-and-error stops were mostly the
  client's short `walkReach` limit, not missing collision data.

## Island route notes

- Newbie Island's jungle blocks direct eastward walks. The reliable route is
  the southern beach: (1300,2400) -> (2200,2400) -> (2500,2400) ->
  (3000,2250) -> (3370,2090).
- Chests on Newbie Island (click ids): 2 Chest, 3 Chest, 4 Cask,
  5 Treas Ches, 7 Treas Ches. Loot seen: Gum x12 (2), Yoghurt Drink x13 (4),
  Instant Noodles x12 (5). Chest 7 triggers event 19 "老魯加入" and
  "Robinson joins the party".
- The two ground items at (1756,3385) and (2262,4325) are far south and were
  not reachable on foot from the explored beach route.
- The east warp area (id 11) leads to map 11016 "Beach" at (134,1126); the
  character arrives on a raft. This is the "sail away" step.

## Starter Beach and the monkey

- Map 11016 "Beach" has a lower beach around y 880..1280 and an upper beach
  around y 350..600, separated by water. The character arrives on Robinson's
  raft, so `waterTravel` is already true.
- To land, call `walkWorld` to a clear land point north of the player (for
  example (722,1050)). `waterTravelTick` sends the dismount and relocates the
  player to the shore.
- After landing, walk north along the sand to S.Monkey at (725,388) and use
  `interactNPC(1)`. Advance the dialogue; the last line is
  "S.Monkey joins the party".
- The recruit persists in `character_pets` as pet_id 17162, party slot 2.
  `state.pets` shows both Robinson (12178) and S.Monkey (17162).
- `AgentBoardRaft` targets a water point only from a water edge with a clear
  direct water-entry ray; a successful boarding sets `waterTravel` and
  `vehicle` (48016 for Robinson's raft).

## Bug log

Test findings are appended to a local, gitignored `tools/headless-agent/session/bugs.log`
(plain text). Current high-severity entries: the character
creation password page is validated against the account password but the UI
describes a new login password, and the client does not handle the create
rejection (blank background, no retry).

## Working end-to-end flow

`tools/headless-agent/examples/agent_flow.example.py` drives the whole path (copy it to `session/`):

1. Create agent.
2. Select region `Local` at (290, 215).
3. Select server `Local 1` at (430, 220).
4. Click `Next` at (350, 450).
5. Fill account/password and click `Login`.
6. Click `Create Char` at (350, 467).
7. Fill the name at (655, 217) and click `Next` at (730, 455).
8. Spend 5 points on STR at (475, 305), then click `Next`.
9. Click `Next` through Attributes and Palette.
10. Fill the character/secret-code page with the account password and a
    distinct secret code, then click `Confirm`.
11. The client enters the world.

The successful run reached map `Ship Deck` (10017) with `agenthero` at
(1042, 1075).

## Next experiments

1. Drive movement and interaction by clicking the ground/NPCs.
2. Expose selected forms and battle state.
3. Add a streaming event channel instead of polling `/state`.
4. Run multiple agents in parallel and verify isolated input/state.

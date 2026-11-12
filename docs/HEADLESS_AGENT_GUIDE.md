# Headless Agent Client Guide

This is the practical guide for running the real Wonderland Gonline client
without a window, driving it from an AI agent, and keeping it alive in combat
and long journeys. Research notes live in `HEADLESS_AGENT_NOTES.md`; test
findings are kept in a local, gitignored `tools/headless-agent/session/bugs.log`.

For the native Go implementation, use the [native host instructions](HEADLESS_API.md#native-host-no-web-functionality).
It supports the semantic API and detachable native spectator without Chrome,
WebAssembly, HTTP or WebSockets. Launch the regular client with explicit
`-headless -api-listen` flags; ordinary startup enables neither API nor web endpoints.
The Chrome-based orchestrator documented below is an optional alternative.

## 1. What it is

```
AI orchestrator
      | JSON-RPC over WebSocket (/rpc)
cmd/wlo-agent
      | Chrome DevTools Protocol
headless Chrome  ->  wasm client (client/web)  ->  /ws gateway  ->  game server
```

- One agent = one Chrome process = one real client with its own input and
  canvas. Run as many as the machine can afford.
- The WASM build is the same `client/wlo` code as the desktop client.
- `window.wlo` exposes state, synthetic input, collision grids and screenshots.
- `cmd/wlo-agent` serves the host page, the WASM binary, the asset packs and
  the `/ws` game gateway on one loopback port, and exposes JSON-RPC at `/rpc`.

## Structured commands (API version 2)

Use the [structured API reference](HEADLESS_API.md) for direct gameplay, forms,
queries, per-session routing, strict payload schemas and completion semantics.
`agent.execute` invokes normal application actions; `agent.state` and
`agent.observe` provide structured observations. Input emulation and arbitrary
JavaScript evaluation are disabled unless `-diagnostic-input` is explicitly set.

The historical helper examples later in this guide use that diagnostic opt-in.
They are not the current API contract and are not required for gameplay workflows.
Hosted sessions reject legacy synthetic-input commands; use semantic API commands
or the [detachable viewer](HEADLESS_API.md#persistent-session-host-and-detachable-viewer)
for human intervention. A viewer can disconnect without logging the character out.

## 2. Build

The host UI uses Vue 3, Tailwind CSS and Vite, like the admin frontend.
Its compiled bundle is checked into `cmd/wlo-agent/host`, so normal Go builds
require no Node.js or external frontend service. After editing its sources, run
`npm run agent-host:test` and `npm run agent-host:build` before rebuilding the
orchestrator. See [host frontend development](../cmd/wlo-agent/frontend/README.md).

```bash
# WASM client (separate Go module under client/)
cd client
GOOS=js GOARCH=wasm CGO_ENABLED=0 go build -o ../bin/wonderland-client.wasm ./web

# Orchestrator and standalone gateway
cd ..
go build -o bin/wlo-agent ./cmd/wlo-agent
go build -o bin/wsgateway ./cmd/wsgateway
```

Notes:

- `bin/client-assets.zip` and the matching companion packs (`-sprites-`,
  `-world-`, `-audio-`) must sit in the `-assets` directory.
- The host reads `_packs.json` from `client-assets.zip` and loads exactly its
  companions, including audio. Stale ZIPs and unrelated files are not served.
- Packs are read in bounded HTTP ranges rather than downloaded into whole-file
  browser Blobs. This avoids Chromium storage quotas on multi-gigabyte exports.
- Headless Chrome can be Chrome for Testing. Pass it with `-chrome`.

## 3. Run

Headless automation is optional and disabled unless `wlo-agent -enable` is
explicitly requested. The normal game server never starts Chrome, the agent
RPC endpoint, its asset server or a browser gateway. Administration is separate
and retains its existing startup behavior.

Set `WLO_AGENT_TOKEN` to a random secret of at least 32 bytes in both the
orchestrator and controller environments. For example, in PowerShell:

```powershell
$env:WLO_AGENT_TOKEN = [guid]::NewGuid().ToString("N") + [guid]::NewGuid().ToString("N")
```

The RPC listener accepts numeric loopback addresses only. Clients authenticate
with `Authorization: Bearer <token>`; unrelated browser origins are rejected.
Keep the token private: this API controls the client. Script evaluation requires a separate diagnostic opt-in.
The Python examples read the environment variable and omit the browser Origin.
Chrome's sandbox remains enabled; run as a regular user.


```bash
./bin/wlo-agent -enable \
  -chrome /path/to/chrome \
  -assets ./bin \
  -game 127.0.0.1:6414 \
  -listen 127.0.0.1:9000
```

JSON-RPC endpoint: `ws://127.0.0.1:9000/rpc`.
`agent.create` creates a client without logging in. Supply credentials through
the structured `login.submit` command; credential parameters to create
are rejected rather than silently ignored.

Methods: `agent.create`, `agent.destroy`, `agent.state`, `agent.command`,
`agent.screenshot`, `agent.eval`.

`tools/headless-agent/examples/wlo.example.py` is the reference client; copy
it to `tools/headless-agent/session/wlo.py` for a session:

```python
from wlo import Agent
a = Agent()
a.create()
```

## 4. Login and character creation

The login flow is coordinate-driven (800x600 logical canvas). The reference
scripts are `agent_flow.py` and `agent_world.py`.

1. Server select:
   - region `Local` at (290, 215);
   - server `Local 1` at (430, 220);
   - `Next` at (350, 450).
2. Credentials:
   - account field (375, 252), password field (375, 287);
   - `Login` at (397, 345).
3. Character select: click the `Enter` button under a slot (320, 445 for slot 1).
4. Character creation:
   - name field (655, 217); `Next` (730, 455);
   - spend all 5 points on the stats page (STR arrow at 475, 305);
   - `Next` through Attributes and Palette;
   - on the password page use the **account password** as the character
     password and a different secret code, then `Confirm` (593, 455).

Limits and gotchas:

- Account and password editors are limited to **10 bytes**; longer values are
  silently truncated and login fails.
- The character creation "Set password" page is validated against the account
  password by the server, despite the UI wording.
- A rejected create packet is not handled by the client: the form hides and
  the screen stays on a blank background. Re-login to retry.
- You can log in and click the existing character's `Enter` button instead of
  creating a new one.

## 5. State

`wlo.state()` returns a JSON snapshot. Fields an agent needs:

- `map` (`id`, `name`), `mapSize`, `player` (`id`, `name`, `x`, `y`, `map`),
  `camera` (`x`, `y`).
- `peers`, `npcs` (click id, name, x, y), `areas` (id, rect, events),
  `floorItems` (slot, id, name, x, y, screen sx/sy).
- `talk`: `shown`, `choosing`, `lines`, `options`, `optionPoints`, `answers`.
- `bag`, `equipment`, `pets`, `hotel`, `shop`.
- `stats`: HP/SP, level, EXP, gold, STR/CON/INT/WIS/AGI, free points.
- `eventActive`, `eventDone`, `movieActive`, `walking`, `waterTravel`,
  `vehicle`.
- `battle`: `active`, `ready`, `turn`, `side`, `victorySide`, `result`
  (`win`/`lose`/empty), `expGained`, `levelGained`, `rewardLines`, `fighters`
  (cell, side, kind, id, hp, sp, canChoose).

Walkability:

- `wlo.grid(radius)` returns a JSON grid centred on the player, 20 px cells,
  `cells` base64 (1 = walkable).
- `wlo.mapGrid(step)` returns a coarse whole-map overview.
- `wlo.plan(x, y)` returns the client's own short-range path.

## 6. Historical diagnostic commands

Synthetic input and client actions available through `agent.command` or
`wlo.command`:

- Input: `click`, `rightClick`, `move`, `key`, `text`.
- World: `walkWorld(x,y)`, `interactNPC(id)`, `pickupFloor(slot)`,
  `clickID(id)`, `boardRaft(x,y)`.
- Battle: `battleAttack(x,y)`, `battleDefend`, `battleSkill(id,x,y)`,
  `battleItem(slot,x,y)`, `battleAuto(on)`, `battleFlee`, `heal(slot)`.
- Inventory: `equip(slot)`, `unequip(slot)`, `allocate(str,con,int,wis,agi)`.
- Shop: `buy(slot,quantity)` (slot is a `state.shop.stock` slot, not an item id).
- Pets: `petSelect(slot)`, `petRest`, `petDeposit(slot)`, `petWithdraw(slot)`.
- Dialogue: `answer(index)` (direct event answer, bypasses UI timing) and
  `pose(cell)`.

The Python helper wraps most of these:

```python
a.state(); a.click(400, 300); a.goto(1200, 900); a.goto_safe(2000, 1400)
a.interact_npc(7); a.pickup_floor(2); a.fight(); a.choose(0)
a.buy(3, 1); a.equip(29); a.unequip(3); a.pet_deposit(1)
```

## 7. Combat without dying

The rule that matters: **heal before the fight**. A lost battle leaves the
player at 1 HP; the game then repeats the defeat line and the quest/event may
look stuck. That is not a quest bug.

Recommended loop (`goto_safe` and `fight()` implement it):

1. On every step, check `battle.active`.
2. If HP < 45% of max and not in battle, use **Spicy Hot Pot** (slot 3), then
   continue walking. (`heal` opens the inventory, uses the item and hides
   the form again.)
3. In battle, wait for `battle.ready` and a fighter with `canChoose`.
4. Attack the first living enemy with `battleAttack`. If HP < 55%, use
   `battleItem(3, selfCell)` instead.
5. After the battle, read `state.battle.result`:
   - `win` — check `expGained`/`rewardLines`, continue;
   - `lose` — heal immediately and retreat; do not keep clicking the event.
6. `fight()` returns `win`/`lose`/`unknown` and appends `{result, exp, level,
   rewards}` to `agent.history`.

If `victorySide` is absent, the client infers `win` from positive EXP and
`lose` from a finished battle at 1 HP with no EXP.

## 8. Movement, collisions and warps

- `Scene.Plan` only reaches about 14 cells (280 px). A single click or
  `walkWorld` to a far point silently does nothing. Walk in intermediate
  waypoints; `goto()` does A* over `wlo.grid(radius)` and re-fetches the grid
  every few cells.
- `goto_safe()` additionally fights encounters and pre-heals.
- **Stop on map change.** The old coordinates are meaningless after a warp or
  door. `goto_safe` returns `False` as soon as `state.map.id` changes;
  `enter_door(x,y)` reports whether a new map was entered. Never keep walking
  a village target inside a house.
- Narrow passages can wedge the sprite. If `walkWorld` does nothing, try a
  short step sideways (e.g. +150 px on x) before re-planning.
- Doors are map areas, not one-to-one with the server warp click ids. If a
  door keeps bouncing you between maps, stop and re-read `state.map`/`areas`.
- Water: `boardRaft(x,y)` targets a water point from a water edge; after
  boarding, `waterTravel` is true and the grid treats water as walkable.
  Walk to a land point to land/dismount.
- If an event/talk menu stays `shown=false` while options remain, use
  `talk.answers` + `answer(index)`. If the event remains active and blocks
  movement, re-login resets the client event state.

## 9. Shops, equipment and pets

- Talk to a shop NPC and answer "Buy equipment". The catalog appears in
  `state.shop.stock` (`slot`, `id`, `name`, `price`).
- `buy(slot, quantity)` purchases; the shop closes after a successful
  purchase. Re-interact to open it again (no relogin needed), then buy the
  next item. Alternatively buy several entries in one packet.
- `equip(bagSlot)` and `unequip(equipmentSlot)` send the native packets.
  The server now broadcasts the new equipment to map peers, so observers see
  the change immediately.
- Pet hotel: talk to the Witch Doctor, choose "Pet hotel", then
  `petDeposit(partySlot)`. `state.pets` and `state.hotel` show the split.
- `allocate` spends free stat points in STR, CON, INT, WIS, AGI order.

## 10. Common pitfalls

| Symptom | Cause | Fix |
|---|---|---|
| Login always says wrong password | password was truncated to 10 bytes | use <= 10 bytes |
| Character creation rejects confirm | password must be the account password | use the account password + distinct secret code |
| Create rejection shows a blank screen | client has no error handler | re-login |
| `Next` on stats does nothing | 5 points not spent | spend all points |
| Shop closes after one buy | server closes the shop on success | reopen and buy again |
| Talk options visible but click does nothing | `Talk.Choosing()` can be false while options remain | `answer(index)` |
| Peer does not see new equipment | old server did not broadcast appearance | fixed: `EquipmentSnapshot` + `SpriteRefresh` |
| Long walk does nothing | `Scene.Plan` reach limit (~280 px) | intermediate `goto` waypoints |
| Player exits the house immediately | walking a village target inside the house | stop on `map.id` change / `enter_door` |
| Quest/event loops a line | often a lost battle at 1 HP, or a stale event state | heal and retry; re-login to reset |

## 11. For AI agents

Operational rules that make long test runs useful:

1. **Keep a bug log.** Append to a local, gitignored
   `tools/headless-agent/session/bugs.log` with timestamp, severity, area, symptom,
   reproduction and evidence (state, packet, screenshot). Mark re-checks and
   downgrade entries when a suspected bug turns out to be a lost battle or a
   UI timing issue.
2. **Prefer logs over screenshots.** Decide from `state`/`grid`/`talk` and
   keep screenshots for evidence, not for navigation.
3. **When stuck, build a tool.** If the problem is a missing capability
   (grid, warp list, shop, outcome, direct answer), add a small agent command
   or Python helper instead of retrying coordinates. Keep helpers in
   `tools/headless-agent/`. The working controllers live in the gitignored
   `tools/headless-agent/session/`; copy the tracked
   `tools/headless-agent/examples/*.example.py` there. Commit reusable tools
   and findings only with user approval.
4. **Propose a debug branch.** New instrumentation (warp/doors map, event
   flags, packet trace, auto-screenshot on defeat) belongs in a separate
   `experiment/debug-tools-*` branch so the main experiment stays reviewable.
5. **Before a fight:** heal; **after a fight:** check `battle.result`; on
   `lose`: heal, retreat, do not keep clicking the event.
6. **After a warp/door:** stop, re-read `state.map`, `camera` and `areas`,
   then navigate; never reuse old coordinates.
7. **Do not read secrets.** Never print `.env`, tokens or keys; use existing
   environment variables inside containers if a service needs them.



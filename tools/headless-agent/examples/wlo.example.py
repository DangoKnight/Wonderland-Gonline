# EXAMPLE: copy to wlo.py and adapt for the current game session.

"""Small shared helpers for the headless agent scripts."""

import base64
import json
import time

import websocket

WS_URL = "ws://127.0.0.1:9000/rpc"
SHOTS = "docs/headless-agent-screenshots"
_count = [0]


class Agent:
    def __init__(self, agent_id=None):
        self.ws = websocket.create_connection(WS_URL)
        self.agent_id = agent_id
        self.history = []

    def rpc(self, method, params=None):
        _count[0] += 1
        self.ws.send(json.dumps({
            "jsonrpc": "2.0",
            "method": method,
            "params": params or {},
            "id": _count[0],
        }))
        return json.loads(self.ws.recv())

    def create(self, account, password):
        r = self.rpc("agent.create", {"account": account, "password": password})
        self.agent_id = r["result"]["id"]
        return self.agent_id

    def state(self):
        return self.rpc("agent.state", {"id": self.agent_id}).get("result") or {}

    def click(self, x, y):
        return self.rpc("agent.command", {
            "id": self.agent_id,
            "command": "click",
            "args": [x, y],
        })

    def click_world(self, wx, wy):
        cam = self.state().get("camera") or {"x": 0, "y": 0}
        return self.click(wx - cam["x"], wy - cam["y"])

    def text(self, value):
        return self.rpc("agent.command", {
            "id": self.agent_id,
            "command": "text",
            "args": [value],
        })

    def key(self, name):
        return self.rpc("agent.command", {
            "id": self.agent_id,
            "command": "key",
            "args": [name],
        })

    def eval(self, script):
        return self.rpc("agent.eval", {"id": self.agent_id, "script": script})

    def shot(self, name):
        r = self.rpc("agent.screenshot", {"id": self.agent_id})
        if "result" not in r:
            print("screenshot failed:", r)
            return
        path = SHOTS + "/" + name
        with open(path, "wb") as f:
            f.write(base64.b64decode(r["result"]["png_base64"]))
        print("saved", path)

    def grid(self, radius=30):
        r = self.rpc("agent.eval", {"id": self.agent_id, "script": f"window.wlo.grid({radius})"})
        return json.loads(r["result"])

    def plan(self, x, y):
        r = self.rpc("agent.eval", {"id": self.agent_id, "script": f"window.wlo.plan({x}, {y})"})
        return json.loads(r["result"])

    def map_grid(self, step=4):
        r = self.rpc("agent.eval", {"id": self.agent_id, "script": f"window.wlo.mapGrid({step})"})
        return json.loads(r["result"])

    def choose(self, index):
        """Answer a dialogue option by index (direct packet, then UI fallback)."""
        talk = self.state().get("talk") or {}
        answers = talk.get("answers") or []
        if answers and 0 <= index < len(answers):
            self.rpc("agent.command", {
                "id": self.agent_id, "command": "answer", "args": [index],
            })
            return True
        points = talk.get("optionPoints") or []
        if index < 0 or index >= len(points):
            return False
        self.click(points[index]["x"], points[index]["y"])
        return True

    def interact_npc(self, npc_id):
        return self.rpc("agent.command", {
            "id": self.agent_id, "command": "interactNPC", "args": [npc_id],
        })

    def pickup_floor(self, slot):
        return self.rpc("agent.command", {
            "id": self.agent_id, "command": "pickupFloor", "args": [slot],
        })

    def shop(self):
        return self.state().get("shop") or {}

    def buy(self, slot, quantity=1):
        return self.rpc("agent.command", {
            "id": self.agent_id, "command": "buy",
            "args": [slot, quantity],
        })

    def equip(self, slot):
        return self.rpc("agent.command", {
            "id": self.agent_id, "command": "equip", "args": [slot],
        })

    def unequip(self, slot):
        return self.rpc("agent.command", {
            "id": self.agent_id, "command": "unequip", "args": [slot],
        })

    def pet_deposit(self, slot):
        return self.rpc("agent.command", {
            "id": self.agent_id, "command": "petDeposit", "args": [slot],
        })

    def pet_withdraw(self, slot):
        return self.rpc("agent.command", {
            "id": self.agent_id, "command": "petWithdraw", "args": [slot],
        })

    def bag(self):
        return self.state().get("bag") or []

    def equipment(self):
        return self.state().get("equipment") or []

    def quests(self):
        return self.state().get("quests") or []

    def ascii_grid(self, radius=10):
        g = self.grid(radius)
        cells = base64.b64decode(g["cells"])
        cols, rows = g["cols"], g["rows"]
        s = self.state()
        p = s.get("player") or {}
        pcx = (p.get("x", 0) - g["originX"]) // g["cellSize"]
        pcy = (p.get("y", 0) - g["originY"]) // g["cellSize"]
        lines = []
        for y in range(rows):
            line = ""
            for x in range(cols):
                if x == pcx and y == pcy:
                    line += "@"
                else:
                    line += "." if cells[y * cols + x] else "#"
            lines.append(line)
        return "\n".join(lines)

    def ascii_map(self, step=4):
        g = self.map_grid(step)
        cells = base64.b64decode(g["cells"])
        cols, rows = g["cols"], g["rows"]
        return "\n".join(
            "".join("." if cells[y * cols + x] else "#" for x in range(cols))
            for y in range(rows)
        )

    def goto(self, tx, ty, radius=30, tolerance=45, max_iters=40, timeout=6.0):
        """Walk to world coordinates using the collision grid for pathing."""
        for _ in range(max_iters):
            s = self.state()
            p = s.get("player") or {}
            if abs(p.get("x", 0) - tx) + abs(p.get("y", 0) - ty) <= tolerance:
                return True
            g = self.grid(radius)
            step = _grid_step(g, p["x"], p["y"], tx, ty)
            if step is None:
                return False
            self.rpc("agent.command", {"id": self.agent_id, "command": "walkWorld", "args": [step[0], step[1]]})
            deadline = time.time() + timeout
            last = (p["x"], p["y"])
            while time.time() < deadline:
                time.sleep(0.3)
                s2 = self.state()
                if (s2.get("battle") or {}).get("active"):
                    break
                p2 = s2.get("player") or {}
                pos = (p2.get("x", 0), p2.get("y", 0))
                if abs(pos[0] - step[0]) + abs(pos[1] - step[1]) <= 40:
                    break
                if pos == last and not s2.get("walking", False):
                    break
                last = pos
        s = self.state()
        p = s.get("player") or {}
        return abs(p.get("x", 0) - tx) + abs(p.get("y", 0) - ty) <= tolerance

    def fight(self, heal_slot=3, max_rounds=120):
        """Fight the current battle with basic attacks and field healing."""
        for _ in range(max_rounds):
            s = self.state()
            b = s.get("battle") or {}
            if not b.get("active"):
                return True
            pid = (s.get("player") or {}).get("id")
            me = next((f for f in b.get("fighters") or [] if f.get("id") == pid), None)
            if me is None:
                time.sleep(0.5)
                continue
            enemies = [
                f for f in b.get("fighters") or []
                if f.get("side") != me.get("side") and f.get("hp", 0) > 0 and not f.get("removed")
            ]
            if not b.get("ready") or not enemies:
                time.sleep(0.7)
                continue
            if me.get("hp", 0) < me.get("maxHP", 1) * 0.55:
                self.rpc("agent.command", {
                    "id": self.agent_id, "command": "battleItem",
                    "args": [heal_slot, me["x"], me["y"]],
                })
            else:
                e = enemies[0]
                self.rpc("agent.command", {
                    "id": self.agent_id, "command": "battleAttack",
                    "args": [e["x"], e["y"]],
                })
            time.sleep(1.7)
        b = self.state().get("battle") or {}
        for _ in range(10):
            if b.get("result"):
                break
            time.sleep(0.4)
            b = self.state().get("battle") or {}
        result = b.get("result") or "unknown"
        self.history.append({
            "result": result,
            "exp": b.get("expGained", 0),
            "level": b.get("levelGained", 0),
            "rewards": b.get("rewardLines") or [],
        })
        return result

    def goto_safe(self, tx, ty, max_iters=60, heal_slot=3, **kwargs):
        """Walk to world coordinates, fighting and pre-healing on the way."""
        start_map = (self.state().get("map") or {}).get("id")
        for _ in range(max_iters):
            s = self.state()
            if start_map and (s.get("map") or {}).get("id") != start_map:
                # A door/warp opened a new map; stop instead of walking the
                # old coordinates inside it.
                return False
            if (s.get("battle") or {}).get("active"):
                print("battle encountered, fighting")
                res = self.fight(heal_slot=heal_slot)
                print("battle result:", res)
                if res == "lose":
                    print("lost battle, healing")
                    self.rpc("agent.command", {
                        "id": self.agent_id, "command": "heal", "args": [heal_slot],
                    })
                    time.sleep(1.5)
                continue
            stats = s.get("stats") or {}
            hp, maxhp = stats.get("hp", 0), stats.get("maxHP", 1)
            if maxhp and hp < maxhp * 0.45:
                print("pre-heal", hp, "/", maxhp)
                self.rpc("agent.command", {
                    "id": self.agent_id, "command": "heal", "args": [heal_slot],
                })
                time.sleep(1.2)
                continue
            p = s.get("player") or {}
            if abs(p.get("x", 0) - tx) + abs(p.get("y", 0) - ty) <= 45:
                return True
            self.goto(tx, ty, max_iters=1, **kwargs)
        s = self.state()
        p = s.get("player") or {}
        return abs(p.get("x", 0) - tx) + abs(p.get("y", 0) - ty) <= 45

    def enter_door(self, tx, ty, **kwargs):
        """Walk to a door and report whether a new map was entered."""
        before = (self.state().get("map") or {}).get("id")
        self.goto_safe(tx, ty, **kwargs)
        after = (self.state().get("map") or {}).get("id")
        return after != before, after

    def close(self):
        self.ws.close()


def _grid_step(g, px, py, tx, ty):
    """A* on a grid around the player; returns the next waypoint or None."""
    import heapq

    cell = g["cellSize"]
    cols, rows = g["cols"], g["rows"]
    ox, oy = g["originX"], g["originY"]
    cells = base64.b64decode(g["cells"])
    scx = (px - ox) // cell
    scy = (py - oy) // cell
    tcx = (tx - ox) // cell
    tcy = (ty - oy) // cell

    def walkable(cx, cy):
        if cx < 0 or cy < 0 or cx >= cols or cy >= rows:
            return False
        return cells[cy * cols + cx] != 0

    if not walkable(scx, scy):
        return None

    moves = [(1, 0), (-1, 0), (0, 1), (0, -1), (1, 1), (1, -1), (-1, 1), (-1, -1)]
    openq = [(0, 0, scx, scy)]
    came = {}
    cost = {(scx, scy): 0}
    best = (scx, scy)
    bestd = abs(scx - tcx) + abs(scy - tcy)
    while openq:
        _, gcost, cx, cy = heapq.heappop(openq)
        d = abs(cx - tcx) + abs(cy - tcy)
        if d < bestd:
            best, bestd = (cx, cy), d
        if (cx, cy) == (tcx, tcy):
            best = (cx, cy)
            break
        for dx, dy in moves:
            nx, ny = cx + dx, cy + dy
            if not walkable(nx, ny):
                continue
            if dx and dy and (not walkable(cx + dx, cy) or not walkable(cx, cy + dy)):
                continue
            ng = gcost + (14 if dx and dy else 10)
            if (nx, ny) in cost and cost[(nx, ny)] <= ng:
                continue
            cost[(nx, ny)] = ng
            came[(nx, ny)] = (cx, cy)
            h = 14 * max(abs(nx - tcx), abs(ny - tcy))
            heapq.heappush(openq, (ng + h, ng, nx, ny))

    path = [best]
    while path[-1] != (scx, scy) and path[-1] in came:
        path.append(came[path[-1]])
    path.reverse()
    if len(path) < 2:
        return None
    # Advance up to 8 cells along the path, but stop before a diagonal corner
    # the client's planner might refuse.
    idx = min(len(path) - 1, 8)
    cx, cy = path[idx]
    return ox + cx * cell + cell // 2, oy + cy * cell + cell // 2


def find_npc(state, name):
    for npc in state.get("npcs") or []:
        if npc.get("name") == name:
            return npc
    return None


def compact(state):
    player = state.get("player") or {}
    cam = state.get("camera") or {}
    talk = state.get("talk") or {}
    return {
        "map": state.get("map"),
        "player": {"x": player.get("x"), "y": player.get("y"), "name": player.get("name")},
        "camera": cam,
        "inWorld": state.get("inWorld"),
        "eventActive": state.get("eventActive"),
        "movieActive": state.get("movieActive"),
        "talk": talk,
        "floorItems": state.get("floorItems") or [],
    }

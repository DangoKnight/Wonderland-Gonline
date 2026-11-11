# EXAMPLE: copy to agent_island.py and adapt for the current game session.

"""Collect the chests on Newbie Island and sail away.

Run from the repository root after agent_world.py has entered the world:

    python tools/headless-agent/agent_island.py AGENT_ID
"""

import json
import sys
import time

from wlo import Agent, compact, find_npc


def advance(a, limit=20):
    """Click through talk lines/events until idle or a choice appears."""
    for _ in range(limit):
        c = compact(a.state())
        talk = c.get("talk") or {}
        if talk.get("choosing"):
            return c
        if not c.get("eventActive") and not c.get("movieActive") and not talk.get("shown"):
            return c
        a.click(400, 300)
        time.sleep(1.2)
    return compact(a.state())


def approach(a, tx, ty, radius=70, timeout=15):
    a.rpc("agent.command", {"id": a.agent_id, "command": "walkWorld", "args": [tx, ty]})
    deadline = time.time() + timeout
    while time.time() < deadline:
        time.sleep(0.6)
        p = (a.state().get("player") or {})
        if abs(p.get("x", 0) - tx) + abs(p.get("y", 0) - ty) <= radius:
            return True
    return False


def interact_npc(a, npc_id):
    a.rpc("agent.command", {"id": a.agent_id, "command": "interactNPC", "args": [npc_id]})


def pickup_slot(a, slot):
    a.rpc("agent.command", {"id": a.agent_id, "command": "pickupFloor", "args": [slot]})


def main():
    agent_id = sys.argv[1]
    a = Agent(agent_id)

    # Dismiss the party notice.
    advance(a)

    s = a.state()
    chests = [n for n in s.get("npcs") or [] if n["name"] in ("Chest", "Cask", "Treas Ches")]
    items = list(s.get("floorItems") or [])

    print("chests:", chests)
    print("floor items:", items)

    for n in chests:
        print("\n=== chest", n)
        ok = approach(a, n["x"], n["y"] - 30)
        print("approach:", ok, compact(a.state())["player"])
        interact_npc(a, n["id"])
        time.sleep(2)
        c = advance(a)
        print("after:", json.dumps(c.get("talk"), ensure_ascii=False))
        if (c.get("talk") or {}).get("choosing"):
            print("choice reached, stopping for manual pick")
            break
        a.shot("island_chest_%d.png" % n["id"])

    for it in items:
        print("\n=== floor item", it)
        ok = approach(a, it["x"], it["y"] - 30)
        print("approach:", ok, compact(a.state())["player"])
        pickup_slot(a, it["slot"])
        time.sleep(2)
        advance(a)
        a.shot("island_item_%d.png" % it["slot"])

    print("\nfinal state:", json.dumps(compact(a.state()), indent=2))


if __name__ == "__main__":
    main()

# EXAMPLE: copy to agent_world.py and adapt for the current game session.

"""Log in and enter the world with an existing character.

Run from the repository root:

    python tools/headless-agent/agent_world.py
"""

import base64
import json
import os
import time

import websocket

WS_URL = "ws://127.0.0.1:9000/rpc"
SHOTS = "docs/headless-agent-screenshots"
ACCOUNT = "agent2test"
PASSWORD = "agentpass1"

_counter = [0]


def rpc(ws, method, params):
    _counter[0] += 1
    ws.send(json.dumps({"jsonrpc": "2.0", "method": method, "params": params, "id": _counter[0]}))
    return json.loads(ws.recv())


def shot(ws, agent_id, name):
    r = rpc(ws, "agent.screenshot", {"id": agent_id})
    if "result" not in r:
        print("screenshot failed:", r)
        return
    path = SHOTS + "/" + name
    with open(path, "wb") as f:
        f.write(base64.b64decode(r["result"]["png_base64"]))
    print("saved", path)


def state(ws, agent_id):
    return rpc(ws, "agent.state", {"id": agent_id}).get("result")


def click(ws, agent_id, x, y):
    r = rpc(ws, "agent.command", {"id": agent_id, "command": "click", "args": [x, y]})
    print("click", x, y, "->", r.get("result", r.get("error")))


def text(ws, agent_id, value):
    rpc(ws, "agent.command", {"id": agent_id, "command": "text", "args": [value]})


def wait_until(ws, agent_id, predicate, timeout=20, interval=0.4):
    deadline = time.time() + timeout
    last = None
    while time.time() < deadline:
        s = state(ws, agent_id) or {}
        last = s
        if predicate(s):
            return s
        time.sleep(interval)
    return last


def main():
    ws = websocket.create_connection(WS_URL, header={"Authorization": "Bearer " + os.environ["WLO_AGENT_TOKEN"]}, suppress_origin=True)
    r = rpc(ws, "agent.create", {})
    print(r)
    agent_id = r["result"]["id"]

    print("waiting for login screen...")
    wait_until(ws, agent_id, lambda s: (s.get("serverSelect") or {}).get("visible"), timeout=40)
    time.sleep(1)

    print("server select...")
    click(ws, agent_id, 290, 215)
    time.sleep(1)
    click(ws, agent_id, 430, 220)
    time.sleep(1)
    click(ws, agent_id, 350, 450)
    time.sleep(4)

    print("credentials...")
    click(ws, agent_id, 375, 252)
    time.sleep(0.8)
    text(ws, agent_id, ACCOUNT)
    time.sleep(0.8)
    click(ws, agent_id, 375, 287)
    time.sleep(0.8)
    text(ws, agent_id, PASSWORD)
    time.sleep(0.8)
    click(ws, agent_id, 397, 345)
    time.sleep(5)
    shot(ws, agent_id, "world01_character_select.png")

    print("enter slot 1...")
    click(ws, agent_id, 320, 445)
    s = wait_until(ws, agent_id, lambda s: s.get("inWorld"), timeout=25)
    print("state:", json.dumps(s, indent=2)[:1200])
    shot(ws, agent_id, "world02_ship_deck.png")
    print("agent id:", agent_id)


if __name__ == "__main__":
    main()

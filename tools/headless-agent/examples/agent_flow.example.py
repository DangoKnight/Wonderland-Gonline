# EXAMPLE: copy to agent_flow.py and adapt for the current game session.

"""End-to-end headless client flow: server select -> login -> create character.

Run the wlo-agent first, then execute this script from the repository root:

    python tools/headless-agent/agent_flow.py
"""

import base64
import json
import time

import websocket

WS_URL = "ws://127.0.0.1:9000/rpc"
SHOTS = "docs/headless-agent-screenshots"
ACCOUNT = "agent2test"
PASSWORD = "agentpass1"  # client fields accept at most 10 bytes
CHAR_NAME = "agenthero"

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
    r = rpc(ws, "agent.command", {"id": agent_id, "command": "text", "args": [value]})
    print("text", repr(value), "->", r.get("result", r.get("error")))


def wait_step(ws, agent_id, step, timeout=15):
    deadline = time.time() + timeout
    while time.time() < deadline:
        s = state(ws, agent_id)
        create = (s or {}).get("createCharacter") or {}
        if create.get("step") == step and create.get("visible"):
            return True
        time.sleep(0.5)
    return False


def main():
    ws = websocket.create_connection(WS_URL)

    print("create agent...")
    r = rpc(ws, "agent.create", {"account": ACCOUNT, "password": PASSWORD})
    print(r)
    agent_id = r["result"]["id"]

    print("waiting for client...")
    time.sleep(25)
    shot(ws, agent_id, "step01_initial.png")
    print("state:", state(ws, agent_id))

    print("select region Local...")
    click(ws, agent_id, 290, 215)
    time.sleep(1)
    shot(ws, agent_id, "step02_region.png")

    print("select server Local 1...")
    click(ws, agent_id, 430, 220)
    time.sleep(1)
    print("state after server click:", state(ws, agent_id))
    shot(ws, agent_id, "step03_server.png")

    print("click Next...")
    click(ws, agent_id, 350, 450)
    time.sleep(5)
    print("state after Next:", state(ws, agent_id))
    shot(ws, agent_id, "step04_next.png")

    print("enter credentials...")
    click(ws, agent_id, 375, 252)
    time.sleep(1)
    text(ws, agent_id, ACCOUNT)
    time.sleep(1)
    shot(ws, agent_id, "step05_account.png")

    click(ws, agent_id, 375, 287)
    time.sleep(1)
    text(ws, agent_id, PASSWORD)
    time.sleep(1)
    shot(ws, agent_id, "step06_password.png")

    click(ws, agent_id, 397, 345)
    time.sleep(6)
    print("state after login:", state(ws, agent_id))
    shot(ws, agent_id, "step07_after_login.png")

    print("open character creation...")
    click(ws, agent_id, 350, 467)
    time.sleep(3)
    shot(ws, agent_id, "step08_create_char.png")

    print("enter character name...")
    click(ws, agent_id, 655, 217)  # NameField rect is x=612 y=207 w=93 h=22
    time.sleep(1)
    text(ws, agent_id, CHAR_NAME)
    time.sleep(1)
    print("state after name:", state(ws, agent_id))
    shot(ws, agent_id, "step09_name.png")

    print("next to stats...")
    click(ws, agent_id, 730, 455)
    time.sleep(4)
    shot(ws, agent_id, "step10_create_stats.png")

    print("spend 5 points on STR...")
    for _ in range(5):
        click(ws, agent_id, 475, 305)
        time.sleep(1)
    shot(ws, agent_id, "step11_stats_spent.png")

    print("next to attributes...")
    click(ws, agent_id, 730, 455)
    time.sleep(3)
    shot(ws, agent_id, "step12_attributes.png")

    print("next to palette...")
    click(ws, agent_id, 730, 455)
    time.sleep(3)
    shot(ws, agent_id, "step13_palette.png")

    print("next to password...")
    click(ws, agent_id, 593, 455)
    time.sleep(3)
    shot(ws, agent_id, "step14_set_password.png")

    print("fill character password page...")
    fields = [
        (325, 263, PASSWORD),
        (325, 313, PASSWORD),
        (325, 363, "agentcode1"),
        (325, 413, "agentcode1"),
    ]
    for x, y, value in fields:
        click(ws, agent_id, x, y)
        time.sleep(0.8)
        text(ws, agent_id, value)
        time.sleep(0.8)
    shot(ws, agent_id, "step15_password_filled.png")

    print("confirm...")
    click(ws, agent_id, 593, 455)
    time.sleep(8)
    print("state after confirm:", state(ws, agent_id))
    shot(ws, agent_id, "step16_after_confirm.png")

    print("agent id:", agent_id)


if __name__ == "__main__":
    main()

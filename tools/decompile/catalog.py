#!/usr/bin/env python3
"""Catalog the decompiled aLogin.exe (WLRI build ca19ee087b60) for its Go port.

Reads the local decompilation under var/decompiled (not tracked) and the Go
sources, and writes:

  docs/client-inventory.json  one record per code region and per subsystem
  docs/ALOGIN_CATALOG.md      the same, for reading

A region is one entry of the unit table that reference/index.json assigns
functions to (a contiguous address range). The unit names in meta/units.txt do
not line up with their contents, so regions are labelled by the classes whose
virtual methods and VMTs they hold and by their strings, and grouped into the
subsystems of SUBSYSTEMS below (hand-authored; review it when the port moves).

"Cited" counts the functions of a region whose ca19ee address a Go file
mentions (FUN_xxxxxxxx). It is a lower bound of the ported code: helpers that a
port reproduces without naming are not counted.

Usage: python3 tools/decompile/catalog.py [--root .] [--exe var/ghidra/aLogin.exe]
"""

import argparse
import bisect
import collections
import json
import pathlib
import re
import struct

BUILD = "ca19ee087b60"
GO_ROOTS = ("client/wlo", "internal", "client/cmd")

# Receive dispatcher FUN_002dde1c: command byte → jump table index → target.
RECV_INDEX, RECV_TABLE, RECV_DEFAULT, RECV_LIMIT = 0x2DDE6C, 0x2DDF34, 0x2EF801, 0xC8

# Statuses: replaced (a Go or Ebitengine facility stands in; nothing to port
# function by function), ported, partial, started (a few pieces), todo.
SUBSYSTEMS = [
    # key, title, go packages, status, regions, notes
    ("rtl", "Delphi RTL and VCL", "Go standard library", "replaced",
     [0, 1, 2, 3, 5, 6, 8, 9, 10, 11, 12, 13, 14, 15, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 30, 33, 35,
      60, 65, 67, 68, 69, 70, 72, 74, 76, 78, 79, 82, 90, 91, 120, 126, 127, 128],
     "System, SysUtils, Classes, Graphics, Controls, Forms, StdCtrls, ComCtrls, Grids, OLE and TWebBrowser. "
     "Only behaviour the game relies on is reproduced (string handling, Delphi date doubles, VCL click order)."),
    ("graphics", "Graphics backend (DelphiX, DirectDraw, JPEG, rodraw2)", "client/wlo/surface, client/wlo/render, Ebitengine", "replaced",
     [36, 39, 40, 42, 43, 53, 55, 58, 59, 66],
     "TDXDraw/DirectDraw surfaces use native-compatible Ebitengine GPU render targets by default, with a CPU RGB565 compatibility path. "
     "GPU shaders preserve colour keys, integer alpha/light/scaling and palette recolouring; UI/text blits can batch. "
     "Visible map tiles and a shared bounded texture cache serve all sessions, with GPU composition/previews and explicit capture-only readbacks. "
     "Graphics and full-client frame parity tests verify the replacement."),
    ("audio", "Audio backend (DirectSound, Ogg, MCI)", "client/wlo/app (sound.go, music.go), Ebitengine audio", "replaced",
     [50, 51, 52, 80, 81, 101, 122, 124, 140, 141, 142],
     "TDXSound buffers, Ogg streams and TMediaPlayer become Ebitengine audio players over the exported PCM WAV files."),
    ("input", "Input backend (DXInput)", "Ebitengine input", "replaced", [99, 100],
     "Keyboard, mouse and joystick devices."),
    ("transport", "Network transport (sockets, HTTP)", "client/wlo/login (net.go), Go net", "replaced",
     [47, 48, 49, 98, 113, 115, 116, 117, 118, 119],
     "TClientSocket and the NetMasters HTTP components. The packet framing and XOR are ported in login."),
    ("assets", "Asset decryption and data files", "internal/clientassets, internal/assets, internal/clientbundle, internal/clientruntime, internal/clientimage, internal/clientfs", "ported", [97, 83],
     "JMA/JXAN decryption, .wmg data; editable exports compile incrementally into separate ZIP64 core/world/sprite/audio packs, binary terrain/event/object records and lazy sprite metadata with build-time palette matching; PNG dimensions and JSON/frame references are validated. Non-sprite images compile into lossless, independently readable tiles with native color preparation, empty-border trimming, small-image atlas packing, content deduplication and byte-bounded LRU caching. Large map backgrounds draw only visible tiles; per-image dependencies preserve incremental builds. Loose data/ remains a development input."),
    ("mainform", "Main form and frame loop (TForm4)", "client/wlo/app (app.go, window.go, workspace.go, profile.go, resources.go)", "partial",
     [279, 280, 281, 283, 284],
     "FormCreate, the 30 ms frame timer (timeSetEvent → message 0x8002 → FUN_004a1d60), the sound table, music start. "
     "Ported: the frame order for login and world, map music, ambience. Not yet: the remaining FormCreate steps, "
     "window modes, most per-frame managers. Go workspace adds independent sessions, a collapsible panel outside the game viewport with four visible preview cards, a scrolling plus card and animated game centering, a persistent workspace profile restoring account-prefilled server login screens without passwords, Xaolan small portrait window icon, shared decoded resources, compiled map/sprite records in separate runtime packs and throttled background drawing."),
    ("protocol", "Packet dispatch (send and receive)", "client/wlo/app (app.go dispatch), internal/protocol", "partial",
     [206, 207],
     "FUN_002c2394 sends, FUN_002dde1c receives. See the command coverage table."),
    ("seui", "UI framework (se_UI, picture database)", "client/wlo/seui, client/wlo/picdb", "ported", [265, 266, 267],
     "TSe_UIHandler, TSe_GrBasic and the components, buttons, editors, lists, scroll bars, forms; TSe_CachePicDB. "
     "Native modal input, owner-routed wheel scrolling and inventory grids are implemented; remaining components are noted by feature."),
    ("text", "Text rendering", "client/wlo/text", "ported", [275, 276], "Twb_MemSur, Twb_PlainPic, Twb_Chinese bitmap fonts."),
    ("login", "Login, server and character selection", "client/wlo/login", "ported", [94, 96, 251, 252, 258],
     "Server list and health checks (TServerDetect), account and password forms, character selection, deletion, notices."),
    ("creation", "Character creation", "client/wlo/login (createrole.go, rolepanel.go)", "ported", [193, 194],
     "TJK_RoleImage, TJK_mansel and TRE_CreateCharacter (region 173)."),
    ("roles", "Roles: players, NPCs, sprites", "client/wlo/role, client/wlo/world", "partial", [262, 263, 264],
     "THuman, TBaseNpc, TMapNpc, TPlayers. Ported: sprite layering and colours, NPC looks and props, walking, peers, "
     "frame stepping, native AC32 expressions/held poses, stop poses, equipment refreshes, nicknames and presence metadata. Not yet: gesture selection UI, riding, transforms, follow NPCs, wandering NPCs, NPC turning."),
    ("ground", "Ground, scene objects and map data", "client/wlo/world", "partial", [249, 257, 268, 102, 103],
     "TGround, TGroundObj, TMap, TFSceneData. Ported: scene layers, walk grid, objects and depth order, sound zones, "
     "camera. Not yet: translucent objects, the map's other lists (sub-regions, the optional grid)."),
    ("gamedata", "Game data tables (eve, Npc.dat, scenes, skills)", "internal/assets, client/wlo/world", "ported",
     [108, 109, 201, 203, 269, 270, 271, 272, 273],
     "TEveData, TRe_Npc, TRE_Scene, TFSkill, TFNpc and the Npc.dat lookup; read from the data/ exports."),
    ("events", "Events, NPC clicks, doors and areas", "client/wlo/app (events.go, areas.go), client/wlo/world", "partial",
     [211, 212, 213, 253, 254],
     "TEventManage: map NPC creation, clicks, the event interpreter (kinds 1, 5, 6), area triggers (20/8), door lights, "
     "sprite placement (FUN_002fe8e8). Not yet: event kinds 2-4 and 7+, NPC walk-in areas (20/2, 20/3), the treasure light."),
    ("movies", "Movies, weather and scene effects", "client/wlo/movie, client/wlo/weather", "partial", [219, 220, 221, 222],
     "TMovie (.sty): actors, camera, lines, pictures, stage effects, music. Weather particles. Not yet: weather "
     "overlays 1-3 and 10 in movies, actor trails and draw modes."),
    ("talk", "Talk and question windows", "client/wlo/hud (talk.go)", "partial", [223, 224],
     "TSe_TalkMsgFormPlus and its variants: faces, bodies, markup, questions, the wide form. Not yet: item and trade "
     "question forms, OK-only mode, scrolling and typing, the talk cursor."),
    ("chat", "Chat log and input bar", "client/wlo/hud (chatlog.go, inputbar.go), client/wlo/app (chat.go)", "partial",
     [277, 278],
     "TTalkMsgForm, TSe_CharMsg: click-through hit test, scroll arrows/thumb/wheel, lock, three modes, native background tiling, unlocked dragging, shared mode geometry, resizing/rewrapping, window-relative ticker, immediate channel recoloring retained across message additions and rewrapping, whisper blur validation, native 31-code emoticon picker, log rendering and animated editor preview with atomic code editing. Not yet: alternate backgrounds, VIP marks and speech bubbles."),
    ("hud", "HUD: status, hot keys, buttons, team, emotes", "client/wlo/hud", "partial", [195, 196, 197],
     "TSe_MainStatus, TSe_HotKeyForm, the button bars, TSe_StatusInfoForm, TSe_TeamForm, TSe_EmotiomForm, skill buttons. "
     "Inventory, Skills and Options actions are wired. Hotbar skill dragging, icons/hints, native AC40 lists, three pages, vertical/horizontal layouts, move/remove controls and F1-F8/manual battle activation are implemented; bindings save per character as local client preferences. Other toolbar actions remain pending."),
    ("minimap", "Minimap, world map and map frame", "", "todo", [214, 215, 168],
     "CH_TMiniMapForm, Tse_MapFrame, TSe_SmallMap, THL_WorldMapForm, user\\Map data."),
    ("cursor", "Cursor and mouse state", "client/wlo/cursor, client/wlo/app (cursor.go)", "ported", [238, 239],
     "Tjo_Cursor and the cursor states; native cursors through the patched Ebitengine."),
    ("sound", "Sound effects, music centre, volume", "client/wlo/app (sound.go, music.go, ambient.go)", "partial",
     [149, 255, 206],
     "Tsound effect list, TFMark, TMusicCenter (jukebox), the volume form. Ported: effects, map music, ambience, "
     "movie music, System options for effects/music volumes and ambient sound. Not yet: the jukebox."),
    ("items", "Items, inventory and equipment", "client/wlo/inventory, client/wlo/app (inventory.go)", "partial", [225, 245, 246, 247, 250, 139, 169],
     "TFItem, TSe_itemObject, TSe_ItemImage, item grids, TEquip, equip forms, crafting materials. Item data and sprite "
     "layering, native combined/status/bag forms, item icons, native item-info layout/type/rank/trade flags, capacity-limited drag quantities, direct recovery/amity use on the inventory-selected player or pet, consumable effect breakdowns, Potential Pill requests/results, authoritative pet target rosters and initial potential snapshots, player/pet previews and stats, native Remote windows and main automation options (walking, basic PvE attacks, supplies, worn equipment, safeguarded discard/logout and optional status display), equipment swaps, AC23 updates and player attribute allocation with native AC8 requests are ported. "
     "Remaining: full equipment bonus/socket/forge tooltip rules, repair, native rebirth/class allocation caps, full animated PotentialForm presentation, pet attribute allocation and advanced Remote skill assignment, secondary item containers and crafting."),
    ("skills", "Skills window and elemental tree", "client/wlo/skills, client/wlo/app/skills.go", "partial", [],
     "TRe_SkillForm: Physical, Magical, Assistant, Life and Intro tabs, player/pet selector, native archive icons, SP costs, grades, proficiency, weapon flags, scroll controls and elemental tree prerequisites. Toolbar/Ctrl+S toggling, AC5:3 snapshots, AC5:11/12 and AC8:1/2 progress updates are implemented. Native skill and animation data supply presentation metadata. Skill dragging and hotbar assignment, target selection through the current battle roster and native AC50 actions are implemented. Native battle-scene targeting/rendering and the battle skill menu remain pending."),
    ("battle", "Battle", "client/wlo/app/remote.go", "started", [227, 228, 230, 231],
     "CH_TBattleGround, CH_TBattleMotion, TSkill, TAttack, TFightHum, TFightField, TFightManage. The server side is ported. Client Remote tracks fighter rosters, vitals, defeats and round readiness and submits owned basic PvE actions. Manual hotbar/Skills actions now select a fighter from a roster dialog and submit native AC50, with ownership/SP/learned-skill checks and duplicate/stale-turn protection. Battle scene rendering, animations, the native battle command menu and advanced Remote skill assignment remain pending."),
    ("system", "Options, teams, organisations, rankings", "client/wlo/settings, client/wlo/app/settings.go", "partial", [232, 234, 235, 237, 256],
     "TCY_OptionForm: five settings windows, native server permissions, local audio/chat/visibility/blacklist settings and the native shaded logout/exit confirmation with player preview are implemented. Pending: Zoom rendering, title entitlements, account/payment services, TCY_SystemTeam, TLD_Top100Form, TCY_OrganManage, TCY_TeamManage."),
    ("trade", "Trade, stalls and vendors", "", "todo", [217, 218, 243],
     "TCY_TradeMenu, vendor and stall forms, TTradeManage."),
    ("social", "Friends, mail, guilds, weddings, PK", "", "todo", [241, 242, 209, 210, 183, 184, 185, 189, 198, 199],
     "TAC_ICQList, mail forms, TSe_ArmyForm (guilds), wedding and couple lists, block lists, PK and cooperative fights."),
    ("shop", "Shops, IM mall and skill trees", "", "todo", [187, 188],
     "THL_ShoppingForm, THL_ShoppingCarForm, TSe_SkillTreeForm."),
    ("services", "NPC services and feature forms", "", "started",
     [143, 144, 145, 146, 147, 158, 159, 165, 169, 170, 171, 172, 173, 180, 181, 182, 186, 200, 204, 205],
     "Careers, dungeon missions, mail boxes, notice boards, banking, express, auto-play, crafting (TRe_CompoundForm), "
     "inns, titles, lottery, exchange lock, quest views (Tjo_TaskView), effect lights. Notice board and creation are ported."),
    ("housing", "Housing and furniture", "", "todo", [244, 248, 111, 112],
     "TRE_FurnitureMng, TGdThing, home pillars, furniture drawing, NPC dolls."),
    ("pets", "Pets and riding", "", "todo", [166, 259, 260],
     "CH_TRidePet, ride pet positions (Data\\AdjustRidePetPos.txt), pet forms."),
    ("minigames", "Minigames and casino games", "client/wlo/minigame, client/wlo/app (minigame.go)", "started",
     [129, 130, 131, 132, 133, 134, 135, 136, 137, 138, 150, 151, 152, 153, 154, 155, 156, 157, 160, 161, 162, 163, 167,
      174, 175, 177, 178, 190, 191, 192, 240],
     "Sports (sheep, catch doll, boxing, Mario, memory, turn, gobang, pumpkin, dig hole, poke, catcher, lucky, "
     "conundrum, watermelon, egg, slot machines, scotd, hunter), chess, dice, lotto, the MP3 player. The sport manager, "
     "scotd (2), moles (3), hunting (4), dreams (5), lucky (13), numbered memory (15) and pumpkin (17) are playable; scotd/pumpkin native collision and animation fidelity remains partial. Egg draws (6/22) "
     "and slots (8/10/19) have native client AC71 flows; these five kinds have atomic server purchases and weighted rewards. Other paid kinds remain pending. See docs/MINIGAMES.md."),
    ("gm", "GM tools", "", "todo", [216], "TGmManage and the GM form."),
]

STATUS_ORDER = {"ported": 0, "partial": 1, "started": 2, "todo": 3, "replaced": 4}


def load_regions(dec):
    idx = json.loads((dec / "reference/index.json").read_text())
    groups, virt, strings = collections.defaultdict(list), collections.defaultdict(collections.Counter), collections.defaultdict(list)
    for x in idx:
        if "unit" not in x:
            continue
        u = x["unit"]
        groups[u].append(int(x["other"], 16))
        for v in x.get("virtual", []):
            virt[u][v.split(".")[0]] += 1
        for s in x.get("strings", [])[:2]:
            if len(strings[u]) < 6:
                strings[u].append(s)
    classes = json.loads((dec / "meta/classes.json").read_text())
    classes = classes if isinstance(classes, list) else classes["classes"]
    ranges = sorted((min(v), max(v), u) for u, v in groups.items())
    return ranges, virt, strings, classes


def cited_addresses(root):
    out = collections.defaultdict(set)
    for base in GO_ROOTS:
        for p in (root / base).rglob("*.go"):
            if "third_party" in p.parts:
                continue
            for m in re.findall(r"FUN_00([0-9a-f]{6})", p.read_text(errors="replace")):
                out[int(m, 16)].add(str(p.relative_to(root)))
    return out


def recv_commands(exe):
    if not exe.exists():
        return None
    b = exe.read_bytes()
    off = lambda va: va - 0x11000 + 0x400
    out = []
    for c in range(RECV_LIMIT):
        i = b[off(RECV_INDEX) + c]
        if struct.unpack_from("<I", b, off(RECV_TABLE) + i * 4)[0] != RECV_DEFAULT:
            out.append(c)
    return out


def protocol_names(root):
    src = (root / "internal/protocol/commands.go").read_text()
    names = {}
    for name, val in re.findall(r"^\s*(Command\w+)\s*=\s*(\d+)", src, re.M):
        names.setdefault(int(val), name[len("Command"):])
    return names


def client_commands(root):
    found = set()
    for p in (root / "client/wlo").rglob("*.go"):
        if p.name.endswith("_test.go"):
            continue
        text = p.read_text()
        found |= set(re.findall(r"p\[0\] == protocol\.Command(\w+)", text))
        # Dispatchers may also switch directly on the received command.
        if "switch p[0]" in text:
            found |= set(re.findall(r"case protocol\.Command(\w+)\s*:", text))
    return found


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--root", default=".")
    ap.add_argument("--exe", help="WLRI ca19ee087b60 binary; defaults to ROOT/var/ghidra/aLogin.exe")
    args = ap.parse_args()
    root = pathlib.Path(args.root).resolve()
    dec = root / "var/decompiled"
    ranges, virt, strings, classes = load_regions(dec)
    starts = [r[0] for r in ranges]
    full = [int(a, 16) for a in re.findall(r"^// Function: \S+ @ ([0-9a-f]+)",
                                          (dec / f"aLogin_{BUILD}_full.c").read_text(errors="replace"), re.M)]
    cited = cited_addresses(root)

    def region_of(a):
        i = bisect.bisect_right(starts, a) - 1
        return ranges[i][2] if i >= 0 else None

    funcs, cites = collections.Counter(), collections.Counter()
    for a in full:
        u = region_of(a)
        funcs[u] += 1
        if a in cited:
            cites[u] += 1
    vmts = collections.defaultdict(list)
    for c in classes:
        u = region_of(c["vmt"])
        if u is not None:
            vmts[u].append(c["name"])

    owner = {}
    for key, *_rest in SUBSYSTEMS:
        for r in _rest[3]:
            owner[r] = key
    regions = []
    for lo, hi, u in ranges:
        names = list(dict.fromkeys([n for n, _ in virt[u].most_common(8)] + vmts[u][:8]))[:10]
        regions.append({"region": u, "start": f"{lo:06x}", "end": f"{hi:06x}", "functions": funcs[u],
                        "cited": cites[u], "classes": names,
                        "strings": [s.replace("\n", " ")[:60] for s in strings[u][:3]],
                        "subsystem": owner.get(u, "unclassified")})

    subs = []
    by_key = {r["region"]: r for r in regions}
    for key, title, pkg, status, rs, notes in SUBSYSTEMS:
        present = [by_key[r] for r in rs if r in by_key]
        subs.append({"key": key, "title": title, "go": pkg, "status": status, "notes": notes,
                     "regions": [r["region"] for r in present],
                     "functions": sum(r["functions"] for r in present),
                     "cited": sum(r["cited"] for r in present),
                     "ranges": [f'{r["start"]}-{r["end"]}' for r in present]})
    loose = [r for r in regions if r["subsystem"] == "unclassified"]
    if loose:
        subs.append({"key": "unclassified", "title": "Unclassified", "go": "", "status": "todo",
                     "notes": "Regions not yet assigned; review and add them to SUBSYSTEMS.",
                     "regions": [r["region"] for r in loose], "functions": sum(r["functions"] for r in loose),
                     "cited": sum(r["cited"] for r in loose), "ranges": [f'{r["start"]}-{r["end"]}' for r in loose]})

    cmds = recv_commands(pathlib.Path(args.exe) if args.exe else root / "var/ghidra/aLogin.exe")
    names = protocol_names(root)
    handled = client_commands(root)
    commands = []
    for c in cmds or []:
        n = names.get(c)
        commands.append({"command": c, "name": n or "", "client": bool(n and n in handled)})

    inventory = {"build": BUILD, "functions": len(full), "classes": len(classes), "regions": regions,
                 "subsystems": subs, "receive_commands": commands,
                 "cited_functions": len([a for a in full if a in cited])}
    (root / "docs/client-inventory.json").write_text(json.dumps(inventory, indent=1) + "\n")
    (root / "docs/ALOGIN_CATALOG.md").write_text(render(inventory))
    print(f"{len(full)} functions, {inventory['cited_functions']} cited, {len(regions)} regions, "
          f"{len(loose)} unclassified, {sum(c['client'] for c in commands)}/{len(commands)} commands")


def render(inv):
    out = []
    w = out.append
    w("# aLogin.exe catalog\n")
    w("<!-- Generated by tools/decompile/catalog.py from var/decompiled; edit SUBSYSTEMS there, not this file. -->\n")
    w(f"The decompiled client (WLRI build `{inv['build']}`) has **{inv['functions']:,} functions** and "
      f"**{inv['classes']} classes**. The Go port cites **{inv['cited_functions']}** of those functions by address. "
      "Regions are the address ranges of the decompiler's unit table; their names in `meta/units.txt` are shifted, so "
      "each is identified by its classes and strings. *Cited* is a lower bound of ported code. The plan is in "
      "[CLIENT_ROADMAP.md](CLIENT_ROADMAP.md); machine-readable data is in `client-inventory.json`.\n")
    w("Statuses: **ported** (in use, details remain), **partial** (core in place, features missing), **started** (a few "
      "pieces), **todo**, **replaced** (a Go or Ebitengine facility stands in; not ported function by function).\n")
    w("## Subsystems\n")
    w("| Subsystem | Status | Functions | Cited | Go |")
    w("|---|---|---:|---:|---|")
    subs = sorted(inv["subsystems"], key=lambda s: (STATUS_ORDER.get(s["status"], 9), -s["functions"]))
    for s in subs:
        w(f"| [{s['title']}](#{s['key']}) | {s['status']} | {s['functions']:,} | {s['cited']} | {s['go'] or '—'} |")
    totals = collections.Counter()
    for s in inv["subsystems"]:
        totals[s["status"]] += s["functions"]
    w("")
    w("Functions by status: " + ", ".join(f"{k} {totals[k]:,}" for k in sorted(totals, key=lambda k: STATUS_ORDER.get(k, 9))) + ".\n")
    w("## Server commands\n")
    cmds = inv["receive_commands"]
    if cmds:
        done = [c for c in cmds if c["client"]]
        w(f"The receive dispatcher (`FUN_002dde1c`) handles **{len(cmds)}** commands; the Go client handles "
          f"**{len(done)}**. Unnamed commands have no name in `internal/protocol` yet.\n")
        w("| Command | Name | Go client |")
        w("|---:|---|---|")
        for c in cmds:
            w(f"| {c['command']} | {c['name'] or '—'} | {'yes' if c['client'] else ''} |")
        w("")
    w("## Subsystem details\n")
    by_region = {r["region"]: r for r in inv["regions"]}
    for s in subs:
        w(f"<a id=\"{s['key']}\"></a>\n### {s['title']}\n")
        w(f"Status **{s['status']}**; {s['functions']:,} functions, {s['cited']} cited. Go: {s['go'] or 'not started'}.\n")
        w(s["notes"] + "\n")
        w("| Region | Range | Functions | Cited | Classes |")
        w("|---:|---|---:|---:|---|")
        for rid in s["regions"]:
            r = by_region[rid]
            cls = ", ".join(r["classes"][:6]) or "—"
            w(f"| {rid} | {r['start']}–{r['end']} | {r['functions']} | {r['cited']} | {cls} |")
        w("")
    return "\n".join(out)


if __name__ == "__main__":
    main()

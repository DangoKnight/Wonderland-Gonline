# Client port roadmap

The Go client (`client/wlo`) recreates aLogin.exe (WLRI build `ca19ee087b60`)
on Ebitengine for Windows, Linux and macOS. The decompile is the source of
truth; the client reads only the repository's `data/` exports. This roadmap
orders the remaining work; [ALOGIN_CATALOG.md](ALOGIN_CATALOG.md) lists every
code region of the original with its subsystem, size and port status, and
[CLIENT.md](CLIENT.md) documents what each ported piece does.

## Where the port stands

Of 12,130 functions, about 5,100 belong to the Delphi runtime, VCL, DelphiX,
DirectX, sockets and codecs. Go and Ebitengine replace those wholesale, and only
behaviour the game relies on is reproduced. The game itself is about 7,000
functions:

| Status | Functions | Subsystems |
|---|---:|---|
| Ported | ~800 | UI framework, login and character selection, creation, text, cursor, game data, asset decoding |
| Partial | ~1,750 | Main frame loop, packet dispatch, roles, ground, events and doors, movies and weather, talk windows, chat, HUD, sound |
| Started | ~2,800 | Items and equipment, NPC services and feature forms, minigames (sport manager, moles, hunting, dreams, lucky, memory; egg and slot client flows) |
| Not started | ~1,700 | Battle, minimap, trade, social, shops, housing, pets, the other minigames, GM tools |

The original handles 91 server commands; the Go client has receive branches for
22. Individual subcommands and their interfaces can still be incomplete. The server
side of most systems is already ported, so each phase below can be tested
end to end.

## Principles

- **Follow the decompile.** Port behaviour from the functions themselves, cite
  them by address in comments (the catalog counts these citations), and keep
  numeric codes named (see [DEVELOPMENT.md](DEVELOPMENT.md)).
- **Verify against the original.** Compare with captures from the WLRI-capture
  install against this repository's server (screenshots and videos in
  `client/reference/screenshots`), with snapshot tests where a picture matters
  and packet-level tests for the protocol.
- **Keep it smooth.** Draw at the display rate; run frame-counted game logic on
  the original's 30 ms game frame only where the original counts frames.
- **Stay portable.** Pure Go plus Ebitengine; no platform-specific code outside
  the vendored, documented Ebitengine patch.

## Phases

Each phase follows the player's path, so it can be played through on the live
server. A phase is done when its features match the captures and its server
commands are handled.

### Phase 1: The starter journey (ship deck to Newbie Island)

Finish everything a new character meets before the first battle.

- Events: the remaining event kinds (2-4, 7 and up), NPC walk-in areas (20/2,
  20/3), the treasure light, **NPCs turning to the player**, speech bubbles and
  wandering NPCs (eve walk steps). Native received AC32 expressions and held poses now render; their selection interface is pending.
- Chat log: alternate backgrounds, VIP marks and
  character speech bubbles. Click-through, scrolling, lock, modes, resizing, whispers and
  the native emoticon picker, log renderer and editor preview with atomic code editing are implemented.
- HUD actions: wire the button bars and hot keys to their forms as those forms
  arrive; the status info form.
- Minimap and world map (`CH_TMiniMapForm`, `Tse_MapFrame`).
- Roles: gesture selection, titles, riding/pets and friend-list presence UI.
  AC32 expressions/poses and AC10 names/nicknames/presence metadata are handled.
- Movies: weather overlays inside movies, actor trails and draw modes.
- Commands: remaining Presence 10/Pose 32 UI, Quest 24 and StoryConstellation 15.
  Native Settings 33 is handled by the settings windows.

### Phase 2: Inventory, items and equipment

The core bag/equipment window is implemented from the native decompilation:
50 slots, three display modes, battle-ready preview, item icons/tooltips, Ctrl
quantity dragging, use, drops and equipment swaps with server-confirmed AC23
updates. Focused tests cover packets, metadata, input requests and world entry.
Player point allocation now previews/cancels/submits native AC8 requests and
waits for server-confirmed stats. Remaining work is repair, native class/rebirth
allocation caps, potential dialogs, pet equipment,
secondary containers and crafting interfaces.

- Item grids, drag and drop, item tooltips (`TSe_itemObject`, `TSe_ItemImage`,
  `TRe_ItemGridForm`), equipment forms (`TSe_EquipForm2`), item use and
  repair, gold.
- Commands: Inventory 23, EquipmentRepair 36, Gold 26 (in full), PackContents 91.

### Phase 3: Battle

The standalone Skills window is implemented: five native tabs, player/pet
selection, progress updates and the elemental prerequisite tree. Learned skills
can be dragged onto a saved per-character hotbar and activated with F1–F8 or a
click. Manual battle actions use a roster target chooser and native AC50 packets.
The native battle scene, battle skill menu and overworld casting remain pending.

The largest single gameplay system (about 360 functions); the server side is
ported.

- The battle scene (`CH_TBattleGround`), fighters and motions
  (`TFightHum`, `CH_TBattleMotion`), skills and attacks (`TSkill`, `TAttack`),
  battle effects and sounds (`CH_TBattleSound`, `CH_TLight`), the battle
  menus (`TFightForm1`, `TCY_SkillListMenu`), battle pets, results.
- Commands: BattleState 11, BattleAction 50, BattleReady 52, BattlePet 19.

### Phase 4: Party, social and economy

- Teams (`TSe_TeamForm`, `TCY_TeamManage`), friends and mail (`TAC_ICQList`,
  `TSe_SendMailForm`), trade (`TCY_TradeMenu`), stalls and vendors, shops and
  the IM mall (`THL_ShoppingForm`), bank and storage, guilds (`TSe_ArmyForm`).
- Commands: Team 13, Friends 14, Trade 25, Shop 27, Storage 29/30, Mall 34/75,
  Bank 45, Guild 39, Stall 56.

### Phase 5: Character growth and services

- Skill trees (`TSe_SkillTreeForm`), pets and riding (`CH_TRidePet`, pet
  forms), NPC service forms: crafting and manufacture (`TRe_CompoundForm`,
  `TRe_MakeThingForm`), alchemy, careers, dungeon missions, titles, inns,
  tents, territory, the palace trial, the monster book, the lucky draw.
- Commands: NPCService 31, Alchemy 40, Manufacture 59, PetFeeding 67,
  PetTraining 68, PetRebirth 69, Tent 65, Territory 70, PalaceTrial 77,
  MonsterBook 53, LuckyDraw 104.

### Phase 6: Housing, minigames and the rest

- Housing and furniture (`TRE_FurnitureMng`, `TGdThing`), the remaining
  minigames and casino games (about 800 functions, many small and
  independent; the sport manager and local kinds 3/4/5/13/15 are playable;
  paid kinds 6/8/10/19/22 have client flows and server purchases/rewards; see
  [MINIGAMES.md](MINIGAMES.md)), options and
  rankings (`TCY_OptionForm`, `TLD_Top100Form`), organisations, GM tools.
- Commands: Minigame 57 and the remaining unnamed commands.

### Phase 7: Startup, options and release

System, Channels, Info Visibility, Titles and Blacklist windows now use native
skins and layouts. Server permissions, audio, chat colors, blacklist filtering and
return/logout actions are wired. Remaining settings services, Zoom rendering and
title entitlement handling are listed in [CLIENT.md](CLIENT.md#settings-ui).

- The rest of FormCreate (`Transition`), skins from `Skins.Flst`, the jukebox and
  window modes.
- Remove the legacy front end (`client/ui`, `client/frontend.go`).
- Packaging for Windows, Linux and macOS.

## Cross-cutting work

- **Protocol names.** 33 of the original's 91 commands have no name in
  `internal/protocol`. Name each one when its first handler is written.
- **The catalog.** Rerun `python3 tools/decompile/catalog.py` after porting work
  and update a subsystem's status in its `SUBSYSTEMS` table when it moves. The
  script regenerates `ALOGIN_CATALOG.md` and `client-inventory.json` from the
  local decompile (`var/decompiled`) and the Go sources.
- **Next steps list.** Short-term items live in [CLIENT.md](CLIENT.md#next-steps);
  move them into a phase here when they grow.

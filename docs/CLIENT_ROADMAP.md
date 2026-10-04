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
| Not started | ~1,700 | Battle, minimap, trade, social, shops, housing, pets, the other minigames, options, GM tools |

The original handles 91 server commands; the Go client handles 17. The server
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
  emotes above characters, wandering NPCs (eve walk steps).
- Chat log: **click-through to the map** (form hit test), scrolling, the lock,
  modes, whispers; the remaining chat channels.
- HUD actions: wire the button bars and hot keys to their forms as those forms
  arrive; the status info form.
- Minimap and world map (`CH_TMiniMapForm`, `Tse_MapFrame`).
- Roles: poses and emotes (Pose, 32), presence (10).
- Movies: weather overlays inside movies, actor trails and draw modes.
- Commands: Presence 10, Pose 32, Quest 24, Settings 33, StoryConstellation 15.

### Phase 2: Inventory, items and equipment

The core bag/equipment window is implemented from the native decompilation:
50 slots, three display modes, battle-ready preview, item icons/tooltips, Ctrl
quantity dragging, use, drops and equipment swaps with server-confirmed AC23
updates. Focused tests cover packets, metadata, input requests and world entry.
Remaining work is repair, point allocation/potential dialogs, pet equipment,
secondary containers and crafting interfaces.

- Item grids, drag and drop, item tooltips (`TSe_itemObject`, `TSe_ItemImage`,
  `TRe_ItemGridForm`), equipment forms (`TSe_EquipForm2`), item use and
  repair, gold.
- Commands: Inventory 23, EquipmentRepair 36, Gold 26 (in full), PackContents 91.

### Phase 3: Battle

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

- The rest of FormCreate (`Transition`), skins from `Skins.Flst`, sound and
  music options (volume sliders, ambient switch), window modes.
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

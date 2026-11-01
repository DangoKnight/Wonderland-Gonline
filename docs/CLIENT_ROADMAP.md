# Client port roadmap

The Go client (`client/wlo`) recreates aLogin.exe (WLRI build `ca19ee087b60`)
on Ebitengine for Windows, Linux and macOS. The decompile is the source of
truth; the client reads compiled packs built from repository `data/`, or the
loose exports selected with `-assets`. This roadmap
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
| Partial | ~3,200 | Main frame loop, packet dispatch, items and equipment, roles and pets, ground, events and doors, movies and weather, talk windows, chat, HUD, sound, options |
| Started | ~2,950 | NPC services and feature forms, minigames (including dodgeball and fruit machines), social and mail, item trades, scene minimap |
| Not started | ~330 | Housing, shops and IM mall, GM tools |

The original handles 91 server commands; the Go client has receive branches for
29. Individual subcommands and their interfaces can still be incomplete. The server
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

- Events: event kinds 0 and 2, NPC walk-in areas (20/2, 20/3), the treasure
  light and
  remaining event motion presentation. Server-driven NPC roaming now renders
  through AC22:2 with native speed, facing, interruption and arrival handling,
  and talking NPCs turn to the player. Event NPC routes (kind 4), turns
  (kinds 11 and 14) and event music (kind 15) are ported.
  Native received AC32 expressions and held poses render, with selection and
  Alt shortcuts in the Social window's Emote tab.
- Chat log: alternate backgrounds and VIP marks (this build shows no chat
  speech bubbles: `FUN_00428ed0` skips its body). Click-through, scrolling, lock, modes, resizing, whispers and
  the native emoticon picker, log renderer and editor preview with atomic code editing are implemented.
- HUD actions: wire the button bars and hot keys to their forms as those forms
  arrive; the status info form.
- Minimap and world map (`CH_TMiniMapForm`, `Tse_MapFrame`). The scene
  minimap (`<map>_Mini_Form`, signs, the player's face, minimize and
  reload) is ported; the tiled-map minimap, the map frame and the world
  map remain.
- Roles: titles, passenger presentation, additional vehicle classes and transforms.
  AC32 expressions/poses and AC10 names/nicknames/presence metadata are handled;
  friend rows show presence in the Social window, with server wire mismatches
  recorded in [CLIENT.md](CLIENT.md).
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
allocation caps, the full animated PotentialForm, pet allocation dialogs,
secondary containers and remaining crafting interfaces. Potential Pill requests,
results and derived stats, player/pet equipment and the animated compounding
form with learned-tier selection are implemented; see [compounding](COMPOUNDING.md).

- Item grids, drag and drop, item tooltips (`TSe_itemObject`, `TSe_ItemImage`,
  `TRe_ItemGridForm`), equipment forms (`TSe_EquipForm2`), item use and
  repair, gold.
- Commands: Inventory 23, EquipmentRepair 36, Gold 26 (in full), PackContents 91.

### Phase 3: Battle

The standalone Skills window is implemented: five native tabs, player/pet
selection, progress updates and the elemental prerequisite tree. Learned skills
can be dragged onto a saved per-character hotbar and activated with F1–F8 or a
click. Manual battle actions now target fighters directly in a dedicated battle scene.
Native formation/backgrounds, radial command icons (including each actor's
previous action), mirrored sprite movement,
HP/SP digit artwork, damage cues and sound tracks are implemented. Precise movement
phase/camera synchronization, spell visuals,
the dedicated battle skill menu, item execution, spectator/results presentation
and overworld casting remain pending. See [battle parity](BATTLE_CLIENT_PARITY.md).

The largest single gameplay system (about 360 functions). Existing server
combat rules are integrated; the parity document distinguishes verified native
presentation from server compatibility formulas and remaining mechanics.

- The battle scene (`CH_TBattleGround`), fighters and motions
  (`TFightHum`, `CH_TBattleMotion`), skills and attacks (`TSkill`, `TAttack`),
  battle effects and sounds (`CH_TBattleSound`, `CH_TLight`), the battle
  menus (`TFightForm1`, `TCY_SkillListMenu`), battle pets, results.
- Commands: BattleState 11, BattleAction 50, BattleReady 52, BattlePet 19.

### Phase 4: Party, social and economy

- The Social window is ported (Friend, Emote and Set Up tabs, sending mail),
  with the mailbox, letters, the blinking mail icon, Make Friends and friend
  questions, Old Mail and the compose form; name-addressed letters (14/6)
  remain.
- Item trading is ported: the toolbar's Business menu, Trade Items (target
  mode 9) and the trade form (`TRe_TradeItemForm`), with the server's AC25
  rewritten to aLogin's protocol. Pet and furniture trades, stalls and
  vendors remain.
- Teams is implemented: pet modes/rename/dismissal, invitations, leadership,
  teammate vitals and instance room browsing. Dungeon objectives/rewards remain
  pending; see [Teams](TEAMS.md).
- Remaining forms include stalls/vendors, shops, the IM mall (`THL_ShoppingForm`),
  bank/storage and guild management (`TSe_ArmyForm`), plus the social/trade gaps above.
- Commands: Team 13, Friends 14, Trade 25, Shop 27, Storage 29/30, Mall 34/75,
  Bank 45, Guild 39, Stall 56.

### Phase 5: Character growth and services

- Skill trees, pet following/riding, and rank/base alchemy with its cauldron
  animation and tier controls are implemented.
- Remaining service forms include timed manufacturing (`TRe_MakeThingForm`),
  careers, dungeon objectives, titles, inns, tents, territory, Palace trials,
  the monster book and Lucky Draw. Their server backends have separate coverage
  in [port status](PORTING.md); a working backend does not imply a Go-client form.
- Commands: NPCService 31, Alchemy 40, Manufacture 59, PetFeeding 67,
  PetTraining 68, PetRebirth 69, Tent 65, Territory 70, PalaceTrial 77,
  MonsterBook 53, LuckyDraw 104.

### Phase 6: Housing, minigames and the rest

- Housing and furniture (`TRE_FurnitureMng`, `TGdThing`), the remaining
  minigames and casino games (about 800 functions, many small and
  independent; the sport manager and local kinds 1/2/3/4/5/13/15/17 are playable;
  paid kinds 6/8/10/19/22 have client flows and server purchases/rewards,
  the fruit machines 11/12/14 are native ports with server purchases; see
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

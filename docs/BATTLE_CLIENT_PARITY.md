# Battle client implementation and reference evidence

The Go client now enters a dedicated battle scene using the existing server
simulation. This is a working core, **not complete native battle parity**.

## Implemented

- AC11 formation, participant, finish and departure records; AC50 variable actor,
  target and stat batches; AC51 batched absolute HP/SP; AC52 round readiness;
  AC53 defeat and action-submission acknowledgements. Decode batches before
  mutating state. Duplicate ready packets do not reset submitted actions.
- Native 4 by 4 formation, player equipment and NPC/pet sprites, battle stances,
  defeated poses, names, HP/SP bars, hover highlighting and transient HUD vitals.
  Battle sprite clocks and state belong to each workspace session.
- Scene-data background selection, fallback scene 59081, three ordered layers
  and native initial camera. Pictures use existing loose/compiled asset loaders.
- The native radial icons and positions for attack, skills, defend, capture and
  flee. Select owned player/pet actors and target fighters directly, with native
  sword/magic/capture cursors. Skills and
  hotbar shortcuts use the same targeting and stale-action checks. Raw Skill.dat
  target policies distinguish enemy/ally/either/self/other-ally, revival requires
  a defeated ally, and starter stunts use their resolved character skill. Escape
  cancels
  targeting. World mouse/key walking is blocked throughout a battle.
- The radial center button shows the selected actor's last confirmed action,
  using that action's native radial artwork. Selection/cancellation does not
  overwrite history. AC53:5 confirms manual choices; executed AC50:1 records
  also update history for Remote actions. Clicking the center repeats the
  command with current SP/ownership/skill checks and fresh target selection.
  History is separate for player/pet actors and cleared on battle exit.
- MBTM fighter action/frame records, actor-relative mirrored movement, return
  movement, directional action mirroring, damage cues and
  percentage distribution, native HP/SP digit sheets, compatibility recovery
  text and authored sound cues. Late cues remain alive beyond the last frame.
  The variable visual-effect section is traversed to decode sound records; it
  is not yet rendered. Missing motions use a named compatibility duration.
- Successful native capture animations remove the captured fighter. Native
  departure records remove participants. Expired damage clears in background
  sessions. Battle menus and HUD references are
  released on exit, character reset and session closure.

- Fighter overlays follow the native draw and the battle captures in
  `client/reference/screenshots/Battle`: `icon_rail2` with `icon_HP2` cut to
  HP×40/max at (x−18, feet−height), never above y 80 (`FUN_00437f28`, drawn for
  every shown fighter by `FUN_0039bd48`), and the name centred above it in
  12-pixel outlined text, cyan for monsters and yellow for players and pets
  (`0x4147f8` through the fighter overlay slot `FUN_0038ace4`). The height is
  +0x20b2: the template's name-height rule (`FUN_004265a4`,
  `world.BattleHeight`) or 0x50 for players. Monster names sit 20 plus 25 (or
  10 at heights ≥ 50) above the bar; player names sit 20 above it, measured
  from `Screenshot_20261008_091702.png` because the player's extra overlay
  branches are not traced (named compatibility `battlePlayerNameLift`). The old
  green HP and blue SP bars are gone; the original shows no SP bar.
- Hovering a fighter while commands are chosen draws `FUN_0038ad1c`'s details
  after the forms (`FUN_0038f460`): name, element (None/Eart/Watr/Fire/Wind/
  Hart), `LV:n` and `HP:a/b` at 50/35/20/10 above the feet, right of fighters in
  columns 2 and 4 (+30) and right-aligned left of columns 1 and 3 (−25). The
  class icons for players and the `Okey3` monster flag are not ported.
- The turn countdown (`FUN_003977a8`, `FUN_0039783c`, `FUN_0039800c`,
  `FUN_00395efc`): `Num_Blue10_2` digits centred at (400, 300), 50 lower in
  kinds 2 and 7, starting at 30 for kinds 2, 4 and 7–9 and 20 otherwise when a
  round opens with one of our fighters to command (AC11:10 now records the
  kind). `wav0145` sounds each second below 10; at −1 the menus close
  (`FUN_00398120`); an action batch stops it (`FUN_003920a0`).
- The radial now has all native buttons (constructor `0x002974a0`, decoded from
  its jump table): 4, the item bag at (90, 126), opens the item form
  (`FUN_00351700`); 5, `auto` at (42, 126), attacks each round with every
  owned fighter (skill 10001) against the first living fighter of the other
  half, scanning columns 1–2 or 4–3 and rows 1–4 (`FUN_0039c238`,
  `FUN_0039a5b8`). While auto runs, the cancel button `Btn_Battle_5_1` (button 3
  of +0x51c, at (762, 526)) stops it (`FUN_00298f7c`). The auto-play form's
  configured skills are not ported; the native fallback attack is used.

- Attack movement: approach and return travel play the native poses
  (`FUN_003747f4`, `FUN_003753ac`): approach mode 0 runs with the sprite
  animating, 0x41/0x40 by direction (walk actions 6/2 for templates whose
  Npc.dat +0x57, disk offset 83, is 1); mode 1 holds the leap pose 0x24 (right
  half) or 0x25 (left half), 0x10/0x11 for walking templates. Only the
  keyframes show their authored fixed poses. Paths aimed at a fighter of the
  other half are scaled by the attacker-to-target distance over the named
  compatibility reference `TargetReferenceDistance` (column 4 to column 1 in
  row 3, 550 px), so attacks stop short of the target instead of running
  through it. The native re-targeting is not traced: `FUN_003875cc` anchors the
  authored offsets (509 px for the basic attack) on the attacker, but
  `Battle/Screenshot_20261008_091727.png` shows the player stopping beside a
  column-2 target. Ally and self targets keep actor-relative offsets.
- Leaps (approach/return mode 1) arc above the ground (`FUN_00373e58`): with
  v the fraction travelled, a = round((0.5-|v-0.5|)*360), the lift is
  round(round(a-0.003*a*a) * scale), the scale being the record's float at
  +0x23 (approach) or +0x27 (return); 2.0 for the basic attack, about 166 px
  at the peak (`Battle/Stage2/Screenshot_20261009_124026.png`). The name and
  bar lift with the body (+0x2078); the ground shadow (`FUN_0030120c`, small
  cut for players, the template's kind otherwise) stays. Mode 0 runs (the
  dash in `_124502`). Only modes 0 and 1 occur in the 1,832 motions.
- Visual effects (`FUN_00386f60`, `FUN_003725f8`, `FUN_00372be4`,
  `FUN_003728cc`, `FUN_0037273c`): each motion's effect section is decoded
  (ID, attacker-relative points, speed, interval, frame count, first/last
  frame, trigger, animation mode, movement, repeat, direction). The picture
  is `S<ID>` plus a direction suffix (none, L, R, F, B; L and R swap for
  fighters on the right, `FUN_003768ec`), a strip of frame-count rows drawn
  centred on the point, additively at level 30 when its first pixel is the
  marker 1, else keyed. Effects start at their trigger (1-100 a keyframe,
  otherwise milliseconds), step a frame per interval (mode 1 loops, Repeat
  times when set; 2 holds the last frame; 3 ends after it), and stay on the
  first point or travel the points at their speed (mode 2 ends on the last).
  Points are placed like the motion's own, anchored where the attacker
  started. Hit reactions carry the hit sparkles (60011 shows `S10416`).
  Mode 2's special paths (`FUN_00373280`, `FUN_00370e54`) and animation mode
  4 (`FUN_00370c10`) are approximated by travelling and holding; the three
  unresolved effect fields are kept by name.
- Defeated fighters use the falling group, 0x1a on the right half and 0x1b on
  the left (`FUN_0038ab70`), played once and held on its last frame for
  monsters too (the flattened Grape in `Stage2/Screenshot_20261009_124204.png`).
  The translucent remains in `_124204-1` are not explained: the battle draw
  passes full alpha (`FUN_00412c50`'s +0x21b4 is only ever zeroed).
- Hit reactions: the motion record's +0x2c names the motion its targets play
  when hit (`FUN_003790d4`, 60011 for the basic attack: the hurt pose with a
  short knock-back), started at the damage cue for every non-missed target.
- Compatibility: a monster sprite without a motion's action keeps its battle
  stance instead of vanishing (the chameleon has no action 39).
- Music: a battle plays its scene record's music (battle scenes such as
  59081 name `BGM0014`, as map scenes name theirs); the map's music returns
  when the battle closes. The original's music switch (`FUN_002c1de4`) names
  `Sound\戰鬥2.wav` for battles, which neither the install nor the exports
  contain.

The server's turn timeout now follows the native countdown for the kind it
announces (kind 1, 20 s) plus a named 2-second grace, instead of 30 s: the
client accepts commands until its counter passes 0 (`internal/battle.TurnLimit`,
`internal/server/combat.go`).

Server healing and revival now send recovery mode 3, and the amount actually
restored. The old mode 1 told native clients to subtract HP. Revival is capped
to maximum HP even for fighters with a maximum below the compatibility minimum.
Existing server damage formulas and authored skill/effect tables are retained.
The server accepts opaque native attack trailers without trusting them. Unsupported
battle subcommands, including AC50:2 items, are rejected without consuming a turn;
an item ID cannot be interpreted as a skill ID and bypass item consumption.

## Native evidence

The reference is `var/decompiled/aLogin_ca19ee087b60_full.c` and its matching
`var/ghidra/aLogin.exe`, rather than the Private Server's presentation guesses.

| Native function | Behavior used |
| --- | --- |
| `FUN_0038c64c` | Formation coordinates; x87 constants additionally checked in assembly |
| `FUN_0036f448`, `FUN_0036ff5c` | Background lookup/fallback, camera, layer drawing order |
| `FUN_003920a0`, `FUN_00398dac` | Variable-length action and target/stat records |
| `FUN_00392d3c` | Seven-byte absolute stat records, including multiple records per packet |
| `FUN_003970e4`, `FUN_0039783c` | Actor notifications and round readiness |
| `FUN_00398700`, `FUN_0038512c` | Idle and defeated actions |
| `FUN_00377060` | MBTM frames, coordinates, damage cues, effect section and seven-byte sounds |
| `FUN_003a1a9c`, `FUN_003741f0` | Actor-relative mirrored coordinates, directional actions and proportional axis movement |
| `FUN_0039e884`, `FUN_003b9c00` | HP/SP digit sheets and centered 20-pixel digit advance |
| `FUN_00379c18` | Trigger values 1–100 are frame numbers; other values are milliseconds |
| `FUN_0039fdd8` | Percentage damage parts and remainder reconciliation |
| `FUN_00481640`, `FUN_0038972c` | Skill.dat target field 99 and direct target restrictions |
| `FUN_0039b218` | Native attack/skill/capture target cursors |
| `FUN_0039b558` | Result modes 1/2 subtract, 3/4 add |
| `FUN_003800a4`, `FUN_003736ec`, `FUN_00373524` | SEB sound IDs, triggers and once-only playback |
| Constructor `0x002974a0`, `FUN_00297f84` | Radial button dimensions/positions and command meanings |
| `FUN_002991d0`, `FUN_00297f84` case 8 | Previous-action radial artwork and repeat command |
| `FUN_002c2394`, assembly `0x002d3f80`–`0x002d4670` | Separate attack/item request branches and opaque attack trailer |
| `FUN_00437f28`, `FUN_0039bd48`, `FUN_00410e04` | Health bar pictures `icon_rail2`/`icon_HP2`, width and placement |
| `0x4147f8`, `FUN_0038ace4`, `FUN_004265a4` | Battle name colours, lifts and the +0x20b2 height |
| `FUN_0038ad1c`, `FUN_0038f460` | Hovered fighter details and their column-dependent side |
| `FUN_003977a8`, `FUN_0039783c`, `FUN_0039800c`, `FUN_00398120`, `FUN_00395efc` | Turn countdown, warning sound, timeout and per-kind limit |
| Constructor `0x002974a0` (jump table at `0x297657`), `FUN_00297f84` cases 4–5, `FUN_0039c238`, `FUN_0039a5b8`, `FUN_00298f7c` | Item and auto buttons, automatic attack target, cancel buttons |

Constructor `0x002974a0` is absent from the C output; inspect the matching binary
instructions with `objdump`. The native sound record's trailing flag is retained
as an explicitly unresolved compatibility field. It is not assigned a guessed
meaning. The motion timing includes an explicitly documented one-game-tick
scheduling margin; precise camera/movement synchronization remains pending.

## Remaining work

- Exact native movement phase/tick synchronization, approach/return poses,
  ground-height handling and camera tracking. Actor-relative interpolation is
  implemented, with the explicitly documented compatibility scheduling margin.
- Visual spell/projectile/effect tracks, status icons and transitions, actor and
  weapon voices, battle-specific music, recovery/critical digit effects and results.
- The dedicated native battle skill form; the existing standalone Skills form
  currently supplies learned skills and direct scene targeting.
- Native battle item actions and their transactional server execution, manual
  auto-action configuration, viewer/spectator mode and remaining radial commands.
- Camera tracking during actions (`Battle/Screenshot_20261008_091706.png`,
  `_091708`, `_091727`): the camera follows per-skill camera records in the
  MBTM motions (`FUN_00385c04`: mode 1 moves to a point, `FUN_003702a4`; mode 2
  follows a list of camera points, `FUN_003701a4`/`FUN_0036f34c`/`FUN_0036fac4`)
  and actor movement (`FUN_00372a94`, `FUN_00370340`), clamped to the 1664 × 832
  battle field (`FUN_0036fe90`), with the far layers offset separately. The
  MBTM decoder does not read those camera fields yet.
- The top toolbar is not shown in the original's battles, but no battle code
  that hides it was found (battle start `FUN_0038e4f4` and the forms it hides,
  `FUN_002a9ccc`, `FUN_002ac9cc`, `FUN_002afaa4`, are PvP forms); the port
  leaves the HUD unchanged rather than guess.
- The native battle skill form (`Battle/Screenshot_20261008_091659.png`), the
  results banner and level-up word (`_091817`), the capture message (`_091814`)
  and orange/critical digit styles (`_091744`).
- Battle items: `FUN_0038972c` emits AC50:2 and selects motion 10024 for human
  targets, 10025 for NPC targets; item type 24 references its own skill instead.
  The native item-ID/slot request payload and transaction/replay boundary need
  tracing before enabling consumption.
- Live original-client packet comparisons and multiplayer/load acceptance.
  Client decompilation cannot independently prove server-only damage, hit, AI,
  capture, reward or RNG formulas. Their existing compatibility rules must stay
  identified as such until stronger evidence is available.
- Server enforcement of every native self/other-ally target restriction. Client
  targeting uses the verified raw field; the server retains its existing authored
  effect recipients and compatibility retargeting rules.

## Validation

Independent raw-byte fixtures cover formation, turn ownership, repeated readiness,
variable actions, malformed/truncated packets, batched stats, recovery and capture.
All 1,832 exported motions are decoded. Additional tests cover scene commands,
world movement exclusion, direct skill/pet targeting, transient HUD state,
background workspace progression, damage distribution and once-only sounds.
CPU/GPU parity includes formation, radial controls, action/damage, the previous
action icon, exit and roaming NPC movement (20 states).
Server rules have an independent recovery-wire golden and low-maximum revival case.

Validation completed in this development pass: full root and client Go suites,
root/client `go vet`, focused battle/protocol/server/app race tests, and 18-stage
CPU/GPU parity with both loose assets and the compiled bundle. Tests used a
60-minute outer timeout rather than the previous ten-minute limit. The final
focused app race run, including the stale-target corrections, completed in
239 seconds.

`make build-client CLIENT_BUNDLE=/tmp/wonderland-battle-assets.zip
CLIENT_OUTPUT=/tmp/wonderland-battle-client` succeeded, packaging 72,829 files
from approximately 6,017 MiB of source assets and compiling the client binary.
Existing authored sprite exports were used without regeneration. Final request
and stale-target corrections have additional focused regression/race coverage.
The server binary also built successfully with
`go build -o /tmp/wonderland-battle-server ./cmd/wonderland`.

The subsequent roaming/previous-action pass has focused world/battle/hotbar
regressions, session-ownership race coverage and 20-state CPU/GPU parity. See
[client roaming](CLIENT_ROAMING.md) for native packet evidence and the explicitly
named initial-zero-speed compatibility case.

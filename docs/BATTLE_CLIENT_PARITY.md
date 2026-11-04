# Battle client implementation and reference evidence

The Go client now enters a dedicated battle scene using the existing server
simulation. This is a working core, **not complete native battle parity**.

## Implemented

- The radial Skills command opens the separate native battle selector
  (`TRe_BattleSkillForm`, constructor `FUN_0029b674`), using
  `Form_BattleSkill_1`: three recent skills, a five-row scrollable learned list,
  original skill icons and SP costs. `FUN_004816d8` supplies the combat type/order
  filter. Single clicks arm targeting through the existing native/chain request
  paths. Player and pet recent selections remain separate; the general Skills
  module retains its tabs and tree. The selector closes on timeout, playback,
  actor changes, battle exit or disconnect. Visual reference:
  `var/battle-skills-window.png` (2026-11-03).
- Post-combat results follow the native entry snapshot (`FUN_00394f68`) and
  exit comparison (`FUN_00394fe8`): confirmed player/pet EXP gains or losses,
  level-ups and pet amity changes. The summary also lists actual gold changes,
  received items/materials grouped by item ID, and newly joined companions.
  It waits for battle playback and the return fade/map acknowledgment, including
  deferred Gonline settlement. Absolute synchronization cannot double-count EXP;
  malformed inventory packets and ungranted drops contribute nothing. A reused
  pet slot cannot assign its replacement's EXP to the departed pet.
  The native six-second message interval is retained. The compact notice sits
  below the top command icons, sizes to its English text and centers each yellow
  line without a title or close button, matching the legacy reference. Long lists
  show five rows with scrolling. Hover does not extend the six-second deadline;
  clicking the panel or result text dismisses it, while the scrollbar remains
  usable. `FUN_00394fe8` supplies 6000 milliseconds to the native notice board's
  slot +0x9c (`FUN_0034e70c`). The pane uses the
  existing notice artwork; its bounded list is a presentation extension to the
  native message board. It grants nothing and sends no acknowledgment.
- Native AC11:12 victory voice selection (`FUN_00395f84`) chooses one voiced,
  living fighter from the winning side, provided the local player is also a
  living winner. `_bta3`, `_bta2`, `_bta1` have 10%, 20%, 30% probabilities;
  the remaining 40% is silent. Lines wait for the final action/removal and play
  once before departure, including queued Gonline endings. The finish operand
  names the winning side (1 attacker / 2 defender), including PvP. The traced
  defeat path has no separate battle-loss line; `_bt01` in `FUN_00382b08` is a
  knockout reaction, not a victory or end-of-battle defeat recording.
- A single click in the Skills list selects a skill for battle targeting.
  Releasing a dragged skill onto the hotbar still assigns it; ordinary
  out-of-battle clicks browse details and retain their double-click action.
- Both peers retain native combat communication. With the server startup setting
  `combat_round_chain: true` (default false) and explicit client negotiation,
  Gonline sends a complete round chain and batches character/pet choices;
  mixed battles preserve each recipient's transport. See
  [combat transport](COMBAT_TRANSPORT.md) for the extension and native fallback.
- Escape succeeds automatically at equal/higher level than the strongest living
  enemy; otherwise it uses the reference level-based chance. Either owner or pet
  succeeding removes both. Nonleaders leave the remaining participants in combat;
  successful team-leader escape ends the battle. Overkill and logout remain
  separate. Failed escapes play native MBTM 60042's run/trip/recovery track, with
  no stat damage, SP cost or defense bonus, and repeat still submits Flee.
- AC11 formation, participant, finish and departure records; AC50 variable actor,
  target and stat batches; AC51 batched absolute HP/SP; AC52 round readiness;
  AC53 overkill removals and action-submission acknowledgements. Decode batches before
  mutating state. Duplicate ready packets do not reset submitted actions.
- Battle entry and exit transitions (`client/wlo/app/transition.go`), the
  screen transition `FUN_0049a2e8` run each game frame by `FUN_0049a484`
  after the scene and before the forms. Entering (mode 1): the battle
  constructor (func 0x38bcdc) starts the battle music, then the map keeps
  running and darkens with black at alpha 12 × step for steps 1..15; after
  step 15 the callback 0x38c8f0 runs and the frame loop draws the battle
  field (+0x565). The port continues with a 15-step reveal of the field, using
  the same alpha/clock; this requested presentation continuation avoids a cut
  to full brightness and is not claimed as an additional native transition.
  Actions and their sounds wait for the reveal to finish. Leaving (mode 2): when the player's battle
  state clears, the frame loop (0x4a2b12) shows the map again under black
  for steps 15 down to 1 and frees the battle (`FUN_0038fefc`) at the end;
  `FUN_0039b81c` and `FUN_0038cb8c` start the same mode for a fled or departed
  player. AC11:1 departures now finish the authored 60041 outward motion
  before this fade (`FUN_00392bb4`, `FUN_0037896c`, `FUN_00385e58`);
  its completion callback replaces the return leg. Defeated units do not run.
  An early AC11:0 closes command input while queued attacks, reactions and
  departures finish, including in background sessions. A new formation or
  session reset discards the old presentation. Neither transition darkens
  map 11009 (0x2af1). One step per 30 ms game frame;
  battle packets received meanwhile queue and play once the field is revealed.
  `TestBattleTransitions` checks the brightness of each step against the lit
  map and field. No capture shows a transition frame. The other native modes (3–14: wipes and
  40 × 40 tile effects) are reached only from the debug form's buttons, and
  map entry's mode 0xff only waits four frames without drawing.
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
  percentage distribution, native HP/SP digit sheets, outlined item recovery
  numbers and authored sound cues. Late cues remain alive beyond the last frame.
  Variable visual effects, camera records and the fixed weapon/body audio
  tables are decoded. Missing motions use a named compatibility duration.
- Battle effects play each general, weapon and body sound at its authored cue,
  once per action (`FUN_003800a4`, `FUN_003736ec`, `FUN_00373524`). Weapon
  impacts on a reacting target retain the attacking player's sound source.
  Recorded player/companion lines use the native body/head/NPC voice families,
  `_bt` variants and roulette, preferring `odd` over `odd_d01`; a line replaces
  the body fallback sound track. Skill.dat +0x7a can select the native
  `Voice/<km|kw|am|aw>_<three-digit ID>.WAV` fallback. Existing Sound On/volume
  and background-session muting apply. Compatibility: weapon sound groups
  use item types 1–5, with group zero for unsupported types; the full weapon
  resolver is absent from the decompile and remains to be independently traced.
- Camera mode 1 focuses the actor/target/field or an effect anchor; mode 2
  follows the authored points, speeds and frame/millisecond waits
  (`FUN_0037b730`, `FUN_00385c04`, `FUN_003701a4`, `FUN_003702a4`). Positions
  mirror by side and clamp to the 1664 × 832 field (`FUN_0036fe90`). Fighter,
  shadow, bar, damage and effect positions use the same camera offset.
- Close-range zoom follows `FUN_00386880`/`FUN_00386a0c`: attack category 1
  crops up to 100 × 75 pixels per side according to horizontal progress toward
  the target, then restores the full view as the attacker returns. The battle
  scene scales through compatible renderer surfaces; HUD/forms stay unscaled.
  Zoom Mode now toggles and persists this effect. Action completion, battle
  exit and session disposal reset the view and release its temporary target.
- Displayed HP/SP change at those authored damage cues, alongside the floating
  numbers and hit reaction. Multiple hits apply their percentage shares at
  each cue; healing, critical modes and capture use the same timing. Fighter
  bars, defeated poses and player/pet HUD vitals read the displayed roster.
  The decoded roster still updates immediately for command validation;
  following absolute-stat packets reconcile after pending actions finish,
  without pulling displayed damage ahead of contact.
- Damage numbers use the native AC50:1 packet boundary as their local combo
  identity in both native streams and round chains. Single-hit participants
  share one total. When one multi-hit skill is present, its pattern splits the
  whole combo total; identical multi-hit skills share that pattern. Different
  multi-hit skill IDs retain separate groups, and singles form their own group
  when multiple different multi-hit skills participate. Groups remain separate
  per target, roster generation and HP/SP resource. Damage and recovery on the
  same resource retain native stat order to preserve clamping.
  Integer splitting conserves the complete total, including wide combo sums;
  repeated full weights normalize across contacts and missing weights split
  evenly. Visible HP/SP follow those same grouped contact cues. Multi-hit numbers
  alternate left/right in widening, side-mirrored arcs (`FUN_0039e884`,
  `FUN_003a2e08`); movement uses contact age, including late-frame catch-up.
  The arch approximates the native `FUN_003a2ff4` curve. Single-hit groups retain
  vertical separation using their bitmap glyph height. Server damage calculations
  and wire packets are unchanged.
- Overkill uses the native AC53:3 removal (`FUN_00393c58` ->
  `FUN_00392484` state 7 -> `FUN_0039b58c`), rather than interpreting every
  knockout as removal. The sprite rises 300 pixels at one pixel/ms and then
  disappears. It waits for its damage cues, not unrelated actions, so units
  can vanish before the round finishes. Ordinary HP-zero fighters retain
  their defeated pose and can still be revived. Vanished units cannot be
  revived or intercept clicks. Local roster generations prevent an old hit
  or removal from affecting a reinforcement reusing the same cell/template.
- The server tracks received HP damage before remaining-HP clamping during
  each round. Overkill requires **strictly more than 150% of maximum HP**;
  exactly 150% is an ordinary knockout. Combo hits contribute their complete
  sum, even when aggregate HP arithmetic is capped. Misses add nothing;
  healing does not erase received damage; the counter resets each round.
  Area overkills share one flight step. AC53:3 is emitted only for these
  removals, fixing the previous native-client behavior where all deaths flew away.
- After an overkilled player's flight, the server removes only that member
  and withdraws its pet, while teammates continue the round. Vitals, pet
  results and the record-point destination (Starter Beach when unset/invalid)
  commit together before map packets. Existing defeat HP recovery is retained.
  The old map receives the departure, the new map acknowledgment restores
  controls, and the removed member receives no later battle actions/rewards.
  A disconnect during the flight still saves the return destination.
  See `internal/battle/overkill.go` and `internal/server/combat_overkill.go`.
- Successful native capture animations remove the captured fighter at their
  cue; departure records animate living
  participants before removal. Expired damage clears in background
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
- Slow-choice turtles (`FUN_00398524`, `FUN_00391f18`, `FUN_0039800c`): the
  native `E119` sheet has four frames advancing every 250 ms. At ten seconds
  remaining, indicators appear above living, available player/pet fighters still
  awaiting AC53:5 submission receipts. They stop on submission, timeout or round
  playback. Submitting our own choices does not hide an unsubmitted teammate's
  turtle; the next round restarts the secondary countdown.
- The radial now has all native buttons (constructor `0x002974a0`, decoded from
  its jump table): 4, the item bag at (90, 126), opens the item form
  (`FUN_00351700`); 5, `auto` at (42, 126), attacks each round with every
  owned fighter (skill 10001) against the first living fighter of the other
  half, scanning columns 1–2 or 4–3 and rows 1–4 (`FUN_0039c238`,
  `FUN_0039a5b8`). While auto runs, the cancel button `Btn_Battle_5_1` (button 3
  of +0x51c, at (762, 526)) stops it (`FUN_00298f7c`). The auto-play form's
  configured skills are not ported; the native fallback attack is used.

- Battle supplies: double-click a usable bag item from the Items command or
  Inventory, then click a fighter; item shortcuts enter the same targeting mode.
  Escape cancels. Native AC50:2 includes the item ID and bag slot, distinct from
  skill requests. Player and pet commands can use HP/SP supplies on living allies,
  including teammates and their pets. Recovery uses existing catalog values above
  the 100 base, capped to maxima. Native `FUN_0039be38` selects the magic cursor
  (state 7). `FUN_0038972c` and its item eligibility helpers allow selecting allies
  at full HP/SP; recovery evaluates their vitals at execution. All four allied
  formation slots and their pets can be clicked, independently of command ownership.
  Ordinary food cannot revive; the linked native
  Secular Feather skill (15138) enables revival items. Type-24 items use their
  linked skill at decoded Item.dat offset 134; supported signed vital effects and
  authored skill effects share the same execution path. Unknown/no-effect specials
  are rejected without consumption. Recovery uses motions 10024/10025 for
  human/creature targets; special items use their linked motion. HP/SP presentation
  follows the motion's contact cue. Items spend no SP or skill EXP. Each fighter
  submits once, and two actors cannot reserve more items than the stack contains.
  At execution SQL revalidates the bag and commits consumption with target vitals
  atomically before publishing success. Failed saves, stale stacks, defeated actors,
  departed targets and unavailable replacement pets cannot grant free effects.
  The same native actions work in optional complete-round delivery.

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

Social, Team, Inventory, Skills, Compound and Settings can open under the battle's
movement/event locks. Browsing does not enable world-only inventory, party,
dismissal or riding actions. The Team handler follows `FUN_00277728`: a Rest or
Battle request is accepted only while the living player still has a command to
choose; it closes selection without sending Defend. The server consumes both
owner/pet commands, saves outgoing HP/SP/amity before announcing the mode, then
sends the old pet's exit before adding the new pet at the same formation cell.
Incoming pets keep their saved vitals and remain unavailable for actions and
all targeting, including area attacks, in the switch round.
The behavior uses native AC19, AC11:1/5 and AC53:5 for both client transports.

Live legacy-server and Gonline traces on 2026-11-02 established the native switch
order: owner AC53:5, four-byte old-pet AC11:1, owner-only AC19 selection receipt,
then AC11:5 with entry mode **5** and the owned pet slot. Mode 5 is not allegiance:
`FUN_0039108c` sets the participant's unavailable flag at +0x21d1, drawing
(`FUN_0038ca44`) and targeting skip it, and `FUN_0039783c` clears it on AC52:1.
The server protects the joining pet from both direct and area targeting.
Its grid determines its side; both transports use the same rules. The successful trace's
first attack followed the switch batch by 342 ms; the server uses a documented
350 ms compatibility floor, without claiming a universal native animation time.

Gonline deliberately presents the new pet walking in while the old pet walks
out. The outgoing fighter/body survives as a detached visual copy, leaving the
new fighter's authoritative identity, HP and Waiting flag independent. The
240-pixel ground entry uses native walking/running poses; it is a requested
cosmetic change from the native hidden waiting controller. Both native bursts
and round chains allow the replacement through the departure barrier, then hold
attacks, readiness and settlement until both movement tracks finish. The pet
stays visible after entry while direct commands/selection remain blocked until
AC52:1. Readiness does not replay its entrance. New battle/disconnect clears the
copies and queues; normal close delivers queued rewards and map packets once.

Countdown expiry (`FUN_0039800c` / `FUN_00398120`) closes selection and leaves
missing commands idle: no action, SP cost, defense bonus or Defend animation.
Already selected commands and explicit Defend remain intact. A partially selected
Gonline batch flushes only its original native request, preserving the existing
complete-batch protocol. See [combat transport](COMBAT_TRANSPORT.md).

Server healing and revival now send recovery mode 3, and the amount actually
restored. The old mode 1 told native clients to subtract HP. Revival is capped
to maximum HP even for fighters with a maximum below the compatibility minimum.
Existing server damage formulas and authored skill/effect tables are retained.
The server accepts opaque native attack/item trailers without trusting them.
Unsupported battle subcommands are rejected without consuming a turn. AC50:2
has its own item validation and transactional execution; an item ID cannot be
interpreted as a learned skill or bypass item consumption.

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
| `FUN_00277728` | Team pet Rest/Battle requests during the player's command phase, consuming selection without Defend |
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
| `FUN_00398524`, `FUN_00391f18`, initializer `0x0038c182` | Slow-choice turtle eligibility, four-frame animation and native `E119` picture |
| Constructor `0x002974a0` (jump table at `0x297657`), `FUN_00297f84` cases 4–5, `FUN_0039c238`, `FUN_0039a5b8`, `FUN_00298f7c` | Item and auto buttons, automatic attack target, cancel buttons |

Constructor `0x002974a0` is absent from the C output; inspect the matching binary
instructions with `objdump`. The native sound record's trailing flag is retained
as an explicitly unresolved compatibility field. It is not assigned a guessed
meaning. The motion timing includes an explicitly documented one-game-tick
scheduling margin; precise camera/movement synchronization remains pending.

## Remaining work

- Exact native movement phase/tick synchronization, approach/return poses,
  ground-height handling and far-layer parallax. Actor-relative interpolation is
  implemented, with the explicitly documented compatibility scheduling margin.
- Remaining spell/projectile paths, status icons and effect transitions,
  critical digit effects and remaining outcome artwork. Battle scene
  music and entry/exit fades are implemented as described above.
- The dedicated native battle skill form; the existing standalone Skills form
  currently supplies learned skills and direct scene targeting.
- Remaining special battle-item effects without authored rules, manual
  auto-action configuration, viewer/spectator mode and remaining radial commands.
- Exact camera/actor phase synchronization (`FUN_00372a94`, `FUN_00370340`),
  independently moving far layers, moving effect-index camera anchors and the
  zoom crop's ground-height adjustment. Effect camera anchors currently use
  their first authored point. The existing one-tick motion scheduling margin
  remains explicit; camera/zoom parity is not claimed from the decompile alone.
- The top toolbar is not shown in the original's battles, but no battle code
  that hides it was found (battle start `FUN_0038e4f4` and the forms it hides,
  `FUN_002a9ccc`, `FUN_002ac9cc`, `FUN_002afaa4`, are PvP forms); the port
  leaves the HUD unchanged rather than guess.
- The native battle skill form (`Battle/Screenshot_20261008_091659.png`), the
  animated level-up word (`_091817`), the capture message (`_091814`)
  and orange/critical digit styles (`_091744`).
- Live battle-item packet comparisons remain pending server availability.
  The request layout was verified in the matching executable's
  `FUN_002c2394` branch `0x002d4247`–`0x002d44ed`; `FUN_0039be38` selects
  the item/slot and `FUN_0038972c` validates its target. Native client code proves
  layouts and presentation, not server-only special-item formulas.
- Live original-client packet comparisons and multiplayer/load acceptance.
  Client decompilation cannot independently prove server-only damage, hit, AI,
  capture, reward or RNG formulas. Their existing compatibility rules must stay
  identified as such until stronger evidence is available.
- Server enforcement of every native self/other-ally target restriction. Client
  targeting uses the verified raw field; the server retains its existing authored
  effect recipients and compatibility retargeting rules.

## Validation

See [battle validation history](TESTING.md#battle-validation-history) for raw-byte,
CPU/GPU and recorded build/suite evidence. Current rendering limits above remain
separate from those historical results.

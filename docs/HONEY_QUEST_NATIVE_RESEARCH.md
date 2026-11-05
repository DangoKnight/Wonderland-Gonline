# Ill Grandfather: native progression audit

Static analysis, 2026-11-05. No Legacy-server capture has yet validated this
sequence. This audit changes no gameplay definitions or player records.

## Evidence

- Local `data/eve_data.json`, extracted from `eve.Emg`, source SHA-256
  `52bf3603fc1bfd5353456f98122a8da6ee0a313ea88cf92f6a86eed564e392ed`.
- Native maps 12000 (Kelan) and 12004 (Civilian House), parsed with
  `assets.ParseEVEMap`, without applying story repairs.
- `var/decompiled/aLogin_ca19ee087b60_full.c`: native AC20 presentation/ACK
  handling and AC22 actor visibility handling. Client decompilation does not
  establish the official server's event selection or durable storage rules.
- Gonline `internal/world/event.go`, `preevent.go`, `view.go`, and
  `internal/server/events.go`.
- C# reference `Game/QuestRelated/PreEventInterpreter.cs` contains a manual
  baby-bee visibility exception. It is a migration reference, not evidence of
  official server behavior.

Reproduction helpers and decoded records are under ignored
`var/quest-research/honey-native/`: `main.go` / `events.txt` and
`probe.go` / `probe.txt`. Run each Go file separately from the repository root.
The probe evaluates conditions in memory; it does not open gameplay databases.

## What the authored records say

| Location / event | Conditions and actions |
| --- | --- |
| House 12004, event 2, branch 1 (+ condition branch 2) | Quest 13022 not active and mark 13023 value zero: introduction lines 30626–30630; advance 13022 by one. |
| House event 2, branch 3 (+ branches 4–5) | 13022 value one, 13023 value zero, no item 30034: reminder dialogue. |
| Kelan 12000, event 25, branch 1 (+ branches 2–3) | 13023 value zero, no item 30034, available bag space: grant one item 30034 and hide actor 17. No quest-mark operation. |
| Kelan event 25, branch 4 (+ branch 5) | 13023 value zero and item 30034 present: actor 18 speaks line 30005, then battle formation 1. |
| Kelan event 25, branches 6 and 7 | Formation 1 outcome 1 or 2: hide actors 18 and 19. Neither branch writes a quest mark. |
| Kelan pre-event 4 | Show actors 17, 18 and 19 only when item 30034 is absent AND mark 13023 value is zero. All three actors are initially hidden by their map flags (4). |
| House pre-event 2 | While mark 13023 value is zero, apply actor 2's pose 26, held frame 1. |
| House event 2, branch 6 (+ branches 7–8) | 13022 value one, 13023 value zero, item 30034 present: hand-in conversation and actor actions; remove one 30034; give one 23108; remove/complete active journal mark 13022; advance mark 13023 by one. |

The house hand-in's authored mark operations are `5 / 13022 / 2 / 1` and
`5 / 13023 / 1 / 1` (the latter has amount one). No authored bee-battle outcome
sets 13023 to Completed. The two marks are not interchangeable: 13022 tracks
the active request, while 13023's value gates recovery and village visibility.

The honeycomb actor links event 25; the two bees link events 33 and 32, which
only contain their warning dialogue. The honeycomb links pre-event 4.

## Reproduced Gonline discrepancies

The following is a replay of the extracted records through Gonline's current
interpreter, not an assertion that the official server runs identical code.
`houseBranch=0` means no applicable branch in event 2.

| Character state | Native records: house branch / lying | With current default overrides: house branch / lying |
| --- | --- | --- |
| Fresh | 1 / yes | 1 / yes |
| Accepted, no honey | 3 / yes | 3 / yes |
| Carrying honey; 13023 untouched | 6 / yes | 6 / yes |
| Carrying honey; added Completed/step-zero 13023 | 0 / no | 0 / yes |
| After native hand-in: 13023 active/step one | 0 / no | 0 / no |

The added `honeycomb_battle_completion` repair writes Completed/step-zero
13023. `world.conditionAt` treats a completed zero-step mark as value one.
This blocks the hand-in's zero-value condition as well as the lying pose.
`honey_patient_pose_until_hand_in` only compensates for the pose; it does not
repair the hand-in eligibility.

The `wine_actors_visible` override checks whether 13023 is not Completed and
ignores honey inventory. Consequently it shows the three actors when carrying
honey with untouched 13023, and again after the native hand-in leaves 13023
active at step one. Native pre-events hide them in both states. Thus the
workaround cannot safely stand in for native visibility evaluation.

## Interpreter questions with wider relevance

1. **Continuation after a reward:** Gonline automatically searches another
   normal branch only for pure mark/companion operations, unless an explicit
   story continuation exists. Native event 25 first grants the honey, then has
   a later branch whose inventory condition becomes true. This suggests a
   continuation rule, but it does not prove that every reward branch should
   fall through. Capture the pickup-to-warning-to-battle boundary before
   generalizing this behavior.
2. **Transient hides versus reconstructed visibility:** Gonline keeps explicit
   actor hides in the current map view; map re-entry evaluates pre-events.
   Native pre-event 4 already reconstructs hiding while carrying honey and
   after hand-in. It would show the actors again if honey were lost while
   13023 remained zero. Permanent disappearance before hand-in therefore
   needs observation, not an invented completion mark.
3. **Quest lifecycle versus mark value:** zero-step Completed is a Gonline
   representation with special comparison semantics. The official mark
   updates and reconnect state must determine whether those semantics are
   valid more broadly. Do not change all completed-mark comparisons based
   solely on this quest.

## Decompilation findings and limits

`FUN_00307300` sends AC20:6 when the current presentation step completes and
its blocking presentation flags clear. The AC20 receive path invokes
`FUN_00304fd0` to execute the server-provided presentation step. This supports
server-owned continuation rather than the client autonomously choosing the
next reward or battle branch.

`FUN_0038cf30` processes AC22:4 actor records in 14-byte strides: actor ID,
state/frame, coordinates, visibility kind, duration and optional pose. It
updates the current actor object (visibility field +0x1eec, pose via
`FUN_00430f90`, timer via `FUN_004187d8`). The handler contains no quest-mark
write or durable quest completion operation. A hide packet alone does not
establish persistence across logout.

## Capture checkpoints

Keep other characters idle and begin recording before login if possible.

1. Visit the house before accepting; record the patient's pose and dialogue.
2. Accept the request. Pause after the final dialogue for mark updates.
3. Pick up the honeycomb and finish the bee battle, pausing at each boundary.
4. Before handing it in, re-enter Kelan and the house; observe all three
   village actors and the patient's pose. Reconnect once with the honey still
   in inventory to distinguish reconstructed state from a visit-local hide.
5. Hand in the honey and finish all dialogue; re-enter/reconnect to inspect
   the resulting mark flags, inventory and NPC visibility.
6. Optional separate character: lose the honey before hand-in, if the item
   permits it, and re-enter. A loss/escape outcome is also a separate useful
   trial. Neither destructive trial is necessary for the first reference run.

Inspect AC20 interaction/ACK/end packets, AC22 visibility/pose packets, AC23
item changes, AC24 journal/flag updates and battle start/end traffic. Determine
whether the transition into the bee battle requires another click or happens
within the original interaction. Record screenshots and pause points alongside
the packet timestamps.

## Follow-up implementation

### Legacy-server capture, 2026-11-05

The fresh-account reference is saved locally as
`var/pet-switch-capture/honey-reference-20261105-01.pcapng`, with decoded
packets in the corresponding `.events.json` and `-decoded.log`. This trace
includes the hand-in; the player clarified that it had already happened before
the request for a separate hand-in capture. Do not treat its later house visit
as a pre-hand-in visit. The player directly observed the patient still lying
down before handing over the honey.

Times below are relative to the first decoded packet:

| Time | Observed server behavior |
| --- | --- |
| 30.342 s | House entry sets actor 2 to pose 26: `[22,9,2,0,26,2]`. |
| 42.321 s | Accepting the request sends `[24,1,222,50,1]` (mark 13022, step 1), followed by branch 3's reminder dialogue. |
| 71.087 s | A single click on actor 17 grants item 30034 and hides actor 17 with `[22,10,17,0,255,255]`. |
| 71.452 s | The same interaction continues into branch 4, dialogue 30005, spoken by bee actor 18. |
| 75.848 s | Battle begins with enemies 17064 and 17065, each with 43 HP and 100 SP. |
| 98.038 s | After battle, `[22,10,18,0,255,255]` and `[22,10,19,0,255,255]` hide the bees. No AC24 mark update occurs at this boundary. |
| 114.237 s | Re-entering the house again sends `[22,9,2,0,26,2]`; the patient is still down. |
| 123.199 s | Clicking the patient plays dialogue 11083. |
| 127.954–142.340 s | The villager's hand-in runs branch 6, including dialogue and actor operations. |
| 143.505 s | Removes one item 30034, grants one item 23108, sends `[24,4,222,50]` and `[24,5,27,0,1]`, then completes the interaction. |
| 173.542 s | A subsequent house entry finishes without sending the patient's pose-26 operation. |

The packed AC24:5 payload is recorded literally; its native wire flag index
must not be mistaken for a literal mark-ID payload. The EVE hand-in operation
still references 13023. This trace establishes ordering and the absence of a
battle-completion mark packet, rather than exposing the official server's
internal persistence representation.

Both capture helpers have stopped and restored their empty initial filter
sets. The short second trace does not establish a reconnect checkpoint.
Persistence after reconnect and behavior after losing the honey before hand-in
remain separate, unobserved cases; no repeat of the completed hand-in is needed
to analyze this successful reference run.

### Implemented correction

Asset schema v43 removes the stock `honeycomb_battle_completion` repair,
`honey_patient_pose_until_hand_in` compensation and `wine_actors_visible` /
`wine_actors_hidden` overrides. The existing native pre-events now determine
visibility and the patient's pose. The captured pickup-to-branch-4 continuation
remains explicit and scoped to Kelan event 25, branch 1. Imported EVE bytes and
all unrelated quest semantics remain intact.

The migration recognizes the original pre-v41 stock rules and the v41/v42
replacements. It preserves edited rules and intentionally removed collections.
No startup reload overwrites SQL content. Administrators retaining edited honey
policies should review those separately against the trace.

The quest uses the shared modular Story/EVE executor. The former
`internal/assetsql/honeycomb.go` migration has been replaced by the reusable
`story_revision.go` helper and declarative `native_quest_revision_v43.json`
before/after policy data. No compiled migration switch contains Kelan maps,
actors, marks or honey policy identities. The independent historical fixture is
retained in `internal/assetsql/testdata/honey_story_v42.json`. A native-data
integration test disables the authored continuation and confirms that the
battle does not start; renaming that same policy still starts the native battle.

`tools/sql/repair-honey-hand-in.sql` is an optional, transactional gameplay
repair for existing affected characters: completed/step-zero 13023, active
step-one request 13022 and Honeycomb 30034 still in the bag. It removes only
the premature mark, leaving the native hand-in to exchange items and advance
progress. It preserves completed hand-ins and performs no reward delivery.
Run it offline against the configured gameplay database after a backup; it is
not applied automatically by the asset upgrade.

Validation covers stock and edited migrations, intentionally empty content,
repeated migration, both authored bee outcomes, SQL reload before and after
hand-in, the patient's pose, actor visibility, the item exchange and prevention
of a second hand-in reward. The standalone repair SQL was also exercised against
isolated SQLite cases, including completed hand-ins, absent honey, honey in
storage and repeat execution.

### Client restoration after the hand-in

Live Gonline testing confirmed the hand-in and saved progression, but the patient
briefly returned to lying down when the interaction ended; leaving and returning
to the house showed the correct standing pose. The client had saved the initial
lying action for conversation restoration and reapplied it on AC20:8.

The native AC22:8 handler `FUN_00443868` and AC22:9 handler `FUN_004464dc`
call `FUN_0030a344`, removing the actor's saved conversation facing. Event-kind
11 scripted turns do likewise. The general client implementation now releases
that restoration when a supported scripted animation, pose or turn takes over.
Later dialogue saves its baseline from the new authored pose. Ordinary dialogue
still restores facing, while expressions do not release it.

The trace's animation packet is `[22,8,2,0,8]`; Gonline currently adds an unused
compatibility byte. The client accepts both layouts. Regression tests replay
both variants, with and without a later dialogue line, plus scripted held-pose
and turn overrides. This fix needs only a client restart; no quest-state reset
or asset migration is required. Visual retesting of the closing scene remains
pending.

# Legacy acquisition and capture reference — 2026-11-03

User-operated legacy client tests against the legacy server. Original PktMon
traces and decoded payloads remain ignored in `var/pet-switch-capture/` as
`rewards-reference-20261103-01.*`, `-02.*` and `-03.*`. All three captures
completed and restored the initially empty Packet Monitor filter set.

## Successful monster capture

The second trace contains two Rubytail enemies (native template 17063), an
attack weakening one, a successful player capture, a failed pet capture against
the other, victory, a second battle and successful escape. Times below are
relative to the first decoded packet in that trace, not the capture start.

| Time (ms) | Server packet | Observation |
| --- | --- | --- |
| 52508.293 | AC50:6, AC50:1 | Player acts at (4,2); motion 10008 targets (2,1), zero stat amount. |
| 58998.758 | AC15:1 | New pet joins immediately during round execution. |
| 58998.758 | AC50:6, AC50:1 | Next actor, the existing pet at (3,2), performs failed capture motion 10009 against (2,2). Recruitment precedes these packets. |
| 71852.699 | AC50:6, AC50:1 | Remaining enemy acts. |
| 73882.806 | AC52:1 | Another command phase opens. |
| 81257.959 | AC50:1 | Player attacks the remaining enemy. |
| 84469.653 | AC11:12; AC8:1; AC8:2; exit packets | Victory and EXP synchronization, approximately 25.47 seconds after recruitment. |
| 91137.013 | AC11:1; AC11:0; AC11:1 | Next encounter ends by escape; no new EXP update was observed for it. |

The 64-byte AC15:1 recruitment packet identifies template 17063, level 1,
attributes STR/CON/INT/WIS/AGI = **7/8/6/7/9**, amity **60**, and the EXP field
**6**. It appears once; no second recruitment packet is sent at victory.
The installed native definition has attributes 14/17/12/15/17. Its truncated
halves are 7/8/6/7/8, with three odd attributes: the one bonus point landed in
AGI. This sample supports the documented initial stat budget, but cannot prove
the bonus-selection probability distribution.

Gonline currently defers captured-pet recruitment and persistence until battle
settlement. This differs from the live legacy behavior. Its new pets also start
with an EXP field of zero, whereas this recruitment packet contains six. No
inference about the meaning of that starting EXP or a general EXP curve is made
from this single sample.

## EXP acquisition

At victory in the second trace, AC8:1 carries stat 36 with absolute player EXP
397277032. AC8:2 carries collection 4, pet slot 3, stat 36, absolute pet EXP
29634. The values are absolute state updates, not explicit gain amounts. They
arrive with AC11:12 before the scene-exit sequence completes. Without a reliable
matching pre-battle baseline, this capture does not establish the EXP delta or
formula. No level-up was established by the second trace alone.

The third trace establishes a pet level-up at 187130.783 ms, alongside victory:
pet slot 1 first receives absolute EXP 43 through AC8:2 stat 36, then level 2
through stat 35, followed by attributes, combat stats and HP/SP updates. Its
previous EXP update was 6 at 154681.945 ms, giving an observed increase of 37
between those two snapshots. Later, after the pet was defeated, EXP falls from
43 to 42 and amity from 66 to 65 at 227603.229 ms. These observations do not
establish the general reward or death-penalty formula. The player EXP field
remains 201618275 in the third trace; an EXP synchronization packet alone is
therefore not proof of a positive reward.

The first trace also contains failed-capture motion 10009 and EXP synchronization
at battle completion. The user reported repeated failures against unweakened
bees, item-stack rearrangement and an attempted floor drop. Those failures alone
do not identify the precise eligibility check or random probability used.

## Inventory changes and combat item acquisition

The first trace includes AC23:10 stack movement/split responses and an AC23:212
response associated with the attempted floor drop. Neither is evidence of a
newly acquired item. No confirmed battle-drop/item-gain packet was present in
the first two traces.

The third trace contains the user's reported combo awarding two items. Both are
item 46062, **Bee Spine** in the installed catalog. At 214568.735 ms, before
battle exit, the server sends, in order:

1. AC23:6, one Bee Spine inventory addition (33 bytes).
2. A second identical AC23:6, one more Bee Spine.
3. AC53:4: `[53,4,238,179,1,1,4,2,3,2]`.
4. AC53:3 removing the defeated enemy at (1,1).

The AC23:6 payload begins `[23,6,238,179,1]` and has 28 trailing zero bytes.
Bytes 238/179 are the little-endian item ID 46062; the following byte is count
one. No explicit bag slot is carried by this captured layout. Each packet is a
separate gain, not a duplicate observation: they appear sequentially in the
reassembled TCP stream. The corresponding combat result at 202866.613 ms is a
two-actor AC50:1 packet targeting (1,1).

`FUN_00394e14`, dispatched by AC53:4, identifies the item, source fighter cell
(1,1), then iterates destination-cell pairs (4,2) and (3,2), passing them to
`FUN_0039bb2c` for the item effect. The `1,1` bytes are source coordinates, not
quantities. Thus the inventory grants and the item flight/recipient effect are
separate messages. The captured pair and the user's observations support two
combo recipients in this encounter; they do not prove a general per-participant
drop probability or multiplication rule.

Another enemy acts in the next round at 218116.677 ms. Victory/exit occurs at
227603.229 ms, **13.034 seconds after the inventory additions**. A pet knockout
between the gains and victory does not prevent the grants already being sent.
The trace also contains a separate early AC23:6 for item 35117 (Small SP Set in
the installed catalog); the user did not identify its cause, so it is not used
as evidence for battle drops.

## Gonline differences at capture time

- Captures are recruited and saved at settlement, whereas legacy sends AC15:1
  during round execution before advancing to the next fighter.
- Monster loot is rolled/granted at settlement. Legacy's observed inventory
  grants occur during combat, followed by a separate AC53:4 item effect.
- Gonline has no AC23:6 acquisition handler for these captured 33-byte,
  item-prefixed grants. Its separate `InventoryCompoundResult` is AC23:8 and
  expects a 34-byte slot-prefixed layout; that is not this acquisition packet.
  Battle-result item accounting currently handles AC23:5.
  This affects compatibility with an actual legacy-server acquisition, not just
  reward notification timing.
- The observed absolute EXP and level-update channels already exist in Gonline.
  A single successful capture and level-up cannot validate its reward formulas.

## Scope

These are observations, not implementation changes. Transport reassembly removes
duplicate observations from PktMon's multiple network components. Outgoing
requests are incomplete in these traces; missing client requests do not establish
that an action occurred without an acknowledgement. The immediate AC15:1 timing
and server-side packet ordering are independently visible.

## Implemented policy

The subsequent implementation uses one loot roll per defeated monster. Every
distinct player/pet contributing a landed strike to its killing combo receives
one copy of the selected item; pet copies go to their owner. A player plus pet
and a second player therefore yield two copies for the first owner and one for
the second. Recipients' SQL updates commit atomically before success packets.
Full inventories receive no additional copy. Neither settlement nor repeated
kill handling grants the items again. Individual damaging-item and area kills
use their acting entity as the contributor; PvP and captures grant no kill loot.

Capture recruitment commits during calculation and appears after its animation,
before the next action. Grants are retained through later escape/disconnect.
Captured pets start with **0 EXP**, as explicitly requested despite the observed
legacy field of six. Pet knockouts retain the existing amity penalty and do not
subtract pet EXP. EXP/level-up settlement remains on its existing channel.

Both native and Gonline round-chain playback carry private AC23:6 gains and the
AC53:4 item-flight effect. The client records native acquisitions for the result
notice and handles immediate AC15:1 recruitment notices. Flight interpolation is
a time-based approximation; packet formats and ordering come from this trace.

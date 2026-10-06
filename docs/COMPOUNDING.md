# Compounding

Gonline uses a rank-and-material model for AC23:14/87/101, compatibility AC40, and the
`/compound` (`/synthesize`) chat shortcut. Legacy `AlchemyRecipes` and
`Economy.Synthesis` rows remain preserved for source provenance; their item
pairs, output choices, success percentages and fees no longer drive synthesis.
Manufacturing remains separate and continues to use its recipe definitions.

## Inputs and material bases

AC23:14/87/101 carry a count followed by two to five ordered, distinct bag slots:
`23, 14, count, slot1, slot2, ...`. AC40 keeps its four-byte two-slot signature;
its seven-byte hotbar assignment path is unchanged. The chat shortcut accepts
`/compound <slot1> <slot2> [slot3] [slot4] [slot5]`.

Every selected slot consumes **one unit**, including books. At least two
non-book materials are required. A stack contributes its rank once; split a
stack into distinct bag anchors to contribute multiple units. Duplicate slot
numbers, empty/locked/reserved slots and an active vehicle are rejected.

Rank is decrypted Item.dat byte 45. Five ordered material-base words occupy
bytes 136–145. The first word is the primary base. Native material eligibility
rejects restriction byte 112, flag bit 2 at word 123, missing primary bases,
and records whose footprint is not 1 × 1. The Exotic item category (type 10)
is excluded by server policy from both ingredients and result candidates; the
Go client also prevents selecting it. Normal output candidates also need a
positive rank. These are typed projections of `catalog_native_items` loaded
from `assets.db`, with no runtime file reads. The derived candidate index is
cached per server and invalidated when administration replaces the catalog.

Example: Magic Gloves (23085) has rank 20 and bases Flower / Magical Item.
Magic Glove (23171) has rank 37 and bases Flower / Fur / Magical Item. The
Compound hover tooltip displays the native base names as well as item rank.

## Outcome rules

1. The minimum rank among non-book materials supplies the **base rank**.
2. Primary uses the first non-book material's primary base. Junior and Superior
   add ingredient ranks by **primary base** and choose the largest sum. Ties
   retain the earliest ingredient. Paper 3 + Wood 2 + Wood 2 therefore chooses
   Wood, whose sum is 4; its base rank is 2.
3. Other bases from all materials form the secondary requirements, without
   duplicate base codes. Stronger totals take precedence in presentation;
   candidate matching preserves the primary and compares secondary bases as an
   exact set. This handles items with more than two bases. A normal result may
   not invent extra bases or lose required ones through rank fallback.
4. Primary and Junior may lose all secondary requirements. Superior retains
   them. This is a separate random roll from rank selection.
5. The maximum positive delta is +4 for Primary/Junior and +5 for Superior.
   Alchemy Books 1–4 (34009–34012) add their number to the entire rolled
   delta. For example, +2 with Book 1 becomes +3; -4 becomes -3. Books contribute
   neither rank nor material bases. Only the highest included book counts;
   **all selected books are still consumed**, so adding weaker books wastes them.
6. Roll a weighted delta from -4 through the tier cap. The rank ceiling is
   `max(1, base_rank + rolled_delta + highest_book_bonus)`. Find candidates with
   the required bases at that rank, then descend without a fixed drop limit until rank 1.
   Choose uniformly among items at the first matching rank. Do not choose a
   higher rank when the rolled ceiling has no match. If no candidate exists at
   any allowed rank, trigger a catastrophic failure instead. Required material
   combinations with more than five distinct bases also trigger a catastrophe.
   These outcomes consume the ingredients and award junk; they never refund
   ingredients because a normal result cannot be found.
7. A separate catastrophic-failure roll takes precedence over the normal roll.
   Its pool is Common Stone (43001), Straw Mushroom (41005), Dirty Water
   (60003, called **Sewage** in WLRI), Crude Oil (47001), and Steamed Buns (32024).
   If any available pool item has the chosen primary base, choose only among
   those matching items; otherwise choose uniformly from the available pool.
   A catastrophe consumes ingredients and delivers its junk output atomically.

Native requests select the tier: AC23:14 uses Primary 15997, AC23:87 uses
Junior 15998, and AC23:101 uses Superior 15999. Junior and Superior requests
require that skill to be learned; a rejection consumes nothing. The native
Junior checkbox and Superior selector control these commands. The Go client automatically uses the only learned alchemy skill and hides tier
controls in that case. With multiple learned skills, it offers the Junior
checkbox or Superior tier selector, showing only learned choices;
compatibility AC40/chat commands choose the highest learned tier. Its own level, clamped to its native maximum of 30,
adjusts the curves.
Characters without an alchemy skill use Primary at level 0. A committed attempt,
including a catastrophe, gives the selected learned skill 1 EXP through the
existing skill progression system, extended to the native 30-level alchemy cap.
Alchemy uses the native Formula.dat curve: advancing from grade `g` costs
`roundToEven((g+1)^3.1) + 5` EXP. Level 1 therefore takes 14 committed
attempts to reach level 2; level 2 takes 35 more. SQL stores per-grade EXP.
Login snapshots and AC8:1 stat 111 carry cumulative EXP; stat 110 updates
the grade. AC5:11 is a three-slot recent-skill list, and AC5:12 changes a
character model; neither is an alchemy proficiency update. These native
corrections apply to alchemy; older combat skill compatibility remains separate. Debug results include skill ID and EXP/grade
before and after the committed attempt. Rejections give no EXP. No skill is granted
implicitly to characters that have not learned one.

## Initial probability policy

These are approved **Gonline balancing defaults**, not recovered original-game
rates. All coefficients live in `internal/game/compounding_policy.go`; edit
that file and rebuild Server to tune them. They cannot change during play.

| Tier | Catastrophe at level 0 → 30 | Secondary loss at level 0 → 30 |
| --- | --- | --- |
| Primary | 24% → 14% | 80% → 60% |
| Junior | 12% → 6% | 45% → 25% |
| Superior | 4% → 1% | 0% |

Catastrophe decreases linearly across all 30 levels, by a total of 10, 6 and
3 percentage points respectively. Secondary loss decreases by 20 percentage
points across all 30 levels for Primary and Junior. Rolls use 10,000 units per
100%, preserving fractional percentages.

Rank-delta weights before skill-level adjustment:

| Tier | -4 | -3 | -2 | -1 | 0 | +1 | +2 | +3 | +4 | +5 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Primary | 10 | 18 | 22 | 25 | 25 | 18 | 12 | 7 | 3 | — |
| Junior | 3 | 6 | 10 | 12 | 20 | 25 | 22 | 16 | 8 | — |
| Superior | 1 | 2 | 4 | 6 | 15 | 23 | 26 | 22 | 16 | 10 |

Positive weights are multiplied by `1 + 0.05 × skill_level`; nonpositive weights
remain unchanged. Normalize by the sum of all weights to obtain probabilities.
Books shift this distribution without changing the weights: Book 1 moves every
delta up by 1, Book 4 by 4. The minimum and maximum both increase by that bonus. Rank fallback and
secondary loss can lower actual output ranks further than these delta weights
suggest. The configured catastrophe chance is rolled before secondary loss and
rank delta. An unavailable normal result forces an additional catastrophe, so
actual catastrophe frequency also depends on candidate availability.

## Persistence and verification

All three entry points validate current SQL ownership, inventory and learned
skills inside `Store.MutateOwnedCharacter`. Debit every selected input, deliver
the output, and update proficiency in one transaction. A full output footprint,
stale snapshot, unknown item, failed random source or failed save rolls back the
whole attempt. The session adopts committed state while retaining pending
walking. Receipts follow commit; a failed receipt does not undo delivery or
cause an automatic reroll. AC23 retains fresh-result placement and AC23:9/8/13
receipts; AC40 and the chat shortcut retain ordinary stack-aware grants.

Debug logging records tier, level, base rank, selected primary, required bases,
book bonus, final delta including the book/ceiling, final rank, secondary loss and catastrophe for
committed attempts. Existing sensitive-resource tests cover metadata, rectangular
placement, full bags, locks, ownership gates, save/receipt failure and replay;
new tests cover rank sums, book caps, fallback, base preservation, curve ordering,
multi-input SQL consumption, proficiency, forced-catastrophe consumption and
concurrent requests.

Manual aLogin scenarios are available as `compound-*` in
[Live testing](LIVE_TESTING.md#compounding-acceptance-batch). Generated reports
include real SQL-derived ingredient tables, isolated credentials and packet/log
checks. They remain NOT RUN until the player performs the checklist.

Native aLogin selects ingredients with a left click and enforces one book per
recipe (`FUN_0021bab0`, `FUN_0021bd90`). Go also supports single-click selection;
its server permits multiple books according to the highest-book policy above.
The corrected native five-input checklist uses four materials plus one book.

## Native tier investigation

`FUN_0021bd90` constructs the outgoing recipe using Delphi string literals
at `0x21cb90` (`17 0e`, Primary), `0x21cb84` (`17 57`, Junior), and
`0x21cb78` (`17 65`, Superior) in the decompiled WLRI build. All three use
`[23, command, count, ordered bag slots...]` and share AC23:9/8/13 replies.
`FUN_0021d50c` loads the native character-specific `MiddleCompoundUse` and
`HighCompoundUse` preferences; the Junior checkbox selects the command locally.
The Go client implements the Junior checkbox and three-way Superior selector.

The `compound-junior-03` capture contained 22 accepted Primary requests
(AC23:14) and four ignored Junior requests (AC23:87). Before this fix, the
server incorrectly awarded the Primary attempts to Junior, leaving Junior
at grade 1/22 EXP. Existing test state is retained; it is not retroactively
reassigned. Rebuild/restart Server before repeating the checked Junior test.

The native tooltip (`FUN_002a2234`) subtracts the cumulative base for the
current grade, divides by the next grade requirement, and clamps an overfilled
unadvanced bar to 99%. This explains grade 1/22 EXP displaying 99%, rather
than 22%. Earlier Go builds used the legacy `grade * 100` curve. Existing
overfilled per-grade EXP is carried forward on the next committed attempt
under the native curve; grade 1/22 EXP becomes grade 2/9 EXP after one attempt.
The coefficients are compiled in `internal/game/alchemy_progress.go`, recovered
from Formula.dat coefficient 30 (3.1) and `level_scaler` (5).

## Client presentation

`client/wlo/inventory/compound_presentation.go` ports `FUN_0021cf04`: the
23-frame cauldron sequence uses 200 ms frames, releases the confirmed result
at index 14, and flies its icon to the AC23:13 bag anchor. Inventory remains
authoritative throughout; only the compound bag icon waits for landing.
`compound_tiers.go` ports the Junior checkbox (`FUN_0021dd44`) and Superior
Primary/Junior/Superior menu (`FUN_0021de54`) with native button artwork.
Selection is local to the session, retained across hide/show and reset on
character handoff. Server revalidates learned skills inside the transaction.
Legacy broadcasts AC23:122 as a synthesis effect, but the native dispatcher
clears fishing state for that packet. Gonline no longer sends it for recipes.

# Fishing

## Playing

Own a Simple Fishing (37105), Great Fishing (37011), or Luxury Fishing (37082)
rod and stand on land near water at a configured beach. Leave your party, finish
active interactions and dismount first. Double-click the rod in aLogin or use
`/fish`; `/stop` cancels casting. Fishing works without learning a fishing skill.
The existing EVE quest system remains responsible for teaching the skill.

The default catch interval is 60 seconds, following the WLRI 2020 observation.
Each interval delivers one weighted catch. There is no catch-up burst after a
scheduler delay. A full bag discards the item while retaining skill proficiency;
a compatible existing stack can still receive it. Movement, travel, disconnect,
battle, trade, party membership and losing the selected rod end casting.

## Definitions and tuning

Structured asset schema v7 introduced the `Fishing` dataset; v8 increases the
default fish/seafood weights. Admin's definition editor
can change it; server gameplay uses its validated startup catalog, so restart to
activate edits. `internal/assetsql/fishing_defaults.json` seeds new databases and
explicit upgrades only. Editing this seed does not replace existing SQL edits.
Runtime never reads the JSON seed from disk.

- `Enabled`: enables fishing; initialization disables it if required rod/catch
  item definitions are unavailable.
- `IntervalSeconds`: seconds between catches (default 60).
- `Maps`: permitted beach map IDs (defaults 11016, 10035, 10039).
- `Rods`: item ID and maximum catch grade (defaults 6, 8, 10).
- `Rewards`: explicit item IDs, content grades and positive weights. The seed
  contains a conservative WLRI-mapped subset of the beginner-beach table: fish,
  coal, steel, sulfur, magnet, quartz, shale, clays, ink and milk. It does not
  automatically include every item below grade ten, equipment or Sky Sword Box.
- `Skills`: primary/junior fishing IDs 15993/15994. The highest learned grade
  determines weighting and receives proficiency; fishing never auto-learns one.
- `SkillBonusPercent`: each skill grade adds this percentage times the catch's
  content grade to its weight. The initial base weight is `floor(10000/grade²)`, multiplied by **10** for
  the eight fish/seafood rewards (Croaker, Tilapia, Sardine, Salmon, Eel, Tuna,
  Octopus and Cuttlefish);
  effective weight is `base × (100 + skillGrade × catchGrade × bonus)`.
- `CatchRequirements`: catches required at each learned grade before the next
  grade. The seed uses the wiki's 14, 35, 79 … 5410 table as per-grade requirements.
  One catch earns one proficiency point. After the last requirement, advancement
  stops at grade 16; no undocumented levels 16–30 thresholds are invented.

Weights, per-grade threshold interpretation, rod grade ceilings and beach-map
assignment are explicit authored defaults where the references are incomplete.
The older Japanese source reports 120-second catches; choose 120 if desired.
Seasonal 2020/2021 rewards and box recipes are excluded because their dated event
windows have expired. Exact event odds, all-region pools and fishing from boats
remain research work. These defaults do not claim exact original probability
parity. See [saved references](NATIVE_COMMANDS.md#saved-fishing-references).

With no learned skill, fish/seafood comprise approximately **55–56%** of
catches across the three default rods. Grade restrictions and the existing
skill bonus still apply. SQL weights remain editable.

## Protocol and persistence

The aLogin decompilation's `FUN_0046a354` starts fishing and
`FUN_0046a8e4` stops it (WLRI equivalents `FUN_00450700`/`FUN_00450c90`).
The generic sender's AC23 dispatch was not recovered by the old decompilation.
Inspection of WLRI build `ca19ee087b60` resolves its AC23:53 arm at
`0x2c9ea8`: it writes **[23, 53, bagSlot, presentation]**, reading client fields
`0x20b6` and `0x20b7`. The former indexes the inventory; the latter selects
fishing animation frames. The live capture `17 35 01 01` selects bag slot 1.

Native starts use that explicit slot and validate the owned, unlocked rod.
The presentation byte does not select catch grades, weights or rewards; SQL rod
rules remain authoritative. Historical two-byte starts choose the strongest
available configured rod; the previous four-byte zero-word adapter is retained
with the same behavior. Partial operands, out-of-range slots and extra bytes are
rejected. An empty/non-rod/locked valid slot declines casting without disconnecting.

The native stop sender at `0x2c9f8e` emits **[23, 54]** without operands.
The older zero-word stop adapter remains accepted. AC23:54 still serves legacy
mall refresh when no cast is active. Debug diagnostics include bounded hex bytes
for received fishing controls. AC23 item-use requests retain their explicit slot.
AC90:1/2/3 remain compatible
start/attempt-catch/stop requests. Successful legacy casts also receive the source
AC90:2 item/count receipt after each committed catch (count zero when discarded).
A catch request cannot bypass the deadline.
### Native feedback and progress

Rod visibility uses peer-only **AC23:123 + character ID + presentation byte**
and **AC23:122 + character ID** on stop. The WLRI receive dispatcher sets and
clears fishing flags for those IDs; late map entrants receive the held rod state.
The presentation comes from the SQL native item record's byte 45, as recovered
from `FUN_003cfaf0`; malformed client animation hints cannot select arbitrary
frames. Movement, logout, warp, lost rods and other fishing interruptions clear
peer state. The owner updates its selected rod slot locally before sending these
requests, so start/stop replies are not echoed to it: its stop handler dereferences
the selected slot, which the native sender has already cleared.

Successful delivered catches send **AC23:51 [petSlot=0, itemID:u16, count=1]**.
`FUN_003d9984` uses the client's item name and logs the acquisition in chat.
The catcher also receives a private AC2:16 notification:
`Fishing: caught [item name] x1.` The name comes from the SQL-derived item
catalog. Both messages go directly to the catching session after the reward
transaction commits; observers receive neither.
Full bags retain their warning and proficiency, without a false receipt or flight.

The catch flight reuses the recovered native ground pickup renderer:
`FUN_003dc408`/`FUN_003d73b8` creates one temporary visual with AC23:3; the
immediately paired AC23:2 animated removal calls `FUN_003e0548`, which flies
its item icon to the catcher. The source is a nearby SQL water-cell center.
Only the catcher sees these packets. No shared ground item, claim or second
reward is created. Both writes remain ordered under the world lock. The adapter
checks the client's first-free-slot allocation against available server ground
slots; if they disagree or all slots are occupied, it skips the visual while
retaining the catch and chat notice. It avoids replacing the full ground snapshot.
This is reuse of a verified renderer, **not a captured original fishing reward
sequence**; live visual acceptance remains required.

Fishing EXP now uses **AC8:1 stat 111**, with stat 110 for grade changes. The
previous AC5:11 reply was incorrect: `FUN_0029c5d0` reads three UI operands and
does not update skill EXP. `FUN_00453018` renders the fishing overlay using raw,
cumulative skill EXP and subtracts the previous level's cumulative requirement.
`FUN_0036e96c`/`FUN_0036e9b8` calculate those native requirements; the installed
Formula.Dat gives `round(level^3.1) + 5` from level two, matching the seeded
14, 35, 79 … table. SQL still stores per-grade progress. Catch packets and all
login/GM/admin base-stat snapshots add completed requirements when serializing
fishing skills. This keeps reconnect displays consistent without changing saved
values or combat-skill progression. Custom requirements may differ from the
native client's compiled/Formula.Dat percentage display.

These addresses refer to the recovered WLRI build with SHA-256
`ca19ee087b601182e872235e58e6bf367f4bb21879226dfe3cc96f1c99624f61`.
Focused packet tests pass. On 2026-10-04, the tester reported Fishing Skilled
02 working properly; the first run was blocked by external conditions. See
[recorded live outcomes](LIVE_TESTING.md#recorded-live-outcomes). Individual rod,
flight, notification and overlay checklist observations were not supplied.
The subsequent Fishing Unskilled, Full Bag and Advancement runs were reported
with all checks passing. Advancement played the native level-up animation and
voice line. Exact original probability parity is still unresolved.

The SQL-derived Ground.MMG collision grid provides eligibility: walkable land
with water cell 2 in the native 14×14 neighborhood. Missing terrain fails closed.
Active casts remain in memory. Gameplay schema v13 adds `fishing_progress`,
owned by the character, containing total catches and the next eligible deadline.
Rod ownership, replay/cooldown validation, inventory delivery and learned-skill
progress commit in one transaction. Failures roll back everything; reconnect or
recast cannot reset a future deadline. Starting after an expired deadline waits
a fresh interval. SQL results preserve pending walking, and no packets are sent
before a catch commits. Offline time does not generate rewards.

## Upgrading

Stop the server, then follow [the database copy procedure](ASSET_DATABASE.md).
`go run ./cmd/database-migrate -config config.local.json -output-dir <new-directory>`
creates upgraded copies without replacing the source databases. The v7 → v8
upgrade changes fish weights only where both grade and weight still match the
old defaults. Custom weights/grades, removed rewards and other rules are preserved.
Repeating migration never reapplies the multiplier. Point the startup
configuration at those copies before starting the updated binary. The gameplay
upgrade is additive; no reset or loss of accounts is required.

Focused verification covers weight boundaries, shoreline/bounds, native start/
stop, early/replayed catches, full-bag proficiency, pending walking, SQL rollback,
wrong ownership, rod removal, concurrent claims, reopening and preserved asset
edits. No live client test has been performed by these automated checks.

# Manufacturing and character state

## Native manufacturing

AC64 start accepts `[64,1,bench:u8,formula:u16]`. Formula IDs retain original
Compound2 ordinals. `Manufacturing` in assets.db supplies five ingredient pairs,
output/count, plan, tool, duration in seconds and bag/tent destination. Invalid
native header rows and recipes referencing absent WLRI items are filtered offline.
Admin can edit these typed definitions. Runtime never reads Compound2 or JSON.

Start requires the character's own tent, the matching placed bench or owned bag
tool, and any required plan. Plans/tools are retained. SQL debits all ingredients,
including repeated IDs, and creates one durable job per character atomically.
The transaction checks output capacity before accepting the job. Notifications
follow the commit. Visitors cannot manufacture using another character's tent.

AC64 continue/stop accept the native header-only requests or the source layout
with a bench byte. Stop pauses the job and retains spent materials as pending
output; continue resumes the remaining time. Repeated start never spends twice,
and repeated continue never extends an active timer. Active deadlines survive
restart and elapsed offline time; delivery occurs when the character is connected
and ready. Completion grants the bag output or places tent furniture, then removes
the job in the same transaction. A container that fills during manufacture retains
the output and pauses delivery: make space and continue. A failed SQL write retains
the job for retry. There is no periodic whole-character save for these operations.
Buffered walking is merged back into the session after resource transactions.

The reference ignored build time. Imported `build_time` is interpreted as minutes
and converted to seconds; the start reply sends remaining seconds. These units,
the native timer animation and replay need aLogin acceptance. Portable tool
exceptions follow the source. Weapons/consumables go to the bag; furniture goes
to the tent. Floors remain governed by existing placement rules.

## Two-input recipes and synthesis

AC59 retains its native two-input Forge request and reply. `/manufacture` retains
workbench-name matching. `Economy.Manufacturing` recipes support optional
`SuccessPercent` (absent means 100%) and `Fee` in gold (default zero). A failed
attempt consumes the recipe materials and fee without an output. Successful
output overflow, missing materials or insufficient gold rolls the transaction back.

Offline conversion reads authored alchemy recipe chances/fees into
`Economy.Synthesis.Rates`. Existing customized rates survive upgrades and repeat
conversion. `/compound` debits fee and ingredients and grants the selected result
or charcoal atomically. Native deterministic AC23/AC40 paths retain their rules.

AC37 socket requests return failure without consuming items. Reference ForgeGem
never improves equipment, and its hardcoded gem IDs identify WLRI oil/fuel. Real
socket effects and verified gems remain pending; mall forging is separate.

## Character and pet metadata

Gameplay schema v14 stores character nickname, actual class and potential,
plus potential on party, reserve and hotel pets. Login selection/appearance/base
state and social rosters replay applicable metadata. Potential is preserved
independently of level in SQL; native snapshots carry a byte. Player AC5:3 and
pet AC15:1/8 snapshots encode rebirth, potential and job in their verified order.
Do not send potential as stat37: native stat37 addresses another field (0x1fa4).
AC23:213 publishes pill results. Other unverified pet metadata retains its prior
values; hotel record potential semantics remain pending native verification.

Actual `Job` selects class growth. `RebornJob` remains distinct AC66 metadata.
Native inventory potential use is AC23:126; free AC68:1 training remains rejected.
See [Potential Pills](NATIVE_COMMANDS.md#potential-pills) for funded consumption,
stat bonuses and the pending level-dependent normal/golden success rates.

## GM rebirth

`/reborn <Killer|Warrior|Knight|Wit|Priest|Seer>` requires GM permission, idle
ownership, level 100 or greater, and no previous rebirth. SQL grants the class cape,
sets actual class/rebirth, resets level to 1 and EXP to zero, and refills vitals.
Allocated attributes and stat points are retained as in the reference. A full
bag rolls everything back. Repeated requests cannot grant another cape. Appearance,
base stats, EXP, combat stats and the source aura are refreshed after commit.

`RebornClasses` in assets.db controls enabled classes and cape mappings. Embedded
initial defaults in `internal/assetsql/reborn_defaults.json` select the first
WLRI cape variant per class (25231, 25281, 25331, 25381, 25431, 25481). These are
authored defaults replacing the reference's incorrect glove IDs. They can be
changed through Admin. No class skill is granted: the source implements none.

Compiled class bonuses in `internal/game/reborn.go` multiply rounded innate
stats by 1.1 before equipment: Killer ATK/SPD, Warrior ATK/DEF, Knight DEF/SPD,
Wit MAT, Priest MDF, Seer SPD. Existing elemental growth remains compiled.
Reborn EXP uses the source powers 3.3 and 4.9, with displayed levels normalized
to 1–199 to avoid the source's subtract-100 byte wrap. Native HUD contributions
and class display still need aLogin verification.

## Tent metadata and remaining limits

Both floor/wallpaper pairs persist in wonderland.db. Tent snapshots replay each
present peer's nonzero pose through AC32:2. Upper-floor access, upgrade purchases,
funded training, verified sockets and class skills remain pending source systems.
Optional legacy SQLite import still rejects unsupported metadata explicitly;
these additions do not silently reinterpret old string-packed fields.

Upgrade using [the verified database-copy procedure](ASSET_DATABASE.md). Gameplay
schema v15 and asset schema v9 are required. Installation databases are not reset.

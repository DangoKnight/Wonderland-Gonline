# Manufacturing and character state

## Native manufacturing

Native AC64 start accepts `[64,1,bench:u16,formula:u16,batch:u8?]`.
An absent or zero batch means one. The older five-byte start with a byte bench
remains a compatibility adapter. Formula IDs retain Compound2 ordinals; typed
`Manufacturing` definitions supply the five ingredient pairs, plan, tool, output
and duration. Runtime reads SQL. Invalid header rows and unavailable item
references are filtered offline.

Native starts require the owner's tent and the selected placed tool or attached
appliance. Native bench indices are one-based; zero addresses the first bench for
compatibility. The older adapter also permits its owned bag-tool workflow.
Plans and tools are retained. Ingredient multiplication uses wide integers;
output counts above 255 and total durations over seven days are rejected.
Repeated ingredients, exact consumed instances and one job per character are
stored atomically. Start replies describe the batch and remaining seconds.

Native stop cancels the job and refunds every consumed instance, preserving its
damage and metadata. Cancellation rolls back if the whole refund cannot fit.
The old five-byte workflow retains its pause/resume behavior. Native continue
queries an active job or resumes retained output. Deadlines and material escrow
survive restart; elapsed offline time counts toward completion. A full bag or
occupied tent retains the job for retry, without spending ingredients again.

Completion commits output and job removal together. Weapons, consumables and
one-cell appliances go to the bag. Larger appliances/decorations find an available
owned tent position. Wallpaper/floor recipes apply one surface and deliver spare
copies to the bag. SQL failures retain the recoverable job. Buffered walking is
merged back into the session after resource transactions.

Native formulas store build time in minutes; offline conversion produces seconds.
Timer animation and native-client acceptance are tracked in [TESTING.md](TESTING.md).

## Two-input recipes and synthesis

AC59 retains its native two-input Forge request and reply. `/manufacture` retains
workbench-name matching. `Economy.Manufacturing` recipes support optional
`SuccessPercent` (absent means 100%) and `Fee` in gold (default zero). A failed
attempt consumes the recipe materials and fee without an output. Successful
output overflow, missing materials or insufficient gold rolls the transaction back.

Offline conversion preserves authored alchemy recipe chances/fees in
`Economy.Synthesis.Rates` for provenance. They no longer control gameplay.
AC23:14/87/101, AC40 and `/compound` share the rank/base engine, including books,
learned tiers, catastrophe outcomes and transactional proficiency. They do not
charge the legacy synthesis fee. See [compounding](COMPOUNDING.md).

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
stat bonuses and the implemented level-dependent normal/golden success rates.
Super Pills guarantee advancement below the potential cap.

## Character rebirth

Native EVE service 10 opens AC79:1. The server accepts selection only after
opening the form; level 100 and no prior rebirth are required. Rebirth deducts
99 levels (100 → 1, 120 → 21, 199 → 100), switches to the reborn EXP curve,
preserves allocated attributes/stat points and grants the class skill. Both
progression curves cap at level 199. Stat 35 synchronizes the true level; the
native displayed EXP is capped separately without reducing durable EXP.

GM `/reborn <Killer|Warrior|Knight|Wit|Priest|Seer|1-6> [force]` uses the same
transaction; `force` skips only the level requirement. Rebirth no longer grants
a cape or needs free bag space. Class skills are Murder 15241, Holy Shield 15240,
Knight's Spirit 15242, Curse 15243, Rest 15244 and Foretell 15245.

AC79:2 changes an existing reborn class using a Transfer Mirror (34018) and the
selected class's configured cape. SQL validates both owned unlocked slots,
consumes one mirror, retains the cape and learns the new skill atomically.
`RebornClasses` in assets.db controls enabled classes and cape mappings. Existing
cape mappings and administrator edits remain unchanged by the asset upgrade.

Compiled class bonuses in `internal/game/reborn.go` multiply rounded innate
stats by 1.1 before equipment: Killer ATK/SPD, Warrior ATK/DEF, Knight DEF/SPD,
Wit MAT, Priest MDF, Seer SPD. Existing elemental growth remains compiled.
Reborn EXP uses the source powers 3.3 and 4.9, with displayed levels normalized
to 1–199 to avoid the source's subtract-100 byte wrap. Native HUD contributions
and class display still need aLogin verification.

## Tent metadata and remaining limits

Both floor/wallpaper pairs persist in wonderland.db. Tent snapshots replay each
present peer's nonzero pose through AC32:2. Upgrade purchases and verified sockets remain pending. Class skills are learned
from the selected rebirth class. Existing upgraded tents can use their second
floor; this does not implement native AC61 upgrade operations. Funded Potential
Pill training is implemented; native mercenary eligibility still needs verification.
Optional legacy SQLite import still rejects unsupported metadata explicitly;
these additions do not silently reinterpret old string-packed fields.

Upgrade using [the verified database upgrade procedure](ASSET_DATABASE.md). Gameplay
schema v18 and asset schema v15 are required by the current server; this subsystem
was introduced in gameplay v14 and asset v9. Installation databases are not reset.

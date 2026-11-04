# Shared combat stat formulas

Players and owned pets use `game.InnateCombat`, with coefficients owned
by `internal/game/growth_parameters.go`. Their effective permanent attributes,
level and element feed the same calculation. Rounded innate stats precede
equipment and player reborn class bonuses. Enemies share the ATK/DEF/MAT/MDF/SPD
formulas; their HP/SP come from stored NPC definitions. Player appearance and
potential bonuses still contribute to player attributes; pet potential contributes to pet
attributes. Automatic pet attribute growth remains selectable independently.

| Stat | Current common formula before equipment/class bonuses |
| --- | --- |
| ATK | `1.4*LV + 2*STR`; Fire uses `2*LV + 2*STR` |
| DEF | `2*LV + 1.75*CON`; Earth uses `2.6*LV + 1.75*CON` |
| MAT | `1.4*LV + 2*INT`; Fire uses `1.6*LV + 2*INT` |
| MDF | `2*LV + 2.2*WIS`; Earth uses `2.2*LV + 2.2*WIS` |
| SPD | `1.6*LV + 1.8*AGI`; Wind uses `2.1*LV + 1.8*AGI` |
| HP | `180 + LV + 2*CON + 2*CON*LV^0.35` |
| SP | `94 + LV + 2*WIS + 3.2*WIS*LV^0.3` |

Rounding follows the existing player implementation, including ties to even.
Unknown/neutral elements use the documented baseline in `elemental_growth.go`
(DEF `2*LV+2*CON`, MDF `2*LV+2*WIS`, SPD `1.6*LV+2.2*AGI`). A missing Robinson
definition retains the historical Earth alias fallback. Production lookups use
the shared NPC definition, including pet broadcast aliases.

NPC element and STR/CON/INT/WIS/AGI belong to the assets database. They are
resolved for battle creation and pet stat calculation rather than copied into
durable pet ownership state. The client resolves the same NPC element for local
inventory calculations, and still accepts authoritative native pet stat totals.
No wire-layout or database schema change is required. Existing pets receive the
new derived values when recalculated; their permanent allocated attributes are
preserved.

Ordinary wild, proximity, scripted-formation and GM-spawned enemies use stored
NPC HP/SP, independently of encounter level variations. Explicit event HP and
trial HP/attack overrides remain authoritative. Zero SP is valid; absent or zero
HP has a minimum of one, with no player-vitals formula fallback. Missing NPC
attributes use zero attributes rather than inventing level-scaled
STR/CON/INT/WIS/AGI. Reinforcements use the same construction as the first wave.

For an owned Floating pet at level 4, STR/CON/INT/WIS/AGI `2/2/1/1/2`, Wind gives
HP/SP `194/105` and ATK/DEF/MAT/MDF/SPD `10/12/8/10/12`. Its previous pet DEF was
36 because the inherited unknown-element rule used eight defense per level.

## Private Server comparison

Reference: sibling `Wonderland-Private-Server`, revision
`0cb6915d38982391fcf62fb0509da0b0966269ce`, inspected 2026-11-03.

`wlo.pserver.core/Game/PlayerRelated/Equip.cs` calculates player ATK/MAT with
the same Fire level coefficients, but DEF is `2*LV+2*CON` (Earth `3*LV+2*CON`),
MDF is `2*LV+2*WIS`, and SPD is `1.6*LV+2.2*AGI` (Wind `2.1*LV+2.2*AGI`). HP/SP
use the formulas above. Its job-specific 10% bonuses precede equipment.

`Game/Player.cs`, `PlayerPetData.Calculated*`, has separate pet formulas:
element is Earth only for Robinson, otherwise zero. DEF uses `8*LV+1.75*CON`
for zero element and `2*LV+1.75*CON` otherwise. Other pet formulas include
ATK `1.4*LV+2*STR`, MAT `1.4*LV+2*INT`, MDF `2*LV+2.2*WIS`,
SPD `1.6*LV+2.2*AGI`, and the common HP/SP expressions. The code's alternate
element branches are effectively unreachable for ordinary non-Robinson pets.

`Game/Battle/PvEBattleManager.cs` uses separate enemy formulas:
ATK/MAT `round(1.5*LV+5)`, DEF/MDF `round(1.2*LV+3)`,
SPD `round(1.3*LV+4)`, SP `20*LV+50`, and encounter/NPC HP or explicit overrides.
Gonline formerly inherited these pet/enemy paths. The unification deliberately
uses Gonline's current player coefficients, preserving player balance.

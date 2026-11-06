# Combat damage

`internal/battle/damage.go` follows the community formula reference supplied in
`var/damage/formulas`. These are estimated mechanics, not a claim of exact Legacy
server reproduction. Players, pets, monsters and confused monsters share the
same strike calculation.

Single-target physical base damage is `1.4*ATK + P - 0.98*DEF` when ATK is at
least DEF, and `0.599*ATK + P - 0.179*DEF` otherwise. Magical base damage is
`1.4*MATK + P - 0.98*MDEF`. Per the project requirement, physical DEF never
contributes to magical damage. The unconfirmed low-MATK branch is not extrapolated.

Skill power is `P = BasePower * (1 + 0.05 * (grade - 1))`, with grade 1 using the
full base power. The five-percent increase is provisional project tuning: the
Legacy grade curve has not been established. The coefficient increases additively,
not by compounding 1.05 each grade. Blast Attack and Flame Attack both have base
power 20, so P is 20/21/29 at grades 1/2/10 (the existing supported grade cap).
Do not add native `AdditionalDamage` again to an authored base power.

Assets schema v53 stores nullable `Skill.BasePower` as structured skill columns.
224 explicit powers are seeded from the saved Earth, Water, Fire, Wind and unique
skill wiki tables. This includes 153 elemental powers and 71 character/pet/quest
powers. `internal/assetsql/skill_power_defaults.csv` preserves source names, SP,
element and damage layer; the adjacent source manifest records page hashes.
Regenerate with `tools/data_export/extract_skill_powers.py --pages var/reference-web/skills --skill-export data/skill_data.json --npc-export data/npc_data.json --output internal/assetsql/skill_power_defaults.csv`. Native ID bindings
use element/layer/SP and authored family choices where costs repeat. Shared icon
numbers are not skill IDs. Unique-skill bindings are authored in
`tools/data_export/reference/skill-powers/unique_bindings.csv`, checked against
native NPC ownership and SP/type, and recorded in the generated adjacent
`skill_power_defaults.unique-review.csv`. This distinguishes reused squirrel
icons, two Horn Rising skills, player-stunt aliases, and Frederico's SP66 Blowout
from his separate SP115 Rage Thunder. Chinese actor names disambiguate localized
names such as Sam/薩爾斯/サールス and Ginseng/人參精.

All explicit unique-page power rows have ID matches. Six matched skills remain
review-only: Sky Thunder (15135) states power180 only at grades10–12; Circle Run
(25013), Urus Run (17003), Orangutan Rage (30051), and both Viper variants
(11064/15164) give approximate/question-mark powers. These are not seeded as
fixed base powers. Existing explicit database powers, including zero, are preserved.

Unresolved powers retain a named compatibility fallback: the native grade-one
estimate `PowerPerLevel + AdditionalDamage`, with the same provisional grade
increase. This estimate is not a verified base power. Native `PowerPerLevel`,
`AdditionalDamage` and `StatMultiplier` remain imported provenance.
`StatMultiplier` no longer replaces the
reference's attack coefficient. Verified targeting capacity selects coefficients
1.4 for one target, 0.85 for two, 0.6 for three/four and 0.4 for six or more.
Five-target patterns retain 0.6; all-side patterns use their eight-cell capacity.
Capacity, rather than the number of surviving targets, prevents damage changing
merely because a formation has empty cells.

The supplied elemental matrix includes Fire's inherent offense. Skill element
takes precedence when present. Combo bonuses add to that factor: the default is
+0.3, while the existing `combo_damage_per_participant` setting retains its
+0.1 per actual successful participant. Defending subtracts 0.7 and an active
Freeze adds 0.1 once. The factor cannot become negative.

Existing structured buff/debuff definitions and equipment critical settings remain
authoritative. Damage is floored at each integer modifier stage, capped to the
protocol range, and the existing bounded random roll is added last. A small
floating-point tolerance prevents decimal coefficients from losing one point
at mathematically integral boundaries.

The reference explicitly leaves randomness, rounding and several special-skill
rules unresolved. Rain Arrow specialization,
exceptional six-target coefficient and Priest correction are not assigned to
unverified skill/job IDs. They require verified definitions rather than matching
localized display names or inventing content identifiers.

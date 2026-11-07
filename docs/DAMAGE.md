# Combat damage

The [fixed-P polynomial investigation](SKILL_POWER_ANALYSIS.md#general-damage-polynomials-with-fixed-p)
provides the bounded quadratic magical stat adjustment selected below. The P
calculation remains unchanged.

`internal/battle/damage.go` follows the community formula reference supplied in
`var/damage/formulas`. These are estimated mechanics, not a claim of exact Legacy
server reproduction. Players, pets, monsters and confused monsters share the
same strike calculation.

Single-target physical base damage is `1.4*ATK + P - 0.98*DEF` when ATK is at
least DEF, and `0.599*ATK + P - 0.179*DEF` otherwise. Single-target magical base damage is
`max(0, 1.4*MATK - 0.98*MDEF + Q) + P`. Per the project requirement, physical DEF never
contributes to magical damage. The unconfirmed low-MATK branch is not extrapolated.

For single-target magic, including staff basic attacks, the selected stat
correction is:

```
x = clamp(MATK, 24, 1052)
y = clamp(MDEF, 35, 78)
Qfit = 0.244297946284 + 0.00607069954603*x + 0.00653637105727*y
  - 0.00000548960961612*x*x + 0.0000874824337356*x*y
  + 0.000170650039492*y*y

t(v, upper) = clamp((v-upper)/upper, 0, 1)
w(v, upper) = (1-t)^2 * (1+2*t)
Q = Qfit * w(MATK, 1052) * w(MDEF, 78)
```

This is the frozen quadratic fit from the measurement investigation, selected
after the user's MAT238 validation. No skill or target IDs select the correction.
It uses effective post-buff stats for players, pets and enemies alike. P retains
coefficient one, and elements/defend/combo/shield/critical/noise stages retain their
existing order. Area attacks keep their existing coefficients without Q because
the sample set does not validate an area stat adjustment.

Single-target magic floors the combined stat contribution at zero before adding
P. This empirical rule addresses the low-MAT grade-1 Blast discrepancy without
altering native coefficients or adding skill/target exceptions. Previously a
negative stat term consumed most of P: at MAT25 against Straw Chicken, damage
was 6–10 versus two recorded 22s. The new prediction is 18–22. Against
Hyperkinetic Chicken it changes 8–12 to 14–18 versus recorded 16/19. The second
case still has a one-point range miss, so this is an approximation, not an exact
Legacy rule. Spell grades remain monotonic, and the floor does not apply to the
less-established area formula. Enemy definitions and the MDEF-only policy are
unchanged. Estimated target defenses remain a separate source of uncertainty.

Clamping affects only Qfit's inputs. The main `1.4*MATK - 0.98*MDEF` term keeps using
the full effective stats. The weight is one throughout the measured range and
smoothly becomes zero by MATK 2104 or MDEF 156, with zero endpoint slopes. This
extrapolation policy was added after checking independent high-stat examples in
the supplied damage references; their results favor the linear baseline. It is
not a claimed Legacy formula or a refit of P. The fade alone preserved all 91
recorded-cast predictions; the subsequent stat floor changes the weak-caster cases.
The fade avoids carrying a constant correction into unsupported high stats.
Tests check monotonicity across the fit/fade boundaries and extreme values.
Physical damage retains the previous formula, which remains the stronger fit.

Across 91 usable normal casts (24 physical, 67 magical), the stat floor lowers
mean percentage error from 6.135% to 4.477%; magical error falls from 7.992% to
5.741%, and physical error remains 0.948%. Overall median error improves from
1.609% to 1.235%. The four MAT25 grade-1 Blast casts improve from 53.035% to
8.493% mean error. The tradeoff is that the 35 controlled MAT25/29 grade-2/3 casts
change from 8.248% to 9.221%; their predicted-range coverage falls from 27/35 to
24/35. The ten prospective MAT238 casts retain 0.985% error. These figures weight
individual casts; source examples and uncertain/critical casts remain separate.
No new coefficients were fitted. This rule was selected on the existing samples,
so it is not a new blind validation. Target uncertainty and held-out limitations
remain; replay equality proves implementation consistency, not Legacy accuracy.

All 455 random-roll `Battle.Round` replays match the independent selected-model
calculation. Reproduce the comparison with
`python tools/skill-power-analysis/selected_damage_compare.py`; detailed inputs
and results are under ignored `var/reports/damage-selected/`. No asset/database
rebuild is required; rebuild and restart the server to apply the compiled rule.

Skill power uses the empirical whole-attack approximation selected on 2026-11-06:

```
H = AdditionalDamage                 (Skill.dat offset 49)
D = PowerPerLevel                   (offset 32, compatibility name)
G = learned grade, capped to the skill's maximum
physical P = 1.9131446584781497*H + 0.47524025502148415*D*G
magical P  = 2*(H + D*G)
```

The physical coefficients fit the measured physical attacks; they are estimates,
not decoded Legacy constants. The simpler rounded `2*H + 0.5*D*G` is close but
has slightly larger measured errors. Magic uses the slower grade growth supported
by the controlled MATK25/29 experiments. Neither native `StatMultiplier` nor
`ContactEvents` multiplies the attacking-stat term or total P. Offset 40's exact
meaning remains unresolved; its compatibility field name is retained. This avoids
double-counting multi-hit power. The final damage is the total for one affected
target; the client distributes it using each contact's animation weight, including
unequal weights such as Star's weak first hit and strong second hit.

Grades are clamped to each skill's `MaximumGrade` from Skill.dat (offset 105),
with ten as the fallback only for missing metadata. An explicit zero maximum stays
zero. Basic attacks have P zero. Wiki `BasePower` remains authored reference data
and does not override the general formula.

The server stores `Skill.ContactEvents` in asset schema v55. Schema v56 seeds
counts from tracked `internal/assetsql/skill_contact_defaults.csv`, extracted
offline from native MBTM damage modes/contact counts. Initialization and migration
do not require a client installation or a retained client archive.
Single-damage mode and missing/empty animation metadata use one contact, matching
client playback. Existing nonzero administrator counts are preserved. Runtime
combat reads only the SQL-derived catalog; it does not open animation files.
Contacts remain available as metadata; they do not multiply damage. The client
splits the complete server damage total across percentage-weighted contact cues.

The provisional five-percent grade curve and authored BasePower override no
longer drive damage. Reference powers remain stored without destructive edits.

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

Native `PowerPerLevel` and `AdditionalDamage` supply the power formula above.
Targeting capacity selects attacking-stat
coefficients 1.4 for one target, 0.85 for two, 0.6 for three/four and 0.4 for six
or more. Five-target patterns retain 0.6; all-side patterns use eight-cell capacity.
Capacity, rather than the number of surviving targets, prevents damage changing
merely because a formation has empty cells. Explicit SQL grade shapes take
precedence, followed by the same native pattern interpretation used by target
selection. An explicit single-target override or unresolved native pattern uses
one. Area power is additionally multiplied by `2/(capacity+1)`, which becomes
one for single-target attacks. This general approximation improves the 20 saved
area-power references (mean error 83.3% without reduction versus 17.2% with it).
Area shapes and this penalty are less certain than single-target behavior; no
live area-damage samples establish an exact rule, and exceptional skills remain.

Validation compared predicted random-roll midpoints with 81 usable, noncritical
Legacy observations: 24 physical and 57 magical. It excludes the uncertain
Sheet3 target and the flagged critical hit. Mean absolute percentage error falls
from 29.67% with the previous contact/stat-multiplier model to 7.83%, and median
error falls from 25.93% to 1.69%. Physical mean error is 0.95% (fitted samples;
leave-one-skill-out research error 1.09%), the high-MATK bug tests 0.61%, and the
35 controlled low-MATK casts 9.96%. The older low-MATK magic samples remain less
accurate. These are comparisons against sampled damage, not proof of the original
server equation. Enemy defenses are repository estimates. All 405 random-roll
replays through `Battle.Round` matched the independent comparison calculation.
See [the investigation](SKILL_POWER_ANALYSIS.md) for alternatives and limitations.

The supplied elemental matrix includes Fire's inherent offense. Every known
non-basic skill uses its native element: zero means neutral, 1–4 are fixed
Earth/Water/Fire/Wind, and 7 explicitly inherits the caster's element. Zero must
not fall back to the caster: Bloodsuck and other neutral skills do not gain
Fire's bonus or weakness. Neutral damage still receives Earth's inherent 10%
reduction. Basic attacks and missing skill definitions also use the fighter's
element. Combo bonuses add to that factor: the default is
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

## Additional saved-reference audit (2026-11-06)

The twelve pages in `var/damage/references` include mirrors and archived copies;
they are not twelve independent experiments. The English-named "Damage
Calculator" file and Japanese VIP damage page are byte-identical. The audit
records each page's SHA256 and original URL, where embedded, in
`var/reports/damage-references/comparison.json`. Reproduce it with:

```
python tools/skill-power-analysis/reference_compare.py
```

That research command requires the preserved, ignored
`var/reports/damage-selected/runtime-input.json` and saved reference pages. They
are not server build/runtime dependencies. It neither refits P nor modifies raw
observations, reference powers or the databases.

Findings from the saved Japanese VIP damage page, unique/elemental skill pages,
and Bahamut single-target, combo and area articles:

- Skill element determines affinity, including neutral. The source explicitly
  uses neutral Bloodsuck as its example. This exposed the corrected zero-element
  fallback in `damageElement`. The 91 measured casts use non-neutral skills, so
  their predictions are unaffected. Synthetic packet-golden fixtures now state
  their intended Fire element; authored packet bytes remain unchanged.
- The unique-skill page distinguishes neutral from "user-element" skills.
  Existing authored bindings match Chaw Throw (15212), Body Bump (18007), Turtle
  Palm (25055) and Scatter powder (30065) to that description; all four native
  records have element 7. The repository seed has 206 element-7 definitions.
  They now resolve caster affinity instead of falling through to multiplier 1.
  Neutral is independently evidenced by Roca's Tulwar experiment: approximately
  equal damage against Water/Fire/Wind/None, but 10% less against Earth. The
  neutral-to-Earth matrix entry is corrected from 1 to 0.9.
- The newer single-target article gives measured evidence for `max(DEF,MDEF)`
  against magic. Gonline deliberately keeps the requested MDEF-only policy.
  Article rows 12/13 therefore remain a documented intentional mismatch.
- The combo investigation supports adding 0.30 to each participant's elemental
  factor, including mixed elements and a three-person combo. It contradicts the
  older blanket 1.25 multiplier. Gonline already uses the additive rule and keeps
  its administrator-configured alternative.
- Physical `0.599*ATK - 0.179*DEF` below the defense threshold is supported.
  Extending it to low-MATK magic disagrees with the controlled Blast tests.
- Area references disagree: two-target coefficients 0.84/0.85 and three-target
  0.4/0.6 both appear. They explicitly mention skill exceptions (Sky Thunder,
  Rain Arrow and Wing Flap). They do not justify replacing the general capacity
  policy with new unverified skill overrides.
- The old VIP buff examples show whole damage intervals tripling with Hot Fire.
  That suggests noise may precede some buff multipliers, unlike the current
  late-noise pipeline. Later sources leave fluctuation placement unclear. No
  buffed user samples establish rounding/order, so this remains an open issue.
- The single-target article supports Priest effective DEF including half of
  total MDEF, contradicting the older 30%/10%-reduction claims. No unverified job
  behavior is added by this change.

With P held fixed, candidate mean absolute percentage errors against individual
recorded normal casts are:

| Stat/defense rule | All 91 | Magical 67 | Controlled Blast 35 | MAT238 10 |
| --- | ---: | ---: | ---: | ---: |
| Current: bounded correction plus stat floor | 4.477% | 5.741% | 9.221% | 0.985% |
| Before stat floor: bounded correction | 6.135% | 7.992% | 8.248% | 0.985% |
| Linear 1.4 / 0.98, no correction | 7.098% | 9.301% | 9.958% | 1.206% |
| Extend low-offense branch to magic, no correction | 14.122% | 18.841% | 30.067% | 1.206% |
| Defense coefficient 1, no correction | 9.015% | 11.831% | 13.810% | 1.634% |
| Older average DEF/MDEF, coefficient 1, no correction | 4.426% | 5.598% | 6.642% | 0.770% |

The older average rule scores best here, but violates the requested defense
policy and the newer controlled defense evidence. Repository-derived enemy stats
are still estimates, so these scores cannot identify Legacy's defense policy.
The native P formula, physical coefficients, area coefficients and MDEF-only
policy remain unchanged.

The newer article also supplies three independent high-stat magic cases where
MDEF exceeds DEF, making its defense selection compatible with Gonline's policy.
Holding the article's stated P fixed isolates the stat correction:

| Article row | Recorded mean | Previous clamped Q | Refined fading Q |
| --- | ---: | ---: | ---: |
| 14: MAT2346, MDEF360, P440, Water to Wind | 3036.50 | 3045.79 | 3037.44 |
| 15: MAT2346, MDEF561, P440, Water to Wind | 2860.25 | 2868.51 | 2860.16 |
| 17: MAT1429, MDEF561, P50, Earth to Wind | 903.00 | 909.06 | 903.49 |

These continuous values include mean noise 3; production floors before adding
the roll. All three recorded means fall inside the refined runtime's 1–5 roll
intervals. These are checks of the stat term using the article's P, not new live
tests or validation of our skill-power estimate at those grades. Source examples
remain separate from the 91 user measurements and were not used to fit Q.

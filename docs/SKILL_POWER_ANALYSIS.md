# Skill power formula comparison

The refreshed comparison after the reference refinements is available locally at
`var/reports/four-version-damage-updated/report.html` (machine-readable
`analysis.json` alongside it). Regenerate with
`python tools/skill-power-analysis/four_version_compare.py --output var/reports/four-version-damage-updated`.
The current column is recalculated from the updated formula rather than copied
from cached predictions. It includes all 91 user observations, dedicated Blast
Attack summaries (50 casts), explicit version labels, source hashes, the full
formula and separate high-stat reference checks. The original report is preserved.
The element/fade refinements alone left these predictions unchanged. The later
low-MAT stat floor improves grade-1 Blast against Straw Chicken from 6–10 to
18–22 (recorded 22), retaining P. The report includes a before-stat-floor column
and documents the small grade-2/3 tradeoff; see [DAMAGE.md](DAMAGE.md).
`runtime-input.json` supports independent Battle.Round replay verification.

The subsequent [saved-reference audit](DAMAGE.md#additional-saved-reference-audit-2026-11-06)
compares the supplied Japanese and Bahamut documents with all 91 normal casts.
It retains the selected P, physical stat formula and MDEF-only policy, fixes
neutral/caster-dependent skill affinity, and fades the empirical magic correction beyond its upper
measurement bounds. Those changes alone preserved measured-cast predictions; three
independent high-stat source examples favor returning to the linear baseline.
`reference_compare.py` records conflicting source claims and rejected alternatives
without mixing published examples into the player's measurements.

The four-version comparison is reproducible with
`python tools/skill-power-analysis/four_version_compare.py`, producing
`var/reports/four-version-damage/report.html` and `analysis.json`. Its labels
distinguish remote origin/main `e2ebc87` (reference P with 5% grade growth and
grade10 cap), previous current (linear P before the stat correction), deployed
current (bounded quadratic magic), and the local Private Server `e513a3b`.
Remote main was verified with `git ls-remote` on 2026-11-06. All 953 skill elements
were decoded directly from the hash-matching native Skill.dat byte26 and agree
with both the export and seed catalog. Gonline already uses these values. A
regression now explicitly checks that Fire Star stays Fire for an Earth caster.
The user confirmed DangoChaos/DangoKnght are Fire and DangoBunny is Water.

Private Server source uses the caster's element and its own elemental matrix;
the report includes that as-is behavior and a separately labeled hypothetical
skill-element-corrected variant, without modifying the sibling repository. Its
main columns use the standard roaming monster defense `round(1.2*level+3)` for
both DEF and MDEF; an additional same-defense column isolates the equation from
that stat difference. Measured actor stats and matched Skill.dat coefficients
are held fixed. Normal skill damage is deterministic in Private Server's source:
the supplied basicRoll is ignored for skill attacks. Critical/uncertain casts
remain excluded from the 91-observation summary. Complete per-condition recorded
values and prediction ranges are retained in the report.

Latest selection, 2026-11-06: the user's instruction to use the best
approximations retains P and physical damage, and applies the frozen quadratic
stat correction to single-target magic. Its inputs saturate at the measured
MAT/MDEF bounds; the main baseline remains linear, and area coefficients remain
unchanged. [DAMAGE.md](DAMAGE.md) specifies the deployed equation and 91-cast
comparison. Historical `current` results in the fixed-P and MAT238 reports refer
to the pre-correction baseline, and remain preserved. The selected model was not
refitted to the MAT238 measurements.

Production update, 2026-11-06: [DAMAGE.md](DAMAGE.md) describes the implemented
linear whole-attack approximation, selected using the subsequent live damage
samples. Physical P is `1.9131446584781497*H + 0.47524025502148415*D*G`;
magical P is `2*(H+D*G)`. Area P uses `2/(capacity+1)`. Contacts and the unresolved
offset-40 coefficient do not multiply damage. Magic keeps the project's MDEF-only
policy. This improves the 81 usable live observations to 7.83% mean and 1.69%
median absolute percentage error, versus 29.67% and 25.93% for the prior model.
The 405 actual-engine noise-roll replays all match. Area and low-MATK exceptions
remain; this is an approximation, not an exact reverse engineering result.

The sections and generated comparison tools below record earlier research.
Their label `current server` denotes the **pre-update** contact/stat-multiplier
baseline. Historical results and source tables are retained unchanged; a claim
below that an exploratory fit does not change production refers to that research
stage. The complex structured fit was not selected for production because its
live-damage errors were larger despite its closer fit to wiki P values.

The [area follow-up](#area-capacity-follow-up) separates area penalties from the
[structured follow-up](#structured-follow-up-and-unequal-hits), which investigates
unequal contact weights and stronger held-out checks. The structured fit
supersedes the raw-offset search as the most accurate general exploratory fit here.
It does not change the production damage formula or establish native field meanings.

Offline analysis on 2026-11-06 compared 224 independently authored reference P
values with the updated client's native Skill.dat coefficients: 123 physical
and 101 magical skills. These include 153 elemental and 71 unique skills. The
reference table remains unchanged. This investigation does not change combat.

## Polynomial follow-up

The offline `tools/skill-power-analysis/polynomial_search.py` investigation on
2026-11-06 compares linear, quadratic and cubic approximations. It does not change
the runtime formula. Run from the repository root:

```
python tools/skill-power-analysis/polynomial_search.py
python -m unittest discover -s tools/skill-power-analysis -p test_polynomial_search.py
```

The default output is `var/reports/skill-power-polynomial/analysis.json` and
`report.html`. JSON records full coefficients, individual holdout partitions,
source hashes, errors, transfer results and grade diagnostics. Regeneration needs
the existing optional native exports, saved reference pages and prior sample
comparison reports; these remain research inputs, never server dependencies.

Here H is offset49, D is offset32, G is grade, Z is D*G, and S is the unresolved
offset40 float. Polynomial bases are H/Z, H/D/G and H/Z/S, tested through degree
three. Degree is relative to the chosen inputs: Z squared means D²G². There is no
intercept, and every term contains H, D or Z so zero native power remains zero.
S is an empirical feature, not evidence that its old stat-multiplier label is
correct. No IDs, names, animation counts or target identities are predictors.

Relative least squares with ridge penalties controls large coefficients. Training
scales, basis selection and ridge selection are recomputed within each fold.
Five outer folds hold out entire skill IDs, including every observed grade of
each skill. Three inner folds select the basis and ridge for each degree. A
separate test withholds entire reference source pages, since related skills may
otherwise share training and test folds. This remains exploratory model selection
on previously studied data, not a final independent accuracy test.

Training uses 202 references with explicitly known single-target capacity: 116
physical and 86 magical skills. Twenty area references are excluded from fitting
and tested separately with the existing `2/(capacity+1)` penalty. Two references
with unresolved capacity are excluded. The 81 usable live casts are separately
collapsed into 39 skill/grade/actor/target groups: 16 physical groups across five
skills and 23 magical groups across eight skills. Repeated casts are averaged;
each live skill has equal total fitting weight. Live P is inferred conditionally
using estimated enemy defenses, the current stat coefficients, the elemental
matrix and random-noise midpoint three. It is not directly observed P. In
particular, magical damage continues to use MDEF only. Comparisons below use group
means, so they differ from earlier cast-weighted averages.

Mean absolute percentage error on **held-out reference P**:

| Layer | Refitted linear | Quadratic | Cubic |
| --- | ---: | ---: | ---: |
| Physical, whole-skill folds | 7.085% | 6.781% | 6.546% |
| Magical, whole-skill folds | 10.425% | 6.230% | 5.930% |
| Physical, whole-page holdouts | 7.997% | 10.417% | 8.244% |
| Magical, whole-page holdouts | 11.499% | 7.484% | 8.223% |

These linear columns are independently refitted comparators, not the deployed
formula. The quadratic magical fit is the most useful compact reference result:

```
P = (0.198922905 + 0.901713233*S)*H
  + (0.491721512 + 1.355805575*S)*Z
  + 0.003580781*H² - 0.005520144*H*Z + 0.006044674*Z²
```

The physical quadratic fit is:

```
P = 1.354823497*H + 0.744811005*Z
  - 0.001410744*H² + 0.005186410*H*Z - 0.002121542*Z²
```

These equations are fit on all corresponding single-target references after
selection. Their training errors are 5.06% magical and 5.23% physical; the holdout
table above uses fresh fits in each partition. The cubic equations have 16 terms
and their full coefficients are retained in the generated report.

**Reference fits do not transfer consistently to live damage.** Current-runtime
damage error is 0.979% on the 16 physical groups and 9.061% on the 23 magical
groups. Reference-trained quadratic damage error rises to 4.510% and 17.676%;
reference-trained cubic error is 2.282% and 16.865%. These are response-source
transfer comparisons, not unseen-skill tests: some measured skill IDs also occur
in the authored reference table.

Fitting directly to inferred live P gives a different result. With whole skills
held out, physical damage error is 1.193% for linear, 4.272% quadratic and 5.894%
cubic. Magical damage error is 10.627% linear, 21.669% quadratic and 7.461% cubic.
The live-fitted magical cubic also describes the 86 single-target reference powers
with 8.097% mean error, although this inverse transfer overlaps skill IDs. This is
a promising candidate for further samples, not sufficient evidence to replace
the general model: it has 16 coefficients but only eight live magical skills.

Raw predictions are scored without clamping away negative outputs. The live-fitted
physical cubic produces negative power somewhere in the grade range for 24 of
the 116 physical reference skills, and decreasing grade curves for 55. Even the
reference physical quadratic decreases on two skills. The reference magical
quadratic/cubic and live magical cubic do not fail those checks on the 86 tested
skills, but this does not establish safe behavior on all unmeasured skills.
Grade diagnostics vary G from zero through each native maximum; only the
maximum-grade reference is ground truth.

On the 20 held area references, reference-trained cubic physical error is 12.45%
(six skills) and quadratic magical error is 16.37% (14 skills). These retain the
existing capacity penalty and do not isolate the correct native area formula.

Conclusion: polynomials improve reference-P approximation, especially for magic.
Physical live data still favors the compact linear formula. The live-fitted
magical cubic merits more testing with new skills, varied grades and known target
defenses before any runtime change. Authored reference tables and observations
are unchanged. All 28 related research tests pass, including polynomial recovery
on unseen synthetic inputs, whole-skill fold separation and invalid-grade checks.

### Extended polynomial search

The follow-up `tools/skill-power-analysis/polynomial_extended.py` expands this
to **24 polynomial families**, each tested with four ridge penalties. It retains
the earlier linear/quadratic/cubic controls and adds fourth-, fifth- and
sixth-degree models, SP cost (native offset24), and the raw decoded offset30
coefficient T. Offset48 is constant zero in the 224 reference records, so it
provides no useful predictor. Offset31 is not treated as a target count.

```
python tools/skill-power-analysis/polynomial_extended.py
```

The expanded, readable report is
`var/reports/skill-power-polynomial-extended/report.html`; the adjacent
`analysis.json` preserves coefficients, equations, source hashes, grade failures,
per-family results and nested-selection results. Every fitted family has its
complete equation in an expandable report entry. Native source data, reference
tables and runtime damage remain unchanged.

Bases include H/Z through degree6; H/D/G and H/Z/S through degree5; and
H/Z/T, H/Z/SP, H/Z/S/T, H/Z/S/SP and H/D/G/S through degree3. H/D/Z must occur in
every monomial. No skill ID or name is a predictor. The use of SP and T is only a
statistical hypothesis; correlation does not establish their native semantics.
The same nested skill folds, conditional live P inference, MDEF-only policy and
separate source-page checks apply. Ridge paths share training-only normal
equations, without accumulating regularization between candidates.

Like-for-like physical H/Z models, trained on the 116 single-target references:

| Degree | Terms | Training mean P error | Held-skill mean P error |
| --- | ---: | ---: | ---: |
| Cubic | 9 | 4.768% | 5.306% |
| Quartic | 14 | 4.468% | 5.247% |
| Quintic | 20 | 4.262% | 5.111% |
| Sextic | 27 | 4.154% | 5.151% |

The quintic is the best individual physical reference candidate in this search:

```
P = 1.32801314627H + 0.789013215534Z
  + 0.00223329853812H² + 0.0066213459007HZ - 0.00380968735865Z²
  - 8.57021403296e-5H³ + 2.49415226154e-5H²Z
  + 9.20172026962e-6HZ² - 2.42814133828e-6Z³
  - 2.61684101211e-7H⁴ - 7.65844264505e-8H³Z
  + 4.72642880654e-8H²Z² + 1.37666267181e-8HZ³ + 7.05666112983e-9Z⁴
  + 5.54613508075e-9H⁵ - 1.70025513776e-9H⁴Z
  - 2.45715926717e-10H³Z² - 8.6361597243e-11H²Z³
  - 1.34819920761e-11HZ⁴ + 1.58441744756e-11Z⁵
```

The extra complexity brings diminishing returns: the quintic has a decreasing
grade curve on one of the 116 skills, the sextic on two; the H/Z quartic has none
on these checks. The quintic's live physical damage error is still 4.214%, versus
0.979% for the current formula on the same 16 groups.

For magical references, H/Z/S cubic remains the best individual family: 5.930%
held-skill mean P error. Quartic increases it to 6.687%, and quintic to 8.493%.
Adding SP or T does not improve that result. A different reference-trained
H/D/G/S cubic reaches 6.780% reference P error but transfers better to live damage
(7.441% group-mean damage error, versus the current 9.061%). This transfer is
not an unseen-skill test because the two sources overlap skill IDs.

For live-fitted magic, the individual H/Z/S/T cubic gives 6.918% held-skill
damage error and 7.499% inferred-P error; H/D/G cubic gives 7.133% held damage
and 6.947% inferred-P error. The latter retains the previously reported full-data
coefficients; the changed number here comes from selecting its ridge alone
instead of also selecting a basis within the cubic category. These remain
exploratory comparisons over only eight measured magical skills.

**Selecting among many families is itself unstable.** Selecting the whole
structure and ridge exclusively inside each outer training partition produces
8.256% physical and 6.195% magical reference P error. Whole-source-page selection
gives 8.709% and 8.340%. Applying the complete search to live data gives 2.157%
physical and 21.335% magical held damage error, despite some individually
promising candidates. Report rankings must not be mistaken for an unbiased
estimate of the best candidate selected after seeing those rankings.

The expansion supplies more concrete equations and useful candidates, but does
not justify a general runtime replacement. More independently measured skills
and grades are still needed. All 33 related research tests pass, including
quartic recovery on unseen inputs, raw-offset decoding, degree-six feature
generation and ridge-order independence.

## General damage polynomials with fixed P

On 2026-11-06 the user chose to retain the deployed P calculation. The new
`tools/skill-power-analysis/damage_polynomial.py` investigation therefore fixes
P's inputs, coefficients and grade behavior, and also fixes its additive
coefficient in the damage equation to exactly one. It tests only the
stat-dependent remainder:

```
Damage = floor(E * (P + F(O,B))) + noise
O = ATK for physical attacks, MAT for magic
B = DEF for physical attacks, MDEF for magic
```

Integer flooring, minimum/maximum damage, the elemental matrix and the one-through-five
noise bounds match the current pipeline. Buffs, criticals, combos and area
coefficients are not fitted: the measurements do not establish those mechanics.
No gameplay files or reference tables change.

Run `python tools/skill-power-analysis/damage_polynomial.py`. The report and full
JSON are written to `var/reports/damage-polynomial/`. The HTML shows every
measurement condition, explicitly including the eight controlled Blast Attack
conditions (35 casts), and labels fitted ranges separately from validation.
Input and runtime-source hashes are retained.

The search includes 29 families: current damage; direct linear through quartic
stat polynomials; polynomial corrections around the current formula; corrections
active only below defense; and continuous hinge polynomials in
`L = min(0,O-B)`. Constants are tested both present and absent. Five ridge
penalties per fitted family shrink unstable coefficients. No P terms or P/stat
interactions are permitted. All 81 usable casts form 39 condition means: 16
physical and 23 magical. Equal total skill weight is recomputed inside each
training partition. The fit minimizes relative final-damage residuals, which
remain valid when the stat remainder is zero or negative.

Validation independently withholds an entire skill, target, or offense value.
Ridge selection and complete-family selection are repeated using training-only
inner partitions. This produces three separate generalization checks; they are
not interchangeable. Report rankings across families remain exploratory.

Physical comparisons (mean final-damage percentage error):

| Stat model | Fitted | Held skill | Held target | Held offense |
| --- | ---: | ---: | ---: | ---: |
| Current | 0.979% | 0.979% | 0.979% | 0.979% |
| Refit linear | 0.943% | 1.195% | 2.558% | 41.444% |
| Quadratic correction + constant | 0.864% | 1.009% | 1.089% | 0.899% |
| Cubic correction + constant | 0.815% | 1.101% | 1.089% | 0.899% |

Physical data still favors current damage. Its only offense values are 77 and 89,
its three target DEF values span only 32–36, and it has no below-DEF physical
observations. The high offense-holdout errors of direct refits show that these
samples do not identify a general stat curve. Shrinking corrections toward the
current rule avoids much of that failure, but gains are negligible and mixed.

Magical comparisons:

| Stat model | Fitted | Held skill | Held target | Held offense |
| --- | ---: | ---: | ---: | ---: |
| Current | 9.061% | 9.061% | 9.061% | 9.061% |
| Refit linear | 4.980% | 9.462% | 8.588% | 6.984% |
| Refit affine (constant + linear) | 5.011% | 5.853% | 12.351% | 6.554% |
| Direct quadratic | 5.683% | 10.616% | 15.195% | 13.615% |
| Quadratic correction + constant | 7.937% | 5.515% | 18.278% | 8.090% |
| Cubic correction + constant | 7.734% | 5.467% | 13.300% | 8.176% |
| Linear low-offense hinge | 5.961% | 7.196% | 10.202% | 6.567% |
| Quadratic low-offense hinge | 4.425% | 6.736% | 9.547% | 5.241% |

For example, the full-data magical quadratic correction is:

```
F(O,B) = 1.4O - 0.98B
       + 0.244297946284 + 0.00607069954603O + 0.00653637105727B
       - 0.00000548960961612O² + 0.0000874824337356OB
       + 0.000170650039492B²
```

The more compact refit is `F(O,B)=1.38090125499O-0.866565914483B`.
These do not change P. The latter modestly improves unseen-target error but
slightly worsens unseen-skill error. The quadratic correction substantially
improves unseen-skill results but fails on unseen targets. Do not mistake its
5.515% held-skill error for a reliable overall gain.

Physical plausibility is checked by stepping offense and defense separately,
both within observed bounds and across a wider 0–1200 offense / 0–800 defense
grid. The quadratic correction stays monotonic on that grid, but direct
quadratic, cubic correction and quadratic-hinge fits can predict damage
increasing with defense. Some below-defense-only fits introduce discontinuities.
Those are research failures, not candidates for unguarded runtime deployment.

When the entire selection procedure is tested rather than a family chosen after
viewing results, magical error is 9.595% on held skills, 15.401% on held targets
and 7.822% on held offense values. Physical selection gives 1.039%, 0.979% and
0.979%. The broader search does not establish a consistently superior general
formula.

The six magical targets and five MAT values (24,25,29,815,1052) leave almost the
entire intermediate MAT range unmeasured. Enemy MDEF values are estimates, and
with P fixed any residual P error can be absorbed incorrectly into fitted stat
coefficients. More independent targets and intermediate MAT samples are needed
to identify curvature. Keep current P and current runtime damage for now.
All 40 related research tests pass, including fixed additive P, independent
damage examples, bounds, unseen-input quadratic recovery, and whole-group
holdout separation.

### Prospective MAT238 measurements

The next user batch, preserved in
`var/damage/samples/dangochaos-mat238.json`, records DangoChaos at level149,
MAT238 and ATK1104. The actor element is unconfirmed; both tested skill
definitions explicitly specify Fire, so the actor-element fallback is unused.
It contains ten normal Blast Attack grade1 casts on six targets, two explicitly
critical Blast casts, and two Five Star Beating grade1 casts with uncertain
critical/target-count context. Observation order, flags and individual contact
values are retained. No observation is silently divided by a critical multiplier.

`python tools/skill-power-analysis/mid_matk_compare.py` evaluates the existing
fixed-P damage fits **without retraining or selecting models on this batch**.
The result is under `var/reports/damage-mat238/`, with hashes of the source and
frozen-model report. It does not append these measurements to earlier fitting
inputs or overwrite their fits. Mean errors below average six target conditions:

| Frozen stat model | MAT238 mean damage error |
| --- | ---: |
| Current | 1.198% |
| Quadratic correction + constant | 1.000% |
| Cubic correction + constant | 1.359% |
| Refit linear | 1.428% |
| Refit affine | 4.139% |

All MAT238 targets have MDEF below MAT, so the low-offense hinge corrections
are inactive and equal current damage. The small quadratic advantage is not
evidence of a generally better equation; these are one skill at one MAT setting,
and the 0.198-percentage-point difference is small relative to the sparse sample.

| Blast Attack grade1 target | Observed mean | Current range | Frozen quadratic-correction range |
| --- | ---: | ---: | ---: |
| Male Bug | 357 | 353–357 | 359–363 |
| Female Bug | 352.333 | 348–352 | 354–358 |
| Hyperkinetic Chicken | 404.5 | 394–398 | 398–402 |
| Rice Chicken | 373 | 362–366 | 365–369 |
| Irrigated Chicken | 217 | 215–219 | 216–220 |
| Damp Snail | 379.5 | 372–376 | 375–379 |

Fifty actual `Battle.Round` replays (ten normal observations times five random
rolls) match the current-formula predictions exactly. All 43 related research
tests pass. The new observations mostly exceed the current predicted midpoint,
especially the Hyperkinetic/Rice Chickens; this does not by itself establish
whether the remaining error belongs to P, monster MDEF or the general stat term.

Five Star Beating is physical and has native two-target capacity. Current area
prediction uses stat coefficient0.85 and P attenuation2/3, not the single-target
magic equation or a six-contact damage multiplier. The reported hit sequences sum
to1502 on Irrigated Chicken and1288 on Damp Snail. Normal current predictions
are752–756 and1293–1297. The first is close to a double-critical hypothesis
(1503–1507), but the user cannot confirm critical status or how many enemies were
hit; both casts remain descriptive and excluded from normal regression scoring.
The five roughly equal small contacts plus a final contact about five times
larger support unequal damage allocation and are preserved for later animation
analysis. They do not independently establish the total-power formula.

## Notation and results

- A: AdditionalDamage, decoded Skill.dat offset 49.
- B: PowerPerLevel, offset 32.
- G: the native maximum grade, offset 105, including an explicit zero.
- S: StatMultiplier, offset 40.
- C: effective animation contact count.
- W: summed contact percentages divided by 100; single-damage mode uses one.

The tool compares 44 fixed hypotheses, a 64-pair coefficient grid per damage
layer, and fitted relative-least-squares coefficients. Metrics use each skill's
absolute proportional error, so expensive skills do not dominate the ranking.

| Formula for P | Median error | Within 10% | Within 20% |
| --- | ---: | ---: | ---: |
| Current: `(A + BG) * C` | 50.61% | 37 / 224 | 54 / 224 |
| `A + BG` | 23.02% | 62 / 224 | 101 / 224 |
| Physical `A + BG`; magical `2(A + BG)` | 9.21% | 120 / 224 | 171 / 224 |
| Physical `2A + 0.5BG`; magical `2.5A + 1.5BG` | 8.23% | 144 / 224 | 183 / 224 |
| Physical `2A + 0.5BG`; magical `2(A + BG)` | **7.96%** | **145 / 224** | 181 / 224 |

The last row gives the smallest median error among these fixed hypotheses, but
the preceding row covers slightly more skills within 20%. The last row's mean
proportional error is 18.36%, versus 18.02% for the simpler layer multiplier and 152.15% for
the current formula. Thus the median does not imply every skill is close.

## Contact count is the clearest mismatch

Every matched skill's effective contact weights sum to 100%, including
multi-contact animations. Weight-based multiplication therefore leaves P
unchanged for this sample. Multiplying by the number of contacts counts the
same complete power repeatedly. Removing C reduces physical median error from
126.25% to 10.62% before changing any coefficients.

Flame Attack has two 50% contacts. Its native `A + BG` is 21, against reference
P 20; multiplying by two gives 42. Flame Hit becomes 150 instead of 50 when
multiplied by its three contacts, against reference 47. This is consistent with
the client splitting one damage total into contact portions. The comparison
alone does not prove the complete Legacy server formula.

## Physical and magical coefficients differ

Without contact multiplication, magical `A + BG` has median error 47.15%.
Doubling it reduces that to 9.00%. For physical skills, `2A + 0.5BG` has median
error 7.79%, compared with 10.62% for `A + BG`.

| Skill | Reference P | Current P | Physical `2A + 0.5BG`, magical `2(A + BG)` |
| --- | ---: | ---: | ---: |
| Wind Attack, 11007 | 20 | 20 | 19 |
| Flame Attack, 11016 | 20 | 42 | 21 |
| Flame Hit, 11029 | 47 | 150 | 46 |
| Blast Attack, 11166 | 20 | 10.90 | 21.80 |
| Icicle Attack, 11001 | 45 | 45.20 | 43.60 |
| Tulwar, 15072 | 145 | 153.20 | 157.60 |
| Hagendis Beating, 15045 | 250 | 3465 | 401 |

The alternative magical `2.5A + 1.5BG` gives Blast Attack 20.35, but its overall
tail errors are larger. StatMultiplier substitutions and multiplication or
division by normalized StatMultiplier did not outperform the layer formulas.
This experiment does not change how StatMultiplier affects the attacking stat.

## Checks against fitting the sample

For each of the five reference pages, the coefficient search trained only on
the other four pages and evaluated the omitted page. The aggregate held-out
median error was 8.23%, with 144 skills within 10% and 183 within 20%. Physical
coefficients remained `(2, 0.5)` in every fold; magical coefficients varied
between `(2, 2)` and `(2.5, 1.5)`.

Unrestricted relative-least-squares fitting did not improve the median: it gave
physical coefficients `(1.3272, 0.6588)` and magical `(1.5369, 1.5268)`, with
13.34% in-sample median error and 14.28% page-held-out error. Its lower mean
error trades away accuracy around the typical skill to reduce larger outliers.

Treat all selected coefficients as empirical approximations. Reference P values
come from community reconstruction, not original server code. Source-version
differences, skill-specific behavior and ID binding errors remain possible.
Large residuals include Hagendis Beating, Rock Elf Beating, Rockfall Beating,
Sword Awn Beating and Goddess' Anger. Several improve while others worsen.

Max-grade observations cannot identify the grade curve: many different curves
have the same endpoint. A diagnostic ten-grade clamp only modestly changed the
fit and is not evidence to revert native grade limits. Low/middle/max-grade
damage observations with known ATK/MATK and target defenses would distinguish
the remaining hypotheses. No per-skill exceptions have been introduced.

## Reproduce

```sh
python tools/skill-power-analysis/analyze.py
```

Inputs are the optional local skill/animation exports and the tracked
`internal/assetsql/skill_power_defaults.csv`. Command-line switches can select
other input paths. The tool requires only Python's standard library and writes
`var/reports/skill-power-formulas/comparison.json` and `report.html`. The report
includes every prediction, source-page metrics, coefficient-search folds,
contact weights and source hashes. These optional analysis exports are not
server build or runtime dependencies.

## Raw-offset follow-up

The follow-up ignores the semantic names on the fields. It audits every declared
numeric field and the remaining non-text bytes, and searches arithmetic terms
formed from decoded disk offsets. Names, descriptions, IDs, ordering, icons,
voices and linked skill IDs are not used as numerical power predictors. The
two skill-reference words are used only to keep linked records in the same
validation fold. This avoids fitting the skill's identity as a disguised lookup.

The search considers one to three terms, relative least squares and robust
refinement of the strongest candidates. The selection score combines median
error, mean error and a term-count penalty. It includes 17,343 candidate term
sets for the byte-29 partition and 18,472 for the alternative float-40 partition.
Feature selection and coefficient fitting are both repeated on the training
portion of each held-out test.

### New useful fields

The unlabelled byte at offset 30 ranges from 1 to 11 in this reference sample.
Its correlation with reference P is 0.874, higher than the additive word at 49
(0.772). A tier-like interpretation is plausible, but remains unverified.
Its square and its product with word 49 improve predictions.

Byte 31 is target-count-like, but is not a reliable replacement for the pattern
bytes. For example, Sword Awn Beating and Goddess' Anger contain 1 at byte 31
and non-single-target values in their pattern bytes. Direct arithmetic involving
byte 112 reduces several large residuals. Neither byte 31 nor pattern values
are being renamed to confirmed target counts from this statistical evidence.

The loader `FUN_00480f8c` decrypts manager offset 0x39, disk offset 53, with the
byte cipher. The current export's declared cipher-field table omits this byte.
The research tool independently exposes `((raw[53] ^ 0xfd) - 4) mod 256`, which
is 0 or 1 in the source. It was included in the search but was not selected by
the strongest model. No production decoder or exported archive was rewritten.

Many other unresolved numeric fields are zero throughout all 224 reference
skills. Their coverage is retained in `field_audit` rather than assigning them
unsupported meanings or silently excluding them from inspection.

### Strongest fitted equation

Let these names denote **raw offsets**, not the existing semantic labels:

- `T = u8[30]`
- `D = float64[32]`
- `S = float64[40]`
- `H = u16[49]`
- `G = u8[105]`
- `K = max(1, u8[112])`

At the maximum-grade reference points, the strongest selected structure is:

```text
S < 1:   P ≈ 0.032156*T*H + 1.448613*H/K + 0.622562*D*G/K
S >= 1:  P ≈ 0.131157*T*H + 0.886926*H/K + 2.216883*D*G/K
```

The split at raw double 40 fits better than presuming that UI byte 29 selects
the power coefficients. This does not mean the split establishes physical versus
magical damage. Bat Wind and Fox's Fire Ball are described as magical in the
reference, but have `S=0.6`; they use the lower-S calibration curve successfully.
Damage type and power calibration therefore must not be conflated.

| Model/check | Median error | Mean error | Within 10% | Within 20% |
| --- | ---: | ---: | ---: | ---: |
| Earlier fixed hybrid | 7.96% | 18.36% | 145 | 181 |
| Raw model, split by byte 29, fitted on all references | 3.71% | 7.45% | 183 | 207 |
| Raw model, split by double 40, fitted on all references | **3.32%** | **6.80%** | 184 | 207 |
| Double-40 search, held-out linked-record groups | 4.29% | 7.50% | 182 | 207 |
| Double-40 search, held-out reference pages | 5.52% | **24.18%** | 155 | 197 |

Counts are out of 224. Splitting by byte 27 instead produced a worse 6.41%
in-sample median error, with 7.29% on held-out linked groups.

The page test exposes an important limit: omitting the entire unique-skill page
causes selection of an equation containing division by G, which extrapolates
poorly to grade-one character stunts absent from the elemental training pages.
The good aggregate median does not erase that high mean error. These are
exploratory calibration results, not independently verified server mechanics.

| Skill | Reference P | Earlier fixed hybrid | Raw double-40 fitted model |
| --- | ---: | ---: | ---: |
| Flame Attack | 20 | 21 | 19.08 |
| Blast Attack | 20 | 21.80 | 19.37 |
| Rockfall Beating | 75 | 196.08 | 75.62 |
| Hagendis Beating | 250 | 401 | 213.01 |
| Bat Wind | 120 | 207.20 | 117.79 |
| Fox's Fire Ball | 120 | 218.40 | 118.93 |
| Goddess' Anger | 205 | 508.64 | 207.94 |

Hagendis remains substantially low. Precise coefficients and every prediction,
including worse cases, are in the report rather than hidden by rounding.

### Version and reference checks

The local IGG version contains 223 of the 224 reference skills. Their doubles
at 32/40, word 49 and bytes 30/31/105 match the updated version. There are a few
other differences, mainly voice/icon data, and SP/pattern changes for Goddess'
Anger. The remaining discrepancies cannot generally be attributed to changed
native power coefficients between these two installations.

Some records share all non-identity numeric bytes but have different reference
P values:

- Combo x3 Attack (11076), Ghost Hammer (11182), and Leap (12036): 53, 75, 53.
- Deacon Attack (11183) and Palm (15040): 35 and 39.
- Bee Swirl (30027) and Combo Stab (30037): 40 and 41.

Their MBTM animation modes and contacts differ, so this is not proof that the
references are wrong. It does rule out expecting an exact shared formula from
only these non-identity Skill.dat numbers. Skill-specific behavior, animation
semantics or uncertainty in the experimental P estimates remain possible.
Identity-based memorization is not presented as a recovered formula.

No runtime semantics, authored P values, skill labels or database fields were
changed by this investigation. In particular, the fitted endpoint equation does
not establish what should replace raw G at intermediate learned grades.

The inspected native loader establishes byte locations and cipher operations;
it does not establish a server-side damage equation. The names PowerPerLevel and
StatMultiplier remain compatibility interpretations, inherited from the private
server's Decimal1/Decimal2 fields. The comparison therefore does not validate
the production assumption that double 40 multiplies ATK/MATK independently of P.
Community P estimates also depend on assumptions about that attacking-stat term.

The next discriminating measurements are differences in damage from the same
skill/grade against the same enemy at two or more known attacking-stat values,
followed by measurements at several skill grades while stats stay fixed. Keep
element, target, buffs and equipment effects fixed, exclude critical/combo hits,
and collect repeats to measure the random variation. Those differences isolate
the stat and grade slopes without first assuming that any field is a multiplier.

### Reproduce the raw search

```sh
python tools/skill-power-analysis/raw_search.py
python tools/skill-power-analysis/raw_search.py --strata float40 --output var/reports/skill-power-raw/float40
python tools/skill-power-analysis/raw_search.py --strata byte27 --output var/reports/skill-power-raw/byte27
python -m unittest discover -s tools/skill-power-analysis -p test_raw_search.py
```

Add `--igg-skill PATH_TO_IGG/data/Skill.dat` for the optional version audit.
`raw-analysis.json` and `report.html` contain the field census, candidate models,
fold selections, held-out predictions and conflicting records. Arithmetic tests
check an independent linear system, recovery of a known scale and reference-link
grouping. This remains optional offline tooling without server dependencies.

## Structured follow-up and unequal hits

The next investigation treats reference P as the **whole attack's additive
power, summed across all contacts, for one affected target**. An area attack
does not mean summing damage to several different targets into that P value.
This follows the user's clarification and the native presentation evidence.

### Star's unequal contacts

Star (15071, Xaolan) has MBTM damage mode 2 and two authored weights, **5 and
95**. They total 100, with a 19:1 ratio before rounding. Flame Attack uses
50/50, Combo x3 Attack uses 33/33/34, and Hagendis Beating uses eight 11s and
one 12. Unequal contact damage is therefore explicitly represented in the
animation data; equal division by hit count is inappropriate.

The decompiled `FUN_0039fdd8` accumulates received HP/SP changes (stat codes
25/26), distributes those totals through the motion's contact weights, and
reconciles rounding excess/shortfall against the last contacts. The existing
client implementation is `Motion.DamageParts64` in `client/wlo/battle/assets.go`.
For illustration, a Star attack dealing 200 total damage displays 10 and 190.
Those are portions of the final damage, **not** the reference P of 195 being
sent or applied separately to each hit.

All 224 matched skills have an effective contact-weight sum of 100%. This
supports separating the total-power equation from presentation distribution.
It does not establish how the original server computes that total internally.

### Search scope and constraints

The new tool compares 83,454 one-, two- and three-term subsets using 63 numeric
features, both with shared coefficients and with coefficients split at raw
`S < 1`. It adds continuous S interactions, grade-square/root alternatives,
`G-1` terms and shape attenuation. A separate 109,618-subset screen includes
six contact-count/weight interaction features. Neither search uses skill IDs,
names, icon IDs, per-skill exceptions or the reference P itself as predictors.

Coefficients are constrained nonnegative. No inverse-grade term is available,
avoiding the previous grade-one extrapolation failure. The ranking penalizes
the actual number of fitted coefficients, including both sides of a split.
Coefficient refinement uses positive active-set least squares and robust
relative-error weights. Both structure selection and fitting run anew on the
training portion of every validation fold.

### Most accurate candidate

Using the same raw-offset notation (`H=u16[49]`, `D=f64[32]`, `S=f64[40]`,
`G=u8[105]`, `K=max(1,u8[112])`), the strongest selected total-power equation is:

```text
P = a*H*S/K + b*D*(G-1)*S^2/sqrt(K) + c*D*sqrt(G)

             a          b          c
S < 1     2.308306   0.320553   1.816243
S >= 1    1.264396   1.784979   0.277546
```

This is a fitted maximum-grade estimator. In particular, the square root does
**not** prove that actual skill progression uses a square-root curve. Nor does
S establish physical/magical classification. K is an ordinal shape code, not
a proven numeric target count; its use as a divisor remains an empirical proxy.
The tool also tests mapped shape capacity and smoother attenuation alternatives.

| Check | Median error | Mean error | Within 10% | Within 20% |
| --- | ---: | ---: | ---: | ---: |
| Production `(H + DG) * contacts` | 50.61% | 152.15% | 37 | 54 |
| Previous raw-offset fit | 3.32% | 6.80% | 184 | 207 |
| New candidate, fitted on all 224 | **2.12%** | **5.40%** | 192 | 209 |
| New search, held-out linked-record groups | **2.53%** | **5.89%** | 189 | 208 |
| New search, held-out reference pages | **2.26%** | **7.00%** | 181 | 202 |

Leaving the entire unique-skill page out now yields 3.02% median / 9.34% mean
error on its 71 skills. That avoids the earlier large grade-one extrapolation
errors, but does not eliminate unique-skill exceptions. These remain internal
checks on a repeatedly studied reference collection, not fresh independent
damage measurements.

Rounding `(a,b,c)` to `(2.3,.32,1.8)` and `(1.25,1.8,.28)` gives 2.11% median
and 5.44% mean error. This rounding was assessed on the full sample after
selection, so it has no separate held-out claim.

A simpler, shared two-term approximation is:

```text
P = 1.4*H/sqrt(K) + 1.7*D*G*S^2/K
```

It has 5.98% median / 8.72% mean error, with 208/224 within 20%, and avoids
branching on S. Its constants are rounded fitted values, not discovered native
constants. The lower complexity costs accuracy; it is included as an alternative
approximation rather than claimed as the original equation.

### Examples and remaining failures

| Skill | Reference total P | New fitted P |
| --- | ---: | ---: |
| Flame Attack, 11016 | 20 | 22.36 |
| Blast Attack, 11166 | 20 | 18.34 |
| Star, 15071 | 195 | 193.64 |
| Hagendis Beating, 15045 | 250 | 257.85 |
| Rockfall Beating, 11091 | 75 | 94.35 |
| Goddess' Anger, 30133 | 205 | 277.55 |
| Sound Wave, 30103 | 83 | 170.67 |

The 204 single-shape skills have 1.90% median error, while the 20 area-shape
skills still have **14.90%** median error. Only 6/20 area attacks are within 10%.
The 15 grade-one skills have 9.31% median error. These subgroups and the large
Sound Wave residual prevent treating the aggregate score as a universal rule.

Adding contact count, its inverse and the sum of squared contact weights as
candidate interactions selects exactly the same best equation. This screen
provides no evidence that those animation features improve total P. Multiplying
the selected total by contact count instead increases median error to 96.11%;
multiplying by the sum of contact percentages leaves it unchanged. These are
diagnostic comparisons, not independently fitted contact models.

No production damage calculation, decoder, authored power table, grade cap,
asset database or seed changed in this follow-up. A possible next measurement
is Star's total damage (both numbers combined) at several fixed-stat grades,
then at several known MATK values at one grade. That distinguishes grade and
stat effects without mistaking its 5/95 presentation split for separate powers.

### Reproduce

```sh
python tools/skill-power-analysis/structured_search.py
python tools/skill-power-analysis/structured_search.py --animation --no-validation --output var/reports/skill-power-structured/animation
python -m unittest discover -s tools/skill-power-analysis -p 'test_*.py'
```

The first command writes `var/reports/skill-power-structured/analysis.json` and
`report.html`; the JSON includes every contact record, prediction, subgroup
metric, comparison and fold-selected equation. The animation command is a
separate exploratory screen and makes no additional held-out claim. Six focused
tests verify the independent solver, positive boundary/singular cases, a known
synthetic power equation and linked-record grouping. Client extraction remains
an optional research input, never a server build/runtime requirement.

## Area-capacity follow-up

The user's area-attack clue is supported, but two different effects must be
kept separate: a coefficient on the attacking **stat**, and any reduction of
the additive **P**. Neither automatically implies dividing final damage by the
number of enemies actually present.

### Stat coefficient: evidence outside Skill.dat

The supplied `var/damage/formulas` specifies this attack-range table:

| Maximum targets | ATK/MATK coefficient | Relative to single-target coefficient |
| --- | ---: | ---: |
| 1 | 1.40 | 1.0000 |
| 2 | 0.85 | 0.6071 |
| 3 or 4 | 0.60 | 0.4286 |
| 6 | 0.40 | 0.2857 |

This is already represented by `Rules.damageBase` in
`internal/battle/damage.go`: it selects the coefficient using targeting capacity
at the current grade, independently of occupied positions. The current formula
is `coefficient * STAT * nativeStatMultiplier + P - defenseCorrection`.
Consequently, adding another identical multiplier to final damage would also
scale P and defense, and would not reproduce this rule.

The [Japanese experimental damage page](https://wikiwiki.jp/vipwo/ダメージ計算式)
also changes the stat coefficient by range, but reports 0.84 for two targets
and 0.40 for three, rather than the supplied table's 0.85 and 0.60. It cautions
that defense constants may also change and gives Rain Arrow a separate formula.
The saved Earth skill table explicitly reports 0.85 for Rock Blast Hit and 0.60
for Rock Blast Beating. These sources do not justify treating all range rules
as fully settled or applying one multiplier to the entire damage expression.

### Independent target-count extraction

`tools/skill-power-analysis/area_search.py` reads the saved wiki tables' range
columns, resolving merged cells and grade bands through the existing authored
source bindings. It does not infer target counts from Skill.dat. The 224 power
references contain **202 explicitly single-target**, **20 area**, and **two
unspecified-range** entries. Missing range data is excluded, not treated as one.

The area sample comprises 13 two-target, four three-target, two four-target and
one six-target skill. The final Fire Stone Bead range is four at grades 9–18;
earlier range rows are not mistaken for its maximum-grade capacity. Goddess'
Anger likewise uses the reference's grades 9–18 four-target row.

Blaze Beating (15108) is explicitly a three-by-two, **six-target** attack in the
saved Fire table. The previous analysis's C#-derived shape mapping interpreted
its raw code 7 as eight. This follow-up uses six from the independent reference;
it does not silently change runtime targeting or claim every code 7 is six-target.
The two unknown-range references are Tail Hit (30011) and Summon Eagle (30072).

### Does additive P have a separate area penalty?

Select and fit a baseline using only the 202 single-target references. All 20
area powers are excluded from that baseline's structure and coefficient fit.
The same three positive terms are selected, without any shape/target divisor:

```text
P0 = a*H*S + b*D*(G-1)*S^2 + c*D*sqrt(G)

             a          b          c
S < 1     2.171964   0.337769   1.943066
S >= 1    1.263623   1.782639   0.280566
```

These remain estimated maximum-grade inputs, not recovered native meanings or
a proven grade-progression curve. The single-target fit has 1.93% median and
4.24% mean error. Applying it to area skills without attenuation overestimates
their P: 57.42% median / 77.76% mean error on those 20 skills.

| Wiki target capacity | Skills | Median reference P / estimated P0 | Observed ratio range |
| --- | ---: | ---: | ---: |
| 2 | 13 | 0.6723 | 0.5060–0.8716 |
| 3 | 4 | 0.4108 | 0.4051–0.6541 |
| 4 | 2 | 0.3780 | 0.3383–0.4177 |
| 6 | 1 | 0.5320 | one observation |

The broad reduction is consistent with an area penalty, but the within-count
variation and six-target exception rule out claiming a universal count-only
law from this sample. Ratios inherit baseline-estimation error as well as any
reference/version/skill-specific differences.

Matched inputs provide an additional check independent of the fitted baseline:
Thunder Beating / Fire Combo Beating share raw H, D, S and maximum grade, with
P values 275 / 405 (ratio 0.6790); Blaze Hit / Rock Ball Hit give 130 / 190
(0.6842). Both pairs contrast two-target with single-target skills. The two
Goddess' Anger records give four-target 205 versus single-target 660 (0.3106).
Other matched pairs range up to 0.8148. These are different skills; matching
four inputs does not control every other field, element, grade history or version.

### Compact penalty candidates and limits

The tool tests `P = P0 * m(N)`, where N is reference target capacity. It fits
two one-parameter families on the 20 area powers, with parameters from 0 to 4
in steps of 0.01. The reciprocal family selects `alpha=0.50`:

```text
m(N) = 1 / (1 + alpha*(N-1)) = 2 / (N+1)
```

| Area-power model | Median error | Mean error |
| --- | ---: | ---: |
| No area penalty | 57.42% | 77.76% |
| `1/N` | 25.87% | 28.33% |
| `1/sqrt(N)` | 13.53% | 18.53% |
| Supplied stat-coefficient ratio applied to P | 10.12% | 14.17% |
| Fitted `1/(1+alpha*(N-1))`, alpha 0.50 | 9.12% | 13.99% |
| Fitted `N^-q`, q 0.59 | 9.09% | 14.75% |

The reciprocal model has 14.88% median error when refitting alpha with each
area skill held out, and 16.68% when holding out each reference page's area
skills. The power-law equivalents are 9.84% and 16.40%. The single-target
baseline remains fixed in these checks; this is validation of the area
parameter, not an end-to-end page holdout. The wider research has repeatedly
examined these references, so neither check is independent live validation.

`2/(N+1)` is a useful compact approximation, not a confirmed server formula.
For example, it predicts approximately 86 for six-target Blaze Beating versus
reference P 160. Applying the attack-stat ratio to P is also only a hypothesis:
the source formula does not instruct that multiplication.

### What would distinguish capacity from actual targets?

Use the **same skill and grade**, attacker stats, defender type and formation
anchor. Compare damage to an identical defender with one, two and then more
occupied positions in the skill's area, without buffs, combos or criticals.
Sum all contact numbers for that defender, not damage across different targets.
Changing occupancy alone tests whether the penalty follows actual hits.

Then vary ATK/MATK while keeping everything else fixed: the damage slope
estimates the stat coefficient, while the intercept contains P and defense.
Repeat against known defenses to separate those two terms. Static skill power
tables and animation weights cannot establish that occupancy-dependent rule.

### Reproduce the area investigation

```sh
python tools/skill-power-analysis/area_search.py
python -m unittest discover -s tools/skill-power-analysis -p 'test_*.py'
```

Outputs are `var/reports/skill-power-area/analysis.json` and `report.html`.
They retain source hashes, exact table/row evidence, grade ranges, predictions,
matched-input pairs and every held-out area fit. Ten focused analysis tests
pass, including explicit range parsing, unknown-range handling, grade-band
selection and recovery of a known synthetic attenuation parameter. No runtime
formula, targeting data, authored P table, server seed or database changed.

## Sample 1: live Legacy measurements

Read `var/damage/samples/sample1.ods` without editing it. The three sheets contain
31 attacks, including per-contact numbers which are summed into one damage total
per cast. The user confirmed Star is grade 8 and that these attacks had no buffs,
combos or criticals. Sheet3 D24's 203 damage may have been against Straw Chicken
rather than the recorded Hyperkinetic Chicken; retain both interpretations and
exclude this one uncertain-target observation from aggregate metrics.

Actor inputs are the recorded combat stats, not values reconstructed from STR
or level. Sheet1 has Fire / level 13 / ATK 77 / MATK 25; Sheet2 has an unnamed
Earth actor / level 13 / ATK 31 / MATK 24; Sheet3 has Fire / level 14 / ATK 89 /
MATK 24. Star's Fire skill element overrides the Earth actor's element.

Enemy definitions come from the tracked schema-57 repository seed, read in
memory, rather than the older development assets database. Applying current
`game.InnateCombat` gives these **Gonline** estimates; their exact Legacy server
defenses are not measured in the workbook:

| Enemy | ID | Level / element | DEF | MDEF |
| --- | ---: | --- | ---: | ---: |
| Straw Chicken | 17045 | 12 / Wind | 34 | 44 |
| Hyperkinetic Chicken | 17044 | 11 / Fire | 32 | 42 |
| Rice Chicken | 17042 | 11 / Earth | 36 | 44 |

The current damage ranges below include the actual server's integer floors and
uniform `Next(1,6)` roll (1 through 5). All are landed standalone attack totals;
damage is not capped to the chicken's remaining HP. Independently replaying
`battle.New` and `Battle.Round` checks all five rolls for each of the 31 input
rows: **155/155 runtime checks exactly match** the analysis calculations.

| Sheet | Skill / grade | Target | Legacy totals | Current Gonline range |
| --- | --- | --- | --- | --- |
| 1 | Flame Attack 5 | Straw | 153, 157, 156 | 118–122 |
| 1 | Flame Hit 10 | Straw | 192, 193, 192 | 292–296 |
| 1 | Flame Beating 3 | Straw | 206 | 237–241 |
| 1 | Flame Beating 4 | Straw | 210, 210 | 259–263 |
| 1 | Sword Awn Attack 1 | Straw | 175, 178, 178 | 85–89 |
| 1 | Sword Awn Attack 2 | Straw | 181 | 93–97 |
| 1 | Blast Attack 1 | Straw | 22, 22 | 2–6 |
| 1 | Blast Attack 1 | Hyperkinetic | 16, 19 | 4–8 |
| 2 | Star 8 | Straw | 310 | 305–309 |
| 2 | Star 8 | Hyperkinetic | 255 | 251–255 |
| 3 | Flame Attack 5 | Rice | 135 | 98–102 |
| 3 | Flame Attack 5 | Hyperkinetic | 150 | 112–116 |
| 3 | Flame Attack 5 | Straw | 179 | 134–138 |
| 3 | Flame Hit 9 | Hyperkinetic | 177, 179 | 239–243 |
| 3 | Flame Hit 9 | Rice | 163 | 216–220 |
| 3 | Sword Awn Attack 1 | Hyperkinetic | 164 | 84–88 |
| 3 | Sword Awn Attack 1 | Rice | 151 | 73–77 |
| 3 | Turning Fire Attack 1 | Straw | 241 | 124–128 |
| 3 | Turning Fire Attack 1 | Hyperkinetic | 199 | 103–107 |
| 3 | Turning Fire Attack 2 | Rice | 187 | 100–104 |

For the uncertain 203 observation, the current prediction is 84–88 if the target
was Hyperkinetic and 101–105 if it was Straw. The frozen-P/no-extra-stat-multiplier
research variant predicts 163–167 and 197–201 respectively. The Straw possibility
is more consistent with that variant, but is not silently accepted as a correction.

### What the sample distinguishes

The current formula is inaccurate in both directions. Its mean absolute
percentage error against the 30 certain-target observations is **40.15%**, with
46.35% median error. Only one observation lies inside its predicted random range.
Repeated casts receive equal weight; the report also preserves grouped counts.

Three diagnostic alternatives are compared without refitting the earlier P
coefficients. The full structured P equation above uses each observed grade in
place of maximum grade; all of this sample's skills have K=1. This grade use is
an exploratory extension, not a previously established grade curve.

| Model | Physical mean error, 24 casts | Magical mean error, 6 casts |
| --- | ---: | ---: |
| Current server | 37.84% | 49.38% |
| Native `H+DG` total P, extra stat multiplier omitted | 6.87% | 70.92% |
| Frozen structured P, native attacking-stat multiplier retained | 38.28% | 45.90% |
| Frozen structured P, extra attacking-stat multiplier omitted | **1.62%** | 54.35% |

The last physical variant uses `1.4*ATK + Pfit - .98*DEF` before element and
randomness. S remains a predictor inside Pfit; it no longer additionally scales
ATK. For Sheet1 Flame Hit 10 it predicts **190–194**, close to observed 192–193,
versus the current **292–296**. For Sheet3 Sword Awn Attack 1 / Hyperkinetic it
predicts **163–167**, versus observed 164 and current **84–88**. No coefficients
were fitted to Sample 1, but the decision to compare the omitted-stat-multiplier
variant uses this sample; these figures are not blind validation of a complete
new formula.

This is evidence against treating the raw physical S values around 0.6 as an
additional direct ATK multiplier in the current pipeline. It also supports
revisiting the assumption that H+DG represents per-contact rather than total P.
It does not establish a universal physical or magical equation.

Magic remains a separate failure. Star is already close with the current
pipeline: `(40+7.08*8)*2 = 193.28` P, approximately the authored 195 total power.
Replacing that P with the frozen fit while retaining native S gives **306–310**
and **251–255**, covering both Star observations. In contrast, Blast Attack 1
still predicts only **2–6 / 4–8** against observed **22 / 16–19**. The sample
does not identify whether that gap is the low-offense magic branch, defense,
grade progression or another original-server rule. Omitting S globally would
not resolve magic. None of these attacks has an area range, so Sample 1 cannot
test the proposed area-count attenuation.

### Reproduce Sample 1 comparison

```sh
python tools/skill-power-analysis/sample_compare.py --star-grade 8
python -m unittest discover -s tools/skill-power-analysis -p 'test_*.py'
```

The first command writes `var/reports/skill-power-sample1/comparison.json` and
`report.html`, including workbook/seed hashes, all original observations, both
uncertain-target interpretations and grouped/model metrics. The local
`server-verification.json` records the independent 155-check Go replay.
Thirteen focused research tests pass. No gameplay formula or database changed.

## Higher base power and lower grade growth

The user's next hypothesis is supported by the physical attacks in Sample 1.
Hold the attacking-stat treatment fixed at the previous investigation's
`1.4*ATK`, and treat P as the whole cast's power. Then compare these two simple
power equations directly:

```text
Baseline:   P = H + D*G
Candidate:  P = 2*H + 0.5*D*G
```

H is raw AdditionalDamage, D is raw PowerPerLevel, and G is the observed grade.
The comparison doubles the fixed H term and halves the grade-dependent term;
it does not multiply either total by animation contact count. This change is
examined independently of the earlier decision to omit S from the ATK term.

### Evidence from changes in grade

Within the same actor/target pairing, the stat and fixed-power terms cancel
when comparing different grades:

| Skill / target | Recorded grade change | Legacy mean damage change | Current expected grade contribution | Candidate expected grade contribution |
| --- | --- | ---: | ---: | ---: |
| Sword Awn Attack / Straw | 1 to 2 | 177 to 181: +4 | `5.2*1.6 = 8.32` | `.5*5.2*1.6 = 4.16` |
| Flame Beating / Straw | 3 to 4 | 206 to 210: +4 | `4.4*3*1.6 = 21.12` | `.5*4.4*1.6 = 3.52` |

The expected contributions in this table are before integer rounding and
randomness. The observations include few casts at each grade, so the differences
support slower growth without fixing its exact coefficient to 0.5.

### Physical damage accuracy

All 24 certain-target physical attacks are retained. The uncertain 203 target
is excluded as before. These observations cover five skills and 16 distinct
sheet/skill/grade/target groups, not 24 independent skill definitions.

| Physical model | Mean damage error | Median damage error | Observations inside estimated random range |
| --- | ---: | ---: | ---: |
| Current server | 37.84% | 40.81% | 0/24 |
| Total P `H+DG`, S omitted from ATK | 6.87% | 6.57% | 6/24 |
| Previous frozen structured P, S omitted from ATK | 1.62% | 1.44% | 11/24 |
| Total P `2H+.5DG`, S omitted from ATK | **1.24%** | **1.13%** | 15/24 |
| Total P `2H+.5DG`, native S retained on ATK | 36.70% | 36.39% | 0/24 |

The direct base/grade adjustment improves 6.87% to 1.24%. Comparing 37.84% with
1.24% also includes the separate ATK/contact-treatment changes, so that entire
gain must not be attributed to the base/grade adjustment alone.

Fitting `P=aH+bDG` to the 16 group means, with one weight per group, gives:

```text
P = 1.913145*H + 0.475240*D*G
```

The fit uses positive relative-power least squares after reversing the assumed
physical damage pipeline and subtracting the estimated midpoint random roll.
It produces 0.95% mean / 0.96% median damage error in-sample. Leaving each entire
skill out, including all its grades/actors/targets, and refitting on the others
gives **1.09% mean / 1.02% median** damage error. These are internal checks using
current Gonline enemy defenses and the assumed noise midpoint. They do not
recover the original server constants or eliminate defense/rounding uncertainty.

For example, `2H+.5DG` gives Flame Attack 5 total P 21 rather than current P 42,
and damage **153–157** against Sheet1's Straw Chicken, matching the recorded
153, 157 and 156. Sword Awn Attack 1 gives P 34.6 and **175–179**, close to the
recorded 175, 178 and 178. Flame Beating remains slightly high at **210–214**
versus 206 at grade 3, so the rounded formula is not exact.

### Cross-check against the maximum-grade references

Compare only the 116 explicitly single-target physical reference powers;
area powers have an additional unresolved penalty, and two reference entries
have unspecified target ranges.

| Physical maximum-grade P model | Median power error | Mean power error | Within 10% |
| --- | ---: | ---: | ---: |
| `H+DG` | 10.16% | 11.49% | 58/116 |
| `2H+.5DG` | 6.95% | 11.45% | 83/116 |
| Sample-fitted `1.913145H+.475240DG` | **3.69%** | **8.42%** | 95/116 |

The sample-fitted coefficients also transfer reasonably to these reference
powers without being fitted to them in this experiment. This reference set has
been repeatedly studied in the wider investigation; it is not a fresh blind
validation set. Grade-one/unique-skill exceptions remain, and its best general
structured estimator is still more accurate on the broader collection.

The useful conclusion is a stronger fixed power term plus slower grade growth
for **physical attacks**, with a simple approximation near `2H+.5DG`. This
does not settle magic: applying those same coefficients to the six magical
observations while retaining native S leaves 48.70% mean damage error. Star and
low-MATK Blast Attack still require separate validation. No server formula or
database changed.

### Reproduce the base/grade check

```sh
python tools/skill-power-analysis/grade_search.py --star-grade 8
python -m unittest discover -s tools/skill-power-analysis -p 'test_*.py'
```

Outputs are `var/reports/skill-power-grade/analysis.json` and `report.html`, with
source hashes, all group predictions, fitted coefficients and each held-out
skill's fit. Fifteen focused research tests pass, including recovery of known
synthetic base/grade coefficients and keeping grade growth independent of
animation contact count.

## Saved high-MATK bug measurements

The completed `outputs/bug-damage-20261106/bug-damage-recording.xlsx` contains
17 attacks: eight for DangoKnght (Fire, MATK 1052) and nine for DangoBunny
(Water, MATK 815). All seven recorded skills are native magical, single-target
skills, including Fire Combo Beating. The user confirmed the two blank targets
at DangoKnght D22/D23 are Female Bug; the source cells remain unchanged.

Fire Combo Beating's `2346 + 2346 = 4692` row is marked "Likely Critical".
Half is 2346, matching the two ordinary totals 2346 and 2345. Preserve the raw
measurement and exclude this flagged row from normal-error metrics. The remaining
16 observations cover 11 sheet/skill/grade/target groups across seven skills.

### Neutral enemies require baseline growth

The repository definitions identify Male Bug as 17070, level 23, and Female Bug
as 17071, level 24. Both have element zero. `game.InnateCombat` uses baseline
growth for element zero, with 2 points per CON/WIS, rather than the elemental
1.75 CON / 2.2 WIS coefficients. This gives Male DEF/MDEF **72/74** and Female
**76/78**. The offline sample helper now mirrors that fallback; previous chicken
estimates are unchanged. No gameplay growth code changed.

Using those inputs, the actual Go `battle.New` / `Battle.Round` replay agrees
with all **85** current-formula checks (five random rolls per input). The replay
verifies ordinary formula predictions, including an ordinary hypothetical for
the critical-marked row; it does not assert that the marked critical was ordinary.

### New evidence about the extra stat multiplier

The same frozen structured P coefficients from the earlier reference analysis
are evaluated at the recorded grades, without refitting to this new workbook:

| Model | Mean damage error | Median damage error | Inside current estimated random range |
| --- | ---: | ---: | ---: |
| Current Gonline | 12.16% | 13.18% | 0/16 |
| Frozen structured P, native S still on MATK | 15.31% | 16.01% | 0/16 |
| Frozen structured P, S omitted from MATK | **0.91%** | **0.49%** | 3/16 |
| Physical candidate `2H+.5DG`, S omitted | 4.45% | 3.98% | 3/16 |
| Previously considered magic candidate `2(H+DG)`, S omitted | **0.61%** | **0.50%** | 6/16 |

The comparison between the two frozen-P rows changes only the extra S factor
on MATK, providing stronger evidence than Sample 1's low-stat magical attacks.
For eight ordinary rows, retaining S on MATK would require **negative additive
P** to reproduce the observation with the assumed enemy MDEF, element and
midpoint random roll. Omitting S gives positive implied powers throughout.
The relevant skills are Blast Attack, Fire Ball Attack, Water Thunder Attack and
Thunder Attack. This supports investigating S as a power-side field rather than
automatically multiplying MATK by it in addition to its use in the P estimator.
It still depends on the enemy-defense and elemental assumptions.

### Representative observed and predicted totals

All numbers sum the contact displays for one target. The ranges use the
current Gonline 1–5 random roll and integer floors; Legacy's exact noise and
defenses remain unverified.

| Skill / grade | Target | Legacy total | Current Gonline | `2(H+DG)`, S omitted |
| --- | --- | ---: | ---: | ---: |
| Fire Stone Hit 11 | Female | 2217, 2218 | 2398–2402 | 2214–2218 |
| Fire Stone Hit 11 | Male | 2226 | 2403–2407 | 2219–2223 |
| Fire Combo Beating 18 | Female | 2346, 2345 | 2659–2663 | 2314–2318 |
| Blast Attack 4 | Female | 1845 | 2020–2024 | 1841–1845 |
| Fire Ball Attack 1 | Female | 1871 | 2111–2115 | 1869–1873 |
| Water Thunder Attack 1 | Male | 1053, 1053 | 1270–1274 | 1065–1069 |
| Water Thunder Attack 1 | Female | 1051 | 1267–1271 | 1061–1065 |
| Ice Elf Beating 13 | Female | 1185, 1185 | 1236–1240 | 1184–1188 |
| Ice Elf Beating 13 | Male | 1185 | 1239–1243 | 1188–1192 |
| Thunder Attack 1 | Male | 1033, 1037 | 1187–1191 | 1041–1045 |
| Thunder Attack 1 | Female | 1031 | 1183–1187 | 1038–1042 |

The mean final-damage error of 0.61% must not be read as 0.61% power accuracy:
high MATK dominates much of the total. Against P inferred using `1.4*MATK`, the
same `2(H+DG)` candidate has **6.93% mean / 5.51% median power error**. The frozen
structured P has 13.28% mean inferred-power error. Those inferred powers also
inherit MDEF/element/noise uncertainty.

Two additional magic grade-offset diagnostics, `2H+2D(G-1)` and
`2H+D(2G-1)`, give 0.66% and 0.57% mean damage error respectively. Their marginal
scores do not establish an exact grade offset. They are exploratory comparisons,
with no claimed independent validation or original-server status.

### Remaining low-MATK mismatch

The simple `2(H+DG)` magic candidate, with S omitted from MATK, still fails
Sample 1's six low-MATK magic observations: **42.13% mean damage error**. For
Blast Attack 1 against Straw Chicken it predicts **5–9** instead of observed
22; against Hyperkinetic it predicts **6–10** instead of 16 and 19. Star remains
approximately 4% low. The high-MATK result therefore supports the stat-side
correction but does not settle the low-offense branch, grade progression,
defense correction or every skill's P. Both samples must be respected in any
eventual implementation. These samples still contain no area skills.

### Reproduce the bug comparison

```sh
python tools/skill-power-analysis/bug_compare.py
python -m unittest discover -s tools/skill-power-analysis -p 'test_*.py'
```

The read-only tool preserves the workbook hash, original contacts/notes and
explicit target clarifications in `var/reports/skill-power-bugs/analysis.json`.
Its `report.html` includes all grouped comparisons, power diagnostics and the
low-MATK cross-check. `server-verification.json` records the 85 exact runtime
checks. Eighteen focused tests pass, including neutral-enemy golden values,
contact parsing and unchanged-source XLSX reading. No damage formula, runtime
catalog, server seed or database changed.

### Legacy magic defense versus Gonline policy

The user reaffirmed that Gonline deliberately uses **MDEF only** for magical
damage. The Legacy research comparator instead uses **max(DEF, MDEF)**, following
the supplied community investigation. Keep those policies separate when inferring
P rather than treating the Gonline policy as proof of Legacy behavior.

This distinction does not change any current measurement comparison: Straw
Chicken has DEF/MDEF 34/44, Hyperkinetic Chicken 32/42, Rice Chicken 36/44,
Male Bug 72/74 and Female Bug 76/78. MDEF is the greater value in every case.
Selecting max therefore cannot explain the remaining low-MATK mismatch with
the current estimated enemy stats. Their true Legacy stat values and the
defense-correction branch remain separate uncertainties.

Research `predictions` preserves MDEF-only for the current-server model and
selects the Legacy maximum for research candidates. Direct `damage_from_power`
defaults to the current policy; callers must explicitly choose the Legacy policy
when comparing to Legacy. The bug candidate and inferred-P paths do so. A focused
test checks a synthetic target with DEF greater than MDEF and confirms existing
targets with MDEF greater than DEF give identical results. Runtime combat is
unchanged.

## Blast Attack grade 2: MAT 25 baseline

The user supplied a new controlled batch before changing MAT. The screenshot
records Test0001, Fire, level 13, ATK 77, DEF 41, MATK 25, MDEF 44, SPD 27,
STR/CON/INT/WIS/AGI 24/8/2/8/2, HP 448/448 and SP 228/228. The skill is
Blast Attack grade 2 throughout.

| Target | Recorded damage | Mean | Current Gonline estimate | `2(H+DG)`, S omitted |
| --- | --- | ---: | ---: | ---: |
| Irrigated Chicken | 12, 9, 11, 9 | 10.25 | 5–9 | 8–12 |
| Damp Snail | 23, 23, 25 | 23.67 | 14–18 | 18–22 |

The repository identifies Irrigated Chicken as 17043, level 12, Water,
estimated DEF/MDEF 33/39, and Damp Snail as 17013, level 9, Earth, estimated
30/35. Both Legacy's max-defense hypothesis and Gonline's MDEF-only policy
select MDEF. Fire's element factors are 0.7 and 1.2 respectively.

Save the authored observations unchanged in
`var/damage/samples/blast-attack-grade2-mat25.json`; the local analysis is
`var/reports/skill-power-low-matk/mat25.json`. This batch is a baseline for the
user's next MAT equipment change. Keep skill grade and enemy identity fixed,
record the new displayed MATK, and compare damage increases rather than
attributing the entire intercept to P.

Under an unchanged 1.4 MATK coefficient, expected damage change per MAT point
before rounding/noise is 0.98 against Irrigated Chicken and 1.68 against Damp
Snail. Including native S=1.1 instead gives 1.078 and 1.848. A larger attainable
MAT change distinguishes these slopes more clearly than a small change.

Conditionally assuming the current enemy estimates, no extra S on MATK, the
upper-branch 0.98 defense correction, floor rounding and integer noise 1–5,
all seven observations admit a shared P in [15.9667, 16.0771). This is a
compatibility interval under those assumptions, not a recovered Legacy power
or proof of its noise distribution. The next MAT batch is needed to separate
stat-slope/branch errors from the additive intercept.

## Blast Attack: MAT 29 and grades 2–3

The next screenshot records the same Fire character Test0001, level 13, with
MATK **29**, ATK **74**, DEF 41, MDEF **48** and SPD **24**. The prior equipment
state was MATK 25 / ATK 77 / MDEF 44 / SPD 27. All permanent attributes and level
remain the same. Preserve these changes rather than describing the equipment
swap as changing only MATK. The snapshot is taken as applying to both reported
grades in this batch.

The 17 authored observations are saved in
`var/damage/samples/blast-attack-grades2-3-mat29.json`; the preceding seven remain
in their original MAT 25 file. No measurement is overwritten or averaged away.

| MATK | Grade | Irrigated Chicken values | Mean | Damp Snail values | Mean |
| ---: | ---: | --- | ---: | --- | ---: |
| 25 | 2 | 12, 9, 11, 9 | 10.25 | 23, 23, 25 | 23.67 |
| 29 | 2 | 14, 15, 16, 14 | 14.75 | 29, 32, 29 | 30.00 |
| 29 | 3 | 17, 19, 16, 16 | 17.00 | 32, 34, 33, 33, 35, 35 | 33.67 |

### Grade growth is better constrained than the base/intercept

At MATK 29, the same-target grade 2→3 changes are +2.25 damage for Irrigated
Chicken and +3.6667 for Damp Snail. Dividing by the assumed element factors 0.7
and 1.2 gives +3.2143 and +3.0556 additive power, approximately **3.1 P per grade**.
This cancels a constant enemy-defense and fixed-power contribution, though the
small cast counts, rounding and random samples still limit precision.

Raw Blast Attack D is 1.38. Its current one-contact rule adds only 1.38 P per
grade. The simple magic `2(H+DG)` candidate adds 2.76. The earlier frozen
structured P goes from 9.08556 at grade 2 to 12.18786 at grade 3, an increase
of **3.10229**, closely matching the new differences without refitting.

For the frozen structured P with S omitted from MATK, predicted midpoint damage
at MATK 29 goes 11→13 against Irrigated Chicken and 21→25 against Damp Snail.
Recorded means go 14.75→17 and 30→33.67. The grade-dependent changes are close,
while each target retains a broadly similar absolute offset at both grades.
This points toward the additive intercept/defense correction as a remaining
issue; it does not prove that P alone is wrong, because actual Legacy defenses
and its low-offense branch remain unknown.

### MAT change remains too small to identify the exact coefficient

At grade 2, increasing displayed MATK by four raises mean damage +4.50 against
Irrigated Chicken and +6.3333 against Damp Snail. A 1.4 coefficient predicts
+3.92 and +6.72 before rounding/noise. Including S=1.1 predicts +4.312 and
+7.392. Differences between those hypotheses are only 0.392 and 0.672 damage,
smaller than the observed within-group noise; this batch alone cannot reliably
select one coefficient. Unrecorded equipment-specific modifiers are another
possible confound in any gear-based test.

A further MAT change at fixed grade 3, ideally with a substantially larger
attainable MAT separation, is more useful for the stat slope than another
four-point change. Continue using the same two enemy templates and record the
displayed MAT and any other gear effects. Further grades at unchanged equipment
would refine grade progression independently.

```sh
python tools/skill-power-analysis/low_matk_compare.py
python -m unittest discover -s tools/skill-power-analysis -p 'test_*.py'
```

`var/reports/skill-power-low-matk/controlled-batches.json` and
`controlled-batches.html` retain all 24 observations, source hashes, current
and research predictions and the controlled mean changes. New focused tests
exclude mixed MAT-and-grade changes and cross-character comparisons from
isolated comparisons. No gameplay formula or database changed.

## Returning to MAT 25 at grade 3

The user supplied 11 additional grade-3 attacks after returning to MATK 25.
The screenshot confirms Test0001 is still Fire / level 13, now ATK 74, DEF 41,
MATK 25, MDEF 44 and SPD 24. HP/SP are 397/448 and 162/228; permanent
attributes remain 24/8/2/8/2. The data are preserved in
`var/damage/samples/blast-attack-grade3-mat25.json`.

The complete local experiment now has **35 casts** and all four MAT/grade
settings for both targets:

| MATK | Grade | Irrigated Chicken mean (casts) | Damp Snail mean (casts) |
| ---: | ---: | ---: | ---: |
| 25 | 2 | 10.2500 (4) | 23.6667 (3) |
| 29 | 2 | 14.7500 (4) | 30.0000 (3) |
| 25 | 3 | 12.8333 (6) | 26.0000 (5) |
| 29 | 3 | 17.0000 (4) | 33.6667 (6) |

The new grade-3 observations are Chicken 14,12,13,14,13,11 and Snail
28,27,25,24,26. Comparing MAT 29→25 at fixed grade 3 lowers means by 4.1667
and 7.6667. Dividing by four MAT points and the assumed elemental factors
gives local coefficient estimates 1.4881 and 1.5972. ATK is 74 in both of
these grade-3 setups, making this pair cleaner than the original gear swap;
the actor's MDEF changes 48→44, and possible equipment-specific effects are
still not independently measured.

### Joint grade/MAT estimate without assumed absolute enemy defenses

Fit the raw cast damages, not just the group means, with ordinary least squares:

```text
damage = target_intercept
       + element_factor * a * (MATK - 25)
       + element_factor * b * (grade - 2)
       + residual
```

Each target gets its own free intercept. Thus this local slope fit does not
assume either monster's DEF/MDEF or absolute P. It still assumes the given
element factors, locally additive/linear behavior, no separate gear damage
effect and independent equal-variance residuals. Rounding/noise are included
in the residual; standard errors are conditional on those modeling assumptions.

| Parameter | Estimate | Ordinary least-squares standard error |
| --- | ---: | ---: |
| MATK coefficient a, before element | 1.50354 | 0.11213 |
| Additional P per grade b | **2.78098** | 0.46298 |

There are 35 observations, four fitted parameters and 31 residual degrees of
freedom. Residual standard deviation is 1.29039 damage. These are raw-cast
sampling estimates, not proof of the original server constants. In particular,
both MAT coefficients 1.4 and 1.54 remain compatible with this narrow four-point
MAT separation; rounding 1.50354 to a claimed native constant 1.5 is unwarranted.

The joint grade-growth estimate refines the earlier incomplete-batch estimate
of roughly 3.1. It closely matches **2D = 2*1.38 = 2.76 P per grade** for Blast
Attack, consistent with the previously considered magic `2(H+DG)` candidate.
This supports its grade coefficient locally; it does not establish its fixed
base term, remove the low-MATK intercept discrepancy, or prove every magic skill
uses the same curve. Additional grades at fixed equipment would test whether
that local slope continues.

The controlled-batch report now retains all 35 observations and the joint-fit
coefficients/standard errors. Focused tests recover known synthetic slopes
with independent target intercepts and reject an underidentified fit. Runtime
damage and source observations remain unchanged.

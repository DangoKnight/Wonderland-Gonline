"""Test higher base power and lower grade growth against measured attacks.

Offline only. Raw ODS observations and authored power tables remain unchanged.
The key comparison holds ATK treatment fixed and treats P as a cast's total.
"""
import argparse
import hashlib
import html
import json
from pathlib import Path
import statistics

import analyze
import area_search
import raw_search as raw
import sample_compare as sample
import structured_search as structured

PHYSICAL_LAYER = 1
BASELINE_COEFFICIENTS = (1., 1.)
HIGHER_BASE_LOWER_GRADE = (2., .5)
POWER_NOISE_MIDPOINT = statistics.mean(sample.RANDOM_ROLLS)


def estimate(row, skills, coefficients, native_stat=False):
    skill = skills[row['skill']]
    a, b = coefficients
    power = a*skill['value_additional_damage'] + b*skill['value_power_per_level']*row['grade']
    return sample.damage_from_power(row['actor'], row['enemy'], skill, power,
                                    skill['value_stat_multiplier'] if native_stat else 1)


def damage_metrics(rows, predictions):
    errors = [abs(p['midpoint']/r['observed']-1)*100 for r, p in zip(rows, predictions)]
    return dict(n=len(rows), median_percent_error=statistics.median(errors),
                mean_percent_error=statistics.mean(errors),
                within_range=sum(p['minimum'] <= r['observed'] <= p['maximum'] for r, p in zip(rows, predictions)))


def fit_inputs(groups, skills, native_stat=False):
    """Each sheet/skill/grade/target group receives one weight, not each repeat."""
    result = []
    for row in groups:
        skill = skills[row['skill']]
        factor = row['predicted']['current server']['element_factor']
        multiplier = skill['value_stat_multiplier'] if native_stat else 1
        # Reverse only the assumed pre-rounding physical pipeline. The noise
        # midpoint is an estimate; the original Legacy random law is unknown.
        implied = (row['observed']-POWER_NOISE_MIDPOINT)/factor
        implied -= sample.SINGLE_COEFFICIENT*row['actor']['atk']*multiplier
        implied += sample.DEFENSE_COEFFICIENT*row['enemy']['defense']
        if implied <= 0:
            raise ValueError('Nonpositive inferred power cannot use relative-power fitting')
        result.append(dict(id=row['skill_id'], reference=implied,
                           features=[skill['value_additional_damage'], skill['value_power_per_level']*row['grade']]))
    return result


def fit(groups, skills, native_stat=False):
    return structured.positive_fit(*raw.normal(fit_inputs(groups, skills, native_stat), [0, 1]))


def leave_skill_out(rows, groups, skills, native_stat=False):
    predictions, held, folds = [], [], []
    for skill_id in sorted({r['skill_id'] for r in rows}):
        train = [r for r in groups if r['skill_id'] != skill_id]
        testing = [r for r in rows if r['skill_id'] == skill_id]
        coefficients = fit(train, skills, native_stat)
        values = [estimate(r, skills, coefficients, native_stat) for r in testing]
        held.extend(testing)
        predictions.extend(values)
        folds.append(dict(skill_id=skill_id, skill=testing[0]['skill'], coefficients=coefficients,
                          metrics=damage_metrics(testing, values)))
    return dict(metrics=damage_metrics(held, predictions), folds=folds)


def investigate(comparison):
    skills = comparison['skills']
    rows = [r for r in comparison['rows'] if 'uncertain_target' not in r
            and skills[r['skill']]['value_effect_layer'] == PHYSICAL_LAYER]
    groups = []
    for group in comparison['groups']:
        if skills[group['skill']]['value_effect_layer'] != PHYSICAL_LAYER:
            continue
        first = next(r for r in rows if (r['sheet'], r['skill'], r['grade'], r['target']) ==
                     (group['sheet'], group['skill'], group['grade'], group['target']))
        groups.append(dict(first, observed=group['mean']))
    selected = fit(groups, skills)
    alternatives = {
        'H+D*G total, S omitted from ATK': (BASELINE_COEFFICIENTS, False),
        '2H+.5D*G total, S omitted from ATK': (HIGHER_BASE_LOWER_GRADE, False),
        '2H+.5D*G total, native S on ATK': (HIGHER_BASE_LOWER_GRADE, True),
        'fitted aH+bD*G total, S omitted from ATK': (selected, False),
    }
    metrics = {name: damage_metrics(rows, [estimate(r, skills, cs, native) for r in rows])
               for name, (cs, native) in alternatives.items()}
    metrics['current server'] = comparison['by_layer']['physical']['current server']
    metrics['previous frozen structured P, S omitted from ATK'] = comparison['by_layer']['physical'][sample.FORMULAS[3]]
    references, _, _, provenance = raw.raw_data()
    provenance['target_references'] = area_search.reference_targets(references)
    single = [r for r in references if r['layer'] == PHYSICAL_LAYER and r['targets'] == 1]
    ref_metrics = {label: analyze.metrics(single, [a*r['a']+b*r['b']*r['grade'] for r in single])
                   for label, (a, b) in [('H+D*G', BASELINE_COEFFICIENTS), ('2H+.5D*G', HIGHER_BASE_LOWER_GRADE),
                                         ('sample-fitted aH+bD*G', selected)]}
    details = []
    for row in groups:
        details.append(dict(sheet=row['sheet'], skill=row['skill'], grade=row['grade'], target=row['target'],
                            observed_mean=row['observed'],
                            predictions={name: estimate(row, skills, cs, native) for name, (cs, native) in alternatives.items()}))
    return dict(sample_provenance=comparison['provenance'], reference_provenance=provenance,
                sample_physical_casts=len(rows), sample_physical_groups=len(groups),
                sample_physical_skills=len({r['skill_id'] for r in rows}),
                grouped_fit_coefficients=selected, sample_metrics=metrics,
                leave_skill_out=leave_skill_out(rows, groups, skills),
                max_grade_reference_metrics=ref_metrics, groups=details)


def write_report(result, output):
    output.mkdir(parents=True, exist_ok=True)
    (output/'analysis.json').write_text(json.dumps(result, ensure_ascii=False, indent=2)+'\n', encoding='utf-8')
    parts = ['<!doctype html><meta charset="utf-8"><title>Base power and grade growth</title>',
             '<style>body{font:15px system-ui;margin:24px;max-width:1300px}table{border-collapse:collapse}td,th{border:1px solid #aaa;padding:8px}pre{white-space:pre-wrap}</style>',
             '<h1>Higher base power and lower grade growth</h1>',
             '<p>Physical damage only. All candidate P values are per-cast totals. The key comparison keeps the extra native S multiplier off ATK in both models. Enemy defenses, elemental factors and random bounds are current Gonline estimates. The uncertain 203-target observation is excluded.</p>',
             '<p>The simple candidate is P = 2H + 0.5DG. Its shape was examined in earlier research; this remains a repeatedly studied sample. The fitted alternative uses only 16 groups from five skills; leave-skill-out checks refit without that entire skill. Magic and area skills are not established by this test.</p>',
             '<table><tr><th>Sheet</th><th>Skill / grade</th><th>Target</th><th>Legacy mean</th><th>H+DG</th><th>2H+.5DG</th></tr>']
    for r in result['groups']:
        values = [r['sheet'], f'{r["skill"]} {r["grade"]}', r['target'], round(r['observed_mean'], 3)]
        for key in ('H+D*G total, S omitted from ATK', '2H+.5D*G total, S omitted from ATK'):
            p = r['predictions'][key]
            values.append(f'{p["minimum"]}–{p["maximum"]}')
        parts.append('<tr>'+''.join('<td>'+html.escape(str(v))+'</td>' for v in values)+'</tr>')
    parts.extend(['</table><h2>Methods and metrics</h2><pre>',
                  html.escape(json.dumps({k: v for k, v in result.items() if k != 'groups'}, indent=2)), '</pre>'])
    (output/'report.html').write_text('\n'.join(parts), encoding='utf-8')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--sample', type=Path, default=Path('var/damage/samples/sample1.ods'))
    parser.add_argument('--star-grade', type=int, required=True)
    parser.add_argument('--output', type=Path, default=Path('var/reports/skill-power-grade'))
    args = parser.parse_args()
    comparison = sample.compare(sample.read_ods(args.sample), args.star_grade)
    comparison['provenance'] = dict(sample=str(args.sample), sample_sha256=hashlib.sha256(args.sample.read_bytes()).hexdigest(),
                                    seed_sha256=hashlib.sha256(sample.SERVER_SEED.read_bytes()).hexdigest(), star_grade=args.star_grade)
    result = investigate(comparison)
    write_report(result, args.output)
    print(json.dumps({k: result[k] for k in ('grouped_fit_coefficients', 'sample_metrics', 'max_grade_reference_metrics')}, indent=2))
    print(json.dumps(dict(leave_skill_out=result['leave_skill_out']['metrics'])))


if __name__ == '__main__':
    main()

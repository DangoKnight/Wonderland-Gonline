"""Prospective MAT238 validation using frozen, previously fitted damage models.

The 'current' labels here preserve the pre-correction baseline. For the later
deployed bounded correction, use selected_damage_compare.py instead.

Never refits models or changes P/gameplay. Critical/uncertain casts are retained
separately, with individual contact values preserved. Run from repository root.
"""
import argparse
import collections
import hashlib
import html
import json
import math
from pathlib import Path
import statistics

import damage_polynomial as damage
import polynomial_search as power
import sample_compare as sample

NATIVE_SINGLE_PATTERN = 1
NATIVE_TEAM_PATTERN = 7
NATIVE_ALL_PATTERN = 8
SIDE_CAPACITY = 8
DEFAULT_MAXIMUM_GRADE = 10
NATIVE_PATTERN_FIRST_OFFSET = 109
NATIVE_PATTERN_GRADE_BAND = 3
NATIVE_PATTERN_LAST_BAND = 3
NATIVE_LARGEST_ORDINARY_PATTERN = 6


def capacity(skill, grade):
    if skill['value_targeting_present']:
        raise ValueError('Explicit SQL shapes require full shape loading before prediction')
    if skill['value_area_attack_present'] and not skill['value_area_attack']:
        return 1
    band = min(NATIVE_PATTERN_LAST_BAND, max(0, (grade-1)//NATIVE_PATTERN_GRADE_BAND))
    pattern = next((skill[f'value_native_pattern{NATIVE_PATTERN_FIRST_OFFSET+i}'] for i in range(band, -1, -1)
                    if skill[f'value_native_pattern{NATIVE_PATTERN_FIRST_OFFSET+i}']), NATIVE_SINGLE_PATTERN)
    return pattern if NATIVE_SINGLE_PATTERN <= pattern <= NATIVE_LARGEST_ORDINARY_PATTERN else SIDE_CAPACITY if pattern in (NATIVE_TEAM_PATTERN, NATIVE_ALL_PATTERN) else 1


def current_prediction(actor, enemy, skill, grade, critical=False):
    maximum = skill['value_maximum_grade'] if skill['value_maximum_grade_present'] else DEFAULT_MAXIMUM_GRADE
    grade = min(maximum, max(1, grade))
    n = capacity(skill, grade)
    magical = skill['value_effect_layer'] == power.MAGICAL_LAYER
    offense, defense = (actor['matk'], enemy['magic_defense']) if magical else (actor['atk'], enemy['defense'])
    coefficient = .4 if n >= 6 else .6 if n >= 3 else .85 if n == 2 else 1.4
    correction = .98
    if not magical and n == 1 and offense < defense:
        coefficient, correction = .599, .179
    row = dict(H=skill['value_additional_damage'], Z=skill['value_power_per_level']*grade, layer=skill['value_effect_layer'])
    p = power.baseline(row)*2/(n+1)
    element = skill['value_element'] or actor['element']
    if element is None:
        raise ValueError('Actor element is required for a skill without an explicit element')
    factor = sample.ELEMENTAL_FACTORS[element][enemy['element']]
    base = max(1, math.floor((coefficient*offense+p-correction*defense)*factor+sample.ROUNDING_TOLERANCE))
    # This is only Gonline's ordinary 2x critical hypothesis, not a fitted
    # critical rule. Critical observations never enter the normal-error score.
    outcomes = [min(damage.MAX_DAMAGE, base*(2 if critical else 1)+roll) for roll in sample.RANDOM_ROLLS]
    return dict(minimum=min(outcomes), maximum=max(outcomes), midpoint=statistics.mean(outcomes),
                P=p, capacity=n, element_factor=factor)


def analyze(source, frozen):
    content, models_content = source.read_bytes(), frozen.read_bytes()
    raw, previous = json.loads(content), json.loads(models_content)
    skills, npcs = sample.seed_definitions()
    actor = raw['actor']
    selected = ('current', 'direct:1', 'direct:1+constant', 'correction:2+constant', 'correction:3+constant', 'hinge:1', 'hinge:2')
    models = {c['name']: c['model'] for c in previous['layers']['magical']['fits'] if c['name'] in selected}
    groups, excluded = collections.defaultdict(list), []
    for observation in raw['observations']:
        if 'hits' in observation and sum(observation['hits']) != observation['damage']:
            raise ValueError('Contact total differs from reported damage')
        if observation.get('critical') is not False or observation.get('context_uncertain'):
            sk = skills[observation['skill']]
            en = sample.enemy_stats(npcs[observation['target']])
            excluded.append(dict(observation, current_normal=current_prediction(actor,en,sk,observation['grade']),
                                 current_double_critical_hypothesis=current_prediction(actor,en,sk,observation['grade'],True)))
            continue
        groups[observation['skill'], observation['grade'], observation['target']].append(observation['damage'])
    rows = []
    for (name, grade, target), observed in groups.items():
        sk, en = skills[name], sample.enemy_stats(npcs[target])
        current = current_prediction(actor, en, sk, grade)
        if sk['value_effect_layer'] != power.MAGICAL_LAYER or current['capacity'] != 1:
            raise ValueError('Frozen stat-polynomial validation is limited to single-target magic')
        row = dict(O=actor['matk'], B=en['magic_defense'], P=current['P'], E=current['element_factor'],
                   layer=sk['value_effect_layer'], observed=statistics.mean(observed))
        predictions = {label: damage.predict(row, model) for label, model in models.items()}
        if predictions['current']['minimum'] != current['minimum']:
            raise AssertionError('Frozen baseline does not match runtime-formula prediction')
        rows.append(dict(row, skill=name, grade=grade, target=target, skill_id=sk['value_id'],
                         observed_values=observed, casts=len(observed), predictions=predictions, enemy=en))
    metrics = {name: damage.metrics(rows, [r['predictions'][name] for r in rows]) for name in models}
    return dict(source_sha256=hashlib.sha256(content).hexdigest(), frozen_models_sha256=hashlib.sha256(models_content).hexdigest(),
                actor=actor, noncritical_casts=sum(r['casts'] for r in rows), groups=rows,
                frozen_model_metrics=metrics, excluded=excluded,
                note='No new observations were used for fitting or model selection. Errors average condition means. Five Star critical/target context is unconfirmed.')


def write_report(result, output):
    output.mkdir(parents=True, exist_ok=True)
    (output/'analysis.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    labels = list(result['frozen_model_metrics'])
    parts = ['<!doctype html><meta charset="utf-8"><title>MAT238 prospective damage validation</title><style>body{font:15px system-ui;margin:24px}td,th{border:1px solid #bbb;padding:8px}table{border-collapse:collapse}pre{white-space:pre-wrap}</style><h1>MAT238 prospective damage validation</h1><p>'+html.escape(result['note'])+'</p><table><tr><th>Target</th><th>Observed mean</th>'+''.join('<th>'+html.escape(label)+'</th>' for label in labels)+'</tr>']
    for row in result['groups']:
        cells = [row['target'],round(row['observed'],3)]
        cells.extend(f"{row['predictions'][label]['minimum']}–{row['predictions'][label]['maximum']}" for label in labels)
        parts.append('<tr>'+''.join('<td>'+html.escape(str(v))+'</td>' for v in cells)+'</tr>')
    parts.append('</table><h2>Frozen-model errors</h2><table><tr><th>Model</th><th>Mean % error</th></tr>')
    for label, metric in result['frozen_model_metrics'].items():
        parts.append('<tr><td>'+html.escape(label)+'</td><td>'+str(round(metric['mean_percent_error'],3))+'</td></tr>')
    parts.append('</table><h2>Critical or uncertain observations</h2><pre>'+html.escape(json.dumps(result['excluded'],ensure_ascii=False,indent=2))+'</pre>')
    (output/'report.html').write_text('\n'.join(parts),encoding='utf-8')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--sample',type=Path,default=Path('var/damage/samples/dangochaos-mat238.json'))
    parser.add_argument('--models',type=Path,default=Path('var/reports/damage-polynomial/analysis.json'))
    parser.add_argument('--output',type=Path,default=Path('var/reports/damage-mat238'))
    args = parser.parse_args()
    result = analyze(args.sample,args.models)
    write_report(result,args.output)
    for row in result['groups']:
        print(row['target'],round(row['observed'],3),row['predictions'])
    print('ERRORS', {label:round(m['mean_percent_error'],3) for label,m in result['frozen_model_metrics'].items()})
    print('UNCERTAIN',[(r['skill'],r['target'],r['damage']) for r in result['excluded']])


if __name__ == '__main__':
    main()

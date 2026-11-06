"""Evaluate saved community formulas without fitting P or editing measurements.

The 91 normal casts are the preserved input to the selected-damage replay.
Source examples are reported separately: their defenses and reference powers
have different provenance, so they must not silently become player samples.
"""
import argparse
import hashlib
import html
import json
import math
from pathlib import Path
import re
import statistics

import sample_compare as sample
import selected_damage_compare as selected

MODELS = ('current', 'pre_stat_floor', 'linear', 'magic_below_defense', 'magic_below_with_q',
          'defense_coefficient_one', 'older_average_defense', 'legacy_max_defense')
NATIVE_NEUTRAL_ELEMENT = 0
NATIVE_CASTER_ELEMENT = 7
EARTH_ELEMENT = 1


def prediction(row, skill, model):
    """Keep the selected native P and vary only documented stat/defense rules."""
    if model not in MODELS:
        raise ValueError(model)
    magical = skill['value_effect_layer'] == 2
    attack = row['actor']['matk' if magical else 'atk']
    defense = row['enemy']['magic_defense' if magical else 'defense']
    grade = min(skill['value_maximum_grade'] if skill['value_maximum_grade_present'] else 10,
                max(1, row['grade']))
    fixed, growth = skill['value_additional_damage'], skill['value_power_per_level']*grade
    power = 2*(fixed+growth) if magical else 1.9131446584781497*fixed+.47524025502148415*growth
    coefficient, reduction = 1.4, .98
    if magical and model == 'older_average_defense':
        defense = (row['enemy']['defense']+defense)/2
    if magical and model == 'legacy_max_defense':
        defense = max(row['enemy']['defense'], defense)
    if attack < defense and (not magical or model in ('magic_below_defense', 'magic_below_with_q')):
        coefficient, reduction = .599, .179
    elif model in ('defense_coefficient_one', 'older_average_defense'):
        reduction = 1
    base = coefficient*attack+power-reduction*defense
    if magical and model in ('current', 'pre_stat_floor', 'magic_below_with_q'):
        base += selected.adjustment(attack, defense)
    if magical and model == 'current':
        base = max(0, base-power)+power
    # Skill.dat zero is a neutral element; these rows are offensive skills,
    # not the basic attacks for which actor affinity is the explicit exception.
    element = skill['value_element']
    if element == NATIVE_CASTER_ELEMENT:
        element = row['actor']['element']
    factor = sample.ELEMENTAL_FACTORS[element][row['enemy']['element']]
    if element == NATIVE_NEUTRAL_ELEMENT and row['enemy']['element'] == EARTH_ELEMENT:
        factor = .9
    start = max(1, math.floor(base*factor+1e-9))
    return dict(minimum=start+1, maximum=start+5, midpoint=start+3, power=power)


def metrics(rows, model):
    errors = [abs(row['predictions'][model]['midpoint']/row['observed']-1)*100 for row in rows]
    return dict(casts=len(rows), mean_percent_error=statistics.mean(errors),
                mean_absolute_damage_error=statistics.mean(abs(row['predictions'][model]['midpoint']-row['observed']) for row in rows),
                within_range=sum(row['predictions'][model]['minimum'] <= row['observed'] <= row['predictions'][model]['maximum'] for row in rows))


def source_checks():
    # Independently reported non-priest rows from the 2022 single-target post.
    # IDs below are that article's row numbers, not game content IDs. P is the
    # article's claimed max-grade power, not our native P approximation.
    rows = [
        dict(row=8, layer='physical', attack=2322, power=0, defense=406, mdef=561, element=1.6, observed=4567.5),
        dict(row=12, layer='magical', attack=2346, power=440, defense=977, mdef=774, element=.9, observed=2493.2),
        dict(row=13, layer='magical', attack=2346, power=440, defense=977, mdef=830, element=.9, observed=2492.6),
        dict(row=14, layer='magical', attack=2346, power=440, defense=330, mdef=360, element=.9, observed=3036.5),
        dict(row=15, layer='magical', attack=2346, power=440, defense=408, mdef=561, element=.9, observed=2860.25),
        dict(row=17, layer='magical', attack=1429, power=50, defense=408, mdef=561, element=.6, observed=903),
    ]
    for row in rows:
        magic = row['layer'] == 'magical'
        defenses = {'legacy_max': max(row['defense'], row['mdef']) if magic else row['defense'],
                    'project_mdef': row['mdef'] if magic else row['defense']}
        row['predictions'] = {name: (1.4*row['attack']+row['power']-.98*defense)*row['element']+3
                              for name, defense in defenses.items()}
        clamped_attack = min(selected.ATTACK_BOUNDS[1], max(selected.ATTACK_BOUNDS[0], row['attack']))
        clamped_defense = min(selected.DEFENSE_BOUNDS[1], max(selected.DEFENSE_BOUNDS[0], row['mdef']))
        row['predictions']['previous_clamped_q'] = row['predictions']['project_mdef'] + (
            selected.adjustment(clamped_attack, clamped_defense)*row['element'] if magic else 0)
        row['predictions']['refined_fading_q'] = row['predictions']['project_mdef'] + (
            selected.adjustment(row['attack'], row['mdef'])*row['element'] if magic else 0)
    return rows


def references(directory):
    result = []
    for path in sorted(directory.glob('*.htm')):
        raw = path.read_bytes()
        text = raw.decode('utf-8')
        url = re.search(r'<meta[^>]*property=["\']og:url["\'][^>]*content=["\']([^"\']+)', text)
        result.append(dict(file=path.name, sha256=hashlib.sha256(raw).hexdigest(),
                           url=html.unescape(url.group(1)) if url else None))
    return result


def analyze(input_path, reference_dir):
    raw = input_path.read_bytes()
    document = json.loads(raw)
    rows = document['rows']
    for row in rows:
        skill = document['skills'][row['skill']]
        row['predictions'] = {model: prediction(row, skill, model) for model in MODELS}
        # Reject drift from the independently replayed current calculation.
        expected = row['selected']
        if any(row['predictions']['current'][key] != expected[key] for key in ('minimum', 'maximum', 'midpoint')):
            raise ValueError(f"Current replay differs: {row['skill']} {row['cell']}")
    subsets = {'all': rows, 'physical': [r for r in rows if r['layer'] == 1],
               'magical': [r for r in rows if r['layer'] == 2],
               'controlled MAT25/29': [r for r in rows if r.get('source') == 'controlled'],
               'MAT238': [r for r in rows if r['actor']['matk'] == 238]}
    return dict(input_sha256=hashlib.sha256(raw).hexdigest(), references=references(reference_dir),
                summary={label: {model: metrics(subset, model) for model in MODELS} for label, subset in subsets.items()},
                source_examples=source_checks(), rows=rows,
                limitations=['No new coefficients fitted; P remains fixed. All 91 known normal casts retained.',
                             'Critical hits, uncertain Five Star Beating casts and ambiguous targets stay excluded.',
                             'Target stats in the player dataset are repository estimates, not captured Legacy defenses.',
                             'Average/max DEF policies are offline comparisons only; Gonline keeps MDEF-only magic.',
                             'Source examples retain their own supplied P and defenses; they are not mixed into the 91-cast scores.',
                             'Sources disagree on area coefficients and do not establish a universal grade formula.'])


def write_report(result, directory):
    directory.mkdir(parents=True, exist_ok=True)
    (directory/'comparison.json').write_text(json.dumps(result, ensure_ascii=False, indent=2)+'\n', encoding='utf-8')
    lines = ['<!doctype html><meta charset="utf-8"><title>Damage reference audit</title>',
             '<style>body{font:15px system-ui;margin:24px;max-width:1500px}table{border-collapse:collapse}td,th{border:1px solid #bbb;padding:8px}pre{white-space:pre-wrap}</style>',
             '<h1>Damage reference audit</h1><p>Fixed P; no fitting. Mean absolute percentage error against recorded individual casts.</p>',
             '<ul>'+''.join('<li>'+html.escape(s)+'</li>' for s in result['limitations'])+'</ul>',
             '<table><tr><th>Measurements</th>'+''.join('<th>'+html.escape(model)+'</th>' for model in MODELS)+'</tr>']
    for label, models in result['summary'].items():
        lines.append('<tr><th>'+html.escape(label)+'</th>'+''.join(f'<td>{models[model]["mean_percent_error"]:.3f}%</td>' for model in MODELS)+'</tr>')
    lines.append('</table><h2>Independent article examples</h2><p>Continuous prediction plus mean noise 3. Article powers retained; no attempt to fit P.</p><pre>'+html.escape(json.dumps(result['source_examples'], indent=2))+'</pre>')
    lines.append('<h2>Saved sources</h2><pre>'+html.escape(json.dumps(result['references'], ensure_ascii=False, indent=2))+'</pre>')
    (directory/'report.html').write_text('\n'.join(lines), encoding='utf-8')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--input', type=Path, default=Path('var/reports/damage-selected/runtime-input.json'))
    parser.add_argument('--references', type=Path, default=Path('var/damage/references'))
    parser.add_argument('--output', type=Path, default=Path('var/reports/damage-references'))
    args = parser.parse_args()
    result = analyze(args.input, args.references)
    write_report(result, args.output)
    for label, models in result['summary'].items():
        print(label, {model: round(models[model]['mean_percent_error'], 3) for model in MODELS})
    for row in result['source_examples']:
        print(row['row'], row['observed'], row['predictions'])


if __name__ == '__main__':
    main()

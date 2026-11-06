"""Polynomial damage investigation with the deployed P calculation held fixed.

The 'current' labels preserve the baseline before the selected stat correction.
See selected_damage_compare.py for the later deployment comparison.

Only the stat-dependent remainder is fitted. No gameplay or authored data writes.
The elemental matrix, MDEF-only magic, integer flooring and noise bounds remain
fixed. Inputs are the previously captured, standalone noncritical measurements.
"""
import argparse
import collections
import hashlib
import html
import itertools
import json
import math
from pathlib import Path
import statistics

import polynomial_search as power
import raw_search
import sample_compare as sample

RIDGES = (1e-8, .001, .03, .3, 3.)
NOISE_MEAN = statistics.mean(sample.RANDOM_ROLLS)
MAX_DAMAGE = 2**31-1
CONFIGURATIONS = [('current', 0, False)] + [
    (kind, degree, intercept)
    for kind in ('direct', 'correction', 'low-correction')
    for degree in range(1, 5) for intercept in (False, True)
] + [('hinge', degree, False) for degree in range(1, 5)]
SKILL_SPLIT = 'skill'
TARGET_SPLIT = 'target'
OFFENSE_SPLIT = 'offense'


def load_rows(reports):
    rows, provenance = power.live_rows(reports)
    for r in rows:
        source, skill = r['observation'], r['skill']
        magical = r['layer'] == power.MAGICAL_LAYER
        r['O'] = source['actor']['matk' if magical else 'atk']
        r['B'] = source['enemy']['magic_defense' if magical else 'defense']
        r['P'] = power.baseline(r)
        r['E'] = sample.ELEMENTAL_FACTORS[skill['value_element'] or source['actor']['element']][source['enemy']['element']]
        r['target'] = source['enemy']['id']
        r['skill_name'] = source['skill']
        r['target_name'] = source['enemy']['name']
    provenance['runtime_damage_sha256'] = hashlib.sha256(Path('internal/battle/damage.go').read_bytes()).hexdigest()
    return rows, provenance


def current_stat(row):
    if row['layer'] != power.MAGICAL_LAYER and row['O'] < row['B']:
        return sample.LOW_PHYSICAL_COEFFICIENT*row['O']-sample.HIGH_DEFENSE_COEFFICIENT*row['B']
    return sample.SINGLE_COEFFICIENT*row['O']-sample.DEFENSE_COEFFICIENT*row['B']


def key(config):
    kind, degree, intercept = config
    return kind if kind == 'current' else f'{kind}:{degree}'+('+constant' if intercept else '')


def terms_for(config):
    kind, degree, intercept = config
    if kind == 'current':
        return []
    if kind == 'hinge':
        return [('L',)*size for size in range(1, degree+1)]
    return ([()] if intercept else []) + [term for size in range(1, degree+1)
            for term in itertools.combinations_with_replacement(('O', 'B'), size)]


def features(row, config, terms):
    kind = config[0]
    if kind == 'low-correction' and row['O'] >= row['B']:
        return [0.]*len(terms)
    values = dict(row, L=min(0, row['O']-row['B']))
    return [math.prod(values[name] for name in term) for term in terms]


def offset(row, config):
    return 0. if config[0] == 'direct' else current_stat(row)


def fit_path(rows, config):
    terms = terms_for(config)
    if not terms:
        return [dict(config=config, ridge=0, terms=[], coefficients=[])]
    # Fit relative final-damage residuals, not relative stat remainders: the
    # latter may be negative/zero and would distort weighting when P is large.
    matrix_rows = [[row['E']*v/row['observed'] for v in features(row, config, terms)] for row in rows]
    target = [(row['observed']-NOISE_MEAN-row['E']*(row['P']+offset(row, config)))/row['observed'] for row in rows]
    scales = [math.sqrt(statistics.mean(v[i]**2 for v in matrix_rows)) or 1 for i in range(len(terms))]
    x = [[v/s for v, s in zip(values, scales)] for values in matrix_rows]
    counts = collections.Counter(r['id'] for r in rows)
    weights = [1/counts[r['id']] for r in rows]
    mass = sum(weights)
    matrix = [[sum(w*v[i]*v[j] for w, v in zip(weights, x))/mass for j in range(len(terms))] for i in range(len(terms))]
    rhs = [sum(w*v[i]*y for w, v, y in zip(weights, x, target))/mass for i in range(len(terms))]
    models = []
    for ridge in RIDGES:
        regularized = [line[:] for line in matrix]
        for i in range(len(terms)):
            regularized[i][i] += ridge
        coefficients = raw_search.solve(regularized, rhs)
        if coefficients is None:
            raise ValueError('Singular damage polynomial')
        models.append(dict(config=config, ridge=ridge, terms=terms,
                           coefficients=[c/s for c, s in zip(coefficients, scales)]))
    return models


def stat_prediction(row, model):
    return offset(row, model['config']) + sum(c*v for c, v in zip(model['coefficients'], features(row, model['config'], model['terms'])))


def predict(row, model):
    value = (row['P']+stat_prediction(row, model))*row['E']
    base = max(1, min(MAX_DAMAGE, math.floor(value+sample.ROUNDING_TOLERANCE)))
    outcomes = [min(MAX_DAMAGE, base+noise) for noise in sample.RANDOM_ROLLS]
    return dict(minimum=min(outcomes), maximum=max(outcomes), midpoint=statistics.mean(outcomes))


def metrics(rows, predictions):
    errors = [abs(q['midpoint']/r['observed']-1)*100 for r, q in zip(rows, predictions)]
    return dict(groups=len(rows), mean_percent_error=statistics.mean(errors), median_percent_error=statistics.median(errors),
                mean_absolute_damage_error=statistics.mean(abs(q['midpoint']-r['observed']) for r, q in zip(rows, predictions)),
                observed_mean_within_range=sum(q['minimum']<=r['observed']<=q['maximum'] for r, q in zip(rows, predictions)))


def group_key(row, split):
    return row['id'] if split == SKILL_SPLIT else row['target'] if split == TARGET_SPLIT else row['O']


def folds(rows, split):
    # Leave one entire skill/target/offense value out; no rows of the held group
    # can appear in either fit or hyperparameter selection for that partition.
    for value in sorted({group_key(r, split) for r in rows}):
        train = [r for r in rows if group_key(r, split) != value]
        test = [r for r in rows if group_key(r, split) == value]
        if train and test:
            yield value, train, test


def choose(rows, split, configs=CONFIGURATIONS):
    scores = {}
    for _, train, test in folds(rows, split):
        for config in configs:
            for model in fit_path(train, config):
                scores.setdefault((key(config), model['ridge']), []).extend(
                    abs(predict(r, model)['midpoint']/r['observed']-1)*100 for r in test)
    candidates = []
    for config in configs:
        models = fit_path(rows, config)
        # A single remaining split group cannot identify hyperparameters.
        # Prefer strongest shrinkage, never tune against the held outer group.
        if len({group_key(r, split) for r in rows}) < 2:
            selected = models[-1]
            score = None
        else:
            selected = min(models, key=lambda m: (statistics.mean(scores[key(config), m['ridge']]), -m['ridge']))
            score = statistics.mean(scores[key(config), selected['ridge']])
        candidates.append(dict(model=selected, inner_error=score))
    return sorted(candidates, key=lambda c: (c['inner_error'] if c['inner_error'] is not None else math.inf,
                                           len(c['model']['terms']), key(c['model']['config'])))


def equation(model):
    kind = model['config'][0]
    expression = ' + '.join(f'{c:.12g}*'+('*'.join(t) or '1') for c, t in zip(model['coefficients'], model['terms'])) or '0'
    if kind == 'low-correction':
        expression = 'I(O<B)*('+expression+')'
    return ('F0 + ' if kind != 'direct' else '')+expression


def validation(rows, split):
    held, selected_values, details = [], [], []
    by_family = {key(c): [] for c in CONFIGURATIONS}
    for value, train, test in folds(rows, split):
        choices = choose(train, split)
        held.extend(test)
        for candidate in choices:
            model = candidate['model']
            predictions = [predict(r, model) for r in test]
            by_family[key(model['config'])].extend(predictions)
            if candidate is choices[0]:
                selected_values.extend(predictions)
                details.append(dict(held_group=value, selected=key(model['config']), ridge=model['ridge'],
                                    model=model, metrics=metrics(test, predictions)))
    return dict(selected=metrics(held, selected_values), baseline=metrics(held, [predict(r, dict(config=('current', 0, False), coefficients=[], terms=[])) for r in held]),
                families={name: metrics(held, predictions) for name, predictions in by_family.items()}, folds=details)


def diagnostics(rows, model):
    def inspect(offenses, defenses):
        decreasing_offense = increasing_defense = 0
        for o, b in itertools.product(offenses, defenses):
            row = dict(rows[0], O=o, B=b)
            value = stat_prediction(row, model)
            decreasing_offense += stat_prediction(dict(row, O=o+1), model) < value-1e-7
            increasing_defense += stat_prediction(dict(row, B=b+1), model) > value+1e-7
        return dict(points=len(offenses)*len(defenses), decreasing_offense=decreasing_offense, increasing_defense=increasing_defense)
    os, bs = [r['O'] for r in rows], [r['B'] for r in rows]
    observed = inspect([min(os)+(max(os)-min(os))*i/20 for i in range(21)],
                       [min(bs)+(max(bs)-min(bs))*i/20 for i in range(21)])
    broad = inspect(range(0, 1201, 50), range(0, 801, 50))
    jumps = [abs(stat_prediction(dict(rows[0], O=b+1e-6, B=b), model)-stat_prediction(dict(rows[0], O=b-1e-6, B=b), model)) for b in range(0, 801, 50)]
    return dict(observed_domain=observed, broad_domain=broad, maximum_boundary_jump=max(jumps))


def investigate(rows, label):
    validations = {}
    for split in (SKILL_SPLIT, TARGET_SPLIT, OFFENSE_SPLIT):
        validations[split] = validation(rows, split)
        print(label, split, 'done', flush=True)
    candidates = choose(rows, SKILL_SPLIT)
    fitted = []
    for c in candidates:
        model = c['model']
        fitted.append(dict(name=key(model['config']), model=model, equation=equation(model),
                           training=metrics(rows, [predict(r, model) for r in rows]),
                           diagnostics=diagnostics(rows, model), inner_error=c['inner_error']))
    examples = []
    for row in rows:
        examples.append(dict(skill=row['skill_name'], grade=row['G'], target=row['target_name'],
                             offense=row['O'], defense=row['B'], P=row['P'], E=row['E'], observed=row['observed'], casts=row['casts'],
                             baseline=predict(row, dict(config=('current', 0, False), coefficients=[], terms=[])),
                             fitted={c['name']: predict(row, c['model']) for c in fitted}))
    return dict(groups=len(rows), casts=sum(r['casts'] for r in rows), skills=len({r['id'] for r in rows}),
                targets=len({r['target'] for r in rows}), offense_values=sorted({r['O'] for r in rows}),
                below_defense_groups=sum(r['O']<r['B'] for r in rows), validations=validations, fits=fitted, examples=examples)


def write_report(result, output):
    output.mkdir(parents=True, exist_ok=True)
    payload = json.dumps(result, ensure_ascii=False, indent=2)
    (output/'analysis.json').write_text(payload+'\n', encoding='utf-8')
    parts = ['<!doctype html><meta charset="utf-8"><title>Damage polynomials with fixed P</title><style>body{font:15px system-ui;margin:24px;max-width:1400px}td,th{border:1px solid #aaa;padding:7px}table{border-collapse:collapse}pre{white-space:pre-wrap;overflow-wrap:anywhere}</style><h1>Damage polynomials with fixed P</h1><p>Damage = floor(E*(P+F(O,B))) + noise, clamped as in Gonline. P is unchanged and has coefficient exactly one. Magic uses MAT/MDEF; physical uses ATK/DEF. No gameplay changes.</p>',
             '<p>O is offense, B is defense, E is the unchanged elemental factor. F0 is the current stat-dependent remainder; L=min(0,O-B). Error is mean absolute percentage error on observed group means. Repeated casts are averaged; every skill has equal total fitting weight within each training partition. Each held group is excluded from both fitting and inner model selection.</p>',
             '<p>All 81 usable casts are included as 39 conditions, including all 35 controlled Blast Attack casts in eight conditions. Target defenses are estimates. There are no live AoE, combo, defend or critical samples in this comparison.</p>']
    for label, layer in result['layers'].items():
        parts.append('<h2>'+label+'</h2><p>'+html.escape(json.dumps({k:v['selected'] for k,v in layer['validations'].items()}))+'</p><table><tr><th>Family</th><th>Fit error %</th><th>Held skill %</th><th>Held target %</th><th>Held offense %</th><th>Stat remainder</th></tr>')
        for c in layer['fits']:
            cells = [c['name'], round(c['training']['mean_percent_error'],3)]
            cells.extend(round(layer['validations'][split]['families'][c['name']]['mean_percent_error'],3) for split in (SKILL_SPLIT,TARGET_SPLIT,OFFENSE_SPLIT))
            cells.append(c['equation'])
            parts.append('<tr>'+''.join('<td>'+html.escape(str(v))+'</td>' for v in cells)+'</tr>')
        parts.append('</table>')
        shown = ('current', 'direct:1', 'direct:1+constant', 'correction:2+constant', 'hinge:2')
        parts.append('<h3>Individual conditions, including Blast Attack</h3><table><tr><th>Skill</th><th>Grade</th><th>Target</th><th>O</th><th>B</th><th>Fixed P</th><th>Casts</th><th>Observed mean</th>'+''.join('<th>'+name+'</th>' for name in shown)+'</tr>')
        for example in layer['examples']:
            cells = [example['skill'], example['grade'], example['target'], example['offense'], example['defense'], round(example['P'],6), example['casts'], round(example['observed'],3)]
            for name in shown:
                q = example['fitted'][name]
                cells.append(f"{q['minimum']}–{q['maximum']}")
            parts.append('<tr>'+''.join('<td>'+html.escape(str(v))+'</td>' for v in cells)+'</tr>')
        parts.append('</table><p>Individual ranges above use full-data fitted coefficients, not out-of-fold predictions. Use the validation table for generalization comparisons.</p>')
    parts.append('<details><summary>Samples, diagnostics and provenance</summary><pre>'+html.escape(payload)+'</pre></details>')
    (output/'report.html').write_text('\n'.join(parts), encoding='utf-8')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--reports', type=Path, default=Path('var/reports'))
    parser.add_argument('--output', type=Path, default=Path('var/reports/damage-polynomial'))
    args = parser.parse_args()
    rows, provenance = load_rows(args.reports)
    result = dict(provenance=provenance, model_count=len(CONFIGURATIONS), layers={})
    for layer, label in ((1, 'physical'), (2, 'magical')):
        result['layers'][label] = investigate([r for r in rows if r['layer']==layer], label)
    write_report(result, args.output)
    print('Saved', args.output, flush=True)


if __name__ == '__main__':
    main()

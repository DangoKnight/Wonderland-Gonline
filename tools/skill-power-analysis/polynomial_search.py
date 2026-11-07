"""Offline polynomial P approximation with nested, whole-skill validation.

No gameplay, workbook or authored table writes. Run from the repository root.
P is the complete attack's power per target, not power per animation contact.
Only the Python standard library is required.
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

import analyze
import area_search
import raw_search
import sample_compare as sample

OUTER_FOLDS = 5
INNER_FOLDS = 3
RIDGES = (1e-8, .001, .03, .3)
BASES = (('H', 'Z'), ('H', 'D', 'G'), ('H', 'Z', 'S'))
MAGICAL_LAYER = 2


def monomials(names, degree):
    """All monomials up to degree, without intercept or pure grade/S terms.

    H/D/Z must occur: a skill with no native power has zero predicted power.
    Z=D*G, so degree in the H/Z basis is degree in these composite inputs.
    """
    return [term for size in range(1, degree+1)
            for term in itertools.combinations_with_replacement(names, size)
            if any(name in ('H', 'D', 'Z') for name in term)]


def features(row, terms):
    return [math.prod(row[name] for name in term) for term in terms]


def fit(rows, names, degree, ridge):
    return fit_path(rows, names, degree, (ridge,))[0]


def fit_path(rows, names, degree, ridges):
    """Share training-only scales and normal equations across ridge candidates."""
    terms = monomials(names, degree)
    values = [[v/r['reference'] for v in features(r, terms)] for r in rows]
    # Scaling and fitting use training rows only. Relative squared error avoids
    # allowing high-power skills to overwhelm the low-power references.
    scales = [math.sqrt(statistics.mean(v[i]**2 for v in values)) or 1
              for i in range(len(terms))]
    weights = [r.get('weight', 1) for r in rows]
    mass = sum(weights)
    x = [[v/s for v, s in zip(row, scales)] for row in values]
    matrix = [[sum(w*v[i]*v[j] for w, v in zip(weights, x))/mass
               for j in range(len(terms))] for i in range(len(terms))]
    target = [sum(w*v[i] for w, v in zip(weights, x))/mass for i in range(len(terms))]
    models = []
    for ridge in ridges:
        regularized = [row[:] for row in matrix]
        for i in range(len(terms)):
            regularized[i][i] += ridge
        coefficients = raw_search.solve(regularized, target)
        if coefficients is None:
            raise ValueError('Singular polynomial system')
        models.append(dict(names=names, degree=degree, ridge=ridge, terms=terms,
                           coefficients=[c/s for c, s in zip(coefficients, scales)]))
    return models


def predict(row, model):
    return sum(c*v for c, v in zip(model['coefficients'], features(row, model['terms'])))


def folds(rows, count):
    keys = sorted({r['id'] for r in rows})
    membership = {key: i % min(count, len(keys)) for i, key in enumerate(keys)}
    for index in range(min(count, len(keys))):
        train = [r for r in rows if membership[r['id']] != index]
        test = [r for r in rows if membership[r['id']] == index]
        if train and test:
            yield train, test


def choose(rows, degree):
    choices = []
    partitions = list(folds(rows, INNER_FOLDS))
    for names, ridge in itertools.product(BASES, RIDGES):
        errors = []
        for train, test in partitions:
            model = fit(train, names, degree, ridge)
            errors.extend(abs(predict(r, model)/r['reference']-1) for r in test)
        if errors:
            choices.append((statistics.mean(errors), names, ridge))
    _, names, ridge = min(choices)
    return fit(rows, names, degree, ridge)


def metrics(rows, values):
    result = analyze.metrics(rows, values)
    result['negative_predictions'] = sum(v < 0 for v in values)
    return result


def baseline(row):
    return 2*(row['H']+row['Z']) if row['layer'] == MAGICAL_LAYER else 1.9131446584781497*row['H']+.47524025502148415*row['Z']


def nested(rows, degree):
    held, values, details = [], [], []
    for train, test in folds(rows, OUTER_FOLDS):
        model = choose(train, degree)
        predicted = [predict(r, model) for r in test]
        held.extend(test)
        values.extend(predicted)
        details.append(dict(test_skill_ids=sorted({r['id'] for r in test}),
                            basis=model['names'], ridge=model['ridge'], metrics=metrics(test, predicted)))
    model = choose(rows, degree)
    result = dict(model=model, training=metrics(rows, [predict(r, model) for r in rows]),
                  nested_held_skills=metrics(held, values), folds=details)
    if all('observation' in r for r in held):
        result['nested_held_damage'] = damage_metrics(held, values)
    return result


def summarize(rows):
    return dict(rows=len(rows), skills=len({r['id'] for r in rows}),
                current_formula=metrics(rows, [baseline(r) for r in rows]),
                models={str(d): nested(rows, d) for d in (1, 2, 3)})


def reference_rows():
    rows, _, _, provenance = raw_search.raw_data()
    provenance['targets'] = area_search.reference_targets(rows)
    for r in rows:
        r.update(H=r['a'], D=r['b'], G=r['grade'], Z=r['b']*r['grade'], S=r['s'])
    return rows, provenance


def live_rows(directory):
    paths = [directory/'skill-power-sample1/comparison.json',
             directory/'skill-power-bugs/analysis.json',
             directory/'skill-power-low-matk/controlled-batches.json']
    skills, _ = sample.seed_definitions()
    observations = []
    for r in json.loads(paths[0].read_bytes())['rows']:
        if 'uncertain_target' not in r:
            observations.append(r)
    for r in json.loads(paths[1].read_bytes())['rows']:
        if not r['critical_flag']:
            observations.append(r)
    for group in json.loads(paths[2].read_bytes())['groups']:
        observations.extend(dict(group, observed=v) for v in group['observed'])
    grouped = collections.defaultdict(list)
    for r in observations:
        key = (r['skill_id'], r['grade'], json.dumps(r['actor'], sort_keys=True), r['enemy']['id'])
        grouped[key].append(r)
    rows = []
    for group in grouped.values():
        r = group[0]
        sk = skills[r['skill']]
        g = min(sk['value_maximum_grade'] if sk['value_maximum_grade_present'] else 10, max(1, r['grade']))
        observed = statistics.mean(v['observed'] for v in group)
        # Conditional inference, not direct Legacy P: assumes Gonline's defense
        # estimates, MDEF-only magic, elemental matrix and noise midpoint of 3.
        q = sample.damage_from_power(r['actor'], r['enemy'], sk, 0, 1)
        magic = sk['value_effect_layer'] == MAGICAL_LAYER
        offense = r['actor']['matk' if magic else 'atk']
        defense = r['enemy']['magic_defense' if magic else 'defense']
        coefficient, correction = sample.SINGLE_COEFFICIENT, sample.DEFENSE_COEFFICIENT
        if not magic and offense < defense:
            coefficient, correction = sample.LOW_PHYSICAL_COEFFICIENT, sample.HIGH_DEFENSE_COEFFICIENT
        implied = (observed-3)/q['element_factor']-coefficient*offense+correction*defense
        if implied <= 0 or observed <= 5:
            continue  # Censored minimum-damage observations cannot identify P.
        h, d = sk['value_additional_damage'], sk['value_power_per_level']
        rows.append(dict(id=r['skill_id'], layer=sk['value_effect_layer'], H=h, D=d, G=g, Z=d*g,
                         S=sk['value_stat_multiplier'], reference=implied, observed=observed,
                         casts=len(group), observation=r, skill=sk))
    # A heavily sampled skill must not dominate the fitted coefficients.
    counts = collections.Counter(r['id'] for r in rows)
    for r in rows:
        r['weight'] = 1/counts[r['id']]
    return rows, dict(casts=len(observations), groups=len(rows),
                      files={str(p): hashlib.sha256(p.read_bytes()).hexdigest() for p in paths})


def damage_metrics(rows, values):
    errors = []
    for r, p in zip(rows, values):
        q = sample.damage_from_power(r['observation']['actor'], r['observation']['enemy'], r['skill'], p, 1)
        errors.append(abs(q['midpoint']/r['observed']-1)*100)
    return dict(groups=len(rows), mean_percent_error=statistics.mean(errors), median_percent_error=statistics.median(errors))


def grade_diagnostics(rows, model):
    negative, decreasing = [], []
    for r in rows:
        curve = [predict(dict(r, G=g, Z=r['D']*g), model) for g in range(r['G']+1)]
        if min(curve) < -1e-8:
            negative.append(r['id'])
        if any(b < a-1e-8 for a, b in zip(curve, curve[1:])):
            decreasing.append(r['id'])
    return dict(skills=len(rows), negative_skill_ids=negative, decreasing_skill_ids=decreasing,
                note='Vary grade from zero through native maximum; only maximum-grade P is authored ground truth.')


def run(directory):
    refs, provenance = reference_rows()
    live, live_provenance = live_rows(directory)
    result = dict(provenance=provenance, live_provenance=live_provenance, layers={})
    for layer, label in ((1, 'physical'), (2, 'magical')):
        training = [r for r in refs if r['layer']==layer and r['targets']==1]
        area = [r for r in refs if r['layer']==layer and (r['targets'] or 0)>1]
        measured = [r for r in live if r['layer']==layer]
        layer_result = dict(reference=summarize(training), inferred_live=summarize(measured),
                            current_damage=damage_metrics(measured, [baseline(r) for r in measured]), transfer={})
        for degree in (1, 2, 3):
            ref_model = layer_result['reference']['models'][str(degree)]['model']
            live_model = layer_result['inferred_live']['models'][str(degree)]['model']
            # No live fitting in reference->live transfer; inverse transfer uses
            # an overlapping skill catalog and is explicitly descriptive only.
            layer_result['transfer'][str(degree)] = dict(
                reference_to_live_power=metrics(measured, [predict(r, ref_model) for r in measured]),
                reference_to_live_damage=damage_metrics(measured, [predict(r, ref_model) for r in measured]),
                live_to_reference_power=metrics(training, [predict(r, live_model) for r in training]),
                reference_to_area_power=metrics(area, [predict(r, ref_model)*2/(r['targets']+1) for r in area]),
                reference_model_grade_diagnostics=grade_diagnostics(training, ref_model),
                live_model_grade_diagnostics=grade_diagnostics(training, live_model))
        # Stronger domain check: withhold entire source page, reselect within
        # training pages, and evaluate only on the unseen page.
        page_results = {}
        for degree in (1, 2, 3):
            held, predictions = [], []
            for page in sorted({r['source'] for r in training}):
                train = [r for r in training if r['source'] != page]
                test = [r for r in training if r['source'] == page]
                model = choose(train, degree)
                held.extend(test)
                predictions.extend(predict(r, model) for r in test)
            page_results[str(degree)] = metrics(held, predictions)
        layer_result['reference_held_pages'] = page_results
        result['layers'][label] = layer_result
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--reports', type=Path, default=Path('var/reports'))
    parser.add_argument('--output', type=Path, default=Path('var/reports/skill-power-polynomial'))
    args = parser.parse_args()
    result = run(args.reports)
    args.output.mkdir(parents=True, exist_ok=True)
    payload = json.dumps(result, ensure_ascii=False, indent=2)
    (args.output/'analysis.json').write_text(payload+'\n', encoding='utf-8')
    (args.output/'report.html').write_text('<!doctype html><meta charset="utf-8"><title>Polynomial skill power</title><style>body{font:15px system-ui;margin:24px}pre{white-space:pre-wrap}</style><h1>Polynomial P approximation</h1><p>Offline research. Raw polynomial predictions are scored without clamping. No combat changes.</p><pre>'+html.escape(payload)+'</pre>', encoding='utf-8')
    for layer, details in result['layers'].items():
        print(layer)
        for dataset in ('reference', 'inferred_live'):
            print(dataset, details[dataset]['rows'], 'rows', details[dataset]['skills'], 'skills')
            for degree, model in details[dataset]['models'].items():
                print(degree, model['model']['names'], 'train', round(model['training']['mean_percent_error'], 3), 'held skills', round(model['nested_held_skills']['mean_percent_error'], 3))
        print('held pages', {k: round(v['mean_percent_error'], 3) for k, v in details['reference_held_pages'].items()})
        print('reference -> live damage', {k: round(v['reference_to_live_damage']['mean_percent_error'], 3) for k, v in details['transfer'].items()})


if __name__ == '__main__':
    main()

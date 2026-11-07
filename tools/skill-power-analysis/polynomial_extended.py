"""Explore more polynomial skill-power models without changing gameplay.

Extends polynomial_search with degrees four through six, native offset30 and
SP interactions. Model/basis/ridge selection happens inside each outer holdout.
"""
import argparse
import hashlib
import html
import json
from pathlib import Path
import statistics

import polynomial_search as poly

RIDGES = (1e-8, .001, .03, .3)
NATIVE_SP_OFFSET = 24
NATIVE_SP_BYTES = 2
NATIVE_UNRESOLVED_OFFSET30 = 30
CONFIGURATIONS = [
    (names, degree)
    for names, degrees in (
        (('H', 'Z'), range(1, 7)),
        (('H', 'D', 'G'), range(2, 6)),
        (('H', 'Z', 'S'), range(2, 6)),
        (('H', 'Z', 'T'), (2, 3)),
        (('H', 'Z', 'SP'), (2, 3)),
        (('H', 'Z', 'S', 'T'), (2, 3)),
        (('H', 'Z', 'S', 'SP'), (2, 3)),
        (('H', 'D', 'G', 'S'), (2, 3)),
    ) for degree in degrees
]


def key(config):
    names, degree = config
    return '/'.join(names)+f':{degree}'


def choose_candidates(rows, configs=CONFIGURATIONS):
    scores = {(key(config), ridge): [] for config in configs for ridge in RIDGES}
    for train, test in poly.folds(rows, poly.INNER_FOLDS):
        for config in configs:
            for model in poly.fit_path(train, *config, RIDGES):
                scores[(key(config), model['ridge'])].extend(
                    abs(poly.predict(r, model)/r['reference']-1) for r in test)
    candidates = []
    for config in configs:
        choices = [(statistics.mean(scores[key(config), ridge]), ridge) for ridge in RIDGES]
        score, ridge = min(choices)
        candidates.append(dict(config=config, ridge=ridge, inner_error=100*score))
    return sorted(candidates, key=lambda c: (c['inner_error'], key(c['config']), c['ridge']))


def train_candidate(rows, candidate):
    return poly.fit(rows, *candidate['config'], candidate['ridge'])


def evaluation(rows, predictions):
    result = dict(power=poly.metrics(rows, predictions))
    if all('observation' in r for r in rows):
        result['damage'] = poly.damage_metrics(rows, predictions)
    return result


def investigate(rows, progress):
    held, selected_predictions, folds = [], [], []
    by_config = {key(c): [] for c in CONFIGURATIONS}
    for index, (train, test) in enumerate(poly.folds(rows, poly.OUTER_FOLDS)):
        choices = choose_candidates(train)
        held.extend(test)
        for candidate in choices:
            model = train_candidate(train, candidate)
            values = [poly.predict(r, model) for r in test]
            by_config[key(candidate['config'])].extend(values)
            if candidate == choices[0]:
                selected_predictions.extend(values)
                folds.append(dict(skill_ids=sorted({r['id'] for r in test}),
                                  selected=key(candidate['config']), ridge=candidate['ridge'],
                                  metrics=evaluation(test, values)))
        print(f'{progress}: outer fold {index+1}/{poly.OUTER_FOLDS}', flush=True)
    choices = choose_candidates(rows)
    candidates = []
    for candidate in choices:
        model = train_candidate(rows, candidate)
        candidates.append(dict(name=key(candidate['config']), model=model,
                               inner_error=candidate['inner_error'],
                               training=evaluation(rows, [poly.predict(r, model) for r in rows]),
                               held_skills=evaluation(held, by_config[key(candidate['config'])])))
    return dict(rows=len(rows), skills=len({r['id'] for r in rows}),
                baseline=evaluation(rows, [poly.baseline(r) for r in rows]),
                selected_model=candidates[0]['model'], nested_selection=evaluation(held, selected_predictions),
                folds=folds, candidates=candidates)


def augment(rows, native):
    for row in rows:
        record = native[row['id']]
        row['T'] = record[NATIVE_UNRESOLVED_OFFSET30]  # Deliberately no semantic claim.
        row['SP'] = int.from_bytes(record[NATIVE_SP_OFFSET:NATIVE_SP_OFFSET+NATIVE_SP_BYTES], 'little')


def equation(model):
    return ' + '.join(f'{c:.12g}*'+ '*'.join(term)
                      for c, term in zip(model['coefficients'], model['terms']))


def run(reports):
    refs, provenance = poly.reference_rows()
    live, live_provenance = poly.live_rows(reports)
    source = Path('data/skill_data.json')
    records = {r['fields']['id']: bytes.fromhex(r['decoded_hex'])
               for r in json.loads(source.read_bytes())['records']}
    augment(refs, records)
    augment(live, records)
    result = dict(provenance=provenance, live_provenance=live_provenance,
                  native_export_sha256=hashlib.sha256(source.read_bytes()).hexdigest(),
                  candidate_count=len(CONFIGURATIONS), ridges=RIDGES, layers={})
    for layer, name in ((1, 'physical'), (2, 'magical')):
        reference = [r for r in refs if r['layer']==layer and r['targets']==1]
        measured = [r for r in live if r['layer']==layer]
        area = [r for r in refs if r['layer']==layer and (r['targets'] or 0)>1]
        ref_result = investigate(reference, name+' reference')
        live_result = investigate(measured, name+' live')
        # Keep every fitted equation, including failures, so this search does not
        # hide negative predictions by presenting only a clamped winner.
        for origin, analysis in (('reference', ref_result), ('live', live_result)):
            for candidate in analysis['candidates']:
                model = candidate['model']
                candidate['equation'] = equation(model)
                candidate['grade_diagnostics'] = poly.grade_diagnostics(reference, model)
                candidate['live_transfer'] = evaluation(measured, [poly.predict(r, model) for r in measured])
                candidate['reference_transfer'] = poly.metrics(reference, [poly.predict(r, model) for r in reference])
                candidate['area_transfer'] = poly.metrics(area, [poly.predict(r, model)*2/(r['targets']+1) for r in area])
                candidate['training_source'] = origin
        # Entire-page holdout tests the selected search, not a winner chosen using
        # the held page. All candidate/ridge selection is repeated within train.
        held, predictions, details = [], [], []
        pages = sorted({r['source'] for r in reference})
        for index, page in enumerate(pages):
            train = [r for r in reference if r['source'] != page]
            test = [r for r in reference if r['source'] == page]
            choice = choose_candidates(train)[0]
            model = train_candidate(train, choice)
            values = [poly.predict(r, model) for r in test]
            held.extend(test)
            predictions.extend(values)
            details.append(dict(page=page, selected=key(choice['config']), ridge=choice['ridge'],
                                metrics=poly.metrics(test, values)))
            print(f'{name}: held page {index+1}/{len(pages)}', flush=True)
        result['layers'][name] = dict(reference=ref_result, inferred_live=live_result,
                                    reference_held_pages=dict(metrics=poly.metrics(held, predictions), folds=details))
    return result


def write_report(result, output):
    output.mkdir(parents=True, exist_ok=True)
    payload = json.dumps(result, ensure_ascii=False, indent=2)
    (output/'analysis.json').write_text(payload+'\n', encoding='utf-8')
    parts = ['<!doctype html><meta charset="utf-8"><title>Extended polynomial P search</title>',
             '<style>body{font:15px system-ui;margin:24px;max-width:1400px}table{border-collapse:collapse}th,td{border:1px solid #bbb;padding:7px}details{margin:12px 0}pre{white-space:pre-wrap;overflow-wrap:anywhere}</style>',
             '<h1>Extended polynomial P search</h1>',
             '<p>Offline investigation; runtime unchanged. H=offset49, D=offset32, G=grade, Z=D*G, S=offset40, T=raw decoded offset30, SP=offset24. Degrees apply to these inputs. Negative predictions remain visible.</p>',
             '<p>Each candidate selects its ridge within training-only inner folds. The complete search also selects its structure inside outer folds. Ranking individual candidates by outer error is exploratory; see nested_selection for the entire selection procedure.</p>']
    for layer, details in result['layers'].items():
        for source in ('reference', 'inferred_live'):
            data = details[source]
            parts.append(f'<h2>{layer}: {source}</h2><p>Nested selection: '+html.escape(json.dumps(data['nested_selection']))+'</p>')
            parts.append('<table><tr><th>Polynomial</th><th>Terms</th><th>Train P mean error</th><th>Held P mean error</th><th>Held damage mean error</th><th>Negative/decreasing grade curves</th></tr>')
            ranked = sorted(data['candidates'], key=lambda c: c['held_skills']['power']['mean_percent_error'])
            for c in ranked:
                diag = c['grade_diagnostics']
                values = (c['name'], len(c['model']['terms']),
                          round(c['training']['power']['mean_percent_error'], 3),
                          round(c['held_skills']['power']['mean_percent_error'], 3),
                          round(c['held_skills']['damage']['mean_percent_error'], 3) if 'damage' in c['held_skills'] else '',
                          f"{len(diag['negative_skill_ids'])}/{len(diag['decreasing_skill_ids'])}")
                parts.append('<tr>'+''.join('<td>'+html.escape(str(v))+'</td>' for v in values)+'</tr>')
            parts.append('</table>')
            for c in ranked:
                parts.append('<details><summary>'+html.escape(c['name'])+'</summary><pre>'+html.escape(c['equation'])+'</pre></details>')
    parts.append('<details><summary>Complete diagnostics and provenance</summary><pre>'+html.escape(payload)+'</pre></details>')
    (output/'report.html').write_text('\n'.join(parts), encoding='utf-8')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--reports', type=Path, default=Path('var/reports'))
    parser.add_argument('--output', type=Path, default=Path('var/reports/skill-power-polynomial-extended'))
    args = parser.parse_args()
    result = run(args.reports)
    write_report(result, args.output)
    print('Saved', args.output, flush=True)


if __name__ == '__main__':
    main()

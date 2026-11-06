"""Compare Skill.dat power hypotheses with the independently authored P table.

Offline analysis only: does not modify catalogs, reference powers or combat.
Run from the repository root. No third-party Python packages are needed.
"""
import argparse
import csv
import hashlib
import html
import json
import math
from pathlib import Path
import statistics

SKILL_BYTES = 148
MAXIMUM_GRADE_OFFSET = 105
CONTACT_MODE_OFFSET = 24
CONTACT_COUNT_OFFSET = 25
CONTACT_START_OFFSET = 26
CONTACT_BYTES = 5
CONTACT_WEIGHT_OFFSET = 4
SINGLE_DAMAGE_MODE = 0


def load_rows(skills_path, animations_path, powers_path):
    skills = json.loads(skills_path.read_bytes())
    animations = json.loads(animations_path.read_bytes())
    motions = {r['id']: bytes.fromhex(r['decoded_hex']) for r in animations['records']}
    definitions = {r['fields']['id']: r for r in skills['records']}
    rows = []
    with powers_path.open(encoding='utf-8', newline='') as source:
        for reference in csv.DictReader(source):
            skill_id = int(reference['skill_id'])
            definition = definitions[skill_id]
            record = bytes.fromhex(definition['decoded_hex'])
            if len(record) != SKILL_BYTES:
                raise ValueError(f'Invalid native skill record {skill_id}')
            fields = definition['fields']
            motion = motions[skill_id]
            mode, count = motion[CONTACT_MODE_OFFSET:CONTACT_COUNT_OFFSET + 1]
            weights = [motion[CONTACT_START_OFFSET + i * CONTACT_BYTES + CONTACT_WEIGHT_OFFSET]
                       for i in range(count)]
            contacts = max(1, count) if mode != SINGLE_DAMAGE_MODE else 1
            weight = sum(weights) / 100 if mode != SINGLE_DAMAGE_MODE and count else 1
            grade = record[MAXIMUM_GRADE_OFFSET]
            rows.append(dict(
                id=skill_id, name=definition['name']['text'], layer=int(fields['effect_layer']),
                source=reference['source'], source_name=reference['source_name'],
                reference=float(reference['power']), grade=grade,
                a=float(fields['additional_damage']), b=float(fields['power_per_level']),
                s=float(fields['stat_multiplier']), c=contacts, w=weight or 1,
                contact_mode=mode, contact_weights=weights,
            ))
    if any(r['reference'] <= 0 or r['layer'] not in (1, 2) for r in rows):
        raise ValueError('Analysis requires positive physical/magical reference powers')
    provenance = {
        'skills_native_sha256': skills['source_sha256'],
        'animations_native_sha256': animations['source_sha256'],
        'reference_table_sha256': hashlib.sha256(powers_path.read_bytes()).hexdigest(),
    }
    return rows, provenance


def metrics(rows, predictions):
    errors = [abs(p - r['reference']) for r, p in zip(rows, predictions)]
    relative = [e / r['reference'] for r, e in zip(rows, errors)]
    signed = [(p / r['reference'] - 1) for r, p in zip(rows, predictions)]
    return dict(n=len(rows), median_percent_error=100 * statistics.median(relative),
                mean_percent_error=100 * statistics.mean(relative),
                median_bias_percent=100 * statistics.median(signed),
                p90_percent_error=100 * sorted(relative)[math.ceil(.9 * len(relative)) - 1],
                mean_absolute_error=statistics.mean(errors),
                within_10_percent=sum(e <= .100000001 for e in relative),
                within_20_percent=sum(e <= .200000001 for e in relative))


def evaluate(rows, formula):
    predictions = [formula(r) for r in rows]
    return {key: metrics([r for r in rows if keep(r)],
                         [p for r, p in zip(rows, predictions) if keep(r)])
            for key, keep in (
                ('all', lambda r: True), ('physical', lambda r: r['layer'] == 1),
                ('magical', lambda r: r['layer'] == 2),
                ('elemental', lambda r: not r['source'].startswith('固有')),
                ('unique', lambda r: r['source'].startswith('固有')),
            )}


def hypotheses():
    base = {
        'A+B*G': lambda r: r['a'] + r['b'] * r['grade'],
        'A+B*(G-1)': lambda r: r['a'] + r['b'] * max(0, r['grade'] - 1),
        'A+B*(G+1)': lambda r: r['a'] + r['b'] * (r['grade'] + 1),
        '2*A+B*G': lambda r: 2 * r['a'] + r['b'] * r['grade'],
        '2*A+0.5*B*G': lambda r: 2 * r['a'] + .5 * r['b'] * r['grade'],
        '3*A+B*G': lambda r: 3 * r['a'] + r['b'] * r['grade'],
        '2*(A+B*G)': lambda r: 2 * (r['a'] + r['b'] * r['grade']),
        '(A+B*G)*S': lambda r: (r['a'] + r['b'] * r['grade']) * r['s'],
        'A+B*G*S': lambda r: r['a'] + r['b'] * r['grade'] * r['s'],
        'A*S+B*G': lambda r: r['a'] * r['s'] + r['b'] * r['grade'],
        '(A+B*G)/S': lambda r: (r['a'] + r['b'] * r['grade']) / (r['s'] or 1),
        '(A+B*G)*normalized-S': lambda r: (r['a'] + r['b'] * r['grade']) * r['s'] / (.6 if r['layer'] == 1 else 1.2),
        '(A+B*G)/normalized-S': lambda r: (r['a'] + r['b'] * r['grade']) * (.6 if r['layer'] == 1 else 1.2) / (r['s'] or 1),
    }
    formulas = {}
    for name, formula in base.items():
        for suffix, multiplier in (('', lambda r: 1), ('*C', lambda r: r['c']), ('*W', lambda r: r['w'])):
            key = f'({name}){suffix}' if suffix else name
            formulas[key] = lambda r, f=formula, m=multiplier: f(r) * m(r)
    formulas['layer-simple: physical A+B*G; magic 2*(A+B*G)'] = lambda r: (r['a'] + r['b'] * r['grade']) * (1 if r['layer'] == 1 else 2)
    formulas['layer-grid: physical 2*A+0.5*B*G; magic 2.5*A+1.5*B*G'] = lambda r: ((2 * r['a'] + .5 * r['b'] * r['grade']) if r['layer'] == 1 else (2.5 * r['a'] + 1.5 * r['b'] * r['grade']))
    formulas['layer-hybrid: physical 2*A+0.5*B*G; magic 2*(A+B*G)'] = lambda r: ((2 * r['a'] + .5 * r['b'] * r['grade']) if r['layer'] == 1 else 2 * (r['a'] + r['b'] * r['grade']))
    formulas['layer-hybrid: physical A+B*G; magic 2.5*A+1.5*B*G'] = lambda r: ((r['a'] + r['b'] * r['grade']) if r['layer'] == 1 else 2.5 * r['a'] + 1.5 * r['b'] * r['grade'])
    formulas['diagnostic grade cap 10: physical A+B*min(G,10); magic 2*(A+B*G)'] = lambda r: ((r['a'] + r['b'] * min(r['grade'], 10)) if r['layer'] == 1 else 2 * (r['a'] + r['b'] * r['grade']))
    return formulas


def choose_grid(rows):
    choices = (0, .5, 1, 1.5, 2, 2.5, 3, 4)
    rankings = []
    for a in choices:
        for b in choices:
            predictions = [a*r['a'] + b*r['b']*r['grade'] for r in rows]
            rankings.append(dict(a=a, b=b, metrics=metrics(rows, predictions)))
    return sorted(rankings, key=lambda r: (r['metrics']['median_percent_error'], r['metrics']['mean_percent_error']))


def grid_analysis(rows):
    ranked = {layer: choose_grid([r for r in rows if r['layer'] == layer]) for layer in (1, 2)}
    held_rows, predictions, folds = [], [], []
    for source in sorted({r['source'] for r in rows}):
        trained = {layer: choose_grid([r for r in rows if r['layer'] == layer and r['source'] != source])[0]
                   for layer in (1, 2)}
        held = [r for r in rows if r['source'] == source]
        values = [trained[r['layer']]['a']*r['a'] + trained[r['layer']]['b']*r['b']*r['grade'] for r in held]
        held_rows.extend(held); predictions.extend(values)
        folds.append(dict(source=source, coefficients={layer: [v['a'], v['b']] for layer, v in trained.items()}, metrics=metrics(held, values)))
    return dict(top_coefficients={layer: v[:5] for layer, v in ranked.items()},
                leave_one_source_out=metrics(held_rows, predictions), folds=folds)


def coefficients(rows):
    # Relative least squares: each row has equal proportional weight. No intercept
    # ensures absent native power remains zero. This is a fit, not native evidence.
    xx = xy = yy = xz = yz = 0.
    for row in rows:
        x, y = row['a'] / row['reference'], row['b'] * row['grade'] / row['reference']
        xx += x*x; xy += x*y; yy += y*y; xz += x; yz += y
    determinant = xx*yy - xy*xy
    if abs(determinant) < 1e-12:
        raise ValueError('Cannot independently fit A and B*G coefficients')
    return [(xz*yy-yz*xy)/determinant, (yz*xx-xz*xy)/determinant]


def fitted_analysis(rows):
    fits = {layer: coefficients([r for r in rows if r['layer'] == layer]) for layer in (1, 2)}
    formula = lambda r: fits[r['layer']][0]*r['a'] + fits[r['layer']][1]*r['b']*r['grade']
    sources = sorted({r['source'] for r in rows})
    held_predictions, held_rows, folds = [], [], []
    for source in sources:
        train = [r for r in rows if r['source'] != source]
        held = [r for r in rows if r['source'] == source]
        trained = {layer: coefficients([r for r in train if r['layer'] == layer]) for layer in (1, 2)}
        predictions = [trained[r['layer']][0]*r['a'] + trained[r['layer']][1]*r['b']*r['grade'] for r in held]
        held_predictions.extend(predictions); held_rows.extend(held)
        folds.append(dict(source=source, coefficients=trained, metrics=metrics(held, predictions)))
    correlation = {}
    for layer in (1, 2):
        subset = [r for r in rows if r['layer'] == layer]
        a, bg = [r['a'] for r in subset], [r['b']*r['grade'] for r in subset]
        correlation[layer] = statistics.correlation(a, bg)
    return dict(coefficients=fits, in_sample=evaluate(rows, formula),
                leave_one_source_out=metrics(held_rows, held_predictions),
                folds=folds, a_bg_correlation=correlation), formula


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--skills', type=Path, default=Path('data/skill_data.json'))
    parser.add_argument('--animations', type=Path, default=Path('data/animation_data.json'))
    parser.add_argument('--powers', type=Path, default=Path('internal/assetsql/skill_power_defaults.csv'))
    parser.add_argument('--output', type=Path, default=Path('var/reports/skill-power-formulas'))
    args = parser.parse_args()
    rows, provenance = load_rows(args.skills, args.animations, args.powers)
    formulas = hypotheses()
    fit, fitted = fitted_analysis(rows)
    grid = grid_analysis(rows)
    formulas['fitted layer coefficients (in-sample)'] = fitted
    results = {name: evaluate(rows, formula) for name, formula in formulas.items()}
    baseline = '(A+B*G)*C'
    chosen = 'layer-hybrid: physical 2*A+0.5*B*G; magic 2*(A+B*G)'
    pages = {source: {name: metrics([r for r in rows if r['source'] == source],
                                   [formula(r) for r in rows if r['source'] == source])
                      for name, formula in formulas.items()}
             for source in sorted({r['source'] for r in rows})}
    for row in rows:
        row['predictions'] = {name: formula(row) for name, formula in formulas.items()}
    output = dict(provenance=provenance, notation=dict(A='AdditionalDamage', B='PowerPerLevel', G='native maximum grade',
                  S='StatMultiplier', C='effective contact count', W='sum of contact percentages / 100; single mode or empty uses 1'),
                  baseline=baseline, simple_candidate=chosen, results=results, fitted=fit, grid=grid, by_source=pages, skills=rows)
    args.output.mkdir(parents=True, exist_ok=True)
    (args.output / 'comparison.json').write_text(json.dumps(output, ensure_ascii=False, indent=2)+'\n', encoding='utf-8')
    h = html.escape
    table = ['<tr><th>Formula</th><th>Median error</th><th>Physical</th><th>Magic</th><th>Within 10%</th><th>Within 20%</th><th>90th percentile error</th></tr>']
    for name in sorted(results, key=lambda n: results[n]['all']['median_percent_error']):
        result = results[name]
        table.append('<tr>' + ''.join(f'<td>{v}</td>' for v in (
            h(name), f"{result['all']['median_percent_error']:.2f}%", f"{result['physical']['median_percent_error']:.2f}%",
            f"{result['magical']['median_percent_error']:.2f}%", result['all']['within_10_percent'],
            result['all']['within_20_percent'], f"{result['all']['p90_percent_error']:.1f}%")) + '</tr>')
    detail = ['<tr><th>ID</th><th>Skill</th><th>Layer</th><th>A</th><th>B</th><th>G</th><th>S</th><th>C</th><th>Weights</th><th>Reference P</th><th>Current P</th><th>Simple candidate P</th><th>Candidate error</th></tr>']
    for r in sorted(rows, key=lambda r: abs(r['predictions'][chosen]/r['reference']-1), reverse=True):
        values = [r['id'], h(r['name']), r['layer'], r['a'], r['b'], r['grade'], r['s'], r['c'], r['contact_weights'],
                  r['reference'], round(r['predictions'][baseline], 3), round(r['predictions'][chosen], 3),
                  f"{100*abs(r['predictions'][chosen]/r['reference']-1):.1f}%"]
        detail.append('<tr>' + ''.join(f'<td>{value}</td>' for value in values) + '</tr>')
    document = '<!doctype html><meta charset="utf-8"><title>Skill power formula comparison</title><style>body{font:15px system-ui;margin:24px;color:#dde6f5;background:#141b29}table{border-collapse:collapse;width:100%;margin:20px 0}td,th{border:1px solid #42516a;padding:7px;text-align:right}th{background:#26344d}td:nth-child(1),td:nth-child(2){text-align:left}h1,h2{color:#9cc8ff}p{max-width:1000px;line-height:1.6}</style>'
    document += '<h1>Skill power hypotheses</h1><p>224 independent reference powers: 123 physical, 101 magical. G is the native maximum grade, including literal zero. A=AdditionalDamage; B=PowerPerLevel; S=StatMultiplier; C=effective contacts; W=sum of native contact percentages / 100. No gameplay or reference definitions were changed.</p>'
    document += '<p>Hypotheses and empirical coefficients are exploratory. Max-grade references do not identify the progression curve. Different client/server versions, uncertain mappings and stat-dependent interpretations of community P remain possible confounders. Fitted coefficients use relative least squares; page-held-out errors are recorded in the JSON report.</p>'
    document += '<h2>Candidate comparison</h2><table>'+''.join(table)+'</table><h2>Skills (largest simple-candidate errors first)</h2><table>'+''.join(detail)+'</table>'
    (args.output / 'report.html').write_text(document, encoding='utf-8')
    print(json.dumps(dict(baseline=results[baseline], simple=results[chosen], fit=fit, grid=grid), indent=2))


if __name__ == '__main__':
    main()

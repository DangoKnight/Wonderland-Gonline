"""Explore compact, positive P equations; never modify gameplay or references.

Run from the repository root. Uses only the standard library. The sparse search
has no inverse-grade terms, identity fields or per-skill exceptions. Validation
repeats coefficient AND structure selection within each training partition.
"""
import argparse
import hashlib
import html
import itertools
import json
import math
from pathlib import Path
import statistics

import analyze
import raw_search as raw

# Disk offsets; names deliberately do not claim damage semantics.
SP_OFFSET = '24'
T_OFFSET = '30'
D_OFFSET = '32'
S_OFFSET = '40'
H_OFFSET = '49'
GRADE_OFFSET = '105'
LAST_PATTERN_OFFSET = '112'
PATTERN_OFFSETS = ('109', '110', '111', '112')
PATTERN_GRADE_BAND = 3  # Existing compatibility rule, not proven native behavior.
FOLD_COUNT = 5
MAX_TERMS = 3
ROBUST_CANDIDATES = 20
ROBUST_STEPS = 16
ERROR_WEIGHT_FLOOR = .02
COEFFICIENT_PENALTY = .25  # Percentage points in the exploratory ranking score.


def active_pattern(x):
    band = min(len(PATTERN_OFFSETS)-1, max(0, (int(x[GRADE_OFFSET])-1)//PATTERN_GRADE_BAND))
    for index in range(band, -1, -1):
        if x[PATTERN_OFFSETS[index]]:
            return x[PATTERN_OFFSETS[index]]
    return 1


def prepare(row):
    x = row['x']
    h, d, s, g, t = (x[o] for o in (H_OFFSET, D_OFFSET, S_OFFSET, GRADE_OFFSET, T_OFFSET))
    k = max(1, x[LAST_PATTERN_OFFSET])
    # N is a hypothesis using the C#-derived shape capacity, not raw byte 31.
    pattern = active_pattern(x)
    n = pattern if pattern in (1, 2, 3, 4, 5, 6) else 8 if pattern in (7, 8) else 1
    return dict(H=h, D=d, S=s, G=g, T=t, K=k, N=n, SP=x[SP_OFFSET],
                C=row['c'], M=row['contact_mode'],
                concentration=sum((w/100)**2 for w in row['contact_weights']) or 1)


BASE_TERMS = {
    'H': lambda v: v['H'],
    'H*S': lambda v: v['H']*v['S'],
    'H*S^2': lambda v: v['H']*v['S']**2,
    'H*T': lambda v: v['H']*v['T'],
    'H*G': lambda v: v['H']*v['G'],
    'H*(G-1)': lambda v: v['H']*max(0, v['G']-1),
    'H*sqrt(G)': lambda v: v['H']*math.sqrt(v['G']),
    'D*G': lambda v: v['D']*v['G'],
    'D*(G-1)': lambda v: v['D']*max(0, v['G']-1),
    'D*G*S': lambda v: v['D']*v['G']*v['S'],
    'D*G*S^2': lambda v: v['D']*v['G']*v['S']**2,
    'D*(G-1)*S': lambda v: v['D']*max(0, v['G']-1)*v['S'],
    'D*(G-1)*S^2': lambda v: v['D']*max(0, v['G']-1)*v['S']**2,
    'D*G^2': lambda v: v['D']*v['G']**2,
    'D*sqrt(G)': lambda v: v['D']*math.sqrt(v['G']),
    'D*G*T': lambda v: v['D']*v['G']*v['T'],
    'T^2': lambda v: v['T']**2,
    'T*G': lambda v: v['T']*v['G'],
    'SP': lambda v: v['SP'],
}
ATTENUATIONS = {
    '1': lambda v: 1,
    '1/K': lambda v: 1/v['K'],
    '1/sqrt(K)': lambda v: 1/math.sqrt(v['K']),
}


def feature_table(rows, animation=False):
    terms = {f'({name})*{suffix}': lambda v, f=f, a=a: f(v)*a(v)
             for name, f in BASE_TERMS.items() for suffix, a in ATTENUATIONS.items()}
    # Tests actual mapped shape capacity rather than treating code 7 as 7 targets.
    for name in ('H', 'D*G', 'H*T'):
        f = BASE_TERMS[name]
        terms[f'({name})/N'] = lambda v, f=f: f(v)/v['N']
        terms[f'({name})/(1+.65*(N-1))'] = lambda v, f=f: f(v)/(1+.65*(v['N']-1))
    if animation:
        # Add modifiers to native power terms; do not let frame IDs memorize skills.
        for name in ('H', 'D*G'):
            f = BASE_TERMS[name]
            terms[f'({name})*C'] = lambda v, f=f: f(v)*v['C']
            terms[f'({name})/C'] = lambda v, f=f: f(v)/v['C']
            terms[f'({name})*weight_concentration'] = lambda v, f=f: f(v)*v['concentration']
    values = [prepare(r) for r in rows]
    return list(terms), [[f(v) for f in terms.values()] for v in values]


def positive_fit(matrix, target):
    """Small NNLS via all active sets, including the zero solution (<=3 terms)."""
    n = len(target)
    best, best_loss = [0.]*n, 0.
    for size in range(1, n+1):
        for chosen in itertools.combinations(range(n), size):
            coefficients = raw.regularized([[matrix[i][j] for j in chosen] for i in chosen],
                                          [target[i] for i in chosen])
            if coefficients is None or min(coefficients) < 0:
                continue
            result = [0.]*n
            for index, coefficient in zip(chosen, coefficients):
                result[index] = coefficient
            loss = sum(result[i]*matrix[i][j]*result[j] for i in range(n) for j in range(n))
            loss -= 2*sum(c*t for c, t in zip(result, target))
            if loss < best_loss:
                best, best_loss = result, loss
    return best


def group(row, split):
    return int(row['x'][S_OFFSET] >= 1) if split else 0


def predict(row, candidate):
    coefficients = candidate['coefficients'][group(row, candidate['split'])]
    return sum(row['features'][i]*c for i, c in zip(candidate['indices'], coefficients))


def evaluate(rows, candidate):
    metrics = analyze.metrics(rows, [predict(r, candidate) for r in rows])
    # Count fitted coefficients, not just terms, when comparing shared/split models.
    complexity = len(candidate['indices'])*len(candidate['coefficients'])
    objective = metrics['median_percent_error'] + .25*metrics['mean_percent_error']
    return objective + COEFFICIENT_PENALTY*complexity, metrics


def search(rows, maximum_terms=MAX_TERMS, split_modes=(False, True)):
    count = len(rows[0]['features'])
    ranked = []
    for split in split_modes:
        groups = {k: [r for r in rows if group(r, split) == k] for k in ({0, 1} if split else {0})}
        if any(not rs for rs in groups.values()):
            continue
        matrices = {k: raw.normal(rs, list(range(count))) for k, rs in groups.items()}
        for size in range(1, maximum_terms+1):
            for indices in itertools.combinations(range(count), size):
                fits = {}
                for k, (matrix, target) in matrices.items():
                    a = [[matrix[i][j] for j in indices] for i in indices]
                    b = [target[i] for i in indices]
                    # Boundary solutions are already represented by smaller subsets.
                    cs = raw.regularized(a, b)
                    if cs is None or min(cs) < 0:
                        break
                    fits[k] = cs
                if len(fits) != len(groups):
                    continue
                candidate = dict(indices=list(indices), coefficients=fits, split=split, robust=False)
                objective, metrics = evaluate(rows, candidate)
                candidate.update(objective=objective, metrics=metrics)
                ranked.append(candidate)
    ranked.sort(key=lambda c: c['objective'])
    # Refine each model class separately, so larger split fits do not crowd out
    # promising shared/coarser candidates before robust fitting.
    seeds = []
    for split, size in itertools.product(split_modes, range(1, maximum_terms+1)):
        seeds.extend([c for c in ranked if c['split'] == split and len(c['indices']) == size][:ROBUST_CANDIDATES])
    for seed in seeds:
        candidate = dict(seed, coefficients={k: list(v) for k, v in seed['coefficients'].items()}, robust=True)
        indices = candidate['indices']
        for _ in range(ROBUST_STEPS):
            for k in candidate['coefficients']:
                subset = [r for r in rows if group(r, candidate['split']) == k]
                weights = [1/max(ERROR_WEIGHT_FLOOR, abs(predict(r, candidate)/r['reference']-1)) for r in subset]
                candidate['coefficients'][k] = positive_fit(*raw.normal(subset, indices, weights))
        objective, metrics = evaluate(rows, candidate)
        candidate.update(objective=objective, metrics=metrics)
        ranked.append(candidate)
    return sorted(ranked, key=lambda c: c['objective'])


def cross_validate(rows, groups):
    values, held_rows, folds = [], [], []
    for key in sorted(set(groups)):
        train = [r for r, g in zip(rows, groups) if g != key]
        held = [r for r, g in zip(rows, groups) if g == key]
        candidate = search(train)[0]
        predictions = [predict(r, candidate) for r in held]
        held_rows.extend(held)
        values.extend(predictions)
        folds.append(dict(group=str(key), selected=candidate, metrics=analyze.metrics(held, predictions)))
        print(json.dumps(dict(fold=str(key), metrics=folds[-1]['metrics'])), flush=True)
    return dict(metrics=analyze.metrics(held_rows, values), folds=folds,
                predictions={r['id']: p for r, p in zip(held_rows, values)})


def diagnostics(rows, selected):
    def rounded(v):
        a, b, c = (2.3, .32, 1.8) if v['S'] < 1 else (1.25, 1.8, .28)
        return a*v['H']*v['S']/v['K'] + b*v['D']*max(0, v['G']-1)*v['S']**2/math.sqrt(v['K']) + c*v['D']*math.sqrt(v['G'])

    formulas = {
        'production (H+D*G)*C': lambda r, v: (v['H']+v['D']*v['G'])*v['C'],
        'no contact multiplication H+D*G': lambda r, v: v['H']+v['D']*v['G'],
        'rounded shared two-term': lambda r, v: 1.4*v['H']/math.sqrt(v['K'])+1.7*v['D']*v['G']*v['S']**2/v['K'],
        'rounded split three-term': lambda r, v: rounded(v),
        'selected total power': lambda r, v: predict(r, selected),
        'selected times contact count': lambda r, v: predict(r, selected)*v['C'],
        'selected times sum of contact weights': lambda r, v: predict(r, selected)*r['w'],
        'selected times strongest contact weight': lambda r, v: predict(r, selected)*max(r['contact_weights'], default=100)/100,
    }
    values = {name: [f(r, prepare(r)) for r in rows] for name, f in formulas.items()}
    subsets = {
        'all': lambda r: True,
        'single shape': lambda r: active_pattern(r['x']) == 1,
        'area shape': lambda r: active_pattern(r['x']) != 1,
        'one contact': lambda r: r['c'] == 1,
        'multiple contacts': lambda r: r['c'] > 1,
        'grade one': lambda r: r['grade'] == 1,
    }
    return dict(
        comparisons={name: {label: analyze.metrics([r for r in rows if keep(r)],
                                                  [p for r, p in zip(rows, predictions) if keep(r)])
                            for label, keep in subsets.items()}
                     for name, predictions in values.items()},
        predictions={name: {r['id']: p for r, p in zip(rows, predictions)} for name, predictions in values.items()},
        contact_records=[dict(id=r['id'], name=r['name'], mode=r['contact_mode'],
                              weights=r['contact_weights'], effective_sum=r['w']) for r in rows],
    )


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, default=Path('var/reports/skill-power-structured'))
    parser.add_argument('--animation', action='store_true', help='add six contact-based interaction terms')
    parser.add_argument('--no-validation', action='store_true', help='exploratory in-sample screening only')
    args = parser.parse_args()
    rows, _, records, provenance = raw.raw_data()
    names, table = feature_table(rows, args.animation)
    for row, features in zip(rows, table):
        row['features'] = features
    ranked = search(rows)
    print(json.dumps(dict(best=ranked[0], terms=[names[i] for i in ranked[0]['indices']])), flush=True)
    result = dict(provenance=provenance, terms=names, animation=args.animation,
                  candidate_subsets=2*sum(math.comb(len(names), i) for i in range(1, MAX_TERMS+1)),
                  top_models=ranked[:30], model_classes=[], diagnostics=diagnostics(rows, ranked[0]))
    for split, size in itertools.product((False, True), range(1, MAX_TERMS+1)):
        candidate = next(c for c in ranked if c['split'] == split and len(c['indices']) == size)
        result['model_classes'].append(candidate)
    if not args.no_validation:
        families = raw.families(records)
        groups = [int(hashlib.sha256(str(families[r['id']]).encode()).hexdigest()[:8], 16)%FOLD_COUNT for r in rows]
        result['linked_validation'] = cross_validate(rows, groups)
        result['page_validation'] = cross_validate(rows, [r['source'] for r in rows])
    result['skills'] = [dict(id=r['id'], name=r['name'], reference=r['reference'],
                             raw=prepare(r), prediction=predict(r, ranked[0])) for r in rows]
    write_report(result, args.output)


def write_report(result, output):
    output.mkdir(parents=True, exist_ok=True)
    (output/'analysis.json').write_text(json.dumps(result, ensure_ascii=False, indent=2)+'\n', encoding='utf-8')
    headers = ['ID', 'Skill', 'Reference P', 'Fitted P', 'Error %']
    body = ['<tr>'+''.join('<th>'+h+'</th>' for h in headers)+'</tr>']
    for r in sorted(result['skills'], key=lambda r: abs(r['prediction']/r['reference']-1), reverse=True):
        cells = [r['id'], r['name'], r['reference'], round(r['prediction'], 3), round(100*abs(r['prediction']/r['reference']-1), 2)]
        body.append('<tr>'+''.join('<td>'+html.escape(str(v))+'</td>' for v in cells)+'</tr>')
    overview = {k: v for k, v in result.items() if k not in ('skills', 'top_models', 'diagnostics')}
    document = '<!doctype html><meta charset="utf-8"><title>Structured skill-power investigation</title>'
    document += '<style>body{font:15px system-ui;margin:24px;background:#172133;color:#e3ecff}td,th{padding:7px;border:1px solid #4f617c}table{border-collapse:collapse}pre{white-space:pre-wrap}</style>'
    document += '<h1>Structured P hypotheses</h1><p>Exploratory fits, not recovered server rules. Raw numeric inputs; no changes to combat. Internal holdouts reuse the same reference collection and are not independent live validation.</p>'
    document += '<h2>Total-power checks</h2><table><tr><th>Formula</th><th>Median error</th><th>Mean error</th><th>Within 10%</th></tr>'
    for name, groups in result['diagnostics']['comparisons'].items():
        metrics = groups['all']
        document += '<tr>'+''.join('<td>'+html.escape(str(v))+'</td>' for v in
                                  (name, round(metrics['median_percent_error'], 2),
                                   round(metrics['mean_percent_error'], 2), metrics['within_10_percent']))+'</tr>'
    document += '</table>'
    document += '<h2>All skills</h2><table>'+''.join(body)+'</table><h2>Methods and metrics</h2><pre>'+html.escape(json.dumps(overview, indent=2))+'</pre>'
    (output/'report.html').write_text(document, encoding='utf-8')


if __name__ == '__main__':
    main()

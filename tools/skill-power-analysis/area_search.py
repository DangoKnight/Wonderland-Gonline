"""Separate target capacity from total skill power using saved wiki evidence.

Offline research only. Reference target counts are independent of Skill.dat's
shape codes. Fit a base equation on single-target rows, then test area penalties
on the withheld area rows. No gameplay definitions or authored tables change.
"""
import argparse
import csv
import hashlib
import html
import itertools
import json
import math
from pathlib import Path
import re
import statistics
import sys
import unicodedata

import analyze
import raw_search as raw
import structured_search as structured

EXPORT_TOOLS = Path(__file__).resolve().parents[1] / 'data_export'
sys.path.insert(0, str(EXPORT_TOOLS))
from extract_skill_powers import expanded_rows

REFERENCE_PAGES = Path('var/reference-web/skills')
UNIQUE_BINDINGS = EXPORT_TOOLS / 'reference/skill-powers/unique_bindings.csv'
ELEMENTAL_TABLE_COUNT = 2
UNIQUE_DESCRIPTION_COLUMN = 6
UNIQUE_GRADE_COLUMN = 3
PARAMETER_STEPS = 400
PARAMETER_STEP_SIZE = .01
SINGLE_STAT_COEFFICIENT = 1.4
# Competing research claims about STAT, tested on P only as hypotheses.
SUPPLIED_STAT_COEFFICIENTS = {1: 1.4, 2: .85, 3: .6, 4: .6, 6: .4}
VIPWO_STAT_COEFFICIENTS = {1: 1.4, 2: .84, 3: .4, 4: .6, 6: .4}
PAIR_OFFSETS = ('32', '40', '49', '105')  # Numeric inputs, not identity fields.


def grade_band(text):
    value = unicodedata.normalize('NFKC', text).replace('〜', '~')
    found = re.search(r'(\d+)\s*~\s*(\d+)', value)
    return (int(found[1]), int(found[2])) if found else None


def target_capacity(text):
    """Parse explicit counts/shapes; blank or unknown remains unknown, not one."""
    value = unicodedata.normalize('NFKC', text).split('(')[0].strip()
    if not value:
        return None
    value = re.sub(r'\s+', '', value)
    if re.fullmatch(r'(?:横|縦|十字)?\d+(?:体)?', value):
        return int(re.search(r'\d+', value)[0])
    if re.fullmatch(r'(?:横|縦)\d+体?[*×x](?:横|縦)\d+体?', value):
        return math.prod(map(int, re.findall(r'\d+', value)))
    raise ValueError(f'Unrecognized target label: {text!r}')


def select_target(candidates, grade):
    selected = [c for c in candidates if c['grade_band'] is None or
                c['grade_band'][0] <= grade <= c['grade_band'][1]]
    capacities = {c['capacity'] for c in selected}
    if len(capacities) != 1:
        raise ValueError(f'Ambiguous/missing target evidence at grade {grade}: {selected}')
    return selected[0]['capacity'], selected


def reference_targets(rows, pages=REFERENCE_PAGES, bindings_path=UNIQUE_BINDINGS):
    parsed, sources = {}, []
    for name in sorted({r['source'] for r in rows}):
        content = (pages / name).read_bytes()
        parsed[name] = list(expanded_rows(content.decode('utf-8-sig')))
        sources.append(dict(file=name, sha256=hashlib.sha256(content).hexdigest()))
    with bindings_path.open(encoding='utf-8', newline='') as source:
        bindings = list(csv.DictReader(source))
    for row in rows:
        unique = row['source'].startswith('固有')
        keys = {(int(b['table']), int(b['row'])) for b in bindings
                if int(b['skill_id']) == row['id'] and b['status'] == 'verified'}
        candidates = []
        for table, index, cells in parsed[row['source']]:
            if unique:
                if (table, index) not in keys:
                    continue
            elif table >= ELEMENTAL_TABLE_COUNT or table != row['layer']-1 or row['source_name'] not in cells:
                continue
            description_index = next((i for i, cell in enumerate(cells) if 'スキル威力' in cell), None)
            if description_index is None:
                continue
            description = cells[description_index]
            power = re.search(r'スキル威力\s*(\d+)', description)
            if not power or int(power[1]) != row['reference']:
                raise ValueError(f'Reference power mismatch for {row["id"]} at {table}:{index}')
            if row['source_name'] not in cells:
                raise ValueError(f'Reference name mismatch for {row["id"]}')
            label = cells[description_index-1]
            band = grade_band(label)
            if unique and description_index == UNIQUE_DESCRIPTION_COLUMN:
                band = grade_band(cells[UNIQUE_GRADE_COLUMN]) or band
            candidates.append(dict(table=table, row=index, label=label, grade_band=band,
                                   capacity=target_capacity(label), description=description))
        row['targets'], row['target_evidence'] = select_target(candidates, row['grade'])
    return dict(pages=sources, bindings_sha256=hashlib.sha256(bindings_path.read_bytes()).hexdigest())


def attenuation(count, kind, parameter):
    if kind == 'reciprocal':
        return 1/(1+parameter*(count-1))
    if kind == 'power':
        return count**(-parameter)
    raise ValueError(kind)


def fit_curve(rows, kind):
    ranked = []
    for step in range(PARAMETER_STEPS+1):
        parameter = step*PARAMETER_STEP_SIZE
        predictions = [r['base']*attenuation(r['targets'], kind, parameter) for r in rows]
        metrics = analyze.metrics(rows, predictions)
        ranked.append((metrics['median_percent_error']+.25*metrics['mean_percent_error'], parameter, metrics))
    _, parameter, metrics = min(ranked)
    return dict(kind=kind, parameter=parameter, metrics=metrics)


def validate_curve(rows, kind, groups):
    """Retune the area parameter without the held-out area skill/page."""
    values, held_rows, folds = [], [], []
    for key in sorted(set(groups)):
        train = [r for r, group in zip(rows, groups) if group != key]
        held = [r for r, group in zip(rows, groups) if group == key]
        model = fit_curve(train, kind)
        predictions = [r['base']*attenuation(r['targets'], kind, model['parameter']) for r in held]
        held_rows.extend(held)
        values.extend(predictions)
        folds.append(dict(group=str(key), parameter=model['parameter'],
                          metrics=analyze.metrics(held, predictions)))
    return dict(metrics=analyze.metrics(held_rows, values), folds=folds)


def matching_pairs(rows):
    """Matched numeric inputs reduce dependence on the fitted base estimator.

    These remain different skills, not controlled measurements of one skill
    with different occupied-target counts. Element/version/other fields differ.
    """
    pairs = []
    for area, single in itertools.product([r for r in rows if r['targets'] and r['targets'] > 1],
                                          [r for r in rows if r['targets'] == 1]):
        if all(area['x'][offset] == single['x'][offset] for offset in PAIR_OFFSETS):
            pairs.append(dict(area_id=area['id'], single_id=single['id'], area_name=area['name'],
                              single_name=single['name'], targets=area['targets'],
                              area_power=area['reference'], single_power=single['reference'],
                              ratio=area['reference']/single['reference']))
    return pairs


def investigate(rows):
    names = list(structured.BASE_TERMS)
    for row in rows:
        values = structured.prepare(row)
        row['features'] = [structured.BASE_TERMS[name](values) for name in names]
    single = [r for r in rows if r['targets'] == 1]
    area = [r for r in rows if r['targets'] and r['targets'] > 1]
    base = structured.search(single)[0]
    for row in rows:
        row['base'] = structured.predict(row, base)
    fixed = {
        'no attenuation': lambda n: 1,
        '1/N': lambda n: 1/n,
        '1/sqrt(N)': lambda n: 1/math.sqrt(n),
        'supplied STAT ratio applied to P (hypothesis)': lambda n: SUPPLIED_STAT_COEFFICIENTS[n]/SINGLE_STAT_COEFFICIENT,
        'vipwo STAT ratio applied to P (hypothesis)': lambda n: VIPWO_STAT_COEFFICIENTS[n]/SINGLE_STAT_COEFFICIENT,
    }
    curves = []
    for kind in ('reciprocal', 'power'):
        model = fit_curve(area, kind)
        model['leave_skill_out'] = validate_curve(area, kind, [r['id'] for r in area])
        model['leave_area_page_out'] = validate_curve(area, kind, [r['source'] for r in area])
        curves.append(model)
    grouped = []
    for count in sorted({r['targets'] for r in area}):
        subset = [r for r in area if r['targets'] == count]
        ratios = [r['reference']/r['base'] for r in subset]
        grouped.append(dict(targets=count, skills=len(subset), median_ratio=statistics.median(ratios),
                            minimum=min(ratios), maximum=max(ratios)))
    return dict(
        counts=dict(single=len(single), area=len(area), unknown=len(rows)-len(single)-len(area)),
        base_model=dict(terms=[names[i] for i in base['indices']], fit=base),
        fixed_penalties={name: analyze.metrics(area, [r['base']*f(r['targets']) for r in area]) for name, f in fixed.items()},
        fitted_curves=curves, ratios_by_capacity=grouped, matched_inputs=matching_pairs(rows),
        skills=[dict(id=r['id'], name=r['name'], reference=r['reference'], targets=r['targets'],
                     base=r['base'], ratio=r['reference']/r['base'], raw=structured.prepare(r),
                     source=r['source'], evidence=r['target_evidence']) for r in rows],
    )


def write_report(result, output):
    output.mkdir(parents=True, exist_ok=True)
    (output/'analysis.json').write_text(json.dumps(result, ensure_ascii=False, indent=2)+'\n', encoding='utf-8')
    body = ['<!doctype html><meta charset="utf-8"><title>Area skill power research</title>',
            '<style>body{font:15px system-ui;margin:24px}table{border-collapse:collapse}td,th{padding:8px;border:1px solid #888}pre{white-space:pre-wrap}</style>',
            '<h1>Target capacity and total skill power</h1>',
            '<p>Research estimates, not recovered server rules. P is the total for one target across all contacts. Target capacities come from saved wiki tables, independently of Skill.dat shape codes. They are not measured occupied-target counts.</p>',
            '<p>The base model is selected/fitted only on single-target rows. Area parameter validation holds out area rows/pages; the base model remains fixed. Internal reuse of these references is not independent live validation.</p>',
            '<h2>Area attacks</h2><table><tr><th>ID</th><th>Name</th><th>Targets</th><th>Reference P</th><th>Estimated unattenuated P</th><th>Ratio</th></tr>']
    for row in result['skills']:
        if not row['targets'] or row['targets'] == 1:
            continue
        cells = (row['id'], row['name'], row['targets'], row['reference'], round(row['base'], 3), round(row['ratio'], 4))
        body.append('<tr>'+''.join('<td>'+html.escape(str(value))+'</td>' for value in cells)+'</tr>')
    body.extend(['</table><h2>Methods and results</h2><pre>',
                 html.escape(json.dumps({k: v for k, v in result.items() if k != 'skills'}, indent=2)), '</pre>'])
    (output/'report.html').write_text('\n'.join(body), encoding='utf-8')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--pages', type=Path, default=REFERENCE_PAGES)
    parser.add_argument('--output', type=Path, default=Path('var/reports/skill-power-area'))
    args = parser.parse_args()
    rows, _, _, provenance = raw.raw_data()
    provenance['target_references'] = reference_targets(rows, args.pages)
    result = investigate(rows)
    result['provenance'] = provenance
    write_report(result, args.output)
    summary = {k: result[k] for k in ('counts', 'base_model', 'fixed_penalties', 'ratios_by_capacity')}
    summary['fitted_curves'] = [dict(kind=c['kind'], parameter=c['parameter'], fit=c['metrics'],
                                    leave_skill_out=c['leave_skill_out']['metrics'],
                                    leave_area_page_out=c['leave_area_page_out']['metrics'])
                                for c in result['fitted_curves']]
    print(json.dumps(summary, indent=2))


if __name__ == '__main__':
    main()

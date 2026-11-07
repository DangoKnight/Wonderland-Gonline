"""Compare a read-only Legacy ODS sample with repository server definitions.

The original workbook is never modified. Server estimates assume landed,
standalone attacks without buffs, criticals, defending or equipment procs.
Research P coefficients are frozen from the earlier investigation, not fitted
here. The separate stat-multiplier variants are examined on this sample.
The label 'current server' is the historical pre-2026-11-06-update baseline,
retained so earlier comparisons remain reproducible. See docs/DAMAGE.md for
the subsequently implemented model.
"""
import argparse
import collections
import gzip
import hashlib
import html
import json
import math
from pathlib import Path
import re
import sqlite3
import statistics
import xml.etree.ElementTree as ET
import zipfile

NS = {'table': 'urn:oasis:names:tc:opendocument:xmlns:table:1.0',
      'text': 'urn:oasis:names:tc:opendocument:xmlns:text:1.0',
      'office': 'urn:oasis:names:tc:opendocument:xmlns:office:1.0'}
SERVER_SEED = Path('internal/assetsql/server_defaults/catalog.db.gz')
NONE, EARTH, WATER, FIRE, WIND = 0, 1, 2, 3, 4
ELEMENTAL_FACTORS = ((1, 1, 1, 1, 1), (1, .9, 1.3, 1, .6), (.9, .5, .9, 1.4, .9),
                     (1.3, 1.2, .7, 1.3, 1.6), (1, 1.3, 1, .7, 1))
SINGLE_COEFFICIENT, LOW_PHYSICAL_COEFFICIENT = 1.4, .599
DEFENSE_COEFFICIENT, HIGH_DEFENSE_COEFFICIENT = .98, .179
RANDOM_ROLLS = range(1, 6)  # Rules.Next(1, 6) has an exclusive upper bound.
ROUNDING_TOLERANCE = 1e-9
MAGIC_DEFENSE_MDEF_ONLY = 'mdef-only'
MAGIC_DEFENSE_LEGACY_MAX = 'legacy-max'
STAT_ROWS = {'ATK': 'atk', 'MAT': 'matk', 'DEF': 'def', 'MDF': 'mdef', 'Level': 'level'}
# Explicit user clarifications; the ODS itself does not encode these.
STAR_SKILL_NAME = 'Star'
UNCERTAIN_TARGET_CELL = ('Sheet3', 'D24')
ALTERNATIVE_TARGET_NAME = 'Straw Chicken'
FIXED_STRUCTURED_COEFFICIENTS = ((2.30830617230515, .32055304972251586, 1.81624305326342),
                                 (1.2643960336972326, 1.7849786840668838, .2775461274580808))


def column_name(index):
    result = ''
    while index:
        index, remainder = divmod(index-1, 26)
        result = chr(65+remainder)+result
    return result


def read_ods(path):
    with zipfile.ZipFile(path) as archive:
        root = ET.fromstring(archive.read('content.xml'))
    sheets = []
    for sheet in root.findall('.//table:table', NS):
        cells, row_index = {}, 1
        for row in sheet.findall('table:table-row', NS):
            row_repeat = int(row.get('{'+NS['table']+'}number-rows-repeated', '1'))
            column_index = 1
            for cell in row:
                if cell.tag not in ('{'+NS['table']+'}table-cell', '{'+NS['table']+'}covered-table-cell'):
                    continue
                column_repeat = int(cell.get('{'+NS['table']+'}number-columns-repeated', '1'))
                text = '\n'.join(''.join(p.itertext()) for p in cell.findall('text:p', NS))
                if text:
                    if row_repeat != 1 or column_repeat != 1:
                        raise ValueError('Populated repeated cells require explicit handling')
                    cells[f'{column_name(column_index)}{row_index}'] = text
                column_index += column_repeat
            row_index += row_repeat
        sheets.append(dict(name=sheet.get('{'+NS['table']+'}name'), cells=cells))
    return sheets


def seed_definitions():
    connection = sqlite3.connect(':memory:')
    connection.deserialize(gzip.decompress(SERVER_SEED.read_bytes()))
    connection.row_factory = sqlite3.Row
    skills = {r['value_name']: dict(r) for r in connection.execute('SELECT * FROM catalog_skills')}
    npcs = {r['value_name']: dict(r) for r in connection.execute('SELECT * FROM catalog_n_p_cs')}
    connection.close()
    return skills, npcs


def enemy_stats(npc):
    level, element = npc['value_level'], npc['value_element']
    con, wis = npc['value_stats1'], npc['value_stats3']
    return dict(id=npc['value_id'], name=npc['value_name'], level=level, element=element,
                defense=round((2.6 if element == EARTH else 2)*level+(2 if element == NONE else 1.75)*con),
                magic_defense=round((2.2 if element == EARTH else 2)*level+(2 if element == NONE else 2.2)*wis),
                stats=[npc[f'value_stats{i}'] for i in range(5)])


def predictions(actor, enemy, skill, grade, formula):
    maximum = skill['value_maximum_grade'] if skill['value_maximum_grade_present'] else 10
    grade = min(maximum, max(1, grade))
    h, d, s = (skill['value_'+key] for key in ('additional_damage', 'power_per_level', 'stat_multiplier'))
    contacts = max(1, skill['value_contact_events'])
    power = (h+d*grade)*contacts
    stat_multiplier = s
    if formula == 'native total P, no stat multiplier':
        power, stat_multiplier = h+d*grade, 1
    elif formula.startswith('frozen structured P'):
        a, b, c = FIXED_STRUCTURED_COEFFICIENTS[int(s >= 1)]
        power = a*h*s + b*d*max(0, grade-1)*s*s + c*d*math.sqrt(grade)
        if formula.endswith('no stat multiplier'):
            stat_multiplier = 1
    elif formula != 'current server':
        raise ValueError(formula)
    policy = MAGIC_DEFENSE_MDEF_ONLY if formula == 'current server' else MAGIC_DEFENSE_LEGACY_MAX
    return damage_from_power(actor, enemy, skill, power, stat_multiplier, policy)


def damage_from_power(actor, enemy, skill, power, stat_multiplier, magic_defense_policy=MAGIC_DEFENSE_MDEF_ONLY):
    """Keep power/grade experiments separate from the damage pipeline."""
    magical = skill['value_effect_layer'] == 2
    offense, defense = (actor['matk'], enemy['magic_defense']) if magical else (actor['atk'], enemy['defense'])
    if magical:
        if magic_defense_policy == MAGIC_DEFENSE_LEGACY_MAX:
            defense = max(enemy['defense'], enemy['magic_defense'])
        elif magic_defense_policy != MAGIC_DEFENSE_MDEF_ONLY:
            raise ValueError(f'Unknown magic-defense policy: {magic_defense_policy}')
    coefficient, correction = SINGLE_COEFFICIENT, DEFENSE_COEFFICIENT
    if not magical and offense < defense:
        coefficient, correction = LOW_PHYSICAL_COEFFICIENT, HIGH_DEFENSE_COEFFICIENT
    base = coefficient*offense*stat_multiplier + power - correction*defense
    element = skill['value_element'] or actor['element']
    factor = ELEMENTAL_FACTORS[element][enemy['element']]
    pre_random = max(1, math.floor(base*factor+ROUNDING_TOLERANCE))
    return dict(power=power, base=base, element_factor=factor,
                minimum=pre_random+min(RANDOM_ROLLS), maximum=pre_random+max(RANDOM_ROLLS),
                midpoint=pre_random+statistics.mean(RANDOM_ROLLS))


FORMULAS = ('current server', 'native total P, no stat multiplier',
            'frozen structured P, native stat multiplier', 'frozen structured P, no stat multiplier')


def compare(sheets, star_grade):
    skills, npcs = seed_definitions()
    rows, actors, enemies, selected_skills = [], [], {}, {}
    for sheet in sheets:
        cells = sheet['cells']
        actor = dict(sheet=sheet['name'], name=cells.get('B2', 'unnamed actor'))
        actor['element'] = {'Earth': EARTH, 'Water': WATER, 'Fire': FIRE, 'Wind': WIND}[cells['B3'].split()[1]]
        for source, key in STAT_ROWS.items():
            actor[key] = int(next(v.split()[1] for k, v in cells.items() if k.startswith('B') and v.startswith(source+' ')))
        actors.append(actor)
        indices = sorted(int(k[1:]) for k in cells if k.startswith('D'))
        for index in indices:
            label, target, damage = (cells[f'{col}{index}'] for col in 'BCD')
            parsed = re.fullmatch(r'(.+?)\s+(\d+)', label)
            name, grade = (parsed[1], int(parsed[2])) if parsed else (label, star_grade if label == STAR_SKILL_NAME else None)
            if grade is None:
                raise ValueError(f'Unspecified skill grade: {sheet["name"]}:{label}')
            skill = skills[name]
            # This sample has no area skills; do not substitute capacity guesses.
            if any(skill[f'value_native_pattern{o}'] not in (0, 1) for o in (109, 110, 111, 112)):
                raise ValueError(f'Area skill in single-target sample: {name}')
            enemy = enemy_stats(npcs[target])
            enemies[target], selected_skills[name] = enemy, skill
            hits = [int(value.strip()) for value in damage.split('+')]
            row = dict(sheet=sheet['name'], cell=f'D{index}', label=label, skill=name, skill_id=skill['value_id'],
                       grade=grade, actor=actor, target=target, enemy=enemy, contacts=hits, observed=sum(hits),
                       predicted={f: predictions(actor, enemy, skill, grade, f) for f in FORMULAS})
            if (sheet['name'], row['cell']) == UNCERTAIN_TARGET_CELL:
                alternative = enemy_stats(npcs[ALTERNATIVE_TARGET_NAME])
                row['uncertain_target'] = dict(name=ALTERNATIVE_TARGET_NAME,
                    predicted={f: predictions(actor, alternative, skill, grade, f) for f in FORMULAS})
            rows.append(row)
    groups = collections.defaultdict(list)
    for row in rows:
        if 'uncertain_target' not in row:
            groups[row['sheet'], row['skill'], row['grade'], row['target']].append(row)
    comparisons = []
    for key, values in groups.items():
        predicted = values[0]['predicted']
        observations = [r['observed'] for r in values]
        comparisons.append(dict(sheet=key[0], skill=key[1], grade=key[2], target=key[3], count=len(values),
                                observed=observations, mean=statistics.mean(observations), predicted=predicted))
    certain = [r for r in rows if 'uncertain_target' not in r]
    def metrics(subset):
        summary = {}
        for formula in FORMULAS:
            errors = [abs(r['predicted'][formula]['midpoint']/r['observed']-1)*100 for r in subset]
            summary[formula] = dict(n=len(subset), median_percent_error=statistics.median(errors),
                                   mean_percent_error=statistics.mean(errors),
                                   within_range=sum(r['predicted'][formula]['minimum'] <= r['observed'] <= r['predicted'][formula]['maximum'] for r in subset))
        return summary
    layers = {label: metrics([r for r in certain if selected_skills[r['skill']]['value_effect_layer'] == layer])
              for label, layer in (('physical', 1), ('magical', 2))}
    return dict(actors=actors, enemies=enemies, skills=selected_skills, rows=rows, groups=comparisons,
                summary=metrics(certain), by_layer=layers)


def write_report(result, directory):
    directory.mkdir(parents=True, exist_ok=True)
    (directory/'comparison.json').write_text(json.dumps(result, ensure_ascii=False, indent=2)+'\n', encoding='utf-8')
    text = ['<!doctype html><meta charset="utf-8"><title>Sample 1 damage comparison</title>',
            '<style>body{font:15px system-ui;margin:24px;max-width:1400px}table{border-collapse:collapse}td,th{border:1px solid #aaa;padding:8px}pre{white-space:pre-wrap}</style>',
            '<h1>Sample 1: Legacy measurements and Gonline estimates</h1>',
            '<p>Numbers are total damage across all contacts. Recorded stats are used directly. Enemy defenses come from current repository definitions and formulas, not a Legacy server stat capture. Star grade 8 was confirmed by the user. No buffs, combos or criticals were used.</p>',
            '<p>Research P estimates reuse previously fitted coefficients without fitting these observations. The choice to omit the extra attacking-stat multiplier is explored on this sample, so the complete variant is not a blind holdout. Substituting actual grades into the maximum-grade fit is exploratory extrapolation. All listed skills are single-target, so this sample does not measure an area penalty.</p>',
            '<table><tr><th>Sheet</th><th>Skill / grade</th><th>Target</th><th>Legacy totals</th><th>Current server range</th><th>Frozen P, native S</th><th>Frozen P, S omitted</th></tr>']
    for row in result['groups']:
        ranges = [f'{row["predicted"][f]["minimum"]}–{row["predicted"][f]["maximum"]}'
                  for f in (FORMULAS[0], FORMULAS[2], FORMULAS[3])]
        cells = [row['sheet'], f'{row["skill"]} {row["grade"]}', row['target'], ', '.join(map(str, row['observed'])), *ranges]
        text.append('<tr>'+''.join('<td>'+html.escape(str(v))+'</td>' for v in cells)+'</tr>')
    text.append('</table><h2>Uncertain target</h2>')
    uncertain = [r for r in result['rows'] if 'uncertain_target' in r]
    text.append('<p>Sheet3 D24 recorded 203 against Hyperkinetic Chicken. The user later said this could have been Straw Chicken. The measurement is retained and excluded from aggregate error metrics.</p>')
    for row in uncertain:
        for name, predictions_by_model in ((row['target'], row['predicted']),
                                            (row['uncertain_target']['name'], row['uncertain_target']['predicted'])):
            values = {f: f'{p["minimum"]}–{p["maximum"]}' for f, p in predictions_by_model.items()}
            text.append('<pre>'+html.escape(json.dumps(dict(target=name, observed=row['observed'], predictions=values), indent=2))+'</pre>')
    text.append('<h2>Metrics and provenance</h2><pre>'+html.escape(json.dumps(dict(summary=result['summary'], by_layer=result['by_layer'], provenance=result['provenance']), indent=2))+'</pre>')
    (directory/'report.html').write_text('\n'.join(text), encoding='utf-8')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--sample', type=Path, default=Path('var/damage/samples/sample1.ods'))
    parser.add_argument('--star-grade', type=int, required=True, help='explicit user-confirmed grade missing from the ODS')
    parser.add_argument('--output', type=Path, default=Path('var/reports/skill-power-sample1'))
    args = parser.parse_args()
    result = compare(read_ods(args.sample), args.star_grade)
    result['provenance'] = dict(sample=str(args.sample), sample_sha256=hashlib.sha256(args.sample.read_bytes()).hexdigest(),
                                repository_seed_sha256=hashlib.sha256(SERVER_SEED.read_bytes()).hexdigest(),
                                star_grade=args.star_grade, uncertain_target='Sheet3 D24 may have been Straw Chicken',
                                assumptions='Landed standalone attacks; no buffs, combos, criticals, defending or extra equipment effects.')
    write_report(result, args.output)
    print(json.dumps(dict(summary=result['summary'], enemies=result['enemies'],
                         groups=[{k: v for k, v in g.items() if k != 'predicted'} | {'predicted': {f: (p['minimum'], p['maximum']) for f, p in g['predicted'].items()}}
                                 for g in result['groups']]), indent=2))


if __name__ == '__main__':
    main()

"""Read the completed bug workbook without editing it; compare damage models."""
import argparse
import collections
import hashlib
import html
import json
from pathlib import Path
import re
import statistics
import xml.etree.ElementTree as ET
import zipfile

import sample_compare as sample

XLSX = {'s':'http://schemas.openxmlformats.org/spreadsheetml/2006/main',
        'r':'http://schemas.openxmlformats.org/officeDocument/2006/relationships'}
# Explicit user clarification for the two blank target cells; source stays intact.
TARGET_CLARIFICATIONS = {('DangoKnght', 22): 'Female Bug', ('DangoKnght', 23): 'Female Bug'}
POWER_MODELS = {
    '2H+.5DG total, S omitted': lambda h, d, g: 2*h+.5*d*g,
    '2H+2DG total, S omitted': lambda h, d, g: 2*h+2*d*g,
    '2H+2D(G-1) total, S omitted': lambda h, d, g: 2*h+2*d*max(0,g-1),
    '2H+D(2G-1) total, S omitted': lambda h, d, g: 2*h+d*(2*g-1),
}


def read_xlsx(path):
    with zipfile.ZipFile(path) as archive:
        shared = []
        if 'xl/sharedStrings.xml' in archive.namelist():
            shared = [''.join(r.itertext()) for r in ET.fromstring(archive.read('xl/sharedStrings.xml')).findall('s:si',XLSX)]
        workbook = ET.fromstring(archive.read('xl/workbook.xml'))
        relationships = {r.get('Id'):r.get('Target') for r in ET.fromstring(archive.read('xl/_rels/workbook.xml.rels'))}
        sheets = []
        for sheet in workbook.findall('s:sheets/s:sheet',XLSX):
            target = relationships[sheet.get('{'+XLSX['r']+'}id')]
            target = target.lstrip('/') if target.startswith('/') else 'xl/'+target
            root = ET.fromstring(archive.read(target))
            cells = {}
            for cell in root.findall('.//s:sheetData/s:row/s:c',XLSX):
                value = cell.find('s:v',XLSX)
                value = value.text if value is not None else None
                if cell.get('t') == 's' and value is not None:
                    value = shared[int(value)]
                elif cell.get('t') == 'inlineStr':
                    value = ''.join(cell.find('s:is',XLSX).itertext())
                if value not in (None, ''):
                    cells[cell.get('r')] = value
            sheets.append(dict(name=sheet.get('name'),cells=cells))
    return sheets


def damage_values(text):
    parts = re.split(r'\s*\+\s*', text.strip())
    if not parts or any(not re.fullmatch(r'\d+', p) for p in parts):
        raise ValueError(f'Invalid damage observation: {text!r}')
    return list(map(int, parts))


def metrics(rows, name):
    errors = [abs(r['predictions'][name]['midpoint']/r['observed']-1)*100 for r in rows]
    return dict(n=len(rows),mean_percent_error=statistics.mean(errors),median_percent_error=statistics.median(errors),
                within_range=sum(r['predictions'][name]['minimum'] <= r['observed'] <= r['predictions'][name]['maximum'] for r in rows))


def investigate(sheets):
    skills, npcs = sample.seed_definitions()
    rows, actors, selected, enemies = [], [], {}, {}
    for sheet in sheets:
        cells = sheet['cells']
        actor = dict(name=sheet['name'], level=int(cells['E5']),
                     element={'Fire':sample.FIRE,'Water':sample.WATER}[cells['C5']],
                     atk=int(cells['E7']),matk=int(cells['C8']),**{'def':int(cells['G7']),'mdef':int(cells['E8'])})
        actors.append(actor)
        for index in sorted({int(address[1:]) for address in cells if re.fullmatch(r'E\d+',address) and int(address[1:]) >= 16}):
            if f'B{index}' not in cells:
                raise ValueError(f'Damage without skill: {sheet["name"]}:E{index}')
            name, grade = cells[f'B{index}'], int(cells[f'C{index}'])
            target = cells.get(f'D{index}') or TARGET_CLARIFICATIONS.get((sheet['name'],index))
            if not target:
                raise ValueError(f'Missing target: {sheet["name"]}:D{index}')
            skill, enemy = skills[name], sample.enemy_stats(npcs[target])
            if skill['value_effect_layer'] != 2:
                raise ValueError(f'Expected magical skill: {name}')
            if any(skill[f'value_native_pattern{o}'] not in (0,1) for o in (109,110,111,112)):
                raise ValueError('This comparison needs explicit handling for area skills')
            hits = damage_values(cells[f'E{index}'])
            notes = cells.get(f'F{index}','')
            predictions = {model:sample.predictions(actor,enemy,skill,grade,model) for model in sample.FORMULAS}
            h, d = skill['value_additional_damage'], skill['value_power_per_level']
            for model, formula in POWER_MODELS.items():
                predictions[model] = sample.damage_from_power(actor,enemy,skill,formula(h,d,grade),1,
                                                              sample.MAGIC_DEFENSE_LEGACY_MAX)
            factor = predictions['current server']['element_factor']
            inferred = {}
            for label, multiplier in (('native S',skill['value_stat_multiplier']),('S omitted',1)):
                inferred[label] = (sum(hits)-statistics.mean(sample.RANDOM_ROLLS))/factor
                inferred[label] += sample.DEFENSE_COEFFICIENT*max(enemy['defense'],enemy['magic_defense'])
                inferred[label] -= sample.SINGLE_COEFFICIENT*actor['matk']*multiplier
            rows.append(dict(sheet=sheet['name'],cell=f'E{index}',actor=actor,skill=name,skill_id=skill['value_id'],
                             grade=grade,target=target,enemy=enemy,contacts=hits,observed=sum(hits),notes=notes,
                             critical_flag='critical' in notes.lower(),target_clarified=f'D{index}' not in cells,
                             predictions=predictions,implied_power=inferred))
            selected[name],enemies[target] = skill,enemy
    normal = [r for r in rows if not r['critical_flag']]
    model_names = list(normal[0]['predictions'])
    power_diagnostics = {}
    for name in model_names:
        errors = [abs(r['predictions'][name]['power']/r['implied_power']['S omitted']-1)*100 for r in normal]
        power_diagnostics[name] = dict(n=len(normal),mean_percent_error=statistics.mean(errors),
                                       median_percent_error=statistics.median(errors))
    grouped = collections.defaultdict(list)
    for row in normal:
        grouped[row['sheet'],row['skill'],row['grade'],row['target']].append(row)
    groups = [dict(sheet=key[0],skill=key[1],grade=key[2],target=key[3],count=len(values),
                   observations=[r['observed'] for r in values],mean=statistics.mean(r['observed'] for r in values),
                   predictions=values[0]['predictions']) for key,values in grouped.items()]
    return dict(actors=actors,enemies=enemies,skills=selected,rows=rows,groups=groups,
                summary={name:metrics(normal,name) for name in model_names},
                inferred_power_comparisons=power_diagnostics,
                by_actor={a['name']:{name:metrics([r for r in normal if r['sheet']==a['name']],name) for name in model_names} for a in actors},
                negative_native_power=[dict(sheet=r['sheet'],cell=r['cell'],skill=r['skill'],power=r['implied_power']['native S']) for r in normal if r['implied_power']['native S'] < 0])


def write_report(result, output):
    output.mkdir(parents=True,exist_ok=True)
    (output/'analysis.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    body = ['<!doctype html><meta charset="utf-8"><title>Saved bug measurements</title>',
            '<style>body{font:15px system-ui;margin:24px;max-width:1500px}td,th{padding:8px;border:1px solid #aaa}table{border-collapse:collapse}pre{white-space:pre-wrap}</style>',
            '<h1>Bug damage measurements</h1>',
            '<p>17 recorded attacks: 16 ordinary comparisons and one marked likely critical. Both blank Fire targets were confirmed Female Bug by the user. The source workbook is not edited. Estimates use current Gonline enemy defenses and the server random roll 1–5; exact Legacy defenses and noise remain unverified.</p>',
            '<p>All seven skills are magical and single-target. Frozen structured P coefficients come from the earlier reference-table investigation; the simple magic alternatives are exploratory, not original-server rules.</p>',
            '<table><tr><th>Character</th><th>Skill / grade</th><th>Target</th><th>Legacy totals</th><th>Current Gonline</th><th>Frozen P, native S</th><th>Frozen P, S omitted</th><th>2H+2DG, S omitted</th></tr>']
    displayed = ('current server',sample.FORMULAS[2],sample.FORMULAS[3],'2H+2DG total, S omitted')
    for row in result['groups']:
        values = [row['sheet'],f'{row["skill"]} {row["grade"]}',row['target'],', '.join(map(str,row['observations']))]
        for name in displayed:
            p = row['predictions'][name]
            values.append(f'{p["minimum"]}–{p["maximum"]}')
        body.append('<tr>'+''.join('<td>'+html.escape(str(v))+'</td>' for v in values)+'</tr>')
    body.append('</table><h2>Critical note</h2><p>Fire Combo Beating at E19 is 2346 + 2346 = 4692, marked “Likely Critical”. Half is 2346, matching the ordinary 2346 and 2345 totals. It is preserved and excluded from ordinary-error metrics.</p>')
    body.append('<p>Small total-damage errors at high MATK do not imply equally accurate P values. The inferred-power comparison reverses the assumed 1.4*MATK pipeline using the midpoint roll; it inherits uncertainty in enemy defenses, elemental factors and Legacy rounding.</p>')
    body.append('<h2>Metrics and evidence</h2><pre>'+html.escape(json.dumps({k:v for k,v in result.items() if k not in ('rows','groups','skills')},indent=2))+'</pre>')
    (output/'report.html').write_text('\n'.join(body),encoding='utf-8')


def low_matk_transfer(path):
    comparison = sample.compare(sample.read_ods(path),8)
    model = '2H+2DG total, S omitted'
    rows = [r for r in comparison['rows'] if comparison['skills'][r['skill']]['value_effect_layer']==2]
    details = []
    for row in rows:
        skill = comparison['skills'][row['skill']]
        power = POWER_MODELS[model](skill['value_additional_damage'],skill['value_power_per_level'],row['grade'])
        row['predictions'] = {model:sample.damage_from_power(row['actor'],row['enemy'],skill,power,1,
                                                           sample.MAGIC_DEFENSE_LEGACY_MAX)}
        details.append(dict(skill=row['skill'],grade=row['grade'],target=row['target'],observed=row['observed'],
                            prediction=row['predictions'][model]))
    return dict(source=str(path),source_sha256=hashlib.sha256(path.read_bytes()).hexdigest(),
                model=model,metrics=metrics(rows,model),observations=details)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--sample',type=Path,default=Path('outputs/bug-damage-20261106/bug-damage-recording.xlsx'))
    parser.add_argument('--output',type=Path,default=Path('var/reports/skill-power-bugs'))
    args = parser.parse_args()
    before = hashlib.sha256(args.sample.read_bytes()).hexdigest()
    result = investigate(read_xlsx(args.sample))
    result['provenance'] = dict(source=str(args.sample),source_sha256=before,
                                repository_seed_sha256=hashlib.sha256(sample.SERVER_SEED.read_bytes()).hexdigest(),
                                clarifications='DangoKnght rows 22 and 23 target Female Bug; user confirmed in chat.')
    earlier = Path('var/damage/samples/sample1.ods')
    if earlier.exists():
        result['low_matk_transfer'] = low_matk_transfer(earlier)
    write_report(result,args.output)
    if hashlib.sha256(args.sample.read_bytes()).hexdigest() != before:
        raise RuntimeError('Source workbook changed during read-only analysis')
    print(json.dumps(dict(summary=result['summary'],by_actor=result['by_actor'],negative_native_power=result['negative_native_power'],
                         groups=[{k:v for k,v in r.items() if k!='predictions'} | {'ranges':{m:(r['predictions'][m]['minimum'],r['predictions'][m]['maximum']) for m in ('current server',sample.FORMULAS[3],'2H+2DG total, S omitted')}} for r in result['groups']]),indent=2))


if __name__=='__main__':
    main()

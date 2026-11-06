"""Compare remote origin, pre-correction Gonline, current and Private Server.

Read-only source inspection and offline predictions; no server/data changes.
Private Server's actual actor-element rule and an explicitly corrected variant
are both retained. All Gonline versions resolve elements from native Skill.dat.
"""
import argparse
import collections
import csv
from datetime import datetime, timezone
import gzip
import hashlib
import html
import io
import json
import math
from pathlib import Path
import statistics
import sqlite3
import subprocess

import sample_compare as sample
import reference_compare as references

ORIGIN_MAX_GRADE = 10
ORIGIN_GRADE_GROWTH = .05
PRIVATE_MAX_GRADE = 10
NATIVE_ELEMENT_OFFSET = 26
NATIVE_BYTE_MASK = 0xfd
NATIVE_VALUE_OFFSET = 4
METHODS = ('origin', 'previous', 'pre_stat_floor', 'current', 'private', 'private_skill_element', 'private_same_defense')
PRIVATE_MATRIX = ((1,1,1,1,1),(1,1,1.7,1,.6),(1,.6,1,1.7,1),(1,1,.6,1,1.5),(1,1.7,1,.6,1))


def private_prediction(row, skill, use_skill_element=False, same_defense=False):
    magical = skill['value_effect_layer']==2
    offense = row['actor']['matk' if magical else 'atk']
    # Standard roaming encounter constructors set MonsterDef to this value;
    # both DEF and MDEF then use MonsterDef. Explicit encounter overrides are
    # outside this comparison. The extra same-defense column isolates formula.
    defense = row['enemy']['magic_defense' if magical else 'defense'] if same_defense else round(row['enemy']['level']*1.2+3)
    grade = min(PRIVATE_MAX_GRADE,max(1,row['grade']))
    base = max(1,math.floor(offense*skill['value_stat_multiplier']+grade*skill['value_power_per_level']+skill['value_additional_damage']-defense/2))
    element = row['actor']['element']
    if use_skill_element and skill['value_element'] != references.NATIVE_CASTER_ELEMENT:
        element = skill['value_element']
    if element is None:
        raise ValueError('Private Server prediction requires the confirmed actor element')
    factor = PRIVATE_MATRIX[element][row['enemy']['element']]
    value = max(1,int(base*factor))
    return dict(minimum=value,maximum=value,midpoint=value,defense=defense,element=element,element_factor=factor)


def origin_prediction(row, skill, powers):
    grade = min(ORIGIN_MAX_GRADE,max(1,row['grade']))
    base = powers.get(skill['value_id'],skill['value_additional_damage']+skill['value_power_per_level'])
    p = base*(1+(grade-1)*ORIGIN_GRADE_GROWTH)
    return sample.damage_from_power(row['actor'],row['enemy'],skill,p,1)


def current_prediction(row, skill):
    # Recalculate from the current formula, never the cached row['selected'].
    # The reference evaluator includes fading Q and native neutral/caster affinity.
    return references.prediction(row, skill, 'current')


def metrics(rows, method):
    errors = [abs(r['predictions'][method]['midpoint']/r['observed']-1)*100 for r in rows]
    return dict(casts=len(rows),mean_percent_error=statistics.mean(errors),median_percent_error=statistics.median(errors))


def audit_elements(export):
    document = json.loads(export.read_bytes())
    source = Path(document['source'])
    native = source.read_bytes()
    checksum = hashlib.sha256(native).hexdigest()
    if checksum != document['source_sha256']:
        raise ValueError('Native Skill.dat differs from the recorded export')
    connection=sqlite3.connect(':memory:')
    connection.deserialize(gzip.decompress(sample.SERVER_SEED.read_bytes()))
    byid=dict(connection.execute('SELECT value_id,value_element FROM catalog_skills'))
    connection.close()
    for record in document['records']:
        element = ((native[record['file_offset']+NATIVE_ELEMENT_OFFSET]^NATIVE_BYTE_MASK)-NATIVE_VALUE_OFFSET)&255
        fields = record['fields']
        if fields['element'] != element or byid[fields['id']] != element:
            raise ValueError(f'Skill element mismatch: {fields["id"]}')
    return dict(records=len(document['records']),source_sha256=checksum,all_matched=True)


def analyze(reports, private_root, export):
    origin_commit = subprocess.check_output(['git','rev-parse','origin/main'],text=True).strip()
    origin_source = subprocess.check_output(['git','show','origin/main:internal/battle/damage.go'],text=True)
    if 'damageSkillPowerGradeIncrease = .05' not in origin_source:
        raise ValueError('origin/main no longer matches this comparison model; review its formula')
    csv_source = subprocess.check_output(['git','show','origin/main:internal/assetsql/skill_power_defaults.csv'],text=True,encoding='utf-8')
    powers = {int(r['skill_id']):float(r['power']) for r in csv.DictReader(io.StringIO(csv_source))}
    ps_source = private_root/'wlo.pserver.core/Game/Battle/PvEBattleManager.cs'
    ps_text = ps_source.read_text(encoding='utf-8-sig')
    if 'offense * skill.StatMultiplier + grade * skill.PowerPerLevel' not in ps_text or 'GetElementalMultiplier(actor.Element, target.Element)' not in ps_text:
        raise ValueError('Private Server changed; review its damage and element rules')
    input_bytes = (reports/'damage-selected/runtime-input.json').read_bytes()
    data = json.loads(input_bytes)
    confirmed = json.loads(Path('var/damage/samples/dangochaos-mat238.json').read_bytes())['actor']
    skills,_ = sample.seed_definitions()
    audit = audit_elements(export)
    rows = data['rows']
    for row in rows:
        if row['actor']['name']==confirmed['name']:
            row['actor']['element']=confirmed['element']
        sk=skills[row['skill']]
        row['predictions'] = dict(origin=origin_prediction(row,sk,powers),previous=row['baseline'],
                                  pre_stat_floor=references.prediction(row,sk,'pre_stat_floor'),current=current_prediction(row,sk),
                                  private=private_prediction(row,sk),private_skill_element=private_prediction(row,sk,True),
                                  private_same_defense=private_prediction(row,sk,False,True))
        row['predicted'] = {'current server': row['predictions']['current']}
    grouped = collections.defaultdict(list)
    for row in rows:
        key=(row['actor']['name'],row['actor']['level'],row['actor']['atk'],row['actor']['matk'],row['skill'],row['grade'],row['enemy']['id'])
        grouped[key].append(row)
    groups=[]
    for repeated in grouped.values():
        row=repeated[0]
        groups.append(dict(actor=row['actor'],skill=row['skill'],grade=row['grade'],target=row['enemy']['name'],
                           observed=[r['observed'] for r in repeated],observed_mean=statistics.mean(r['observed'] for r in repeated),
                           predictions=row['predictions']))
    summary = {}
    for label,keep in [('all',lambda r:True),('physical',lambda r:r['layer']==1),('magical',lambda r:r['layer']==2),
                       ('Blast Attack',lambda r:r['skill']=='Blast Attack'),
                       ('controlled MAT25/29',lambda r:r.get('source')=='controlled'),
                       ('MAT238',lambda r:r['actor']['matk']==238)]:
        selected=[r for r in rows if keep(r)]
        summary[label]={method:metrics(selected,method) for method in METHODS}
    return dict(generated_utc=datetime.now(timezone.utc).isoformat(),
                input_sha256=hashlib.sha256(input_bytes).hexdigest(),
                current_source_sha256={str(path):hashlib.sha256(path.read_bytes()).hexdigest() for path in
                                       map(Path,('internal/battle/damage.go','internal/battle/battle.go','internal/battle/targeting.go','internal/assets/skills.go'))},
                origin_commit=origin_commit,private_commit=subprocess.check_output(['git','rev-parse','HEAD'],cwd=private_root,text=True).strip(),
                private_source_sha256=hashlib.sha256(ps_source.read_bytes()).hexdigest(),element_audit=audit,
                assumptions=['Origin main is the recorded origin/main revision shown above; this command does not fetch a newer remote revision.',
                             'Previous current means the saved native-P model before any quadratic stat correction. Updated current is freshly recalculated, including the latest fade and element rules.',
                             'Updated current includes the low-MAT single-target magic stat floor. The earlier element/fade changes alone did not change these 91 observations.',
                             'Same measured actor ATK/MAT for every version. Gonline uses the same native-derived target DEF/MDEF estimates throughout.',
                             'Main Private Server columns use its standard roaming MonsterDef=round(1.2*level+3), also used as MDEF. No encounter overrides.',
                             'Private Server as-is uses actor element; corrected variant uses Skill.dat element. No Private Server files are modified.',
                             'Extra same-defense Private Server column uses Gonline target defenses to isolate the damage-equation difference.',
                             'Skill damage in Private Server is deterministic here: basicRoll is ignored for skills. All models exclude combos, buffs and critical/uncertain casts.',
                             'Gonline entries are 1..5-noise ranges. Error compares their midpoint against each recorded cast, not the closest endpoint.'],
                summary=summary,groups=groups,source_examples=references.source_checks(),
                source_references=references.references(Path('var/damage/references')),
                runtime_input=dict(rows=rows,skills=skills))


def format_prediction(value):
    return str(value['minimum']) if value['minimum']==value['maximum'] else f"{value['minimum']}–{value['maximum']}"


def write_report(result,output):
    output.mkdir(parents=True,exist_ok=True)
    (output/'analysis.json').write_text(json.dumps({k:v for k,v in result.items() if k!='runtime_input'},ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    (output/'runtime-input.json').write_text(json.dumps(result['runtime_input'],ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    labels={'origin':'Origin main','previous':'Previous: before Q','pre_stat_floor':'Previous: before stat floor','current':'Updated current','private':'Private Server as-is','private_skill_element':'Private Server: skill element corrected','private_same_defense':'Private Server: same target defenses'}
    parts=['<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Updated damage comparison</title><style>body{font:15px system-ui;margin:24px;max-width:1700px;color:#17202a;background:#fff}table{border-collapse:collapse}th,td{border:1px solid #bbb;padding:8px}th{background:#e9eff5}pre{white-space:pre-wrap;background:#f3f6f8;padding:16px}section{overflow-x:auto}td{font-variant-numeric:tabular-nums}</style><body><h1>Updated damage comparison</h1><p>Generated '+html.escape(result['generated_utc'])+'</p><p>Origin: '+result['origin_commit']+'; Private Server: '+result['private_commit']+'</p><ul>'+''.join('<li>'+html.escape(x)+'</li>' for x in result['assumptions'])+'</ul>']
    formula = '''H = native AdditionalDamage; D = native PowerPerLevel; G = clamped native grade.
Pphysical = 1.9131446584781497 H + 0.47524025502148415 D G
Pmagic = 2(H + D G)

Single-target physical base = 1.4 ATK + P - 0.98 DEF when ATK >= DEF;
                             0.599 ATK + P - 0.179 DEF otherwise.
Single-target magic base = max(0, 1.4 MATK - 0.98 MDEF + Q) + P.
x = clamp(MATK, 24, 1052); y = clamp(MDEF, 35, 78)
Qfit = 0.244297946284 + 0.00607069954603 x + 0.00653637105727 y
       - 0.00000548960961612 x² + 0.0000874824337356 xy + 0.000170650039492 y²
t(v,u) = clamp((v-u)/u, 0, 1); w(v,u) = (1-t)²(1+2t)
Q = Qfit × w(MATK,1052) × w(MDEF,78)

For these unbuffed noncritical observations:
damage = max(1, floor(base × elemental_factor)) + random integer 1..5.
Native element 0 is neutral; 1/2/3/4 are Earth/Water/Fire/Wind; 7 uses caster affinity.
Neutral attacks have factor 0.9 against Earth, 1 against the other elements.
The fade is an extrapolation policy, not a decoded Legacy mechanic.
The stat floor is empirical: low-MAT spells retain P when defense exceeds the
combined stat contribution. It does not change P, grades or native skill data.

Area attacks (not established by these normal-cast samples):
stat coefficient 0.85 for 2 targets, 0.6 for 3–5, 0.4 for 6–8;
P multiplied by 2/(capacity+1), defense coefficient 0.98, Q = 0.
Capacity uses the skill's grade-dependent shape, not occupied target count.
Contacts divide the final total for animation; they do not multiply P.'''
    parts.append('<h2>Updated formula</h2><pre>'+html.escape(formula)+'</pre><h2>Mean absolute percentage damage error</h2><section><table><tr><th>Measurements (casts)</th>'+''.join('<th>'+v+'</th>' for v in labels.values())+'</tr>')
    for category,methods in result['summary'].items():
        cells=[f"{category} ({methods['current']['casts']})"]+[f"{methods[k]['mean_percent_error']:.3f}%" for k in METHODS]
        parts.append('<tr>'+''.join('<td>'+html.escape(str(v))+'</td>' for v in cells)+'</tr>')
    parts.append('</table></section><h2>Recorded values and expected damage</h2><section><table><tr><th>Actor / ATK / MAT</th><th>Skill / grade</th><th>Target</th><th>Recorded damage</th>'+''.join('<th>'+v+'</th>' for v in labels.values())+'</tr>')
    for g in result['groups']:
        a=g['actor'];cells=[f"{a['name']} / {a['atk']} / {a['matk']}",g['skill']+' '+str(g['grade']),g['target'],', '.join(str(x) for x in g['observed'])]
        cells.extend(format_prediction(g['predictions'][k]) for k in METHODS)
        parts.append('<tr>'+''.join('<td>'+html.escape(str(v))+'</td>' for v in cells)+'</tr>')
    parts.append('</table></section><h2>Independent high-stat reference examples</h2><p>Separate from the 91 user observations. Each calculation holds the article’s P fixed to isolate the stat term. These are continuous predictions plus mean noise 3; production floors before the random roll. Rows 12/13 deliberately differ from Legacy because this project uses MDEF only.</p><section><table><tr><th>Article row</th><th>MAT / DEF / MDEF / P</th><th>Recorded mean</th><th>Previous clamped Q</th><th>Updated fading Q</th><th>Article max(DEF,MDEF) rule</th></tr>')
    for row in result['source_examples']:
        if row['layer']!='magical':
            continue
        q=row['predictions']
        cells=[row['row'],f"{row['attack']} / {row['defense']} / {row['mdef']} / {row['power']}",f"{row['observed']:.2f}",f"{q['previous_clamped_q']:.2f}",f"{q['refined_fading_q']:.2f}",f"{q['legacy_max']:.2f}"]
        parts.append('<tr>'+''.join('<td>'+html.escape(str(v))+'</td>' for v in cells)+'</tr>')
    parts.append('</table></section><h2>Element changes (mechanical examples, not recorded hits)</h2><table><tr><th>Attack</th><th>Target</th><th>Before refinement</th><th>Updated factor</th></tr><tr><td>Neutral skill, Fire caster</td><td>Wind</td><td>1.6</td><td>1.0</td></tr><tr><td>Neutral skill, Fire caster</td><td>Earth</td><td>1.2</td><td>0.9</td></tr><tr><td>Element-7 skill, Fire caster</td><td>Wind</td><td>1.0</td><td>1.6</td></tr><tr><td>Element-7 skill, Water caster</td><td>Earth</td><td>1.0</td><td>0.5</td></tr></table>')
    parts.append('<h2>Skill.dat audit and provenance</h2><pre>'+html.escape(json.dumps({k:result[k] for k in ('element_audit','input_sha256','current_source_sha256','private_source_sha256','source_references')},ensure_ascii=False,indent=2))+'</pre></body></html>')
    (output/'report.html').write_text('\n'.join(parts),encoding='utf-8')


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--reports',type=Path,default=Path('var/reports'))
    parser.add_argument('--private-server',type=Path,default=Path('../Wonderland-Private-Server'))
    parser.add_argument('--skill-export',type=Path,default=Path('data/skill_data.json'))
    parser.add_argument('--output',type=Path,default=Path('var/reports/four-version-damage'))
    args=parser.parse_args()
    result=analyze(args.reports,args.private_server,args.skill_export)
    write_report(result,args.output)
    print(json.dumps(result['summary'],indent=2))
    for g in result['groups']:
        if g['actor']['matk']==238:
            print(g['target'],g['observed'],{k:format_prediction(v) for k,v in g['predictions'].items()})


if __name__=='__main__':
    main()

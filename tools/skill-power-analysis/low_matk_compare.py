"""Compare controlled MAT/grade batches without modifying authored observations."""
import argparse
import collections
import hashlib
import html
import json
from pathlib import Path
import statistics

import sample_compare as sample
import raw_search

DEFAULT_SAMPLES = [Path('var/damage/samples/blast-attack-grade2-mat25.json'),
                   Path('var/damage/samples/blast-attack-grades2-3-mat29.json'),
                   Path('var/damage/samples/blast-attack-grade3-mat25.json')]


def comparisons(groups, growth):
    changes = []
    for i, earlier in enumerate(groups):
        for later in groups[i+1:]:
            if earlier['target'] != later['target'] or earlier['skill'] != later['skill']:
                continue
            if (earlier['actor']['name'],earlier['actor']['level']) != (later['actor']['name'],later['actor']['level']):
                continue
            if earlier['grade'] == later['grade'] and earlier['matk'] != later['matk']:
                kind, difference = 'MAT change', later['matk']-earlier['matk']
                factor = earlier['element_factor']
                observed = later['mean']-earlier['mean']
                expected = dict(coefficient_1_4=1.4*difference*factor,
                                coefficient_1_4_times_native_S=1.4*growth['value_stat_multiplier']*difference*factor)
            elif earlier['matk'] == later['matk'] and earlier['grade'] != later['grade']:
                kind, difference = 'grade change', later['grade']-earlier['grade']
                factor = earlier['element_factor']
                observed = later['mean']-earlier['mean']
                expected = dict(native_D=growth['value_power_per_level']*difference*factor,
                                twice_D=2*growth['value_power_per_level']*difference*factor)
            else:
                continue
            changes.append(dict(kind=kind,target=earlier['target'],start_matk=earlier['matk'],end_matk=later['matk'],
                                start_grade=earlier['grade'],end_grade=later['grade'],observed_mean_change=observed,
                                normalized_change_per_unit=observed/(factor*difference),expected_change_before_rounding_noise=expected,
                                other_stat_changes={name:(earlier['actor'].get(name),later['actor'].get(name))
                                                    for name in ('atk','def','mdef','spd')
                                                    if earlier['actor'].get(name)!=later['actor'].get(name)}))
    return changes


def fit_controlled_effects(groups):
    """Estimate MAT/grade slopes with independent nuisance intercepts per target.

    Uses raw-cast OLS; does not assume monster defenses or absolute P. Its
    standard errors assume independent, equal-variance residuals. Gear effects
    and nonlinearity can confound these estimates. No gameplay fitting occurs.
    """
    targets = sorted({g['target'] for g in groups})
    mat_origin,grade_origin = min(g['matk'] for g in groups),min(g['grade'] for g in groups)
    size = 2+len(targets)
    matrix = [[0.]*size for _ in range(size)]
    rhs = [0.]*size
    observations = []
    for g in groups:
        factor = g['element_factor']
        features = [factor*(g['matk']-mat_origin),factor*(g['grade']-grade_origin)]
        features += [int(g['target']==target) for target in targets]
        for value in g['observed']:
            observations.append((features,value))
            for i in range(size):
                rhs[i] += features[i]*value
                for j in range(size):
                    matrix[i][j] += features[i]*features[j]
    coefficients = raw_search.solve(matrix,rhs)
    if coefficients is None or len(observations)<=size:
        return dict(available=False,reason='Insufficient independent MAT/grade settings')
    sse = sum((value-sum(c*x for c,x in zip(coefficients,features)))**2 for features,value in observations)
    variance = sse/(len(observations)-size)
    standard_errors = []
    for index in range(2):
        unit = [float(i==index) for i in range(size)]
        inverse_column = raw_search.solve(matrix,unit)
        standard_errors.append((variance*inverse_column[index])**.5)
    return dict(available=True,casts=len(observations),parameters=size,residual_degrees_of_freedom=len(observations)-size,
                mat_origin=mat_origin,grade_origin=grade_origin,
                coefficient_per_MAT_before_element=coefficients[0],standard_error_MAT=standard_errors[0],
                additive_power_per_grade=coefficients[1],standard_error_grade=standard_errors[1],
                target_intercepts=dict(zip(targets,coefficients[2:])),residual_standard_deviation=variance**.5,
                caution='Exploratory raw-cast OLS with separate target intercepts; assumes local linearity and independent equal-variance residuals. Equipment changes can confound slopes.')


def investigate(paths):
    skills,npcs = sample.seed_definitions()
    groups,rows,sources = [],[],[]
    for path in paths:
        content = path.read_bytes()
        raw = json.loads(content)
        sources.append(dict(file=str(path),sha256=hashlib.sha256(content).hexdigest(),actor=raw['actor']))
        buckets = collections.defaultdict(list)
        for observed in raw['observations']:
            grade = observed.get('grade',raw.get('grade'))
            if grade is None:
                raise ValueError('Missing grade in observation')
            buckets[grade,observed['target']].append(observed['damage'])
        for (grade,target),values in buckets.items():
            skill = skills[raw['skill']]
            enemy = sample.enemy_stats(npcs[target])
            predictions = {model:sample.predictions(raw['actor'],enemy,skill,grade,model) for model in sample.FORMULAS}
            magic_power = 2*(skill['value_additional_damage']+skill['value_power_per_level']*grade)
            predictions['2(H+DG), S omitted'] = sample.damage_from_power(raw['actor'],enemy,skill,magic_power,1,
                                                                       sample.MAGIC_DEFENSE_LEGACY_MAX)
            group = dict(source=str(path),skill=raw['skill'],skill_id=skill['value_id'],grade=grade,target=target,
                         matk=raw['actor']['matk'],actor=raw['actor'],enemy=enemy,observed=values,n=len(values),
                         mean=statistics.mean(values),element_factor=predictions['current server']['element_factor'],predicted=predictions)
            groups.append(group)
            for value in values:
                rows.append(dict(group,observed=value))
    growth = skills[groups[0]['skill']]
    if len({g['skill'] for g in groups}) != 1:
        raise ValueError('Controlled comparison requires one skill')
    return dict(sources=sources,groups=groups,changes=comparisons(groups,growth),observations=len(rows),
                controlled_slope_fit=fit_controlled_effects(groups),
                policy='Gonline uses MDEF; Legacy research uses max(DEF,MDEF). Current enemy stat estimates and noise 1..5 remain assumptions.')


def write_report(result,output):
    output.mkdir(parents=True,exist_ok=True)
    (output/'controlled-batches.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    body = ['<!doctype html><meta charset="utf-8"><title>Controlled Blast Attack batches</title>',
            '<style>body{font:15px system-ui;margin:24px;max-width:1300px}table{border-collapse:collapse}td,th{padding:8px;border:1px solid #aaa}pre{white-space:pre-wrap}</style>',
            '<h1>Blast Attack: MAT and grade changes</h1>',
            '<p>Original observations are preserved. Mean differences include sampling noise; four MAT points are a small separation. A constant additive P or defense correction cancels in same-target, same-grade MAT comparisons, assuming gear introduces no separate damage modifier.</p>',
            '<table><tr><th>MAT</th><th>Grade</th><th>Target</th><th>Legacy values</th><th>Mean</th><th>Current Gonline</th><th>2(H+DG), S omitted</th></tr>']
    for g in result['groups']:
        values = [g['matk'],g['grade'],g['target'],', '.join(map(str,g['observed'])),round(g['mean'],4)]
        for name in ('current server','2(H+DG), S omitted'):
            p = g['predicted'][name]
            values.append(f'{p["minimum"]}–{p["maximum"]}')
        body.append('<tr>'+''.join('<td>'+html.escape(str(v))+'</td>' for v in values)+'</tr>')
    body.append('</table><h2>Controlled changes</h2><pre>'+html.escape(json.dumps(result['changes'],indent=2))+'</pre>')
    body.append('<h2>Shared slope estimates with independent target intercepts</h2><pre>'+html.escape(json.dumps(result['controlled_slope_fit'],indent=2))+'</pre>')
    body.append('<p>The ATK change 77→74 and MDEF change 44→48 are preserved as equipment context. These observations alone do not identify every possible weapon modifier or prove a precise MAT/grade coefficient.</p>')
    (output/'controlled-batches.html').write_text('\n'.join(body),encoding='utf-8')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--samples',nargs='+',type=Path,default=DEFAULT_SAMPLES)
    parser.add_argument('--output',type=Path,default=Path('var/reports/skill-power-low-matk'))
    args = parser.parse_args()
    result = investigate(args.samples)
    write_report(result,args.output)
    print(json.dumps(dict(observations=result['observations'],groups=[{k:g[k] for k in ('matk','grade','target','observed','mean','predicted')} for g in result['groups']],changes=result['changes']),indent=2))


if __name__=='__main__':
    main()

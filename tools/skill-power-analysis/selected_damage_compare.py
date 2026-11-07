"""Compare the selected bounded magic correction to its historical baseline.

No fitting, reference edits or production writes. The previous 81 normal casts
and ten prospective MAT238 casts are replay inputs. Critical/uncertain casts
remain excluded. Runtime expectations can be checked through Battle.Round.
"""
import argparse
import json
from pathlib import Path
import statistics

import polynomial_search as power
import sample_compare as sample

ATTACK_BOUNDS = (24, 1052)
DEFENSE_BOUNDS = (35, 78)
COEFFICIENTS = (.244297946284, .00607069954603, .00653637105727,
                -.00000548960961612, .0000874824337356, .000170650039492)


def adjustment(attack, defense):
    x = min(ATTACK_BOUNDS[1], max(ATTACK_BOUNDS[0], attack))
    y = min(DEFENSE_BOUNDS[1], max(DEFENSE_BOUNDS[0], defense))
    a,b,c,d,e,f = COEFFICIENTS
    def weight(value, upper):
        t = min(1, max(0, (value-upper)/upper))
        return (1-t)**2*(1+2*t)
    return (a+b*x+c*y+d*x*x+e*x*y+f*y*y)*weight(attack, ATTACK_BOUNDS[1])*weight(defense, DEFENSE_BOUNDS[1])


def analyze(reports):
    old = json.loads((reports/'skill-power-formulas/runtime-input.json').read_bytes())
    new = json.loads((reports/'damage-mat238/analysis.json').read_bytes())
    skills, _ = sample.seed_definitions()
    new_rows = [dict(actor=new['actor'],enemy=g['enemy'],skill=g['skill'],grade=g['grade'],observed=value,sheet='MAT238')
                for g in new['groups'] for value in g['observed_values']]
    rows = old['rows'] + new_rows
    for i, row in enumerate(rows):
        skill = skills[row['skill']]
        grade = min(skill['value_maximum_grade'] if skill['value_maximum_grade_present'] else 10, max(1,row['grade']))
        magic = skill['value_effect_layer'] == power.MAGICAL_LAYER
        p = power.baseline(dict(H=skill['value_additional_damage'], Z=skill['value_power_per_level']*grade, layer=skill['value_effect_layer']))
        before = sample.damage_from_power(row['actor'],row['enemy'],skill,p,1)
        q = adjustment(row['actor']['matk'],row['enemy']['magic_defense']) if magic else 0
        stat = 1.4*row['actor']['matk']-.98*row['enemy']['magic_defense']+q
        floor_compensation = max(0, -stat) if magic else 0
        # P remains fixed. Compensation cancels only a negative combined magic
        # stat contribution; it is not extra skill power or a skill-ID override.
        after = sample.damage_from_power(row['actor'],row['enemy'],skill,p+q+floor_compensation,1)
        after.update(native_power=p,stat_adjustment=q,stat_floor_compensation=floor_compensation)
        row.update(baseline=before,selected=after,layer=skill['value_effect_layer'])
        row['predicted'] = {'current server': after}
        row['cell'] = str(i+1)
    metrics = {}
    for label, keep in [('all',lambda r:True),('physical',lambda r:r['layer']==1),('magical',lambda r:r['layer']==2),('MAT238',lambda r:r['actor']['matk']==238),('controlled MAT25/29',lambda r:r.get('source')=='controlled')]:
        selected = [r for r in rows if keep(r)]
        metrics[label] = {}
        for model in ('baseline','selected'):
            errors = [abs(r[model]['midpoint']/r['observed']-1)*100 for r in selected]
            metrics[label][model] = dict(casts=len(errors),mean_percent_error=statistics.mean(errors),median_percent_error=statistics.median(errors))
    return dict(rows=rows,skills=skills), metrics


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--reports',type=Path,default=Path('var/reports'))
    parser.add_argument('--output',type=Path,default=Path('var/reports/damage-selected'))
    args = parser.parse_args()
    data, metrics = analyze(args.reports)
    args.output.mkdir(parents=True,exist_ok=True)
    (args.output/'runtime-input.json').write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    (args.output/'comparison.json').write_text(json.dumps(metrics,indent=2)+'\n',encoding='utf-8')
    print(json.dumps(metrics,indent=2))


if __name__ == '__main__':
    main()

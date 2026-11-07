"""Label-agnostic Skill.dat investigation with page/family held-out checks.

No third-party dependencies; no gameplay writes. All x-offset names refer to
decoded disk offsets, not current compatibility labels. Run from repo root.
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
import struct
import sys

import analyze

NUMERIC_SPANS = ((21, 66), (97, 148))  # Exclude both text objects.
IDENTITY_OFFSETS = {22, 54, 97, 100, 102, 118}


def raw_data():
    rows, provenance = analyze.load_rows(Path('data/skill_data.json'), Path('data/animation_data.json'),
                                       Path('internal/assetsql/skill_power_defaults.csv'))
    doc = json.loads(Path('data/skill_data.json').read_bytes())
    records = {int.from_bytes(bytes.fromhex(r['decoded_hex'])[22:24], 'little'): bytes.fromhex(r['decoded_hex'])
               for r in doc['records']}
    for r in rows:
        b = records[r['id']]
        r['x'] = {str(f['offset']): (struct.unpack_from('<d', b, f['offset'])[0] if f['bytes'] == 8
                                  else int.from_bytes(b[f['offset']:f['offset']+f['bytes']], 'little'))
                  for f in doc['field_layout']}
        r['raw_hex'] = b.hex()
        r['layer'] = b[29]  # Strata are raw byte values; no damage-label interpretation is needed.
        # FUN_00480f8c decrypts manager +0x39, disk +53. The current export's
        # declared field table omits this byte, so expose both interpretations.
        r['x']['53'] = ((b[53] ^ 0xfd)-4)&255
    return rows, doc, records, provenance


def audit_fields(rows, doc):
    result = []
    p = [r['reference'] for r in rows]
    for field in doc['field_layout']:
        offset = field['offset']
        values = [r['x'][str(offset)] for r in rows]
        counts = collections.Counter(values)
        result.append(dict(offset=offset, bytes=field['bytes'], distinct=len(counts),
                           minimum=min(values), maximum=max(values), values=counts.most_common(12),
                           correlation=statistics.correlation(values, p) if len(counts) > 1 else None,
                           identity_control=offset in IDENTITY_OFFSETS))
    # Look at every remaining non-text byte, including otherwise unexposed bytes.
    known = {o+i for f in doc['field_layout'] for o in [f['offset']] for i in range(f['bytes'])}
    for start, end in NUMERIC_SPANS:
        for offset in range(start, end):
            if offset in known:
                continue
            values = [bytes.fromhex(r['raw_hex'])[offset] for r in rows]
            counts = collections.Counter(values)
            result.append(dict(offset=offset, bytes=1, undeclared=True, distinct=len(counts),
                               values=counts.most_common(), correlation=statistics.correlation(values, p) if len(counts)>1 else None))
    return result


FEATURES = {
    '1': lambda x: 1,
    'x24': lambda x: x['24'],
    'sqrt(x24)': lambda x: math.sqrt(x['24']),
    'x30': lambda x: x['30'],
    'x30^2': lambda x: x['30']**2,
    'x32': lambda x: x['32'],
    'x40': lambda x: x['40'],
    'x49': lambda x: x['49'],
    'x105': lambda x: x['105'],
    'x32*x105': lambda x: x['32']*x['105'],
    'x30*x105': lambda x: x['30']*x['105'],
    'x49*x40': lambda x: x['49']*x['40'],
    'x49/x31': lambda x: x['49']/max(1,x['31']),
    'x32*x105/x31': lambda x: x['32']*x['105']/max(1,x['31']),
    'x30^2/x31': lambda x: x['30']**2/max(1,x['31']),
    'x24/x31': lambda x: x['24']/max(1,x['31']),
    'x105*x40': lambda x: x['105']*x['40'],
    'x30*x40': lambda x: x['30']*x['40'],
    'x31': lambda x: x['31'],
    '1/x31': lambda x: 1/max(1,x['31']),
    'x32^2': lambda x: x['32']**2,
    'x32*x30': lambda x: x['32']*x['30'],
    'x40^2': lambda x: x['40']**2,
    'x30*x49': lambda x: x['30']*x['49'],
    'sqrt(x49)': lambda x: math.sqrt(x['49']),
    'x49/x40': lambda x: x['49']/(x['40'] or 1),
    'x32/x40': lambda x: x['32']/(x['40'] or 1),
    'x24/x40': lambda x: x['24']/(x['40'] or 1),
    'x32/x105': lambda x: x['32']/max(1,x['105']),
    'x49/x105': lambda x: x['49']/max(1,x['105']),
    'x30^3': lambda x: x['30']**3,
    'x49/x112': lambda x: x['49']/max(1,x['112']),
    'x32*x105/x112': lambda x: x['32']*x['105']/max(1,x['112']),
    'x30^2/x112': lambda x: x['30']**2/max(1,x['112']),
    'x49/sqrt(x112)': lambda x: x['49']/math.sqrt(max(1,x['112'])),
    'x32*x105/sqrt(x112)': lambda x: x['32']*x['105']/math.sqrt(max(1,x['112'])),
    'x30^2/sqrt(x112)': lambda x: x['30']**2/math.sqrt(max(1,x['112'])),
}


def solve(a, b):
    n = len(b)
    a = [list(row)+[value] for row, value in zip(a,b)]
    for i in range(n):
        pivot = max(range(i,n), key=lambda j:abs(a[j][i]))
        a[i], a[pivot] = a[pivot], a[i]
        scale = a[i][i]
        if abs(scale)<1e-14:
            return None
        a[i] = [v/scale for v in a[i]]
        for j in range(n):
            if j != i:
                factor = a[j][i]
                a[j] = [v-factor*w for v,w in zip(a[j],a[i])]
    return [row[-1] for row in a]


def normal(rows, indices, weights=None):
    n = len(indices)
    a, b = [[0.]*n for _ in range(n)], [0.]*n
    for ri,r in enumerate(rows):
        values = [r['features'][i]/r['reference'] for i in indices]
        weight = weights[ri] if weights else 1
        for i,v in enumerate(values):
            b[i] += weight*v
            for j,w in enumerate(values):
                a[i][j] += weight*v*w
    return a,b


def regularized(a,b):
    a = [list(row) for row in a]
    for i in range(len(b)):
        a[i][i] += max(a[i][i],1e-8)*1e-6
    return solve(a,b)


def prediction(row, indices, coefficients):
    return max(0.,sum(row['features'][i]*c for i,c in zip(indices,coefficients)))


def score(rows, indices, fits):
    values = [prediction(r,indices,fits[r['layer']]) for r in rows]
    result = analyze.metrics(rows,values)
    objective = result['median_percent_error'] + .25*result['mean_percent_error'] + .5*len(indices)
    return objective,result


def search(rows, maximum_terms=3, robust_candidates=15):
    count = len(FEATURES)
    layers = {layer:[r for r in rows if r['layer']==layer] for layer in (1,2)}
    matrices = {layer:normal(subset,list(range(count))) for layer,subset in layers.items()}
    ranked = []
    for size in range(1,maximum_terms+1):
        for indices in itertools.combinations(range(count),size):
            fits = {}
            for layer,(a,b) in matrices.items():
                fits[layer] = regularized([[a[i][j] for j in indices] for i in indices],[b[i] for i in indices])
            if any(v is None for v in fits.values()):
                continue
            objective,result = score(rows,indices,fits)
            ranked.append(dict(indices=list(indices), coefficients=fits, objective=objective, metrics=result, robust=False))
    ranked.sort(key=lambda r:r['objective'])
    for candidate in ranked[:robust_candidates]:
        indices = candidate['indices']
        fits = {k:list(v) for k,v in candidate['coefficients'].items()}
        for _ in range(12):
            for layer, subset in layers.items():
                weights = [1/max(.02,abs(prediction(r,indices,fits[layer])/r['reference']-1)) for r in subset]
                a,b = normal(subset,indices,weights)
                value = regularized(a,b)
                if value is not None:
                    fits[layer] = value
        objective,result = score(rows,indices,fits)
        ranked.append(dict(indices=list(indices), coefficients=fits, objective=objective, metrics=result, robust=True))
    return sorted(ranked,key=lambda r:r['objective'])


def families(records):
    parents = {}
    def root(i):
        parents.setdefault(i,i)
        if parents[i]!=i:
            parents[i]=root(parents[i])
        return parents[i]
    for i,b in records.items():
        for offset in (54,102):
            other = int.from_bytes(b[offset:offset+2],'little')
            if other and other!=i and other in records:
                a,c = root(i),root(other)
                parents[max(a,c)] = min(a,c)
    return {i:root(i) for i in records}


def cross_validate(rows, groups, name):
    predictions, held_rows, folds = [],[],[]
    for group in sorted(set(groups)):
        train=[r for r,g in zip(rows,groups) if g!=group]
        held=[r for r,g in zip(rows,groups) if g==group]
        selected=search(train)[0]
        values=[prediction(r,selected['indices'],selected['coefficients'][r['layer']]) for r in held]
        predictions.extend(values); held_rows.extend(held)
        folds.append(dict(group=str(group),features=[list(FEATURES)[i] for i in selected['indices']],
                          coefficients=selected['coefficients'],metrics=analyze.metrics(held,values)))
    return dict(method=name,metrics=analyze.metrics(held_rows,predictions),folds=folds,
                predictions={r['id']:v for r,v in zip(held_rows,predictions)})


def area_comparisons(rows):
    base=lambda r:(2*r['x']['49']+.5*r['x']['32']*r['x']['105']) if r['layer']==1 else 2*(r['x']['49']+r['x']['32']*r['x']['105'])
    for r in rows:
        r['pattern']=bytes.fromhex(r['raw_hex'])[min(3,max(0,(int(r['x']['105'])-1)//3))+109]
    factors={1:1,2:.6,3:.4,4:.4,5:.4,6:.3,7:.6,8:.3}
    fixed={
        'prior-hybrid':lambda r:base(r),
        'raw-x31-reduction':lambda r:base(r)*factors.get(r['x']['31'],1),
        'raw-pattern-reduction':lambda r:base(r)*factors.get(r['pattern'],1),
        'raw-pattern-reduction*0.95':lambda r:base(r)*factors.get(r['pattern'],1)*.95,
    }
    return {name:dict(metrics=analyze.metrics(rows,[f(r) for r in rows]),
                      predictions={r['id']:f(r) for r in rows}) for name,f in fixed.items()}


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output',type=Path,default=Path('var/reports/skill-power-raw'))
    parser.add_argument('--igg-skill',type=Path,help='optional independent IGG Skill.dat for version comparison')
    parser.add_argument('--strata',choices=('byte29','float40','byte27'),default='byte29',
                        help='raw-field partition; float40 splits below/above 1, byte27 splits value1/other')
    args=parser.parse_args()
    rows,doc,records,provenance=raw_data()
    for r in rows:
        r['native_byte29']=r['layer']
        if args.strata=='float40':
            r['layer']=1 if r['x']['40']<1 else 2
        elif args.strata=='byte27':
            r['layer']=1 if r['x']['27']==1 else 2
    # Include every varying declared non-identity primitive, regardless of its
    # current name. Zero-only fields remain in the audit. Identity fields stay
    # out of predictive models to avoid fitting IDs rather than mechanics.
    known_names=set(FEATURES)
    partition_offsets={'byte29':{29},'float40':set(),'byte27':{27}}[args.strata]
    additions=[str(f['offset']) for f in doc['field_layout'] if f['bytes']<=4
               and f['offset'] not in IDENTITY_OFFSETS|partition_offsets]+['53']
    vectors={tuple(f(r['x']) for r in rows) for f in FEATURES.values()}
    for offset in additions:
        name='x'+offset
        values=tuple(r['x'][offset] for r in rows)
        if name not in known_names and len(set(values))>1 and values not in vectors:
            FEATURES[name]=lambda x,o=offset:x[o]
            vectors.add(values)
    for r in rows:
        r['features']=[f(r['x']) for f in FEATURES.values()]
    ranked=search(rows)
    families_by_id=families(records)
    family_groups=[int(hashlib.sha256(str(families_by_id[r['id']]).encode()).hexdigest()[:8],16)%5 for r in rows]
    by_page=cross_validate(rows,[r['source'] for r in rows],'nested page holdout; select features on training pages only')
    by_family=cross_validate(rows,family_groups,'nested five-fold holdout; references at offsets54/102 stay in the same family')
    for candidate in ranked[:30]:
        candidate['features']=[list(FEATURES)[i] for i in candidate['indices']]
    result=dict(provenance=provenance,strata=args.strata,field_audit=audit_fields(rows,doc),
                feature_names=list(FEATURES),candidate_subsets=sum(math.comb(len(FEATURES),k) for k in (1,2,3)),
                top_models=ranked[:30],page_validation=by_page,family_validation=by_family,
                area_hypotheses=area_comparisons(rows),skills=rows)
    excluded=set(range(0,21))|set(range(66,97))
    for offset,size in ((22,2),(54,2),(97,2),(100,2),(102,2),(118,2)):
        excluded.update(range(offset,offset+size))
    signature_positions=[o for o in range(148) if o not in excluded]
    same=collections.defaultdict(list)
    for r in rows:
        same[bytes(records[r['id']][o] for o in signature_positions)].append(r)
    result['conflicting_numeric_twins']=[[
        dict(id=r['id'],name=r['name'],reference=r['reference'],contacts=r['c'],weights=r['contact_weights']) for r in group]
        for group in same.values() if len(group)>1 and len({r['reference'] for r in group})>1]
    if args.igg_skill:
        sys.path.insert(0,str(Path(__file__).resolve().parents[1]/'data_export'))
        from extract_igg_translations import decode_igg
        raw=args.igg_skill.read_bytes()
        older={r['fields']['id']:bytes.fromhex(r['decoded_hex']) for r in decode_igg('skill',raw)}
        offsets=[(f['offset'],f['bytes']) for f in doc['field_layout']]
        changes={}
        for r in rows:
            if r['id'] not in older:
                continue
            a,b=records[r['id']],older[r['id']]
            changed=[offset for offset,size in offsets if a[offset:offset+size]!=b[offset:offset+size]]
            if changed:
                changes[r['id']]=changed
        result['igg_comparison']=dict(source_sha256=hashlib.sha256(raw).hexdigest(),
                                      matched=sum(r['id'] in older for r in rows),
                                      missing=[r['id'] for r in rows if r['id'] not in older],numeric_changes=changes)
    args.output.mkdir(parents=True,exist_ok=True)
    (args.output/'raw-analysis.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    selected=ranked[0]
    lines=['<tr><th>ID</th><th>Skill</th><th>Reference P</th><th>Fit P</th><th>Family-held-out P</th><th>Page-held-out P</th><th>x30</th><th>x31</th><th>x32</th><th>x40</th><th>x49</th><th>x105</th><th>x112</th></tr>']
    for r in sorted(rows,key=lambda r:abs(prediction(r,selected['indices'],selected['coefficients'][r['layer']])/r['reference']-1),reverse=True):
        fit=prediction(r,selected['indices'],selected['coefficients'][r['layer']])
        values=[r['id'],r['name'],r['reference'],round(fit,3),round(by_family['predictions'][r['id']],3),round(by_page['predictions'][r['id']],3)]
        values.extend(r['x'][str(o)] for o in (30,31,32,40,49,105,112))
        lines.append('<tr>'+''.join('<td>'+html.escape(str(v))+'</td>' for v in values)+'</tr>')
    warning='Exploratory calibration, not a recovered server equation. All coefficients and term selection were fitted on reference data. Family/page predictions select features on training records only. The source-page test has large unique-skill extrapolation errors; inspect both means and medians. No gameplay data or formulas were changed.'
    document='<!doctype html><meta charset="utf-8"><title>Raw Skill.dat power investigation</title><style>body{font:15px system-ui;background:#172133;color:#e3ecff;margin:24px}p{max-width:1100px;line-height:1.6}table{border-collapse:collapse}td,th{padding:6px;border:1px solid #4f617c;text-align:right}td:nth-child(2){text-align:left}th{background:#293e5e}pre{white-space:pre-wrap}</style><h1>Raw Skill.dat power investigation</h1><p>'+warning+'</p><h2>Best fitted structure</h2><pre>'+html.escape(json.dumps(selected,indent=2))+'</pre><h2>All skills, largest fit errors first</h2><table>'+''.join(lines)+'</table>'
    (args.output/'report.html').write_text(document,encoding='utf-8')
    print(json.dumps(dict(top=ranked[:5],page=by_page['metrics'],family=by_family['metrics'],
                         area={k:v['metrics'] for k,v in result['area_hypotheses'].items()}),indent=2))


if __name__=='__main__':
    main()

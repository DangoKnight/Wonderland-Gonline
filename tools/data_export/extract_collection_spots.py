#!/usr/bin/env python3
"""Extract saved collection references into auditable, partially resolved SQL seeds."""
import argparse
import csv
import hashlib
import html
import json
from pathlib import Path
import re
import sqlite3
from urllib.parse import unquote

from extract_monster_names import text, write_csv
from extract_item_names import NATIVE_ITEM_RANK_OFFSET

REFERENCE = Path(__file__).parent/'reference'/'wlohub-collection'


def csv_rows(path):
    with path.open(encoding='utf-8', newline='') as stream:
        return list(csv.DictReader(stream))


def extract(pages):
    spots, sources, totals = {}, [], set()
    for page in sorted(pages.glob('*.htm*')):
        raw = page.read_bytes()
        source = raw.decode('utf-8-sig')
        snapshot = re.search(r'wire:snapshot="([^"]+)"', source)
        if not snapshot:
            raise ValueError(f'{page.name}: missing page identity')
        state = json.loads(html.unescape(snapshot[1]))['data']
        if state.get('searchLocation') or state.get('searchItem'):
            raise ValueError(f'{page.name}: filtered page')
        number = state['paginators'][0]['page']
        total = re.search(r'<span>of</span>\s*<span[^>]*>([\d,]+)</span>\s*<span>results</span>', source)
        if total:
            totals.add(int(total[1].replace(',', '')))
        count = 0
        for identity, row in re.findall(r'<tr\s+wire:key="col-(\d+)"[^>]*>(.*?)</tr>', source, re.S):
            location = re.search(r'<span class="loc-name">(.*?)</span>', row, re.S)
            if not location:
                raise ValueError(f'{page.name}: missing location {identity}')
            region = re.search(r'<div class="source-tag">(.*?)</div>', row, re.S)
            coords = dict(re.findall(r'<span class="coordinate-tag">([XY]):\s*(\d+)</span>', row))
            if coords and set(coords) != {'X', 'Y'}:
                raise ValueError(f'{page.name}: incomplete coordinates {identity}')
            rewards = [dict(source_item_id=int(i), name=text(name)) for i, name in re.findall(r'<a href="https://wlohub.com/items/(\d+)".*?<span class="ing-name">(.*?)</span>.*?</a>', row, re.S)]
            if not rewards or len({r['source_item_id'] for r in rewards}) != len(rewards):
                raise ValueError(f'{page.name}: missing/duplicate rewards {identity}')
            images = [unquote(html.unescape(p)).rsplit('/', 1)[-1] for p in re.findall(r'<img src="([^"]+)"[^>]*class="collection-thumb"', row)]
            sid = int(identity)
            if sid in spots:
                raise ValueError(f'duplicate collection spot {sid}')
            spots[sid] = dict(location=text(location[1]), region=text(region[1]).removeprefix('Via: ') if region else '',
                              position_known=bool(coords), x=int(coords.get('X', 0)), y=int(coords.get('Y', 0)), rewards=rewards, images=images,
                              kind='vacuum' if len(rewards)>1 else 'unresolved', source='wlohub')
            count += 1
        if not count:
            raise ValueError(f'{page.name}: no spots found')
        sources.append(dict(file=page.name, page=number, records=count, sha256=hashlib.sha256(raw).hexdigest()))
    numbers = sorted(s['page'] for s in sources)
    if not numbers or numbers != list(range(1, max(numbers)+1)) or totals and totals != {len(spots)}:
        raise ValueError('missing/duplicate pages or advertised total mismatch')
    return spots, sources


def resolve_items(spots, reference, native, choices):
    refs = {int(r['source_id']): r for r in reference}
    native_by_id = {r['definition']['id']: r for r in native}
    mapping, report = {}, []
    for source_id in sorted({r['source_item_id'] for s in spots.values() for r in s['rewards']}):
        ref = refs[source_id]
        candidates = []
        for row in native:
            name = row['definition']['name']
            same_name = name == ref['name'] or len(name.encode('utf-8')) >= 13 and ref['name'].startswith(name)
            if same_name and str(row['icon']) == ref['icon'] and str(bytes.fromhex(row['decoded_hex'])[NATIVE_ITEM_RANK_OFFSET]) == ref['rank']:
                candidates.append(row['definition']['id'])
        if source_id in choices:
            chosen = native_by_id[choices[source_id]]
            if chosen['definition']['name'] != ref['name'] or str(bytes.fromhex(chosen['decoded_hex'])[NATIVE_ITEM_RANK_OFFSET]) != ref['rank']:
                raise ValueError(f'item choice {source_id}: name/rank mismatch')
            candidates = [choices[source_id]]
        mapping[source_id] = candidates[0] if len(candidates) == 1 else 0
        report.append(dict(source_item_id=source_id, name=ref['name'], item_id=mapping[source_id], candidates=';'.join(map(str,candidates)), method='authored choice' if source_id in choices else 'name/icon/rank'))
    return mapping, report


def resolve_maps(spots, scenes, bounds, aliases):
    groups = {}
    for sid, spot in spots.items():
        groups.setdefault((spot['location'], spot['region']), []).append(sid)
    for (location, region), ids in groups.items():
        native_name = aliases.get(location, location)
        possible = sorted({r['fields']['id'] for r in scenes if r['name']['text'] == native_name and r['fields']['id'] in bounds})
        coords = [spots[sid] for sid in ids if spots[sid]['position_known']]
        candidates = [mid for mid in possible if all(0 <= s['x'] < bounds[mid][0] and 0 <= s['y'] < bounds[mid][1] for s in coords)]
        for sid in ids:
            spot = spots[sid]
            local = candidates
            if not local:
                # One inconsistent coordinate must not invalidate other spots
                # under the same coarse website location heading.
                local = [mid for mid in possible if not spot['position_known'] or 0 <= spot['x'] < bounds[mid][0] and 0 <= spot['y'] < bounds[mid][1]]
            spot['map_candidates'] = local
            spot['map_id'] = local[0] if len(local) == 1 else 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('pages', 'item-export', 'scene-export', 'assets-db', 'output-dir'):
        parser.add_argument('--'+name, type=Path, required=True)
    parser.add_argument('--item-reference', type=Path, default=Path(__file__).parent/'reference'/'wlohub-items'/'items.csv')
    parser.add_argument('--item-choices', type=Path, default=REFERENCE/'item_choices.csv')
    parser.add_argument('--location-aliases', type=Path, default=REFERENCE/'location_aliases.csv')
    parser.add_argument('--supplemental-spots', type=Path, default=REFERENCE/'supplemental_spots.csv')
    args = parser.parse_args()
    spots, sources = extract(args.pages)
    reference_count = len(spots)
    for row in csv_rows(args.supplemental_spots):
        sid = int(row['id'])
        if sid in spots:
            raise ValueError(f'duplicate supplemental spot {sid}')
        spots[sid] = dict(location=row['location'], region=row['region'], position_known=True, x=int(row['x']), y=int(row['y']), kind=row['kind'], source='owner', rewards=[], images=[])
    item_map, item_report = resolve_items(spots, csv_rows(args.item_reference), json.loads(args.item_export.read_text(encoding='utf-8'))['items'], {int(r['source_item_id']): int(r['item_id']) for r in csv_rows(args.item_choices)})
    c = sqlite3.connect(args.assets_db.resolve().as_uri()+'?mode=ro', uri=True)
    try:
        bounds = {mid: (w,h) for mid,w,h in c.execute('select terrains_key,value_width,value_height from catalog_terrains')}
    finally:
        c.close()
    resolve_maps(spots, json.loads(args.scene_export.read_text(encoding='utf-8'))['records'], bounds, {r['location']:r['native_name'] for r in csv_rows(args.location_aliases)})
    seed, report = {}, []
    for sid, spot in sorted(spots.items()):
        seed[str(sid)] = {k:v for k,v in spot.items() if k not in ('images','rewards')}
        seed[str(sid)]['rewards'] = [dict(item_id=item_map[r['source_item_id']], source_item_id=r['source_item_id']) for r in spot['rewards']]
        problems = []
        if not spot['map_id']: problems.append('map unresolved')
        if not spot['position_known']: problems.append('coordinates missing')
        if not spot['rewards'] or any(not item_map[r['source_item_id']] for r in spot['rewards']): problems.append('reward unresolved')
        report.append(dict(source_id=sid, source=spot['source'],kind=spot['kind'],location=spot['location'], region=spot['region'], map_id=spot['map_id'], x=spot['x'] if spot['position_known'] else '', y=spot['y'] if spot['position_known'] else '', map_candidates=';'.join(map(str,spot['map_candidates'])), rewards=';'.join(r['name'] for r in spot['rewards']), images=';'.join(spot['images']), resolution=';'.join(problems) or 'location and rewards resolved'))
    write_csv(args.output_dir/'spots.csv', report, ['source_id','source','kind','location','region','map_id','x','y','map_candidates','rewards','images','resolution'])
    write_csv(args.output_dir/'items.csv', item_report, ['source_item_id','name','item_id','candidates','method'])
    (args.output_dir/'collection_spots_defaults.json').write_bytes((json.dumps(seed,ensure_ascii=False,indent=2)+'\n').encode())
    manifest = dict(source='https://wlohub.com/collections', pages=sources, reference_spots=reference_count,spots=len(spots),vacuum_spots=sum(s['kind']=='vacuum' for s in spots.values()),rewards=sum(len(s['rewards']) for s in spots.values()), distinct_items=len(item_map), resolved_items=sum(bool(i) for i in item_map.values()), resolved_locations=sum(r['resolution']=='location and rewards resolved' for r in report), tool_requirements='not specified', intervals_and_probabilities='not specified')
    manifest['terrain_bounds_sha256'] = hashlib.sha256(json.dumps(sorted((mid,w,h) for mid,(w,h) in bounds.items()),separators=(',',':')).encode()).hexdigest()
    for key in ('item_export','scene_export','item_reference','item_choices','location_aliases','supplemental_spots'):
        manifest[key+'_sha256'] = hashlib.sha256(getattr(args,key).read_bytes()).hexdigest()
    (args.output_dir/'sources.json').write_bytes((json.dumps(manifest,indent=2)+'\n').encode())
    print(json.dumps({k:v for k,v in manifest.items() if k not in ('pages',) and not k.endswith('_sha256')}))


if __name__ == '__main__':
    main()

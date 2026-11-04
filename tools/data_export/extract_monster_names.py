#!/usr/bin/env python3
"""Extract saved WLOHub pages and conservatively match native NPC names offline."""
import argparse
import csv
import hashlib
import html
import json
from pathlib import Path
import re
import sqlite3

STAT_NAMES = ('LVL', 'HP', 'SP', 'SPD', 'STR', 'CON', 'INT', 'WIS', 'AGI', 'Catch')
ELEMENTS = {'None': 0, 'Earth': 1, 'Water': 2, 'Fire': 3, 'Wind': 4}
NATIVE_NPC_NAME_BYTES = 10


def text(value):
    return ' '.join(html.unescape(re.sub('<[^>]+>', '', value)).split())


def extract(pages):
    monsters, sources, totals = {}, [], set()
    for page in sorted(pages.glob('*.htm*')):
        raw = page.read_bytes()
        source = raw.decode('utf-8-sig')
        snapshot = re.search(r'wire:snapshot="([^"]+)"', source)
        if not snapshot:
            raise ValueError(f'{page.name}: missing page identity')
        state = json.loads(html.unescape(snapshot[1]))['data']
        if state.get('search') or state.get('elementFilter'):
            raise ValueError(f'{page.name}: filtered page cannot form a complete extraction')
        number = state['paginators'][0]['page']
        total = re.search(r'<span>of</span>\s*<span[^>]*>([\d,]+)</span>\s*<span>results</span>', source)
        if total:
            totals.add(int(total[1].replace(',', '')))
        cards = re.split(r'<div\s+class="monster-card"\s+wire:key="monster-(\d+)"[^>]*>', source)
        count = 0
        for index in range(1, len(cards), 2):
            identity, card = int(cards[index]), cards[index+1]
            name = re.search(r'<h3[^>]*class="monster-name"[^>]*>(.*?)</h3>', card, re.S)
            element = re.search(r'class="element-icon"[^>]*title="([^"]*)"', card)
            pairs = re.findall(r'<span class="stat-title">(.*?)</span>\s*<span class="stat-val">(.*?)</span>', card, re.S)
            stats = {text(k): text(v) for k, v in pairs}
            if name is None or element is None or set(stats) != set(STAT_NAMES):
                raise ValueError(f'{page.name}: incomplete monster {identity}')
            row = {'source_id': identity, 'name': text(name[1]), 'element': ELEMENTS[html.unescape(element[1])],
                   **{k.lower(): int(stats[k]) for k in STAT_NAMES if k != 'Catch'}, 'catch': stats['Catch'], 'page': number}
            if identity in monsters:
                raise ValueError(f'duplicate monster {identity}; pages overlap')
            monsters[identity] = row
            count += 1
        if not count:
            raise ValueError(f'{page.name}: no monster cards found')
        sources.append({'file': page.name, 'page': number, 'records': count, 'sha256': hashlib.sha256(raw).hexdigest()})
    if not monsters:
        raise ValueError('no monster cards found')
    numbers = sorted(s['page'] for s in sources)
    if numbers != list(range(1, max(numbers)+1)):
        raise ValueError('saved pages are incomplete or duplicated')
    if totals and totals != {len(monsters)}:
        raise ValueError(f'incomplete extraction: found {len(monsters)}, pages advertise {totals}')
    return list(monsters.values()), sources


def display_name(name, strip_quest):
    if not strip_quest:
        return name
    name = re.sub(r'\(\s*quest\s*\)', '', name, flags=re.I)
    name = re.sub(r'\bquest\b', '', name, flags=re.I)
    return ' '.join(name.split())


def match(monsters, database=None, npc_export=None, accept_single=False, strip_quest=False, choices=None, reviewed=None):
    if npc_export is not None:
        npcs = []
        for row in json.loads(npc_export.read_text(encoding='utf-8'))['records']:
            f = row['fields']
            npcs.append({'value_id': f['id'], 'value_name': row['name']['text'],
                         'value_level': f['level'], 'value_element': f['element'],
                         'value_hp': f.get('hp'), 'value_sp': f.get('sp'),
                         **{f'value_stats{i}': f[k] for i, k in enumerate(('strength', 'constitution', 'intelligence', 'wisdom', 'agility'))}})
        npcs.sort(key=lambda n: n['value_id'])
    else:
        connection = sqlite3.connect(database.resolve().as_uri()+'?mode=ro', uri=True)
        connection.row_factory = sqlite3.Row
        try:
            npcs = connection.execute('SELECT * FROM catalog_n_p_cs ORDER BY value_id').fetchall()
        finally:
            connection.close()
    patches, review = [], []
    for npc in npcs:
        old = npc['value_name']
        if reviewed and npc['value_id'] in reviewed:
            decision = reviewed[npc['value_id']]
            if old != decision['original_name']:
                raise ValueError(f'review {npc["value_id"]}: native name changed')
            if int(decision['native_level']) != npc['value_level'] or int(decision['native_element']) != npc['value_element'] or [int(i) for i in decision['native_stats'].split(';')] != [npc[f'value_stats{i}'] for i in range(5)]:
                raise ValueError(f'review {npc["value_id"]}: native identity changed')
            source_ids = {int(i) for i in decision['source_ids'].split(';')}
            selected = [m for m in monsters if m['source_id'] in source_ids]
            if len(selected) != len(source_ids) or not selected or any(display_name(m['name'], strip_quest) != decision['name'] for m in selected):
                raise ValueError(f'review {npc["value_id"]}: invalid source identity')
            if decision['method'] in ('exact-prefix', 'exact-species', 'exact-vitals'):
                if decision['method'] == 'exact-prefix' and not decision['name'].startswith(old):
                    raise ValueError(f'review {npc["value_id"]}: prefix changed')
                if any(m['lvl'] != npc['value_level'] or m['element'] != npc['value_element'] or [m[k] for k in ('str', 'con', 'int', 'wis', 'agi')] != [npc[f'value_stats{i}'] for i in range(5)] for m in selected):
                    raise ValueError(f'review {npc["value_id"]}: exact metadata changed')
                if decision['method'] == 'exact-vitals' and any(m['hp'] != npc['value_hp'] or m['sp'] != npc['value_sp'] for m in selected):
                    raise ValueError(f'review {npc["value_id"]}: exact vitals changed')
            elif decision['method'] != 'reviewed-species' or not decision['notes']:
                raise ValueError(f'review {npc["value_id"]}: missing reviewed evidence')
            patches.append({'id': npc['value_id'], 'original_name': old, 'name': decision['name'],
                            'source_ids': decision['source_ids'],
                            'previous_name': next((m['name'] for m in selected if m['name'] != display_name(m['name'], strip_quest)), '')})
            continue
        # A prefix alone cannot distinguish names sharing the same native bytes.
        eligible = len(old.encode('utf-8')) == NATIVE_NPC_NAME_BYTES or choices and npc['value_id'] in choices
        candidates = [m for m in monsters if m['name'].startswith(old) and len(m['name']) > len(old)] if eligible and old else []
        if not candidates:
            continue
        exact = [m for m in candidates if m['lvl'] == npc['value_level'] and m['element'] == npc['value_element']
                 and [m[k] for k in ('str', 'con', 'int', 'wis', 'agi')] == [npc[f'value_stats{i}'] for i in range(5)]]
        names = {display_name(m['name'], strip_quest) for m in exact}
        all_names = {display_name(m['name'], strip_quest) for m in candidates}
        # Explicit owner choices resolve the remaining multi-name candidates.
        if choices and npc['value_id'] in choices:
            chosen = choices[npc['value_id']]
            exact = [m for m in candidates if display_name(m['name'], strip_quest) == chosen]
            if not exact:
                raise ValueError(f'choice {npc["value_id"]}: {chosen!r} is not a reference candidate')
            names = {chosen}
        if len(names) != 1 and accept_single and len(all_names) == 1:
            exact, names = candidates, all_names
        if len(names) == 1:
            patches.append({'id': npc['value_id'], 'original_name': old, 'name': names.pop(),
                            'source_ids': ';'.join(str(m['source_id']) for m in exact),
                            'previous_name': next((m['name'] for m in exact if m['name'] != display_name(m['name'], strip_quest)), '')})
        else:
            review.append({'id': npc['value_id'], 'original_name': old,
                           'reason': 'ambiguous' if len(names) > 1 else 'no exact level/element/attribute match',
                           'candidates': ';'.join(sorted(all_names))})
    return patches, review


def write_csv(path, rows, fields):
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open('w', encoding='utf-8', newline='') as stream:
        writer = csv.DictWriter(stream, fields, lineterminator='\n')
        writer.writeheader()
        writer.writerows(rows)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--pages', type=Path, required=True)
    baseline = parser.add_mutually_exclusive_group(required=True)
    baseline.add_argument('--assets-db', type=Path, help='unpatched structured assets database')
    baseline.add_argument('--npc-export', type=Path, help='lossless npc_data.json export (preferred for reproducible builds)')
    parser.add_argument('--output-dir', type=Path, required=True)
    parser.add_argument('--accept-single-candidates', action='store_true', help='accept the owner-approved single-name prefix suggestions despite stat differences')
    parser.add_argument('--strip-quest', action='store_true', help='remove Quest annotations from display names, preserving raw reference names')
    parser.add_argument('--name-choices', type=Path, default=Path(__file__).parent/'reference'/'wlohub'/'name_choices.csv', help='authored ID-specific owner choices')
    parser.add_argument('--reviewed-names', type=Path, default=Path(__file__).parent/'reference'/'wlohub'/'enemy_name_resolutions.csv', help='ID-specific researched corrections with native/source identity checks')
    args = parser.parse_args()
    monsters, sources = extract(args.pages)
    with args.name_choices.open(encoding='utf-8', newline='') as stream:
        choices = {int(row['id']): row['name'] for row in csv.DictReader(stream)}
    with args.reviewed_names.open(encoding='utf-8', newline='') as stream:
        rows = list(csv.DictReader(stream))
    reviewed = {int(row['id']): row for row in rows}
    if len(reviewed) != len(rows) or choices.keys() & reviewed.keys():
        raise ValueError('duplicate reviewed/owner identity')
    patches, review = match(monsters, args.assets_db, args.npc_export, args.accept_single_candidates, args.strip_quest, choices, reviewed)
    # Keep raw names in memory for earlier-name migration aliases, but export
    # the same cleaned display names as the compiled patch.
    exported_monsters = [{**m, 'name': display_name(m['name'], args.strip_quest)} for m in monsters]
    write_csv(args.output_dir/'monsters.csv', exported_monsters, ['source_id', 'name', 'element', *[k.lower() for k in STAT_NAMES if k != 'Catch'], 'catch', 'page'])
    write_csv(args.output_dir/'npc_names.csv', patches, ['id', 'original_name', 'name', 'source_ids', 'previous_name'])
    write_csv(args.output_dir/'review.csv', review, ['id', 'original_name', 'reason', 'candidates'])
    manifest = {'source': 'https://wlohub.com/monsters', 'pages': sources, 'monsters': len(monsters), 'patched': len(patches), 'review': len(review)}
    manifest['accept_single_candidates'] = args.accept_single_candidates
    manifest['strip_quest'] = args.strip_quest
    manifest['name_choices_sha256'] = hashlib.sha256(args.name_choices.read_bytes()).hexdigest()
    manifest['reviewed_names_sha256'] = hashlib.sha256(args.reviewed_names.read_bytes()).hexdigest()
    if args.npc_export:
        manifest['npc_export_sha256'] = hashlib.sha256(args.npc_export.read_bytes()).hexdigest()
    (args.output_dir/'sources.json').write_bytes((json.dumps(manifest, ensure_ascii=False, indent=2)+'\n').encode())
    print(json.dumps({k:v for k,v in manifest.items() if k != 'pages'}))


if __name__ == '__main__':
    main()

#!/usr/bin/env python3
"""Audit short names excluded by the full-name patch length thresholds."""
import argparse
import csv
import hashlib
import json
from pathlib import Path
import re

from extract_monster_names import NATIVE_NPC_NAME_BYTES, write_csv
from extract_item_names import NATIVE_ITEM_NAME_BYTES, NATIVE_ITEM_RANK_OFFSET

REPO = Path(__file__).resolve().parents[2]
STAT_KEYS = ('str', 'con', 'int', 'wis', 'agi')
NATIVE_STATS = ('strength', 'constitution', 'intelligence', 'wisdom', 'agility')


def read_csv(path):
    with path.open(encoding='utf-8', newline='') as stream:
        return list(csv.DictReader(stream))


def classify(old, matches):
    names = sorted({row['name'] for row in matches})
    if len(names) != 1:
        return 'ambiguous metadata' if names else 'no matching metadata'
    suffix = names[0][len(old):].strip()
    if suffix.startswith('('):
        return 'context annotation'
    if not names[0][len(old):].startswith(' ') and len(suffix) == 1:
        return 'spelling difference'
    if suffix.startswith(('+', '-')) or re.fullmatch(r'\d+\w*|[A-Z]|Lord|Capsule', suffix):
        return 'variant-sensitive suffix'
    return 'unique metadata match'


def monster_rows(native, reference, patched):
    rows = []
    for row in sorted(native, key=lambda n: n['fields']['id']):
        f, old = row['fields'], row['name']['text']
        if f['id'] in patched or not old or len(old.encode('utf-8')) >= NATIVE_NPC_NAME_BYTES:
            continue
        candidates = [m for m in reference if m['name'].startswith(old) and len(m['name']) > len(old)]
        if not candidates:
            continue
        exact = [m for m in candidates if int(m['lvl']) == f['level'] and int(m['element']) == f['element']
                 and [int(m[k]) for k in STAT_KEYS] == [f[k] for k in NATIVE_STATS]]
        complete = any(m['name'] == old and int(m['lvl']) == f['level'] and int(m['element']) == f['element']
                       and [int(m[k]) for k in STAT_KEYS] == [f[k] for k in NATIVE_STATS] for m in reference)
        rows.append(dict(id=f['id'], original_name=old, reason='already complete' if complete else classify(old, exact),
                         matching_names=';'.join(sorted({m['name'] for m in exact})),
                         candidates=';'.join(sorted({m['name'] for m in candidates})),
                         matching_source_ids=';'.join(m['source_id'] for m in exact)))
    return rows


def item_rows(native, reference):
    rows = []
    for row in sorted(native, key=lambda n: n['definition']['id']):
        d, old = row['definition'], row['definition']['name']
        if not old or len(old.encode('utf-8')) >= NATIVE_ITEM_NAME_BYTES-1:
            continue
        candidates = [i for i in reference if i['name'].startswith(old) and len(i['name']) > len(old)]
        if not candidates:
            continue
        rank = str(bytes.fromhex(row['decoded_hex'])[NATIVE_ITEM_RANK_OFFSET])
        exact = [i for i in candidates if i['icon'] == str(row['icon']) and i['rank'] == rank]
        complete = any(i['name'] == old and i['icon'] == str(row['icon']) and i['rank'] == rank for i in reference)
        if len({i['name'] for i in exact}) > 1:
            described = [i for i in exact if i['description'] == ' '.join(row['description'].split())]
            if described:
                exact = described
        rows.append(dict(id=d['id'], original_name=old, reason='already complete' if complete else classify(old, exact),
                         matching_names=';'.join(sorted({i['name'] for i in exact})),
                         candidates=';'.join(sorted({i['name'] for i in candidates})),
                         matching_source_ids=';'.join(i['source_id'] for i in exact)))
    return rows


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--npc-export', type=Path, required=True)
    parser.add_argument('--item-export', type=Path, required=True)
    parser.add_argument('--monster-reference', type=Path, default=REPO/'tools/data_export/reference/wlohub/monsters.csv')
    parser.add_argument('--item-reference', type=Path, default=REPO/'tools/data_export/reference/wlohub-items/items.csv')
    parser.add_argument('--npc-patch', type=Path, default=REPO/'internal/assets/npc_names.csv')
    parser.add_argument('--output-dir', type=Path, required=True)
    args = parser.parse_args()
    patched = {int(r['id']) for r in read_csv(args.npc_patch)}
    monsters = monster_rows(json.loads(args.npc_export.read_text(encoding='utf-8'))['records'], read_csv(args.monster_reference), patched)
    items = item_rows(json.loads(args.item_export.read_text(encoding='utf-8'))['items'], read_csv(args.item_reference))
    fields = ['id', 'original_name', 'reason', 'matching_names', 'candidates', 'matching_source_ids']
    write_csv(args.output_dir/'monster_short_names.csv', monsters, fields)
    write_csv(args.output_dir/'item_short_names.csv', items, fields)
    summary = {'monsters': len(monsters), 'monster_unique_extensions': sum(bool(r['matching_names']) and ';' not in r['matching_names'] and r['reason'] != 'already complete' for r in monsters),
               'items': len(items), 'item_unique_extensions': sum(bool(r['matching_names']) and ';' not in r['matching_names'] and r['reason'] != 'already complete' for r in items),
               'input_sha256': {key: hashlib.sha256(getattr(args, key).read_bytes()).hexdigest() for key in ('npc_export', 'item_export', 'monster_reference', 'item_reference', 'npc_patch')}}
    (args.output_dir/'sources.json').write_bytes((json.dumps(summary, indent=2)+'\n').encode())
    print(json.dumps({k:v for k,v in summary.items() if k != 'input_sha256'}))


if __name__ == '__main__':
    main()

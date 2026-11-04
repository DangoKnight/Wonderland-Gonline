#!/usr/bin/env python3
"""Reproduce the remaining enemy-name evidence report from saved native/reference data."""
import argparse
import collections
import csv
import hashlib
import json
from pathlib import Path

from extract_monster_names import NATIVE_NPC_NAME_BYTES, write_csv

REFERENCE = Path(__file__).parent / 'reference' / 'wlohub'
STAT_KEYS = ('str', 'con', 'int', 'wis', 'agi')
NATIVE_STATS = ('strength', 'constitution', 'intelligence', 'wisdom', 'agility')
SPRITE_FIELD = 'unknown_u16_offset_14'
ICON_FIELD = 'unknown_u16_offset_16'
PALETTE_FIELDS = tuple(f'unknown_u32_offset_{offset}' for offset in (18, 22, 26, 30))


def read_csv(path):
    with path.open(encoding='utf-8', newline='') as stream:
        return list(csv.DictReader(stream))


def review(native, reference, patches, resolutions, manual=None):
    manual = manual or {}
    results = []
    for record in sorted(native, key=lambda r: r['fields']['id']):
        f, old = record['fields'], record['name']['text']
        identity = f['id']
        if (identity in patches and identity not in resolutions and identity not in manual) or not old:
            continue
        candidates = [r for r in reference if r['name'].startswith(old) and len(r['name']) > len(old)]
        if not candidates and len(old.encode('utf-8')) != NATIVE_NPC_NAME_BYTES:
            continue
        exact = [r for r in reference if int(r['lvl']) == f['level'] and int(r['element']) == f['element']
                 and [int(r[k]) for k in STAT_KEYS] == [f[k] for k in NATIVE_STATS]]
        full = [r for r in exact if r['name'] == old]
        vital_names = sorted({r['name'] for r in exact if int(r['hp']) == f['hp'] and int(r['sp']) == f['sp']})
        extension = sorted({r['name'] for r in exact if r in candidates})
        resolution = resolutions.get(identity)
        if identity in manual:
            owner = manual[identity]
            if owner['original_name'] != old:
                raise ValueError(f'owner review {identity}: native name changed')
            status, chosen = ('owner renamed' if owner['name'] != old else 'owner confirmed'), owner['name']
            notes, sources = 'Owner visual review with certainty checked; not inferred from shared metadata.', 'owner-review'
        elif resolution:
            status, chosen, notes = 'rename', resolution['name'], resolution['notes']
            sources = resolution['source_ids']
        elif full:
            status, chosen, notes = 'already complete', old, 'Exact reference name, level, element and all attributes; no expansion needed.'
            sources = ';'.join(r['source_id'] for r in full)
        elif len(extension) == 1 and extension[0].startswith(old + ' ('):
            status, chosen, notes = 'preserve context-free name', old, 'Reference adds location/quest context, not a missing creature name.'
            sources = ';'.join(r['source_id'] for r in exact if r in candidates)
        elif len(extension) == 1 and not extension[0][len(old):].startswith(' ') and len(extension[0]) == len(old) + 1:
            status, chosen, notes = 'preserve spelling', old, 'Reference spelling differs; native spelling is valid and is not truncated.'
            sources = ';'.join(r['source_id'] for r in exact if r in candidates)
        else:
            status, chosen, sources = 'unresolved', old, ''
            notes = ('No longer prefix candidate in the saved reference; capacity-length names may already be complete.' if not candidates else
                     'Prefix/metadata evidence does not establish this identity. Shared sprites or copied stats alone cannot prove a variant name.')
        results.append(dict(id=identity, original_name=old, decision=status, display_name=chosen,
                            native_type=f['type'], level=f['level'], element=f['element'], hp=f['hp'], sp=f['sp'],
                            stats=';'.join(str(f[k]) for k in NATIVE_STATS), sprite=f[SPRITE_FIELD], icon=f[ICON_FIELD],
                            palette=';'.join(str(f[k]) for k in PALETTE_FIELDS),
                            candidates=';'.join(sorted({r['name'] for r in candidates})),
                            exact_metadata_names=';'.join(sorted({r['name'] for r in exact})),
                            exact_vital_names=';'.join(vital_names),
                            source_ids=sources, notes=notes))
    missing = resolutions.keys() - {r['id'] for r in results}
    if missing:
        raise ValueError(f'resolutions not represented: {sorted(missing)}')
    return results


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--npc-export', type=Path, required=True)
    p.add_argument('--output-dir', type=Path, required=True)
    p.add_argument('--reference', type=Path, default=REFERENCE / 'monsters.csv')
    p.add_argument('--resolutions', type=Path, default=REFERENCE / 'enemy_name_resolutions.csv')
    p.add_argument('--npc-patch', type=Path, default=Path(__file__).resolve().parents[2] / 'internal/assets/npc_names.csv')
    p.add_argument('--manual-reviews', type=Path, default=Path(__file__).resolve().parents[2] / 'internal/assets/npc_name_reviews.csv')
    a = p.parse_args()
    native = json.loads(a.npc_export.read_text(encoding='utf-8'))['records']
    manual_rows = read_csv(a.manual_reviews)
    if len({r['id'] for r in manual_rows}) != len(manual_rows) or any(r['certain'] not in ('true','false') for r in manual_rows):
        raise ValueError('invalid owner review identities or certainty')
    manual = {int(r['id']): r for r in manual_rows if r['certain']=='true'}
    rows = review(native, read_csv(a.reference), {int(r['id']) for r in read_csv(a.npc_patch)},
                  {int(r['id']): r for r in read_csv(a.resolutions)}, manual)
    write_csv(a.output_dir / 'enemy_name_review.csv', rows, list(rows[0]))
    counts = dict(collections.Counter(r['decision'] for r in rows))
    manifest = dict(native_records=len(native), reviewed=len(rows), decisions=counts,
                    input_sha256={k: hashlib.sha256(getattr(a, k).read_bytes()).hexdigest() for k in ('npc_export', 'reference', 'resolutions', 'npc_patch', 'manual_reviews')})
    (a.output_dir / 'enemy_review_sources.json').write_text(json.dumps(manifest, indent=2) + '\n', encoding='utf-8', newline='\n')
    print(json.dumps({k: v for k, v in manifest.items() if k != 'input_sha256'}))


if __name__ == '__main__':
    main()

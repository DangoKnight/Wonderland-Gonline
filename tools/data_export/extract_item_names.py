#!/usr/bin/env python3
"""Extract saved WLOHub item pages and match display names to native exports."""
import argparse
import csv
import hashlib
import html
import json
from pathlib import Path
import re

from extract_monster_names import text, write_csv

NATIVE_ITEM_NAME_BYTES = 14
NATIVE_ITEM_RANK_OFFSET = 45


def display_name(name):
    return ' '.join(re.sub(r'\(\s*quest\s*\)', '', name, flags=re.I).split())


def optional_number(value):
    value = text(value).replace(',', '')
    return int(value) if value.isdecimal() else None


def extract(pages):
    items, sources, totals = {}, [], set()
    for page in sorted(pages.glob('*.htm*')):
        raw = page.read_bytes()
        source = raw.decode('utf-8-sig')
        snapshot = re.search(r'wire:snapshot="([^"]+)"', source)
        if not snapshot:
            raise ValueError(f'{page.name}: missing page identity')
        state = json.loads(html.unescape(snapshot[1]))['data']
        if any(value for key, value in state.items() if key == 'search' or key.endswith('Filter') or key in ('minRank', 'maxRank')):
            raise ValueError(f'{page.name}: filtered page')
        number = state['paginators'][0]['page']
        total = re.search(r'<span>of</span>\s*<span[^>]*>([\d,]+)</span>\s*<span>results</span>', source)
        if total:
            totals.add(int(total[1].replace(',', '')))
        count = 0
        for identity, row in re.findall(r'<tr\s+wire:key="item-(\d+)"[^>]*>(.*?)</tr>', source, re.S):
            cells = re.findall(r'<td\b[^>]*>(.*?)</td>', row, re.S)
            if len(cells) != 8:
                raise ValueError(f'{page.name}: invalid item {identity} columns')
            icon = re.search(r'<img[^>]*src="[^"]*/(\d+)[_.][^"]*"', cells[0])
            subtype = re.search(r'<span class="subtype-text">(.*?)</span>', cells[4], re.S)
            full_type = text(cells[4])
            sub = text(subtype[1]).strip('()') if subtype else ''
            item = dict(source_id=int(identity), name=display_name(text(cells[1])), icon=int(icon[1]) if icon else 0,
                        level=optional_number(cells[2]), rank=optional_number(cells[3]),
                        type=full_type.split(' (')[0], subtype=sub, bases=text(cells[5]),
                        effects=text(cells[6]), description=text(cells[7]), page=number)
            if item['source_id'] in items:
                raise ValueError(f'duplicate item {identity}; pages overlap')
            items[item['source_id']] = item
            count += 1
        if not count:
            raise ValueError(f'{page.name}: no item rows')
        sources.append(dict(file=page.name, page=number, records=count, sha256=hashlib.sha256(raw).hexdigest()))
    numbers = sorted(s['page'] for s in sources)
    if not numbers or numbers != list(range(1, max(numbers)+1)):
        raise ValueError('missing or duplicate pages')
    if totals and totals != {len(items)}:
        raise ValueError(f'incomplete extraction: {len(items)} records, advertised {totals}')
    return list(items.values()), sources


def match(items, native):
    patches, review = [], []
    for row in sorted(native, key=lambda r: r['definition']['id']):
        definition = row['definition']
        old = definition['name']
        # English translations commonly reserve the last byte for a terminator.
        if len(old.encode('utf-8')) < NATIVE_ITEM_NAME_BYTES-1:
            continue
        candidates = [i for i in items if i['name'].startswith(old) and len(i['name']) > len(old)]
        if not candidates:
            continue
        rank = bytes.fromhex(row['decoded_hex'])[NATIVE_ITEM_RANK_OFFSET]
        if any(i['name'] == old and i['icon'] == row['icon'] and i['rank'] == rank for i in items):
            continue # Already a verified complete name, not a longer variant.
        exact = [i for i in candidates if i['icon'] == row['icon'] and i['rank'] == rank]
        if re.search(r'[+-]\d+$', old):
            exact = [i for i in exact if not i['name'][len(old):len(old)+1].isdigit()]
        if len({i['name'] for i in exact}) > 1:
            described = [i for i in exact if i['description'] == ' '.join(row['description'].split())]
            if described:
                exact = described
        names = {i['name'] for i in exact}
        if len(names) == 1:
            patches.append(dict(id=definition['id'], original_name=old, name=names.pop(), source_ids=';'.join(str(i['source_id']) for i in exact)))
        else:
            review.append(dict(id=definition['id'], original_name=old, reason='ambiguous' if len(names)>1 else 'icon/rank mismatch', candidates=';'.join(sorted({i['name'] for i in candidates}))))
    return patches, review


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--pages', type=Path, required=True)
    parser.add_argument('--item-export', type=Path, required=True)
    parser.add_argument('--output-dir', type=Path, required=True)
    args = parser.parse_args()
    items, sources = extract(args.pages)
    native = json.loads(args.item_export.read_text(encoding='utf-8'))['items']
    patches, review = match(items, native)
    write_csv(args.output_dir/'items.csv', items, ['source_id', 'name', 'icon', 'level', 'rank', 'type', 'subtype', 'bases', 'effects', 'description', 'page'])
    write_csv(args.output_dir/'item_names.csv', patches, ['id', 'original_name', 'name', 'source_ids'])
    write_csv(args.output_dir/'review.csv', review, ['id', 'original_name', 'reason', 'candidates'])
    manifest = dict(source='https://wlohub.com/items', pages=sources, items=len(items), patched=len(patches), review=len(review), item_export_sha256=hashlib.sha256(args.item_export.read_bytes()).hexdigest())
    (args.output_dir/'sources.json').write_bytes((json.dumps(manifest, ensure_ascii=False, indent=2)+'\n').encode())
    print(json.dumps({k:v for k,v in manifest.items() if k != 'pages'}))


if __name__ == '__main__':
    main()

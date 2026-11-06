#!/usr/bin/env python3
"""Extract explicit elemental and unique skill powers from saved wiki tables.

Native IDs are matched by element, effect layer and SP cost. Authored choices
disambiguate repeated SP costs using the native progression's family/order.
Icon filenames are deliberately not IDs: several reference icons are shared.
"""
import argparse
import csv
import hashlib
import html
import json
from pathlib import Path
import re

ELEMENTS = {'土': 1, '水': 2, '火': 3, '風': 4}
CHOICES = {
    '真・ブレイク': 15074, '大地重撃・改': 11086,
    '真・地裂撃': 11088, '狂岩撃・改': 12025,
    '真・爆岩弾': 11105, '岩石変化・改': 15087,
    '真・岩石変化': 15088, '巨石陣・改': 15152,
    '氷刃': 15091, '真・水の刃': 11096, '旋氷殺・改': 15059,
    '真・旋氷殺': 15073, '凍気爆・改': 11010,
    '水柱術': 15097, '水流弾': 11040,
    '真・無双弾': 11058, '狂雷招来・改': 15010,
    '真・空打': 15081, 'トルネード・改': 12009,
    '真・風塵': 12032, '逆風撃・改': 15115,
    '真・分身斬り': 11012, '裂空術': 30002, '風切り・改': 15009,
}
FIELDS = ['skill_id', 'power', 'element', 'effect_layer', 'sp', 'native_name', 'source', 'source_name']
UNIQUE_BINDINGS = Path(__file__).parent/'reference'/'skill-powers'/'unique_bindings.csv'


def expanded_rows(text):
    """Keep merged owner/name/description cells attached to every skill row."""
    for table_index, table in enumerate(re.findall(r'<table\b[^>]*>(.*?)</table>', text, re.S)):
        active = {}
        for row_index, row in enumerate(re.findall(r'<tr\b[^>]*>(.*?)</tr>', table, re.S)):
            cells = {i: v[0] for i, v in active.items()}
            active = {i: (v, n-1) for i, (v, n) in active.items() if n > 1}
            column = 0
            for attrs, raw in re.findall(r'<t[dh]\b([^>]*)>(.*?)</t[dh]>', row, re.S):
                while column in cells:
                    column += 1
                value = ' '.join(html.unescape(re.sub(r'<[^>]+>', ' ', raw)).split())
                rowspan = re.search(r'rowspan="(\d+)"', attrs)
                colspan = re.search(r'colspan="(\d+)"', attrs)
                for _ in range(int(colspan[1]) if colspan else 1):
                    cells[column] = value
                    if rowspan and int(rowspan[1]) > 1:
                        active[column] = (value, int(rowspan[1])-1)
                    column += 1
            yield table_index, row_index, [cells.get(i, '') for i in range(max(cells, default=-1)+1)]


def unique_power_rows(text):
    for table, row, cells in expanded_rows(text):
        for i, description in enumerate(cells):
            power = re.search(r'スキル威力\s*(\d+)([^\s]*)', description)
            if power and i in (5, 6):
                yield dict(table=table, row=row, name=cells[1], owner=cells[2],
                           grade='' if i == 5 else cells[3], sp=cells[i-2],
                           power=int(power[1]), uncertain=power[2].startswith(('?', '？', '程度', '～', '~')),
                           description=description)


def match_unique_powers(page, native, npcs, bindings):
    source = {(r['table'], r['row']): r for r in unique_power_rows(page.read_text(encoding='utf-8-sig'))}
    skills = {r['fields']['id']: r for r in native}
    owners = {r['fields']['id']: r for r in npcs}
    result, review, matched = [], [], set()
    for binding in bindings:
        key = int(binding['table']), int(binding['row'])
        row = source[key]
        if row['name'] != binding['source_name']:
            raise ValueError(f'unique reference row changed: {key}')
        matched.add(key)
        sid, nid = int(binding['skill_id']), int(binding['npc_id'])
        skill = skills[sid]
        f = skill['fields']
        if row['sp'].isdigit() and int(row['sp']) != f['sp']:
            raise ValueError(f'unique skill SP mismatch: {sid}')
        if nid and sid not in [owners[nid]['fields'][f'skill_{i}'] for i in (1, 2, 3)]:
            raise ValueError(f'NPC {nid} does not own unique skill {sid}')
        layer = 1 if '物理' in row['description'] else 2 if '魔法' in row['description'] else None
        if layer and layer != f['effect_layer']:
            raise ValueError(f'unique skill damage type mismatch: {sid}')
        status = binding['status']
        if row['uncertain'] and status == 'verified':
            raise ValueError(f'uncertain unique power cannot be imported: {sid}')
        if status not in ('verified', 'conditional', 'approximate'):
            raise ValueError(f'unknown binding status: {status}')
        review.append(dict(table=key[0], row=key[1], source_name=row['name'], owner=row['owner'],
                           grade=row['grade'], source_sp=row['sp'], source_power=row['power'],
                           skill_id=sid, native_name=skill['name']['text'], native_sp=f['sp'],
                           npc_id=nid, npc_name=owners[nid]['name']['text'] if nid else '',
                           status=status, evidence=binding['evidence']))
        if status == 'verified':
            result.append(dict(skill_id=sid, power=row['power'], element=f['element'],
                               effect_layer=f['effect_layer'], sp=f['sp'], native_name=skill['name']['text'],
                               source=page.name, source_name=row['name']))
    if matched != set(source):
        raise ValueError(f'unmatched unique skill power rows: {set(source)-matched}')
    return result, review


def power_rows(text):
    # Only physical/magical progression tables; support descriptions may mention
    # another skill's damage, and unique-skill icon numbers are not reliable IDs.
    for layer, table in enumerate(re.findall(r'<table\b[^>]*>(.*?)</table>', text, re.S)[:2], 1):
        for row in re.findall(r'<tr\b[^>]*>(.*?)</tr>', table, re.S):
            cells = [html.unescape(re.sub(r'<[^>]+>', ' ', s)).strip()
                     for s in re.findall(r'<t[dh]\b[^>]*>(.*?)</t[dh]>', row, re.S)]
            if cells and not cells[0]:
                cells = cells[1:]
            if len(cells) < 5 or not re.fullmatch(r'\d+', cells[2]):
                continue
            power = re.search(r'スキル威力\s*(\d+)([^\s]*)', cells[4])
            if power and not power[2].startswith(('?', '？', '程度', '～', '~')):
                yield layer, cells[0], int(cells[2]), int(power[1])


def extract(pages, native, npcs, bindings):
    result, sources = [], []
    for page in sorted(pages.glob('*.htm*')):
        raw = page.read_bytes()
        element = ELEMENTS.get(page.name[0])
        sources.append(dict(file=page.name, sha256=hashlib.sha256(raw).hexdigest(), bytes=len(raw)))
        if not element:
            continue
        if not raw:
            raise ValueError(f'empty elemental reference: {page.name}')
        for layer, name, sp, power in power_rows(raw.decode('utf-8-sig')):
            matches = [r for r in native if r['fields']['element'] == element
                       and r['fields']['effect_layer'] == layer and r['fields']['sp'] == sp
                       and r['fields']['table_order'] > 0]
            if name in CHOICES:
                matches = [r for r in matches if r['fields']['id'] == CHOICES[name]]
            if len(matches) != 1:
                raise ValueError(f'{name}: unresolved native skill {[r["fields"]["id"] for r in matches]}')
            r = matches[0]
            result.append(dict(skill_id=r['fields']['id'], power=power, element=element,
                               effect_layer=layer, sp=sp, native_name=r['name']['text'],
                               source=page.name, source_name=name))
    unique, review = match_unique_powers(next(pages.glob('固*.htm*')), native, npcs, bindings)
    by_id = {}
    for row in result+unique:
        previous = by_id.get(row['skill_id'])
        if previous and previous != row:
            raise ValueError(f'conflicting skill power binding: {row["skill_id"]}')
        by_id[row['skill_id']] = row
    return sorted(by_id.values(), key=lambda r: r['skill_id']), sources, review


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--pages', type=Path, required=True)
    p.add_argument('--skill-export', type=Path, required=True)
    p.add_argument('--npc-export', type=Path, required=True)
    p.add_argument('--unique-bindings', type=Path, default=UNIQUE_BINDINGS)
    p.add_argument('--output', type=Path, required=True)
    a = p.parse_args()
    with a.unique_bindings.open(encoding='utf-8', newline='') as f:
        bindings = list(csv.DictReader(f))
    rows, sources, review = extract(a.pages, json.loads(a.skill_export.read_text(encoding='utf-8'))['records'],
                                   json.loads(a.npc_export.read_text(encoding='utf-8'))['records'], bindings)
    with a.output.open('w', encoding='utf-8', newline='') as f:
        writer = csv.DictWriter(f, FIELDS, lineterminator='\n')
        writer.writeheader()
        writer.writerows(rows)
    with a.output.with_suffix('.unique-review.csv').open('w', encoding='utf-8', newline='') as f:
        writer = csv.DictWriter(f, list(review[0]), lineterminator='\n')
        writer.writeheader()
        writer.writerows(review)
    a.output.with_suffix('.sources.json').write_text(json.dumps(dict(
        pages=sources, skills=len(rows), native_sha256=hashlib.sha256(a.skill_export.read_bytes()).hexdigest(),
        npc_sha256=hashlib.sha256(a.npc_export.read_bytes()).hexdigest(),
        bindings_sha256=hashlib.sha256(a.unique_bindings.read_bytes()).hexdigest(),
        unique_bindings=len(review), unique_imported=len({r['skill_id'] for r in review if r['status']=='verified'}),
        scope='Explicit elemental and verified unique powers; approximate and grade-conditional powers remain review-only.'),
        ensure_ascii=False, indent=2)+'\n', encoding='utf-8')
    print(f'Extracted {len(rows)} verified skill powers')


if __name__ == '__main__':
    main()

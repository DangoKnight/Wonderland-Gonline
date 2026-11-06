"""Import ID-matched original Traditional Chinese text as language-catalog data."""
import argparse
import csv
import hashlib
import json
from pathlib import Path

from export import FORMATS, export_records, transform
from extract_igg_translations import TABLE_FILES, EXPORT_FILES, ITEM_CIPHER, target_rows, identity_matches, without_text

ITEM_ID_OFFSET = 16
ITEM_ID_BYTES = 2
ITEM_NAME_LENGTH_OFFSET = 0
ITEM_NAME_OFFSET = 1
ITEM_DESCRIPTION_LENGTH_OFFSET = 146
ITEM_DESCRIPTION_OFFSET = 147


def extract(client, target, output):
    entries = []
    counts = {}
    for table, filename in TABLE_FILES.items():
        source = next(p for p in (client / 'data').iterdir() if p.name.lower() == filename.lower())
        raw = source.read_bytes()
        if table == 'item':
            rows = []
            for offset in range(0, len(raw), ITEM_CIPHER['size']):
                b = transform(raw[offset:offset+ITEM_CIPHER['size']], ITEM_CIPHER)
                rows.append({'fields': {'id': int.from_bytes(b[ITEM_ID_OFFSET:ITEM_ID_OFFSET+ITEM_ID_BYTES], 'little')},
                             'name': {'text': b[ITEM_NAME_OFFSET:ITEM_NAME_OFFSET+b[ITEM_NAME_LENGTH_OFFSET]].decode('cp950', errors='replace')},
                             'description': {'text': b[ITEM_DESCRIPTION_OFFSET:ITEM_DESCRIPTION_OFFSET+b[ITEM_DESCRIPTION_LENGTH_OFFSET]].decode('cp950', errors='replace')},
                             'decoded_hex': b.hex()})
        else:
            rows = export_records(raw, FORMATS[filename.lower()])['records']
        targets = target_rows(table, json.loads((target / EXPORT_FILES[table]).read_text(encoding='utf-8')))
        count = 0
        for row in rows:
            ident = row['fields']['id']
            other = targets.get(ident)
            if not other or not identity_matches(table, row, other):
                continue
            for field in ('name', 'description', 'text', 'secondary_name'):
                if field not in row or not row[field]['text']:
                    continue
                if field == 'description' and without_text(table, bytes.fromhex(row['decoded_hex'])) != without_text(table, bytes.fromhex(other['decoded_hex'])):
                    continue
                entries.append([table, ident, field, row[field]['text'],
                                hashlib.sha256(bytes.fromhex(row['decoded_hex'])).hexdigest(),
                                hashlib.sha256(bytes.fromhex(other['decoded_hex'])).hexdigest()])
                count += 1
        counts[table] = count
    output.parent.mkdir(parents=True, exist_ok=True)
    with output.open('w', encoding='utf-8', newline='') as f:
        writer = csv.writer(f)
        writer.writerow(['table', 'id', 'field', 'text', 'source_record_sha256', 'target_record_sha256'])
        writer.writerows(entries)
    return counts


def extract_skins(client, output):
    # Rasterized labels retain their logical bitmap names, with locale-specific
    # artwork registered before the common skin at client startup.
    from ui import export_ui
    return len(export_ui(client, output, 'zh-TW')['files'])


if __name__ == '__main__':
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--client', type=Path, required=True)
    p.add_argument('--target', type=Path, default=Path('data'))
    p.add_argument('--output', type=Path, default=Path('internal/clienttext/chinese_text.csv'))
    p.add_argument('--media-output', type=Path, help='Also export original Chinese bitmap UI labels into this media directory')
    args = p.parse_args()
    print(json.dumps(extract(args.client, args.target, args.output)))
    if args.media_output:
        print(json.dumps({'skin_bitmaps': extract_skins(args.client, args.media_output)}))

"""Attach language sources to exports without changing native records or bytes."""
import json
from pathlib import Path

from export import FORMATS, export_records, require, sha256, write_json
from extract_igg_translations import (EXPORT_FILES, TABLE_FILES, ITEM_CIPHER,
                                     target_rows, identity_matches, without_text)
from extract_chinese_text import (ITEM_ID_OFFSET, ITEM_ID_BYTES, ITEM_NAME_OFFSET,
                                 ITEM_NAME_LENGTH_OFFSET, ITEM_DESCRIPTION_OFFSET,
                                 ITEM_DESCRIPTION_LENGTH_OFFSET)
from export import transform

LANGUAGES = ('en', 'zh-TW')


def native_rows(client, table):
    source = next((p for p in (client / 'data').iterdir()
                   if p.name.lower() == TABLE_FILES[table].lower()), None)
    require(source is not None, f'missing language source: {TABLE_FILES[table]}')
    raw = source.read_bytes()
    if table != 'item':
        return source, export_records(raw, FORMATS[TABLE_FILES[table].lower()])['records']
    require(len(raw) % ITEM_CIPHER['size'] == 0, 'invalid native item table length')
    rows = []
    # The first item record is the format header, not an item.
    for offset in range(ITEM_CIPHER['size'], len(raw), ITEM_CIPHER['size']):
        b = transform(raw[offset:offset + ITEM_CIPHER['size']], ITEM_CIPHER)
        rows.append({'fields': {'id': int.from_bytes(b[ITEM_ID_OFFSET:ITEM_ID_OFFSET + ITEM_ID_BYTES], 'little')},
                     'name': {'text': b[ITEM_NAME_OFFSET:ITEM_NAME_OFFSET + b[ITEM_NAME_LENGTH_OFFSET]].decode('cp950', errors='replace')},
                     'description': {'text': b[ITEM_DESCRIPTION_OFFSET:ITEM_DESCRIPTION_OFFSET + b[ITEM_DESCRIPTION_LENGTH_OFFSET]].decode('cp950', errors='replace')},
                     'decoded_hex': b.hex()})
    return source, rows


def annotate_languages(destination, language='en', chinese_client=None):
    require(language in LANGUAGES, 'unsupported client text language')
    require(language != 'zh-TW' or chinese_client is None,
            '--chinese-client supplements an English --client; omit it for Chinese-only input')
    manifest_path = destination / 'asset_manifest.json'
    manifest = json.loads(manifest_path.read_text(encoding='utf-8'))
    for table, filename in EXPORT_FILES.items():
        path = destination / filename
        if not path.is_file():
            continue
        document = json.loads(path.read_text(encoding='utf-8'))
        document['source_language'] = language
        if chinese_client is not None:
            source, rows = native_rows(chinese_client, table)
            targets = target_rows(table, document)
            texts = {}
            for row in rows:
                ident = row['fields']['id']
                other = targets.get(ident)
                if other is None or not identity_matches(table, row, other):
                    continue
                for field in ('name', 'description', 'text', 'secondary_name'):
                    if field not in row:
                        continue
                    if field == 'description' and without_text(table, bytes.fromhex(row['decoded_hex'])) != without_text(table, bytes.fromhex(other['decoded_hex'])):
                        continue
                    texts[f'{table}.{ident}.{field}'] = row[field]['text']
            document['text_languages'] = {'zh-TW': texts}
            document['text_language_sources'] = {'zh-TW': {'source': str(source), 'source_sha256': sha256(source)}}
        write_json(path, document)
        entry = next(e for e in manifest['assets'] if e['output'] == filename)
        entry['output_bytes'], entry['output_sha256'] = path.stat().st_size, sha256(path)
    write_json(manifest_path, manifest)


def verify_language_sources(document):
    for language, provenance in document.get('text_language_sources', {}).items():
        require(language in LANGUAGES, 'unsupported companion text language')
        source = Path(provenance['source'])
        require(source.is_file() and sha256(source) == provenance['source_sha256'],
                f'companion text source changed: {source}')

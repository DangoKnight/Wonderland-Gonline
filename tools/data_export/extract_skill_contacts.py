"""Extract server-only contact counts; client animation payloads stay untracked."""
import argparse
import csv
import hashlib
import io
import json
from pathlib import Path

from export import write_json

ANIMATION_HEADER_BYTES = 27
ANIMATION_DAMAGE_MODE_OFFSET = 24
ANIMATION_CONTACT_COUNT_OFFSET = 25
ANIMATION_CONTACT_BYTES = 5
MAX_CONTACT_EVENTS = 50
SINGLE_DAMAGE_MODE = 0


def extract(path, output):
    raw = path.read_bytes()
    doc = json.loads(raw)
    counts = {}
    for row in doc['records']:
        record = bytes.fromhex(row['decoded_hex'])
        if (len(record) < ANIMATION_HEADER_BYTES
                or record[ANIMATION_CONTACT_COUNT_OFFSET] > MAX_CONTACT_EVENTS
                or len(record) < ANIMATION_HEADER_BYTES
                + record[ANIMATION_CONTACT_COUNT_OFFSET] * ANIMATION_CONTACT_BYTES):
            raise ValueError(f'invalid contact records for animation {row["id"]}')
        if row['id'] in counts:
            raise ValueError('duplicate animation ID')
        counts[row['id']] = (max(1, record[ANIMATION_CONTACT_COUNT_OFFSET])
                             if record[ANIMATION_DAMAGE_MODE_OFFSET] != SINGLE_DAMAGE_MODE else 1)
    stream = io.StringIO(newline='')
    writer = csv.writer(stream, lineterminator='\n')
    writer.writerow(('animation_id', 'contact_events'))
    writer.writerows(sorted(counts.items()))
    output.parent.mkdir(parents=True, exist_ok=True)
    data = stream.getvalue().encode('utf-8')
    output.write_bytes(data)
    write_json(output.with_suffix('.sources.json'), {
        'format': 'server-skill-contact-counts', 'version': 1,
        'source': 'SkillData.MBTM', 'source_sha256': doc['source_sha256'],
        'export_sha256': hashlib.sha256(raw).hexdigest(),
        'table_sha256': hashlib.sha256(data).hexdigest(), 'records': len(counts),
        'decoder': 'FUN_00377060; disk offsets 24 (damage mode), 25 (event count)',
        'single_damage_mode': SINGLE_DAMAGE_MODE, 'missing_or_empty_contacts': 1,
    })
    return counts


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--animation-export', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    print(f'Exported {len(extract(args.animation_export, args.output))} contact counts')

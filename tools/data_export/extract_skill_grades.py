"""Extract repository-owned Skill.dat grade limits for server initialization."""
import argparse
import csv
import hashlib
import io
import json
from pathlib import Path

from export import write_json

SKILL_RECORD_BYTES = 148
SKILL_MAXIMUM_GRADE_OFFSET = 105


def extract(source, output):
    raw = source.read_bytes()
    doc = json.loads(raw)
    grades = {}
    for row in doc['records']:
        record = bytes.fromhex(row['decoded_hex'])
        if len(record) != SKILL_RECORD_BYTES:
            raise ValueError('invalid Skill.dat record size')
        skill_id = row['fields']['id']
        if skill_id == 0:
            continue
        if skill_id in grades:
            raise ValueError('duplicate skill ID')
        grades[skill_id] = record[SKILL_MAXIMUM_GRADE_OFFSET]
    stream = io.StringIO(newline='')
    writer = csv.writer(stream, lineterminator='\n')
    writer.writerow(('skill_id', 'maximum_grade'))
    writer.writerows(sorted(grades.items()))
    data = stream.getvalue().encode('utf-8')
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_bytes(data)
    write_json(output.with_suffix('.sources.json'), {
        'format': 'server-skill-maximum-grades', 'version': 1,
        'source': 'Skill.dat', 'source_sha256': doc['source_sha256'],
        'export_sha256': hashlib.sha256(raw).hexdigest(),
        'table_sha256': hashlib.sha256(data).hexdigest(), 'records': len(grades),
        'decoded_maximum_grade_offset': SKILL_MAXIMUM_GRADE_OFFSET,
        'missing_grade_limit': 10, 'explicit_zero_grade_limit': 0,
    })
    return grades


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--skill-export', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    print(f'Exported {len(extract(args.skill_export, args.output))} skill grade limits')

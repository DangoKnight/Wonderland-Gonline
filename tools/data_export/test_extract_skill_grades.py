import hashlib
import json
from pathlib import Path
import tempfile
import unittest

from extract_skill_grades import extract


class SkillGradeExportTests(unittest.TestCase):
    def test_native_grade_and_portable_provenance(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            record = bytearray(148)
            record[105] = 15
            source, output = root / 'skills.json', root / 'grades.csv'
            source.write_text(json.dumps({'source_sha256': 'native-hash', 'records': [
                {'fields': {'id': 50000}, 'decoded_hex': record.hex()},
            ]}))
            self.assertEqual(extract(source, output), {50000: 15})
            self.assertEqual(output.read_bytes(), b'skill_id,maximum_grade\n50000,15\n')
            metadata = json.loads(output.with_suffix('.sources.json').read_text())
            self.assertEqual(metadata['table_sha256'], hashlib.sha256(output.read_bytes()).hexdigest())
            record[105] = 0
            doc = json.loads(source.read_text())
            doc['records'][0]['decoded_hex'] = record.hex()
            source.write_text(json.dumps(doc))
            self.assertEqual(extract(source, output), {50000: 0})

    def test_invalid_records_do_not_publish(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source, output = root / 'skills.json', root / 'grades.csv'
            source.write_text(json.dumps({'source_sha256': 'hash', 'records': [
                {'fields': {'id': 50000}, 'decoded_hex': '00'},
            ]}))
            with self.assertRaisesRegex(ValueError, 'record size'):
                extract(source, output)
            self.assertFalse(output.exists())

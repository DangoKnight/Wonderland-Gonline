import hashlib
import json
from pathlib import Path
import tempfile
import unittest

from extract_skill_contacts import extract


class SkillContactExportTests(unittest.TestCase):
    def test_server_table_uses_damage_mode_and_records_provenance(self):
        with tempfile.TemporaryDirectory() as root:
            root=Path(root)
            record=bytearray(42);record[24:26]=bytes([2,3])
            doc={'source_sha256':'native-hash','records':[{'id':11016,'decoded_hex':record.hex()}]}
            source=root/'animation.json';source.write_text(json.dumps(doc))
            output=root/'counts.csv'
            self.assertEqual(extract(source,output),{11016:3})
            self.assertEqual(output.read_text(),'animation_id,contact_events\n11016,3\n')
            provenance=json.loads(output.with_suffix('.sources.json').read_text())
            self.assertEqual(provenance['table_sha256'],hashlib.sha256(output.read_bytes()).hexdigest())
            record[24]=0;doc['records'][0]['decoded_hex']=record.hex();source.write_text(json.dumps(doc))
            self.assertEqual(extract(source,output),{11016:1})

    def test_truncated_records_do_not_publish_a_table(self):
        with tempfile.TemporaryDirectory() as root:
            root=Path(root);source=root/'animation.json';output=root/'counts.csv'
            source.write_text(json.dumps({'source_sha256':'hash','records':[{'id':1,'decoded_hex':'00'}]}))
            with self.assertRaisesRegex(ValueError,'invalid contact'):
                extract(source,output)
            self.assertFalse(output.exists())

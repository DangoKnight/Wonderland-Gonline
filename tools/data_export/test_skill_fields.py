"""Independent wire fixtures for readable Skill.dat labels and legacy aliases."""
import copy
import struct
import unittest

from export import FORMATS, export_records
from verify import check_readable, reconstruct
from skill_fields import label_existing_export


class SkillFieldTests(unittest.TestCase):
    def fixture(self):
        header = bytearray(148)
        header[21] = 11
        wire = bytearray(148)
        # Literal cipher bytes, independent of the production transform map.
        wire[22:24] = bytes.fromhex('ac45')  # Skill 11016.
        wire[49:51] = bytes.fromhex('b86e')  # Additive coefficient 20.
        wire[51:53] = bytes.fromhex('fac5')  # Three rounds, effect code 52.
        wire[54:56] = bytes.fromhex('ac45')  # Secondary skill reference 11016.
        wire[99] = 250                      # Self target, 3.
        wire[100:102] = bytes.fromhex('df6e')  # Icon 123.
        wire[105:107] = bytes.fromhex('f3fa')  # Grade 10; additional effect 3.
        wire[109:114] = bytes.fromhex('fbfaf6f1f8')  # Patterns 2/3/7/8; injury 1.
        wire[118:120] = bytes.fromhex('806a')  # Voice 1052.
        struct.pack_into('<dd', wire, 32, 2.5, 0.75)
        return bytes(header + wire)

    def test_readable_names_preserve_values_and_original_wire(self):
        wire = self.fixture()
        doc = export_records(wire, FORMATS['skill.dat'])
        fields = doc['records'][0]['fields']
        for name, value in {
            'id': 11016, 'additional_damage': 20, 'effect_rounds': 3,
            'effect_code': 52, 'secondary_skill_id': 11016, 'target_policy': 3,
            'icon_id': 123, 'maximum_grade': 10, 'additional_effect_code': 3,
            'target_pattern_grade_1_3': 2, 'target_pattern_grade_4_6': 3,
            'target_pattern_grade_7_9': 7, 'target_pattern_grade_10': 8,
            'injury_type': 1, 'battle_voice_id': 1052,
            'power_per_level': 2.5, 'stat_multiplier': 0.75,
        }.items():
            self.assertEqual(fields[name], value, name)
        self.assertEqual(fields['unknown_u8_offset_51'], 3)
        self.assertEqual(fields['unknown_u8_offset_52'], 52)
        self.assertEqual(fields['unknown_u8_offset_109'], 2)
        self.assertIn('unknown_u8_offset_30', fields)
        self.assertNotIn('unknown_u8_offset_99', fields)
        check_readable(doc)
        self.assertEqual(reconstruct(doc, 'skill.dat'), wire)

    def test_labels_distinguish_native_reference_and_compatibility(self):
        doc = export_records(self.fixture(), FORMATS['skill.dat'])
        layout = {field['offset']: field for field in doc['field_layout']}
        self.assertEqual(layout[99]['confidence'], 'native')
        self.assertEqual(layout[113]['confidence'], 'reference')
        self.assertEqual(layout[32]['confidence'], 'compatibility')
        self.assertEqual(layout[30]['confidence'], 'unresolved')
        self.assertEqual(layout[32]['encoding'], 'float64-le')
        self.assertEqual(layout[51]['legacy_name'], 'unknown_u8_offset_51')
        for field in layout.values():
            self.assertTrue(field['label'])
            if field['confidence'] != 'unresolved':
                self.assertTrue(field['evidence'])

    def test_verifier_rejects_edited_float_or_compatibility_alias(self):
        doc = export_records(self.fixture(), FORMATS['skill.dat'])
        for key in ('power_per_level', 'unknown_u8_offset_51', 'effect_code'):
            with self.subTest(key=key):
                corrupt = copy.deepcopy(doc)
                corrupt['records'][0]['fields'][key] += 1
                with self.assertRaisesRegex(ValueError, 'differs from decoded bytes'):
                    check_readable(corrupt)

    def test_existing_export_preserves_authored_data_and_is_idempotent(self):
        doc = export_records(self.fixture(), FORMATS['skill.dat'])
        row = doc['records'][0]
        row['name'] = {'text': '@{skill.11016.name}'}
        row['fields']['effect_refs'] = ['authored.effect']
        row['fields']['base_power'] = 20
        # Simulate the previous offset-based export layout.
        field = next(f for f in doc['field_layout'] if f['offset'] == 99)
        field['name'] = 'unknown_u8_offset_99'
        row['fields']['unknown_u8_offset_99'] = row['fields'].pop('target_policy')
        labelled = label_existing_export(doc)
        self.assertEqual(labelled['records'][0]['name'], row['name'])
        self.assertEqual(labelled['records'][0]['fields']['effect_refs'], ['authored.effect'])
        self.assertEqual(labelled['records'][0]['fields']['base_power'], 20)
        self.assertEqual(labelled['records'][0]['decoded_hex'], row['decoded_hex'])
        self.assertEqual(label_existing_export(labelled), labelled)
        self.assertEqual(reconstruct(labelled, 'skill.dat'), self.fixture())
        self.assertIn('unknown_u8_offset_99', row['fields'])  # Input was not mutated.

    def test_conflicting_aliases_are_rejected(self):
        doc = export_records(self.fixture(), FORMATS['skill.dat'])
        doc['records'][0]['fields']['unknown_u8_offset_51'] = 9
        # Both aliases must remain equal even when relabelling an existing file.
        with self.assertRaisesRegex(ValueError, 'conflicting skill field aliases'):
            label_existing_export(doc)

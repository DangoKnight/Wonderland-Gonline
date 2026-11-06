import unittest
import tempfile
from pathlib import Path

from extract_skill_powers import power_rows, unique_power_rows, match_unique_powers


class SkillPowerExtractionTests(unittest.TestCase):
    def test_explicit_power_and_modified_row(self):
        table = '<table><tr><td></td><td>Base</td><td>Initial</td><td>9</td><td>1</td><td>スキル威力20</td></tr><tr><td>Modified</td><td>BaseLv5</td><td>10</td><td>1</td><td>スキル威力50</td></tr></table>'
        self.assertEqual(list(power_rows(table)), [(1, 'Base', 9, 20), (1, 'Modified', 10, 50)])

    def test_uncertain_power_is_not_imported(self):
        for suffix in ('?', '？', '程度', '～90'):
            table = f'<table><tr><td>Unknown</td><td>Quest</td><td>9</td><td>1</td><td>スキル威力80{suffix}</td></tr></table>'
            self.assertEqual(list(power_rows(table)), [])

    def test_unique_merged_owner_and_name_are_retained(self):
        table = '<table><tr><td rowspan="2"></td><td rowspan="2">Horn</td><td>Beetle</td><td>8</td><td>8</td><td>1</td><td>物理 スキル威力20</td></tr><tr><td>Strong Beetle</td><td>8</td><td>9</td><td>1</td><td>物理 スキル威力35</td></tr></table>'
        rows = list(unique_power_rows(table))
        self.assertEqual([(r['name'], r['owner'], r['sp'], r['power']) for r in rows], [('Horn', 'Beetle', '8', 20), ('Horn', 'Strong Beetle', '9', 35)])

    def test_unique_binding_checks_ownership_and_defers_conditional_power(self):
        table = '<table><tr><td></td><td>Thunder</td><td>Pet</td><td>10~12</td><td>110</td><td>6</td><td>魔法 スキル威力180</td></tr></table>'
        skill = dict(fields=dict(id=15135, sp=110, element=2, effect_layer=2), name=dict(text='Sky Thunder'))
        owner = dict(fields=dict(id=14081, skill_1=15135, skill_2=0, skill_3=0), name=dict(text='Niss'))
        binding = dict(table='0', row='0', source_name='Thunder', skill_id='15135', npc_id='14081', status='conditional', evidence='Owned; grade-specific power')
        with tempfile.TemporaryDirectory() as directory:
            page = Path(directory)/'unique.htm'
            page.write_text(table, encoding='utf-8')
            powers, report = match_unique_powers(page, [skill], [owner], [binding])
            self.assertEqual(powers, [])
            self.assertEqual(report[0]['skill_id'], 15135)
            owner['fields']['skill_1'] = 0
            with self.assertRaisesRegex(ValueError, 'does not own'):
                match_unique_powers(page, [skill], [owner], [binding])


if __name__ == '__main__':
    unittest.main()

from pathlib import Path
import tempfile
import unittest
import zipfile

import sample_compare as sample


class SampleComparisonTests(unittest.TestCase):
    def test_reads_observation_cells_without_evaluating_text_as_formula(self):
        source = '''<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
            xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0"
            xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0"><office:body><office:spreadsheet>
            <table:table table:name="Measurements"><table:table-row table:number-rows-repeated="16"/>
            <table:table-row><table:table-cell table:number-columns-repeated="3"/>
            <table:table-cell><text:p>76 + 77</text:p></table:table-cell></table:table-row>
            </table:table></office:spreadsheet></office:body></office:document-content>'''
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory)/'measurements.ods'
            with zipfile.ZipFile(path, 'w') as archive:
                archive.writestr('content.xml', source)
            before = path.read_bytes()
            self.assertEqual(sample.read_ods(path), [dict(name='Measurements', cells={'D17': '76 + 77'})])
            self.assertEqual(path.read_bytes(), before)

    def test_flame_attack_matches_independent_runtime_replay(self):
        skill = dict(value_maximum_grade=5, value_maximum_grade_present=True,
                     value_additional_damage=7, value_power_per_level=2.8, value_stat_multiplier=.6,
                     value_contact_events=2, value_effect_layer=1, value_element=3)
        actor = dict(atk=77, matk=25, element=3)
        enemy = dict(defense=34, magic_defense=44, element=4)
        current = sample.predictions(actor, enemy, skill, 5, 'current server')
        self.assertEqual((current['minimum'], current['maximum']), (118, 122))
        self.assertEqual(current['power'], 42)
        candidate = sample.predictions(actor, enemy, skill, 5, 'native total P, no stat multiplier')
        self.assertEqual((candidate['minimum'], candidate['maximum']), (153, 157))
        self.assertEqual(candidate['power'], 21)

    def test_magical_underflow_clamps_before_randomness(self):
        skill = dict(value_maximum_grade=5, value_maximum_grade_present=True,
                     value_additional_damage=4, value_power_per_level=1.38, value_stat_multiplier=1.1,
                     value_contact_events=1, value_effect_layer=2, value_element=3)
        result = sample.predictions(dict(atk=77, matk=25, element=3),
                                     dict(defense=34, magic_defense=44, element=4),
                                     skill, 1, 'current server')
        self.assertEqual((result['minimum'], result['maximum']), (2, 6))

    def test_neutral_enemy_uses_baseline_growth(self):
        male = sample.enemy_stats(dict(value_id=17070,value_name='Male Bug',value_level=23,value_element=0,
                                      **{f'value_stats{i}':v for i,v in enumerate((15,13,13,14,14))}))
        self.assertEqual((male['defense'],male['magic_defense']), (72,74))
        female = sample.enemy_stats(dict(value_id=17071,value_name='Female Bug',value_level=24,value_element=0,
                                        **{f'value_stats{i}':v for i,v in enumerate((14,14,15,15,14))}))
        self.assertEqual((female['defense'],female['magic_defense']), (76,78))

    def test_legacy_magic_uses_max_while_gonline_keeps_mdef_only(self):
        actor = dict(atk=0,matk=100,element=3)
        enemy = dict(defense=100,magic_defense=20,element=0)
        skill = dict(value_effect_layer=2,value_element=3)
        current = sample.damage_from_power(actor,enemy,skill,0,1)
        legacy = sample.damage_from_power(actor,enemy,skill,0,1,sample.MAGIC_DEFENSE_LEGACY_MAX)
        self.assertEqual((current['minimum'],current['maximum']), (157,161))
        self.assertEqual((legacy['minimum'],legacy['maximum']), (55,59))
        enemy.update(defense=34,magic_defense=44)
        self.assertEqual(sample.damage_from_power(actor,enemy,skill,0,1),
                         sample.damage_from_power(actor,enemy,skill,0,1,sample.MAGIC_DEFENSE_LEGACY_MAX))


if __name__ == '__main__':
    unittest.main()

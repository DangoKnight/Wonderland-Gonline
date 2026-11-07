import unittest

import reference_compare as reference
import selected_damage_compare as selected


class ReferenceComparisonTests(unittest.TestCase):
    def test_native_neutral_does_not_inherit_fire(self):
        row = dict(actor=dict(atk=100, matk=0, element=3), enemy=dict(defense=50, magic_defense=0, element=4), grade=1)
        skill = dict(value_element=0, value_effect_layer=1, value_maximum_grade_present=False,
                     value_maximum_grade=0, value_additional_damage=0, value_power_per_level=0)
        result = reference.prediction(row, skill, 'current')
        self.assertEqual((result['minimum'], result['maximum']), (92, 96))
        row['enemy']['element'] = 1
        result = reference.prediction(row, skill, 'current')
        self.assertEqual((result['minimum'], result['maximum']), (82, 86))
        row['enemy']['element'] = 4
        skill['value_element'] = 7
        result = reference.prediction(row, skill, 'current')
        self.assertEqual((result['minimum'], result['maximum']), (146, 150))

    def test_low_mat_alternative_is_separate_from_current(self):
        row = dict(actor=dict(atk=74, matk=25, element=3), enemy=dict(defense=33, magic_defense=39, element=2), grade=2)
        skill = dict(value_element=3, value_effect_layer=2, value_maximum_grade_present=True,
                     value_maximum_grade=5, value_additional_damage=4, value_power_per_level=1.38)
        current = reference.prediction(row, skill, 'current')
        alternate = reference.prediction(row, skill, 'magic_below_defense')
        self.assertEqual((current['minimum'], current['maximum']), (10, 14))
        self.assertEqual((alternate['minimum'], alternate['maximum']), (16, 20))
        self.assertEqual(current['power'], alternate['power'])

    def test_grade_one_low_mat_is_checked_without_changing_native_power(self):
        row = dict(actor=dict(atk=77,matk=25,element=3),
                   enemy=dict(defense=34,magic_defense=44,element=4),grade=1)
        skill = dict(value_element=3,value_effect_layer=2,value_maximum_grade_present=True,
                     value_maximum_grade=5,value_additional_damage=4,value_power_per_level=1.38)
        result=reference.prediction(row,skill,'current')
        self.assertEqual(result['power'],10.76)
        self.assertEqual((result['minimum'],result['maximum']),(18,22))
        previous=reference.prediction(row,skill,'pre_stat_floor')
        self.assertEqual((previous['minimum'],previous['maximum']),(6,10))

    def test_policy_conflict_examples_stay_visible(self):
        row = next(row for row in reference.source_checks() if row['row'] == 12)
        self.assertAlmostEqual(row['predictions']['legacy_max'], 2493.246)
        self.assertAlmostEqual(row['predictions']['project_mdef'], 2672.292)

    def test_high_stat_examples_remove_unmeasured_correction(self):
        for row in reference.source_checks():
            self.assertEqual(row['predictions']['project_mdef'], row['predictions']['refined_fading_q'])
            if row['row'] in (14, 15, 17):
                self.assertLess(abs(row['predictions']['refined_fading_q']-row['observed']), 1)
                self.assertGreater(abs(row['predictions']['previous_clamped_q']-row['observed']), 6)

    def test_fit_retains_measured_boundary_and_fades_monotonically(self):
        self.assertAlmostEqual(selected.adjustment(1052, 78), 9.281831313453857)
        self.assertAlmostEqual(selected.adjustment(1578, 78), selected.adjustment(1052, 78)/2)
        self.assertEqual(selected.adjustment(2104, 50), 0)
        self.assertEqual(selected.adjustment(238, 156), 0)


if __name__ == '__main__':
    unittest.main()

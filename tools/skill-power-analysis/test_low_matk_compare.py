import unittest

import low_matk_compare as low


class ControlledComparisonTests(unittest.TestCase):
    def test_separates_mat_and_grade_changes(self):
        common = dict(skill='Synthetic',target='Synthetic target',element_factor=.7,
                      actor=dict(name='Research actor',level=13))
        groups = [dict(common,matk=25,grade=2,mean=10),dict(common,matk=29,grade=2,mean=14),
                  dict(common,matk=29,grade=3,mean=16)]
        changes = low.comparisons(groups,dict(value_power_per_level=1.38,value_stat_multiplier=1.1))
        self.assertEqual([r['kind'] for r in changes],['MAT change','grade change'])
        self.assertEqual([r['observed_mean_change'] for r in changes],[4,2])
        self.assertAlmostEqual(changes[0]['expected_change_before_rounding_noise']['coefficient_1_4'],3.92)
        self.assertAlmostEqual(changes[1]['expected_change_before_rounding_noise']['twice_D'],1.932)

    def test_different_actors_are_not_treated_as_controlled_change(self):
        common = dict(skill='Synthetic',target='Synthetic target',element_factor=1.2,grade=2,mean=20)
        groups = [dict(common,matk=25,actor=dict(name='A',level=13)),
                  dict(common,matk=29,actor=dict(name='B',level=13))]
        self.assertEqual(low.comparisons(groups,dict(value_power_per_level=1.38,value_stat_multiplier=1.1)),[])

    def test_recovers_slopes_with_unknown_separate_target_intercepts(self):
        groups = []
        for target,factor,baseline in [('A',.7,10),('B',1.2,25)]:
            for mat in (25,29):
                for grade in (2,3):
                    center = baseline+factor*(1.4*(mat-25)+2.76*(grade-2))
                    groups.append(dict(target=target,element_factor=factor,matk=mat,grade=grade,
                                       observed=[center-1,center+1]))
        result = low.fit_controlled_effects(groups)
        self.assertTrue(result['available'])
        self.assertAlmostEqual(result['coefficient_per_MAT_before_element'],1.4)
        self.assertAlmostEqual(result['additive_power_per_grade'],2.76)
        self.assertAlmostEqual(result['target_intercepts']['A'],10)
        self.assertAlmostEqual(result['target_intercepts']['B'],25)
        self.assertGreater(result['standard_error_MAT'],0)

    def test_slope_fit_reports_insufficient_settings(self):
        result = low.fit_controlled_effects([dict(target='A',element_factor=.7,matk=25,grade=2,observed=[10,11,12])])
        self.assertFalse(result['available'])


if __name__=='__main__':
    unittest.main()

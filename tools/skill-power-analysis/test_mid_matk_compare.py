import unittest

import mid_matk_compare as mid


class MidMatkTests(unittest.TestCase):
    def skill(self):
        return dict(value_targeting_present=False, value_area_attack_present=False,
                    value_native_pattern109=1, value_native_pattern110=1,
                    value_native_pattern111=1, value_native_pattern112=1,
                    value_maximum_grade_present=True, value_maximum_grade=5,
                    value_effect_layer=2, value_additional_damage=4,
                    value_power_per_level=1.38, value_element=3)

    def test_new_blast_current_prediction_uses_mat_and_mdef(self):
        actor = dict(matk=238, atk=1104, element=None)
        enemy = dict(defense=9999, magic_defense=74, element=0)
        q = mid.current_prediction(actor, enemy, self.skill(), 1)
        self.assertEqual((q['minimum'], q['maximum']), (353,357))
        self.assertEqual(q['P'], 10.76)

    def test_multihit_skill_uses_area_capacity_not_contact_count(self):
        sk = self.skill()
        sk.update(value_effect_layer=1, value_additional_damage=128,
                  value_power_per_level=14, value_native_pattern109=2,
                  value_maximum_grade=15)
        q = mid.current_prediction(dict(matk=238,atk=1104,element=None),
                                   dict(defense=33,magic_defense=39,element=2),sk,1)
        self.assertEqual(q['capacity'], 2)
        self.assertEqual((q['minimum'],q['maximum']), (752,756))

    def test_explicit_single_target_override_and_unknown_element(self):
        sk = self.skill()
        sk.update(value_native_pattern109=2,value_area_attack_present=True,value_area_attack=False)
        self.assertEqual(mid.capacity(sk,1),1)
        sk['value_element'] = 0
        with self.assertRaisesRegex(ValueError,'Actor element'):
            mid.current_prediction(dict(matk=238,element=None),dict(magic_defense=74,element=0),sk,1)


if __name__ == '__main__':
    unittest.main()

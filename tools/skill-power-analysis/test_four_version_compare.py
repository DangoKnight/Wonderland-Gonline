import unittest

import four_version_compare as compare


class FourVersionComparisonTests(unittest.TestCase):
    def skill(self):
        return dict(value_id=11166,value_element=3,value_effect_layer=2,
                    value_stat_multiplier=1.1,value_additional_damage=4,value_power_per_level=1.38)

    def row(self):
        return dict(actor=dict(element=3,matk=238,atk=1104),enemy=dict(element=0,level=23,defense=72,magic_defense=74),grade=1)

    def test_private_skill_ignores_basic_noise_and_uses_default_monster_defense(self):
        q=compare.private_prediction(self.row(),self.skill())
        self.assertEqual((q['minimum'],q['maximum'],q['defense']),(251,251,31))
        q=compare.private_prediction(self.row(),self.skill(),same_defense=True)
        self.assertEqual(q['midpoint'],230)

    def test_private_actor_and_corrected_skill_element_are_distinct(self):
        row=dict(actor=dict(element=1,matk=24),enemy=dict(element=4,level=12,magic_defense=44),grade=8)
        skill=dict(value_effect_layer=2,value_element=3,value_stat_multiplier=1.2,value_additional_damage=40,value_power_per_level=7.08)
        self.assertEqual(compare.private_prediction(row,skill)['midpoint'],69)
        self.assertEqual(compare.private_prediction(row,skill,use_skill_element=True)['midpoint'],174)

    def test_origin_caps_grade_at_ten_and_uses_reference_power(self):
        row=self.row()
        row['grade']=18
        q=compare.origin_prediction(row,self.skill(),{11166:405})
        self.assertEqual(q['power'],587.25)

    def test_updated_prediction_recalculates_instead_of_using_cached_damage(self):
        row = dict(actor=dict(element=2,matk=2346,atk=0),
                   enemy=dict(element=4,defense=330,magic_defense=360),grade=1,
                   selected=dict(minimum=999,maximum=999,midpoint=999))
        skill = dict(value_element=2,value_effect_layer=2,value_additional_damage=220,
                     value_power_per_level=0,value_maximum_grade_present=False,value_maximum_grade=0)
        q = compare.current_prediction(row, skill)
        self.assertEqual((q['minimum'],q['maximum']), (3035,3039))

    def test_updated_and_private_corrected_variants_resolve_neutral_and_caster_elements(self):
        row=self.row()
        row['enemy']['element']=4
        skill=self.skill()
        skill.update(value_element=0,value_maximum_grade_present=False,value_maximum_grade=0)
        neutral=compare.current_prediction(row,skill)
        self.assertEqual(compare.private_prediction(row,skill,use_skill_element=True)['element'],0)
        skill['value_element']=7
        inherited=compare.current_prediction(row,skill)
        self.assertGreater(inherited['minimum'],neutral['maximum'])
        self.assertEqual(compare.private_prediction(row,skill,use_skill_element=True)['element'],3)


if __name__=='__main__':
    unittest.main()

import unittest

import grade_search as grade


class GradePowerTests(unittest.TestCase):
    def test_recovers_known_base_and_growth_from_grouped_damage(self):
        skills, rows = {}, []
        for index, (h, d, g) in enumerate(((7, 2.8, 5), (14, 3.6, 10), (25, 4.4, 3), (16, 5.2, 2))):
            name = f'synthetic {index}'
            skills[name] = dict(value_additional_damage=h, value_power_per_level=d, value_stat_multiplier=.6)
            factor, attack, defense = 1.6, 77, 34
            observed = factor*(1.4*attack+2*h+.5*d*g-.98*defense)+3
            rows.append(dict(skill=name, skill_id=index, grade=g, observed=observed,
                             actor=dict(atk=attack), enemy=dict(defense=defense),
                             predicted={'current server': dict(element_factor=factor)}))
        a, b = grade.fit(rows, skills)
        self.assertAlmostEqual(a, 2, places=4)
        self.assertAlmostEqual(b, .5, places=4)

    def test_grade_growth_is_total_not_multiplied_by_contact_count(self):
        skills = {'synthetic': dict(value_additional_damage=14, value_power_per_level=4,
                                    value_stat_multiplier=.6, value_effect_layer=1,
                                    value_element=3, value_contact_events=3)}
        row = dict(skill='synthetic', grade=1, actor=dict(atk=100, element=3),
                   enemy=dict(defense=0, magic_defense=0, element=4))
        first = grade.estimate(row, skills, (2, .5))
        second = grade.estimate(dict(row, grade=2), skills, (2, .5))
        self.assertEqual(first['power'], 30)
        self.assertEqual(second['power']-first['power'], 2)


if __name__ == '__main__':
    unittest.main()

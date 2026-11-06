import unittest

import damage_polynomial as damage


class DamagePolynomialTests(unittest.TestCase):
    def setUp(self):
        self.current = dict(config=('current', 0, False), terms=[], coefficients=[])

    def test_independent_current_damage_example_and_magic_defense_policy(self):
        row = dict(O=25, B=39, P=13.52, E=.7, layer=2)
        self.assertEqual(damage.predict(row, self.current), dict(minimum=8, maximum=12, midpoint=10))
        row.update(physical_defense=9999)
        self.assertEqual(damage.predict(row, self.current)['midpoint'], 10)

    def test_p_is_additive_with_fixed_unit_coefficient(self):
        model = dict(config=('direct', 2, True), terms=[(), ('O',), ('B',), ('O', 'B')], coefficients=[3, 1.2, -.8, .001])
        row = dict(O=100, B=40, P=20, E=1.6, layer=2)
        before = damage.stat_prediction(row, model)
        first = damage.predict(row, model)
        row['P'] += 10
        self.assertEqual(damage.stat_prediction(row, model), before)
        self.assertEqual(damage.predict(row, model)['midpoint']-first['midpoint'], 16)

    def test_no_model_features_can_change_p_or_element(self):
        for config in damage.CONFIGURATIONS:
            row = dict(O=25, B=39, P=10, E=.7)
            terms = damage.terms_for(config)
            before = damage.features(row, config, terms)
            row.update(P=9999, E=1.6)
            self.assertEqual(damage.features(row, config, terms), before)

    def test_low_physical_branch_and_damage_bounds(self):
        row = dict(O=100, B=200, P=0, E=1, layer=1)
        self.assertAlmostEqual(damage.current_stat(row), 24.1)
        self.assertEqual(damage.predict(dict(row, P=-10000), self.current)['midpoint'], 4)
        self.assertEqual(damage.predict(dict(row, P=1e20), self.current)['maximum'], damage.MAX_DAMAGE)

    def test_entire_validation_group_is_held_out(self):
        rows = [dict(id=skill, target=target, O=offense) for skill in (1,2,3) for target in (7,8) for offense in (25,29)]
        for split in (damage.SKILL_SPLIT, damage.TARGET_SPLIT, damage.OFFENSE_SPLIT):
            tested = []
            for held, train, test in damage.folds(rows, split):
                self.assertTrue(all(damage.group_key(r, split)!=held for r in train))
                self.assertTrue(all(damage.group_key(r, split)==held for r in test))
                tested.extend(test)
            self.assertCountEqual(tested, rows)

    def test_recovers_quadratic_stat_remainder_on_unseen_input(self):
        rows = []
        for i, (offense, defense) in enumerate(( (o,b) for o in (10,30,60,100) for b in (5,15,25) )):
            stat = 1.3*offense-.9*defense+.002*offense*defense
            rows.append(dict(id=i, O=offense, B=defense, E=1, P=20, layer=2, observed=20+stat+3))
        model = damage.fit_path(rows, ('direct', 2, False))[0]
        self.assertAlmostEqual(damage.stat_prediction(dict(O=40, B=20, layer=2), model), 35.6, places=4)

    def test_monotonicity_diagnostics_detect_defense_increasing_damage(self):
        model = dict(config=('direct', 1, False), terms=[('O',), ('B',)], coefficients=[1, 1])
        d = damage.diagnostics([dict(O=25, B=39, layer=2)], model)
        self.assertGreater(d['broad_domain']['increasing_defense'], 0)


if __name__ == '__main__':
    unittest.main()

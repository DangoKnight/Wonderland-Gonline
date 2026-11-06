import unittest

import polynomial_search as poly


class PolynomialSearchTests(unittest.TestCase):
    def test_recovers_quadratic_on_unseen_inputs(self):
        rows = [dict(id=i, H=h, Z=z, reference=2*h+.4*z+.02*h*z+.01*z*z)
                for i, (h, z) in enumerate(( (h, z) for h in (2, 6, 12, 20) for z in (3, 9, 17) ))]
        model = poly.fit(rows, ('H', 'Z'), 2, 1e-12)
        self.assertAlmostEqual(poly.predict(dict(H=8, Z=13), model), 24.97, places=6)

    def test_polynomial_is_zero_without_native_power(self):
        for names in poly.BASES:
            terms = poly.monomials(names, 3)
            values = poly.features(dict(H=0, D=0, Z=0, G=10, S=1.5), terms)
            self.assertEqual(values, [0]*len(terms))

    def test_validation_holds_all_grades_of_a_skill(self):
        rows = [dict(id=skill, G=grade) for skill in range(9) for grade in (1, 2, 10)]
        tested = []
        for train, test in poly.folds(rows, poly.OUTER_FOLDS):
            self.assertFalse({r['id'] for r in train} & {r['id'] for r in test})
            tested.extend((r['id'], r['G']) for r in test)
        self.assertCountEqual(tested, [(r['id'], r['G']) for r in rows])

    def test_predictions_are_not_clamped_to_conceal_failures(self):
        model = dict(terms=[('H',)], coefficients=[-2])
        self.assertEqual(poly.predict(dict(H=3), model), -6)
        self.assertEqual(poly.metrics([dict(reference=2)], [-6])['negative_predictions'], 1)

    def test_grade_diagnostics_flag_invalid_extrapolation(self):
        model = dict(terms=[('H',), ('Z',)], coefficients=[1, -2])
        diagnostics = poly.grade_diagnostics([dict(id=5, H=1, D=2, G=3)], model)
        self.assertEqual(diagnostics['negative_skill_ids'], [5])
        self.assertEqual(diagnostics['decreasing_skill_ids'], [5])

    def test_ridge_path_does_not_accumulate_regularization(self):
        rows = [dict(id=i, H=i+1, Z=(i+2)**2, reference=2*(i+1)+.4*(i+2)**2)
                for i in range(10)]
        forward = poly.fit_path(rows, ('H', 'Z'), 2, (.001, .03, .3))
        backward = poly.fit_path(rows, ('H', 'Z'), 2, (.3, .03, .001))
        for a, b in zip(forward, reversed(backward)):
            self.assertEqual(a, b)

    def test_recovers_quartic_on_unseen_inputs(self):
        rows = [dict(id=h*10+z, H=h, Z=z,
                     reference=1.2*h+.8*z+.01*h*h*z*z+.002*z**4)
                for h in range(1, 7) for z in range(1, 7)]
        model = poly.fit(rows, ('H', 'Z'), 4, 1e-12)
        self.assertAlmostEqual(poly.predict(dict(H=2.5, Z=3.5), model), 6.86575, places=5)


if __name__ == '__main__':
    unittest.main()

import unittest

import structured_search as analysis


class PositiveFitTests(unittest.TestCase):
    def test_independent_solution_and_active_boundary(self):
        # X'X = identity; positive solution should be (2,3).
        result = analysis.positive_fit([[1., 0.], [0., 1.]], [2., 3.])
        self.assertAlmostEqual(result[0], 2., places=5)
        self.assertAlmostEqual(result[1], 3., places=5)
        # Unconstrained optimum is (1,-1); the nonnegative optimum is (.5,0).
        result = analysis.positive_fit([[2., 1.], [1., 2.]], [1., -1.])
        self.assertAlmostEqual(result[0], .5, places=5)
        self.assertEqual(result[1], 0.)

    def test_zero_and_singular_inputs(self):
        self.assertEqual(analysis.positive_fit([[0., 0.], [0., 0.]], [0., 0.]), [0., 0.])
        result = analysis.positive_fit([[1., 1.], [1., 1.]], [2., 2.])
        self.assertTrue(all(c >= 0 for c in result))
        self.assertAlmostEqual(sum(result), 2., places=5)

    def test_known_power_relation_without_identity_predictors(self):
        # Independent synthetic observations of P = 2H+3D.
        rows = [dict(features=[h, d], reference=2*h+3*d, x={'40': .6})
                for h, d in ((1, 0), (0, 1), (2, 3), (9, 2), (4, 8))]
        result = analysis.search(rows, maximum_terms=2, split_modes=(False,))[0]
        for row in rows:
            self.assertAlmostEqual(analysis.predict(row, result), row['reference'], places=3)


if __name__ == '__main__':
    unittest.main()

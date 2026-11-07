import unittest

import raw_search


class AnalysisArithmeticTests(unittest.TestCase):
    def test_solver_independent_system(self):
        # 2x+y=5; x+3y=5 gives x=2,y=1.
        actual = raw_search.solve([[2., 1.], [1., 3.]], [5., 5.])
        self.assertAlmostEqual(actual[0], 2.)
        self.assertAlmostEqual(actual[1], 1.)
        self.assertIsNone(raw_search.solve([[1., 2.], [2., 4.]], [3., 6.]))

    def test_relative_fit_recovers_known_scale(self):
        rows = [dict(features=[x], reference=2*x) for x in (1., 3., 8.)]
        matrix, target = raw_search.normal(rows, [0])
        self.assertAlmostEqual(raw_search.solve(matrix, target)[0], 2.)

    def test_reference_links_stay_together_without_using_ids_as_features(self):
        records = {i: bytearray(148) for i in (100, 101, 102, 200)}
        records[101][54:56] = bytes([100, 0])
        records[102][102:104] = bytes([101, 0])
        groups = raw_search.families(records)
        self.assertEqual(groups[100], groups[102])
        self.assertNotEqual(groups[100], groups[200])


if __name__ == '__main__':
    unittest.main()

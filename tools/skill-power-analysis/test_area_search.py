import unittest

import area_search as area


class AreaEvidenceTests(unittest.TestCase):
    def test_explicit_counts_and_shapes(self):
        for text, expected in [('1', 1), ('横2', 2), ('縦3', 3), ('十字4 (Lv9～18)', 4),
                               ('縦3体*横2体', 6), ('横２×縦３', 6), ('', None)]:
            with self.subTest(text=text):
                self.assertEqual(area.target_capacity(text), expected)
        with self.assertRaises(ValueError):
            area.target_capacity('unknown')

    def test_grade_band_selects_capacity_without_raw_shape(self):
        self.assertEqual(area.grade_band('Lv9～18'), (9, 18))
        self.assertEqual(area.grade_band('1〜8'), (1, 8))
        candidates = [dict(capacity=1, grade_band=(1, 8)), dict(capacity=4, grade_band=(9, 18))]
        self.assertEqual(area.select_target(candidates, 8)[0], 1)
        self.assertEqual(area.select_target(candidates, 9)[0], 4)
        with self.assertRaises(ValueError):
            area.select_target(candidates, 19)
        with self.assertRaises(ValueError):
            area.select_target([dict(capacity=1, grade_band=None), dict(capacity=2, grade_band=None)], 10)

    def test_known_synthetic_penalty(self):
        rows = [dict(targets=n, base=120., reference=120./(1+.75*(n-1))) for n in (2, 3, 4, 6)]
        model = area.fit_curve(rows, 'reciprocal')
        self.assertEqual(model['parameter'], .75)
        self.assertLess(model['metrics']['mean_percent_error'], 1e-10)
        self.assertEqual(area.attenuation(1, 'reciprocal', .75), 1)

    def test_blank_target_is_not_single(self):
        selected, _ = area.select_target([dict(capacity=None, grade_band=None)], 1)
        self.assertIsNone(selected)


if __name__ == '__main__':
    unittest.main()

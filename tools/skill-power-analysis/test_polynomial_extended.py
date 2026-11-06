import unittest

import polynomial_extended as extended
import polynomial_search as poly


class ExtendedPolynomialTests(unittest.TestCase):
    def test_degree_six_and_new_native_fields_are_present(self):
        self.assertEqual(len(extended.CONFIGURATIONS), len({extended.key(c) for c in extended.CONFIGURATIONS}))
        self.assertIn((('H', 'Z'), 6), extended.CONFIGURATIONS)
        self.assertTrue(any('SP' in names for names, _ in extended.CONFIGURATIONS))
        self.assertTrue(any('T' in names for names, _ in extended.CONFIGURATIONS))
        for names, degree in extended.CONFIGURATIONS:
            row = dict(H=0, D=0, Z=0, S=1.1, T=7, SP=20, G=10)
            self.assertTrue(all(v==0 for v in poly.features(row, poly.monomials(names, degree))))

    def test_extra_fields_use_raw_decoded_offsets(self):
        record = bytearray(32)
        record[24:26] = bytes([44, 1])
        record[30] = 7
        rows = [dict(id=99)]
        extended.augment(rows, {99: record})
        self.assertEqual(rows[0], dict(id=99, SP=300, T=7))

    def test_inner_selection_produces_finite_models(self):
        rows = [dict(id=i, H=i+1, Z=(i+2)**2, reference=2*(i+1)+.4*(i+2)**2)
                for i in range(9)]
        choices = extended.choose_candidates(rows, [(('H', 'Z'), 1), (('H', 'Z'), 2)])
        self.assertEqual(len(choices), 2)
        self.assertLess(choices[0]['inner_error'], .001)
        model = extended.train_candidate(rows, choices[0])
        self.assertAlmostEqual(poly.predict(dict(H=4, Z=25), model), 18, places=4)


if __name__ == '__main__':
    unittest.main()

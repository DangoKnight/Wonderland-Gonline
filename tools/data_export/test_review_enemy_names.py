import unittest

import review_enemy_names as review


class EnemyReviewTest(unittest.TestCase):
    def test_false_prefix_spelling_context_and_missing_reference_stay_unchanged(self):
        def native(identity, name):
            fields = dict(id=identity, type=7, level=10, element=1, hp=50, sp=20,
                          strength=1, constitution=2, intelligence=3, wisdom=4, agility=5)
            fields.update({k: 0 for k in (review.SPRITE_FIELD, review.ICON_FIELD, *review.PALETTE_FIELDS)})
            return dict(fields=fields, name=dict(text=name))
        def source(name):
            return dict(name=name, lvl='10', element='1', hp='50', sp='20', str='1', con='2', int='3', wis='4', agi='5', source_id='1')
        native_rows = [native(1, 'Bear'), native(2, 'Orangutan'), native(3, 'Tower'), native(4, 'Unknown Na'), native(5, 'Pig')]
        rows = review.review(native_rows, [source('Beard the Pirate'), source('Orangutang'), source('Tower (Trojan War)'), source('Pig'), source('Pig Lord')], set(), {})
        decisions = {r['id']: (r['decision'], r['display_name']) for r in rows}
        self.assertEqual(decisions, {1: ('unresolved', 'Bear'), 2: ('preserve spelling', 'Orangutan'), 3: ('preserve context-free name', 'Tower'), 4: ('unresolved', 'Unknown Na'), 5: ('already complete', 'Pig')})


if __name__ == '__main__':
    unittest.main()

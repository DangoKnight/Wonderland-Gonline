import unittest
import tempfile
from pathlib import Path

from extract_wiki_quests import notepad_index, event_references, completion_candidates, reward_bundle_candidates, actor_candidates, write_notepad_document


class WikiQuestInventoryTest(unittest.TestCase):
    def test_index_preserves_unlinked_entries_and_link_display_aliases(self):
        body = '''<b>2 Quests Total</b><table><tr><th>Page 1</th></tr>
        <tr><td><a href="/wiki/A_Big_Bear_Mission" title="A Big Bear Mission">A Cave Bear Mission</a></td></tr>
        <tr><td>Unlinked quest</td></tr></table>'''
        rows = notepad_index(dict(parse=dict(text={'*': body})))
        self.assertEqual([(r['notepad_page'], r['slot'], r['name'], r['wiki_title']) for r in rows],
                         [(1, 1, 'A Cave Bear Mission', 'A Big Bear Mission'), (1, 2, 'Unlinked quest', 'Unlinked quest')])
        with self.assertRaisesRegex(ValueError, 'incomplete'):
            notepad_index(dict(parse=dict(text=body.replace('2 Quests', '3 Quests'))))

    def test_native_linking_uses_mark_operands_and_preserves_multiple_maps(self):
        condition = [5, 200, 50] + [0] * 18
        action = [5, 201, 50] + [0] * 18
        event = dict(ClickID=35, Branches=[dict(Condition=condition, Operations=[dict(Data=action)])])
        linked = event_references(dict(maps={'12050': dict(events=[event]), '12051': dict(events=[event])}))
        self.assertEqual(linked[13000], {(12050, 35), (12051, 35)})
        self.assertEqual(linked[13001], linked[13000])
        self.assertFalse(linked[9999])

    def test_completion_candidates_require_quest_gate_and_supported_entire_tail(self):
        def operation(code, first, second, third=0, value=0):
            raw = [code] + list(first.to_bytes(2, 'little')) + list(second.to_bytes(2, 'little')) + list(third.to_bytes(2, 'little')) + [0] + list(value.to_bytes(4, 'little')) + [0] * 9
            return dict(Data=raw)
        # The cost precedes the receipt: unsigned bytes must preserve -1.
        ops = [operation(2, 1, 1, 10001), operation(1, 1, 1, 30023, 0xffffffff),
               operation(5, 13000, 2), operation(1, 1, 1, 34058, 1)]
        branch = dict(Index=11, Condition=[5] + [0] * 20, Operations=ops)
        catalog = dict(items={'30023': {}, '34058': {}}, maps={'12050': dict(events=[dict(ClickID=35, Branches=[branch])])})
        rows = [dict(native_marks='13000;13001', notepad_page=2, slot=1)]
        rules, evidence = completion_candidates(catalog, rows, {(12050, 35, 11): 'soup'})
        self.assertEqual((rules[0]['id'], rules[0]['value'], rules[0]['pattern']['value']), ('soup', 1, 0xffffffff))
        self.assertEqual(evidence[0]['notepad_positions'], '2:1')
        # An unsupported operation after the receipt invalidates the reward tail.
        ops.append(operation(3, 99, 1))
        self.assertEqual(completion_candidates(catalog, rows, {})[0], [])
        ops.pop()
        branch['Condition'][0] = 7
        self.assertEqual(completion_candidates(catalog, rows, {})[0], [])

    def test_document_preserves_slots_and_escapes_table_labels(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'quests.md'
            write_notepad_document(path, [dict(notepad_page=1, slot=2, name='A | B', source_url='https://example.com/(quest)', native_marks='1;2')])
            result = path.read_text(encoding='utf-8')
            self.assertIn('| 2 | [A \\| B](https://example.com/%28quest%29) | 1, 2 |', result)

    def test_reward_bundle_extends_existing_tail_without_losing_parent_identity(self):
        def operation(code, first, second, third, value):
            return dict(Data=[code] + list(first.to_bytes(2, 'little')) + list(second.to_bytes(2, 'little')) + list(third.to_bytes(2, 'little')) + [0] + list(value.to_bytes(4, 'little')) + [0] * 9)
        ops = [operation(3, 1, 12095, 1, 0), operation(5, 13172, 2, 1, 0), operation(1, 1, 1, 34064, 1)]
        branch = dict(Index=20, Condition=[5] + [0] * 20, Operations=ops)
        catalog = dict(npcs={'12095': {}}, skills={}, items={'34064': {}}, maps={'11149': dict(events=[dict(ClickID=2, Branches=[branch])])})
        rows = [dict(native_marks='13172', notepad_page=1, slot=1)]
        rules, parents, evidence = reward_bundle_candidates(catalog, rows, {(11149, 2, 20): 'historical_parent'})
        self.assertEqual((rules[0]['value'], rules[0]['kind'], evidence[0]['tail_index']), (0, 'quest_reward_bundle', 0))
        self.assertEqual(parents, {'wiki_quest_bundle_11149_2_20': 'historical_parent'})
        # A changed companion compatibility operand must remain outside the bundle.
        ops[0] = operation(3, 1, 12095, 2, 0)
        self.assertEqual(reward_bundle_candidates(catalog, rows, {})[0], [])

    def test_actor_candidates_do_not_restore_historical_capabilities(self):
        raw = [2, 1, 0, 8, 0, 2, 0] + [0] * 14
        catalog = dict(maps={'12050': dict(events=[dict(ClickID=35, Branches=[dict(Index=1, Operations=[dict(Index=1, Data=raw)])])])})
        rows = [dict(native_events='12050:35')]
        rules, evidence = actor_candidates(catalog, rows, [])
        self.assertEqual((rules[0]['pattern']['d2'], evidence[0]['operand']), (8, 2))
        historical = [dict(kind='actor_capability', scope=dict(maps=[12050]), pattern=dict(code=2, d2=8))]
        self.assertEqual(actor_candidates(catalog, rows, historical)[0], [])


if __name__ == '__main__':
    unittest.main()
